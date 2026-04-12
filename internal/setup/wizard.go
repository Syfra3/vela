package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Step-by-step linear wizard flow:
// 1. Check system requirements
// 2. Choose local vs remote LLM
// 3. Configure local (install Ollama + model) OR configure remote (API keys)
// 4. Configure MCP
// 5. Validate installation
// 6. Finish

type WizardStep int

const (
	StepWelcome WizardStep = iota
	StepSystemCheck
	StepProviderChoice
	StepLocalSetup  // Install Ollama + start + pull model
	StepRemoteSetup // Configure API keys
	StepMCPConfig
	StepValidation
	StepComplete
	StepError
)

type WizardModel struct {
	step     WizardStep
	err      error
	quitting bool
	message  []string // Progress messages

	// System check results
	sysOS     string
	sysArch   string
	sysOK     bool
	sysIssues []string

	// Provider choice
	providerChoice int // 0=local, 1=remote

	// Local setup state
	ollamaInstalled bool
	ollamaRunning   bool
	ollamaPath      string
	modelPulled     bool
	selectedModel   string

	// Remote setup state
	remoteProvider int // 0=anthropic, 1=openai
	apiKey         string
	keyInput       string

	// MCP config
	mcpTarget     int // 0=OpenCode, 1=Claude Desktop, 2=Skip
	mcpConfigured bool

	// Validation results
	llmHealthy bool
	mcpHealthy bool

	// UI state
	cursor  int
	working bool
}

func NewWizard() WizardModel {
	return WizardModel{
		step:           StepWelcome,
		providerChoice: 0,
		selectedModel:  "llama3",
	}
}

func (m WizardModel) Init() tea.Cmd {
	return nil
}

func (m WizardModel) Quitting() bool {
	return m.quitting || m.step == StepComplete || m.step == StepError
}

// Messages
type systemCheckMsg struct {
	ok     bool
	os     string
	arch   string
	issues []string
}

type ollamaCheckMsg struct {
	installed bool
	running   bool
	path      string
}

type ollamaInstallMsg struct {
	success bool
	err     error
}

type ollamaStartMsg struct {
	success bool
	err     error
}

type modelPullMsg struct {
	success bool
	err     error
}

type mcpInstallMsg struct {
	success bool
	err     error
}

type validationMsg struct {
	llmOK bool
	mcpOK bool
	err   error
}

func (m WizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case systemCheckMsg:
		m.sysOK = msg.ok
		m.sysOS = msg.os
		m.sysArch = msg.arch
		m.sysIssues = msg.issues
		m.working = false

		if msg.ok {
			m.step = StepProviderChoice
		} else {
			m.step = StepError
			m.err = fmt.Errorf("system requirements not met: %v", msg.issues)
		}

	case ollamaCheckMsg:
		m.ollamaInstalled = msg.installed
		m.ollamaRunning = msg.running
		m.ollamaPath = msg.path
		m.working = false

		// Continue to next local setup phase
		if !m.ollamaInstalled {
			return m, m.installOllama()
		} else if !m.ollamaRunning {
			return m, m.startOllama()
		} else {
			return m, m.pullModel()
		}

	case ollamaInstallMsg:
		m.working = false
		if msg.success {
			m.ollamaInstalled = true
			m.addMessage("✓ Ollama installed")
			return m, m.startOllama()
		} else {
			m.step = StepError
			m.err = msg.err
		}

	case ollamaStartMsg:
		m.working = false
		if msg.success {
			m.ollamaRunning = true
			m.addMessage("✓ Ollama started")
			time.Sleep(2 * time.Second) // Give Ollama time to start
			return m, m.pullModel()
		} else {
			m.step = StepError
			m.err = msg.err
		}

	case modelPullMsg:
		m.working = false
		if msg.success {
			m.modelPulled = true
			m.addMessage(fmt.Sprintf("✓ Model %s ready", m.selectedModel))
			m.step = StepMCPConfig
		} else {
			m.step = StepError
			m.err = msg.err
		}

	case mcpInstallMsg:
		m.working = false
		if msg.success {
			m.mcpConfigured = true
			m.addMessage("✓ MCP configured")
			m.step = StepValidation
			return m, m.validate()
		} else {
			m.step = StepError
			m.err = msg.err
		}

	case validationMsg:
		m.working = false
		m.llmHealthy = msg.llmOK
		m.mcpHealthy = msg.mcpOK

		if msg.llmOK {
			m.addMessage("✓ LLM provider healthy")
		}
		if msg.mcpOK {
			m.addMessage("✓ MCP server ready")
		}

		m.step = StepComplete
	}

	return m, nil
}

func (m WizardModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.working {
		// Block input while working
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c", "esc":
		m.quitting = true
		return m, nil

	case "enter", " ":
		return m.handleEnter()

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}

	case "down", "j":
		maxCursor := m.getMaxCursor()
		if m.cursor < maxCursor {
			m.cursor++
		}

	case "backspace":
		if m.step == StepRemoteSetup && len(m.keyInput) > 0 {
			m.keyInput = m.keyInput[:len(m.keyInput)-1]
		}

	default:
		// Key input for API keys
		if m.step == StepRemoteSetup && len(msg.String()) == 1 {
			m.keyInput += msg.String()
		}
	}

	return m, nil
}

