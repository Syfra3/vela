package extract

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Syfra3/vela/pkg/types"
)

// TestExtractAll_CodeStampsRepoLayerMetadata runs ExtractAll end-to-end over
// the Go code fixture and verifies that every emitted node and edge carries
// the repo-layer + evidence metadata that US-003 requires for graph insertion,
// chunking, retrieval, and evidence attribution.
func TestExtractAll_CodeStampsRepoLayerMetadata(t *testing.T) {
	root := fixtureDir()
	fixture := filepath.Join(root, "sample.go")

	src := &types.Source{
		Type: types.SourceTypeCodebase,
		Name: "unit-test-repo",
		Path: root,
	}

	nodes, edges, err := ExtractAll(root, []string{fixture}, nil, src)
	if err != nil {
		t.Fatalf("ExtractAll error: %v", err)
	}
	if len(nodes) == 0 || len(edges) == 0 {
		t.Fatalf("expected non-empty extraction, got %d nodes / %d edges", len(nodes), len(edges))
	}

	for _, n := range nodes {
		if n.Metadata == nil {
			t.Errorf("node %q missing metadata", n.ID)
			continue
		}
		if layer, _ := n.Metadata[MetaLayer].(string); layer != string(types.LayerRepo) {
			t.Errorf("node %q layer = %q, want %q", n.ID, layer, types.LayerRepo)
		}
		if ev, _ := n.Metadata[MetaEvidenceType].(string); ev == "" {
			t.Errorf("node %q missing evidence_type", n.ID)
		}
		if cf, _ := n.Metadata[MetaEvidenceConfidence].(string); cf == "" {
			t.Errorf("node %q missing evidence_confidence", n.ID)
		}
	}

	for _, e := range edges {
		if e.Metadata == nil {
			t.Errorf("edge %s→%s missing metadata", e.Source, e.Target)
			continue
		}
		if layer, _ := e.Metadata[MetaLayer].(string); layer != string(types.LayerRepo) {
			t.Errorf("edge %s→%s layer = %q, want %q", e.Source, e.Target, layer, types.LayerRepo)
		}
	}
}

// TestExtractAll_NoLocalGraphIdentity asserts the architectural invariant that
// extractors must not invent graph-level identity — every non-bare node ID
// returned by ExtractAll must be canonically namespaced with the project name.
func TestExtractAll_NoLocalGraphIdentity(t *testing.T) {
	root := fixtureDir()
	fixture := filepath.Join(root, "sample.go")

	src := &types.Source{
		Type: types.SourceTypeCodebase,
		Name: "identity-check",
		Path: root,
	}

	nodes, _, err := ExtractAll(root, []string{fixture}, nil, src)
	if err != nil {
		t.Fatalf("ExtractAll error: %v", err)
	}

	prefix := src.Name + ":"
	for _, n := range nodes {
		if n.ID == ProjectNodeID(src.Name) {
			continue // project root uses its own canonical form
		}
		if !strings.HasPrefix(n.ID, prefix) {
			t.Errorf("node %q is missing project prefix %q — extractor invented local identity", n.ID, prefix)
		}
	}
}

// TestExtractAll_EvidenceByExtension covers the extension-specific evidence
// mapping across the supported language/document mix: Go, Python, and
// TypeScript (the current AST-backed languages in the repo).
func TestExtractAll_EvidenceByExtension(t *testing.T) {
	root := fixtureDir()
	files := []string{
		filepath.Join(root, "sample.go"),
		filepath.Join(root, "sample.py"),
		filepath.Join(root, "sample.ts"),
	}

	src := &types.Source{
		Type: types.SourceTypeCodebase,
		Name: "multi-lang",
		Path: root,
	}

	nodes, _, err := ExtractAll(root, files, nil, src)
	if err != nil {
		t.Fatalf("ExtractAll error: %v", err)
	}

	sawAST := false
	for _, n := range nodes {
		if n.NodeType == string(types.NodeTypeProject) {
			if ev, _ := n.Metadata[MetaEvidenceType].(string); ev != EvidenceTypeProject {
				t.Errorf("project node evidence_type = %q, want %q", ev, EvidenceTypeProject)
			}
			continue
		}
		if n.NodeType == string(types.NodeTypeFile) {
			if ev, _ := n.Metadata[MetaEvidenceType].(string); ev != EvidenceTypeFilesystem {
				t.Errorf("file node %q evidence_type = %q, want %q", n.ID, ev, EvidenceTypeFilesystem)
			}
			continue
		}
		if ev, _ := n.Metadata[MetaEvidenceType].(string); ev == EvidenceTypeAST {
			sawAST = true
		}
	}
	if !sawAST {
		t.Errorf("expected at least one AST-stamped symbol node across %d AST-backed files", len(files))
	}
}

// TestEvidenceForExt locks the extension → evidence mapping so extractors and
// the orchestrator stay in agreement about confidence levels.
func TestEvidenceForExt(t *testing.T) {
	cases := []struct {
		ext        string
		wantType   string
		wantConfid types.Confidence
	}{
		{".go", EvidenceTypeAST, types.ConfidenceExtracted},
		{".py", EvidenceTypeAST, types.ConfidenceExtracted},
		{".ts", EvidenceTypeAST, types.ConfidenceExtracted},
		{".js", EvidenceTypeAST, types.ConfidenceExtracted},
		{".md", EvidenceTypeDocLLM, types.ConfidenceInferred},
		{".txt", EvidenceTypeDocLLM, types.ConfidenceInferred},
		{".pdf", EvidenceTypePDFLLM, types.ConfidenceInferred},
		{".xyz", EvidenceTypeFilesystem, types.ConfidenceInferred},
	}
	for _, tc := range cases {
		gotType, gotConf := evidenceForExt(tc.ext)
		if gotType != tc.wantType || gotConf != tc.wantConfid {
			t.Errorf("evidenceForExt(%q) = (%q, %q), want (%q, %q)",
				tc.ext, gotType, gotConf, tc.wantType, tc.wantConfid)
		}
	}
}
