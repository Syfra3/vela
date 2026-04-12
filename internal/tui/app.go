package tui

import (
	"fmt"
	"os"

	"github.com/Syfra3/vela/pkg/types"
	tea "github.com/charmbracelet/bubbletea"
)

// progressMsg carries a progress update into the Bubbletea runtime.
type progressMsg types.ProgressUpdate

// model is the Bubbletea model for the minimal Phase-0 TUI.
type model struct {
	progress  types.ExtractionProgress
	provider  string
	healthy   bool
	done      bool
	err       error
	termWidth int
	updatesCh <-chan types.ProgressUpdate
}

// NewProgram creates and returns a Bubbletea program that reads from the
// provided channel. Returns nil if stdout is not a TTY (CI-safe fallback).
func NewProgram(ch <-chan types.ProgressUpdate, provider string) *tea.Program {
	if !isTTY() {
		return nil
	}
	m := model{
		updatesCh: ch,
		provider:  provider,
		termWidth: 80,
	}
	return tea.NewProgram(m)
}

// Init starts waiting for progress messages from the channel.
func (m model) Init() tea.Cmd {
	return waitForUpdate(m.updatesCh)
}

// Update handles incoming messages.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case progressMsg:
		u := types.ProgressUpdate(msg)
		m.progress = u.Progress
		m.err = u.Error
		if u.IsComplete {
			m.done = true
			return m, tea.Quit
		}
		return m, waitForUpdate(m.updatesCh)

	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
	}
	return m, nil
}

// View renders the current progress.
func (m model) View() string {
	if m.done {
		return fmt.Sprintf("\nExtraction complete: %d files processed.\n",
			m.progress.ProcessedFiles)
	}
	provider := RenderProviderStatus(m.provider, m.healthy)
	progress := RenderProgress(m.progress, m.termWidth)
	return fmt.Sprintf("\n%s\n\n%s\n", provider, progress)
}

// waitForUpdate returns a tea.Cmd that blocks on the next channel message.
func waitForUpdate(ch <-chan types.ProgressUpdate) tea.Cmd {
	return func() tea.Msg {
		update, ok := <-ch
		if !ok {
			return progressMsg(types.ProgressUpdate{IsComplete: true})
		}
		return progressMsg(update)
	}
}

// isTTY returns true if stdout is an interactive terminal.
func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// RunPlainProgress reads from ch and prints progress lines to stdout.
// Used when not running in a TTY.
func RunPlainProgress(ch <-chan types.ProgressUpdate) {
	for u := range ch {
		if u.Error != nil {
			fmt.Fprintf(os.Stderr, "[error] %v\n", u.Error)
			continue
		}
		p := u.Progress
		fmt.Printf("[progress] %s  %d/%d chunks (%d%%)\n",
			p.CurrentFile, p.ProcessedChunks, p.TotalChunks, p.Percentage())
		if u.IsComplete {
			return
		}
	}
}
