package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Syfra3/vela/internal/app"
	"github.com/Syfra3/vela/pkg/types"
)

func TestMenuModelRestoresClassicMainSurface(t *testing.T) {
	t.Parallel()

	m := NewMenuModel()
	got := make([]string, 0, len(m.items))
	for _, item := range m.items {
		got = append(got, item.key)
	}
	joined := strings.Join(got, ",")
	if joined != "extract,graphstatus,obsidian,projects,purge,quit" {
		t.Fatalf("menu keys = %q, want extract,graphstatus,obsidian,projects,purge,quit", joined)
	}
	view := m.View()
	if !strings.Contains(view, "██╗   ██╗███████╗██╗") {
		t.Fatalf("expected branded VELA header in view, got %q", view)
	}
	if strings.Contains(view, "_    __    __") {
		t.Fatalf("did not expect legacy compact header in view, got %q", view)
	}
	for _, want := range []string{"Status:", "Classic navigation restored"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected classic header content %q in view, got %q", want, view)
		}
	}
}

func TestHandleMenuSelectObsidianRestoresExportScreen(t *testing.T) {
	t.Parallel()

	m := NewMenuModel()
	obsidianIndex := -1
	for i, item := range m.items {
		if item.key == "obsidian" {
			obsidianIndex = i
			break
		}
	}
	if obsidianIndex == -1 {
		t.Fatal("obsidian menu item not found")
	}

	m.cursor = obsidianIndex
	updated, cmd := m.handleMenuSelect()
	menu := updated.(MenuModel)

	if menu.screen != screenObsidian {
		t.Fatalf("screen = %v, want %v", menu.screen, screenObsidian)
	}
	if cmd == nil {
		t.Fatal("expected obsidian export command")
	}
	view := menu.viewObsidian()
	for _, want := range []string{"Export to Obsidian", "starting export", "Progress:  0 / 4 chunks"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in obsidian view, got %q", want, view)
		}
	}
	if !strings.Contains(view, "██╗   ██╗███████╗██╗") {
		t.Fatalf("expected branded VELA header in export view, got %q", view)
	}
}

func TestViewObsidianShowsProgressContextWhileRunning(t *testing.T) {
	t.Parallel()

	m := NewMenuModelWithVersion("0.1.0")
	m.screen = screenObsidian
	m.obsidianRunning = true
	m.obsidianStep = 2
	m.obsidianTotal = 4
	m.obsidianStatus = "loading graph.json"
	m.obsidianStarted = time.Now().Add(-2 * time.Second)

	view := m.viewObsidian()
	for _, want := range []string{"Export to Obsidian", "loading graph.json", "Progress:  2 / 4 chunks"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in obsidian progress view, got %q", want, view)
		}
	}
}

func TestExtractModelNavigatesFoldersThenRunsBuildSummary(t *testing.T) {
	originalReadDir := readDirEntries
	originalRun := runTUIBuild
	t.Cleanup(func() {
		readDirEntries = originalReadDir
		runTUIBuild = originalRun
	})

	readDirEntries = func(root string) ([]dirEntry, error) {
		switch root {
		case "/repo":
			return []dirEntry{{Name: "cmd", Path: "/repo/cmd", IsDir: true}, {Name: "README.md", Path: "/repo/README.md", IsDir: false}}, nil
		case "/repo/cmd":
			return []dirEntry{{Name: "vela", Path: "/repo/cmd/vela", IsDir: true}}, nil
		default:
			return nil, nil
		}
	}
	runTUIBuild = func(req BuildRunRequest) (BuildRunResult, error) {
		return BuildRunResult{GraphPath: "/repo/cmd/vela/.vela/graph.json", HTMLPath: "/repo/cmd/vela/.vela/graph.html", ReportPath: "/repo/cmd/vela/.vela/GRAPH_REPORT.md", ObsidianPath: "/vault/obsidian", Files: 3, Facts: 2}, nil
	}

	m := NewExtractModelWithRoot("/repo")
	if !strings.Contains(m.ViewContent(), "Browse folders") {
		t.Fatalf("expected browser copy in initial view, got %q", m.ViewContent())
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(ExtractModel)
	if m.phase != extractPhaseBrowse {
		t.Fatalf("phase = %v, want browse", m.phase)
	}
	if m.currentDir != "/repo/cmd" {
		t.Fatalf("currentDir = %q, want /repo/cmd", m.currentDir)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = updated.(ExtractModel)
	if m.phase != extractPhaseConfirm {
		t.Fatalf("phase after select current = %v, want confirm", m.phase)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ExtractModel)
	if !m.running {
		t.Fatal("expected build to enter running state from confirm screen")
	}

	updated, _ = m.Update(buildFinishedMsg{result: BuildRunResult{GraphPath: "/repo/cmd/vela/.vela/graph.json", HTMLPath: "/repo/cmd/vela/.vela/graph.html", ReportPath: "/repo/cmd/vela/.vela/GRAPH_REPORT.md", ObsidianPath: "/vault/obsidian", Files: 3, Facts: 2}})
	m = updated.(ExtractModel)
	view := m.ViewContent()
	for _, want := range []string{"Build summary", "graph.json", "graph.html", "GRAPH_REPORT.md", "/vault/obsidian"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in view, got %q", want, view)
		}
	}
}

func TestExtractModelRunningViewShowsProgressBar(t *testing.T) {
	t.Parallel()

	m := ExtractModel{
		phase:      extractPhaseRunning,
		startedAt:  time.Now().Add(-3 * time.Second),
		totalFiles: 12,
		stage:      types.BuildStageScan,
		stageCount: 42,
		events:     []app.BuildEvent{{Kind: app.BuildEventStart, Message: "build started"}, {Kind: app.BuildEventStage, Stage: types.BuildStageDetect, Message: "detected source files", Count: 12}, {Kind: app.BuildEventStage, Stage: types.BuildStageScan, Message: "scanned source graph", Count: 42}},
	}

	view := m.ViewContent()
	for _, want := range []string{"Running build", "Files discovered: 12", "Current stage: scan (42)", "Recent events:"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in extraction progress view, got %q", want, view)
		}
	}
}

