package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Syfra3/vela/internal/query"
)

func TestParseBenchmarkProfiles(t *testing.T) {
	profiles, err := parseBenchmarkProfiles("federated,ancora,graph,graph-hybrid,lexical,structural,vector")
	if err != nil {
		t.Fatalf("parseBenchmarkProfiles() error = %v", err)
	}
	if len(profiles) != 7 {
		t.Fatalf("len(profiles) = %d, want 7", len(profiles))
	}
}

func TestLoadRetrievalBenchmarkSuite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "suite.json")
	if err := os.WriteFile(path, []byte(`{
		"name": "smoke",
		"limit": 5,
		"cases": [{"name": "retriever", "query": "federated retriever", "relevant_ids": ["code:retriever"]}]
	}`), 0o644); err != nil {
		t.Fatalf("write suite: %v", err)
	}
	suite, err := loadRetrievalBenchmarkSuite(path)
	if err != nil {
		t.Fatalf("loadRetrievalBenchmarkSuite() error = %v", err)
	}
	if suite.Name != "smoke" || len(suite.Cases) != 1 {
		t.Fatalf("unexpected suite: %#v", suite)
	}
}

func TestLoadCuratedRetrievalBenchmarkSuite(t *testing.T) {
	path := filepath.Join("..", "..", "bench", "retrieval", "vela-curated.json")
	suite, err := loadRetrievalBenchmarkSuite(path)
	if err != nil {
		t.Fatalf("loadRetrievalBenchmarkSuite(curated) error = %v", err)
	}
	if suite.Name != "vela-curated" {
		t.Fatalf("suite.Name = %q, want vela-curated", suite.Name)
	}
	if len(suite.Cases) < 5 {
		t.Fatalf("len(suite.Cases) = %d, want at least 5", len(suite.Cases))
	}
}

func TestBenchmarkMetricHelpers(t *testing.T) {
	relevance := map[string]float64{"a": 3, "b": 1}
	topIDs := []string{"x", "a", "b"}
	if got := recallAtK(topIDs, relevance); got != 1 {
		t.Fatalf("recallAtK() = %f, want 1", got)
	}
	if got := reciprocalRank(topIDs, relevance); got != 0.5 {
		t.Fatalf("reciprocalRank() = %f, want 0.5", got)
	}
	if got := ndcgAtK(topIDs, relevance); got <= 0 || got > 1 {
		t.Fatalf("ndcgAtK() = %f, want (0,1]", got)
	}
}

func TestWriteRetrievalBenchSnapshot(t *testing.T) {
	historyDir := t.TempDir()
	result := retrievalBenchmarkResult{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Suite:       "smoke",
		Settings:    retrievalBenchmarkSettings{MaxHops: 2, MaxExpansions: 24},
		Summary: map[string]retrievalProfileSummary{
			string(query.SearchProfileFederated): {Queries: 1, RecallAtK: 1},
		},
	}
	path, err := writeRetrievalBenchSnapshot(historyDir, result)
	if err != nil {
		t.Fatalf("writeRetrievalBenchSnapshot() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if !strings.Contains(string(data), `"suite": "smoke"`) {
		t.Fatalf("snapshot missing suite: %s", data)
	}
	if !strings.Contains(string(data), `"max_hops": 2`) {
		t.Fatalf("snapshot missing settings: %s", data)
	}
}

func TestLatestRetrievalBenchSnapshot(t *testing.T) {
	historyDir := t.TempDir()
	first := retrievalBenchmarkResult{GeneratedAt: "2026-04-18T20:00:00Z", Suite: "smoke", Summary: map[string]retrievalProfileSummary{string(query.SearchProfileFederated): {Queries: 1}}}
	second := retrievalBenchmarkResult{GeneratedAt: "2026-04-18T20:01:00Z", Suite: "smoke", Summary: map[string]retrievalProfileSummary{string(query.SearchProfileFederated): {Queries: 2}}}
	if _, err := writeRetrievalBenchSnapshot(historyDir, first); err != nil {
		t.Fatalf("write first snapshot: %v", err)
	}
	secondPath, err := writeRetrievalBenchSnapshot(historyDir, second)
	if err != nil {
		t.Fatalf("write second snapshot: %v", err)
	}
	path, result, err := latestRetrievalBenchSnapshot(historyDir)
	if err != nil {
		t.Fatalf("latestRetrievalBenchSnapshot() error = %v", err)
	}
	if path != secondPath {
		t.Fatalf("latest path = %q, want %q", path, secondPath)
	}
	if result == nil || result.Summary[string(query.SearchProfileFederated)].Queries != 2 {
		t.Fatalf("unexpected latest result: %#v", result)
	}
}

func TestBuildRetrievalDeltas(t *testing.T) {
	deltas := buildRetrievalDeltas(
		map[string]retrievalProfileSummary{"federated": {RecallAtK: 0.8, MRR: 0.6, AvgLatencyMs: 12, P95LatencyMs: 20}},
		map[string]retrievalProfileSummary{"federated": {RecallAtK: 0.5, MRR: 0.4, AvgLatencyMs: 10, P95LatencyMs: 18}},
	)
	delta := deltas["federated"]
	if math.Abs(delta.RecallAtKDelta-0.3) > 1e-9 {
		t.Fatalf("RecallAtKDelta = %f, want 0.3", delta.RecallAtKDelta)
	}
	if delta.P95LatencyMsDelta != 2 {
		t.Fatalf("P95LatencyMsDelta = %d, want 2", delta.P95LatencyMsDelta)
	}
}
