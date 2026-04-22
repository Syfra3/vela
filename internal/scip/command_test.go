package scip

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type fakeRunner struct {
	command string
	args    []string
	dir     string
	err     error
}

func (r *fakeRunner) Run(_ context.Context, dir string, name string, args ...string) error {
	r.dir = dir
	r.command = name
	r.args = append([]string(nil), args...)
	return r.err
}

func TestDefaultRegistry_RegistersConcreteDrivers(t *testing.T) {
	registry, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry() error = %v", err)
	}

	if got, want := registry.Languages(), []string{"go", "python", "typescript"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("DefaultRegistry() languages = %v, want %v", got, want)
	}
	for _, language := range []string{"go", "typescript", "python"} {
		driver, ok := registry.Driver(language)
		if !ok {
			t.Fatalf("Driver(%q) missing from default registry", language)
		}
		if driver.Name() == "" {
			t.Fatalf("Driver(%q) has empty name", language)
		}
	}
}

func TestGoDriverIndex_RunsScipGoAndReturnsArtifact(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module example.com/test\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	runner := &fakeRunner{}
	driver := NewGoDriver(runner)

	result, err := driver.Index(context.Background(), Request{RepoRoot: repoRoot, Language: "go"})
	if err != nil {
		t.Fatalf("Index() error = %v", err)
	}

	wantArgs := []string{"--project-root", repoRoot, "--module-root", repoRoot, "--output", filepath.Join(repoRoot, ".vela", "scip", "go.scip")}
	if runner.command != "scip-go" {
		t.Fatalf("runner.command = %q, want scip-go", runner.command)
	}
	if !reflect.DeepEqual(runner.args, wantArgs) {
		t.Fatalf("runner.args = %v, want %v", runner.args, wantArgs)
	}
	if runner.dir != repoRoot {
		t.Fatalf("runner.dir = %q, want %q", runner.dir, repoRoot)
	}
	if result.Artifact != filepath.Join(repoRoot, ".vela", "scip", "go.scip") {
		t.Fatalf("result.Artifact = %q", result.Artifact)
	}
	if result.Driver != "scip-go" || result.Language != "go" {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(filepath.Dir(result.Artifact)); err != nil {
		t.Fatalf("expected output dir to exist: %v", err)
	}
}

func TestTypeScriptDriverSupports_JavaScriptFallback(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	if !NewTypeScriptDriver(&fakeRunner{}).Supports(repoRoot) {
		t.Fatal("Supports() = false, want true for javascript/typescript repo")
	}
}

func TestPythonDriverIndex_PropagatesRunnerFailures(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "pyproject.toml"), []byte("[project]\nname = 'demo'\n"), 0o644); err != nil {
		t.Fatalf("write pyproject.toml: %v", err)
	}
	runner := &fakeRunner{err: errors.New("boom")}

	_, err := NewPythonDriver(runner).Index(context.Background(), Request{RepoRoot: repoRoot, Language: "python"})
	if err == nil {
		t.Fatal("Index() error = nil, want runner error")
	}
	if runner.command != "scip-python" {
		t.Fatalf("runner.command = %q, want scip-python", runner.command)
	}
}

func TestTypeScriptDriverIndex_ReturnsMissingBinaryGuidance(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	runner := &fakeRunner{err: &exec.Error{Name: "scip-typescript", Err: exec.ErrNotFound}}

	_, err := NewTypeScriptDriver(runner).Index(context.Background(), Request{RepoRoot: repoRoot, Language: "typescript"})
	if err == nil {
		t.Fatal("Index() error = nil, want missing binary guidance")
	}
	var missing *MissingBinaryError
	if !errors.As(err, &missing) {
		t.Fatalf("Index() error = %T, want MissingBinaryError", err)
	}
	if missing.Command != "scip-typescript" {
		t.Fatalf("missing.Command = %q, want scip-typescript", missing.Command)
	}
	if got := missing.Error(); !containsAll(got, []string{"scip-typescript is not installed", "npm install -g @sourcegraph/scip-typescript", repoRoot}) {
		t.Fatalf("missing guidance = %q", got)
	}
}

func TestTypeScriptDriverBootstrap_InstallsMissingBinary(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	runner := &fakeRunner{}
	driver := NewTypeScriptDriver(runner).(*CommandDriver)
	lookups := 0
	driver.lookPath = func(name string) (string, error) {
		lookups++
		if lookups == 1 {
			return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
		}
		return "/usr/bin/" + name, nil
	}

	err := driver.Bootstrap(context.Background(), Request{RepoRoot: repoRoot, Language: "typescript"})
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if runner.command != "npm" {
		t.Fatalf("runner.command = %q, want npm", runner.command)
	}
	if want := []string{"install", "-g", "@sourcegraph/scip-typescript"}; !reflect.DeepEqual(runner.args, want) {
		t.Fatalf("runner.args = %v, want %v", runner.args, want)
	}
}

func TestTypeScriptDriverBootstrap_SkipsInstallWhenBinaryExists(t *testing.T) {
	repoRoot := t.TempDir()
	runner := &fakeRunner{}
	driver := NewTypeScriptDriver(runner).(*CommandDriver)
	driver.lookPath = func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}

	err := driver.Bootstrap(context.Background(), Request{RepoRoot: repoRoot, Language: "typescript"})
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if runner.command != "" {
		t.Fatalf("runner.command = %q, want no install command", runner.command)
	}
}

func TestGoDriverIndex_SummarizesCapturedCommandFailure(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module example.com/test\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	runner := &fakeRunner{err: &commandRunError{Err: errors.New("exit status 2"), Output: "Resolving module name\npanic: runtime error: invalid memory address or nil pointer dereference\nstack line\n"}}

	_, err := NewGoDriver(runner).Index(context.Background(), Request{RepoRoot: repoRoot, Language: "go"})
	if err == nil {
		t.Fatal("Index() error = nil, want runner error")
	}
	if got := err.Error(); !containsAll(got, []string{"run scip-go", "exit status 2", "panic: runtime error: invalid memory address or nil pointer dereference"}) {
		t.Fatalf("error = %q, want captured panic summary", got)
	}
	if strings.Contains(err.Error(), "stack line") {
		t.Fatalf("error = %q, did not expect raw stack trace", err.Error())
	}
}

func TestSummarizeCommandOutput_PrefersPanicLine(t *testing.T) {
	t.Parallel()

	got := summarizeCommandOutput("Resolving module name\npanic: runtime error: invalid memory address\nstack line\n")
	if got != "panic: runtime error: invalid memory address" {
		t.Fatalf("summary = %q, want panic line", got)
	}
}

func containsAll(haystack string, needles []string) bool {
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			return false
		}
	}
	return true
}
