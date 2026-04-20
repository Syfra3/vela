package graph

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Syfra3/vela/internal/ancora"
	"github.com/Syfra3/vela/pkg/types"
)

// Memory-layer graph.
//
// Memory is historical truth — decisions, bugfixes, discoveries — sourced from
// Ancora observations. It is NOT a dump of code symbols. The memory layer
// lives alongside the repo, contract, and workspace graphs in its own
// "memory:" identity namespace so nothing in the repo layer can be silently
// overwritten by a memory entry, and nothing in memory invents code identity
// locally. Links to other layers are expressed as reference edges whose target
// carries a canonical "<layer>:<kind>:<key>" form so joins are explicit.
//
// Derivation is deterministic: re-running BuildMemory on the same observation
// set produces the same node/edge set, which is the canonical refresh path.

const (
	// MemoryRootID is the single root node for the memory layer.
	MemoryRootID = "memory:root:ancora"

	memoryObservationPrefix  = "memory:observation:"
	memoryWorkspacePrefix    = "memory:workspace:"
	memoryVisibilityPrefix   = "memory:visibility:"
	memoryOrganizationPrefix = "memory:organization:"
	memoryConceptPrefix      = "memory:concept:"

	// Cross-layer canonical-key target prefixes used on memory reference edges.
	// Targets in these namespaces do NOT get a corresponding memory-layer node —
	// they are pointers out of the memory graph.
	RefRepoFile      = "repo:file:"
	RefRepoSymbol    = "repo:symbol:"
	RefWorkspaceRepo = "workspace:repo:"
	RefMemoryObs     = "memory:observation:"
	RefMemoryConcept = "memory:concept:"
)

// Memory edge relations.
const (
	MemoryRelContains    = "contains"    // root -> workspace, workspace -> visibility
	MemoryRelBelongsTo   = "belongs_to"  // observation -> workspace/organization
	MemoryRelScopedTo    = "scoped_to"   // observation -> visibility
	MemoryRelMentions    = "mentions"    // observation -> workspace:repo (cross-layer)
	MemoryRelRelatedTo   = "related_to"  // observation -> observation
	MemoryRelDefines     = "defines"     // observation -> concept
	MemoryRelReferences  = "references"  // observation -> external artifact
	MemoryRelDocuments   = "documents"   // architecture/discovery obs -> code
	MemoryRelDecidesOn   = "decides_on"  // decision obs -> code
	MemoryRelConstrains  = "constrains"  // bugfix obs -> code
	MemoryRelExemplifies = "exemplifies" // pattern obs -> code
)

// Evidence type tags specific to the memory layer.
const (
	MemoryEvidenceObservation = "observation"
	MemoryEvidenceReference   = "observation-reference"
	MemoryEvidenceStructural  = "memory-structure"
)

// ID helpers for memory-layer nodes.
func MemoryObservationID(id int64) string  { return fmt.Sprintf("%s%d", memoryObservationPrefix, id) }
func MemoryWorkspaceID(name string) string { return memoryWorkspacePrefix + strings.ToLower(name) }
func MemoryVisibilityID(v string) string   { return memoryVisibilityPrefix + strings.ToLower(v) }
func MemoryOrganizationID(o string) string { return memoryOrganizationPrefix + strings.ToLower(o) }
func MemoryConceptID(c string) string      { return memoryConceptPrefix + strings.ToLower(c) }

// MemoryOptions carries optional resolution hints that let the memory graph
// point at other layers by canonical reference. Memory never synthesizes
// repo, contract, or workspace nodes — it only emits reference edges when a
// known target exists.
type MemoryOptions struct {
	// KnownRepos is the set of workspace-layer repo names the caller already
	// knows about. Case-insensitive. When an observation's workspace matches
	// one of these, a cross-layer "mentions" edge to workspace:repo:<name>
	// is emitted in addition to the memory-internal workspace binding.
	KnownRepos []string
}

// MemoryGraph is the derived memory-layer graph. Nodes and Edges are exposed
// for persistence alongside the repo + contract + workspace graphs.
type MemoryGraph struct {
	Nodes []types.Node
	Edges []types.Edge
}

// ResolvedMemoryReference is the canonicalized form of a raw observation
// reference. It keeps memory provenance explicit without folding memory into
// repo/workspace truth.
type ResolvedMemoryReference struct {
	Target       string
	Relation     string
	Confidence   types.Confidence
	Verification types.VerificationState
	CrossLayer   bool
}

// ancoraWireRef mirrors the JSON object stored in observations.references.
type ancoraWireRef struct {
	Type   string `json:"type"`
	Target string `json:"target"`
}

