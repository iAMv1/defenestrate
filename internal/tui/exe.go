package tui

import "os"

// osExecutable returns this binary's path so menu items can re-invoke
// subcommands with a real terminal attached.
func osExecutable() string {
	exe, err := os.Executable()
	if err != nil {
		return "DEFENESTRATE"
	}
	return exe
}
