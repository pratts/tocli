// Package dashboard implements the interactive `tocli list` screen: a
// live table refreshed on the same interval internal/engine.Run writes
// state.json, with inline pause/resume/remove for the selected row.
//
// Like every other tui screen, this one performs no torrent/process/store
// logic itself: refreshing reads via internal/store exactly as the plain
// `tocli list` command does, and the inline actions call
// internal/engine.PauseTorrent/ResumeTorrent/RemoveTorrent -- the same
// functions the plain `tocli pause`/`resume`/`remove` commands call.
package dashboard

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/pratts/tocli/internal/engine"
	"github.com/pratts/tocli/internal/humanize"
	"github.com/pratts/tocli/internal/store"
	"github.com/pratts/tocli/internal/tui"
	"github.com/pratts/tocli/internal/tui/actions"
)

// refreshInterval matches internal/engine.Run's state.json write cadence,
// so the dashboard never looks staler than the data actually is.
const refreshInterval = 2 * time.Second

type subStage int

const (
	subNone subStage = iota
	subRemoveMenu
	subMessage
)

// rowData is one torrent as loaded fresh on every refresh tick.
type rowData struct {
	ID      string
	Name    string
	Status  store.Status
	Percent string
	Rate    string
	Peers   string
	Broken  bool // config.json failed to load
}

// Model is the dashboard's tea.Model.
type Model struct {
	table table.Model
	rows  []rowData

	sub        subStage
	removeMenu actions.Menu
	message    string

	width, height int

	Quit bool
}

// New builds an empty dashboard; Init triggers the first data load.
func New() Model {
	return Model{table: newTable()}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(refreshCmd(), tickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.table.SetColumns(columnsForWidth(msg.Width))
		m.table.SetWidth(msg.Width)
		m.table.SetHeight(msg.Height - 6)
		return m, nil

	case tickMsg:
		return m, tea.Batch(refreshCmd(), tickCmd())

	case refreshMsg:
		m.rows = msg.rows
		m.table.SetRows(rowsToTable(m.rows))
		return m, nil

	case actionDoneMsg:
		m.sub = subNone
		m.message = msg.message
		if m.message != "" {
			m.sub = subMessage
		}
		return m, refreshCmd()
	}

	switch m.sub {
	case subRemoveMenu:
		return m.updateRemoveMenu(msg)
	case subMessage:
		if _, ok := msg.(tea.KeyMsg); ok {
			m.sub = subNone
		}
		return m, nil
	}

	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "q", "ctrl+c":
			m.Quit = true
			return m, tea.Quit
		case "p":
			return m.handlePause()
		case "r":
			return m.handleResume()
		case "d":
			return m.handleRemoveMenu()
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	switch m.sub {
	case subRemoveMenu:
		return "\n" + m.removeMenu.View()
	case subMessage:
		return "\n  " + m.message + "\n\n" + tui.Footer("any key", "continue")
	}

	var b strings.Builder
	b.WriteString(tui.TitleStyle.Render("tocli"))
	if len(m.rows) == 0 {
		b.WriteString("\n\n  " + tui.SubtleStyle.Render("no torrents tracked") + "\n\n")
		b.WriteString(tui.Footer("q", "quit"))
		return b.String()
	}
	b.WriteString("\n")
	b.WriteString(m.table.View())
	b.WriteString("\n")
	b.WriteString(m.footer())
	return b.String()
}

func (m Model) selectedRow() *rowData {
	i := m.table.Cursor()
	if i < 0 || i >= len(m.rows) {
		return nil
	}
	return &m.rows[i]
}

func (m Model) footer() string {
	var pairs []string
	if row := m.selectedRow(); row != nil && !row.Broken {
		switch {
		case row.Status == store.StatusRunning:
			pairs = append(pairs, "p", "pause")
		case row.Status.IsResumable():
			pairs = append(pairs, "r", "resume")
		}
		pairs = append(pairs, "d", "remove")
	} else if row != nil && row.Broken {
		pairs = append(pairs, "d", "remove")
	}
	pairs = append(pairs, "q", "quit")
	return tui.Footer(pairs...)
}

