package export

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"github.com/Syfra3/vela/pkg/types"
)

// obsidianColor is the wire format Obsidian uses in graph.json colorGroups.
// rgb is a packed 24-bit integer: (r<<16)|(g<<8)|b.
type obsidianColor struct {
	A   float64 `json:"a"`
	RGB int     `json:"rgb"`
}

// obsidianColorGroup maps a search query to a node color in Obsidian graph view.
type obsidianColorGroup struct {
	Query string        `json:"query"`
	Color obsidianColor `json:"color"`
}

// obsidianGraphConfig is the subset of .obsidian/graph.json we write/merge.
type obsidianGraphConfig struct {
	CollapseFilter     bool                 `json:"collapse-filter"`
	Search             string               `json:"search"`
	ShowTags           bool                 `json:"showTags"`
	ShowAttachments    bool                 `json:"showAttachments"`
	HideUnresolved     bool                 `json:"hideUnresolved"`
	ShowOrphans        bool                 `json:"showOrphans"`
	CollapseColor      bool                 `json:"collapse-color-groups"`
	ColorGroups        []obsidianColorGroup `json:"colorGroups"`
	CollapseDisplay    bool                 `json:"collapse-display"`
	ShowArrow          bool                 `json:"showArrow"`
	TextFadeMultiplier float64              `json:"textFadeMultiplier"`
	NodeSizeMultiplier float64              `json:"nodeSizeMultiplier"`
	LineSizeMultiplier float64              `json:"lineSizeMultiplier"`
	CollapseForces     bool                 `json:"collapse-forces"`
	CenterStrength     float64              `json:"centerStrength"`
	RepelStrength      float64              `json:"repelStrength"`
	LinkStrength       float64              `json:"linkStrength"`
	LinkDistance       float64              `json:"linkDistance"`
	Scale              float64              `json:"scale"`
	Close              bool                 `json:"close"`
}

// rgb packs r, g, b bytes into a 24-bit int (Obsidian wire format).
func rgb(r, g, b int) obsidianColor {
	return obsidianColor{A: 1, RGB: (r << 16) | (g << 8) | b}
}

// Color palette — designed for dark Obsidian theme, enough contrast to distinguish.
var (
	// Memory hierarchy colors
	colorMemoryRoot  = rgb(242, 142, 43)  // amber       — Ancora Memory root
	colorWorkspace   = rgb(255, 200, 80)  // gold        — workspace hubs
	colorVisWork     = rgb(78, 190, 160)  // teal        — work visibility
	colorVisPersonal = rgb(225, 87, 89)   // rose        — personal visibility
	colorObsIndex    = rgb(200, 170, 100) // warm tan    — _index notes

	// Project colors (cycled per project)
	projectPalette = []obsidianColor{
		rgb(78, 121, 167),  // steel blue  — project root
		rgb(89, 161, 79),   // green
		rgb(107, 153, 195), // light blue
		rgb(176, 122, 162), // mauve
		rgb(157, 223, 166), // mint
		rgb(255, 157, 167), // salmon
		rgb(200, 220, 100), // lime
	}
)

// WriteObsidianConfig writes .obsidian/graph.json for the default vault at
// outDir/obsidian using colorGroups derived from the graph's node types and
// workspace/project structure.
//
// Group priority (Obsidian first-match wins):
//  1. Memories/_root        — amber memory source root
//  2. Memories/_index       — gold workspace/visibility index notes
//  3. Memories/*/work       — teal (work-scoped observations)
//  4. Memories/*/personal   — rose (personal observations)
//  5. Memories/*/           — workspace catch-all
//  6. Projects/<name>/_index — project root (per-project color)
//  7. Projects/<name>/       — code symbols catch-all
//
// Physics settings are preserved from existing graph.json if present.
func WriteObsidianConfig(g *types.Graph, outDir string) error {
	vaultDir := filepath.Join(outDir, "obsidian")
	return writeObsidianConfig(g, vaultDir)
}

