package apps

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iAMv1/defenestrate/internal/safety"
)

func TestAppxNameFallback(t *testing.T) {
	if got := appxNameFromFull("Microsoft.MicrosoftStickyNotes_4.0.6105.0_x64__8wekyb3d8bbwe"); got != "Microsoft.MicrosoftStickyNotes" {
		t.Errorf("got %q", got)
	}
	if got := appxNameFromFull("NoVersionSegment"); got != "NoVersionSegment" {
		t.Errorf("got %q", got)
	}
}

func TestExePathFromCmdline(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`"C:\Program Files\App\unins000.exe" /SILENT`, `C:\Program Files\App\unins000.exe`},
		{`C:\Tools\uninstall.exe /quiet`, `C:\Tools\uninstall.exe`},
		{`MsiExec.exe /I{GUID}`, `MsiExec.exe`},
		{``, ``},
	}
	for _, c := range cases {
		if got := exePathFromCmdline(c.in); got != c.want {
			t.Errorf("exePathFromCmdline(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestEvidenceDirsFromRegistryFields(t *testing.T) {
	dir := t.TempDir()
	app := &App{
		Name:        "TestApp",
		InstallLoc:  filepath.Join(dir, "install"),
		Uninstall:   `"` + filepath.Join(dir, "install", "unins.exe") + `" /S`,
		DisplayIcon: filepath.Join(dir, "install", "icon.exe") + ",0",
	}
	os.MkdirAll(filepath.Join(dir, "install"), 0o755)

	got := evidenceDirs(app)
	if len(got) != 1 {
		t.Fatalf("want 1 deduped evidence dir, got %v", got)
	}
}

func TestEvidenceDirsDropsMissingPaths(t *testing.T) {
	app := &App{InstallLoc: `C:\definitely\not\here__DEFENESTRATE_test`}
	got := evidenceDirs(app)
	if len(got) != 0 {
		t.Fatalf("nonexistent InstallLocation must yield no evidence, got %v", got)
	}
}

// Regression pin for the live incident: vendor names must never become
// deletion/matching tokens. Distinctive tokens feed ONLY the review list
// (never deleted); evidence dirs come from registry fields alone.
func TestDistinctiveTokensExcludeVendorsAndShortWords(t *testing.T) {
	got := distinctiveTokens("Microsoft Visual Studio Code (User)")
	for _, tok := range got {
		if tok == "microsoft" {
			t.Errorf("vendor name %q must never survive filtering: %v", tok, got)
		}
	}
	if len(got) == 0 {
		t.Fatal("expected at least visual/studio as review tokens")
	}
}

// The real safety property: evidence dirs derive ONLY from registry fields,
// so a VS-Code-like app can never propose unrelated AppData trees.
func TestEvidenceDirsStructurallyLimited(t *testing.T) {
	app := &App{Name: "Microsoft Visual Studio Code (User)"} // no registry fields set
	if got := evidenceDirs(app); len(got) != 0 {
		t.Fatalf("no registry evidence => no recycle candidates, got %v", got)
	}
}

// The safety funnel must refuse authored-state dirs even mid-tree.
func TestCheckRejectsAuthoredStateInsideEligibleTree(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "SomeApp", ".ssh")
	os.MkdirAll(target, 0o755)
	if err := safety.Check(target); err == nil {
		t.Fatal(".ssh directory must be refused by Check")
	}
}

func TestEvidenceDirsRejectsRelativePaths(t *testing.T) {
	cwd, _ := os.Getwd()
	app := &App{
		Name:       "MSI App",
		Quiet:      `MsiExec.exe /I{23170F69-40C1-2702-2601-000001000000}`,
		InstallLoc: "", // MSIs often leave this empty
	}
	for _, d := range evidenceDirs(app) {
		if d == cwd || d == filepath.Clean(cwd) {
			t.Fatalf("evidence dir resolved to CWD %q — relative-path leak", d)
		}
	}
}
