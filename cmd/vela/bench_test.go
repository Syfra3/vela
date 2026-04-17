package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	igraph "github.com/Syfra3/vela/internal/graph"
)

func TestSanitizeHistoryName(t *testing.T) {
	got := sanitizeHistoryName("Graph Report.JSON")
	if got != "graph-report-json" {
		t.Fatalf("sanitizeHistoryName() = %q, want %q", got, "graph-report-json")
	}
}

func TestLatestBenchSnapshotMissingDir(t *testing.T) {
	path, err := latestBenchSnapshot(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("latestBenchSnapshot() error = %v", err)
	}
	if path != "" {
		t.Fatalf("latestBenchSnapshot() = %q, want empty path", path)
	}
}

func TestWriteBenchSnapshotAndResolveLatest(t *testing.T) {
	historyDir := t.TempDir()

	first := igraph.HealthMetrics{GeneratedAt: "2026-04-17T18:10:01Z", Nodes: 1}
	second := igraph.HealthMetrics{GeneratedAt: "2026-04-17T18:10:02Z", Nodes: 2}

	firstPath, err := writeBenchSnapshot(historyDir, first)
	if err != nil {
		t.Fatalf("writeBenchSnapshot(first) error = %v", err)
	}
	secondPath, err := writeBenchSnapshot(historyDir, second)
	if err != nil {
		t.Fatalf("writeBenchSnapshot(second) error = %v", err)
	}

	latest, err := latestBenchSnapshot(historyDir)
	if err != nil {
		t.Fatalf("latestBenchSnapshot() error = %v", err)
	}
	if latest != secondPath {
		t.Fatalf("latestBenchSnapshot() = %q, want %q", latest, secondPath)
	}
	if latest == firstPath {
		t.Fatalf("latestBenchSnapshot() returned first snapshot, want latest")
	}

	data, err := os.ReadFile(latest)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", latest, err)
	}
	if !strings.Contains(string(data), "\"nodes\": 2") {
		t.Fatalf("snapshot contents missing expected metrics: %s", data)
	}
	if filepath.Ext(latest) != ".json" {
		t.Fatalf("snapshot extension = %q, want .json", filepath.Ext(latest))
	}
}
