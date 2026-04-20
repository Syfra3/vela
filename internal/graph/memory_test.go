package graph_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Syfra3/vela/internal/ancora"
	"github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/pkg/types"
)

func sampleObs() []ancora.Observation {
	now := time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC)
	return []ancora.Observation{
		{
			ID: 1, Title: "Fix auth bug", Content: "root cause X",
			Type: "bugfix", Workspace: "vela", Visibility: "work",
			Organization: "glim",
			References: `[{"type":"file","target":"internal/auth/auth.go"},` +
				`{"type":"function","target":"ValidateToken"},` +
				`{"type":"observation","target":"ancora:obs:2"}]`,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: 2, Title: "Decision: use JWT", Content: "chose JWT",
			Type: "decision", Workspace: "vela", Visibility: "work",
			References: `[{"type":"concept","target":"auth"}]`,
			CreatedAt:  now, UpdatedAt: now,
		},
		{
			ID: 3, Title: "Architecture note", Content: "layered retrieval",
			Type: "architecture", Workspace: "ancora", Visibility: "personal",
			CreatedAt: now, UpdatedAt: now,
		},
	}
}

func nodeByID(nodes []types.Node, id string) (types.Node, bool) {
	for _, n := range nodes {
		if n.ID == id {
			return n, true
		}
	}
	return types.Node{}, false
}

func edgesFrom(edges []types.Edge, src string) []types.Edge {
	var out []types.Edge
	for _, e := range edges {
		if e.Source == src {
			out = append(out, e)
		}
	}
	return out
}

func TestBuildMemory_EmptyInput(t *testing.T) {
	g := graph.BuildMemory(nil, graph.MemoryOptions{})
	if g == nil {
		t.Fatal("BuildMemory returned nil")
	}
	if len(g.Nodes) != 0 || len(g.Edges) != 0 {
		t.Errorf("expected empty graph, got %d nodes %d edges", len(g.Nodes), len(g.Edges))
	}
}

func TestBuildMemory_RootAndScopeNodes(t *testing.T) {
	g := graph.BuildMemory(sampleObs(), graph.MemoryOptions{})

	if _, ok := nodeByID(g.Nodes, graph.MemoryRootID); !ok {
		t.Fatalf("missing memory root node")
	}
	for _, id := range []string{
		graph.MemoryWorkspaceID("vela"),
		graph.MemoryWorkspaceID("ancora"),
		graph.MemoryVisibilityID("work"),
		graph.MemoryVisibilityID("personal"),
		graph.MemoryOrganizationID("glim"),
	} {
		if _, ok := nodeByID(g.Nodes, id); !ok {
			t.Errorf("missing scope node %q", id)
		}
	}

	// All memory nodes stamped with layer=memory.
	for _, n := range g.Nodes {
		if !strings.HasPrefix(n.ID, "memory:") {
			t.Errorf("node %q is not in memory namespace", n.ID)
		}
		if got := n.Metadata["layer"]; got != string(types.LayerMemory) {
			t.Errorf("node %q layer=%v want memory", n.ID, got)
		}
		if n.Metadata["evidence_confidence"] == nil {
			t.Errorf("node %q missing evidence_confidence", n.ID)
		}
	}
}

func TestBuildMemory_ObservationBindings(t *testing.T) {
	g := graph.BuildMemory(sampleObs(), graph.MemoryOptions{})

	obs1 := graph.MemoryObservationID(1)
	relTargets := map[string]string{}
	for _, e := range edgesFrom(g.Edges, obs1) {
		relTargets[e.Relation+"|"+e.Target] = "ok"
	}
	for _, want := range []string{
		"belongs_to|" + graph.MemoryWorkspaceID("vela"),
		"scoped_to|" + graph.MemoryVisibilityID("work"),
		"belongs_to|" + graph.MemoryOrganizationID("glim"),
	} {
		if _, ok := relTargets[want]; !ok {
			t.Errorf("obs1 missing binding %q", want)
		}
	}
}

