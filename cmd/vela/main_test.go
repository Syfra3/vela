package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Syfra3/vela/pkg/types"
)

func TestRootCommandExposesReducedBuildAndQuerySurface(t *testing.T) {
	t.Parallel()

	root := rootCmd()
	commands := map[string]bool{}
	for _, cmd := range root.Commands() {
		commands[cmd.Name()] = true
	}

	for _, want := range []string{"build", "extract", "query", "serve", "tui", "version"} {
		if !commands[want] {
			t.Fatalf("expected command %q to be registered", want)
		}
	}
	for _, blocked := range []string{"search", "watch", "hook", "doctor", "config"} {
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
		{name: "extract alias", args: []string{"extract", "/repo", "--language", "go", "--driver", "scip-go"}, wantUse: "extract"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured types.BuildRequest
			runBuildService = func(_ context.Context, outDir string, req types.BuildRequest) (buildOutput, error) {
				captured = req
				return buildOutput{GraphPath: outDir + "/graph.json", HTMLPath: outDir + "/graph.html", ObsidianPath: "/vault/obsidian", Files: 1}, nil
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
			for _, want := range []string{"graph.json", "graph.html", "/vault/obsidian"} {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("expected build output to mention %q, got %q", want, stdout.String())
				}
			}
		})
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
