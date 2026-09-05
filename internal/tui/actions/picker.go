// Package actions holds small, screen-agnostic interactive pieces reused
// across the pause/resume/remove flows and from within the dashboard: a
// torrent picker, a yes/no confirm dialog, and a fixed-choice menu (used
// for remove's "list only" vs "and downloaded files" decision). None of
// these touch internal/store or internal/engine themselves -- callers feed
// them pre-fetched data and read back a selection.
package actions

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/pratts/tocli/internal/tui"
)

// TorrentItem is one selectable row in a Picker.
type TorrentItem struct {
	ID     string
	Name   string
	Status string
}

func (i TorrentItem) FilterValue() string { return i.Name }
func (i TorrentItem) Title() string       { return i.Name }
func (i TorrentItem) Description() string { return fmt.Sprintf("%s  ·  %s", i.ID, i.Status) }

// Picker lets the user choose one torrent from a pre-filtered list (e.g.
// only running ones, for pause; only resumable ones, for resume).
type Picker struct {
	list      list.Model
	empty     bool
	emptyMsg  string
	Selected  *TorrentItem
	Cancelled bool
	width     int
	height    int
}

// NewPicker builds a picker over items. emptyMsg is shown instead of the
// list when items is empty (e.g. "no running torrents to pause").
func NewPicker(title string, items []TorrentItem, emptyMsg string) Picker {
	if len(items) == 0 {
		return Picker{empty: true, emptyMsg: emptyMsg}
	}

	listItems := make([]list.Item, len(items))
	for i, it := range items {
		listItems[i] = it
	}
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Foreground(tui.ColorAccent).BorderLeftForeground(tui.ColorAccent)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Foreground(tui.ColorAccent).BorderLeftForeground(tui.ColorAccent)

	l := list.New(listItems, delegate, 0, 0)
	l.Title = title
	l.Styles.Title = tui.TitleStyle
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(len(items) > 6)

	return Picker{list: l}
}

func (m Picker) Init() tea.Cmd { return nil }

func (m Picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.list.SetSize(msg.Width, msg.Height-4)
		return m, nil

	case tea.KeyMsg:
		if m.empty {
			return m, tea.Quit
		}
		switch msg.String() {
		case "esc", "ctrl+c":
			m.Cancelled = true
			return m, tea.Quit
		case "enter":
			if m.list.FilterState() == list.Filtering {
				break
			}
			if it, ok := m.list.SelectedItem().(TorrentItem); ok {
				sel := it
				m.Selected = &sel
			}
			return m, tea.Quit
		}
	}

	if m.empty {
		return m, nil
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Picker) View() string {
	if m.empty {
		return "\n  " + tui.SubtleStyle.Render(m.emptyMsg) + "\n\n" + tui.Footer("any key", "back")
	}
	return m.list.View() + "\n" + tui.Footer("enter", "select", "esc", "cancel")
}
