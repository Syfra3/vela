package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Syfra3/vela/internal/detect"
	"github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/internal/extract"
	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/internal/scip"
	"github.com/Syfra3/vela/pkg/types"
)

var ErrUnknownPatcher = errors.New("unknown pipeline patcher")

var codeExtensions = []string{".go", ".py", ".ts", ".tsx", ".js", ".jsx"}

var latestRelevantRepoChange = latestRelevantRepoModTime
var currentExecutableChange = executableModTime

type Scanner interface {
	Scan(root string, files []string, src *types.Source) ([]types.Node, []types.Edge, error)
}

type Patcher interface {
	Name() string
	Patch(ctx context.Context, build types.BuildRequest, facts []types.Fact) ([]types.Fact, error)
}

type StageReport struct {
	Stage types.BuildStage
	Count int
}

type StageEvent struct {
	Stage   types.BuildStage
	Count   int
	Message string
}

type Observer func(StageEvent)

type Result struct {
	Graph         *types.Graph
	Facts         []types.Fact
	Warnings      []string
	DetectedFiles []string
	GraphPath     string
	StageReports  []StageReport
}

type Config struct {
	Detect       func(root string) ([]string, error)
	Scanner      Scanner
	Registry     *scip.Registry
	Patchers     map[string]Patcher
	GraphBuilder func([]types.Node, []types.Edge) (*igraph.Graph, error)
	Cluster      func(*igraph.Graph) (map[string]int, error)
	Persist      func(*types.Graph, string) error
	OutDir       string
	Observer     Observer
}

type Builder struct {
	detect       func(root string) ([]string, error)
	scanner      Scanner
	registry     *scip.Registry
	patchers     map[string]Patcher
	graphBuilder func([]types.Node, []types.Edge) (*igraph.Graph, error)
	cluster      func(*igraph.Graph) (map[string]int, error)
	persist      func(*types.Graph, string) error
	outDir       string
	observer     Observer
}

type extractScanner struct{}

func (extractScanner) Scan(root string, files []string, src *types.Source) ([]types.Node, []types.Edge, error) {
	return extract.ExtractAll(root, files, nil, src)
}

func NewBuilder(cfg Config) *Builder {
	if cfg.Detect == nil {
		cfg.Detect = func(root string) ([]string, error) {
			return detect.Collect(root, codeExtensions)
		}
	}
	if cfg.Scanner == nil {
		cfg.Scanner = extractScanner{}
	}
	if cfg.GraphBuilder == nil {
		cfg.GraphBuilder = igraph.Build
	}
	if cfg.Persist == nil {
		cfg.Persist = export.WriteJSONAtomic
	}
	if cfg.Patchers == nil {
		cfg.Patchers = map[string]Patcher{}
	}
	return &Builder{
		detect:       cfg.Detect,
		scanner:      cfg.Scanner,
		registry:     cfg.Registry,
		patchers:     cfg.Patchers,
		graphBuilder: cfg.GraphBuilder,
		cluster:      cfg.Cluster,
		persist:      cfg.Persist,
		outDir:       cfg.OutDir,
		observer:     cfg.Observer,
	}
}

