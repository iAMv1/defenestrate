// Package purge finds and removes rebuildable project build artifacts
// (node_modules, target, .next, …) — but ONLY inside directories that carry
// project-marker evidence (.git, package.json, go.mod, Cargo.toml, pyproject).
//
// Mole's hard-won rules adopted verbatim:
//   - Git-worktree staleness is undecidable → never delete the project itself,
//     never emit a "safe to delete" verdict about it.
//   - Only exact artifact directory names inside marker-bearing projects count.
//   - Projects touched within the last 7 days are flagged "recent" and are
//     unchecked by default.
package purge

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/iAMv1/defenestrate/internal/safety"
	"github.com/iAMv1/defenestrate/internal/ui"
)

// Artifacts is the data-only list of rebuildable artifact directory names.
// Adding coverage = adding a name here. Nothing generic like "build"/"dist"
// is accepted WITHOUT a project marker in the parent.
var Artifacts = []string{
	"node_modules",          // npm/yarn/pnpm — `npm ci` restores
	"target",                // cargo build output
	".next",                 // next.js build
	".nuxt",                 // nuxt build
	"__pycache__",           // python bytecode
	".pytest_cache",         // pytest cache
	".mypy_cache",           // mypy cache
	".ruff_cache",           // ruff cache
	"bin",                   // dotnet/go per-project (marker-gated)
	"obj",                   // dotnet intermediate
	".gradle",               // gradle project cache
	"cmake-build-debug",     // clion
	"cmake-build-release",   // clion
}

// markers prove a directory is a real project root.
var markers = []string{
	".git", "package.json", "go.mod", "Cargo.toml",
	"pyproject.toml", "requirements.txt", "*.csproj", "*.sln",
	"pom.xml", "build.gradle",
}

const recentDays = 7

// Finding is one discovered artifact.
type Finding struct {
	Project  string // containing project root
	Name     string // artifact dir name
	Path     string
	Bytes    int64
	Recent   bool // modified within recentDays — unchecked by default
	TimedOut bool // true on every finding when the walk hit its deadline
}

// ScanConfig controls discovery.
type ScanConfig struct {
	Roots    []string
	MaxDepth int
	Now      time.Time
	// Deadline bounds the walk. On expiry Scan returns partial findings —
	// a timed-out producer must never masquerade as a complete scan.
	Deadline time.Time
}

// Scan walks roots looking for artifacts inside marker-bearing projects.
func Scan(cfg ScanConfig) ([]Finding, error) {
	now := cfg.Now.Unix()
	var out []Finding
	seen := map[string]bool{}
	timedOut := false

	for _, root := range cfg.Roots {
		if !cfg.Deadline.IsZero() && time.Now().After(cfg.Deadline) {
			timedOut = true
			break
		}
		st, err := os.Stat(root)
		if err != nil || !st.IsDir() {
			continue
		}
		filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !cfg.Deadline.IsZero() && time.Now().After(cfg.Deadline) {
				timedOut = true
				return filepath.SkipAll
			}
			if !d.IsDir() {
				return nil
			}
			depth := strings.Count(strings.TrimPrefix(p, root), string(filepath.Separator))
			if depth > cfg.MaxDepth {
				return filepath.SkipDir
			}
			base := d.Name()
			lower := strings.ToLower(base)
			isArtifact := false
			for _, a := range Artifacts {
				if lower == a {
					isArtifact = true
					break
				}
			}
			if isArtifact {
				project := findProjectMarker(filepath.Dir(p))
				if project == "" {
					return nil // no marker evidence: skip silently
				}
				key := strings.ToLower(p)
				if seen[key] {
					return filepath.SkipDir // nested artifact inside artifact
				}
				seen[key] = true
				info, ierr := d.Info()
				bytesVal := dirBytes(p, cfg.Deadline)
				recent := false
				if ierr == nil && now-info.ModTime().Unix() < recentDays*86400 {
					recent = true
				}
				out = append(out, Finding{Project: project, Name: base, Path: p, Bytes: bytesVal, Recent: recent})
				return filepath.SkipDir // never recurse into the artifact
			}
			// Dot-directory containers stay unexplored unless they ARE roots.
			if strings.HasPrefix(base, ".") && p != root {
				return filepath.SkipDir
			}
			return nil
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Bytes > out[j].Bytes })
	if timedOut {
		for i := range out {
			out[i].TimedOut = true
		}
	}
	return out, nil
}

