package graph

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/Syfra3/vela/pkg/types"
)

const (
	memoryBindingStateKey       = "binding_state"
	memoryBindingEvidenceKey    = "binding_evidence"
	memoryBindingSuggestionsKey = "binding_suggestions"
	memoryBoundTargetKey        = "bound_target"
	memoryReferenceTargetKey    = "reference_target"
)

// MemoryReferenceBinding captures the live binding state for a memory reference.
// The original reference target is preserved separately so history does not get
// silently rewritten when code moves.
type MemoryReferenceBinding struct {
	State       types.VerificationState
	BoundTarget string
	Suggestions []string
	Evidence    string
}

type memoryBinderIndex struct {
	filesByPath    map[string]types.Node
	filesByBase    map[string][]types.Node
	symbolsByLabel map[string][]types.Node
	workspaceRepos map[string][]types.Node
}

// BindMemoryReferences projects memory reference edges onto the current live
// code/workspace nodes. Current and redirected bindings are rewritten to point
// at the live node ID; stale and ambiguous references keep their original
// canonical target and carry queryable binder metadata instead.
func BindMemoryReferences(edges []types.Edge, liveNodes []types.Node) []types.Edge {
	idx := buildMemoryBinderIndex(liveNodes)
	out := make([]types.Edge, 0, len(edges))
	for _, edge := range edges {
		out = append(out, idx.bindEdge(edge))
	}
	return out
}

// BindMemoryReferenceEdge binds a single memory reference edge against the
// current graph state.
func BindMemoryReferenceEdge(edge types.Edge, liveNodes []types.Node) types.Edge {
	idx := buildMemoryBinderIndex(liveNodes)
	return idx.bindEdge(edge)
}

func buildMemoryBinderIndex(nodes []types.Node) memoryBinderIndex {
	idx := memoryBinderIndex{
		filesByPath:    map[string]types.Node{},
		filesByBase:    map[string][]types.Node{},
		symbolsByLabel: map[string][]types.Node{},
		workspaceRepos: map[string][]types.Node{},
	}
	for _, node := range nodes {
		if node.NodeType == string(types.NodeTypeFile) {
			path := strings.ToLower(strings.TrimSpace(node.SourceFile))
			if path == "" {
				path = strings.ToLower(strings.TrimSpace(node.Label))
			}
			if path == "" {
				continue
			}
			idx.filesByPath[path] = node
			base := strings.ToLower(filepath.Base(path))
			idx.filesByBase[base] = append(idx.filesByBase[base], node)
			continue
		}
		canonical := types.CanonicalKeyForNode(node)
		if canonical.Layer == types.LayerWorkspace && canonical.Kind == "repo" {
			key := strings.ToLower(strings.TrimSpace(canonical.Key))
			if key != "" {
				idx.workspaceRepos[key] = append(idx.workspaceRepos[key], node)
			}
			continue
		}
		if node.NodeType == string(types.NodeTypeFile) || node.NodeType == string(types.NodeTypeProject) {
			continue
		}
		label := strings.ToLower(strings.TrimSpace(node.Label))
		if label == "" {
			continue
		}
		idx.symbolsByLabel[label] = append(idx.symbolsByLabel[label], node)
	}
	return idx
}

func (idx memoryBinderIndex) bindEdge(edge types.Edge) types.Edge {
	if !isMemoryReferenceEdge(edge) {
		return edge
	}
	original := memoryReferenceTarget(edge)
	if original == "" {
		return edge
	}
	binding := idx.bindTarget(original)
	if edge.Metadata == nil {
		edge.Metadata = map[string]interface{}{}
	}
	edge.Metadata[memoryReferenceTargetKey] = original
	edge.Metadata[memoryBindingStateKey] = string(binding.State)
	edge.Metadata[memoryBindingEvidenceKey] = binding.Evidence
	edge.Metadata["verification"] = string(binding.State)
	delete(edge.Metadata, memoryBindingSuggestionsKey)
	delete(edge.Metadata, memoryBoundTargetKey)
	if len(binding.Suggestions) > 0 {
		edge.Metadata[memoryBindingSuggestionsKey] = binding.Suggestions
	}
	if binding.BoundTarget != "" {
		edge.Metadata[memoryBoundTargetKey] = binding.BoundTarget
		if binding.State == types.VerificationCurrent || binding.State == types.VerificationRedirected {
			edge.Target = binding.BoundTarget
		}
	}
	return edge
}

func (idx memoryBinderIndex) bindTarget(target string) MemoryReferenceBinding {
	switch {
	case strings.HasPrefix(target, RefRepoFile):
		return idx.bindRepoFile(strings.TrimPrefix(target, RefRepoFile))
	case strings.HasPrefix(target, RefRepoSymbol):
		return idx.bindRepoSymbol(strings.TrimPrefix(target, RefRepoSymbol))
	case strings.HasPrefix(target, RefWorkspaceRepo):
		return idx.bindWorkspaceRepo(strings.TrimPrefix(target, RefWorkspaceRepo))
	default:
		return MemoryReferenceBinding{State: types.VerificationCurrent, BoundTarget: target, Evidence: "memory-internal reference"}
	}
}

