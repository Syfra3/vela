package tui

import (
	"fmt"
	"strings"

	"github.com/Syfra3/vela/internal/config"
	"github.com/Syfra3/vela/internal/export"
	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/internal/registry"
	"github.com/Syfra3/vela/pkg/types"
)

func loadGraph(graphPath string) (*types.Graph, error) {
	g, err := export.LoadJSON(graphPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", graphPath, err)
	}
	return g, nil
}

func loadObsidianExportGraph(lastGraphPath string) (*types.Graph, string, error) {
	entries, err := registry.Load()
	if err != nil {
		return nil, "", err
	}
	if len(entries) > 0 {
		g, loaded, err := loadTrackedGraphs(entries)
		if loaded > 0 {
			return g, config.RegistryFilePath(), nil
		}
		if err != nil {
			return nil, config.RegistryFilePath(), err
		}
	}
	graphPath := strings.TrimSpace(lastGraphPath)
	if graphPath == "" {
		graphPath, err = config.FindGraphFile(".")
		if err != nil {
			return nil, "", err
		}
	}
	g, err := loadGraph(graphPath)
	if err != nil {
		return nil, graphPath, err
	}
	return g, graphPath, nil
}

func loadTrackedGraphs(entries []registry.Entry) (*types.Graph, int, error) {
	merged := &types.Graph{}
	loaded := 0
	var lastErr error
	for _, entry := range entries {
		graphPath := strings.TrimSpace(entry.GraphPath)
		if graphPath == "" {
			continue
		}
		g, err := loadGraph(graphPath)
		if err != nil {
			lastErr = err
			continue
		}
		merged.Nodes = append(merged.Nodes, g.Nodes...)
		merged.Edges = append(merged.Edges, g.Edges...)
		loaded++
	}
	if loaded == 0 {
		if lastErr != nil {
			return nil, 0, lastErr
		}
		return nil, 0, fmt.Errorf("no tracked repository graphs available")
	}
	merged.Nodes, merged.Edges = igraph.Canonicalize(merged.Nodes, merged.Edges)
	return merged, loaded, nil
}
