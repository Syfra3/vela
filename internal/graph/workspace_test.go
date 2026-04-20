package graph

import (
	"testing"

	"github.com/Syfra3/vela/pkg/types"
)

func projectNode(name string) types.Node {
	return types.Node{
		ID:       "project:" + name,
		Label:    name,
		NodeType: string(types.NodeTypeProject),
		Source:   &types.Source{Type: types.SourceTypeCodebase, Name: name},
	}
}

func serviceNode(name string) types.Node {
	return types.Node{
		ID:       "contract:service:" + name,
		Label:    name,
		NodeType: string(types.NodeTypeService),
	}
}

func declaredInEdge(service, project string) types.Edge {
	return types.Edge{
		Source:     "contract:service:" + service,
		Target:     "project:" + project,
		Relation:   "declared_in",
		Confidence: "DECLARED",
		Metadata: map[string]interface{}{
			"layer":               string(types.LayerContract),
			"evidence_confidence": string(types.ConfidenceDeclared),
		},
	}
}

// BuildWorkspace turns project nodes into repo nodes and contract services
// into workspace services, wired via declared_in bindings.
func TestBuildWorkspace_DerivesReposAndServices(t *testing.T) {
	nodes := []types.Node{
		projectNode("billing-api"),
		projectNode("auth-api"),
		serviceNode("billing"),
		serviceNode("auth"),
	}
	edges := []types.Edge{
		declaredInEdge("billing", "billing-api"),
		declaredInEdge("auth", "auth-api"),
	}

	w := BuildWorkspace(nodes, edges, nil)

	if got := w.Repos(); len(got) != 2 {
		t.Fatalf("Repos = %v, want 2 entries", got)
	}
	if got := w.ReposExposing("billing"); len(got) != 1 || got[0] != "billing-api" {
		t.Errorf("ReposExposing(billing) = %v, want [billing-api]", got)
	}

	// Every workspace node/edge must be stamped with layer=workspace so the
	// orchestrator can distinguish routing truth from code/contract truth.
	for _, n := range w.Nodes {
		if n.Metadata["layer"] != string(types.LayerWorkspace) {
			t.Errorf("node %s missing workspace layer stamp: %v", n.ID, n.Metadata)
		}
	}
	for _, e := range w.Edges {
		if e.Metadata["layer"] != string(types.LayerWorkspace) {
			t.Errorf("edge %s->%s missing workspace layer stamp: %v", e.Source, e.Target, e.Metadata)
		}
	}
}

// Overrides carry domain/package/dependency routing hints. Domain and package
// nodes must appear and their repo→tag edges use the correct relations.
func TestBuildWorkspace_OverridesWireRoutingMetadata(t *testing.T) {
	nodes := []types.Node{
		projectNode("billing-api"),
		projectNode("shared-lib"),
	}
	overrides := map[string]RepoOverrides{
		"billing-api": {
			Domains:      []string{"billing"},
			Packages:     []string{"github.com/acme/billing"},
			Dependencies: []string{"shared-lib"},
		},
	}

	w := BuildWorkspace(nodes, nil, overrides)

	var sawDomain, sawPackage, sawDep bool
	for _, e := range w.Edges {
		switch e.Relation {
		case WorkspaceRelOwns:
			if e.Source == WorkspaceRepoID("billing-api") && e.Target == WorkspaceDomainID("billing") {
				sawDomain = true
			}
		case WorkspaceRelHosts:
			if e.Source == WorkspaceRepoID("billing-api") && e.Target == WorkspacePackageID("github.com/acme/billing") {
				sawPackage = true
			}
		case WorkspaceRelDependsOn:
			if e.Source == WorkspaceRepoID("billing-api") && e.Target == WorkspaceRepoID("shared-lib") {
				sawDep = true
			}
		}
	}
	if !sawDomain {
		t.Error("missing repo→domain owns edge")
	}
	if !sawPackage {
		t.Error("missing repo→package hosts edge")
	}
	if !sawDep {
		t.Error("missing repo→repo depends_on edge")
	}
}

// Dependencies pointing to unknown repos are dropped rather than synthesised
// into ghost repo nodes. Workspace truth ties to real project nodes.
func TestBuildWorkspace_DropsUnknownDependencyTargets(t *testing.T) {
	nodes := []types.Node{projectNode("billing-api")}
	overrides := map[string]RepoOverrides{
		"billing-api": {Dependencies: []string{"ghost-repo"}},
	}
	w := BuildWorkspace(nodes, nil, overrides)
	for _, e := range w.Edges {
		if e.Relation == WorkspaceRelDependsOn {
			t.Errorf("unexpected depends_on edge to unknown repo: %+v", e)
		}
	}
	if got := w.Repos(); len(got) != 1 || got[0] != "billing-api" {
		t.Errorf("Repos = %v, want [billing-api]", got)
	}
}

