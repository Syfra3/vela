package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// QueryMode represents the query operation type.
type QueryMode int

const (
	QueryPath QueryMode = iota
	QueryExplain
	QueryNodes
)

// QueryModel is the TUI model for query operations.
type QueryModel struct {
	mode      QueryMode
	input     string
	result    string
	cursor    int
	quitting  bool
	executing bool
	err       error
}

func NewQueryModel() QueryModel {
	return QueryModel{
		mode:   QueryPath,
		cursor: 0,
	}
}

func (m QueryModel) Init() tea.Cmd {
	return nil
}

func (m QueryModel) Quitting() bool {
	return m.quitting
}

func (m QueryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.quitting = true
			return m, nil

		case "tab":
			// Cycle through query modes
			m.mode = (m.mode + 1) % 3
			m.result = ""
			m.err = nil

		case "enter":
			if m.input == "" {
				m.err = fmt.Errorf("query cannot be empty")
				return m, nil
			}
			// TODO: Execute query based on mode
			m.result = fmt.Sprintf("Query result for '%s' (mode: %v) - TODO: wire query engine", m.input, m.mode)
			return m, nil

		case "backspace":
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}

		default:
			// Append character
			if len(msg.String()) == 1 {
				m.input += msg.String()
			}
		}
	}
	return m, nil
}

func (m QueryModel) View() string {
	// Content only — menu will wrap with header/footer
	return m.ViewContent()
}

func (m QueryModel) ViewContent() string {
	var b strings.Builder

	textStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196"))

	inputStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("42")).
		Bold(true)

	resultStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Width(76).
		Padding(1)

	// Mode name will be in section header
	modeNames := []string{"Path", "Explain", "Nodes"}
	_ = modeNames // Used by menu

	// Mode description
	modeDesc := map[QueryMode]string{
		QueryPath:    "Find path between two nodes (e.g., 'NodeA -> NodeB')",
		QueryExplain: "Explain a concept using LLM (e.g., 'authentication')",
		QueryNodes:   "Search for nodes by pattern (e.g., 'User*')",
	}
	b.WriteString(textStyle.Render(modeDesc[m.mode]))
	b.WriteString("\n\n")

	// Input
	b.WriteString(inputStyle.Render("> " + m.input))
	b.WriteString("\n\n")

	// Error
	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		b.WriteString("\n\n")
	}

	// Result
	if m.result != "" {
		b.WriteString(textStyle.Render("Result:"))
		b.WriteString("\n")
		b.WriteString(resultStyle.Render(m.result))
		b.WriteString("\n\n")
	}

	return b.String()
}

func (m QueryModel) ModeName() string {
	modeNames := []string{"Path", "Explain", "Nodes"}
	return modeNames[m.mode]
}
