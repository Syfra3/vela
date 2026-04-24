package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Syfra3/vela/internal/hooks"
	"github.com/Syfra3/vela/pkg/types"
)

func TestRootCommandExposesReducedBuildAndQuerySurface(t *testing.T) {
	t.Parallel()

	root := rootCmd()
	commands := map[string]bool{}
	for _, cmd := range root.Commands() {
		commands[cmd.Name()] = true
	}

	for _, want := range []string{"build", "update", "watch", "hooks", "extract", "status", "lookup", "search", "query", "serve", "tui", "version"} {
		if !commands[want] {
			t.Fatalf("expected command %q to be registered", want)
		}
	}
	for _, blocked := range []string{"hook", "doctor", "config"} {
		if commands[blocked] {
			t.Fatalf("did not expect legacy command %q to remain active", blocked)
		}
	}

	queryCommand, _, err := root.Find([]string{"query", "dependencies"})
	if err != nil {
		t.Fatalf("Find(query dependencies) error = %v", err)
	}
	if queryCommand == nil || queryCommand.Name() != "dependencies" {
		t.Fatalf("expected dependencies subcommand, got %#v", queryCommand)
	}
}

func TestBuildAndExtractCommandsRouteThroughSharedBuildService(t *testing.T) {
	restore := runBuildService
	t.Cleanup(func() { runBuildService = restore })

	tests := []struct {
		name    string
		args    []string
		wantUse string
	}{
		{name: "build", args: []string{"build", "/repo", "--language", "go", "--driver", "scip-go"}, wantUse: "build"},
		{name: "update", args: []string{"update", "/repo", "--language", "go", "--driver", "scip-go"}, wantUse: "update"},
		{name: "extract alias", args: []string{"extract", "/repo", "--language", "go", "--driver", "scip-go"}, wantUse: "extract"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured types.BuildRequest
			runBuildService = func(_ context.Context, outDir string, req types.BuildRequest) (buildOutput, error) {
				captured = req
				return buildOutput{GraphPath: outDir + "/graph.json", HTMLPath: outDir + "/graph.html", ReportPath: outDir + "/GRAPH_REPORT.md", ObsidianPath: "/vault/obsidian", Files: 1}, nil
			}

			root := rootCmd()
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			root.SetOut(stdout)
			root.SetErr(stderr)
			root.SetArgs(tt.args)

			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if captured.RepoRoot != "/repo" {
				t.Fatalf("RepoRoot = %q, want /repo", captured.RepoRoot)
			}
			if len(captured.Languages) != 1 || captured.Languages[0] != "go" {
				t.Fatalf("Languages = %v, want [go]", captured.Languages)
			}
			if len(captured.Drivers) != 1 || captured.Drivers[0] != "scip-go" {
				t.Fatalf("Drivers = %v, want [scip-go]", captured.Drivers)
			}
			for _, want := range []string{"graph.json", "graph.html", "GRAPH_REPORT.md", "/vault/obsidian"} {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("expected build output to mention %q, got %q", want, stdout.String())
				}
			}
		})
	}
}

func TestWatchCommandRoutesThroughSharedWatchService(t *testing.T) {
	restore := runWatchService
	t.Cleanup(func() { runWatchService = restore })

	var captured types.BuildRequest
	var capturedOutDir string
	runWatchService = func(ctx context.Context, outDir string, req types.BuildRequest, stdout, stderr io.Writer) error {
		captured = req
		capturedOutDir = outDir
		return nil
	}

	root := rootCmd()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs([]string{"watch", "/repo", "--language", "go", "--driver", "scip-go", "--out-dir", "/repo/.vela"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if captured.RepoRoot != "/repo" {
		t.Fatalf("RepoRoot = %q, want /repo", captured.RepoRoot)
	}
	if len(captured.Languages) != 1 || captured.Languages[0] != "go" {
		t.Fatalf("Languages = %v, want [go]", captured.Languages)
	}
	if len(captured.Drivers) != 1 || captured.Drivers[0] != "scip-go" {
		t.Fatalf("Drivers = %v, want [scip-go]", captured.Drivers)
	}
	if capturedOutDir != "/repo/.vela" {
		t.Fatalf("outDir = %q, want /repo/.vela", capturedOutDir)
	}
	if !strings.Contains(stdout.String(), "watching for changes") {
		t.Fatalf("expected watch startup message, got %q", stdout.String())
	}
}

func TestHooksInstallCommandRoutesThroughInstaller(t *testing.T) {
	restore := installRepoHooks
	t.Cleanup(func() { installRepoHooks = restore })

	called := false
	installRepoHooks = func(repoRoot, executablePath string) error {
		called = true
		if repoRoot != "/repo" {
			t.Fatalf("repoRoot = %q, want /repo", repoRoot)
		}
		if executablePath == "" {
			t.Fatal("expected executable path")
		}
		return nil
	}

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"hooks", "install", "/repo"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !called {
		t.Fatal("expected installRepoHooks to be called")
	}
	if !strings.Contains(stdout.String(), "installed Vela hooks") {
		t.Fatalf("expected install output, got %q", stdout.String())
	}
}

func TestHooksStatusCommandPrintsHookStates(t *testing.T) {
	restore := inspectRepoHooks
	t.Cleanup(func() { inspectRepoHooks = restore })

	inspectRepoHooks = func(repoRoot string) (hooks.Status, error) {
		return hooks.Status{RepoRoot: repoRoot, Hooks: map[string]bool{"post-commit": true, "post-checkout": false}}, nil
	}

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"hooks", "status", "/repo"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"repo: /repo", "post-commit: installed", "post-checkout: missing"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected %q in status output, got %q", want, stdout.String())
		}
	}
}

