package tui

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Syfra3/vela/pkg/types"
)

func loadGraph(graphPath string) (*types.Graph, error) {
	data, err := os.ReadFile(graphPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", graphPath, err)
	}

	var raw struct {
		Nodes []struct {
			ID           string                 `json:"id"`
			Label        string                 `json:"label"`
			Kind         string                 `json:"kind"`
			File         string                 `json:"file"`
			Description  string                 `json:"description"`
			SourceType   string                 `json:"source_type"`
			SourceName   string                 `json:"source_name"`
			SourcePath   string                 `json:"source_path"`
			SourceRemote string                 `json:"source_remote"`
			Metadata     map[string]interface{} `json:"metadata"`
		} `json:"nodes"`
		Edges []struct {
			From       string                 `json:"from"`
			To         string                 `json:"to"`
			Kind       string                 `json:"kind"`
			File       string                 `json:"file"`
			Confidence string                 `json:"confidence"`
			Score      float64                `json:"score"`
			Metadata   map[string]interface{} `json:"metadata"`
		} `json:"edges"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing graph.json: %w", err)
	}

	g := &types.Graph{
		Nodes: make([]types.Node, len(raw.Nodes)),
		Edges: make([]types.Edge, len(raw.Edges)),
	}
	for i, n := range raw.Nodes {
		g.Nodes[i] = types.Node{
			ID:          n.ID,
			Label:       n.Label,
			NodeType:    n.Kind,
			SourceFile:  n.File,
			Description: n.Description,
			Metadata:    n.Metadata,
		}
		if n.SourceType != "" || n.SourceName != "" || n.SourcePath != "" || n.SourceRemote != "" {
			g.Nodes[i].Source = &types.Source{
				Type:   types.SourceType(n.SourceType),
				Name:   n.SourceName,
				Path:   n.SourcePath,
				Remote: n.SourceRemote,
			}
		}
	}
	for i, e := range raw.Edges {
		g.Edges[i] = types.Edge{
			Source:     e.From,
			Target:     e.To,
			Relation:   e.Kind,
			SourceFile: e.File,
			Confidence: e.Confidence,
			Score:      e.Score,
			Metadata:   e.Metadata,
		}
	}

	return g, nil
}
