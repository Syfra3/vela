// Package detect provides file discovery for Vela extraction pipelines.
//
// It walks a directory tree recursively, collecting files while filtering out
// generated artefacts and dependency directories using a three-layer strategy:
//
//  1. Tech-detected defaults — opinionated per-stack ignore rules applied
//     whenever a stack manifest (package.json, go.mod, etc.) is found.
//  2. .gitignore — standard git ignore rules found at each directory level.
//  3. .velignore — vela-specific overrides with the same syntax as .gitignore,
//     evaluated last so they take precedence over .gitignore and tech defaults.
//
// Rules are scoped to the directory that contains the ignore file, and
// inherited by all subdirectories — matching git's own behaviour.
//
// Usage:
//
//	result, err := detect.Files("/path/to/project")
//	for _, f := range result.Files {
//	    fmt.Println(f.RelPath)
//	}
package detect

import (
	"os"
	"path/filepath"
)

// Result holds the output of a detection pass.
type Result struct {
	// Files contains every file that passed all ignore filters.
	Files []FileEntry

	// Root is the absolute path that was walked.
	Root string
}

// Files walks root and returns all non-ignored files.
// This is the primary entry point for callers outside this package.
func Files(root string) (*Result, error) {
	w, err := NewWalker(root)
	if err != nil {
		return nil, err
	}

	entries, err := w.Walk()
	if err != nil {
		return nil, err
	}

	return &Result{
		Files: entries,
		Root:  w.root,
	}, nil
}

// Collect walks root and returns the absolute paths of all non-ignored files
// whose extension is in exts. If exts is empty, all files are returned.
func Collect(root string, exts []string) ([]string, error) {
	result, err := Files(root)
	if err != nil {
		return nil, err
	}

	if len(exts) == 0 {
		paths := make([]string, len(result.Files))
		for i, f := range result.Files {
			paths[i] = f.AbsPath
		}
		return paths, nil
	}

	set := make(map[string]struct{}, len(exts))
	for _, e := range exts {
		set[e] = struct{}{}
	}

	var paths []string
	for _, f := range result.Files {
		if _, ok := set[filepath.Ext(f.AbsPath)]; ok {
			paths = append(paths, f.AbsPath)
		}
	}
	return paths, nil
}

// EnsureVelignore creates a .velignore file at root with sensible defaults if
// one does not already exist. Returns the created path and nil on creation,
// or empty string and nil if the file already existed.
func EnsureVelignore(root string) (string, error) {
	path := filepath.Join(root, ".velignore")
	if _, err := os.Stat(path); err == nil {
		return "", nil
	}

	const defaults = `# Vela ignore file — same syntax as .gitignore
# Files matched here are excluded from graph extraction.

# Dependencies
node_modules/
vendor/
.venv/
venv/

# Build output
dist/
build/
out/
target/

# Generated
*.pb.go
*.gen.go
`
	if err := os.WriteFile(path, []byte(defaults), 0644); err != nil {
		return "", err
	}
	return path, nil
}
