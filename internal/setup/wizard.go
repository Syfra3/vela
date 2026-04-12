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
	StateOllamaCheck
	StateInstallOllama
	StateStartOllama
	StateModelChoice
	StatePullingModel
	StateAPIKeyChoice
	StateClientChoice
	StateInstallingMCP
	StateSuccess
	StateError
)

type WizardModel struct {
	state    WizardState
	err      error
	quitting bool
	message  string

	// Ollama state
	ollamaInstalled bool
	ollamaRunning   bool
	ollamaPath      string
	ollamaModels    []string
	selectedModel   string
	modelIndex      int

	// API keys state
	useRemoteLLM  bool
	providerIndex int // 0=local, 1=anthropic, 2=openai
	anthropicKey  string
	openaiKey     string
	keyInputMode  string // "anthropic", "openai", ""
	keyInput      string

	// MCP state
	clientIndex int // 0=OpenCode, 1=Claude Desktop, 2=Skip

	// Progress tracking
	installing bool
}

func NewWizard() WizardModel {
	return WizardModel{
		state:         StateWelcome,
		clientIndex:   0,
		providerIndex: 0,
	}
}

func (m WizardModel) Init() tea.Cmd {
	return nil
}

func (m WizardModel) Quitting() bool {
	return m.quitting || m.state == StateSuccess || m.state == StateError
}

// Messages
type ollamaCheckMsg struct {
	installed bool
	running   bool
	path      string
	err       error
}

type ollamaInstallMsg struct {
	success bool
	err     error
}

type ollamaStartMsg struct {
	success bool
	err     error
}

type modelListMsg struct {
	models []string
	err    error
}

type modelPullMsg struct {
	success bool
	err     error
}

type mcpInstallMsg struct {
	success bool
	err     error
}

func (m WizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case ollamaCheckMsg:
		m.ollamaInstalled = msg.installed
		m.ollamaRunning = msg.running
		m.ollamaPath = msg.path
		m.err = msg.err

		if !m.ollamaInstalled {
			m.state = StateInstallOllama
		} else if !m.ollamaRunning {
			m.state = StateStartOllama
		} else {
			m.state = StateModelChoice
			return m, m.checkModels()
		}

	case ollamaInstallMsg:
		if msg.success {
			m.state = StateStartOllama
			return m, m.startOllama()
		} else {
			m.state = StateError
			m.err = msg.err
		}

	case ollamaStartMsg:
		if msg.success {
			m.state = StateModelChoice
			return m, m.checkModels()
		} else {
			m.state = StateError
			m.err = msg.err
		}

	case modelListMsg:
		m.ollamaModels = msg.models
		if len(m.ollamaModels) == 0 {
			// No models — suggest llama3
			m.selectedModel = "llama3"
		}

	case modelPullMsg:
		if msg.success {
			m.state = StateAPIKeyChoice
		} else {
			m.state = StateError
			m.err = msg.err
		}

	case mcpInstallMsg:
		if msg.success {
			m.state = StateSuccess
			m.message = "Setup complete"
		} else {
			m.state = StateError
			m.err = msg.err
		}
	}

	return m, nil
}

func (m WizardModel) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.quitting = true
		return m, nil

	case "enter", " ":
		return m.handleEnter()

	case "up", "k":
		return m.handleUp(), nil

	case "down", "j":
		return m.handleDown(), nil

	case "s", "n":
		return m.handleSkip()

	case "backspace":
		if m.keyInputMode != "" && len(m.keyInput) > 0 {
			m.keyInput = m.keyInput[:len(m.keyInput)-1]
		}

	default:
		// Append character for key input
		if m.keyInputMode != "" && len(msg.String()) == 1 {
			m.keyInput += msg.String()
		}
	}

	return m, nil
}

func (m WizardModel) handleEnter() (tea.Model, tea.Cmd) {
	switch m.state {
	case StateWelcome:
		m.state = StateOllamaCheck
		return m, m.checkOllama()

	case StateInstallOllama:
		m.state = StateInstallingMCP // temp state while installing
		return m, m.installOllama()

	case StateStartOllama:
		m.state = StateInstallingMCP
		return m, m.startOllama()

	case StateModelChoice:
		if len(m.ollamaModels) > 0 && m.modelIndex < len(m.ollamaModels) {
			m.selectedModel = m.ollamaModels[m.modelIndex]
			m.state = StateAPIKeyChoice
		} else {
			// Pull llama3
			m.selectedModel = "llama3"
			m.state = StatePullingModel
			return m, m.pullModel()
		}

	case StateAPIKeyChoice:
		// Move to MCP config
		m.state = StateClientChoice

	case StateClientChoice:
		m.state = StateInstallingMCP
		return m, m.installMCP()

	case StateSuccess, StateError:
		m.quitting = true
		return m, nil
	}

	return m, nil
}

func (m WizardModel) handleUp() WizardModel {
	switch m.state {
	case StateModelChoice:
		if m.modelIndex > 0 {
			m.modelIndex--
		}
	case StateAPIKeyChoice:
		if m.providerIndex > 0 {
			m.providerIndex--
		}
	case StateClientChoice:
		if m.clientIndex > 0 {
			m.clientIndex--
		}
	}
	return m
}

