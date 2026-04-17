package extract

import (
	"path/filepath"
	"runtime"
	"testing"
)

// fixtureDir returns the absolute path to tests/fixtures/extract
func fixtureDir() string {
	_, file, _, _ := runtime.Caller(0)
	// file = .../internal/extract/code_test.go
	// go up two levels to repo root, then into tests/fixtures/extract
	root := filepath.Join(filepath.Dir(file), "..", "..")
	return filepath.Join(root, "tests", "fixtures", "extract")
}

func TestParseGoFile_RootNode(t *testing.T) {
	fixture := filepath.Join(fixtureDir(), "sample.go")
	root, src, err := ParseGoFile(fixture)
	if err != nil {
		t.Fatalf("ParseGoFile error: %v", err)
	}
	if root == nil {
		t.Fatal("expected non-nil root node")
	}
	if root.file == nil {
		t.Fatal("expected parsed Go AST file")
	}
	if root.file.Name == nil || root.file.Name.Name != "sample" {
		t.Errorf("expected package name 'sample', got %v", root.file.Name)
	}
	if len(src) == 0 {
		t.Error("expected non-empty source bytes")
	}
}

func TestExtractGoNodes_Functions(t *testing.T) {
	fixture := filepath.Join(fixtureDir(), "sample.go")
	root, src, err := ParseGoFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	nodes := ExtractGoNodes(root, src, "sample.go")

	byLabel := make(map[string]string)
	for _, n := range nodes {
		byLabel[n.Label] = n.NodeType
	}

	// Should find function NewServer
	if kind, ok := byLabel["NewServer"]; !ok {
		t.Error("expected NewServer node")
	} else if kind != "function" {
		t.Errorf("expected kind=function for NewServer, got %q", kind)
	}

	// Should find method Start
	if kind, ok := byLabel["Start"]; !ok {
		t.Error("expected Start node")
	} else if kind != "method" {
		t.Errorf("expected kind=method for Start, got %q", kind)
	}
}

func TestExtractGoNodes_Struct(t *testing.T) {
	fixture := filepath.Join(fixtureDir(), "sample.go")
	root, src, err := ParseGoFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	nodes := ExtractGoNodes(root, src, "sample.go")

	byLabel := make(map[string]string)
	for _, n := range nodes {
		byLabel[n.Label] = n.NodeType
	}

	if kind, ok := byLabel["Server"]; !ok {
		t.Error("expected Server struct node")
	} else if kind != "struct" {
		t.Errorf("expected kind=struct, got %q", kind)
	}
}

func TestExtractGoNodes_Interface(t *testing.T) {
	fixture := filepath.Join(fixtureDir(), "sample.go")
	root, src, err := ParseGoFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	nodes := ExtractGoNodes(root, src, "sample.go")

	byLabel := make(map[string]string)
	for _, n := range nodes {
		byLabel[n.Label] = n.NodeType
	}

	if kind, ok := byLabel["Handler"]; !ok {
		t.Error("expected Handler interface node")
	} else if kind != "interface" {
		t.Errorf("expected kind=interface, got %q", kind)
	}
}

func TestExtractGoEdges_CallEdge(t *testing.T) {
	fixture := filepath.Join(fixtureDir(), "sample.go")
	root, src, err := ParseGoFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	edges := ExtractGoEdges(root, src, "sample.go")

	// Start() calls listen() — expect that edge to exist
	found := false
	for _, e := range edges {
		if e.Target == "listen" && e.Relation == "calls" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected edge Start→listen, got edges: %+v", edges)
	}
}

func TestExtractGoEdges_Confidence(t *testing.T) {
	fixture := filepath.Join(fixtureDir(), "sample.go")
	root, src, err := ParseGoFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	edges := ExtractGoEdges(root, src, "sample.go")
	for _, e := range edges {
		if e.Confidence != "EXTRACTED" {
			t.Errorf("expected confidence=EXTRACTED, got %q", e.Confidence)
		}
	}
}