// BuildMemory materializes the memory-layer graph from a slice of Ancora
// observations. It is intentionally independent of the repo extraction path:
// the memory graph can be rebuilt from Ancora alone without re-parsing code.
//
// Emitted structure:
//   - memory:root:ancora — single root
//   - memory:workspace:<ws>, memory:visibility:<v>, memory:organization:<o>
//     — scope nodes aggregated from the observations actually present
//   - memory:observation:<ancora_id> — one node per observation
//   - memory:concept:<name> — one node per concept reference target
//
// Emitted edges:
//   - structural: root->workspace (contains), workspace->visibility (contains)
//   - binding: observation->workspace (belongs_to), observation->visibility
//     (scoped_to), observation->organization (belongs_to)
//   - cross-memory: observation->observation (related_to),
//     observation->concept (defines)
//   - cross-layer reference edges to repo files/symbols or workspace repos,
//     with canonical "<layer>:<kind>:<key>" targets and a verification state
//     so the identity resolver can later rebind stale references.
//
// Every node and edge is stamped with layer=memory plus evidence_type and
// evidence_confidence so ranking and explainability stay consistent across
// layers.
func BuildMemory(obs []ancora.Observation, opts MemoryOptions) *MemoryGraph {
	g := &MemoryGraph{}
	if len(obs) == 0 {
		return g
	}

	knownRepos := map[string]string{} // lower -> display
	for _, r := range opts.KnownRepos {
		if r = strings.TrimSpace(r); r != "" {
			knownRepos[strings.ToLower(r)] = r
		}
	}

	// ── Root ─────────────────────────────────────────────────────────────
	root := types.Node{
		ID:          MemoryRootID,
		Label:       "Ancora Memory",
		NodeType:    string(types.NodeTypeMemorySource),
		Description: "Persistent memory: decisions, bugs, architecture observations",
	}
	stampMemoryNode(&root, MemoryEvidenceStructural, types.ConfidenceDeclared, "")
	g.Nodes = append(g.Nodes, root)

	// ── Aggregate scope sets ─────────────────────────────────────────────
	wsSet := map[string]string{}  // lower -> display
	visSet := map[string]string{} // lower -> display
	orgSet := map[string]string{}
	wsVisPairs := map[[2]string]bool{} // [lowerWs, lowerVis]

	for _, o := range obs {
		if o.Workspace != "" {
			wsSet[strings.ToLower(o.Workspace)] = o.Workspace
		}
		if o.Visibility != "" {
			visSet[strings.ToLower(o.Visibility)] = o.Visibility
		}
		if o.Organization != "" {
			orgSet[strings.ToLower(o.Organization)] = o.Organization
		}
		if o.Workspace != "" && o.Visibility != "" {
			wsVisPairs[[2]string{strings.ToLower(o.Workspace), strings.ToLower(o.Visibility)}] = true
		}

		for _, r := range parseMemoryRefs(o.References) {
			target := strings.TrimSpace(r.Target)
			if target == "" {
				continue
			}
			switch r.Type {
			case "workspace", "repo":
				if _, ok := knownRepos[strings.ToLower(target)]; ok {
					continue
				}
				wsSet[strings.ToLower(target)] = target
			case "organization", "org":
				orgSet[strings.ToLower(target)] = target
			}
		}
	}

	// Emit scope nodes deterministically.
	for _, key := range sortedStringKeys(wsSet) {
		n := types.Node{
			ID:       MemoryWorkspaceID(wsSet[key]),
			Label:    wsSet[key],
			NodeType: string(types.NodeTypeWorkspace),
		}
		stampMemoryNode(&n, MemoryEvidenceStructural, types.ConfidenceDeclared, "")
		g.Nodes = append(g.Nodes, n)
		g.Edges = append(g.Edges, memoryEdge(MemoryRootID, n.ID, MemoryRelContains,
			MemoryEvidenceStructural, types.ConfidenceDeclared, ""))
	}
	for _, key := range sortedStringKeys(visSet) {
		n := types.Node{
			ID:       MemoryVisibilityID(visSet[key]),
			Label:    visSet[key],
			NodeType: string(types.NodeTypeVisibility),
		}
		stampMemoryNode(&n, MemoryEvidenceStructural, types.ConfidenceDeclared, "")
		g.Nodes = append(g.Nodes, n)
	}
	for _, key := range sortedStringKeys(orgSet) {
		n := types.Node{
			ID:       MemoryOrganizationID(orgSet[key]),
			Label:    orgSet[key],
			NodeType: string(types.NodeTypeOrganization),
		}
		stampMemoryNode(&n, MemoryEvidenceStructural, types.ConfidenceDeclared, "")
		g.Nodes = append(g.Nodes, n)
	}

	// Workspace → visibility edges (only observed pairs; avoid cartesian blow-up).
	pairs := make([][2]string, 0, len(wsVisPairs))
	for p := range wsVisPairs {
		pairs = append(pairs, p)
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i][0] != pairs[j][0] {
			return pairs[i][0] < pairs[j][0]
		}
		return pairs[i][1] < pairs[j][1]
	})
	for _, p := range pairs {
		g.Edges = append(g.Edges, memoryEdge(
			memoryWorkspacePrefix+p[0],
			memoryVisibilityPrefix+p[1],
			MemoryRelContains,
			MemoryEvidenceStructural, types.ConfidenceDeclared, "",
		))
	}

	// ── Observations ─────────────────────────────────────────────────────
	conceptSeen := map[string]string{} // lower -> display
	for _, o := range obs {
		obsID := MemoryObservationID(o.ID)
		artifact := fmt.Sprintf("ancora:obs:%d", o.ID)
		label := o.Title
		if label == "" {
			label = fmt.Sprintf("obs-%d", o.ID)
		}

		node := types.Node{
			ID:          obsID,
			Label:       label,
			NodeType:    string(types.NodeTypeObservation),
			Description: truncateRunes(o.Content, 300),
			Metadata: map[string]interface{}{
				"ancora_id":    o.ID,
				"obs_type":     o.Type,
				"workspace":    o.Workspace,
				"visibility":   o.Visibility,
				"organization": o.Organization,
				"topic_key":    o.TopicKey,
				"created_at":   o.CreatedAt.Format("2006-01-02"),
			},
		}
		stampMemoryNode(&node, MemoryEvidenceObservation, types.ConfidenceDeclared, artifact)
		g.Nodes = append(g.Nodes, node)

		// Scope bindings — memory-internal.
		if o.Workspace != "" {
			g.Edges = append(g.Edges, memoryEdge(
				obsID, MemoryWorkspaceID(o.Workspace), MemoryRelBelongsTo,
				MemoryEvidenceStructural, types.ConfidenceDeclared, artifact,
			))
			// Cross-layer mention when the workspace matches a known repo.
			if display, ok := knownRepos[strings.ToLower(o.Workspace)]; ok {
				g.Edges = append(g.Edges, memoryRefEdge(
					obsID, RefWorkspaceRepo+strings.ToLower(display),
					MemoryRelMentions, types.ConfidenceDeclared,
					types.VerificationCurrent, artifact,
				))
			}
		}
		if o.Visibility != "" {
			g.Edges = append(g.Edges, memoryEdge(
				obsID, MemoryVisibilityID(o.Visibility), MemoryRelScopedTo,
				MemoryEvidenceStructural, types.ConfidenceDeclared, artifact,
			))
		}
		if o.Organization != "" {
			g.Edges = append(g.Edges, memoryEdge(
				obsID, MemoryOrganizationID(o.Organization), MemoryRelBelongsTo,
				MemoryEvidenceStructural, types.ConfidenceDeclared, artifact,
			))
		}

		// Declared references stored on the observation.
		refs := parseMemoryRefs(o.References)
		for _, r := range refs {
			if target := strings.TrimSpace(r.Target); target != "" {
				switch r.Type {
				case "workspace", "repo":
					if _, ok := knownRepos[strings.ToLower(target)]; !ok {
						g.Edges = append(g.Edges, memoryEdge(
							obsID, MemoryWorkspaceID(target), MemoryRelMentions,
							MemoryEvidenceReference, types.ConfidenceDeclared, artifact,
						))
						continue
					}
				case "organization", "org":
					g.Edges = append(g.Edges, memoryEdge(
						obsID, MemoryOrganizationID(target), MemoryRelBelongsTo,
						MemoryEvidenceReference, types.ConfidenceDeclared, artifact,
					))
					continue
				}
			}

			ref, ok := ResolveMemoryReference(r.Type, r.Target, o.Type, opts)
			if !ok {
				continue
			}

			// Internal concept links emit concept nodes on demand.
			if strings.HasPrefix(ref.Target, memoryConceptPrefix) {
				low := strings.ToLower(r.Target)
				if _, seen := conceptSeen[low]; !seen {
					conceptSeen[low] = r.Target
				}
			}
			g.Edges = append(g.Edges, MemoryReferenceEdge(obsID, ref, artifact))
		}
	}

	// Emit concept nodes discovered via references.
	for _, key := range sortedStringKeys(conceptSeen) {
		n := types.Node{
			ID:       MemoryConceptID(conceptSeen[key]),
			Label:    conceptSeen[key],
			NodeType: string(types.NodeTypeConcept),
		}
		stampMemoryNode(&n, MemoryEvidenceReference, types.ConfidenceInferred, "")
		g.Nodes = append(g.Nodes, n)
	}

	return g
}

