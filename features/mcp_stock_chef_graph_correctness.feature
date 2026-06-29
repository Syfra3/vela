Feature: Vela MCP Stock-Chef Graph Correctness — Slice A+B+C
  MCP answers for stock-chef must use the same selected graph and freshness semantics as CLI status,
  expose canonical tool names, and avoid silently mixing active workspaces with similarly named corpora.

  Background:
    Given Vela has graph metadata for a real stock-chef workspace
    And Vela may also have benchmark, fixture, or dependency-evaluation corpora with the stock-chef display name

  @SCN-001 @REQ-001 @REQ-002
  Scenario: Fresh stock-chef workspace reports fresh MCP graph and no build recommendation
    Given CLI status selects the real stock-chef workspace graph as fresh
    When an MCP client asks for graph status from the same stock-chef working directory
    Then the MCP response identifies the same selected graph source as CLI status
    And the MCP response reports the selected graph freshness as fresh
    And the MCP response does not recommend running vela build or vela update

  @SCN-002 @REQ-001
  Scenario: MCP response includes graph source evidence for debugging selection
    Given CLI status selects the real stock-chef workspace graph as fresh
    When an MCP client asks a graph-backed question from the real stock-chef working directory
    Then the MCP response identifies the selected graph path used for evidence
    And the MCP response identifies the selected project or repository identity
    And the MCP response identifies the workspace root or project path used for selection
    And the MCP response includes the selected graph updated timestamp when that timestamp is available

  @SCN-003 @REQ-002
  Scenario: Stale or unknown freshness explains why build is recommended
    Given CLI status selects the real stock-chef workspace graph as stale or freshness-unavailable
    When an MCP client asks a graph-backed question from the real stock-chef working directory
    Then the MCP response applies stale or freshness-unavailable guidance to the selected graph only
    And the MCP response identifies the selected graph root or source that the guidance applies to
    And the MCP response explains why vela build, vela update, or status follow-up is recommended

  @SCN-004 @REQ-003
  Scenario: MCP exposes non-duplicated tool names when server is named vela
    Given the Vela MCP server is initialized
    When an MCP client lists available tools
    Then the raw MCP local tool list includes preferred unprefixed Vela tool names
    And the raw MCP local tool list includes explore, lookup, status, explain, impact, and path
    And a client display may prefix the server name once, such as vela_explore
    But neither the raw tool list nor client display includes vela_vela_explore
    And neither the raw tool list nor client display includes vela_vela_explain
    And neither the raw tool list nor client display includes vela_vela_impact
    And neither the raw tool list nor client display includes vela_vela_path
    And the server does not require the client to call any duplicated vela_vela_* name

  @SCN-005 @REQ-004
  Scenario: Active real stock-chef workspace is preferred over dep-eval stock-chef corpus
    Given the MCP server is running with the real stock-chef workspace as its active root
    And a dependency-evaluation graph corpus also has the stock-chef display name
    When an MCP client asks who uses a symbol that exists in the active stock-chef workspace graph
    Then MCP selects the active workspace graph for graph evidence
    And the response identifies the active workspace or corpus used
    And the response does not include graph facts from the dependency-evaluation stock-chef corpus

  @SCN-006 @REQ-005
  Scenario: Ambiguous stock-chef corpora require explicit disambiguation
    Given multiple usable stock-chef corpora are available
    And MCP cannot determine a single active workspace or explicit corpus for the request
    When an MCP client asks a graph-backed stock-chef question
    Then MCP returns an ambiguous result instead of a merged answer
    And the response lists candidate roots or corpus identifiers when available
    And the response does not mix nodes, edges, snippets, freshness states, or diagnostics across unresolved corpora
    And the response tells the caller how to disambiguate the target corpus

  @SCN-007 @REQ-004 @REQ-005
  Scenario: Missing active graph does not silently fall back to dep-eval corpus
    Given the MCP server is running with the real stock-chef workspace as its active root
    And the active workspace graph is missing, invalid, or unreadable
    And a dependency-evaluation stock-chef corpus exists outside the active root
    When an MCP client asks a graph-backed stock-chef question
    Then MCP returns a structured unavailable or ambiguous diagnostic for the active request
    And MCP does not silently answer from the dependency-evaluation stock-chef corpus
    And the response includes guidance to build, update, check status, or choose an explicit corpus/root
