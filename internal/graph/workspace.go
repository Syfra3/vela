package graph

import (
	"sort"
	"strings"

	"github.com/Syfra3/vela/pkg/types"
)

// Workspace-layer graph.
//
// The workspace graph is deliberately *not* a deep code graph. It is the
// lightweight routing layer that answers "which repos should we search?"
// before repo-local retrieval runs. It lives alongside — never inside — the
// repo graph, and its identity is kept in its own "workspace:" namespace so
// nothing in the repo layer can accidentally overwrite workspace truth.
//
// Derivation is intentionally shallow and idempotent: BuildWorkspace reads
// the merged repo+contract graph and emits a small set of routing nodes and
// edges. Re-running it after a repo refresh is the canonical refresh path —
// no incremental state is kept.

const (
	// Workspace edge relations.
	WorkspaceRelExposes   = "exposes"    // repo -> service
	WorkspaceRelHosts     = "hosts"      // repo -> package
	WorkspaceRelOwns      = "owns"       // repo -> domain
	WorkspaceRelDependsOn = "depends_on" // repo -> repo
)

// WorkspaceRepoID returns the canonical workspace ID for a repo.
func WorkspaceRepoID(name string) string { return "workspace:repo:" + strings.ToLower(name) }

// WorkspaceServiceID returns the canonical workspace ID for a service. The
// identity mirrors the contract-layer service so routing and contract views
// can share identity without a cross-layer join.
func WorkspaceServiceID(name string) string { return "workspace:service:" + strings.ToLower(name) }

// WorkspacePackageID returns the canonical workspace ID for a package.
func WorkspacePackageID(name string) string { return "workspace:package:" + strings.ToLower(name) }

// WorkspaceDomainID returns the canonical workspace ID for a domain tag.
func WorkspaceDomainID(name string) string { return "workspace:domain:" + strings.ToLower(name) }

// RepoOverrides carries declarative routing hints the caller wants to stamp
// onto a repo that cannot be derived from the merged graph alone (domain
// tags from repo config, declared dependencies from manifests, language
// packages, etc.). All fields are optional.
type RepoOverrides struct {
	Domains      []string
	Packages     []string
	Dependencies []string // other repo names this repo depends on
}

// RepoRouteHit is one ranked repo candidate returned by SelectRepos. Reasons
// explains why the repo matched so the orchestrator can surface provenance.
type RepoRouteHit struct {
	Repo    string
	Score   float64
	Reasons []string
}

// WorkspaceGraph is the routing-only derived graph. It exposes its nodes and
// edges for persistence alongside the repo+contract graph, plus the indexes
// needed for SelectRepos.
type WorkspaceGraph struct {
	Nodes []types.Node
	Edges []types.Edge

	// repos keyed by lower-case repo name.
	repos map[string]*workspaceRepo
	// reverse lookups: lower-case key -> set of repo names.
	services map[string]map[string]bool
	packages map[string]map[string]bool
	domains  map[string]map[string]bool
}

type workspaceRepo struct {
	Name         string // display name (preserves original casing)
	Services     []string
	Packages     []string
	Domains      []string
	Dependencies []string
}

