package addflow

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/pratts/tocli/internal/tui"
)

func (m Model) updateLoading(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case beginLoadingMsg:
		return m.startLoading(m.source)

	case previewReadyMsg:
		m.stopWaitingForMetadata()
		m.session = msg.session
		m.name = displayName(m.session)
		m.totalSize = m.session.Info.TotalLength()
		m.trackers = trackerList(m.session)
		m.viewport = viewport.New(m.width, previewTreeHeight(m.height))
		m.viewport.SetContent(renderFileTree(m.session.Files()))
		m.stage = stagePreview
		return m, tickStatsCmd()

	case previewErrMsg:
		m.stopWaitingForMetadata()
		m.err = msg.err
		m.stage = stageError
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "esc" {
			// Abandon the in-flight resolution right now rather than
			// leaving it running for up to engine.MetadataTimeout: OpenPreview
			// closes its own torrent.Client as soon as ctx is cancelled.
			m.closeSession()
			m.Cancelled = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m Model) viewLoading() string {
	var b strings.Builder
	b.WriteString(tui.TitleStyle.Render("Add a torrent"))
	b.WriteString("\n\n  ")
	b.WriteString(m.spinner.View())
	b.WriteString(" resolving metadata for ")
	b.WriteString(tui.SubtleStyle.Render(m.source))
	b.WriteString("...\n\n")
	b.WriteString(tui.Footer("esc", "cancel"))
	return b.String()
}
