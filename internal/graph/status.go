package graph

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Syfra3/vela/internal/extract"
	"github.com/Syfra3/vela/internal/hooks"
	"github.com/Syfra3/vela/internal/registry"
	"github.com/Syfra3/vela/pkg/types"
)

type FreshnessStats struct {
	GraphPath         string
	ManifestPath      string
	ReportPath        string
	GraphUpdatedAt    time.Time
	ManifestUpdatedAt time.Time
	ManifestPresent   bool
	ReportPresent     bool
	TrackedFiles      int
	BuildMode         string
}

type ProjectStatus struct {
	Name          string `json:"name"`
	NodeID        string `json:"node_id"`
	Path          string `json:"path,omitempty"`
	Remote        string `json:"remote,omitempty"`
	Nodes         int    `json:"nodes"`
	Files         int    `json:"files"`
	Symbols       int    `json:"symbols"`
	OutgoingEdges int    `json:"outgoing_edges"`
}

type StatusSnapshot struct {
	GraphPath string          `json:"graph_path"`
	Metrics   HealthMetrics   `json:"metrics"`
	Freshness FreshnessStats  `json:"freshness"`
	Projects  []ProjectStatus `json:"projects"`
}

type RepoStatusSnapshot struct {
	RepoRoot      string         `json:"repo_root"`
	Name          string         `json:"name"`
	Remote        string         `json:"remote,omitempty"`
	GraphPath     string         `json:"graph_path,omitempty"`
	HookInstalled bool           `json:"hook_installed"`
	HookStatus    string         `json:"hook_status"`
	Snapshot      StatusSnapshot `json:"snapshot"`
	LoadError     string         `json:"load_error,omitempty"`
	TrackedAt     time.Time      `json:"tracked_at,omitempty"`
	ManifestPath  string         `json:"manifest_path,omitempty"`
	ReportPath    string         `json:"report_path,omitempty"`
}

type RegistrySummary struct {
	Repositories    int       `json:"repositories"`
	HealthyGraphs   int       `json:"healthy_graphs"`
	MissingGraphs   int       `json:"missing_graphs"`
	InstalledHooks  int       `json:"installed_hooks"`
	MissingHooks    int       `json:"missing_hooks"`
	MissingManifest int       `json:"missing_manifest"`
	MissingReport   int       `json:"missing_report"`
	Nodes           int       `json:"nodes"`
	Edges           int       `json:"edges"`
	BrokenEdges     int       `json:"broken_edges"`
	LatestGraphUTC  time.Time `json:"latest_graph_utc"`
}

type RegistryStatusSnapshot struct {
	Summary RegistrySummary      `json:"summary"`
	Repos   []RepoStatusSnapshot `json:"repos"`
}

