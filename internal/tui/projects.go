package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Syfra3/vela/internal/cache"
	"github.com/Syfra3/vela/internal/config"
	"github.com/Syfra3/vela/internal/detect"
	"github.com/Syfra3/vela/internal/hooks"
	"github.com/Syfra3/vela/internal/registry"
)

type trackedProject struct {
	Name          string
	NodeID        string
	Path          string
	Remote        string
	GraphPath     string
	GraphStatus   string
	GraphPresent  bool
	ManifestState string
	ReportState   string
	HookInstalled bool
	HookStatus    string
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
	installProjectHooksFunc   = installProjectHooks
	uninstallProjectHooksFunc = uninstallProjectHooks
)

type ProjectsModel struct {
	cursor        int
	quitting      bool
	loading       bool
	running       bool
	graphStatus   string
	preferredPath string
	termWidth     int
	termHeight    int
	scrollOffset  int

	graphPath string
	projects  []trackedProject
	selected  map[string]bool

	message  string
	msgIsErr bool
	err      error
}

func NewProjectsModel() ProjectsModel {
	return NewProjectsModelWithGraphPath("")
}

func NewProjectsModelWithGraphPath(graphPath string) ProjectsModel {
	return ProjectsModel{loading: true, selected: make(map[string]bool), preferredPath: strings.TrimSpace(graphPath), termWidth: 100, termHeight: 24}
}

func (m ProjectsModel) Init() tea.Cmd { return loadTrackedProjectsCmd(m.preferredPath) }

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
		m.ensureCursorVisible()
		return m, nil
	case projectsActionMsg:
		m.running = false
		m.message = msg.message
		m.msgIsErr = msg.err != nil
		if msg.err != nil {
			m.message = msg.err.Error()
		}
		return m, loadTrackedProjectsCmd(m.graphPath)
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height
		m.ensureCursorVisible()
		return m, nil
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
				m.ensureCursorVisible()
			}
		case "down", "j":
			if m.cursor < len(m.projects)-1 {
				m.cursor++
				m.ensureCursorVisible()
			}
		case "pgup", "ctrl+u":
			m.scrollOffset -= m.viewportHeight()
			m.clampScroll()
		case "pgdown", "ctrl+d":
			m.scrollOffset += m.viewportHeight()
			m.clampScroll()
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
		case "enter":
			fallthrough
		case "r":
			project, ok := m.currentProject()
			if !ok {
				return m, nil
			}
			m.running = true
			m.message = ""
			return m, refreshTrackedProjectCmd(m.graphPath, project)
		case "h":
			project, ok := m.currentProject()
			if !ok {
				return m, nil
			}
			if project.Path == "" {
				m.message = "Project path metadata is unavailable; re-extract from disk to manage hooks."
				m.msgIsErr = true
				return m, nil
			}
			m.running = true
			m.message = ""
			if project.HookInstalled {
				return m, uninstallTrackedProjectHooksCmd(project)
			}
			return m, installTrackedProjectHooksCmd(project)
		case "g":
			project, ok := m.currentProject()
			if !ok {
				return m, nil
			}
			if strings.TrimSpace(project.GraphPath) == "" {
				m.message = "No tracked graph snapshot found for this project. Refresh it first."
				m.msgIsErr = true
				return m, nil
			}
			m.graphStatus = project.GraphPath
			m.message = ""
			m.msgIsErr = false
			return m, nil
		}
	}
	return m, nil
}

func (m ProjectsModel) View() string { return m.ViewContent() }

func (m ProjectsModel) ViewContent() string {
	content := m.renderContent()
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return ""
	}
	viewportHeight := m.viewportHeight()
	if viewportHeight <= 0 || len(lines) <= viewportHeight {
		return content
	}
	start := m.scrollOffset
	if start < 0 {
		start = 0
	}
	maxStart := len(lines) - viewportHeight
	if start > maxStart {
		start = maxStart
	}
	end := start + viewportHeight
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[start:end], "\n")
}

