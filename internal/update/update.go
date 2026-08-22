package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

// Repo coordinates are injected at build time (never hardcoded in source):
//
//	go build -ldflags "-X main.version=v1.0.0 -X update.repoOwner=acme -X update.repoName=DEFENESTRATE"
//
// An empty owner/name means the update channel is unconfigured; `DEFENESTRATE
// update` says so instead of guessing an endpoint.
var (
	repoOwner string
	repoName  string
)

const httpTimeout = 30 * time.Second

// Release describes one GitHub release we care about.
type Release struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

// LatestRelease fetches the newest published release for the configured repo.
func LatestRelease() (*Release, error) {
	if repoOwner == "" || repoName == "" {
		return nil, fmt.Errorf("update channel not configured (build without repo ldflags)")
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repoOwner, repoName)
	client := &http.Client{Timeout: httpTimeout}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("github api: %s", resp.Status)
	}
	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

// AssetName is the exact artifact name publish.yml produces for this platform.
func AssetName() string {
	return fmt.Sprintf("DEFENESTRATE-windows-%s.exe", runtime.GOARCH)
}

// FindAsset locates this platform's binary in a release.
func (r *Release) FindAsset() (url string, size int64, err error) {
	want := AssetName()
	for _, a := range r.Assets {
		if a.Name == want {
			return a.BrowserDownloadURL, a.Size, nil
		}
	}
	return "", 0, fmt.Errorf("release %s has no asset %q", r.TagName, want)
}

// CompareSemver returns -1/0/+1 for a<b / equal / a>b. Handles "v" prefixes
// and numeric segments only ("v1.2.3"); non-numeric suffixes compare shorter-
// is-older at that segment boundary.
func CompareSemver(a, b string) int {
	pa := segs(a)
	pb := segs(b)
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		x, y := seg(pa, i), seg(pb, i)
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	return 0
}

func segs(v string) []string {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" {
		return nil
	}
	return strings.Split(v, ".")
}

func seg(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	v := 0
	fmt.Sscanf(parts[i], "%d", &v)
	return v
}

// Apply downloads the asset and atomically replaces the running binary.
// The download lands next to the target and is renamed only after full write,
// so a failed download never leaves a broken executable behind.
func Apply(rel *Release, target string) error {
	url, size, err := rel.FindAsset()
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download: %s", resp.Status)
	}

	tmp := target + ".update"
	fh, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	written, err := io.Copy(fh, resp.Body)
	closeErr := fh.Close()
	if err != nil {
		os.Remove(tmp) // SAFE: our own failed-download temp file
		return err
	}
	if closeErr != nil {
		os.Remove(tmp) // SAFE: our own failed-download temp file
		return closeErr
	}
	if size > 0 && written != size {
		os.Remove(tmp) // SAFE: our own size-mismatch temp file
		return fmt.Errorf("size mismatch: got %d want %d", written, size)
	}

	old := target + ".old"
	os.Remove(old) // SAFE: discards our own previous-version swap file from an earlier update
	if err := os.Rename(target, old); err != nil {
		os.Remove(tmp) // SAFE: our own failed-download temp file
		return fmt.Errorf("swap running binary: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Rename(old, target) // roll back
		os.Remove(tmp)         // SAFE: our own failed-activation temp file
		return fmt.Errorf("activate new binary: %w", err)
	}
	os.Remove(old) // SAFE: replaced version's swap artifact
	return nil
}

// NeedsUpdate decides whether installed should move to latest.
func NeedsUpdate(installed, latest string) bool { return CompareSemver(installed, latest) < 0 }

// Run performs the full self-update flow: check → compare → swap binary.
// installedVersion comes from the caller's injected version var.
func Run(installedVersion string, onUpdated func(newVersion string)) error {
	fmt.Println("Checking for updates…")
	rel, err := LatestRelease()
	if err != nil {
		return err
	}
	switch CompareSemver(installedVersion, rel.TagName) {
	case 1, 0:
		fmt.Printf("Already up to date (%s ≥ %s).\n", installedVersion, rel.TagName)
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	fmt.Printf("Updating %s → %s (%s)…\n", installedVersion, rel.TagName, AssetName())
	if err := Apply(rel, exe); err != nil {
		return err
	}
	if onUpdated != nil {
		onUpdated(rel.TagName)
	}
	return nil
}
