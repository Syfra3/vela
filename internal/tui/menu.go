package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Syfra3/vela/internal/setup"
)

// ---------------------------------------------------------------------------
// Menu Screen - Main TUI entry point
// ---------------------------------------------------------------------------

type menuScreen int

const (
	screenMain menuScreen = iota
	screenInstall
	screenExtract
	screenQuery
	screenConfig
	screenDoctor
)

type menuItem struct {
	label       string
	description string
	key         string // action key
}

type MenuModel struct {
	screen     menuScreen
	cursor     int
	items      []menuItem
	termWidth  int
	termHeight int
	message    string // status/error message
	installed  bool   // MCP installation status

	// Nested screens can inject themselves here
	extractModel  ExtractModel
	queryModel    QueryModel
	doctorModel   DoctorModel
	configModel   ConfigViewModel
	installWizard setup.WizardModel
}

// NewMenuModel creates the main menu TUI.
func NewMenuModel() MenuModel {
	m := MenuModel{
		screen:     screenMain,
		cursor:     0,
		termWidth:  100,
		termHeight: 24,
		installed:  setup.CheckMCPInstalled(),
	}
	m.rebuildMenu()
	return m
}

func (m *MenuModel) rebuildMenu() {
	m.items = []menuItem{}

	if !m.installed {
		m.items = append(m.items, menuItem{
			label:       "Install MCP",
			description: "Download and configure Vela for OpenCode/Claude Desktop",
			key:         "install",
		})
	}

	m.items = append(m.items, []menuItem{
		{
			label:       "Extract",
			description: "Extract knowledge graph from directory",
			key:         "extract",
		},
		{
			label:       "Query",
			description: "Query existing graph (path, explain, nodes)",
			key:         "query",
		},
		{
			label:       "Doctor",
			description: "Check LLM provider health",
			key:         "doctor",
		},
		{
			label:       "Config",
			description: "Manage configuration (~/.vela/config.yaml)",
			key:         "config",
		},
		{
			label:       "Quit",
			description: "Exit Vela",
			key:         "quit",
		},
	}...)
}

// ---------------------------------------------------------------------------
// Init / Update / View
// ---------------------------------------------------------------------------

func (m MenuModel) Init() tea.Cmd {
	return nil
}

func (m MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenMain:
		return m.updateMain(msg)
	case screenInstall:
		return m.updateInstall(msg)
	case screenExtract:
		return m.updateExtract(msg)
	case screenQuery:
		return m.updateQuery(msg)
	case screenConfig:
		return m.updateConfig(msg)
	case screenDoctor:
		return m.updateDoctor(msg)
	}
	return m, nil
}

func (m MenuModel) View() string {
	switch m.screen {
	case screenMain:
		return m.viewMain()
	case screenInstall:
		return m.viewInstall()
	case screenExtract:
		return m.viewExtract()
	case screenQuery:
		return m.viewQuery()
	case screenConfig:
		return m.viewConfig()
	case screenDoctor:
		return m.viewDoctor()
	}
	return ""
}

// ---------------------------------------------------------------------------
// Main Menu - Update
// ---------------------------------------------------------------------------

func (m MenuModel) updateMain(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "enter", " ":
			if m.cursor < len(m.items) {
				return m.handleMenuSelect()
			}
		}
	}
	return m, nil
}

