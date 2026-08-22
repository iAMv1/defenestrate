package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// prefs persists small user preferences across runs. Kept deliberately tiny:
// a new knob must earn its place (mole rule), so only genuinely sticky
// choices live here.
type prefs struct {
	CoreRows int `json:"core_rows"` // status: 2/4/8/0=all
}

func prefsPath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
	}
	return filepath.Join(base, "DEFENESTRATE", "prefs.json")
}

func loadPrefs() prefs {
	var p prefs
	b, err := os.ReadFile(prefsPath())
	if err == nil {
		json.Unmarshal(b, &p)
	}
	return p
}

func savePrefs(p prefs) {
	os.MkdirAll(filepath.Dir(prefsPath()), 0o755)
	b, _ := json.MarshalIndent(p, "", "  ")
	os.WriteFile(prefsPath(), b, 0o644)
}
