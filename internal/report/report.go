package report

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/pkg/types"
)

// Generate writes GRAPH_REPORT.md to outDir, summarising the graph structure,
// god nodes, communities, surprise edges, and suggested questions.
func Generate(g *igraph.Graph, outDir string) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	godNodes := igraph.GodNodes(g, 5)
	surprises := igraph.SurpriseEdges(g, 5)
	questions := igraph.SuggestedQuestions(g.Nodes, g.ResolvedEdges)
	communities := communityMap(g)

	var sb strings.Builder

	// Header
	sb.WriteString("# Vela Graph Report\n\n")
	sb.WriteString(fmt.Sprintf("**Generated**: %s\n\n", time.Now().UTC().Format("2006-01-02 15:04:05 UTC")))
	sb.WriteString("---\n\n")

	// Summary
	sb.WriteString("## Summary\n\n")
	sb.WriteString(fmt.Sprintf("| Metric | Value |\n"))
	sb.WriteString(fmt.Sprintf("|--------|-------|\n"))
	sb.WriteString(fmt.Sprintf("| Nodes | %d |\n", g.NodeCount()))
	sb.WriteString(fmt.Sprintf("| Edges | %d |\n", g.EdgeCount()))
	sb.WriteString(fmt.Sprintf("| Communities | %d |\n", len(communities)))
	sb.WriteString(fmt.Sprintf("| God nodes (top 5) | %d |\n", len(godNodes)))
	sb.WriteString(fmt.Sprintf("| Surprise edges (top 5) | %d |\n\n", len(surprises)))

	// God Nodes
	sb.WriteString("## God Nodes (Top 5 by Degree)\n\n")
	if len(godNodes) == 0 {
		sb.WriteString("_No nodes found._\n\n")
	} else {
		sb.WriteString("| Rank | Label | Kind | Degree | File |\n")
		sb.WriteString("|------|-------|------|--------|------|\n")
		for i, n := range godNodes {
			sb.WriteString(fmt.Sprintf("| %d | `%s` | %s | %d | %s |\n",
				i+1, n.Label, n.NodeType, n.Degree, n.SourceFile))
		}
		sb.WriteString("\n")
	}

	// Communities
	sb.WriteString("## Communities\n\n")
	if len(communities) == 0 {
		sb.WriteString("_No community data (run with clustering enabled)._\n\n")
	} else {
		commIDs := make([]int, 0, len(communities))
		for id := range communities {
			commIDs = append(commIDs, id)
		}
		sort.Ints(commIDs)

		sb.WriteString("| Community | Members |\n")
		sb.WriteString("|-----------|--------|\n")
		for _, id := range commIDs {
			members := communities[id]
			labels := make([]string, 0, len(members))
			for _, nodeID := range members {
				// Resolve ID to label
				if n, ok := g.NodeByID[g.NodeIndex[nodeID]]; ok {
					labels = append(labels, n.Label)
				} else {
					labels = append(labels, nodeID)
				}
			}
			sort.Strings(labels)
			display := strings.Join(labels, ", ")
			if len(display) > 80 {
				display = display[:77] + "..."
			}
			sb.WriteString(fmt.Sprintf("| %d | %s (%d nodes) |\n", id, display, len(members)))
		}
		sb.WriteString("\n")
	}

	// Surprise Edges
	sb.WriteString("## Surprise Edges (Cross-Community)\n\n")
	if len(surprises) == 0 {
		sb.WriteString("_No cross-community edges detected._\n\n")
	} else {
		sb.WriteString("| From | To | Relation | File |\n")
		sb.WriteString("|------|----|----------|------|\n")
		for _, e := range surprises {
			sb.WriteString(fmt.Sprintf("| `%s` | `%s` | %s | %s |\n",
				e.Source, e.Target, e.Relation, e.SourceFile))
		}
		sb.WriteString("\n")
	}

	// Suggested Questions
	sb.WriteString("## Suggested Questions\n\n")
	for i, q := range questions {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, q))
	}
	sb.WriteString("\n")

	outPath := filepath.Join(outDir, "GRAPH_REPORT.md")
	return os.WriteFile(outPath, []byte(sb.String()), 0644)
}

// communityMap builds community → []nodeID from the graph's current node community assignments.
func communityMap(g *igraph.Graph) map[int][]string {
	m := make(map[int][]string)
	for _, n := range g.Nodes {
		m[n.Community] = append(m[n.Community], n.ID)
	}
	// All nodes in community 0 and no other communities means clustering wasn't run
	if len(m) == 1 {
		if _, onlyZero := m[0]; onlyZero {
			return nil // treat as unclustered
		}
	}
	return m
}

// NodesByType returns counts of each node type for summary stats (used by callers).
func NodesByType(nodes []types.Node) map[string]int {
	counts := make(map[string]int)
	for _, n := range nodes {
		counts[n.NodeType]++
	}
	return counts
}
