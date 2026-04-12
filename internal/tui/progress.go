package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Syfra3/vela/pkg/types"
)

// RenderProgress renders a progress bar with extraction stats
// This is a pure rendering function - state lives in the Bubbletea model
func RenderProgress(p types.ExtractionProgress, width int) string {
	if p.TotalChunks == 0 {
		return "Scanning files..."
	}

	pct := p.Percentage()
	barWidth := width - 20
	if barWidth < 10 {
		barWidth = 10
	}
	filled := int(float64(barWidth) * float64(pct) / 100)
	empty := barWidth - filled

	bar := "[" + strings.Repeat("█", filled) + strings.Repeat("░", empty) + "]"

	elapsed := p.ElapsedSeconds()
	remaining := p.EstimatedRemainingSeconds()

	return fmt.Sprintf(
		"%s %3d%%\n  File:      %s\n  Progress:  %d / %d chunks\n  Elapsed:   %s\n  ETA:       %s",
		bar,
		pct,
		truncate(p.CurrentFile, width-14),
		p.ProcessedChunks,
		p.TotalChunks,
		formatDuration(elapsed),
		formatDuration(remaining),
	)
}

// RenderFileSummary renders a summary of files discovered
func RenderFileSummary(total, code, docs, other int) string {
	return fmt.Sprintf(
		"  Files discovered: %d total  (%d code · %d docs · %d other)",
		total, code, docs, other,
	)
}

// RenderProviderStatus renders the current LLM provider info
func RenderProviderStatus(provider string, isHealthy bool) string {
	status := "offline"
	if isHealthy {
		status = "ready"
	}
	return fmt.Sprintf("  LLM Provider: %s  [%s]", provider, status)
}

func formatDuration(seconds int) string {
	if seconds <= 0 {
		return "calculating..."
	}
	d := time.Duration(seconds) * time.Second
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return "..." + s[len(s)-max+3:]
}
