package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Syfra3/vela/internal/config"
	"github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/internal/extract"
	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/internal/pipeline"
	"github.com/Syfra3/vela/internal/report"
	"github.com/Syfra3/vela/internal/scip"
	"github.com/Syfra3/vela/pkg/types"
)

type BuildEventKind string

const (
	BuildEventStart    BuildEventKind = "start"
	BuildEventStage    BuildEventKind = "stage"
	BuildEventWarning  BuildEventKind = "warning"
	BuildEventComplete BuildEventKind = "complete"
)

type BuildEvent struct {
	Kind    BuildEventKind
	Stage   types.BuildStage
	Message string
	Count   int
}

type BuildRequest struct {
	RepoRoot  string
	OutDir    string
	Languages []string
	Drivers   []string
	Patchers  []string
	Obsidian  types.ObsidianConfig
	Observe   func(BuildEvent)
}

type BuildResult struct {
	GraphPath     string
	HTMLPath      string
	ReportPath    string
	ObsidianPath  string
	Files         int
	Facts         int
	StageReports  []pipeline.StageReport
	Warnings      []string
	DetectedFiles []string
	Graph         *types.Graph
	Repos         []RepoBuildResult
}

type RepoBuildResult struct {
	RepoRoot   string
	GraphPath  string
	ReportPath string
}

type BuildService struct {
	RunPipeline     func(context.Context, string, types.BuildRequest, pipeline.Observer) (pipeline.Result, error)
	WriteHTML       func(*types.Graph, string) error
	WriteReport     func(*types.Graph, string) error
	WriteObsidian   func(*types.Graph, string) error
	ResolveVaultDir func(string) string
}

func (s BuildService) Run(ctx context.Context, req BuildRequest) (BuildResult, error) {
	if strings.TrimSpace(req.RepoRoot) == "" {
		return BuildResult{}, fmt.Errorf("repository path is required")
	}
	runPipeline := s.RunPipeline
	if runPipeline == nil {
		runPipeline = defaultRunPipeline
	}
	writeHTML := s.WriteHTML
	if writeHTML == nil {
		writeHTML = export.WriteHTML
	}
	writeReport := s.WriteReport
	if writeReport == nil {
		writeReport = defaultWriteReport
	}
	writeObsidian := s.WriteObsidian
	if writeObsidian == nil {
		writeObsidian = export.WriteObsidian
	}
	resolveVaultDir := s.ResolveVaultDir
	if resolveVaultDir == nil {
		resolveVaultDir = config.ResolveVaultDir
	}
	observer := func(event pipeline.StageEvent) {
		if req.Observe != nil {
			req.Observe(BuildEvent{Kind: BuildEventStage, Stage: event.Stage, Message: event.Message, Count: event.Count})
		}
	}
	if req.Observe != nil {
		req.Observe(BuildEvent{Kind: BuildEventStart, Message: "build started"})
	}
	buildReq := types.BuildRequest{
		RepoRoot:  req.RepoRoot,
		Languages: req.Languages,
		Drivers:   req.Drivers,
		Patchers:  req.Patchers,
		Stages:    nil,
	}.Normalize()
	if !extract.IsGitRepoRoot(buildReq.RepoRoot) {
		childRepos, discoverErr := extract.DiscoverChildGitRepos(buildReq.RepoRoot)
		if discoverErr != nil {
			return BuildResult{}, fmt.Errorf("discover child repos: %w", discoverErr)
		}
		if len(childRepos) > 0 {
			return s.runMultiRepo(ctx, req, childRepos, runPipeline)
		}
	}
	result, err := runPipeline(ctx, req.OutDir, buildReq, observer)
	if err != nil {
		return BuildResult{}, err
	}
	outDir := filepath.Dir(result.GraphPath)
	buildResult := BuildResult{
		GraphPath:     result.GraphPath,
		HTMLPath:      filepath.Join(outDir, "graph.html"),
		ReportPath:    filepath.Join(outDir, "GRAPH_REPORT.md"),
		Files:         len(result.DetectedFiles),
		Facts:         len(result.Facts),
		StageReports:  append([]pipeline.StageReport(nil), result.StageReports...),
		Warnings:      append([]string(nil), result.Warnings...),
		DetectedFiles: append([]string(nil), result.DetectedFiles...),
		Graph:         result.Graph,
	}
	if result.Graph != nil {
		if err := writeHTML(result.Graph, outDir); err != nil {
			buildResult.Warnings = append(buildResult.Warnings, fmt.Sprintf("HTML export failed: %v", err))
			if req.Observe != nil {
				req.Observe(BuildEvent{Kind: BuildEventWarning, Message: buildResult.Warnings[len(buildResult.Warnings)-1]})
			}
		}
		if err := writeReport(result.Graph, outDir); err != nil {
			buildResult.Warnings = append(buildResult.Warnings, fmt.Sprintf("Graph report export failed: %v", err))
			if req.Observe != nil {
				req.Observe(BuildEvent{Kind: BuildEventWarning, Message: buildResult.Warnings[len(buildResult.Warnings)-1]})
			}
		}
		if req.Obsidian.AutoSync {
			vaultDir := resolveVaultDir(req.Obsidian.VaultDir)
			buildResult.ObsidianPath = filepath.Join(vaultDir, "obsidian")
			if err := writeObsidian(result.Graph, vaultDir); err != nil {
				buildResult.Warnings = append(buildResult.Warnings, fmt.Sprintf("Obsidian export failed: %v", err))
				if req.Observe != nil {
					req.Observe(BuildEvent{Kind: BuildEventWarning, Message: buildResult.Warnings[len(buildResult.Warnings)-1]})
				}
			}
		}
	}
	if req.Observe != nil {
		req.Observe(BuildEvent{Kind: BuildEventComplete, Message: "build complete"})
	}
	return buildResult, nil
}

