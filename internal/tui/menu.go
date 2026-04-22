package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Syfra3/vela/internal/config"
	"github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/pkg/types"
)

type menuScreen int

const (
	screenMain menuScreen = iota
	screenExtract
	screenObsidian
	screenQuery
	screenGraphStatus
	screenProjects
	screenPurge
)

type menuItem struct {
	label       string
	description string
	key         string
}

type MenuModel struct {
	screen           menuScreen
	cursor           int
	items            []menuItem
	termWidth        int
	termHeight       int
	message          string
	version          string
	lastGraphPath    string
	obsidianRunning  bool
	obsidianStep     int
	obsidianTotal    int
	obsidianStatus   string
	obsidianStarted  time.Time
	obsidianResult   string
	obsidianErr      error
	extractModel     ExtractModel
	queryModel       QueryModel
	graphStatusModel GraphStatusModel
	projectsModel    ProjectsModel
	purgeModel       UninstallModel
}

func NewMenuModel() MenuModel {
	m := MenuModel{screen: screenMain, termWidth: 100, termHeight: 24}
	m.rebuildMenu()
	return m
}

func NewMenuModelWithVersion(ver string) MenuModel {
	m := NewMenuModel()
	m.version = ver
	return m
}

func (m *MenuModel) rebuildMenu() {
	m.items = []menuItem{
		{label: "Extract", description: "Browse folders and build a graph snapshot", key: "extract"},
		{label: "Graph Status", description: "Inspect the current graph snapshot", key: "graphstatus"},
		{label: "Export to Obsidian", description: "Export graph.json to an Obsidian vault", key: "obsidian"},
		{label: "Query", description: "Run dependency queries against graph.json", key: "query"},
		{label: "Projects", description: "Inspect, refresh, or delete tracked codebases", key: "projects"},
		{label: "Purge Data", description: "Delete Vela-managed graph, cache, and vault data", key: "purge"},
		{label: "Quit", description: "Exit Vela", key: "quit"},
	}
}

func (m MenuModel) Init() tea.Cmd { return nil }

func (m MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height
	}

	switch m.screen {
	case screenMain:
		return m.updateMain(msg)
	case screenExtract:
		return m.updateExtract(msg)
	case screenObsidian:
		return m.updateObsidian(msg)
	case screenQuery:
		return m.updateQuery(msg)
	case screenGraphStatus:
		return m.updateGraphStatus(msg)
	case screenProjects:
		return m.updateProjects(msg)
	case screenPurge:
		return m.updatePurge(msg)
	default:
		return m, nil
	}
}

func (m MenuModel) View() string {
	switch m.screen {
	case screenMain:
		return m.viewMain()
	case screenExtract:
		return m.viewExtract()
	case screenObsidian:
		return m.viewObsidian()
	case screenQuery:
		return m.viewQuery()
	case screenGraphStatus:
		return m.viewGraphStatus()
	case screenProjects:
		return m.viewProjects()
	case screenPurge:
		return m.viewPurge()
	default:
		return ""
	}
}

func (m MenuModel) updateMain(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
			return m.handleMenuSelect()
		}
	}
	return m, nil
}

func (m MenuModel) handleMenuSelect() (tea.Model, tea.Cmd) {
	switch m.items[m.cursor].key {
	case "extract":
		m.screen = screenExtract
		m.extractModel = NewExtractModel()
		return m, nil
	case "graphstatus":
		m.screen = screenGraphStatus
		m.graphStatusModel = NewGraphStatusModelWithGraphPath(m.lastGraphPath)
		return m, m.graphStatusModel.Init()
	case "obsidian":
		m.screen = screenObsidian
		m.obsidianRunning = true
		m.obsidianStep = 0
		m.obsidianTotal = 4
		m.obsidianStatus = "starting export"
		m.obsidianStarted = time.Now()
		m.obsidianResult = ""
		m.obsidianErr = nil
		progress := make(chan obsidianProgressMsg, 8)
		done := make(chan obsidianExportMsg, 1)
		startObsidianExport(m.lastGraphPath, progress, done)
		return m, tea.Batch(waitForObsidianProgress(progress), waitForObsidianDone(done))
	case "query":
		m.screen = screenQuery
		m.queryModel = NewQueryModel()
		return m, nil
	case "projects":
		m.screen = screenProjects
		m.projectsModel = NewProjectsModelWithGraphPath(m.lastGraphPath)
		return m, m.projectsModel.Init()
	case "purge":
		m.screen = screenPurge
		m.purgeModel = NewUninstallModel()
		return m, m.purgeModel.Init()
	default:
		return m, tea.Quit
	}
}

