// Package detect handles file discovery and filtering for Vela extraction.
// It implements gitignore-compatible pattern matching without external dependencies.
package detect

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// IgnoreList holds compiled patterns from a single .gitignore / .velignore file.
// Patterns are scoped to the directory that contains the ignore file.
type IgnoreList struct {
	root     string    // absolute dir containing the ignore file
	patterns []pattern // ordered — later patterns override earlier ones
}

type pattern struct {
	raw      string
	negated  bool // line starts with !
	dirOnly  bool // line ends with /
	anchored bool // line contains / (other than trailing)
	segments []string
}

// LoadIgnoreFile parses a .gitignore or .velignore file rooted at dir.
// Returns nil, nil if the file does not exist.
func LoadIgnoreFile(dir, filename string) (*IgnoreList, error) {
	path := filepath.Join(dir, filename)
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	il := &IgnoreList{root: dir}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if p, ok := parseLine(line); ok {
			il.patterns = append(il.patterns, p)
		}
	}
	return il, scanner.Err()
}

// NewIgnoreList builds an IgnoreList from raw pattern strings.
// Useful for embedding built-in tech defaults without needing a file.
func NewIgnoreList(root string, lines []string) *IgnoreList {
	il := &IgnoreList{root: root}
	for _, line := range lines {
		if p, ok := parseLine(line); ok {
			il.patterns = append(il.patterns, p)
		}
	}
	return il
}

// Ignored reports whether absPath should be excluded.
// isDir must be true when absPath refers to a directory.
func (il *IgnoreList) Ignored(absPath string, isDir bool) bool {
	rel, err := filepath.Rel(il.root, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return false
	}
	rel = filepath.ToSlash(rel)

	result := false
	for _, p := range il.patterns {
		if p.dirOnly && !isDir {
			continue
		}
		if matchPattern(p, rel, isDir) {
			result = !p.negated
		}
	}
	return result
}

// parseLine converts a raw line into a pattern.
// Returns ok=false for blank lines and comments.
func parseLine(line string) (pattern, bool) {
	// strip trailing spaces (not escaped ones)
	line = strings.TrimRight(line, " \t")
	if line == "" || strings.HasPrefix(line, "#") {
		return pattern{}, false
	}

	p := pattern{raw: line}

	if strings.HasPrefix(line, "!") {
		p.negated = true
		line = line[1:]
	}
	// strip leading backslash escape
	if strings.HasPrefix(line, `\`) {
		line = line[1:]
	}

	if strings.HasSuffix(line, "/") {
		p.dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}

	// anchored = has a slash anywhere (now that we stripped the trailing one).
	// A leading slash is also an anchor marker — strip it before splitting.
	p.anchored = strings.Contains(line, "/")
	line = strings.TrimPrefix(line, "/")

	p.segments = strings.Split(line, "/")
	return p, true
}

// matchPattern tests a forward-slash relative path against a compiled pattern.
func matchPattern(p pattern, rel string, isDir bool) bool {
	parts := strings.Split(rel, "/")

	if p.anchored {
		// must match from root
		return matchSegments(p.segments, parts)
	}

	// unanchored: pattern can match any suffix of parts
	// e.g. "node_modules" matches "a/b/node_modules"
	for i := range parts {
		if matchSegments(p.segments, parts[i:]) {
			return true
		}
	}
	return false
}

// matchSegments matches glob segments against path parts.
// Supports * and ? wildcards within a segment; ** matches zero or more path components.
func matchSegments(segs, parts []string) bool {
	si, pi := 0, 0
	for si < len(segs) && pi < len(parts) {
		seg := segs[si]
		if seg == "**" {
			// consume zero or more parts
			si++
			if si == len(segs) {
				return true // ** at end matches everything remaining
			}
			for ; pi <= len(parts); pi++ {
				if matchSegments(segs[si:], parts[pi:]) {
					return true
				}
			}
			return false
		}
		ok, _ := filepath.Match(seg, parts[pi])
		if !ok {
			return false
		}
		si++
		pi++
	}
	// consume trailing ** segments
	for si < len(segs) && segs[si] == "**" {
		si++
	}
	return si == len(segs) && pi == len(parts)
}
