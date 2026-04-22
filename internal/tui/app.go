package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Syfra3/vela/pkg/types"
)

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

type progressMsg types.ProgressUpdate

type workerMsg WorkerUpdate

type queryResultMsg struct {
	query  string
	result string
}

// ---------------------------------------------------------------------------
// Mode
// ---------------------------------------------------------------------------

type tuiMode int

const (
	modeExtract tuiMode = iota
	modeQuery
)

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

// Model is the top-level Bubbletea model.
type Model struct {
	mode tuiMode

	// Extraction state
	progress    types.ExtractionProgress
	backendName string
	backendOK   bool
	workers     []WorkerStatus
	done        bool
	updatesCh   <-chan types.ProgressUpdate
	workerCh    <-chan WorkerUpdate

	// Query state
	queryInput  string
	queryResult string
	queryFn     func(input string) string // injected by caller

	termWidth  int
	termHeight int
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

// NewProgram builds a Bubbletea program. Returns nil when stdout is not a TTY.
// workerCh may be nil for single-goroutine mode. queryFn handles query commands
// after extraction completes.
func NewProgram(
	progressCh <-chan types.ProgressUpdate,
	workerCh <-chan WorkerUpdate,
	backendName string,
	backendOK bool,
	numWorkers int,
	queryFn func(string) string,
) *tea.Program {
	if !isTTY() {
		return nil
	}
	workers := make([]WorkerStatus, numWorkers)
	for i := range workers {
		workers[i] = WorkerStatus{ID: i, Idle: true}
	}
	m := Model{
		mode:        modeExtract,
		updatesCh:   progressCh,
		workerCh:    workerCh,
		backendName: backendName,
		backendOK:   backendOK,
		workers:     workers,
		queryFn:     queryFn,
		termWidth:   100,
		termHeight:  24,
	}
	return tea.NewProgram(m, tea.WithAltScreen())
}

// ---------------------------------------------------------------------------
// Init / Update / View
// ---------------------------------------------------------------------------

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{waitForProgress(m.updatesCh)}
	if m.workerCh != nil {
		cmds = append(cmds, waitForWorker(m.workerCh))
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height

	case tea.KeyMsg:
		return m.handleKey(msg)

	case progressMsg:
		u := types.ProgressUpdate(msg)
		m.progress = u.Progress
		if u.Error != nil {
			// Non-fatal: keep running
		}
		if u.IsComplete {
			m.done = true
			if m.queryFn != nil {
				m.mode = modeQuery
				return m, nil // stay open for queries
			}
			return m, tea.Quit
		}
		return m, waitForProgress(m.updatesCh)

	case workerMsg:
		ws := WorkerUpdate(msg)
		if ws.Status.ID < len(m.workers) {
			m.workers[ws.Status.ID] = ws.Status
		}
		if m.workerCh != nil {
			return m, waitForWorker(m.workerCh)
		}

	case queryResultMsg:
		m.queryResult = msg.result
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeExtract:
		if msg.Type == tea.KeyCtrlC || msg.String() == "q" {
			return m, tea.Quit
		}

	case modeQuery:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEsc:
			if m.queryInput == "" {
				return m, tea.Quit
			}
			m.queryInput = ""
			m.queryResult = ""
		case tea.KeyEnter:
			input := strings.TrimSpace(m.queryInput)
			if input == "quit" || input == "exit" {
				return m, tea.Quit
			}
			if input != "" && m.queryFn != nil {
				result := m.queryFn(input)
				m.queryResult = result
			}
			m.queryInput = ""
		case tea.KeyBackspace, tea.KeyDelete:
			if len(m.queryInput) > 0 {
				runes := []rune(m.queryInput)
				m.queryInput = string(runes[:len(runes)-1])
			}
		default:
			if msg.Type == tea.KeyRunes {
				m.queryInput += msg.String()
			} else if msg.Type == tea.KeySpace {
				m.queryInput += " "
			}
		}
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (m Model) View() string {
	switch m.mode {
	case modeExtract:
		return m.viewExtract()
	case modeQuery:
		return m.viewQuery()
	}
	return ""
}

func (m Model) viewExtract() string {
	w := m.termWidth
	if w < 40 {
		w = 40
	}

	var sb strings.Builder

	// Header
	sb.WriteString("\n")
	sb.WriteString(styleHeader.Render(" Vela — Extracting Knowledge Graph "))
	sb.WriteString("\n\n")

	// Backend status
	provStatus := styleOK.Render("ready")
	if !m.backendOK {
		provStatus = styleWarn.Render("standby")
	}
	sb.WriteString(fmt.Sprintf("  %s  %s  [%s]\n\n",
		styleLabel.Render("Build Backend:"),
		m.backendName,
		provStatus,
	))

	// Overall progress bar
	pct := m.progress.Percentage()
	barW := w - 14
	if barW < 20 {
		barW = 20
	}
	sb.WriteString(fmt.Sprintf("  %s %3d%%\n", bar(pct, barW), pct))
	sb.WriteString(fmt.Sprintf("  %s %d / %d files\n",
		styleLabel.Render("Files:  "),
		m.progress.ProcessedFiles, m.progress.TotalFiles))
	sb.WriteString(fmt.Sprintf("  %s %d / %d chunks\n",
		styleLabel.Render("Chunks: "),
		m.progress.ProcessedChunks, m.progress.TotalChunks))

	elapsed := m.progress.ElapsedSeconds()
	remaining := m.progress.EstimatedRemainingSeconds()
	sb.WriteString(fmt.Sprintf("  %s %s   %s %s\n\n",
		styleLabel.Render("Elapsed:"),
		formatDuration(elapsed),
		styleLabel.Render("ETA:"),
		formatDuration(remaining),
	))

	// Current file
	if m.progress.CurrentFile != "" {
		maxFile := w - 16
		sb.WriteString(fmt.Sprintf("  %s %s\n\n",
			styleLabel.Render("Processing:"),
			truncate(m.progress.CurrentFile, maxFile),
		))
	}

	// Worker pool
	if len(m.workers) > 0 {
		sb.WriteString(styleLabel.Render("  Workers:\n"))
		for _, w := range m.workers {
			icon := styleOK.Render("●")
			label := styleDim.Render("idle")
			if !w.Idle {
				icon = styleWarn.Render("◉")
				label = truncate(w.File, m.termWidth-14)
			}
			if w.ErrMsg != "" {
				icon = styleErr.Render("✗")
				label = styleErr.Render(w.ErrMsg)
			}
			sb.WriteString(fmt.Sprintf("    [%d] %s  %s\n", w.ID, icon, label))
		}
		sb.WriteString("\n")
	}

	if m.done {
		sb.WriteString(styleOK.Render("  ✓ Extraction complete!"))
		sb.WriteString("\n")
	} else {
		sb.WriteString(styleDim.Render("  press q to quit"))
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m Model) viewQuery() string {
	w := m.termWidth
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(styleHeader.Render(" Vela — Query Mode "))
	sb.WriteString("\n\n")

	// Result box
	if m.queryResult != "" {
		resultStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(0, 1).
			Width(w - 4)
		sb.WriteString(resultStyle.Render(m.queryResult))
		sb.WriteString("\n\n")
	}

	// Input prompt
	sb.WriteString(stylePrompt.Render("  › "))
	sb.WriteString(m.queryInput)
	sb.WriteString(styleDim.Render("█")) // cursor
	sb.WriteString("\n\n")

	sb.WriteString(styleDim.Render("  Commands: path <A> <B>  ·  explain <node>  ·  quit"))
	sb.WriteString("\n")

	return sb.String()
}

// ---------------------------------------------------------------------------
// Commands (blocking channel reads)
// ---------------------------------------------------------------------------

func waitForProgress(ch <-chan types.ProgressUpdate) tea.Cmd {
	return func() tea.Msg {
		u, ok := <-ch
		if !ok {
			return progressMsg(types.ProgressUpdate{IsComplete: true})
		}
		return progressMsg(u)
	}
}

func waitForWorker(ch <-chan WorkerUpdate) tea.Cmd {
	return func() tea.Msg {
		u, ok := <-ch
		if !ok {
			return nil
		}
		return workerMsg(u)
	}
}

// ---------------------------------------------------------------------------
// Plain-text fallback
// ---------------------------------------------------------------------------

// RunPlainProgress reads from ch and prints progress lines to stdout.
// Used when stdout is not a TTY.
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

// ---------------------------------------------------------------------------
// TTY detection
// ---------------------------------------------------------------------------

func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
