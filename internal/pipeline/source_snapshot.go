package pipeline

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/Syfra3/vela/internal/generation"
	"github.com/Syfra3/vela/pkg/types"
)

type sourceSnapshot struct {
	Root     string
	Files    []string
	Manifest *types.Manifest
	Closure  *generation.Closure
}

func (s *sourceSnapshot) Close() {
	if s != nil {
		_ = os.RemoveAll(s.Root)
	}
}

// Capture once into private regular files. The manifest is then derived from
// those copies, not a second live read. A-to-B-to-A mutations cannot substitute
// bytes consumed by extraction while retaining A's provenance.
func captureSource(root string, req types.ManifestRequest) (*sourceSnapshot, error) {
	c, err := generation.ClosedInputs(root)
	if err != nil {
		return nil, err
	}
	module := ""
	for _, line := range strings.Split(string(c.Bytes["go.mod"]), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			module = strings.TrimSpace(strings.TrimPrefix(line, "module "))
			break
		}
	}
	for p, data := range c.Bytes {
		if filepath.Ext(p) != ".go" {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), p, data, parser.ImportsOnly)
		if err != nil {
			continue
		}
		for _, spec := range f.Imports {
			imp := strings.Trim(spec.Path.Value, "\"")
			rel := ""
			if module != "" && strings.HasPrefix(imp, module) {
				rel = strings.TrimPrefix(strings.TrimPrefix(imp, module), "/")
			} else if module == "" && (strings.HasPrefix(imp, "internal/") || strings.HasPrefix(imp, "pkg/") || strings.HasPrefix(imp, "cmd/")) {
				rel = imp
			}
			if rel == "" {
				continue
			}
			clean := filepath.Clean(filepath.FromSlash(rel))
			if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
				return nil, fmt.Errorf("escaping Go import: %s", imp)
			}
			for _, part := range strings.Split(filepath.ToSlash(clean), "/") {
				if part == ".git" || part == ".vela" || part == ".generations" || part == ".extraction-cache" || strings.HasPrefix(part, ".scip-build-") {
					return nil, fmt.Errorf("unclosed Go import: %s", imp)
				}
			}
		}
	}
	private, err := os.MkdirTemp("", "vela-source-")
	if err != nil {
		return nil, err
	}
	s := &sourceSnapshot{Root: private, Closure: c}
	success := false
	defer func() {
		if !success {
			s.Close()
		}
	}()
	for _, dir := range c.Dirs {
		if err := os.MkdirAll(filepath.Join(private, dir), 0700); err != nil {
			return nil, err
		}
	}
	for p, data := range c.Bytes {
		path := filepath.Join(private, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, data, 0400); err != nil {
			return nil, err
		}
	}
	m, files, err := generation.Inventory(private, req)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		if filepath.Ext(file) != ".go" {
			return nil, fmt.Errorf("non-Go source profile")
		}
	}
	m.RepoRoot = root
	m.SourceBinding = generation.BoundGoProfile
	m.ConsumedFiles = c.Files
	m.DirectoryFingerprint = c.DirectoryFingerprint
	m.DependencyFingerprint = c.DependencyFingerprint
	m.ClosureConfigFingerprint = c.ConfigFingerprint
	s.Files = files
	s.Manifest = m
	success = true
	return s, nil
}
