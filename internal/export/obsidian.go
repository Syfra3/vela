package export

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Syfra3/vela/pkg/types"
)

// WriteObsidian creates an Obsidian vault at outDir/obsidian/.
//
// Vault layout:
//
//	obsidian/
//	  Memories/                       ← ancora memory root
//	    _index/                       ← workspace + visibility hub notes
//	      workspace-glim.md
//	      visibility-work.md
//	    glim/                         ← one folder per workspace
//	      work/                       ← one subfolder per visibility
//	        Fixed auth bug.md         ← rich frontmatter observation note
//	      personal/
//	    ancora/
//	      work/
//	  Projects/                       ← codebase extractions
//	    vela/                         ← one folder per project
//	      _index.md                   ← project root note
//	      internal_auth_middleware.go.md
//	    glim/
//	  _root/                          ← memory_source and project root nodes
//
// Edges between memories and code (cross-source references) are written as
// wikilinks so Obsidian graph view draws connections between the two domains.
func WriteObsidian(g *types.Graph, outDir string) error {
	vaultDir := filepath.Join(outDir, "obsidian")
	if err := os.MkdirAll(vaultDir, 0755); err != nil {
		return fmt.Errorf("creating obsidian vault dir: %w", err)
	}

	// Build outgoing edge index: source node ID → []target labels.
	outEdges := make(map[string][]string, len(g.Nodes))
	for _, e := range g.Edges {
		outEdges[e.Source] = append(outEdges[e.Source], e.Target)
	}

	// Build reverse membership indexes for index notes.
	wsMembers := make(map[string][]string)  // workspace label → []obs labels
	visMembers := make(map[string][]string) // visibility label → []obs labels
	orgMembers := make(map[string][]string) // org label → []obs labels

	for _, n := range g.Nodes {
		if n.NodeType != string(types.NodeTypeObservation) {
			continue
		}
		if ws, ok := metaStr(n, "workspace"); ok && ws != "" {
			wsMembers[ws] = append(wsMembers[ws], n.Label)
		}
		if vis, ok := metaStr(n, "visibility"); ok && vis != "" {
			visMembers[vis] = append(visMembers[vis], n.Label)
		}
		if org, ok := metaStr(n, "organization"); ok && org != "" {
			orgMembers[org] = append(orgMembers[org], n.Label)
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
				err = writeRootNote(notePath, n, outEdges[n.ID])
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
				err = writeObsNote(notePath, n, outEdges[n.ID])
			}

		// ── Project root ────────────────────────────────────────────────────
		case string(types.NodeTypeProject):
			notePath, err = projectRootNotePath(vaultDir, n.Label)
			if err == nil {
				err = writeProjectRootNote(notePath, n, outEdges[n.ID])
			}

		// ── File nodes ──────────────────────────────────────────────────────
		case string(types.NodeTypeFile):
			notePath, err = projectFileNotePath(vaultDir, n)
			if err == nil {
				err = writeFileNote(notePath, n, outEdges[n.ID])
			}

		// ── Code symbols (function, struct, interface, method, etc.) ────────
		default:
			notePath, err = codeSymbolNotePath(vaultDir, n)
			if err == nil {
				err = writeSymbolNote(notePath, n, outEdges[n.ID])
			}
		}

		if err != nil {
			return fmt.Errorf("writing note for %s: %w", n.Label, err)
		}
	}

	// Write .obsidian/graph.json with colorGroups.
	if err := WriteObsidianConfig(g, outDir); err != nil {
		return fmt.Errorf("writing obsidian graph config: %w", err)
	}

	return nil
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
	return filepath.Join(dir, sanitize(kind+"-"+label)+".md"), nil
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
	dir := filepath.Join(vaultDir, "Memories", sanitize(ws), sanitize(vis))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, sanitize(n.Label)+".md"), nil
}

func projectRootNotePath(vaultDir, projectName string) (string, error) {
	dir := filepath.Join(vaultDir, "Projects", sanitize(projectName))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "_index.md"), nil
}

func projectFileNotePath(vaultDir string, n types.Node) (string, error) {
	project := projectName(n)
	dir := filepath.Join(vaultDir, "Projects", sanitize(project))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	// Flatten the file path into the filename: internal/auth/middleware.go → internal_auth_middleware.go.md
	flat := strings.ReplaceAll(sanitize(n.SourceFile), "/", "_")
	if flat == "" {
		flat = sanitize(n.Label)
	}
	return filepath.Join(dir, flat+".md"), nil
}

func codeSymbolNotePath(vaultDir string, n types.Node) (string, error) {
	project := projectName(n)
	// Group symbols under a subdirectory matching the source file (flattened).
	flat := strings.ReplaceAll(sanitize(n.SourceFile), "/", "_")
	var dir string
	if flat != "" {
		dir = filepath.Join(vaultDir, "Projects", sanitize(project), flat)
	} else {
		dir = filepath.Join(vaultDir, "Projects", sanitize(project), "_symbols")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, sanitize(n.Label)+".md"), nil
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

func writeObsNote(notePath string, n types.Node, targets []string) error {
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

	// Breadcrumb wikilinks → index hub notes.
	var crumbs []string
	if ws != "" {
		crumbs = append(crumbs, fmt.Sprintf("[[workspace-%s]]", ws))
	}
	if vis != "" {
		crumbs = append(crumbs, fmt.Sprintf("[[visibility-%s]]", vis))
	}
	if org != "" {
		crumbs = append(crumbs, fmt.Sprintf("[[organization-%s]]", org))
	}
	if len(crumbs) > 0 {
		sb.WriteString("**Context:** " + strings.Join(crumbs, " · ") + "\n\n")
	}

	writeLinks(&sb, targets)
	return os.WriteFile(notePath, []byte(sb.String()), 0644)
}

func writeProjectRootNote(notePath string, n types.Node, targets []string) error {
	path, _ := n.Metadata["path"].(string)
	remote, _ := n.Metadata["remote"].(string)

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %q\n", n.ID))
	sb.WriteString("kind: \"project\"\n")
	if path != "" {
		sb.WriteString(fmt.Sprintf("path: %q\n", path))
	}
	if remote != "" {
		sb.WriteString(fmt.Sprintf("remote: %q\n", remote))
	}
	sb.WriteString("---\n\n")
	sb.WriteString(fmt.Sprintf("# %s\n\n", n.Label))
	if n.Description != "" {
		sb.WriteString(n.Description + "\n\n")
	}
	writeLinks(&sb, targets)
	return os.WriteFile(notePath, []byte(sb.String()), 0644)
}

func writeFileNote(notePath string, n types.Node, targets []string) error {
	project := projectName(n)

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %q\n", n.ID))
	sb.WriteString("kind: \"file\"\n")
	sb.WriteString(fmt.Sprintf("project: %q\n", project))
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

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %q\n", n.ID))
	sb.WriteString(fmt.Sprintf("kind: %q\n", n.NodeType))
	sb.WriteString(fmt.Sprintf("project: %q\n", project))
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
