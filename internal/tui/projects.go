package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Syfra3/vela/internal/cache"
	"github.com/Syfra3/vela/internal/config"
	"github.com/Syfra3/vela/internal/detect"
	"github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/internal/extract"
	"github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/internal/llm"
	"github.com/Syfra3/vela/pkg/types"
)

type trackedProject struct {
	Name   string
	NodeID string
	Path   string
	Remote string
}

type projectsLoadedMsg struct {
	graphPath string
	projects  []trackedProject
	err       error
}

type projectsActionMsg struct {
	message string
	err     error
}

var (
	loadTrackedProjectsFunc   = loadTrackedProjects
	deleteTrackedProjectsFunc = deleteTrackedProjects
	refreshTrackedProjectFunc = refreshTrackedProject
)

type ProjectsModel struct {
	cursor   int
	quitting bool
	loading  bool
	running  bool

	graphPath string
	projects  []trackedProject
	selected  map[string]bool

	message  string
	msgIsErr bool
	err      error
}

func NewProjectsModel() ProjectsModel {
	return ProjectsModel{
		loading:  true,
		selected: make(map[string]bool),
	}
}

func (m ProjectsModel) Init() tea.Cmd { return loadTrackedProjectsCmd() }

func (m ProjectsModel) Quitting() bool { return m.quitting }

func (m ProjectsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case projectsLoadedMsg:
		m.loading = false
		m.running = false
		m.graphPath = msg.graphPath
		m.projects = msg.projects
		m.err = msg.err
		if len(m.projects) == 0 {
			m.cursor = 0
		} else if m.cursor >= len(m.projects) {
			m.cursor = len(m.projects) - 1
		}
		m.pruneSelections()
		return m, nil

	case projectsActionMsg:
		m.running = false
		m.message = msg.message
		m.msgIsErr = msg.err != nil
		if msg.err != nil {
			m.message = msg.err.Error()
		}
		return m, loadTrackedProjectsCmd()

	case tea.KeyMsg:
		if m.loading || m.running {
			switch msg.String() {
			case "ctrl+c", "esc", "b":
				m.quitting = true
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "esc", "b":
			m.quitting = true
			return m, nil

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.projects)-1 {
				m.cursor++
			}

		case "x", " ":
			if project, ok := m.currentProject(); ok {
				if m.selected[project.NodeID] {
					delete(m.selected, project.NodeID)
				} else {
					m.selected[project.NodeID] = true
				}
			}

		case "d":
			marked := m.markedProjects()
			if len(marked) == 0 {
				m.message = "Mark one or more projects with x before deleting."
				m.msgIsErr = true
				return m, nil
			}
			m.running = true
			m.message = ""
			return m, deleteTrackedProjectsCmd(m.graphPath, marked)

		case "enter", "r":
			project, ok := m.currentProject()
			if !ok {
				return m, nil
			}
			m.running = true
			m.message = ""
			return m, refreshTrackedProjectCmd(m.graphPath, project)
		}
	}

	return m, nil
}

func (m ProjectsModel) View() string { return m.ViewContent() }

func (m ProjectsModel) ViewContent() string {
	var b strings.Builder

	textStyle := lipgloss.NewStyle().Foreground(colorText)
	mutedStyle := lipgloss.NewStyle().Foreground(colorSubtext)
	accentStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	warnStyle := lipgloss.NewStyle().Foreground(colorWarn).Bold(true)
	errorStyle := lipgloss.NewStyle().Foreground(colorErr)

	if m.loading {
		b.WriteString(mutedStyle.Render("Loading tracked projects..."))
		return b.String()
	}

	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		b.WriteString("\n\n")
		b.WriteString(mutedStyle.Render("Run Extract first to create graph.json, then return here."))
		return b.String()
	}

	if len(m.projects) == 0 {
		b.WriteString(mutedStyle.Render("No tracked codebases found in graph.json."))
		b.WriteString("\n\n")
		b.WriteString(textStyle.Render("Use Extract to add a codebase, then return here to refresh or remove it."))
		return b.String()
	}

	b.WriteString(accentStyle.Render("Tracked Codebases"))
	b.WriteString("\n\n")

	for i, project := range m.projects {
		cursor := "  "
		rowStyle := textStyle
		if i == m.cursor {
			cursor = lipgloss.NewStyle().Foreground(colorAccent).Render("▸ ")
			rowStyle = rowStyle.Copy().Foreground(colorAccent)
		}

		mark := "[ ]"
		if m.selected[project.NodeID] {
			mark = warnStyle.Render("[x]")
		}

		b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, mark, rowStyle.Render(project.Name)))
		if project.Path != "" {
			b.WriteString(mutedStyle.Render("     " + project.Path))
			b.WriteString("\n")
		} else {
			b.WriteString(mutedStyle.Render("     path unavailable in this graph snapshot"))
			b.WriteString("\n")
		}
		if project.Remote != "" {
			b.WriteString(mutedStyle.Render("     remote: " + project.Remote))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(mutedStyle.Render("Delete removes tracked graph/cache data only. It does not delete source directories."))
	if m.running {
		b.WriteString("\n\n")
		b.WriteString(textStyle.Render("Applying project update..."))
	} else if m.message != "" {
		b.WriteString("\n\n")
		if m.msgIsErr {
			b.WriteString(errorStyle.Render(m.message))
		} else {
			b.WriteString(textStyle.Render(m.message))
		}
	}

	return b.String()
}