func (m MenuModel) handleMenuSelect() (tea.Model, tea.Cmd) {
	key := m.items[m.cursor].key
	switch key {
	case "install":
		m.screen = screenInstall
		m.installWizard = setup.NewWizard()
	case "extract":
		m.screen = screenExtract
		m.extractModel = NewExtractModel()
	case "query":
		m.screen = screenQuery
		m.queryModel = NewQueryModel()
	case "config":
		m.screen = screenConfig
		m.configModel = NewConfigViewModel()
	case "doctor":
		m.screen = screenDoctor
		m.doctorModel = NewDoctorModel()
	case "quit":
		return m, tea.Quit
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// Main Menu - View
// ---------------------------------------------------------------------------

func (m MenuModel) viewMain() string {
	var b strings.Builder

	// Header
	b.WriteString(m.renderHeader(""))

	// Menu items with two-column layout (label + description)
	menuLabelStyle := lipgloss.NewStyle().
		Foreground(colorText).
		Bold(false).
		Width(22)

	menuDescStyle := lipgloss.NewStyle().
		Foreground(colorSubtext).
		Width(70)

	for i, item := range m.items {
		cursor := "  "
		labelStyle := menuLabelStyle
		if i == m.cursor {
			cursor = lipgloss.NewStyle().Foreground(colorAccent).Render("▸ ")
			labelStyle = menuLabelStyle.Copy().Foreground(colorAccent)
		}

		label := labelStyle.Render(item.label)
		desc := menuDescStyle.Render(item.description)
		b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, label, desc))
	}

	// Footer
	footerHelp := "↑↓ navigate • Enter select • q exit"
	b.WriteString(m.renderFooter(footerHelp))

	return appStyle.Render(b.String())
}

// ---------------------------------------------------------------------------
// Header / Footer (like Ancora)
// ---------------------------------------------------------------------------

func (m MenuModel) renderHeader(sectionTitle string) string {
	var b strings.Builder

	// Logo
	b.WriteString(renderAsciiLogo())
	b.WriteString("\n")
	b.WriteString(renderTagline())
	b.WriteString("\n\n")

	// Status line
	b.WriteString(m.renderStatusLine())
	b.WriteString("\n")

	// Section title (if provided)
	if sectionTitle != "" {
		b.WriteString(headerStyle.Render(sectionTitle))
		b.WriteString("\n")
	}

	// Separator
	b.WriteString(renderSeparator())
	b.WriteString("\n\n")

	return b.String()
}

func (m MenuModel) renderStatusLine() string {
	statusDot := lipgloss.NewStyle().Foreground(colorSuccess).Render("●")
	statusText := lipgloss.NewStyle().Foreground(colorSubtext).Render("ready")

	mcpDot := lipgloss.NewStyle().Foreground(colorErr).Render("●")
	mcpText := lipgloss.NewStyle().Foreground(colorSubtext).Render("offline")
	if m.installed {
		mcpDot = lipgloss.NewStyle().Foreground(colorSuccess).Render("●")
		mcpText = lipgloss.NewStyle().Foreground(colorSuccess).Render("operational")
	}

	return fmt.Sprintf("Status: %s %s  | MCP Status: %s %s", statusDot, statusText, mcpDot, mcpText)
}

func (m MenuModel) renderFooter(helpText string) string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(renderSeparator())
	b.WriteString(helpStyle.Render("\n" + helpText))
	return b.String()
}

func renderAsciiLogo() string {
	logoText := []string{
		`██╗   ██╗███████╗██╗      █████╗ `,
		`██║   ██║██╔════╝██║     ██╔══██╗`,
		`██║   ██║█████╗  ██║     ███████║`,
		`╚██╗ ██╔╝██╔══╝  ██║     ██╔══██║`,
		` ╚████╔╝ ███████╗███████╗██║  ██║`,
		`  ╚═══╝  ╚══════╝╚══════╝╚═╝  ╚═╝`,
	}

	// Gradient colors (Vela palette: blue to cyan)
	colors := []lipgloss.Color{
		colorAccent, colorAccent, colorAccentLight,
		colorSuccess, colorSuccess, colorSuccess,
	}

	var b strings.Builder
	for i, line := range logoText {
		b.WriteString(lipgloss.NewStyle().Foreground(colors[i]).Bold(true).Render(line) + "\n")
	}

	return b.String()
}

func renderTagline() string {
	brandStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	taglineStyle := lipgloss.NewStyle().Foreground(colorSubtext)

	return brandStyle.Render("Vela ") +
		taglineStyle.Render("— Privacy-first knowledge graph builder for codebases")
}

func renderSeparator() string {
	return lipgloss.NewStyle().Foreground(colorMuted).Render(strings.Repeat("─", 60))
}

// ---------------------------------------------------------------------------
// Nested Screen Handlers (stubs for now)
// ---------------------------------------------------------------------------