// LoadWorkspace rebuilds routing indexes from persisted workspace-layer nodes
// and edges already present in a merged graph.
func LoadWorkspace(nodes []types.Node, edges []types.Edge) *WorkspaceGraph {
	w := &WorkspaceGraph{
		repos:    map[string]*workspaceRepo{},
		services: map[string]map[string]bool{},
		packages: map[string]map[string]bool{},
		domains:  map[string]map[string]bool{},
	}

	nodeByID := make(map[string]types.Node)
	for _, node := range nodes {
		key := types.CanonicalKeyForNode(node)
		if key.Layer != types.LayerWorkspace {
			continue
		}
		nodeByID[node.ID] = node
		w.Nodes = append(w.Nodes, node)
		if key.Kind != "repo" {
			continue
		}
		name := strings.TrimSpace(node.Label)
		if name == "" {
			name = key.Key
		}
		if name == "" {
			continue
		}
		lookup := strings.ToLower(name)
		if _, ok := w.repos[lookup]; !ok {
			w.repos[lookup] = &workspaceRepo{Name: name}
		}
	}

	for _, edge := range edges {
		if edge.Metadata["layer"] != string(types.LayerWorkspace) {
			continue
		}
		w.Edges = append(w.Edges, edge)
		src, ok := nodeByID[edge.Source]
		if !ok {
			continue
		}
		srcKey := types.CanonicalKeyForNode(src)
		if srcKey.Layer != types.LayerWorkspace || srcKey.Kind != "repo" {
			continue
		}
		repo := w.repos[strings.ToLower(src.Label)]
		if repo == nil {
			continue
		}
		tgt, ok := nodeByID[edge.Target]
		if !ok {
			continue
		}
		tgtKey := types.CanonicalKeyForNode(tgt)
		switch edge.Relation {
		case WorkspaceRelExposes:
			if tgtKey.Kind == "service" && !contains(repo.Services, tgt.Label) {
				repo.Services = append(repo.Services, tgt.Label)
				w.services[strings.ToLower(tgt.Label)] = addSet(w.services[strings.ToLower(tgt.Label)], repo.Name)
			}
		case WorkspaceRelHosts:
			if tgtKey.Kind == "package" && !contains(repo.Packages, tgt.Label) {
				repo.Packages = append(repo.Packages, tgt.Label)
				w.packages[strings.ToLower(tgt.Label)] = addSet(w.packages[strings.ToLower(tgt.Label)], repo.Name)
			}
		case WorkspaceRelOwns:
			if tgtKey.Kind == "domain" && !contains(repo.Domains, tgt.Label) {
				repo.Domains = append(repo.Domains, tgt.Label)
				w.domains[strings.ToLower(tgt.Label)] = addSet(w.domains[strings.ToLower(tgt.Label)], repo.Name)
			}
		case WorkspaceRelDependsOn:
			if tgtKey.Kind == "repo" && !contains(repo.Dependencies, tgt.Label) {
				repo.Dependencies = append(repo.Dependencies, tgt.Label)
			}
		}
	}

	return w
}

// BuildWorkspace derives a workspace graph from the merged repo+contract
// graph plus optional per-repo overrides. Inputs follow the same shape that
// MergeContract produces.
//
// Derivation rules (lightweight by design):
//   - one workspace:repo node per project node
//   - one workspace:service node per contract:service node
//   - repo -> service ("exposes") edges inferred from contract "declared_in"
//     edges (declared evidence flows through unchanged)
//   - repo -> package ("hosts"), repo -> domain ("owns"), repo -> repo
//     ("depends_on") edges from the caller-supplied overrides
//
// BuildWorkspace is idempotent: running it twice on the same inputs produces
// the same output. It makes no attempt to mine code structure — packages,
// domains, and dependencies must come from declared sources (overrides) to
// keep routing trustworthy.
func BuildWorkspace(nodes []types.Node, edges []types.Edge, overrides map[string]RepoOverrides) *WorkspaceGraph {
	w := &WorkspaceGraph{
		repos:    map[string]*workspaceRepo{},
		services: map[string]map[string]bool{},
		packages: map[string]map[string]bool{},
		domains:  map[string]map[string]bool{},
	}

	// Pass 1: project nodes become repos.
	repoDisplay := map[string]string{} // repo id -> display name
	for _, n := range nodes {
		if n.NodeType != string(types.NodeTypeProject) {
			continue
		}
		name := repoDisplayName(n)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := w.repos[key]; ok {
			continue
		}
		w.repos[key] = &workspaceRepo{Name: name}
		repoDisplay[n.ID] = name
	}

	// Pass 2: contract services become workspace services. Track id -> name.
	services := map[string]string{} // contract service node id -> display name
	for _, n := range nodes {
		if n.NodeType != string(types.NodeTypeService) {
			continue
		}
		if !strings.HasPrefix(n.ID, "contract:service:") {
			continue
		}
		label := n.Label
		if strings.TrimSpace(label) == "" {
			label = strings.TrimPrefix(n.ID, "contract:service:")
		}
		services[n.ID] = label
	}

	// Pass 3: declared_in edges bind services to repos.
	for _, e := range edges {
		if e.Relation != "declared_in" {
			continue
		}
		svcName, okSvc := services[e.Source]
		if !okSvc {
			continue
		}
		repoName, okRepo := repoDisplay[e.Target]
		if !okRepo {
			continue
		}
		repo := w.repos[strings.ToLower(repoName)]
		if repo == nil {
			continue
		}
		if !contains(repo.Services, svcName) {
			repo.Services = append(repo.Services, svcName)
		}
	}

	// Pass 4: overrides. Overrides are authoritative for package/domain/dep
	// routing because these cannot be safely inferred from code alone.
	for rawName, ov := range overrides {
		repo := w.repos[strings.ToLower(rawName)]
		if repo == nil {
			// Overrides for a repo we don't know about are ignored rather than
			// synthesizing a ghost repo — routing truth must tie to a real
			// project node.
			continue
		}
		for _, d := range ov.Domains {
			if d = strings.TrimSpace(d); d != "" && !contains(repo.Domains, d) {
				repo.Domains = append(repo.Domains, d)
			}
		}
		for _, p := range ov.Packages {
			if p = strings.TrimSpace(p); p != "" && !contains(repo.Packages, p) {
				repo.Packages = append(repo.Packages, p)
			}
		}
		for _, d := range ov.Dependencies {
			if d = strings.TrimSpace(d); d != "" && !contains(repo.Dependencies, d) {
				repo.Dependencies = append(repo.Dependencies, d)
			}
		}
	}

	// Emit nodes + edges in a deterministic order.
	w.materialize()
	return w
}

