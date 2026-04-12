package detect

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Collect walks root recursively, returning all files whose extension matches
// one of the provided exts (e.g. ".go", ".md"). Paths matching patterns in a
// .velignore file at root are excluded. exts may be nil/empty to collect all
// files regardless of extension.
func Collect(root string, exts []string) ([]string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	ignorePatterns, err := loadVelignore(absRoot)
	if err != nil {
		return nil, err
	}

	extSet := make(map[string]struct{}, len(exts))
	for _, e := range exts {
		extSet[e] = struct{}{}
	}

	var files []string
	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, relErr := filepath.Rel(absRoot, path)
		if relErr != nil {
			return relErr
		}

		// Check ignore patterns against relative path
		if isIgnored(rel, d.IsDir(), ignorePatterns) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			return nil
		}

		if len(extSet) > 0 {
			if _, ok := extSet[filepath.Ext(path)]; !ok {
				return nil
			}
		}

		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

// loadVelignore reads .velignore from root and returns non-empty, non-comment lines.
func loadVelignore(root string) ([]string, error) {
	path := filepath.Join(root, ".velignore")
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns, scanner.Err()
}

// isIgnored returns true if rel matches any of the gitignore-style patterns.
// Supports:
//   - exact name match (e.g. "vendor")
//   - leading slash = anchored to root (e.g. "/dist")
//   - trailing slash = directory only (e.g. "node_modules/")
//   - glob wildcards via filepath.Match (e.g. "*.pb.go")
func isIgnored(rel string, isDir bool, patterns []string) bool {
	// Never ignore the root itself
	if rel == "." {
		return false
	}

	name := filepath.Base(rel)

	for _, pattern := range patterns {
		// Directory-only pattern
		dirOnly := strings.HasSuffix(pattern, "/")
		p := strings.TrimSuffix(pattern, "/")
		if dirOnly && !isDir {
			continue
		}

		// Anchored pattern (starts with /)
		if strings.HasPrefix(p, "/") {
			anchored := strings.TrimPrefix(p, "/")
			matched, _ := filepath.Match(anchored, rel)
			if matched {
				return true
			}
			// Also try matching as prefix for directory patterns
			if dirOnly && strings.HasPrefix(rel, anchored+string(filepath.Separator)) {
				return true
			}
			continue
		}

		// Match against full relative path or just the base name
		if matched, _ := filepath.Match(p, name); matched {
			return true
		}
		if matched, _ := filepath.Match(p, rel); matched {
			return true
		}
		// Match any path component (e.g. "vendor" matches "a/vendor/b")
		for _, part := range strings.Split(rel, string(filepath.Separator)) {
			if matched, _ := filepath.Match(p, part); matched {
				return true
			}
		}
	}
	return false
}
