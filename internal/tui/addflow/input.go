package addflow

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pratts/tocli/internal/tui"
)

func (m Model) updateInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.Cancelled = true
			return m, tea.Quit
		case "enter":
			source := strings.TrimSpace(m.textInput.Value())
			if source == "" {
				return m, nil
			}
			m.source = source
			m.stage = stageLoading
			m.spinner = newSpinner()
			return m, tea.Batch(m.spinner.Tick, openPreviewCmd(source))
		}
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m Model) viewInput() string {
	var b strings.Builder
	b.WriteString(tui.TitleStyle.Render("Add a torrent"))
	b.WriteString("\n\n  ")
	b.WriteString(m.textInput.View())
	b.WriteString("\n")
	b.WriteString(tui.Footer("enter", "continue", "esc", "cancel"))
	return b.String()
}