// MergeMemory folds the memory graph onto an existing repo+contract+workspace
// graph. Memory nodes keep their "memory:" identity; cross-layer reference
// edges target canonical keys that already exist elsewhere. Memory never
// overwrites repo/contract/workspace nodes.
func MergeMemory(
	baseNodes []types.Node, baseEdges []types.Edge,
	memNodes []types.Node, memEdges []types.Edge,
) ([]types.Node, []types.Edge) {
	memEdges = BindMemoryReferences(memEdges, baseNodes)

	seenNode := map[string]bool{}
	mergedNodes := make([]types.Node, 0, len(baseNodes)+len(memNodes))
	for _, n := range baseNodes {
		if seenNode[n.ID] {
			continue
		}
		seenNode[n.ID] = true
		mergedNodes = append(mergedNodes, n)
	}
	for _, n := range memNodes {
		if seenNode[n.ID] {
			continue // base wins — memory never overwrites other layers
		}
		seenNode[n.ID] = true
		mergedNodes = append(mergedNodes, n)
	}

	type ek struct{ s, t, r string }
	seenEdge := map[ek]bool{}
	mergedEdges := make([]types.Edge, 0, len(baseEdges)+len(memEdges))
	for _, e := range baseEdges {
		k := ek{e.Source, e.Target, e.Relation}
		if seenEdge[k] {
			continue
		}
		seenEdge[k] = true
		mergedEdges = append(mergedEdges, e)
	}
	for _, e := range memEdges {
		k := ek{e.Source, e.Target, e.Relation}
		if seenEdge[k] {
			continue
		}
		seenEdge[k] = true
		mergedEdges = append(mergedEdges, e)
	}
	return mergedNodes, mergedEdges
}

