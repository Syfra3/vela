package export

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Syfra3/vela/internal/extract"
	"github.com/Syfra3/vela/pkg/types"
)

const maxObsidianPathComponent = 120

// WriteObsidian creates Obsidian vault exports under outDir/obsidian/.
//
// Repos with organization metadata are split into org-scoped vaults under
// obsidian/Organizations/<org>/ so each org can be opened as its own vault,
// with repos written directly under that org vault root.
// Repos without organization metadata keep the legacy combined layout under
// obsidian/Projects/.
//
// Each org vault duplicates memory notes so cross-source wikilinks remain
// self-contained inside that vault instead of pointing outside it.
func WriteObsidian(g *types.Graph, outDir string) error {
	rootDir := filepath.Join(outDir, "obsidian")
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return fmt.Errorf("creating obsidian vault dir: %w", err)
	}

	for _, vault := range buildObsidianVaults(g, rootDir) {
		if err := writeObsidianVault(vault.graph, vault.dir); err != nil {
			return err
		}
	}

	return nil
}

type obsidianVault struct {
	dir   string
	graph *types.Graph
}

func buildObsidianVaults(g *types.Graph, rootDir string) []obsidianVault {
	orgs := collectCodeOrganizations(g)
	vaults := make([]obsidianVault, 0, len(orgs)+1)

	if base := subgraphForObsidianOrg(g, ""); len(base.Nodes) > 0 {
		vaults = append(vaults, obsidianVault{
			dir:   rootDir,
			graph: base,
		})
	}

	for _, org := range orgs {
		vaults = append(vaults, obsidianVault{
			dir:   filepath.Join(rootDir, "Organizations", filepath.FromSlash(joinProjectSegments("", strings.Split(org, "/"), ""))),
			graph: subgraphForObsidianOrg(g, org),
		})
	}

	return vaults
}

func subgraphForObsidianOrg(g *types.Graph, org string) *types.Graph {
	selected := make(map[string]types.Node, len(g.Nodes))
	for _, n := range g.Nodes {
		if includeNodeInObsidianOrgVault(n, org) {
			selected[n.ID] = n
		}
	}

	selectedProjectFiles := make(map[string]map[string]bool)
	selectedFileSymbols := make(map[string]map[string]bool)
	for _, n := range selected {
		switch n.NodeType {
		case string(types.NodeTypeFile):
			projectKey := strings.TrimSpace(projectPathKey(n))
			fileLabel := strings.TrimSpace(n.Label)
			if projectKey != "" && fileLabel != "" {
				if selectedProjectFiles[projectKey] == nil {
					selectedProjectFiles[projectKey] = make(map[string]bool)
				}
				selectedProjectFiles[projectKey][fileLabel] = true
			}
		default:
			projectKey := strings.TrimSpace(projectPathKey(n))
			fileLabel := strings.TrimSpace(n.SourceFile)
			symbolLabel := strings.TrimSpace(n.Label)
			if n.NodeType != string(types.NodeTypeProject) && n.NodeType != string(types.NodeTypeObservation) && n.NodeType != string(types.NodeTypeWorkspace) && n.NodeType != string(types.NodeTypeVisibility) && n.NodeType != string(types.NodeTypeOrganization) && n.NodeType != string(types.NodeTypeMemorySource) && projectKey != "" && fileLabel != "" && symbolLabel != "" {
				bucketKey := obsidianFileBucketKey(projectKey, fileLabel)
				if selectedFileSymbols[bucketKey] == nil {
					selectedFileSymbols[bucketKey] = make(map[string]bool)
				}
				selectedFileSymbols[bucketKey][symbolLabel] = true
			}
		}
	}

	nodes := make([]types.Node, 0, len(selected))
	for _, n := range g.Nodes {
		if _, ok := selected[n.ID]; ok {
			nodes = append(nodes, n)
		}
	}

	edges := make([]types.Edge, 0, len(g.Edges))
	for _, e := range g.Edges {
		sourceNode, ok := selected[e.Source]
		if !ok {
			continue
		}
		if _, ok := selected[e.Target]; !ok {
			if !obsidianEdgeTargetSelected(sourceNode, e.Target, selectedProjectFiles, selectedFileSymbols) {
				continue
			}
		}
		edges = append(edges, e)
	}

	return &types.Graph{
		Nodes:       nodes,
		Edges:       edges,
		Communities: g.Communities,
		ExtractedAt: g.ExtractedAt,
	}
}

