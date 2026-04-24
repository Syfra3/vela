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

const serverInstructions = `Vela exposes read-only graph-truth dependency queries over graph.json.

Treat Vela as a structural graph tool, not as free-text or keyword search.

Rules:
- Do not send bag-of-words or full feature descriptions directly to graph query tools.
- Do not guess generic node names like movement, transaction, service, or handler unless the exact label is already known.
- For broad product questions, discover concrete files, symbols, DTOs, types, services, or modules first.
- Use graph query tools only after you have an exact subject or path endpoints.

Valid structural queries:
- who uses X / what uses X / where is X used?
- what does X depend on / dependencies of X
- impact of X / what breaks if X changes?
- path A -> B / path from A to B / how does A reach B?
- explain X

Workflow:
1. Start broad questions with discovery, not graph queries.
2. Find exact node candidates first.
3. Run dependency, reverse dependency, impact, path, or explain queries on the most specific exact label or ID.
4. If the subject is ambiguous, list candidates or ask a clarifying question instead of guessing.`

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
