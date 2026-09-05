package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pratts/tocli/internal/config"
	"github.com/pratts/tocli/internal/engine"
	"github.com/pratts/tocli/internal/humanize"
	"github.com/pratts/tocli/internal/tui"
	"github.com/pratts/tocli/internal/tui/addflow"
)

func newStartCmd() *cobra.Command {
	var tuiFlag bool
	var yes bool
	cmd := &cobra.Command{
		Use:   "start [file-or-magnet]",
		Short: "Resolve a torrent's metadata and start downloading it in the background",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var source string
			if len(args) == 1 {
				source = args[0]
			}
			return runStart(cmd, source, tuiFlag, yes)
		},
	}
	cmd.Flags().BoolVarP(&tuiFlag, "tui", "i", false, "always show the interactive preview screen")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation entirely for scripted/non-interactive use")
	return cmd
}

// runStart implements the dual-mode trigger rule for `start`:
//   - --yes always skips confirmation and the TUI outright, for scripted
//     use; a source is still required (there's nothing to skip a prompt
//     for otherwise).
//   - no source given: open the add-flow's input screen if a terminal is
//     attached (there's nowhere to type into otherwise), else the classic
//     usage error.
//   - --tui, or a source given on a real terminal: the interactive preview
//     (file tree, live peer count, trackers) replaces the old plain Y/N
//     confirm -- it confirms before downloading either way, just with
//     richer information.
//   - a source given but stdout/stdin isn't a terminal (piped, cron,
//     CI): the plain, scriptable Y/N confirm, unchanged from before the
//     TUI existed.
func runStart(cmd *cobra.Command, source string, tuiFlag, yes bool) error {
	switch {
	case yes:
		if source == "" {
			return fmt.Errorf("a torrent file or magnet link is required with --yes")
		}
		return runStartDirect(cmd, source, true)
	case source == "" && !isInteractiveTerminal():
		return fmt.Errorf("usage: tocli start <path-to-torrent-file-or-magnet-link>")
	case source == "", tuiFlag, isInteractiveTerminal():
		return runStartTUI(cmd, source)
	default:
		return runStartDirect(cmd, source, false)
	}
}

// runStartDirect is the plain, scriptable path: resolve, print a plain
// file list, confirm (unless autoConfirm), start.
func runStartDirect(cmd *cobra.Command, source string, autoConfirm bool) error {
	out := cmd.OutOrStdout()

	globalCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), engine.MetadataTimeout)
	defer cancel()
	mi, err := engine.ResolveMetainfo(ctx, source, out)
	if err != nil {
		return fmt.Errorf("resolve torrent metadata: %w", err)
	}

	info, err := mi.UnmarshalInfo()
	if err != nil {
		return fmt.Errorf("parse torrent info: %w", err)
	}

	fmt.Fprintln(out, "\nFiles in torrent:")
	for _, f := range engine.ListFiles(info) {
		fmt.Fprintf(out, "  %s (%s)\n", f.Path, humanize.Bytes(f.Length))
	}
	fmt.Fprintf(out, "\nTotal size: %s\n", humanize.Bytes(info.TotalLength()))

	if !autoConfirm && !confirm("\nStart download? [Y/N]: ") {
		fmt.Fprintln(out, "aborted")
		return nil
	}

	tc, err := engine.StartTorrent(mi, source, *globalCfg)
	if err != nil {
		if errors.Is(err, engine.ErrAlreadyTracked) {
			fmt.Fprintf(out, "torrent %s is already tracked as %q (status: %s); use `tocli resume %s` if it isn't running\n",
				tc.ID, tc.Name, tc.Status, tc.ID)
			return nil
		}
		return err
	}

	fmt.Fprintf(out, "\nstarted torrent %s (%s), downloading in background\n", tc.ID, tc.Name)
	return nil
}

// runStartTUI runs the interactive add-flow. source may be empty, in
// which case the flow opens on its input screen.
func runStartTUI(cmd *cobra.Command, source string) error {
	globalCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	final, err := tui.Run(addflow.New(source, *globalCfg))
	if err != nil {
		return fmt.Errorf("run interactive add flow: %w", err)
	}

	out := cmd.OutOrStdout()
	if final.Cancelled {
		fmt.Fprintln(out, "aborted")
		return nil
	}
	if final.Started {
		fmt.Fprintf(out, "started torrent %s (%s), downloading in background\n", final.StartedID, final.StartedName)
	}
	return nil
}
