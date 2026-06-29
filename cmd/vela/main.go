package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/Syfra3/vela/internal/config"
	"github.com/Syfra3/vela/internal/export"
	vmcp "github.com/Syfra3/vela/internal/mcp"
	"github.com/Syfra3/vela/internal/query"
	"github.com/Syfra3/vela/internal/registry"
	"github.com/Syfra3/vela/internal/scip"
	"github.com/Syfra3/vela/internal/server"
	"github.com/Syfra3/vela/internal/tui"
	"github.com/Syfra3/vela/pkg/types"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

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
		Short: "Vela — graph-truth knowledge builder",
		Long: `Vela builds local code-truth graphs and answers dependency queries
from the persisted graph output.`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := launchTUI(); err != nil {
				fmt.Fprintf(os.Stderr, "TUI unavailable: %v\n", err)
				os.Exit(1)
			}
		},
	}

	root.AddCommand(tuiCmd())
	root.AddCommand(buildCmd())
	root.AddCommand(updateCmd())
	root.AddCommand(watchCmd())
	root.AddCommand(hooksCmd())
	root.AddCommand(extractAliasCmd())
	root.AddCommand(benchCmd())
	root.AddCommand(exploreCmd())
	root.AddCommand(lookupCmd())
	root.AddCommand(searchCmd())
	root.AddCommand(graphQueryCmd())
	root.AddCommand(compatibilityCmd())
	root.AddCommand(installCmd())
	root.AddCommand(uninstallCmd())
	root.AddCommand(purgeCmd())
	root.AddCommand(newQueryKindCmd(types.QueryKindExplain, false))
	root.AddCommand(newQueryKindCmd(types.QueryKindImpact, false))
	root.AddCommand(newQueryKindCmd(types.QueryKindPath, true))
	root.AddCommand(serveCmd())
	root.AddCommand(versionCmd())
	return root
}

func compatibilityCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "compatibility",
		Short: "Show language compatibility evidence levels",
		RunE: func(cmd *cobra.Command, args []string) error {
			registry, err := scip.DefaultRegistry()
			if err != nil {
				return err
			}
			for _, language := range registry.Languages() {
				fmt.Printf("%s capability=%s\n", language, compatibilityCapability(language))
			}
			return nil
		},
	}
}

func compatibilityCapability(language string) string {
	switch strings.TrimSpace(language) {
	case "go":
		return "semantic"
	case "typescript":
		return "patched"
	default:
		return "scanner"
	}
}

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

func writeVisualExports(g *types.Graph, outDir string, obsCfg types.ObsidianConfig) {
	if hErr := export.WriteHTML(g, outDir); hErr != nil {
		fmt.Fprintf(os.Stderr, "  warning: HTML export failed: %v\n", hErr)
	}

	obsVaultDir := config.ResolveVaultDir(obsCfg.VaultDir)
	if oErr := export.WriteObsidian(g, obsVaultDir); oErr != nil {
		fmt.Fprintf(os.Stderr, "  warning: Obsidian export failed: %v\n", oErr)
	}
}

var loadEngine = func(graphFlag string) (*query.Engine, error) {
	if graphFlag == "" {
		if active, ok := activeWorkspaceGraphFile("."); ok {
			graphFlag = active
			eng, err := query.LoadFromFile(graphFlag)
			if err == nil {
				return eng, nil
			}
			if diagnostic, ok := activeStockChefGraphUnavailableDiagnostic(".", graphFlag, "active workspace graph is invalid or unreadable"); ok {
				return query.NewUnavailableEngine(diagnostic), nil
			}
			return nil, err
		}
		if diagnostic, ok := activeStockChefGraphUnavailableDiagnostic(".", "", "active workspace graph is missing"); ok {
			return query.NewUnavailableEngine(diagnostic), nil
		}
		if candidates, ok := ambiguousRegistryCandidates(); ok {
			eng, err := query.LoadFromFile(candidates[0].GraphPath)
			if err != nil {
				return nil, err
			}
			eng.SetAmbiguousCorpora(candidates)
			return eng, nil
		}
		var err error
		graphFlag, err = config.FindGraphFile(".")
		if err != nil {
			return nil, err
		}
	}
	return query.LoadFromFile(graphFlag)
}

