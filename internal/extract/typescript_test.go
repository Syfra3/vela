package extract

import (
	"path/filepath"
	"testing"
)

func TestParseTSFile(t *testing.T) {
	fixture := filepath.Join(fixtureDir(), "sample.ts")
	root, src, err := ParseTSFile(fixture)
	if err != nil {
		t.Fatalf("ParseTSFile error: %v", err)
	}
	if root == nil {
		t.Fatal("expected non-nil root")
	}
	if len(src) == 0 {
		t.Error("expected non-empty source")
	}
}

func TestExtractTSNodes_ClassAndInterface(t *testing.T) {
	fixture := filepath.Join(fixtureDir(), "sample.ts")
	root, src, err := ParseTSFile(fixture)
	if err != nil || root == nil {
		t.Fatal("parse failed")
	}

	nodes := ExtractTSNodes(root, src, "sample.ts")
	byLabel := make(map[string]string)
	for _, n := range nodes {
		byLabel[n.Label] = n.NodeType
	}

	if kind, ok := byLabel["Server"]; !ok {
		t.Error("expected Server class node")
	} else if kind != "struct" {
		t.Errorf("expected kind=struct for class, got %q", kind)
	}

	if kind, ok := byLabel["Logger"]; !ok {
		t.Error("expected Logger interface node")
	} else if kind != "interface" {
		t.Errorf("expected kind=interface, got %q", kind)
	}

	if kind, ok := byLabel["createServer"]; !ok {
		t.Error("expected createServer function node")
	} else if kind != "function" {
		t.Errorf("expected kind=function, got %q", kind)
	}
}

func TestExtractTSEdges_CallEdge(t *testing.T) {
	fixture := filepath.Join(fixtureDir(), "sample.ts")
	root, src, err := ParseTSFile(fixture)
	if err != nil || root == nil {
		t.Fatal("parse failed")
	}

	edges := ExtractTSEdges(root, src, "sample.ts")

	// start() calls this.listen()
	found := false
	for _, e := range edges {
		if e.Target == "listen" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected edge to 'listen', got: %+v", edges)
	}
}
