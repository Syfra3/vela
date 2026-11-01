package graph_test

import (
	"context"
	"github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/pkg/types"
	"os"
	"path/filepath"
	"testing"
)

func TestHarnessReliabilityStatusHealthPinnedAndUnavailable(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, ".vela")
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a"), 0600); err != nil {
		t.Fatal(err)
	}
	publish := func(id string) *export.Snapshot {
		t.Helper()
		m, _, err := export.Inventory(root, types.ManifestRequest{})
		if err != nil {
			t.Fatal(err)
		}
		w, err := export.AcquireWriter(context.Background(), out)
		if err != nil {
			t.Fatal(err)
		}
		defer w.Close()
		s, err := w.Publish(&types.Graph{Nodes: []types.Node{{ID: id, Label: id, NodeType: "file"}}}, m, nil)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	a := publish("A")
	publish("B")
	status, err := graph.LoadStatusSnapshot(filepath.Join(a.Dir, "graph.json"), 5)
	if err != nil || status.Metrics.Nodes != 1 {
		t.Fatalf("status %v %v", status, err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.go"), []byte("package b"), 0600); err != nil {
		t.Fatal(err)
	}
	status, err = graph.LoadStatusSnapshot(filepath.Join(out, "graph.json"), 5)
	if err != nil || status.Freshness.Status != "stale" {
		t.Fatalf("addition status %v %v", status.Freshness, err)
	}
	if err := os.Remove(filepath.Join(out, ".current")); err != nil {
		t.Fatal(err)
	}
	if _, err := graph.LoadHealthMetrics(filepath.Join(out, "graph.json"), 5); err == nil {
		t.Fatal("health fallback")
	}
	if _, err := graph.LoadStatusSnapshot(filepath.Join(out, "graph.json"), 5); err == nil {
		t.Fatal("status fallback")
	}
}

func TestHarnessReliabilityPublicReportSurvivesTruthPin(t *testing.T) {
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
	_, err = w.Publish(&types.Graph{}, m, nil)
	w.Close()
	if err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(out, "GRAPH_REPORT.md")
	if err := os.WriteFile(report, []byte("report"), 0600); err != nil {
		t.Fatal(err)
	}
	status, err := graph.LoadStatusSnapshot(filepath.Join(out, "graph.json"), 5)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Freshness.ReportPresent || status.Freshness.ReportPath != report {
		t.Fatalf("public report lost: %+v", status.Freshness)
	}
}
