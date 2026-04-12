package graph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// leidenInput is the payload sent to the Python subprocess via stdin.
type leidenInput struct {
	Nodes []string     `json:"nodes"`
	Edges []leidenEdge `json:"edges"`
}

type leidenEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// RunLeiden runs Leiden community detection on the graph by delegating to the
// bundled Python script (scripts/leiden.py). Returns a map of node ID →
// community ID (0-indexed integers).
//
// If Python or the script is unavailable, a clear error is returned (no panic).
// The script itself falls back to connected-components if graspologic is missing.
func RunLeiden(g *Graph) (map[string]int, error) {
	scriptPath, err := findScript("leiden.py")
	if err != nil {
		return nil, fmt.Errorf("leiden script not found: %w", err)
	}

	// Build input payload
	input := leidenInput{
		Nodes: make([]string, 0, len(g.Nodes)),
		Edges: make([]leidenEdge, 0, len(g.EdgeList)),
	}
	for _, n := range g.Nodes {
		input.Nodes = append(input.Nodes, n.ID)
	}
	for _, e := range g.EdgeList {
		input.Edges = append(input.Edges, leidenEdge{From: e.Source, To: e.Target})
	}

	payload, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshalling leiden input: %w", err)
	}

	// Locate Python interpreter
	python, err := findPython()
	if err != nil {
		return nil, fmt.Errorf("python not found: %w (install python3 to enable clustering)", err)
	}

	cmd := exec.Command(python, scriptPath)
	cmd.Stdin = bytes.NewReader(payload)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("leiden subprocess failed: %w\nstderr: %s", err, stderr.String())
	}

	var partition map[string]int
	if err := json.Unmarshal(stdout.Bytes(), &partition); err != nil {
		return nil, fmt.Errorf("parsing leiden output: %w\nraw: %s", err, stdout.String())
	}

	return partition, nil
}

// ApplyCommunities writes community IDs back onto the graph's node metadata
// and populates the community → []nodeID index for report generation.
func (g *Graph) ApplyCommunities(partition map[string]int) map[int][]string {
	communities := make(map[int][]string)
	for i, n := range g.Nodes {
		comm, ok := partition[n.ID]
		if !ok {
			comm = 0
		}
		g.Nodes[i].Community = comm
		communities[comm] = append(communities[comm], n.ID)
	}
	return communities
}

// findScript locates scripts/<name> relative to the binary or repo root.
// Search order: next to the binary, then up the directory tree to find
// a directory containing "go.mod" (repo root).
func findScript(name string) (string, error) {
	// 1. Next to the running binary
	exe, err := os.Executable()
	if err == nil {
		p := filepath.Join(filepath.Dir(exe), "scripts", name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	// 2. Walk up from the source file location (works in tests)
	_, src, _, ok := runtime.Caller(0)
	if ok {
		dir := filepath.Dir(src)
		for i := 0; i < 6; i++ {
			p := filepath.Join(dir, "scripts", name)
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	// 3. Current working directory
	if cwd, err := os.Getwd(); err == nil {
		p := filepath.Join(cwd, "scripts", name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("scripts/%s not found near binary or repo root", name)
}

// findPython returns the path to a Python 3 interpreter.
func findPython() (string, error) {
	for _, candidate := range []string{"python3", "python"} {
		if p, err := exec.LookPath(candidate); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no python3 or python interpreter in PATH")
}
