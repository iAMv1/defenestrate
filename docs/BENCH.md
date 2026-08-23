# DEFENESTRATE vs Mole (windows branch) - measured head-to-head

Date: 2026-08-23 - same machine, same disk state, back-to-back runs.
Mole reference: github.com/tw93/Mole `windows` branch @ 0f13ad2 (v1.30.0),
invoked through its own entrypoint (`mole.ps1 clean --dry-run`).
DEFENESTRATE: local build v0.2.0 (`clean --dry-run`).

## Results

| Metric | DEFENESTRATE | Mole (windows) |
|---|---|---|
| Clean dry-run wall time | **38 s** | 121 s |
| Reclaimable reported | **3.3 GB** (+2.8 GB held-for-review surfaced) | "No significant reclaimable space detected" |
| Clean rule categories | 29 rows in one data table | ~46 functions across 5 modules |
| Recycle batches (8x) | persistent engine, 371 ms | per-call spawn model |
| Tree measurement engine | compiled Go walker | PowerShell interpreter |
| Deletion funnel | machine-enforced (TestDeletionFunnelEnforced) | convention + docs |

## Why the split

Spawn-bound work (recycle, registry, system ops) lives in ONE persistent
PowerShell session (-95% vs spawn-per-batch). CPU-bound work (walking
hundreds of thousands of files) stays in compiled Go - a like-for-like PS
sizing script measured 23 s where the Go walker needs seconds. Mole's 86%
shell ratio is a portability choice, not a speed feature; its own dry-run on
this machine demonstrates the interpreter tax.

## Attribution

Category/path inventory cross-checked against Mole's windows branch to close
coverage gaps (app caches, game launchers, GPU shader caches, dev stores).
All rules are original implementations with DEFENESTRATE guard semantics;
Mole is GPL-3 and no code was copied.