func LoadStatusSnapshot(path string, topN int) (StatusSnapshot, error) {
	snapshot := StatusSnapshot{GraphPath: path}
	fresh := loadFreshnessStats(path)
	metrics, err := LoadHealthMetrics(path, topN)
	snapshot.Metrics = metrics
	snapshot.Freshness = fresh
	if err != nil {
		return snapshot, err
	}
	projects, err := loadProjectStatuses(path)
	snapshot.Projects = projects
	if err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

func LoadRegistryStatusSnapshot(entries []registry.Entry, topN int) RegistryStatusSnapshot {
	result := RegistryStatusSnapshot{Repos: make([]RepoStatusSnapshot, 0, len(entries))}
	for _, entry := range entries {
		repoStatus := RepoStatusSnapshot{
			RepoRoot:     entry.RepoRoot,
			Name:         entry.Name,
			Remote:       entry.Remote,
			GraphPath:    entry.GraphPath,
			TrackedAt:    entry.UpdatedAt,
			ManifestPath: entry.ManifestPath,
			ReportPath:   entry.ReportPath,
		}
		repoStatus.HookInstalled, repoStatus.HookStatus = loadHookState(entry.RepoRoot)
		repoStatus.Snapshot, repoStatus.LoadError = loadRepoSnapshot(entry.GraphPath, topN)
		result.Summary.Repositories++
		if repoStatus.HookInstalled {
			result.Summary.InstalledHooks++
		} else {
			result.Summary.MissingHooks++
		}
		if repoStatus.LoadError != "" {
			result.Summary.MissingGraphs++
		} else {
			result.Summary.HealthyGraphs++
		}
		if !repoStatus.Snapshot.Freshness.ManifestPresent {
			result.Summary.MissingManifest++
		}
		if !repoStatus.Snapshot.Freshness.ReportPresent {
			result.Summary.MissingReport++
		}
		result.Summary.Nodes += repoStatus.Snapshot.Metrics.Nodes
		result.Summary.Edges += repoStatus.Snapshot.Metrics.Edges
		result.Summary.BrokenEdges += repoStatus.Snapshot.Metrics.BrokenEdges
		if repoStatus.Snapshot.Freshness.GraphUpdatedAt.After(result.Summary.LatestGraphUTC) {
			result.Summary.LatestGraphUTC = repoStatus.Snapshot.Freshness.GraphUpdatedAt
		}
		result.Repos = append(result.Repos, repoStatus)
	}
	sort.Slice(result.Repos, func(i, j int) bool {
		if result.Repos[i].Name == result.Repos[j].Name {
			return result.Repos[i].RepoRoot < result.Repos[j].RepoRoot
		}
		return result.Repos[i].Name < result.Repos[j].Name
	})
	return result
}

func loadFreshnessStats(graphPath string) FreshnessStats {
	fresh := FreshnessStats{GraphPath: graphPath}
	if info, err := os.Stat(graphPath); err == nil {
		fresh.GraphUpdatedAt = info.ModTime().UTC()
	}
	outDir := filepath.Dir(graphPath)
	fresh.ManifestPath = filepath.Join(outDir, "manifest.json")
	fresh.ReportPath = filepath.Join(outDir, "GRAPH_REPORT.md")
	if info, err := os.Stat(fresh.ManifestPath); err == nil {
		fresh.ManifestPresent = true
		fresh.ManifestUpdatedAt = info.ModTime().UTC()
		if manifest, loadErr := loadManifest(fresh.ManifestPath); loadErr == nil {
			fresh.TrackedFiles = len(manifest.Files)
			fresh.BuildMode = manifest.BuildMode
			if fresh.ManifestUpdatedAt.IsZero() && !manifest.GeneratedAt.IsZero() {
				fresh.ManifestUpdatedAt = manifest.GeneratedAt.UTC()
			}
		}
	}
	if _, err := os.Stat(fresh.ReportPath); err == nil {
		fresh.ReportPresent = true
	}
	return fresh
}

func loadRepoSnapshot(graphPath string, topN int) (StatusSnapshot, string) {
	if strings.TrimSpace(graphPath) == "" {
		return StatusSnapshot{}, "graph path unavailable"
	}
	snapshot, err := LoadStatusSnapshot(graphPath, topN)
	if err != nil {
		return snapshot, err.Error()
	}
	return snapshot, ""
}

func loadHookState(repoRoot string) (bool, string) {
	if strings.TrimSpace(repoRoot) == "" {
		return false, "path unavailable"
	}
	status, err := hooks.Inspect(repoRoot)
	if err != nil {
		return false, "unavailable"
	}
	installed := status.Hooks["post-commit"] && status.Hooks["post-checkout"]
	if installed {
		return true, "installed"
	}
	if status.Hooks["post-commit"] || status.Hooks["post-checkout"] {
		return false, "partial"
	}
	return false, "missing"
}

func loadManifest(path string) (*types.Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest types.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

type statusGraph struct {
	Nodes []statusNode `json:"nodes"`
	Edges []statusEdge `json:"edges"`
}

type statusNode struct {
	ID           string                 `json:"id"`
	Label        string                 `json:"label"`
	Kind         string                 `json:"kind"`
	SourceID     string                 `json:"source_id"`
	SourceName   string                 `json:"source_name"`
	SourceOrg    string                 `json:"source_organization"`
	SourcePath   string                 `json:"source_path"`
	SourceRemote string                 `json:"source_remote"`
	Metadata     map[string]interface{} `json:"metadata"`
}

type statusEdge struct {
	From string `json:"from"`
}

func loadProjectStatuses(path string) ([]ProjectStatus, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw statusGraph
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	projectsByID := make(map[string]*ProjectStatus)
	for _, node := range raw.Nodes {
		if node.Kind != string(types.NodeTypeProject) {
			continue
		}
		projectID := statusProjectID(node)
		if projectID == "" {
			continue
		}
		projectsByID[projectID] = &ProjectStatus{
			Name:   statusProjectName(node),
			NodeID: node.ID,
			Path:   statusProjectPath(node),
			Remote: statusProjectRemote(node),
		}
	}

	ownerByNodeID := make(map[string]string, len(raw.Nodes))
	for _, node := range raw.Nodes {
		owner := statusProjectOwner(node, projectsByID)
		if owner == "" {
			continue
		}
		ownerByNodeID[node.ID] = owner
		project := projectsByID[owner]
		project.Nodes++
		switch node.Kind {
		case string(types.NodeTypeProject):
		case string(types.NodeTypeFile):
			project.Files++
		default:
			project.Symbols++
		}
	}

	for _, edge := range raw.Edges {
		if owner := ownerByNodeID[edge.From]; owner != "" {
			projectsByID[owner].OutgoingEdges++
		}
	}

	projects := make([]ProjectStatus, 0, len(projectsByID))
	for _, project := range projectsByID {
		projects = append(projects, *project)
	}
	sort.Slice(projects, func(i, j int) bool {
		if projects[i].Name == projects[j].Name {
			return projects[i].Path < projects[j].Path
		}
		return projects[i].Name < projects[j].Name
	})
	return projects, nil
}

func statusProjectOwner(node statusNode, projects map[string]*ProjectStatus) string {
	if node.Kind == string(types.NodeTypeProject) {
		if id := statusProjectID(node); id != "" {
			if _, ok := projects[id]; ok {
				return id
			}
		}
	}
	if id := strings.TrimSpace(node.SourceID); id != "" {
		if _, ok := projects[id]; ok {
			return id
		}
	}
	if idx := strings.Index(node.ID, ":"); idx > 0 {
		id := node.ID[:idx]
		if _, ok := projects[id]; ok {
			return id
		}
	}
	return ""
}

func statusProjectID(node statusNode) string {
	if node.Metadata != nil {
		if sourceID, ok := node.Metadata["source_id"].(string); ok && strings.TrimSpace(sourceID) != "" {
			return strings.TrimSpace(sourceID)
		}
	}
	if id := strings.TrimSpace(node.SourceID); id != "" {
		return id
	}
	if strings.HasPrefix(node.ID, "project:") {
		return strings.TrimPrefix(node.ID, "project:")
	}
	return strings.TrimSpace(node.Label)
}

func statusProjectPath(node statusNode) string {
	if node.Metadata != nil {
		if path, ok := node.Metadata["path"].(string); ok {
			return path
		}
	}
	return node.SourcePath
}

func statusProjectRemote(node statusNode) string {
	if node.Metadata != nil {
		if remote, ok := node.Metadata["remote"].(string); ok {
			return remote
		}
	}
	return node.SourceRemote
}

func statusProjectName(node statusNode) string {
	source := &types.Source{
		ID:           strings.TrimSpace(statusProjectID(node)),
		Name:         strings.TrimSpace(node.Label),
		Organization: strings.TrimSpace(statusProjectOrganization(node)),
	}
	if display := strings.TrimSpace(extract.SourceDisplayName(source)); display != "" {
		return display
	}
	if source.ID != "" {
		return source.ID
	}
	return source.Name
}

func statusProjectOrganization(node statusNode) string {
	if node.Metadata != nil {
		if org, ok := node.Metadata["organization"].(string); ok {
			return org
		}
	}
	return node.SourceOrg
}
