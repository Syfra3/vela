package query

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	graphExport "github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/pkg/types"
)

func TestRunRequestSupportsGraphTruthQueryKinds(t *testing.T) {
	t.Parallel()

	engine := loadRequestTestEngine(t)
	tests := []struct {
		name string
		req  types.QueryRequest
		want []string
	}{
		{
			name: "dependencies",
			req:  types.QueryRequest{Kind: types.QueryKindDependencies, Subject: "AuthService", Limit: 5},
			want: []string{"Dependencies for \"AuthService\"", "Database", "UserRepo"},
		},
		{
			name: "reverse dependencies",
			req:  types.QueryRequest{Kind: types.QueryKindReverseDependencies, Subject: "Database", Limit: 5},
			want: []string{"Reverse dependencies for \"Database\"", "AuthService", "UserRepo"},
		},
		{
			name: "impact",
			req:  types.QueryRequest{Kind: types.QueryKindImpact, Subject: "Database", Limit: 5},
			want: []string{"Impact for \"Database\"", "AuthService", "APIHandler"},
		},
		{
			name: "path",
			req:  types.QueryRequest{Kind: types.QueryKindPath, Subject: "APIHandler", Target: "Database"},
			want: []string{"APIHandler", "AuthService", "Database"},
		},
		{
			name: "explain",
			req:  types.QueryRequest{Kind: types.QueryKindExplain, Subject: "AuthService"},
			want: []string{"Edges for \"AuthService\"", "APIHandler", "Database"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.RunRequest(tt.req)
			if err != nil {
				t.Fatalf("RunRequest() error = %v", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(result, want) {
					t.Fatalf("expected %q in result, got:\n%s", want, result)
				}
			}
		})
	}
}

func TestRunRequest_FileQueriesPreferFileDependencyEdges(t *testing.T) {
	t.Parallel()

	graph := map[string]any{
		"nodes": []map[string]any{
			{"id": "project:vela", "label": "vela", "kind": "project", "file": "vela"},
			{"id": "vela:file:cmd/vela/main.go", "label": "cmd/vela/main.go", "kind": "file", "file": "cmd/vela/main.go"},
			{"id": "vela:file:internal/config/config.go", "label": "internal/config/config.go", "kind": "file", "file": "internal/config/config.go"},
			{"id": "vela:file:pkg/types/types.go", "label": "pkg/types/types.go", "kind": "file", "file": "pkg/types/types.go"},
		},
		"edges": []map[string]any{
			{"from": "project:vela", "to": "vela:file:cmd/vela/main.go", "kind": "contains"},
			{"from": "project:vela", "to": "vela:file:internal/config/config.go", "kind": "contains"},
			{"from": "project:vela", "to": "vela:file:pkg/types/types.go", "kind": "contains"},
			{"from": "vela:file:cmd/vela/main.go", "to": "vela:file:internal/config/config.go", "kind": "depends_on"},
			{"from": "vela:file:internal/config/config.go", "to": "vela:file:pkg/types/types.go", "kind": "depends_on"},
		},
	}
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	engine, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}

	pathResult, err := engine.RunRequest(types.QueryRequest{Kind: types.QueryKindPath, Subject: "cmd/vela/main.go", Target: "pkg/types/types.go"})
	if err != nil {
		t.Fatalf("RunRequest(path) error = %v", err)
	}
	for _, want := range []string{"cmd/vela/main.go", "internal/config/config.go", "pkg/types/types.go"} {
		if !strings.Contains(pathResult, want) {
			t.Fatalf("expected %q in path result, got %q", want, pathResult)
		}
	}

	reverseResult, err := engine.RunRequest(types.QueryRequest{Kind: types.QueryKindReverseDependencies, Subject: "pkg/types/types.go", Limit: 5})
	if err != nil {
		t.Fatalf("RunRequest(reverse) error = %v", err)
	}
	if !strings.Contains(reverseResult, "internal/config/config.go") {
		t.Fatalf("expected file reverse dependency in result, got %q", reverseResult)
	}
	if strings.Contains(reverseResult, "vela [repo/project]") {
		t.Fatalf("did not expect containment-only reverse dependency result, got %q", reverseResult)
	}
}

