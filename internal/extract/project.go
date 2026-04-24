package extract

import (
	"net/url"
	"os"
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
	return DetectProjectInWorkspace(dir, dir)
}

// DetectProjectInWorkspace derives a stable repo identity for dir relative to a
// selected workspace root when a git remote is unavailable.
func DetectProjectInWorkspace(workspaceRoot, dir string) *types.Source {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}
	absWorkspaceRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		absWorkspaceRoot = workspaceRoot
	}

	src := &types.Source{
		Type: types.SourceTypeCodebase,
		Name: filepath.Base(absDir),
		Path: absDir,
	}

	remote := gitRemoteURL(absDir)
	if remote != "" {
		src.Remote = remote
		identity, org, name := parseRemoteIdentity(remote)
		if identity != "" {
			src.ID = identity
			src.Organization = org
		}
		if name != "" {
			src.Name = name
		}
	}
	if src.ID == "" {
		src.ID = fallbackRepoIdentity(absWorkspaceRoot, absDir)
	}

	return src
}

// IsGitRepoRoot reports whether dir itself is a git repository root.
func IsGitRepoRoot(dir string) bool {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}
	top := gitRepoTopLevel(absDir)
	return top != "" && filepath.Clean(top) == filepath.Clean(absDir)
}

// DiscoverChildGitRepos finds nested git repositories below root.
func DiscoverChildGitRepos(root string) ([]string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	var repos []string
	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			return nil
		}
		if path != absRoot && isGitDir(path) {
			repos = append(repos, path)
			return filepath.SkipDir
		}
		name := d.Name()
		if name == ".git" || name == ".vela" {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return repos, nil
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

func gitRepoTopLevel(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func isGitDir(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir() || info.Mode().IsRegular()
}

// extractRepoName extracts the repository name from a git remote URL.
// Handles:
//   - https://github.com/Syfra3/vela.git  → "vela"
//   - git@github.com:Syfra3/vela.git      → "vela"
//   - https://github.com/Syfra3/vela      → "vela"
//   - /path/to/local/repo.git             → "repo"
func extractRepoName(remote string) string {
	_, _, name := parseRemoteIdentity(remote)
	return name
}

func parseRemoteIdentity(remote string) (identity, organization, name string) {
	remote = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(remote, "/"), ".git"))
	if remote == "" {
		return "", "", ""
	}

	if strings.Contains(remote, "://") {
		if parsed, err := url.Parse(remote); err == nil {
			return remoteIdentityFromParts(parsed.Hostname(), splitRemotePath(parsed.Path))
		}
	}

	if m := regexp.MustCompile(`^[^@]+@([^:]+):(.+)$`).FindStringSubmatch(remote); len(m) == 3 {
		return remoteIdentityFromParts(m[1], splitRemotePath(m[2]))
	}

	return "", "", fallbackRemoteName(remote)
}

func remoteIdentityFromParts(host string, parts []string) (identity, organization, name string) {
	if len(parts) == 0 {
		return "", "", ""
	}
	name = parts[len(parts)-1]
	if name == "" {
		return "", "", ""
	}
	if len(parts) > 1 {
		organization = strings.Join(parts[:len(parts)-1], "/")
	}
	identityParts := make([]string, 0, len(parts)+1)
	if host != "" {
		identityParts = append(identityParts, host)
	}
	identityParts = append(identityParts, parts...)
	return strings.Join(identityParts, "/"), organization, name
}

func splitRemotePath(raw string) []string {
	raw = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(raw, "/"), ".git"))
	raw = strings.TrimPrefix(raw, "/")
	raw = strings.TrimPrefix(raw, "~/")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, "/")
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." {
			continue
		}
		clean = append(clean, part)
	}
	return clean
}

func fallbackRemoteName(remote string) string {
	parts := strings.Split(remote, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func fallbackRepoIdentity(workspaceRoot, dir string) string {
	if rel, err := filepath.Rel(workspaceRoot, dir); err == nil {
		rel = filepath.ToSlash(filepath.Clean(rel))
		if rel != "." && rel != "" && !strings.HasPrefix(rel, "../") {
			return rel
		}
	}
	return filepath.Base(dir)
}

func SourceIdentity(src *types.Source) string {
	if src == nil {
		return ""
	}
	if id := strings.TrimSpace(src.ID); id != "" {
		return id
	}
	return strings.TrimSpace(src.Name)
}

func SourceDisplayName(src *types.Source) string {
	if src == nil {
		return ""
	}
	name := strings.TrimSpace(src.Name)
	org := strings.TrimSpace(src.Organization)
	if org != "" && name != "" {
		return org + "/" + name
	}
	if id := strings.TrimSpace(src.ID); id != "" {
		return id
	}
	return name
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
		ID:          ProjectNodeID(SourceIdentity(src)),
		Label:       src.Name,
		NodeType:    string(types.NodeTypeProject),
		Description: desc,
		Source:      src,
		Metadata: map[string]interface{}{
			"path":         src.Path,
			"remote":       src.Remote,
			"source_id":    SourceIdentity(src),
			"organization": src.Organization,
		},
	}
}
