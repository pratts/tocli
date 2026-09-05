package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/pratts/tocli/internal/humanize"
	"github.com/pratts/tocli/internal/store"
	"github.com/pratts/tocli/internal/tui"
	"github.com/pratts/tocli/internal/tui/dashboard"
)

func newListCmd() *cobra.Command {
	var jsonOut bool
	var plain bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tracked torrents and their status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			switch {
			case jsonOut:
				return runListJSON(cmd)
			case plain || !isInteractiveTerminal():
				return runListTable(cmd)
			default:
				return runListTUI(cmd)
			}
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print a JSON array of torrent status objects instead of the table")
	cmd.Flags().BoolVar(&plain, "plain", false, "force the static table even on a terminal")
	return cmd
}

// listEntry mirrors one row of the table/dashboard, exported as JSON for
// --json so scripts get the same data a human sees, structured.
type listEntry struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Status          string  `json:"status"`
	Percent         float64 `json:"percent"`
	DownloadRateBps float64 `json:"download_rate_bps"`
	ActivePeers     int     `json:"active_peers"`
	TotalPeers      int     `json:"total_peers"`
	Error           string  `json:"error,omitempty"`
}

func loadListEntries() ([]listEntry, error) {
	ids, err := store.ListIDs()
	if err != nil {
		return nil, fmt.Errorf("list torrents: %w", err)
	}
	sort.Strings(ids)

	entries := make([]listEntry, 0, len(ids))
	for _, id := range ids {
		tc, err := store.LoadTorrentConfig(id)
		if err != nil {
			entries = append(entries, listEntry{ID: id, Error: err.Error()})
			continue
		}
		_ = store.ReconcileLiveness(tc)

		e := listEntry{ID: tc.ID, Name: tc.Name, Status: string(tc.Status)}
		if st, err := store.LoadState(id); err == nil {
			e.Percent = st.Percent
			e.DownloadRateBps = st.DownloadRateBps
			e.ActivePeers = st.ActivePeers
			e.TotalPeers = st.TotalPeers
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func runListJSON(cmd *cobra.Command) error {
	entries, err := loadListEntries()
	if err != nil {
		return err
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}

// runListTable is the original, fully scriptable static table -- the
// default whenever stdout isn't a real terminal (or --plain/--json is
// passed), so existing scripts and pipelines keep working unchanged.
func runListTable(cmd *cobra.Command) error {
	entries, err := loadListEntries()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "no torrents tracked")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tSTATUS\tPERCENT\tRATE\tPEERS")
	for _, e := range entries {
		if e.Error != "" {
			// A config.json that fails to parse (corrupt or truncated --
			// e.g. the parent died mid-write, or the disk filled up) must
			// not take the whole command down with it. Surface it as its
			// own row so it's visible in the normal listing, not just
			// buried in stderr, and let the user `tocli remove` it by hand.
			fmt.Fprintf(w, "%s\t?\terror: unreadable config\t-\t-\t-\n", e.ID)
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s: %s\n", e.ID, e.Error)
			continue
		}
		percent := fmt.Sprintf("%.1f%%", e.Percent)
		rate := humanize.Rate(e.DownloadRateBps)
		peers := fmt.Sprintf("%d/%d", e.ActivePeers, e.TotalPeers)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", e.ID, e.Name, e.Status, percent, rate, peers)
	}
	return w.Flush()
}

// runListTUI opens the live dashboard: tocli list's default when
// interactive.
func runListTUI(cmd *cobra.Command) error {
	_, err := tui.Run(dashboard.New())
	return err
}
