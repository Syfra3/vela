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

// codebaseSource is a reusable test source for codebase nodes.
var codebaseSource = &types.Source{
	Type: types.SourceTypeCodebase,
	Name: "testproject",
	Path: "/tmp/testproject",
}

// memorySource is a reusable test source for memory nodes.
var memorySource = &types.Source{
	Type: types.SourceTypeMemory,
	Name: "ancora",
}

// TestWriteObsidian_ProjectNodes verifies codebase project + file + symbol nodes
// land under Projects/<name>/ with correct frontmatter.
func TestWriteObsidian_ProjectNodes(t *testing.T) {
	g := &types.Graph{
		Nodes: []types.Node{
			{
				ID:       "project:testproject",
				Label:    "testproject",
				NodeType: string(types.NodeTypeProject),
				Source:   codebaseSource,
				Metadata: map[string]interface{}{
					"path":   "/tmp/testproject",
					"remote": "https://github.com/org/testproject.git",
				},
			},
			{
				ID:         "testproject:file:internal/auth/middleware.go",
				Label:      "internal/auth/middleware.go",
				NodeType:   string(types.NodeTypeFile),
				SourceFile: "internal/auth/middleware.go",
				Source:     codebaseSource,
			},
			{
				ID:         "testproject:internal/auth/middleware.go:validateToken",
				Label:      "validateToken",
				NodeType:   "function",
				SourceFile: "internal/auth/middleware.go",
				Source:     codebaseSource,
			},
		},
		Edges: []types.Edge{
			{Source: "project:testproject", Target: "testproject:file:internal/auth/middleware.go", Relation: "contains"},
		},
		Communities: map[int][]string{},
		ExtractedAt: time.Now(),
	}

	outDir := t.TempDir()
	if err := WriteObsidian(g, outDir); err != nil {
		t.Fatalf("WriteObsidian error: %v", err)
	}

	vaultDir := filepath.Join(outDir, "obsidian")

	// Project root note
	projIndex := filepath.Join(vaultDir, "Projects", "testproject", "_index.md")
	if _, err := os.Stat(projIndex); err != nil {
		t.Errorf("Projects/testproject/_index.md not created: %v", err)
	}
	data, _ := os.ReadFile(projIndex)
	if !strings.Contains(string(data), `kind: "project"`) {
		t.Errorf("project root note missing kind frontmatter:\n%s", data)
	}
	if !strings.Contains(string(data), "remote") {
		t.Errorf("project root note missing remote frontmatter:\n%s", data)
	}

	// File note
	fileNote := filepath.Join(vaultDir, "Projects", "testproject", "internal_auth_middleware.go.md")
	if _, err := os.Stat(fileNote); err != nil {
		t.Errorf("Projects/testproject/internal_auth_middleware.go.md not created: %v", err)
	}

	// Symbol note — under flattened file subdirectory
	symNote := filepath.Join(vaultDir, "Projects", "testproject",
		"internal_auth_middleware.go", "validateToken.md")
	if _, err := os.Stat(symNote); err != nil {
		t.Errorf("symbol note not created at expected path: %v", err)
	}
	symData, _ := os.ReadFile(symNote)
	if !strings.Contains(string(symData), `kind: "function"`) {
		t.Errorf("symbol note missing kind frontmatter:\n%s", symData)
	}
}

// TestWriteObsidian_MemoryNodes verifies ancora observation nodes land under
// Memories/<workspace>/<visibility>/ with rich frontmatter.
func TestWriteObsidian_MemoryNodes(t *testing.T) {
	g := &types.Graph{
		Nodes: []types.Node{
			{
				ID:       "memory:ancora",
				Label:    "Ancora Memory",
				NodeType: string(types.NodeTypeMemorySource),
				Source:   memorySource,
			},
			{
				ID:       "ancora:workspace:glim",
				Label:    "glim",
				NodeType: string(types.NodeTypeWorkspace),
				Source:   memorySource,
			},
			{
				ID:       "ancora:visibility:work",
				Label:    "work",
				NodeType: string(types.NodeTypeVisibility),
				Source:   memorySource,
			},
			{
				ID:       "ancora:obs:1",
				Label:    "Fixed auth bug",
				NodeType: string(types.NodeTypeObservation),
				Source:   memorySource,
				Metadata: map[string]interface{}{
					"workspace":  "glim",
					"visibility": "work",
					"obs_type":   "bugfix",
					"topic_key":  "auth/token-expiry",
					"created_at": "2026-04-01",
				},
			},
		},
		Edges:       []types.Edge{},
		Communities: map[int][]string{},
		ExtractedAt: time.Now(),
	}

	outDir := t.TempDir()
	if err := WriteObsidian(g, outDir); err != nil {
		t.Fatalf("WriteObsidian error: %v", err)
	}

	vaultDir := filepath.Join(outDir, "obsidian")

	// Memory root note
	memRoot := filepath.Join(vaultDir, "Memories", "_root.md")
	if _, err := os.Stat(memRoot); err != nil {
		t.Errorf("Memories/_root.md not created: %v", err)
	}

	// Workspace index note
	wsNote := filepath.Join(vaultDir, "Memories", "_index", "workspace-glim.md")
	data, err := os.ReadFile(wsNote)
	if err != nil {
		t.Fatalf("Memories/_index/workspace-glim.md not created: %v", err)
	}
	if !strings.Contains(string(data), "[[Fixed auth bug]]") {
		t.Errorf("workspace index note missing obs wikilink:\n%s", data)
	}

	// Observation note under Memories/glim/work/
	obsNote := filepath.Join(vaultDir, "Memories", "glim", "work", "Fixed auth bug.md")
	obsData, err := os.ReadFile(obsNote)
	if err != nil {
		t.Fatalf("Memories/glim/work/Fixed auth bug.md not created: %v", err)
	}
	content := string(obsData)
	checks := []string{
		`obs_type: "bugfix"`,
		`workspace: "glim"`,
		`visibility: "work"`,
		`topic_key: "auth/token-expiry"`,
		"[[workspace-glim]]",
		"[[visibility-work]]",
	}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Errorf("obs note missing %q:\n%s", want, content)
		}
	}
}

