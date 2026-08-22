package clean

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iAMv1/defenestrate/internal/rules"
	"github.com/iAMv1/defenestrate/internal/safety"
)

// Path-probe hardening: traversal out of a safe zone into guarded territory
// must be refused, while the safe zone itself stays eligible.
func TestCheckRefusesTraversalAndRoots(t *testing.T) {
	windir := os.Getenv("WINDIR")
	cases := []string{
		filepath.Join(windir, "Temp", "a", "..", "..", "System32", "config"), // escapes zone
		filepath.Join(windir, "System32", "config"),
		`C:\`,
	}
	for _, p := range cases {
		if err := safety.Check(p); err == nil {
			t.Errorf("Check(%q) must refuse", p)
		}
	}
	// Safe-zone contents stay eligible.
	ok := filepath.Join(windir, "Temp")
	if err := safety.Check(ok); err != nil {
		t.Errorf("Check(%q) should pass: %v", ok, err)
	}
	// A normal subdir stays eligible.
	base := t.TempDir()
	ok2 := filepath.Join(base, "SomeApp")
	os.MkdirAll(ok2, 0o755)
	if err := safety.Check(ok2); err != nil {
		t.Errorf("Check(%q) should pass: %v", ok2, err)
	}
}

func TestClassifyFileVerdicts(t *testing.T) {
	cases := map[string]safety.FileClass{
		"cache.tmp":          safety.ClassKnownJunk,
		"edge.log":           safety.ClassKnownJunk,
		"dump.dmp":           safety.ClassKnownJunk,
		"tool.exe":           safety.ClassExecutable,
		"setup.msi":          safety.ClassExecutable,
		"script.ps1":         safety.ClassExecutable,
		"backup.iso":         safety.ClassExecutable,
		"noext":              safety.ClassUnknown,
		"mystery.xyzformat":  safety.ClassUnknown,
	}
	for path, want := range cases {
		if got := safety.ClassifyFile(path); got != want {
			t.Errorf("ClassifyFile(%q)=%v want %v", path, got, want)
		}
	}
}

// The user's law, as a test: broad temp-style rules must HOLD executables and
// unknown formats for review instead of recycling them with the junk.
func TestBroadRuleHoldsUnknowables(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "junk.log"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "old.txt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "portable-tool.exe"), []byte("MZ"), 0o644)
	os.WriteFile(filepath.Join(dir, "datafile.weird"), []byte("x"), 0o644)

	tgt := rules.Target{Category: "test", Label: "test", Path: func() string { return dir }}
	f, err := evalTarget(tgt, dir)
	if err != nil {
		t.Fatal(err)
	}
	held := strings.Join(f.HeldPaths, ";")
	for _, p := range f.Paths {
		base := filepath.Base(p)
		if base == "portable-tool.exe" || base == "datafile.weird" || base == "old.txt" {
			t.Errorf("%s must be held, not auto-recycled", p)
		}
	}
	if !strings.Contains(held, "portable-tool.exe") || !strings.Contains(held, "datafile.weird") || !strings.Contains(held, "old.txt") {
		t.Errorf("exe/unknown/txt must land in HeldPaths, got %v", f.HeldPaths)
	}
	if !containsPath(f.Paths, "junk.log") {
		t.Errorf("known-junk files should still be recycled, got %v", f.Paths)
	}
}

func containsPath(paths []string, suffix string) bool {
	for _, p := range paths {
		if filepath.Base(p) == suffix {
			return true
		}
	}
	return false
}

// TrustContents is the ONLY sanctioned bypass of the hold-back law, and it
// requires the rule to cite an owner-documented reset contract.
func TestTrustContentsBypassesHoldBack(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pkg.tgz"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "binary.exe"), []byte("MZ"), 0o644)

	trusted := rules.Target{
		Category: "test", Label: "trusted",
		Path: func() string { return dir }, TrustContents: true,
	}
	f, err := evalTarget(trusted, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.HeldPaths) != 0 {
		t.Errorf("TrustContents must hold nothing back, got %v", f.HeldPaths)
	}
	if len(f.Paths) != 2 {
		t.Errorf("both files recyclable under documented reset contract, got %v", f.Paths)
	}

	untrusted := trusted
	untrusted.Label = "untrusted"
	untrusted.TrustContents = false
	f2, err := evalTarget(untrusted, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(f2.HeldPaths) != 2 {
		t.Errorf("without trust, exe+unknown must be held, got %v (paths=%v)", f2.HeldPaths, f2.Paths)
	}
}

func TestDecideSkipTriState(t *testing.T) {
	orig := processProbe
	defer func() { processProbe = orig }()

	tgt := rules.Target{SkipIfRunning: []string{"app.exe"}}

	processProbe = func([]string) ([]string, bool) { return []string{"app.exe"}, true }
	if got := decideSkip(tgt); !strings.Contains(got, "running") {
		t.Errorf("running => skip with reason, got %q", got)
	}

	processProbe = func([]string) ([]string, bool) { return nil, true }
	if got := decideSkip(tgt); got != "" {
		t.Errorf("not running => proceed, got %q", got)
	}

	processProbe = func([]string) ([]string, bool) { return nil, false }
	got := decideSkip(tgt)
	if got != "process state unknown" {
		t.Errorf("unknown state must DENY, got %q", got)
	}
}