func obsidianEdgeTargetSelected(source types.Node, rawTarget string, selectedProjectFiles, selectedFileSymbols map[string]map[string]bool) bool {
	rawTarget = strings.TrimSpace(rawTarget)
	if rawTarget == "" {
		return false
	}

	switch source.NodeType {
	case string(types.NodeTypeProject):
		projectKey := strings.TrimSpace(projectPathKey(source))
		return projectKey != "" && selectedProjectFiles[projectKey][rawTarget]
	case string(types.NodeTypeFile):
		projectKey := strings.TrimSpace(projectPathKey(source))
		fileLabel := strings.TrimSpace(source.SourceFile)
		if projectKey == "" || fileLabel == "" {
			return false
		}
		return selectedFileSymbols[obsidianFileBucketKey(projectKey, fileLabel)][rawTarget]
	default:
		return false
	}
}

func includeNodeInObsidianOrgVault(n types.Node, org string) bool {
	if n.Source == nil || n.Source.Type != types.SourceTypeCodebase {
		return true
	}
	return projectOrganization(n) == org
}

func collectCodeOrganizations(g *types.Graph) []string {
	seen := make(map[string]bool)
	for _, n := range g.Nodes {
		org := projectOrganization(n)
		if org == "" {
			continue
		}
		if n.Source == nil || n.Source.Type != types.SourceTypeCodebase {
			continue
		}
		seen[org] = true
	}
	out := make([]string, 0, len(seen))
	for org := range seen {
		out = append(out, org)
	}
	sort.Strings(out)
	return out
}

func writeObsidianVault(g *types.Graph, vaultDir string) error {
	if err := os.MkdirAll(vaultDir, 0755); err != nil {
		return fmt.Errorf("creating obsidian vault dir: %w", err)
	}

	projectDirs := buildProjectNoteDirs(g)
	links, err := buildObsidianLinkIndex(vaultDir, g, projectDirs)
	if err != nil {
		return fmt.Errorf("building obsidian link index: %w", err)
	}

	// Build outgoing edge index: source node ID -> edges.
	outEdges := make(map[string][]types.Edge, len(g.Nodes))
	for _, e := range g.Edges {
		outEdges[e.Source] = append(outEdges[e.Source], e)
	}

	// Build reverse membership indexes for index notes.
	wsMembers := make(map[string][]string)  // workspace label → []obs note targets
	visMembers := make(map[string][]string) // visibility label → []obs note targets
	orgMembers := make(map[string][]string) // org label → []obs note targets

	for _, n := range g.Nodes {
		if n.NodeType != string(types.NodeTypeObservation) {
			continue
		}
		obsTarget := links.nodeTargets[n.ID]
		if obsTarget == "" {
			continue
		}
		if ws, ok := metaStr(n, "workspace"); ok && ws != "" {
			wsMembers[ws] = append(wsMembers[ws], obsTarget)
		}
		if vis, ok := metaStr(n, "visibility"); ok && vis != "" {
			visMembers[vis] = append(visMembers[vis], obsTarget)
		}
		if org, ok := metaStr(n, "organization"); ok && org != "" {
			orgMembers[org] = append(orgMembers[org], obsTarget)
		}
	}

	for _, n := range g.Nodes {
		var notePath string
		var err error

		switch n.NodeType {

		// ── Memory source root ─────────────────────────────────────────────
		case string(types.NodeTypeMemorySource):
			notePath, err = rootNotePath(vaultDir, "Memories", "_root.md")
			if err == nil {
				err = writeRootNote(notePath, n, resolveObsidianTargets(n, outEdges[n.ID], links))
			}

		// ── Workspace / visibility / org — memory hierarchy ─────────────────
		case string(types.NodeTypeWorkspace):
			notePath, err = memIndexNotePath(vaultDir, "workspace", n.Label)
			if err == nil {
				err = writeIndexNote(notePath, n, "workspace", wsMembers[n.Label])
			}

		case string(types.NodeTypeVisibility):
			notePath, err = memIndexNotePath(vaultDir, "visibility", n.Label)
			if err == nil {
				err = writeIndexNote(notePath, n, "visibility", visMembers[n.Label])
			}

		case string(types.NodeTypeOrganization):
			notePath, err = memIndexNotePath(vaultDir, "organization", n.Label)
			if err == nil {
				err = writeIndexNote(notePath, n, "organization", orgMembers[n.Label])
			}

		// ── Observations ────────────────────────────────────────────────────
		case string(types.NodeTypeObservation):
			notePath, err = obsNotePath(vaultDir, n)
			if err == nil {
				err = writeObsNote(notePath, n, obsidianBreadcrumbTargets(n, links), resolveObsidianTargets(n, outEdges[n.ID], links))
			}

		// ── Project root ────────────────────────────────────────────────────
		case string(types.NodeTypeProject):
			notePath, err = projectRootNotePath(vaultDir, n, projectDirs)
			if err == nil {
				err = writeProjectRootNote(notePath, n, resolveObsidianTargets(n, outEdges[n.ID], links))
			}

		// ── File nodes ──────────────────────────────────────────────────────
		case string(types.NodeTypeFile):
			notePath, err = projectFileNotePath(vaultDir, n, projectDirs)
			if err == nil {
				err = writeFileNote(notePath, n, resolveObsidianTargets(n, outEdges[n.ID], links))
			}

		// ── Code symbols (function, struct, interface, method, etc.) ────────
		default:
			notePath, err = codeSymbolNotePath(vaultDir, n, projectDirs)
			if err == nil {
				err = writeSymbolNote(notePath, n, resolveObsidianTargets(n, outEdges[n.ID], links))
			}
		}

		if err != nil {
			return fmt.Errorf("writing note for %s: %w", n.Label, err)
		}
	}

	// Write .obsidian/graph.json with colorGroups.
	if err := writeObsidianConfig(g, vaultDir); err != nil {
		return fmt.Errorf("writing obsidian graph config: %w", err)
	}

	return nil
}

