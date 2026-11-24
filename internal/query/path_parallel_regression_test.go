package query

import (
	"strings"
	"testing"

	"github.com/Syfra3/vela/pkg/types"
)

func TestFilePathParallelEdgesUseEligibleDeterministicEvidence(t *testing.T) {
	nodes := []types.Node{{ID: "a", Label: "a.go", NodeType: "file"}, {ID: "b", Label: "b.go", NodeType: "file"}}
	edges := []types.Edge{
		{Source: "a", Target: "b", Relation: "contains", Confidence: "high"},
		{Source: "a", Target: "b", Relation: "depends_on", Confidence: "low", SourceFile: "z.go"},
		{Source: "a", Target: "b", Relation: "depends_on", Confidence: "low", SourceFile: "a.go"},
	}
	var first string
	for i := 0; i < len(edges); i++ {
		ordered := append(append([]types.Edge{}, edges[i:]...), edges[:i]...)
		e := newEngine(&types.Graph{Nodes: nodes, Edges: ordered})
		result := e.PathResult("a", "b")
		if result.Status != ResultStatusOK || len(result.Facts) != 1 || result.Facts[0].Predicate != "depends_on" {
			t.Fatalf("result: %+v", result)
		}
		edge, ok := e.pathEdgeBetween("a", "b", true)
		if !ok || edge.SourceFile != "a.go" {
			t.Fatalf("non-deterministic selected edge: %+v", edge)
		}
		text := e.Path("a", "b")
		if !strings.Contains(text, "--[depends_on]-->") || strings.Contains(text, "--[contains]-->") {
			t.Fatalf("text: %s", text)
		}
		if i > 0 && text != first {
			t.Fatalf("different text across edge orders: %q != %q", text, first)
		}
		first = text
	}
}
