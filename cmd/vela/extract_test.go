package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Syfra3/vela/pkg/types"
)

func TestWriteVisualExportsWritesHTMLAndObsidianVault(t *testing.T) {
	outDir := t.TempDir()
	vaultDir := t.TempDir()

	g := &types.Graph{
		Nodes: []types.Node{{
			ID:       "project:vela",
			Label:    "vela",
			NodeType: string(types.NodeTypeProject),
			Source: &types.Source{
				Type: types.SourceTypeCodebase,
				Name: "vela",
			},
		}},
		Edges:       []types.Edge{},
		Communities: map[int][]string{},
		ExtractedAt: time.Now(),
	}

	writeVisualExports(g, outDir, types.ObsidianConfig{VaultDir: vaultDir})

	if _, err := os.Stat(filepath.Join(outDir, "graph.html")); err != nil {
		t.Fatalf("expected graph.html to be written: %v", err)
	}

	obsidianIndex := filepath.Join(vaultDir, "obsidian", "Projects", "vela", "_index.md")
	if _, err := os.Stat(obsidianIndex); err != nil {
		t.Fatalf("expected Obsidian project index to be written: %v", err)
	}
}
