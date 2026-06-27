Feature: Vela v0.4 Truth Graph with Evidence
  Vela v0.4 provides coding agents and humans with graph-backed context.
  Answers must be grounded in graph facts, evidence, confidence, provenance, and freshness state.

  Background:
    Given a Vela v0.4 workspace under test
    And the workspace can be built into a runtime graph

  @SCN-001 @REQ-001
  Scenario: Important answers include proof metadata when evidence is available
    Given the runtime graph contains a known node with evidence, provenance, confidence, and freshness metadata
    When a caller asks Vela to explain that node
    Then the answer includes the graph fact for the node
    And the answer includes evidence or provenance for the fact
    And the answer includes the confidence of the fact
    And the answer includes freshness status for the graph data used

  @SCN-002 @REQ-001 @REQ-015
  Scenario: Vela refuses to invent an answer when no graph-backed fact exists
    Given the runtime graph contains no fact that supports the requested claim
    When a caller asks Vela for that claim
    Then Vela returns a no-result or unresolved response
    And Vela does not present a speculative answer as graph truth
    And Vela includes a diagnostic explaining that no graph-backed answer was available

  @SCN-003 @REQ-002
  Scenario: SQLite graph database is required for runtime queries
    Given `.vela/graph.json` exists
    And `.vela/graph.db` is missing
    When a caller runs a runtime graph query
    Then Vela rejects the query as runtime graph unavailable
    And Vela does not use `.vela/graph.json` as the source of query truth
    And Vela recommends `vela build` or `vela update`

  @SCN-004 @REQ-002 @REQ-003
  Scenario: Build creates runtime and generated graph artifacts with SQLite as truth
    Given a fixture repository with extractable symbols and dependencies
    When the caller runs `vela build`
    Then `.vela/graph.db` exists as the runtime query store
    And `.vela/manifest.json` exists for freshness detection
    And generated debug or report artifacts are not treated as runtime truth

  @SCN-005 @REQ-004
  Scenario: CLI and MCP return equivalent core results for the same explain query
    Given a fresh runtime graph contains a known node
    When the caller asks the CLI to explain the node
    And an MCP client asks `vela_explain` for the same node
    Then both answers are derived from the same core result fields
    And both answers describe the same resolved subject and graph facts
    And both answers preserve evidence, confidence, freshness, and diagnostics semantics

  @SCN-006 @REQ-005
  Scenario: Explore resolves natural language into graph-backed structural context
    Given the workspace graph contains multiple concrete nodes related to a broad feature term
    When a caller runs `vela explore` with that broad term
    Then Vela returns resolved candidate nodes or routes before giving a final answer
    And any final answer cites graph facts used to support it
    And free-text matching alone is not presented as proof

  @SCN-007 @REQ-005 @REQ-015
  Scenario: Ambiguous explore query returns candidates instead of choosing silently
    Given two graph nodes match the same user term
    When a caller runs `vela explore` for that term
    Then Vela returns an ambiguous result
    And the result lists candidate nodes with distinguishing information
    And Vela asks for refinement or selection before making a strong claim

  @SCN-008 @REQ-006
  Scenario: MCP server exposes the required v0.4 toolset
    Given a fresh runtime graph is available
    When the caller starts `vela serve --mcp`
    And an MCP client lists available tools
    Then the tool list includes `vela_explore`
    And the tool list includes `vela_lookup`
    And the tool list includes `vela_explain`
    And the tool list includes `vela_impact`
    And the tool list includes `vela_path`
    And the tool list includes `vela_status`

  @SCN-009 @REQ-006
  Scenario: OpenCode and Claude-compatible MCP clients can call Vela tools
    Given `vela serve --mcp` is running against a fresh runtime graph
    When an OpenCode-compatible MCP client calls a Vela tool
    And a Claude Code-compatible MCP client calls the same Vela tool
    Then each client receives a structured MCP response
    And each response contains the core result fields required for agents

  @SCN-010 @REQ-007 @REQ-013
  Scenario: MCP startup reports stale graph state when safe update is unavailable
    Given `.vela/manifest.json` indicates the runtime graph is stale
    And Vela cannot safely update the graph during MCP startup
    When the caller starts `vela serve --mcp`
    Then MCP status reports the graph as stale or unknown
    And relevant tool responses include a stale warning
    And the warning identifies stale files or stale scopes when known

  @SCN-011 @REQ-008
  Scenario: CLI provides required v0.4 command surface
    Given the Vela CLI is installed for v0.4
    When the caller inspects available commands
    Then the CLI exposes `vela explore`
    And the CLI exposes `vela lookup`
    And the CLI exposes `vela status`
    And the CLI exposes `vela build`
    And the CLI exposes `vela update`
    And the CLI exposes `vela watch`
    And the CLI exposes `vela serve --mcp`
    And the CLI supports explain, impact, and path structural queries through commands or structural search forms

  @SCN-012 @REQ-009 @REQ-010
  Scenario: Interface evidence fixture preserves declared, extracted, inferred, and ambiguous facts
    Given an interface evidence fixture with declared contracts, framework routes, HTTP client calls, manifests, workspace hints, and naming heuristics
    When Vela builds the runtime graph
    Then interface facts from declared contracts are marked declared
    And interface facts from framework routes are marked extracted
    And interface facts from HTTP client calls are marked extracted or inferred according to certainty
    And interface facts from manifests are marked inferred unless explicitly declared
    And interface facts from workspace hints are marked declared hint
    And interface facts from naming heuristics are marked ambiguous

  @SCN-013 @REQ-010 @REQ-015
  Scenario: Higher-confidence evidence outranks conflicting lower-confidence evidence without hiding conflict
    Given two evidence providers report conflicting facts about the same interface relationship
    And one fact is declared
    And the other fact is inferred or ambiguous
    When a caller asks Vela to explain the relationship
    Then Vela presents the declared fact as higher confidence
    And Vela includes a conflict diagnostic for the lower-confidence contradictory fact
    And Vela does not merge the conflicting facts silently

  @SCN-014 @REQ-011
  Scenario: Workspace YAML declares multi-codebase topology
    Given `.vela/workspace.yaml` declares an organization, repositories, services, interfaces, and known links
    When Vela builds the runtime graph
    Then workspace topology facts are included in graph results
    And those facts carry provenance to `.vela/workspace.yaml`
    And those facts are identified as routing or topology truth rather than deep code truth

  @SCN-015 @REQ-011 @REQ-015
  Scenario: Invalid workspace YAML fails with actionable validation diagnostics
    Given `.vela/workspace.yaml` contains an invalid required field
    When the caller runs `vela build` or `vela status`
    Then Vela reports a workspace validation error
    And the diagnostic identifies the invalid field or path
    And Vela does not treat the invalid topology as declared routing truth

  @SCN-016 @REQ-012
  Scenario: Multi-repo exploration routes first and retrieves deeply second
    Given a multi-repo workspace graph with declared services and interface links
    When a caller asks Vela to explore a cross-service feature area
    Then Vela identifies candidate repositories, services, or interfaces before deep retrieval
    And Vela distinguishes workspace routing facts from code graph facts
    And Vela marks any route ambiguity in the result

  @SCN-017 @REQ-012 @REQ-010
  Scenario: Cross-repo path includes confidence for interface bridge evidence
    Given two repositories are connected by an inferred interface relationship
    When a caller asks Vela for the path between a node in the first repo and a node in the second repo
    Then Vela returns the cross-repo path if graph facts support it
    And the interface bridge in the path is marked inferred
    And the path is not presented as a declared contract path

  @SCN-018 @REQ-013
  Scenario: Status reports pending stale files after source changes
    Given the runtime graph was built from a workspace manifest
    And a tracked source file changes after the build
    When the caller runs `vela status`
    Then Vela reports the graph as stale
    And Vela lists the changed file or stale scope when known
    And Vela recommends `vela update` or `vela build`

  @SCN-019 @REQ-013
  Scenario: Update safely refreshes stale graph state
    Given the runtime graph is stale
    When the caller runs `vela update`
    Then Vela performs a safe incremental update or fallback rebuild
    And a later `vela status` reports the graph as fresh if the update succeeds
    And an interrupted or failed update does not mark a corrupt graph as fresh

  @SCN-020 @REQ-013
  Scenario: Watch debounces bursts of changes before updating
    Given `vela watch` is running for a workspace
    When several relevant files change in a short burst
    Then Vela debounces the burst into a safe update cycle
    And Vela does not perform one rebuild per individual file event

  @SCN-021 @REQ-014
  Scenario: Single-repo fixture proves SQLite graph build and symbol dependency queries
    Given the v0.4 single-repo fixture
    When release verification runs against the fixture
    Then Vela builds `.vela/graph.db`
    And symbol queries return expected graph nodes
    And dependency queries return expected relationships from SQLite runtime truth

  @SCN-022 @REQ-014
  Scenario: Multi-repo fixture proves declared workspace routing
    Given the v0.4 multi-repo fixture with `.vela/workspace.yaml`
    When release verification explores a cross-repo request
    Then Vela routes through the declared workspace topology
    And the result identifies the expected repositories or services
    And the result includes provenance to `.vela/workspace.yaml`

  @SCN-023 @REQ-014 @REQ-006
  Scenario: MCP fixture proves required tools can be served and called
    Given the v0.4 MCP fixture
    When release verification starts `vela serve --mcp`
    Then an MCP client can list the required Vela tools
    And the client can call each required tool successfully or receive the expected structured diagnostic

  @SCN-024 @REQ-014 @REQ-004
  Scenario: CLI and MCP equivalence fixture proves shared schema behavior
    Given a fixture query with a known expected core result
    When release verification runs the equivalent CLI command
    And release verification calls the equivalent MCP tool
    Then both adapter outputs match the expected core result semantics
    And neither adapter adds unsupported claims

  @SCN-025 @REQ-014
  Scenario: Real workspace smoke test proves release behavior outside toy fixtures
    Given a maintainer-selected real workspace
    When release verification runs the v0.4 smoke test
    Then Vela builds or updates the runtime graph successfully
    And `vela status` returns a meaningful freshness result
    And at least one CLI query returns an evidence-bearing graph answer
    And at least one MCP tool call returns an evidence-bearing structured graph answer

  @SCN-026 @REQ-015
  Scenario: Multiple diagnostics are preserved in degraded results
    Given a query is affected by both stale graph state and ambiguous subject resolution
    When a caller asks Vela for graph context
    Then the result includes the stale graph diagnostic
    And the result includes the ambiguity diagnostic
    And the result does not hide available lower-confidence facts
    And the result qualifies those facts with their confidence and freshness status