func writeObsidianConfig(g *types.Graph, vaultDir string) error {
	obsidianDir := filepath.Join(vaultDir, ".obsidian")
	if err := os.MkdirAll(obsidianDir, 0755); err != nil {
		return err
	}

	workspaces := collectWorkspaces(g)
	projects := collectProjectDirs(g)

	var groups []obsidianColorGroup

	// ── Memory hierarchy ──────────────────────────────────────────────────
	groups = append(groups,
		obsidianColorGroup{Query: "path:Memories/_root", Color: colorMemoryRoot},
		obsidianColorGroup{Query: "path:Memories/_index", Color: colorObsIndex},
		obsidianColorGroup{Query: "path:Memories/work", Color: colorVisWork},
		obsidianColorGroup{Query: "path:Memories/personal", Color: colorVisPersonal},
	)

	// Per-workspace groups (work > personal > catch-all).
	for _, ws := range workspaces {
		safe := safePathComponent(ws)
		groups = append(groups,
			obsidianColorGroup{Query: "path:Memories/" + safe + "/work", Color: colorVisWork},
			obsidianColorGroup{Query: "path:Memories/" + safe + "/personal", Color: colorVisPersonal},
			obsidianColorGroup{Query: "path:Memories/" + safe, Color: colorWorkspace},
		)
	}

	// Catch-all for all Memories
	groups = append(groups,
		obsidianColorGroup{Query: "path:Memories", Color: colorWorkspace},
	)

	// ── Project hierarchy ─────────────────────────────────────────────────
	for i, proj := range projects {
		projColor := projectPalette[i%len(projectPalette)]

		groups = append(groups,
			// Project root index note
			obsidianColorGroup{Query: "path:" + proj + "/_index", Color: projColor},
		)

		// Symbol type groups within this project
		groups = append(groups,
			obsidianColorGroup{Query: "path:" + proj, Color: projColor},
		)
	}

	// Catch-all for all Projects
	groups = append(groups,
		obsidianColorGroup{Query: "path:Organizations", Color: projectPalette[0]},
		obsidianColorGroup{Query: "path:Projects", Color: projectPalette[0]},
	)

	// Load existing config to preserve physics settings.
	cfg := defaultGraphConfig()
	configPath := filepath.Join(obsidianDir, "graph.json")
	if existing, err := os.ReadFile(configPath); err == nil {
		_ = json.Unmarshal(existing, &cfg)
	}
	cfg.ColorGroups = groups

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

// collectWorkspaces returns workspace labels in stable alphabetical order.
func collectWorkspaces(g *types.Graph) []string {
	seen := make(map[string]bool)
	for _, n := range g.Nodes {
		if n.NodeType == string(types.NodeTypeWorkspace) {
			seen[n.Label] = true
		}
	}
	// Also collect from observation metadata (workspace nodes may be absent).
	for _, n := range g.Nodes {
		if n.NodeType == string(types.NodeTypeObservation) {
			if ws, ok := metaStr(n, "workspace"); ok && ws != "" {
				seen[ws] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for ws := range seen {
		out = append(out, ws)
	}
	sort.Strings(out)
	return out
}

// collectProjectDirs returns project note directories in stable alphabetical order.
func collectProjectDirs(g *types.Graph) []string {
	dirs := buildProjectNoteDirs(g)
	out := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		out = append(out, dir)
	}
	sort.Strings(out)
	return out
}

// defaultGraphConfig returns sensible physics defaults.
func defaultGraphConfig() obsidianGraphConfig {
	return obsidianGraphConfig{
		ShowOrphans:        true,
		TextFadeMultiplier: 0,
		NodeSizeMultiplier: 1,
		LineSizeMultiplier: 1,
		CenterStrength:     0.518713248970312,
		RepelStrength:      10,
		LinkStrength:       1,
		LinkDistance:       250,
		Scale:              0.0889892578125,
		CollapseDisplay:    true,
		CollapseForces:     true,
	}
}
