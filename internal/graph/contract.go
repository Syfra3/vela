package graph

import (
	"github.com/Syfra3/vela/pkg/types"
)

// MergeContract folds a contract-layer graph (nodes + edges) on top of a
// repo-layer graph while preserving the declared > derived invariant.
//
// Guarantees:
//   - Repo and contract nodes live side-by-side. Contract nodes keep their
//     "contract:*" identity; they are never merged into repo-local entities.
//   - When a derived edge and a declared edge claim the same (source, target,
//     relation) triple, the derived edge is dropped — declared truth wins.
//   - Duplicate declared edges are deduplicated on (source, target, relation)
//     so ingesting the same artifact twice is idempotent.
//
// This helper stays structural: it does not inspect evidence scores beyond
// the `layer` and `evidence_confidence` metadata that extractors stamp. That
// keeps the invariant easy to audit in tests and keeps the contract layer
// from quietly reaching into repo ranking logic.
func MergeContract(
	repoNodes []types.Node, repoEdges []types.Edge,
	contractNodes []types.Node, contractEdges []types.Edge,
) ([]types.Node, []types.Edge) {

	// Deduplicate nodes by ID. Contract nodes are appended after repo nodes;
	// if a contract node collides with a repo node (it shouldn't, given the
	// "contract:" prefix), the repo node keeps its slot — the merge never
	// overwrites repo identity from the contract side.
	seenNode := map[string]bool{}
	mergedNodes := make([]types.Node, 0, len(repoNodes)+len(contractNodes))
	for _, n := range repoNodes {
		if seenNode[n.ID] {
			continue
		}
		seenNode[n.ID] = true
		mergedNodes = append(mergedNodes, n)
	}
	for _, n := range contractNodes {
		if seenNode[n.ID] {
			continue
		}
		seenNode[n.ID] = true
		mergedNodes = append(mergedNodes, n)
	}

	type edgeKey struct{ src, tgt, rel string }
	key := func(e types.Edge) edgeKey { return edgeKey{e.Source, e.Target, e.Relation} }
	order := make([]edgeKey, 0, len(repoEdges)+len(contractEdges))
	mergedByKey := map[edgeKey]types.Edge{}
	seenOrder := map[edgeKey]bool{}
	remember := func(k edgeKey) {
		if seenOrder[k] {
			return
		}
		seenOrder[k] = true
		order = append(order, k)
	}
	mergeEdge := func(e types.Edge) {
		k := key(e)
		remember(k)
		if current, ok := mergedByKey[k]; ok {
			if types.PreferEdgeEvidence(e, current) {
				mergedByKey[k] = e
			}
			return
		}
		mergedByKey[k] = e
	}

	for _, e := range contractEdges {
		mergeEdge(e)
	}
	for _, e := range repoEdges {
		mergeEdge(e)
	}
	mergedEdges := make([]types.Edge, 0, len(repoEdges)+len(contractEdges))
	for _, k := range order {
		mergedEdges = append(mergedEdges, mergedByKey[k])
	}

	return mergedNodes, mergedEdges
}
