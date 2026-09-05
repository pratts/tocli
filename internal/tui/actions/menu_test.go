package actions

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRemoveMenu_NavigateAndSelect(t *testing.T) {
	m := RemoveMenu("my-torrent")

	// Move down once: list-only (0) -> with-data (1).
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Menu)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Menu)

	if m.Cancelled {
		t.Fatal("did not expect Cancelled")
	}
	if m.Selected != "with-data" {
		t.Fatalf("selected = %q, want %q", m.Selected, "with-data")
	}
	if cmd == nil {
		t.Fatal("expected a quit command")
	}
}

func TestRemoveMenu_SelectingCancelRowSetsCancelled(t *testing.T) {
	m := RemoveMenu("my-torrent")

	// Down twice: list-only -> with-data -> cancel.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Menu)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Menu)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Menu)

	if !m.Cancelled {
		t.Fatal("expected Cancelled when the Cancel row is chosen")
	}
	if m.Selected != "" {
		t.Fatalf("Selected = %q, want empty", m.Selected)
	}
}

func TestRemoveMenu_EscCancelsDirectly(t *testing.T) {
	m := RemoveMenu("my-torrent")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Menu)

	if !m.Cancelled {
		t.Fatal("expected Cancelled")
	}
}

func TestRemoveMenu_CursorDoesNotGoOutOfBounds(t *testing.T) {
	m := RemoveMenu("my-torrent")

	// Up from the first row should stay put, not panic or wrap.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Menu)
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", m.cursor)
	}

	// Down past the last row should stop at the last row.
	for range m.choices {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(Menu)
	}
	if m.cursor != len(m.choices)-1 {
		t.Fatalf("cursor = %d, want %d", m.cursor, len(m.choices)-1)
	}
}
