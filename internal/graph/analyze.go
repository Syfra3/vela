package graph

import (
	"fmt"
	"sort"

	"github.com/Syfra3/vela/pkg/types"
)

// GodNodes returns the topN nodes sorted by total degree (in + out).
// These are the most-connected concepts in the graph — architectural hubs.
func GodNodes(g *Graph, topN int) []types.Node {
	if len(g.Nodes) == 0 {
		return nil
	}

	type scored struct {
		node   types.Node
		degree int
	}

	items := make([]scored, len(g.Nodes))
	for i, n := range g.Nodes {
		items[i] = scored{node: n, degree: g.Degree(n.ID)}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].degree > items[j].degree
	})

	limit := topN
	if limit > len(items) {
		limit = len(items)
	}

	// Write degree back onto a copy of the node so callers can display it
	result := make([]types.Node, limit)
	for i := 0; i < limit; i++ {
		n := items[i].node
		n.Degree = items[i].degree
		result[i] = n
	}
	return result
}

// SurpriseEdges returns the topN edges that cross community boundaries,
// ranked by the product of the degrees of their endpoints (a proxy for
// betweenness centrality: high-degree nodes in different communities that
// are directly connected are architecturally "surprising").
func SurpriseEdges(g *Graph, topN int) []types.Edge {
	if len(g.ResolvedEdges) == 0 {
		return nil
	}

	// Build community lookup: nodeID → communityID
	commOf := make(map[string]int, len(g.Nodes))
	// Also build label → nodeID for reverse lookup (targets are labels after resolution)
	labelToID := make(map[string]string, len(g.Nodes))
	for _, n := range g.Nodes {
		commOf[n.ID] = n.Community
		labelToID[n.Label] = n.ID
	}

	type scored struct {
		edge  types.Edge
		score float64
	}

	var candidates []scored
	for _, e := range g.ResolvedEdges {
		cFrom, okFrom := commOf[e.Source]
		// After Build(), e.Target is the resolved node label — map back to ID.
		toID := labelToID[e.Target]
		cTo, okTo := commOf[toID]

		if !okFrom || !okTo {
			continue
		}
		if cFrom == cTo {
			continue // same community — not surprising
		}

		degFrom := float64(g.Degree(e.Source))
		degTo := float64(g.Degree(toID))
		score := degFrom * degTo

		candidates = append(candidates, scored{edge: e, score: score})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	limit := topN
	if limit > len(candidates) {
		limit = len(candidates)
	}

	result := make([]types.Edge, limit)
	for i := 0; i < limit; i++ {
		result[i] = candidates[i].edge
	}
	return result
}

// SuggestedQuestions returns 4–5 question templates populated with the names
// of the top god nodes. These are questions the graph is uniquely positioned
// to answer through path-finding and community analysis.
func SuggestedQuestions(nodes []types.Node, edges []types.Edge) []string {
	g, _ := Build(nodes, edges)
	godN := GodNodes(g, 3)

	names := make([]string, len(godN))
	for i, n := range godN {
		names[i] = n.Label
	}

	// Pad with placeholders if fewer than 2 god nodes
	for len(names) < 2 {
		names = append(names, "<node>")
	}

	return []string{
		fmt.Sprintf("What does %s depend on, and what depends on it?", names[0]),
		fmt.Sprintf("What is the shortest dependency path from %s to %s?", names[0], names[1]),
		fmt.Sprintf("Which components would break if %s changed its interface?", names[0]),
		"Which communities are most isolated from the rest of the graph?",
		"What are the most unexpected cross-domain connections in this codebase?",
	}
}
