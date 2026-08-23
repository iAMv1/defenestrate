// Package rules holds every cleanable target as DATA, not code. Adding a new
// cleanable location means adding a row to Targets — never a new function.
// All paths derive from Windows environment variables or registry lookups;
// nothing user-specific or drive-specific is written down anywhere.
package rules

import (
	"os"
	"path/filepath"
)

// Risk communicates how conservative the default should be in the TUI.
type Risk int

const (
	Low    Risk = iota // caches, dumps — always selected
	Medium             // logs, update cache — selected with confirmation
	High               // never auto-selected; expert only (unused in v1 defaults)
)

// Target describes one cleanable location.
//
// Dir semantics:
//   - Patterns == nil  → the directory's CONTENTS are cleanable (dir kept).
//   - Patterns != nil  → only entries matching a pattern are cleanable.
//   - MinAgeDays > 0   → files newer than N days are skipped (mtime).
//   - SkipIfRunning    → target ignored while any listed process runs; if the
//     process list itself cannot be read, the target is SKIPPED (tri-state:
//     unknown denies).
//   - TrustContents    → bypasses the executable/unknown hold-back. Only for
//     caches whose OWNER DOCUMENTS full-content resets (npm/pip/yarn/nuget),
//     cited in the rule comment. Never set for mixed-state trees.
type Target struct {
	Category      string // report grouping, e.g. "System temp"
	Label         string // human label, e.g. "Windows Update download cache"
	Path          func() string
	Patterns      []string // relative globs; nil = whole contents
	MinAgeDays    int
	SkipIfRunning []string
	TrustContents bool
	Risk          Risk
}

func env(keys ...string) func() string {
	return func() string {
		for _, k := range keys {
			if v := os.Getenv(k); v != "" {
				return v
			}
		}
		return ""
	}
}

func join(base func() string, elems ...string) func() string {
	return func() string {
		b := base()
		if b == "" {
			return ""
		}
		return filepath.Join(append([]string{b}, elems...)...)
	}
}

var localAppData = env("LOCALAPPDATA")
var roamingAppData = env("APPDATA")
var winDir = env("WINDIR")

