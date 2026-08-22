// Package whitelist manages the user's protected-targets list: entries that
// `DEFENESTRATE clean` must skip even when a rule matches. One entry per line in
// %LOCALAPPDATA%\DEFENESTRATE\whitelist.txt; lines may be rule labels ("Browser
// caches") or absolute path prefixes.
package whitelist

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iAMv1/defenestrate/internal/ui"
)

func path() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
	}
	return filepath.Join(base, "DEFENESTRATE", "whitelist.txt")
}

// Load returns all entries (lowercased for matching).
func Load() []string {
	f, err := os.Open(path())
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, strings.ToLower(line))
	}
	return out
}

// Add appends an entry, deduped.
func Add(entry string) (added bool) {
	entry = strings.ToLower(strings.TrimSpace(entry))
	if entry == "" {
		return false
	}
	existing := Load()
	for _, e := range existing {
		if e == entry {
			return false
		}
	}
	fh, err := os.OpenFile(path(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false
	}
	defer fh.Close()
	fmt.Fprintln(fh, strings.ToLower(strings.TrimSpace(entry)))
	return true
}

// Remove deletes every exact match; returns how many were removed.
func Remove(entry string) int {
	entry = strings.ToLower(strings.TrimSpace(entry))
	lines := Load()
	var kept []string
	removed := 0
	for _, l := range lines {
		if l == entry {
			removed++
			continue
		}
		kept = append(kept, l)
	}
	if removed > 0 {
		writeAll(kept)
	}
	return removed
}

func writeAll(entries []string) {
	os.MkdirAll(filepath.Dir(path()), 0o755)
	fh, err := os.OpenFile(path(), os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer fh.Close()
	for _, e := range entries {
		fmt.Fprintln(fh, e)
	}
}

// Matches reports whether a finding label or any candidate path is covered.
// Label match is exact-insensitive; path match is prefix-based.
func Matches(label string, paths []string, entries []string) bool {
	lowerLabel := strings.ToLower(label)
	for _, e := range entries {
		if e == lowerLabel {
			return true
		}
		for _, p := range paths {
			lp := strings.ToLower(p)
			if lp == e || strings.HasPrefix(lp, e+string(filepath.Separator)) {
				return true
			}
		}
	}
	return false
}

// Run implements `DEFENESTRATE clean --whitelist [add|remove|list] [value]`.
func Run(args []string) error {
	if len(args) == 0 || args[0] == "list" {
		entries := Load()
		if len(entries) == 0 {
			fmt.Println("Whitelist is empty — clean touches everything its rules allow.")
			fmt.Println(ui.StyleDim.Render("Add: DEFENESTRATE clean --whitelist add \"<label or path>\""))
			return nil
		}
		fmt.Println(ui.Title("Protected targets"))
		for _, e := range entries {
			fmt.Println("  " + e)
		}
		return nil
	}
	cmd := strings.ToLower(args[0])
	switch cmd {
	case "add":
		if len(args) < 2 {
			return fmt.Errorf("usage: whitelist add \"<label or path>\"")
		}
		v := strings.Join(args[1:], " ")
		if Add(v) {
			fmt.Println(ui.Good("Added: " + v))
		} else {
			fmt.Println("Already whitelisted.")
		}
	case "remove", "rm":
		if len(args) < 2 {
			return fmt.Errorf("usage: whitelist remove \"<entry>\"")
		}
		v := strings.Join(args[1:], " ")
		if n := Remove(v); n > 0 {
			fmt.Println(ui.Good(fmt.Sprintf("Removed %d entr%s.", n, plural(n))))
		} else {
			fmt.Println("No matching entry.")
		}
	default:
		return fmt.Errorf("unknown whitelist action %q (use add/remove/list)", cmd)
	}
	return nil
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
