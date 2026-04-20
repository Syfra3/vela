package types

import "testing"

func TestCanonicalKeyString(t *testing.T) {
	k := CanonicalKey{Layer: LayerRepo, Kind: "function", Key: "internal/query/search.go#Run"}
	got := k.String()
	want := "repo:function:internal/query/search.go#Run"
	if got != want {
		t.Fatalf("CanonicalKey.String() = %q, want %q", got, want)
	}
}

func TestCanonicalKeyIsZero(t *testing.T) {
	var zero CanonicalKey
	if !zero.IsZero() {
		t.Fatalf("zero-valued CanonicalKey should be zero")
	}
	if (CanonicalKey{Layer: LayerMemory}).IsZero() {
		t.Fatalf("non-zero CanonicalKey must not report zero")
	}
}

func TestLayerOf(t *testing.T) {
	cases := map[SourceType]Layer{
		SourceTypeCodebase:    LayerRepo,
		SourceTypeMemory:      LayerMemory,
		SourceTypeWebhook:     LayerMemory,
		SourceType("unknown"): Layer(""),
	}
	for src, want := range cases {
		if got := LayerOf(src); got != want {
			t.Errorf("LayerOf(%q) = %q, want %q", src, got, want)
		}
	}
}

func TestLayerConstantsDistinct(t *testing.T) {
	layers := []Layer{LayerRepo, LayerContract, LayerWorkspace, LayerMemory}
	seen := make(map[Layer]bool, len(layers))
	for _, l := range layers {
		if l == "" {
			t.Fatalf("layer constant must not be empty")
		}
		if seen[l] {
			t.Fatalf("duplicate layer constant: %q", l)
		}
		seen[l] = true
	}
}

func TestEvidenceZeroValueIsUsable(t *testing.T) {
	// Zero Evidence must be safe to pass through fusion/ranking code paths
	// before any layer fills it in.
	var e Evidence
	if e.Layer != "" || e.Confidence != "" || e.Verification != "" {
		t.Fatalf("zero Evidence must have empty discriminators, got %+v", e)
	}
}

func TestCanonicalKeyForID_ObservationAliasesCollapse(t *testing.T) {
	legacy := CanonicalKeyForID("ancora:obs:42", "observation", nil)
	graph := CanonicalKeyForID("memory:observation:42", "observation", nil)
	want := "memory:observation:ancora:obs:42"
	if legacy.String() != want {
		t.Fatalf("legacy canonical = %q, want %q", legacy.String(), want)
	}
	if graph.String() != want {
		t.Fatalf("graph canonical = %q, want %q", graph.String(), want)
	}
}

func TestCanonicalKeyForNode_ResolvesRepoAndWorkspaceNodes(t *testing.T) {
	repo := CanonicalKeyForNode(Node{ID: "project:vela", NodeType: string(NodeTypeProject)})
	if repo.String() != "repo:repo:vela" {
		t.Fatalf("repo canonical = %q", repo.String())
	}

	workspace := CanonicalKeyForNode(Node{ID: "workspace:repo:vela", NodeType: string(NodeTypeRepo)})
	if workspace.String() != "workspace:repo:vela" {
		t.Fatalf("workspace canonical = %q", workspace.String())
	}
}

func TestCanonicalJoinKey_FallsBackToCanonicalAliases(t *testing.T) {
	if got := CanonicalJoinKey("ancora:obs:42", "observation", nil); got != "memory:observation:ancora:obs:42" {
		t.Fatalf("legacy join key = %q", got)
	}
	if got := CanonicalJoinKey("memory:observation:42", "observation", nil); got != "memory:observation:ancora:obs:42" {
		t.Fatalf("graph join key = %q", got)
	}
	if got := CanonicalJoinKey("plain-id", "", nil); got != "plain-id" {
		t.Fatalf("fallback join key = %q", got)
	}
}

func TestEdgeEvidence_ReadsTypedMetadata(t *testing.T) {
	edge := Edge{
		Confidence: "DECLARED",
		Score:      0.75,
		Metadata: map[string]interface{}{
			"layer":                    string(LayerMemory),
			"evidence_type":            "observation-reference",
			"evidence_source_artifact": "ancora:obs:7",
			"evidence_confidence":      string(ConfidenceDeclared),
			"verification":             string(VerificationRedirected),
		},
	}

	ev := EdgeEvidence(edge)
	if ev.Layer != LayerMemory || ev.Type != "observation-reference" {
		t.Fatalf("unexpected evidence identity: %+v", ev)
	}
	if ev.SourceArtifact != "ancora:obs:7" {
		t.Fatalf("source artifact = %q", ev.SourceArtifact)
	}
	if ev.Confidence != ConfidenceDeclared || ev.Verification != VerificationRedirected {
		t.Fatalf("unexpected evidence state: %+v", ev)
	}
	if ev.Score != 0.75 {
		t.Fatalf("score = %v, want 0.75", ev.Score)
	}
}

func TestPreferEdgeEvidence_PicksStrongerEvidence(t *testing.T) {
	current := Edge{Metadata: map[string]interface{}{
		"evidence_confidence": string(ConfidenceInferred),
	}}
	candidate := Edge{Metadata: map[string]interface{}{
		"evidence_confidence": string(ConfidenceExtracted),
	}}
	if !PreferEdgeEvidence(candidate, current) {
		t.Fatal("expected extracted evidence to beat inferred evidence")
	}
}
