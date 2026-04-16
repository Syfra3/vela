package export

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Syfra3/vela/pkg/types"
)

func TestWriteObsidianConfig_ColorGroups(t *testing.T) {
	g := &types.Graph{
		Nodes: []types.Node{
			{
				ID: "ancora:workspace:glim", Label: "glim",
				NodeType: string(types.NodeTypeWorkspace),
				Source:   &types.Source{Type: types.SourceTypeMemory, Name: "ancora"},
			},
			{
				ID: "ancora:workspace:ancora", Label: "ancora",
				NodeType: string(types.NodeTypeWorkspace),
				Source:   &types.Source{Type: types.SourceTypeMemory, Name: "ancora"},
			},
			{
				ID: "project:vela", Label: "vela",
				NodeType: string(types.NodeTypeProject),
				Source:   &types.Source{Type: types.SourceTypeCodebase, Name: "vela"},
			},
			{
				ID: "ancora:obs:1", Label: "obs1",
				NodeType: string(types.NodeTypeObservation),
				Source:   &types.Source{Type: types.SourceTypeMemory, Name: "ancora"},
				Metadata: map[string]interface{}{"workspace": "glim", "visibility": "work"},
			},
		},
		Edges:       []types.Edge{},
		Communities: map[int][]string{},
		ExtractedAt: time.Now(),
	}

	outDir := t.TempDir()
	if err := WriteObsidianConfig(g, outDir); err != nil {
		t.Fatalf("WriteObsidianConfig error: %v", err)
	}

	configPath := filepath.Join(outDir, "obsidian", ".obsidian", "graph.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("graph.json not created: %v", err)
	}

	var cfg obsidianGraphConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("graph.json not valid JSON: %v", err)
	}
	if len(cfg.ColorGroups) == 0 {
		t.Fatal("colorGroups must not be empty")
	}

	queries := make(map[string]bool)
	for _, cg := range cfg.ColorGroups {
		queries[cg.Query] = true
	}

	// Must contain memory hierarchy groups.
	if !queries["path:Memories/_root"] {
		t.Error("missing path:Memories/_root color group")
	}
	if !queries["path:Memories/_index"] {
		t.Error("missing path:Memories/_index color group")
	}

	// Per-workspace groups for glim and ancora workspaces.
	if !queries["path:Memories/glim/work"] {
		t.Error("missing path:Memories/glim/work color group")
	}
	if !queries["path:Memories/ancora/work"] {
		t.Error("missing path:Memories/ancora/work color group")
	}

	// Project group for vela.
	found := false
	for q := range queries {
		if strings.Contains(q, "vela") {
			found = true
			break
		}
	}
	if !found {
		t.Error("no color group for project 'vela'")
	}

	// All RGB values non-zero, alpha == 1.
	for _, cg := range cfg.ColorGroups {
		if cg.Color.RGB == 0 {
			t.Errorf("color group %q has zero RGB (black)", cg.Query)
		}
		if cg.Color.A != 1 {
			t.Errorf("color group %q has alpha != 1: %v", cg.Query, cg.Color.A)
		}
	}
}

// TestWriteObsidianConfig_PreservesPhysics verifies existing physics settings
// are preserved when colorGroups is updated.
func TestWriteObsidianConfig_PreservesPhysics(t *testing.T) {
	outDir := t.TempDir()
	obsidianDir := filepath.Join(outDir, "obsidian", ".obsidian")
	if err := os.MkdirAll(obsidianDir, 0755); err != nil {
		t.Fatal(err)
	}

	existing := obsidianGraphConfig{
		RepelStrength: 42,
		LinkDistance:  999,
		ColorGroups:   []obsidianColorGroup{},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(filepath.Join(obsidianDir, "graph.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	g := &types.Graph{
		Nodes: []types.Node{
			{
				ID: "ancora:workspace:test", Label: "test",
				NodeType: string(types.NodeTypeWorkspace),
				Source:   &types.Source{Type: types.SourceTypeMemory, Name: "ancora"},
			},
		},
		Edges:       []types.Edge{},
		Communities: map[int][]string{},
		ExtractedAt: time.Now(),
	}

	if err := WriteObsidianConfig(g, outDir); err != nil {
		t.Fatalf("WriteObsidianConfig error: %v", err)
	}

	result, _ := os.ReadFile(filepath.Join(obsidianDir, "graph.json"))
	var cfg obsidianGraphConfig
	json.Unmarshal(result, &cfg)

	if cfg.RepelStrength != 42 {
		t.Errorf("RepelStrength not preserved: got %v want 42", cfg.RepelStrength)
	}
	if cfg.LinkDistance != 999 {
		t.Errorf("LinkDistance not preserved: got %v want 999", cfg.LinkDistance)
	}
	if len(cfg.ColorGroups) == 0 {
		t.Error("colorGroups should have been populated")
	}
}
