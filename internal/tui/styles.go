// Package tui provides the shared building blocks for tocli's interactive
// screens: lipgloss styles, the tea.Program bootstrap helper, and (in its
// subpackages addflow/dashboard/actions) the screens themselves. Every
// model here is presentation only -- state changes (starting, pausing,
// resuming, removing a torrent) always go through the same
// internal/engine, internal/store, and internal/process functions the
// plain CLI commands call directly; nothing here re-implements them.
package tui

import "github.com/charmbracelet/lipgloss"

// Color palette. Kept to a small, deliberately restrained set (an accent,
// a muted foreground for secondary text, and semantic colors for status)
// rather than styling every screen differently -- the goal is one
// consistent, unobtrusive look across all five flows.
var (
	ColorAccent     = lipgloss.Color("6") // cyan: selection, focus, headers
	ColorMuted      = lipgloss.Color("8") // gray: secondary text, hints
	ColorGood       = lipgloss.Color("2") // green: running/completed
	ColorWarn       = lipgloss.Color("3") // yellow: paused/stopped
	ColorBad        = lipgloss.Color("1") // red: crashed/error
	ColorForeground = lipgloss.Color("15")
)

var (
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorAccent).
			Padding(0, 1)

	SubtleStyle = lipgloss.NewStyle().Foreground(ColorMuted)

	ErrorStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorBad)

	// FooterStyle renders the keybinding hint bar shown at the bottom of
	// every screen, e.g. "[enter] start   [esc] cancel".
	FooterStyle = lipgloss.NewStyle().Foreground(ColorMuted).Padding(1, 1, 0, 1)

	BorderStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(ColorMuted).
			Padding(0, 1)

	SelectedStyle = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true)
)

// StatusColor maps a store.Status string to a semantic color, used
// consistently across the dashboard and every picker.
func StatusColor(status string) lipgloss.Color {
	switch status {
	case "running":
		return ColorGood
	case "completed":
		return ColorGood
	case "paused", "stopped":
		return ColorWarn
	case "crashed", "interrupted", "error":
		return ColorBad
	default:
		return ColorForeground
	}
}

// Footer renders a keybinding hint bar from label/action pairs, e.g.
// Footer("enter", "start", "esc", "cancel") ->
// "[enter] start   [esc] cancel".
func Footer(pairs ...string) string {
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	var out string
	for i := 0; i+1 < len(pairs); i += 2 {
		if i > 0 {
			out += "   "
		}
		out += keyStyle.Render("["+pairs[i]+"]") + " " + pairs[i+1]
	}
	return FooterStyle.Render(out)
}
