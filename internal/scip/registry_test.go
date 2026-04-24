package scip

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Syfra3/vela/pkg/types"
)

type fakeDriver struct {
	name      string
	language  string
	supported bool
}

func (d fakeDriver) Name() string { return d.name }

func (d fakeDriver) Language() string { return d.language }

func (d fakeDriver) Supports(string) bool { return d.supported }

func (d fakeDriver) Index(context.Context, Request) (Result, error) {
	return Result{Driver: d.name, Language: d.language}, nil
}

func TestRequestNormalize_DefaultsArtifactPath(t *testing.T) {
	req := Request{RepoRoot: "/repo", Language: " Go "}

	normalized := req.Normalize()

	if normalized.Language != "go" {
		t.Fatalf("Normalize() language = %q, want go", normalized.Language)
	}
	if normalized.OutputPath != filepath.Join("/repo", ".vela", "scip", "go.scip") {
		t.Fatalf("Normalize() output_path = %q", normalized.OutputPath)
	}
	if err := normalized.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestRegistryRegister_RejectsDuplicateLanguage(t *testing.T) {
	registry, err := NewRegistry(fakeDriver{name: "scip-go", language: "go", supported: true})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	err = registry.Register(fakeDriver{name: "other-go", language: "go", supported: true})
	if err == nil {
		t.Fatal("Register() error = nil, want duplicate language error")
	}
}

func TestRegistryResolve_UsesRequestedLanguagesAndDriverAllowList(t *testing.T) {
	registry, err := NewRegistry(
		fakeDriver{name: "scip-go", language: "go", supported: true},
		fakeDriver{name: "scip-typescript", language: "typescript", supported: true},
		fakeDriver{name: "scip-python", language: "python", supported: true},
	)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	drivers, err := registry.Resolve(types.BuildRequest{
		RepoRoot:  "/repo",
		Languages: []string{"typescript", "go", "typescript"},
		Drivers:   []string{"scip-go", "scip-typescript", "scip-go"},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if len(drivers) != 2 {
		t.Fatalf("Resolve() len = %d, want 2", len(drivers))
	}
	if drivers[0].Name() != "scip-go" || drivers[1].Name() != "scip-typescript" {
		t.Fatalf("Resolve() order = [%s %s], want [scip-go scip-typescript]", drivers[0].Name(), drivers[1].Name())
	}
	if languages := registry.Languages(); len(languages) != 3 || languages[0] != "go" || languages[2] != "typescript" {
		t.Fatalf("Languages() = %v", languages)
	}
}

func TestRegistryResolve_FallsBackToRepoLanguageHints(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write tsconfig.json: %v", err)
	}

	registry, err := NewRegistry(
		fakeDriver{name: "scip-go", language: "go", supported: true},
		fakeDriver{name: "scip-typescript", language: "typescript", supported: true},
	)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	drivers, err := registry.Resolve(types.BuildRequest{RepoRoot: dir})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if len(drivers) != 2 {
		t.Fatalf("Resolve() len = %d, want 2", len(drivers))
	}
	if drivers[0].Language() != "go" || drivers[1].Language() != "typescript" {
		t.Fatalf("Resolve() languages = [%s %s], want [go typescript]", drivers[0].Language(), drivers[1].Language())
	}
}

func TestRegistryResolve_ErrorsForExplicitMissingLanguage(t *testing.T) {
	registry, err := NewRegistry(fakeDriver{name: "scip-go", language: "go", supported: true})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	_, err = registry.Resolve(types.BuildRequest{RepoRoot: "/repo", Languages: []string{"python"}})
	if err == nil {
		t.Fatal("Resolve() error = nil, want missing driver error")
	}
}
