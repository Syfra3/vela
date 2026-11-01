package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
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
	Snapshot      *export.Snapshot
}

type RepoBuildResult struct {
	RepoRoot   string
	GraphPath  string
	ReportPath string
}

type BuildService struct {
	RunPipeline      func(context.Context, string, types.BuildRequest, pipeline.Observer) (pipeline.Result, error)
	WriteHTML        func(*types.Graph, string) error
	WriteReport      func(*types.Graph, string) error
	WriteObsidian    func(*types.Graph, string) error
	ResolveVaultDir  func(string) string
	PublicationFault func(string) error
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
		childRepos, discoverErr := export.DiscoverChildRoots(buildReq.RepoRoot)
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
		Snapshot:      result.Snapshot,
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
	parentRoot, err := export.CanonicalRoot(req.RepoRoot)
	if err != nil {
		return BuildResult{}, err
	}
	req.RepoRoot = parentRoot
	roots, err := export.DiscoverChildRoots(parentRoot)
	if err != nil {
		return BuildResult{}, err
	}
	canonicalRoots := make([]string, 0, len(repoRoots))
	for _, root := range repoRoots {
		canonical, err := export.CanonicalRoot(root)
		if err != nil {
			return BuildResult{}, err
		}
		canonicalRoots = append(canonicalRoots, canonical)
	}
	sort.Strings(canonicalRoots)
	if len(roots) == 0 || !reflect.DeepEqual(roots, canonicalRoots) {
		return BuildResult{}, fmt.Errorf("aggregate child discovery is incomplete")
	}
	repoRoots = canonicalRoots
	buildReq := (types.BuildRequest{RepoRoot: parentRoot, Languages: req.Languages, Drivers: req.Drivers, Patchers: req.Patchers}).Normalize()
	manifestReq := types.ManifestRequest{Languages: buildReq.Languages, Drivers: buildReq.Drivers, Patchers: buildReq.Patchers}
	manifest := &types.Manifest{Version: export.InventoryVersion, RepoRoot: parentRoot, InventoryComplete: true, ExtractorFingerprint: export.ExtractorFingerprint, DiscoveryFingerprint: export.DiscoveryFingerprint, Request: manifestReq, RequestFingerprint: export.RequestFingerprint(manifestReq), Coverage: "unknown", BuildMode: "aggregate_snapshot_vector", Aggregate: &types.AggregateManifest{ParentRoot: parentRoot, DiscoveryFingerprint: export.ChildDiscoveryFingerprint, ChildRoots: roots}}
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
		repoLabel := repoWarningLabel(req.RepoRoot, repoRoot)
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
		// Validate the returned pinned directory, never the child's mutable
		// selection. A sibling writer may already have selected a newer child.
		if result.Snapshot == nil {
			return BuildResult{}, fmt.Errorf("missing validated child snapshot: %s", repoRoot)
		}
		pinned, err := export.ValidateGeneration(result.Snapshot.Dir)
		if err != nil {
			return BuildResult{}, fmt.Errorf("invalid child snapshot %s: %w", repoRoot, err)
		}
		if pinned.ID != result.Snapshot.ID || pinned.Manifest.RepoRoot != repoRoot || pinned.Manifest.RequestFingerprint != manifest.RequestFingerprint || pinned.Manifest.Aggregate != nil || result.Graph == nil || export.GraphDigest(result.Graph) != export.GraphDigest(pinned.Graph) {
			return BuildResult{}, fmt.Errorf("child graph/provenance mismatch: %s", repoRoot)
		}
		manifest.Aggregate.Children = append(manifest.Aggregate.Children, types.ChildSnapshot{Root: repoRoot, Generation: pinned.ID, GraphDigest: export.GraphDigest(pinned.Graph), Manifest: *pinned.Manifest})
		buildResult.Repos = append(buildResult.Repos, RepoBuildResult{RepoRoot: repoRoot, GraphPath: result.GraphPath, ReportPath: filepath.Join(filepath.Dir(result.GraphPath), "GRAPH_REPORT.md")})
		buildResult.Warnings = append(buildResult.Warnings, prefixRepoWarnings(repoLabel, result.Warnings)...)
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
			childGraph := export.NamespaceChild(repoRoot, pinned.Graph)
			aggregate.Nodes = append(aggregate.Nodes, childGraph.Nodes...)
			aggregate.Edges = append(aggregate.Edges, childGraph.Edges...)
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
	w, err := export.AcquireWriter(ctx, outDir)
	if err != nil {
		return BuildResult{}, err
	}
	w.Fault = s.PublicationFault
	manifest.GeneratedAt = time.Now().UTC()
	buildResult.Snapshot, err = w.Publish(aggregate, manifest, nil)
	closeErr := w.Close()
	if err != nil {
		return BuildResult{}, fmt.Errorf("persist aggregate generation: %w", err)
	}
	if closeErr != nil {
		return BuildResult{}, closeErr
	}
	// The sealed export retains reversible child provenance. Existing report
	// and Obsidian consumers need exact endpoint IDs, not prefixed bare labels.
	aggregate = aggregateAncillaryProjection(aggregate, manifest.Aggregate)
	buildResult.Graph = aggregate
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

func repoWarningLabel(root, repoRoot string) string {
	if rel, err := filepath.Rel(root, repoRoot); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.Base(repoRoot)
}

func aggregateAncillaryProjection(g *types.Graph, m *types.AggregateManifest) *types.Graph {
	data, _ := json.Marshal(g)
	var out types.Graph
	_ = json.Unmarshal(data, &out)
	ids := map[string]bool{}
	labels := map[string]string{}
	for _, n := range out.Nodes {
		ids[n.ID] = true
		for _, child := range m.Children {
			prefix := export.ChildNamespace(child.Root)
			if strings.HasPrefix(n.ID, prefix) {
				key := prefix + n.Label
				if _, ok := labels[key]; !ok {
					labels[key] = n.ID
				}
				break
			}
		}
	}
	for i, e := range out.Edges {
		if !ids[e.Source] {
			if id, ok := labels[e.Source]; ok {
				out.Edges[i].Source = id
			}
		}
		if !ids[e.Target] {
			if id, ok := labels[e.Target]; ok {
				out.Edges[i].Target = id
			}
		}
	}
	return &out
}

func prefixRepoWarnings(repoLabel string, warnings []string) []string {
	if len(warnings) == 0 {
		return nil
	}
	prefixed := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		prefixed = append(prefixed, fmt.Sprintf("%s: %s", repoLabel, warning))
	}
	return prefixed
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
	cfg, err := defaultPipelineConfig()
	if err != nil {
		return pipeline.Result{}, fmt.Errorf("load SCIP registry: %w", err)
	}
	cfg.OutDir = outDir
	cfg.Observer = observer
	builder := pipeline.NewBuilder(cfg)
	return builder.Build(ctx, req)
}

var defaultPipelineConfig = func() (pipeline.Config, error) {
	registry, err := scip.DefaultRegistry()
	return pipeline.Config{Registry: registry, Cluster: igraph.RunLeiden}, err
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