func (b *Builder) Build(ctx context.Context, req types.BuildRequest) (Result, error) {
	req = req.Normalize()
	if strings.TrimSpace(req.RepoRoot) == "" {
		return Result{}, errors.New("pipeline repo root is required")
	}
	if b == nil {
		return Result{}, errors.New("pipeline builder is nil")
	}
	outDir := b.outDir
	if strings.TrimSpace(outDir) == "" {
		outDir = filepath.Join(req.RepoRoot, ".vela")
	}
	if cached, ok, err := loadCachedResult(req, outDir); err != nil {
		return Result{}, fmt.Errorf("cache check: %w", err)
	} else if ok {
		b.emit(types.BuildStagePersist, len(cached.Graph.Edges), "reused cached graph")
		return cached, nil
	}

	source := extract.DetectProject(req.RepoRoot)
	b.emit(types.BuildStageDetect, 0, "starting detect stage")
	files, err := b.detect(req.RepoRoot)
	if err != nil {
		return Result{}, fmt.Errorf("detect stage: %w", err)
	}
	files = filterCodeFiles(files)
	b.emit(types.BuildStageDetect, len(files), "detected source files")

	if b.registry != nil {
		if err := b.registry.Bootstrap(ctx, req); err != nil {
			return Result{}, fmt.Errorf("driver bootstrap stage: %w", err)
		}
	}

	b.emit(types.BuildStageScan, 0, "starting scan stage")
	nodes, edges, err := b.scanner.Scan(req.RepoRoot, files, source)
	if err != nil {
		return Result{}, fmt.Errorf("scan stage: %w", err)
	}
	b.emit(types.BuildStageScan, len(nodes), "scanned source graph")

	b.emit(types.BuildStageDrivers, 0, "starting drivers stage")
	facts, warnings, err := b.runDrivers(ctx, req)
	if err != nil {
		return Result{}, err
	}
	b.emit(types.BuildStageDrivers, len(facts), "resolved driver facts")
	b.emit(types.BuildStagePatch, 0, "starting patch stage")
	facts, err = b.runPatchers(ctx, req, facts)
	if err != nil {
		return Result{}, err
	}
	b.emit(types.BuildStagePatch, len(facts), "patched facts")

	b.emit(types.BuildStageMerge, 0, "starting merge stage")
	nodes, edges = MergeFacts(nodes, edges, facts)
	edges = append(edges, projectFileDependencyEdges(nodes, edges)...)
	graph, err := b.graphBuilder(nodes, edges)
	if err != nil {
		return Result{}, fmt.Errorf("merge stage: %w", err)
	}
	if b.cluster != nil {
		partition, clusterErr := b.cluster(graph)
		if clusterErr != nil {
			warnings = append(warnings, clusteringWarning(clusterErr))
		} else {
			graph.ApplyCommunities(partition)
		}
	}

	tg := graph.ToTypes()
	b.emit(types.BuildStagePersist, 0, "starting persist stage")
	if err := b.persist(tg, outDir); err != nil {
		return Result{}, fmt.Errorf("persist stage: %w", err)
	}
	b.emit(types.BuildStageMerge, len(tg.Edges), "merged graph")
	b.emit(types.BuildStagePersist, 1, "persisted graph")

	return Result{
		Graph:         tg,
		Facts:         facts,
		Warnings:      append([]string(nil), warnings...),
		DetectedFiles: files,
		GraphPath:     filepath.Join(outDir, "graph.json"),
		StageReports: []StageReport{
			{Stage: types.BuildStageDetect, Count: len(files)},
			{Stage: types.BuildStageScan, Count: len(nodes)},
			{Stage: types.BuildStageDrivers, Count: len(facts)},
			{Stage: types.BuildStagePatch, Count: len(facts)},
			{Stage: types.BuildStageMerge, Count: len(tg.Edges)},
			{Stage: types.BuildStagePersist, Count: 1},
		},
	}, nil
}

func clusteringWarning(err error) string {
	message := strings.TrimSpace(err.Error())
	if errors.Is(err, igraph.ErrGraspologicMissing) {
		return "Community detection unavailable: " + message
	}
	return "Community detection failed: " + message
}