func (idx memoryBinderIndex) bindRepoFile(path string) MemoryReferenceBinding {
	path = strings.TrimSpace(path)
	if path == "" {
		return MemoryReferenceBinding{State: types.VerificationStale, Evidence: "empty file reference"}
	}
	if node, ok := idx.filesByPath[strings.ToLower(path)]; ok {
		return MemoryReferenceBinding{State: types.VerificationCurrent, BoundTarget: node.ID, Evidence: "exact file path match"}
	}
	base := strings.ToLower(filepath.Base(path))
	candidates := append([]types.Node(nil), idx.filesByBase[base]...)
	if len(candidates) == 0 {
		return MemoryReferenceBinding{State: types.VerificationStale, Evidence: "no live file candidate matched the historical path"}
	}
	if len(candidates) == 1 {
		return MemoryReferenceBinding{State: types.VerificationRedirected, BoundTarget: candidates[0].ID, Evidence: "unique basename match"}
	}
	type scoredNode struct {
		node  types.Node
		score int
	}
	scored := make([]scoredNode, 0, len(candidates))
	for _, candidate := range candidates {
		score := sharedPathSuffixScore(path, candidate.SourceFile)
		scored = append(scored, scoredNode{node: candidate, score: score})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].node.ID < scored[j].node.ID
	})
	if len(scored) > 0 && scored[0].score > 1 && (len(scored) == 1 || scored[0].score > scored[1].score) {
		return MemoryReferenceBinding{State: types.VerificationRedirected, BoundTarget: scored[0].node.ID, Evidence: "best unique path-suffix match"}
	}
	return MemoryReferenceBinding{
		State:       types.VerificationAmbiguous,
		Suggestions: nodeIDsForBinding(candidates),
		Evidence:    "multiple live files share the historical basename",
	}
}

func (idx memoryBinderIndex) bindRepoSymbol(name string) MemoryReferenceBinding {
	name = strings.TrimSpace(name)
	if name == "" {
		return MemoryReferenceBinding{State: types.VerificationStale, Evidence: "empty symbol reference"}
	}
	candidates := idx.symbolsByLabel[strings.ToLower(name)]
	switch len(candidates) {
	case 0:
		return MemoryReferenceBinding{State: types.VerificationStale, Evidence: "no live symbol label matched the historical reference"}
	case 1:
		return MemoryReferenceBinding{State: types.VerificationCurrent, BoundTarget: candidates[0].ID, Evidence: "unique symbol label match"}
	default:
		return MemoryReferenceBinding{
			State:       types.VerificationAmbiguous,
			Suggestions: nodeIDsForBinding(candidates),
			Evidence:    "multiple live symbols share the historical label",
		}
	}
}

func (idx memoryBinderIndex) bindWorkspaceRepo(name string) MemoryReferenceBinding {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return MemoryReferenceBinding{State: types.VerificationStale, Evidence: "empty workspace repo reference"}
	}
	candidates := idx.workspaceRepos[name]
	switch len(candidates) {
	case 0:
		return MemoryReferenceBinding{State: types.VerificationStale, Evidence: "workspace repo no longer exists in the live graph"}
	case 1:
		return MemoryReferenceBinding{State: types.VerificationCurrent, BoundTarget: candidates[0].ID, Evidence: "exact workspace repo match"}
	default:
		return MemoryReferenceBinding{
			State:       types.VerificationAmbiguous,
			Suggestions: nodeIDsForBinding(candidates),
			Evidence:    "multiple workspace repos matched the historical reference",
		}
	}
}

func isMemoryReferenceEdge(edge types.Edge) bool {
	if edge.Metadata == nil {
		return false
	}
	if edge.Metadata["layer"] != string(types.LayerMemory) {
		return false
	}
	if edge.Metadata["evidence_type"] != MemoryEvidenceReference {
		return false
	}
	return true
}

func memoryReferenceTarget(edge types.Edge) string {
	if edge.Metadata != nil {
		if ref, ok := edge.Metadata[memoryReferenceTargetKey].(string); ok && strings.TrimSpace(ref) != "" {
			return ref
		}
	}
	return strings.TrimSpace(edge.Target)
}

func sharedPathSuffixScore(a, b string) int {
	aParts := splitPathParts(a)
	bParts := splitPathParts(b)
	score := 0
	for i, j := len(aParts)-1, len(bParts)-1; i >= 0 && j >= 0; i, j = i-1, j-1 {
		if aParts[i] != bParts[j] {
			break
		}
		score++
	}
	return score
}

func splitPathParts(path string) []string {
	clean := filepath.ToSlash(strings.TrimSpace(path))
	if clean == "" {
		return nil
	}
	parts := strings.Split(clean, "/")
	for i := range parts {
		parts[i] = strings.ToLower(parts[i])
	}
	return parts
}

func nodeIDsForBinding(nodes []types.Node) []string {
	ids := make([]string, 0, len(nodes))
	seen := map[string]struct{}{}
	for _, node := range nodes {
		if _, ok := seen[node.ID]; ok {
			continue
		}
		seen[node.ID] = struct{}{}
		ids = append(ids, node.ID)
	}
	sort.Strings(ids)
	return ids
}
