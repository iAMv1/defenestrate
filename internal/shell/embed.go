// Package shell carries the embedded PowerShell script library: the
// shell-first engine. Go dispatches and guards; these scripts do the
// Windows-native work inside the persistent pshost session.
package shell

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

//go:embed scripts/*.ps1
var Scripts embed.FS

var (
	tmpMu   sync.Mutex
	tmpN    int
	tmpBase string
)

// WriteTempSpec persists a JSON payload for a -File style script invocation
// (avoids stdin/encoding pitfalls for structured data) and returns its path.
func WriteTempSpec(payload []byte) (string, error) {
	tmpMu.Lock()
	defer tmpMu.Unlock()
	if tmpBase == "" {
		base := os.Getenv("TEMP")
		if base == "" {
			base = os.TempDir()
		}
		dir := filepath.Join(base, "DEFENESTRATE", "ps")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		tmpBase = dir
	}
	tmpN++
	p := filepath.Join(tmpBase, "spec_"+filepath.Base(fmt.Sprintf("call_%d.json", tmpN)))
	if err := os.WriteFile(p, payload, 0o600); err != nil {
		return "", err
	}
	return p, nil
}