type obsidianLinkIndex struct {
	nodeTargets         map[string]string
	projectFileTargets  map[string]map[string]string
	fileSymbolTargets   map[string]map[string]string
	workspaceTargets    map[string]string
	visibilityTargets   map[string]string
	organizationTargets map[string]string
}

func buildObsidianLinkIndex(vaultDir string, g *types.Graph, projectDirs map[string]string) (obsidianLinkIndex, error) {
	links := obsidianLinkIndex{
		nodeTargets:         make(map[string]string, len(g.Nodes)),
		projectFileTargets:  make(map[string]map[string]string),
		fileSymbolTargets:   make(map[string]map[string]string),
		workspaceTargets:    make(map[string]string),
		visibilityTargets:   make(map[string]string),
		organizationTargets: make(map[string]string),
	}

	for _, n := range g.Nodes {
		notePath, err := obsidianNotePathForNode(vaultDir, n, projectDirs)
		if err != nil {
			return obsidianLinkIndex{}, err
		}
		target := obsidianLinkTarget(vaultDir, notePath)
		links.nodeTargets[n.ID] = target

		switch n.NodeType {
		case string(types.NodeTypeFile):
			projectKey := strings.TrimSpace(projectPathKey(n))
			fileLabel := strings.TrimSpace(n.Label)
			if projectKey != "" && fileLabel != "" {
				if links.projectFileTargets[projectKey] == nil {
					links.projectFileTargets[projectKey] = make(map[string]string)
				}
				links.projectFileTargets[projectKey][fileLabel] = target
			}
		default:
			projectKey := strings.TrimSpace(projectPathKey(n))
			fileLabel := strings.TrimSpace(n.SourceFile)
			symbolLabel := strings.TrimSpace(n.Label)
			if n.NodeType != string(types.NodeTypeProject) && n.NodeType != string(types.NodeTypeObservation) && n.NodeType != string(types.NodeTypeWorkspace) && n.NodeType != string(types.NodeTypeVisibility) && n.NodeType != string(types.NodeTypeOrganization) && n.NodeType != string(types.NodeTypeMemorySource) && projectKey != "" && fileLabel != "" && symbolLabel != "" {
				bucketKey := obsidianFileBucketKey(projectKey, fileLabel)
				if links.fileSymbolTargets[bucketKey] == nil {
					links.fileSymbolTargets[bucketKey] = make(map[string]string)
				}
				links.fileSymbolTargets[bucketKey][symbolLabel] = target
			}
		}

		switch n.NodeType {
		case string(types.NodeTypeWorkspace):
			links.workspaceTargets[n.Label] = target
		case string(types.NodeTypeVisibility):
			links.visibilityTargets[n.Label] = target
		case string(types.NodeTypeOrganization):
			links.organizationTargets[n.Label] = target
		}
	}

	return links, nil
}