func (m ProjectsModel) FooterHelp() string {
	if m.loading {
		return "loading tracked projects"
	}
	if m.running {
		return "waiting for project action to finish"
	}
	return "↑↓ navigate • x mark • Enter/r refresh selected • d delete marked • esc back"
}

func (m ProjectsModel) currentProject() (trackedProject, bool) {
	if len(m.projects) == 0 || m.cursor < 0 || m.cursor >= len(m.projects) {
		return trackedProject{}, false
	}
	return m.projects[m.cursor], true
}

func (m ProjectsModel) markedProjects() []trackedProject {
	marked := make([]trackedProject, 0, len(m.selected))
	for _, project := range m.projects {
		if m.selected[project.NodeID] {
			marked = append(marked, project)
		}
	}
	return marked
}

func (m *ProjectsModel) pruneSelections() {
	if len(m.selected) == 0 {
		return
	}
	valid := make(map[string]bool, len(m.projects))
	for _, project := range m.projects {
		valid[project.NodeID] = true
	}
	for id := range m.selected {
		if !valid[id] {
			delete(m.selected, id)
		}
	}
}

func loadTrackedProjectsCmd() tea.Cmd {
	return func() tea.Msg {
		projects, graphPath, err := loadTrackedProjectsFunc()
		return projectsLoadedMsg{projects: projects, graphPath: graphPath, err: err}
	}
}

func deleteTrackedProjectsCmd(graphPath string, projects []trackedProject) tea.Cmd {
	return func() tea.Msg {
		message, err := deleteTrackedProjectsFunc(graphPath, projects)
		return projectsActionMsg{message: message, err: err}
	}
}

func refreshTrackedProjectCmd(graphPath string, project trackedProject) tea.Cmd {
	return func() tea.Msg {
		message, err := refreshTrackedProjectFunc(graphPath, project)
		return projectsActionMsg{message: message, err: err}
	}
}

func loadTrackedProjects() ([]trackedProject, string, error) {
	graphPath, err := config.FindGraphFile(".")
	if err != nil {
		return nil, "", err
	}

	g, err := loadExistingGraph(graphPath)
	if err != nil {
		return nil, graphPath, err
	}

	projects := make([]trackedProject, 0)
	for _, node := range g.Nodes {
		if node.NodeType != string(types.NodeTypeProject) {
			continue
		}
		projects = append(projects, trackedProject{
			Name:   node.Label,
			NodeID: node.ID,
			Path:   projectPath(node),
			Remote: projectRemote(node),
		})
	}

	sort.Slice(projects, func(i, j int) bool {
		if projects[i].Name == projects[j].Name {
			return projects[i].Path < projects[j].Path
		}
		return projects[i].Name < projects[j].Name
	})

	return projects, graphPath, nil
}

func deleteTrackedProjects(graphPath string, projects []trackedProject) (string, error) {
	if len(projects) == 0 {
		return "No projects selected.", nil
	}

	g, err := loadExistingGraph(graphPath)
	if err != nil {
		return "", err
	}

	nodes, edges := removeProjectsFromGraph(g, projects)
	if err := export.WriteJSON(&types.Graph{Nodes: nodes, Edges: edges}, filepath.Dir(graphPath)); err != nil {
		return "", err
	}

	if err := pruneProjectCache(projects); err != nil {
		return "", err
	}

	return fmt.Sprintf("Removed %d tracked project(s).", len(projects)), nil
}

