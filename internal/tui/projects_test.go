package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Syfra3/vela/internal/registry"
)

func TestLoadTrackedProjectsReadsProjectMetadata(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	registryPath := writeProjectsRegistry(t, home)

	projects, gotGraphPath, err := loadTrackedProjects(registryPath)
	if err != nil {
		t.Fatalf("loadTrackedProjects() error = %v", err)
	}
	if gotGraphPath != registryPath {
		t.Fatalf("graphPath = %q, want %q", gotGraphPath, registryPath)
	}
	if len(projects) != 2 {
		t.Fatalf("projects = %d, want 2", len(projects))
	}
	if projects[0].Name != "alpha" || projects[0].Path == "" {
		t.Fatalf("first project = %+v, want alpha with path", projects[0])
	}
	if projects[1].Remote != "https://github.com/org/vela.git" {
		t.Fatalf("second project remote = %q", projects[1].Remote)
	}
}

func TestDeleteTrackedProjectsRemovesGraphNodesAndCacheEntries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	registryPath := writeProjectsRegistry(t, home)
	cacheDir := filepath.Join(home, ".vela", "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	cacheJSON := []byte("{\n  \"/work/alpha/main.go\": \"a\",\n  \"/work/vela/main.go\": \"b\"\n}")
	if err := os.WriteFile(filepath.Join(cacheDir, "cache.json"), cacheJSON, 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	projectOutDir := filepath.Join(home, "alpha-output")
	message, err := deleteTrackedProjects(registryPath, []trackedProject{{Name: "alpha", NodeID: "/work/alpha", Path: "/work/alpha", GraphPath: filepath.Join(projectOutDir, "graph.json")}})
	if err != nil {
		t.Fatalf("deleteTrackedProjects() error = %v", err)
	}
	if message != "Purged index for alpha." {
		t.Fatalf("message = %q", message)
	}

	entries, err := registry.Load()
	if err != nil {
		t.Fatalf("registry.Load() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "vela" {
		t.Fatalf("entries = %+v, want vela only", entries)
	}
	if _, err := os.Stat(filepath.Join(projectOutDir, "graph.json")); !os.IsNotExist(err) {
		t.Fatalf("expected alpha graph.json removed, stat err = %v", err)
	}

	cacheData, err := os.ReadFile(filepath.Join(cacheDir, "cache.json"))
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	if string(cacheData) == string(cacheJSON) {
		t.Fatal("expected cache file to change")
	}
	if strings.Contains(string(cacheData), "/work/alpha/main.go") {
		t.Fatalf("alpha cache entry still present: %s", string(cacheData))
	}
	if !strings.Contains(string(cacheData), "/work/vela/main.go") {
		t.Fatalf("expected vela cache entry preserved: %s", string(cacheData))
	}
}

func TestProjectsModelMarksAndStartsActions(t *testing.T) {
	originalDelete := deleteTrackedProjectsFunc
	originalRefresh := refreshTrackedProjectFunc
	originalInstallHooks := installProjectHooksFunc
	originalUninstallHooks := uninstallProjectHooksFunc
	t.Cleanup(func() {
		deleteTrackedProjectsFunc = originalDelete
		refreshTrackedProjectFunc = originalRefresh
		installProjectHooksFunc = originalInstallHooks
		uninstallProjectHooksFunc = originalUninstallHooks
	})

	deleteTrackedProjectsFunc = func(string, []trackedProject) (string, error) { return "deleted", nil }
	refreshTrackedProjectFunc = func(string, trackedProject) (string, error) { return "refreshed", nil }
	installProjectHooksFunc = func(trackedProject) (string, error) { return "hooks installed", nil }
	uninstallProjectHooksFunc = func(trackedProject) (string, error) { return "hooks removed", nil }

	model := ProjectsModel{graphPath: "/tmp/graph.json", projects: []trackedProject{{Name: "vela", NodeID: "project:vela", Path: "/work/vela", HookStatus: "missing"}}, selected: map[string]bool{}}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	model = updated.(ProjectsModel)
	if !model.selected["project:vela"] {
		t.Fatal("expected project to be marked")
	}

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = updated.(ProjectsModel)
	if cmd != nil || !model.confirmingPurge {
		t.Fatal("expected delete action to wait for destructive confirmation")
	}
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(ProjectsModel)
	if cmd == nil || !model.running {
		t.Fatal("expected confirmed delete command and running state")
	}

	model.running = false
	model.projects[0].GraphPath = "/tmp/vela/graph.json"
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(ProjectsModel)
	if cmd == nil || !model.running {
		t.Fatal("expected refresh command and running state")
	}

	model.running = false
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	model = updated.(ProjectsModel)
	if cmd == nil || !model.running {
		t.Fatal("expected hook install command and running state")
	}

	model.running = false
	model.projects[0].HookInstalled = true
	model.projects[0].HookStatus = "installed"
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	model = updated.(ProjectsModel)
	if cmd == nil || !model.running {
		t.Fatal("expected hook uninstall command and running state")
	}
}

func TestProjectsModelViewFocusesOnActions(t *testing.T) {
	t.Parallel()

	model := ProjectsModel{
		projects: []trackedProject{{Name: "vela", NodeID: "/work/vela", Path: "/work/vela", GraphPath: "/tmp/vela/graph.json", HookStatus: "installed"}},
		selected: map[string]bool{},
	}

	view := model.ViewContent()
	for _, unwanted := range []string{"graph: present", "hooks: installed", "manifest:"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("did not expect %q in action-oriented view, got %q", unwanted, view)
		}
	}
	if !strings.Contains(view, "actions: refresh local data") {
		t.Fatalf("expected action copy in view, got %q", view)
	}
}

func TestProjectsModelFooterMentionsActions(t *testing.T) {
	t.Parallel()

	model := ProjectsModel{
		projects: []trackedProject{{Name: "vela", NodeID: "/work/vela", Path: "/work/vela", GraphStatus: "present", ManifestState: "present", ReportState: "present", HookInstalled: true, HookStatus: "installed"}},
		selected: map[string]bool{},
	}

	view := model.ViewContent()
	if !strings.Contains(view, "actions: refresh local data") {
		t.Fatalf("expected action copy in view, got %q", view)
	}
	if !strings.Contains(model.FooterHelp(), "enter/r refresh") {
		t.Fatalf("expected footer help to mention refresh action, got %q", model.FooterHelp())
	}
	if !strings.Contains(model.FooterHelp(), "g graph status") {
		t.Fatalf("expected footer help to mention graph status action, got %q", model.FooterHelp())
	}
	if !strings.Contains(model.FooterHelp(), "toggle hooks") {
		t.Fatalf("expected footer help to mention hook toggle, got %q", model.FooterHelp())
	}
}

func TestProjectsModelStartsGraphStatusForSelectedProject(t *testing.T) {
	t.Parallel()

	model := ProjectsModel{
		projects: []trackedProject{{Name: "vela", NodeID: "/work/vela", Path: "/work/vela", GraphPath: "/tmp/vela/graph.json"}},
		selected: map[string]bool{},
	}

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model = updated.(ProjectsModel)
	if cmd != nil {
		t.Fatal("did not expect graph status key to start a background command")
	}
	path, ok := model.ConsumeGraphStatusPath()
	if !ok {
		t.Fatal("expected graph status path to be available")
	}
	if path != "/tmp/vela/graph.json" {
		t.Fatalf("graph status path = %q, want /tmp/vela/graph.json", path)
	}
	if _, ok := model.ConsumeGraphStatusPath(); ok {
		t.Fatal("expected graph status path to be consumed once")
	}
}

// REQ-002/REQ-004 → SCN-003 → TestSCN003_ProjectsModelShowsSelectedProjectQueryResults
func TestSCN003_ProjectsModelShowsSelectedProjectQueryResults(t *testing.T) {
	// Scenario: User queries the selected project's graph from the TUI.
	original := runTrackedProjectQueryFunc
	t.Cleanup(func() { runTrackedProjectQueryFunc = original })
	runTrackedProjectQueryFunc = func(project trackedProject, query string) (string, error) {
		if project.Name != "alpha" || project.GraphPath != "/work/alpha/.vela/graph.json" {
			t.Fatalf("queried project = %+v, want alpha graph", project)
		}
		if query != "explain AuthService" {
			t.Fatalf("query = %q, want explain AuthService", query)
		}
		return "AuthService details", nil
	}

	model := ProjectsModel{
		projects: []trackedProject{{Name: "alpha", NodeID: "/work/alpha", Path: "/work/alpha", GraphPath: "/work/alpha/.vela/graph.json"}},
		selected: map[string]bool{},
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model = updated.(ProjectsModel)
	for _, r := range "explain AuthService" {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		model = updated.(ProjectsModel)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(ProjectsModel)

	view := model.ViewContent()
	for _, want := range []string{"Query results for alpha", "source: alpha", "AuthService details"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in query view, got %q", want, view)
		}
	}
}

// REQ-002/REQ-003 → SCN-004 → TestSCN004_ProjectsModelReportsUnreadableGraphRecoverably
func TestSCN004_ProjectsModelReportsUnreadableGraphRecoverably(t *testing.T) {
	// Scenario: TUI query shows recoverable status when graph is unavailable.
	original := runTrackedProjectQueryFunc
	t.Cleanup(func() { runTrackedProjectQueryFunc = original })
	runTrackedProjectQueryFunc = func(project trackedProject, query string) (string, error) {
		if project.Name != "alpha" {
			t.Fatalf("queried project = %+v, want alpha", project)
		}
		return "", os.ErrPermission
	}

	model := ProjectsModel{
		projects:   []trackedProject{{Name: "alpha", NodeID: "/work/alpha", Path: "/work/alpha", GraphPath: "/work/alpha/.vela/graph.json"}},
		selected:   map[string]bool{},
		termHeight: 40,
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model = updated.(ProjectsModel)
	for _, r := range "explain AuthService" {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		model = updated.(ProjectsModel)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(ProjectsModel)

	view := model.ViewContent()
	for _, want := range []string{"selected graph is unreadable", "alpha", "permission denied"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in recoverable query error view, got %q", want, view)
		}
	}
	if model.quitting || model.querying {
		t.Fatalf("model state after unreadable graph = quitting:%v querying:%v, want open for another action", model.quitting, model.querying)
	}
	if !strings.Contains(model.FooterHelp(), "q query graph") {
		t.Fatalf("expected footer to remain on project actions, got %q", model.FooterHelp())
	}
}

func TestProjectsModelGraphStatusRequiresTrackedGraph(t *testing.T) {
	t.Parallel()

	model := ProjectsModel{
		projects: []trackedProject{{Name: "vela", NodeID: "/work/vela", Path: "/work/vela"}},
		selected: map[string]bool{},
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model = updated.(ProjectsModel)
	if !model.msgIsErr {
		t.Fatal("expected missing graph path to set an error message")
	}
	if !strings.Contains(model.message, "No tracked graph snapshot found") {
		t.Fatalf("unexpected error message %q", model.message)
	}
	if _, ok := model.ConsumeGraphStatusPath(); ok {
		t.Fatal("did not expect graph status path when graph is missing")
	}
}

// REQ-004 → SCN-006 → TestSCN006_ProjectsModelDisambiguatesDuplicateProjectNames
func TestSCN006_ProjectsModelDisambiguatesDuplicateProjectNames(t *testing.T) {
	// Scenario: TUI disambiguates indexed projects with the same display name.
	model := ProjectsModel{
		projects: []trackedProject{
			{Name: "api", NodeID: "registry:api:/work/services/api", Path: "/work/services/api"},
			{Name: "api", NodeID: "registry:api:remote-only", Path: ""},
		},
		selected:   map[string]bool{},
		termHeight: 24,
	}

	view := model.ViewContent()
	if strings.Count(view, "api") < 2 {
		t.Fatalf("expected both duplicate projects to be shown, got %q", view)
	}
	for _, want := range []string{"/work/services/api", "id: registry:api:remote-only"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected duplicate project distinguisher %q in view, got %q", want, view)
		}
	}
}

// REQ-005/REQ-012 → SCN-007 → TestSCN007_ProjectsModelCancelsPerProjectPurge
func TestSCN007_ProjectsModelCancelsPerProjectPurge(t *testing.T) {
	// Scenario: User cancels per-project purge in the TUI.
	original := deleteTrackedProjectsFunc
	t.Cleanup(func() { deleteTrackedProjectsFunc = original })
	deleteTrackedProjectsFunc = func(string, []trackedProject) (string, error) {
		t.Fatal("deleteTrackedProjectsFunc must not run when destructive confirmation is canceled")
		return "", nil
	}

	model := ProjectsModel{
		graphPath: "/tmp/registry.json",
		projects: []trackedProject{
			{Name: "alpha", NodeID: "/work/alpha", Path: "/work/alpha", GraphPath: "/work/alpha/.vela/graph.db"},
			{Name: "beta", NodeID: "/work/beta", Path: "/work/beta", GraphPath: "/work/beta/.vela/graph.db"},
		},
		selected: map[string]bool{},
	}

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(ProjectsModel)
	if cmd != nil || !model.confirmingPurge {
		t.Fatalf("expected purge to wait for destructive confirmation, confirming:%v cmd:%v", model.confirmingPurge, cmd != nil)
	}
	if !strings.Contains(model.ViewContent(), "Confirm destructive purge for alpha") {
		t.Fatalf("expected confirmation to name alpha, got %q", model.ViewContent())
	}

	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(ProjectsModel)
	if cmd != nil || model.confirmingPurge || model.running {
		t.Fatalf("cancel state = confirming:%v running:%v cmd:%v, want no purge", model.confirmingPurge, model.running, cmd != nil)
	}
	if len(model.projects) != 2 || model.projects[0].Name != "alpha" || model.projects[1].Name != "beta" {
		t.Fatalf("projects after cancel = %+v, want alpha and beta still indexed", model.projects)
	}
	if !strings.Contains(model.message, "Purge canceled") {
		t.Fatalf("cancel message = %q, want purge canceled", model.message)
	}
}

// REQ-005/REQ-012 → SCN-008 → TestSCN008_ProjectsModelConfirmsPerProjectPurge
func TestSCN008_ProjectsModelConfirmsPerProjectPurge(t *testing.T) {
	// Scenario: User confirms per-project purge in the TUI.
	home := t.TempDir()
	t.Setenv("HOME", home)
	alphaGraphPath := filepath.Join(home, "alpha-output", "graph.json")
	betaGraphPath := filepath.Join(home, "beta-output", "graph.json")
	registryPath := writePurgeProjectsRegistry(t, home, alphaGraphPath, betaGraphPath)

	model := ProjectsModel{
		graphPath:  registryPath,
		termHeight: 40,
		projects: []trackedProject{
			{Name: "alpha", NodeID: "/work/alpha", Path: "/work/alpha", GraphPath: alphaGraphPath},
			{Name: "beta", NodeID: "/work/beta", Path: "/work/beta", GraphPath: betaGraphPath},
		},
		selected: map[string]bool{},
	}

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(ProjectsModel)
	if cmd != nil || !model.confirmingPurge {
		t.Fatalf("expected purge confirmation before deleting, confirming:%v cmd:%v", model.confirmingPurge, cmd != nil)
	}
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(ProjectsModel)
	if cmd == nil || !model.running {
		t.Fatalf("expected confirmed purge command, running:%v cmd:%v", model.running, cmd != nil)
	}
	actionMsg, ok := cmd().(projectsActionMsg)
	if !ok {
		t.Fatalf("purge command message = %T, want projectsActionMsg", cmd())
	}
	updated, _ = model.Update(actionMsg)
	model = updated.(ProjectsModel)
	if !strings.Contains(model.ViewContent(), "Purged index for alpha") {
		t.Fatalf("view = %q, want purge result for alpha", model.ViewContent())
	}

	entries, err := registry.Load()
	if err != nil {
		t.Fatalf("registry.Load() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "beta" {
		t.Fatalf("entries after alpha purge = %+v, want beta only", entries)
	}
	if _, err := os.Stat(alphaGraphPath); !os.IsNotExist(err) {
		t.Fatalf("alpha graph should be deleted, stat err = %v", err)
	}
	if _, err := os.Stat(betaGraphPath); err != nil {
		t.Fatalf("beta graph should remain indexed, stat err = %v", err)
	}
}

// REQ-005/REQ-012 → SCN-009 → TestSCN009_ProjectsModelConfirmsAllProjectsPurge
func TestSCN009_ProjectsModelConfirmsAllProjectsPurge(t *testing.T) {
	// Scenario: User confirms all-projects purge in the TUI.
	original := deleteTrackedProjectsFunc
	t.Cleanup(func() { deleteTrackedProjectsFunc = original })

	var attempted []trackedProject
	deleteTrackedProjectsFunc = func(_ string, projects []trackedProject) (string, error) {
		attempted = append([]trackedProject(nil), projects...)
		return "Purged index for alpha.\nFailed to purge beta: permission denied", os.ErrPermission
	}

	model := ProjectsModel{
		graphPath:    "/tmp/registry.json",
		termHeight:   40,
		termWidth:    100,
		scrollOffset: 3,
		projects: []trackedProject{
			{Name: "alpha", NodeID: "/work/alpha", Path: "/work/alpha", GraphPath: "/work/alpha/.vela/graph.db"},
			{Name: "beta", NodeID: "/work/beta", Path: "/work/beta", GraphPath: "/work/beta/.vela/graph.db"},
		},
		selected: map[string]bool{},
	}

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	model = updated.(ProjectsModel)
	if cmd != nil || !model.confirmingPurge {
		t.Fatalf("expected all-projects purge to wait for destructive confirmation, confirming:%v cmd:%v", model.confirmingPurge, cmd != nil)
	}
	view := model.ViewContent()
	for _, want := range []string{"Confirm destructive purge for all tracked projects", "alpha", "beta"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected confirmation view to contain %q, got %q", want, view)
		}
	}

	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(ProjectsModel)
	if cmd == nil || !model.running {
		t.Fatalf("expected confirmed all-projects purge command, running:%v cmd:%v", model.running, cmd != nil)
	}
	actionMsg, ok := cmd().(projectsActionMsg)
	if !ok {
		t.Fatalf("purge command message = %T, want projectsActionMsg", cmd())
	}
	updated, _ = model.Update(actionMsg)
	model = updated.(ProjectsModel)
	if len(attempted) != 2 || attempted[0].Name != "alpha" || attempted[1].Name != "beta" {
		t.Fatalf("attempted projects = %+v, want alpha and beta", attempted)
	}
	resultView := model.ViewContent()
	for _, want := range []string{"Purged index for alpha", "Failed to purge beta"} {
		if !strings.Contains(resultView, want) {
			t.Fatalf("expected result view to contain %q, got %q", want, resultView)
		}
	}
}

func TestProjectsModelScrollsToKeepSelectionVisible(t *testing.T) {
	t.Parallel()

	projects := make([]trackedProject, 0, 8)
	for i := 0; i < 8; i++ {
		projects = append(projects, trackedProject{
			Name:   "project-" + string(rune('a'+i)),
			NodeID: "/work/project-" + string(rune('a'+i)),
			Path:   "/work/project-" + string(rune('a'+i)),
			Remote: "git@example.com/org/project.git",
		})
	}
	model := ProjectsModel{
		projects:   projects,
		selected:   map[string]bool{},
		termWidth:  100,
		termHeight: 18,
	}

	before := model.ViewContent()
	if !strings.Contains(before, "Tracked Codebases") {
		t.Fatalf("expected header in initial viewport, got %q", before)
	}
	if !strings.Contains(before, "project-a") {
		t.Fatalf("expected first project in initial viewport, got %q", before)
	}
	if strings.Contains(before, "project-h") {
		t.Fatalf("did not expect last project before scrolling, got %q", before)
	}

	var updated tea.Model
	for i := 0; i < len(projects)-1; i++ {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model = updated.(ProjectsModel)
	}

	after := model.ViewContent()
	if model.scrollOffset == 0 {
		t.Fatal("expected scroll offset to advance for long project list")
	}
	if !strings.Contains(after, "project-h") {
		t.Fatalf("expected last project after navigating down, got %q", after)
	}
	if strings.Contains(after, "project-a") {
		t.Fatalf("did not expect first project after scrolling to bottom, got %q", after)
	}
	if !strings.Contains(model.FooterHelp(), "pgup/pgdown scroll") {
		t.Fatalf("expected footer to mention scroll help, got %q", model.FooterHelp())
	}
}

func TestMenuModelProjectsScreenUsesCurrentTerminalSize(t *testing.T) {
	t.Parallel()

	menu := NewMenuModel()
	menu.termWidth = 91
	menu.termHeight = 17
	menu.cursor = 3

	updated, _ := menu.handleMenuSelect()
	menu = updated.(MenuModel)

	if menu.screen != screenProjects {
		t.Fatalf("screen = %v, want %v", menu.screen, screenProjects)
	}
	if menu.projectsModel.termWidth != 91 || menu.projectsModel.termHeight != 17 {
		t.Fatalf("projects model size = %dx%d, want 91x17", menu.projectsModel.termWidth, menu.projectsModel.termHeight)
	}
}

func TestMenuModelProjectsOpensGraphStatusWithCurrentTerminalSize(t *testing.T) {
	t.Parallel()

	menu := NewMenuModel()
	menu.termWidth = 88
	menu.termHeight = 19
	menu.screen = screenProjects
	menu.projectsModel = ProjectsModel{
		projects:   []trackedProject{{Name: "vela", NodeID: "/work/vela", Path: "/work/vela", GraphPath: "/tmp/vela/graph.json"}},
		selected:   map[string]bool{},
		termWidth:  88,
		termHeight: 19,
	}

	updated, cmd := menu.updateProjects(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	menu = updated.(MenuModel)

	if menu.screen != screenGraphStatus {
		t.Fatalf("screen = %v, want %v", menu.screen, screenGraphStatus)
	}
	if cmd == nil {
		t.Fatal("expected graph status init command")
	}
	if menu.graphStatusModel.graphPath != "/tmp/vela/graph.json" {
		t.Fatalf("graph status path = %q, want /tmp/vela/graph.json", menu.graphStatusModel.graphPath)
	}
	if menu.graphStatusModel.termWidth != 88 || menu.graphStatusModel.termHeight != 19 {
		t.Fatalf("graph status model size = %dx%d, want 88x19", menu.graphStatusModel.termWidth, menu.graphStatusModel.termHeight)
	}
}

func writeProjectsRegistry(t *testing.T, home string) string {
	t.Helper()
	registryPath := filepath.Join(home, ".vela", "registry.json")
	if err := os.MkdirAll(filepath.Dir(registryPath), 0o755); err != nil {
		t.Fatalf("mkdir registry dir: %v", err)
	}
	alphaOutDir := filepath.Join(home, "alpha-output")
	velaOutDir := filepath.Join(home, "vela-output")
	for _, outDir := range []string{alphaOutDir, velaOutDir} {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			t.Fatalf("mkdir output dir: %v", err)
		}
		for _, name := range []string{"graph.json", "manifest.json", "GRAPH_REPORT.md"} {
			if err := os.WriteFile(filepath.Join(outDir, name), []byte("{}"), 0o644); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
		}
	}
	data, err := json.MarshalIndent(struct {
		Version int              `json:"version"`
		Entries []registry.Entry `json:"entries"`
	}{
		Version: 1,
		Entries: []registry.Entry{
			{Name: "alpha", RepoRoot: "/work/alpha", GraphPath: filepath.Join(alphaOutDir, "graph.json"), ManifestPath: filepath.Join(alphaOutDir, "manifest.json"), ReportPath: filepath.Join(alphaOutDir, "GRAPH_REPORT.md")},
			{Name: "vela", RepoRoot: "/work/vela", Remote: "https://github.com/org/vela.git", GraphPath: filepath.Join(velaOutDir, "graph.json"), ManifestPath: filepath.Join(velaOutDir, "manifest.json"), ReportPath: filepath.Join(velaOutDir, "GRAPH_REPORT.md")},
		},
	}, "", "  ")
	if err != nil {
		t.Fatalf("marshal registry: %v", err)
	}
	if err := os.WriteFile(registryPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write registry: %v", err)
	}
	return registryPath
}

func writePurgeProjectsRegistry(t *testing.T, home, alphaGraphPath, betaGraphPath string) string {
	t.Helper()
	for _, path := range []string{alphaGraphPath, betaGraphPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir graph dir: %v", err)
		}
		if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
			t.Fatalf("write graph: %v", err)
		}
	}
	registryPath := filepath.Join(home, ".vela", "registry.json")
	if err := os.MkdirAll(filepath.Dir(registryPath), 0o755); err != nil {
		t.Fatalf("mkdir registry dir: %v", err)
	}
	data, err := json.MarshalIndent(struct {
		Version int              `json:"version"`
		Entries []registry.Entry `json:"entries"`
	}{
		Version: 1,
		Entries: []registry.Entry{
			{Name: "alpha", RepoRoot: "/work/alpha", GraphPath: alphaGraphPath},
			{Name: "beta", RepoRoot: "/work/beta", GraphPath: betaGraphPath},
		},
	}, "", "  ")
	if err != nil {
		t.Fatalf("marshal registry: %v", err)
	}
	if err := os.WriteFile(registryPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write registry: %v", err)
	}
	return registryPath
}
