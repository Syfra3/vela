package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"

	"github.com/Syfra3/vela/internal/config"
	"github.com/Syfra3/vela/internal/daemon"
	"github.com/Syfra3/vela/internal/llm"
	"github.com/Syfra3/vela/pkg/types"
)

var wizardCheckLLMHealth = func(ctx context.Context, cfg *types.LLMConfig) error {
	client, err := llm.NewClient(cfg)
	if err != nil {
		return err
	}
	return client.Health(ctx)
}

var wizardCheckMCPInstalled = CheckMCPInstalled
var wizardCheckVelaMCPForTarget = CheckVelaMCPInstalledForTarget
var wizardCheckAncoraMCPForTarget = CheckAncoraMCPInstalledForTarget
var wizardCheckOllama = CheckOllama
var wizardGetOllamaModels = GetOllamaModels
var wizardLookPath = exec.LookPath
var wizardEnableObsidianAutoSync = enableObsidianAutoSync
var wizardEnsureDaemonRunning = ensureDaemonRunning
var wizardSaveIntegrationState = SaveIntegrationState
var wizardLoadIntegrationState = LoadIntegrationState

// Step-by-step linear wizard flow:
// 1. Check system requirements
// 2. Choose integration mode
// 3. Choose local vs remote LLM (for Vela-enabled modes)
// 4. Configure local (install Ollama + model) OR configure remote (API keys)
// 5. Configure MCP (Vela-only mode)
// 6. Validate installation
// 7. Finish

type WizardStep int

const (
	StepWelcome WizardStep = iota
	StepSystemCheck
	StepModeChoice
	StepProviderChoice
	StepLocalSetup  // Install Ollama + start + pull model
	StepRemoteSetup // Configure API keys
	StepMCPConfig
	StepValidation
	StepComplete
	StepError
)

// RequirementCheck represents a single system requirement validation
type RequirementCheck struct {
	Name        string
	Description string
	Status      string // "checking", "pass", "fail", "warning"
	Message     string
	Required    bool
}

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

	// Detailed requirement checks
	checkResults []RequirementCheck
	hardware     *HardwareInfo

	// Provider choice
	providerChoice int // 0=local, 1=remote
	setupMode      string

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
	mcpTarget     int // 0=Claude Code, 1=OpenCode, 2=Skip
	mcpConfigured bool

	// Validation results
	llmHealthy bool
	mcpHealthy bool

	// UI state
	cursor  int
	working bool
}

func NewWizard() WizardModel {
	m := WizardModel{
		step:           StepSystemCheck, // Start at system check, not welcome
		providerChoice: 0,
		setupMode:      IntegrationModeAncoraVela,
		selectedModel:  "llama3",
		working:        true, // Show "checking..." immediately
	}
	if state, err := wizardLoadIntegrationState(); err == nil && state != nil && validIntegrationMode(state.Mode) {
		m.setupMode = state.Mode
	}
	return m
}

func (m WizardModel) Init() tea.Cmd {
	// Auto-start system check on init
	return m.checkSystem()
}

func (m WizardModel) Quitting() bool {
	return m.quitting || m.step == StepComplete || m.step == StepError
}

// Messages
type systemCheckMsg struct {
	ok       bool
	os       string
	arch     string
	issues   []string
	checks   []RequirementCheck
	hardware *HardwareInfo
}

type startPullMsg struct{}

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
	llmOK    bool
	mcpOK    bool
	messages []string
	err      error
}

