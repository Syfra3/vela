package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	ancoradb "github.com/Syfra3/vela/internal/ancora"
	"github.com/Syfra3/vela/internal/cache"
	"github.com/Syfra3/vela/internal/config"
	"github.com/Syfra3/vela/internal/daemon"
	"github.com/Syfra3/vela/internal/detect"
	vdoctor "github.com/Syfra3/vela/internal/doctor"
	"github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/internal/extract"
	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/internal/listener"
	"github.com/Syfra3/vela/internal/llm"
	vmcp "github.com/Syfra3/vela/internal/mcp"
	"github.com/Syfra3/vela/internal/query"
	"github.com/Syfra3/vela/internal/report"
	"github.com/Syfra3/vela/internal/retrieval"
	"github.com/Syfra3/vela/internal/security"
	"github.com/Syfra3/vela/internal/server"
	"github.com/Syfra3/vela/internal/tui"
	"github.com/Syfra3/vela/internal/watch"
	"github.com/Syfra3/vela/pkg/types"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// version is set via ldflags at build time. Falls back to "dev" for local builds.
var version = Version

func init() {
	if version != "dev" && version != "" {
		return
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = strings.TrimPrefix(info.Main.Version, "v")
	}
}

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "vela",
		Short: "Vela — Knowledge Explorer & Graph Builder",
		Long: `Vela is a Go-native, privacy-first knowledge graph builder for
codebases and technical documentation.`,
		Run: func(cmd *cobra.Command, args []string) {
			// Default: launch TUI if no subcommand
			if err := launchTUI(); err != nil {
				fmt.Fprintf(os.Stderr, "TUI unavailable: %v\n", err)
				os.Exit(1)
			}
		},
	}

	root.AddCommand(tuiCmd())
	root.AddCommand(extractCmd())
	root.AddCommand(benchCmd())
	root.AddCommand(configCmd())
	root.AddCommand(doctorCmd())
	root.AddCommand(pathCmd())
	root.AddCommand(explainCmd())
	root.AddCommand(searchCmd())
	root.AddCommand(queryCmd())
	root.AddCommand(serveCmd())
	root.AddCommand(hookCmd())
	root.AddCommand(versionCmd())
	root.AddCommand(watchCmd())
	return root
}

// ---------------------------------------------------------------------------
// vela tui
// ---------------------------------------------------------------------------

func tuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch interactive TUI menu",
		RunE: func(cmd *cobra.Command, args []string) error {
			return launchTUI()
		},
	}
}

