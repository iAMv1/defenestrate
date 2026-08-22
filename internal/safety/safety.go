// Package safety is the single gate every destructive operation passes
// through: dry-run state, protected-path guards, Recycle-Bin deletion and the
// operations log. No other package may delete anything directly.
package safety

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	mu     sync.Mutex
	dryRun bool
)

// SetDryRun flips global preview mode. In dry-run nothing is ever deleted and
// every mutation logs with a [dry-run] tag instead.
func SetDryRun(v bool) { mu.Lock(); dryRun = v; mu.Unlock() }

// DryRun reports current preview mode.
func DryRun() bool { mu.Lock(); defer mu.Unlock(); return dryRun }

// ---------------------------------------------------------------------------
// Protected paths.
//
// Guards are expressed as environment-derived prefixes plus an explicit
// allowlist of system subdirectories that ARE safe to clean (their contents,
// never the directory itself). Nothing here is machine-specific: no user
// names, no drive letters beyond the Windows-managed env vars.
// ---------------------------------------------------------------------------

var (
	guardRoots     []string // deletion refused outright (as prefix)
	safeZones      []string // deletable content zones inside guarded roots
	exactProtected []string // exact paths that may never be recycled
)

func init() {
	for _, env := range []string{"WINDIR", "ProgramFiles", "ProgramFiles(x86)", "ProgramData"} {
		if p := os.Getenv(env); p != "" {
			guardRoots = append(guardRoots, strings.ToLower(p))
		}
	}
	// Per-user app-data zones are legitimate cleanup territory (the whole
	// point of an uninstaller), so the profile root is NOT a blanket guard;
	// the roots themselves stay irrecyclable via exactProtected.
	for _, env := range []string{"USERPROFILE", "LOCALAPPDATA", "APPDATA", "SystemDrive"} {
		if p := os.Getenv(env); p != "" {
			exactProtected = append(exactProtected, strings.ToLower(p))
		}
	}
	// Contents of these are explicitly cleanable despite living under WINDIR.
	for _, p := range []string{
		`${windir}\Temp`,
		`${windir}\SoftwareDistribution\Download`,
		`${windir}\Logs`,
		`${windir}\Minidump`,
		`${windir}\LiveKernelReports`,
	} {
		if expanded, err := expandEnv(p); err == nil {
			safeZones = append(safeZones, strings.ToLower(expanded))
		}
	}
}

// expandEnv resolves the ${VAR} placeholders used in safe-zone definitions.
func expandEnv(p string) (string, error) {
	p = os.Expand(p, func(k string) string {
		return os.Getenv(strings.ToUpper(k))
	})
	return filepath.Abs(p)
}

// Check decides whether path may be recycled. It returns nil only when the
// path exists inside an allowed zone or outside every guard root.
func Check(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve %q: %w", path, err)
	}
	abs = strings.ToLower(strings.TrimSuffix(abs, string(filepath.Separator)))
	if abs == "" || len(abs) <= 3 { // drive roots: never
		return fmt.Errorf("refusing to delete a drive root")
	}
	for _, p := range exactProtected {
		vol := strings.ToLower(os.Getenv("SystemDrive"))
		if abs == p || abs == strings.TrimSuffix(vol, `\`) {
			return fmt.Errorf("%q is a protected root", path)
		}
	}
	for _, g := range guardRoots {
		if abs == g {
			return fmt.Errorf("%q is a protected root", path)
		}
		if strings.HasPrefix(abs, g+string(filepath.Separator)) {
			for _, z := range safeZones {
				if abs == z || strings.HasPrefix(abs, z+string(filepath.Separator)) {
					goto allowed
				}
			}
			return fmt.Errorf("%q is inside protected %q", path, g)
		}
	}
allowed:
	// Authored-state protection: identity and credential stores are refused
	// even when they sit inside otherwise-eligible trees (see
	// protection_data.go for the classification principle).
	if containsProtectedName(abs) || containsProtectedFragment(abs) {
		return fmt.Errorf("%q holds authored state protected by policy", path)
	}
	return nil
}

// Recycle moves paths to the Recycle Bin via PowerShell's FileSystem API.
// One PowerShell process handles the whole batch. Dry-run short-circuits.
func Recycle(paths []string) error {
	var valid []string
	for _, p := range paths {
		if err := Check(p); err != nil {
			return err
		}
		if st, err := os.Stat(p); err == nil {
			_ = st
			valid = append(valid, p)
		}
	}
	if len(valid) == 0 {
		return nil
	}
	if DryRun() {
		for _, p := range valid {
			Logf("[dry-run] recycle", p, 0)
		}
		return nil
	}
	script := "Add-Type -AssemblyName Microsoft.VisualBasic; " +
		"foreach ($p in $args) { if (Test-Path -LiteralPath $p) { " +
		"[Microsoft.VisualBasic.FileIO.FileSystem]::DeleteDirectory($p,'OnlyErrorDialogs','SendToRecycleBin') } }"
	args := append([]string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script}, valid...)
	out, err := execPowershell(args)
	if err != nil {
		return fmt.Errorf("recycle bin: %w: %s", err, strings.TrimSpace(out))
	}
	for _, p := range valid {
		Logf("recycle", p, 0)
	}
	return nil
}

// FlushDNS refreshes the DNS cache ("optimize" hygiene), skipped in dry-run.
func FlushDNS() error {
	if DryRun() {
		Logf("[dry-run] flushdns", "", 0)
		return nil
	}
	_, err := execPowershell([]string{"-NoProfile", "-NonInteractive", "-Command", "ipconfig /flushdns"})
	return err
}

// ---------------------------------------------------------------------------
// Operations log — ~/.local/share/DEFENESTRATE/operations.log style but Windows-
// native: %LOCALAPPDATA%\DEFENESTRATE\operations.log
// ---------------------------------------------------------------------------

func logPath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
	}
	return filepath.Join(base, "DEFENESTRATE", "operations.log")
}

// Logf records one mutation (or preview). Failures to write must never break
// the actual operation.
func Logf(action, path string, size int64) {
	f := logPath()
	_ = os.MkdirAll(filepath.Dir(f), 0o755)
	fh, err := os.OpenFile(f, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer fh.Close()
	fmt.Fprintf(fh, "%s\t%s\t%s\t%d\n", time.Now().Format(time.RFC3339), action, path, size)
}

// PrintHistory dumps the operations log newest-last.
func PrintHistory() error {
	b, err := os.ReadFile(logPath())
	if err != nil {
		fmt.Println("no operations logged yet")
		return nil
	}
	os.Stdout.Write(b)
	return nil
}

// OpEntry is one parsed operations-log line.
type OpEntry struct {
	Time   string `json:"ts"`
	Action string `json:"action"`
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
}

// HistoryJSON parses the operations log into structured entries.
func HistoryJSON() ([]OpEntry, error) {
	b, err := os.ReadFile(logPath())
	if err != nil {
		return []OpEntry{}, nil // no log yet = empty, not an error
	}
	var out []OpEntry
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 3 {
			continue
		}
		e := OpEntry{Time: parts[0], Action: parts[1], Path: parts[2]}
		if len(parts) == 4 {
			fmt.Sscanf(parts[3], "%d", &e.Bytes)
		}
		out = append(out, e)
	}
	return out, nil
}
