package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/internal/retrieval"
	"github.com/Syfra3/vela/pkg/types"
)

func withProjectStubEmbeddings(t *testing.T) {
	t.Helper()
	restore := retrieval.SetEmbedTextsForTesting(func(texts []string) ([][]float32, error) {
		out := make([][]float32, 0, len(texts))
		for range texts {
			out = append(out, []float32{1, 0})
		}
		return out, nil
	})
	t.Cleanup(restore)
}

func TestLoadTrackedProjectsReadsProjectMetadata(t *testing.T) {
	withProjectStubEmbeddings(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	graphPath := writeProjectsGraph(t, home)

	projects, gotGraphPath, err := loadTrackedProjects()
	if err != nil {
		t.Fatalf("loadTrackedProjects() error = %v", err)
	}
	if gotGraphPath != graphPath {
		t.Fatalf("graphPath = %q, want %q", gotGraphPath, graphPath)
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
	withProjectStubEmbeddings(t)
	originalHome := os.Getenv("HOME")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", originalHome)
	})

	home := t.TempDir()
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatalf("set HOME: %v", err)
	}

	graphPath := writeProjectsGraph(t, home)
	cacheDir := filepath.Join(home, ".vela", "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	cacheJSON := []byte("{\n  \"/work/alpha/main.go\": \"a\",\n  \"/work/vela/main.go\": \"b\"\n}")
	if err := os.WriteFile(filepath.Join(cacheDir, "cache.json"), cacheJSON, 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	message, err := deleteTrackedProjects(graphPath, []trackedProject{{Name: "alpha", NodeID: "project:alpha", Path: "/work/alpha"}})
	if err != nil {
		t.Fatalf("deleteTrackedProjects() error = %v", err)
	}
	if message != "Removed 1 tracked project(s)." {
		t.Fatalf("message = %q", message)
	}

	g, err := loadExistingGraph(graphPath)
	if err != nil {
		t.Fatalf("loadExistingGraph() error = %v", err)
	}
	if len(g.Nodes) != 2 {
		t.Fatalf("remaining nodes = %d, want 2", len(g.Nodes))
	}
	for _, node := range g.Nodes {
		if belongsToAnyProject(node.ID, map[string]bool{"alpha": true}) {
			t.Fatalf("alpha node still present: %s", node.ID)
		}
	}

	cacheData, err := os.ReadFile(filepath.Join(cacheDir, "cache.json"))
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	if string(cacheData) == string(cacheJSON) {
		t.Fatal("expected cache file to change")
	}
	if string(cacheData) == "" || contains(string(cacheData), "/work/alpha/main.go") {
		t.Fatalf("alpha cache entry still present: %s", string(cacheData))
	}
	if !contains(string(cacheData), "/work/vela/main.go") {
		t.Fatalf("expected vela cache entry preserved: %s", string(cacheData))
	}
}

func TestProjectsModelMarksAndStartsActions(t *testing.T) {
	originalDelete := deleteTrackedProjectsFunc
	originalRefresh := refreshTrackedProjectFunc
	t.Cleanup(func() {
		deleteTrackedProjectsFunc = originalDelete
		refreshTrackedProjectFunc = originalRefresh
	})

	deleteTrackedProjectsFunc = func(string, []trackedProject) (string, error) {
		return "deleted", nil
	}
	refreshTrackedProjectFunc = func(string, trackedProject) (string, error) {
		return "refreshed", nil
	}

	model := ProjectsModel{
		graphPath: "/tmp/graph.json",
		projects:  []trackedProject{{Name: "vela", NodeID: "project:vela", Path: "/work/vela"}},
		selected:  map[string]bool{},
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	model = updated.(ProjectsModel)
	if !model.selected["project:vela"] {
		t.Fatal("expected project to be marked")
	}

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = updated.(ProjectsModel)
	if cmd == nil || !model.running {
		t.Fatal("expected delete command and running state")
	}

	model.running = false
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(ProjectsModel)
	if cmd == nil || !model.running {
		t.Fatal("expected refresh command and running state")
	}
}

func TestGraphContainsProject(t *testing.T) {
	project := &types.Source{Type: types.SourceTypeCodebase, Name: "vela", Path: "/work/vela"}
	if graphContainsProject(&types.Graph{Nodes: []types.Node{{ID: "memory:observation:1", NodeType: string(types.NodeTypeObservation)}}}, project) {
		t.Fatal("expected memory-only graph to report project missing")
	}
	if !graphContainsProject(&types.Graph{Nodes: []types.Node{{ID: "project:vela", NodeType: string(types.NodeTypeProject), Source: project}}}, project) {
		t.Fatal("expected graph to contain project source")
	}
}

func writeProjectsGraph(t *testing.T, home string) string {
	t.Helper()

	outDir := filepath.Join(home, ".vela")

	g := &types.Graph{
		Nodes: []types.Node{
			{
				ID:       "project:alpha",
				Label:    "alpha",
				NodeType: string(types.NodeTypeProject),
				Metadata: map[string]interface{}{"path": "/work/alpha"},
				Source:   &types.Source{Type: types.SourceTypeCodebase, Name: "alpha", Path: "/work/alpha"},
			},
			{
				ID:         "alpha:file:main.go",
				Label:      "main.go",
				NodeType:   string(types.NodeTypeFile),
				SourceFile: "main.go",
				Source:     &types.Source{Type: types.SourceTypeCodebase, Name: "alpha", Path: "/work/alpha"},
			},
			{
				ID:       "project:vela",
				Label:    "vela",
				NodeType: string(types.NodeTypeProject),
				Metadata: map[string]interface{}{"path": "/work/vela", "remote": "https://github.com/org/vela.git"},
				Source:   &types.Source{Type: types.SourceTypeCodebase, Name: "vela", Path: "/work/vela", Remote: "https://github.com/org/vela.git"},
			},
			{
				ID:         "vela:file:main.go",
				Label:      "main.go",
				NodeType:   string(types.NodeTypeFile),
				SourceFile: "main.go",
				Source:     &types.Source{Type: types.SourceTypeCodebase, Name: "vela", Path: "/work/vela"},
			},
		},
		Edges: []types.Edge{{Source: "project:alpha", Target: "alpha:file:main.go", Relation: "contains"}},
	}

	if err := export.WriteJSON(g, outDir); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	return filepath.Join(outDir, "graph.json")
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
