// Package apps implements the smart uninstaller: enumerate installed
// programs from the registry, run their quiet uninstaller when available,
// then sweep leftover folders/registry keys by name tokens.
package apps

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/windows/registry"

	"github.com/iAMv1/defenestrate/internal/safety"
	"github.com/iAMv1/defenestrate/internal/ui"
)

// App is one installed program found in an Uninstall registry key.
type App struct {
	Name            string
	Version         string
	Publisher       string
	Uninstall       string // full command line (may need wrapping)
	Quiet           string // silent uninstall command if advertised
	InstallLoc      string
	DisplayIcon     string // icon resource path; its directory is install evidence
	SizeKB          uint64
	KeyPath         string // for diagnostics only
	SubKeyName      string // exact uninstall subkey name (registry evidence)
	Store           bool   // UWP/Store app (Remove-AppxPackage, no folder sweep)
	PackageFullName string // Store apps only
}

// uninstallHives covers 64-bit, 32-bit and per-user installs.
var uninstallHives = []struct {
	root registry.Key
	path string
}{
	{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
	{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`},
	{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
}

// ListInstalled enumerates every visible installed program.
func ListInstalled() ([]App, error) {
	var out []App
	for _, h := range uninstallHives {
		k, err := registry.OpenKey(h.root, h.path, registry.ENUMERATE_SUB_KEYS)
		if err != nil {
			continue
		}
		names, err := k.ReadSubKeyNames(-1)
		k.Close()
		if err != nil {
			continue
		}
		for _, sub := range names {
			sk, err := registry.OpenKey(h.root, h.path+`\`+sub, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			name, _, _ := sk.GetStringValue("DisplayName")
			if strings.TrimSpace(name) == "" || isSystemComponent(sk) {
				sk.Close()
				continue
			}
			a := App{
				Name:       name,
				KeyPath:    h.path + `\` + sub,
				SubKeyName: sub,
			}
			a.Version, _, _ = sk.GetStringValue("DisplayVersion")
			a.Publisher, _, _ = sk.GetStringValue("Publisher")
			a.Uninstall, _, _ = sk.GetStringValue("UninstallString")
			a.Quiet, _, _ = sk.GetStringValue("QuietUninstallString")
			a.InstallLoc, _, _ = sk.GetStringValue("InstallLocation")
			a.DisplayIcon, _, _ = sk.GetStringValue("DisplayIcon")
			sz, _, _ := sk.GetIntegerValue("EstimatedSize")
			a.SizeKB = uint64(sz)
			sk.Close()
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })

	// Store apps join the same list; removal routes through Remove-AppxPackage
	// and never touches WindowsApps (servicing owns that tree).
	if appx, err := listAppx(); err == nil {
		out = append(out, appx...)
		sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	}
	return out, nil
}

func isSystemComponent(k registry.Key) bool {
	v, _, err := k.GetIntegerValue("SystemComponent")
	return err == nil && v == 1
}

// Run implements `DEFENESTRATE uninstall [name words...] [--dry-run] [--yes]`.
// Multiple name words form one filter phrase ("visual studio"); commas
// batch several targets: DEFENESTRATE uninstall foo,bar --yes.
func Run(args []string) error {
	var words []string
	yes := false
	for _, a := range args {
		switch a {
		case "--yes", "-y":
			yes = true
		default:
			if !strings.HasPrefix(a, "-") {
				words = append(words, a)
			}
		}
	}
	filter := strings.Join(words, " ")

	all, err := ListInstalled()
	if err != nil {
		return err
	}
	if filter == "" {
		fmt.Println(ui.Title("Installed programs") + ui.If(safety.DryRun(), ui.Warn("  [DRY RUN]")))
		for i, a := range all {
			fmt.Printf("  %3d. %-48s %-12s %s\n", i+1, truncate(a.Name, 48), a.Version, humanSize(a.SizeKB))
		}
		fmt.Println(ui.Dim("\nRun: DEFENESTRATE uninstall \"<name substring>\" to remove one; commas batch (foo,bar)."))
		return nil
	}

	var failed []string
	for _, phrase := range strings.Split(filter, ",") {
		phrase = strings.TrimSpace(phrase)
		if phrase == "" {
			continue
		}
		fmt.Println(ui.Rule())
		if err := uninstallOne(all, phrase, yes); err != nil {
			fmt.Println(ui.Bad("uninstall " + phrase + ": " + err.Error()))
			failed = append(failed, phrase)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("failed: %s", strings.Join(failed, ", "))
	}
	return nil
}

// uninstallOne resolves one filter phrase against the install list and runs
// the full evidence-backed removal flow for the single match.
func uninstallOne(all []App, filter string, yes bool) error {
	needle := strings.ToLower(filter)
	var matches []App
	for _, a := range all {
		if strings.Contains(strings.ToLower(a.Name), needle) {
			matches = append(matches, a)
		}
	}
	switch len(matches) {
	case 0:
		return fmt.Errorf("no installed program matching %q", filter)
	case 1:
	default:
		fmt.Println(ui.Warn("Multiple matches — be specific:"))
		for _, m := range matches {
			fmt.Println("  - " + m.Name)
		}
		return nil
	}
	app := matches[0]
	fmt.Println(ui.Title("Uninstalling "+app.Name) + ui.If(safety.DryRun(), ui.Warn("  [DRY RUN]")))

	sweep := SweepLeftoversFor(&app)
	lb := int64(0)
	for _, l := range sweep.Evidence {
		lb += dirSize(l)
	}

	printPlan(&app, sweep, lb)

	if safety.DryRun() {
		fmt.Println(ui.Warn("\nDry run — nothing executed."))
		return nil
	}
	if !yes && !confirm(fmt.Sprintf("Proceed with uninstall of %q?", app.Name)) {
		fmt.Println(ui.Dim("Cancelled — nothing was changed."))
		return nil
	}

	if app.Store {
		if safety.DryRun() {
			return nil
		}
		fmt.Println(ui.Good("Removing Store package…"))
		if err := removeAppx(app.PackageFullName); err != nil {
			return fmt.Errorf("Remove-AppxPackage: %w", err)
		}
		fmt.Println(ui.Good("Store package removed."))
		return nil
	}

	if app.Quiet != "" || app.Uninstall != "" {
		cmdline := firstNonEmpty(app.Quiet, withSilentFlag(app.Uninstall))
		fmt.Println(ui.Good("Running vendor uninstaller…"))
		if err := runUninstaller(cmdline); err != nil {
			fmt.Println(ui.Bad("vendor uninstaller failed:", err, "\ncontinuing with leftover sweep anyway"))
		}
	} else {
		fmt.Println(ui.Warn("No uninstaller registered; sweeping leftovers only."))
	}

	fmt.Println(ui.Good(fmt.Sprintf("Recycling %d evidence-backed locations (%s)…", len(sweep.Evidence), ui.HumanBytes(lb))))
	for _, l := range sweep.Evidence {
		if err := safety.Recycle([]string{l}); err != nil {
			fmt.Println(ui.Bad("  skip:", l, "-", err))
		} else {
			fmt.Println(ui.Check + " " + l)
		}
	}
	for _, k := range sweep.RegKeys {
		if safety.DryRun() {
			safety.Logf("[dry-run] would delete regkey", k, 0)
			continue
		}
		if sub := strings.TrimPrefix(k, `HKCU\Software\`); sub != k {
			_ = registry.DeleteKey(registry.CURRENT_USER, `Software\`+sub)
			fmt.Println(ui.Check + " " + k)
		}
	}
	for _, s := range sweep.StartupShortcuts {
		if err := safety.Recycle([]string{s}); err == nil {
			fmt.Println(ui.Check + " " + s)
		}
	}
	if len(sweep.Review) > 0 {
		fmt.Println(ui.Section("Review manually (NOT touched — name-similarity only)"))
		for _, r := range sweep.Review {
			fmt.Printf("       %10s  %s\n", ui.HumanBytes(dirSize(r)), r)
		}
	}
	if len(sweep.Tasks) > 0 {
		fmt.Println(ui.Section("Scheduled tasks found (NOT touched — remove via schtasks if unwanted)"))
		for _, t := range sweep.Tasks {
			fmt.Println("       task: " + t)
		}
	}
	fmt.Println(ui.Rule())
	fmt.Println(ui.Good("Done. A reboot may finish removal of in-use files."))
	return nil
}

func printPlan(app *App, sweep SweepResult, lb int64) {
	fmt.Println(ui.Warn("\nPlan:"))
	if app.Store {
		fmt.Printf("  1. Remove Store package: Remove-AppxPackage -Package '%s'\n", app.PackageFullName)
		fmt.Println("  2. No filesystem sweep — Windows services the WindowsApps tree.")
		return
	}
	fmt.Printf("  1. Run vendor uninstaller: %s\n", firstNonEmpty(app.Quiet, app.Uninstall, "(none registered)"))
	if len(sweep.Evidence) == 0 && len(sweep.RegKeys) == 0 {
		fmt.Println("  2. No registry-evidence locations found — nothing auto-recycled.")
	} else {
		fmt.Printf("  2. Recycle %d evidence-backed locations (%s):\n", len(sweep.Evidence), ui.HumanBytes(lb))
		for _, l := range sweep.Evidence {
			guarded := ""
			if err := safety.Check(l); err != nil {
				guarded = ui.Warn("   ⚠ protected — will be skipped")
			}
			fmt.Printf("       %10s  %s%s\n", ui.HumanBytes(dirSize(l)), l, guarded)
		}
		for _, k := range sweep.RegKeys {
			fmt.Printf("            regkey   %s\n", k)
		}
	}
	if len(sweep.StartupShortcuts) > 0 {
		fmt.Printf("  3. Recycle %d startup shortcuts:\n", len(sweep.StartupShortcuts))
		for _, s := range sweep.StartupShortcuts {
			fmt.Println("       - " + s)
		}
	}
	if len(sweep.Review) > 0 || len(sweep.Tasks) > 0 {
		fmt.Printf("  4. Review manually (never auto-touched): %d folders, %d scheduled tasks\n",
			len(sweep.Review), len(sweep.Tasks))
		for _, t := range sweep.Tasks {
			fmt.Println("       task: " + t)
		}
	}
}

// confirm asks a y/N question on stdin.
func confirm(q string) bool {
	fmt.Printf("%s [y/N] ", q)
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}

// SweepResult separates what the uninstaller may recycle automatically from
// what a human should review. Evidence comes from the registry record itself;
// name tokens never delete anything.
type SweepResult struct {
	// Evidence dirs: derived from InstallLocation, the uninstaller executable's
	// parent, and the DisplayIcon resource dir. Safe to recycle post-uninstall
	// because the registry entry itself points at them.
	Evidence []string
	// RegistryKeys: HKCU\Software subkeys whose name equals an evidence
	// identity exactly (case-insensitive). Never substring-matched.
	RegKeys []string
	// Review: name-token matches shown read-only for manual follow-up.
	Review []string
	// StartupShortcuts: Start Menu → Startup folder shortcuts whose filename
	// contains an evidence identity. Recyclable post-uninstall.
	StartupShortcuts []string
	// Tasks: scheduled tasks whose name contains a distinctive token.
	// Review-only — DEFENESTRATE never deletes tasks; use schtasks manually.
	Tasks []string
}

// SweepLeftoversFor builds the sweep plan from one app's registry evidence.
func SweepLeftoversFor(app *App) SweepResult {
	var res SweepResult
	res.Evidence = evidenceDirs(app)

	// Review-only suggestions from distinctive tokens (never auto-deleted).
	tokens := distinctiveTokens(app.Name)
	if len(tokens) > 0 {
		res.Review = reviewCandidates(tokens)
		res.Tasks = scheduledTaskHits(tokens)
	}

	// Startup-folder shortcuts: filename must contain the app's normalized
	// identity (exact folder/file-name evidence, not loose tokens).
	identity := normalizeIdentity(strings.TrimSpace(strings.TrimSuffix(app.Name, " (User)")))
	for _, dir := range startupDirs() {
		ents, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range ents {
			n := strings.ToLower(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
			if n == identity || strings.Contains(n, identity) && len(identity) > 3 {
				res.StartupShortcuts = append(res.StartupShortcuts, filepath.Join(dir, e.Name()))
			}
		}
	}

	// Exact-name registry keys under HKCU\Software.
	norm := strings.ToLower(normalizeIdentity(app.SubKeyName))
	hkcu, err := registry.OpenKey(registry.CURRENT_USER, `Software`, registry.ENUMERATE_SUB_KEYS)
	if err == nil {
		keys, kerr := hkcu.ReadSubKeyNames(-1)
		hkcu.Close()
		if kerr == nil {
			for _, k := range keys {
				if strings.ToLower(k) == norm {
					res.RegKeys = append(res.RegKeys, `HKCU\Software\`+k)
				}
			}
		}
	}
	return res
}

// startupDirs returns the per-user and all-users Startup folders.
func startupDirs() []string {
	var out []string
	if ad := os.Getenv("APPDATA"); ad != "" {
		out = append(out, filepath.Join(ad, "Microsoft", "Windows", "Start Menu", "Programs", "Startup"))
	}
	if pd := os.Getenv("ProgramData"); pd != "" {
		out = append(out, filepath.Join(pd, "Microsoft", "Windows", "Start Menu", "Programs", "Startup"))
	}
	return out
}

// normalizeIdentity collapses an app display name to its comparable form:
// lowercased, punctuation spaced, trailing edition noise dropped.
func normalizeIdentity(name string) string {
	clean := strings.NewReplacer("_", " ", "-", " ", ".", " ", "(", " ", ")", " ").Replace(strings.ToLower(name))
	f := strings.Fields(clean)
	// Drop parenthetical scope suffixes like "(User)" already handled; join back.
	return strings.Join(f, "")
}

// evidenceDirs extracts on-disk install directories from registry fields only.
func evidenceDirs(app *App) []string {
	var dirs []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, `"`)
		// DisplayIcon often carries ",0" resource index suffixes.
		if i := strings.LastIndex(p, ","); i > 0 && !strings.Contains(p[i:], `\`) {
			p = p[:i]
		}
		if p == "" || !filepath.IsAbs(p) {
			return // registry evidence must be absolute; relative paths are
			// malformed entries (e.g. bare "MsiExec.exe") and would otherwise
			// resolve against the CWD — never recycle based on those.
		}
		st, serr := os.Stat(p)
		if serr != nil || !st.IsDir() {
			return // must exist as a directory to count as evidence
		}
		dirs = append(dirs, filepath.Clean(p))
	}
	add(app.InstallLoc)
	if exe := exePathFromCmdline(firstNonEmpty(app.Quiet, app.Uninstall)); exe != "" {
		add(filepath.Dir(exe))
	}
	if app.DisplayIcon != "" {
		add(filepath.Dir(app.DisplayIcon))
	}
	return dedupeDirs(dirs)
}

// exePathFromCmdline extracts the executable path from an UninstallString,
// handling both quoted and unquoted forms.
func exePathFromCmdline(cmdline string) string {
	cmdline = strings.TrimSpace(cmdline)
	if cmdline == "" {
		return ""
	}
	if strings.HasPrefix(cmdline, `"`) {
		if end := strings.Index(cmdline[1:], `"`); end >= 0 {
			return cmdline[1 : 1+end]
		}
		return ""
	}
	fields := strings.Fields(cmdline)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func dedupeDirs(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		k := strings.ToLower(strings.TrimSuffix(s, string(filepath.Separator)))
		if !seen[k] {
			seen[k] = true
			out = append(out, s)
		}
	}
	return out
}

// reviewCandidates lists folders matching distinctive tokens under standard
// per-user roots — READ-ONLY output, printed for manual follow-up.
func reviewCandidates(tokens []string) []string {
	roots := []string{
		os.Getenv("LOCALAPPDATA"),
		os.Getenv("APPDATA"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs"),
	}
	var found []string
	for _, r := range roots {
		if r == "" {
			continue
		}
		ents, err := os.ReadDir(r)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if !e.IsDir() {
				continue
			}
			n := strings.ToLower(e.Name())
			for _, t := range tokens {
				if strings.Contains(n, t) {
					found = append(found, filepath.Join(r, e.Name()))
					break
				}
			}
		}
	}
	return dedupe(found)
}

// scheduledTaskHits lists scheduled tasks whose name contains a distinctive
// token of the app. Review-only: DEFENESTRATE never deletes tasks.
func scheduledTaskHits(tokens []string) []string {
	out, err := exec.Command("schtasks", "/query", "/fo", "csv", "/nh").Output()
	if err != nil {
		return nil
	}
	var hits []string
	for _, rec := range strings.Split(string(out), "\n") {
		rec = strings.TrimSpace(rec)
		if !strings.HasPrefix(rec, `"`) {
			continue
		}
		name := strings.SplitN(strings.TrimPrefix(rec, `"`), `"`, 2)[0]
		low := strings.ToLower(name)
		for _, t := range tokens {
			if strings.Contains(low, t) {
				hits = append(hits, name)
				break
			}
		}
	}
	return dedupe(hits)
}

// findInstalled locates one app by case-insensitive full/partial name match.
func findInstalled(nameOrFragment string) *App {
	all, err := ListInstalled()
	if err != nil {
		return nil
	}
	needle := strings.ToLower(nameOrFragment)
	for i := range all {
		if strings.Contains(strings.ToLower(all[i].Name), needle) {
			return &all[i]
		}
	}
	return nil
}

// sweepRegistryKeys removed: registry cleanup is now exact-name only via
// SweepResult.RegKeys (see SweepLeftoversFor). Substring key matching deleted
// an unrelated vendor's settings in testing.

// RunUninstallCommand executes a raw UninstallString (exported for the TUI).
func RunUninstallCommand(cmdline string) error { return runUninstaller(cmdline) }

// RemoveAppx removes one Store package (exported for the TUI).
func RemoveAppx(full string) error { return removeAppx(full) }
//   - "C:\path\unins000.exe"                       (bare path)
//   - "\"C:\path\unins000.exe\" /param"            (quoted exe + args)
//   - "MsiExec.exe /I{GUID}"                       (MSI)
func runUninstaller(cmdline string) error {
	cmdline = strings.TrimSpace(cmdline)
	var exe string
	var args []string
	if strings.HasPrefix(cmdline, `"`) {
		if end := strings.Index(cmdline[1:], `"`); end >= 0 {
			exe = cmdline[1 : 1+end]
			args = splitArgs(strings.TrimSpace(cmdline[1+end+1:]))
		}
	}
	if exe == "" {
		fields := strings.Fields(cmdline)
		if len(fields) == 0 {
			return fmt.Errorf("empty uninstall command")
		}
		exe = fields[0]
		args = fields[1:]
	}
	lower := strings.ToLower(exe)
	if strings.Contains(lower, "msiexec") {
		// Convert advertise/modify invocations to silent removal.
		m := map[string]bool{"/i": true}
		var cleaned []string
		for _, a := range args {
			if m[strings.ToUpper(a)] || strings.EqualFold(a, "/x") {
				cleaned = append(cleaned, "/x", "/qn", "/norestart")
				continue
			}
			cleaned = append(cleaned, a)
		}
		return exec.Command("msiexec.exe", append([]string{"/qn", "/norestart"}, cleaned...)...).Run()
	}
	return exec.Command(exe, args...).Run()
}

func splitArgs(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Fields(s)
}

// withSilentFlag adds common silent switches to uninstallers that don't
// advertise QuietUninstallString. Conservative: only well-known flags.
func withSilentFlag(u string) string {
	if u == "" {
		return ""
	}
	switch {
	case strings.Contains(strings.ToLower(u), "msiexec"):
		return strings.Replace(u, "/I", "/X", 1) + " /qn /norestart"
	case strings.Contains(strings.ToLower(u), "unins"):
		return u + " /S" // NSIS convention
	default:
		return u
	}
}

// distinctiveTokens: words from the app name used for leftover matching.
// Hard vendor names are excluded (they'd match unrelated folders); everything
// else is fair game BECAUSE the user reviews the full candidate list with
// sizes before anything is recycled. Visibility is the safety mechanism.
func distinctiveTokens(name string) []string {
	var out []string
	for _, f := range tokenize(name) {
		if len(f) < 4 {
			continue
		}
		if hardVendor(f) {
			continue
		}
		out = append(out, f)
	}
	return out
}

func hardVendor(w string) bool {
	switch w {
	case "microsoft", "google", "apple", "adobe", "intel", "nvidia":
		return true
	}
	return false
}

func tokenize(name string) []string {
	clean := strings.NewReplacer("_", " ", "-", " ", ".", " ", "(", " ", ")", " ").Replace(strings.ToLower(name))
	fields := strings.Fields(clean)
	var out []string
	for _, f := range fields {
		if len(f) > 2 && !stopword(f) {
			out = append(out, f)
		}
	}
	return out
}

func stopword(w string) bool {
	switch w {
	case "the", "and", "for", "inc", "ltd", "llc", "software", "app", "application":
		return true
	}
	return false
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		k := strings.ToLower(s)
		if !seen[k] {
			seen[k] = true
			out = append(out, s)
		}
	}
	return out
}

func dirSize(p string) int64 {
	var total int64
	filepath.WalkDir(p, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, e2 := d.Info(); e2 == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func humanSize(kb uint64) string {
	if kb == 0 {
		return ui.Dim("?")
	}
	return ui.HumanBytesU(kb * 1024)
}
