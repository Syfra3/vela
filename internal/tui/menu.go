package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Syfra3/vela/internal/ancora"
	"github.com/Syfra3/vela/internal/cache"
	"github.com/Syfra3/vela/internal/config"
	"github.com/Syfra3/vela/internal/detect"
	"github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/internal/extract"
	"github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/internal/llm"
	"github.com/Syfra3/vela/internal/setup"
	"github.com/Syfra3/vela/pkg/types"
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
	screenObsidian
	screenGraphStatus
	screenWatch
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
	version    string // version string to display

	// Global integration status shown in every screen header
	ancoraOK bool // ancora mcp socket alive
	daemonOK bool // vela watch daemon running

	// Nested screens can inject themselves here
	extractModel     ExtractModel
	queryModel       QueryModel
	doctorModel      DoctorModel
	configModel      ConfigViewModel
	installWizard    setup.WizardModel
	obsidianResult   string
	obsidianErr      error
	graphStatusModel GraphStatusModel
	watchModel       WatchModel
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

// NewMenuModelWithVersion creates the main menu TUI with version info.
func NewMenuModelWithVersion(ver string) MenuModel {
	m := NewMenuModel()
	m.version = ver
	return m
}

func (m *MenuModel) rebuildMenu() {
	m.items = []menuItem{}

	if !m.installed {
		m.items = append(m.items, menuItem{
			label:       "Setup Environment",
			description: "Install Ollama, configure LLM, and setup MCP server",
			key:         "install",
		})
	}

	m.items = append(m.items, []menuItem{
		{
			label:       "Graph Status",
			description: "View metrics: nodes, edges, connectivity, vault health",
			key:         "graphstatus",
		},
		{
			label:       "Extract",
			description: "Extract knowledge graph from directory",
			key:         "extract",
		},
		{
			label:       "Export to Obsidian",
			description: "Export graph.json to Obsidian vault",
			key:         "obsidian",
		},
		{
			label:       "Query",
			description: "Query existing graph (path, explain, nodes)",
			key:         "query",
		},
		{
			label:       "Watch",
			description: "Manage real-time Ancora integration daemon",
			key:         "watch",
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
	// Handle global extraction messages
	switch msg := msg.(type) {
	case extractionStartMsg:
		return m, runExtraction(msg.dir, msg.mode, msg.source)

	case extractionProgressMsg:
		if m.screen == screenExtract {
			m.extractModel.processedFiles = msg.current
			m.extractModel.totalFiles = msg.total
			m.extractModel.currentFile = msg.file
		}
		// Keep ticking for more progress updates
		return m, tickExtractionProgress()

	case extractionCompleteMsg:
		if m.screen == screenExtract {
			m.extractModel.extracting = false
			if msg.success {
				m.message = fmt.Sprintf("✓ Extraction complete: %s", msg.dir)
			} else {
				m.message = fmt.Sprintf("✗ Extraction failed: %v", msg.err)
			}
			m.screen = screenMain
		}
		return m, nil
	}

	// Route to screen-specific updates
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
	case screenObsidian:
		return m.updateObsidian(msg)
	case screenGraphStatus:
		return m.updateGraphStatus(msg)
	case screenWatch:
		return m.updateWatch(msg)
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
	case screenObsidian:
		return m.viewObsidian()
	case screenGraphStatus:
		return m.viewGraphStatus()
	case screenWatch:
		return m.viewWatch()
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
	case "graphstatus":
		m.screen = screenGraphStatus
		m.graphStatusModel = NewGraphStatusModel()
		return m, m.graphStatusModel.Init()
	case "watch":
		m.screen = screenWatch
		m.watchModel = NewWatchModel()
		return m, m.watchModel.Init()
	case "install":
		m.screen = screenInstall
		m.installWizard = setup.NewWizard()
		return m, m.installWizard.Init()
	case "extract":
		m.screen = screenExtract
		m.extractModel = NewExtractModel()
	case "obsidian":
		m.screen = screenObsidian
		return m, exportToObsidianCmd()
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
	b.WriteString(m.renderTagline())
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
	dot := func(ok bool) string {
		if ok {
			return lipgloss.NewStyle().Foreground(colorSuccess).Render("●")
		}
		return lipgloss.NewStyle().Foreground(colorErr).Render("○")
	}
	dim := lipgloss.NewStyle().Foreground(colorSubtext)

	// Vela: always ready
	velaStr := dim.Render("ready")

	// Vela MCP: installed in agent config
	mcpStr := dim.Render("offline")
	if m.installed {
		mcpStr = lipgloss.NewStyle().Foreground(colorSuccess).Render("operational")
	}

	// Ancora MCP: socket probe
	ancoraStr := dim.Render("offline")
	if m.ancoraOK {
		ancoraStr = lipgloss.NewStyle().Foreground(colorSuccess).Render("online")
	}

	// Vela daemon
	daemonStr := dim.Render("stopped")
	if m.daemonOK {
		daemonStr = lipgloss.NewStyle().Foreground(colorSuccess).Render("running")
	}

	return fmt.Sprintf("%s %s  |  MCP %s  |  Ancora %s  |  Daemon %s",
		dot(true), velaStr,
		mcpStr,
		ancoraStr,
		daemonStr,
	)
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

func (m MenuModel) renderTagline() string {
	brandStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	taglineStyle := lipgloss.NewStyle().Foreground(colorSubtext)

	versionStr := ""
	if m.version != "" {
		versionStr = " " + m.version
	}

	return brandStyle.Render("Vela"+versionStr) +
		taglineStyle.Render(" — Privacy-first knowledge graph builder for codebases")
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
		dir := m.extractModel.Directory()
		mode := m.extractModel.Mode()
		src := m.extractModel.Source()
		// Reset starting flag after launching
		m.extractModel.starting = false
		m.extractModel.extracting = true
		m.extractModel.step = stepExtracting
		// Launch extraction in background
		return m, launchExtraction(dir, mode, src)
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

func (m MenuModel) updateGraphStatus(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.graphStatusModel.Update(msg)
	m.graphStatusModel = updated.(GraphStatusModel)

	if m.graphStatusModel.Quitting() {
		m.screen = screenMain
		return m, nil
	}

	return m, cmd
}

func (m MenuModel) updateWatch(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.watchModel.Update(msg)
	m.watchModel = updated.(WatchModel)

	// Bubble integration status up to the global header.
	m.ancoraOK = m.watchModel.ancoraOK
	m.daemonOK = m.watchModel.daemonOK

	if m.watchModel.Quitting() {
		m.screen = screenMain
		return m, nil
	}

	return m, cmd
}

// ---------------------------------------------------------------------------
// View Screens
// ---------------------------------------------------------------------------

func (m MenuModel) viewGraphStatus() string {
	var b strings.Builder
	b.WriteString(m.renderHeader("Graph Status"))
	b.WriteString(m.graphStatusModel.ViewContent())
	b.WriteString(m.renderFooter(m.graphStatusModel.FooterHelp()))
	return appStyle.Render(b.String())
}

func (m MenuModel) viewWatch() string {
	var b strings.Builder
	b.WriteString(m.renderHeader("Watch — Real-Time Daemon"))
	b.WriteString(m.watchModel.ViewContent())
	b.WriteString(m.renderFooter(m.watchModel.FooterHelp()))
	return appStyle.Render(b.String())
}

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
	b.WriteString(m.renderFooter(m.extractModel.FooterHelp()))
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
// Extraction Launcher
// ---------------------------------------------------------------------------

type extractionStartMsg struct {
	dir    string
	mode   ExtractionMode
	source ExtractSource
}

type extractionProgressMsg struct {
	current int
	total   int
	file    string
}

type extractionCompleteMsg struct {
	success bool
	dir     string
	err     error
}

func launchExtraction(dir string, mode ExtractionMode, src ExtractSource) tea.Cmd {
	return func() tea.Msg {
		return extractionStartMsg{dir: dir, mode: mode, source: src}
	}
}

func runExtraction(dir string, mode ExtractionMode, src ExtractSource) tea.Cmd {
	return tea.Batch(
		startExtractionWorker(dir, mode, src),
		tickExtractionProgress(),
	)
}

type extractionState struct {
	files          []string
	processedCount int
	currentFile    string
	done           bool
	err            error
}

var globalExtractionState *extractionState

func startExtractionWorker(dir string, mode ExtractionMode, src ExtractSource) tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.Load()
		if err != nil {
			return extractionCompleteMsg{success: false, dir: dir, err: fmt.Errorf("loading config: %w", err)}
		}

		// Build LLM provider (nil = structural-only mode)
		var provider types.LLMProvider
		if cfg.LLM.Provider != "" && cfg.LLM.Provider != "none" {
			if llmClient, lErr := llm.NewClient(&cfg.LLM); lErr == nil {
				provider = llmClient
			}
		}

		if src == SourceAncora {
			// ── Ancora memory extraction ──────────────────────────────────
			dbPath, dbErr := ancora.DefaultDBPath()
			if dbErr != nil {
				return extractionCompleteMsg{success: false, dir: "ancora", err: dbErr}
			}

			// Count observations first so we can set up progress state.
			r, rErr := ancora.Open(dbPath)
			if rErr != nil {
				return extractionCompleteMsg{success: false, dir: "ancora", err: rErr}
			}
			total, _ := r.Count()
			r.Close()

			globalExtractionState = &extractionState{
				files:          make([]string, total), // placeholder for count
				processedCount: 0,
				done:           false,
			}

			go func() {
				outDir := config.OutDir(".")

				progress := func(done, _ int, title string) {
					globalExtractionState.processedCount = done
					globalExtractionState.currentFile = title
				}

				nodes, edges, extractErr := extract.ExtractAncora(
					dbPath, provider, cfg.LLM.MaxChunkTokens, progress,
				)
				if extractErr != nil {
					globalExtractionState.err = extractErr
					globalExtractionState.done = true
					return
				}

				g, buildErr := graph.Build(nodes, edges)
				if buildErr != nil {
					globalExtractionState.err = fmt.Errorf("building graph: %w", buildErr)
					globalExtractionState.done = true
					return
				}

				tg := g.ToTypes()
				if writeErr := export.WriteJSON(tg, outDir); writeErr != nil {
					globalExtractionState.err = fmt.Errorf("writing graph.json: %w", writeErr)
					globalExtractionState.done = true
					return
				}

				// Auto-sync Obsidian if enabled in config.
				if cfg.Obsidian.AutoSync {
					vaultDir := cfg.Obsidian.VaultDir
					if vaultDir == "" {
						vaultDir = config.DefaultVaultDir()
					}
					if syncErr := export.WriteObsidian(tg, vaultDir); syncErr != nil {
						globalExtractionState.err = fmt.Errorf("obsidian auto-sync: %w", syncErr)
					}
				}

				globalExtractionState.done = true
			}()

			return nil
		}

		// ── Path (filesystem) extraction ──────────────────────────────────
		fileCache, _ := cache.Load(cfg.Extraction.CacheDir)

		detected, collectErr := detect.Files(dir)
		if collectErr != nil {
			return extractionCompleteMsg{success: false, dir: dir, err: fmt.Errorf("detecting files: %w", collectErr)}
		}
		extSet := map[string]bool{
			".go": true, ".py": true, ".ts": true, ".tsx": true,
			".js": true, ".jsx": true, ".md": true, ".txt": true, ".pdf": true,
		}
		var files []string
		for _, e := range detected.Files {
			if extSet[filepath.Ext(e.AbsPath)] {
				files = append(files, e.AbsPath)
			}
		}
		if len(files) == 0 {
			return extractionCompleteMsg{success: false, dir: dir, err: fmt.Errorf("no files found in %s", dir)}
		}

		globalExtractionState = &extractionState{
			files:          files,
			processedCount: 0,
			done:           false,
		}

		go func() {
			outDir := config.OutDir(".")

			// Detect project once — all files in this run share the same source.
			projectSrc := extract.DetectProject(dir)

			var seededNodes []types.Node
			var seededEdges []types.Edge
			if mode == ModeRegenerate {
				fileCache = nil
			} else if existing, readErr := loadExistingGraph(config.GraphFilePath(outDir)); readErr == nil {
				seededNodes = existing.Nodes
				seededEdges = existing.Edges
			} else if fileCache != nil {
				fileCache = nil
			}

			var freshNodes []types.Node
			var freshEdges []types.Edge
			reextractedFiles := make(map[string]bool)
			projectEmitted := false

			for i, f := range files {
				rel := extract.RelPath(dir, f)
				globalExtractionState.currentFile = rel

				if fileCache != nil {
					sha, shaErr := cache.SHA256File(f)
					if shaErr == nil && fileCache.IsCached(f, sha) {
						globalExtractionState.processedCount++
						continue
					}
				}

				reextractedFiles[rel] = true

				nodes, edges, extractErr := extract.ExtractAll(dir, []string{f}, provider, projectSrc, cfg.LLM.MaxChunkTokens)
				if extractErr == nil {
					// ExtractAll always prepends the project root node.
					// Only keep it from the first successful file extraction.
					if !projectEmitted {
						freshNodes = append(freshNodes, nodes...)
						projectEmitted = true
					} else if len(nodes) > 0 {
						freshNodes = append(freshNodes, nodes[1:]...) // skip duplicate project node
					}
					freshEdges = append(freshEdges, edges...)
					if fileCache != nil {
						if sha, shaErr := cache.SHA256File(f); shaErr == nil {
							fileCache.Mark(f, sha)
						}
					}
				}

				globalExtractionState.processedCount = i + 1
			}

			if len(reextractedFiles) > 0 {
				seededNodes = filterNodesByFile(seededNodes, reextractedFiles)
				seededEdges = filterEdgesByFile(seededEdges, reextractedFiles)
			}
			allNodes := append(seededNodes, freshNodes...)
			allEdges := append(seededEdges, freshEdges...)

			if fileCache != nil {
				_ = fileCache.Save()
			}

			g, buildErr := graph.Build(allNodes, allEdges)
			if buildErr == nil {
				tg := g.ToTypes()
				if writeErr := export.WriteJSON(tg, outDir); writeErr != nil {
					globalExtractionState.err = fmt.Errorf("writing graph.json: %w", writeErr)
				} else if cfg.Obsidian.AutoSync {
					// Auto-sync Obsidian if enabled in config.
					vaultDir := cfg.Obsidian.VaultDir
					if vaultDir == "" {
						vaultDir = outDir
					}
					if syncErr := export.WriteObsidian(tg, vaultDir); syncErr != nil {
						globalExtractionState.err = fmt.Errorf("obsidian auto-sync: %w", syncErr)
					}
				}
			} else {
				globalExtractionState.err = fmt.Errorf("building graph: %w", buildErr)
			}

			globalExtractionState.done = true
		}()

		return nil
	}
}

func tickExtractionProgress() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		if globalExtractionState == nil {
			return extractionProgressMsg{current: 0, total: 0, file: "Initializing..."}
		}

		if globalExtractionState.done {
			if globalExtractionState.err != nil {
				return extractionCompleteMsg{success: false, dir: config.OutDir("."), err: globalExtractionState.err}
			}
			return extractionCompleteMsg{success: true, dir: config.OutDir("."), err: nil}
		}

		return extractionProgressMsg{
			current: globalExtractionState.processedCount,
			total:   len(globalExtractionState.files),
			file:    globalExtractionState.currentFile,
		}
	})
}

