package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Syfra3/vela/internal/pipeline"
	"github.com/Syfra3/vela/pkg/types"
)

func TestBuildServiceRun_EmitsEventsAndExportsVisualArtifacts(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	vaultDir := t.TempDir()
	outDir := filepath.Join(repoRoot, ".vela-out")
	graphPath := filepath.Join(outDir, "graph.json")
	htmlPath := filepath.Join(outDir, "graph.html")
	obsidianPath := filepath.Join(vaultDir, "obsidian")

	var events []BuildEvent
	svc := BuildService{
		RunPipeline: func(_ context.Context, _ string, _ types.BuildRequest, observer pipeline.Observer) (pipeline.Result, error) {
			observer(pipeline.StageEvent{Stage: types.BuildStageDetect, Count: 2, Message: "detected source files"})
			observer(pipeline.StageEvent{Stage: types.BuildStageMerge, Count: 4, Message: "merged graph"})
			return pipeline.Result{
				Graph:         sampleGraph(),
				GraphPath:     graphPath,
				DetectedFiles: []string{"main.go", "internal/app/service.go"},
				Facts:         []types.Fact{{From: "svc", To: "db", Kind: types.FactKindDependsOn}},
				StageReports:  []pipeline.StageReport{{Stage: types.BuildStageDetect, Count: 2}, {Stage: types.BuildStageMerge, Count: 4}},
			}, nil
		},
		WriteHTML: func(g *types.Graph, out string) error {
			if out != outDir {
				t.Fatalf("html outDir = %q, want %q", out, outDir)
			}
			return nil
		},
		WriteObsidian: func(g *types.Graph, out string) error {
			if out != vaultDir {
				t.Fatalf("obsidian outDir = %q, want %q", out, vaultDir)
			}
			return nil
		},
	}

	result, err := svc.Run(context.Background(), BuildRequest{
		RepoRoot: repoRoot,
		OutDir:   outDir,
		Obsidian: types.ObsidianConfig{AutoSync: true, VaultDir: vaultDir},
		Observe: func(event BuildEvent) {
			events = append(events, event)
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.GraphPath != graphPath {
		t.Fatalf("GraphPath = %q, want %q", result.GraphPath, graphPath)
	}
	if result.HTMLPath != htmlPath {
		t.Fatalf("HTMLPath = %q, want %q", result.HTMLPath, htmlPath)
	}
	if result.ObsidianPath != obsidianPath {
		t.Fatalf("ObsidianPath = %q, want %q", result.ObsidianPath, obsidianPath)
	}
	if len(events) < 3 {
		t.Fatalf("events len = %d, want at least start/stage/complete", len(events))
	}
	if events[0].Kind != BuildEventStart {
		t.Fatalf("first event kind = %q, want %q", events[0].Kind, BuildEventStart)
	}
	if events[len(events)-1].Kind != BuildEventComplete {
		t.Fatalf("last event kind = %q, want %q", events[len(events)-1].Kind, BuildEventComplete)
	}
	if result.StageReports[0].Stage != types.BuildStageDetect {
		t.Fatalf("unexpected stage report: %+v", result.StageReports[0])
	}
}

func TestBuildServiceRun_ReportsExportFailuresWithoutHidingGraphBuild(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	vaultDir := t.TempDir()
	htmlErr := errors.New("html down")
	obsErr := errors.New("obsidian down")

	svc := BuildService{
		RunPipeline: func(context.Context, string, types.BuildRequest, pipeline.Observer) (pipeline.Result, error) {
			return pipeline.Result{Graph: sampleGraph(), GraphPath: filepath.Join(repoRoot, ".vela", "graph.json")}, nil
		},
		WriteHTML:     func(*types.Graph, string) error { return htmlErr },
		WriteObsidian: func(*types.Graph, string) error { return obsErr },
	}

	result, err := svc.Run(context.Background(), BuildRequest{RepoRoot: repoRoot, Obsidian: types.ObsidianConfig{AutoSync: true, VaultDir: vaultDir}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(result.Warnings) != 2 {
		t.Fatalf("warnings len = %d, want 2", len(result.Warnings))
	}
	if result.Warnings[0] != "HTML export failed: html down" {
		t.Fatalf("first warning = %q", result.Warnings[0])
	}
	if result.Warnings[1] != "Obsidian export failed: obsidian down" {
		t.Fatalf("second warning = %q", result.Warnings[1])
	}
}

func TestBuildServiceRun_SkipsObsidianExportWhenAutoSyncDisabled(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	called := false
	svc := BuildService{
		RunPipeline: func(context.Context, string, types.BuildRequest, pipeline.Observer) (pipeline.Result, error) {
			return pipeline.Result{Graph: sampleGraph(), GraphPath: filepath.Join(repoRoot, ".vela", "graph.json")}, nil
		},
		WriteHTML: func(*types.Graph, string) error { return nil },
		WriteObsidian: func(*types.Graph, string) error {
			called = true
			return nil
		},
	}

	result, err := svc.Run(context.Background(), BuildRequest{RepoRoot: repoRoot, Obsidian: types.ObsidianConfig{AutoSync: false, VaultDir: t.TempDir()}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if called {
		t.Fatal("expected Obsidian export to be skipped when auto sync is disabled")
	}
	if result.ObsidianPath != "" {
		t.Fatalf("ObsidianPath = %q, want empty when export is skipped", result.ObsidianPath)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings len = %d, want 0", len(result.Warnings))
	}
}

func TestBuildServiceRun_PropagatesPipelineWarnings(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	svc := BuildService{
		RunPipeline: func(context.Context, string, types.BuildRequest, pipeline.Observer) (pipeline.Result, error) {
			return pipeline.Result{
				Graph:     sampleGraph(),
				GraphPath: filepath.Join(repoRoot, ".vela", "graph.json"),
				Warnings:  []string{"SCIP driver unavailable: scip-typescript is not installed. Install it with: npm install -g @sourcegraph/scip-typescript (repo: /repo)"},
			}, nil
		},
		WriteHTML:     func(*types.Graph, string) error { return nil },
		WriteObsidian: func(*types.Graph, string) error { return nil },
	}

	result, err := svc.Run(context.Background(), BuildRequest{RepoRoot: repoRoot})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings len = %d, want 1", len(result.Warnings))
	}
	if got := result.Warnings[0]; !strings.Contains(got, "scip-typescript is not installed") {
		t.Fatalf("warning = %q, want missing binary guidance", got)
	}
}

func TestBuildServiceRun_PropagatesDriverFailureWarnings(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	svc := BuildService{
		RunPipeline: func(context.Context, string, types.BuildRequest, pipeline.Observer) (pipeline.Result, error) {
			return pipeline.Result{
				Graph:     sampleGraph(),
				GraphPath: filepath.Join(repoRoot, ".vela", "graph.json"),
				Warnings:  []string{"SCIP driver failed: scip-go: panic: nil pointer dereference"},
			}, nil
		},
		WriteHTML:     func(*types.Graph, string) error { return nil },
		WriteObsidian: func(*types.Graph, string) error { return nil },
	}

	result, err := svc.Run(context.Background(), BuildRequest{RepoRoot: repoRoot})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings len = %d, want 1", len(result.Warnings))
	}
	if got := result.Warnings[0]; !strings.Contains(got, "SCIP driver failed: scip-go") {
		t.Fatalf("warning = %q, want driver failure guidance", got)
	}
}

func sampleGraph() *types.Graph {
	return &types.Graph{
		Nodes:       []types.Node{{ID: "project:vela", Label: "vela", NodeType: string(types.NodeTypeProject)}},
		Communities: map[int][]string{},
	}
}