func (m ProjectsModel) renderContent() string {
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
		b.WriteString(mutedStyle.Render("No tracked codebases found in ~/.vela/registry.json."))
		b.WriteString("\n\n")
		b.WriteString(textStyle.Render("Use Extract to add a codebase, then return here to inspect, refresh, or remove it."))
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
		b.WriteString(mutedStyle.Render("     actions: refresh local data • view graph status • toggle hooks • remove tracked data"))
		b.WriteString("\n")
		b.WriteString("\n")
	}
	b.WriteString(mutedStyle.Render("Delete removes tracked local data, registry state, and managed hooks. Source directories stay intact."))
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
	help := "↑↓ navigate • enter/r refresh • g graph status • x mark • h toggle hooks • d delete marked • esc back"
	if m.maxScrollOffset() > 0 {
		help += " • pgup/pgdown scroll"
	}
	return help
}

func (m *ProjectsModel) ConsumeGraphStatusPath() (string, bool) {
	if m == nil || strings.TrimSpace(m.graphStatus) == "" {
		return "", false
	}
	graphPath := m.graphStatus
	m.graphStatus = ""
	return graphPath, true
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

func (m *ProjectsModel) ensureCursorVisible() {
	if m == nil {
		return
	}
	start, end := m.projectLineRange(m.cursor)
	viewportHeight := m.viewportHeight()
	if viewportHeight <= 0 {
		m.scrollOffset = 0
		return
	}
	if start < m.scrollOffset {
		m.scrollOffset = start
	}
	if end > m.scrollOffset+viewportHeight {
		m.scrollOffset = end - viewportHeight
	}
	if end-start > viewportHeight {
		m.scrollOffset = start
	}
	m.clampScroll()
}

func (m *ProjectsModel) clampScroll() {
	if m == nil {
		return
	}
	maxOffset := m.maxScrollOffset()
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
}

func (m ProjectsModel) viewportHeight() int {
	height := m.termHeight - 14
	if height < 6 {
		height = 6
	}
	return height
}

func (m ProjectsModel) maxScrollOffset() int {
	if m.loading || m.err != nil {
		return 0
	}
	lines := strings.Split(strings.TrimRight(m.renderContent(), "\n"), "\n")
	viewportHeight := m.viewportHeight()
	if len(lines) <= viewportHeight {
		return 0
	}
	return len(lines) - viewportHeight
}

func (m ProjectsModel) projectLineRange(index int) (int, int) {
	line := 0
	if m.loading || m.err != nil || len(m.projects) == 0 {
		return 0, 0
	}
	line += 2
	for i, project := range m.projects {
		start := line
		line += projectLineCount(project)
		if i == index {
			return start, line
		}
	}
	return line, line
}

func projectLineCount(project trackedProject) int {
	count := 4
	if strings.TrimSpace(project.Remote) != "" {
		count++
	}
	return count
}

func loadTrackedProjectsCmd(graphPath string) tea.Cmd {
	return func() tea.Msg {
		projects, resolvedGraphPath, err := loadTrackedProjectsFunc(strings.TrimSpace(graphPath))
		return projectsLoadedMsg{projects: projects, graphPath: resolvedGraphPath, err: err}
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

func installTrackedProjectHooksCmd(project trackedProject) tea.Cmd {
	return func() tea.Msg {
		message, err := installProjectHooksFunc(project)
		return projectsActionMsg{message: message, err: err}
	}
}

func uninstallTrackedProjectHooksCmd(project trackedProject) tea.Cmd {
	return func() tea.Msg {
		message, err := uninstallProjectHooksFunc(project)
		return projectsActionMsg{message: message, err: err}
	}
}

func loadTrackedProjects(preferredGraphPath string) ([]trackedProject, string, error) {
	entries, err := registry.Load()
	if err != nil {
		return nil, "", err
	}
	projects := make([]trackedProject, 0)
	for _, entry := range entries {
		path := entry.RepoRoot
		hookInstalled, hookStatus := trackedProjectHookState(path)
		graphPresent := fileExists(entry.GraphPath)
		graphStatus := "missing"
		if graphPresent {
			graphStatus = "present"
		}
		projects = append(projects, trackedProject{
			Name:          entry.Name,
			NodeID:        entry.RepoRoot,
			Path:          path,
			Remote:        entry.Remote,
			GraphPath:     entry.GraphPath,
			GraphStatus:   graphStatus,
			GraphPresent:  graphPresent,
			ManifestState: presentState(fileExists(entry.ManifestPath)),
			ReportState:   presentState(fileExists(entry.ReportPath)),
			HookInstalled: hookInstalled,
			HookStatus:    hookStatus,
		})
	}
	sort.Slice(projects, func(i, j int) bool {
		if projects[i].Name == projects[j].Name {
			return projects[i].Path < projects[j].Path
		}
		return projects[i].Name < projects[j].Name
	})
	return projects, config.RegistryFilePath(), nil
}

func deleteTrackedProjects(graphPath string, projects []trackedProject) (string, error) {
	if len(projects) == 0 {
		return "No projects selected.", nil
	}
	for _, project := range projects {
		if project.Path != "" {
			if err := hooks.Uninstall(project.Path); err != nil {
				return "", err
			}
		}
		if err := removeTrackedProjectArtifacts(project); err != nil {
			return "", err
		}
		if err := registry.RemoveTrackedRepo(project.Path); err != nil {
			return "", err
		}
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
	if _, err := runTUIBuild(BuildRunRequest{RepoRoot: project.Path, Mode: "update"}); err != nil {
		return "", err
	}
	if err := refreshProjectCacheForBuild(project.Path); err != nil {
		return "", err
	}
	return fmt.Sprintf("Refreshed %s.", project.Name), nil
}

func installProjectHooks(project trackedProject) (string, error) {
	if project.Path == "" {
		return "", fmt.Errorf("project path metadata is unavailable; re-extract the codebase once from disk to enable hook management")
	}
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	if err := hooks.Install(project.Path, exePath); err != nil {
		return "", err
	}
	return fmt.Sprintf("Installed hooks for %s.", project.Name), nil
}

func uninstallProjectHooks(project trackedProject) (string, error) {
	if project.Path == "" {
		return "", fmt.Errorf("project path metadata is unavailable; re-extract the codebase once from disk to enable hook management")
	}
	if err := hooks.Uninstall(project.Path); err != nil {
		return "", err
	}
	return fmt.Sprintf("Removed hooks for %s.", project.Name), nil
}

func trackedProjectHookState(projectPath string) (bool, string) {
	if strings.TrimSpace(projectPath) == "" {
		return false, "path unavailable"
	}
	status, err := hooks.Inspect(projectPath)
	if err != nil {
		return false, "unavailable"
	}
	installed := status.Hooks["post-commit"] && status.Hooks["post-checkout"]
	if installed {
		return true, "installed"
	}
	if status.Hooks["post-commit"] || status.Hooks["post-checkout"] {
		return false, "partial"
	}
	return false, "missing"
}

func trackedProjectFiles(dir string) ([]string, error) {
	detected, err := detect.Files(dir)
	if err != nil {
		return nil, fmt.Errorf("detecting files: %w", err)
	}
	extSet := map[string]bool{".go": true, ".py": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".md": true, ".txt": true, ".pdf": true}
	files := make([]string, 0, len(detected.Files))
	for _, entry := range detected.Files {
		if extSet[filepath.Ext(entry.AbsPath)] {
			files = append(files, entry.AbsPath)
		}
	}
	sort.Strings(files)
	return files, nil
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

func refreshProjectCacheForBuild(projectPath string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	files, detectErr := trackedProjectFiles(projectPath)
	if detectErr != nil {
		return detectErr
	}
	return refreshProjectCache(cfg.Extraction.CacheDir, projectPath, files)
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

func removeTrackedProjectArtifacts(project trackedProject) error {
	for _, target := range []string{project.GraphPath, strings.TrimSpace(filepath.Join(filepath.Dir(project.GraphPath), "graph.html")), strings.TrimSpace(filepath.Join(filepath.Dir(project.GraphPath), "manifest.json")), project.ReportPath(), strings.TrimSpace(filepath.Join(project.Path, ".vela"))} {
		if strings.TrimSpace(target) == "" {
			continue
		}
		if err := os.RemoveAll(target); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", target, err)
		}
	}
	outDir := strings.TrimSpace(filepath.Dir(project.GraphPath))
	if outDir != "" {
		_ = os.Remove(outDir)
	}
	return nil
}

func (p trackedProject) ReportPath() string {
	if strings.TrimSpace(p.GraphPath) == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p.GraphPath), "GRAPH_REPORT.md")
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func presentState(ok bool) string {
	if ok {
		return "present"
	}
	return "missing"
}