func (m MenuModel) updateInstall(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Delegate to wizard
	updatedWizard, cmd := m.installWizard.Update(msg)
	m.installWizard = updatedWizard.(setup.WizardModel)

	// Check if wizard wants to quit
	if m.installWizard.Quitting() {
		m.screen = screenMain
		m.installed = setup.CheckMCPInstalled() // Refresh status
		m.rebuildMenu()
		return m, nil
	}

	return m, cmd
}

func (m MenuModel) updateExtract(msg tea.Msg) (tea.Model, tea.Cmd) {
	updatedExtract, cmd := m.extractModel.Update(msg)
	m.extractModel = updatedExtract.(ExtractModel)

	if m.extractModel.Quitting() {
		m.screen = screenMain
		return m, nil
	}

	if m.extractModel.Starting() {
		// TODO: launch actual extraction with m.extractModel.Directory()
		m.message = fmt.Sprintf("Extraction started for: %s", m.extractModel.Directory())
		m.screen = screenMain
		return m, nil
	}

	return m, cmd
}

func (m MenuModel) updateQuery(msg tea.Msg) (tea.Model, tea.Cmd) {
	updatedQuery, cmd := m.queryModel.Update(msg)
	m.queryModel = updatedQuery.(QueryModel)

	if m.queryModel.Quitting() {
		m.screen = screenMain
		return m, nil
	}

	return m, cmd
}

func (m MenuModel) updateConfig(msg tea.Msg) (tea.Model, tea.Cmd) {
	updatedConfig, cmd := m.configModel.Update(msg)
	m.configModel = updatedConfig.(ConfigViewModel)

	if m.configModel.Quitting() {
		m.screen = screenMain
		return m, nil
	}

	return m, cmd
}

func (m MenuModel) updateDoctor(msg tea.Msg) (tea.Model, tea.Cmd) {
	updatedDoctor, cmd := m.doctorModel.Update(msg)
	m.doctorModel = updatedDoctor.(DoctorModel)

	if m.doctorModel.Quitting() {
		m.screen = screenMain
		return m, nil
	}

	return m, cmd
}

// ---------------------------------------------------------------------------
// View Screens (stubs)
// ---------------------------------------------------------------------------

func (m MenuModel) viewInstall() string {
	var b strings.Builder
	b.WriteString(m.renderHeader("Setup Wizard"))
	b.WriteString(m.installWizard.ViewContent())
	b.WriteString(m.renderFooter(m.installWizard.FooterHelp()))
	return appStyle.Render(b.String())
}

func (m MenuModel) viewExtract() string {
	var b strings.Builder
	b.WriteString(m.renderHeader("Extract — Knowledge Graph"))
	b.WriteString(m.extractModel.ViewContent())
	b.WriteString(m.renderFooter("Enter start extraction • esc back to menu"))
	return appStyle.Render(b.String())
}

func (m MenuModel) viewQuery() string {
	var b strings.Builder
	b.WriteString(m.renderHeader(fmt.Sprintf("Query — %s", m.queryModel.ModeName())))
	b.WriteString(m.queryModel.ViewContent())
	b.WriteString(m.renderFooter("Tab change mode • Enter execute • esc back to menu"))
	return appStyle.Render(b.String())
}

func (m MenuModel) viewConfig() string {
	var b strings.Builder
	b.WriteString(m.renderHeader("Config — Vela Settings"))
	b.WriteString(m.configModel.ViewContent())
	b.WriteString(m.renderFooter("e edit in $EDITOR • esc back to menu"))
	return appStyle.Render(b.String())
}

func (m MenuModel) viewDoctor() string {
	var b strings.Builder
	b.WriteString(m.renderHeader("Doctor — LLM Health Check"))
	b.WriteString(m.doctorModel.ViewContent())
	footer := "esc back to menu"
	if !m.doctorModel.checking && len(m.doctorModel.results) > 0 {
		footer = "r re-check • esc back to menu"
	}
	b.WriteString(m.renderFooter(footer))
	return appStyle.Render(b.String())
}

// ---------------------------------------------------------------------------
// TTY Check
// ---------------------------------------------------------------------------

func IsTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
