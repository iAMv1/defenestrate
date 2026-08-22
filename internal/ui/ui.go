// Package ui centralizes presentation and small shared helpers.
package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/shirou/gopsutil/v3/process"
	"golang.org/x/sys/windows"
)

var Check = Good("✓")

func Title(s string) string   { return bold(s) }
func Section(s string) string { return "\n" + bold(s) }

func bold(s string) string { return "\033[1m" + s + "\033[0m" }

// Bold renders bold text.
func Bold(s string) string { return bold(s) }
func Good(s string) string { return "\033[32m" + s + "\033[0m" }
func Warn(s string) string { return "\033[33m" + s + "\033[0m" }
func Bad(args ...any) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = fmt.Sprint(a)
	}
	return "\033[31m" + strings.Join(parts, " ") + "\033[0m"
}
func Dim(s string) string { return "\033[2m" + s + "\033[0m" }

func If(cond bool, s string) string {
	if cond {
		return s
	}
	return ""
}

func Rule() string { return strings.Repeat("=", 68) }

// HumanBytes renders binary units with one decimal: 1.2 MB.
func HumanBytes(b int64) string { return HumanBytesU(uint64(b)) }

func HumanBytesU(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// Bar renders ████████░░░░ style fill bars.
func Bar(frac float64, width int) string {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	filled := int(frac * float64(width))
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// FreeBytes returns free bytes on path's volume.
func FreeBytes(path string) uint64 {
	root, err := volumeRoot(path)
	if err != nil {
		return 0
	}
	var free, total uint64
	if err := windows.GetDiskFreeSpaceEx(windows.StringToUTF16Ptr(root), &free, &total, nil); err != nil {
		return 0
	}
	return free
}

// VolumeStats returns used fraction / total / free for path's volume.
func VolumeStats(path string) (usedPct float64, total, free uint64) {
	root, err := volumeRoot(path)
	if err != nil {
		return 0, 0, 0
	}
	var freeB, totalB uint64
	if err := windows.GetDiskFreeSpaceEx(windows.StringToUTF16Ptr(root), &freeB, &totalB, nil); err != nil || totalB == 0 {
		return 0, totalB, freeB
	}
	return float64(totalB-freeB) / float64(totalB), totalB, freeB
}

func volumeRoot(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	vol := filepath.VolumeName(abs)
	if vol == "" {
		return "", fmt.Errorf("no volume in %q", path)
	}
	return vol + string(filepath.Separator), nil
}

// RunningAny returns which of the named processes are alive.
func RunningAny(names []string) []string {
	hits, _ := RunningAnySafe(names)
	return hits
}

// RunningAnySafe is the tri-state probe: (hits, ok). ok=false means process
// enumeration FAILED — callers must treat that as "unknown", and unknown
// denies cleanup (we cannot prove the app is closed).
func RunningAnySafe(names []string) ([]string, bool) {
	if len(names) == 0 {
		return nil, true
	}
	want := map[string]bool{}
	for _, n := range names {
		want[strings.ToLower(n)] = true
	}
	procs, err := process.Processes()
	if err != nil {
		return nil, false // unknown state — deny
	}
	var hit []string
	for _, p := range procs {
		if n, err := p.Name(); err == nil && want[strings.ToLower(n)] {
			hit = append(hit, n)
		}
	}
	return hit, true
}