// REQ-011/REQ-012 → SCN-004 → TestSCN004_DependencyCompatibilityThroughSQLiteRuntimeBoundary
func TestSCN004_DependencyCompatibilityThroughSQLiteRuntimeBoundary(t *testing.T) {
	// Scenario: Existing dependency queries keep working after IR support is added.
	dir := t.TempDir()
	velaDir := filepath.Join(dir, ".vela")
	if err := os.Mkdir(velaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	graphJSON := filepath.Join(velaDir, "graph.json")
	if err := os.WriteFile(graphJSON, []byte(`{"nodes":[],"edges":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := graphExport.WriteSQLiteGraphAtomic(&types.Graph{
		Nodes: []types.Node{
			{ID: "subject-a", Label: "SubjectA", NodeType: "service", SourceFile: "subject.go"},
			{ID: "legacy-b", Label: "DependencyB", NodeType: "service", SourceFile: "legacy.go"},
			{ID: "ir-c", Label: "IRDependencyC", NodeType: "service", SourceFile: "ir.go"},
		},
		Edges: []types.Edge{
			{Source: "subject-a", Target: "legacy-b", Relation: string(types.FactKindDependsOn)},
			{
				Source:   "subject-a",
				Target:   "ir-c",
				Relation: string(types.FactKindDependsOn),
				Metadata: map[string]interface{}{
					"common_ir":                true,
					"ir_kind":                  "DEPENDS_ON",
					"ir_origin":                "deterministic_extractor",
					"evidence_confidence":      "extracted",
					"freshness":                "fresh",
					"evidence_type":            "common-ir",
					"evidence_source_artifact": "ir.go",
				},
			},
		},
	}, velaDir); err != nil {
		t.Fatalf("WriteSQLiteGraphAtomic error: %v", err)
	}

	engine, err := LoadFromFile(graphJSON)
	if err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}
	result, err := engine.RunRequest(types.QueryRequest{Kind: types.QueryKindDependencies, Subject: "SubjectA", Limit: 5})
	if err != nil {
		t.Fatalf("RunRequest(dependencies) error: %v", err)
	}

	if !strings.Contains(result, "DependencyB") {
		t.Fatalf("expected legacy dependency B to remain visible, got:\n%s", result)
	}
	for _, want := range []string{"IRDependencyC", "kind=DEPENDS_ON", "origin=deterministic", "confidence=high", "freshness=fresh"} {
		if !strings.Contains(result, want) {
			t.Fatalf("expected additive IR metadata %q in dependency result, got:\n%s", want, result)
		}
	}
	if strings.Contains(result, "DependencyB [repo/service] via depends_on {") {
		t.Fatalf("legacy dependency should not be mislabeled as IR-backed, got:\n%s", result)
	}
}

// REQ-011/REQ-012 → SCN-005 → TestSCN005_ImpactCompatibilityThroughSQLiteRuntimeBoundary
func TestSCN005_ImpactCompatibilityThroughSQLiteRuntimeBoundary(t *testing.T) {
	// Scenario: Existing impact queries keep working after migration or compatibility wrapping.
	dir := t.TempDir()
	velaDir := filepath.Join(dir, ".vela")
	if err := os.Mkdir(velaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	graphJSON := filepath.Join(velaDir, "graph.json")
	if err := os.WriteFile(graphJSON, []byte(`{"nodes":[],"edges":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := graphExport.WriteSQLiteGraphAtomic(&types.Graph{
		Nodes: []types.Node{
			{ID: "subject-a", Label: "SubjectA", NodeType: "service", SourceFile: "subject-a.go"},
			{ID: "subject-c", Label: "impacted subject C", NodeType: "service", SourceFile: "subject-c.go"},
		},
		Edges: []types.Edge{{
			Source:   "subject-c",
			Target:   "subject-a",
			Relation: string(types.FactKindDependsOn),
			Metadata: map[string]interface{}{
				"stable_id":                "legacy-impact-a-c",
				"evidence_type":            "legacy-impact",
				"evidence_source_artifact": "legacy-impact.go",
				"evidence_snippet":         "subject A impacts subject C",
				"source_range":             "legacy-unavailable: legacy graph impact did not record a source range",
				"extractor_version":        "legacy-unavailable: legacy graph impact did not record an extractor version",
			},
		}},
	}, velaDir); err != nil {
		t.Fatalf("WriteSQLiteGraphAtomic error: %v", err)
	}

	engine, err := LoadFromFile(graphJSON)
	if err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}
	result, err := engine.RunRequest(types.QueryRequest{Kind: types.QueryKindImpact, Subject: "SubjectA", Limit: 5})
	if err != nil {
		t.Fatalf("RunRequest(impact) error: %v", err)
	}

	for _, want := range []string{
		"impacted subject C",
		"legacy-impact-a-c",
		"subject-c",
		"legacy-impact.go",
		"subject A impacts subject C",
		"legacy-unavailable: legacy graph impact did not record a source range",
		"legacy-unavailable: legacy graph impact did not record an extractor version",
	} {
		if !strings.Contains(result, want) {
			t.Fatalf("expected impact runtime result to expose %q, got:\n%s", want, result)
		}
	}
}

// REQ-007/REQ-008/REQ-013 → SCN-011 → TestSCN011_EnrichmentPersistenceReuseThroughSQLiteRuntimeBoundary
func TestSCN011_EnrichmentPersistenceReuseThroughSQLiteRuntimeBoundary(t *testing.T) {
	// Scenario: Approved exploration is persisted as typed enrichment and reused.
	dir := t.TempDir()
	velaDir := filepath.Join(dir, ".vela")
	if err := os.Mkdir(velaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	graphJSON := filepath.Join(velaDir, "graph.json")
	if err := os.WriteFile(graphJSON, []byte(`{"nodes":[],"edges":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := graphExport.WriteSQLiteGraphAtomic(&types.Graph{
		Nodes: []types.Node{
			{ID: "handler-checkout", Label: "CheckoutHandler", NodeType: "handler", SourceFile: "checkout/handler.go"},
			{ID: "effect-session-cookie", Label: "writes session cookie", NodeType: "side_effect", SourceFile: "checkout/handler.go"},
		},
		Edges: []types.Edge{{
			Source:   "handler-checkout",
			Target:   "effect-session-cookie",
			Relation: "side_effect",
			Metadata: map[string]interface{}{
				"common_ir":                true,
				"ir_kind":                  "SIDE_EFFECT",
				"ir_origin":                "exploration_enriched",
				"freshness":                "fresh",
				"source_file_hash":         "sha256:checkout-handler-v1",
				"last_seen_at":             "2026-06-28T20:00:00Z",
				"evidence_confidence":      "high",
				"evidence_type":            "targeted-exploration",
				"evidence_source_artifact": "checkout/handler.go",
				"evidence_snippet":         "http.SetCookie(w, sessionCookie)",
				"source_range":             "L42-L44",
				"extractor_name":           "targeted-exploration",
				"extractor_version":        "enrichment-v1",
			},
		}},
	}, velaDir); err != nil {
		t.Fatalf("WriteSQLiteGraphAtomic error: %v", err)
	}

	firstSession, err := LoadFromFile(graphJSON)
	if err != nil {
		t.Fatalf("LoadFromFile(first session) error: %v", err)
	}
	firstResult, err := firstSession.RunRequest(types.QueryRequest{Kind: types.QueryKindExplain, Subject: "CheckoutHandler", Limit: 5})
	if err != nil {
		t.Fatalf("RunRequest(first explain) error: %v", err)
	}

	secondSession, err := LoadFromFile(graphJSON)
	if err != nil {
		t.Fatalf("LoadFromFile(second session) error: %v", err)
	}
	secondResult, err := secondSession.RunRequest(types.QueryRequest{Kind: types.QueryKindExplain, Subject: "CheckoutHandler", Limit: 5})
	if err != nil {
		t.Fatalf("RunRequest(second explain) error: %v", err)
	}

	for _, result := range []string{firstResult, secondResult} {
		for _, want := range []string{
			"CheckoutHandler",
			"writes session cookie",
			"kind=SIDE_EFFECT",
			"origin=exploration_enriched",
			"freshness=fresh",
			"source_hash=sha256:checkout-handler-v1",
			"last_seen=2026-06-28T20:00:00Z",
			"confidence=high",
			"type=targeted-exploration",
			"artifact=checkout/handler.go",
			"evidence=http.SetCookie(w, sessionCookie)",
			"range=L42-L44",
			"extractor=targeted-exploration@enrichment-v1",
			"exploration_reused=true",
		} {
			if !strings.Contains(result, want) {
				t.Fatalf("expected persisted enrichment metadata %q in runtime result, got:\n%s", want, result)
			}
		}
		for _, forbidden := range []string{"approval_required", "repeat exploration", "exploration_repeated=true"} {
			if strings.Contains(result, forbidden) {
				t.Fatalf("expected fresh persisted enrichment to avoid %q, got:\n%s", forbidden, result)
			}
		}
	}
}

// REQ-015/REQ-011 → SCN-016 → TestSCN016_LowLevelQueriesLabelLegacyAndIREvidenceThroughSQLiteRuntimeBoundary
func TestSCN016_LowLevelQueriesLabelLegacyAndIREvidenceThroughSQLiteRuntimeBoundary(t *testing.T) {
	// Scenario: Prior runtime and low-level graph behavior coexists with the new IR.
	graphJSON := writeSCN016MixedRuntimeGraph(t)
	engine, err := LoadFromFile(graphJSON)
	if err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}

	lookup := engine.RenderLookup("CheckoutService", 5)
	assertContainsAll(t, lookup, []string{"Candidates for \"CheckoutService\"", "CheckoutService"})
	assertNoFullReplacementClaim(t, lookup)

	queries := []struct {
		name string
		req  types.QueryRequest
		want []string
	}{
		{
			name: "dependencies",
			req:  types.QueryRequest{Kind: types.QueryKindDependencies, Subject: "CheckoutService", Limit: 5},
			want: []string{"LegacyGateway", "legacy-backed", "IRRepository", "IR-backed", "kind=DEPENDS_ON", "origin=deterministic"},
		},
		{
			name: "reverse_dependencies",
			req:  types.QueryRequest{Kind: types.QueryKindReverseDependencies, Subject: "CheckoutService", Limit: 5},
			want: []string{"LegacyHandler", "legacy-backed", "IRJob", "IR-backed", "kind=CALLS", "origin=deterministic"},
		},
		{
			name: "path",
			req:  types.QueryRequest{Kind: types.QueryKindPath, Subject: "CheckoutService", Target: "IRRepository", Limit: 5},
			want: []string{"CheckoutService", "IRRepository", "IR-backed"},
		},
		{
			name: "explain",
			req:  types.QueryRequest{Kind: types.QueryKindExplain, Subject: "CheckoutService", Limit: 5},
			want: []string{"LegacyGateway", "legacy-backed", "IRRepository", "IR-backed"},
		},
		{
			name: "impact",
			req:  types.QueryRequest{Kind: types.QueryKindImpact, Subject: "CheckoutService", Limit: 5},
			want: []string{"LegacyHandler", "legacy-backed", "IRJob", "IR-backed"},
		},
	}

	for _, tt := range queries {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.RunRequest(tt.req)
			if err != nil {
				t.Fatalf("RunRequest(%s) error: %v", tt.name, err)
			}
			assertContainsAll(t, result, tt.want)
			assertNoFullReplacementClaim(t, result)
		})
	}
}

// REQ-006/REQ-010/REQ-014 → SCN-009 → TestSCN009_StaleCommonIRFactsAreDiagnosticThroughSQLiteRuntimeBoundary
func TestSCN009_StaleCommonIRFactsAreDiagnosticThroughSQLiteRuntimeBoundary(t *testing.T) {
	// Scenario: Stale facts are disclosed and do not satisfy high-confidence gates.
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "schema.go")
	if err := os.WriteFile(sourcePath, []byte("package checkout\ntype CheckoutSchema struct{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	staleHash := "sha256:definitely-not-current"
	graphJSON := writeRuntimeGraph(t, dir, &types.Graph{
		Nodes: []types.Node{
			{ID: "route-checkout", Label: "POST /checkout", NodeType: "route", SourceFile: "schema.go"},
			{ID: "schema-checkout", Label: "CheckoutSchema", NodeType: "schema", SourceFile: "schema.go"},
		},
		Edges: []types.Edge{{
			Source:   "route-checkout",
			Target:   "schema-checkout",
			Relation: "uses_schema",
			Metadata: map[string]interface{}{
				"common_ir":                true,
				"ir_kind":                  "USES_SCHEMA",
				"ir_origin":                "deterministic",
				"freshness":                "fresh",
				"source_file_hash":         staleHash,
				"evidence_confidence":      "high",
				"evidence_type":            "common-ir",
				"evidence_source_artifact": "schema.go",
			},
		}},
	})
	writeRuntimeManifest(t, filepath.Dir(graphJSON), dir, "schema.go", staleHash)

	engine, err := LoadFromFile(graphJSON)
	if err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}
	result := engine.ExplainResult("POST /checkout")

	if result.Freshness.Status != FreshnessStale {
		t.Fatalf("expected stale runtime freshness, got %+v", result.Freshness)
	}
	if result.Status == ResultStatusOK || result.Confidence == types.ConfidenceExtracted {
		t.Fatalf("stale Common IR facts must not satisfy high-confidence answer claims, got status=%s confidence=%s diagnostics=%+v", result.Status, result.Confidence, result.Diagnostics)
	}
	assertDiagnostic(t, result.Diagnostics, "STALE_GRAPH", "schema.go")
}

// REQ-009/REQ-010/REQ-014 → SCN-013 → TestSCN013_DeterministicFactWinsConflictThroughSQLiteRuntimeBoundary
func TestSCN013_DeterministicFactWinsConflictThroughSQLiteRuntimeBoundary(t *testing.T) {
	// Scenario: Deterministic facts win over conflicting enrichment.
	graphJSON := writeRuntimeGraph(t, t.TempDir(), &types.Graph{
		Nodes: []types.Node{
			{ID: "route-checkout", Label: "POST /checkout", NodeType: "route", SourceFile: "routes.go"},
			{ID: "handler-b", Label: "HandlerB", NodeType: "handler", SourceFile: "handler_b.go"},
			{ID: "handler-a", Label: "HandlerA", NodeType: "handler", SourceFile: "handler_a.go"},
		},
		Edges: []types.Edge{
			commonIRHandlerEdge("route-checkout", "handler-b", "exploration_enriched", "conflict", "exploration saw HandlerB"),
			commonIRHandlerEdge("route-checkout", "handler-a", "deterministic", "authoritative", "router binds HandlerA"),
		},
	})

	engine, err := LoadFromFile(graphJSON)
	if err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}
	result := engine.ExplainResult("POST /checkout")

	if len(result.Facts) < 2 {
		t.Fatalf("expected authoritative and conflicting facts, got %+v", result.Facts)
	}
	if result.Facts[0].Object != "handler-a" {
		t.Fatalf("expected deterministic HandlerA fact to rank first, got %+v", result.Facts)
	}
	conflict := result.Facts[1]
	if conflict.Object != "handler-b" || conflict.Metadata["ir_origin"] != "exploration_enriched" || conflict.Metadata["claim_status"] != "conflict" {
		t.Fatalf("expected conflicting enrichment to remain observable with metadata, got %+v", conflict)
	}
	assertDiagnostic(t, result.Diagnostics, "EVIDENCE_CONFLICT", "authoritative")
}

// REQ-005/REQ-007/REQ-010 → SCN-007 and REQ-010/REQ-013/REQ-014 → SCN-015 → TestSCN015_MissingTestUsageIsUnknownThroughSQLiteRuntimeBoundary
func TestSCN015_MissingTestUsageIsUnknownThroughSQLiteRuntimeBoundary(t *testing.T) {
	// Scenario: Empty results distinguish known-empty from unavailable or unsupported data.
	graphJSON := writeRuntimeGraph(t, t.TempDir(), &types.Graph{
		Nodes: []types.Node{
			{ID: "schema-checkout", Label: "CheckoutSchema", NodeType: "schema", SourceFile: "schema.go"},
		},
	})

	engine, err := LoadFromFile(graphJSON)
	if err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}
	result := engine.ExplainResult("CheckoutSchema")

	if result.Status == ResultStatusOK {
		t.Fatalf("missing TestUsage category must not look like a complete graph-backed answer, got %+v", result)
	}
	assertDiagnostic(t, result.Diagnostics, "TEST_USAGE_NOT_INDEXED", "unknown")
	for _, diagnostic := range result.Diagnostics {
		if strings.Contains(strings.ToLower(diagnostic.Message), "no tests exist") {
			t.Fatalf("diagnostic must not make unsupported negative test-existence claim: %+v", diagnostic)
		}
	}
}

func writeSCN016MixedRuntimeGraph(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	velaDir := filepath.Join(dir, ".vela")
	if err := os.Mkdir(velaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	graphJSON := filepath.Join(velaDir, "graph.json")
	if err := os.WriteFile(graphJSON, []byte(`{"nodes":[],"edges":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	graph := &types.Graph{
		Nodes: []types.Node{
			{ID: "legacy-handler", Label: "LegacyHandler", NodeType: "handler", SourceFile: "legacy_handler.go"},
			{ID: "ir-job", Label: "IRJob", NodeType: "job", SourceFile: "ir_job.go"},
			{ID: "checkout-service", Label: "CheckoutService", NodeType: "service", SourceFile: "checkout.go"},
			{ID: "legacy-gateway", Label: "LegacyGateway", NodeType: "client", SourceFile: "legacy_gateway.go"},
			{ID: "ir-repository", Label: "IRRepository", NodeType: "repository", SourceFile: "ir_repository.go"},
		},
		Edges: []types.Edge{
			{Source: "checkout-service", Target: "legacy-gateway", Relation: string(types.FactKindDependsOn), Metadata: legacySCN016Metadata("legacy-service-gateway")},
			{Source: "checkout-service", Target: "ir-repository", Relation: string(types.FactKindDependsOn), Metadata: irSCN016Metadata("DEPENDS_ON")},
			{Source: "legacy-handler", Target: "checkout-service", Relation: "calls", Metadata: legacySCN016Metadata("legacy-handler-service")},
			{Source: "ir-job", Target: "checkout-service", Relation: "calls", Metadata: irSCN016Metadata("CALLS")},
		},
	}
	if err := graphExport.WriteSQLiteGraphAtomic(graph, velaDir); err != nil {
		t.Fatalf("WriteSQLiteGraphAtomic error: %v", err)
	}
	return graphJSON
}

func writeRuntimeGraph(t *testing.T, dir string, graph *types.Graph) string {
	t.Helper()
	velaDir := filepath.Join(dir, ".vela")
	if err := os.MkdirAll(velaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	graphJSON := filepath.Join(velaDir, "graph.json")
	if err := os.WriteFile(graphJSON, []byte(`{"nodes":[],"edges":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := graphExport.WriteSQLiteGraphAtomic(graph, velaDir); err != nil {
		t.Fatalf("WriteSQLiteGraphAtomic error: %v", err)
	}
	return graphJSON
}

func writeRuntimeManifest(t *testing.T, velaDir, repoRoot, path, recordedHash string) {
	t.Helper()
	manifest := types.Manifest{
		Version:     1,
		RepoRoot:    repoRoot,
		GeneratedAt: time.Now().UTC(),
		Files: []types.ManifestFile{{
			Path:   path,
			SHA256: strings.TrimPrefix(recordedHash, "sha256:"),
		}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(velaDir, "manifest.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(filepath.Join(repoRoot, path))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(current)
	if hex.EncodeToString(sum[:]) == strings.TrimPrefix(recordedHash, "sha256:") {
		t.Fatalf("test fixture recorded hash unexpectedly matches current source")
	}
}

func commonIRHandlerEdge(source, target, origin, claimStatus, snippet string) types.Edge {
	return types.Edge{
		Source:   source,
		Target:   target,
		Relation: "handled_by",
		Metadata: map[string]interface{}{
			"common_ir":                true,
			"ir_kind":                  "HANDLED_BY",
			"ir_origin":                origin,
			"freshness":                "fresh",
			"evidence_confidence":      "high",
			"evidence_type":            "common-ir",
			"evidence_source_artifact": "routes.go",
			"evidence_snippet":         snippet,
			"interface_provider":       "http-router",
			"interface_kind":           "route",
			"interface_name":           "POST /checkout",
			"interface_route":          "/checkout",
			"interface_method":         "POST",
			"claim_status":             claimStatus,
		},
	}
}

func assertDiagnostic(t *testing.T, diagnostics []Diagnostic, code, messagePart string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code && strings.Contains(diagnostic.Message, messagePart) {
			return
		}
	}
	t.Fatalf("expected diagnostic %s containing %q, got %+v", code, messagePart, diagnostics)
}

func legacySCN016Metadata(stableID string) map[string]interface{} {
	return map[string]interface{}{
		"stable_id":                stableID,
		"evidence_type":            "legacy-runtime",
		"evidence_source_artifact": "legacy_runtime.go",
		"evidence_confidence":      "legacy",
	}
}

func irSCN016Metadata(kind string) map[string]interface{} {
	return map[string]interface{}{
		"common_ir":                true,
		"ir_kind":                  kind,
		"ir_origin":                "deterministic",
		"freshness":                "fresh",
		"evidence_type":            "common-ir",
		"evidence_source_artifact": "ir_runtime.go",
		"evidence_confidence":      "high",
	}
}

func assertContainsAll(t *testing.T, got string, wants []string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in output, got:\n%s", want, got)
		}
	}
}

func assertNoFullReplacementClaim(t *testing.T, got string) {
	t.Helper()
	for _, forbidden := range []string{"full replacement", "fully replaced", "completed full replacement", "Phase 1 replaced prior runtime"} {
		if strings.Contains(strings.ToLower(got), strings.ToLower(forbidden)) {
			t.Fatalf("output must not claim Phase 1 fully replaced prior runtime behavior via %q, got:\n%s", forbidden, got)
		}
	}
}

func TestRunRequest_PathPrefersTargetPackageBarrelChain(t *testing.T) {
	t.Parallel()

	graph := map[string]any{
		"nodes": []map[string]any{
			{"id": "app:employee-selection", "label": "apps/mobile/src/modules/auth/presentation/screens/EmployeeSelection/hook.ts", "kind": "file", "file": "apps/mobile/src/modules/auth/presentation/screens/EmployeeSelection/hook.ts"},
			{"id": "app:auth-context", "label": "apps/mobile/src/modules/auth/context/AuthContext.tsx", "kind": "file", "file": "apps/mobile/src/modules/auth/context/AuthContext.tsx"},
			{"id": "app:enrollment-service", "label": "apps/mobile/src/modules/auth/domain/EnrollmentService.ts", "kind": "file", "file": "apps/mobile/src/modules/auth/domain/EnrollmentService.ts"},
			{"id": "pkg:index", "label": "packages/api-client/src/index.ts", "kind": "file", "file": "packages/api-client/src/index.ts"},
			{"id": "pkg:hooks", "label": "packages/api-client/src/hooks/index.ts", "kind": "file", "file": "packages/api-client/src/hooks/index.ts"},
			{"id": "pkg:users-index", "label": "packages/api-client/src/hooks/users/index.ts", "kind": "file", "file": "packages/api-client/src/hooks/users/index.ts"},
			{"id": "pkg:users", "label": "packages/api-client/src/hooks/users/use-users.ts", "kind": "file", "file": "packages/api-client/src/hooks/users/use-users.ts"},
		},
		"edges": []map[string]any{
			{"from": "app:employee-selection", "to": "app:auth-context", "kind": "depends_on", "metadata": map[string]any{"projected_from": "static_import"}},
			{"from": "app:auth-context", "to": "app:enrollment-service", "kind": "depends_on", "metadata": map[string]any{"projected_from": "static_import"}},
			{"from": "app:enrollment-service", "to": "pkg:users", "kind": "depends_on", "metadata": map[string]any{"projected_from": "static_import"}},
			{"from": "app:employee-selection", "to": "pkg:index", "kind": "depends_on", "metadata": map[string]any{"projected_from": "workspace_package"}},
			{"from": "pkg:index", "to": "pkg:hooks", "kind": "depends_on", "metadata": map[string]any{"projected_from": "static_import"}},
			{"from": "pkg:hooks", "to": "pkg:users-index", "kind": "depends_on", "metadata": map[string]any{"projected_from": "static_import"}},
			{"from": "pkg:users-index", "to": "pkg:users", "kind": "depends_on", "metadata": map[string]any{"projected_from": "static_import"}},
		},
	}
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	engine, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}

	pathResult, err := engine.RunRequest(types.QueryRequest{Kind: types.QueryKindPath, Subject: "apps/mobile/src/modules/auth/presentation/screens/EmployeeSelection/hook.ts", Target: "packages/api-client/src/hooks/users/use-users.ts"})
	if err != nil {
		t.Fatalf("RunRequest(path) error = %v", err)
	}
	for _, want := range []string{
		"packages/api-client/src/index.ts",
		"packages/api-client/src/hooks/index.ts",
		"packages/api-client/src/hooks/users/index.ts",
	} {
		if !strings.Contains(pathResult, want) {
			t.Fatalf("expected %q in ranked path result, got %q", want, pathResult)
		}
	}
	for _, unwanted := range []string{
		"apps/mobile/src/modules/auth/context/AuthContext.tsx",
		"apps/mobile/src/modules/auth/domain/EnrollmentService.ts",
	} {
		if strings.Contains(pathResult, unwanted) {
			t.Fatalf("did not expect %q in ranked path result, got %q", unwanted, pathResult)
		}
	}
}

func TestRunRequest_PathPrefersExplanatoryIntermediateOverDirectJump(t *testing.T) {
	t.Parallel()

	graph := map[string]any{
		"nodes": []map[string]any{
			{"id": "main", "label": "cmd/vela/main.go", "kind": "file", "file": "cmd/vela/main.go"},
			{"id": "config", "label": "internal/config/config.go", "kind": "file", "file": "internal/config/config.go"},
			{"id": "types", "label": "pkg/types/types.go", "kind": "file", "file": "pkg/types/types.go"},
		},
		"edges": []map[string]any{
			{"from": "main", "to": "types", "kind": "depends_on", "metadata": map[string]any{"projected_from": "static_import"}},
			{"from": "main", "to": "config", "kind": "depends_on", "metadata": map[string]any{"projected_from": "static_import"}},
			{"from": "config", "to": "types", "kind": "depends_on", "metadata": map[string]any{"projected_from": "static_import"}},
		},
	}
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	engine, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}

	pathResult, err := engine.RunRequest(types.QueryRequest{Kind: types.QueryKindPath, Subject: "cmd/vela/main.go", Target: "pkg/types/types.go"})
	if err != nil {
		t.Fatalf("RunRequest(path) error = %v", err)
	}
	if !strings.Contains(pathResult, "internal/config/config.go") {
		t.Fatalf("expected config intermediary in ranked path result, got %q", pathResult)
	}
}

func TestRunRequest_ReverseDependenciesPreferExternalPackageBarrelCallers(t *testing.T) {
	t.Parallel()

	graph := map[string]any{
		"nodes": []map[string]any{
			{"id": "pkg:index", "label": "packages/api-client/src/index.ts", "kind": "file", "file": "packages/api-client/src/index.ts"},
			{"id": "pkg:hooks", "label": "packages/api-client/src/hooks/index.ts", "kind": "file", "file": "packages/api-client/src/hooks/index.ts"},
			{"id": "app:employee-selection", "label": "apps/mobile/src/modules/auth/presentation/screens/EmployeeSelection/hook.ts", "kind": "file", "file": "apps/mobile/src/modules/auth/presentation/screens/EmployeeSelection/hook.ts"},
			{"id": "app:auth-context", "label": "apps/mobile/src/modules/auth/presentation/context/AuthContext.tsx", "kind": "file", "file": "apps/mobile/src/modules/auth/presentation/context/AuthContext.tsx"},
		},
		"edges": []map[string]any{
			{"from": "pkg:hooks", "to": "pkg:index", "kind": "depends_on", "metadata": map[string]any{"projected_from": "static_import"}},
			{"from": "app:employee-selection", "to": "pkg:index", "kind": "depends_on", "metadata": map[string]any{"projected_from": "static_import"}},
			{"from": "app:auth-context", "to": "pkg:index", "kind": "depends_on", "metadata": map[string]any{"projected_from": "static_import"}},
		},
	}
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	engine, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}

	reverseResult, err := engine.RunRequest(types.QueryRequest{Kind: types.QueryKindReverseDependencies, Subject: "packages/api-client/src/index.ts", Limit: 5})
	if err != nil {
		t.Fatalf("RunRequest(reverse) error = %v", err)
	}
	for _, want := range []string{
		"apps/mobile/src/modules/auth/presentation/screens/EmployeeSelection/hook.ts",
		"apps/mobile/src/modules/auth/presentation/context/AuthContext.tsx",
	} {
		if !strings.Contains(reverseResult, want) {
			t.Fatalf("expected %q in reverse dependency result, got %q", want, reverseResult)
		}
	}
	if strings.Contains(reverseResult, "packages/api-client/src/hooks/index.ts") {
		t.Fatalf("did not expect internal package barrel caller in result, got %q", reverseResult)
	}
}

func loadRequestTestEngine(t *testing.T) *Engine {
	t.Helper()
	graph := map[string]any{
		"nodes": []map[string]any{
			{"id": "api", "label": "APIHandler", "kind": "handler", "file": "api.go"},
			{"id": "auth", "label": "AuthService", "kind": "struct", "file": "auth.go"},
			{"id": "db", "label": "Database", "kind": "struct", "file": "db.go"},
			{"id": "user", "label": "UserRepo", "kind": "struct", "file": "user.go"},
		},
		"edges": []map[string]any{
			{"from": "api", "to": "auth", "kind": "calls"},
			{"from": "auth", "to": "db", "kind": "depends_on"},
			{"from": "auth", "to": "user", "kind": "depends_on"},
			{"from": "user", "to": "db", "kind": "depends_on"},
		},
	}
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	engine, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}
	return engine
}
