# DEFENESTRATE CLI — Agent Skill

Use this skill when an AI agent needs to clean, uninstall, analyze, or
monitor a Windows machine on the user's behalf using `DEFENESTRATE`.

## Prime directive

**Dry-run first, always.** Every destructive command accepts `--dry-run`.
Run it, show the user the plan (paths + sizes), and only execute after the
user approves. Never pass `-y`/`--yes` unless the user explicitly said so
for that exact action.

## Command map

| Goal | Command |
|---|---|
| See reclaimable caches | `DEFENESTRATE clean --dry-run` |
| Execute approved cleanup | `DEFENESTRATE clean` (confirm prompt appears) |
| Find an app's leftovers | `DEFENESTRATE uninstall "<name>" --dry-run` |
| Remove an app | `DEFENESTRATE uninstall "<name>"` |
| Explore disk usage | `DEFENESTRATE analyze <path> --json` |
| Recycle one specific file | `DEFENESTRATE analyze --delete "<path>"` |
| Machine snapshot | `DEFENESTRATE status --json` |
| Live metrics stream | `DEFENESTRATE status --watch --json` (NDJSON) |
| Build artifacts | `DEFENESTRATE purge --dry-run` then `DEFENESTRATE purge` |
| Leftover installers | `DEFENESTRATE installer --dry-run` then `DEFENESTRATE installer` |
| Audit trail | `DEFENESTRATE history --json` |
| Protect something | `DEFENESTRATE clean --whitelist add "<label or path>"` |

## Safety contract for agents

1. `--dry-run` output is the contract: act only on what it listed.
2. Items shown as "held for review" are NEVER deletable via `clean` — if the
   user wants them gone, use `DEFENESTRATE analyze --delete "<path>"` per item and
   say what you are deleting.
3. Whitelisted targets are skipped by design; do not work around them.
4. Store apps remove via `Remove-AppxPackage`; never touch
   `C:\Program Files\WindowsApps` manually.
5. If a command errors with "protected", that is the guard list working —
   report it, don't retry with elevated shells.
6. All deletions go to the Recycle Bin; recovery is possible until the bin is
   emptied (`DEFENESTRATE optimize` includes an explicit gated task).

## Machine-readable surfaces

- `DEFENESTRATE status --json` → health_score, cpu_percent, memory, disks[],
  network[], processes[] (cpu_share_percent)
- `DEFENESTRATE analyze <path> --json` → entries[], large_files[], total_size
- `DEFENESTRATE history --json` → entries[{ts,action,path,bytes}]

## When NOT to use DEFENESTRATE

- Antivirus/malware removal (not a security scanner)
- File recovery (cleanup is permanent once the Recycle Bin empties)
- Anything outside Windows
