package cli

import (
	"os"

	"golang.org/x/term"
)

// isInteractiveTerminal reports whether both stdin and stdout are real
// terminals -- the condition under which a command missing enough
// arguments to act directly opens a TUI screen instead of erroring with a
// usage message, and under which `start`/`list` prefer their richer
// interactive views by default.
func isInteractiveTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}
