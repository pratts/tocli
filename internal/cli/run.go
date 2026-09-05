package cli

import (
	"github.com/spf13/cobra"

	"github.com/pratts/tocli/internal/engine"
)

// newRunCmd wires up `tocli __run <id>`, the hidden entrypoint that `start`
// and `resume` spawn as a detached background process. It is not meant for
// direct interactive use.
func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "__run <id>",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return engine.Run(args[0])
		},
	}
}
