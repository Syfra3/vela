package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/internal/hooks"
	"github.com/Syfra3/vela/internal/query"
	"github.com/Syfra3/vela/pkg/types"
	mcppkg "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func TestRootCommandExposesReducedBuildAndQuerySurface(t *testing.T) {
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

// REQ-008 → SCN-011 → TestSCN011_CLIExposesRequiredV04CommandSurface
func TestSCN011_CLIExposesRequiredV04CommandSurface(t *testing.T) {
	// Scenario: CLI provides required v0.4 command surface
	root := rootCmd()
	commands := map[string]bool{}
	for _, cmd := range root.Commands() {
		commands[cmd.Name()] = true
	}

	for _, want := range []string{"explore", "lookup", "status", "build", "update", "watch", "serve", "explain", "impact", "path"} {
		if !commands[want] {
			t.Fatalf("expected v0.4 command %q to be registered", want)
		}
	}

	serve := commands["serve"]
	if !serve {
		t.Fatal("expected serve command to be registered")
	}
	serveCmd, _, err := root.Find([]string{"serve", "--mcp"})
	if err != nil {
		t.Fatalf("Find(serve --mcp) error = %v", err)
	}
	if serveCmd == nil || serveCmd.Name() != "serve" {
		t.Fatalf("expected serve --mcp to resolve to serve command, got %#v", serveCmd)
	}
}

