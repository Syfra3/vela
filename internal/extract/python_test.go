package extract

import (
	"path/filepath"
	"testing"
)

func TestParsePythonFile(t *testing.T) {
	fixture := filepath.Join(fixtureDir(), "sample.py")
	root, src, err := ParsePythonFile(fixture)
	if err != nil {
		t.Fatalf("ParsePythonFile error: %v", err)
	}
	if root == nil {
		t.Fatal("expected non-nil root")
	}
	if len(src) == 0 {
		t.Error("expected non-empty source")
	}
}

func TestExtractPythonNodes_FunctionAndClass(t *testing.T) {
	fixture := filepath.Join(fixtureDir(), "sample.py")
	root, src, err := ParsePythonFile(fixture)
	if err != nil || root == nil {
		t.Fatal("parse failed")
	}

	nodes := ExtractPythonNodes(root, src, "sample.py")

	byLabel := make(map[string]string)
	for _, n := range nodes {
		byLabel[n.Label] = n.NodeType
	}

	// Class
	if kind, ok := byLabel["Server"]; !ok {
		t.Error("expected Server class node")
	} else if kind != "struct" {
		t.Errorf("expected kind=struct for class, got %q", kind)
	}

	// Top-level function
	if kind, ok := byLabel["create_server"]; !ok {
		t.Error("expected create_server function node")
	} else if kind != "function" {
		t.Errorf("expected kind=function, got %q", kind)
	}

	// Methods are also function_definitions
	if _, ok := byLabel["start"]; !ok {
		t.Error("expected start method node")
	}
}

func TestExtractPythonEdges_CallEdge(t *testing.T) {
	fixture := filepath.Join(fixtureDir(), "sample.py")
	root, src, err := ParsePythonFile(fixture)
	if err != nil || root == nil {
		t.Fatal("parse failed")
	}

	edges := ExtractPythonEdges(root, src, "sample.py")

	// start() calls self.listen()
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
