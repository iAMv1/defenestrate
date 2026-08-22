package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iAMv1/defenestrate/internal/analyze"
	"github.com/iAMv1/defenestrate/internal/monitor"
)

// stripANSI makes rendered views assertable without color noise.
func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case r == '\033':
			inEsc = true
		case inEsc:
			if r == 'm' {
				inEsc = false
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestStatusViewRendersCards(t *testing.T) {
	m := newStatusModel()
	m.snap = &monitor.Snapshot{
		Health: 91,
		CPU:    42.5,
		Cores:  8,
		Uptime: "1d 2h",
		Mem: monitor.MemStats{
			Total: 32 << 30, Used: 16 << 30, UsedPercent: 50,
		},
		Detail: monitor.CPUDetail{PerCore: []float64{10, 20}},
		Disks: []monitor.DiskStats{{
			Drive: "C:", UsedPct: 0.6, Total: 10 << 30, Free: 4 << 30,
			ReadMBs: 1.5, WriteMBs: 2.5,
		}},
	}
	out := stripANSI(m.View())
	for _, want := range []string{"DEFENESTRATE STATUS", "Health", "CPU", "Memory", "Disks", "C1", "History (60s)", "read"} {
		if !strings.Contains(out, want) {
			t.Errorf("status view missing %q\n%s", want, out)
		}
	}
}

func TestAnalyzeViewTreeAndSelection(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "big")
	os.MkdirAll(sub, 0o755)
	m := newAnalyzeModel(dir)
	m.loading = false
	m.entries = []analyze.Entry{
		{Name: "big", Path: sub, Size: 900, IsDir: true},
		{Name: "small.txt", Path: filepath.Join(dir, "small.txt"), Size: 3},
	}
	m.large = []analyze.File{{Name: "big.bin", Path: filepath.Join(dir, "big.bin"), Size: 900}}
	m.total = 903
	m.cursor = 0

	tree := stripANSI(m.View())
	if !strings.Contains(tree, "big") || !strings.Contains(tree, "small.txt") {
		t.Errorf("tree rows missing:\n%s", tree)
	}
	if !strings.Contains(tree, "▶") {
		t.Errorf("cursor marker missing:\n%s", tree)
	}

	// Large pane renders file rows.
	m.pane = 1
	large := stripANSI(m.View())
	if !strings.Contains(large, "big.bin") {
		t.Errorf("large pane missing entry:\n%s", large)
	}
}

// The confirm overlay must be visible and must gate deletion behind y.
func TestAnalyzeConfirmOverlay(t *testing.T) {
	m := newAnalyzeModel(t.TempDir())
	m.loading = false
	m.confirm = `C:\some\path`
	out := stripANSI(m.View())
	if !strings.Contains(out, "Recycle to bin") || !strings.Contains(out, "y confirm") {
		t.Errorf("confirm overlay missing:\n%s", out)
	}
}
