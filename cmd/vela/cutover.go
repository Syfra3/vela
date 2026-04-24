package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Syfra3/vela/internal/app"
	"github.com/Syfra3/vela/internal/config"
	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/internal/hooks"
	"github.com/Syfra3/vela/internal/pipeline"
	"github.com/Syfra3/vela/internal/query"
	"github.com/Syfra3/vela/internal/registry"
	"github.com/Syfra3/vela/internal/scip"
	"github.com/Syfra3/vela/internal/watch"
	"github.com/Syfra3/vela/pkg/types"
)

type buildOutput = app.BuildResult

var runPipelineBuild = func(ctx context.Context, outDir string, req types.BuildRequest) (pipeline.Result, error) {
	registry, err := scip.DefaultRegistry()
	if err != nil {
		return pipeline.Result{}, fmt.Errorf("load SCIP registry: %w", err)
	}
	builder := pipeline.NewBuilder(pipeline.Config{Registry: registry, OutDir: outDir, Cluster: igraph.RunLeiden})
	return builder.Build(ctx, req)
}

var runBuildService = func(ctx context.Context, outDir string, req types.BuildRequest) (buildOutput, error) {
	cfg, err := config.Load()
	if err != nil {
		return buildOutput{}, err
	}
	return app.BuildService{
		RunPipeline: func(ctx context.Context, outDir string, req types.BuildRequest, _ pipeline.Observer) (pipeline.Result, error) {
			return runPipelineBuild(ctx, outDir, req)
		},
	}.Run(ctx, app.BuildRequest{
		RepoRoot:  req.RepoRoot,
		OutDir:    outDir,
		Languages: req.Languages,
		Drivers:   req.Drivers,
		Patchers:  req.Patchers,
		Obsidian:  cfg.Obsidian,
	})
}

var runWatchService = func(ctx context.Context, outDir string, req types.BuildRequest, stdout, stderr io.Writer) error {
	w, err := watch.New(req.RepoRoot, []string{".go", ".py", ".ts", ".tsx", ".js", ".jsx"}, func(changed []string) error {
		fmt.Fprintf(stdout, "changed: %d files\n", len(changed))
		result, buildErr := runBuildService(ctx, outDir, req)
		if buildErr != nil {
			fmt.Fprintf(stderr, "update failed: %v\n", buildErr)
			return buildErr
		}
		if err := registry.UpsertTrackedRepo(req.RepoRoot, result.GraphPath, result.ReportPath); err != nil {
			fmt.Fprintf(stderr, "warning: registry update failed: %v\n", err)
		}
		fmt.Fprintf(stdout, "graph: %s\n", result.GraphPath)
		return nil
	})
	if err != nil {
		return err
	}
	stop := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(stop)
	}()
	return w.Run(stop)
}

var installRepoHooks = func(repoRoot, executablePath string) error { return hooks.Install(repoRoot, executablePath) }
var uninstallRepoHooks = func(repoRoot string) error { return hooks.Uninstall(repoRoot) }
var inspectRepoHooks = func(repoRoot string) (hooks.Status, error) { return hooks.Inspect(repoRoot) }

func buildCmd() *cobra.Command {
	return newBuildCommand("build", nil, false)
}

func updateCmd() *cobra.Command {
	cmd := newBuildCommand("update", nil, false)
	cmd.Short = "Refresh graph outputs using manifest-aware build logic"
	return cmd
}

func watchCmd() *cobra.Command {
	var languages []string
	var drivers []string
	var patchers []string
	var outDir string
	cmd := &cobra.Command{
		Use:   "watch <path>",
		Short: "Watch source files and auto-refresh graph outputs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			fmt.Fprintln(cmd.OutOrStdout(), "watching for changes... press Ctrl+C to stop")
			return runWatchService(ctx, outDir, types.BuildRequest{
				RepoRoot:  args[0],
				Languages: languages,
				Drivers:   drivers,
				Patchers:  patchers,
			}, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringSliceVar(&languages, "language", nil, "Language filter for driver resolution")
	cmd.Flags().StringSliceVar(&drivers, "driver", nil, "Explicit SCIP drivers to run")
	cmd.Flags().StringSliceVar(&patchers, "patcher", nil, "Pipeline patchers to apply after drivers")
	cmd.Flags().StringVar(&outDir, "out-dir", "", "Output directory for graph.json (default: <repo>/.vela)")
	return cmd
}

func hooksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hooks",
		Short: "Manage repo-local Git hooks for graph freshness",
	}
	cmd.AddCommand(hooksInstallCmd(), hooksUninstallCmd(), hooksStatusCmd())
	return cmd
}

func hooksInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install <path>",
		Short: "Install Vela Git hooks into a repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			exePath, err := os.Executable()
			if err != nil {
				return fmt.Errorf("resolve executable path: %w", err)
			}
			if err := installRepoHooks(args[0], exePath); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "installed Vela hooks in %s\n", args[0])
			return nil
		},
	}
}

func hooksUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall <path>",
		Short: "Remove Vela-managed Git hook blocks from a repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := uninstallRepoHooks(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed Vela hooks from %s\n", args[0])
			return nil
		},
	}
}

func hooksStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <path>",
		Short: "Show whether Vela Git hooks are installed in a repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := inspectRepoHooks(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "repo: %s\n", status.RepoRoot)
			for _, hookName := range []string{"post-commit", "post-checkout"} {
				state := "missing"
				if status.Hooks[hookName] {
					state = "installed"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", hookName, state)
			}
			return nil
		},
	}
}

func extractAliasCmd() *cobra.Command {
	cmd := newBuildCommand("extract", nil, true)
	cmd.Short = "Compatibility alias for build"
	return cmd
}

func searchCmd() *cobra.Command {
	var graphFile string
	var limit int

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Route structural questions to graph-truth queries",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req, err := app.ParseSearchQuery(args[0])
			if err != nil {
				return err
			}
			if limit > 0 {
				req.Limit = limit
			}
			engine, err := loadEngine(graphFile)
			if err != nil {
				return err
			}
			result, err := engine.RunRequest(req)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), result)
			return nil
		},
	}
	cmd.Flags().StringVar(&graphFile, "graph", "", "Path to graph.json (default: ~/.vela/graph.json)")
	cmd.Flags().IntVar(&limit, "limit", types.DefaultQueryLimit, "Maximum related nodes to return")
	return cmd
}

func newBuildCommand(use string, aliases []string, alias bool) *cobra.Command {
	var languages []string
	var drivers []string
	var patchers []string
	var outDir string

	cmd := &cobra.Command{
		Use:     use + " <path>",
		Aliases: aliases,
		Short:   "Build graph-truth knowledge graph from a repository",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := runBuildService(cmd.Context(), outDir, types.BuildRequest{
				RepoRoot:  args[0],
				Languages: languages,
				Drivers:   drivers,
				Patchers:  patchers,
			})
			if err != nil {
				return err
			}
			if alias {
				fmt.Fprintf(cmd.OutOrStdout(), "extract is now an alias for build\n")
			}
			if err := registry.UpsertTrackedRepo(args[0], result.GraphPath, result.ReportPath); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: registry update failed: %v\n", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "graph: %s\n", result.GraphPath)
			fmt.Fprintf(cmd.OutOrStdout(), "html: %s\n", result.HTMLPath)
			fmt.Fprintf(cmd.OutOrStdout(), "report: %s\n", result.ReportPath)
			fmt.Fprintf(cmd.OutOrStdout(), "obsidian: %s\n", result.ObsidianPath)
			fmt.Fprintf(cmd.OutOrStdout(), "files: %d\n", result.Files)
			fmt.Fprintf(cmd.OutOrStdout(), "facts: %d\n", result.Facts)
			for _, warning := range result.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", warning)
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&languages, "language", nil, "Language filter for driver resolution")
	cmd.Flags().StringSliceVar(&drivers, "driver", nil, "Explicit SCIP drivers to run")
	cmd.Flags().StringSliceVar(&patchers, "patcher", nil, "Pipeline patchers to apply after drivers")
	cmd.Flags().StringVar(&outDir, "out-dir", "", "Output directory for graph.json (default: <repo>/.vela)")
	return cmd
}

func graphQueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Run graph-truth-only queries against graph.json",
	}
	cmd.AddCommand(
		newQueryKindCmd(types.QueryKindDependencies, false),
		newQueryKindCmd(types.QueryKindReverseDependencies, false),
		newQueryKindCmd(types.QueryKindImpact, false),
		newQueryKindCmd(types.QueryKindPath, true),
		newQueryKindCmd(types.QueryKindExplain, false),
	)
	return cmd
}

func newQueryKindCmd(kind types.QueryKind, needsTarget bool) *cobra.Command {
	var graphFile string
	var limit int
	use := string(kind) + " <subject>"
	if needsTarget {
		use += " <target>"
	}
	cmd := &cobra.Command{
		Use:   use,
		Short: fmt.Sprintf("Run %s query", strings.ReplaceAll(string(kind), "_", " ")),
		Args: func(cmd *cobra.Command, args []string) error {
			if needsTarget {
				return cobra.ExactArgs(2)(cmd, args)
			}
			return cobra.ExactArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := loadEngine(graphFile)
			if err != nil {
				return err
			}
			req := types.QueryRequest{Kind: kind, Subject: args[0], Limit: limit}
			if needsTarget {
				req.Target = args[1]
			}
			result, err := engine.RunRequest(req)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), result)
			return nil
		},
	}
	cmd.Flags().StringVar(&graphFile, "graph", "", "Path to graph.json (default: ~/.vela/graph.json)")
	cmd.Flags().IntVar(&limit, "limit", types.DefaultQueryLimit, "Maximum related nodes to return")
	return cmd
}

func newQueryOnlyMCPServer(engine *query.Engine) any {
	return nil
}
