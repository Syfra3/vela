package query

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	graphExport "github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/pkg/types"
)

func writeTestGraph(t *testing.T, dir string) string {
	t.Helper()
	g := map[string]interface{}{
		"nodes": []map[string]interface{}{
			{"id": "auth", "label": "AuthService", "kind": "struct", "file": "auth.go"},
			{"id": "db", "label": "Database", "kind": "struct", "file": "db.go"},
			{"id": "user", "label": "UserRepo", "kind": "struct", "file": "user.go"},
			{"id": "workspace:repo:auth-api", "label": "auth-api", "kind": "repo", "file": "workspace:repo:auth-api", "metadata": map[string]interface{}{"layer": "workspace", "evidence_type": "routing", "evidence_confidence": "extracted"}},
			{"id": "workspace:service:auth", "label": "auth", "kind": "service", "file": "workspace:service:auth", "metadata": map[string]interface{}{"layer": "workspace", "evidence_type": "routing", "evidence_confidence": "extracted"}},
			{"id": "memory:observation:7", "label": "Auth note", "kind": "observation", "file": "ancora:obs:7"},
			{"id": "memory:observation:8", "label": "Config note", "kind": "observation", "file": "ancora:obs:8"},
		},
		"edges": []map[string]interface{}{
			{"from": "workspace:repo:auth-api", "to": "workspace:service:auth", "kind": "exposes", "metadata": map[string]interface{}{"layer": "workspace", "evidence_type": "routing", "evidence_confidence": "extracted"}},
			{"from": "auth", "to": "db", "kind": "uses", "confidence": "DECLARED", "metadata": map[string]interface{}{"evidence_type": "openapi", "evidence_confidence": "declared", "evidence_source_artifact": "openapi.yaml"}},
			{"from": "auth", "to": "user", "kind": "uses"},
			{"from": "user", "to": "db", "kind": "uses"},
			{"from": "memory:observation:7", "to": "auth", "kind": "documents", "metadata": map[string]interface{}{"layer": "memory", "evidence_type": "observation-reference", "evidence_confidence": "declared", "verification": "redirected", "reference_target": "repo:file:internal/legacy/auth.go", "bound_target": "auth", "binding_state": "redirected", "binding_evidence": "unique basename match"}},
			{"from": "memory:observation:8", "to": "repo:file:legacy/config.go", "kind": "constrains", "metadata": map[string]interface{}{"layer": "memory", "evidence_type": "observation-reference", "evidence_confidence": "declared", "verification": "ambiguous", "reference_target": "repo:file:legacy/config.go", "binding_state": "ambiguous", "binding_evidence": "multiple live files share the historical basename", "binding_suggestions": []string{"auth", "db"}}},
		},
		"meta": map[string]interface{}{"nodeCount": 7, "edgeCount": 6},
	}
	data, _ := json.MarshalIndent(g, "", "  ")
	path := filepath.Join(dir, "graph.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)

	eng, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}
	if len(eng.graph.Nodes) != 7 {
		t.Errorf("expected 7 nodes, got %d", len(eng.graph.Nodes))
	}
	if len(eng.graph.Edges) != 6 {
		t.Errorf("expected 6 edges, got %d", len(eng.graph.Edges))
	}
	declaredFound := false
	for _, edge := range eng.graph.Edges {
		if edge.Confidence != "DECLARED" {
			continue
		}
		if got, _ := edge.Metadata["evidence_type"].(string); got == "openapi" {
			declaredFound = true
			break
		}
	}
	if !declaredFound {
		t.Fatal("expected declared openapi edge in graph")
	}
}

// REQ-002 → SCN-003 → TestSCN003_SQLiteGraphDatabaseRequiredForRuntimeQueries
func TestSCN003_SQLiteGraphDatabaseRequiredForRuntimeQueries(t *testing.T) {
	// Scenario: SQLite graph database is required for runtime queries.
	dir := t.TempDir()
	velaDir := filepath.Join(dir, ".vela")
	if err := os.Mkdir(velaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := writeTestGraph(t, velaDir)

	_, err := LoadFromFile(path)
	if err == nil {
		t.Fatal("LoadFromFile() error = nil, want runtime graph unavailable")
	}
	message := err.Error()
	for _, want := range []string{"runtime graph unavailable", "graph.db", "graph.json", "vela build", "vela update"} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected %q in error %q", want, message)
		}
	}
}

