package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Syfra3/vela/internal/cache"
	"github.com/Syfra3/vela/internal/config"
	"github.com/Syfra3/vela/internal/detect"
	"github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/internal/extract"
	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/internal/llm"
	"github.com/Syfra3/vela/internal/query"
	"github.com/Syfra3/vela/internal/report"
	"github.com/Syfra3/vela/internal/security"
	"github.com/Syfra3/vela/internal/server"
	"github.com/Syfra3/vela/internal/tui"
	"github.com/Syfra3/vela/internal/watch"
	"github.com/Syfra3/vela/pkg/types"
)

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
	}

	root.AddCommand(extractCmd())
	root.AddCommand(configCmd())
	root.AddCommand(doctorCmd())
	root.AddCommand(pathCmd())
	root.AddCommand(explainCmd())
	root.AddCommand(queryCmd())
	root.AddCommand(serveCmd())
	root.AddCommand(hookCmd())
	return root
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

			// Discover files
			exts := []string{".go", ".py", ".ts", ".tsx", ".js", ".jsx", ".md", ".txt", ".pdf"}
			files, err := detect.Collect(root, exts)
			if err != nil {
				return fmt.Errorf("detecting files: %w", err)
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

			var allNodes []types.Node
			var allEdges []types.Edge

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

				nodes, edges, err := extract.ExtractAll(root, []string{f}, provider, cfg.LLM.MaxChunkTokens)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", rel, err)
					continue
				}

				// Mark in cache on success
				if fileCache != nil {
					if sha, shaErr := cache.SHA256File(f); shaErr == nil {
						fileCache.Mark(f, sha)
					}
				}

				allNodes = append(allNodes, nodes...)
				allEdges = append(allEdges, edges...)
				progress.ProcessedFiles++
				progress.ProcessedChunks = i + 1
			}

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

			// Export JSON
			tg := g.ToTypes()
			if err := export.WriteJSON(tg, outDir); err != nil {
				return fmt.Errorf("writing graph.json: %w", err)
			}

			// Wire query engine now that graph.json is on disk
			if qe, qErr := query.LoadFromFile(outDir + "/graph.json"); qErr == nil {
				queryEngine = qe
			}

			// GRAPH_REPORT.md
			if rErr := report.Generate(g, outDir); rErr != nil {
				fmt.Fprintf(os.Stderr, "  warning: report generation failed: %v\n", rErr)
			}

			if !noViz {
				// HTML export
				if hErr := export.WriteHTML(tg, outDir); hErr != nil {
					fmt.Fprintf(os.Stderr, "  warning: HTML export failed: %v\n", hErr)
				}
				// Obsidian vault
				if oErr := export.WriteObsidian(tg, outDir); oErr != nil {
					fmt.Fprintf(os.Stderr, "  warning: Obsidian export failed: %v\n", oErr)
				}
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
			fmt.Printf("  Nodes: %d  Edges: %d\n", len(allNodes), len(allEdges))
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

						nodes, edges, err := extract.ExtractAll(root, []string{f}, provider, cfg.LLM.MaxChunkTokens)
						if err != nil {
							fmt.Fprintf(os.Stderr, "[watch] skipping %s: %v\n", rel, err)
							continue
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

	cmd.Flags().StringVar(&outDir, "out", "vela-out", "Output directory")
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
				fmt.Printf("  [FAIL] %s provider: %v\n", cfg.LLM.Provider, err)
				os.Exit(1)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := client.Health(ctx); err != nil {
				fmt.Printf("  [UNREACHABLE] %s: %v\n", client.Provider(), err)
				os.Exit(1)
			}

			fmt.Printf("  [OK] %s\n", client.Provider())
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// Query commands
// ---------------------------------------------------------------------------

const defaultGraphFile = "vela-out/graph.json"

func loadEngine(graphFlag string) (*query.Engine, error) {
	if graphFlag == "" {
		graphFlag = defaultGraphFile
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
	cmd.Flags().StringVar(&graphFile, "graph", "", "Path to graph.json (default: vela-out/graph.json)")
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
	cmd.Flags().StringVar(&graphFile, "graph", "", "Path to graph.json (default: vela-out/graph.json)")
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
	cmd.Flags().StringVar(&graphFile, "graph", "", "Path to graph.json (default: vela-out/graph.json)")
	return cmd
}

// ---------------------------------------------------------------------------
// vela serve
// ---------------------------------------------------------------------------

func serveCmd() *cobra.Command {
	var graphFile string
	var port int

	cmd := &cobra.Command{
		Use:   "serve [graph-file]",
		Short: "Serve the knowledge graph via MCP-compatible HTTP endpoints",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				graphFile = args[0]
			}
			if graphFile == "" {
				graphFile = defaultGraphFile
			}
			eng, err := query.LoadFromFile(graphFile)
			if err != nil {
				return fmt.Errorf("loading graph: %w", err)
			}
			srv := server.New(eng, port)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			return srv.Start(ctx)
		},
	}
	cmd.Flags().StringVar(&graphFile, "graph", "", "Path to graph.json (default: vela-out/graph.json)")
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
