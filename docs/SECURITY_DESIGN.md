# DEFENESTRATE — Security Design (skeleton)

Layered deletion contract, modeled on Mole's design but Windows-native.

## Layer 1 — Single funnel
Every filesystem mutation routes through `safety.Recycle`. No other package
may call `os.Remove*`/`os.RemoveAll`/`DeleteFile*` — enforced by
`TestDeletionFunnelEnforced` (source scan, fails CI).

## Layer 2 — Guard evaluation order (`safety.Check`)
1. Drive roots refused.
2. `exactProtected`: profile/appdata/appdata-roaming/systemdrive roots.
3. `guardRoots` prefix refusal: WINDIR, Program Files ×2, ProgramData —
   except `safeZones` whose CONTENTS are cleanable (Windows\Temp,
   SoftwareDistribution\Download, Logs, Minidump, LiveKernelReports).
4. Authored-state protection (`protection_data.go`): `.ssh`, password
   managers, browser Login Data/extensions, cloud-credential fragments —
   refused even inside eligible trees. Classification principle: recovery
   contract (authored vs regenerable), never directory vibes.

## Layer 3 — Uninstaller evidence contract
Leftover recycling requires REGISTRY EVIDENCE: InstallLocation, the vendor
uninstaller's parent dir, or the DisplayIcon resource dir. Name-token matches
are REVIEW-ONLY output; registry keys match exact subkey names only.

## Layer 4 — Traversal policy
Walkers never descend reparse points (`safety.IsReparsePoint`); a junction is
at most itself, never its target's contents.

## Known limitations
- HKLM registry cleanup needs elevation (v1 = HKCU only).
- safeZones contents cleaning still respects per-rule MinAgeDays.
- Review suggestions can contain false positives by design; they are printed,
  sized, and never executed.