// REQ-002 → SCN-003 → TestSCN003_SQLiteRuntimeTruthBeatsDisagreeingJSON
func TestSCN003_SQLiteRuntimeTruthBeatsDisagreeingJSON(t *testing.T) {
	// Scenario: SQLite graph database is required for runtime queries.
	dir := t.TempDir()
	velaDir := filepath.Join(dir, ".vela")
	if err := os.Mkdir(velaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	graphJSON := writeTestGraph(t, velaDir)

	if err := graphExport.WriteSQLiteGraphAtomic(&types.Graph{
		Nodes: []types.Node{
			{ID: "auth", Label: "AuthService", NodeType: "struct", SourceFile: "auth.go"},
			{ID: "sqlite-db", Label: "SQLiteDatabase", NodeType: "struct", SourceFile: "sqlite_db.go"},
		},
		Edges: []types.Edge{
			{Source: "auth", Target: "sqlite-db", Relation: "uses", SourceFile: "auth.go"},
		},
	}, velaDir); err != nil {
		t.Fatalf("WriteSQLiteGraphAtomic error: %v", err)
	}

	eng, err := LoadFromFile(graphJSON)
	if err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}
	result := eng.ExplainResult("AuthService")

	if len(result.Facts) != 1 {
		t.Fatalf("ExplainResult facts len = %d, want 1 SQLite-backed fact", len(result.Facts))
	}
	fact := result.Facts[0]
	if fact.Object != "sqlite-db" {
		t.Fatalf("ExplainResult fact object = %q, want SQLite object sqlite-db", fact.Object)
	}
	if fact.Object == "db" {
		t.Fatal("ExplainResult used graph.json truth; want SQLite graph.db truth")
	}
}

// REQ-001 → SCN-001 → TestSCN001_ImportantExplainAnswersIncludeProofMetadata
func TestSCN001_ImportantExplainAnswersIncludeProofMetadata(t *testing.T) {
	// Scenario: Important answers include proof metadata when evidence is available.
	eng := newEngine(&types.Graph{
		Nodes: []types.Node{
			{ID: "auth", Label: "AuthService", NodeType: "struct", SourceFile: "auth.go"},
			{ID: "db", Label: "Database", NodeType: "struct", SourceFile: "db.go"},
		},
		Edges: []types.Edge{
			{
				Source:     "auth",
				Target:     "db",
				Relation:   "uses",
				SourceFile: "auth.go",
				Metadata: map[string]interface{}{
					"layer":                    "repo",
					"evidence_type":            "static-analysis",
					"evidence_source_artifact": "auth.go",
					"evidence_confidence":      "extracted",
				},
			},
		},
		Metadata: map[string]interface{}{"freshness_status": "fresh"},
	})

	result := eng.ExplainResult("AuthService")

	if result.Status != ResultStatusOK {
		t.Fatalf("ExplainResult status = %q, want %q", result.Status, ResultStatusOK)
	}
	if len(result.Facts) != 1 {
		t.Fatalf("ExplainResult facts len = %d, want 1", len(result.Facts))
	}
	fact := result.Facts[0]
	if fact.Subject != "auth" || fact.Predicate != "uses" || fact.Object != "db" {
		t.Fatalf("unexpected graph fact: %#v", fact)
	}
	if len(fact.Evidence) != 1 {
		t.Fatalf("fact evidence len = %d, want 1", len(fact.Evidence))
	}
	proof := fact.Evidence[0]
	if proof.Type != "static-analysis" || proof.SourceArtifact != "auth.go" || proof.Confidence != types.ConfidenceExtracted || proof.Layer != types.LayerRepo {
		t.Fatalf("unexpected proof metadata: %#v", proof)
	}
	if fact.Confidence != types.ConfidenceExtracted {
		t.Fatalf("fact confidence = %q, want %q", fact.Confidence, types.ConfidenceExtracted)
	}
	if result.Freshness.Status != FreshnessFresh {
		t.Fatalf("freshness status = %q, want %q", result.Freshness.Status, FreshnessFresh)
	}
}

