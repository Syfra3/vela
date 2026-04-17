package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Syfra3/vela/internal/config"
	igraph "github.com/Syfra3/vela/internal/graph"
)

func benchCmd() *cobra.Command {
	var graphFile string
	var jsonOut bool
	var baseline string
	var topN int

	cmd := &cobra.Command{
		Use:   "bench",
		Short: "Compute graph health metrics (coverage, quality, structure)",
		Long: `bench reads an existing graph.json and reports a full health snapshot:
  - Coverage: node/edge counts, breakdowns by kind, confidence distribution
  - Quality:  resolution rate, broken edges, self-loops, duplicates
  - Structure: degree stats (avg/median/p95/max), hub/leaf/isolated counts
  - Communities: count, modularity, singleton communities
  - Top nodes by out-degree

Use --baseline to compare against a previous bench run (JSON output).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if graphFile == "" {
				var err error
				graphFile, err = config.FindGraphFile(".")
				if err != nil {
					return err
				}
			}

			m, err := igraph.LoadHealthMetrics(graphFile, topN)
			if err != nil {
				return fmt.Errorf("loading graph: %w", err)
			}

			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(m)
			}

			printBenchReport(m, baseline)
			return nil
		},
	}

	cmd.Flags().StringVar(&graphFile, "graph", "", "Path to graph.json (default: vela-out/graph.json)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output metrics as JSON (useful for baseline comparison)")
	cmd.Flags().StringVar(&baseline, "baseline", "", "Path to a previous bench JSON to diff against")
	cmd.Flags().IntVar(&topN, "top", 10, "Number of top nodes to list")
	return cmd
}

func printBenchReport(m igraph.HealthMetrics, baselinePath string) {
	var base *igraph.HealthMetrics
	if baselinePath != "" {
		data, err := os.ReadFile(baselinePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: cannot read baseline %s: %v\n", baselinePath, err)
		} else {
			var b igraph.HealthMetrics
			if err := json.Unmarshal(data, &b); err == nil {
				base = &b
			}
		}
	}

	h := "═"
	fmt.Println()
	fmt.Printf("  VELA GRAPH HEALTH REPORT\n")
	if m.GeneratedAt != "" {
		fmt.Printf("  generated %s\n", m.GeneratedAt)
	}
	fmt.Printf("  source: %s\n", m.Path)
	fmt.Println()

	section := func(title string) {
		fmt.Printf("  %s %s %s\n", h+h+h, title, h+h+h)
		fmt.Println()
	}

	diffInt := func(label string, cur, prev int) {
		if base != nil && cur != prev {
			arrow := "▲"
			if cur < prev {
				arrow = "▼"
			}
			fmt.Printf("  %-28s %d  %s%+d\n", label, cur, arrow, cur-prev)
		} else {
			fmt.Printf("  %-28s %d\n", label, cur)
		}
	}

	diffF := func(label string, cur, prev float64, unit string) {
		if base != nil && cur != prev {
			arrow := "▲"
			if cur < prev {
				arrow = "▼"
			}
			fmt.Printf("  %-28s %.2f%s  %s%+.2f\n", label, cur, unit, arrow, cur-prev)
		} else {
			fmt.Printf("  %-28s %.2f%s\n", label, cur, unit)
		}
	}

	// ── Coverage ────────────────────────────────────────────────────────────
	section("COVERAGE")

	var bNodes, bEdges int
	if base != nil {
		bNodes, bEdges = base.Nodes, base.Edges
	}
	diffInt("Nodes", m.Nodes, bNodes)
	diffInt("Edges", m.Edges, bEdges)
	fmt.Println()

	if len(m.NodesByKind) > 0 {
		fmt.Printf("  %-28s\n", "Nodes by kind:")
		for k, n := range m.NodesByKind {
			fmt.Printf("    %-24s %d\n", k, n)
		}
		fmt.Println()
	}
	if len(m.EdgesByRelation) > 0 {
		fmt.Printf("  %-28s\n", "Edges by relation:")
		for k, n := range m.EdgesByRelation {
			fmt.Printf("    %-24s %d\n", k, n)
		}
		fmt.Println()
	}

	// ── Quality / Resolution ────────────────────────────────────────────────
	section("QUALITY")

	bRR := 0.0
	bBE := 0
	bER := 0.0
	if base != nil {
		bRR = base.ResolutionRate
		bBE = base.BrokenEdges
		bER = base.ExtractedRate
	}
	diffF("Resolution rate", m.ResolutionRate*100, bRR*100, "%")
	diffInt("Broken edges", m.BrokenEdges, bBE)
	diffInt("Self-loops", m.SelfLoops, 0)
	diffInt("Duplicate edges", m.DuplicateEdges, 0)
	fmt.Println()

	if len(m.ConfidenceDist) > 0 {
		fmt.Printf("  %-28s\n", "Confidence distribution:")
		for k, n := range m.ConfidenceDist {
			pct := 0.0
			if m.Edges > 0 {
				pct = float64(n) * 100 / float64(m.Edges)
			}
			fmt.Printf("    %-24s %d  (%.0f%%)\n", k, n, pct)
		}
		diffF("  EXTRACTED rate", m.ExtractedRate*100, bER*100, "%")
	}
	fmt.Println()

	// ── Structure ────────────────────────────────────────────────────────────
	section("STRUCTURE")

	bAvg := 0.0
	bIso := 0
	bHub := 0
	if base != nil {
		bAvg = base.AvgDegree
		bIso = base.IsolatedNodes
		bHub = base.HubNodes
	}
	diffF("Avg degree", m.AvgDegree, bAvg, "")
	fmt.Printf("  %-28s median:%d  p95:%d  max:%d\n", "Degree distribution", m.MedianDegree, m.P95Degree, m.MaxDegree)
	diffInt("Connected nodes", 100*m.ConnectedPct/100, m.ConnectedPct) // just print pct
	fmt.Printf("  %-28s %d%%\n", "Connected", m.ConnectedPct)
	diffInt("Isolated nodes", m.IsolatedNodes, bIso)
	diffInt("Hub nodes (≥10 edges)", m.HubNodes, bHub)
	fmt.Printf("  %-28s %d\n", "Leaf nodes (1 edge)", m.LeafNodes)
	fmt.Println()

	// ── Communities ─────────────────────────────────────────────────────────
	section("COMMUNITIES")

	bComm := 0
	bMod := 0.0
	if base != nil {
		bComm = base.Communities
		bMod = base.Modularity
	}
	diffInt("Communities", m.Communities, bComm)
	fmt.Printf("  %-28s %d\n", "Largest community", m.LargestCommunitySize)
	fmt.Printf("  %-28s %d\n", "Singletons", m.SingletonCommunities)
	diffF("Modularity (Q)", m.Modularity, bMod, "")
	modNote := "— no community structure"
	if m.Modularity > 0.3 {
		modNote = "— strong community structure ✓"
	} else if m.Modularity > 0.1 {
		modNote = "— weak community structure"
	}
	fmt.Printf("    %s\n", modNote)
	fmt.Println()

	// ── Top nodes ────────────────────────────────────────────────────────────
	section("TOP NODES BY OUT-DEGREE")

	for i, n := range m.TopByOutDegree {
		shortFile := n.File
		if len(shortFile) > 30 {
			shortFile = "…" + shortFile[len(shortFile)-28:]
		}
		fmt.Printf("  %2d. %-28s [%-12s]  out:%-4d in:%-4d  %s\n",
			i+1, n.Label, n.Kind, n.OutDeg, n.InDeg, shortFile)
	}
	fmt.Println()

	fmt.Printf("  %s\n\n", "Tip: run with --json to save a baseline for future comparisons.")
}
