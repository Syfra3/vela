package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Colours
	colorAccent  = lipgloss.Color("#4e79a7")
	colorMuted   = lipgloss.Color("#888888")
	colorSuccess = lipgloss.Color("#59a14f")
	colorWarn    = lipgloss.Color("#f28e2b")
	colorErr     = lipgloss.Color("#e15759")

	// Styles
	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	styleLabel = lipgloss.NewStyle().
			Foreground(colorMuted)

	styleOK = lipgloss.NewStyle().
		Foreground(colorSuccess).
		Bold(true)

	styleWarn = lipgloss.NewStyle().
			Foreground(colorWarn)

	styleErr = lipgloss.NewStyle().
			Foreground(colorErr).
			Bold(true)

	styleDim = lipgloss.NewStyle().
			Foreground(colorMuted)

	stylePrompt = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	styleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorMuted).
			Padding(0, 1)
)

// bar renders a filled/empty progress bar of the given width.
func bar(pct, width int) string {
	if width < 4 {
		width = 4
	}
	filled := int(float64(width) * float64(pct) / 100.0)
	if filled > width {
		filled = width
	}
	empty := width - filled
	filledStr := lipgloss.NewStyle().Foreground(colorAccent).Render(repeat("█", filled))
	emptyStr := styleDim.Render(repeat("░", empty))
	return "[" + filledStr + emptyStr + "]"
}

func repeat(s string, n int) string {
	out := make([]byte, 0, n*len(s))
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
