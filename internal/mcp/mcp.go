package mcp

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Syfra3/vela/internal/query"
	"github.com/Syfra3/vela/pkg/types"
	markmcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const serverInstructions = `Vela provides graph extraction and retrieval tools over the local knowledge graph.

Use vela_* tools for graph structure, node lookup, path finding, and graph-aware retrieval.
Do not use Vela as canonical memory storage. Memory ownership belongs elsewhere.`

// NewServer creates a stdio MCP server exposing Vela retrieval tools.
func NewServer(engine *query.Engine) *server.MCPServer {
	srv := server.NewMCPServer(
		"vela",
		"0.1.0",
		server.WithToolCapabilities(true),
		server.WithInstructions(serverInstructions),
	)

	registerTools(srv, engine)
	return srv
}

func registerTools(srv *server.MCPServer, engine *query.Engine) {
	srv.AddTool(
		markmcp.NewTool("vela_query_graph",
			markmcp.WithDescription("Search the local graph for relevant nodes and structural context."),
			markmcp.WithReadOnlyHintAnnotation(true),
			markmcp.WithString("question", markmcp.Required(), markmcp.Description("Natural language query or graph search term")),
			markmcp.WithNumber("limit", markmcp.Description("Maximum number of matching nodes to return (default: 5)")),
		),
		handleQueryGraph(engine),
	)

	srv.AddTool(
		markmcp.NewTool("vela_shortest_path",
			markmcp.WithDescription("Find the shortest path between two nodes in the graph."),
			markmcp.WithReadOnlyHintAnnotation(true),
			markmcp.WithString("source", markmcp.Required(), markmcp.Description("Source node label or ID")),
			markmcp.WithString("target", markmcp.Required(), markmcp.Description("Target node label or ID")),
		),
		handleShortestPath(engine),
	)

	srv.AddTool(
		markmcp.NewTool("vela_get_node",
			markmcp.WithDescription("Resolve a single node by ID, exact label, or fuzzy label match."),
			markmcp.WithReadOnlyHintAnnotation(true),
			markmcp.WithString("label", markmcp.Required(), markmcp.Description("Node ID or label")),
		),
		handleGetNode(engine),
	)

	srv.AddTool(
		markmcp.NewTool("vela_get_neighbors",
			markmcp.WithDescription("Return direct incoming and outgoing neighbors for a node."),
			markmcp.WithReadOnlyHintAnnotation(true),
			markmcp.WithString("label", markmcp.Required(), markmcp.Description("Node ID or label")),
		),
		handleGetNeighbors(engine),
	)

	srv.AddTool(
		markmcp.NewTool("vela_graph_stats",
			markmcp.WithDescription("Summarize graph size, node types, and confidence distribution."),
			markmcp.WithReadOnlyHintAnnotation(true),
		),
		handleGraphStats(engine),
	)

	srv.AddTool(
		markmcp.NewTool("vela_explain_graph",
			markmcp.WithDescription("Explain all edges involving a node."),
			markmcp.WithReadOnlyHintAnnotation(true),
			markmcp.WithString("label", markmcp.Required(), markmcp.Description("Node label or ID")),
		),
		handleExplainGraph(engine),
	)

	srv.AddTool(
		markmcp.NewTool("vela_federated_search",
			markmcp.WithDescription("Search across the current graph corpus, including any memory-derived nodes already present in the graph."),
			markmcp.WithReadOnlyHintAnnotation(true),
			markmcp.WithString("query", markmcp.Required(), markmcp.Description("Search query")),
			markmcp.WithNumber("limit", markmcp.Description("Maximum number of matching nodes to return (default: 5)")),
		),
		handleFederatedSearch(engine),
	)
}

func handleQueryGraph(engine *query.Engine) server.ToolHandlerFunc {
	return func(ctx context.Context, req markmcp.CallToolRequest) (*markmcp.CallToolResult, error) {
		_ = ctx
		question := strings.TrimSpace(stringArg(req, "question"))
		if question == "" {
			return errorResult("question is required"), nil
		}
		limit := intArg(req, "limit", 5)
		matches := engine.Search(question, limit)
		if len(matches) == 0 {
			return textResult(fmt.Sprintf("No graph matches found for %q.", question)), nil
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("Top matches for %q:", question))
		for _, node := range matches {
			lines = append(lines, formatNodeLine(node))
		}
		return textResult(strings.Join(lines, "\n")), nil
	}
}

func handleShortestPath(engine *query.Engine) server.ToolHandlerFunc {
	return func(ctx context.Context, req markmcp.CallToolRequest) (*markmcp.CallToolResult, error) {
		_ = ctx
		source := stringArg(req, "source")
		target := stringArg(req, "target")
		if source == "" || target == "" {
			return errorResult("source and target are required"), nil
		}
		return textResult(engine.Path(source, target)), nil
	}
}