func (s BuildService) runMultiRepo(
	ctx context.Context,
	req BuildRequest,
	repoRoots []string,
	runPipeline func(context.Context, string, types.BuildRequest, pipeline.Observer) (pipeline.Result, error),
) (BuildResult, error) {
	writeHTML := s.WriteHTML
	if writeHTML == nil {
		writeHTML = export.WriteHTML
	}
	writeReport := s.WriteReport
	if writeReport == nil {
		writeReport = defaultWriteReport
	}
	writeObsidian := s.WriteObsidian
	if writeObsidian == nil {
		writeObsidian = export.WriteObsidian
	}
	resolveVaultDir := s.ResolveVaultDir
	if resolveVaultDir == nil {
		resolveVaultDir = config.ResolveVaultDir
	}
	stageTotals := map[types.BuildStage]int{}
	aggregate := &types.Graph{}
	buildResult := BuildResult{Repos: make([]RepoBuildResult, 0, len(repoRoots))}
	for _, repoRoot := range repoRoots {
		observer := func(event pipeline.StageEvent) {
			if req.Observe != nil {
				req.Observe(BuildEvent{Kind: BuildEventStage, Stage: event.Stage, Message: fmt.Sprintf("%s: %s", filepath.Base(repoRoot), event.Message), Count: event.Count})
			}
		}
		result, err := runPipeline(ctx, "", types.BuildRequest{
			RepoRoot:  repoRoot,
			Languages: req.Languages,
			Drivers:   req.Drivers,
			Patchers:  req.Patchers,
		}.Normalize(), observer)
		if err != nil {
			return BuildResult{}, err
		}
		buildResult.Repos = append(buildResult.Repos, RepoBuildResult{RepoRoot: repoRoot, GraphPath: result.GraphPath, ReportPath: filepath.Join(filepath.Dir(result.GraphPath), "GRAPH_REPORT.md")})
		buildResult.Warnings = append(buildResult.Warnings, result.Warnings...)
		buildResult.DetectedFiles = append(buildResult.DetectedFiles, result.DetectedFiles...)
		buildResult.Facts += len(result.Facts)
		for _, report := range result.StageReports {
			stageTotals[report.Stage] += report.Count
		}
		if result.Graph != nil {
			repoOutDir := filepath.Dir(result.GraphPath)
			if err := writeReport(result.Graph, repoOutDir); err != nil {
				buildResult.Warnings = append(buildResult.Warnings, fmt.Sprintf("Graph report export failed for %s: %v", filepath.Base(repoRoot), err))
			}
			aggregate.Nodes = append(aggregate.Nodes, result.Graph.Nodes...)
			aggregate.Edges = append(aggregate.Edges, result.Graph.Edges...)
		}
	}
	aggregate.ExtractedAt = time.Now().UTC()
	aggregate.Nodes, aggregate.Edges = igraph.Canonicalize(aggregate.Nodes, aggregate.Edges)
	buildResult.Graph = aggregate
	buildResult.Files = len(buildResult.DetectedFiles)
	outDir := req.OutDir
	if strings.TrimSpace(outDir) == "" {
		outDir = filepath.Join(req.RepoRoot, ".vela")
	}
	if err := export.WriteJSONAtomic(aggregate, outDir); err != nil {
		return BuildResult{}, fmt.Errorf("persist aggregate graph: %w", err)
	}
	buildResult.GraphPath = filepath.Join(outDir, "graph.json")
	buildResult.HTMLPath = filepath.Join(outDir, "graph.html")
	buildResult.ReportPath = filepath.Join(outDir, "GRAPH_REPORT.md")
	if err := writeHTML(aggregate, outDir); err != nil {
		buildResult.Warnings = append(buildResult.Warnings, fmt.Sprintf("HTML export failed: %v", err))
	}
	if err := writeReport(aggregate, outDir); err != nil {
		buildResult.Warnings = append(buildResult.Warnings, fmt.Sprintf("Graph report export failed: %v", err))
	}
	if req.Obsidian.AutoSync {
		vaultDir := resolveVaultDir(req.Obsidian.VaultDir)
		buildResult.ObsidianPath = filepath.Join(vaultDir, "obsidian")
		if err := writeObsidian(aggregate, vaultDir); err != nil {
			buildResult.Warnings = append(buildResult.Warnings, fmt.Sprintf("Obsidian export failed: %v", err))
		}
	}
	buildResult.StageReports = summarizeStageReports(stageTotals)
	if req.Observe != nil {
		req.Observe(BuildEvent{Kind: BuildEventComplete, Message: fmt.Sprintf("build complete (%d repos)", len(repoRoots))})
	}
	return buildResult, nil
}

