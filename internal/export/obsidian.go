package export

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Syfra3/vela/pkg/types"
)

// WriteObsidian creates an Obsidian vault at outDir/obsidian/.
// Each node becomes a markdown file with frontmatter and wikilinks to targets.
func WriteObsidian(g *types.Graph, outDir string) error {
	vaultDir := filepath.Join(outDir, "obsidian")
	if err := os.MkdirAll(vaultDir, 0755); err != nil {
		return fmt.Errorf("creating obsidian vault dir: %w", err)
	}

	// Build outgoing edge index: source node ID → []target labels
	outEdges := make(map[string][]string, len(g.Nodes))
	for _, e := range g.Edges {
		outEdges[e.Source] = append(outEdges[e.Source], e.Target)
	}

	for _, n := range g.Nodes {
		if err := writeObsidianNote(vaultDir, n, outEdges[n.ID]); err != nil {
			return fmt.Errorf("writing note for %s: %w", n.Label, err)
		}
	}

	return nil
}

// writeObsidianNote writes a single node's markdown file.
func writeObsidianNote(vaultDir string, n types.Node, targets []string) error {
	// Sanitize filename: replace path separators and problematic chars
	safeName := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_").Replace(n.Label)
	if safeName == "" {
		safeName = n.ID
	}
	notePath := filepath.Join(vaultDir, safeName+".md")

	var sb strings.Builder

	// YAML frontmatter
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %q\n", n.ID))
	sb.WriteString(fmt.Sprintf("kind: %q\n", n.NodeType))
	sb.WriteString(fmt.Sprintf("file: %q\n", n.SourceFile))
	sb.WriteString(fmt.Sprintf("community: %d\n", n.Community))
	if n.Degree > 0 {
		sb.WriteString(fmt.Sprintf("degree: %d\n", n.Degree))
	}
	sb.WriteString("---\n\n")

	// Title
	sb.WriteString(fmt.Sprintf("# %s\n\n", n.Label))

	// Description
	if n.Description != "" {
		sb.WriteString(n.Description + "\n\n")
	}

	// Outgoing links
	if len(targets) > 0 {
		sb.WriteString("## Links\n\n")
		seen := make(map[string]bool)
		for _, t := range targets {
			if seen[t] {
				continue
			}
			seen[t] = true
			// Use wikilink syntax
			sb.WriteString(fmt.Sprintf("- [[%s]]\n", t))
		}
		sb.WriteString("\n")
	}

	return os.WriteFile(notePath, []byte(sb.String()), 0644)
}