func obsidianNotePathForNode(vaultDir string, n types.Node, projectDirs map[string]string) (string, error) {
	switch n.NodeType {
	case string(types.NodeTypeMemorySource):
		return rootNotePath(vaultDir, "Memories", "_root.md")
	case string(types.NodeTypeWorkspace):
		return memIndexNotePath(vaultDir, "workspace", n.Label)
	case string(types.NodeTypeVisibility):
		return memIndexNotePath(vaultDir, "visibility", n.Label)
	case string(types.NodeTypeOrganization):
		return memIndexNotePath(vaultDir, "organization", n.Label)
	case string(types.NodeTypeObservation):
		return obsNotePath(vaultDir, n)
	case string(types.NodeTypeProject):
		return projectRootNotePath(vaultDir, n, projectDirs)
	case string(types.NodeTypeFile):
		return projectFileNotePath(vaultDir, n, projectDirs)
	default:
		return codeSymbolNotePath(vaultDir, n, projectDirs)
	}
}

func obsidianLinkTarget(vaultDir, notePath string) string {
	rel, err := filepath.Rel(vaultDir, notePath)
	if err != nil {
		rel = notePath
	}
	rel = filepath.ToSlash(rel)
	return strings.TrimSuffix(rel, filepath.Ext(rel))
}

func resolveObsidianTargets(source types.Node, edges []types.Edge, links obsidianLinkIndex) []string {
	resolved := make([]string, 0, len(edges))
	for _, edge := range edges {
		if target := resolveObsidianTarget(source, edge.Target, links); target != "" {
			resolved = append(resolved, target)
		}
	}
	return resolved
}

func resolveObsidianTarget(source types.Node, rawTarget string, links obsidianLinkIndex) string {
	rawTarget = strings.TrimSpace(rawTarget)
	if rawTarget == "" {
		return ""
	}
	if target := strings.TrimSpace(links.nodeTargets[rawTarget]); target != "" {
		return target
	}

	switch source.NodeType {
	case string(types.NodeTypeProject):
		projectKey := strings.TrimSpace(projectPathKey(source))
		if projectKey == "" {
			return ""
		}
		if target := strings.TrimSpace(links.projectFileTargets[projectKey][rawTarget]); target != "" {
			return target
		}
	case string(types.NodeTypeFile):
		projectKey := strings.TrimSpace(projectPathKey(source))
		fileLabel := strings.TrimSpace(source.SourceFile)
		if projectKey == "" || fileLabel == "" {
			return ""
		}
		if target := strings.TrimSpace(links.fileSymbolTargets[obsidianFileBucketKey(projectKey, fileLabel)][rawTarget]); target != "" {
			return target
		}
	}

	return ""
}

func obsidianFileBucketKey(projectKey, fileLabel string) string {
	return projectKey + "\x00" + fileLabel
}

func obsidianBreadcrumbTargets(n types.Node, links obsidianLinkIndex) []string {
	targets := make([]string, 0, 3)
	if ws, ok := metaStr(n, "workspace"); ok {
		if target := strings.TrimSpace(links.workspaceTargets[ws]); target != "" {
			targets = append(targets, target)
		}
	}
	if vis, ok := metaStr(n, "visibility"); ok {
		if target := strings.TrimSpace(links.visibilityTargets[vis]); target != "" {
			targets = append(targets, target)
		}
	}
	if org, ok := metaStr(n, "organization"); ok {
		if target := strings.TrimSpace(links.organizationTargets[org]); target != "" {
			targets = append(targets, target)
		}
	}
	return targets
}