func TestBuildMemory_ReferenceTargetsAreCanonical(t *testing.T) {
	g := graph.BuildMemory(sampleObs(), graph.MemoryOptions{})
	obs1 := graph.MemoryObservationID(1)

	found := map[string]types.Edge{}
	for _, e := range edgesFrom(g.Edges, obs1) {
		found[e.Target] = e
	}

	fileEdge, ok := found[graph.RefRepoFile+"internal/auth/auth.go"]
	if !ok {
		t.Fatal("file reference not emitted with canonical repo:file: target")
	}
	if fileEdge.Relation != graph.MemoryRelConstrains {
		t.Errorf("bugfix file edge relation = %q, want %q", fileEdge.Relation, graph.MemoryRelConstrains)
	}
	if fileEdge.Metadata["cross_layer"] != true {
		t.Errorf("file reference edge missing cross_layer=true")
	}
	if fileEdge.Metadata["target_layer"] != string(types.LayerRepo) {
		t.Errorf("file reference edge target_layer = %v, want repo", fileEdge.Metadata["target_layer"])
	}
	if fileEdge.Metadata["verification"] != string(types.VerificationCurrent) {
		t.Errorf("file reference edge verification = %v, want current", fileEdge.Metadata["verification"])
	}

	if _, ok := found[graph.RefRepoSymbol+"ValidateToken"]; !ok {
		t.Error("function reference not emitted with canonical repo:symbol: target")
	}

	if obsEdge, ok := found[graph.MemoryObservationID(2)]; !ok {
		t.Error("observation -> observation reference not emitted")
	} else if obsEdge.Relation != graph.MemoryRelRelatedTo {
		t.Errorf("obs ref relation = %q, want related_to", obsEdge.Relation)
	}
}

func TestBuildMemory_KnownRepoEmitsCrossLayerMention(t *testing.T) {
	g := graph.BuildMemory(sampleObs(), graph.MemoryOptions{
		KnownRepos: []string{"vela"},
	})

	want := graph.RefWorkspaceRepo + "vela"
	var mention *types.Edge
	for _, e := range g.Edges {
		if e.Target == want && e.Relation == graph.MemoryRelMentions {
			ee := e
			mention = &ee
			break
		}
	}
	if mention == nil {
		t.Fatalf("expected a cross-layer mention edge to %s", want)
	}
	if mention.Metadata["target_layer"] != string(types.LayerWorkspace) {
		t.Errorf("mention edge target_layer = %v, want workspace", mention.Metadata["target_layer"])
	}
}

func TestBuildMemory_WorkspaceAndOrganizationReferencesStayInMemoryLayer(t *testing.T) {
	now := time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC)
	g := graph.BuildMemory([]ancora.Observation{
		{
			ID: 10, Title: "Workspace and org note", Content: "scope note",
			Type: "architecture", Workspace: "vela", Visibility: "work",
			References: `[{"type":"workspace","target":"operations"},` +
				`{"type":"organization","target":"glim"}]`,
			CreatedAt: now, UpdatedAt: now,
		},
	}, graph.MemoryOptions{})

	obsID := graph.MemoryObservationID(10)
	got := map[string]types.Edge{}
	for _, e := range edgesFrom(g.Edges, obsID) {
		got[e.Target] = e
	}

	workspaceEdge, ok := got[graph.MemoryWorkspaceID("operations")]
	if !ok {
		t.Fatalf("missing workspace reference edge")
	}
	if workspaceEdge.Metadata["cross_layer"] != nil {
		t.Fatalf("workspace reference unexpectedly marked cross-layer: %#v", workspaceEdge.Metadata)
	}
	if workspaceEdge.Metadata["layer"] != string(types.LayerMemory) {
		t.Fatalf("workspace reference layer = %v, want memory", workspaceEdge.Metadata["layer"])
	}

	orgEdge, ok := got[graph.MemoryOrganizationID("glim")]
	if !ok {
		t.Fatalf("missing organization reference edge")
	}
	if orgEdge.Metadata["cross_layer"] != nil {
		t.Fatalf("organization reference unexpectedly marked cross-layer: %#v", orgEdge.Metadata)
	}
}

