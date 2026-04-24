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

// TestWriteObsidian_ProjectNodes verifies org-backed codebase nodes land inside
// an org-scoped vault with repo metadata and tags.
func TestWriteObsidian_ProjectNodes(t *testing.T) {
	projectSource := &types.Source{
		Type:         types.SourceTypeCodebase,
		ID:           "github.com/org/testproject",
		Name:         "testproject",
		Organization: "org",
		Path:         "/tmp/testproject",
	}
	g := &types.Graph{
		Nodes: []types.Node{
			{
				ID:       "project:testproject",
				Label:    "father",
				NodeType: string(types.NodeTypeProject),
				Source:   projectSource,
				Metadata: map[string]interface{}{
					"path":         "/tmp/testproject",
					"remote":       "https://github.com/org/testproject.git",
					"organization": "org",
				},
			},
			{
				ID:         "testproject:file:internal/auth/middleware.go",
				Label:      "internal/auth/middleware.go",
				NodeType:   string(types.NodeTypeFile),
				SourceFile: "internal/auth/middleware.go",
				Source:     projectSource,
			},
			{
				ID:         "testproject:internal/auth/middleware.go:validateToken",
				Label:      "validateToken",
				NodeType:   "function",
				SourceFile: "internal/auth/middleware.go",
				Source:     projectSource,
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
	projIndex := filepath.Join(vaultDir, "Organizations", "org", "testproject", "_index.md")
	if _, err := os.Stat(projIndex); err != nil {
		t.Errorf("Organizations/org/testproject/_index.md not created: %v", err)
	}
	data, _ := os.ReadFile(projIndex)
	content := string(data)
	if !strings.Contains(content, `kind: "project"`) {
		t.Errorf("project root note missing kind frontmatter:\n%s", data)
	}
	if !strings.Contains(content, `organization: "org"`) {
		t.Errorf("project root note missing organization frontmatter:\n%s", data)
	}
	if !strings.Contains(content, `repo: "testproject"`) {
		t.Errorf("project root note missing repo frontmatter:\n%s", data)
	}
	if !strings.Contains(content, `"kind/project"`) || !strings.Contains(content, `"repo/testproject"`) {
		t.Errorf("project root note missing filter tags:\n%s", data)
	}
	if !strings.Contains(content, "remote") {
		t.Errorf("project root note missing remote frontmatter:\n%s", data)
	}
	if !strings.Contains(content, "# testproject") {
		t.Errorf("project root note should render repo title instead of generic label:\n%s", data)
	}

	// File note
	fileNote := filepath.Join(vaultDir, "Organizations", "org", "testproject", "internal_auth_middleware.go.md")
	if _, err := os.Stat(fileNote); err != nil {
		t.Errorf("Organizations/org/testproject/internal_auth_middleware.go.md not created: %v", err)
	}
	fileData, _ := os.ReadFile(fileNote)
	if !strings.Contains(string(fileData), `repo: "testproject"`) || !strings.Contains(string(fileData), `"kind/file"`) {
		t.Errorf("file note missing repo metadata/tags:\n%s", fileData)
	}

	// Symbol note — under flattened file subdirectory
	symNote := filepath.Join(vaultDir, "Organizations", "org", "testproject",
		"internal_auth_middleware.go", "validateToken.md")
	if _, err := os.Stat(symNote); err != nil {
		t.Errorf("symbol note not created at expected path: %v", err)
	}
	symData, _ := os.ReadFile(symNote)
	if !strings.Contains(string(symData), `kind: "function"`) {
		t.Errorf("symbol note missing kind frontmatter:\n%s", symData)
	}
	if !strings.Contains(string(symData), `"repo/testproject"`) {
		t.Errorf("symbol note missing repo tag:\n%s", symData)
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
	if !strings.Contains(string(data), "[[Memories/glim/work/Fixed auth bug]]") {
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
		"[[Memories/_index/workspace-glim]]",
		"[[Memories/_index/visibility-work]]",
	}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Errorf("obs note missing %q:\n%s", want, content)
		}
	}
}

func TestWriteObsidian_ResolvesMixedGraphLinksToExportTargets(t *testing.T) {
	projectSource := &types.Source{
		Type:         types.SourceTypeCodebase,
		ID:           "github.com/org/testproject",
		Name:         "testproject",
		Organization: "org",
		Path:         "/tmp/testproject",
	}
	obsSource := &types.Source{Type: types.SourceTypeMemory, Name: "ancora"}
	projectID := "project:github.com/org/testproject"
	fileID := "github.com/org/testproject:file:internal/auth/middleware.go"
	secondFileID := "github.com/org/testproject:file:pkg/types/types.go"
	symbolID := "github.com/org/testproject:function:validateToken"
	obsID := "ancora:obs:1"
	g := &types.Graph{
		Nodes: []types.Node{
			{ID: projectID, Label: "testproject", NodeType: string(types.NodeTypeProject), Source: projectSource},
			{ID: fileID, Label: "internal/auth/middleware.go", NodeType: string(types.NodeTypeFile), SourceFile: "internal/auth/middleware.go", Source: projectSource},
			{ID: secondFileID, Label: "pkg/types/types.go", NodeType: string(types.NodeTypeFile), SourceFile: "pkg/types/types.go", Source: projectSource},
			{ID: symbolID, Label: "validateToken", NodeType: "function", SourceFile: "internal/auth/middleware.go", Source: projectSource},
			{ID: "ancora:workspace:team/platform", Label: "team/platform", NodeType: string(types.NodeTypeWorkspace), Source: obsSource},
			{ID: "ancora:visibility:work", Label: "work", NodeType: string(types.NodeTypeVisibility), Source: obsSource},
			{ID: "ancora:organization:Syfra/Platform", Label: "Syfra/Platform", NodeType: string(types.NodeTypeOrganization), Source: obsSource},
			{ID: obsID, Label: "Fix / auth links", NodeType: string(types.NodeTypeObservation), Source: obsSource, Metadata: map[string]interface{}{"workspace": "team/platform", "visibility": "work", "organization": "Syfra/Platform"}},
		},
		Edges: []types.Edge{
			{Source: projectID, Target: "internal/auth/middleware.go", Relation: "contains"},
			{Source: projectID, Target: secondFileID, Relation: "contains"},
			{Source: fileID, Target: "validateToken", Relation: "contains"},
			{Source: obsID, Target: projectID, Relation: "references"},
		},
		Communities: map[int][]string{},
		ExtractedAt: time.Now(),
	}

	outDir := t.TempDir()
	if err := WriteObsidian(g, outDir); err != nil {
		t.Fatalf("WriteObsidian error: %v", err)
	}

	vaultDir := filepath.Join(outDir, "obsidian")
	projectNote := filepath.Join(vaultDir, "Organizations", "org", "testproject", "_index.md")
	projectData, err := os.ReadFile(projectNote)
	if err != nil {
		t.Fatalf("project note read error: %v", err)
	}
	if !strings.Contains(string(projectData), "[[testproject/internal_auth_middleware.go]]") {
		t.Fatalf("project note missing resolved file link:\n%s", projectData)
	}
	if !strings.Contains(string(projectData), "[[testproject/pkg_types_types.go]]") {
		t.Fatalf("project note missing preserved ID-based file link:\n%s", projectData)
	}

	fileNote := filepath.Join(vaultDir, "Organizations", "org", "testproject", "internal_auth_middleware.go.md")
	fileData, err := os.ReadFile(fileNote)
	if err != nil {
		t.Fatalf("file note read error: %v", err)
	}
	if !strings.Contains(string(fileData), "[[testproject/internal_auth_middleware.go/validateToken]]") {
		t.Fatalf("file note missing resolved symbol link:\n%s", fileData)
	}
	if !strings.Contains(string(fileData), "**Project:** [[_index]]") {
		t.Fatalf("file note missing sanitized project breadcrumb:\n%s", fileData)
	}

	obsNote := filepath.Join(vaultDir, "Memories", "team_platform", "work", "Fix _ auth links.md")
	obsData, err := os.ReadFile(obsNote)
	if err != nil {
		t.Fatalf("obs note read error: %v", err)
	}
	for _, want := range []string{
		"[[Memories/_index/workspace-team_platform]]",
		"[[Memories/_index/visibility-work]]",
		"[[Memories/_index/organization-Syfra_Platform]]",
	} {
		if !strings.Contains(string(obsData), want) {
			t.Fatalf("obs note missing resolved target %q:\n%s", want, obsData)
		}
	}

	indexData, err := os.ReadFile(filepath.Join(vaultDir, "Memories", "_index", "workspace-team_platform.md"))
	if err != nil {
		t.Fatalf("workspace index read error: %v", err)
	}
	if !strings.Contains(string(indexData), "[[Memories/team_platform/work/Fix _ auth links]]") {
		t.Fatalf("workspace index note missing resolved observation link:\n%s", indexData)
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

func TestWriteObsidianConfig_UsesSanitizedWorkspacePaths(t *testing.T) {
	g := &types.Graph{
		Nodes: []types.Node{{
			ID:       "ancora:obs:1",
			Label:    "Obs",
			NodeType: string(types.NodeTypeObservation),
			Source:   memorySource,
			Metadata: map[string]interface{}{"workspace": "team/platform", "visibility": "work"},
		}},
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
		t.Fatalf("graph.json read error: %v", err)
	}
	if !strings.Contains(string(data), `"path:Memories/team_platform/work"`) {
		t.Fatalf("graph.json missing sanitized workspace path:\n%s", data)
	}
}

func TestWriteObsidian_SeparatesSameNamedReposBySourceID(t *testing.T) {
	g := &types.Graph{
		Nodes: []types.Node{
			{
				ID:       "project:github.com/org-a/vela",
				Label:    "vela",
				NodeType: string(types.NodeTypeProject),
				Source:   &types.Source{Type: types.SourceTypeCodebase, ID: "github.com/org-a/vela", Name: "vela", Organization: "org-a"},
			},
			{
				ID:       "project:github.com/org-b/vela",
				Label:    "vela",
				NodeType: string(types.NodeTypeProject),
				Source:   &types.Source{Type: types.SourceTypeCodebase, ID: "github.com/org-b/vela", Name: "vela", Organization: "org-b"},
			},
		},
		Edges:       []types.Edge{},
		Communities: map[int][]string{},
		ExtractedAt: time.Now(),
	}

	outDir := t.TempDir()
	if err := WriteObsidian(g, outDir); err != nil {
		t.Fatalf("WriteObsidian() error = %v", err)
	}

	for _, path := range []string{
		filepath.Join(outDir, "obsidian", "Organizations", "org-a", "vela", "_index.md"),
		filepath.Join(outDir, "obsidian", "Organizations", "org-b", "vela", "_index.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}
}

func TestWriteObsidian_SplitsOrganizationVaults(t *testing.T) {
	g := &types.Graph{
		Nodes: []types.Node{
			{
				ID:       "project:github.com/org-a/vela",
				Label:    "vela",
				NodeType: string(types.NodeTypeProject),
				Source:   &types.Source{Type: types.SourceTypeCodebase, ID: "github.com/org-a/vela", Name: "vela", Organization: "org-a"},
			},
			{
				ID:       "project:github.com/org-b/docs",
				Label:    "docs",
				NodeType: string(types.NodeTypeProject),
				Source:   &types.Source{Type: types.SourceTypeCodebase, ID: "github.com/org-b/docs", Name: "docs", Organization: "org-b"},
			},
		},
		Edges:       []types.Edge{},
		Communities: map[int][]string{},
		ExtractedAt: time.Now(),
	}

	outDir := t.TempDir()
	if err := WriteObsidian(g, outDir); err != nil {
		t.Fatalf("WriteObsidian() error = %v", err)
	}

	for _, path := range []string{
		filepath.Join(outDir, "obsidian", "Organizations", "org-a", ".obsidian", "graph.json"),
		filepath.Join(outDir, "obsidian", "Organizations", "org-b", ".obsidian", "graph.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}

	if _, err := os.Stat(filepath.Join(outDir, "obsidian", ".obsidian", "graph.json")); !os.IsNotExist(err) {
		t.Fatalf("expected no combined root vault config for org-only export, got err=%v", err)
	}
}

func TestWriteObsidian_FallsBackToProjectSegmentsWithoutOrganization(t *testing.T) {
	g := &types.Graph{
		Nodes: []types.Node{{
			ID:       "project:team-a/vela",
			Label:    "vela",
			NodeType: string(types.NodeTypeProject),
			Source:   &types.Source{Type: types.SourceTypeCodebase, ID: "team-a/vela", Name: "vela"},
		}},
		Edges:       []types.Edge{},
		Communities: map[int][]string{},
		ExtractedAt: time.Now(),
	}

	outDir := t.TempDir()
	if err := WriteObsidian(g, outDir); err != nil {
		t.Fatalf("WriteObsidian() error = %v", err)
	}

	path := filepath.Join(outDir, "obsidian", "Projects", "team-a", "vela", "_index.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
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
