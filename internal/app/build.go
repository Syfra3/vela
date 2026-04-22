package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Syfra3/vela/internal/config"
	"github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/internal/pipeline"
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
	ObsidianPath  string
	Files         int
	Facts         int
	StageReports  []pipeline.StageReport
	Warnings      []string
	DetectedFiles []string
	Graph         *types.Graph
}

type BuildService struct {
	RunPipeline     func(context.Context, string, types.BuildRequest, pipeline.Observer) (pipeline.Result, error)
	WriteHTML       func(*types.Graph, string) error
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
	result, err := runPipeline(ctx, req.OutDir, buildReq, observer)
	if err != nil {
		return BuildResult{}, err
	}
	outDir := filepath.Dir(result.GraphPath)
	buildResult := BuildResult{
		GraphPath:     result.GraphPath,
		HTMLPath:      filepath.Join(outDir, "graph.html"),
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
		vaultDir := resolveVaultDir(req.Obsidian.VaultDir)
		buildResult.ObsidianPath = filepath.Join(vaultDir, "obsidian")
		if err := writeObsidian(result.Graph, vaultDir); err != nil {
			buildResult.Warnings = append(buildResult.Warnings, fmt.Sprintf("Obsidian export failed: %v", err))
			if req.Observe != nil {
				req.Observe(BuildEvent{Kind: BuildEventWarning, Message: buildResult.Warnings[len(buildResult.Warnings)-1]})
			}
		}
	}
	if req.Observe != nil {
		req.Observe(BuildEvent{Kind: BuildEventComplete, Message: "build complete"})
	}
	return buildResult, nil
}

func defaultRunPipeline(ctx context.Context, outDir string, req types.BuildRequest, observer pipeline.Observer) (pipeline.Result, error) {
	registry, err := scip.DefaultRegistry()
	if err != nil {
		return pipeline.Result{}, fmt.Errorf("load SCIP registry: %w", err)
	}
	builder := pipeline.NewBuilder(pipeline.Config{Registry: registry, OutDir: outDir, Observer: observer})
	return builder.Build(ctx, req)
}
