package export

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Syfra3/vela/pkg/types"
)

// WriteHTML generates an interactive vis.js Network graph as a self-contained
// HTML file at outDir/graph.html. Nodes are coloured by community ID; god
// nodes (high degree) are rendered larger.
func WriteHTML(g *types.Graph, outDir string) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	// Compute max degree for scaling node sizes
	degreeOf := make(map[string]int, len(g.Nodes))
	for _, e := range g.Edges {
		degreeOf[e.Source]++
		degreeOf[e.Target]++
	}
	maxDeg := 1
	for _, d := range degreeOf {
		if d > maxDeg {
			maxDeg = d
		}
	}

	// Palette: 12 distinct community colours
	palette := []string{
		"#4e79a7", "#f28e2b", "#e15759", "#76b7b2", "#59a14f",
		"#edc948", "#b07aa1", "#ff9da7", "#9c755f", "#bab0ac",
		"#d37295", "#fabfd2",
	}

	type visNode struct {
		ID    string `json:"id"`
		Label string `json:"label"`
		Title string `json:"title"` // tooltip
		Color string `json:"color"`
		Size  int    `json:"size"`
		Group int    `json:"group"`
	}
	type visEdge struct {
		From   string `json:"from"`
		To     string `json:"to"`
		Title  string `json:"title"`
		Arrows string `json:"arrows"`
	}

	visNodes := make([]visNode, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		deg := degreeOf[n.ID]
		// Size 10–40 scaled by degree
		size := 10
		if maxDeg > 0 {
			size = 10 + int(30.0*float64(deg)/float64(maxDeg))
		}
		color := palette[n.Community%len(palette)]
		visNodes = append(visNodes, visNode{
			ID:    n.ID,
			Label: n.Label,
			Title: fmt.Sprintf("%s | kind:%s | file:%s | degree:%d | community:%d",
				n.Label, n.NodeType, n.SourceFile, deg, n.Community),
			Color: color,
			Size:  size,
			Group: n.Community,
		})
	}

	visEdges := make([]visEdge, 0, len(g.Edges))
	for _, e := range g.Edges {
		visEdges = append(visEdges, visEdge{
			From:   e.Source,
			To:     e.Target,
			Title:  e.Relation,
			Arrows: "to",
		})
	}

	nodesJSON, err := json.Marshal(visNodes)
	if err != nil {
		return err
	}
	edgesJSON, err := json.Marshal(visEdges)
	if err != nil {
		return err
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Vela Knowledge Graph</title>
  <script src="https://unpkg.com/vis-network/standalone/umd/vis-network.min.js"></script>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body { font-family: sans-serif; background: #1a1a2e; color: #eee; }
    #header { padding: 12px 20px; background: #16213e; border-bottom: 1px solid #0f3460; }
    #header h1 { font-size: 1.1rem; font-weight: 600; }
    #header p  { font-size: 0.8rem; color: #888; margin-top: 2px; }
    #network { width: 100%%; height: calc(100vh - 56px); }
    #tooltip {
      position: fixed; padding: 8px 12px; background: rgba(0,0,0,0.85);
      border: 1px solid #333; border-radius: 4px; font-size: 0.75rem;
      pointer-events: none; display: none; z-index: 999; max-width: 320px;
    }
  </style>
</head>
<body>
  <div id="header">
    <h1>Vela Knowledge Graph</h1>
    <p>%d nodes &middot; %d edges &middot; nodes coloured by community</p>
  </div>
  <div id="network"></div>
  <div id="tooltip"></div>
  <script>
    const nodes = new vis.DataSet(%s);
    const edges = new vis.DataSet(%s);
    const container = document.getElementById('network');
    const data = { nodes, edges };
    const options = {
      nodes: {
        shape: 'dot',
        font: { color: '#eee', size: 11 },
        borderWidth: 1,
        borderWidthSelected: 3,
      },
      edges: {
        color: { color: '#555', highlight: '#aaa' },
        smooth: { type: 'dynamic' },
        width: 1,
      },
      physics: {
        stabilization: { iterations: 200 },
        barnesHut: { gravitationalConstant: -8000, springLength: 120 },
      },
      interaction: { hover: true, tooltipDelay: 100 },
    };
    const network = new vis.Network(container, data, options);

    const tooltip = document.getElementById('tooltip');
    network.on('hoverNode', params => {
      const node = nodes.get(params.node);
      tooltip.textContent = node.title;
      tooltip.style.display = 'block';
    });
    network.on('blurNode', () => { tooltip.style.display = 'none'; });
    document.addEventListener('mousemove', e => {
      tooltip.style.left = (e.clientX + 14) + 'px';
      tooltip.style.top  = (e.clientY + 14) + 'px';
    });
  </script>
</body>
</html>
`, len(g.Nodes), len(g.Edges), nodesJSON, edgesJSON)

	outPath := filepath.Join(outDir, "graph.html")
	return os.WriteFile(outPath, []byte(html), 0644)
}