// ── Path helpers ──────────────────────────────────────────────────────────────

func rootNotePath(vaultDir, folder, file string) (string, error) {
	dir := filepath.Join(vaultDir, folder)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, file), nil
}

func memIndexNotePath(vaultDir, kind, label string) (string, error) {
	dir := filepath.Join(vaultDir, "Memories", "_index")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, safePathComponent(kind+"-"+label)+".md"), nil
}

func obsNotePath(vaultDir string, n types.Node) (string, error) {
	ws, _ := metaStr(n, "workspace")
	vis, _ := metaStr(n, "visibility")
	if ws == "" {
		ws = "_unsorted"
	}
	if vis == "" {
		vis = "_unsorted"
	}
	dir := filepath.Join(vaultDir, "Memories", safePathComponent(ws), safePathComponent(vis))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, safePathComponent(n.Label)+".md"), nil
}

func projectRootNotePath(vaultDir string, n types.Node, projectDirs map[string]string) (string, error) {
	dir := filepath.Join(vaultDir, filepath.FromSlash(projectRelativeDir(n, projectDirs)))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "_index.md"), nil
}

func projectFileNotePath(vaultDir string, n types.Node, projectDirs map[string]string) (string, error) {
	dir := filepath.Join(vaultDir, filepath.FromSlash(projectRelativeDir(n, projectDirs)))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	// Flatten the file path into the filename: internal/auth/middleware.go → internal_auth_middleware.go.md
	flat := strings.ReplaceAll(safePathComponent(n.SourceFile), "/", "_")
	if flat == "" {
		flat = safePathComponent(n.Label)
	}
	return filepath.Join(dir, flat+".md"), nil
}

func codeSymbolNotePath(vaultDir string, n types.Node, projectDirs map[string]string) (string, error) {
	// Group symbols under a subdirectory matching the source file (flattened).
	flat := strings.ReplaceAll(safePathComponent(n.SourceFile), "/", "_")
	var dir string
	if flat != "" {
		dir = filepath.Join(vaultDir, filepath.FromSlash(projectRelativeDir(n, projectDirs)), flat)
	} else {
		dir = filepath.Join(vaultDir, filepath.FromSlash(projectRelativeDir(n, projectDirs)), "_symbols")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, safePathComponent(n.Label)+".md"), nil
}

// ── Note writers ──────────────────────────────────────────────────────────────

func writeRootNote(notePath string, n types.Node, targets []string) error {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %q\n", n.ID))
	sb.WriteString(fmt.Sprintf("kind: %q\n", n.NodeType))
	sb.WriteString("---\n\n")
	sb.WriteString(fmt.Sprintf("# %s\n\n", n.Label))
	if n.Description != "" {
		sb.WriteString(n.Description + "\n\n")
	}
	writeLinks(&sb, targets)
	return os.WriteFile(notePath, []byte(sb.String()), 0644)
}

func writeIndexNote(notePath string, n types.Node, kind string, members []string) error {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %q\n", n.ID))
	sb.WriteString(fmt.Sprintf("kind: %q\n", kind))
	sb.WriteString(fmt.Sprintf("label: %q\n", n.Label))
	sb.WriteString("---\n\n")
	sb.WriteString(fmt.Sprintf("# %s\n\n", n.Label))
	sb.WriteString(fmt.Sprintf("> **%s** — %d observation(s)\n\n", kind, len(members)))
	if len(members) > 0 {
		sb.WriteString("## Observations\n\n")
		for _, m := range members {
			sb.WriteString(fmt.Sprintf("- [[%s]]\n", m))
		}
		sb.WriteString("\n")
	}
	return os.WriteFile(notePath, []byte(sb.String()), 0644)
}