func TestHooksUninstallCommandRoutesThroughRemover(t *testing.T) {
	restore := uninstallRepoHooks
	t.Cleanup(func() { uninstallRepoHooks = restore })

	called := false
	uninstallRepoHooks = func(repoRoot string) error {
		called = true
		if repoRoot != "/repo" {
			t.Fatalf("repoRoot = %q, want /repo", repoRoot)
		}
		return nil
	}

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"hooks", "uninstall", "/repo"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !called {
		t.Fatal("expected uninstallRepoHooks to be called")
	}
	if !strings.Contains(stdout.String(), "removed Vela hooks") {
		t.Fatalf("expected uninstall output, got %q", stdout.String())
	}
}

func TestServeCommandOmitsLegacyAncoraFlag(t *testing.T) {
	t.Parallel()

	cmd := serveCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	help := buf.String()
	if strings.Contains(help, "ancora-db") {
		t.Fatalf("expected serve help to omit legacy ancora-db flag, got %q", help)
	}
	for _, want := range []string{"--graph", "--http", "--port"} {
		if !strings.Contains(help, want) {
			t.Fatalf("expected serve help to contain %q, got %q", want, help)
		}
	}
}

func TestSearchCommandRoutesStructuralPromptToQueryService(t *testing.T) {
	graphPath := writeSearchTestGraph(t)

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"search", "who uses rootCmd", "--graph", graphPath, "--limit", "7"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"Reverse dependencies for \"rootCmd\":", "main [repo/function] via calls"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected stdout to contain %q, got %q", want, stdout.String())
		}
	}
}

func TestLookupCommandPrintsCandidateNodes(t *testing.T) {
	graphPath := writeSearchTestGraph(t)

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"lookup", "root", "--graph", graphPath, "--limit", "2"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"Candidates for \"root\":", "1. rootCmd", "id: cmd/vela/main.go:rootCmd", "Next steps:"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected stdout to contain %q, got %q", want, stdout.String())
		}
	}
}

func TestQueryCommandSuggestsLookupWhenSubjectIsMissing(t *testing.T) {
	graphPath := writeSearchTestGraph(t)

	root := rootCmd()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs([]string{"query", "dependencies", "MissingNode", "--graph", graphPath})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected missing-node error")
	}
	for _, want := range []string{"node \"MissingNode\" not found", "hint: try `vela lookup \"MissingNode\"` to find candidate nodes"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %q", want, err.Error())
		}
	}
}

func writeSearchTestGraph(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	graph := map[string]any{
		"nodes": []map[string]any{
			{"id": "cmd/vela/main.go:rootCmd", "label": "rootCmd", "kind": "function", "file": "cmd/vela/main.go"},
			{"id": "cmd/vela/main.go:main", "label": "main", "kind": "function", "file": "cmd/vela/main.go"},
		},
		"edges": []map[string]any{
			{"from": "cmd/vela/main.go:main", "to": "cmd/vela/main.go:rootCmd", "kind": "calls"},
		},
		"meta": map[string]any{"nodeCount": 2, "edgeCount": 1},
	}
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	path := filepath.Join(dir, "graph.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