// REQ-001/REQ-015 → SCN-002 → TestSCN002_UnsupportedExplainClaimReturnsUnresolvedDiagnostic
func TestSCN002_UnsupportedExplainClaimReturnsUnresolvedDiagnostic(t *testing.T) {
	// Scenario: Vela refuses to invent an answer when no graph-backed fact exists.
	eng := newEngine(&types.Graph{
		Nodes: []types.Node{
			{ID: "auth", Label: "AuthService", NodeType: "struct", SourceFile: "auth.go"},
		},
		Metadata: map[string]interface{}{"freshness_status": "fresh"},
	})

	result := eng.ExplainResult("AuthService")

	if result.Status != ResultStatusUnresolved {
		t.Fatalf("ExplainResult status = %q, want %q", result.Status, ResultStatusUnresolved)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("ExplainResult facts len = %d, want 0 unsupported facts", len(result.Facts))
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("ExplainResult diagnostics len = %d, want 1", len(result.Diagnostics))
	}
	diagnostic := result.Diagnostics[0]
	if diagnostic.Code != "NO_GRAPH_BACKED_ANSWER" || !strings.Contains(diagnostic.Message, "no graph-backed answer") {
		t.Fatalf("unexpected diagnostic: %#v", diagnostic)
	}
}

// REQ-010/REQ-015 → SCN-013 → TestSCN013_HigherConfidenceInterfaceEvidenceOutranksConflict
func TestSCN013_HigherConfidenceInterfaceEvidenceOutranksConflict(t *testing.T) {
	// Scenario: Higher-confidence evidence outranks conflicting lower-confidence evidence without hiding conflict.
	eng := newEngine(&types.Graph{
		Nodes: []types.Node{
			{ID: "client", Label: "CheckoutClient", NodeType: "service", SourceFile: "client.go"},
			{ID: "declared-api", Label: "OrdersAPI", NodeType: "service", SourceFile: "openapi.yaml"},
			{ID: "inferred-api", Label: "OrderServiceGuess", NodeType: "service", SourceFile: "client.go"},
		},
		Edges: []types.Edge{
			{
				Source:     "client",
				Target:     "inferred-api",
				Relation:   "calls",
				SourceFile: "client.go",
				Metadata: map[string]interface{}{
					"interface_name":           "orders-http",
					"interface_route":          "/orders",
					"interface_method":         "GET",
					"interface_provider":       "HttpClientProvider",
					"claim_status":             "inferred",
					"layer":                    "repo",
					"evidence_type":            "http-client",
					"evidence_source_artifact": "client.go",
					"evidence_confidence":      "inferred",
				},
			},
			{
				Source:     "client",
				Target:     "declared-api",
				Relation:   "calls",
				SourceFile: "openapi.yaml",
				Metadata: map[string]interface{}{
					"interface_name":           "orders-http",
					"interface_route":          "/orders",
					"interface_method":         "GET",
					"interface_provider":       "OpenAPIProvider",
					"claim_status":             "declared",
					"layer":                    "contract",
					"evidence_type":            "openapi",
					"evidence_source_artifact": "openapi.yaml",
					"evidence_confidence":      "declared",
				},
			},
		},
		Metadata: map[string]interface{}{"freshness_status": "fresh"},
	})

	result := eng.ExplainResult("CheckoutClient")

	if result.Status != ResultStatusOK {
		t.Fatalf("ExplainResult status = %q, want %q", result.Status, ResultStatusOK)
	}
	if len(result.Facts) != 2 {
		t.Fatalf("ExplainResult facts len = %d, want 2 preserved facts", len(result.Facts))
	}
	primary := result.Facts[0]
	if primary.Object != "declared-api" || primary.Confidence != types.ConfidenceDeclared {
		t.Fatalf("primary fact = %#v, want declared fact to outrank inferred conflict", primary)
	}
	if got := primary.Metadata["claim_status"]; got != "declared" {
		t.Fatalf("primary claim_status = %v, want declared", got)
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics len = %d, want 1 conflict diagnostic", len(result.Diagnostics))
	}
	diagnostic := result.Diagnostics[0]
	if diagnostic.Code != "EVIDENCE_CONFLICT" || !strings.Contains(diagnostic.Message, "inferred") || !strings.Contains(diagnostic.Message, "declared") {
		t.Fatalf("unexpected conflict diagnostic: %#v", diagnostic)
	}
}

