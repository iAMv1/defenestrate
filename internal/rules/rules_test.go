package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TrustContents bypasses the executable/unknown hold-back, so granting it to
// a vendor ROOT (e.g. all of %LOCALAPPDATA%\AMD, which also holds driver
// installers) would recycle executables. Pin the invariant: a TrustContents
// target must be scoped at least one directory BELOW its env base.
func TestTrustContentsNeverOnVendorRoots(t *testing.T) {
	bases := map[string]string{
		"LOCALAPPDATA": os.Getenv("LOCALAPPDATA"),
		"APPDATA":      os.Getenv("APPDATA"),
		"USERPROFILE":  os.Getenv("USERPROFILE"),
		"ProgramData":  os.Getenv("ProgramData"),
	}
	for _, tgt := range Targets {
		if !tgt.TrustContents {
			continue
		}
		p := tgt.Path()
		if p == "" {
			continue
		}
		scoped := false
		for _, base := range bases {
			if base == "" || !strings.HasPrefix(strings.ToLower(p), strings.ToLower(base)) {
				continue
			}
			rel, err := filepath.Rel(base, p)
			if err == nil && rel != "." && strings.ContainsRune(rel, filepath.Separator) {
				scoped = true
			}
		}
		if !scoped {
			t.Errorf("TrustContents on vendor root %q (%s): driver installers/state would bypass hold-back", tgt.Label, p)
		}
	}
}

// Every TrustContents grant must cite an owner-documented reset contract in
// the surrounding source; here we at least pin the known-good set so a new
// grant without review is visible in diff.
func TestTrustContentsKnownSet(t *testing.T) {
	allowed := map[string]bool{
		"npm cache": true, "pip cache": true, "yarn cache": true,
		"NuGet http cache": true,
		"NVIDIA GLShader cache": true, "AMD shader caches": true,
		"Intel shader cache": true, "DirectX D3DSCache": true,
		"pnpm store": true, "Bun install cache": true,
		"node-gyp headers cache": true,
		"Poetry cache": true, "electron builder cache": true,
		"Steam HTML cache": true,
	}
	for _, tgt := range Targets {
		if tgt.TrustContents && !allowed[tgt.Label] {
			t.Errorf("new TrustContents grant %q not reviewed — add it to the allowed set with a reset-contract citation", tgt.Label)
		}
	}
}
