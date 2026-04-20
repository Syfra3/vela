package graph

import (
	"testing"

	"github.com/Syfra3/vela/pkg/types"
)

func declaredEdge(src, tgt, rel string) types.Edge {
	return types.Edge{
		Source:     src,
		Target:     tgt,
		Relation:   rel,
		Confidence: "DECLARED",
		Metadata: map[string]interface{}{
			"layer":               string(types.LayerContract),
			"evidence_type":       "openapi",
			"evidence_confidence": string(types.ConfidenceDeclared),
		},
	}
}

func derivedEdge(src, tgt, rel string) types.Edge {
	return types.Edge{
		Source:     src,
		Target:     tgt,
		Relation:   rel,
		Confidence: "INFERRED",
		Metadata: map[string]interface{}{
			"layer":               string(types.LayerRepo),
			"evidence_type":       "ast",
			"evidence_confidence": string(types.ConfidenceInferred),
		},
	}
}

// A declared edge and a weaker derived edge claim the same triple. Merge must
// keep declared truth and drop the derived duplicate — this is the headline
// invariant for the contract layer.
func TestMergeContract_DeclaredBeatsDerived(t *testing.T) {
	repoNodes := []types.Node{{ID: "project:acme", NodeType: "project"}}
	contractNodes := []types.Node{{ID: "contract:service:billing", NodeType: "service"}}

	declared := declaredEdge("contract:service:billing", "project:acme", "declared_in")
	derived := derivedEdge("contract:service:billing", "project:acme", "declared_in")

	nodes, edges := MergeContract(repoNodes, []types.Edge{derived}, contractNodes, []types.Edge{declared})

	if len(nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(nodes))
	}
	if len(edges) != 1 {
		t.Fatalf("edges = %d, want 1 (declared only)", len(edges))
	}
	if edges[0].Confidence != "DECLARED" {
		t.Errorf("surviving edge confidence = %q, want DECLARED", edges[0].Confidence)
	}
}

// Derived edges that do not collide with a declared triple must still be
// preserved. Declared does not erase independent repo-layer truth.
func TestMergeContract_PreservesNonCollidingDerivedEdges(t *testing.T) {
	declared := declaredEdge("contract:service:billing", "project:acme", "declared_in")
	derived := derivedEdge("project:acme:file:main.go", "project:acme:file:main.go:main", "contains")

	_, edges := MergeContract(
		[]types.Node{{ID: "project:acme"}},
		[]types.Edge{derived},
		[]types.Node{{ID: "contract:service:billing"}},
		[]types.Edge{declared},
	)
	if len(edges) != 2 {
		t.Fatalf("edges = %d, want 2", len(edges))
	}
}

// Contract nodes never collapse into repo nodes — they live in their own
// "contract:" namespace. The merge must preserve both sets.
func TestMergeContract_KeepsContractNodesDistinct(t *testing.T) {
	repoNodes := []types.Node{
		{ID: "project:acme", NodeType: "project"},
		{ID: "project:acme:file:billing.go", NodeType: "file"},
	}
	contractNodes := []types.Node{
		{ID: "contract:service:billing", NodeType: "service"},
		{ID: "contract:endpoint:billing:get:/invoices", NodeType: "contract"},
	}
	nodes, _ := MergeContract(repoNodes, nil, contractNodes, nil)
	if len(nodes) != 4 {
		t.Fatalf("nodes = %d, want 4", len(nodes))
	}
	seen := map[string]bool{}
	for _, n := range nodes {
		seen[n.ID] = true
	}
	for _, want := range []string{"project:acme", "contract:service:billing", "contract:endpoint:billing:get:/invoices"} {
		if !seen[want] {
			t.Errorf("missing node %q after merge", want)
		}
	}
}

// Re-ingesting the same declared edge twice must be idempotent.
func TestMergeContract_IdempotentDeclaredDedupe(t *testing.T) {
	declared := declaredEdge("contract:service:billing", "project:acme", "declared_in")
	_, edges := MergeContract(nil, nil, nil, []types.Edge{declared, declared})
	if len(edges) != 1 {
		t.Fatalf("edges = %d, want 1", len(edges))
	}
}

func TestMergeContract_PrefersStrongerEvidenceForSameTriple(t *testing.T) {
	repo := types.Edge{
		Source:   "contract:service:billing",
		Target:   "project:acme",
		Relation: "declared_in",
		Metadata: map[string]interface{}{
			"layer":               string(types.LayerRepo),
			"evidence_type":       "ast",
			"evidence_confidence": string(types.ConfidenceExtracted),
		},
	}
	contract := types.Edge{
		Source:   "contract:service:billing",
		Target:   "project:acme",
		Relation: "declared_in",
		Metadata: map[string]interface{}{
			"layer":               string(types.LayerContract),
			"evidence_type":       "heuristic",
			"evidence_confidence": string(types.ConfidenceInferred),
		},
	}

	_, edges := MergeContract(nil, []types.Edge{repo}, nil, []types.Edge{contract})
	if len(edges) != 1 {
		t.Fatalf("edges = %d, want 1", len(edges))
	}
	ev := types.EdgeEvidence(edges[0])
	if ev.Confidence != types.ConfidenceExtracted {
		t.Fatalf("surviving evidence confidence = %q, want %q", ev.Confidence, types.ConfidenceExtracted)
	}
}