func (m WizardModel) handleDown() WizardModel {
	switch m.state {
	case StateModelChoice:
		maxIndex := len(m.ollamaModels)
		if m.modelIndex < maxIndex {
			m.modelIndex++
		}
	case StateAPIKeyChoice:
		if m.providerIndex < 2 {
			m.providerIndex++
		}
	case StateClientChoice:
		if m.clientIndex < 2 {
			m.clientIndex++
		}
	}
	return m
}

func (m WizardModel) handleSkip() (tea.Model, tea.Cmd) {
	switch m.state {
	case StateInstallOllama:
		// Skip local LLM — go to API key choice
		m.state = StateAPIKeyChoice
		m.useRemoteLLM = true

	case StateModelChoice:
		// Skip model pull — go to API keys
		m.state = StateAPIKeyChoice

	case StateAPIKeyChoice:
		// Skip API keys — go to MCP
		m.state = StateClientChoice

	case StateClientChoice:
		// Skip MCP install
		m.state = StateSuccess
		m.message = "Setup skipped"
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
	case StateOllamaCheck:
		return m.viewChecking("Checking Ollama installation...")
	case StateInstallOllama:
		return m.viewInstallOllama()
	case StateStartOllama:
		return m.viewStartOllama()
	case StateModelChoice:
		return m.viewModelChoice()
	case StatePullingModel:
		return m.viewPulling()
	case StateAPIKeyChoice:
		return m.viewAPIKeyChoice()
	case StateClientChoice:
		return m.viewClientChoice()
	case StateInstallingMCP:
		return m.viewInstalling()
	case StateSuccess:
		return m.viewSuccess()
	case StateError:
		return m.viewError()
	}
	return ""
}

// ---------------------------------------------------------------------------
// View Helpers
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
	b.WriteString(headerStyle.Render("Vela Setup Wizard"))
	b.WriteString("\n\n")
	b.WriteString(textStyle.Render("This wizard will:"))
	b.WriteString("\n")
	b.WriteString(textStyle.Render("  1. Check/install Ollama (local LLM runtime)"))
	b.WriteString("\n")
	b.WriteString(textStyle.Render("  2. Download a model (llama3 recommended)"))
	b.WriteString("\n")
	b.WriteString(textStyle.Render("  3. Optionally configure remote LLM (Anthropic/OpenAI)"))
	b.WriteString("\n")
	b.WriteString(textStyle.Render("  4. Configure MCP for OpenCode/Claude Desktop"))
	b.WriteString("\n\n")
	b.WriteString(textStyle.Render("Press Enter to begin, or q to quit"))
	return boxStyle.Render(b.String())
}

func (m WizardModel) viewChecking(msg string) string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Setup"))
	b.WriteString("\n\n")
	b.WriteString(textStyle.Render(msg))
	b.WriteString("\n")
	return boxStyle.Render(b.String())
}

func (m WizardModel) viewInstallOllama() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Install Ollama"))
	b.WriteString("\n\n")
	b.WriteString(textStyle.Render("Ollama not found. Install now?"))
	b.WriteString("\n\n")
	b.WriteString(textStyle.Render(fmt.Sprintf("Platform: %s/%s", runtime.GOOS, runtime.GOARCH)))
	b.WriteString("\n")
	b.WriteString(textStyle.Render("Method: brew (macOS) or curl (Linux)"))
	b.WriteString("\n\n")
	b.WriteString(textStyle.Render("Enter install • s skip (use remote LLM) • q quit"))
	return boxStyle.Render(b.String())
}

func (m WizardModel) viewStartOllama() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Start Ollama"))
	b.WriteString("\n\n")
	b.WriteString(textStyle.Render(fmt.Sprintf("Ollama installed: %s", m.ollamaPath)))
	b.WriteString("\n")
	b.WriteString(textStyle.Render("But not running. Start now?"))
	b.WriteString("\n\n")
	b.WriteString(textStyle.Render("Enter start • s skip • q quit"))
	return boxStyle.Render(b.String())
}

func (m WizardModel) viewModelChoice() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Choose Model"))
	b.WriteString("\n\n")

	if len(m.ollamaModels) > 0 {
		b.WriteString(textStyle.Render("Installed models:"))
		b.WriteString("\n")
		for i, model := range m.ollamaModels {
			cursor := "  "
			style := textStyle
			if i == m.modelIndex {
				cursor = cursorStyle.Render("▸ ")
				style = cursorStyle
			}
			b.WriteString(cursor + style.Render(model) + "\n")
		}

		// Add "Pull llama3" option
		cursor := "  "
		style := textStyle
		if m.modelIndex == len(m.ollamaModels) {
			cursor = cursorStyle.Render("▸ ")
			style = cursorStyle
		}
		b.WriteString(cursor + style.Render("Pull llama3 (recommended)") + "\n")
	} else {
		b.WriteString(textStyle.Render("No models found. Pull llama3?"))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(textStyle.Render("↑↓ navigate • Enter select • s skip • q quit"))
	return boxStyle.Render(b.String())
}

