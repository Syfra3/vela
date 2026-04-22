package scip

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Syfra3/vela/pkg/types"
)

// Registry stores the available language-specific SCIP drivers behind one
// language-agnostic lookup surface.
type Registry struct {
	byLanguage map[string]Driver
}

// NewRegistry builds a registry from the supplied drivers.
func NewRegistry(drivers ...Driver) (*Registry, error) {
	r := &Registry{byLanguage: make(map[string]Driver, len(drivers))}
	for _, driver := range drivers {
		if err := r.Register(driver); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Register adds one driver keyed by normalized language.
func (r *Registry) Register(driver Driver) error {
	if r == nil {
		return errors.New("scip registry is nil")
	}
	if driver == nil {
		return errors.New("scip driver is nil")
	}
	language := normalizeLanguage(driver.Language())
	if language == "" {
		return errors.New("scip driver language is required")
	}
	if strings.TrimSpace(driver.Name()) == "" {
		return errors.New("scip driver name is required")
	}
	if _, exists := r.byLanguage[language]; exists {
		return fmt.Errorf("scip driver already registered for language %q", language)
	}
	r.byLanguage[language] = driver
	return nil
}

// Driver returns the registered driver for a language, if any.
func (r *Registry) Driver(language string) (Driver, bool) {
	if r == nil {
		return nil, false
	}
	driver, ok := r.byLanguage[normalizeLanguage(language)]
	return driver, ok
}

// Languages returns the normalized registered languages in stable order.
func (r *Registry) Languages() []string {
	if r == nil {
		return nil
	}
	languages := make([]string, 0, len(r.byLanguage))
	for language := range r.byLanguage {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	return languages
}

// Resolve chooses the drivers to run for a build request. Explicitly requested
// languages are strict; repository-derived hints are best-effort.
func (r *Registry) Resolve(build types.BuildRequest) ([]Driver, error) {
	if r == nil {
		return nil, errors.New("scip registry is nil")
	}
	build = build.Normalize()
	languages, explicit := resolveLanguages(build)
	allow := make(map[string]struct{}, len(build.Drivers))
	for _, name := range build.Drivers {
		allow[strings.TrimSpace(name)] = struct{}{}
	}

	resolved := make([]Driver, 0, len(languages))
	missing := make([]string, 0)
	for _, language := range languages {
		driver, ok := r.Driver(language)
		if !ok {
			if explicit {
				missing = append(missing, language)
			}
			continue
		}
		if len(allow) > 0 {
			if _, ok := allow[strings.TrimSpace(driver.Name())]; !ok {
				continue
			}
		}
		if !driver.Supports(build.RepoRoot) {
			continue
		}
		resolved = append(resolved, driver)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("no scip driver registered for languages: %s", strings.Join(missing, ", "))
	}
	return resolved, nil
}

// Bootstrap preflights resolved drivers before indexing starts.
func (r *Registry) Bootstrap(ctx context.Context, build types.BuildRequest) error {
	if r == nil {
		return errors.New("scip registry is nil")
	}
	drivers, err := r.Resolve(build)
	if err != nil {
		return err
	}
	for _, driver := range drivers {
		bootstrapper, ok := driver.(Bootstrapper)
		if !ok {
			continue
		}
		if err := bootstrapper.Bootstrap(ctx, Request{RepoRoot: build.RepoRoot, Language: driver.Language()}.Normalize()); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
		}
	}
	return nil
}

func resolveLanguages(build types.BuildRequest) ([]string, bool) {
	if len(build.Languages) > 0 {
		return normalizeLanguages(build.Languages), true
	}
	return detectLanguages(build.RepoRoot), false
}

func detectLanguages(root string) []string {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	var languages []string
	if fileExists(filepath.Join(root, "go.mod")) {
		languages = append(languages, "go")
	}
	if fileExists(filepath.Join(root, "tsconfig.json")) {
		languages = append(languages, "typescript")
	}
	if fileExists(filepath.Join(root, "package.json")) && !fileExists(filepath.Join(root, "tsconfig.json")) {
		languages = append(languages, "javascript")
	}
	if fileExists(filepath.Join(root, "pyproject.toml")) || fileExists(filepath.Join(root, "requirements.txt")) || fileExists(filepath.Join(root, "setup.py")) {
		languages = append(languages, "python")
	}
	if fileExists(filepath.Join(root, "Cargo.toml")) {
		languages = append(languages, "rust")
	}
	if fileExists(filepath.Join(root, "pom.xml")) || fileExists(filepath.Join(root, "build.gradle")) || fileExists(filepath.Join(root, "build.gradle.kts")) {
		languages = append(languages, "java")
	}
	return normalizeLanguages(languages)
}

func normalizeLanguages(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := normalizeLanguage(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	sort.Strings(result)
	return result
}

func normalizeLanguage(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