func TestBuildMemory_ConceptNodeEmitted(t *testing.T) {
	g := graph.BuildMemory(sampleObs(), graph.MemoryOptions{})
	cid := graph.MemoryConceptID("auth")
	if _, ok := nodeByID(g.Nodes, cid); !ok {
		t.Fatalf("concept node %q not emitted", cid)
	}
	// And the defines edge from obs2 exists.
	obs2 := graph.MemoryObservationID(2)
	var found bool
	for _, e := range edgesFrom(g.Edges, obs2) {
		if e.Target == cid && e.Relation == graph.MemoryRelDefines {
			found = true
		}
	}
	if !found {
		t.Errorf("expected defines edge obs2 -> %s", cid)
	}
}

func TestResolveMemoryReference_WorkspaceAndOrganization(t *testing.T) {
	t.Parallel()

	repoRef, ok := graph.ResolveMemoryReference("workspace", "billing", "architecture", graph.MemoryOptions{})
	if !ok {
		t.Fatal("workspace reference did not resolve")
	}
	if repoRef.Target != graph.RefWorkspaceRepo+"billing" {
		t.Fatalf("workspace target = %q, want %q", repoRef.Target, graph.RefWorkspaceRepo+"billing")
	}
	if repoRef.Relation != graph.MemoryRelMentions {
		t.Fatalf("workspace relation = %q, want %q", repoRef.Relation, graph.MemoryRelMentions)
	}
	if repoRef.Verification != types.VerificationAmbiguous {
		t.Fatalf("workspace verification = %q, want %q", repoRef.Verification, types.VerificationAmbiguous)
	}

	orgRef, ok := graph.ResolveMemoryReference("organization", "glim", "architecture", graph.MemoryOptions{})
	if !ok {
		t.Fatal("organization reference did not resolve")
	}
	if orgRef.Target != graph.MemoryOrganizationID("glim") {
		t.Fatalf("organization target = %q, want %q", orgRef.Target, graph.MemoryOrganizationID("glim"))
	}
	if orgRef.Relation != graph.MemoryRelBelongsTo {
		t.Fatalf("organization relation = %q, want %q", orgRef.Relation, graph.MemoryRelBelongsTo)
	}
	if orgRef.CrossLayer {
		t.Fatal("organization reference should stay in memory layer")
	}
}

func TestBuildMemory_IsDeterministic(t *testing.T) {
	a := graph.BuildMemory(sampleObs(), graph.MemoryOptions{KnownRepos: []string{"vela"}})
	b := graph.BuildMemory(sampleObs(), graph.MemoryOptions{KnownRepos: []string{"vela"}})
	if len(a.Nodes) != len(b.Nodes) || len(a.Edges) != len(b.Edges) {
		t.Fatalf("non-deterministic size: a=(%d,%d) b=(%d,%d)",
			len(a.Nodes), len(a.Edges), len(b.Nodes), len(b.Edges))
	}
	for i := range a.Nodes {
		if a.Nodes[i].ID != b.Nodes[i].ID {
			t.Errorf("node order differs at %d: %q vs %q", i, a.Nodes[i].ID, b.Nodes[i].ID)
		}
	}
	for i := range a.Edges {
		if a.Edges[i].Source != b.Edges[i].Source ||
			a.Edges[i].Target != b.Edges[i].Target ||
			a.Edges[i].Relation != b.Edges[i].Relation {
			t.Errorf("edge order differs at %d", i)
		}
	}
}

func TestMergeMemory_NamespacesDoNotCollide(t *testing.T) {
	repoNodes := []types.Node{
		{ID: "project:vela", NodeType: string(types.NodeTypeProject)},
		{ID: "vela:file:internal/auth/auth.go", NodeType: string(types.NodeTypeFile)},
	}
	repoEdges := []types.Edge{
		{Source: "project:vela", Target: "vela:file:internal/auth/auth.go", Relation: "contains"},
	}
	mem := graph.BuildMemory(sampleObs(), graph.MemoryOptions{KnownRepos: []string{"vela"}})

	mergedNodes, mergedEdges := graph.MergeMemory(repoNodes, repoEdges, mem.Nodes, mem.Edges)

	// Every repo node survives untouched.
	for _, rn := range repoNodes {
		n, ok := nodeByID(mergedNodes, rn.ID)
		if !ok {
			t.Errorf("merge dropped repo node %q", rn.ID)
			continue
		}
		if n.NodeType != rn.NodeType {
			t.Errorf("merge mutated repo node %q type %q -> %q", rn.ID, rn.NodeType, n.NodeType)
		}
	}
	// Memory nodes are present but only in the memory namespace.
	memCount := 0
	for _, n := range mergedNodes {
		if strings.HasPrefix(n.ID, "memory:") {
			memCount++
		}
	}
	if memCount == 0 {
		t.Error("no memory nodes present in merged graph")
	}
	// And all originally-distinct edges preserved.
	if len(mergedEdges) < len(repoEdges) {
		t.Errorf("merged edges = %d, want >= %d repo edges", len(mergedEdges), len(repoEdges))
	}
}