func loadCachedResult(req types.BuildRequest, outDir string) (Result, bool, error) {
	if len(req.Languages) > 0 || len(req.Drivers) > 0 || len(req.Patchers) > 0 {
		return Result{}, false, nil
	}
	graphPath := filepath.Join(outDir, "graph.json")
	info, err := os.Stat(graphPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{}, false, nil
		}
		return Result{}, false, err
	}
	if info.IsDir() || info.Size() == 0 {
		return Result{}, false, nil
	}
	latestRepoChange, err := latestRelevantRepoChange(req.RepoRoot)
	if err != nil {
		return Result{}, false, err
	}
	if !latestRepoChange.IsZero() && latestRepoChange.After(info.ModTime()) {
		return Result{}, false, nil
	}
	exeChange, err := currentExecutableChange()
	if err != nil {
		return Result{}, false, err
	}
	if !exeChange.IsZero() && exeChange.After(info.ModTime()) {
		return Result{}, false, nil
	}
	g, err := export.LoadJSON(graphPath)
	if err != nil {
		return Result{}, false, nil
	}
	fileCount := 0
	for _, node := range g.Nodes {
		if node.NodeType == string(types.NodeTypeFile) {
			fileCount++
		}
	}
	return Result{
		Graph:         g,
		GraphPath:     graphPath,
		DetectedFiles: make([]string, 0, fileCount),
		StageReports: []StageReport{
			{Stage: types.BuildStageDetect, Count: fileCount},
			{Stage: types.BuildStageScan, Count: len(g.Nodes)},
			{Stage: types.BuildStageDrivers, Count: 0},
			{Stage: types.BuildStagePatch, Count: 0},
			{Stage: types.BuildStageMerge, Count: len(g.Edges)},
			{Stage: types.BuildStagePersist, Count: 1},
		},
	}, true, nil
}

func latestRelevantRepoModTime(repoRoot string) (time.Time, error) {
	var latest time.Time
	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipRepoDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isRelevantRepoFile(path) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
		return nil
	})
	if err != nil {
		return time.Time{}, err
	}
	return latest, nil
}

