package detect_test

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/Syfra3/vela/internal/detect"
)

// fixtureDir returns the absolute path to a named fixture directory.
func fixtureDir(name string) string {
	_, file, _, _ := runtime.Caller(0)
	// detect_test.go lives in internal/detect/ — go up two levels to project root
	root := filepath.Join(filepath.Dir(file), "..", "..")
	return filepath.Join(root, "tests", "fixtures", "detect", name)
}

// relPaths extracts RelPath from FileEntry slice and sorts them for stable comparison.
func relPaths(entries []detect.FileEntry) []string {
	paths := make([]string, len(entries))
	for i, e := range entries {
		paths[i] = filepath.ToSlash(e.RelPath)
	}
	sort.Strings(paths)
	return paths
}

// containsPath returns true if path is in the slice.
func containsPath(paths []string, target string) bool {
	for _, p := range paths {
		if p == target {
			return true
		}
	}
	return false
}

// ---- IgnoreList unit tests -----------------------------------------------

func TestIgnoreList_SimplePattern(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		path     string
		isDir    bool
		want     bool
	}{
		{
			name:     "exact filename match",
			patterns: []string{"secret.txt"},
			path:     "secret.txt",
			isDir:    false,
			want:     true,
		},
		{
			name:     "no match",
			patterns: []string{"secret.txt"},
			path:     "public.txt",
			isDir:    false,
			want:     false,
		},
		{
			name:     "dir-only pattern ignores dir",
			patterns: []string{"node_modules/"},
			path:     "node_modules",
			isDir:    true,
			want:     true,
		},
		{
			name:     "dir-only pattern does not ignore file with same name",
			patterns: []string{"node_modules/"},
			path:     "node_modules",
			isDir:    false,
			want:     false,
		},
		{
			name:     "wildcard extension",
			patterns: []string{"*.pyc"},
			path:     "main.pyc",
			isDir:    false,
			want:     true,
		},
		{
			name:     "wildcard no match",
			patterns: []string{"*.pyc"},
			path:     "main.py",
			isDir:    false,
			want:     false,
		},
		{
			name:     "nested unanchored match",
			patterns: []string{"vendor/"},
			path:     "a/b/vendor",
			isDir:    true,
			want:     true,
		},
		{
			name:     "anchored pattern matches from root only",
			patterns: []string{"/dist"},
			path:     "dist",
			isDir:    false,
			want:     true,
		},
		{
			name:     "anchored pattern does not match nested path",
			patterns: []string{"/dist"},
			path:     "src/dist",
			isDir:    false,
			want:     false,
		},
		{
			name:     "negation re-includes file",
			patterns: []string{"*.log", "!important.log"},
			path:     "important.log",
			isDir:    false,
			want:     false,
		},
		{
			name:     "negation does not affect other files",
			patterns: []string{"*.log", "!important.log"},
			path:     "debug.log",
			isDir:    false,
			want:     true,
		},
		{
			name:     "comment line ignored",
			patterns: []string{"# this is a comment", "*.log"},
			path:     "debug.log",
			isDir:    false,
			want:     true,
		},
		{
			name:     "blank line ignored",
			patterns: []string{"", "*.log"},
			path:     "debug.log",
			isDir:    false,
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			il := detect.NewIgnoreList(root, tt.patterns)

			absPath := filepath.Join(root, filepath.FromSlash(tt.path))
			got := il.Ignored(absPath, tt.isDir)

			if got != tt.want {
				t.Errorf("Ignored(%q, dir=%v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
			}
		})
	}
}

func TestIgnoreList_OutsideRoot(t *testing.T) {
	root := t.TempDir()
	il := detect.NewIgnoreList(root, []string{"*.go"})

	// path is outside the root — should never be ignored
	outside := "/tmp/other/main.go"
	if il.Ignored(outside, false) {
		t.Errorf("path outside root should not be ignored")
	}
}

// ---- DetectTech unit tests -----------------------------------------------

