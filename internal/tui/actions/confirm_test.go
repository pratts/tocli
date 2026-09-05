package actions

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestConfirm_YConfirms(t *testing.T) {
	m := NewConfirm("are you sure?")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = updated.(Confirm)

	if !m.Confirmed {
		t.Fatal("expected Confirmed")
	}
	if cmd == nil {
		t.Fatal("expected a quit command")
	}
}

func TestConfirm_NCancels(t *testing.T) {
	m := NewConfirm("are you sure?")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = updated.(Confirm)

	if !m.Cancelled {
		t.Fatal("expected Cancelled")
	}
}

func TestAck_AnyKeyConfirms(t *testing.T) {
	m := NewAck("done")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updated.(Confirm)

	if !m.Confirmed {
		t.Fatal("expected any keypress to confirm an ack dialog")
	}
	if cmd == nil {
		t.Fatal("expected a quit command")
	}
}
