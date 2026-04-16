package extract

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Syfra3/vela/pkg/types"
)

func TestExtractRepoName(t *testing.T) {
	cases := []struct {
		remote string
		want   string
	}{
		{"https://github.com/Syfra3/vela.git", "vela"},
		{"https://github.com/Syfra3/vela", "vela"},
		{"git@github.com:Syfra3/glim.git", "glim"},
		{"git@github.com:Syfra3/ancora.git", "ancora"},
		{"https://github.com/org/my-project.git", "my-project"},
		{"/local/path/to/repo.git", "repo"},
	}
	for _, tc := range cases {
		got := extractRepoName(tc.remote)
		if got != tc.want {
			t.Errorf("extractRepoName(%q) = %q, want %q", tc.remote, got, tc.want)
		}
	}
}

func TestDetectProject_FolderFallback(t *testing.T) {
	dir := t.TempDir()
	src := DetectProject(dir)

	if src.Type != types.SourceTypeCodebase {
		t.Errorf("Type = %q, want codebase", src.Type)
	}
	if src.Name != filepath.Base(dir) {
		t.Errorf("Name = %q, want %q", src.Name, filepath.Base(dir))
	}
	if src.Path != dir {
		t.Errorf("Path = %q, want %q", src.Path, dir)
	}
	if src.Remote != "" {
		t.Errorf("Remote should be empty for non-git dir, got %q", src.Remote)
	}
}

func TestDetectProject_GitRepo(t *testing.T) {
	dir := t.TempDir()

	// Init git repo and add remote.
	mustGit(t, dir, "init")
	mustGit(t, dir, "remote", "add", "origin", "https://github.com/test/myrepo.git")

	src := DetectProject(dir)

	if src.Name != "myrepo" {
		t.Errorf("Name = %q, want myrepo", src.Name)
	}
	if src.Remote != "https://github.com/test/myrepo.git" {
		t.Errorf("Remote = %q, want https://github.com/test/myrepo.git", src.Remote)
	}
	if src.Type != types.SourceTypeCodebase {
		t.Errorf("Type = %q, want codebase", src.Type)
	}
}

func TestPrefixID(t *testing.T) {
	cases := []struct {
		project, id, want string
	}{
		{"vela", "internal/auth.go:validateToken", "vela:internal/auth.go:validateToken"},
		// Already prefixed — must not double-prefix.
		{"vela", "vela:internal/auth.go:validateToken", "vela:internal/auth.go:validateToken"},
		{"ancora", "internal/store.go:Save", "ancora:internal/store.go:Save"},
	}
	for _, tc := range cases {
		got := prefixID(tc.project, tc.id)
		if got != tc.want {
			t.Errorf("prefixID(%q, %q) = %q, want %q", tc.project, tc.id, got, tc.want)
		}
	}
}

func TestCreateProjectNode(t *testing.T) {
	src := &types.Source{
		Type:   types.SourceTypeCodebase,
		Name:   "myproject",
		Path:   "/tmp/myproject",
		Remote: "https://github.com/org/myproject.git",
	}
	node := CreateProjectNode(src)

	if node.ID != "project:myproject" {
		t.Errorf("ID = %q, want project:myproject", node.ID)
	}
	if node.NodeType != string(types.NodeTypeProject) {
		t.Errorf("NodeType = %q, want project", node.NodeType)
	}
	if node.Source == nil || node.Source.Name != "myproject" {
		t.Errorf("Source not set correctly: %+v", node.Source)
	}
	if node.Label != "myproject" {
		t.Errorf("Label = %q, want myproject", node.Label)
	}
	// Description should contain the remote URL.
	if node.Description == "" {
		t.Error("Description should not be empty for a repo with remote")
	}
}

// mustGit runs a git command in dir, skipping the test if git is not available.
func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		// If git is not available or fails, skip — don't fail tests in CI without git.
		t.Skipf("git %v failed (%v): %s", args, err, out)
	}
}
