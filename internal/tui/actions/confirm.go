package actions

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/pratts/tocli/internal/tui"
)

// Confirm is a simple yes/no dialog, used for the brief "Paused <name>"
// style acknowledgements after an action completes.
type Confirm struct {
	Message   string
	needsAck  bool // if true, any key dismisses (no y/n choice, just an ack)
	Confirmed bool
	Cancelled bool
}

// NewConfirm builds a yes/no confirmation prompt.
func NewConfirm(message string) Confirm {
	return Confirm{Message: message}
}

// NewAck builds a message that's dismissed by any keypress, for
// after-the-fact acknowledgements rather than yes/no decisions.
func NewAck(message string) Confirm {
	return Confirm{Message: message, needsAck: true}
}

func (m Confirm) Init() tea.Cmd { return nil }

func (m Confirm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.needsAck {
		m.Confirmed = true
		return m, tea.Quit
	}
	switch k.String() {
	case "y", "Y", "enter":
		m.Confirmed = true
		return m, tea.Quit
	case "n", "N", "esc":
		m.Cancelled = true
		return m, tea.Quit
	}
	return m, nil
}

func (m Confirm) View() string {
	body := tui.BorderStyle.Render(m.Message)
	if m.needsAck {
		return "\n" + body + "\n" + tui.Footer("any key", "continue")
	}
	return "\n" + body + "\n" + tui.Footer("y/enter", "confirm", "n/esc", "cancel")
}
