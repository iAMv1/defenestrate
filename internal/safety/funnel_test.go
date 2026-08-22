package safety

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDeletionFunnelEnforced is the Go analog of Mole's "# SAFE:" whitelist:
// direct filesystem deletion calls are allowed ONLY inside the safety package.
// Any new call site must go through Recycle, so dry-run, guards, protection
// data, and the operations log cannot be bypassed.
func TestDeletionFunnelEnforced(t *testing.T) {
	banned := []string{
		"os.RemoveAll", "os.Remove(", "os.RemoveDir", "syscall.DeleteFile",
		"DeleteFileW", "DeleteDirectory(",
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable trees can't hide deletions from the compiler anyway
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "dist" || name == "state" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		if strings.Contains(p, string(filepath.Separator)+"internal"+string(filepath.Separator)+"safety"+string(filepath.Separator)) {
			return nil // the funnel itself
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		lines := strings.Split(string(b), "\n")
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue // comments may discuss the banned calls
			}
			if strings.Contains(line, "SAFE:") {
				continue // annotated exception: self-created scratch files only
			}
			for _, bannedCall := range banned {
				if strings.Contains(line, bannedCall) {
					t.Errorf("%s:%d: deletion outside the safety funnel (%s). Route through safety.Recycle.", p, i+1, bannedCall)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAuthoredStatePredicates(t *testing.T) {
	if !containsProtectedName(`C:\Users\x\.ssh`) {
		t.Error(".ssh must match protected dir names")
	}
	if !containsProtectedFragment(`C:\Users\x\AppData\Local\Google\Chrome\User Data\Default\Login Data`) {
		t.Error("browser Login Data must match protected fragments")
	}
	if containsProtectedFragment(`C:\Users\x\AppData\Local\Google\Chrome\User Data\Default\Cache`) {
		t.Error("plain Cache leaf must stay cleanable")
	}
	if containsProtectedName(`C:\Program Files\Mozilla Firefox`) {
		t.Error("Firefox program dir is not authored state")
	}
}

// The safety funnel must refuse authored-state dirs even mid-tree, while
// leaving the parent app directory recyclable.
func TestCheckRejectsAuthoredStateInsideEligibleTree(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "SomeApp", ".ssh")
	os.MkdirAll(target, 0o755)
	if err := Check(target); err == nil {
		t.Fatal(".ssh directory must be refused by Check")
	}
	parent := filepath.Join(base, "SomeApp")
	if err := Check(parent); err != nil {
		t.Fatalf("parent app dir should remain recyclable: %v", err)
	}
}
