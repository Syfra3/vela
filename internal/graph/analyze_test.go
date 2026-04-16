package graph

import (
	"testing"

	"github.com/Syfra3/vela/pkg/types"
)

func TestGodNodes_TopByDegree(t *testing.T) {
	// Node "hub" has 3 edges; "leaf" has 1; "mid" has 2
	nodes := makeTestNodes("hub", "mid", "leaf", "other")
	edges := []types.Edge{
		{Source: "hub", Target: "mid", Relation: "calls", Confidence: "EXTRACTED"},
		{Source: "hub", Target: "leaf", Relation: "calls", Confidence: "EXTRACTED"},
		{Source: "hub", Target: "other", Relation: "calls", Confidence: "EXTRACTED"},
		{Source: "mid", Target: "leaf", Relation: "calls", Confidence: "EXTRACTED"},
	}

	g, _ := Build(nodes, edges)
	gods := GodNodes(g, 1)

	if len(gods) != 1 {
		t.Fatalf("expected 1 god node, got %d", len(gods))
	}
	if gods[0].Label != "hub" {
		t.Errorf("expected 'hub' as top god node, got %q", gods[0].Label)
	}
}

func TestGodNodes_Empty(t *testing.T) {
	g, _ := Build(nil, nil)
	gods := GodNodes(g, 5)
	if len(gods) != 0 {
		t.Errorf("expected 0 god nodes for empty graph")
	}
}

func TestGodNodes_FewerThanTopN(t *testing.T) {
	nodes := makeTestNodes("a", "b")
	g, _ := Build(nodes, nil)
	gods := GodNodes(g, 10)
	if len(gods) != 2 {
		t.Errorf("expected 2 god nodes (capped at node count), got %d", len(gods))
	}
}

func TestSurpriseEdges_CrossCommunity(t *testing.T) {
	nodes := []types.Node{
		{ID: "a", Label: "a", NodeType: "function", Community: 0},
		{ID: "b", Label: "b", NodeType: "function", Community: 0},
		{ID: "c", Label: "c", NodeType: "function", Community: 1},
		{ID: "d", Label: "d", NodeType: "function", Community: 1},
	}
	edges := []types.Edge{
		{Source: "a", Target: "b", Relation: "calls", Confidence: "EXTRACTED"}, // same community
		{Source: "a", Target: "c", Relation: "calls", Confidence: "EXTRACTED"}, // cross-community
		{Source: "b", Target: "d", Relation: "calls", Confidence: "EXTRACTED"}, // cross-community
	}

	g, _ := Build(nodes, edges)
	surprises := SurpriseEdges(g, 5)

	// Only cross-community edges should appear
	if len(surprises) != 2 {
		t.Errorf("expected 2 surprise edges, got %d: %+v", len(surprises), surprises)
	}
	for _, e := range surprises {
		if e.Source == "a" && e.Target == "b" {
			t.Error("same-community edge a→b should not be a surprise")
		}
	}
}

func TestSuggestedQuestions_Count(t *testing.T) {
	nodes := makeTestNodes("auth", "payment", "user")
	edges := []types.Edge{
		{Source: "auth", Target: "user", Relation: "calls", Confidence: "EXTRACTED"},
		{Source: "payment", Target: "user", Relation: "calls", Confidence: "EXTRACTED"},
	}

	questions := SuggestedQuestions(nodes, edges)
	if len(questions) < 4 {
		t.Errorf("expected at least 4 questions, got %d", len(questions))
	}
	// First question should reference a node name
	if len(questions[0]) < 10 {
		t.Errorf("question too short: %q", questions[0])
	}
}