// REQ-015 → SCN-026 → TestSCN026_MultipleDiagnosticsPreservedInDegradedExploreResult
func TestSCN026_MultipleDiagnosticsPreservedInDegradedExploreResult(t *testing.T) {
	// Scenario: Multiple diagnostics are preserved in degraded results.
	eng := newEngine(&types.Graph{
		Nodes: []types.Node{
			{ID: "auth-service", Label: "auth", NodeType: "service", SourceFile: "services/auth/main.go"},
			{ID: "auth-client", Label: "auth", NodeType: "client", SourceFile: "clients/auth/client.go"},
			{ID: "login-handler", Label: "LoginHandler", NodeType: "function", SourceFile: "services/auth/main.go"},
		},
		Edges: []types.Edge{
			{
				Source:     "auth-service",
				Target:     "login-handler",
				Relation:   "defines",
				SourceFile: "services/auth/main.go",
				Metadata: map[string]interface{}{
					"layer":                    "repo",
					"evidence_type":            "static-analysis",
					"evidence_source_artifact": "services/auth/main.go",
					"evidence_confidence":      "extracted",
				},
			},
		},
		Metadata: map[string]interface{}{
			"freshness_status":    "stale",
			"stale_files":         []string{"services/auth/main.go"},
			"recommended_actions": []string{"vela update", "vela build"},
		},
	})

	result := eng.ExploreResult("auth", 5)

	if result.Status != ResultStatusAmbiguous {
		t.Fatalf("ExploreResult status = %q, want %q", result.Status, ResultStatusAmbiguous)
	}
	if !queryDiagnosticContains(result.Diagnostics, "STALE_GRAPH", "services/auth/main.go") {
		t.Fatalf("diagnostics = %+v, want stale graph diagnostic naming stale file", result.Diagnostics)
	}
	if !queryDiagnosticContains(result.Diagnostics, "AMBIGUOUS_SUBJECT", "auth") {
		t.Fatalf("diagnostics = %+v, want ambiguity diagnostic naming query", result.Diagnostics)
	}
	if len(result.Facts) != 1 {
		t.Fatalf("ExploreResult facts len = %d, want available lower-confidence fact preserved", len(result.Facts))
	}
	fact := result.Facts[0]
	if fact.Confidence != types.ConfidenceExtracted {
		t.Fatalf("fact confidence = %q, want %q", fact.Confidence, types.ConfidenceExtracted)
	}
	if result.Freshness.Status != FreshnessStale {
		t.Fatalf("freshness status = %q, want stale", result.Freshness.Status)
	}
}

func queryDiagnosticContains(diagnostics []Diagnostic, code, text string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code && strings.Contains(diagnostic.Message, text) {
			return true
		}
	}
	return false
}

func TestExplain_ResolvesCanonicalIDAndShowsEvidence(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	result := eng.Explain("repo:struct:auth")
	if !strings.Contains(result, "AuthService [repo/struct]") {
		t.Fatalf("expected layer-aware node description in explain output, got: %q", result)
	}
	if !strings.Contains(result, "type=openapi") {
		t.Fatalf("expected evidence type in explain output, got: %q", result)
	}
	if !strings.Contains(result, "confidence=declared") {
		t.Fatalf("expected evidence confidence in explain output, got: %q", result)
	}
}

func TestExplain_ShowsBindingMetadata(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	result := eng.Explain("Auth note")
	if !strings.Contains(result, "Auth note [memory/observation]") {
		t.Fatalf("expected node layer/type label in explain output, got: %q", result)
	}
	if !strings.Contains(result, "reference=repo:file:internal/legacy/auth.go") {
		t.Fatalf("expected original reference target in explain output, got: %q", result)
	}
	if !strings.Contains(result, "binding=unique basename match") {
		t.Fatalf("expected binder evidence in explain output, got: %q", result)
	}
}

