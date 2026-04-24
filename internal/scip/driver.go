package scip

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/Syfra3/vela/pkg/types"
)

// Driver defines the language-specific SCIP integration contract used by the
// graph-truth build pipeline.
type Driver interface {
	Name() string
	Language() string
	Supports(repoRoot string) bool
	Index(ctx context.Context, req Request) (Result, error)
}

// Bootstrapper optionally prepares a driver before indexing starts.
type Bootstrapper interface {
	Bootstrap(ctx context.Context, req Request) error
}

// Request is the normalized input passed to a specific SCIP driver.
type Request struct {
	RepoRoot   string
	Language   string
	OutputPath string
}

// Normalize trims wire input and fills the default SCIP artifact path.
func (r Request) Normalize() Request {
	r.RepoRoot = strings.TrimSpace(r.RepoRoot)
	r.Language = normalizeLanguage(r.Language)
	r.OutputPath = strings.TrimSpace(r.OutputPath)
	if r.OutputPath == "" && r.RepoRoot != "" && r.Language != "" {
		r.OutputPath = filepath.Join(r.RepoRoot, ".vela", "scip", r.Language+".scip")
	}
	return r
}

// Validate enforces the minimum contract every driver invocation needs.
func (r Request) Validate() error {
	r = r.Normalize()
	if r.RepoRoot == "" {
		return errors.New("scip request repo root is required")
	}
	if r.Language == "" {
		return errors.New("scip request language is required")
	}
	if r.OutputPath == "" {
		return errors.New("scip request output path is required")
	}
	return nil
}

// Result captures the semantic facts produced by a driver run.
type Result struct {
	Driver   string
	Language string
	Artifact string
	Facts    []types.Fact
}