func executableModTime() (time.Time, error) {
	path, err := os.Executable()
	if err != nil {
		return time.Time{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

func shouldSkipRepoDir(name string) bool {
	switch strings.TrimSpace(name) {
	case ".git", ".vela", "node_modules", "dist", "build", "coverage", "__pycache__", ".next", ".turbo", ".venv", "venv":
		return true
	default:
		return false
	}
}

func isRelevantRepoFile(path string) bool {
	base := filepath.Base(path)
	trimmed := strings.TrimSpace(base)
	if trimmed == "" {
		return false
	}
	ext := strings.ToLower(filepath.Ext(trimmed))
	if ext == ".go" || ext == ".py" || ext == ".ts" || ext == ".tsx" || ext == ".js" || ext == ".jsx" || ext == ".mts" || ext == ".cts" || ext == ".mjs" || ext == ".cjs" {
		return true
	}
	switch trimmed {
	case "go.mod", "go.sum", "go.work", "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "pyproject.toml", "requirements.txt", "setup.py", "poetry.lock":
		return true
	default:
		return strings.HasPrefix(trimmed, "tsconfig")
	}
}

func (b *Builder) emit(stage types.BuildStage, count int, message string) {
	if b == nil || b.observer == nil {
		return
	}
	b.observer(StageEvent{Stage: stage, Count: count, Message: message})
}

func filterCodeFiles(files []string) []string {
	filtered := make([]string, 0, len(files))
	for _, file := range files {
		ext := strings.ToLower(filepath.Ext(strings.TrimSpace(file)))
		for _, allowed := range codeExtensions {
			if ext == allowed {
				filtered = append(filtered, file)
				break
			}
		}
	}
	return filtered
}

func (b *Builder) runDrivers(ctx context.Context, req types.BuildRequest) ([]types.Fact, []string, error) {
	if b.registry == nil {
		return nil, nil, nil
	}
	drivers, err := b.registry.Resolve(req)
	if err != nil {
		return nil, nil, fmt.Errorf("driver stage: %w", err)
	}
	facts := make([]types.Fact, 0)
	warnings := make([]string, 0)
	for _, driver := range drivers {
		result, err := driver.Index(ctx, scip.Request{RepoRoot: req.RepoRoot, Language: driver.Language()}.Normalize())
		if err != nil {
			var missing *scip.MissingBinaryError
			if errors.As(err, &missing) {
				warnings = append(warnings, "SCIP driver unavailable: "+missing.Error())
				continue
			}
			warnings = append(warnings, scipDriverFailureWarning(driver.Name(), req.RepoRoot, err))
			continue
		}
		facts = append(facts, result.Facts...)
	}
	return facts, warnings, nil
}

func scipDriverFailureWarning(driverName, repoRoot string, err error) string {
	base := fmt.Sprintf("SCIP driver failed: %s: %v", strings.TrimSpace(driverName), err)
	message := strings.TrimSpace(err.Error())
	if strings.TrimSpace(driverName) == "scip-go" {
		if strings.Contains(message, "panic: runtime error") || strings.Contains(message, "nil module") {
			return base + ". Known scip-go incompatibility on this repo/toolchain. Try: go install github.com/sourcegraph/scip-go/cmd/scip-go@latest. If it still fails, continue with structural extraction only for this repo. (repo: " + strings.TrimSpace(repoRoot) + ")"
		}
		return base + ". Try updating scip-go: go install github.com/sourcegraph/scip-go/cmd/scip-go@latest. (repo: " + strings.TrimSpace(repoRoot) + ")"
	}
	return base
}

func projectFileDependencyEdges(nodes []types.Node, edges []types.Edge) []types.Edge {
	nodeByID := make(map[string]types.Node, len(nodes))
	firstByLabel := make(map[string]types.Node, len(nodes))
	for _, node := range nodes {
		nodeByID[node.ID] = node
		if _, ok := firstByLabel[node.Label]; !ok {
			firstByLabel[node.Label] = node
		}
	}
	projected := make([]types.Edge, 0)
	seen := make(map[string]bool)
	for _, edge := range edges {
		if edge.Relation == string(types.FactKindContains) {
			continue
		}
		fromNode, ok := nodeByID[edge.Source]
		if !ok {
			continue
		}
		fromFileNode, ok := fileNodeFor(nodeByID, firstByLabel, fromNode)
		if !ok {
			continue
		}
		toNode, ok := nodeByID[edge.Target]
		if !ok {
			toNode, ok = firstByLabel[edge.Target]
		}
		if !ok {
			continue
		}
		toFileNode, ok := fileNodeFor(nodeByID, firstByLabel, toNode)
		if !ok || fromFileNode.ID == toFileNode.ID {
			continue
		}
		projectedEdge := types.Edge{
			Source:     fromFileNode.ID,
			Target:     toFileNode.ID,
			Relation:   string(types.FactKindDependsOn),
			Confidence: edge.Confidence,
			SourceFile: fromFileNode.SourceFile,
			Metadata: map[string]interface{}{
				"projected_from":   edge.Relation,
				"projected_target": toFileNode.SourceFile,
			},
		}
		key := projectedEdge.Source + "|" + projectedEdge.Target + "|" + projectedEdge.Relation
		if seen[key] {
			continue
		}
		seen[key] = true
		projected = append(projected, projectedEdge)
	}
	return projected
}

func fileNodeFor(nodeByID map[string]types.Node, firstByLabel map[string]types.Node, node types.Node) (types.Node, bool) {
	if node.NodeType == string(types.NodeTypeFile) {
		return node, true
	}
	if strings.TrimSpace(node.SourceFile) == "" || node.Source == nil || strings.TrimSpace(node.Source.Name) == "" {
		return types.Node{}, false
	}
	fileID := node.Source.Name + ":file:" + node.SourceFile
	if fileNode, ok := nodeByID[fileID]; ok {
		return fileNode, true
	}
	if fileNode, ok := firstByLabel[node.SourceFile]; ok && fileNode.NodeType == string(types.NodeTypeFile) {
		return fileNode, true
	}
	return types.Node{}, false
}

func (b *Builder) runPatchers(ctx context.Context, req types.BuildRequest, facts []types.Fact) ([]types.Fact, error) {
	for _, name := range req.Patchers {
		patcher, ok := b.patchers[name]
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnknownPatcher, name)
		}
		next, err := patcher.Patch(ctx, req, facts)
		if err != nil {
			return nil, fmt.Errorf("patcher %s: %w", name, err)
		}
		facts = next
	}
	return facts, nil
}
