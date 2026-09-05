package addflow

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/pratts/tocli/internal/engine"
	"github.com/pratts/tocli/internal/humanize"
	"github.com/pratts/tocli/internal/tui"
)

func (m Model) updatePreview(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.closeSession()
			m.Cancelled = true
			return m, tea.Quit
		case "enter":
			m.stage = stageStarting
			m.spinner = newSpinner()
			return m, tea.Batch(m.spinner.Tick, startTorrentCmd(m.session, m.source, m.globalCfg))
		}

	case statsTickMsg:
		return m, tickStatsCmd()

	case tea.WindowSizeMsg:
		m.viewport.Width = msg.Width
		m.viewport.Height = previewTreeHeight(msg.Height)
		return m, nil
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m Model) viewPreview() string {
	stats := m.session.Stats()

	var b strings.Builder
	b.WriteString(tui.TitleStyle.Render(m.name))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("  Total size: %s\n", humanize.Bytes(m.totalSize)))
	b.WriteString(fmt.Sprintf("  Peers: %d active / %d known   Seeders: %d\n",
		stats.ActivePeers, stats.TotalPeers, stats.ConnectedSeeders))
	if len(m.trackers) > 0 {
		b.WriteString(fmt.Sprintf("  Trackers: %d\n", len(m.trackers)))
	} else {
		b.WriteString("  Trackers: none (DHT/PEX only)\n")
	}
	b.WriteString("\n")
	b.WriteString(m.viewport.View())
	b.WriteString("\n")
	b.WriteString(tui.Footer("enter", "start", "esc", "cancel"))
	return b.String()
}

func (m Model) updateStarting(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case startedMsg:
		m.Started = true
		m.StartedID = msg.config.ID
		m.StartedName = msg.config.Name
		return m, tea.Quit

	case startErrMsg:
		if msg.existing != nil {
			// Already tracked: not an error worth alarming over, just
			// report it plainly, matching the direct CLI's message.
			m.StartedID = msg.existing.ID
			m.StartedName = msg.existing.Name
			m.err = msg.err
			m.stage = stageError
			return m, nil
		}
		m.err = msg.err
		m.stage = stageError
		return m, nil
	}

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m Model) viewStarting() string {
	return "\n  " + m.spinner.View() + " starting download...\n"
}

func (m Model) updateError(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc", "q":
			m.Cancelled = true
			return m, tea.Quit
		case "enter":
			// Back to the input screen to try again.
			m.err = nil
			m.stage = stageInput
			m.textInput = newTextInput()
			return m, textinput.Blink
		}
	}
	return m, nil
}

func (m Model) viewError() string {
	var b strings.Builder
	if errors.Is(m.err, engine.ErrAlreadyTracked) {
		b.WriteString(tui.TitleStyle.Render("Already tracked"))
		b.WriteString(fmt.Sprintf("\n\n  Torrent %s is already tracked as %q.\n  Use `tocli resume %s` if it isn't running.\n\n",
			m.StartedID, m.StartedName, m.StartedID))
	} else {
		b.WriteString(tui.ErrorStyle.Render("Couldn't add this torrent"))
		b.WriteString("\n\n  ")
		b.WriteString(m.err.Error())
		b.WriteString("\n\n")
	}
	b.WriteString(tui.Footer("enter", "try again", "esc", "cancel"))
	return b.String()
}

// previewTreeHeight leaves room for the header (name/size/peers/trackers)
// and footer above/below the scrollable file tree.
func previewTreeHeight(termHeight int) int {
	const reserved = 10
	h := termHeight - reserved
	if h < 3 {
		h = 3
	}
	return h
}

func displayName(session *engine.PreviewSession) string {
	name := session.Info.BestName()
	if name == "" {
		return "(unnamed torrent)"
	}
	return name
}

func trackerList(session *engine.PreviewSession) []string {
	mi := session.Metainfo()
	var out []string
	for _, tier := range mi.UpvertedAnnounceList() {
		out = append(out, tier...)
	}
	return out
}

// treeNode is a directory or file in the rendered preview tree.
type treeNode struct {
	name     string
	isDir    bool
	size     int64
	children []*treeNode
}

func renderFileTree(files []engine.FileSummary) string {
	root := &treeNode{isDir: true}
	for _, f := range files {
		insertPath(root, strings.Split(f.Path, "/"), f.Length)
	}
	sortTree(root)

	var b strings.Builder
	writeTree(&b, root, "")
	return strings.TrimRight(b.String(), "\n")
}

func insertPath(root *treeNode, parts []string, size int64) {
	cur := root
	for i, part := range parts {
		isLast := i == len(parts)-1
		var child *treeNode
		for _, c := range cur.children {
			if c.name == part && c.isDir == !isLast {
				child = c
				break
			}
		}
		if child == nil {
			child = &treeNode{name: part, isDir: !isLast}
			cur.children = append(cur.children, child)
		}
		if isLast {
			child.size = size
		}
		cur = child
	}
}

func sortTree(n *treeNode) {
	sort.Slice(n.children, func(i, j int) bool {
		a, b := n.children[i], n.children[j]
		if a.isDir != b.isDir {
			return a.isDir // directories first
		}
		return a.name < b.name
	})
	for _, c := range n.children {
		sortTree(c)
	}
}

func writeTree(b *strings.Builder, node *treeNode, prefix string) {
	for i, c := range node.children {
		last := i == len(node.children)-1
		branch, childPrefix := "├── ", prefix+"│   "
		if last {
			branch, childPrefix = "└── ", prefix+"    "
		}
		line := prefix + branch + c.name
		if c.isDir {
			line += "/"
		} else {
			line += "  " + tui.SubtleStyle.Render(humanize.Bytes(c.size))
		}
		b.WriteString(line + "\n")
		if c.isDir {
			writeTree(b, c, childPrefix)
		}
	}
}