// REQ-013 → SCN-018 → TestSCN018_StatusCommandReportsPendingStaleFiles
func TestSCN018_StatusCommandReportsPendingStaleFiles(t *testing.T) {
	// Scenario: Status reports pending stale files after source changes
	repoRoot := t.TempDir()
	outDir := filepath.Join(repoRoot, ".vela")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(.vela) error = %v", err)
	}
	sourcePath := filepath.Join(repoRoot, "main.go")
	if err := os.WriteFile(sourcePath, []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.go) error = %v", err)
	}
	graphPath := filepath.Join(outDir, "graph.json")
	if err := os.WriteFile(graphPath, []byte(`{"nodes":[],"edges":[],"meta":{"generatedAt":"2026-04-23T22:47:00Z"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile(graph.json) error = %v", err)
	}
	manifestJSON := `{
	  "version": 1,
	  "repo_root": ` + strconv.Quote(repoRoot) + `,
	  "generated_at": "2026-04-23T22:47:00Z",
	  "build_mode": "full_rebuild",
	  "files": [
	    {"path":"main.go", "sha256":` + strconv.Quote(testFileSHA256(t, sourcePath)) + `, "status":"active"}
	  ]
	}`
	if err := os.WriteFile(filepath.Join(outDir, "manifest.json"), []byte(manifestJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(manifest.json) error = %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("package main\nfunc main() { println(\"changed\") }\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.go changed) error = %v", err)
	}

	stdout := captureStdout(t, func() {
		root := rootCmd()
		root.SetErr(&bytes.Buffer{})
		root.SetArgs([]string{"status", "--graph", graphPath, "--baseline", ""})
		if err := root.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})
	for _, want := range []string{"freshness: stale", "stale files: main.go", "recommended: vela update, vela build"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected status output to contain %q, got %q", want, stdout)
		}
	}
}

// REQ-013 → SCN-019 → TestSCN019_UpdateFailurePreservesPreviousStaleGraphState
func TestSCN019_UpdateFailurePreservesPreviousStaleGraphState(t *testing.T) {
	// Scenario: Update safely refreshes stale graph state
	restore := runBuildService
	t.Cleanup(func() { runBuildService = restore })

	repoRoot := t.TempDir()
	outDir := filepath.Join(repoRoot, ".vela")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(.vela) error = %v", err)
	}
	sourcePath := filepath.Join(repoRoot, "main.go")
	if err := os.WriteFile(sourcePath, []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.go) error = %v", err)
	}
	graphPath := filepath.Join(outDir, "graph.json")
	previousGraph := []byte(`{"nodes":[],"edges":[],"meta":{"state":"previous-valid"}}`)
	if err := os.WriteFile(graphPath, previousGraph, 0o644); err != nil {
		t.Fatalf("WriteFile(graph.json) error = %v", err)
	}
	manifestPath := filepath.Join(outDir, "manifest.json")
	previousManifest := []byte(`{
	  "version": 1,
	  "repo_root": ` + strconv.Quote(repoRoot) + `,
	  "generated_at": "2026-04-23T22:47:00Z",
	  "build_mode": "full_rebuild",
	  "files": [
	    {"path":"main.go", "sha256":` + strconv.Quote(testFileSHA256(t, sourcePath)) + `, "status":"active"}
	  ]
	}`)
	if err := os.WriteFile(manifestPath, previousManifest, 0o644); err != nil {
		t.Fatalf("WriteFile(manifest.json) error = %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("package main\nfunc main() { println(\"changed\") }\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.go changed) error = %v", err)
	}

	runBuildService = func(_ context.Context, outDir string, req types.BuildRequest) (buildOutput, error) {
		if err := os.WriteFile(filepath.Join(outDir, "graph.json"), []byte(`{"corrupt":true}`), 0o644); err != nil {
			t.Fatalf("WriteFile(corrupt graph) error = %v", err)
		}
		freshManifest := []byte(`{
		  "version": 1,
		  "repo_root": ` + strconv.Quote(req.RepoRoot) + `,
		  "generated_at": "2026-04-23T22:48:00Z",
		  "build_mode": "full_rebuild",
		  "files": [
		    {"path":"main.go", "sha256":` + strconv.Quote(testFileSHA256(t, sourcePath)) + `, "status":"active"}
		  ]
		}`)
		if err := os.WriteFile(filepath.Join(outDir, "manifest.json"), freshManifest, 0o644); err != nil {
			t.Fatalf("WriteFile(fresh manifest) error = %v", err)
		}
		return buildOutput{}, fmt.Errorf("simulated interrupted update")
	}

	root := rootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"update", repoRoot, "--out-dir", outDir})
	if err := root.Execute(); err == nil {
		t.Fatal("Execute(update) error = nil, want interrupted update error")
	}

	graphBytes, err := os.ReadFile(graphPath)
	if err != nil {
		t.Fatalf("ReadFile(graph.json) error = %v", err)
	}
	if string(graphBytes) != string(previousGraph) {
		t.Fatalf("graph.json = %s, want previous valid graph", graphBytes)
	}
	snapshot, err := igraph.LoadStatusSnapshot(graphPath, 5)
	if err != nil {
		t.Fatalf("LoadStatusSnapshot() error = %v", err)
	}
	if snapshot.Freshness.Status != "stale" {
		t.Fatalf("freshness status after failed update = %q, want stale", snapshot.Freshness.Status)
	}
}

// REQ-014/REQ-006 → SCN-023 → TestSCN023_MCPFixtureServesAndCallsRequiredTools
func TestSCN023_MCPFixtureServesAndCallsRequiredTools(t *testing.T) {
	// Scenario: MCP fixture proves required tools can be served and called.
	restore := serveMCPStdio
	t.Cleanup(func() { serveMCPStdio = restore })

	graphPath := writeMCPFixtureGraph(t)
	var served *mcpserver.MCPServer
	serveMCPStdio = func(srv *mcpserver.MCPServer) error {
		served = srv
		return nil
	}

	root := rootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"serve", "--mcp", "--graph", graphPath})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(serve --mcp) error = %v", err)
	}
	if served == nil {
		t.Fatal("serve --mcp did not start an MCP server")
	}

	toolCalls := map[string]map[string]any{
		"vela_explore": {"query": "AuthService", "limit": 5},
		"vela_lookup":  {"term": "AuthService", "limit": 5},
		"vela_explain": {"subject": "AuthService", "limit": 5},
		"vela_impact":  {"subject": "Database", "limit": 5},
		"vela_path":    {"subject": "AuthService", "target": "Database", "limit": 5},
		"vela_status":  {},
	}
	for toolName, args := range toolCalls {
		tool := served.GetTool(toolName)
		if tool == nil {
			t.Fatalf("required MCP tool %q was not listed by served fixture", toolName)
		}
		res, err := tool.Handler(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: args}})
		if err != nil {
			t.Fatalf("%s handler error = %v", toolName, err)
		}
		core, ok := res.StructuredContent.(query.Result)
		if !ok {
			t.Fatalf("%s structured content = %T, want query.Result", toolName, res.StructuredContent)
		}
		if core.Status != query.ResultStatusOK && len(core.Diagnostics) == 0 {
			t.Fatalf("%s returned %q without structured diagnostic: %+v", toolName, core.Status, core)
		}
	}
}

// REQ-014/REQ-004 → SCN-024 → TestSCN024_CLIMCPEquivalenceFixtureUsesSharedCoreSchema
func TestSCN024_CLIMCPEquivalenceFixtureUsesSharedCoreSchema(t *testing.T) {
	// Scenario: CLI and MCP equivalence fixture proves shared schema behavior.
	restore := serveMCPStdio
	t.Cleanup(func() { serveMCPStdio = restore })

	graphPath := writeMCPFixtureGraph(t)
	engine, err := query.LoadFromFile(graphPath)
	if err != nil {
		t.Fatalf("LoadFromFile(%s) error = %v", graphPath, err)
	}
	expected := engine.ExplainResult("AuthService")

	cliOut := &bytes.Buffer{}
	root := rootCmd()
	root.SetOut(cliOut)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"explain", "AuthService", "--graph", graphPath, "--format", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(CLI explain fixture) error = %v", err)
	}
	var cliCore query.Result
	if err := json.Unmarshal(cliOut.Bytes(), &cliCore); err != nil {
		t.Fatalf("CLI explain fixture output was not shared core JSON: %v\n%s", err, cliOut.String())
	}

	var served *mcpserver.MCPServer
	serveMCPStdio = func(srv *mcpserver.MCPServer) error {
		served = srv
		return nil
	}
	serve := rootCmd()
	serve.SetOut(&bytes.Buffer{})
	serve.SetErr(&bytes.Buffer{})
	serve.SetArgs([]string{"serve", "--mcp", "--graph", graphPath})
	if err := serve.Execute(); err != nil {
		t.Fatalf("Execute(serve --mcp fixture) error = %v", err)
	}
	if served == nil {
		t.Fatal("serve --mcp did not start an MCP server for equivalence fixture")
	}
	res, err := served.GetTool("vela_explain").Handler(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: map[string]any{"subject": "AuthService", "limit": 5}}})
	if err != nil {
		t.Fatalf("MCP explain handler error = %v", err)
	}
	mcpCore, ok := res.StructuredContent.(query.Result)
	if !ok {
		t.Fatalf("MCP explain structured content = %T, want query.Result", res.StructuredContent)
	}

	assertEquivalentCoreResult(t, "CLI", cliCore, expected)
	assertEquivalentCoreResult(t, "MCP", mcpCore, expected)
	if len(cliCore.Facts) != len(mcpCore.Facts) || cliCore.Facts[0].Subject != mcpCore.Facts[0].Subject || cliCore.Facts[0].Predicate != mcpCore.Facts[0].Predicate || cliCore.Facts[0].Object != mcpCore.Facts[0].Object {
		t.Fatalf("CLI/MCP core facts diverged: CLI=%+v MCP=%+v", cliCore.Facts, mcpCore.Facts)
	}
}

// REQ-014 → SCN-025 → TestSCN025_RealWorkspaceSmokeReportIsRedactedReleaseProof
func TestSCN025_RealWorkspaceSmokeReportIsRedactedReleaseProof(t *testing.T) {
	// Scenario: Real workspace smoke test proves release behavior outside toy fixtures.
	reportPath := filepath.Join("..", "..", "reports", "SCN-025-real-workspace-smoke.md")
	reportBytes, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", reportPath, err)
	}
	report := string(reportBytes)

	for _, want := range []string{
		"SCN-025 Real Workspace Smoke Report",
		"workspace: <REAL_WORKSPACE>",
		"redaction_policy: no secrets",
		"vela build <REAL_WORKSPACE>",
		"graph_db: present",
		"vela status --graph <REAL_WORKSPACE>/.vela/graph.json",
		"freshness:",
		"vela lookup",
		"vela explain",
		"MCP tool call: vela_explain",
		"evidence-bearing: yes",
		"secret scan: pass",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("expected smoke report to contain %q, got:\n%s", want, report)
		}
	}
	for _, forbidden := range []string{"/home/geen/Documents/personal/stock-chef", "AIza", "sk-", "ghp_", "BEGIN PRIVATE KEY", "password="} {
		if strings.Contains(report, forbidden) {
			t.Fatalf("smoke report leaked forbidden content %q", forbidden)
		}
	}
}

// REQ-014 → SCN-025 → TestSCN025_RealWorkspaceSmokeHarness
func TestSCN025_RealWorkspaceSmokeHarness(t *testing.T) {
	// Scenario: Real workspace smoke test proves release behavior outside toy fixtures.
	workspace := os.Getenv("VELA_SCN025_WORKSPACE")
	if workspace == "" {
		t.Skip("set VELA_SCN025_WORKSPACE to run the external SCN-025 real workspace smoke")
	}
	restore := serveMCPStdio
	t.Cleanup(func() { serveMCPStdio = restore })

	buildOut := &bytes.Buffer{}
	build := rootCmd()
	build.SetOut(buildOut)
	build.SetErr(&bytes.Buffer{})
	build.SetArgs([]string{"build", workspace})
	if err := build.Execute(); err != nil {
		t.Fatalf("Execute(build real workspace) error = %v", err)
	}
	outDir := filepath.Join(workspace, ".vela")
	graphJSON := filepath.Join(outDir, "graph.json")
	if _, err := os.Stat(filepath.Join(outDir, "graph.db")); err != nil {
		t.Fatalf("graph.db after real workspace build: %v", err)
	}

	var statusErr error
	statusOut := captureStdout(t, func() {
		status := rootCmd()
		status.SetErr(&bytes.Buffer{})
		status.SetArgs([]string{"status", "--graph", graphJSON, "--baseline", ""})
		statusErr = status.Execute()
	})
	if statusErr != nil {
		t.Fatalf("Execute(status real workspace) error = %v", statusErr)
	}
	if !strings.Contains(statusOut, "freshness:") {
		t.Fatalf("status output missing freshness: %q", statusOut)
	}

	subject := "ExecuteAugusteToolUseCase"
	cliOut := &bytes.Buffer{}
	cliExplain := rootCmd()
	cliExplain.SetOut(cliOut)
	cliExplain.SetErr(&bytes.Buffer{})
	cliExplain.SetArgs([]string{"explain", subject, "--graph", graphJSON, "--format", "json"})
	if err := cliExplain.Execute(); err != nil {
		t.Fatalf("Execute(explain real workspace) error = %v", err)
	}
	var cliCore query.Result
	if err := json.Unmarshal(cliOut.Bytes(), &cliCore); err != nil {
		t.Fatalf("real workspace CLI explain was not core JSON: %v", err)
	}
	if cliCore.Status != query.ResultStatusOK || len(cliCore.Facts) == 0 || len(cliCore.Evidence) == 0 {
		t.Fatalf("real workspace CLI explain lacks evidence-bearing graph answer: %+v", cliCore)
	}

	var served *mcpserver.MCPServer
	serveMCPStdio = func(srv *mcpserver.MCPServer) error {
		served = srv
		return nil
	}
	serve := rootCmd()
	serve.SetOut(&bytes.Buffer{})
	serve.SetErr(&bytes.Buffer{})
	serve.SetArgs([]string{"serve", "--mcp", "--graph", graphJSON})
	if err := serve.Execute(); err != nil {
		t.Fatalf("Execute(serve --mcp real workspace) error = %v", err)
	}
	if served == nil {
		t.Fatal("serve --mcp did not start an MCP server for real workspace smoke")
	}
	res, err := served.GetTool("vela_explain").Handler(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: map[string]any{"subject": subject, "limit": 5}}})
	if err != nil {
		t.Fatalf("MCP explain real workspace handler error = %v", err)
	}
	mcpCore, ok := res.StructuredContent.(query.Result)
	if !ok {
		t.Fatalf("MCP explain structured content = %T, want query.Result", res.StructuredContent)
	}
	if mcpCore.Status != query.ResultStatusOK || len(mcpCore.Facts) == 0 || len(mcpCore.Evidence) == 0 {
		t.Fatalf("real workspace MCP explain lacks evidence-bearing graph answer: %+v", mcpCore)
	}
}

func assertEquivalentCoreResult(t *testing.T, adapter string, got, expected query.Result) {
	t.Helper()
	if got.SchemaVersion != expected.SchemaVersion || got.QueryKind != expected.QueryKind || got.Status != expected.Status {
		t.Fatalf("%s core header = (%q, %q, %q), want (%q, %q, %q)", adapter, got.SchemaVersion, got.QueryKind, got.Status, expected.SchemaVersion, expected.QueryKind, expected.Status)
	}
	if len(got.ResolvedSubjects) != len(expected.ResolvedSubjects) || got.ResolvedSubjects[0].ID != expected.ResolvedSubjects[0].ID {
		t.Fatalf("%s resolved subjects = %+v, want %+v", adapter, got.ResolvedSubjects, expected.ResolvedSubjects)
	}
	if len(got.Facts) != len(expected.Facts) || got.Facts[0].Subject != expected.Facts[0].Subject || got.Facts[0].Predicate != expected.Facts[0].Predicate || got.Facts[0].Object != expected.Facts[0].Object {
		t.Fatalf("%s facts = %+v, want %+v", adapter, got.Facts, expected.Facts)
	}
	if got.Facts[0].Confidence != expected.Facts[0].Confidence || got.Freshness.Status != expected.Freshness.Status {
		t.Fatalf("%s proof semantics = confidence %q freshness %q, want confidence %q freshness %q", adapter, got.Facts[0].Confidence, got.Freshness.Status, expected.Facts[0].Confidence, expected.Freshness.Status)
	}
}

func writeMCPFixtureGraph(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	graph := map[string]any{
		"nodes": []map[string]any{
			{"id": "auth", "label": "AuthService", "kind": "struct", "file": "auth.go"},
			{"id": "db", "label": "Database", "kind": "struct", "file": "db.go"},
		},
		"edges": []map[string]any{{"from": "auth", "to": "db", "kind": "uses", "metadata": map[string]any{"evidence_confidence": "extracted"}}},
	}
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent graph error = %v", err)
	}
	path := filepath.Join(dir, "graph.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(graph.json) error = %v", err)
	}
	return path
}

func testFileSHA256(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe() error = %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("Close(stdout pipe) error = %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll(stdout pipe) error = %v", err)
	}
	return string(data)
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

// REQ-005 → SCN-006 → TestSCN006_ExploreResolvesBroadRequestIntoGraphBackedContext
func TestSCN006_ExploreResolvesBroadRequestIntoGraphBackedContext(t *testing.T) {
	// Scenario: Explore resolves natural language into graph-backed structural context.
	graphPath := writeSearchTestGraph(t)

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"explore", "root command", "--graph", graphPath, "--limit", "2"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"Resolved candidates for \"root command\":", "rootCmd", "Graph facts used:", "main [repo/function] --[calls]--> rootCmd [repo/function]"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected stdout to contain %q, got %q", want, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "free-text proof") {
		t.Fatalf("explore output presented free-text matching as proof: %q", stdout.String())
	}
}

// REQ-005/REQ-015 → SCN-007 → TestSCN007_AmbiguousExploreQueryReturnsCandidates
func TestSCN007_AmbiguousExploreQueryReturnsCandidates(t *testing.T) {
	// Scenario: Ambiguous explore query returns candidates instead of choosing silently.
	graphPath := writeAmbiguousExploreGraph(t)

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"explore", "auth", "--graph", graphPath, "--limit", "5"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{
		"Ambiguous explore query for \"auth\"",
		"AuthService",
		"AuthController",
		"file: services/auth/service.go",
		"file: services/auth/controller.go",
		"Refine the request or run `vela lookup \"auth\"` before asking for a strong graph claim.",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected stdout to contain %q, got %q", want, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "Graph facts used:") {
		t.Fatalf("ambiguous explore output chose a graph-backed answer instead of asking for refinement: %q", stdout.String())
	}
}

