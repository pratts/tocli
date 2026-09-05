// Package cli defines tocli's command-line interface. Every command here is
// deliberately thin glue: it reads/writes state via internal/store, spawns
// or signals processes via internal/process, and resolves torrent metadata
// via internal/engine. None of them construct a torrent.Client directly --
// only internal/engine.Run (invoked as the hidden `__run` subcommand) does.
package cli

import "github.com/spf13/cobra"

// NewRootCmd builds the tocli command tree. version is reported by
// `tocli --version`; pass the build-time version (see cmd/tocli/main.go)
// or "dev" for local/test builds.
func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "tocli",
		Short:         "A process-per-torrent terminal torrent client",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		newStartCmd(),
		newListCmd(),
		newPauseCmd(),
		newResumeCmd(),
		newRemoveCmd(),
		newRunCmd(),
	)
	return root
}