func handleGetNode(engine *query.Engine) server.ToolHandlerFunc {
	return func(ctx context.Context, req markmcp.CallToolRequest) (*markmcp.CallToolResult, error) {
		_ = ctx
		label := stringArg(req, "label")
		if label == "" {
			return errorResult("label is required"), nil
		}
		node, ok := engine.FindNode(label)
		if !ok {
			return errorResult(fmt.Sprintf("node %q not found", label)), nil
		}
		return textResult(strings.Join([]string{
			fmt.Sprintf("Node: %s", node.Label),
			fmt.Sprintf("ID: %s", node.ID),
			fmt.Sprintf("Type: %s", emptyFallback(node.NodeType, "unknown")),
			fmt.Sprintf("Source: %s", emptyFallback(node.SourceFile, "n/a")),
			fmt.Sprintf("Description: %s", emptyFallback(node.Description, "n/a")),
		}, "\n")), nil
	}
}

func handleGetNeighbors(engine *query.Engine) server.ToolHandlerFunc {
	return func(ctx context.Context, req markmcp.CallToolRequest) (*markmcp.CallToolResult, error) {
		_ = ctx
		label := stringArg(req, "label")
		if label == "" {
			return errorResult("label is required"), nil
		}
		neighbors, err := engine.Neighbors(label)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		if len(neighbors) == 0 {
			return textResult(fmt.Sprintf("No neighbors found for %q.", label)), nil
		}
		lines := []string{fmt.Sprintf("Neighbors for %q:", label)}
		for _, neighbor := range neighbors {
			lines = append(lines, fmt.Sprintf("- %s %s --[%s]--> %s", neighbor.Direction, neighbor.Edge.Source, emptyFallback(neighbor.Edge.Relation, "related_to"), neighbor.Node.Label))
		}
		return textResult(strings.Join(lines, "\n")), nil
	}
}

func handleGraphStats(engine *query.Engine) server.ToolHandlerFunc {
	return func(ctx context.Context, req markmcp.CallToolRequest) (*markmcp.CallToolResult, error) {
		_ = ctx
		_ = req
		stats := engine.Stats()
		lines := []string{
			fmt.Sprintf("Nodes: %d", stats.NodeCount),
			fmt.Sprintf("Edges: %d", stats.EdgeCount),
			fmt.Sprintf("Communities: %d", stats.CommunityCount),
			"Node types:",
		}
		lines = append(lines, formatCounts(stats.NodeTypes)...)
		lines = append(lines, "Confidence:")
		lines = append(lines, formatCounts(stats.ConfidenceTypes)...)
		return textResult(strings.Join(lines, "\n")), nil
	}
}

func handleExplainGraph(engine *query.Engine) server.ToolHandlerFunc {
	return func(ctx context.Context, req markmcp.CallToolRequest) (*markmcp.CallToolResult, error) {
		_ = ctx
		label := stringArg(req, "label")
		if label == "" {
			return errorResult("label is required"), nil
		}
		return textResult(engine.Explain(label)), nil
	}
}

func handleFederatedSearch(engine *query.Engine) server.ToolHandlerFunc {
	return func(ctx context.Context, req markmcp.CallToolRequest) (*markmcp.CallToolResult, error) {
		_ = ctx
		queryText := strings.TrimSpace(stringArg(req, "query"))
		if queryText == "" {
			return errorResult("query is required"), nil
		}
		limit := intArg(req, "limit", 5)
		matches := engine.Search(queryText, limit)
		if len(matches) == 0 {
			return textResult(fmt.Sprintf("No federated matches found for %q.", queryText)), nil
		}
		lines := []string{fmt.Sprintf("Federated matches for %q:", queryText)}
		for _, node := range matches {
			lines = append(lines, formatNodeLine(node))
		}
		return textResult(strings.Join(lines, "\n")), nil
	}
}

func textResult(text string) *markmcp.CallToolResult {
	return markmcp.NewToolResultText(text)
}

func errorResult(text string) *markmcp.CallToolResult {
	return markmcp.NewToolResultError(text)
}

func stringArg(req markmcp.CallToolRequest, key string) string {
	return req.GetString(key, "")
}

func intArg(req markmcp.CallToolRequest, key string, defaultVal int) int {
	return req.GetInt(key, defaultVal)
}

func emptyFallback(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func formatNodeLine(node types.Node) string {
	return fmt.Sprintf("- %s [%s] file=%s", node.Label, emptyFallback(node.NodeType, "unknown"), emptyFallback(node.SourceFile, "n/a"))
}

func formatCounts(counts map[string]int) []string {
	if len(counts) == 0 {
		return []string{"- none"}
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("- %s: %d", key, counts[key]))
	}
	return lines
}
