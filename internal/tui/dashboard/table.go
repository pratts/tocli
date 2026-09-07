package dashboard

import (
	"github.com/charmbracelet/bubbles/table"

	"github.com/pratts/tocli/internal/tui"
)

func newTable() table.Model {
	t := table.New(
		table.WithColumns(columnsForWidth(80)),
		table.WithFocused(true),
	)
	styles := table.DefaultStyles()
	styles.Header = styles.Header.Bold(true).Foreground(tui.ColorAccent)
	styles.Selected = styles.Selected.Foreground(tui.ColorAccent).Bold(true)
	t.SetStyles(styles)
	return t
}

// columnsForWidth gives NAME the leftover space after the other, mostly
// fixed-width columns, so the table fills the terminal instead of leaving
// a ragged edge or truncating names unnecessarily on wide terminals.
func columnsForWidth(width int) []table.Column {
	const (
		idW = 12
		// Wide enough for "running (ephemeral)" -- see rowsToTable -- not
		// just the bare status word.
		statusW  = 20
		percentW = 8
		rateW    = 12
		peersW   = 10
	)
	nameW := width - idW - statusW - percentW - rateW - peersW - 10
	if nameW < 16 {
		nameW = 16
	}
	return []table.Column{
		{Title: "ID", Width: idW},
		{Title: "NAME", Width: nameW},
		{Title: "STATUS", Width: statusW},
		{Title: "PERCENT", Width: percentW},
		{Title: "RATE", Width: rateW},
		{Title: "PEERS", Width: peersW},
	}
}

func rowsToTable(rows []rowData) []table.Row {
	out := make([]table.Row, len(rows))
	for i, r := range rows {
		if r.Broken {
			out[i] = table.Row{r.ID, "error: unreadable config", "-", "-", "-", "-"}
			continue
		}
		status := string(r.Status)
		if r.PortEphemeral {
			// Visible here, not just logged to log.txt -- see
			// store.TorrentConfig.PortEphemeral's doc comment for why.
			status += " (ephemeral)"
		}
		out[i] = table.Row{r.ID, r.Name, status, r.Percent, r.Rate, r.Peers}
	}
	return out
}
