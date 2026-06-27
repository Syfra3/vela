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
	registerExploreTool(srv, engine)
	registerLookupTool(srv, engine)
	registerQueryTool(srv, engine, "vela_dependencies", types.QueryKindDependencies, false)
	registerQueryTool(srv, engine, "vela_reverse_dependencies", types.QueryKindReverseDependencies, false)
	registerQueryTool(srv, engine, "vela_impact", types.QueryKindImpact, false)
	registerQueryTool(srv, engine, "vela_path", types.QueryKindPath, true)
	registerQueryTool(srv, engine, "vela_explain", types.QueryKindExplain, false)
	registerStatusTool(srv, engine)
}

func registerExploreTool(srv *server.MCPServer, engine *query.Engine) {
	srv.AddTool(markmcp.NewTool("vela_explore",
		markmcp.WithDescription("Resolve broad graph context requests into graph-backed candidates."),
		markmcp.WithReadOnlyHintAnnotation(true),
		markmcp.WithString("query", markmcp.Required(), markmcp.Description("Natural-language request to resolve")),
		markmcp.WithNumber("limit", markmcp.Description("Maximum candidates to include")),
	), func(ctx context.Context, req markmcp.CallToolRequest) (*markmcp.CallToolResult, error) {
		return handleExploreTool(engine)(ctx, req)
	})
}

func handleExploreTool(engine *query.Engine) server.ToolHandlerFunc {
	return func(ctx context.Context, req markmcp.CallToolRequest) (*markmcp.CallToolResult, error) {
		_ = ctx
		input := strings.TrimSpace(req.GetString("query", ""))
		if input == "" {
			return markmcp.NewToolResultStructuredOnly(engine.DiagnosticResult("explore", "VALIDATION_ERROR", "query is required")), nil
		}
		return markmcp.NewToolResultStructuredOnly(engine.ExploreResult(input, req.GetInt("limit", types.DefaultQueryLimit))), nil
	}
}

func registerLookupTool(srv *server.MCPServer, engine *query.Engine) {
	srv.AddTool(markmcp.NewTool("vela_lookup",
		markmcp.WithDescription("Resolve a term into exact graph node candidates."),
		markmcp.WithReadOnlyHintAnnotation(true),
		markmcp.WithString("term", markmcp.Required(), markmcp.Description("Term to resolve")),
		markmcp.WithNumber("limit", markmcp.Description("Maximum candidates to include")),
	), func(ctx context.Context, req markmcp.CallToolRequest) (*markmcp.CallToolResult, error) {
		return handleLookupTool(engine)(ctx, req)
	})
}

func handleLookupTool(engine *query.Engine) server.ToolHandlerFunc {
	return func(ctx context.Context, req markmcp.CallToolRequest) (*markmcp.CallToolResult, error) {
		_ = ctx
		term := strings.TrimSpace(req.GetString("term", ""))
		if term == "" {
			return markmcp.NewToolResultStructuredOnly(engine.DiagnosticResult("lookup", "VALIDATION_ERROR", "term is required")), nil
		}
		return markmcp.NewToolResultStructuredOnly(engine.LookupResult(term, req.GetInt("limit", types.DefaultQueryLimit))), nil
	}
}

func registerStatusTool(srv *server.MCPServer, engine *query.Engine) {
	srv.AddTool(markmcp.NewTool("vela_status",
		markmcp.WithDescription("Report Vela runtime graph status."),
		markmcp.WithReadOnlyHintAnnotation(true),
	), handleStatusTool(engine))
}

func handleStatusTool(engine *query.Engine) server.ToolHandlerFunc {
	return func(ctx context.Context, req markmcp.CallToolRequest) (*markmcp.CallToolResult, error) {
		_ = ctx
		_ = req
		return markmcp.NewToolResultStructuredOnly(engine.StatusResult()), nil
	}
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
			return markmcp.NewToolResultStructuredOnly(engine.DiagnosticResult(kind, "VALIDATION_ERROR", err.Error())), nil
		}
		switch queryReq.Kind {
		case types.QueryKindExplain:
			return markmcp.NewToolResultStructuredOnly(engine.ExplainResult(queryReq.Subject)), nil
		case types.QueryKindImpact:
			return markmcp.NewToolResultStructuredOnly(engine.ImpactResult(queryReq.Subject, queryReq.Limit)), nil
		case types.QueryKindPath:
			return markmcp.NewToolResultStructuredOnly(engine.PathResult(queryReq.Subject, queryReq.Target)), nil
		}
		output, err := engine.RunRequest(queryReq)
		if err != nil {
			return markmcp.NewToolResultStructuredOnly(engine.DiagnosticResult(kind, "QUERY_ERROR", err.Error())), nil
		}
		return markmcp.NewToolResultText(output), nil
	}
}
