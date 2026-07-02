package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/Syfra3/vela/internal/app"
	"github.com/Syfra3/vela/internal/query"
	"github.com/Syfra3/vela/pkg/types"
	markmcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const serverInstructions = `Vela exposes read-only graph-truth dependency queries through the MCP ` + "`explore`" + ` tool and specialized graph tools.

Treat Vela as a structural graph tool, not as free-text or keyword search.

Rules:
- For structural, architectural, flow, dependency, ownership, or impact questions, call ` + "`explore`" + ` first.
- For ranking/hotspot questions such as highest impact, most depended-on, most dependencies, central module, or biggest blast radius, call compact ` + "`rank`" + ` or ` + "`hotspots`" + ` first with low limits.
- Use ` + "`module_summary`" + ` for exact top candidates instead of dumping all edges; full edge dumps require explicit opt-in via explain-style queries.
- Treat returned source snippets and graph paths as already-read evidence.
- Preserve raw grep or file reads for exact text lookup, stale files named by Vela, unavailable graphs, or verification of latest source.
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
- rank/hotspots for highest-impact or module-ranking questions
- module_summary X for bounded counts/examples on an exact node or path

Workflow:
1. Start structural questions with ` + "`explore`" + ` so Vela can route to the right graph primitive.
2. Find exact node candidates before specialized tool calls when more precision is needed.
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
	registerQueryTool(srv, engine, "dependencies", types.QueryKindDependencies, false)
	registerQueryTool(srv, engine, "reverse_dependencies", types.QueryKindReverseDependencies, false)
	registerQueryTool(srv, engine, "impact", types.QueryKindImpact, false)
	registerQueryTool(srv, engine, "path", types.QueryKindPath, true)
	registerQueryTool(srv, engine, "explain", types.QueryKindExplain, false)
	registerRankTool(srv, engine)
	registerHotspotsTool(srv, engine)
	registerModuleSummaryTool(srv, engine)
	registerStatusTool(srv, engine)
}

func registerRankTool(srv *server.MCPServer, engine *query.Engine) {
	srv.AddTool(markmcp.NewTool("rank",
		markmcp.WithDescription("Rank graph nodes/files/modules with split structural metrics and bounded examples."),
		markmcp.WithReadOnlyHintAnnotation(true),
		markmcp.WithString("scope", markmcp.Description("Path, glob-like prefix, node ID, or label scope to rank within")),
		markmcp.WithString("metric", markmcp.Description("Ranking metric: in_degree, out_degree, total_degree, or downstream_count")),
		markmcp.WithNumber("limit", markmcp.Description("Maximum ranked candidates to include; default 10")),
		markmcp.WithNumber("examples", markmcp.Description("Maximum examples per candidate; default 3")),
	), func(ctx context.Context, req markmcp.CallToolRequest) (*markmcp.CallToolResult, error) {
		_ = ctx
		return markmcp.NewToolResultStructuredOnly(engine.RankResult(req.GetString("scope", ""), req.GetString("metric", ""), req.GetInt("limit", query.DefaultRankLimit), req.GetInt("examples", query.DefaultRankExamples))), nil
	})
}

func registerHotspotsTool(srv *server.MCPServer, engine *query.Engine) {
	srv.AddTool(markmcp.NewTool("hotspots",
		markmcp.WithDescription("Ergonomic wrapper for highest-impact/dependency/centrality hotspot questions."),
		markmcp.WithReadOnlyHintAnnotation(true),
		markmcp.WithString("intent", markmcp.Description("Hotspot intent, e.g. highest impact, most dependencies, most depended-on")),
		markmcp.WithString("scope", markmcp.Description("Optional path, glob-like prefix, node ID, or label scope")),
		markmcp.WithNumber("limit", markmcp.Description("Maximum candidates; default 10")),
		markmcp.WithNumber("examples", markmcp.Description("Maximum examples per candidate; default 3")),
	), func(ctx context.Context, req markmcp.CallToolRequest) (*markmcp.CallToolResult, error) {
		_ = ctx
		return markmcp.NewToolResultStructuredOnly(engine.HotspotsResult(req.GetString("intent", ""), req.GetString("scope", ""), req.GetInt("limit", query.DefaultRankLimit), req.GetInt("examples", query.DefaultRankExamples))), nil
	})
}

func registerModuleSummaryTool(srv *server.MCPServer, engine *query.Engine) {
	srv.AddTool(markmcp.NewTool("module_summary",
		markmcp.WithDescription("Summarize one exact graph node/path with counts, bounded examples, confidence, and gaps."),
		markmcp.WithReadOnlyHintAnnotation(true),
		markmcp.WithString("target", markmcp.Required(), markmcp.Description("Exact node ID, label, or path to summarize")),
		markmcp.WithNumber("examples", markmcp.Description("Maximum incoming/outgoing examples; default 5")),
	), func(ctx context.Context, req markmcp.CallToolRequest) (*markmcp.CallToolResult, error) {
		_ = ctx
		target := strings.TrimSpace(req.GetString("target", ""))
		if target == "" {
			return markmcp.NewToolResultStructuredOnly(engine.DiagnosticResult("module_summary", "VALIDATION_ERROR", "target is required")), nil
		}
		return markmcp.NewToolResultStructuredOnly(engine.ModuleSummaryResult(target, req.GetInt("examples", query.DefaultSummaryExamples))), nil
	})
}

func registerExploreTool(srv *server.MCPServer, engine *query.Engine) {
	srv.AddTool(markmcp.NewTool("explore",
		markmcp.WithDescription("Resolve broad graph context requests into graph-backed candidates."),
		markmcp.WithReadOnlyHintAnnotation(true),
		markmcp.WithString("query", markmcp.Required(), markmcp.Description("Natural-language request to resolve")),
		markmcp.WithNumber("limit", markmcp.Description("Maximum candidates to include")),
	), func(ctx context.Context, req markmcp.CallToolRequest) (*markmcp.CallToolResult, error) {
		return handleExploreTool(engine)(ctx, req)
	})
}

func handleExploreTool(engine *query.Engine) server.ToolHandlerFunc {
	return handleExploreToolWithConnectTimeCatchUp(engine, false)
}

func handleExploreToolWithConnectTimeCatchUp(engine *query.Engine, catchUpRunning bool) server.ToolHandlerFunc {
	return func(ctx context.Context, req markmcp.CallToolRequest) (*markmcp.CallToolResult, error) {
		_ = ctx
		input := strings.TrimSpace(req.GetString("query", ""))
		if input == "" {
			return markmcp.NewToolResultStructuredOnly(engine.DiagnosticResult("explore", "VALIDATION_ERROR", "query is required")), nil
		}
		result := engine.ExploreResult(input, req.GetInt("limit", types.DefaultQueryLimit))
		if catchUpRunning && result.Freshness.Status != query.FreshnessFresh {
			result.Freshness.Status = query.FreshnessWarming
			result.Status = query.ResultStatusPartial
			result.Diagnostics = append(result.Diagnostics, query.Diagnostic{
				Code:    "MCP_CATCHUP_WARMING",
				Message: "MCP connect-time graph catch-up is still running; retry the explore call or run `vela status` before trusting graph freshness.",
			})
		}
		return markmcp.NewToolResultStructuredOnly(result), nil
	}
}

func registerLookupTool(srv *server.MCPServer, engine *query.Engine) {
	srv.AddTool(markmcp.NewTool("lookup",
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
	srv.AddTool(markmcp.NewTool("status",
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
		if diagnostic := engine.UnavailableDiagnostic(); diagnostic != nil {
			return markmcp.NewToolResultText(formatUnavailableActiveGraph(queryReq.Subject, *diagnostic)), nil
		}
		if candidates := engine.AmbiguousCorpora(); len(candidates) > 1 {
			return markmcp.NewToolResultText(formatCorpusAmbiguity(queryReq.Subject, candidates)), nil
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
		output = appendSelectedGraphEvidence(output, engine.Freshness())
		return markmcp.NewToolResultText(output), nil
	}
}

func formatUnavailableActiveGraph(subject string, diagnostic query.UnavailableDiagnostic) string {
	var b strings.Builder
	status := strings.TrimSpace(diagnostic.Status)
	if status == "" {
		status = "unavailable"
	}
	fmt.Fprintf(&b, "Status: %s\n", status)
	message := strings.TrimSpace(diagnostic.Message)
	if message == "" {
		message = "active workspace graph is missing or unreadable"
	}
	fmt.Fprintf(&b, "MCP cannot answer %q because %s.\n", subject, message)
	if diagnostic.Workspace != "" {
		fmt.Fprintf(&b, "Active workspace root: %s\n", diagnostic.Workspace)
	}
	if diagnostic.GraphPath != "" {
		fmt.Fprintf(&b, "Expected active graph: %s\n", diagnostic.GraphPath)
	}
	if len(diagnostic.Candidates) > 0 {
		b.WriteString("Other stock-chef corpora were not used automatically:\n")
		for _, candidate := range diagnostic.Candidates {
			fmt.Fprintf(&b, "- project=%s", candidate.Project)
			if candidate.Root != "" {
				fmt.Fprintf(&b, " root=%s", candidate.Root)
			}
			if candidate.GraphPath != "" {
				fmt.Fprintf(&b, " graph_path=%s", candidate.GraphPath)
			}
			b.WriteByte('\n')
		}
	}
	b.WriteString("Guidance: run vela build, vela update, or vela status in the active workspace, or choose an explicit corpus/root with --graph; no dep-eval nodes or edges were used as fallback.")
	return b.String()
}

func formatCorpusAmbiguity(subject string, candidates []query.CorpusCandidate) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Status: ambiguous\n")
	fmt.Fprintf(&b, "MCP cannot choose a single graph corpus for %q without an active workspace root or explicit corpus selector.\n", subject)
	b.WriteString("Candidates:\n")
	for _, candidate := range candidates {
		fmt.Fprintf(&b, "- project=%s", candidate.Project)
		if candidate.Root != "" {
			fmt.Fprintf(&b, " root=%s", candidate.Root)
		}
		if candidate.GraphPath != "" {
			fmt.Fprintf(&b, " graph_path=%s", candidate.GraphPath)
		}
		b.WriteByte('\n')
	}
	b.WriteString("Disambiguate: choose an explicit corpus by running from the intended workspace or passing --graph / an explicit corpus path; no nodes or edges were merged across candidates.")
	return b.String()
}

func appendSelectedGraphEvidence(output string, freshness query.Freshness) string {
	parts := make([]string, 0, 4)
	if freshness.SelectedGraphPath != "" {
		parts = append(parts, fmt.Sprintf("selected_graph_path=%s", freshness.SelectedGraphPath))
	}
	if freshness.WorkspaceRoot != "" {
		parts = append(parts, fmt.Sprintf("workspace_root=%s", freshness.WorkspaceRoot))
	}
	if freshness.Project != "" {
		parts = append(parts, fmt.Sprintf("project=%s", freshness.Project))
	}
	if freshness.Status != "" {
		parts = append(parts, fmt.Sprintf("freshness=%s", freshness.Status))
	}
	if len(parts) == 0 {
		return output
	}
	return output + "\n\nGraph evidence: " + strings.Join(parts, ", ")
}
