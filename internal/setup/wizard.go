package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// WizardState represents the current screen in the setup wizard.
type WizardState int

const (
	StateWelcome WizardState = iota
	StateClientChoice
	StateInstalling
	StateSuccess
	StateError
)

type WizardModel struct {
	state       WizardState
	err         error
	quitting    bool
	clientIndex int // 0=OpenCode, 1=Claude Desktop, 2=Skip
	installing  bool
	message     string
}

func NewWizard() WizardModel {
	return WizardModel{
		state:       StateWelcome,
		clientIndex: 0,
	}
}

// Quitting returns true if the wizard wants to exit.
func (m WizardModel) Quitting() bool {
	return m.quitting || m.state == StateSuccess || m.state == StateError
}

func (m WizardModel) Init() tea.Cmd {
	return nil
}

type installCompleteMsg struct {
	success bool
	err     error
}

func (m WizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit

		case "enter", " ":
			switch m.state {
			case StateWelcome:
				m.state = StateClientChoice
				return m, nil

			case StateClientChoice:
				// Install to selected client
				m.state = StateInstalling
				return m, m.installMCP()

			case StateSuccess, StateError:
				return m, tea.Quit
			}

		case "up", "k":
			if m.state == StateClientChoice && m.clientIndex > 0 {
				m.clientIndex--
			}

		case "down", "j":
			if m.state == StateClientChoice && m.clientIndex < 2 {
				m.clientIndex++
			}

		case "s", "n":
			if m.state == StateClientChoice {
				// Skip installation
				m.state = StateSuccess
				m.message = "Installation skipped"
				return m, nil
			}
		}

	case installCompleteMsg:
		if msg.success {
			m.state = StateSuccess
			m.message = "Installation complete"
		} else {
			m.state = StateError
			m.err = msg.err
		}
		return m, nil
	}

	return m, nil
}

func (m WizardModel) View() string {
	if m.quitting {
		return ""
	}

	switch m.state {
	case StateWelcome:
		return m.viewWelcome()
	case StateClientChoice:
		return m.viewClientChoice()
	case StateInstalling:
		return m.viewInstalling()
	case StateSuccess:
		return m.viewSuccess()
	case StateError:
		return m.viewError()
	}
	return ""
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

var (
	boxStyle = lipgloss.NewStyle().
			Padding(1, 2).
			Width(80)

	headerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true)

	textStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	cursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39"))
)

func (m WizardModel) viewWelcome() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Vela MCP Setup Wizard"))
	b.WriteString("\n\n")
	b.WriteString(textStyle.Render("This wizard will configure Vela as an MCP server for:"))
	b.WriteString("\n")
	b.WriteString(textStyle.Render("  • OpenCode (Anthropic's CLI)"))
	b.WriteString("\n")
	b.WriteString(textStyle.Render("  • Claude Desktop"))
	b.WriteString("\n\n")
	b.WriteString(textStyle.Render("Press Enter to continue, or q to quit"))
	return boxStyle.Render(b.String())
}

func (m WizardModel) viewClientChoice() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Choose Installation Target"))
	b.WriteString("\n\n")

	clients := []string{
		"OpenCode (Anthropic CLI)",
		"Claude Desktop",
		"Skip installation",
	}

	for i, client := range clients {
		cursor := "  "
		style := textStyle
		if i == m.clientIndex {
			cursor = cursorStyle.Render("▸ ")
			style = cursorStyle
		}
		b.WriteString(cursor + style.Render(client) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(textStyle.Render("↑↓ navigate • Enter select • s skip • q quit"))
	return boxStyle.Render(b.String())
}

func (m WizardModel) viewInstalling() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Installing MCP Configuration"))
	b.WriteString("\n\n")
	b.WriteString(textStyle.Render("Configuring Vela..."))
	b.WriteString("\n")
	return boxStyle.Render(b.String())
}

func (m WizardModel) viewSuccess() string {
	var b strings.Builder
	b.WriteString(successStyle.Render("✓ Setup Complete"))
	b.WriteString("\n\n")
	b.WriteString(textStyle.Render(m.message))
	b.WriteString("\n\n")
	b.WriteString(textStyle.Render("Press Enter to exit"))
	return boxStyle.Render(b.String())
}

func (m WizardModel) viewError() string {
	var b strings.Builder
	b.WriteString(errorStyle.Render("✗ Setup Failed"))
	b.WriteString("\n\n")
	b.WriteString(textStyle.Render(fmt.Sprintf("Error: %v", m.err)))
	b.WriteString("\n\n")
	b.WriteString(textStyle.Render("Press Enter to exit"))
	return boxStyle.Render(b.String())
}

// ---------------------------------------------------------------------------
// Installation Logic
// ---------------------------------------------------------------------------

func (m WizardModel) installMCP() tea.Cmd {
	return func() tea.Msg {
		var configPath string
		var err error

		switch m.clientIndex {
		case 0: // OpenCode
			configPath, err = getOpenCodeConfigPath(), nil
		case 1: // Claude Desktop
			configPath, err = getClaudeDesktopConfigPath(), nil
		case 2: // Skip
			return installCompleteMsg{success: true, err: nil}
		}

		if err != nil {
			return installCompleteMsg{success: false, err: err}
		}

		if configPath == "" {
			return installCompleteMsg{success: false, err: fmt.Errorf("could not determine config path for platform %s", runtime.GOOS)}
		}

		// Create config directory if it doesn't exist
		configDir := filepath.Dir(configPath)
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return installCompleteMsg{success: false, err: err}
		}

		// Read existing config or create new one
		var cfg map[string]interface{}
		data, err := os.ReadFile(configPath)
		if err != nil {
			// File doesn't exist, create new config
			cfg = map[string]interface{}{
				"mcpServers": map[string]interface{}{},
			}
		} else {
			if err := json.Unmarshal(data, &cfg); err != nil {
				return installCompleteMsg{success: false, err: err}
			}
		}

		// Ensure mcpServers exists
		mcpServers, ok := cfg["mcpServers"].(map[string]interface{})
		if !ok {
			mcpServers = map[string]interface{}{}
			cfg["mcpServers"] = mcpServers
		}

		// Add vela server
		mcpServers["vela"] = map[string]interface{}{
			"command": "vela",
			"args":    []string{"serve"},
		}

		// Write config back
		data, err = json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return installCompleteMsg{success: false, err: err}
		}

		if err := os.WriteFile(configPath, data, 0644); err != nil {
			return installCompleteMsg{success: false, err: err}
		}

		return installCompleteMsg{success: true, err: nil}
	}
}