// ---------------------------------------------------------------------------
// Graph helpers for incremental extraction
// ---------------------------------------------------------------------------

// loadExistingGraph reads a previously written graph.json and returns its
// nodes and edges as types.Node / types.Edge slices. Used to seed the
// accumulator so that cached (unchanged) files still contribute their data.
//
// graph.json uses "file" (not "source_file") for the export format — see
// internal/export/json.go. We match that schema here explicitly.
func loadExistingGraph(path string) (*types.Graph, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Nodes []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
			Kind  string `json:"kind"`
			File  string `json:"file"`
		} `json:"nodes"`
		Edges []struct {
			From string `json:"from"`
			To   string `json:"to"`
			Kind string `json:"kind"`
			File string `json:"file"`
		} `json:"edges"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	g := &types.Graph{
		Nodes: make([]types.Node, len(raw.Nodes)),
		Edges: make([]types.Edge, len(raw.Edges)),
	}
	for i, n := range raw.Nodes {
		g.Nodes[i] = types.Node{ID: n.ID, Label: n.Label, NodeType: n.Kind, SourceFile: n.File}
	}
	for i, e := range raw.Edges {
		g.Edges[i] = types.Edge{Source: e.From, Target: e.To, Relation: e.Kind, SourceFile: e.File}
	}
	return g, nil
}

// filterNodesByFile removes nodes whose SourceFile is in reextracted, so that
// fresh results can be appended without duplicates.
func filterNodesByFile(nodes []types.Node, reextracted map[string]bool) []types.Node {
	out := nodes[:0]
	for _, n := range nodes {
		if !reextracted[n.SourceFile] {
			out = append(out, n)
		}
	}
	return out
}

// filterEdgesByFile removes edges whose SourceFile is in reextracted.
func filterEdgesByFile(edges []types.Edge, reextracted map[string]bool) []types.Edge {
	out := edges[:0]
	for _, e := range edges {
		if !reextracted[e.SourceFile] {
			out = append(out, e)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Obsidian Export
// ---------------------------------------------------------------------------

type obsidianExportMsg struct {
	success bool
	path    string
	err     error
}

func exportToObsidianCmd() tea.Cmd {
	return func() tea.Msg {
		outDir := config.OutDir(".")
		graphFile := config.GraphFilePath(outDir)

		// graph.json uses "from"/"to"/"kind" (export format from WriteJSON),
		// NOT the types.Graph JSON tags ("source"/"target"/"relation"/"type").
		// Unmarshal via the same raw struct used by loadExistingGraph.
		g, err := loadExistingGraph(graphFile)
		if err != nil {
			return obsidianExportMsg{success: false, err: fmt.Errorf("reading graph.json: %w", err)}
		}

		// Export to Obsidian
		if err := export.WriteObsidian(g, outDir); err != nil {
			return obsidianExportMsg{success: false, err: fmt.Errorf("writing obsidian vault: %w", err)}
		}

		// Get absolute path
		absPath, _ := filepath.Abs(filepath.Join(outDir, "obsidian"))
		return obsidianExportMsg{success: true, path: absPath}
	}
}

func (m MenuModel) updateObsidian(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.screen = screenMain
			return m, nil
		}
	case obsidianExportMsg:
		if msg.success {
			m.obsidianResult = fmt.Sprintf("✓ Exported to %s", msg.path)
			m.obsidianErr = nil
		} else {
			m.obsidianResult = ""
			m.obsidianErr = msg.err
		}
	}
	return m, nil
}

func (m MenuModel) viewObsidian() string {
	var b strings.Builder
	b.WriteString(m.renderHeader("Export to Obsidian"))

	textStyle := lipgloss.NewStyle().Foreground(colorText)
	successStyle := lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
	errorStyle := lipgloss.NewStyle().Foreground(colorErr)

	if m.obsidianErr != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.obsidianErr)))
		b.WriteString("\n\n")
		b.WriteString(textStyle.Render("Ensure you have run 'vela extract' first to generate graph.json"))
		b.WriteString("\n")
	} else if m.obsidianResult != "" {
		b.WriteString(successStyle.Render(m.obsidianResult))
		b.WriteString("\n\n")
		b.WriteString(textStyle.Render("Open the vault in Obsidian and use Graph View to visualize."))
		b.WriteString("\n")
	} else {
		b.WriteString(textStyle.Render("Exporting graph.json to Obsidian vault..."))
		b.WriteString("\n")
	}

	b.WriteString(m.renderFooter("esc back to menu"))
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
