Feature: Vela agent explore runtime contract
  AI coding agents need one graph-first exploration surface with explicit runtime state
  so that structural answers are grounded in Vela graph evidence without hiding stale or missing graph truth.

  Background:
    Given a Vela workspace can be queried through CLI and MCP surfaces

  @SCN-001 @REQ-002 @REQ-004
  Scenario: CLI explore answers a known structural question with the stable sections
    Given `.vela/graph.db` is available and fresh
    And the runtime graph contains graph-backed facts for symbol "RefundService"
    When the caller runs `vela explore "explain RefundService"`
    Then the CLI response includes the section "Answer"
    And the CLI response includes the section "Freshness"
    And the CLI response includes the section "Relevant source"
    And the CLI response includes the section "Paths and relationships"
    And the CLI response includes the section "Impact radius"
    And the CLI response includes the section "Layered evidence"
    And the CLI response includes the section "Confidence and limits"
    And the CLI response includes the section "Suggested next queries"
    And the response identifies the interpreted intent as "explain"
    And the response cites graph-backed evidence for "RefundService"

  @SCN-002 @REQ-003 @REQ-004
  Scenario: MCP exposes vela_explore with the shared structured envelope
    Given `.vela/graph.db` is available and fresh
    When an MCP client lists available Vela tools
    Then the tool list includes `vela_explore`
    When the MCP client calls `vela_explore` with query "explain RefundService"
    Then the tool result uses schema version "vela.explore.v1"
    And the tool result includes structured freshness, intent, evidence, diagnostics, and suggested next queries
    And the tool result preserves the same core graph facts as the equivalent CLI explore request

  @SCN-003 @REQ-004 @REQ-006
  Scenario: Explore response includes every required section even when sections are not relevant
    Given `.vela/graph.db` is available and fresh
    And the runtime graph contains graph-backed facts for symbol "RefundStatus"
    When the caller runs `vela explore "who uses RefundStatus?"`
    Then the response includes freshness state "fresh"
    And the response includes relevant source evidence when available
    And the response includes paths or relationships used as evidence
    And the impact radius section is present even if marked not relevant
    And the confidence and limits section explains any missing snippets or graph families
    And suggested next queries use exact discovered labels

  @SCN-004 @REQ-007 @REQ-012
  Scenario Outline: Planner routes common intent families to existing graph primitives
    Given `.vela/graph.db` is available and fresh
    And the runtime graph contains facts for the requested subjects
    When the caller runs `vela explore "<question>"`
    Then the interpreted intent is "<intent>"
    And the response is derived from existing "<primitive>" behavior
    And the response does not require the caller to provide the low-level query form first

    Examples:
      | question                                      | intent     | primitive                         |
      | explain RefundService                         | explain    | lookup/explain                    |
      | who uses RefundStatus?                         | usage      | reverse dependency / who uses     |
      | what does WebhookHandler depend on?            | dependency | dependency / callee neighborhood  |
      | how does StripeWebhook reach RefundService?    | path       | path                              |
      | what breaks if RefundStatus changes?           | impact     | impact / bounded reverse reach    |

  @SCN-005 @REQ-007 @REQ-008
  Scenario: Ambiguous explore query returns candidates instead of a strong claim
    Given `.vela/graph.db` is available and fresh
    And the runtime graph contains multiple distinct nodes matching "auth"
    When the caller runs `vela explore "explain auth"`
    Then the response status is "ambiguous"
    And the response lists candidate nodes with distinguishing metadata
    And the response asks for refinement or suggests exact follow-up queries
    And the response does not present one candidate as the graph-backed answer

  @SCN-006 @REQ-005 @REQ-006
  Scenario: Missing runtime DB fails with actionable diagnostics and no JSON fallback
    Given `.vela/graph.json` exists
    And `.vela/graph.db` is missing
    When the caller runs `vela explore "explain RefundService"`
    Then the response freshness state is "unavailable"
    And the response says `.vela/graph.db` is required for runtime graph answers
    And the response recommends `vela build`, `vela update`, and `vela status`
    And Vela does not use `.vela/graph.json` as runtime graph truth
    And the CLI command fails instead of returning a graph-backed answer

  @SCN-007 @REQ-003 @REQ-006
  Scenario: MCP connect-time catch-up returns warming unless the DB is already fresh
    Given the MCP server starts while connect-time graph catch-up is running
    And `.vela/graph.db` is not yet known to be fresh
    When an MCP client calls `vela_explore` with query "explain RefundService"
    Then the tool result freshness state is "warming"
    And the tool result status is "partial" or "unavailable"
    And the tool result includes retry or status guidance
    And the MCP server does not block indefinitely before returning the tool result

  @SCN-008 @REQ-006
  Scenario: MCP first explore call returns fresh when the runtime DB is already fresh
    Given the MCP server starts with `.vela/graph.db` already available and fresh
    When an MCP client calls `vela_explore` with query "explain RefundService"
    Then the tool result freshness state is "fresh"
    And the tool result is not reported as "warming"

  @SCN-009 @REQ-006 @REQ-008
  Scenario: Stale or pending freshness names affected files when known
    Given `.vela/graph.db` is available
    And Vela knows that `internal/query/query.go` may be stale or pending
    When the caller runs `vela explore "who uses QueryEngine?"`
    Then the response freshness state is "stale" or "pending"
    And the response names `internal/query/query.go` as affected when known
    And the response warns that exact latest source may require a direct file read
    And the response recommends `vela update` or `vela build` when the graph is stale

  @SCN-010 @REQ-009
  Scenario: Layered evidence labels separate code, workspace, and contract facts
    Given `.vela/graph.db` contains repo/code evidence, workspace routing evidence, and contract evidence for a refund workflow
    When the caller runs `vela explore "where is the refund API contract enforced?"`
    Then repo/code evidence is labeled "repo_code"
    And workspace routing evidence is labeled "workspace"
    And contract evidence is labeled "contract"
    And contract evidence is not presented as inferred executable code truth
    And missing relevant graph families are reported as unavailable or not configured

  @SCN-011 @REQ-010
  Scenario: Normal structural queries omit memory evidence by default
    Given `.vela/graph.db` is available and fresh
    And memory observations exist for "RefundService"
    When the caller runs `vela explore "explain RefundService"`
    Then the response may include repo_code, workspace, or contract evidence when relevant
    And the response does not include memory evidence
    And the response marks memory as not requested if memory status is shown

  @SCN-012 @REQ-010
  Scenario: Decision-history queries include memory evidence as a separate layer
    Given `.vela/graph.db` is available and fresh
    And memory observations exist for "Stripe refunds"
    When the caller runs `vela explore "what did we decide about Stripe refunds?"`
    Then the interpreted intent includes "memory"
    And the response includes memory evidence when available
    And memory evidence is labeled "memory"
    And memory evidence is not merged into repo_code graph facts

  @SCN-013 @REQ-011
  Scenario: Agent instructions prefer vela_explore first without promising auto-sync
    Given Vela exposes MCP agent instructions for coding agents
    When an agent reads the Vela instructions
    Then the instructions say to call `vela_explore` first for structural, architectural, flow, dependency, ownership, or impact questions
    And the instructions say returned source snippets and graph paths can be treated as already-read evidence
    And the instructions preserve raw grep or file reads for exact text lookup, stale files named by Vela, or unavailable graphs
    And the instructions do not promise watcher, debounce, or auto-sync behavior for the Phase 1 shell

  @SCN-014 @REQ-012
  Scenario: Existing specialized CLI and MCP tools remain available
    Given Vela is installed with the explore runtime contract
    When the caller inspects available CLI commands and MCP tools
    Then low-level CLI commands or query forms for lookup, search, explain, impact, path, build, update, and status remain available
    And low-level MCP tools for lookup, explain, impact, path, and status remain available when they were previously supported
    And `vela explore` is presented as the default agent surface rather than the only graph capability

  @SCN-015 @REQ-001 @REQ-011
  Scenario: Phase 1 shell defers watcher and debounce implementation to later phases
    Given the Phase 1 explore shell is implemented
    When the caller asks Vela how active-session freshness is handled
    Then Vela may report known freshness, stale, warming, unavailable, or pending states from existing runtime state
    And Vela does not claim that MCP-session file watching or debounced auto-sync is implemented by this slice
    And Vela points stale or unavailable users to `vela update`, `vela build`, or `vela status`
