package mcp

import (
	"context"
	"strings"

	"github.com/Syfra3/vela/internal/app"
	"github.com/Syfra3/vela/internal/query"
	"github.com/Syfra3/vela/pkg/types"
	markmcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const serverInstructions = `Vela exposes read-only graph-truth dependency queries.

Use the query tools to inspect dependencies, reverse dependencies, impact, paths, and explanations from graph.json.`

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
	registerQueryTool(srv, engine, "vela_dependencies", types.QueryKindDependencies, false)
	registerQueryTool(srv, engine, "vela_reverse_dependencies", types.QueryKindReverseDependencies, false)
	registerQueryTool(srv, engine, "vela_impact", types.QueryKindImpact, false)
	registerQueryTool(srv, engine, "vela_path", types.QueryKindPath, true)
	registerQueryTool(srv, engine, "vela_explain", types.QueryKindExplain, false)
}

func registerQueryTool(srv *server.MCPServer, engine *query.Engine, name string, kind types.QueryKind, needsTarget bool) {
	options := []markmcp.ToolOption{
		markmcp.WithDescription("Run a read-only graph-truth query."),
		markmcp.WithReadOnlyHintAnnotation(true),
		markmcp.WithString("subject", markmcp.Required(), markmcp.Description("Primary node label or ID")),
		markmcp.WithNumber("limit", markmcp.Description("Maximum related nodes to include")),
	}
	if needsTarget {
		options = append(options, markmcp.WithString("target", markmcp.Required(), markmcp.Description("Target node label or ID")))
	}
	srv.AddTool(markmcp.NewTool(name, options...), handleQueryTool(engine, string(kind)))
}

func handleQueryTool(engine *query.Engine, kind string) server.ToolHandlerFunc {
	return func(ctx context.Context, req markmcp.CallToolRequest) (*markmcp.CallToolResult, error) {
		_ = ctx
		queryReq, err := app.NormalizeQueryRequest(app.QueryRequestInput{
			Kind:    types.QueryKind(strings.TrimSpace(kind)),
			Subject: strings.TrimSpace(req.GetString("subject", "")),
			Target:  strings.TrimSpace(req.GetString("target", "")),
			Limit:   req.GetInt("limit", types.DefaultQueryLimit),
		})
		if err != nil {
			return markmcp.NewToolResultError(err.Error()), nil
		}
		output, err := engine.RunRequest(queryReq)
		if err != nil {
			return markmcp.NewToolResultError(err.Error()), nil
		}
		return markmcp.NewToolResultText(output), nil
	}
}
