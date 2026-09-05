package dashboard

import (
	"testing"

	"github.com/pratts/tocli/internal/store"
)

func TestColumnsForWidth_NameGetsLeftoverSpace(t *testing.T) {
	narrow := columnsForWidth(60)
	wide := columnsForWidth(160)

	var narrowName, wideName int
	for _, c := range narrow {
		if c.Title == "NAME" {
			narrowName = c.Width
		}
	}
	for _, c := range wide {
		if c.Title == "NAME" {
			wideName = c.Width
		}
	}
	if narrowName < 16 {
		t.Errorf("narrow NAME width = %d, want at least the 16-column floor", narrowName)
	}
	if wideName <= narrowName {
		t.Errorf("wide NAME width (%d) should exceed narrow NAME width (%d)", wideName, narrowName)
	}
}

func TestRowsToTable_BrokenRowShowsErrorDistinctly(t *testing.T) {
	rows := []rowData{
		{ID: "aaa", Name: "Good Torrent", Status: store.StatusRunning, Percent: "50.0%", Rate: "1.0 MiB/s", Peers: "2/5"},
		{ID: "bbb", Broken: true},
	}

	out := rowsToTable(rows)
	if len(out) != 2 {
		t.Fatalf("got %d rows, want 2", len(out))
	}
	for _, r := range out {
		if len(r) != 6 {
			t.Errorf("row %v has %d columns, want 6 (matching columnsForWidth)", r, len(r))
		}
	}
	if out[1][0] != "bbb" || out[1][1] != "error: unreadable config" {
		t.Errorf("broken row = %v, want id + distinct error message", out[1])
	}
}