// ─── Reference resolution ────────────────────────────────────────────────

// resolveRef translates an observation's raw (type, target) reference into a
// canonical cross-layer target, its kind tag, a confidence, and a verification
// state. Unknown or malformed refs return an empty target so the caller can
// skip them rather than emitting ambiguous edges.
//
// kind tags:
//   - "repo-file"         -> RefRepoFile + path
//   - "repo-symbol"       -> RefRepoSymbol + name
//   - "workspace-repo"    -> RefWorkspaceRepo + name
//   - "memory-observation"-> memory-internal observation edge (already canonical)
//   - "memory-concept"    -> memory-internal concept edge
func resolveRef(r ancoraWireRef, knownRepos map[string]string) (target, kind string, conf types.Confidence, verif types.VerificationState) {
	t := strings.TrimSpace(r.Target)
	if t == "" {
		return "", "", "", ""
	}
	switch r.Type {
	case "observation":
		// Accept either "ancora:obs:<n>" (legacy) or a bare numeric id.
		if strings.HasPrefix(t, "ancora:obs:") {
			return memoryObservationPrefix + strings.TrimPrefix(t, "ancora:obs:"),
				"memory-observation", types.ConfidenceDeclared, ""
		}
		if strings.HasPrefix(t, memoryObservationPrefix) {
			return t, "memory-observation", types.ConfidenceDeclared, ""
		}
		return "", "", "", ""
	case "concept":
		return MemoryConceptID(t), "memory-concept", types.ConfidenceDeclared, ""
	case "file":
		return RefRepoFile + t, "repo-file", types.ConfidenceDeclared, types.VerificationCurrent
	case "function", "symbol":
		return RefRepoSymbol + t, "repo-symbol", types.ConfidenceDeclared, types.VerificationCurrent
	case "workspace", "repo":
		canonical := RefWorkspaceRepo + strings.ToLower(t)
		if display, ok := knownRepos[strings.ToLower(t)]; ok {
			canonical = RefWorkspaceRepo + strings.ToLower(display)
			return canonical, "workspace-repo", types.ConfidenceDeclared, types.VerificationCurrent
		}
		return canonical, "workspace-repo", types.ConfidenceDeclared, types.VerificationAmbiguous
	case "organization", "org":
		return MemoryOrganizationID(t), "memory-organization", types.ConfidenceDeclared, ""
	}
	return "", "", "", ""
}

