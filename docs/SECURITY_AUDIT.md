# DEFENESTRATE Security Audit Notes

Status: v1 self-audit. This file states what DEFENESTRATE treats as security-
sensitive, the current boundaries, and known limitations. It is a living
document: every change to deletion logic must update it in the same commit.

## Threat model

DEFENESTRATE is a local maintenance tool whose worst-case failure is destroying
user data. The adversary is not (primarily) a remote attacker; it is
*incorrect classification* — our own code deleting something it did not
understand. Secondary concerns: privilege confusion (admin vs user scope),
path traversal through links/junctions, and supply-chain integrity of the
update channel.

## Deletion boundary layers (see SECURITY_DESIGN.md)

1. Single funnel (`safety.Recycle`) — enforced by `TestDeletionFunnelEnforced`
   source scan; raw delete calls outside `internal/safety` fail CI unless
   carrying an inline `// SAFE:` justification for self-created scratch files.
2. Guard evaluation — drive roots, exact-protected roots (profile/appdata),
   guard prefixes (WINDIR/Program Files/ProgramData) with explicit safe zones,
   authored-state protection (`.ssh`, password managers, browser Login Data).
3. Evidence contract — uninstall leftovers require registry evidence;
   name-similarity output is review-only.
4. Traversal policy — reparse points are never descended.

## What we consider security issues

- Any deletion outside the documented boundaries
- Path validation bypasses (traversal, junction escape, case tricks)
- Whitelist/protection bypasses
- Update-channel integrity (asset substitution, downgrade attacks)
- Privilege escalation via spawned processes

## Known limitations (honest)

- HKLM registry cleanup requires elevation; v1 is HKCU-only by design.
- The protection database is far smaller than Mole's curated list; it grows
  with incident reports. Treat unknown app-data folders as untrusted.
- `TrustContents` rules trust vendor documentation; a vendor changing cache
  semantics invalidates the rule until reviewed.
- The update flow verifies size but not yet cryptographic signatures; the
  checksums.txt published per release is advisory until signature checking
  lands.
- Junction handling is skip-based; a malicious junction planted inside a
  cleanable tree makes its target invisible to scans rather than deletable —
  fail-safe direction, but worth knowing.

## Review checklist for destructive-code PRs

1. Does every new deletion route through `safety.Recycle`?
2. Is every candidate classifiable (KnownJunk) or explicitly held?
3. Are running-process guards tri-state (unknown denies)?
4. Do tests pin the exact refusal and the exact allowance?
5. Does this change touch `protection_data.go`? If yes, justify each row
   against the recovery contract (authored vs regenerable).