func (w *WorkspaceGraph) materialize() {
	repoNames := make([]string, 0, len(w.repos))
	for k := range w.repos {
		repoNames = append(repoNames, k)
	}
	sort.Strings(repoNames)

	seenService := map[string]bool{}
	seenPackage := map[string]bool{}
	seenDomain := map[string]bool{}

	for _, k := range repoNames {
		repo := w.repos[k]
		repoNode := types.Node{
			ID:       WorkspaceRepoID(repo.Name),
			Label:    repo.Name,
			NodeType: string(types.NodeTypeRepo),
		}
		stampWorkspaceNode(&repoNode)
		w.Nodes = append(w.Nodes, repoNode)

		sort.Strings(repo.Services)
		for _, svc := range repo.Services {
			id := WorkspaceServiceID(svc)
			if !seenService[id] {
				seenService[id] = true
				node := types.Node{ID: id, Label: svc, NodeType: string(types.NodeTypeService)}
				stampWorkspaceNode(&node)
				w.Nodes = append(w.Nodes, node)
			}
			w.Edges = append(w.Edges, workspaceEdge(repoNode.ID, id, WorkspaceRelExposes))
			w.services[strings.ToLower(svc)] = addSet(w.services[strings.ToLower(svc)], repo.Name)
		}

		sort.Strings(repo.Packages)
		for _, pkg := range repo.Packages {
			id := WorkspacePackageID(pkg)
			if !seenPackage[id] {
				seenPackage[id] = true
				node := types.Node{ID: id, Label: pkg, NodeType: string(types.NodeTypePackage)}
				stampWorkspaceNode(&node)
				w.Nodes = append(w.Nodes, node)
			}
			w.Edges = append(w.Edges, workspaceEdge(repoNode.ID, id, WorkspaceRelHosts))
			w.packages[strings.ToLower(pkg)] = addSet(w.packages[strings.ToLower(pkg)], repo.Name)
		}

		sort.Strings(repo.Domains)
		for _, dom := range repo.Domains {
			id := WorkspaceDomainID(dom)
			if !seenDomain[id] {
				seenDomain[id] = true
				node := types.Node{ID: id, Label: dom, NodeType: string(types.NodeTypeDomain)}
				stampWorkspaceNode(&node)
				w.Nodes = append(w.Nodes, node)
			}
			w.Edges = append(w.Edges, workspaceEdge(repoNode.ID, id, WorkspaceRelOwns))
			w.domains[strings.ToLower(dom)] = addSet(w.domains[strings.ToLower(dom)], repo.Name)
		}

		sort.Strings(repo.Dependencies)
		for _, dep := range repo.Dependencies {
			depKey := strings.ToLower(dep)
			depRepo, ok := w.repos[depKey]
			if !ok {
				continue // unknown dependency target — drop rather than invent
			}
			w.Edges = append(w.Edges, workspaceEdge(repoNode.ID, WorkspaceRepoID(depRepo.Name), WorkspaceRelDependsOn))
		}
	}
}

