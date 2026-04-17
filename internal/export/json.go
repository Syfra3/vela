package export

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Syfra3/vela/pkg/types"
)

// graphJSON is the on-disk representation of the graph.
type graphJSON struct {
	Nodes []nodeJSON `json:"nodes"`
	Edges []edgeJSON `json:"edges"`
	Meta  metaJSON   `json:"meta"`
}

type nodeJSON struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Kind       string `json:"kind"`
	File       string `json:"file,omitempty"`
	Community  int    `json:"community,omitempty"`
	SourceType string `json:"source_type,omitempty"` // "codebase" | "memory" | "webhook"
	SourceName string `json:"source_name,omitempty"` // repo/project name
}

type edgeJSON struct {
	From       string  `json:"from"`
	To         string  `json:"to"`
	Kind       string  `json:"kind"`
	File       string  `json:"file,omitempty"`
	Confidence string  `json:"confidence,omitempty"`
	Score      float64 `json:"score,omitempty"`
}

type metaJSON struct {
	NodeCount   int    `json:"nodeCount"`
	EdgeCount   int    `json:"edgeCount"`
	GeneratedAt string `json:"generatedAt"`
}

// WriteJSON serialises g to <outDir>/graph.json, creating outDir if necessary.
func WriteJSON(g *types.Graph, outDir string) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("creating output dir %s: %w", outDir, err)
	}

	out := graphJSON{
		Nodes: make([]nodeJSON, len(g.Nodes)),
		Edges: make([]edgeJSON, len(g.Edges)),
		Meta: metaJSON{
			NodeCount:   len(g.Nodes),
			EdgeCount:   len(g.Edges),
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}

	for i, n := range g.Nodes {
		nj := nodeJSON{
			ID:        n.ID,
			Label:     n.Label,
			Kind:      n.NodeType,
			File:      n.SourceFile,
			Community: n.Community,
		}
		if n.Source != nil {
			nj.SourceType = string(n.Source.Type)
			nj.SourceName = n.Source.Name
		}
		out.Nodes[i] = nj
	}

	for i, e := range g.Edges {
		out.Edges[i] = edgeJSON{
			From:       e.Source,
			To:         e.Target,
			Kind:       e.Relation,
			File:       e.SourceFile,
			Confidence: e.Confidence,
			Score:      e.Score,
		}
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling graph: %w", err)
	}

	outPath := filepath.Join(outDir, "graph.json")
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", outPath, err)
	}
	return nil
}
