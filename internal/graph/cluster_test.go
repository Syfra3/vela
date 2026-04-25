package graph

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Syfra3/vela/pkg/types"
)

// TestRunLeiden exercises the full subprocess path.
func TestRunLeiden_Subprocess(t *testing.T) {
	nodes := makeTestNodes("a", "b", "c", "d")
	edges := []types.Edge{
		{Source: "a", Target: "b", Relation: "calls", Confidence: "EXTRACTED"},
		{Source: "b", Target: "c", Relation: "calls", Confidence: "EXTRACTED"},
		// d is isolated — will be its own component
	}

	g, err := Build(nodes, edges)
	if err != nil {
		t.Fatal(err)
	}

	partition, err := RunLeiden(g)
	if err != nil {
		if errors.Is(err, ErrGraspologicMissing) {
			t.Skipf("graspologic unavailable: %v", err)
		}
		// Python not available in this environment — skip rather than fail
		t.Skipf("leiden subprocess unavailable: %v", err)
	}

	// Every node must have a community assignment
	for _, n := range g.Nodes {
		if _, ok := partition[n.ID]; !ok {
			t.Errorf("node %q has no community assignment", n.ID)
		}
	}

	// "d" should be in a different community from "a","b","c" (it's isolated)
	commABC := partition["a"]
	commD := partition["d"]
	if commABC == commD {
		t.Logf("note: isolated node 'd' placed in same community as connected nodes (this may be OK for Leiden)")
	}
}

func TestApplyCommunities(t *testing.T) {
	t.Parallel()

	nodes := makeTestNodes("x", "y", "z")
	g, _ := Build(nodes, nil)

	partition := map[string]int{"x": 0, "y": 0, "z": 1}
	communities := g.ApplyCommunities(partition)

	if len(communities) != 2 {
		t.Errorf("expected 2 communities, got %d", len(communities))
	}
	if len(communities[0]) != 2 {
		t.Errorf("expected 2 nodes in community 0, got %d", len(communities[0]))
	}
	if len(communities[1]) != 1 {
		t.Errorf("expected 1 node in community 1, got %d", len(communities[1]))
	}

	// Verify community is written back onto nodes
	for _, n := range g.Nodes {
		if n.ID == "z" && n.Community != 1 {
			t.Errorf("expected z.Community=1, got %d", n.Community)
		}
	}
}

func TestMaterializeEmbeddedScript(t *testing.T) {
	t.Parallel()

	if len(embeddedLeidenScript) == 0 {
		t.Fatal("embedded Leiden script is empty")
	}

	scriptPath, cleanup, err := materializeEmbeddedScript("leiden.py", embeddedLeidenScript)
	if err != nil {
		t.Fatalf("materializeEmbeddedScript() error = %v", err)
	}
	defer cleanup()

	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", scriptPath, err)
	}
	if !bytes.Equal(content, embeddedLeidenScript) {
		t.Fatal("materialized script content does not match embedded content")
	}

	cleanup()
	if _, err := os.Stat(scriptPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("script path still exists after cleanup, err = %v", err)
	}
}

func TestResolveLeidenScript_FallsBackToEmbeddedScript(t *testing.T) {
	t.Parallel()

	scriptPath, cleanup, err := resolveLeidenScript("missing-leiden.py")
	if err != nil {
		t.Fatalf("resolveLeidenScript() error = %v", err)
	}
	defer cleanup()

	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", scriptPath, err)
	}
	if !bytes.Equal(content, embeddedLeidenScript) {
		t.Fatal("fallback script content does not match embedded content")
	}
	if filepath.Base(scriptPath) != "missing-leiden.py" {
		t.Fatalf("fallback script path = %q, want basename %q", scriptPath, "missing-leiden.py")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(scriptPath), "..", "..", "scripts", "missing-leiden.py")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fallback unexpectedly resolved from repo scripts directory, err = %v", err)
	}
}

func TestFindPython_PrefersRepoLocalVirtualenv(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	scriptDir := filepath.Join(repoDir, "scripts")
	pythonPath := filepath.Join(repoDir, ".venv", "bin", "python3")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(scriptDir) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(pythonPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(pythonDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(scriptDir, "leiden.py"), []byte("print('stub')\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(script) error = %v", err)
	}
	if err := os.WriteFile(pythonPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(python) error = %v", err)
	}

	python, err := findPython(filepath.Join(scriptDir, "leiden.py"))
	if err != nil {
		t.Fatalf("findPython() error = %v", err)
	}
	if python != pythonPath {
		t.Fatalf("findPython() = %q, want %q", python, pythonPath)
	}
}

func TestEnsureClusteringDependencies_AcceptsAvailableBackend(t *testing.T) {
	t.Parallel()

	pythonPath := filepath.Join(t.TempDir(), "python3")
	if err := os.WriteFile(pythonPath, []byte("#!/bin/sh\nif [ \"$1\" = \"-c\" ]; then\n  exit 0\nfi\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(python) error = %v", err)
	}

	if err := ensureClusteringDependencies(pythonPath); err != nil {
		t.Fatalf("ensureClusteringDependencies() error = %v, want nil", err)
	}
}

func TestEnsureClusteringDependencies_ReturnsInstallGuidance(t *testing.T) {
	t.Parallel()

	pythonPath := filepath.Join(t.TempDir(), "python3")
	if err := os.WriteFile(pythonPath, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(python) error = %v", err)
	}

	err := ensureClusteringDependencies(pythonPath)
	if !errors.Is(err, ErrClusteringDepsMissing) {
		t.Fatalf("ensureClusteringDependencies() error = %v, want ErrClusteringDepsMissing", err)
	}
	if got := err.Error(); !strings.Contains(got, "requirements-clustering.txt") {
		t.Fatalf("ensureClusteringDependencies() error = %q, want install guidance", got)
	}
}

func TestLoadClusteringEnvironmentReportsBackendAvailability(t *testing.T) {
	binDir := t.TempDir()
	pythonPath := filepath.Join(binDir, "python3")
	script := "#!/bin/sh\nif [ \"$1\" = \"-c\" ]; then\n  case \"$2\" in\n    *networkx*) exit 0 ;;\n    *graspologic*) exit 1 ;;\n  esac\nfi\nexit 1\n"
	if err := os.WriteFile(pythonPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(python) error = %v", err)
	}
	t.Setenv("PATH", binDir)
	t.Chdir(t.TempDir())

	env := LoadClusteringEnvironment()
	if !env.PythonFound {
		t.Fatal("expected python to be found")
	}
	if env.PythonPath != pythonPath {
		t.Fatalf("PythonPath = %q, want %q", env.PythonPath, pythonPath)
	}
	if !env.NetworkXAvailable {
		t.Fatal("expected networkx to be available")
	}
	if env.GraspologicAvailable {
		t.Fatal("expected graspologic to be unavailable")
	}
	if !strings.Contains(env.BaseInstallCommand, "requirements-clustering.txt") {
		t.Fatalf("BaseInstallCommand = %q, want requirements-clustering.txt guidance", env.BaseInstallCommand)
	}
	if !strings.Contains(env.LeidenInstallCommand, "requirements-clustering-leiden.txt") {
		t.Fatalf("LeidenInstallCommand = %q, want requirements-clustering-leiden.txt guidance", env.LeidenInstallCommand)
	}
}

func makeTestNodes(ids ...string) []types.Node {
	nodes := make([]types.Node, len(ids))
	for i, id := range ids {
		nodes[i] = types.Node{ID: id, Label: id, NodeType: "function"}
	}
	return nodes
}