func TestExplain_ResolvesIncomingLabelTargetsAgainstLayer(t *testing.T) {
	dir := t.TempDir()
	g := map[string]interface{}{
		"nodes": []map[string]interface{}{
			{"id": "workspace:repo:billing-api", "label": "billing-api", "kind": "repo", "file": "workspace:repo:billing-api", "metadata": map[string]interface{}{"layer": "workspace", "evidence_type": "routing", "evidence_confidence": "extracted"}},
			{"id": "workspace:service:billing", "label": "billing", "kind": "service", "file": "workspace:service:billing", "metadata": map[string]interface{}{"layer": "workspace", "evidence_type": "routing", "evidence_confidence": "extracted"}},
			{"id": "contract:service:billing", "label": "billing", "kind": "service", "file": "openapi.yaml", "metadata": map[string]interface{}{"layer": "contract", "evidence_type": "openapi", "evidence_confidence": "declared"}},
		},
		"edges": []map[string]interface{}{
			{"from": "workspace:repo:billing-api", "to": "billing", "kind": "exposes", "metadata": map[string]interface{}{"layer": "workspace", "evidence_type": "routing", "evidence_confidence": "extracted"}},
		},
	}
	data, _ := json.MarshalIndent(g, "", "  ")
	path := filepath.Join(dir, "graph.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	eng, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}

	result := eng.Explain("workspace:service:billing")
	if !strings.Contains(result, "billing-api [workspace/repo] --[exposes]--> billing [workspace/service]") {
		t.Fatalf("expected workspace-layer target resolution, got: %q", result)
	}
	if strings.Contains(result, "contract/service") {
		t.Fatalf("expected explain to avoid resolving workspace edge to contract node, got: %q", result)
	}
}

func TestBindings_ReturnsBinderState(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	result := eng.Bindings("Config note")
	if !strings.Contains(result, "[ambiguous]") {
		t.Fatalf("expected ambiguous state in bindings output, got: %q", result)
	}
	if !strings.Contains(result, "suggestions=auth,db") {
		t.Fatalf("expected suggestions in bindings output, got: %q", result)
	}
}

func TestRoute_ReturnsWorkspaceRouting(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	result := eng.Route("auth")
	if !strings.Contains(result, "score=") {
		t.Fatalf("expected scored route output, got: %q", result)
	}
}

func TestPath_DirectEdge(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	result := eng.Path("AuthService", "Database")
	if !strings.Contains(result, "AuthService") {
		t.Errorf("expected path containing AuthService, got: %q", result)
	}
	if !strings.Contains(result, "Database") {
		t.Errorf("expected path containing Database, got: %q", result)
	}
	if !strings.Contains(result, "[repo/struct]") {
		t.Errorf("expected layer-aware path output, got: %q", result)
	}
}

// REQ-012/REQ-010 → SCN-017 → TestSCN017_CrossRepoPathIncludesInferredInterfaceBridgeEvidence
func TestSCN017_CrossRepoPathIncludesInferredInterfaceBridgeEvidence(t *testing.T) {
	// Scenario: Cross-repo path includes confidence for interface bridge evidence.
	eng := newEngine(&types.Graph{
		Nodes: []types.Node{
			{ID: "repo:billing-ui:symbol:CheckoutPage", Label: "CheckoutPage", NodeType: "symbol", SourceFile: "apps/billing-ui/checkout.ts"},
			{ID: "interface:orders-http", Label: "Orders HTTP", NodeType: "interface", SourceFile: ".vela/workspace.yaml", Metadata: map[string]interface{}{"layer": "workspace"}},
			{ID: "repo:orders-api:symbol:CreateOrderHandler", Label: "CreateOrderHandler", NodeType: "symbol", SourceFile: "services/orders-api/handler.go"},
		},
		Edges: []types.Edge{
			{
				Source:   "repo:billing-ui:symbol:CheckoutPage",
				Target:   "interface:orders-http",
				Relation: "calls",
				Metadata: map[string]interface{}{
					"interface_name":             "orders-http",
					"interface_provider":         "HttpClientProvider",
					"claim_status":               "inferred",
					"layer":                      "workspace",
					"evidence_type":              "interface-bridge",
					"evidence_source_artifact":   "apps/billing-ui/checkout.ts",
					"evidence_confidence":        "inferred",
					"bridge_evidence_confidence": "inferred",
				},
			},
			{
				Source:   "interface:orders-http",
				Target:   "repo:orders-api:symbol:CreateOrderHandler",
				Relation: "routes_to",
				Metadata: map[string]interface{}{
					"interface_name":             "orders-http",
					"interface_provider":         "WorkspaceHintsProvider",
					"claim_status":               "inferred",
					"layer":                      "workspace",
					"evidence_type":              "interface-bridge",
					"evidence_source_artifact":   ".vela/workspace.yaml",
					"evidence_confidence":        "inferred",
					"bridge_evidence_confidence": "inferred",
				},
			},
		},
	})

	result := eng.Path("CheckoutPage", "CreateOrderHandler")

	for _, want := range []string{"CheckoutPage", "Orders HTTP", "CreateOrderHandler", "confidence=inferred", "interface bridge=inferred", "not a declared contract path"} {
		if !strings.Contains(result, want) {
			t.Fatalf("expected %q in cross-repo path output, got: %q", want, result)
		}
	}
	if strings.Contains(result, "declared contract path") && !strings.Contains(result, "not a declared contract path") {
		t.Fatalf("path was presented as declared contract truth: %q", result)
	}
}

func TestPath_NoPath(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	// Database has no outgoing edges → no path to AuthService
	result := eng.Path("Database", "AuthService")
	if !strings.Contains(result, "no path") {
		t.Errorf("expected 'no path' message, got: %q", result)
	}
}

func TestPath_NodeNotFound(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	result := eng.Path("NonExistent", "Database")
	if !strings.Contains(result, "no path found") {
		t.Errorf("expected degraded no-path message, got: %q", result)
	}
	if !strings.Contains(result, "reason:") {
		t.Errorf("expected degraded reason in message, got: %q", result)
	}
	if !strings.Contains(result, "provenance: degraded graph lookup") {
		t.Errorf("expected degraded provenance in message, got: %q", result)
	}
}

func TestExplain(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	result := eng.Explain("AuthService")
	if !strings.Contains(result, "AuthService") {
		t.Errorf("expected AuthService in explain result, got: %q", result)
	}
	// Should list at least the two outgoing edges
	if !strings.Contains(result, "uses") {
		t.Errorf("expected 'uses' relation in explain result, got: %q", result)
	}
}

func TestExplain_NotFound(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	result := eng.Explain("Ghost")
	if !strings.Contains(result, "not found") {
		t.Errorf("expected 'not found', got: %q", result)
	}
}

func TestQuery_Dispatcher(t *testing.T) {
	dir := t.TempDir()
	gpath := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(gpath)

	cases := []struct {
		input  string
		wantIn string
	}{
		{"nodes", "7"},
		{"edges", "6"},
		{"help", "path"},
		{"path AuthService Database", "→"},
		{"explain AuthService", "AuthService"},
		{"bindings Config note", "ambiguous"},
		{"route auth", "score="},
		{"lookup auth", "Candidates for \"auth\""},
		{"unknown cmd", "unknown command"},
	}

	for _, tc := range cases {
		result := eng.Query(tc.input)
		if !strings.Contains(result, tc.wantIn) {
			t.Errorf("query(%q): expected %q in result, got: %q", tc.input, tc.wantIn, result)
		}
	}
}

func TestFindNode_FuzzyLabel(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	node, ok := eng.FindNode("auth")
	if !ok {
		t.Fatal("expected fuzzy node match")
	}
	if node.Label != "AuthService" {
		t.Fatalf("expected AuthService, got %q", node.Label)
	}
}

func TestLookupReturnsRankedCandidates(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	results := eng.Lookup("auth", 3)
	if len(results) == 0 {
		t.Fatal("expected lookup candidates")
	}
	if results[0].Node.Label != "AuthService" {
		t.Fatalf("top candidate = %q, want AuthService", results[0].Node.Label)
	}
	if results[0].Score <= 0 {
		t.Fatalf("top candidate score = %d, want > 0", results[0].Score)
	}
}

func TestRenderLookupSuggestsNextSteps(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	result := eng.RenderLookup("auth", 3)
	for _, want := range []string{"Candidates for \"auth\":", "1. AuthService", "Next steps:", "vela search \"explain AuthService\""} {
		if !strings.Contains(result, want) {
			t.Fatalf("expected %q in result, got %q", want, result)
		}
	}
}

func TestNeighbors(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	neighbors, err := eng.Neighbors("AuthService")
	if err != nil {
		t.Fatalf("Neighbors error: %v", err)
	}
	if len(neighbors) != 3 {
		t.Fatalf("expected 3 neighbors, got %d", len(neighbors))
	}
}

func TestStats(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	stats := eng.Stats()
	if stats.NodeCount != 7 {
		t.Fatalf("expected 7 nodes, got %d", stats.NodeCount)
	}
	if stats.EdgeCount != 6 {
		t.Fatalf("expected 6 edges, got %d", stats.EdgeCount)
	}
	if stats.NodeTypes["struct"] != 3 {
		t.Fatalf("expected 3 struct nodes, got %d", stats.NodeTypes["struct"])
	}
}

// REQ-compact-rank → SCN-001 → TestSCN001_RankGraphNodesWithinModulePathScopeCompactly
func TestSCN001_RankGraphNodesWithinModulePathScopeCompactly(t *testing.T) {
	// Scenario: rank graph nodes within module path scope compactly.
	eng := newRankFixtureEngine()

	result := eng.RankResult("apps/server-api/src/modules", "total_degree", 10, 3)

	if result.Status != ResultStatusOK {
		t.Fatalf("RankResult status = %q, want ok: %+v", result.Status, result.Diagnostics)
	}
	if len(result.Rankings) == 0 || result.Rankings[0].Subject.ID != "inventory-module" {
		t.Fatalf("top rank = %+v, want inventory-module first", result.Rankings)
	}
	metrics := result.Rankings[0].Metrics
	if metrics.InDegree != 2 || metrics.OutDegree != 2 || metrics.TotalDegree != 4 || metrics.DownstreamCount != 2 {
		t.Fatalf("metrics = %+v, want split degree/downstream counts", metrics)
	}
	if len(result.Rankings[0].Examples) > 3 {
		t.Fatalf("examples len = %d, want bounded <= 3", len(result.Rankings[0].Examples))
	}
	if got := result.Rankings[0].OptionalMetrics["cross_package_consumers"]; got != "unavailable" {
		t.Fatalf("optional cross_package_consumers = %q, want explicit unavailable", got)
	}
	if strings.Contains(strings.ToLower(result.Answer), "score") {
		t.Fatalf("rank answer should not collapse metrics into ambiguous score: %q", result.Answer)
	}
}

// REQ-compact-rank → SCN-001 → TestSCN001_GlobRankScopeExcludesNonMatchingFiles
func TestSCN001_GlobRankScopeExcludesNonMatchingFiles(t *testing.T) {
	// Scenario: glob-like rank scopes include only exact glob matches.
	eng := newEngine(&types.Graph{
		Nodes: []types.Node{
			{ID: "query-test", Label: "QueryTest", NodeType: "file", SourceFile: "internal/query/query_test.go"},
			{ID: "rank-test", Label: "RankTest", NodeType: "file", SourceFile: "internal/query/rank_test.go"},
			{ID: "query", Label: "Query", NodeType: "file", SourceFile: "internal/query/query.go"},
			{ID: "result", Label: "Result", NodeType: "file", SourceFile: "internal/query/result.go"},
		},
		Metadata: map[string]interface{}{"freshness_status": "fresh"},
	})

	result := eng.RankResult("internal/query/*_test.go", "total_degree", 10, 0)

	if result.Status != ResultStatusOK {
		t.Fatalf("RankResult status = %q, want ok: %+v", result.Status, result.Diagnostics)
	}
	assertRankSubjects(t, result, []string{"query-test", "rank-test"})
}

// REQ-compact-rank → SCN-001 → TestSCN001_RecursiveModuleGlobExcludesServicesAndControllers
func TestSCN001_RecursiveModuleGlobExcludesServicesAndControllers(t *testing.T) {
	// Scenario: recursive module glob rank scopes include module files only.
	result := newRankFixtureEngine().RankResult("apps/server-api/src/modules/**/*.module.ts", "total_degree", 10, 0)

	if result.Status != ResultStatusOK {
		t.Fatalf("RankResult status = %q, want ok: %+v", result.Status, result.Diagnostics)
	}
	assertRankSubjects(t, result, []string{"inventory-module", "menu-a", "menu-b", "order-module", "supplier-module"})
}

// REQ-compact-rank → SCN-003 → TestSCN003_HotspotsDistinguishMetricsAndExplainAmbiguity
func TestSCN003_HotspotsDistinguishMetricsAndExplainAmbiguity(t *testing.T) {
	// Scenario: hotspot intent returns metric breakdown and ambiguity explanation.
	result := newRankFixtureEngine().HotspotsResult("highest impact", "apps/server-api/src/modules", 2, 1)

	if len(result.Rankings) > 2 {
		t.Fatalf("hotspot response len = %d, want bounded by limit", len(result.Rankings))
	}
	if !strings.Contains(result.Answer, "ambiguous") || !strings.Contains(result.Answer, "in_degree") || !strings.Contains(result.Answer, "downstream_count") {
		t.Fatalf("hotspot answer missing ambiguity/metric explanation: %q", result.Answer)
	}
	if result.Rankings[0].Metrics.TotalDegree == 0 || result.Rankings[0].OptionalMetrics["cross_app_consumers"] != "unavailable" {
		t.Fatalf("hotspot ranking missing metric breakdown/unavailable optional metrics: %+v", result.Rankings[0])
	}
}

// REQ-compact-summary → SCN-005 → TestSCN005_ModuleSummaryCountsExamplesConfidenceAndGaps
func TestSCN005_ModuleSummaryCountsExamplesConfidenceAndGaps(t *testing.T) {
	// Scenario: module summary counts/examples/confidence/gaps.
	result := newRankFixtureEngine().ModuleSummaryResult("inventory-module", 2)

	if result.Status != ResultStatusOK || result.Metrics == nil {
		t.Fatalf("summary result = %+v, want ok metrics", result)
	}
	if result.Metrics.InDegree != 2 || result.Metrics.OutDegree != 2 || len(result.Examples) > 2 {
		t.Fatalf("summary metrics/examples = %+v len=%d", result.Metrics, len(result.Examples))
	}
	if !strings.Contains(result.ConfidenceAndLimits, "confidence") || len(result.Gaps) == 0 {
		t.Fatalf("summary missing confidence/gaps: %+v", result)
	}
	if !strings.Contains(result.Gaps[0], "route/client extraction") {
		t.Fatalf("summary gaps should document route/client non-implementation: %+v", result.Gaps)
	}
}

// REQ-compact-summary → SCN-006 → TestSCN006_AmbiguousModuleSummaryTargetReturnsCandidates
func TestSCN006_AmbiguousModuleSummaryTargetReturnsCandidates(t *testing.T) {
	// Scenario: ambiguous summary target returns candidates.
	result := newRankFixtureEngine().ModuleSummaryResult("MenuModule", 5)

	if result.Status != ResultStatusAmbiguous || len(result.ResolvedSubjects) != 2 {
		t.Fatalf("summary ambiguity = %+v, want two candidates", result)
	}
	if !queryDiagnosticContains(result.Diagnostics, "AMBIGUOUS_SUBJECT", "multiple candidates") {
		t.Fatalf("diagnostics = %+v, want ambiguity diagnostic", result.Diagnostics)
	}
}

func newRankFixtureEngine() *Engine {
	return newEngine(&types.Graph{
		Nodes: []types.Node{
			{ID: "inventory-module", Label: "InventoryModule", NodeType: "module", SourceFile: "apps/server-api/src/modules/inventory/inventory.module.ts"},
			{ID: "order-module", Label: "OrderModule", NodeType: "module", SourceFile: "apps/server-api/src/modules/order/order.module.ts"},
			{ID: "supplier-module", Label: "SupplierModule", NodeType: "module", SourceFile: "apps/server-api/src/modules/supplier/supplier.module.ts"},
			{ID: "menu-a", Label: "MenuModule", NodeType: "module", SourceFile: "apps/server-api/src/modules/menu/menu.module.ts"},
			{ID: "menu-b", Label: "MenuModule", NodeType: "module", SourceFile: "apps/server-api/src/modules/menu-v2/menu.module.ts"},
			{ID: "recipe-service", Label: "RecipeService", NodeType: "service", SourceFile: "apps/server-api/src/modules/recipe/recipe.service.ts"},
			{ID: "recipe-controller", Label: "RecipeController", NodeType: "controller", SourceFile: "apps/server-api/src/modules/recipe/recipe.controller.ts"},
		},
		Edges: []types.Edge{
			{Source: "order-module", Target: "inventory-module", Relation: "imports"},
			{Source: "supplier-module", Target: "inventory-module", Relation: "imports"},
			{Source: "inventory-module", Target: "menu-a", Relation: "imports"},
			{Source: "inventory-module", Target: "recipe-service", Relation: "uses"},
			{Source: "menu-a", Target: "recipe-service", Relation: "uses"},
		},
		Metadata: map[string]interface{}{"freshness_status": "fresh"},
	})
}

func assertRankSubjects(t *testing.T, result Result, want []string) {
	t.Helper()
	got := make([]string, 0, len(result.Rankings))
	for _, ranking := range result.Rankings {
		got = append(got, ranking.Subject.ID)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("rank subjects = %v, want %v", got, want)
	}
}