// findProjectMarker walks UP from dir looking for a project marker file;
// returns the project root path or "".
func findProjectMarker(dir string) string {
	cur := dir
	for i := 0; i < 3; i++ { // bounded: artifact's parent + two levels up
		for _, m := range markers {
			if strings.ContainsAny(m, "*") {
				if matches, _ := filepath.Glob(filepath.Join(cur, m)); len(matches) > 0 {
					return cur
				}
				continue
			}
			if _, err := os.Stat(filepath.Join(cur, m)); err == nil {
				return cur
			}
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return ""
}

// dirBytes sizes one artifact tree, bounded by the scan deadline so a huge
// node_modules can't blow the wall-clock budget on its own.
func dirBytes(p string, deadline time.Time) int64 {
	var total int64
	filepath.WalkDir(p, func(_ string, d os.DirEntry, err error) error {
		if !deadline.IsZero() && time.Now().After(deadline) {
			return filepath.SkipAll
		}
		if err == nil && !d.IsDir() {
			if info, e2 := d.Info(); e2 == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

// DefaultRoots seeds sensible scan roots from env only.
func DefaultRoots() []string {
	var out []string
	home := os.Getenv("USERPROFILE")
	for _, sub := range []string{"Dev", "Projects", "repos", "src"} {
		if home != "" {
			p := filepath.Join(home, sub)
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				out = append(out, p)
			}
		}
	}
	if len(out) == 0 && home != "" {
		out = append(out, home)
	}
	return out
}

// ConfigPath returns the user-editable scan-roots file location.
func ConfigPath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
	}
	return filepath.Join(base, "DEFENESTRATE", "purge-paths.txt")
}

// LoadRoots reads ConfigPath if present; falls back to DefaultRoots.
func LoadRoots() []string {
	b, err := os.ReadFile(ConfigPath())
	if err != nil {
		return DefaultRoots()
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if st, err := os.Stat(line); err == nil && st.IsDir() {
			out = append(out, line)
		}
	}
	if len(out) == 0 {
		return DefaultRoots()
	}
	return out
}

// Run implements `DEFENESTRATE purge [--dry-run] [--all] [--paths]`.
func Run(args []string) error {
	fsPkg := flag.NewFlagSet("purge", flag.ContinueOnError)
	all := fsPkg.Bool("all", false, "include recent (<7d) projects")
	showPaths := fsPkg.Bool("paths", false, "print the scan-roots config path and exit")
	yesFlagLocal := fsPkg.Bool("y", false, "skip confirmation")
	if err := fsPkg.Parse(args); err != nil {
		return err
	}
	if *showPaths {
		fmt.Println(ui.StyleDim.Render("scan roots file: " + ConfigPath()))
		fmt.Println(ui.StyleDim.Render("one directory per line · # comments · empty = defaults"))
		for _, r := range LoadRoots() {
			fmt.Println("  " + r)
		}
		return nil
	}

	fmt.Println(ui.Title("DEFENESTRATE purge") + ui.If(safety.DryRun(), ui.Warn("  [DRY RUN]")))
	cfg := ScanConfig{Roots: LoadRoots(), MaxDepth: 4, Now: time.Now(),
		Deadline: time.Now().Add(90 * time.Second)}
	findings, err := Scan(cfg)
	if err != nil {
		return err
	}
	if len(findings) > 0 && findings[0].TimedOut {
		fmt.Println(ui.StyleWarn.Render("⚠ scan hit its 90s budget — results are PARTIAL. Widen roots or re-run to continue."))
	}

	var totalSel, totalAll int64
	for _, f := range findings {
		totalAll += f.Bytes
	}
	fmt.Println(ui.Section(fmt.Sprintf("%d build artifacts found (%s total)", len(findings), ui.HumanBytes(totalAll))))

	var selected []*Finding
	for i := range findings {
		f := findings[i]
		tag := ""
		check := ui.Check
		use := true
		if f.Recent && !*all {
			tag = ui.StyleWarn.Render("  recent")
			use = false
		}
		if use {
			totalSel += f.Bytes
			selected = append(selected, &findings[i])
		} else {
			check = ui.StyleDim.Render("○")
		}
		proj := filepath.Base(f.Project)
		fmt.Printf("  %s %-28s %-14s %10s%s\n",
			check, truncateP(f.Name+" @ "+proj, 42), "", ui.HumanBytes(f.Bytes), tag)
		_ = totalSel
	}

	if len(findings) == 0 {
		fmt.Println(ui.Good("\nNo build artifacts found."))
		return nil
	}
	fmt.Println(ui.Rule())
	fmt.Printf("%s | held: recent items re-run with --all\n",
		ui.Bold(fmt.Sprintf("Selected: %s", ui.HumanBytes(totalSel))))

	if safety.DryRun() {
		fmt.Println(ui.Warn("\nDry run — nothing deleted."))
		for _, s := range selected {
			safety.Logf("[dry-run] would recycle", s.Path, s.Bytes)
		}
		return nil
	}
	if len(selected) == 0 {
		fmt.Println(ui.Dim("\nNothing selected (only recent projects found). Use --all to include them."))
		return nil
	}
	if !*yesFlagLocal && !confirmP(fmt.Sprintf("Recycle %s of build artifacts?", ui.HumanBytes(totalSel))) {
		fmt.Println(ui.Dim("Cancelled."))
		return nil
	}

	var freed int64
	for _, s := range selected {
		if err := safety.Recycle([]string{s.Path}); err != nil {
			fmt.Println(ui.Bad("skip:", s.Path, "-", err))
			continue
		}
		freed += s.Bytes
		safety.Logf("purge-recycle", s.Path, s.Bytes)
		fmt.Println(ui.Check + " " + s.Path)
	}
	fmt.Println(ui.Rule())
	fmt.Println(ui.Good(fmt.Sprintf("Space freed: %s (Recycle Bin)", ui.HumanBytes(freed))))
	return nil
}