// ResolveMemoryReference canonicalizes a raw observation reference into a
// layer-aware target plus relation metadata.
func ResolveMemoryReference(refType, target, obsType string, opts MemoryOptions) (ResolvedMemoryReference, bool) {
	knownRepos := map[string]string{}
	for _, r := range opts.KnownRepos {
		if r = strings.TrimSpace(r); r != "" {
			knownRepos[strings.ToLower(r)] = r
		}
	}

	canonicalTarget, _, conf, verif := resolveRef(ancoraWireRef{Type: refType, Target: target}, knownRepos)
	if canonicalTarget == "" {
		return ResolvedMemoryReference{}, false
	}

	return ResolvedMemoryReference{
		Target:       canonicalTarget,
		Relation:     memoryRelFor(refType, obsType),
		Confidence:   conf,
		Verification: verif,
		CrossLayer:   targetLayerOf(canonicalTarget) != string(types.LayerMemory),
	}, true
}

// memoryRelFor maps (refType, obsType) to a memory-layer edge relation, mirroring
// the semantic relation table used by the extract package but kept private to
// the memory graph so layer responsibilities stay separated.
func memoryRelFor(refType, obsType string) string {
	switch refType {
	case "observation":
		return MemoryRelRelatedTo
	case "concept":
		return MemoryRelDefines
	case "workspace", "repo":
		return MemoryRelMentions
	case "organization", "org":
		return MemoryRelBelongsTo
	}
	switch obsType {
	case "decision":
		return MemoryRelDecidesOn
	case "bugfix":
		return MemoryRelConstrains
	case "pattern":
		return MemoryRelExemplifies
	case "architecture", "discovery", "learning":
		return MemoryRelDocuments
	}
	return MemoryRelReferences
}

func parseMemoryRefs(js string) []ancoraWireRef {
	js = strings.TrimSpace(js)
	if js == "" {
		return nil
	}
	var out []ancoraWireRef
	if err := json.Unmarshal([]byte(js), &out); err != nil {
		return nil
	}
	return out
}

// ─── Stamping helpers ────────────────────────────────────────────────────

func stampMemoryNode(n *types.Node, evType string, conf types.Confidence, artifact string) {
	if n.Metadata == nil {
		n.Metadata = map[string]interface{}{}
	}
	n.Metadata["layer"] = string(types.LayerMemory)
	n.Metadata["evidence_type"] = evType
	n.Metadata["evidence_confidence"] = string(conf)
	if artifact != "" {
		n.Metadata["evidence_source_artifact"] = artifact
	}
}

func memoryEdge(src, tgt, rel, evType string, conf types.Confidence, artifact string) types.Edge {
	e := types.Edge{
		Source:     src,
		Target:     tgt,
		Relation:   rel,
		Confidence: strings.ToUpper(string(conf)),
		SourceFile: artifact,
		Metadata: map[string]interface{}{
			"layer":               string(types.LayerMemory),
			"evidence_type":       evType,
			"evidence_confidence": string(conf),
		},
	}
	if artifact != "" {
		e.Metadata["evidence_source_artifact"] = artifact
	}
	return e
}

// memoryRefEdge is a cross-layer reference edge. It carries a verification
// state so the identity resolver / binder can later move the reference between
// current/redirected/stale/ambiguous without losing provenance.
func memoryRefEdge(src, canonicalTarget, rel string, conf types.Confidence, verif types.VerificationState, artifact string) types.Edge {
	e := memoryEdge(src, canonicalTarget, rel, MemoryEvidenceReference, conf, artifact)
	e.Metadata["cross_layer"] = true
	e.Metadata["target_layer"] = targetLayerOf(canonicalTarget)
	if verif != "" {
		e.Metadata["verification"] = string(verif)
	}
	return e
}

// MemoryReferenceEdge materializes a canonicalized memory reference edge.
func MemoryReferenceEdge(src string, ref ResolvedMemoryReference, artifact string) types.Edge {
	if ref.CrossLayer {
		return memoryRefEdge(src, ref.Target, ref.Relation, ref.Confidence, ref.Verification, artifact)
	}
	return memoryEdge(src, ref.Target, ref.Relation, MemoryEvidenceReference, ref.Confidence, artifact)
}

func targetLayerOf(canonicalTarget string) string {
	switch {
	case strings.HasPrefix(canonicalTarget, "repo:"):
		return string(types.LayerRepo)
	case strings.HasPrefix(canonicalTarget, "contract:"):
		return string(types.LayerContract)
	case strings.HasPrefix(canonicalTarget, "workspace:"):
		return string(types.LayerWorkspace)
	case strings.HasPrefix(canonicalTarget, "memory:"):
		return string(types.LayerMemory)
	}
	return ""
}

func sortedStringKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func truncateRunes(s string, maxLen int) string {
	rs := []rune(s)
	if len(rs) <= maxLen {
		return s
	}
	return string(rs[:maxLen]) + "…"
}