func writeObsNote(notePath string, n types.Node, crumbs []string, targets []string) error {
	ws, _ := metaStr(n, "workspace")
	vis, _ := metaStr(n, "visibility")
	org, _ := metaStr(n, "organization")
	obsType, _ := metaStr(n, "obs_type")
	topicKey, _ := metaStr(n, "topic_key")
	createdAt, _ := metaStr(n, "created_at")

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %q\n", n.ID))
	sb.WriteString("kind: \"observation\"\n")
	sb.WriteString(fmt.Sprintf("obs_type: %q\n", obsType))
	sb.WriteString(fmt.Sprintf("workspace: %q\n", ws))
	sb.WriteString(fmt.Sprintf("visibility: %q\n", vis))
	if org != "" {
		sb.WriteString(fmt.Sprintf("organization: %q\n", org))
	}
	if topicKey != "" {
		sb.WriteString(fmt.Sprintf("topic_key: %q\n", topicKey))
	}
	if createdAt != "" {
		sb.WriteString(fmt.Sprintf("created_at: %q\n", createdAt))
	}
	sb.WriteString("---\n\n")
	sb.WriteString(fmt.Sprintf("# %s\n\n", n.Label))
	if n.Description != "" {
		sb.WriteString(n.Description + "\n\n")
	}

	if len(crumbs) > 0 {
		links := make([]string, 0, len(crumbs))
		for _, crumb := range crumbs {
			links = append(links, fmt.Sprintf("[[%s]]", crumb))
		}
		sb.WriteString("**Context:** " + strings.Join(links, " · ") + "\n\n")
	}

	writeLinks(&sb, targets)
	return os.WriteFile(notePath, []byte(sb.String()), 0644)
}

func writeProjectRootNote(notePath string, n types.Node, targets []string) error {
	path, _ := n.Metadata["path"].(string)
	remote, _ := n.Metadata["remote"].(string)
	project := projectDisplayName(n)
	repo := projectName(n)
	organization := projectOrganization(n)
	title := projectRootTitle(n)

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %q\n", n.ID))
	sb.WriteString("kind: \"project\"\n")
	sb.WriteString(fmt.Sprintf("project: %q\n", project))
	sb.WriteString(fmt.Sprintf("repo: %q\n", repo))
	if organization != "" {
		sb.WriteString(fmt.Sprintf("organization: %q\n", organization))
	}
	writeTagFrontmatter(&sb, noteTags("project", repo, organization))
	if path != "" {
		sb.WriteString(fmt.Sprintf("path: %q\n", path))
	}
	if remote != "" {
		sb.WriteString(fmt.Sprintf("remote: %q\n", remote))
	}
	sb.WriteString("---\n\n")
	sb.WriteString(fmt.Sprintf("# %s\n\n", title))
	if n.Description != "" {
		sb.WriteString(n.Description + "\n\n")
	}
	writeLinks(&sb, targets)
	return os.WriteFile(notePath, []byte(sb.String()), 0644)
}

func writeFileNote(notePath string, n types.Node, targets []string) error {
	project := projectName(n)
	organization := projectOrganization(n)

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %q\n", n.ID))
	sb.WriteString("kind: \"file\"\n")
	sb.WriteString(fmt.Sprintf("project: %q\n", project))
	sb.WriteString(fmt.Sprintf("repo: %q\n", project))
	if organization != "" {
		sb.WriteString(fmt.Sprintf("organization: %q\n", organization))
	}
	writeTagFrontmatter(&sb, noteTags("file", project, organization))
	sb.WriteString(fmt.Sprintf("file: %q\n", n.SourceFile))
	sb.WriteString("---\n\n")
	sb.WriteString(fmt.Sprintf("# %s\n\n", n.Label))
	// Breadcrumb back to project root.
	sb.WriteString(fmt.Sprintf("**Project:** [[_index]]\n\n"))
	writeLinks(&sb, targets)
	return os.WriteFile(notePath, []byte(sb.String()), 0644)
}

func writeSymbolNote(notePath string, n types.Node, targets []string) error {
	project := projectName(n)
	organization := projectOrganization(n)

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %q\n", n.ID))
	sb.WriteString(fmt.Sprintf("kind: %q\n", n.NodeType))
	sb.WriteString(fmt.Sprintf("project: %q\n", project))
	sb.WriteString(fmt.Sprintf("repo: %q\n", project))
	if organization != "" {
		sb.WriteString(fmt.Sprintf("organization: %q\n", organization))
	}
	writeTagFrontmatter(&sb, noteTags(n.NodeType, project, organization))
	sb.WriteString(fmt.Sprintf("file: %q\n", n.SourceFile))
	sb.WriteString("---\n\n")
	sb.WriteString(fmt.Sprintf("# %s\n\n", n.Label))
	if n.Description != "" {
		sb.WriteString(n.Description + "\n\n")
	}
	writeLinks(&sb, targets)
	return os.WriteFile(notePath, []byte(sb.String()), 0644)
}

