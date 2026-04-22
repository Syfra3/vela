package tui

import (
	"strings"
	"testing"

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
