package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ExtractModel is the TUI model for extraction configuration.
type ExtractModel struct {
	directoryInput string
	cursor         int
	quitting       bool
	starting       bool
	err            error
}

func NewExtractModel() ExtractModel {
	cwd, _ := os.Getwd()
	return ExtractModel{
		directoryInput: cwd,
		cursor:         0,
	}
}

func (m ExtractModel) Init() tea.Cmd {
	return nil
}

func (m ExtractModel) Quitting() bool {
	return m.quitting
}

func (m ExtractModel) Starting() bool {
	return m.starting
}

func (m ExtractModel) Directory() string {
	return m.directoryInput
}

func (m ExtractModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.quitting = true
			return m, nil

		case "enter":
			// Validate directory exists
			if _, err := os.Stat(m.directoryInput); err != nil {
				m.err = fmt.Errorf("directory does not exist: %s", m.directoryInput)
				return m, nil
			}
			m.starting = true
			return m, nil

		case "backspace":
			if len(m.directoryInput) > 0 {
				m.directoryInput = m.directoryInput[:len(m.directoryInput)-1]
			}

		default:
			// Append character
			if len(msg.String()) == 1 {
				m.directoryInput += msg.String()
			}
		}
	}
	return m, nil
}

func (m ExtractModel) View() string {
	// Content only — menu will wrap with header/footer
	return m.ViewContent()
}

func (m ExtractModel) ViewContent() string {
	var b strings.Builder

	textStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196"))

	inputStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("42")).
		Bold(true)

	b.WriteString(textStyle.Render("Enter the directory path to extract:"))
	b.WriteString("\n\n")
	b.WriteString(inputStyle.Render("> " + m.directoryInput))
	b.WriteString("\n\n")

	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		b.WriteString("\n\n")
	}

	return b.String()
}
