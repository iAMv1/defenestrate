package safety

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestHistoryJSONRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LOCALAPPDATA", tmp)

	Logf("recycle", filepath.Join(tmp, "a.txt"), 12)
	Logf("[dry-run] would recycle", filepath.Join(tmp, "b.exe"), 0)

	entries, err := HistoryJSON()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}
	if entries[0].Action != "recycle" || entries[0].Bytes != 12 {
		t.Errorf("entry0 = %+v", entries[0])
	}
	if !strings.HasPrefix(entries[1].Action, "[dry-run]") {
		t.Errorf("dry-run tag lost: %+v", entries[1])
	}
}
