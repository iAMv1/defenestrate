// Package safety — protection_data.go
// Data-only: identities whose data must NEVER be recycled even when they sit
// inside an otherwise-eligible directory. Logic lives in safety.go.
//
// Classification principle (recovery contract, not vibes): a path is
// protected when its contents are AUTHORED STATE (passwords, sessions,
// settings, source, models the user trained or downloaded deliberately) as
// opposed to REGENERABLE artifacts (compilers' caches, thumbnails, update
// downloads). When both apply, protection wins.
package safety

import (
	"path/filepath"
	"strings"
)

// ProtectedDirNames matches directory names (case-insensitive, whole-name)
// anywhere in the tree. These hold authored state for almost every user.
var protectedDirNames = []string{
	// Password managers & secrets
	"1password", "bitDEFENESTRATE", "keepass", "keepassxc", "lastpass", "dashlane",
	".ssh", ".gnupg", ".gpg", "keybase",
	// Credential vaults / auth state
	"credentials", "identitycache",
}

// ProtectedPathFragments matches case-insensitive SUBSTRINGS. Used sparingly:
// browser profile DATA beyond caches (logins, extensions), and developer
// identity state. Cache subdirs inside these remain reachable through rule
// patterns that target exact cache leaf names.
var protectedPathFragments = []string{
	// Browser profile data (NOT cache leaves — those are separate rules)
	`\user data\default\login data`,
	`\user data\default\web data`,
	`\user data\default\extensions`,
	`\user data\default\network`,
	`\user data\local state`,
	`\profiles\`, // Firefox per-profile roots beyond cache2
	// Developer identity / machine state
	`\.git-credentials`, `\.netrc`, `\.npmrc`, `\.pypirc`, `\.docker\config.json`,
	`\.aws`, `\.azure`, `\.kube`, `\.config\gh`,
}

// HardVendorNames are display-name tokens that must never drive deletion of a
// folder by similarity. Kept here (not in apps) so policy lives with policy.
var hardVendorNames = []string{
	"microsoft", "google", "apple", "adobe", "intel", "nvidia", "amazon",
}

// ---------------------------------------------------------------------------
// Deletion classification: NEVER delete what you cannot classify.
//
// Every candidate file gets one of three verdicts:
//   - ClassKnownJunk   — extension is on the regenerable-junk list (or the
//     rule named an exact pattern like thumbcache_*.db). Auto-recyclable.
//   - ClassExecutable  — executables, installers, scripts, disk images.
//     NEVER auto-recycled from broad rules; surfaced for review only. A
//     .exe sitting in Temp might be a portable tool someone uses.
//   - ClassUnknown     — no extension or not on either list (databases,
//     proprietary formats, anything we don't understand). Review only.
// ---------------------------------------------------------------------------

type FileClass int

const (
	ClassKnownJunk FileClass = iota
	ClassExecutable
	ClassUnknown
)

// knownJunkExts: regenerable artifacts by extension. Deliberately SHORT —
// when in doubt it's Unknown, and Unknown is never auto-deleted.
var knownJunkExts = map[string]bool{
	".tmp": true, ".temp": true, ".log": true, ".etl": true, ".evtx-parsed": true,
	".dmp": true, ".mdmp": true, ".hdmp": true,
	".old": true, ".bak": true, ".cache": true, ".thm": true,
	".thumb": true, ".db-wal": true, ".db-shm": true, ".tmp~": true,
}

// executableLikeExts: anything that can run or be run later, plus installer
// and disk-image containers. Held for review in every broad rule.
var executableLikeExts = map[string]bool{
	".exe": true, ".dll": true, ".msi": true, ".msix": true, ".msp": true,
	".bat": true, ".cmd": true, ".ps1": true, ".vbs": true, ".js": true,
	".jar": true, ".scr": true, ".com": true, ".apk": true, ".appx": true,
	".msixbundle": true, ".iso": true, ".img": true, ".vhdx": true,
	".reg": true, ".inf": true,
}

// ClassifyFile returns the deletion verdict for one file path.
func ClassifyFile(path string) FileClass {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return ClassUnknown // no extension = we don't know what it is
	}
	if knownJunkExts[ext] {
		return ClassKnownJunk
	}
	if executableLikeExts[ext] {
		return ClassExecutable
	}
	return ClassUnknown
}

func containsProtectedName(path string) bool {
	lower := strings.ToLower(path)
	for _, seg := range strings.Split(lower, `\`) {
		for _, p := range protectedDirNames {
			if seg == p {
				return true
			}
		}
	}
	return false
}

func containsProtectedFragment(path string) bool {
	lower := strings.ToLower(path)
	for _, f := range protectedPathFragments {
		if strings.Contains(lower, f) {
			return true
		}
	}
	return false
}
