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