func (m MenuModel) updateExtract(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.extractModel.Update(msg)
	m.extractModel = updated.(ExtractModel)
	if strings.TrimSpace(m.extractModel.result.GraphPath) != "" {
		m.lastGraphPath = m.extractModel.result.GraphPath
	}
	if m.extractModel.Quitting() {
		m.screen = screenMain
		m.message = m.extractModel.StatusMessage()
		return m, nil
	}
	return m, cmd
}

func (m MenuModel) updateQuery(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.queryModel.Update(msg)
	m.queryModel = updated.(QueryModel)
	if m.queryModel.Quitting() {
		m.screen = screenMain
		return m, nil
	}
	return m, cmd
}

func (m MenuModel) updateGraphStatus(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.graphStatusModel.Update(msg)
	m.graphStatusModel = updated.(GraphStatusModel)
	if m.graphStatusModel.Quitting() {
		m.screen = screenMain
		return m, nil
	}
	return m, cmd
}

func (m MenuModel) updateProjects(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.projectsModel.Update(msg)
	m.projectsModel = updated.(ProjectsModel)
	if m.projectsModel.Quitting() {
		m.screen = screenMain
		return m, nil
	}
	return m, cmd
}

func (m MenuModel) updatePurge(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.purgeModel.Update(msg)
	m.purgeModel = updated.(UninstallModel)
	if m.purgeModel.Quitting() {
		m.screen = screenMain
		return m, nil
	}
	return m, cmd
}

func (m MenuModel) viewMain() string {
	var b strings.Builder
	b.WriteString(renderFrame("Vela", m.version, "Classic navigation restored"))
	b.WriteString("\n")
	for i, item := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = "▸ "
		}
		b.WriteString(fmt.Sprintf("%s%s — %s\n", cursor, item.label, item.description))
	}
	if strings.TrimSpace(m.message) != "" {
		b.WriteString("\n")
		b.WriteString(m.message)
		b.WriteString("\n")
	}
	b.WriteString("\n↑/↓ move • enter select • q quit\n")
	return appStyle.Render(b.String())
}

func (m MenuModel) viewExtract() string {
	return appStyle.Render(renderFrame("Extract", m.version, "Classic folder flow") + "\n" + m.extractModel.ViewContent() + "\n\nesc back • enter select/run\n")
}

func (m MenuModel) viewObsidian() string {
	var b strings.Builder
	b.WriteString(renderFrame("Export to Obsidian", m.version, "Graph visualization vault"))
	b.WriteString("\n")
	if m.obsidianErr != nil {
		b.WriteString(errorStyle.Render("Error: " + m.obsidianErr.Error()))
		b.WriteString("\n\nRun Extract first so Vela has a graph.json to export.\n")
	} else if m.obsidianRunning {
		progress := types.ExtractionProgress{
			TotalChunks:     m.obsidianTotal,
			ProcessedChunks: m.obsidianStep,
			CurrentFile:     m.obsidianStatus,
			StartTime:       m.obsidianStarted,
		}
		b.WriteString(RenderProgress(progress, 64))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("Current step: %s\n", m.obsidianStatus))
	} else if strings.TrimSpace(m.obsidianResult) != "" {
		b.WriteString(m.obsidianResult)
		b.WriteString("\n\nOpen the vault in Obsidian and use Graph View to visualize the generated graph.\n")
	} else {
		b.WriteString("Exporting graph.json to Obsidian vault...\n")
	}
	b.WriteString("\nesc back\n")
	return appStyle.Render(b.String())
}

func (m MenuModel) viewQuery() string {
	return appStyle.Render(renderFrame("Query", m.version, "Classic query console") + "\n" + m.queryModel.ViewContent() + "\n\nesc back • tab next field • enter run\n")
}

func (m MenuModel) viewGraphStatus() string {
	return appStyle.Render(renderFrame("Graph Status", m.version, "Read-only") + "\n" + m.graphStatusModel.ViewContent() + "\n\n" + m.graphStatusModel.FooterHelp() + "\n")
}

func (m MenuModel) viewProjects() string {
	return appStyle.Render(renderFrame("Projects", m.version, "Tracked codebases") + "\n" + m.projectsModel.ViewContent() + "\n\n" + m.projectsModel.FooterHelp() + "\n")
}

func (m MenuModel) viewPurge() string {
	return appStyle.Render(renderFrame("Purge Data", m.version, "Delete Vela-managed data") + "\n" + m.purgeModel.ViewContent() + "\n\n" + m.purgeModel.FooterHelp() + "\n")
}

func renderFrame(title, version, subtitle string) string {
	var b strings.Builder
	b.WriteString(renderAsciiLogo())
	b.WriteString("\n\n")
	b.WriteString(renderStatusLine())
	b.WriteString("\n")
	b.WriteString(renderSeparator())
	b.WriteString("\n\n")
	line := title
	if version != "" {
		line += " v" + version
	}
	if subtitle != "" {
		line += " — " + subtitle
	}
	b.WriteString(headerStyle.Render(line))
	return b.String()
}

