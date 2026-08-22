package update

import (
	"strings"
	"testing"
)

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v1.0.0", "v1.0.1", -1},
		{"v1.2.0", "v1.1.9", 1},
		{"v0.1.0", "v0.1.0", 0},
		{"1.0", "1.0.0", 0},
		{"dev", "v0.0.1", -1}, // unversioned dev builds always update
		{"v2", "v10.0.0", -1},
	}
	for _, c := range cases {
		if got := CompareSemver(c.a, c.b); got != c.want {
			t.Errorf("CompareSemver(%q,%q)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestAssetName(t *testing.T) {
	got := AssetName()
	if got != "DEFENESTRATE-windows-amd64.exe" && got != "DEFENESTRATE-windows-arm64.exe" {
		t.Errorf("unexpected asset name %q", got)
	}
}

func TestLatestReleaseUnconfigured(t *testing.T) {
	repoOwner, repoName = "", ""
	if _, err := LatestRelease(); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("unconfigured channel must say so, got %v", err)
	}
}