func (m WizardModel) handleEnter() (tea.Model, tea.Cmd) {
	switch m.step {
	case StepWelcome:
		m.step = StepSystemCheck
		m.working = true
		return m, m.checkSystem()

	case StepProviderChoice:
		m.providerChoice = m.cursor
		if m.providerChoice == 0 {
			// Local LLM
			m.step = StepLocalSetup
			m.working = true
			return m, m.checkOllama()
		} else {
			// Remote LLM
			m.step = StepRemoteSetup
		}

	case StepRemoteSetup:
		m.remoteProvider = m.cursor
		if m.keyInput != "" {
			m.apiKey = m.keyInput
			m.addMessage("✓ API key configured")
			m.step = StepMCPConfig
		}

	case StepMCPConfig:
		m.mcpTarget = m.cursor
		if m.mcpTarget < 2 {
			m.working = true
			return m, m.installMCP()
		} else {
			// Skip MCP
			m.step = StepValidation
			m.working = true
			return m, m.validate()
		}

	case StepComplete, StepError:
		m.quitting = true
	}

	return m, nil
}

func (m WizardModel) getMaxCursor() int {
	switch m.step {
	case StepProviderChoice:
		return 1 // local, remote
	case StepRemoteSetup:
		return 1 // anthropic, openai
	case StepMCPConfig:
		return 2 // opencode, claude, skip
	default:
		return 0
	}
}

func (m *WizardModel) addMessage(msg string) {
	m.message = append(m.message, msg)
}

func (m WizardModel) View() string {
	// Standalone view (for compatibility)
	return m.ViewContent()
}

func (m WizardModel) ViewContent() string {
	switch m.step {
	case StepWelcome:
		return m.viewWelcome()
	case StepSystemCheck:
		return m.viewSystemCheck()
	case StepProviderChoice:
		return m.viewProviderChoice()
	case StepLocalSetup:
		return m.viewLocalSetup()
	case StepRemoteSetup:
		return m.viewRemoteSetup()
	case StepMCPConfig:
		return m.viewMCPConfig()
	case StepValidation:
		return m.viewValidation()
	case StepComplete:
		return m.viewComplete()
	case StepError:
		return m.viewError()
	}
	return ""
}

func (m WizardModel) FooterHelp() string {
	if m.working {
		return "Please wait..."
	}

	switch m.step {
	case StepWelcome:
		return "Enter start setup • esc quit"
	case StepProviderChoice, StepRemoteSetup, StepMCPConfig:
		return "↑↓ navigate • Enter select • esc quit"
	case StepComplete, StepError:
		return "Enter exit"
	default:
		return "Please wait..."
	}
}

