package tui

import (
	"strings"
	"testing"

	"github.com/iAMv1/defenestrate/internal/analyze"
)

func entriesFixture() []analyze.Entry {
	return []analyze.Entry{
		{Name: "alpha", Path: `C:\d\alpha`, Size: 5000, IsDir: true},
		{Name: "beta", Path: `C:\d\beta`, Size: 3000, IsDir: true},
		{Name: "gamma", Path: `C:\d\gamma`, Size: 1500, IsDir: true},
		{Name: "delta", Path: `C:\d\delta`, Size: 500, IsDir: true},
	}
}

func TestBuildTreeMapLaysAllReadableTiles(t *testing.T) {
	tiles := BuildTreeMap(entriesFixture(), 40, 12)
	if len(tiles) < 3 {
		t.Fatalf("expected most tiles to fit a 40x12 grid, got %d", len(tiles))
	}
	for _, tl := range tiles {
		if tl.W <= 0 || tl.H <= 0 {
			t.Errorf("degenerate tile %+v", tl)
		}
		if tl.Col+tl.W > 40 || tl.Row+tl.H > 12 {
			t.Errorf("tile escapes grid: %+v", tl)
		}
	}
}

func TestBuildTreeMapBiggestGetsRankZero(t *testing.T) {
	tiles := BuildTreeMap(entriesFixture(), 40, 12)
	if len(tiles) == 0 {
		t.Fatal("no tiles")
	}
	for _, tl := range tiles {
		if tl.Rank == 0 && tl.Item.Label != "alpha" {
			t.Errorf("rank0 tile = %s, want alpha", tl.Item.Label)
		}
	}
}

func TestRenderTreeMapFillsGridAndMarksSelection(t *testing.T) {
	tiles := BuildTreeMap(entriesFixture(), 40, 12)
	out := RenderTreeMap(tiles, 40, 12, `C:\d\beta`)
	if !strings.Contains(out, "┏") {
		t.Error("selection marker missing")
	}
	if !strings.Contains(stripANSI(out), "alpha") || !strings.Contains(stripANSI(out), "beta") {
		t.Errorf("labels missing:\n%s", out)
	}
	lines := strings.Count(strings.TrimRight(out, "\n"), "\n") + 1
	if lines > 12 {
		t.Errorf("render exceeds grid rows: %d", lines)
	}
}

func TestBuildTreeMapTinyTerminalReturnsNil(t *testing.T) {
	if tiles := BuildTreeMap(entriesFixture(), 10, 4); tiles != nil {
		t.Errorf("tiny grid must yield no map, got %d tiles", len(tiles))
	}
}