// SelectRepos must return the right repos with score-bearing reasons when
// query tokens match service, domain, package, or name.
func TestSelectRepos_RoutesByServiceDomainAndName(t *testing.T) {
	nodes := []types.Node{
		projectNode("billing-api"),
		projectNode("auth-api"),
		projectNode("docs-site"),
		serviceNode("billing"),
		serviceNode("auth"),
	}
	edges := []types.Edge{
		declaredInEdge("billing", "billing-api"),
		declaredInEdge("auth", "auth-api"),
	}
	overrides := map[string]RepoOverrides{
		"billing-api": {Domains: []string{"billing"}, Packages: []string{"invoice"}},
		"auth-api":    {Domains: []string{"auth"}},
	}
	w := BuildWorkspace(nodes, edges, overrides)

	// Token "billing" matches repo name, declared service, and domain — the
	// repo should rank above others and surface all three reasons.
	hits := w.SelectRepos([]string{"billing"}, 3)
	if len(hits) == 0 {
		t.Fatal("no hits for token 'billing'")
	}
	if hits[0].Repo != "billing-api" {
		t.Fatalf("top hit = %q, want billing-api", hits[0].Repo)
	}
	reasons := map[string]bool{}
	for _, r := range hits[0].Reasons {
		reasons[r] = true
	}
	for _, want := range []string{"name:billing-api", "service:billing", "domain:billing"} {
		if !reasons[want] {
			t.Errorf("missing reason %q in %v", want, hits[0].Reasons)
		}
	}

	// Token "auth" should select only auth-api, not billing or docs.
	authHits := w.SelectRepos([]string{"auth"}, 3)
	if len(authHits) != 1 || authHits[0].Repo != "auth-api" {
		t.Fatalf("auth hits = %+v, want single auth-api", authHits)
	}

	// Token with no matches returns nothing.
	if got := w.SelectRepos([]string{"nonsense-xyz"}, 3); len(got) != 0 {
		t.Errorf("SelectRepos(nonsense) = %v, want empty", got)
	}
}

// Re-running BuildWorkspace after a refresh with the same inputs must produce
// the same node/edge set — workspace derivation has to be idempotent so repo
// refreshes do not churn routing truth.
func TestBuildWorkspace_IdempotentOnRefresh(t *testing.T) {
	nodes := []types.Node{
		projectNode("billing-api"),
		serviceNode("billing"),
	}
	edges := []types.Edge{declaredInEdge("billing", "billing-api")}
	overrides := map[string]RepoOverrides{
		"billing-api": {Domains: []string{"billing"}},
	}

	a := BuildWorkspace(nodes, edges, overrides)
	b := BuildWorkspace(nodes, edges, overrides)

	if len(a.Nodes) != len(b.Nodes) || len(a.Edges) != len(b.Edges) {
		t.Fatalf("re-run mismatch: nodes %d vs %d, edges %d vs %d",
			len(a.Nodes), len(b.Nodes), len(a.Edges), len(b.Edges))
	}
	for i := range a.Nodes {
		if a.Nodes[i].ID != b.Nodes[i].ID {
			t.Errorf("node[%d] id mismatch %q vs %q", i, a.Nodes[i].ID, b.Nodes[i].ID)
		}
	}
	for i := range a.Edges {
		if a.Edges[i].Source != b.Edges[i].Source || a.Edges[i].Target != b.Edges[i].Target || a.Edges[i].Relation != b.Edges[i].Relation {
			t.Errorf("edge[%d] mismatch %+v vs %+v", i, a.Edges[i], b.Edges[i])
		}
	}
}

// Workspace nodes must not overwrite or reuse repo/contract IDs. Their
// identity lives in the "workspace:" namespace.
func TestBuildWorkspace_IdentityNamespaceDisjoint(t *testing.T) {
	nodes := []types.Node{
		projectNode("billing-api"),
		serviceNode("billing"),
	}
	edges := []types.Edge{declaredInEdge("billing", "billing-api")}
	w := BuildWorkspace(nodes, edges, nil)

	for _, n := range w.Nodes {
		if n.ID == "" {
			t.Error("empty workspace node ID")
		}
		if n.ID[:10] != "workspace:" {
			t.Errorf("workspace node ID %q not in workspace: namespace", n.ID)
		}
	}
}
