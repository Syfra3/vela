package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// extractStep tracks which sub-screen is active inside the Extract screen.
type extractStep int

const (
	stepPath       extractStep = iota // user types the directory path
	stepMode                          // user chooses Update vs Regenerate (only shown when graph.json exists)
	stepExtracting                    // extraction is running
)

// ExtractionMode controls whether the cache is honoured.
type ExtractionMode int

const (
	ModeUpdate     ExtractionMode = iota // incremental — respect cache
	ModeRegenerate                       // full — ignore cache
)

// ExtractModel is the TUI model for extraction configuration.
type ExtractModel struct {
	step           extractStep
	directoryInput string
	modeCursor     int // 0 = Update, 1 = Regenerate
	mode           ExtractionMode
	quitting       bool
	starting       bool
	extracting     bool
	err            error

	// Progress tracking
	progress       progress.Model
	currentFile    string
	processedFiles int
	totalFiles     int
}

func NewExtractModel() ExtractModel {
	cwd, _ := os.Getwd()
	prog := progress.New(progress.WithDefaultGradient())
	return ExtractModel{
		step:           stepPath,
		directoryInput: cwd,
		modeCursor:     0,
		mode:           ModeUpdate,
		progress:       prog,
	}
}

func (m ExtractModel) Init() tea.Cmd { return nil }

func (m ExtractModel) Quitting() bool       { return m.quitting }
func (m ExtractModel) Starting() bool       { return m.starting }
func (m ExtractModel) Directory() string    { return m.directoryInput }
func (m ExtractModel) Mode() ExtractionMode { return m.mode }

func (m ExtractModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.extracting {
			if msg.String() == "ctrl+c" || msg.String() == "esc" {
				m.quitting = true
			}
			return m, nil
		}

		switch m.step {
		case stepPath:
			return m.updatePath(msg)
		case stepMode:
			return m.updateMode(msg)
		}
	}
	return m, nil
}

func (m ExtractModel) updatePath(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.quitting = true
		return m, nil

	case "enter":
		if _, err := os.Stat(m.directoryInput); err != nil {
			m.err = fmt.Errorf("directory does not exist: %s", m.directoryInput)
			return m, nil
		}
		m.err = nil
		// Check whether a previous graph exists for this path
		if graphExists(m.directoryInput) {
			m.step = stepMode
		} else {
			// First run — skip mode selection, go straight to extraction
			m.mode = ModeRegenerate
			m.starting = true
		}
		return m, nil

	case "backspace":
		if len(m.directoryInput) > 0 {
			m.directoryInput = m.directoryInput[:len(m.directoryInput)-1]
		}

	default:
		if len(msg.String()) == 1 {
			m.directoryInput += msg.String()
		}
	}
	return m, nil
}

func (m ExtractModel) updateMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		// Go back to path input
		m.step = stepPath
		return m, nil

	case "up", "k":
		if m.modeCursor > 0 {
			m.modeCursor--
		}
	case "down", "j":
		if m.modeCursor < 1 {
			m.modeCursor++
		}

	case "enter", " ":
		if m.modeCursor == 0 {
			m.mode = ModeUpdate
		} else {
			m.mode = ModeRegenerate
		}
		m.starting = true
		return m, nil
	}
	return m, nil
}

// graphExists returns true when vela-out/graph.json is present and non-empty
// relative to the given extraction directory.
func graphExists(dir string) bool {
	// Use a fixed output path relative to cwd, same as the extraction worker.
	info, err := os.Stat("vela-out/graph.json")
	if err != nil {
		return false
	}
	return info.Size() > 0
}

// View delegates to ViewContent (menu wraps with header/footer).
func (m ExtractModel) View() string { return m.ViewContent() }

func (m ExtractModel) ViewContent() string {
	var b strings.Builder

	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	inputStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	accentStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(colorSubtext)
	selectedStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	normalStyle := lipgloss.NewStyle().Foreground(colorText)

	switch m.step {
	case stepPath:
		b.WriteString(textStyle.Render("Enter the directory path to extract:"))
		b.WriteString("\n\n")
		b.WriteString(inputStyle.Render("> " + m.directoryInput))
		b.WriteString("\n\n")
		if m.err != nil {
			b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
			b.WriteString("\n\n")
		}

	case stepMode:
		b.WriteString(accentStyle.Render("Previous extraction found."))
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render(fmt.Sprintf("Path: %s", m.directoryInput)))
		b.WriteString("\n\n")
		b.WriteString(textStyle.Render("Choose extraction mode:"))
		b.WriteString("\n\n")

		modeOptions := []struct {
			label string
			desc  string
		}{
			{"Update", "Incremental — only re-extract changed files (fast)"},
			{"Regenerate", "Full re-extraction — ignore cache, rebuild from scratch"},
		}

		for i, opt := range modeOptions {
			cursor := "  "
			labelS := normalStyle
			descS := mutedStyle
			if i == m.modeCursor {
				cursor = lipgloss.NewStyle().Foreground(colorAccent).Render("▸ ")
				labelS = selectedStyle
			}
			b.WriteString(fmt.Sprintf("%s%s\n", cursor, labelS.Render(opt.label)))
			b.WriteString(fmt.Sprintf("    %s\n\n", descS.Render(opt.desc)))
		}

	case stepExtracting:
		b.WriteString(textStyle.Render("Extracting knowledge graph..."))
		b.WriteString("\n\n")

		if m.totalFiles > 0 {
			percent := float64(m.processedFiles) / float64(m.totalFiles)
			b.WriteString(m.progress.ViewAs(percent))
			b.WriteString("\n\n")
			b.WriteString(textStyle.Render(fmt.Sprintf(
				"Progress: %d/%d files (%.0f%%)",
				m.processedFiles, m.totalFiles, percent*100,
			)))
			b.WriteString("\n")
			if m.currentFile != "" {
				b.WriteString(textStyle.Render(fmt.Sprintf("Current: %s", m.currentFile)))
			}
		} else {
			b.WriteString(textStyle.Render("Discovering files..."))
		}
		b.WriteString("\n\n")
	}

	return b.String()
}

// FooterHelp returns context-appropriate footer text for the current step.
func (m ExtractModel) FooterHelp() string {
	switch m.step {
	case stepPath:
		return "Enter confirm • esc back to menu"
	case stepMode:
		return "↑↓ select mode • Enter confirm • esc back"
	default:
		return "esc cancel"
	}
}