// TestWriteObsidian_WritesGraphConfig verifies that calling WriteObsidian
// also produces a .obsidian/graph.json with populated colorGroups.
func TestWriteObsidian_WritesGraphConfig(t *testing.T) {
	g := &types.Graph{
		Nodes: []types.Node{
			{
				ID:       "project:vela",
				Label:    "vela",
				NodeType: string(types.NodeTypeProject),
				Source: &types.Source{
					Type: types.SourceTypeCodebase,
					Name: "vela",
				},
			},
		},
		Edges:       []types.Edge{},
		Communities: map[int][]string{},
		ExtractedAt: time.Now(),
	}

	outDir := t.TempDir()
	if err := WriteObsidian(g, outDir); err != nil {
		t.Fatalf("WriteObsidian error: %v", err)
	}

	configPath := filepath.Join(outDir, "obsidian", ".obsidian", "graph.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("graph.json not written: %v", err)
	}
	var cfg obsidianGraphConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("graph.json invalid JSON: %v", err)
	}
	if len(cfg.ColorGroups) == 0 {
		t.Error("colorGroups must not be empty")
	}
	// Must have project group
	found := false
	for _, cg := range cfg.ColorGroups {
		if strings.Contains(cg.Query, "vela") {
			found = true
			break
		}
	}
	if !found {
		t.Error("no color group for project 'vela' found in colorGroups")
	}
}

// TestWriteObsidian_FallbackPath verifies observations without workspace/visibility
// land in Memories/_unsorted/_unsorted/ instead of panicking.
func TestWriteObsidian_FallbackPath(t *testing.T) {
	g := &types.Graph{
		Nodes: []types.Node{
			{
				ID:       "ancora:obs:99",
				Label:    "Orphan observation",
				NodeType: string(types.NodeTypeObservation),
				Source:   memorySource,
				Metadata: map[string]interface{}{},
			},
		},
		Edges:       []types.Edge{},
		Communities: map[int][]string{},
		ExtractedAt: time.Now(),
	}

	outDir := t.TempDir()
	if err := WriteObsidian(g, outDir); err != nil {
		t.Fatalf("WriteObsidian error: %v", err)
	}

	fallback := filepath.Join(outDir, "obsidian", "Memories", "_unsorted", "_unsorted", "Orphan observation.md")
	if _, err := os.Stat(fallback); err != nil {
		t.Errorf("fallback path not created: %v", err)
	}
}

func TestWriteObsidian_TruncatesLongObservationNames(t *testing.T) {
	longLabel := strings.Repeat("Cross-platform Code Sharing Strategy ", 12)
	g := &types.Graph{
		Nodes: []types.Node{
			{
				ID:       "ancora:obs:long",
				Label:    longLabel,
				NodeType: string(types.NodeTypeObservation),
				Source:   memorySource,
				Metadata: map[string]interface{}{
					"workspace":  "stock-chef",
					"visibility": "work",
				},
			},
		},
		Edges:       []types.Edge{},
		Communities: map[int][]string{},
		ExtractedAt: time.Now(),
	}

	outDir := t.TempDir()
	if err := WriteObsidian(g, outDir); err != nil {
		t.Fatalf("WriteObsidian error: %v", err)
	}

	noteName := safePathComponent(longLabel) + ".md"
	if len(safePathComponent(longLabel)) > maxObsidianPathComponent {
		t.Fatalf("safePathComponent did not bound the filename")
	}

	notePath := filepath.Join(outDir, "obsidian", "Memories", "stock-chef", "work", noteName)
	if _, err := os.Stat(notePath); err != nil {
		t.Fatalf("truncated observation note not created: %v", err)
	}
	if strings.Contains(noteName, longLabel) {
		t.Fatalf("expected long label to be truncated, got %q", noteName)
	}
	if !strings.Contains(noteName, "-") {
		t.Fatalf("expected hashed suffix in %q", noteName)
	}
	if _, err := os.Stat(filepath.Join(outDir, "obsidian", ".obsidian", "graph.json")); err != nil {
		t.Fatalf("graph.json not written after long-name export: %v", err)
	}
}