// View helpers
var (
	textStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	cursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func (m WizardModel) viewWelcome() string {
	var b strings.Builder
	b.WriteString(textStyle.Render("Welcome to Vela Setup"))
	b.WriteString("\n\n")
	b.WriteString(textStyle.Render("This wizard will guide you through:"))
	b.WriteString("\n")
	b.WriteString(textStyle.Render("  1. System requirements check"))
	b.WriteString("\n")
	b.WriteString(textStyle.Render("  2. LLM provider configuration (local or remote)"))
	b.WriteString("\n")
	b.WriteString(textStyle.Render("  3. MCP server setup"))
	b.WriteString("\n")
	b.WriteString(textStyle.Render("  4. Installation validation"))
	b.WriteString("\n\n")
	return b.String()
}

func (m WizardModel) viewSystemCheck() string {
	var b strings.Builder

	if m.working {
		b.WriteString(textStyle.Render("Checking system requirements..."))
		b.WriteString("\n")
	} else if m.sysOK {
		b.WriteString(successStyle.Render("✓ System check passed"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(fmt.Sprintf("  OS: %s, Arch: %s", m.sysOS, m.sysArch)))
		b.WriteString("\n")
	} else {
		b.WriteString(errorStyle.Render("✗ System check failed"))
		b.WriteString("\n")
		for _, issue := range m.sysIssues {
			b.WriteString(errorStyle.Render(fmt.Sprintf("  • %s", issue)))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	return b.String()
}

func (m WizardModel) viewProviderChoice() string {
	var b strings.Builder
	b.WriteString(textStyle.Render("Choose LLM provider:"))
	b.WriteString("\n\n")

	options := []string{
		"Local (Ollama - privacy-first, runs on your machine)",
		"Remote (Anthropic/OpenAI - cloud-based, requires API key)",
	}

	for i, opt := range options {
		cursor := "  "
		style := textStyle
		if i == m.cursor {
			cursor = cursorStyle.Render("▸ ")
			style = cursorStyle
		}
		b.WriteString(cursor + style.Render(opt) + "\n")
	}

	b.WriteString("\n")
	return b.String()
}

func (m WizardModel) viewLocalSetup() string {
	var b strings.Builder
	b.WriteString(textStyle.Render("Configuring local LLM (Ollama)..."))
	b.WriteString("\n\n")

	for _, msg := range m.message {
		b.WriteString(dimStyle.Render(msg))
		b.WriteString("\n")
	}

	if m.working {
		b.WriteString(textStyle.Render("⏳ Working..."))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	return b.String()
}

func (m WizardModel) viewRemoteSetup() string {
	var b strings.Builder
	b.WriteString(textStyle.Render("Configure remote LLM provider:"))
	b.WriteString("\n\n")

	providers := []string{"Anthropic Claude", "OpenAI GPT"}
	for i, p := range providers {
		cursor := "  "
		style := textStyle
		if i == m.cursor {
			cursor = cursorStyle.Render("▸ ")
			style = cursorStyle
		}
		b.WriteString(cursor + style.Render(p) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(textStyle.Render("API Key: "))
	if m.keyInput == "" {
		b.WriteString(dimStyle.Render("(enter key)"))
	} else {
		b.WriteString(textStyle.Render(strings.Repeat("*", len(m.keyInput))))
	}
	b.WriteString("\n\n")

	return b.String()
}

func (m WizardModel) viewMCPConfig() string {
	var b strings.Builder
	b.WriteString(textStyle.Render("Configure MCP server for:"))
	b.WriteString("\n\n")

	options := []string{
		"OpenCode (Anthropic CLI)",
		"Claude Desktop",
		"Skip MCP setup",
	}

	for i, opt := range options {
		cursor := "  "
		style := textStyle
		if i == m.cursor {
			cursor = cursorStyle.Render("▸ ")
			style = cursorStyle
		}
		b.WriteString(cursor + style.Render(opt) + "\n")
	}

	b.WriteString("\n")
	return b.String()
}

func (m WizardModel) viewValidation() string {
	var b strings.Builder
	b.WriteString(textStyle.Render("Validating installation..."))
	b.WriteString("\n\n")

	for _, msg := range m.message {
		b.WriteString(dimStyle.Render(msg))
		b.WriteString("\n")
	}

	if m.working {
		b.WriteString(textStyle.Render("⏳ Testing connectivity..."))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	return b.String()
}

func (m WizardModel) viewComplete() string {
	var b strings.Builder
	b.WriteString(successStyle.Render("✓ Setup Complete!"))
	b.WriteString("\n\n")
	b.WriteString(textStyle.Render("Installation summary:"))
	b.WriteString("\n\n")

	for _, msg := range m.message {
		b.WriteString(successStyle.Render(msg))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(textStyle.Render("You can now use Vela!"))
	b.WriteString("\n\n")
	return b.String()
}

func (m WizardModel) viewError() string {
	var b strings.Builder
	b.WriteString(errorStyle.Render("✗ Setup Failed"))
	b.WriteString("\n\n")
	b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
	b.WriteString("\n\n")
	return b.String()
}

// Commands
func (m WizardModel) checkSystem() tea.Cmd {
	return func() tea.Msg {
		os := runtime.GOOS
		arch := runtime.GOARCH
		issues := []string{}

		// Check supported platforms
		if os != "darwin" && os != "linux" && os != "windows" {
			issues = append(issues, fmt.Sprintf("unsupported OS: %s", os))
		}

		return systemCheckMsg{
			ok:     len(issues) == 0,
			os:     os,
			arch:   arch,
			issues: issues,
		}
	}
}

func (m WizardModel) checkOllama() tea.Cmd {
	return func() tea.Msg {
		installed, running, path, _ := CheckOllama()
		return ollamaCheckMsg{
			installed: installed,
			running:   running,
			path:      path,
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

		switch m.mcpTarget {
		case 0:
			configPath = getOpenCodeConfigPath()
		case 1:
			configPath = getClaudeDesktopConfigPath()
		case 2:
			return mcpInstallMsg{success: true, err: nil}
		}

		if configPath == "" {
			return mcpInstallMsg{success: false, err: fmt.Errorf("could not determine config path")}
		}

		configDir := filepath.Dir(configPath)
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return mcpInstallMsg{success: false, err: err}
		}

		var cfg map[string]interface{}
		data, err := os.ReadFile(configPath)
		if err != nil {
			cfg = map[string]interface{}{"mcpServers": map[string]interface{}{}}
		} else {
			if err := json.Unmarshal(data, &cfg); err != nil {
				return mcpInstallMsg{success: false, err: err}
			}
		}

		mcpServers, ok := cfg["mcpServers"].(map[string]interface{})
		if !ok {
			mcpServers = map[string]interface{}{}
			cfg["mcpServers"] = mcpServers
		}

		mcpServers["vela"] = map[string]interface{}{
			"command": "vela",
			"args":    []string{"serve"},
		}

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

func (m WizardModel) validate() tea.Cmd {
	return func() tea.Msg {
		// TODO: Actually validate LLM connectivity
		llmOK := m.ollamaRunning || m.apiKey != ""
		mcpOK := m.mcpConfigured || m.mcpTarget == 2

		return validationMsg{
			llmOK: llmOK,
			mcpOK: mcpOK,
			err:   nil,
		}
	}
}