func launchTUI() error {
	if !tui.IsTTY() {
		return fmt.Errorf("TUI requires a terminal (stdout is not a TTY)")
	}
	m := tui.NewMenuModelWithVersion(version)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// ---------------------------------------------------------------------------
// vela version
// ---------------------------------------------------------------------------

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("vela %s\n", version)
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// vela extract
// ---------------------------------------------------------------------------

func extractCmd() *cobra.Command {
	var outDir string
	var noTUI bool
	var noViz bool
	var watchMode bool
	var neo4jURL string
	var neo4jUser string
	var neo4jPass string
	var providerFlag string
	var modelFlag string

	cmd := &cobra.Command{
		Use:   "extract <path>",
		Short: "Extract a knowledge graph from a directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := args[0]

			// Security: validate the provided path
			if err := security.ValidatePath(".", root); err != nil {
				return fmt.Errorf("invalid path: %w", err)
			}

			if outDir == "" {
				outDir = config.OutDir(root)
			}

			// Load config
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			// CLI flags override config
			if providerFlag != "" {
				cfg.LLM.Provider = providerFlag
			}
			if modelFlag != "" {
				cfg.LLM.Model = modelFlag
			}

			// Build LLM provider (nil = no LLM, code-only mode)
			var provider types.LLMProvider
			if cfg.LLM.Provider != "" && cfg.LLM.Provider != "none" {
				llmClient, err := llm.NewClient(&cfg.LLM)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: LLM provider unavailable (%v) — doc extraction disabled\n", err)
				} else {
					provider = llmClient
				}
			}

			// Load cache
			fileCache, err := cache.Load(cfg.Extraction.CacheDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: cache unavailable (%v)\n", err)
				fileCache = nil
			}

			// Discover files (respects .gitignore, .velignore, and tech defaults)
			detected, err := detect.Files(root)
			if err != nil {
				return fmt.Errorf("detecting files: %w", err)
			}
			extSet := map[string]bool{
				".go": true, ".py": true, ".ts": true, ".tsx": true,
				".js": true, ".jsx": true, ".md": true, ".txt": true, ".pdf": true,
			}
			var files []string
			for _, e := range detected.Files {
				if extSet[filepath.Ext(e.AbsPath)] {
					files = append(files, e.AbsPath)
				}
			}
			if len(files) == 0 {
				fmt.Println("No files found.")
				return nil
			}
			fmt.Printf("Found %d files. Extracting...\n", len(files))

			// Set up progress channel
			progressCh := make(chan types.ProgressUpdate, 16)

			providerName := "none"
			providerOK := false
			if provider != nil {
				providerName = provider.Name()
				providerOK = true
			}

			// queryFn is injected into TUI for post-extraction interactive mode.
			// It is resolved after the graph is built; closure captures the pointer.
			var queryEngine *query.Engine
			queryFn := func(input string) string {
				if queryEngine == nil {
					return "graph not ready yet"
				}
				return queryEngine.Query(input)
			}

			if noTUI || !isTTY() {
				go tui.RunPlainProgress(progressCh)
			} else {
				prog := tui.NewProgram(progressCh, nil, providerName, providerOK, 1, queryFn)
				if prog != nil {
					go func() { _, _ = prog.Run() }()
				} else {
					go tui.RunPlainProgress(progressCh)
				}
			}

			progress := types.ExtractionProgress{
				TotalFiles:  len(files),
				TotalChunks: len(files),
				StartTime:   time.Now(),
			}

			// Detect project once — all files share the same source.
			projectSrc := extract.DetectProject(root)

			// Seed from the existing graph.json so that cached (unchanged)
			// files still contribute their data. Without this, a run where
			// all files are cache hits produces an empty graph.
			//
			// If graph.json does not exist (first run, or manually deleted),
			// reset the cache so every file is re-extracted — otherwise the
			// cache would skip all files and produce an empty graph with no
			// seed to fall back on.
			var seededNodes []types.Node
			var seededEdges []types.Edge
			if sn, se, loadErr := loadExistingGraphData(outDir + "/graph.json"); loadErr == nil {
				seededNodes = sn
				seededEdges = se
				if fileCache != nil && !graphNodesContainProject(seededNodes, projectSrc) {
					fileCache = nil
				}
			} else if fileCache != nil {
				// No existing graph — discard cache to force full re-extraction.
				fileCache = nil
			}

			var freshNodes []types.Node
			var freshEdges []types.Edge
			reextractedFiles := make(map[string]bool)
			projectEmitted := false

			for i, f := range files {
				rel := extract.RelPath(root, f)
				progress.CurrentFile = rel
				progress.CurrentChunk = i + 1
				progressCh <- types.ProgressUpdate{Progress: progress}

				// Cache check
				if fileCache != nil {
					sha, shaErr := cache.SHA256File(f)
					if shaErr == nil && fileCache.IsCached(f, sha) {
						progress.ProcessedFiles++
						progress.ProcessedChunks = i + 1
						continue
					}
				}

				reextractedFiles[rel] = true

				nodes, edges, err := extract.ExtractAll(root, []string{f}, provider, projectSrc, cfg.LLM.MaxChunkTokens)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", rel, err)
					continue
				}

				// ExtractAll always prepends the project root node.
				// Only keep it from the first successful file extraction.
				if !projectEmitted {
					projectEmitted = true
				} else if len(nodes) > 0 {
					nodes = nodes[1:] // skip duplicate project node
				}

				// Mark in cache on success
				if fileCache != nil {
					if sha, shaErr := cache.SHA256File(f); shaErr == nil {
						fileCache.Mark(f, sha)
					}
				}

				freshNodes = append(freshNodes, nodes...)
				freshEdges = append(freshEdges, edges...)
				progress.ProcessedFiles++
				progress.ProcessedChunks = i + 1
			}

			// Drop stale seeded data for re-extracted files, then merge.
			if len(reextractedFiles) > 0 {
				seededNodes = filterBySourceFile(seededNodes, reextractedFiles)
				seededEdges = filterEdgesBySourceFile(seededEdges, reextractedFiles)
			}
			allNodes := append(seededNodes, freshNodes...)
			allEdges := append(seededEdges, freshEdges...)

			contractFiles := make([]string, 0, len(detected.Files))
			for _, entry := range detected.Files {
				if extract.IsContractFile(entry.AbsPath) {
					contractFiles = append(contractFiles, entry.AbsPath)
				}
			}
			if len(contractFiles) > 0 {
				contractNodes, contractEdges, err := extract.ExtractContract(root, contractFiles, projectSrc)
				if err != nil {
					return fmt.Errorf("extracting contract artifacts: %w", err)
				}
				allNodes, allEdges = igraph.MergeContract(allNodes, allEdges, contractNodes, contractEdges)
			}
			workspace := igraph.BuildWorkspace(allNodes, allEdges, nil)
			allNodes = append(allNodes, workspace.Nodes...)
			allEdges = append(allEdges, workspace.Edges...)

			// Save cache
			if fileCache != nil {
				_ = fileCache.Save()
			}

			// Signal completion
			progress.ProcessedChunks = progress.TotalChunks
			progressCh <- types.ProgressUpdate{Progress: progress, IsComplete: true}
			close(progressCh)

			// Build graph
			g, err := igraph.Build(allNodes, allEdges)
			if err != nil {
				return fmt.Errorf("building graph: %w", err)
			}

			// Leiden clustering (best-effort — requires Python + graspologic)
			if partition, lErr := igraph.RunLeiden(g); lErr == nil {
				communities := g.ApplyCommunities(partition)
				fmt.Printf("  Communities detected: %d\n", len(communities))
			} else {
				fmt.Fprintf(os.Stderr, "  note: clustering unavailable (%v)\n", lErr)
			}

			// Acquire graph lock — prevents concurrent daemon writes.
			lockPath := filepath.Join(outDir, "graph.lock")
			gLock, lockErr := igraph.AcquireGraphLock(lockPath)
			if lockErr != nil {
				return fmt.Errorf("cannot write graph.json: %w", lockErr)
			}
			defer gLock.Release()

			// Export JSON atomically.
			tg := g.ToTypes()
			if err := export.WriteJSONAtomic(tg, outDir); err != nil {
				return fmt.Errorf("writing graph.json: %w", err)
			}

			// Wire query engine now that graph.json is on disk
			if qe, qErr := query.LoadFromFile(config.GraphFilePath(outDir)); qErr == nil {
				queryEngine = qe
			}

			// GRAPH_REPORT.md
			if rErr := report.Generate(g, outDir); rErr != nil {
				fmt.Fprintf(os.Stderr, "  warning: report generation failed: %v\n", rErr)
			}

			if !noViz {
				writeVisualExports(tg, outDir, cfg.Obsidian)
			}

			// Neo4j push (optional)
			if neo4jURL != "" {
				fmt.Printf("  Pushing to Neo4j at %s...\n", neo4jURL)
				if nErr := export.PushNeo4j(tg, neo4jURL, neo4jUser, neo4jPass); nErr != nil {
					fmt.Fprintf(os.Stderr, "  warning: Neo4j push failed: %v\n", nErr)
				} else {
					fmt.Println("  Neo4j push complete.")
				}
			}

			fmt.Printf("\nGraph written to %s/\n", outDir)
			fmt.Printf("  Nodes: %d  Edges: %d\n", g.NodeCount(), g.EdgeCount())
			fmt.Printf("  Layers: %s\n", formatLayerSummary(tg.Nodes))
			fmt.Printf("  graph.json · GRAPH_REPORT.md")
			if !noViz {
				fmt.Printf(" · graph.html · obsidian/")
			}
			fmt.Println()

			// --watch: start file watcher for incremental re-extraction
			if watchMode {
				fmt.Println("\n[watch] watching for changes (Ctrl-C to stop)...")
				codeExts := []string{".go", ".py", ".ts", ".tsx", ".js", ".jsx", ".md", ".txt"}
				stop := make(chan struct{})

				reextract := func(changed []string) error {
					for _, f := range changed {
						rel := extract.RelPath(root, f)
						fmt.Printf("[watch] re-extracting: %s\n", rel)

						// Invalidate cache for changed file
						if fileCache != nil {
							if sha, shaErr := cache.SHA256File(f); shaErr == nil {
								fileCache.Mark(f, sha+"_dirty") // force cache miss
							}
						}

						nodes, edges, err := extract.ExtractAll(root, []string{f}, provider, projectSrc, cfg.LLM.MaxChunkTokens)
						if err != nil {
							fmt.Fprintf(os.Stderr, "[watch] skipping %s: %v\n", rel, err)
							continue
						}
						// Skip duplicate project node in watch-mode incremental updates.
						if len(nodes) > 0 {
							nodes = nodes[1:]
						}
						allNodes = append(allNodes, nodes...)
						allEdges = append(allEdges, edges...)
					}

					// Rebuild and re-export
					g, err := igraph.Build(allNodes, allEdges)
					if err != nil {
						return fmt.Errorf("rebuilding graph: %w", err)
					}
					tg := g.ToTypes()
					if err := export.WriteJSON(tg, outDir); err != nil {
						return err
					}
					if !noViz {
						writeVisualExports(tg, outDir, cfg.Obsidian)
					}
					fmt.Printf("[watch] graph updated — %d nodes, %d edges\n", len(allNodes), len(allEdges))
					return nil
				}

				w, err := watch.New(root, codeExts, reextract)
				if err != nil {
					return fmt.Errorf("starting watcher: %w", err)
				}
				return w.Run(stop)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&outDir, "out", "", "Output directory (default: ~/.vela)")
	cmd.Flags().BoolVar(&noTUI, "no-tui", false, "Disable TUI, use plain log output")
	cmd.Flags().BoolVar(&noViz, "no-viz", false, "Skip HTML and Obsidian exports")
	cmd.Flags().BoolVar(&watchMode, "watch", false, "Watch for file changes and re-extract automatically")
	cmd.Flags().StringVar(&neo4jURL, "neo4j-push", "", "Push graph to Neo4j Bolt URL (e.g. bolt://localhost:7687)")
	cmd.Flags().StringVar(&neo4jUser, "neo4j-user", "neo4j", "Neo4j username")
	cmd.Flags().StringVar(&neo4jPass, "neo4j-pass", "neo4j", "Neo4j password")
	cmd.Flags().StringVar(&providerFlag, "provider", "", "LLM provider override (local|anthropic|openai|none)")
	cmd.Flags().StringVar(&modelFlag, "model", "", "LLM model override")
	return cmd
}