func TestMergeMemory_IsIdempotent(t *testing.T) {
	mem := graph.BuildMemory(sampleObs(), graph.MemoryOptions{})
	n1, e1 := graph.MergeMemory(nil, nil, mem.Nodes, mem.Edges)
	n2, e2 := graph.MergeMemory(n1, e1, mem.Nodes, mem.Edges)
	if len(n1) != len(n2) || len(e1) != len(e2) {
		t.Errorf("MergeMemory not idempotent: n=(%d->%d) e=(%d->%d)", len(n1), len(n2), len(e1), len(e2))
	}
}

func TestBuildMemory_EdgesStampedWithLayer(t *testing.T) {
	g := graph.BuildMemory(sampleObs(), graph.MemoryOptions{KnownRepos: []string{"vela"}})
	for _, e := range g.Edges {
		if got := e.Metadata["layer"]; got != string(types.LayerMemory) {
			t.Errorf("edge %s->%s (%s) missing memory layer stamp: %v",
				e.Source, e.Target, e.Relation, got)
		}
		if e.Metadata["evidence_confidence"] == nil {
			t.Errorf("edge %s->%s missing evidence_confidence", e.Source, e.Target)
		}
	}
}

func TestMergeMemory_BindsCurrentFileReferenceToLiveNode(t *testing.T) {
	mem := graph.BuildMemory(sampleObs(), graph.MemoryOptions{})
	repoNodes := []types.Node{{
		ID:         "vela:file:internal/auth/auth.go",
		Label:      "internal/auth/auth.go",
		NodeType:   string(types.NodeTypeFile),
		SourceFile: "internal/auth/auth.go",
		Source:     &types.Source{Name: "vela", Type: types.SourceTypeCodebase},
	}}

	_, mergedEdges := graph.MergeMemory(repoNodes, nil, mem.Nodes, mem.Edges)
	for _, edge := range mergedEdges {
		if edge.Source != graph.MemoryObservationID(1) || edge.Relation != graph.MemoryRelConstrains {
			continue
		}
		if ref, _ := edge.Metadata["reference_target"].(string); ref != graph.RefRepoFile+"internal/auth/auth.go" {
			continue
		}
		if edge.Target != "vela:file:internal/auth/auth.go" {
			t.Fatalf("bound target = %q, want live file node", edge.Target)
		}
		if edge.Metadata["binding_state"] != string(types.VerificationCurrent) {
			t.Fatalf("binding_state = %v, want current", edge.Metadata["binding_state"])
		}
		return
	}
	t.Fatal("expected bound current file reference edge")
}

func TestMergeMemory_RedirectsUniqueFileRename(t *testing.T) {
	now := time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC)
	mem := graph.BuildMemory([]ancora.Observation{{
		ID: 11, Title: "Renamed auth file", Content: "moved auth implementation",
		Type: "architecture", Workspace: "vela", Visibility: "work",
		References: `[{"type":"file","target":"internal/legacy/auth.go"}]`,
		CreatedAt:  now, UpdatedAt: now,
	}}, graph.MemoryOptions{})
	repoNodes := []types.Node{{
		ID:         "vela:file:internal/auth/auth.go",
		Label:      "internal/auth/auth.go",
		NodeType:   string(types.NodeTypeFile),
		SourceFile: "internal/auth/auth.go",
		Source:     &types.Source{Name: "vela", Type: types.SourceTypeCodebase},
	}}

	_, mergedEdges := graph.MergeMemory(repoNodes, nil, mem.Nodes, mem.Edges)
	for _, edge := range mergedEdges {
		if edge.Source != graph.MemoryObservationID(11) {
			continue
		}
		if ref, _ := edge.Metadata["reference_target"].(string); ref != graph.RefRepoFile+"internal/legacy/auth.go" {
			continue
		}
		if edge.Metadata["binding_state"] != string(types.VerificationRedirected) {
			t.Fatalf("binding_state = %v, want redirected", edge.Metadata["binding_state"])
		}
		if edge.Target != "vela:file:internal/auth/auth.go" {
			t.Fatalf("redirect target = %q, want live file node", edge.Target)
		}
		if edge.Metadata["binding_evidence"] != "unique basename match" {
			t.Fatalf("binding_evidence = %v", edge.Metadata["binding_evidence"])
		}
		return
	}
	t.Fatal("expected redirected file reference edge")
}

