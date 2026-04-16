package tui

import "github.com/charmbracelet/lipgloss"

// ─── Colors (Vela Palette) ───────────────────────────────────────────────

var (
	// Core colors
	colorBase        = lipgloss.Color("#242426") // Dark background
	colorSurface     = lipgloss.Color("#2a2a2d") // Panel bg
	colorOverlay     = lipgloss.Color("#4a4a4e") // Muted borders
	colorText        = lipgloss.Color("#e0e0e2") // Light text
	colorSubtext     = lipgloss.Color("#8a8a8e") // Dim text
	colorAccent      = lipgloss.Color("#4e79a7") // Primary blue
	colorAccentLight = lipgloss.Color("#6b99c3") // Lighter blue
	colorMuted       = lipgloss.Color("#888888") // Muted grey
	colorSuccess     = lipgloss.Color("#59a14f") // Green
	colorWarn        = lipgloss.Color("#f28e2b") // Orange
	colorErr         = lipgloss.Color("#e15759") // Red
)

// ─── Layout Styles ───────────────────────────────────────────────────────

var (
	// App frame
	appStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Padding(1, 2)

	// Header (section titles)
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent).
			MarginBottom(1)

	// Footer / help bar
	helpStyle = lipgloss.NewStyle().
			Foreground(colorSubtext).
			MarginTop(1)

	// Error message
	errorStyle = lipgloss.NewStyle().
			Foreground(colorErr).
			Bold(true).
			Padding(0, 1)

	// Legacy styles (kept for compatibility with app.go)
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

// nodeKindColor returns a lipgloss color for a given node kind/type.
func nodeKindColor(kind string) lipgloss.Color {
	switch kind {
	case "function":
		return lipgloss.Color("#4ec9b0") // teal — functions
	case "struct":
		return lipgloss.Color("#4e9ae8") // blue — structs/classes
	case "interface":
		return lipgloss.Color("#9b6fd6") // purple — interfaces/contracts
	case "file":
		return lipgloss.Color("#d4a05e") // amber — files
	case "package":
		return lipgloss.Color("#f28e2b") // orange — packages
	case "observation":
		return lipgloss.Color("#59a14f") // green — ancora memories
	case "concept":
		return lipgloss.Color("#e15759") // red — concepts
	default:
		return colorAccentLight // fallback
	}
}

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
