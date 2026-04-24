package graph

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadStatusSnapshotIncludesPerProjectCountsAndFreshness(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	graphPath := filepath.Join(outDir, "graph.json")
	graphJSON := `{
	  "nodes": [
	    {"id":"project:vela","label":"vela","kind":"project","source_name":"vela","source_organization":"Syfra3","source_path":"/work/vela","source_remote":"git@github.com:Syfra3/vela.git"},
	    {"id":"vela:file:main.go","label":"main.go","kind":"file","source_name":"vela"},
	    {"id":"vela:func:rootCmd","label":"rootCmd","kind":"function","source_name":"vela"},
	    {"id":"project:ancora","label":"ancora","kind":"project","source_name":"ancora","source_organization":"Syfra3","source_path":"/work/ancora","source_remote":"git@github.com:Syfra3/ancora.git"},
	    {"id":"ancora:file:main.go","label":"main.go","kind":"file","source_name":"ancora"},
	    {"id":"ancora:func:serve","label":"serve","kind":"function","source_name":"ancora"}
	  ],
	  "edges": [
	    {"from":"vela:file:main.go","to":"vela:func:rootCmd","kind":"contains","confidence":"EXTRACTED"},
	    {"from":"ancora:file:main.go","to":"ancora:func:serve","kind":"contains","confidence":"EXTRACTED"}
	  ],
	  "meta": {"nodeCount": 6, "edgeCount": 2, "generatedAt": "2026-04-23T22:47:00Z"}
	}`
	if err := os.WriteFile(graphPath, []byte(graphJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(graph.json) error = %v", err)
	}
	manifestJSON := `{
	  "version": 1,
	  "generated_at": "2026-04-23T22:47:00Z",
	  "build_mode": "full_rebuild",
	  "files": [
	    {"path":"/work/vela/main.go"},
	    {"path":"/work/ancora/main.go"}
	  ]
	}`
	if err := os.WriteFile(filepath.Join(outDir, "manifest.json"), []byte(manifestJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(manifest.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "GRAPH_REPORT.md"), []byte("report"), 0o644); err != nil {
		t.Fatalf("WriteFile(GRAPH_REPORT.md) error = %v", err)
	}

	snapshot, err := LoadStatusSnapshot(graphPath, 5)
	if err != nil {
		t.Fatalf("LoadStatusSnapshot() error = %v", err)
	}
	if snapshot.Freshness.BuildMode != "full_rebuild" {
		t.Fatalf("BuildMode = %q, want %q", snapshot.Freshness.BuildMode, "full_rebuild")
	}
	if snapshot.Freshness.TrackedFiles != 2 {
		t.Fatalf("TrackedFiles = %d, want 2", snapshot.Freshness.TrackedFiles)
	}
	if !snapshot.Freshness.ManifestPresent || !snapshot.Freshness.ReportPresent {
		t.Fatalf("expected manifest and report to be present, got %+v", snapshot.Freshness)
	}
	if snapshot.Metrics.Nodes != 6 || snapshot.Metrics.Edges != 2 {
		t.Fatalf("metrics = %+v, want 6 nodes and 2 edges", snapshot.Metrics)
	}

	projects := map[string]ProjectStatus{}
	for _, project := range snapshot.Projects {
		projects[project.Name] = project
	}
	if len(projects) != 2 {
		t.Fatalf("projects len = %d, want 2", len(projects))
	}
	if got := projects["Syfra3/vela"]; got.Path != "/work/vela" || got.Files != 1 || got.Symbols != 1 || got.Nodes != 3 || got.OutgoingEdges != 1 {
		t.Fatalf("vela project = %+v", got)
	}
	if got := projects["Syfra3/ancora"]; got.Path != "/work/ancora" || got.Files != 1 || got.Symbols != 1 || got.Nodes != 3 || got.OutgoingEdges != 1 {
		t.Fatalf("ancora project = %+v", got)
	}
	if snapshot.Freshness.GraphUpdatedAt.After(time.Now().UTC().Add(time.Minute)) {
		t.Fatalf("GraphUpdatedAt looks invalid: %v", snapshot.Freshness.GraphUpdatedAt)
	}
}

func TestLoadStatusSnapshotSeparatesProjectsWithSameNameBySourceID(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	graphPath := filepath.Join(outDir, "graph.json")
	graphJSON := `{
	  "nodes": [
	    {"id":"project:github.com/org-a/vela","label":"vela","kind":"project","source_id":"github.com/org-a/vela","source_name":"vela","source_organization":"org-a","source_path":"/work/org-a/vela"},
	    {"id":"github.com/org-a/vela:file:main.go","label":"main.go","kind":"file","source_id":"github.com/org-a/vela","source_name":"vela"},
	    {"id":"project:github.com/org-b/vela","label":"vela","kind":"project","source_id":"github.com/org-b/vela","source_name":"vela","source_organization":"org-b","source_path":"/work/org-b/vela"},
	    {"id":"github.com/org-b/vela:file:main.go","label":"main.go","kind":"file","source_id":"github.com/org-b/vela","source_name":"vela"}
	  ],
	  "edges": [
	    {"from":"github.com/org-a/vela:file:main.go","to":"project:github.com/org-a/vela","kind":"contains","confidence":"EXTRACTED"},
	    {"from":"github.com/org-b/vela:file:main.go","to":"project:github.com/org-b/vela","kind":"contains","confidence":"EXTRACTED"}
	  ],
	  "meta": {"nodeCount": 4, "edgeCount": 2, "generatedAt": "2026-04-23T22:47:00Z"}
	}`
	if err := os.WriteFile(graphPath, []byte(graphJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(graph.json) error = %v", err)
	}

	snapshot, err := LoadStatusSnapshot(graphPath, 5)
	if err != nil {
		t.Fatalf("LoadStatusSnapshot() error = %v", err)
	}
	if len(snapshot.Projects) != 2 {
		t.Fatalf("projects len = %d, want 2", len(snapshot.Projects))
	}
	if snapshot.Projects[0].Name == snapshot.Projects[1].Name {
		t.Fatalf("expected duplicate repo names to receive distinct display names: %+v", snapshot.Projects)
	}
	if snapshot.Projects[0].Path == snapshot.Projects[1].Path {
		t.Fatalf("expected duplicate repo names to remain separated by source identity: %+v", snapshot.Projects)
	}
}