func writeVisualExports(g *types.Graph, outDir string, obsCfg types.ObsidianConfig) {
	if hErr := export.WriteHTML(g, outDir); hErr != nil {
		fmt.Fprintf(os.Stderr, "  warning: HTML export failed: %v\n", hErr)
	}

	obsVaultDir := config.ResolveVaultDir(obsCfg.VaultDir)
	if oErr := export.WriteObsidian(g, obsVaultDir); oErr != nil {
		fmt.Fprintf(os.Stderr, "  warning: Obsidian export failed: %v\n", oErr)
	}
}

// ---------------------------------------------------------------------------
// vela config
// ---------------------------------------------------------------------------

func configCmd() *cobra.Command {
	cfg := &cobra.Command{
		Use:   "config",
		Short: "Manage Vela configuration",
	}

	var force bool
	init_ := &cobra.Command{
		Use:   "init",
		Short: "Create default ~/.vela/config.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.WriteDefault(force)
			if err != nil {
				return err
			}
			fmt.Printf("Config written to %s\n", path)
			return nil
		},
	}
	init_.Flags().BoolVar(&force, "force", false, "Overwrite existing config")
	cfg.AddCommand(init_)
	return cfg
}

// ---------------------------------------------------------------------------
// vela doctor
// ---------------------------------------------------------------------------

