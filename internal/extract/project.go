package extract

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Syfra3/vela/pkg/types"
)

// DetectProject extracts project metadata from a directory.
// Priority:
//  1. Git remote URL → extract repo name (e.g., "github.com/Syfra3/vela" → "vela")
//  2. Fall back to directory basename
//
// Returns a Source with Type=codebase, Name, Path, and Remote (if git).
func DetectProject(dir string) *types.Source {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}

	src := &types.Source{
		Type: types.SourceTypeCodebase,
		Name: filepath.Base(absDir),
		Path: absDir,
	}

	// Try git remote
	remote := gitRemoteURL(absDir)
	if remote != "" {
		src.Remote = remote
		if name := extractRepoName(remote); name != "" {
			src.Name = name
		}
	}

	return src
}

// gitRemoteURL runs `git remote get-url origin` in the given directory.
// Returns empty string on any error (not a git repo, no origin, etc.).
func gitRemoteURL(dir string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// extractRepoName extracts the repository name from a git remote URL.
// Handles:
//   - https://github.com/Syfra3/vela.git  → "vela"
//   - git@github.com:Syfra3/vela.git      → "vela"
//   - https://github.com/Syfra3/vela      → "vela"
//   - /path/to/local/repo.git             → "repo"
func extractRepoName(remote string) string {
	// Remove trailing .git
	remote = strings.TrimSuffix(remote, ".git")

	// SSH format: git@host:org/repo
	if strings.HasPrefix(remote, "git@") {
		re := regexp.MustCompile(`git@[^:]+:(.+)`)
		if m := re.FindStringSubmatch(remote); len(m) > 1 {
			parts := strings.Split(m[1], "/")
			return parts[len(parts)-1]
		}
	}

	// HTTPS or local path: last segment
	parts := strings.Split(remote, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return ""
}

// ProjectNodeID returns the canonical node ID for a project root node.
// Format: "project:<name>"
func ProjectNodeID(name string) string {
	return "project:" + name
}

// PrefixedNodeID returns a node ID prefixed with the project name.
// Format: "<project>:<file>:<symbol>" or "<project>:<file>" for file nodes.
func PrefixedNodeID(project, fileRelPath, symbol string) string {
	if symbol == "" {
		return project + ":" + fileRelPath
	}
	return project + ":" + fileRelPath + ":" + symbol
}

// CreateProjectNode creates the root node for a codebase extraction.
// All file/function nodes link to this via "belongs_to" edges.
func CreateProjectNode(src *types.Source) types.Node {
	desc := "Codebase: " + src.Path
	if src.Remote != "" {
		desc = "Repository: " + src.Remote
	}

	return types.Node{
		ID:          ProjectNodeID(src.Name),
		Label:       src.Name,
		NodeType:    string(types.NodeTypeProject),
		Description: desc,
		Source:      src,
		Metadata: map[string]interface{}{
			"path":   src.Path,
			"remote": src.Remote,
		},
	}
}
