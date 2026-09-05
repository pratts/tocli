package actions

import (
	tea "github.com/charmbracelet/bubbletea"
	"testing"
)

func TestPicker_SelectsItemOnEnter(t *testing.T) {
	items := []TorrentItem{
		{ID: "aaa", Name: "Alpha", Status: "running"},
		{ID: "bbb", Name: "Beta", Status: "paused"},
	}
	m := NewPicker("pick one", items, "")
	m, _ = update(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	m, _ = update(m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.Cancelled {
		t.Fatal("did not expect Cancelled")
	}
	if m.Selected == nil {
		t.Fatal("expected a selection")
	}
	if m.Selected.ID != "aaa" {
		t.Fatalf("selected id = %q, want %q (first item, default cursor position)", m.Selected.ID, "aaa")
	}
}

func TestPicker_EscCancels(t *testing.T) {
	items := []TorrentItem{{ID: "aaa", Name: "Alpha", Status: "running"}}
	m := NewPicker("pick one", items, "")

	m, cmd := update(m, tea.KeyMsg{Type: tea.KeyEsc})

	if !m.Cancelled {
		t.Fatal("expected Cancelled")
	}
	if m.Selected != nil {
		t.Fatal("expected no selection")
	}
	if cmd == nil {
		t.Fatal("expected a quit command")
	}
}

func TestPicker_EmptyListShowsMessageAndExitsOnAnyKey(t *testing.T) {
	m := NewPicker("pick one", nil, "nothing to pick")

	if m.View() == "" {
		t.Fatal("expected a non-empty view for the empty state")
	}

	_, cmd := update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected any keypress to quit out of the empty picker")
	}
}

// update is a small helper so tests read as
// `m, cmd := update(m, msg)` instead of repeating the tea.Model type
// assertion at every call site.
func update(m Picker, msg tea.Msg) (Picker, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(Picker), cmd
}