func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check LLM provider health",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			client, err := llm.NewClient(&cfg.LLM)
			if err != nil {
				return fmt.Errorf("%s provider: %w", cfg.LLM.Provider, err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := client.Health(ctx); err != nil {
				return fmt.Errorf("%s: %w", client.Provider(), err)
			}

			fmt.Printf("  [OK] %s\n", client.Provider())
			hasFailure := false
			for _, step := range vdoctor.IntegrationChecks(cfg, "") {
				label := "OK"
				if step.Status == vdoctor.StepWarn {
					label = "WARN"
				}
				if step.Status == vdoctor.StepFail {
					label = "FAIL"
					hasFailure = true
				}
				fmt.Printf("  [%s] %s: %s\n", label, step.Name, step.Detail)
			}
			if hasFailure {
				return fmt.Errorf("doctor checks failed")
			}
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// Query commands
// ---------------------------------------------------------------------------

func loadEngine(graphFlag string) (*query.Engine, error) {
	if graphFlag == "" {
		var err error
		graphFlag, err = config.FindGraphFile(".")
		if err != nil {
			return nil, err
		}
	}
	return query.LoadFromFile(graphFlag)
}

func pathCmd() *cobra.Command {
	var graphFile string
	cmd := &cobra.Command{
		Use:   "path <from> <to>",
		Short: "Find the shortest dependency path between two nodes",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := loadEngine(graphFile)
			if err != nil {
				return err
			}
			fmt.Println(eng.Path(args[0], args[1]))
			return nil
		},
	}
	cmd.Flags().StringVar(&graphFile, "graph", "", "Path to graph.json (default: ~/.vela/graph.json)")
	return cmd
}

func explainCmd() *cobra.Command {
	var graphFile string
	cmd := &cobra.Command{
		Use:   "explain <node>",
		Short: "List all edges involving a node",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := loadEngine(graphFile)
			if err != nil {
				return err
			}
			fmt.Println(eng.Explain(strings.Join(args, " ")))
			return nil
		},
	}
	cmd.Flags().StringVar(&graphFile, "graph", "", "Path to graph.json (default: ~/.vela/graph.json)")
	return cmd
}

