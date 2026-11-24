package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/internal/generation"
	"github.com/Syfra3/vela/pkg/types"
	mcppkg "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// Bind fixture provenance to the closed inputs verified by runtime freshness.
func bindFreshFixtureManifest(t *testing.T, m *types.Manifest) {
	t.Helper()
	c, err := generation.ClosedInputs(m.RepoRoot)
	if err != nil {
		t.Fatal(err)
	}
	m.SourceBinding = generation.BoundGoProfile
	m.ConsumedFiles = c.Files
	m.DirectoryFingerprint = c.DirectoryFingerprint
	m.DependencyFingerprint = c.DependencyFingerprint
	m.ClosureConfigFingerprint = c.ConfigFingerprint
}

func TestCLIAndMCPSelectionParity(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workspace := t.TempDir()
	t.Chdir(workspace)
	firstRoot := filepath.Join(home, "one", "stock-chef")
	secondRoot := filepath.Join(home, "two", "stock-chef")
	first := writeStockChefSelectionRuntimeGraph(t, filepath.Join(firstRoot, ".vela"), firstRoot, "LocalCaller", "local.go")
	second := writeStockChefSelectionRuntimeGraph(t, filepath.Join(secondRoot, ".vela"), secondRoot, "DepEvalCaller", "other.go")
	registry := filepath.Join(home, ".vela", "registry.json")
	if err := os.MkdirAll(filepath.Dir(registry), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(registry, []byte(fmt.Sprintf(`{"version":1,"entries":[{"repo_root":%q,"name":"stock-chef","graph_path":%q},{"repo_root":%q,"name":"stock-chef","graph_path":%q}]}`, firstRoot, first, secondRoot, second)), 0600); err != nil {
		t.Fatal(err)
	}
	old := serveMCPStdio
	t.Cleanup(func() { serveMCPStdio = old })
	var srv *mcpserver.MCPServer
	serveMCPStdio = func(s *mcpserver.MCPServer) error { srv = s; return nil }
	cmd := rootCmd()
	cmd.SetArgs([]string{"serve", "--mcp"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	check := func(want string) {
		t.Helper()
		var out bytes.Buffer
		cli := rootCmd()
		cli.SetOut(&out)
		cli.SetErr(&out)
		cli.SetArgs([]string{"explore", "SharedStockChefSymbol"})
		err := cli.Execute()
		if err == nil {
			t.Fatal("CLI accepted unresolved selection")
		}
		out.WriteString(err.Error())
		r, err := srv.GetTool("reverse_dependencies").Handler(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: map[string]any{"subject": "SharedStockChefSymbol"}}})
		if err != nil {
			t.Fatal(err)
		}
		for name, text := range map[string]string{"cli": out.String(), "mcp": callToolResultText(t, r)} {
			if !strings.Contains(strings.ToLower(text), want) || strings.Contains(text, "LocalCaller") || strings.Contains(text, "DepEvalCaller") || strings.Contains(text, "Graph evidence:") {
				t.Fatalf("%s: %s", name, text)
			}
		}
	}
	check("ambiguous")
	for _, path := range []string{first, second} {
		var out bytes.Buffer
		cli := rootCmd()
		cli.SetOut(&out)
		cli.SetErr(&out)
		cli.SetArgs([]string{"explore", "SharedStockChefSymbol"})
		err := cli.Execute()
		if err == nil || !strings.Contains(err.Error(), path) {
			t.Fatalf("missing candidate %s: %v", path, err)
		}
	}
	active := filepath.Join(workspace, ".vela", "graph.json")
	if err := os.MkdirAll(filepath.Dir(active), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(active, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	check("unavailable")
}

// Exercise the real serve command's provider through publication, source edits,
// replacement and a corrupt selected generation, without indexing a workspace.
func TestServeMCPPublicationLifecycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	t.Chdir(root)
	source := filepath.Join(root, "main.go")
	if err := os.WriteFile(source, []byte("package fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	old := serveMCPStdio
	t.Cleanup(func() { serveMCPStdio = old })
	var srv *mcpserver.MCPServer
	serveMCPStdio = func(s *mcpserver.MCPServer) error { srv = s; return nil }
	cmd := rootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"serve", "--mcp"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if srv == nil || len(srv.ListTools()) == 0 {
		t.Fatal("MCP tools unavailable")
	}
	call := func() string {
		t.Helper()
		r, err := srv.GetTool("dependencies").Handler(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: map[string]any{"subject": "A"}}})
		if err != nil {
			t.Fatal(err)
		}
		return callToolResultText(t, r)
	}
	check := func(want string, forbidden string) {
		t.Helper()
		got := call()
		if !strings.Contains(strings.ToLower(got), strings.ToLower(want)) || forbidden != "" && strings.Contains(got, forbidden) {
			t.Fatalf("want %q without %q: %s", want, forbidden, got)
		}
	}
	publish := func(label string) {
		t.Helper()
		m, _, err := export.Inventory(root, types.ManifestRequest{})
		if err != nil {
			t.Fatal(err)
		}
		bindFreshFixtureManifest(t, m)
		w, err := export.AcquireWriter(context.Background(), filepath.Join(root, ".vela"))
		if err != nil {
			t.Fatal(err)
		}
		defer w.Close()
		_, err = w.Publish(&types.Graph{Nodes: []types.Node{{ID: "a", Label: "A", NodeType: "function"}, {ID: "b", Label: label, NodeType: "function"}}, Edges: []types.Edge{{Source: "a", Target: "b", Relation: "uses"}}}, m, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	check("unavailable", "Graph evidence:")
	publish("FirstTarget")
	check("FirstTarget", "")
	check("fresh", "")
	if err := os.WriteFile(source, []byte("package fixture\n// changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	check("stale", "")
	publish("SecondTarget")
	check("SecondTarget", "FirstTarget")
	check("fresh", "")
	// Break only the fixture's selected generation, leaving an older valid one.
	selected, err := export.ResolveGeneration(filepath.Join(root, ".vela"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(selected.Dir, "graph.db"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	check("unavailable", "FirstTarget")
}
