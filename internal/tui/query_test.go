package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	vquery "github.com/Syfra3/vela/internal/query"
)

func TestNewQueryModelStartsLoading(t *testing.T) {
	t.Parallel()

	m := NewQueryModel()
	if !m.loading {
		t.Fatal("expected search model to start loading")
	}
	if !strings.Contains(m.ViewContent(), "Loading search index...") {
		t.Fatalf("expected loading state in view, got %q", m.ViewContent())
	}
}

func TestHandleMenuSelectSearchStartsInit(t *testing.T) {
	t.Parallel()

	m := NewMenuModel()
	searchIndex := -1
	for i, item := range m.items {
		if item.key == "query" {
			searchIndex = i
			if item.label != "Search" {
				t.Fatalf("menu label = %q, want Search", item.label)
			}
			break
		}
	}
	if searchIndex == -1 {
		t.Fatal("search menu item not found")
	}

	m.cursor = searchIndex
	updated, cmd := m.handleMenuSelect()
	menu := updated.(MenuModel)

	if menu.screen != screenQuery {
		t.Fatalf("screen = %v, want %v", menu.screen, screenQuery)
	}
	if !menu.queryModel.loading {
		t.Fatal("expected search screen to start loading")
	}
	if cmd == nil {
		t.Fatal("expected search selection to return init command")
	}
	if !strings.Contains(menu.viewQuery(), "Type to edit") {
		t.Fatalf("expected search footer, got %q", menu.viewQuery())
	}
}

func TestQueryModelExecutesFederatedSearch(t *testing.T) {
	originalLoad := queryLoadSearcherFunc
	originalSearch := querySearchFunc
	t.Cleanup(func() {
		queryLoadSearcherFunc = originalLoad
		querySearchFunc = originalSearch
	})

	queryLoadSearcherFunc = func() (*vquery.Searcher, error) {
		return &vquery.Searcher{}, nil
	}
	querySearchFunc = func(searcher *vquery.Searcher, input string) (vquery.SearchResponse, error) {
		return vquery.SearchResponse{
			Query: input,
			Hits: []vquery.SearchHit{{
				ID:            "code:retriever",
				Label:         "FederatedRetriever",
				Kind:          "struct",
				Path:          "internal/query/search.go",
				Snippet:       "Combines graph and memory retrieval into one ranked result set.",
				Score:         7.5,
				PrimarySource: "vela_graph",
				Sources:       []string{"vela_graph", "ancora"},
			}},
			Metrics: vquery.SearchMetrics{
				Limit:      5,
				AncoraOnly: vquery.StrategyMetrics{LatencyMs: 2, Returned: 1},
				Federated:  vquery.StrategyMetrics{LatencyMs: 4, Returned: 1},
				Comparison: vquery.ComparisonMetrics{OverlapAtK: 1, AddedByFederated: 0},
			},
		}, nil
	}

	m := NewQueryModel()
	loaded, _ := m.Update(querySearcherLoadedMsg{searcher: &vquery.Searcher{}})
	m = loaded.(QueryModel)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("retriever")})
	m = updated.(QueryModel)
	if m.input != "retriever" {
		t.Fatalf("input = %q, want retriever", m.input)
	}

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(QueryModel)
	if !m.loading {
		t.Fatal("expected loading state while search command is running")
	}
	if cmd == nil {
		t.Fatal("expected enter to trigger search command")
	}

	updated, _ = m.Update(querySearchResultMsg{response: vquery.SearchResponse{
		Query: "retriever",
		Hits: []vquery.SearchHit{{
			ID:            "code:retriever",
			Label:         "FederatedRetriever",
			Kind:          "struct",
			Path:          "internal/query/search.go",
			Snippet:       "Combines graph and memory retrieval into one ranked result set.",
			Score:         7.5,
			PrimarySource: "vela_graph",
			Sources:       []string{"vela_graph", "ancora"},
		}},
		Metrics: vquery.SearchMetrics{
			Limit:      5,
			AncoraOnly: vquery.StrategyMetrics{LatencyMs: 2, Returned: 1},
			Federated:  vquery.StrategyMetrics{LatencyMs: 4, Returned: 1},
			Comparison: vquery.ComparisonMetrics{OverlapAtK: 1, AddedByFederated: 0},
		},
	}})
	m = updated.(QueryModel)

	view := m.ViewContent()
	if !m.hasSearched {
		t.Fatal("expected search response to mark hasSearched")
	}
	if !strings.Contains(view, "FederatedRetriever") {
		t.Fatalf("expected result label in view, got %q", view)
	}
	if !strings.Contains(view, "[Graph]") {
		t.Fatalf("expected source label in view, got %q", view)
	}
	if !strings.Contains(view, "sources: Graph, Ancora") {
		t.Fatalf("expected federated source summary in view, got %q", view)
	}
	if !strings.Contains(view, "Ancora 2ms/1  |  Federated 4ms/1") {
		t.Fatalf("expected metrics summary in view, got %q", view)
	}
}