func TestMergeMemory_LeavesAmbiguousRebindAsSuggestion(t *testing.T) {
	now := time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC)
	mem := graph.BuildMemory([]ancora.Observation{{
		ID: 12, Title: "Config move", Content: "config moved somewhere",
		Type: "bugfix", Workspace: "vela", Visibility: "work",
		References: `[{"type":"file","target":"legacy/config.go"}]`,
		CreatedAt:  now, UpdatedAt: now,
	}}, graph.MemoryOptions{})
	repoNodes := []types.Node{
		{ID: "vela:file:internal/config.go", Label: "internal/config.go", NodeType: string(types.NodeTypeFile), SourceFile: "internal/config.go", Source: &types.Source{Name: "vela", Type: types.SourceTypeCodebase}},
		{ID: "vela:file:pkg/config.go", Label: "pkg/config.go", NodeType: string(types.NodeTypeFile), SourceFile: "pkg/config.go", Source: &types.Source{Name: "vela", Type: types.SourceTypeCodebase}},
	}

	_, mergedEdges := graph.MergeMemory(repoNodes, nil, mem.Nodes, mem.Edges)
	for _, edge := range mergedEdges {
		if edge.Source != graph.MemoryObservationID(12) {
			continue
		}
		if ref, _ := edge.Metadata["reference_target"].(string); ref != graph.RefRepoFile+"legacy/config.go" {
			continue
		}
		if edge.Metadata["binding_state"] != string(types.VerificationAmbiguous) {
			t.Fatalf("binding_state = %v, want ambiguous", edge.Metadata["binding_state"])
		}
		if edge.Target != graph.RefRepoFile+"legacy/config.go" {
			t.Fatalf("ambiguous target = %q, want original canonical target", edge.Target)
		}
		suggestions, ok := edge.Metadata["binding_suggestions"].([]string)
		if !ok {
			t.Fatalf("binding_suggestions type = %T, want []string", edge.Metadata["binding_suggestions"])
		}
		if len(suggestions) != 2 {
			t.Fatalf("len(binding_suggestions) = %d, want 2", len(suggestions))
		}
		return
	}
	t.Fatal("expected ambiguous file reference edge")
}

func TestMergeMemory_MarksMissingTargetStale(t *testing.T) {
	now := time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC)
	mem := graph.BuildMemory([]ancora.Observation{{
		ID: 13, Title: "Deleted file", Content: "file removed",
		Type: "bugfix", Workspace: "vela", Visibility: "work",
		References: `[{"type":"file","target":"internal/removed/dead.go"}]`,
		CreatedAt:  now, UpdatedAt: now,
	}}, graph.MemoryOptions{})

	_, mergedEdges := graph.MergeMemory(nil, nil, mem.Nodes, mem.Edges)
	for _, edge := range mergedEdges {
		if edge.Source != graph.MemoryObservationID(13) {
			continue
		}
		if ref, _ := edge.Metadata["reference_target"].(string); ref != graph.RefRepoFile+"internal/removed/dead.go" {
			continue
		}
		if edge.Metadata["binding_state"] != string(types.VerificationStale) {
			t.Fatalf("binding_state = %v, want stale", edge.Metadata["binding_state"])
		}
		if _, ok := edge.Metadata["bound_target"]; ok {
			t.Fatal("stale edge should not have bound_target")
		}
		return
	}
	t.Fatal("expected stale file reference edge")
}
