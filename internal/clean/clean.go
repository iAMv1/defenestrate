// Package clean scans rule targets, reports sizes by category and recycles
// what was confirmed. Mole-style output: one ✓ line per category.
package clean

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/iAMv1/defenestrate/internal/rules"
	"github.com/iAMv1/defenestrate/internal/safety"
	"github.com/iAMv1/defenestrate/internal/ui"
	"github.com/iAMv1/defenestrate/internal/whitelist"
)
// Finding is one cleanable category with everything it would remove.
type Finding struct {
	Target rules.Target
	Paths  []string // concrete files/dirs to recycle
	Bytes  int64
	// HeldPaths: candidates we cannot classify (executables, installers,
	// unknown formats). NEVER recycled by clean — shown for manual review.
	HeldPaths []string
	HeldBytes int64
}

// Run implements `DEFENESTRATE clean [--dry-run] [--yes]`.
func Run(args []string) error {
	fs_ := flag.NewFlagSet("clean", flag.ContinueOnError)
	yes := fs_.Bool("y", false, "do not ask for confirmation")
	wlMode := fs_.Bool("whitelist", false, "manage protected targets instead of cleaning")
	if err := fs_.Parse(args); err != nil {
		return err
	}
	if *wlMode {
		return whitelist.Run(fs_.Args())
	}

	fmt.Println(ui.Title("DEFENESTRATE deep clean") + ui.If(safety.DryRun(), ui.Warn("  [DRY RUN]")))
	fmt.Println(ui.Dim("Scanning cleanable locations…"))

	findings, skipped, err := Scan(rules.Targets)
	if err != nil {
		return err
	}

	var total int64
	for _, f := range findings {
		total += f.Bytes
	}
	if total == 0 {
		fmt.Println(ui.Good("Nothing to clean — already tidy."))
		return nil
	}

	lastCat := ""
	for _, f := range findings {
		if f.Target.Category != lastCat {
			fmt.Println(ui.Section(f.Target.Category))
			lastCat = f.Target.Category
		}
		note := ""
		if len(skipped[f.Target.Label]) > 0 {
			note = ui.Dim("  (skipped: " + strings.Join(unique(skipped[f.Target.Label]), ", ") + ")")
		}
		fmt.Printf("  %s %-42s %s%s\n",
			ui.Check, f.Target.Label, ui.HumanBytes(f.Bytes), note)
		if len(f.HeldPaths) > 0 {
			fmt.Printf("    %s %-40s %s\n",
				ui.Warn("⚠"), ui.Dim(strings.ToLower(f.Target.Label)+" held for review"),
				ui.Dim(ui.HumanBytes(f.HeldBytes)))
		}
	}
	fmt.Println(ui.Rule())
	fmt.Printf("%s | free now: %s\n",
		ui.Bold(fmt.Sprintf("Space reclaimable: %s", ui.HumanBytes(total))),
		ui.HumanBytesU(freeOnSystemDrive()))

	var heldCount int
	var heldBytes int64
	for _, f := range findings {
		heldCount += len(f.HeldPaths)
		heldBytes += f.HeldBytes
	}
	if heldCount > 0 {
		fmt.Println(ui.Warn(fmt.Sprintf(
			"Held for review: %d items (%s) — executables, installers and unknown formats are never auto-deleted. Use `DEFENESTRATE analyze --delete` on specific paths.",
			heldCount, ui.HumanBytes(heldBytes))))
	}

	if safety.DryRun() {
		fmt.Println(ui.Warn("\nDry run complete — nothing was deleted. Re-run without --dry-run to clean."))
		for _, f := range findings {
			for _, p := range f.Paths {
				safety.Logf("[dry-run] would recycle", p, 0)
			}
		}
		return nil
	}
	if !*yes && !confirm(fmt.Sprintf("Recycle %s?", ui.HumanBytes(total))) {
		fmt.Println(ui.Dim("Cancelled."))
		return nil
	}

	recycled := int64(0)
	for _, f := range findings {
		if err := safety.Recycle(f.Paths); err != nil {
			fmt.Println(ui.Bad("error:", err))
			continue
		}
		recycled += f.Bytes
	}
	fmt.Println(ui.Rule())
	fmt.Println(ui.Good(fmt.Sprintf("Space freed: %s (to Recycle Bin)", ui.HumanBytes(recycled))))
	return nil
}

// processProbe is overridable in tests to simulate the tri-state probe
// (including the "cannot enumerate processes" denial path).
var processProbe = ui.RunningAnySafe

// decideSkip returns a skip reason when the target must be skipped, or ""
// when scanning may proceed.
func decideSkip(t rules.Target) string {
	if t.SkipIfRunning == nil {
		return ""
	}
	hits, ok := processProbe(t.SkipIfRunning)
	if !ok {
		return "process state unknown" // tri-state: unknown denies
	}
	if len(hits) > 0 {
		return t.SkipIfRunning[0] + " running"
	}
	return ""
}

// Scan evaluates every target: existence, running processes, age filter,
// glob patterns, and the user whitelist. Returns findings sorted by category
// and per-target skip notes.
func Scan(targets []rules.Target) ([]Finding, map[string][]string, error) {
	var findings []Finding
	skipped := map[string][]string{}
	wl := whitelist.Load()
	for _, t := range targets {
		dir := t.Path()
		if dir == "" {
			continue
		}
		if reason := decideSkip(t); reason != "" {
			skipped[t.Label] = append(skipped[t.Label], reason)
			continue
		}
		if whitelist.Matches(t.Label, []string{dir}, wl) {
			skipped[t.Label] = append(skipped[t.Label], "whitelisted")
			continue
		}
		f, err := evalTarget(t, dir)
		if err != nil {
			skipped[t.Label] = append(skipped[t.Label], err.Error())
			continue
		}
		if len(f.Paths) > 0 {
			findings = append(findings, f)
		}
	}
	sortFindings(findings)
	return findings, skipped, nil
}

