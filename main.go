// Command DEFENESTRATE is an all-in-one Windows deep-clean, uninstaller, disk
// analyzer and live monitor for the terminal — CleanMyMac × AppCleaner ×
// DaisyDisk for Windows, inspired by tw93/mole.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/iAMv1/defenestrate/internal/analyze"
	"github.com/iAMv1/defenestrate/internal/apps"
	"github.com/iAMv1/defenestrate/internal/clean"
	"github.com/iAMv1/defenestrate/internal/hud"
	"github.com/iAMv1/defenestrate/internal/installer"
	"github.com/iAMv1/defenestrate/internal/monitor"
	"github.com/iAMv1/defenestrate/internal/optimize"
	"github.com/iAMv1/defenestrate/internal/purge"
	"github.com/iAMv1/defenestrate/internal/safety"
	"github.com/iAMv1/defenestrate/internal/tui"
	"github.com/iAMv1/defenestrate/internal/update"
)

// version is injected at build time:
//
//	go build -ldflags "-X main.version=v1.2.3"
//
// Never hardcode a release number here; CI passes the git tag.
var version = "dev"

const usage = `DEFENESTRATE — deep clean, uninstall, analyze and monitor Windows.

Usage:
  DEFENESTRATE                     Interactive menu (GUI on top of the TUI)
  DEFENESTRATE clean [--dry-run]   Deep clean: caches, browser data, old logs
  DEFENESTRATE uninstall [name] [--dry-run]
                             Smart uninstaller + leftover sweep
  DEFENESTRATE analyze [path] [--json] [--top N]
                             Disk space analyzer with visual tree
  DEFENESTRATE status [--watch] [--json]
                             Live CPU / memory / disk dashboard
  DEFENESTRATE optimize [--all] [--dry-run]
                             Bounded maintenance tasks (DNS, ARP, Recycle Bin)
  DEFENESTRATE purge [--all] [--dry-run] [--paths]
                             Remove project build artifacts (node_modules, target…)
  DEFENESTRATE installer [--dry-run]
                             Find and remove leftover installers
  DEFENESTRATE hud                 System-tray live CPU/RAM widget
  DEFENESTRATE update              Self-update from the configured release channel
  DEFENESTRATE history             Show the operations log
  DEFENESTRATE version             Print version

Safety:
  --dry-run                  Preview what would be deleted, change nothing
  Everything deletable goes to the Recycle Bin, never permanent delete.
`

func main() {
	args := os.Args[1:]
	// Global flags may appear anywhere.
	filtered := args[:0]
	for _, a := range args {
		if a == "--dry-run" || a == "-n" {
			safety.SetDryRun(true)
			continue
		}
		filtered = append(filtered, a)
	}
	args = filtered

	cmd := "menu"
	if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}

	var err error
	switch cmd {
	case "clean":
		err = clean.Run(args)
	case "uninstall":
		if isTTY() && len(args) == 0 && !hasFlag(args, "--dry-run") {
			err = tui.RunUninstallTUI()
		} else {
			err = apps.Run(args)
		}
	case "analyze", "analyse":
		if isTTY() && !hasFlag(args, "--json") && !hasFlag(args, "--delete") {
			target := "."
			for _, a := range args {
				if !strings.HasPrefix(a, "-") {
					target = a
				}
			}
			err = tui.RunAnalyzeTUI(target)
		} else {
			err = analyze.Run(args)
		}
	case "status":
		if isTTY() && !hasFlag(args, "--json") {
			err = tui.RunStatusTUI()
		} else {
			err = monitor.Run(args)
		}
	case "optimize":
		err = optimize.Run(args)
	case "installer":
		err = installer.Run(args)
	case "purge":
		err = purge.Run(args)
	case "history":
		if hasFlag(args, "--json") {			entries, jerr := safety.HistoryJSON()
			if jerr != nil {
				err = jerr
				break
			}
			out := struct {
				Entries []safety.OpEntry `json:"entries"`
				Count   int              `json:"count"`
			}{entries, len(entries)}
			jsonOut, _ := json.MarshalIndent(out, "", "  ")
			fmt.Println(string(jsonOut))
		} else {
			err = safety.PrintHistory()
		}
	case "version", "--version", "-v":
		fmt.Println("DEFENESTRATE", version)
	case "hud":
		err = hud.Run()
	case "update":
		err = update.Run(version, func(newVersion string) {
			fmt.Println("updated to", newVersion)
			safety.Logf("self-update", newVersion, 0)
		})
	case "menu", "--help", "-h", "help":
		if cmd == "menu" && !isTTY() {
			fmt.Fprint(os.Stderr, usage)
			os.Exit(2)
		}
		err = tui.Menu(version)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}
