package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Syfra3/vela/internal/config"
	"github.com/Syfra3/vela/internal/llm"
	"github.com/Syfra3/vela/internal/setup"
	"github.com/Syfra3/vela/pkg/types"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var llmHealthCheck = func(ctx context.Context, cfg *types.LLMConfig) error {
	client, err := llm.NewClient(cfg)
	if err != nil {
		return err
	}
	return client.Health(ctx)
}

var checkOllama = setup.CheckOllama
var getOllamaModels = setup.GetOllamaModels
var checkMCPInstalled = setup.CheckMCPInstalled

type StepStatus string

const (
	StepOK   StepStatus = "ok"
	StepWarn StepStatus = "warn"
	StepFail StepStatus = "fail"
)

type Step struct {
	Name   string
	Status StepStatus
	Detail string
}

func IntegrationChecks(cfg *types.Config, graphPath string) []Step {
	if cfg == nil {
		cfg = &types.Config{}
	}

	source, hasSource := findAncoraSource(cfg)
	steps := []Step{checkAncoraSource(source, hasSource)}
	steps = append(steps, checkSocketListener(cfg, source, hasSource))
	steps = append(steps, checkLLMRuntime(cfg))
	steps = append(steps, checkGraph(graphPath))
	steps = append(steps, checkMCP())
	steps = append(steps, checkObsidian(cfg))
	return steps
}

func findAncoraSource(cfg *types.Config) (types.WatchSourceConfig, bool) {
	for _, src := range cfg.Watch.Sources {
		if src.Name == "ancora" || strings.Contains(filepath.Base(src.Socket), "ancora") {
			return src, true
		}
	}
	return types.WatchSourceConfig{}, false
}

func checkAncoraSource(source types.WatchSourceConfig, ok bool) Step {
	if !ok {
		return Step{Name: "Ancora event source", Status: StepFail, Detail: "No Ancora watch source configured"}
	}
	return Step{Name: "Ancora event source", Status: StepOK, Detail: fmt.Sprintf("Configured socket %s", source.Socket)}
}

func checkSocketListener(cfg *types.Config, source types.WatchSourceConfig, hasSource bool) Step {
	if !hasSource {
		return Step{Name: "Socket / listener", Status: StepFail, Detail: "Ancora source is not configured"}
	}

	if cfg.Daemon.StatusFile != "" {
		if data, err := os.ReadFile(cfg.Daemon.StatusFile); err == nil {
			var status types.DaemonStatus
			if json.Unmarshal(data, &status) == nil {
				if src, ok := status.Sources[source.Name]; ok {
					if src.Connected {
						return Step{Name: "Socket / listener", Status: StepOK, Detail: fmt.Sprintf("Daemon connected, %d event(s) seen", src.EventCount)}
					}
					return Step{Name: "Socket / listener", Status: StepFail, Detail: "Daemon sees source, but listener is disconnected"}
				}
			}
		}
	}

	if _, err := os.Stat(source.Socket); err == nil {
		return Step{Name: "Socket / listener", Status: StepWarn, Detail: "Socket exists, but daemon status is unavailable"}
	}

	return Step{Name: "Socket / listener", Status: StepFail, Detail: "Socket not found and daemon status is unavailable"}
}

