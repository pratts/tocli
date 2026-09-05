package actions

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pratts/tocli/internal/tui"
)

// MenuChoice is one row in a Menu.
type MenuChoice struct {
	Label string
	Value string
}

// Menu is a small fixed-choice list (not backed by bubbles/list, which is
// built for browsing/filtering larger collections -- this is just a
// handful of named actions, e.g. remove's "list only" vs "and downloaded
// files" vs "cancel").
type Menu struct {
	Title     string
	choices   []MenuChoice
	cursor    int
	Selected  string // choices[i].Value once chosen
	Cancelled bool
}

func NewMenu(title string, choices []MenuChoice) Menu {
	return Menu{Title: title, choices: choices}
}

// RemoveMenu is the menu shown after picking a torrent to remove, matching
// the existing `--with-data` distinction from the direct CLI command.
func RemoveMenu(torrentName string) Menu {
	return NewMenu("Remove "+torrentName+"?", []MenuChoice{
		{Label: "Remove from list only", Value: "list-only"},
		{Label: "Remove torrent and downloaded files", Value: "with-data"},
		{Label: "Cancel", Value: ""},
	})
}

func (m Menu) Init() tea.Cmd { return nil }

func (m Menu) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.choices)-1 {
			m.cursor++
		}
	case "enter":
		choice := m.choices[m.cursor]
		if choice.Value == "" {
			m.Cancelled = true
		} else {
			m.Selected = choice.Value
		}
		return m, tea.Quit
	case "esc":
		m.Cancelled = true
		return m, tea.Quit
	}
	return m, nil
}

func (m Menu) View() string {
	var b strings.Builder
	b.WriteString(tui.TitleStyle.Render(m.Title))
	b.WriteString("\n\n")
	for i, c := range m.choices {
		cursor := "  "
		label := c.Label
		if i == m.cursor {
			cursor = "> "
			label = tui.SelectedStyle.Render(label)
		}
		b.WriteString(cursor + label + "\n")
	}
	b.WriteString("\n" + tui.Footer("up/down", "move", "enter", "select", "esc", "cancel"))
	return b.String()
}
