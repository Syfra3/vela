package doctor

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Syfra3/vela/internal/config"
	"github.com/Syfra3/vela/pkg/types"
)

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
	source, hasSource := findAncoraSource(cfg)
	steps := []Step{checkAncoraSource(source, hasSource)}
	steps = append(steps, checkSocketListener(cfg, source, hasSource))
	steps = append(steps, checkGraph(graphPath))
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
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Step{Name: "Graph update / reconcile", Status: StepFail, Detail: fmt.Sprintf("parsing graph: %v", err)}
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

	return Step{Name: "Graph update / reconcile", Status: StepOK, Detail: fmt.Sprintf("graph.json contains %d Ancora observation node(s)", observations)}
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