func checkLLMRuntime(cfg *types.Config) Step {
	llmCfg := cfg.LLM
	if llmCfg.Provider == "" {
		return Step{Name: "LLM runtime", Status: StepFail, Detail: "No LLM provider configured"}
	}

	if llmCfg.Provider == "local" {
		installed, running, _, err := checkOllama()
		if err != nil {
			return Step{Name: "LLM runtime", Status: StepFail, Detail: fmt.Sprintf("checking Ollama: %v", err)}
		}
		if !installed {
			return Step{Name: "LLM runtime", Status: StepFail, Detail: "Ollama is not installed"}
		}
		if !running {
			return Step{Name: "LLM runtime", Status: StepFail, Detail: "Ollama is installed but not running"}
		}
		if llmCfg.Model != "" {
			models, err := getOllamaModels()
			if err != nil {
				return Step{Name: "LLM runtime", Status: StepFail, Detail: fmt.Sprintf("listing Ollama models: %v", err)}
			}
			if !hasModel(models, llmCfg.Model) {
				return Step{Name: "LLM runtime", Status: StepFail, Detail: fmt.Sprintf("Ollama model %q is not available", llmCfg.Model)}
			}
			if !hasModel(models, setup.LocalSearchEmbeddingModel) {
				return Step{Name: "LLM runtime", Status: StepFail, Detail: fmt.Sprintf("Ollama embedding model %q is not available", setup.LocalSearchEmbeddingModel)}
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := llmHealthCheck(ctx, &llmCfg); err != nil {
		return Step{Name: "LLM runtime", Status: StepFail, Detail: err.Error()}
	}

	return Step{Name: "LLM runtime", Status: StepOK, Detail: fmt.Sprintf("%s provider is reachable", llmCfg.Provider)}
}

func checkGraph(graphPath string) Step {
	path := graphPath
	if path == "" {
		var err error
		path, err = config.FindGraphFile(".")
		if err != nil {
			return Step{Name: "Graph update / reconcile", Status: StepFail, Detail: "graph.json not found"}
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Step{Name: "Graph update / reconcile", Status: StepFail, Detail: fmt.Sprintf("reading graph: %v", err)}
	}

	var raw struct {
		Nodes []struct {
			ID         string `json:"id"`
			Kind       string `json:"kind"`
			SourceType string `json:"source_type,omitempty"`
			SourceName string `json:"source_name,omitempty"`
		} `json:"nodes"`
		Edges []json.RawMessage `json:"edges"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Step{Name: "Graph update / reconcile", Status: StepFail, Detail: fmt.Sprintf("parsing graph: %v", err)}
	}
	if len(raw.Nodes) == 0 {
		return Step{Name: "Graph update / reconcile", Status: StepFail, Detail: "graph.json has no nodes"}
	}

	observations := 0
	for _, n := range raw.Nodes {
		if n.Kind == string(types.NodeTypeObservation) && (n.SourceType == string(types.SourceTypeMemory) || n.SourceName == "ancora" || strings.HasPrefix(n.ID, "ancora:obs:")) {
			observations++
		}
	}
	if observations == 0 {
		return Step{Name: "Graph update / reconcile", Status: StepFail, Detail: "No Ancora observation nodes found in graph.json"}
	}

	return Step{Name: "Graph update / reconcile", Status: StepOK, Detail: fmt.Sprintf("graph.json is healthy (%d nodes, %d edges, %d Ancora observations)", len(raw.Nodes), len(raw.Edges), observations)}
}

func checkMCP() Step {
	if !checkMCPInstalled() {
		return Step{Name: "MCP registration", Status: StepWarn, Detail: "Vela MCP server is not registered in a supported client"}
	}
	return Step{Name: "MCP registration", Status: StepOK, Detail: "Vela MCP server is registered"}
}

func checkObsidian(cfg *types.Config) Step {
	if !cfg.Obsidian.AutoSync {
		return Step{Name: "Obsidian sync / export", Status: StepWarn, Detail: "obsidian.auto_sync is disabled"}
	}

	vaultDir := config.ResolveVaultDir(cfg.Obsidian.VaultDir)

	memDir := filepath.Join(vaultDir, "obsidian", "Memories")
	notes := 0
	_ = filepath.WalkDir(memDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".md") {
			notes++
		}
		return nil
	})
	if notes == 0 {
		return Step{Name: "Obsidian sync / export", Status: StepFail, Detail: "No exported memory notes found in Obsidian vault"}
	}

	return Step{Name: "Obsidian sync / export", Status: StepOK, Detail: fmt.Sprintf("Found %d exported memory note(s)", notes)}
}

func hasModel(models []string, target string) bool {
	for _, model := range models {
		if model == target || strings.HasPrefix(model, target+":") {
			return true
		}
	}
	return false
}
