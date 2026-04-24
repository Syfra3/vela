package app

import (
	"context"
	"errors"
	"os"
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
	reportPath := filepath.Join(outDir, "GRAPH_REPORT.md")
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
		WriteReport: func(g *types.Graph, out string) error {
			if out != outDir {
				t.Fatalf("report outDir = %q, want %q", out, outDir)
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
	if result.ReportPath != reportPath {
		t.Fatalf("ReportPath = %q, want %q", result.ReportPath, reportPath)
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
	reportErr := errors.New("report down")

	svc := BuildService{
		RunPipeline: func(context.Context, string, types.BuildRequest, pipeline.Observer) (pipeline.Result, error) {
			return pipeline.Result{Graph: sampleGraph(), GraphPath: filepath.Join(repoRoot, ".vela", "graph.json")}, nil
		},
		WriteHTML:     func(*types.Graph, string) error { return htmlErr },
		WriteReport:   func(*types.Graph, string) error { return reportErr },
		WriteObsidian: func(*types.Graph, string) error { return obsErr },
	}

	result, err := svc.Run(context.Background(), BuildRequest{RepoRoot: repoRoot, Obsidian: types.ObsidianConfig{AutoSync: true, VaultDir: vaultDir}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(result.Warnings) != 3 {
		t.Fatalf("warnings len = %d, want 3", len(result.Warnings))
	}
	if result.Warnings[0] != "HTML export failed: html down" {
		t.Fatalf("first warning = %q", result.Warnings[0])
	}
	if result.Warnings[1] != "Graph report export failed: report down" {
		t.Fatalf("second warning = %q", result.Warnings[1])
	}
	if result.Warnings[2] != "Obsidian export failed: obsidian down" {
		t.Fatalf("third warning = %q", result.Warnings[2])
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
		WriteHTML:   func(*types.Graph, string) error { return nil },
		WriteReport: func(*types.Graph, string) error { return nil },
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
		WriteReport:   func(*types.Graph, string) error { return nil },
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
		WriteReport:   func(*types.Graph, string) error { return nil },
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

func TestBuildServiceRun_MergesChildGitReposUnderNonRepoRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repoA := filepath.Join(root, "org-a", "vela")
	repoB := filepath.Join(root, "org-b", "vela")
	for _, repo := range []string{repoA, repoB} {
		if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
			t.Fatalf("mkdir .git for %q: %v", repo, err)
		}
	}

	var seen []string
	svc := BuildService{
		RunPipeline: func(_ context.Context, _ string, req types.BuildRequest, _ pipeline.Observer) (pipeline.Result, error) {
			seen = append(seen, req.RepoRoot)
			repoName := filepath.Base(req.RepoRoot)
			projectID := filepath.ToSlash(strings.TrimPrefix(req.RepoRoot, root+string(filepath.Separator)))
			graphPath := filepath.Join(req.RepoRoot, ".vela", "graph.json")
			return pipeline.Result{
				GraphPath:     graphPath,
				DetectedFiles: []string{filepath.Join(req.RepoRoot, "main.go")},
				Facts:         []types.Fact{{From: projectID + ":a", To: projectID + ":b", Kind: types.FactKindDependsOn}},
				Graph:         &types.Graph{Nodes: []types.Node{{ID: "project:" + projectID, Label: repoName, NodeType: string(types.NodeTypeProject), Source: &types.Source{Type: types.SourceTypeCodebase, ID: projectID, Name: repoName, Path: req.RepoRoot}}}},
				StageReports:  []pipeline.StageReport{{Stage: types.BuildStageDetect, Count: 1}, {Stage: types.BuildStagePersist, Count: 1}},
			}, nil
		},
		WriteHTML:     func(*types.Graph, string) error { return nil },
		WriteReport:   func(*types.Graph, string) error { return nil },
		WriteObsidian: func(*types.Graph, string) error { return nil },
	}

	result, err := svc.Run(context.Background(), BuildRequest{RepoRoot: root})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(seen) != 2 {
		t.Fatalf("runPipeline calls = %v, want 2 child repos", seen)
	}
	if len(result.Repos) != 2 {
		t.Fatalf("Repos len = %d, want 2", len(result.Repos))
	}
	if result.GraphPath != filepath.Join(root, ".vela", "graph.json") {
		t.Fatalf("GraphPath = %q, want aggregate graph under selected root", result.GraphPath)
	}
	if result.Files != 2 {
		t.Fatalf("Files = %d, want 2", result.Files)
	}
	if len(result.Graph.Nodes) != 2 {
		t.Fatalf("merged graph nodes = %d, want 2", len(result.Graph.Nodes))
	}
}

func TestBuildServiceRun_WritesPerRepoReportsForMultiRepoBuilds(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repoA := filepath.Join(root, "org-a", "vela")
	repoB := filepath.Join(root, "org-b", "ancora")
	for _, repo := range []string{repoA, repoB} {
		if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
			t.Fatalf("mkdir .git for %q: %v", repo, err)
		}
	}

	var reportOutDirs []string
	svc := BuildService{
		RunPipeline: func(_ context.Context, _ string, req types.BuildRequest, _ pipeline.Observer) (pipeline.Result, error) {
			graphPath := filepath.Join(req.RepoRoot, ".vela", "graph.json")
			return pipeline.Result{
				GraphPath: graphPath,
				Graph:     &types.Graph{Nodes: []types.Node{{ID: req.RepoRoot, Label: filepath.Base(req.RepoRoot), NodeType: string(types.NodeTypeProject)}}},
			}, nil
		},
		WriteHTML: func(*types.Graph, string) error { return nil },
		WriteReport: func(_ *types.Graph, out string) error {
			reportOutDirs = append(reportOutDirs, out)
			return nil
		},
		WriteObsidian: func(*types.Graph, string) error { return nil },
	}

	result, err := svc.Run(context.Background(), BuildRequest{RepoRoot: root})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(result.Repos) != 2 {
		t.Fatalf("Repos len = %d, want 2", len(result.Repos))
	}
	wantDirs := map[string]bool{
		filepath.Join(repoA, ".vela"): true,
		filepath.Join(repoB, ".vela"): true,
		filepath.Join(root, ".vela"):  true,
	}
	if len(reportOutDirs) != len(wantDirs) {
		t.Fatalf("WriteReport calls = %v, want %d calls", reportOutDirs, len(wantDirs))
	}
	for _, outDir := range reportOutDirs {
		delete(wantDirs, outDir)
	}
	if len(wantDirs) != 0 {
		t.Fatalf("missing report exports for outDirs: %+v", wantDirs)
	}
	for _, repo := range result.Repos {
		want := filepath.Join(filepath.Dir(repo.GraphPath), "GRAPH_REPORT.md")
		if repo.ReportPath != want {
			t.Fatalf("repo.ReportPath = %q, want %q", repo.ReportPath, want)
		}
	}
}

func sampleGraph() *types.Graph {
	return &types.Graph{
		Nodes:       []types.Node{{ID: "project:vela", Label: "vela", NodeType: string(types.NodeTypeProject)}},
		Communities: map[int][]string{},
	}
}