func evalTarget(t rules.Target, dir string) (Finding, error) {
	f := Finding{Target: t}
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return f, fmt.Errorf("not present")
	}
	cutoff := timeNow().AddDate(0, 0, -t.MinAgeDays)

	addPath := func(p string, isDir bool, size int64) {
		// "Don't delete what you don't know": exact-pattern matches are known
		// by construction (the rule named them). Broad rules classify every
		// file; executables/installers/unknown formats are HELD for review —
		// unless the rule cites an owner-documented reset contract
		// (TrustContents), which is the only sanctioned exception.
		trusted := t.TrustContents
		if !trusted && !isDir {
			switch safety.ClassifyFile(p) {
			case safety.ClassExecutable, safety.ClassUnknown:
				f.HeldPaths = append(f.HeldPaths, p)
				f.HeldBytes += size
				return
			}
		} else if !trusted && isDir && unknownInside(p) {
			f.HeldPaths = append(f.HeldPaths, p)
			f.HeldBytes += size
			return
		}
		f.Paths = append(f.Paths, p)
		f.Bytes += size
	}

	if t.Patterns == nil && t.MinAgeDays == 0 {
		// Whole-contents mode: every child entry is a candidate.
		ents, err := os.ReadDir(dir)
		if err != nil {
			return f, err
		}
		for _, e := range ents {
			p := filepath.Join(dir, e.Name())
			size := sizeOf(p, e)
			addPath(p, e.IsDir(), size)
		}
		return f, nil
	}

	if t.Patterns == nil {
		// Contents mode WITH an age floor: only entries old enough qualify.
		ents, err := os.ReadDir(dir)
		if err != nil {
			return f, err
		}
		for _, e := range ents {
			info, ierr := e.Info()
			if ierr != nil {
				continue
			}
			if !e.IsDir() && info.ModTime().After(cutoff) {
				continue // too young to touch
			}
			p := filepath.Join(dir, e.Name())
			addPath(p, e.IsDir(), sizeOf(p, e))
		}
		return f, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return f, err
	}
	var matches []string
	for _, e := range entries {
		for _, pat := range t.Patterns {
			ok, err := filepath.Match(pat, e.Name())
			if err == nil && ok {
				matches = append(matches, filepath.Join(dir, e.Name()))
				break
			}
		}
	}
	_ = matches
	// Pattern matching must also reach nested globs like "*\Cache": walk once.
	if hasNestedGlob(t.Patterns) {
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || p == dir {
				return nil
			}
			rel, rerr := filepath.Rel(dir, p)
			if rerr != nil {
				return nil
			}
			for _, pat := range t.Patterns {
				if ok, _ := filepath.Match(pat, rel); ok {
					info, ierr := d.Info()
					var sz int64
					if ierr == nil {
						sz = info.Size()
					}
					if !d.IsDir() && t.MinAgeDays > 0 {
						if ii, ierr2 := d.Info(); ierr2 == nil && ii.ModTime().After(cutoff) {
							return nil // too young
						}
					} else if d.IsDir() && t.MinAgeDays > 0 {
						return filepath.SkipDir // age-gated dirs skipped wholesale
					}
					addPath(p, d.IsDir(), sz)
					if d.IsDir() {
						return filepath.SkipDir // counted whole; don't double-count children
					}
					return nil
				}
			}
			return nil
		})
		return f, nil
	}
	// Flat patterns against direct children.
	for _, e := range entries {
		matched := false
		for _, pat := range t.Patterns {
			if ok, _ := filepath.Match(pat, e.Name()); ok {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		p := filepath.Join(dir, e.Name())
		size := sizeOf(p, e)
		if !e.IsDir() && t.MinAgeDays > 0 {
			if info, ierr := e.Info(); ierr == nil && info.ModTime().After(cutoff) {
				continue
			}
		}
		addPath(p, e.IsDir(), size)
	}
	return f, nil
}

func hasNestedGlob(pats []string) bool {
	for _, p := range pats {
		if strings.Contains(p, string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// unknownInside reports whether a directory directly contains any file we
// cannot classify as known junk (executable, installer, unknown format).
// Top-level peek only — cheap, and conservative in the right direction.
func unknownInside(dir string) bool {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return true // can't see inside = don't touch
	}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		if safety.ClassifyFile(filepath.Join(dir, e.Name())) != safety.ClassKnownJunk {
			return true
		}
	}
	return false
}

func sizeOf(p string, e os.DirEntry) int64 {
	if !e.IsDir() {
		if info, err := e.Info(); err == nil {
			return info.Size()
		}
		return 0
	}
	var total int64
	_ = filepath.WalkDir(p, func(fp string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && fp != p && safety.IsReparsePoint(fp) {
			return filepath.SkipDir // never descend through links/junctions
		}
		if !d.IsDir() {
			if info, iErr := d.Info(); iErr == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

func sortFindings(fs []Finding) {
	for i := 1; i < len(fs); i++ {
		for j := i; j > 0 && fs[j].Target.Category < fs[j-1].Target.Category; j-- {
			fs[j], fs[j-1] = fs[j-1], fs[j]
		}
	}
}

// freeOnSystemDrive reports free bytes on %SystemDrive% (default C:).
func freeOnSystemDrive() uint64 {
	drive := os.Getenv("SystemDrive")
	if drive == "" {
		drive = "C:"
	}
	return ui.FreeBytes(drive + `\`)
}

func unique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func maxInt(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