// REQ-012 → SCN-016 → TestSCN016_MultiRepoExploreRoutesBeforeDeepRetrieval
func TestSCN016_MultiRepoExploreRoutesBeforeDeepRetrieval(t *testing.T) {
	// Scenario: Multi-repo exploration routes first and retrieves deeply second.
	graphPath := writeMultiRepoExploreGraph(t)

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"explore", "billing checkout", "--graph", graphPath, "--limit", "5"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		"Workspace routes for \"billing checkout\":",
		"Route ambiguity: multiple workspace routes match",
		"billing-api score=",
		"checkout-web score=",
		"Workspace routing facts:",
		"billing-api [workspace/repo] --[exposes]--> billing [workspace/service]",
		"Selected workspace routes are routing/topology truth, not deep code truth.",
		"Deep graph retrieval candidates:",
		"BillingHandler",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected stdout to contain %q, got %q", want, output)
		}
	}
	if strings.Index(output, "Workspace routes for") > strings.Index(output, "Deep graph retrieval candidates:") {
		t.Fatalf("expected workspace routes before deep retrieval candidates, got %q", output)
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

func writeAmbiguousExploreGraph(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	graph := map[string]any{
		"nodes": []map[string]any{
			{"id": "services/auth/service.go:AuthService", "label": "AuthService", "kind": "struct", "file": "services/auth/service.go"},
			{"id": "services/auth/controller.go:AuthController", "label": "AuthController", "kind": "struct", "file": "services/auth/controller.go"},
		},
		"edges": []map[string]any{},
		"meta":  map[string]any{"nodeCount": 2, "edgeCount": 0},
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

func writeMultiRepoExploreGraph(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	graph := map[string]any{
		"nodes": []map[string]any{
			{"id": "workspace:repo:billing-api", "label": "billing-api", "kind": "repo", "file": ".vela/workspace.yaml", "metadata": map[string]any{"layer": "workspace", "evidence_type": "routing", "evidence_confidence": "declared_hint"}},
			{"id": "workspace:service:billing", "label": "billing", "kind": "service", "file": ".vela/workspace.yaml", "metadata": map[string]any{"layer": "workspace", "evidence_type": "routing", "evidence_confidence": "declared_hint"}},
			{"id": "workspace:repo:checkout-web", "label": "checkout-web", "kind": "repo", "file": ".vela/workspace.yaml", "metadata": map[string]any{"layer": "workspace", "evidence_type": "routing", "evidence_confidence": "declared_hint"}},
			{"id": "workspace:service:checkout", "label": "checkout", "kind": "service", "file": ".vela/workspace.yaml", "metadata": map[string]any{"layer": "workspace", "evidence_type": "routing", "evidence_confidence": "declared_hint"}},
			{"id": "services/billing/handler.go:BillingHandler", "label": "BillingHandler", "kind": "function", "file": "services/billing/handler.go"},
		},
		"edges": []map[string]any{
			{"from": "workspace:repo:billing-api", "to": "workspace:service:billing", "kind": "exposes", "metadata": map[string]any{"layer": "workspace", "evidence_type": "routing", "evidence_confidence": "declared_hint", "evidence_source_artifact": ".vela/workspace.yaml"}},
			{"from": "workspace:repo:checkout-web", "to": "workspace:service:checkout", "kind": "exposes", "metadata": map[string]any{"layer": "workspace", "evidence_type": "routing", "evidence_confidence": "declared_hint", "evidence_source_artifact": ".vela/workspace.yaml"}},
		},
		"meta": map[string]any{"nodeCount": 5, "edgeCount": 2},
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