// ── Shared helpers ────────────────────────────────────────────────────────────

func writeLinks(sb *strings.Builder, targets []string) {
	if len(targets) == 0 {
		return
	}
	sb.WriteString("## Links\n\n")
	seen := make(map[string]bool)
	for _, t := range targets {
		if seen[t] {
			continue
		}
		seen[t] = true
		sb.WriteString(fmt.Sprintf("- [[%s]]\n", t))
	}
	sb.WriteString("\n")
}

// projectName extracts the project name from a node's Source or falls back to
// splitting the node ID prefix (e.g., "vela:internal/auth.go:Func" → "vela").
func projectName(n types.Node) string {
	if n.Source != nil && n.Source.Name != "" {
		return n.Source.Name
	}
	// Fallback: first segment of ID before ":"
	if idx := strings.Index(n.ID, ":"); idx > 0 {
		return n.ID[:idx]
	}
	return "_unknown"
}

func projectDisplayName(n types.Node) string {
	if n.Source != nil {
		if display := strings.TrimSpace(extract.SourceDisplayName(n.Source)); display != "" {
			return display
		}
	}
	if org := projectOrganization(n); org != "" {
		if name := strings.TrimSpace(projectName(n)); name != "" {
			return org + "/" + name
		}
	}
	if key := strings.TrimSpace(projectPathKey(n)); key != "" {
		return key
	}
	return projectName(n)
}

func projectRootTitle(n types.Node) string {
	if repo := strings.TrimSpace(projectName(n)); repo != "" && repo != "_unknown" {
		return repo
	}
	if display := strings.TrimSpace(projectDisplayName(n)); display != "" {
		return display
	}
	return n.Label
}

func projectOrganization(n types.Node) string {
	if n.Source != nil && strings.TrimSpace(n.Source.Organization) != "" {
		return strings.TrimSpace(n.Source.Organization)
	}
	if org, ok := metaStr(n, "organization"); ok {
		return strings.TrimSpace(org)
	}
	return ""
}

func projectPathKey(n types.Node) string {
	if n.Source != nil && n.Source.ID != "" {
		return n.Source.ID
	}
	if sourceID, ok := metaStr(n, "source_id"); ok && sourceID != "" {
		return sourceID
	}
	if n.NodeType == string(types.NodeTypeProject) && strings.HasPrefix(n.ID, "project:") {
		return strings.TrimPrefix(n.ID, "project:")
	}
	if idx := strings.Index(n.ID, ":"); idx > 0 {
		return n.ID[:idx]
	}
	return projectName(n)
}