func refreshTrackedProject(graphPath string, project trackedProject) (string, error) {
	if project.Path == "" {
		return "", fmt.Errorf("project path metadata is unavailable; re-extract the codebase once from disk to enable refresh")
	}

	cfg, err := config.Load()
	if err != nil {
		return "", fmt.Errorf("loading config: %w", err)
	}

	files, err := trackedProjectFiles(project.Path)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no supported files found in %s", project.Path)
	}

	var provider types.LLMProvider
	if cfg.LLM.Provider != "" && cfg.LLM.Provider != "none" {
		if llmClient, lErr := llm.NewClient(&cfg.LLM); lErr == nil {
			provider = llmClient
		}
	}

	projectSrc := extract.DetectProject(project.Path)
	freshNodes, freshEdges, err := extract.ExtractAll(project.Path, files, provider, projectSrc, cfg.LLM.MaxChunkTokens)
	if err != nil {
		return "", err
	}

	existing, err := loadExistingGraph(graphPath)
	if err != nil {
		return "", err
	}
	seededNodes, seededEdges := removeProjectsFromGraph(existing, []trackedProject{project})

	g, buildErr := graph.Build(append(seededNodes, freshNodes...), append(seededEdges, freshEdges...))
	if buildErr != nil {
		return "", fmt.Errorf("building graph: %w", buildErr)
	}

	tg := g.ToTypes()
	if err := export.WriteJSON(tg, filepath.Dir(graphPath)); err != nil {
		return "", fmt.Errorf("writing graph.json: %w", err)
	}

	if err := refreshProjectCache(cfg.Extraction.CacheDir, project.Path, files); err != nil {
		return "", err
	}

	if cfg.Obsidian.AutoSync {
		vaultDir := config.ResolveVaultDir(cfg.Obsidian.VaultDir)
		if err := export.WriteObsidian(tg, vaultDir); err != nil {
			return "", fmt.Errorf("obsidian auto-sync: %w", err)
		}
	}

	return fmt.Sprintf("Refreshed %s.", project.Name), nil
}

func trackedProjectFiles(dir string) ([]string, error) {
	detected, err := detect.Files(dir)
	if err != nil {
		return nil, fmt.Errorf("detecting files: %w", err)
	}

	extSet := map[string]bool{
		".go": true, ".py": true, ".ts": true, ".tsx": true,
		".js": true, ".jsx": true, ".md": true, ".txt": true, ".pdf": true,
	}

	files := make([]string, 0, len(detected.Files))
	for _, entry := range detected.Files {
		if extSet[filepath.Ext(entry.AbsPath)] {
			files = append(files, entry.AbsPath)
		}
	}
	sort.Strings(files)
	return files, nil
}

func removeProjectsFromGraph(g *types.Graph, projects []trackedProject) ([]types.Node, []types.Edge) {
	projectNames := make(map[string]bool, len(projects))
	for _, project := range projects {
		projectNames[projectNameFromID(project.NodeID)] = true
	}

	removedNodeIDs := make(map[string]bool)
	nodes := make([]types.Node, 0, len(g.Nodes))
	for _, node := range g.Nodes {
		if belongsToAnyProject(node.ID, projectNames) {
			removedNodeIDs[node.ID] = true
			continue
		}
		nodes = append(nodes, node)
	}

	edges := make([]types.Edge, 0, len(g.Edges))
	for _, edge := range g.Edges {
		if removedNodeIDs[edge.Source] || removedNodeIDs[edge.Target] {
			continue
		}
		edges = append(edges, edge)
	}

	return nodes, edges
}

func refreshProjectCache(cacheDir, projectPath string, files []string) error {
	fileCache, err := cache.Load(cacheDir)
	if err != nil {
		return fmt.Errorf("loading cache: %w", err)
	}

	fileCache.DeletePrefix(projectPath)
	for _, file := range files {
		sha, err := cache.SHA256File(file)
		if err != nil {
			continue
		}
		fileCache.Mark(file, sha)
	}

	if err := fileCache.Save(); err != nil {
		return fmt.Errorf("saving cache: %w", err)
	}
	return nil
}

func pruneProjectCache(projects []trackedProject) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	fileCache, err := cache.Load(cfg.Extraction.CacheDir)
	if err != nil {
		return fmt.Errorf("loading cache: %w", err)
	}

	changed := false
	for _, project := range projects {
		if project.Path == "" {
			continue
		}
		if fileCache.DeletePrefix(project.Path) {
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if err := fileCache.Save(); err != nil {
		return fmt.Errorf("saving cache: %w", err)
	}
	return nil
}

func projectPath(node types.Node) string {
	if node.Metadata != nil {
		if path, ok := node.Metadata["path"].(string); ok {
			return path
		}
	}
	if node.Source != nil {
		return node.Source.Path
	}
	return ""
}

func projectRemote(node types.Node) string {
	if node.Metadata != nil {
		if remote, ok := node.Metadata["remote"].(string); ok {
			return remote
		}
	}
	if node.Source != nil {
		return node.Source.Remote
	}
	return ""
}

func projectNameFromID(nodeID string) string {
	return strings.TrimPrefix(nodeID, "project:")
}

func belongsToAnyProject(nodeID string, projectNames map[string]bool) bool {
	for name := range projectNames {
		if nodeID == extract.ProjectNodeID(name) || strings.HasPrefix(nodeID, name+":") {
			return true
		}
	}
	return false
}
