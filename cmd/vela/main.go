package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/Syfra3/vela/internal/config"
	"github.com/Syfra3/vela/internal/export"
	vmcp "github.com/Syfra3/vela/internal/mcp"
	"github.com/Syfra3/vela/internal/query"
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
	root.AddCommand(searchCmd())
	root.AddCommand(graphQueryCmd())
	root.AddCommand(serveCmd())
	root.AddCommand(versionCmd())
	return root
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
		var err error
		graphFlag, err = config.FindGraphFile(".")
		if err != nil {
			return nil, err
		}
	}
	return query.LoadFromFile(graphFlag)
}

func serveCmd() *cobra.Command {
	var graphFile string
	var port int
	var httpMode bool

	cmd := &cobra.Command{
		Use:   "serve [graph-file]",
		Short: "Serve graph-truth queries over MCP or HTTP",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				graphFile = args[0]
			}
			eng, err := loadEngine(graphFile)
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
	cmd.Flags().BoolVar(&httpMode, "http", false, "Serve HTTP endpoints instead of stdio MCP")
	cmd.Flags().IntVar(&port, "port", 7700, "Port to listen on")
	return cmd
}