func buildProjectNoteDirs(g *types.Graph) map[string]string {
	type projectRef struct {
		key  string
		node types.Node
	}
	refs := make(map[string]projectRef)
	for _, n := range g.Nodes {
		if n.NodeType != string(types.NodeTypeProject) && (n.Source == nil || n.Source.Type != types.SourceTypeCodebase) {
			continue
		}
		key := strings.TrimSpace(projectPathKey(n))
		if key == "" {
			key = strings.TrimSpace(projectName(n))
		}
		if key == "" {
			continue
		}
		if _, exists := refs[key]; !exists || n.NodeType == string(types.NodeTypeProject) {
			refs[key] = projectRef{key: key, node: n}
		}
	}

	preferredGroups := make(map[string][]projectRef)
	for _, ref := range refs {
		preferred := projectPreferredRelativeDir(ref.node)
		preferredGroups[preferred] = append(preferredGroups[preferred], ref)
	}

	keys := make([]string, 0, len(preferredGroups))
	for key := range preferredGroups {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	dirs := make(map[string]string, len(refs))
	for _, preferred := range keys {
		group := preferredGroups[preferred]
		if len(group) == 1 {
			dirs[group[0].key] = preferred
			continue
		}
		for _, ref := range group {
			dirs[ref.key] = projectCollisionRelativeDir(ref.node)
		}
	}
	return dirs
}

func projectRelativeDir(n types.Node, projectDirs map[string]string) string {
	key := strings.TrimSpace(projectPathKey(n))
	if key != "" {
		if rel := strings.TrimSpace(projectDirs[key]); rel != "" {
			return rel
		}
	}
	return projectPreferredRelativeDir(n)
}

func projectPreferredRelativeDir(n types.Node) string {
	name := strings.TrimSpace(projectName(n))
	if projectOrganization(n) != "" && name != "" {
		return joinProjectSegments("", nil, name)
	}
	if key := strings.TrimSpace(projectPathKey(n)); key != "" {
		return joinProjectSegments("Projects", strings.Split(key, "/"), "")
	}
	return joinProjectSegments("Projects", nil, name)
}

func projectCollisionRelativeDir(n types.Node) string {
	host := projectHost(n)
	org := projectOrganization(n)
	name := strings.TrimSpace(projectName(n))
	if host != "" && org != "" && name != "" {
		return joinProjectSegments("", append([]string{host}, strings.Split(org, "/")...), name)
	}
	if key := strings.TrimSpace(projectPathKey(n)); key != "" {
		return joinProjectSegments("Projects", strings.Split(key, "/"), "")
	}
	return joinProjectSegments("Projects", nil, name)
}

func projectHost(n types.Node) string {
	if n.Source != nil {
		parts := splitProjectPath(projectPathKey(n))
		if len(parts) > 0 && strings.Contains(parts[0], ".") {
			return parts[0]
		}
	}
	return ""
}

func joinProjectSegments(root string, parts []string, tail string) string {
	segments := make([]string, 0, len(parts)+2)
	if strings.TrimSpace(root) != "" {
		segments = append(segments, safePathComponent(root))
	}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		segments = append(segments, safePathComponent(part))
	}
	if strings.TrimSpace(tail) != "" {
		segments = append(segments, safePathComponent(tail))
	}
	if len(segments) == 0 {
		segments = append(segments, "_unknown")
	}
	return strings.Join(segments, "/")
}

func writeTagFrontmatter(sb *strings.Builder, tags []string) {
	if len(tags) == 0 {
		return
	}
	sb.WriteString("tags:\n")
	for _, tag := range tags {
		sb.WriteString(fmt.Sprintf("  - %q\n", tag))
	}
}

func noteTags(kind, repo, organization string) []string {
	tags := []string{"kind/" + sanitizeTag(kind)}
	if repo = strings.TrimSpace(repo); repo != "" {
		tags = append(tags, "repo/"+sanitizeTag(repo))
	}
	if organization = strings.TrimSpace(organization); organization != "" {
		tags = append(tags, "org/"+sanitizeTag(organization))
	}
	return tags
}

func sanitizeTag(s string) string {
	return strings.ToLower(strings.ReplaceAll(sanitize(s), "_", "-"))
}

func splitProjectPath(raw string) []string {
	parts := strings.Split(strings.TrimSpace(raw), "/")
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		clean = append(clean, part)
	}
	return clean
}

// sanitize replaces Obsidian-unfriendly filename characters.
func sanitize(s string) string {
	if s == "" {
		return "_unnamed"
	}
	r := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_",
		"?", "_", "\"", "_", "<", "_", ">", "_", "|", "_",
	)
	return r.Replace(s)
}

func safePathComponent(s string) string {
	clean := strings.TrimSpace(sanitize(s))
	clean = strings.Trim(clean, ". ")
	if clean == "" {
		clean = "_unnamed"
	}
	if len(clean) <= maxObsidianPathComponent {
		return clean
	}

	sum := sha1.Sum([]byte(clean))
	hash := hex.EncodeToString(sum[:])[:10]
	trimmed := clean[:maxObsidianPathComponent-len(hash)-1]
	trimmed = strings.TrimRight(trimmed, " ._-")
	if trimmed == "" {
		trimmed = clean[:maxObsidianPathComponent-len(hash)-1]
	}
	return trimmed + "-" + hash
}

// metaStr extracts a string value from node Metadata by key.
func metaStr(n types.Node, key string) (string, bool) {
	if n.Metadata == nil {
		return "", false
	}
	v, ok := n.Metadata[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}