func summarizeStageReports(totals map[types.BuildStage]int) []pipeline.StageReport {
	order := []types.BuildStage{
		types.BuildStageDetect,
		types.BuildStageScan,
		types.BuildStageDrivers,
		types.BuildStagePatch,
		types.BuildStageMerge,
		types.BuildStagePersist,
	}
	reports := make([]pipeline.StageReport, 0, len(order))
	for _, stage := range order {
		reports = append(reports, pipeline.StageReport{Stage: stage, Count: totals[stage]})
	}
	return reports
}
func defaultRunPipeline(ctx context.Context, outDir string, req types.BuildRequest, observer pipeline.Observer) (pipeline.Result, error) {
	registry, err := scip.DefaultRegistry()
	if err != nil {
		return pipeline.Result{}, fmt.Errorf("load SCIP registry: %w", err)
	}
	builder := pipeline.NewBuilder(pipeline.Config{Registry: registry, OutDir: outDir, Observer: observer, Cluster: igraph.RunLeiden})
	return builder.Build(ctx, req)
}

func defaultWriteReport(g *types.Graph, outDir string) error {
	if g == nil {
		return fmt.Errorf("graph is nil")
	}
	graph, err := igraph.Build(g.Nodes, g.Edges)
	if err != nil {
		return fmt.Errorf("build graph report view: %w", err)
	}
	return report.Generate(graph, outDir)
}