func searchCmd() *cobra.Command {
	var graphFile string
	var ancoraDB string
	var limit int
	var maxHops int
	var maxExpansions int
	var relationFilter string
	var jsonOut bool
	var showMetrics bool
	var recordMetrics bool

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Run federated retrieval against Ancora and Vela's SQLite index",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := loadEngine(graphFile)
			if err != nil {
				return err
			}
			if ancoraDB == "" {
				ancoraDB, err = ancoradb.DefaultDBPath()
				if err != nil {
					return err
				}
			}
			searcher := query.NewSearcher(eng, ancoraDB).WithTraversal(retrieval.TraversalOptions{
				MaxHops:          maxHops,
				MaxExpansions:    maxExpansions,
				AllowedRelations: splitCSV(relationFilter),
			})
			resp, err := searcher.Search(strings.Join(args, " "), limit)
			if err != nil {
				return err
			}

			var metricsPath string
			if recordMetrics {
				metricsPath, err = query.WriteMetricsSnapshot(resp.Metrics)
				if err != nil {
					return err
				}
			}

			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(resp); err != nil {
					return err
				}
				if metricsPath != "" {
					fmt.Fprintf(os.Stderr, "metrics snapshot: %s\n", metricsPath)
				}
				return nil
			}

			printSearchResponse(resp, showMetrics)
			if metricsPath != "" {
				fmt.Printf("\nmetrics snapshot: %s\n", metricsPath)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&graphFile, "graph", "", "Path to graph.json (default: ~/.vela/graph.json)")
	cmd.Flags().StringVar(&ancoraDB, "ancora-db", "", "Path to ancora.db (default: ~/.ancora/ancora.db)")
	cmd.Flags().IntVar(&limit, "limit", 5, "Maximum results to return")
	cmd.Flags().IntVar(&maxHops, "max-hops", 2, "Maximum graph traversal hops for structural retrieval")
	cmd.Flags().IntVar(&maxExpansions, "max-expansions", 24, "Maximum graph expansions for structural retrieval")
	cmd.Flags().StringVar(&relationFilter, "relations", "", "Comma-separated edge relations to allow during graph traversal")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output the federated response as JSON")
	cmd.Flags().BoolVar(&showMetrics, "metrics", false, "Print comparison metrics after results")
	cmd.Flags().BoolVar(&recordMetrics, "record-metrics", false, "Persist comparison metrics under ~/.vela/retrieval-history")
	return cmd
}

func queryCmd() *cobra.Command {
	var graphFile string
	cmd := &cobra.Command{
		Use:   "query <command>",
		Short: "Run a graph query (path, explain, nodes, edges)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := loadEngine(graphFile)
			if err != nil {
				return err
			}
			fmt.Println(eng.Query(strings.Join(args, " ")))
			return nil
		},
	}
	cmd.Flags().StringVar(&graphFile, "graph", "", "Path to graph.json (default: ~/.vela/graph.json)")
	return cmd
}

func printSearchResponse(resp query.SearchResponse, showMetrics bool) {
	writeSearchResponse(os.Stdout, resp, showMetrics)
}

func writeSearchResponse(w io.Writer, resp query.SearchResponse, showMetrics bool) {
	if len(resp.Routing.RoutedRepos) > 0 {
		fmt.Fprintln(w, "Routing")
		for _, route := range resp.Routing.RoutedRepos {
			line := fmt.Sprintf("  %s score=%.2f", route.Repo, route.Score)
			if len(route.Reasons) > 0 {
				line += " reasons=" + strings.Join(route.Reasons, ",")
			}
			fmt.Fprintln(w, line)
		}
		fmt.Fprintln(w)
	}

	if len(resp.Hits) == 0 {
		fmt.Fprintln(w, "no results")
	} else {
		for i, hit := range resp.Hits {
			fmt.Fprintf(w, "%d. [%s/%s] %s", i+1, hit.PrimaryLayer, hit.PrimarySource, hit.Label)
			if hit.Kind != "" {
				fmt.Fprintf(w, " (%s)", hit.Kind)
			}
			fmt.Fprintf(w, " score=%.2f\n", hit.Score)
			if hit.Path != "" {
				fmt.Fprintf(w, "   path: %s\n", hit.Path)
			}
			if len(hit.Layers) > 0 {
				fmt.Fprintf(w, "   layers: %s\n", strings.Join(hit.Layers, ", "))
			}
			if hit.Snippet != "" {
				fmt.Fprintf(w, "   %s\n", hit.Snippet)
			}
			if len(hit.Signals) > 1 {
				fmt.Fprintf(w, "   signals: %s\n", strings.Join(sortedSignalNames(hit.Signals), ", "))
			}
			if len(hit.Provenance) > 0 {
				fmt.Fprintf(w, "   evidence: %s\n", formatProvenance(hit.Provenance))
			}
			if hit.SupportGraph != nil {
				fmt.Fprintf(w, "   support-graph: %d nodes, %d edges\n", len(hit.SupportGraph.Nodes), len(hit.SupportGraph.Edges))
			}
			if len(hit.Support) > 0 {
				fmt.Fprintf(w, "   context: %s\n", hit.Support[0])
			}
		}
	}

	if !showMetrics {
		return
	}

	m := resp.Metrics
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Metrics")
	fmt.Fprintf(w, "  Ancora-only: latency=%dms returned=%d distinct_kinds=%d\n", m.AncoraOnly.LatencyMs, m.AncoraOnly.Returned, m.AncoraOnly.DistinctKinds)
	fmt.Fprintf(w, "  Federated:   latency=%dms returned=%d distinct_kinds=%d hybrid=%d\n", m.Federated.LatencyMs, m.Federated.Returned, m.Federated.DistinctKinds, m.Federated.HybridResults)
	if len(m.Federated.SignalContribution) > 0 {
		fmt.Fprintf(w, "  Signals:     %v\n", m.Federated.SignalContribution)
	}
	if m.VectorRuntime != nil {
		fmt.Fprintf(w, "  Vector:      %s/%s via %s (%s, requested=%s, sqlite-vec=%t, dims=%d)\n", m.VectorRuntime.Provider, m.VectorRuntime.Model, m.VectorRuntime.SearchMode, m.VectorRuntime.IndexBackend, m.VectorRuntime.RequestedBackend, m.VectorRuntime.SQLiteVecEnabled, m.VectorRuntime.EmbeddingDims)
		if m.VectorRuntime.SQLiteVecReason != "" {
			fmt.Fprintf(w, "  Vec note:    %s\n", m.VectorRuntime.SQLiteVecReason)
		}
	}
	fmt.Fprintf(w, "  Overlap@%d:  %d (%.2f)\n", m.Limit, m.Comparison.OverlapAtK, m.Comparison.OverlapRatio)
	fmt.Fprintf(w, "  Added:       %d %v\n", m.Comparison.AddedByFederated, m.Comparison.AddedBySource)
	fmt.Fprintf(w, "  Graph cost:  %dms\n", m.Comparison.GraphLatencyMs)
}