type obsidianExportMsg struct {
	path string
	err  error
}

type obsidianProgressMsg struct {
	step    int
	total   int
	message string
}

func startObsidianExport(lastGraphPath string, progress chan obsidianProgressMsg, done chan obsidianExportMsg) {
	go func() {
		defer close(progress)
		defer close(done)
		emit := func(step int, message string) {
			progress <- obsidianProgressMsg{step: step, total: 4, message: message}
		}

		emit(1, "resolving graph path")
		graphPath := strings.TrimSpace(lastGraphPath)
		if graphPath == "" {
			var err error
			graphPath, err = config.FindGraphFile(".")
			if err != nil {
				done <- obsidianExportMsg{err: err}
				return
			}
		}
		emit(2, "loading graph.json")
		graph, err := loadGraph(graphPath)
		if err != nil {
			done <- obsidianExportMsg{err: err}
			return
		}
		emit(3, "loading Obsidian config")
		cfg, err := config.Load()
		if err != nil {
			done <- obsidianExportMsg{err: err}
			return
		}
		vaultDir := config.ResolveVaultDir(cfg.Obsidian.VaultDir)
		emit(4, "writing Obsidian vault")
		if err := export.WriteObsidian(graph, vaultDir); err != nil {
			done <- obsidianExportMsg{err: err}
			return
		}
		done <- obsidianExportMsg{path: filepath.Join(vaultDir, "obsidian")}
	}()
}

func (m MenuModel) updateObsidian(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			m.screen = screenMain
			return m, nil
		}
	case obsidianProgressMsg:
		m.obsidianRunning = true
		m.obsidianStep = msg.step
		m.obsidianTotal = msg.total
		m.obsidianStatus = msg.message
		return m, waitForObsidianProgress(m.obsidianProgressCh())
	case obsidianExportMsg:
		m.obsidianRunning = false
		m.obsidianErr = msg.err
		m.obsidianResult = ""
		if msg.err == nil {
			m.message = ""
			m.obsidianResult = "obsidian: " + msg.path
		} else if strings.TrimSpace(m.lastGraphPath) != "" {
			m.obsidianErr = fmt.Errorf("reading %s: %w", m.lastGraphPath, msg.err)
		}
	}
	return m, nil
}

var currentObsidianProgress <-chan obsidianProgressMsg
var currentObsidianDone <-chan obsidianExportMsg

func (m MenuModel) obsidianProgressCh() <-chan obsidianProgressMsg { return currentObsidianProgress }

func waitForObsidianProgress(ch <-chan obsidianProgressMsg) tea.Cmd {
	if ch != nil {
		currentObsidianProgress = ch
	}
	return func() tea.Msg {
		if currentObsidianProgress == nil {
			return nil
		}
		msg, ok := <-currentObsidianProgress
		if !ok {
			currentObsidianProgress = nil
			return nil
		}
		return msg
	}
}

func waitForObsidianDone(ch <-chan obsidianExportMsg) tea.Cmd {
	if ch != nil {
		currentObsidianDone = ch
	}
	return func() tea.Msg {
		if currentObsidianDone == nil {
			return nil
		}
		msg, ok := <-currentObsidianDone
		if !ok {
			currentObsidianDone = nil
			return nil
		}
		return msg
	}
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
	colors := []lipgloss.Color{colorAccent, colorAccent, colorAccentLight, colorSuccess, colorSuccess, colorSuccess}
	var b strings.Builder
	for i, line := range logoText {
		b.WriteString(lipgloss.NewStyle().Foreground(colors[i]).Bold(true).Render(line))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderTagline() string {
	brandStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	taglineStyle := lipgloss.NewStyle().Foreground(colorSubtext)
	return brandStyle.Render("Vela ") + taglineStyle.Render("— Privacy-first knowledge graph builder for codebases")
}

func renderStatusLine() string {
	readyDot := lipgloss.NewStyle().Foreground(colorSuccess).Render("●")
	readyText := lipgloss.NewStyle().Foreground(colorSubtext).Render("ready")
	queryDot := lipgloss.NewStyle().Foreground(colorAccent).Render("●")
	queryText := lipgloss.NewStyle().Foreground(colorSubtext).Render("query-only")
	return fmt.Sprintf("Status: %s %s  | Transport: %s %s", readyDot, readyText, queryDot, queryText)
}

func renderSeparator() string {
	return lipgloss.NewStyle().Foreground(colorMuted).Render(strings.Repeat("─", 60))
}

func IsTTY() bool {
	return isTTY()
}
