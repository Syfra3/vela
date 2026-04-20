package export

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/internal/retrieval"
	"github.com/Syfra3/vela/pkg/types"
)

// graphJSON is the on-disk representation of the graph.
type graphJSON struct {
	Nodes []nodeJSON `json:"nodes"`
	Edges []edgeJSON `json:"edges"`
	Meta  metaJSON   `json:"meta"`
}

type nodeJSON struct {
	ID           string                 `json:"id"`
	Label        string                 `json:"label"`
	Kind         string                 `json:"kind"`
	File         string                 `json:"file,omitempty"`
	Description  string                 `json:"description,omitempty"`
	Community    int                    `json:"community,omitempty"`
	SourceType   string                 `json:"source_type,omitempty"`   // "codebase" | "memory" | "webhook"
	SourceName   string                 `json:"source_name,omitempty"`   // repo/project name
	SourcePath   string                 `json:"source_path,omitempty"`   // local codebase path
	SourceRemote string                 `json:"source_remote,omitempty"` // git remote URL
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

type edgeJSON struct {
	From       string                 `json:"from"`
	To         string                 `json:"to"`
	Kind       string                 `json:"kind"`
	File       string                 `json:"file,omitempty"`
	Confidence string                 `json:"confidence,omitempty"`
	Score      float64                `json:"score,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

type metaJSON struct {
	NodeCount   int    `json:"nodeCount"`
	EdgeCount   int    `json:"edgeCount"`
	GeneratedAt string `json:"generatedAt"`
}

// WriteJSON serialises g to <outDir>/graph.json, creating outDir if necessary.
func WriteJSON(g *types.Graph, outDir string) error {
	g = canonicalGraph(g)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("creating output dir %s: %w", outDir, err)
	}
	data, err := marshalGraph(g)
	if err != nil {
		return err
	}
	outPath := filepath.Join(outDir, "graph.json")
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", outPath, err)
	}
	if err := retrieval.SyncGraph(g, retrieval.DBPath(outDir)); err != nil {
		return fmt.Errorf("writing retrieval.db: %w", err)
	}
	return nil
}

// WriteJSONAtomic serialises g to <outDir>/graph.json atomically using a
// temp file + rename so a crash mid-write leaves the previous file intact.
func WriteJSONAtomic(g *types.Graph, outDir string) error {
	g = canonicalGraph(g)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("creating output dir %s: %w", outDir, err)
	}
	data, err := marshalGraph(g)
	if err != nil {
		return err
	}
	outPath := filepath.Join(outDir, "graph.json")
	tmp := outPath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("syncing temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmp, outPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("renaming temp file: %w", err)
	}
	if err := retrieval.SyncGraph(g, retrieval.DBPath(outDir)); err != nil {
		return fmt.Errorf("writing retrieval.db: %w", err)
	}
	return nil
}

func marshalGraph(g *types.Graph) ([]byte, error) {
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
			ID:          n.ID,
			Label:       n.Label,
			Kind:        n.NodeType,
			File:        n.SourceFile,
			Description: n.Description,
			Community:   n.Community,
		}
		if n.Source != nil {
			nj.SourceType = string(n.Source.Type)
			nj.SourceName = n.Source.Name
			nj.SourcePath = n.Source.Path
			nj.SourceRemote = n.Source.Remote
		}
		if len(n.Metadata) > 0 {
			nj.Metadata = n.Metadata
		}
		out.Nodes[i] = nj
	}

	for i, e := range g.Edges {
		ej := edgeJSON{
			From:       e.Source,
			To:         e.Target,
			Kind:       e.Relation,
			File:       e.SourceFile,
			Confidence: e.Confidence,
			Score:      e.Score,
		}
		if len(e.Metadata) > 0 {
			ej.Metadata = e.Metadata
		}
		out.Edges[i] = ej
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshalling graph: %w", err)
	}
	return data, nil
}

func canonicalGraph(g *types.Graph) *types.Graph {
	if g == nil {
		return nil
	}
	nodes, edges := graph.Canonicalize(g.Nodes, g.Edges)
	return &types.Graph{
		Nodes:       nodes,
		Edges:       edges,
		Communities: g.Communities,
		Metadata:    g.Metadata,
		ExtractedAt: g.ExtractedAt,
	}
}

// LoadJSON reads a graph.json file written by WriteJSON/WriteJSONAtomic.
func LoadJSON(path string) (*types.Graph, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw graphJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("unmarshalling graph: %w", err)
	}

	g := &types.Graph{
		Nodes: make([]types.Node, len(raw.Nodes)),
		Edges: make([]types.Edge, len(raw.Edges)),
	}
	for i, n := range raw.Nodes {
		var source *types.Source
		if n.SourceType != "" || n.SourceName != "" || n.SourcePath != "" || n.SourceRemote != "" {
			source = &types.Source{
				Type:   types.SourceType(n.SourceType),
				Name:   n.SourceName,
				Path:   n.SourcePath,
				Remote: n.SourceRemote,
			}
		}
		g.Nodes[i] = types.Node{
			ID:          n.ID,
			Label:       n.Label,
			NodeType:    n.Kind,
			SourceFile:  n.File,
			Description: n.Description,
			Community:   n.Community,
			Metadata:    n.Metadata,
			Source:      source,
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
	g.Nodes, g.Edges = graph.Canonicalize(g.Nodes, g.Edges)

	return g, nil
}
