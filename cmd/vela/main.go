package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Syfra3/vela/internal/detect"
	"github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/internal/extract"
	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/internal/security"
	"github.com/Syfra3/vela/internal/tui"
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
	return root
}

func extractCmd() *cobra.Command {
	var outDir string
	var noTUI bool

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

			// Discover files
			exts := []string{".go", ".md", ".txt"}
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

			// Run TUI or plain logger in background
			if noTUI || !isTTY() {
				go tui.RunPlainProgress(progressCh)
			} else {
				prog := tui.NewProgram(progressCh, "none")
				if prog != nil {
					go func() { _, _ = prog.Run() }()
				} else {
					go tui.RunPlainProgress(progressCh)
				}
			}

			// Bootstrap progress
			progress := types.ExtractionProgress{
				TotalFiles:  len(files),
				TotalChunks: len(files), // 1 chunk per file in Phase 0
			}

			// Extract nodes and edges
			var allNodes []types.Node
			var allEdges []types.Edge

			for i, f := range files {
				rel := extract.RelPath(root, f)
				progress.CurrentFile = rel
				progress.CurrentChunk = i + 1
				progressCh <- types.ProgressUpdate{Progress: progress}

				nodes, edges, err := extractFile(root, f, rel)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", rel, err)
					continue
				}
				allNodes = append(allNodes, nodes...)
				allEdges = append(allEdges, edges...)

				progress.ProcessedFiles++
				progress.ProcessedChunks = i + 1
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

			// Export to JSON
			tg := g.ToTypes()
			if err := export.WriteJSON(tg, outDir); err != nil {
				return fmt.Errorf("writing graph.json: %w", err)
			}

			fmt.Printf("\nGraph written to %s/graph.json\n", outDir)
			fmt.Printf("  Nodes: %d  Edges: %d\n", len(allNodes), len(allEdges))
			return nil
		},
	}

	cmd.Flags().StringVar(&outDir, "out", "vela-out", "Output directory")
	cmd.Flags().BoolVar(&noTUI, "no-tui", false, "Disable TUI, use plain log output")
	return cmd
}

// extractFile routes a single file to the correct extractor.
func extractFile(root, path, rel string) ([]types.Node, []types.Edge, error) {
	nodes, edges, err := extract.ExtractAll(root, []string{path}, nil)
	return nodes, edges, err
}

// isTTY returns true if stdout is connected to a terminal.
func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