func formatProvenance(values []query.SearchProvenance) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		part := value.Layer
		if value.Signal != "" {
			part += "/" + value.Signal
		}
		if value.Source != "" {
			part += " via " + value.Source
		}
		if value.Repo != "" {
			part += " repo=" + value.Repo
		}
		if len(value.Reasons) > 0 {
			part += " reasons=" + strings.Join(value.Reasons, ",")
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "; ")
}

func formatLayerSummary(nodes []types.Node) string {
	counts := map[string]int{}
	for _, node := range nodes {
		layer := "repo"
		if key := types.CanonicalKeyForNode(node); !key.IsZero() && key.Layer != "" {
			layer = string(key.Layer)
		} else if node.Source != nil && types.LayerOf(node.Source.Type) != "" {
			layer = string(types.LayerOf(node.Source.Type))
		}
		counts[layer]++
	}
	order := []string{string(types.LayerRepo), string(types.LayerContract), string(types.LayerWorkspace), string(types.LayerMemory)}
	parts := make([]string, 0, len(order))
	for _, layer := range order {
		if counts[layer] == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%d", layer, counts[layer]))
	}
	return strings.Join(parts, ", ")
}

func sortedSignalNames(signals map[string]float64) []string {
	names := make([]string, 0, len(signals))
	for signal := range signals {
		names = append(names, signal)
	}
	sort.Strings(names)
	return names
}

func splitCSV(input string) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}
	parts := strings.Split(input, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		values = append(values, part)
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

// ---------------------------------------------------------------------------
// vela serve
// ---------------------------------------------------------------------------

func serveCmd() *cobra.Command {
	var graphFile string
	var ancoraDB string
	var port int
	var httpMode bool

	cmd := &cobra.Command{
		Use:   "serve [graph-file]",
		Short: "Serve the knowledge graph via MCP stdio tools",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				graphFile = args[0]
			}
			if graphFile == "" {
				var err error
				graphFile, err = config.FindGraphFile(".")
				if err != nil {
					return err
				}
			}
			eng, err := query.LoadFromFile(graphFile)
			if err != nil {
				return fmt.Errorf("loading graph: %w", err)
			}
			if !httpMode {
				return mcpserver.ServeStdio(vmcp.NewServer(eng))
			}
			srv := server.New(eng, port)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			return srv.Start(ctx)
		},
	}
	cmd.Flags().StringVar(&graphFile, "graph", "", "Path to graph.json (default: ~/.vela/graph.json)")
	cmd.Flags().StringVar(&ancoraDB, "ancora-db", "", "Path to the Ancora database for hybrid HTTP search")
	cmd.Flags().BoolVar(&httpMode, "http", false, "Serve legacy HTTP endpoints instead of stdio MCP")
	cmd.Flags().IntVar(&port, "port", 7700, "Port to listen on")
	return cmd
}

// ---------------------------------------------------------------------------
// vela hook
// ---------------------------------------------------------------------------

func hookCmd() *cobra.Command {
	hook := &cobra.Command{
		Use:   "hook",
		Short: "Manage git hooks",
	}
	hook.AddCommand(hookInstallCmd())
	return hook
}

func hookInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install a post-commit hook that rebuilds the graph on each commit",
		RunE: func(cmd *cobra.Command, args []string) error {
			return installHook()
		},
	}
}

