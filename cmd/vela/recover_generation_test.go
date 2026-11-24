package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/pkg/types"
)

func TestRecoverGenerationCLIIsolatedFixture(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, ".vela")
	m, _, err := export.Inventory(root, types.ManifestRequest{})
	if err != nil {
		t.Fatal(err)
	}
	w, err := export.AcquireWriter(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	old, err := w.Publish(&types.Graph{Nodes: []types.Node{{ID: "a", Label: "a", NodeType: "file"}}}, m, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err := w.Publish(&types.Graph{Nodes: []types.Node{{ID: "b", Label: "b", NodeType: "file"}}}, m, nil)
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	if err := os.Remove(filepath.Join(out, ".current")); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	cmd := rootCmd()
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"recover-generation", out})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(output.String()) != s.ID {
		t.Fatalf("output %q", output.String())
	}
	selected, err := export.ResolveGeneration(out)
	if err != nil || selected.ID != s.ID {
		t.Fatalf("recovery: %v", err)
	}
	if _, err := export.ValidateGeneration(old.Dir); err != nil {
		t.Fatal("history pruned or changed", err)
	}
}