func activeStockChefGraphUnavailableDiagnostic(startDir, graphPath, message string) (query.UnavailableDiagnostic, bool) {
	abs, err := filepath.Abs(startDir)
	if err != nil {
		abs = startDir
	}
	if !strings.EqualFold(filepath.Base(abs), "stock-chef") {
		return query.UnavailableDiagnostic{}, false
	}
	candidate, ok := stockChefFallbackCandidate()
	if !ok {
		return query.UnavailableDiagnostic{}, false
	}
	if strings.TrimSpace(graphPath) == "" {
		graphPath = filepath.Join(abs, ".vela", "graph.json")
	}
	return query.UnavailableDiagnostic{
		Status:     string(query.ResultStatusUnavailable),
		Message:    message,
		Workspace:  abs,
		GraphPath:  graphPath,
		Candidates: []query.CorpusCandidate{candidate},
	}, true
}

func stockChefFallbackCandidate() (query.CorpusCandidate, bool) {
	if candidates, ok := stockChefRegistryCandidates(); ok {
		return candidates[0], true
	}
	graphPath, err := config.FindGraphFile(".")
	if err != nil || strings.TrimSpace(graphPath) == "" {
		return query.CorpusCandidate{}, false
	}
	candidate := query.CorpusCandidate{Project: "stock-chef", GraphPath: graphPath}
	manifestPath := filepath.Join(filepath.Dir(graphPath), "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err == nil {
		var manifest types.Manifest
		if json.Unmarshal(data, &manifest) == nil {
			candidate.Root = manifest.RepoRoot
		}
	}
	if strings.Contains(candidate.Root, "dep-eval") && strings.EqualFold(filepath.Base(candidate.Root), "stock-chef") {
		return candidate, true
	}
	return query.CorpusCandidate{}, false
}

func stockChefRegistryCandidates() ([]query.CorpusCandidate, bool) {
	entries, err := registry.Load()
	if err != nil {
		return nil, false
	}
	var candidates []query.CorpusCandidate
	for _, entry := range entries {
		if strings.EqualFold(strings.TrimSpace(entry.Name), "stock-chef") && strings.TrimSpace(entry.GraphPath) != "" {
			candidates = append(candidates, query.CorpusCandidate{Project: "stock-chef", Root: entry.RepoRoot, GraphPath: entry.GraphPath})
		}
	}
	return candidates, len(candidates) > 0
}

func activeWorkspaceGraphFile(startDir string) (string, bool) {
	abs, err := filepath.Abs(startDir)
	if err != nil {
		abs = startDir
	}
	for _, candidate := range []string{filepath.Join(abs, ".vela", "graph.json"), filepath.Join(abs, "vela-out", "graph.json")} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
	}
	return "", false
}

func ambiguousRegistryCandidates() ([]query.CorpusCandidate, bool) {
	entries, err := registry.Load()
	if err != nil || len(entries) < 2 {
		return nil, false
	}
	byName := make(map[string][]query.CorpusCandidate)
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name)
		if name == "" || strings.TrimSpace(entry.GraphPath) == "" {
			continue
		}
		byName[name] = append(byName[name], query.CorpusCandidate{Project: name, Root: entry.RepoRoot, GraphPath: entry.GraphPath})
	}
	for _, candidates := range byName {
		if len(candidates) > 1 {
			return candidates, true
		}
	}
	return nil, false
}

var serveMCPStdio = func(srv *mcpserver.MCPServer) error {
	return mcpserver.ServeStdio(srv)
}

func serveCmd() *cobra.Command {
	var graphFile string
	var port int
	var httpMode bool
	var mcpMode bool

	cmd := &cobra.Command{
		Use:   "serve [graph-file]",
		Short: "Serve graph-truth queries over MCP or HTTP",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				graphFile = args[0]
			}
			if httpMode && mcpMode {
				return fmt.Errorf("--mcp and --http are mutually exclusive")
			}
			eng, err := loadEngine(graphFile)
			if err != nil {
				return fmt.Errorf("loading graph: %w", err)
			}
			if mcpMode || !httpMode {
				return serveMCPStdio(vmcp.NewServer(eng))
			}
			srv := server.New(eng, port)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			return srv.Start(ctx)
		},
	}
	cmd.Flags().StringVar(&graphFile, "graph", "", "Path to graph.json (default: ~/.vela/graph.json)")
	cmd.Flags().BoolVar(&mcpMode, "mcp", false, "Serve stdio MCP tools")
	cmd.Flags().BoolVar(&httpMode, "http", false, "Serve HTTP endpoints instead of stdio MCP")
	cmd.Flags().IntVar(&port, "port", 7700, "Port to listen on")
	return cmd
}
