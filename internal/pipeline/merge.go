package pipeline

import (
	"path/filepath"
	"strings"

	"github.com/Syfra3/vela/pkg/types"
)

func MergeFacts(nodes []types.Node, edges []types.Edge, facts []types.Fact) ([]types.Node, []types.Edge) {
	seenNodes := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		seenNodes[node.ID] = struct{}{}
	}
	seenEdges := make(map[string]struct{}, len(edges))
	for _, edge := range edges {
		seenEdges[edgeKey(edge.Source, edge.Target, edge.Relation)] = struct{}{}
	}

	mergedNodes := append([]types.Node(nil), nodes...)
	mergedEdges := append([]types.Edge(nil), edges...)
	for _, fact := range facts {
		if strings.TrimSpace(fact.From) == "" {
			continue
		}
		if _, ok := seenNodes[fact.From]; !ok {
			mergedNodes = append(mergedNodes, factNode(fact.From))
			seenNodes[fact.From] = struct{}{}
		}
		if strings.TrimSpace(fact.To) != "" {
			if _, ok := seenNodes[fact.To]; !ok {
				mergedNodes = append(mergedNodes, factNode(fact.To))
				seenNodes[fact.To] = struct{}{}
			}
			key := edgeKey(fact.From, fact.To, string(fact.Kind))
			if _, ok := seenEdges[key]; ok {
				continue
			}
			mergedEdges = append(mergedEdges, factEdge(fact))
			seenEdges[key] = struct{}{}
		}
	}
	return mergedNodes, mergedEdges
}

func factNode(id string) types.Node {
	return types.Node{
		ID:       id,
		Label:    nodeLabel(id),
		NodeType: "symbol",
		Metadata: map[string]interface{}{
			"layer": "repo",
		},
	}
}

func factEdge(fact types.Fact) types.Edge {
	prov := firstProvenance(fact.Provenance)
	edge := types.Edge{
		Source:   fact.From,
		Target:   fact.To,
		Relation: string(fact.Kind),
		Metadata: map[string]interface{}{
			"layer":                    "repo",
			"evidence_type":            strings.TrimSpace(prov.Source),
			"evidence_confidence":      string(prov.Confidence),
			"evidence_source_artifact": strings.TrimSpace(prov.Artifact),
		},
	}
	if edge.Metadata["evidence_type"] == "" {
		edge.Metadata["evidence_type"] = strings.TrimSpace(prov.Driver)
	}
	if edge.Metadata["evidence_type"] == "" {
		edge.Metadata["evidence_type"] = "semantic"
	}
	if edge.Metadata["evidence_confidence"] == "" {
		edge.Metadata["evidence_confidence"] = string(types.ConfidenceExtracted)
	}
	return edge
}

func firstProvenance(values []types.Provenance) types.Provenance {
	if len(values) == 0 {
		return types.Provenance{}
	}
	return values[len(values)-1]
}

func nodeLabel(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	if idx := strings.LastIndex(id, ":"); idx >= 0 && idx < len(id)-1 {
		return id[idx+1:]
	}
	if idx := strings.LastIndex(id, "/"); idx >= 0 && idx < len(id)-1 {
		return filepath.Base(id)
	}
	return id
}

func edgeKey(source, target, relation string) string {
	return source + "|" + relation + "|" + target
}
