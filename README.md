# DEFENESTRATE 🧹

**Deep clean, uninstall, analyze and monitor Windows — from one terminal binary.**
CleanMyMac × AppCleaner × DaisyDisk for the command line, with an interactive
menu simple enough for non-technical users. Inspired by [tw93/mole](https://github.com/tw93/mole) (macOS).

```
DEFENESTRATE                     Interactive menu
DEFENESTRATE clean --dry-run     Preview gigabytes of reclaimable caches
DEFENESTRATE uninstall appname   Remove a program + every leftover
DEFENESTRATE analyze C:\         Visual tree of what eats your disk
DEFENESTRATE status --watch      Live CPU / memory / disk dashboard
DEFENESTRATE history             Audit log of everything DEFENESTRATE did
```

## Modes

| Mode | What it does |
|---|---|
| **clean** | System temp, Windows Update download cache, Delivery Optimization, crash dumps, old CBS logs, thumbnails, Chromium/Firefox browser caches, developer package caches (npm/pip/yarn/NuGet) — grouped by category with sizes, mole-style ✓ report |
| **uninstall** | Registry programs **+ Store apps**: vendor quiet uninstaller or `Remove-AppxPackage`, then recycles **registry-evidence** locations; name-similar folders printed review-only |
| **analyze** | Concurrent walk (16 workers), largest-children bar chart, ≥64 MB large-file list, `--json` output, `--delete` recycles via Recycle Bin |
| **status** | Health-scored dashboard: CPU total + per-core bars, RAM, per-drive used/free/read/write rates; `--watch` for 1 s refresh |
| **optimize** | Bounded maintenance tasks — DNS/ARP flush, Recycle Bin empty (gated), Search service restart, perf-counter rebuild — plus pending-reboot & uptime report. Admin tasks skip without elevation; never auto-elevates |
| **update** | Self-update from GitHub Releases: semver compare → download → atomic swap with rollback. Channel configured at build time via ldflags; unconfigured builds fail closed |

## The TUIs

Every mode has a real interactive surface:

- **Menu hub** — arrow/vim keys, alt-screen navigation, dry-run badge that propagates into every child action.
- **Analyze** — drill-down tree with colored capacity bars, large-items pane (`tab`), **squarified treemap (`t`)**, `d` recycles behind a y/N confirm, `r` rescans.
- **Status** — bento cards (CPU per-core / Memory / Hardware), **60-second sparklines**, per-drive IO rates, battery · thermal · GPU when present, tri-colored health score; `p` pauses, `c` cycles core rows.
- **Clean** — category checklist (Low-risk pre-selected, Medium opt-in), live held-for-review totals, sequential recycling progress.
- **Uninstall** — type-to-filter list including Store apps; plan screen shows every evidence folder with size and inline ⚠ flags for protected paths.

CLI stays script-first: `--json` on analyze/status/history, plain output when piped.

## The classification law

**DEFENESTRATE never deletes what it cannot classify.** Every file in a broad rule
gets a verdict:

- `KnownJunk` — short, deliberate extension list (`.tmp .log .dmp…`) → recyclable
- `Executable` — `.exe .dll .msi .iso .ps1…` → **held for review**
- `Unknown` — no/unknown extension → **held for review**

Held items are shown per category with sizes and are never touched by `clean`.
The only exception is `TrustContents`, which requires the rule to cite an
owner-documented reset contract (npm/pip/yarn/NuGet caches) — see
`internal/rules/rules.go`. Process guards are tri-state: running skips,
**unknown state denies**.

## Safety model

- **`--dry-run` everywhere**: preview exact paths + sizes, touch nothing.
- **Recycle Bin only**: deletions use `Microsoft.VisualBasic.FileIO.FileSystem`
  → restorable. No permanent-delete code path exists in v1.
- **Guard list**: `%WINDIR%`, Program Files, ProgramData and the profile/appdata
  roots are refused outright, except explicit safe zones (`Windows\Temp`,
  `SoftwareDistribution\Download`, `Windows\Logs`, `Minidump`) whose *contents*
  may be cleaned, never the folders.
- **Authored-state protection**: `.ssh`, password managers, browser Login Data,
  cloud-credential fragments are refused even inside eligible trees.
- **Process locks honored — tri-state**: browser caches skip while the browser
  runs; if process enumeration itself fails, cleanup is DENIED (unknown ≠ safe).
- **Confirmation gate**: uninstall shows every leftover candidate with its size
  and asks before touching anything; protected entries are flagged inline.
- **Audit trail**: every mutation lands in
  `%LOCALAPPDATA%\DEFENESTRATE\operations.log`; review with `DEFENESTRATE history`.

## No hardcoded anything

- Every cleanable path derives from environment variables (`WINDIR`,
  `LOCALAPPDATA`, …) or live registry enumeration — no user names, no drive
  letters baked into rules (`internal/rules/rules.go` is pure data).
- Version stamps come from `-ldflags "-X main.version=…"` at build time.
- `.github/workflows/publish.yml` derives the toolchain from `go.mod`,
  artifact names from the pushed tag (`${{ github.ref_name }}`), and platforms
  from the strategy matrix — no version literals anywhere.

## Build

```powershell
go build -trimpath -ldflags "-s -w -X main.version=$(git describe --tags --always)" -o DEFENESTRATE.exe
```

Requires Go ≥ 1.22. Runtime deps: `bubbletea` (menu), `gopsutil` (monitor),
`golang.org/x/sys` (registry/disks). Everything else is stdlib.

## License

MIT. Docs: [English](README.md) · [简体中文](docs/README.zh-CN.md)
Agent surface: [docs/skills/DEFENESTRATE-cli/SKILL.md](docs/skills/DEFENESTRATE-cli/SKILL.md) ·
Security: [SECURITY_DESIGN](docs/SECURITY_DESIGN.md) · [SECURITY_AUDIT](docs/SECURITY_AUDIT.md) ·
Completion: dot-source `completion/DEFENESTRATE.ps1`

## Known limitations (v1)

- Disk read/write rates can read 0 on some drives whose gopsutil counter names
  don't match the drive letter (per-drive matching is best-effort).
- UWP/Store apps are not listed by the uninstaller yet (registry-only).
- Leftover matching shows every candidate folder with its size and asks once;
  false positives are possible — review the list; guards still refuse
  protected paths at execution time.
- The interactive menu requires a real terminal (no pipe).
