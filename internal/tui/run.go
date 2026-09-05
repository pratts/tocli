package tui

import tea "github.com/charmbracelet/bubbletea"

// Run bootstraps and runs a tea.Program with the options common to every
// tocli screen (alt-screen so the shell's scrollback isn't disturbed), and
// returns the final model already type-asserted back to its concrete type
// via the generic parameter -- callers just do:
//
//	final, err := tui.Run(addflow.New(source, cfg))
//
// instead of repeating the tea.NewProgram/.Run()/type-assert boilerplate
// in every command.
func Run[M tea.Model](model M) (M, error) {
	p := tea.NewProgram(model, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		var zero M
		return zero, err
	}
	return final.(M), nil
}
