package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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

			historyDir, err := benchHistoryDir(graphFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: cannot resolve bench history dir: %v\n", err)
			}

			baselinePath := baseline
			if baselinePath == "" && historyDir != "" {
				baselinePath, err = latestBenchSnapshot(historyDir)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: cannot resolve previous bench snapshot: %v\n", err)
				}
			}

			if historyDir != "" {
				if _, err := writeBenchSnapshot(historyDir, m); err != nil {
					fmt.Fprintf(os.Stderr, "warning: cannot persist bench snapshot: %v\n", err)
				}
			}

			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(m)
			}

			printBenchReport(m, baselinePath)
			return nil
		},
	}

	cmd.Flags().StringVar(&graphFile, "graph", "", "Path to graph.json (default: ~/.vela/graph.json)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output metrics as JSON (useful for baseline comparison)")
	cmd.Flags().StringVar(&baseline, "baseline", "", "Path to a previous bench JSON to diff against")
	cmd.Flags().IntVar(&topN, "top", 10, "Number of top nodes to list")
	cmd.AddCommand(retrievalBenchCmd())
	return cmd
}

func benchHistoryDir(graphFile string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}

	absPath, err := filepath.Abs(graphFile)
	if err != nil {
		absPath = graphFile
	}

	base := filepath.Base(absPath)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = sanitizeHistoryName(base)
	if base == "" {
		base = "graph"
	}

	sum := sha1.Sum([]byte(absPath))
	hash := hex.EncodeToString(sum[:])[:10]
	return filepath.Join(home, ".vela", "bench-history", base+"-"+hash), nil
}

func sanitizeHistoryName(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func latestBenchSnapshot(historyDir string) (string, error) {
	entries, err := os.ReadDir(historyDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		files = append(files, entry.Name())
	}
	if len(files) == 0 {
		return "", nil
	}

	sort.Strings(files)
	return filepath.Join(historyDir, files[len(files)-1]), nil
}

func writeBenchSnapshot(historyDir string, m igraph.HealthMetrics) (string, error) {
	if err := os.MkdirAll(historyDir, 0755); err != nil {
		return "", err
	}

	ts := m.GeneratedAt
	if ts == "" {
		ts = time.Now().UTC().Format(time.RFC3339)
	}
	parsed, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		parsed = time.Now().UTC()
	}

	baseName := "bench-" + parsed.UTC().Format("20060102T150405Z")
	path := filepath.Join(historyDir, baseName+".json")
	for i := 1; ; i++ {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		}
		path = filepath.Join(historyDir, fmt.Sprintf("%s-%d.json", baseName, i))
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return path, nil
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
	if base != nil {
		fmt.Printf("  baseline: %s\n", baselinePath)
	}
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
