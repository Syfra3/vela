package extract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Syfra3/vela/pkg/types"
)

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

func TestIsContractFile(t *testing.T) {
	cases := map[string]bool{
		"foo.proto":        true,
		"openapi.yaml":     true,
		"svc.openapi.json": true,
		"swagger.yml":      true,
		"random.yaml":      false,
		"main.go":          false,
		"README.md":        false,
	}
	for path, want := range cases {
		if got := IsContractFile(path); got != want {
			t.Errorf("IsContractFile(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestExtractContract_OpenAPI_DeclaredEvidence(t *testing.T) {
	dir := t.TempDir()
	spec := `openapi: 3.0.0
info:
  title: Billing
paths:
  /invoices:
    get:
      summary: list
    post:
      summary: create
  /invoices/{id}:
    get:
      summary: read
`
	path := writeFile(t, dir, "openapi.yaml", spec)
	src := &types.Source{Type: types.SourceTypeCodebase, Name: "acme", Path: dir}

	nodes, edges, err := ExtractContract(dir, []string{path}, src)
	if err != nil {
		t.Fatalf("ExtractContract: %v", err)
	}

	// Expect service + 3 endpoints.
	var service *types.Node
	endpoints := 0
	for i, n := range nodes {
		if n.NodeType == string(types.NodeTypeService) {
			service = &nodes[i]
		}
		if n.NodeType == string(types.NodeTypeContract) {
			endpoints++
		}
		// Every node must carry contract + declared evidence.
		if n.Metadata[MetaLayer] != string(types.LayerContract) {
			t.Errorf("node %s: layer = %v, want contract", n.ID, n.Metadata[MetaLayer])
		}
		if n.Metadata[MetaEvidenceConfidence] != string(types.ConfidenceDeclared) {
			t.Errorf("node %s: confidence = %v, want declared", n.ID, n.Metadata[MetaEvidenceConfidence])
		}
		if n.Metadata[MetaEvidenceType] != EvidenceTypeOpenAPI {
			t.Errorf("node %s: evidence_type = %v, want openapi", n.ID, n.Metadata[MetaEvidenceType])
		}
	}
	if service == nil {
		t.Fatalf("no service node emitted, got: %v", nodes)
	}
	if service.Label != "Billing" {
		t.Errorf("service label = %q, want Billing", service.Label)
	}
	if endpoints != 3 {
		t.Errorf("endpoints = %d, want 3", endpoints)
	}
	if !strings.HasPrefix(service.ID, "contract:service:") {
		t.Errorf("service id %q missing contract namespace", service.ID)
	}

	// Binding edge to the project must exist (workspace-consumable).
	foundBinding := false
	declareEdges := 0
	for _, e := range edges {
		if e.Metadata[MetaEvidenceConfidence] != string(types.ConfidenceDeclared) {
			t.Errorf("edge %+v not stamped declared", e)
		}
		if e.Relation == "declared_in" && e.Source == service.ID && e.Target == ProjectNodeID("acme") {
			foundBinding = true
		}
		if e.Relation == "declares" {
			declareEdges++
		}
	}
	if !foundBinding {
		t.Errorf("expected declared_in edge service→project, got %v", edges)
	}
	if declareEdges != 3 {
		t.Errorf("declares edges = %d, want 3", declareEdges)
	}
}

func TestExtractContract_Proto(t *testing.T) {
	dir := t.TempDir()
	body := `syntax = "proto3";
package acme.billing;

service Ledger {
  rpc Post(PostRequest) returns (PostResponse);
  rpc List(ListRequest) returns (ListResponse);
}

service Audit {
  rpc Record(RecordRequest) returns (RecordResponse);
}
`
	path := writeFile(t, dir, "svc.proto", body)
	src := &types.Source{Type: types.SourceTypeCodebase, Name: "acme", Path: dir}

	nodes, edges, err := ExtractContract(dir, []string{path}, src)
	if err != nil {
		t.Fatalf("ExtractContract: %v", err)
	}

	services := map[string]bool{}
	rpcs := 0
	for _, n := range nodes {
		if n.NodeType == string(types.NodeTypeService) {
			services[n.Label] = true
		}
		if n.NodeType == string(types.NodeTypeContract) {
			rpcs++
		}
		if n.Metadata[MetaEvidenceType] != EvidenceTypeProto {
			t.Errorf("node %s evidence_type = %v want proto", n.ID, n.Metadata[MetaEvidenceType])
		}
	}
	if !services["Ledger"] || !services["Audit"] {
		t.Errorf("want both services, got %v", services)
	}
	if rpcs != 3 {
		t.Errorf("rpc nodes = %d, want 3", rpcs)
	}

	declares := 0
	for _, e := range edges {
		if e.Relation == "declares" {
			declares++
		}
	}
	if declares != 3 {
		t.Errorf("declares edges = %d, want 3", declares)
	}
}

func TestExtractContract_IgnoresNonContractFiles(t *testing.T) {
	dir := t.TempDir()
	p1 := writeFile(t, dir, "config.yaml", "server: true\nport: 8080\n")
	p2 := writeFile(t, dir, "main.go", "package main\nfunc main(){}\n")
	src := &types.Source{Type: types.SourceTypeCodebase, Name: "acme", Path: dir}
	nodes, edges, err := ExtractContract(dir, []string{p1, p2}, src)
	if err != nil {
		t.Fatalf("ExtractContract: %v", err)
	}
	if len(nodes) != 0 || len(edges) != 0 {
		t.Errorf("expected no contract output, got %d nodes %d edges", len(nodes), len(edges))
	}
}
