package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Syfra3/vela/internal/app"
	"github.com/Syfra3/vela/internal/config"
	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/internal/pipeline"
	"github.com/Syfra3/vela/internal/query"
	"github.com/Syfra3/vela/internal/scip"
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

func buildCmd() *cobra.Command {
	return newBuildCommand("build", nil, false)
}

func extractAliasCmd() *cobra.Command {
	cmd := newBuildCommand("extract", nil, true)
	cmd.Short = "Compatibility alias for build"
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
			fmt.Fprintf(cmd.OutOrStdout(), "graph: %s\n", result.GraphPath)
			fmt.Fprintf(cmd.OutOrStdout(), "html: %s\n", result.HTMLPath)
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