func installHook() error {
	const hookScript = `#!/bin/sh
# Vela post-commit hook — auto-regenerate knowledge graph
vela extract . --no-tui --no-viz --provider none 2>/dev/null || true
`
	hookDir := ".git/hooks"
	if _, err := os.Stat(hookDir); os.IsNotExist(err) {
		return fmt.Errorf(".git/hooks not found — run from the root of a git repository")
	}
	hookPath := hookDir + "/post-commit"
	if err := os.WriteFile(hookPath, []byte(hookScript), 0755); err != nil {
		return fmt.Errorf("writing hook: %w", err)
	}
	fmt.Printf("Hook installed at %s\n", hookPath)
	return nil
}

// ---------------------------------------------------------------------------
// vela watch
// ---------------------------------------------------------------------------

// watchCmd builds the `vela watch` command tree.
//
// Usage:
//
//	vela watch start              Start daemon in background
//	vela watch stop               Stop running daemon
//	vela watch status             Show daemon status + connected sources
//	vela watch logs               Tail daemon log file
//	vela watch add ancora         Add Ancora as event source
//	vela watch add <name> <sock>  Add custom source
//	vela watch remove <name>      Remove source
//	vela watch list               List configured sources
//	vela watch install            Install as system service
//	vela watch uninstall          Remove system service
func watchCmd() *cobra.Command {
	w := &cobra.Command{
		Use:   "watch",
		Short: "Manage the real-time watch daemon (Ancora integration)",
	}

	w.AddCommand(watchStartCmd())
	w.AddCommand(watchStopCmd())
	w.AddCommand(watchStatusCmd())
	w.AddCommand(watchLogsCmd())
	w.AddCommand(watchAddCmd())
	w.AddCommand(watchRemoveCmd())
	w.AddCommand(watchListCmd())
	w.AddCommand(watchInstallCmd())
	w.AddCommand(watchUninstallCmd())
	return w
}

func loadDaemon(graphFile string) (*daemon.Daemon, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	if graphFile == "" {
		var findErr error
		graphFile, findErr = config.FindGraphFile(".")
		if findErr != nil {
			// No existing graph.json — resolve canonical path so persist loop can write there.
			home, _ := os.UserHomeDir()
			graphFile = filepath.Join(home, ".vela", "graph.json")
		}
	}
	// Load the existing graph so the daemon can patch it in-place.
	nodes, edges, err := loadExistingGraphData(graphFile)
	if err != nil {
		// Start with an empty graph if none exists yet.
		nodes = nil
		edges = nil
	}
	g, err := igraph.Build(nodes, edges)
	if err != nil {
		return nil, fmt.Errorf("building graph: %w", err)
	}
	d, err := daemon.New(cfg, g)
	if err != nil {
		return nil, fmt.Errorf("building daemon: %w", err)
	}
	// Register the config loader so the daemon can hot-reload on SIGHUP.
	daemon.SetConfigLoader(config.Load)
	// Tell the daemon where to persist graph.json.
	d.SetGraphPath(graphFile)
	return d, nil
}

func watchStartCmd() *cobra.Command {
	var graphFile string
	var foreground bool

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the watch daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			if foreground {
				// Foreground mode: run the daemon in this process (blocks until Ctrl-C).
				d, err := loadDaemon(graphFile)
				if err != nil {
					return err
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				if err := d.Start(ctx); err != nil {
					return err
				}

				fmt.Println("Watch daemon running in foreground (Ctrl-C to stop)")
				<-ctx.Done()
				return d.Stop()
			}

			// Background mode: re-exec this binary with --foreground as a detached process.
			// This is the standard Unix daemonization pattern — the parent returns immediately,
			// the child keeps running in the background.
			self, err := os.Executable()
			if err != nil {
				return fmt.Errorf("resolving executable: %w", err)
			}

			fwdArgs := []string{"watch", "start", "--foreground"}
			if graphFile != "" {
				fwdArgs = append(fwdArgs, "--graph", graphFile)
			}

			child := exec.Command(self, fwdArgs...)
			child.Stdout = nil
			child.Stderr = nil
			child.Stdin = nil
			if err := child.Start(); err != nil {
				return fmt.Errorf("starting background daemon: %w", err)
			}

			// Detach: do not wait for child. It owns itself now.
			_ = child.Process.Release()

			// Give the child a moment to write its PID and connect sources,
			// then query status from our side (reads PID file + registry).
			// We use a fresh daemon instance purely for the Status() call.
			// Brief pause so child has time to write PID file.
			time.Sleep(200 * time.Millisecond)

			d, err := loadDaemon(graphFile)
			fmt.Println("Watch daemon started.")
			if err == nil {
				fmt.Printf("Status: %s\n", d.Status())
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&graphFile, "graph", "", "Path to graph.json (default: ~/.vela/graph.json)")
	cmd.Flags().BoolVar(&foreground, "foreground", false, "Run in foreground (for service managers)")
	return cmd
}

func watchStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the running watch daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := loadDaemon("")
			if err != nil {
				return err
			}
			if err := d.Stop(); err != nil {
				return err
			}
			fmt.Println("Watch daemon stopped.")
			return nil
		},
	}
}

func watchStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show watch daemon status and connected sources",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := loadDaemon("")
			if err != nil {
				return err
			}
			fmt.Println(d.Status())

			// Show per-source status if running.
			for name, s := range d.Registry().Statuses() {
				dot := "○"
				if s.Connected {
					dot = "●"
				}
				errStr := ""
				if s.LastError != nil {
					errStr = fmt.Sprintf(" [err: %v]", s.LastError)
				}
				fmt.Printf("  %s %-12s  events: %d%s\n", dot, name, s.EventCount, errStr)
			}
			return nil
		},
	}
}

func watchLogsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logs",
		Short: "Display the daemon log file path",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.Daemon.LogFile == "" {
				fmt.Println("No log file configured.")
				return nil
			}
			fmt.Printf("Log file: %s\n", cfg.Daemon.LogFile)
			fmt.Printf("Run: tail -f %s\n", cfg.Daemon.LogFile)
			return nil
		},
	}
}

func watchAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <name> [socket]",
		Short: "Add an event source (use 'ancora' for the default Ancora socket)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			socketPath := ""
			if len(args) >= 2 {
				socketPath = args[1]
			}
			_ = listener.NewAncoraListener(socketPath, "")
			// TODO: persist source config to ~/.vela/config.yaml
			fmt.Printf("Source '%s' added (socket: %s)\n", name, socketPath)
			fmt.Println("Restart the daemon for changes to take effect: vela watch stop && vela watch start")
			return nil
		},
	}
}

func watchRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a configured event source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: remove from ~/.vela/config.yaml
			fmt.Printf("Source '%s' removed.\n", args[0])
			fmt.Println("Restart the daemon for changes to take effect.")
			return nil
		},
	}
}

func watchListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured event sources",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if len(cfg.Watch.Sources) == 0 {
				fmt.Println("No sources configured.")
				return nil
			}
			for _, src := range cfg.Watch.Sources {
				fmt.Printf("  %-12s type=%-8s socket=%s\n", src.Name, src.Type, src.Socket)
			}
			return nil
		},
	}
}

func watchInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install the watch daemon as a system service (systemd/launchd)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return daemon.InstallService(cfg.Daemon.PIDFile, cfg.Daemon.LogFile)
		},
	}
}

func watchUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the system service",
		RunE: func(cmd *cobra.Command, args []string) error {
			return daemon.UninstallService()
		},
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// isTTY returns true if stdout is connected to a terminal.
func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// loadExistingGraphData reads a previously-written graph.json and returns its
// nodes and edges. graph.json uses "file" (not "source_file") for the export
// format — see internal/export/json.go. Returns an error when no graph exists.
func loadExistingGraphData(path string) ([]types.Node, []types.Edge, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var raw struct {
		Nodes []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
			Kind  string `json:"kind"`
			File  string `json:"file"`
		} `json:"nodes"`
		Edges []struct {
			From string `json:"from"`
			To   string `json:"to"`
			Kind string `json:"kind"`
			File string `json:"file"`
		} `json:"edges"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, err
	}
	nodes := make([]types.Node, len(raw.Nodes))
	for i, n := range raw.Nodes {
		nodes[i] = types.Node{ID: n.ID, Label: n.Label, NodeType: n.Kind, SourceFile: n.File}
	}
	edges := make([]types.Edge, len(raw.Edges))
	for i, e := range raw.Edges {
		edges[i] = types.Edge{Source: e.From, Target: e.To, Relation: e.Kind, SourceFile: e.File}
	}
	return nodes, edges, nil
}

// filterBySourceFile removes nodes whose SourceFile is present in reextracted.
func filterBySourceFile(nodes []types.Node, reextracted map[string]bool) []types.Node {
	out := nodes[:0]
	for _, n := range nodes {
		if !reextracted[n.SourceFile] {
			out = append(out, n)
		}
	}
	return out
}

// filterEdgesBySourceFile removes edges whose SourceFile is present in reextracted.
func filterEdgesBySourceFile(edges []types.Edge, reextracted map[string]bool) []types.Edge {
	out := edges[:0]
	for _, e := range edges {
		if !reextracted[e.SourceFile] {
			out = append(out, e)
		}
	}
	return out
}

func graphNodesContainProject(nodes []types.Node, src *types.Source) bool {
	if src == nil {
		return false
	}
	projectID := extract.ProjectNodeID(src.Name)
	for _, node := range nodes {
		if node.NodeType == string(types.NodeTypeProject) && node.ID == projectID {
			return true
		}
	}
	return false
}
