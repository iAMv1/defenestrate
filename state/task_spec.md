# DEFENESTRATE CLI — P0 Safety Spine

Status: COMPLETE (all milestones shipped + verified)

## Milestones

1. [x] Evidence-based leftover matching — registry fields only
   (InstallLocation / uninstaller parent / DisplayIcon dir); name tokens are
   review-only output; registry keys exact-name match. Regression tests pin
   the Roaming-Microsoft incident class.
2. [x] Protection database — protection_data.go (data-only): authored-state
   names (.ssh, password managers) + fragments (Login Data, extensions,
   .aws/.kube/.npmrc...). Wired into safety.Check; predicate tests.
3. [x] Funnel enforcement gate — TestDeletionFunnelEnforced scans all source;
   delete calls outside internal/safety fail CI.
4. [x] Junction/symlink policy — IsReparsePoint; descent blocked in clean +
   analyze walkers.
5. [x] SECURITY_DESIGN.md (docs/) + README safety section.

## Beyond-spec additions this run

- FileClass law: KnownJunk/Executable/Unknown; unknown NEVER auto-deleted;
  broad rules hold candidates for review with sizes (user directive).
- TrustContents exception requiring owner-documented reset contract
  (npm/pip/yarn/NuGet caches cited in rules.go).
- Tri-state process guard: running skips, enumeration-failure DENIES
  (decideSkip + overridable probe, tested).
- Developer-cache category: 27.7 GB measured reclaimable on live machine.
- UWP/Store apps: Get-AppxPackage enumeration (null-DisplayName fallback
  tested), Remove-AppxPackage removal, no WindowsApps sweep.
- optimize mode: bounded task registry, admin-skip (never auto-elevate),
  destructive gating (--all + confirm), pending-reboot + uptime report.

## Validation at close

go vet=0 · go test ok ×3 packages · build=0 · live dry-runs confirm plan shape
