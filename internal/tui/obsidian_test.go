package tui

import (
	"path/filepath"
	"testing"

	"github.com/Syfra3/vela/internal/config"
	"github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/internal/registry"
	"github.com/Syfra3/vela/pkg/types"
)

func TestLoadObsidianExportGraph_UsesTrackedRegistryGraphs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	alphaGraph := writeTestGraphJSON(t, filepath.Join(home, "alpha-out"), "alpha")
	betaGraph := writeTestGraphJSON(t, filepath.Join(home, "beta-out"), "beta")
	lastGraph := writeTestGraphJSON(t, filepath.Join(home, "last-out"), "last-only")

	if err := registry.Save([]registry.Entry{
		{RepoRoot: filepath.Join(home, "src", "alpha"), Name: "alpha", GraphPath: alphaGraph},
		{RepoRoot: filepath.Join(home, "src", "beta"), Name: "beta", GraphPath: betaGraph},
	}); err != nil {
		t.Fatalf("registry.Save() error = %v", err)
	}

	g, source, err := loadObsidianExportGraph(lastGraph)
	if err != nil {
		t.Fatalf("loadObsidianExportGraph() error = %v", err)
	}
	if source != config.RegistryFilePath() {
		t.Fatalf("source = %q, want %q", source, config.RegistryFilePath())
	}
	if !hasNodeLabel(g, "alpha") || !hasNodeLabel(g, "beta") {
		t.Fatalf("expected merged tracked repos in graph, got labels %v", nodeLabels(g))
	}
	if hasNodeLabel(g, "last-only") {
		t.Fatalf("expected registry-backed export to ignore lastGraphPath, got labels %v", nodeLabels(g))
	}
}

func TestLoadObsidianExportGraph_FallsBackToSingleGraphPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	graphPath := writeTestGraphJSON(t, filepath.Join(home, "solo-out"), "solo")

	g, source, err := loadObsidianExportGraph(graphPath)
	if err != nil {
		t.Fatalf("loadObsidianExportGraph() error = %v", err)
	}
	if source != graphPath {
		t.Fatalf("source = %q, want %q", source, graphPath)
	}
	if !hasNodeLabel(g, "solo") {
		t.Fatalf("expected fallback graph node label %q, got %v", "solo", nodeLabels(g))
	}
}

func writeTestGraphJSON(t *testing.T, outDir, project string) string {
	t.Helper()

	g := &types.Graph{
		Nodes: []types.Node{{
			ID:       "project:" + project,
			Label:    project,
			NodeType: string(types.NodeTypeProject),
			Source: &types.Source{
				Type: types.SourceTypeCodebase,
				Name: project,
				Path: filepath.Join(outDir, project),
			},
		}},
	}
	if err := export.WriteJSON(g, outDir); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	return filepath.Join(outDir, "graph.json")
}

func hasNodeLabel(g *types.Graph, want string) bool {
	for _, node := range g.Nodes {
		if node.Label == want {
			return true
		}
	}
	return false
}

func nodeLabels(g *types.Graph) []string {
	labels := make([]string, 0, len(g.Nodes))
	for _, node := range g.Nodes {
		labels = append(labels, node.Label)
	}
	return labels
}