func (m WizardModel) viewPulling() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Pulling Model"))
	b.WriteString("\n\n")
	b.WriteString(textStyle.Render(fmt.Sprintf("Downloading %s...", m.selectedModel)))
	b.WriteString("\n")
	return boxStyle.Render(b.String())
}

func (m WizardModel) viewAPIKeyChoice() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("LLM Provider"))
	b.WriteString("\n\n")
	b.WriteString(textStyle.Render("Choose default LLM provider:"))
	b.WriteString("\n\n")

	providers := []string{
		fmt.Sprintf("Local (%s)", m.selectedModel),
		"Anthropic Claude",
		"OpenAI GPT",
	}

	for i, provider := range providers {
		cursor := "  "
		style := textStyle
		if i == m.providerIndex {
			cursor = cursorStyle.Render("▸ ")
			style = cursorStyle
		}
		b.WriteString(cursor + style.Render(provider) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(textStyle.Render("↑↓ navigate • Enter select • s skip • q quit"))
	return boxStyle.Render(b.String())
}

func (m WizardModel) viewClientChoice() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("MCP Configuration"))
	b.WriteString("\n\n")
	b.WriteString(textStyle.Render("Install Vela MCP server for:"))
	b.WriteString("\n\n")

	clients := []string{
		"OpenCode (Anthropic CLI)",
		"Claude Desktop",
		"Skip MCP installation",
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
	b.WriteString(headerStyle.Render("Installing"))
	b.WriteString("\n\n")
	b.WriteString(textStyle.Render("Configuring Vela..."))
	b.WriteString("\n")
	return boxStyle.Render(b.String())
}

func (m WizardModel) viewSuccess() string {
	var b strings.Builder
	b.WriteString(successStyle.Render("✓ Setup Complete"))
	b.WriteString("\n\n")

	if m.ollamaInstalled {
		b.WriteString(textStyle.Render(fmt.Sprintf("✓ Ollama: %s", m.ollamaPath)))
		b.WriteString("\n")
	}
	if m.selectedModel != "" {
		b.WriteString(textStyle.Render(fmt.Sprintf("✓ Model: %s", m.selectedModel)))
		b.WriteString("\n")
	}
	if m.clientIndex < 2 {
		clients := []string{"OpenCode", "Claude Desktop"}
		b.WriteString(textStyle.Render(fmt.Sprintf("✓ MCP: %s", clients[m.clientIndex])))
		b.WriteString("\n")
	}

	b.WriteString("\n")
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
// Commands
// ---------------------------------------------------------------------------

func (m WizardModel) checkOllama() tea.Cmd {
	return func() tea.Msg {
		installed, running, path, err := CheckOllama()
		return ollamaCheckMsg{
			installed: installed,
			running:   running,
			path:      path,
			err:       err,
		}
	}
}

func (m WizardModel) installOllama() tea.Cmd {
	return func() tea.Msg {
		err := InstallOllama()
		return ollamaInstallMsg{
			success: err == nil,
			err:     err,
		}
	}
}

func (m WizardModel) startOllama() tea.Cmd {
	return func() tea.Msg {
		err := StartOllama()
		return ollamaStartMsg{
			success: err == nil,
			err:     err,
		}
	}
}

func (m WizardModel) checkModels() tea.Cmd {
	return func() tea.Msg {
		models, err := GetOllamaModels()
		return modelListMsg{
			models: models,
			err:    err,
		}
	}
}

func (m WizardModel) pullModel() tea.Cmd {
	return func() tea.Msg {
		err := PullModel(m.selectedModel)
		return modelPullMsg{
			success: err == nil,
			err:     err,
		}
	}
}

func (m WizardModel) installMCP() tea.Cmd {
	return func() tea.Msg {
		var configPath string

		switch m.clientIndex {
		case 0: // OpenCode
			configPath = getOpenCodeConfigPath()
		case 1: // Claude Desktop
			configPath = getClaudeDesktopConfigPath()
		case 2: // Skip
			return mcpInstallMsg{success: true, err: nil}
		}

		if configPath == "" {
			return mcpInstallMsg{success: false, err: fmt.Errorf("could not determine config path for platform %s", runtime.GOOS)}
		}

		// Create config directory
		configDir := filepath.Dir(configPath)
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return mcpInstallMsg{success: false, err: err}
		}

		// Read or create config
		var cfg map[string]interface{}
		data, err := os.ReadFile(configPath)
		if err != nil {
			cfg = map[string]interface{}{
				"mcpServers": map[string]interface{}{},
			}
		} else {
			if err := json.Unmarshal(data, &cfg); err != nil {
				return mcpInstallMsg{success: false, err: err}
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

		// Write config
		data, err = json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return mcpInstallMsg{success: false, err: err}
		}

		if err := os.WriteFile(configPath, data, 0644); err != nil {
			return mcpInstallMsg{success: false, err: err}
		}

		return mcpInstallMsg{success: true, err: nil}
	}
}