// SelectRepos ranks repos against a set of query tokens. Matching is
// case-insensitive and token-wise. The returned slice is sorted by descending
// score; limit<=0 returns all non-zero hits.
//
// Scoring is intentionally coarse — the workspace layer is a prefilter, not
// a ranker. Declared evidence (service name matches) outranks derived hints
// (domain/package tags) so contract truth flows through routing.
func (w *WorkspaceGraph) SelectRepos(tokens []string, limit int) []RepoRouteHit {
	if len(w.repos) == 0 {
		return nil
	}

	norm := make([]string, 0, len(tokens))
	for _, t := range tokens {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" {
			norm = append(norm, t)
		}
	}
	if len(norm) == 0 {
		return nil
	}

	type scored struct {
		hit   RepoRouteHit
		order int
	}
	var hits []scored
	order := 0

	for _, key := range sortedKeys(w.repos) {
		repo := w.repos[key]
		var score float64
		var reasons []string

		for _, tok := range norm {
			if strings.Contains(strings.ToLower(repo.Name), tok) {
				score += 3.0
				reasons = append(reasons, "name:"+repo.Name)
			}
			for _, svc := range repo.Services {
				if strings.Contains(strings.ToLower(svc), tok) {
					score += 2.5
					reasons = append(reasons, "service:"+svc)
				}
			}
			for _, dom := range repo.Domains {
				if strings.Contains(strings.ToLower(dom), tok) {
					score += 1.5
					reasons = append(reasons, "domain:"+dom)
				}
			}
			for _, pkg := range repo.Packages {
				if strings.Contains(strings.ToLower(pkg), tok) {
					score += 1.0
					reasons = append(reasons, "package:"+pkg)
				}
			}
			for _, dep := range repo.Dependencies {
				if strings.Contains(strings.ToLower(dep), tok) {
					score += 0.5
					reasons = append(reasons, "depends_on:"+dep)
				}
			}
		}

		if score > 0 {
			hits = append(hits, scored{
				hit:   RepoRouteHit{Repo: repo.Name, Score: score, Reasons: dedupeStrings(reasons)},
				order: order,
			})
		}
		order++
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].hit.Score != hits[j].hit.Score {
			return hits[i].hit.Score > hits[j].hit.Score
		}
		return hits[i].order < hits[j].order
	})

	out := make([]RepoRouteHit, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.hit)
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Repos returns the repo names known to the workspace graph, sorted.
func (w *WorkspaceGraph) Repos() []string {
	out := make([]string, 0, len(w.repos))
	for _, k := range sortedKeys(w.repos) {
		out = append(out, w.repos[k].Name)
	}
	return out
}

// ReposExposing returns the repo names that expose a given service.
func (w *WorkspaceGraph) ReposExposing(service string) []string {
	return setToSorted(w.services[strings.ToLower(service)])
}

// ─── helpers ──────────────────────────────────────────────────────────────

func repoDisplayName(n types.Node) string {
	if n.Source != nil && n.Source.Name != "" {
		return n.Source.Name
	}
	if n.Label != "" {
		return n.Label
	}
	return strings.TrimPrefix(n.ID, "project:")
}

func stampWorkspaceNode(n *types.Node) {
	if n.Metadata == nil {
		n.Metadata = map[string]interface{}{}
	}
	n.Metadata["layer"] = string(types.LayerWorkspace)
	n.Metadata["evidence_type"] = "routing"
	n.Metadata["evidence_confidence"] = string(types.ConfidenceExtracted)
}

func workspaceEdge(src, tgt, rel string) types.Edge {
	e := types.Edge{
		Source:     src,
		Target:     tgt,
		Relation:   rel,
		Confidence: "EXTRACTED",
		Metadata: map[string]interface{}{
			"layer":               string(types.LayerWorkspace),
			"evidence_type":       "routing",
			"evidence_confidence": string(types.ConfidenceExtracted),
		},
	}
	return e
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

func addSet(s map[string]bool, v string) map[string]bool {
	if s == nil {
		s = map[string]bool{}
	}
	s[v] = true
	return s
}

func setToSorted(s map[string]bool) []string {
	out := make([]string, 0, len(s))
	for k := range s {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedKeys(m map[string]*workspaceRepo) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