type setupFinalizeMsg struct {
	messages []string
	err      error
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
		m.checkResults = msg.checks
		m.hardware = msg.hardware
		m.working = false

		if !msg.ok {
			m.step = StepError
			m.err = fmt.Errorf("system requirements not met")
		}
		// Stay on StepSystemCheck — wait for user to press Enter

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
			// Give Ollama time to start before pulling model
			return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
				return startPullMsg{}
			})
		} else {
			m.step = StepError
			m.err = msg.err
		}

	case startPullMsg:
		m.working = true
		return m, m.pullModel()

	case modelPullMsg:
		m.working = false
		if msg.success {
			m.modelPulled = true
			if m.selectedModel == LocalSearchEmbeddingModel {
				m.addMessage(fmt.Sprintf("✓ Model %s ready", m.selectedModel))
			} else {
				m.addMessage(fmt.Sprintf("✓ Models %s and %s ready", m.selectedModel, LocalSearchEmbeddingModel))
			}
			if m.setupMode == IntegrationModeVelaOnly {
				m.step = StepMCPConfig
			} else {
				m.step = StepValidation
				m.working = true
				return m, m.validate()
			}
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
		for _, detail := range msg.messages {
			m.addMessage(detail)
		}

		if msg.err != nil {
			m.step = StepError
			m.err = msg.err
			return m, nil
		}

		if msg.llmOK {
			m.addMessage("✓ LLM provider healthy")
		}
		if msg.mcpOK {
			m.addMessage("✓ MCP server ready")
		}

		m.working = true
		return m, m.finalizeSetup()

	case setupFinalizeMsg:
		m.working = false
		for _, detail := range msg.messages {
			m.addMessage(detail)
		}
		if msg.err != nil {
			m.step = StepError
			m.err = msg.err
			return m, nil
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

	case StepSystemCheck:
		if m.sysOK && !m.working {
			m.step = StepModeChoice
		}

	case StepModeChoice:
		switch m.cursor {
		case 0:
			m.setupMode = IntegrationModeAncoraOnly
			m.step = StepValidation
			m.working = true
			return m, m.validate()
		case 1:
			m.setupMode = IntegrationModeAncoraVela
			m.step = StepProviderChoice
		case 2:
			m.setupMode = IntegrationModeVelaOnly
			m.step = StepProviderChoice
		}

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
			if m.setupMode == IntegrationModeVelaOnly {
				m.step = StepMCPConfig
			} else {
				m.step = StepValidation
				m.working = true
				return m, m.validate()
			}
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
	case StepModeChoice:
		return 2 // ancora-only, ancora+vela, vela-only
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
	case StepModeChoice:
		return m.viewModeChoice()
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
	case StepSystemCheck:
		if m.sysOK {
			return "Enter continue • esc quit"
		}
		return "esc quit"
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

	b.WriteString(textStyle.Render("System Requirements Check"))
	b.WriteString("\n\n")

	// Show hardware info prominently
	if m.hardware != nil {
		hardwareBox := lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("39")).
			Padding(0, 1).
			Width(60)

		hardwareText := fmt.Sprintf("Detected Hardware:\n\n"+
			"OS/Arch:  %s/%s\n"+
			"CPU:      %s (%d cores)\n"+
			"RAM:      %dGB total, %dGB free\n"+
			"Disk:     %dGB available\n\n"+
			"Recommendation: %s",
			m.hardware.OS, m.hardware.Arch,
			m.hardware.CPUModel, m.hardware.CPUCores,
			m.hardware.TotalRAMGB, m.hardware.FreeRAMGB,
			m.hardware.DiskFreeGB,
			m.hardware.RecommendedModel())

		b.WriteString(hardwareBox.Render(hardwareText))
		b.WriteString("\n\n")
	}

	if len(m.checkResults) == 0 && m.working {
		b.WriteString(dimStyle.Render("Analyzing system..."))
		b.WriteString("\n")
	}

	// Display all checks with status
	for _, check := range m.checkResults {
		var icon string
		var style lipgloss.Style

		switch check.Status {
		case "pass":
			icon = "✓"
			style = successStyle
		case "fail":
			icon = "✗"
			style = errorStyle
		case "warning":
			icon = "⚠"
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // orange
		case "checking":
			icon = "⏳"
			style = dimStyle
		default:
			icon = "○"
			style = dimStyle
		}

		b.WriteString(style.Render(fmt.Sprintf("%s %s", icon, check.Name)))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(fmt.Sprintf("  %s", check.Description)))
		b.WriteString("\n")

		if check.Message != "" {
			b.WriteString(dimStyle.Render(fmt.Sprintf("  → %s", check.Message)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Summary
	if !m.working && len(m.checkResults) > 0 {
		if m.sysOK {
			b.WriteString("\n")
			b.WriteString(successStyle.Render("All required checks passed!"))
			b.WriteString("\n")
		} else {
			b.WriteString("\n")
			b.WriteString(errorStyle.Render("Some required checks failed."))
			b.WriteString("\n")
			for _, issue := range m.sysIssues {
				b.WriteString(errorStyle.Render(fmt.Sprintf("  • %s", issue)))
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n")
	return b.String()
}

func (m WizardModel) viewModeChoice() string {
	var b strings.Builder
	b.WriteString(textStyle.Render("Choose integration mode:"))
	b.WriteString("\n\n")

	options := []string{
		"Ancora only (memory MCP stays in Ancora; disable Vela MCP ownership)",
		"Ancora + Vela (Ancora stays primary MCP; Vela powers graph retrieval)",
		"Vela only (register Vela directly as the MCP surface)",
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
	b.WriteString(textStyle.Render("Local LLM Setup (Ollama)"))
	b.WriteString("\n\n")

	// Show setup steps with status
	steps := []struct {
		name   string
		desc   string
		status string
	}{
		{
			name:   "Install Ollama",
			desc:   "Download and install Ollama runtime",
			status: m.getStepStatus(m.ollamaInstalled, m.working && !m.ollamaInstalled),
		},
		{
			name:   "Start Ollama Service",
			desc:   "Launch Ollama background service",
			status: m.getStepStatus(m.ollamaRunning, m.working && m.ollamaInstalled && !m.ollamaRunning),
		},
		{
			name:   fmt.Sprintf("Pull %s + %s", m.selectedModel, LocalSearchEmbeddingModel),
			desc:   "Download extraction and search models",
			status: m.getStepStatus(m.modelPulled, m.working && m.ollamaRunning && !m.modelPulled),
		},
	}

	for _, step := range steps {
		var icon string
		var style lipgloss.Style

		switch step.status {
		case "pass":
			icon = "✓"
			style = successStyle
		case "working":
			icon = "⏳"
			style = dimStyle
		case "pending":
			icon = "○"
			style = dimStyle
		}

		b.WriteString(style.Render(fmt.Sprintf("%s %s", icon, step.name)))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(fmt.Sprintf("  %s", step.desc)))
		b.WriteString("\n\n")
	}

	// Show progress messages
	if len(m.message) > 0 {
		b.WriteString("\n")
		b.WriteString(textStyle.Render("Progress:"))
		b.WriteString("\n")
		for _, msg := range m.message {
			b.WriteString(dimStyle.Render(fmt.Sprintf("  • %s", msg)))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	return b.String()
}

func (m WizardModel) getStepStatus(completed bool, working bool) string {
	if completed {
		return "pass"
	}
	if working {
		return "working"
	}
	return "pending"
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
	b.WriteString(textStyle.Render("Register Vela directly as MCP in:"))
	b.WriteString("\n\n")

	options := []string{
		"Claude Code",
		"OpenCode",
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
	b.WriteString(textStyle.Render(fmt.Sprintf("Mode: %s", integrationModeLabel(m.setupMode))))
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
		osName := runtime.GOOS
		arch := runtime.GOARCH
		issues := []string{}
		checks := []RequirementCheck{}

		// Detect hardware
		hardware, _ := DetectHardware()

		// Check 1: Operating System
		osCheck := RequirementCheck{
			Name:        "Operating System",
			Description: "Supported: Linux, macOS, Windows",
			Required:    true,
			Status:      "checking",
		}

		if osName == "darwin" {
			osCheck.Status = "pass"
			osCheck.Message = fmt.Sprintf("macOS (%s) supported", arch)
		} else if osName == "linux" {
			osCheck.Status = "pass"
			osCheck.Message = fmt.Sprintf("Linux (%s) supported", arch)
		} else if osName == "windows" {
			osCheck.Status = "pass"
			osCheck.Message = fmt.Sprintf("Windows (%s) supported", arch)
		} else {
			osCheck.Status = "fail"
			osCheck.Message = fmt.Sprintf("%s is not supported", osName)
			issues = append(issues, fmt.Sprintf("Unsupported OS: %s", osName))
		}
		checks = append(checks, osCheck)

		// Check 2: Architecture
		archCheck := RequirementCheck{
			Name:        "CPU Architecture",
			Description: "64-bit processor required",
			Required:    true,
			Status:      "checking",
		}

		if arch == "amd64" || arch == "arm64" {
			archCheck.Status = "pass"
			archCheck.Message = fmt.Sprintf("%s supported", arch)
		} else {
			archCheck.Status = "fail"
			archCheck.Message = fmt.Sprintf("%s not supported (need amd64 or arm64)", arch)
			issues = append(issues, fmt.Sprintf("Unsupported architecture: %s", arch))
		}
		checks = append(checks, archCheck)

		// Check 3: Memory (use detected hardware)
		memCheck := RequirementCheck{
			Name:        "System Memory",
			Description: "Recommended: 8GB+ for local LLM",
			Required:    false,
			Status:      "checking",
		}

		if hardware != nil && hardware.TotalRAMGB > 0 {
			if hardware.TotalRAMGB >= 8 {
				memCheck.Status = "pass"
				memCheck.Message = fmt.Sprintf("%dGB total, %dGB available", hardware.TotalRAMGB, hardware.FreeRAMGB)
			} else if hardware.TotalRAMGB >= 4 {
				memCheck.Status = "warning"
				memCheck.Message = fmt.Sprintf("%dGB total - consider remote LLM for better performance", hardware.TotalRAMGB)
			} else {
				memCheck.Status = "warning"
				memCheck.Message = fmt.Sprintf("%dGB total - remote LLM recommended", hardware.TotalRAMGB)
			}
		} else {
			memCheck.Status = "warning"
			memCheck.Message = "Cannot detect RAM - ensure 8GB+ for local LLM"
		}
		checks = append(checks, memCheck)

		// Check 4: Network connectivity
		netCheck := RequirementCheck{
			Name:        "Network Connectivity",
			Description: "Required for model downloads and remote LLM",
			Required:    false,
			Status:      "pass",
			Message:     "Network check skipped (will verify during setup)",
		}
		checks = append(checks, netCheck)

		// Check 5: Disk space (use detected hardware)
		diskCheck := RequirementCheck{
			Name:        "Disk Space",
			Description: "Recommended: 10GB+ for Ollama models",
			Required:    false,
			Status:      "checking",
		}

		if hardware != nil && hardware.DiskFreeGB > 0 {
			if hardware.DiskFreeGB >= 10 {
				diskCheck.Status = "pass"
				diskCheck.Message = fmt.Sprintf("%dGB available", hardware.DiskFreeGB)
			} else {
				diskCheck.Status = "warning"
				diskCheck.Message = fmt.Sprintf("Only %dGB available - may not fit larger models", hardware.DiskFreeGB)
			}
		} else {
			diskCheck.Status = "pass"
			diskCheck.Message = "Disk space check skipped (will verify during model pull)"
		}
		checks = append(checks, diskCheck)

		// Check 6: Package manager (for local LLM setup)
		pkgCheck := RequirementCheck{
			Name:        "Package Manager",
			Description: "brew (macOS) or curl (Linux) for Ollama install",
			Required:    false,
			Status:      "checking",
		}

		if osName == "darwin" {
			// Check for brew
			if _, err := exec.LookPath("brew"); err == nil {
				pkgCheck.Status = "pass"
				pkgCheck.Message = "Homebrew found"
			} else {
				pkgCheck.Status = "warning"
				pkgCheck.Message = "Homebrew not found - will use curl fallback"
			}
		} else if osName == "linux" {
			// Check for curl
			if _, err := exec.LookPath("curl"); err == nil {
				pkgCheck.Status = "pass"
				pkgCheck.Message = "curl found"
			} else {
				pkgCheck.Status = "warning"
				pkgCheck.Message = "curl not found - may need manual Ollama install"
			}
		} else if osName == "windows" {
			pkgCheck.Status = "warning"
			pkgCheck.Message = "Ollama must be installed manually on Windows"
		}
		checks = append(checks, pkgCheck)

		// Check 7: Existing Ollama installation
		ollamaCheck := RequirementCheck{
			Name:        "Ollama",
			Description: "Local LLM runtime (optional - will install if needed)",
			Required:    false,
			Status:      "checking",
		}

		installed, running, path, _ := CheckOllama()
		if installed {
			if running {
				ollamaCheck.Status = "pass"
				ollamaCheck.Message = fmt.Sprintf("Installed and running (%s)", path)
			} else {
				ollamaCheck.Status = "warning"
				ollamaCheck.Message = fmt.Sprintf("Installed but not running (%s)", path)
			}
		} else {
			ollamaCheck.Status = "pass"
			ollamaCheck.Message = "Not installed - will install during local setup"
		}
		checks = append(checks, ollamaCheck)

		return systemCheckMsg{
			ok:       len(issues) == 0,
			os:       osName,
			arch:     arch,
			issues:   issues,
			checks:   checks,
			hardware: hardware,
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
		if err := PullModelSilent(m.selectedModel); err != nil {
			return modelPullMsg{
				success: false,
				err:     err,
			}
		}
		if m.selectedModel != LocalSearchEmbeddingModel {
			if err := PullModelSilent(LocalSearchEmbeddingModel); err != nil {
				return modelPullMsg{
					success: false,
					err:     err,
				}
			}
		}
		return modelPullMsg{
			success: true,
			err:     nil,
		}
	}
}

func (m WizardModel) installMCP() tea.Cmd {
	return func() tea.Msg {
		var configPath string

		switch m.mcpTarget {
		case 0: // Claude Code
			configPath = getClaudeCodeConfigPath()
		case 1: // OpenCode
			configPath = getOpenCodeConfigPath()
		case 2: // Skip
			return mcpInstallMsg{success: true, err: nil}
		}

		if configPath == "" {
			return mcpInstallMsg{success: false, err: fmt.Errorf("could not determine config path")}
		}

		configDir := filepath.Dir(configPath)
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return mcpInstallMsg{success: false, err: err}
		}

		if m.mcpTarget == 0 {
			data, err := json.MarshalIndent(map[string]interface{}{
				"command": "vela",
				"args":    []string{"serve"},
			}, "", "  ")
			if err != nil {
				return mcpInstallMsg{success: false, err: err}
			}
			if err := os.WriteFile(configPath, data, 0644); err != nil {
				return mcpInstallMsg{success: false, err: err}
			}
			return mcpInstallMsg{success: true, err: nil}
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
		messages := make([]string, 0, 6)
		messages = append(messages, fmt.Sprintf("✓ Integration mode selected: %s", integrationModeLabel(m.setupMode)))

		velaPath, err := wizardLookPath("vela")
		if err != nil {
			return validationMsg{
				llmOK:    false,
				mcpOK:    false,
				messages: messages,
				err:      fmt.Errorf("vela binary is not on PATH; MCP clients will not be able to launch 'vela serve'"),
			}
		}
		_ = velaPath
		messages = append(messages, "✓ Vela binary is available on PATH")

		llmOK := true
		if m.setupMode != IntegrationModeAncoraOnly {
			llmCfg := &types.LLMConfig{Timeout: 10 * time.Second}
			if m.providerChoice == 0 {
				llmCfg.Provider = "local"
				llmCfg.Model = m.selectedModel
				llmCfg.Endpoint = "http://localhost:11434"

				installed, running, _, err := wizardCheckOllama()
				if err != nil {
					return validationMsg{llmOK: false, mcpOK: false, messages: messages, err: fmt.Errorf("checking Ollama: %w", err)}
				}
				if !installed {
					return validationMsg{llmOK: false, mcpOK: false, messages: messages, err: fmt.Errorf("Ollama is not installed")}
				}
				if !running {
					return validationMsg{llmOK: false, mcpOK: false, messages: messages, err: fmt.Errorf("Ollama is not running")}
				}

				models, err := wizardGetOllamaModels()
				if err != nil {
					return validationMsg{llmOK: false, mcpOK: false, messages: messages, err: fmt.Errorf("listing Ollama models: %w", err)}
				}
				if !wizardHasModel(models, m.selectedModel) {
					return validationMsg{llmOK: false, mcpOK: false, messages: messages, err: fmt.Errorf("Ollama model %q is not available", m.selectedModel)}
				}
				if !wizardHasModel(models, LocalSearchEmbeddingModel) {
					return validationMsg{llmOK: false, mcpOK: false, messages: messages, err: fmt.Errorf("Ollama embedding model %q is not available", LocalSearchEmbeddingModel)}
				}
				messages = append(messages, fmt.Sprintf("✓ Ollama is running with model %s", m.selectedModel))
				messages = append(messages, fmt.Sprintf("✓ Retrieval embedding model %s is available", LocalSearchEmbeddingModel))
			} else {
				llmCfg.Provider = "anthropic"
				if m.remoteProvider == 1 {
					llmCfg.Provider = "openai"
				}
				llmCfg.APIKey = m.apiKey
				if strings.TrimSpace(m.apiKey) == "" {
					return validationMsg{llmOK: false, mcpOK: false, messages: messages, err: fmt.Errorf("API key is required for remote provider setup")}
				}
				messages = append(messages, fmt.Sprintf("✓ %s API key configured", llmCfg.Provider))
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := wizardCheckLLMHealth(ctx, llmCfg); err != nil {
				return validationMsg{llmOK: false, mcpOK: false, messages: messages, err: fmt.Errorf("LLM health check failed: %w", err)}
			}
		} else {
			messages = append(messages, "✓ Vela runtime setup skipped for Ancora-only mode")
		}

		mcpOK := true
		switch m.setupMode {
		case IntegrationModeVelaOnly:
			target := m.selectedMCPTarget()
			messages = append(messages, fmt.Sprintf("✓ Primary MCP target: %s", targetLabel(target)))
			if target == IntegrationTargetSkip {
				messages = append(messages, "✓ MCP setup skipped by choice")
				break
			}
			if !wizardCheckVelaMCPForTarget(target) {
				return validationMsg{llmOK: llmOK, mcpOK: false, messages: messages, err: fmt.Errorf("Vela MCP registration not found for %s", targetLabel(target))}
			}
			messages = append(messages, "✓ Vela MCP registration verified")
		case IntegrationModeAncoraOnly, IntegrationModeAncoraVela:
			if _, err := wizardLookPath("ancora"); err != nil {
				return validationMsg{llmOK: llmOK, mcpOK: false, messages: messages, err: fmt.Errorf("ancora binary is not on PATH; Ancora remains the primary MCP surface in %s mode", integrationModeLabel(m.setupMode))}
			}
			messages = append(messages, "✓ Ancora binary is available on PATH")
			if m.setupMode == IntegrationModeAncoraVela {
				messages = append(messages, "✓ Ancora will expose forwarded Vela retrieval tools once its MCP server is started")
			} else {
				messages = append(messages, "✓ Ancora-only mode disables forwarded Vela retrieval tools")
			}
		}

		return validationMsg{
			llmOK:    llmOK,
			mcpOK:    mcpOK,
			messages: messages,
			err:      nil,
		}
	}
}

func (m WizardModel) finalizeSetup() tea.Cmd {
	return func() tea.Msg {
		messages := make([]string, 0, 2)

		if err := wizardSaveIntegrationState(IntegrationState{
			Mode:      m.setupMode,
			MCPTarget: m.selectedMCPTarget(),
			UpdatedBy: "vela",
		}); err != nil {
			return setupFinalizeMsg{messages: messages, err: fmt.Errorf("saving integration mode: %w", err)}
		}
		messages = append(messages, fmt.Sprintf("✓ Integration mode saved: %s", integrationModeLabel(m.setupMode)))

		if m.setupMode != IntegrationModeVelaOnly {
			if err := UninstallMCP(); err != nil {
				return setupFinalizeMsg{messages: messages, err: fmt.Errorf("removing stale Vela MCP registration: %w", err)}
			}
			messages = append(messages, "✓ Direct Vela MCP registration removed so primary ownership stays with Ancora")
		}

		if err := wizardEnableObsidianAutoSync(true); err != nil {
			return setupFinalizeMsg{messages: messages, err: fmt.Errorf("enabling Obsidian auto-sync: %w", err)}
		}
		messages = append(messages, "✓ Obsidian auto-sync enabled by default")

		started, err := wizardEnsureDaemonRunning()
		if err != nil {
			return setupFinalizeMsg{messages: messages, err: fmt.Errorf("starting watch daemon: %w", err)}
		}
		if started {
			messages = append(messages, "✓ Watch daemon started")
		} else {
			messages = append(messages, "✓ Watch daemon already running")
		}

		return setupFinalizeMsg{messages: messages}
	}
}

func enableObsidianAutoSync(enable bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cfgPath := filepath.Join(home, ".vela", "config.yaml")

	data, err := os.ReadFile(cfgPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading config: %w", err)
	}

	var raw map[string]interface{}
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("parsing config: %w", err)
		}
	}
	if raw == nil {
		raw = make(map[string]interface{})
	}

	obs, _ := raw["obsidian"].(map[string]interface{})
	if obs == nil {
		obs = map[string]interface{}{}
	}
	obs["auto_sync"] = enable
	if _, hasDir := obs["vault_dir"]; !hasDir {
		obs["vault_dir"] = config.DefaultVaultDir()
	}
	raw["obsidian"] = obs

	out, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(cfgPath, out, 0644)
}

func ensureDaemonRunning() (bool, error) {
	cfg, err := config.Load()
	if err != nil {
		return false, err
	}

	pf, err := daemon.NewPIDFile(cfg.Daemon.PIDFile)
	if err != nil {
		return false, err
	}

	alive, _, err := pf.IsAlive()
	if err != nil {
		return false, fmt.Errorf("checking daemon pid: %w", err)
	}
	if alive {
		return false, nil
	}

	self, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("resolving executable: %w", err)
	}

	child := exec.Command(self, "watch", "start", "--foreground")
	child.Stdout = nil
	child.Stderr = nil
	child.Stdin = nil
	if err := child.Start(); err != nil {
		return false, fmt.Errorf("starting daemon: %w", err)
	}

	if err := child.Process.Release(); err != nil {
		return false, fmt.Errorf("detaching daemon: %w", err)
	}
	return true, nil
}

func wizardHasModel(models []string, target string) bool {
	for _, model := range models {
		if model == target || strings.HasPrefix(model, target+":") {
			return true
		}
	}
	return false
}

func (m WizardModel) selectedMCPTarget() string {
	switch m.mcpTarget {
	case 0:
		return IntegrationTargetClaudeCode
	case 1:
		return IntegrationTargetOpenCode
	default:
		return IntegrationTargetSkip
	}
}