func (m Model) handlePause() (tea.Model, tea.Cmd) {
	row := m.selectedRow()
	if row == nil || row.Broken || row.Status != store.StatusRunning {
		return m, nil
	}
	id := row.ID
	return m, func() tea.Msg {
		outcome, err := engine.PauseTorrent(id)
		if err != nil {
			return actionDoneMsg{message: fmt.Sprintf("failed to pause %s: %v", id, err)}
		}
		if !outcome.Signaled {
			return actionDoneMsg{message: fmt.Sprintf("%s was not running (status: %s)", id, outcome.Config.Status)}
		}
		return actionDoneMsg{message: fmt.Sprintf("paused %s", outcome.Config.Name)}
	}
}

func (m Model) handleResume() (tea.Model, tea.Cmd) {
	row := m.selectedRow()
	if row == nil || row.Broken || !row.Status.IsResumable() {
		return m, nil
	}
	id := row.ID
	return m, func() tea.Msg {
		outcome, err := engine.ResumeTorrent(id)
		if err != nil {
			return actionDoneMsg{message: fmt.Sprintf("failed to resume %s: %v", id, err)}
		}
		if outcome.AlreadyRunning {
			return actionDoneMsg{message: fmt.Sprintf("%s is already running", id)}
		}
		return actionDoneMsg{message: fmt.Sprintf("resumed %s", outcome.Config.Name)}
	}
}

func (m Model) handleRemoveMenu() (tea.Model, tea.Cmd) {
	row := m.selectedRow()
	if row == nil {
		return m, nil
	}
	name := row.Name
	if row.Broken {
		name = row.ID
	}
	m.sub = subRemoveMenu
	m.removeMenu = actions.RemoveMenu(name)
	return m, nil
}

func (m Model) updateRemoveMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.removeMenu.Update(msg)
	menu := updated.(actions.Menu)
	m.removeMenu = menu

	if menu.Cancelled {
		m.sub = subNone
		return m, nil
	}
	if menu.Selected != "" {
		row := m.selectedRow()
		if row == nil {
			m.sub = subNone
			return m, nil
		}
		id := row.ID
		withData := menu.Selected == "with-data"
		return m, func() tea.Msg {
			outcome, err := engine.RemoveTorrent(id, withData)
			return actionDoneMsg{message: engine.DescribeRemove(id, outcome, err)}
		}
	}
	return m, cmd
}

type tickMsg struct{}
type refreshMsg struct{ rows []rowData }
type actionDoneMsg struct{ message string }

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func refreshCmd() tea.Cmd {
	return func() tea.Msg {
		return refreshMsg{rows: loadRows()}
	}
}

// loadRows reads every tracked torrent's config.json/state.json exactly as
// the plain `tocli list` command does, self-correcting stale "running"
// statuses via store.ReconcileLiveness along the way.
func loadRows() []rowData {
	ids, err := store.ListIDs()
	if err != nil {
		return nil
	}
	sort.Strings(ids)

	rows := make([]rowData, 0, len(ids))
	for _, id := range ids {
		tc, err := store.LoadTorrentConfig(id)
		if err != nil {
			rows = append(rows, rowData{ID: id, Broken: true})
			continue
		}
		_ = store.ReconcileLiveness(tc)

		percent, rate, peers := "-", "-", "-"
		if st, err := store.LoadState(id); err == nil {
			percent = fmt.Sprintf("%.1f%%", st.Percent)
			rate = humanize.Rate(st.DownloadRateBps)
			peers = fmt.Sprintf("%d/%d", st.ActivePeers, st.TotalPeers)
		}

		rows = append(rows, rowData{
			ID: tc.ID, Name: tc.Name, Status: tc.Status,
			Percent: percent, Rate: rate, Peers: peers,
		})
	}
	return rows
}