// Targets is the single source of truth for `DEFENESTRATE clean`.
// Keep it boring: rows of data, reviewed at a glance.
var Targets = []Target{
	{
		Category:   "System temp",
		Label:      "User temp files",
		Path:       join(localAppData, "Temp"),
		MinAgeDays: 1,
		Risk:       Low,
	},
	{
		Category:   "System temp",
		Label:      "Windows temp",
		Path:       join(winDir, "Temp"),
		MinAgeDays: 1,
		Risk:       Medium,
	},
	{
		Category:   "System temp",
		Label:      "Windows Update download cache",
		Path:       join(winDir, "SoftwareDistribution", "Download"),
		MinAgeDays: 3,
		Risk:       Medium,
	},
	{
		Category: "System temp",
		Label:    "Delivery Optimization cache",
		Path: func() string {
			w := winDir()
			if w == "" {
				return ""
			}
			return filepath.Join(w, "ServiceProfiles", "NetworkService",
				"AppData", "Local", "Microsoft", "Windows", "DeliveryOptimization", "Cache")
		},
		Risk: Medium,
	},
	{
		Category: "Crash dumps & logs",
		Label:    "User crash dumps",
		Path:     join(localAppData, "CrashDumps"),
		Risk:     Low,
	},
	{
		Category: "Crash dumps & logs",
		Label:    "Kernel minidumps",
		Path:     join(winDir, "Minidump"),
		Risk:     Low,
	},
	{
		Category:   "Crash dumps & logs",
		Label:      "Component cleanup logs (>30d)",
		Path:       join(winDir, "Logs", "CBS"),
		Patterns:   []string{"*.log", "*.cab"},
		MinAgeDays: 30,
		Risk:       Medium,
	},
	{
		Category:      "Thumbnails",
		Label:         "Explorer thumbnail caches",
		Path:          join(localAppData, "Microsoft", "Windows", "Explorer"),
		Patterns:      []string{"thumbcache_*.db", "iconcache_*.db"},
		SkipIfRunning: []string{"explorer.exe"},
		Risk:          Low,
	},

	// ---- Developer caches (recovery contract: owner documents full reset) --
	// Each target below is a package-manager cache the owner tool explicitly
	// supports deleting wholesale; contents are re-downloaded on demand.
	// TrustContents=true is justified by that documented contract, NOT by
	// "looks like a cache". Mixed-state stores (cargo registry/src, m2,
	// ivy2, huggingface) deliberately stay out of this list.
	devCache("npm cache", join(localAppData, "npm-cache")),
	devCache("pip cache", join(localAppData, "pip", "cache")),
	devCache("yarn cache", join(localAppData, "Yarn", "Cache")),
	devCache("NuGet http cache", join(localAppData, "NuGet", "v3-cache")),

	// ---- Browser caches (cache data ONLY — never cookies/passwords) --------
	browserCache("Chrome", join(localAppData, "Google", "Chrome", "User Data")),
	browserCache("Edge", join(localAppData, "Microsoft", "Edge", "User Data")),
	browserCache("Brave", join(localAppData, "BraveSoftware", "Brave-Browser", "User Data")),
	browserCache("Vivaldi", join(localAppData, "Vivaldi", "User Data")),
	browserCache("Opera", join(roamingAppData, "Opera Software", "Opera Stable")),
	browserFirefoxCache(),

	// ---- Coverage expansion: app caches, game launchers, GPU shader caches,
	// dev-tool caches (path inventory cross-checked against the Mole windows
	// branch; rules are our own with DEFENESTRATE guard semantics). ----

	{
		Category: "App caches", Label: "VS Code cache",
		Path:          join(roamingAppData, "Code"),
		Patterns:      []string{filepath.Join("Cache*"), filepath.Join("CachedData"), filepath.Join("Code Cache"), filepath.Join("GPUCache")},
		SkipIfRunning: []string{"Code.exe"}, Risk: Low,
	},
	{
		Category: "App caches", Label: "Slack cache",
		Path:          join(roamingAppData, "Slack"),
		Patterns:      []string{filepath.Join("Cache*"), filepath.Join("Service Worker", "CacheStorage"), filepath.Join("GPUCache")},
		SkipIfRunning: []string{"slack.exe"}, Risk: Low,
	},
	{
		Category: "App caches", Label: "Discord cache",
		Path:          join(roamingAppData, "discord"),
		Patterns:      []string{filepath.Join("Cache*"), filepath.Join("Code Cache"), filepath.Join("GPUCache")},
		SkipIfRunning: []string{"Discord.exe"}, Risk: Low,
	},
	{
		Category: "App caches", Label: "Zoom logs and cache",
		Path:          join(roamingAppData, "Zoom"),
		Patterns:      []string{filepath.Join("logs"), filepath.Join("cache")},
		SkipIfRunning: []string{"Zoom.exe"}, Risk: Medium,
	},
	{
		Category: "App caches", Label: "JetBrains caches and logs",
		Path:          join(localAppData, "JetBrains"),
		Patterns:      []string{filepath.Join("*", "caches"), filepath.Join("*", "log")},
		SkipIfRunning: []string{"idea64.exe", "webstorm64.exe", "pycharm64.exe", "rider64.exe"}, Risk: Low,
	},
	{
		Category: "Game launchers", Label: "Steam HTML cache",
		Path: join(localAppData, "Steam", "htmlcache"), TrustContents: true, Risk: Low,
	},
	{
		Category: "Game launchers", Label: "Epic Games Launcher web cache",
		Path:          join(localAppData, "EpicGamesLauncher", "Saved"),
		Patterns:      []string{filepath.Join("webcache*"), filepath.Join("Logs")},
		SkipIfRunning: []string{"EpicGamesLauncher.exe"}, Risk: Low,
	},
	{
		Category: "Game launchers", Label: "Battle.net cache",
		Path:          join(env("ProgramData"), "Battle.net", "Agent"),
		Patterns:      []string{filepath.Join("cache"), filepath.Join("data", "cache")},
		SkipIfRunning: []string{"Agent.exe", "Battle.net.exe"}, Risk: Low,
	},
	{
		Category: "Game launchers", Label: "GOG Galaxy web cache",
		Path:          join(localAppData, "GOG.com", "Galaxy"),
		Patterns:      []string{filepath.Join("webcache*"), filepath.Join("logs")},
		SkipIfRunning: []string{"GalaxyClient.exe"}, Risk: Low,
	},
	devCache("NVIDIA GLShader cache", join(localAppData, "NVIDIA", "GLCache")),
	devCache("AMD shader caches", join(localAppData, "AMD")),
	devCache("Intel shader cache", join(localAppData, "Intel", "ShaderCache")),
	devCache("DirectX D3DSCache", join(localAppData, "D3DSCache")),
	devCache("pnpm store", join(localAppData, "pnpm", "store")),
	devCache("Yarn classic cache", join(localAppData, "Yarn", "Cache")),
	devCache("Bun install cache", join(env("USERPROFILE"), ".bun", "install", "cache")),
	devCache("node-gyp headers cache", join(localAppData, "node-gyp", "Cache")),
	devCache("NuGet v3 http cache", join(localAppData, "NuGet", "v3-cache")),
	devCache("Poetry cache", join(localAppData, "pypoetry", "Cache")),
	devCache("electron builder cache", join(localAppData, "electron", "Cache")),
}

func browserCache(name string, userData func() string) Target {
	// Default-profile layout shared by all Chromium browsers. Only cache-like
	// subdirectories are matched; nothing that holds identities is touched.
	return Target{
		Category: "Browser caches",
		Label:    name + " cache",
		Path:     func() string { return userData() },
		Patterns: []string{
			filepath.Join("*", "Cache"),
			filepath.Join("*", "Code Cache"),
			filepath.Join("*", "GPUCache"),
			filepath.Join("*", "Service Worker", "CacheStorage"),
			filepath.Join("*", "Service Worker", "ScriptCache"),
		},
		SkipIfRunning: browserProcess(name),
		Risk:          Low,
	}
}

func browserProcess(name string) []string {
	switch name {
	case "Chrome":
		return []string{"chrome.exe"}
	case "Edge":
		return []string{"msedge.exe"}
	case "Brave":
		return []string{"brave.exe"}
	case "Vivaldi":
		return []string{"vivaldi.exe"}
	default:
		return nil
	}
}

// devCache declares an owner-documented, fully-regenerable package cache.
// TrustContents is the cited exception to the hold-back law: npm/pip/yarn/
// NuGet all document that removing these directories only costs re-downloads.
func devCache(label string, path func() string) Target {
	return Target{
		Category:      "Developer caches",
		Label:         label,
		Path:          path,
		TrustContents: true,
		Risk:          Low,
	}
}

func browserFirefoxCache() Target {
	// Firefox profiles live under %LOCALAPPDATA%\Mozilla\Firefox\Profiles\<rand>
	return Target{
		Category:      "Browser caches",
		Label:         "Firefox cache",
		Path:          join(localAppData, "Mozilla", "Firefox", "Profiles"),
		Patterns:      []string{filepath.Join("*", "cache2")},
		SkipIfRunning: []string{"firefox.exe"},
		Risk:          Low,
	}
}
