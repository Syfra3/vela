package query

import (
	"fmt"
	"strings"

	"github.com/Syfra3/vela/pkg/types"
)

// Bindings returns queryable memory reference binder state for the given node.
func (e *Engine) Bindings(label string) string {
	nodeIDs := e.resolveNodeIDs(label)
	if len(nodeIDs) == 0 {
		return fmt.Sprintf("node %q not found", label)
	}
	nodeSet := make(map[string]bool, len(nodeIDs))
	for _, id := range nodeIDs {
		nodeSet[id] = true
	}
	var lines []string
	for _, edge := range e.graph.Edges {
		if !nodeSet[edge.Source] {
			continue
		}
		state, _ := edge.Metadata["binding_state"].(string)
		if state == "" {
			continue
		}
		lines = append(lines, "  "+formatBindingEdge(edge))
	}
	if len(lines) == 0 {
		return fmt.Sprintf("no binding states found for %q", label)
	}
	return fmt.Sprintf("Bindings for %q:\n%s", label, strings.Join(lines, "\n"))
}

func formatBindingEdge(edge types.Edge) string {
	reference, _ := edge.Metadata["reference_target"].(string)
	bound, _ := edge.Metadata["bound_target"].(string)
	state, _ := edge.Metadata["binding_state"].(string)
	evidence, _ := edge.Metadata["binding_evidence"].(string)
	parts := []string{fmt.Sprintf("%s [%s]", edge.Relation, state)}
	if reference != "" {
		parts = append(parts, "reference="+reference)
	}
	if bound != "" {
		parts = append(parts, "bound="+bound)
	}
	if suggestions := metadataStringSlice(edge.Metadata["binding_suggestions"]); len(suggestions) > 0 {
		parts = append(parts, "suggestions="+strings.Join(suggestions, ","))
	}
	if evidence != "" {
		parts = append(parts, "evidence="+evidence)
	}
	return strings.Join(parts, " | ")
}

func metadataStringSlice(v interface{}) []string {
	switch raw := v.(type) {
	case []string:
		return raw
	case []interface{}:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