func TestExtractModelRunsAsyncBuild(t *testing.T) {
	originalReadDir := readDirEntries
	original := runTUIBuild
	t.Cleanup(func() {
		readDirEntries = originalReadDir
		runTUIBuild = original
	})

	readDirEntries = func(root string) ([]dirEntry, error) {
		return []dirEntry{{Name: "repo", Path: "/tmp/repo", IsDir: true}}, nil
	}

	runTUIBuild = func(req BuildRunRequest) (BuildRunResult, error) {
		return BuildRunResult{
			GraphPath: "/tmp/repo/.vela/graph.json",
			Files:     3,
			Facts:     2,
			Stages: []BuildStageSummary{
				{Name: "detect", Count: 3},
				{Name: "drivers", Count: 2},
			},
		}, nil
	}

	m := NewExtractModelWithRoot("/tmp")
	var cmd tea.Cmd

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ExtractModel)
	if m.phase != extractPhaseBrowse {
		t.Fatalf("phase = %v, want browse", m.phase)
	}
	if m.currentDir != "/tmp/repo" {
		t.Fatalf("currentDir = %q, want /tmp/repo", m.currentDir)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = updated.(ExtractModel)
	if m.phase != extractPhaseConfirm {
		t.Fatalf("phase after select current = %v, want confirm", m.phase)
	}

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ExtractModel)
	if !m.running {
		t.Fatal("expected build to enter running state")
	}
	if cmd == nil {
		t.Fatal("expected build command")
	}

	updated, _ = m.Update(buildFinishedMsg{result: BuildRunResult{
		GraphPath: "/tmp/repo/.vela/graph.json",
		Files:     3,
		Facts:     2,
		Stages:    []BuildStageSummary{{Name: "detect", Count: 3}},
	}})
	m = updated.(ExtractModel)

	if m.running {
		t.Fatal("expected build to stop running after completion")
	}
	if !strings.Contains(m.ViewContent(), "/tmp/repo/.vela/graph.json") {
		t.Fatalf("expected graph path in view, got %q", m.ViewContent())
	}
}

func TestExtractModelBrowsesIntoFolderAndSelectsCurrentDirectory(t *testing.T) {
	originalReadDir := readDirEntries
	t.Cleanup(func() {
		readDirEntries = originalReadDir
	})

	readDirEntries = func(root string) ([]dirEntry, error) {
		switch root {
		case "/repo":
			return []dirEntry{{Name: "cmd", Path: "/repo/cmd", IsDir: true}, {Name: "pkg", Path: "/repo/pkg", IsDir: true}}, nil
		case "/repo/cmd":
			return []dirEntry{{Name: "vela", Path: "/repo/cmd/vela", IsDir: true}}, nil
		default:
			return nil, nil
		}
	}

	m := NewExtractModelWithRoot("/repo")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ExtractModel)

	if m.phase != extractPhaseBrowse {
		t.Fatalf("phase after entering directory = %v, want browse", m.phase)
	}
	if m.currentDir != "/repo/cmd" {
		t.Fatalf("currentDir = %q, want /repo/cmd", m.currentDir)
	}
	if !strings.Contains(m.ViewContent(), "Current folder") {
		t.Fatalf("expected current folder copy in browser view, got %q", m.ViewContent())
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = updated.(ExtractModel)
	if m.phase != extractPhaseConfirm {
		t.Fatalf("phase after selecting current folder = %v, want confirm", m.phase)
	}
	if m.selected != "/repo/cmd" {
		t.Fatalf("selected = %q, want /repo/cmd", m.selected)
	}
	if !strings.Contains(m.ViewContent(), "/repo/cmd") {
		t.Fatalf("expected selected path in confirm view, got %q", m.ViewContent())
	}
}
