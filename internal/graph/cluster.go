package graph

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	embeddedscripts "github.com/Syfra3/vela/scripts"
)

var ErrGraspologicMissing = errors.New("graspologic is not installed")
var ErrClusteringDepsMissing = errors.New("python clustering dependencies are not installed")
var embeddedLeidenScript = embeddedscripts.LeidenPy

type ClusteringEnvironment struct {
	PythonFound          bool   `json:"python_found"`
	PythonPath           string `json:"python_path,omitempty"`
	NetworkXAvailable    bool   `json:"networkx_available"`
	GraspologicAvailable bool   `json:"graspologic_available"`
	BaseInstallCommand   string `json:"base_install_command"`
	LeidenInstallCommand string `json:"leiden_install_command"`
}

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
	scriptPath, cleanup, err := resolveLeidenScript("leiden.py")
	if err != nil {
		return nil, fmt.Errorf("leiden script not found: %w", err)
	}
	defer cleanup()

	// Build input payload using resolved edges only.
	// e.Target is the resolved node label after Build(); Leiden needs node IDs,
	// so we map label → nodeID via a labelIndex.
	labelToID := make(map[string]string, len(g.Nodes))
	for _, n := range g.Nodes {
		labelToID[n.Label] = n.ID
	}

	input := leidenInput{
		Nodes: make([]string, 0, len(g.Nodes)),
		Edges: make([]leidenEdge, 0, len(g.ResolvedEdges)),
	}
	for _, n := range g.Nodes {
		input.Nodes = append(input.Nodes, n.ID)
	}
	for _, e := range g.ResolvedEdges {
		toID := labelToID[e.Target]
		if toID == "" {
			toID = e.Target // fallback: already an ID
		}
		input.Edges = append(input.Edges, leidenEdge{From: e.Source, To: toID})
	}

	payload, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshalling leiden input: %w", err)
	}

	// Locate Python interpreter
	python, err := findPython(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("python not found: %w (install python3 to enable clustering)", err)
	}
	if err := ensureClusteringDependencies(python); err != nil {
		return nil, err
	}

	cmd := exec.Command(python, scriptPath)
	cmd.Stdin = bytes.NewReader(payload)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrText := strings.TrimSpace(stderr.String())
		if strings.Contains(stderrText, "graspologic is not installed") {
			return nil, fmt.Errorf("%w: %s", ErrGraspologicMissing, stderrText)
		}
		if stderrText != "" {
			return nil, fmt.Errorf("leiden subprocess failed: %w\nstderr: %s", err, stderrText)
		}
		return nil, fmt.Errorf("leiden subprocess failed: %w", err)
	}

	var partition map[string]int
	if err := json.Unmarshal(stdout.Bytes(), &partition); err != nil {
		return nil, fmt.Errorf("parsing leiden output: %w\nraw: %s", err, stdout.String())
	}

	return partition, nil
}

func resolveLeidenScript(name string) (string, func(), error) {
	if scriptPath, err := findScript(name); err == nil {
		return scriptPath, func() {}, nil
	}
	return materializeEmbeddedScript(name, embeddedLeidenScript)
}

func materializeEmbeddedScript(name string, script []byte) (string, func(), error) {
	if len(script) == 0 {
		return "", func() {}, fmt.Errorf("embedded script is empty")
	}
	tempDir, err := os.MkdirTemp("", "vela-leiden-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("create temp dir: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}
	scriptPath := filepath.Join(tempDir, name)
	if err := os.WriteFile(scriptPath, script, 0o600); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("write embedded script: %w", err)
	}
	return scriptPath, cleanup, nil
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

// findPython returns the path to a Python 3 interpreter, preferring a repo-local
// virtualenv when present so clustering dependencies can be installed without
// mutating the system Python environment.
func findPython(scriptPath string) (string, error) {
	for _, candidate := range localPythonCandidates(scriptPath) {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	for _, candidate := range []string{"python3", "python"} {
		if p, err := exec.LookPath(candidate); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no python3 or python interpreter in PATH")
}

func localPythonCandidates(scriptPath string) []string {
	dirs := make([]string, 0, 4)
	if scriptPath != "" {
		dirs = append(dirs, filepath.Dir(filepath.Dir(scriptPath)))
	}
	if cwd, err := os.Getwd(); err == nil {
		dirs = append(dirs, cwd)
	}

	candidates := make([]string, 0, len(dirs)*2)
	seen := make(map[string]struct{}, len(dirs))
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		candidates = append(candidates,
			filepath.Join(dir, ".venv", "bin", "python3"),
			filepath.Join(dir, "venv", "bin", "python3"),
		)
	}
	return candidates
}

func LoadClusteringEnvironment() ClusteringEnvironment {
	env := ClusteringEnvironment{
		BaseInstallCommand:   "python3 -m venv .venv && .venv/bin/pip install -r requirements-clustering.txt",
		LeidenInstallCommand: ".venv/bin/pip install -r requirements-clustering-leiden.txt",
	}

	python, err := findPython("")
	if err != nil {
		return env
	}
	env.PythonFound = true
	env.PythonPath = python
	env.NetworkXAvailable = pythonModuleAvailable(python, "networkx")
	env.GraspologicAvailable = pythonModuleAvailable(python, "graspologic")
	return env
}

func pythonModuleAvailable(python, module string) bool {
	cmd := exec.Command(python, "-c", fmt.Sprintf(`import importlib.util, sys; sys.exit(0 if importlib.util.find_spec(%q) else 1)`, module))
	return cmd.Run() == nil
}

func ensureClusteringDependencies(python string) error {
	if pythonModuleAvailable(python, "graspologic") || pythonModuleAvailable(python, "networkx") {
		return nil
	}
	return fmt.Errorf("%w; install them with: %s", ErrClusteringDepsMissing, LoadClusteringEnvironment().BaseInstallCommand)
}