func TestDetectTech(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		want     detect.Tech
	}{
		{"node via package.json", "package.json", detect.TechNode},
		{"go via go.mod", "go.mod", detect.TechGo},
		{"rust via Cargo.toml", "Cargo.toml", detect.TechRust},
		{"java via pom.xml", "pom.xml", detect.TechJava},
		{"python via requirements.txt", "requirements.txt", detect.TechPython},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			// create manifest file
			f := filepath.Join(dir, tt.manifest)
			if err := writeEmpty(f); err != nil {
				t.Fatal(err)
			}
			got := detect.DetectTech(dir)
			if got != tt.want {
				t.Errorf("DetectTech = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetectTech_Unknown(t *testing.T) {
	dir := t.TempDir()
	got := detect.DetectTech(dir)
	if got != detect.TechUnknown {
		t.Errorf("empty dir: DetectTech = %v, want TechUnknown", got)
	}
}

// ---- Walker integration tests --------------------------------------------

func TestWalk_NodeProject(t *testing.T) {
	result, err := detect.Files(fixtureDir("node_project"))
	if err != nil {
		t.Fatal(err)
	}
	paths := relPaths(result.Files)

	// should include source files
	mustContain(t, paths, "package.json")
	mustContain(t, paths, "src/index.ts")
	mustContain(t, paths, "src/utils.ts")

	// should exclude generated / dependency dirs
	mustNotContain(t, paths, "node_modules/lodash/index.js")
	mustNotContain(t, paths, ".next/cache/data.json")
	mustNotContain(t, paths, "dist/bundle.js")
}

func TestWalk_GoProject(t *testing.T) {
	result, err := detect.Files(fixtureDir("go_project"))
	if err != nil {
		t.Fatal(err)
	}
	paths := relPaths(result.Files)

	mustContain(t, paths, "go.mod")
	mustContain(t, paths, "main.go")
	mustContain(t, paths, "internal/service/svc.go")

	// vendor excluded by Go tech defaults
	mustNotContain(t, paths, "vendor/github.com/foo/bar.go")
}

func TestWalk_PythonProject(t *testing.T) {
	result, err := detect.Files(fixtureDir("python_project"))
	if err != nil {
		t.Fatal(err)
	}
	paths := relPaths(result.Files)

	mustContain(t, paths, "requirements.txt")
	mustContain(t, paths, "src/main.py")

	mustNotContain(t, paths, "__pycache__/main.cpython-311.pyc")
	mustNotContain(t, paths, ".venv/lib/site.py")
}

func TestWalk_GitignoreProject(t *testing.T) {
	result, err := detect.Files(fixtureDir("gitignore_project"))
	if err != nil {
		t.Fatal(err)
	}
	paths := relPaths(result.Files)

	mustContain(t, paths, "src/app.go")
	mustNotContain(t, paths, "secret/token.txt")
	// .gitignore itself should be included (vela may want to read it)
	mustContain(t, paths, ".gitignore")
}

func TestWalk_VelignoreProject(t *testing.T) {
	result, err := detect.Files(fixtureDir("velignore_project"))
	if err != nil {
		t.Fatal(err)
	}
	paths := relPaths(result.Files)

	mustContain(t, paths, "src/main.go")
	// generated/ excluded by .velignore
	mustNotContain(t, paths, "generated/schema.go")
}

func TestWalk_NestedMonorepo(t *testing.T) {
	result, err := detect.Files(fixtureDir("nested"))
	if err != nil {
		t.Fatal(err)
	}
	paths := relPaths(result.Files)

	// source files from both sub-projects
	mustContain(t, paths, "frontend/src/App.tsx")
	mustContain(t, paths, "backend/cmd/main.go")

	// each sub-project's deps/build excluded by tech defaults detected per-dir
	mustNotContain(t, paths, "frontend/node_modules/react/index.js")
	mustNotContain(t, paths, "backend/vendor/pkg/lib.go")
}

// ---- helpers --------------------------------------------------------------

func mustContain(t *testing.T, paths []string, target string) {
	t.Helper()
	if !containsPath(paths, target) {
		t.Errorf("expected %q in result, got: %v", target, paths)
	}
}

func mustNotContain(t *testing.T, paths []string, target string) {
	t.Helper()
	if containsPath(paths, target) {
		t.Errorf("expected %q to be excluded, but it was included", target)
	}
}

func writeEmpty(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return f.Close()
}
