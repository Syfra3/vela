package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	igraph "github.com/Syfra3/vela/internal/graph"
)

func TestGraphStatusModelScrollsLongContent(t *testing.T) {
	t.Parallel()

	m := GraphStatusModel{
		termWidth:  100,
		termHeight: 18,
		metrics: igraph.HealthMetrics{
			Nodes:                5491,
			Edges:                12466,
			ConnectedPct:         88,
			ResolutionRate:       1,
			ExtractedRate:        0.22,
			Communities:          1,
			LargestCommunitySize: 5491,
			TopByOutDegree: []igraph.NodeRank{
				{Label: "stock-chef", Kind: "project", OutDeg: 1748, InDeg: 0},
				{Label: "apps/server-api/src/database/seeders/seed.module.ts", Kind: "file", OutDeg: 58, InDeg: 4, File: "apps/server-api/src/database/seeders/seed.module.ts"},
				{Label: "apps/server-api/src/app.module.ts", Kind: "file", OutDeg: 52, InDeg: 18, File: "apps/server-api/src/app.module.ts"},
				{Label: "packages/api-client/src/hooks/index.ts", Kind: "file", OutDeg: 47, InDeg: 2, File: "packages/api-client/src/hooks/index.ts"},
				{Label: "apps/server-api/src/shared/database/database.module.ts", Kind: "file", OutDeg: 47, InDeg: 3, File: "apps/server-api/src/shared/database/database.module.ts"},
			},
		},
		projects: []igraph.ProjectStatus{{Name: "stock-chef", Path: "/work/stock-chef", Nodes: 5491, Files: 312, Symbols: 5178, OutgoingEdges: 12466}},
	}

	before := m.ViewContent()
	if !strings.Contains(before, "Graph") {
		t.Fatalf("expected top of graph status content, got %q", before)
	}
	if strings.Contains(before, "Obsidian Vault") {
		t.Fatalf("did not expect bottom section before scrolling, got %q", before)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(GraphStatusModel)
	for i := 0; i < m.maxScrollOffset()+2; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = updated.(GraphStatusModel)
	}

	after := m.ViewContent()
	if !strings.Contains(after, "Obsidian Vault") {
		t.Fatalf("expected scrolled content to reach lower sections, got %q", after)
	}
	if !strings.Contains(m.FooterHelp(), "scroll") {
		t.Fatalf("expected scroll help, got %q", m.FooterHelp())
	}
}

func TestGraphStatusModelTruncatesLongTopNodeLabels(t *testing.T) {
	t.Parallel()

	longLabel := "apps/server-api/src/database/seeders/seed.module.ts"
	m := GraphStatusModel{
		termWidth:  72,
		termHeight: 30,
		metrics: igraph.HealthMetrics{
			TopByOutDegree: []igraph.NodeRank{{
				Label:  longLabel,
				Kind:   "file",
				OutDeg: 58,
				InDeg:  4,
				File:   longLabel,
			}},
		},
	}

	view := m.renderContent()
	if !strings.Contains(view, "Top Nodes by Out-Degree") {
		t.Fatalf("expected top nodes section, got %q", view)
	}
	if !strings.Contains(view, "…") {
		t.Fatalf("expected truncated label with ellipsis, got %q", view)
	}
	if strings.Contains(view, "seeders\n") || strings.Contains(view, "module.ts\n    [file]") {
		t.Fatalf("expected single-line top node row without wrapped fragments, got %q", view)
	}
}

func TestGraphStatusModelRendersFreshnessSection(t *testing.T) {
	t.Parallel()

	m := GraphStatusModel{
		termWidth:  100,
		termHeight: 30,
		fresh: igraph.FreshnessStats{
			GraphPath:         "/repo/.vela/graph.json",
			ManifestPresent:   true,
			ReportPresent:     true,
			TrackedFiles:      42,
			BuildMode:         "deleted_only_prune",
			GraphUpdatedAt:    time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC),
			ManifestUpdatedAt: time.Date(2026, 4, 23, 12, 1, 0, 0, time.UTC),
		},
	}

	view := m.renderContent()
	for _, want := range []string{"Freshness", "Tracked files", "42", "Last refresh mode", "deleted-file prune", "GRAPH_REPORT.md"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in freshness view, got %q", want, view)
		}
	}
}

func TestGraphStatusModelRendersProjectSection(t *testing.T) {
	t.Parallel()

	m := GraphStatusModel{
		termWidth:  100,
		termHeight: 30,
		projects: []igraph.ProjectStatus{{
			Name:          "ancora",
			Path:          "/home/geen/Documents/personal/ancora",
			Remote:        "git@github.com:Syfra3/ancora.git",
			Nodes:         60,
			Files:         12,
			Symbols:       47,
			OutgoingEdges: 98,
		}},
	}

	view := m.renderContent()
	for _, want := range []string{"Projects", "ancora", "git@github.com:Syfra3/ancora.git", "nodes:60", "files:12", "outgoing edges:98"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in project status view, got %q", want, view)
		}
	}
}

func TestGraphStatusModelRendersGlobalRegistrySection(t *testing.T) {
	t.Parallel()

	m := GraphStatusModel{
		global:     true,
		termWidth:  100,
		termHeight: 30,
		registryData: igraph.RegistryStatusSnapshot{
			Summary: igraph.RegistrySummary{Repositories: 2, HealthyGraphs: 1, MissingGraphs: 1, InstalledHooks: 1, MissingHooks: 1, Nodes: 120, Edges: 240},
			Repos: []igraph.RepoStatusSnapshot{
				{Name: "vela", RepoRoot: "/work/vela", ManifestPath: "/work/vela/.vela/manifest.json", ReportPath: "/work/vela/.vela/GRAPH_REPORT.md", HookInstalled: true, HookStatus: "installed", Snapshot: igraph.StatusSnapshot{Freshness: igraph.FreshnessStats{ManifestPresent: true, ReportPresent: true}, Metrics: igraph.HealthMetrics{Nodes: 60, Edges: 120}, Projects: []igraph.ProjectStatus{{Name: "Syfra3/vela", Path: "/work/vela", Nodes: 60, Files: 12, Symbols: 47, OutgoingEdges: 98}}}},
				{Name: "ancora", RepoRoot: "/work/ancora", HookStatus: "missing", LoadError: "graph path unavailable"},
			},
		},
	}

	view := m.renderContent()
	for _, want := range []string{"Global Summary", "Tracked repos", "2", "Repositories", "vela", "ancora", "graph path unavailable", "manifest file", "report file", "Syfra3/vela", "outgoing edges:98"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in global registry view, got %q", want, view)
		}
	}
}

func TestGraphStatusModelSubtitleUsesSelectedRepositoryName(t *testing.T) {
	t.Parallel()

	m := GraphStatusModel{repoName: "ancora"}
	if got := m.Subtitle(); got != "Repository: ancora" {
		t.Fatalf("Subtitle() = %q", got)
	}
}

func TestGraphStatusModelSubtitleUsesGlobalLabel(t *testing.T) {
	t.Parallel()

	m := GraphStatusModel{global: true}
	if got := m.Subtitle(); got != "Global tracked repositories" {
		t.Fatalf("Subtitle() = %q", got)
	}
}
