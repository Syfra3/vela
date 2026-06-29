package ir

import "testing"

// REQ-001/REQ-015 → SCN-001 → TestSCN001_Phase1ExposesOnlyCommonIRCapabilities
func TestSCN001_Phase1ExposesOnlyCommonIRCapabilities(t *testing.T) {
	// Scenario: Phase 1 exposes only Common IR capabilities.
	capabilities := Phase1Capabilities()

	for _, want := range []string{
		"Common IR node taxonomy",
		"Common IR edge taxonomy",
		"fact metadata",
		"freshness",
		"confidence",
		"migration",
		"compatibility",
		"enrichment approval contract",
	} {
		if !capabilities.Includes(want) {
			t.Fatalf("expected Phase 1 capabilities to include %q, got %+v", want, capabilities)
		}
	}

	for _, forbidden := range []string{
		"JS/TS extraction support",
		"NestJS extraction support",
		"Python extraction support",
		"Go extraction support",
		"JVM extraction support",
		"C# extraction support",
		"Rust extraction support",
		"Rails extraction support",
		"Laravel extraction support",
		"framework-specific extraction support",
		"prior explore/runtime fully replaced",
	} {
		if capabilities.Claims(forbidden) {
			t.Fatalf("Phase 1 capabilities must not claim %q, got %+v", forbidden, capabilities)
		}
	}
}

// REQ-002/REQ-004/REQ-014 → SCN-002 → TestSCN002_RequiredCommonIRNodeKindsExposeTrustMetadata
func TestSCN002_RequiredCommonIRNodeKindsExposeTrustMetadata(t *testing.T) {
	// Scenario: All required Common IR node kinds are observable with trust metadata.
	nodes := InspectCommonIRNodes(ValidFactForEachRequiredNodeKind())

	wantKinds := []NodeKind{
		NodeKindRoute,
		NodeKindHTTPCall,
		NodeKindSchema,
		NodeKindSchemaField,
		NodeKindSideEffect,
		NodeKindTestUsage,
		NodeKindConfigBinding,
		NodeKindArtifact,
	}

	if len(nodes) != len(wantKinds) {
		t.Fatalf("expected %d inspected Common IR nodes, got %d: %+v", len(wantKinds), len(nodes), nodes)
	}

	byKind := make(map[NodeKind]NodeFact, len(nodes))
	for _, node := range nodes {
		byKind[node.Kind] = node
	}

	for _, wantKind := range wantKinds {
		node, ok := byKind[wantKind]
		if !ok {
			t.Fatalf("expected inspected nodes to include kind %q, got %+v", wantKind, nodes)
		}

		assertCompleteTrustMetadata(t, node.Metadata)
	}
}

// REQ-003/REQ-004/REQ-014 → SCN-003 → TestSCN003_RequiredCommonIREdgeKindsExposeTrustMetadata
func TestSCN003_RequiredCommonIREdgeKindsExposeTrustMetadata(t *testing.T) {
	// Scenario: All required Common IR edge kinds are observable with trust metadata.
	edges := InspectCommonIRRelationships(ValidFactForEachRequiredEdgeKind())

	wantKinds := []EdgeKind{
		EdgeKindHandledBy,
		EdgeKindCalls,
		EdgeKindUsesSchema,
		EdgeKindHasField,
		EdgeKindProducesSideEffect,
		EdgeKindCoveredByTest,
		EdgeKindReadsConfig,
		EdgeKindGeneratesArtifact,
		EdgeKindDependsOn,
		EdgeKindImpacts,
	}

	if len(edges) != len(wantKinds) {
		t.Fatalf("expected %d inspected Common IR edges, got %d: %+v", len(wantKinds), len(edges), edges)
	}

	byKind := make(map[EdgeKind]EdgeFact, len(edges))
	for _, edge := range edges {
		byKind[edge.Kind] = edge
	}

	for _, wantKind := range wantKinds {
		edge, ok := byKind[wantKind]
		if !ok {
			t.Fatalf("expected inspected edges to include kind %q, got %+v", wantKind, edges)
		}

		assertCompleteTrustMetadata(t, edge.Metadata)
	}

	if byKind[EdgeKindImpacts].Metadata.Origin != "inferred" {
		t.Fatalf("expected graph-reasoned IMPACTS edges to be labeled inferred, got %q", byKind[EdgeKindImpacts].Metadata.Origin)
	}
}

// REQ-011/REQ-012 → SCN-004 → TestSCN004_ExistingDependencyQueriesKeepWorkingAfterIRSupport
func TestSCN004_ExistingDependencyQueriesKeepWorkingAfterIRSupport(t *testing.T) {
	// Scenario: Existing dependency queries keep working after IR support is added.
	result := RunDependencyQueryWithIRSupport(DependencyQueryFixture{
		SubjectID: "subject-a",
		LegacyRelationships: []LegacyDependencyRelationship{{
			SubjectID:     "subject-a",
			DependencyID:  "dependency-b",
			DependencyLbl: "dependency B",
		}},
		AdditiveIRFacts: []EdgeFact{{
			ID:       "ir-dependency-edge",
			SourceID: "subject-a",
			TargetID: "ir-dependency-c",
			Kind:     EdgeKindDependsOn,
			Metadata: validEdgeFactMetadata(EdgeKindDependsOn),
		}},
	})

	if !result.IncludesDependency("dependency-b") {
		t.Fatalf("expected legacy dependency B to remain returned, got %+v", result)
	}
	if result.LegacyRelationshipRemovedOrHidden("dependency-b") {
		t.Fatalf("legacy dependency B must not be removed or hidden by migration, got %+v", result)
	}

	irFact, ok := result.IRFact("ir-dependency-edge")
	if !ok {
		t.Fatalf("expected additive IR-backed fact to be returned, got %+v", result)
	}
	if irFact.Kind == "" || irFact.Metadata.Origin == "" || irFact.Metadata.Confidence == "" || irFact.Metadata.Freshness.SourceFileHash == "" {
		t.Fatalf("expected additive IR-backed fact to be labeled with kind, origin, confidence, and freshness, got %+v", irFact)
	}
}

// REQ-011/REQ-012 → SCN-005 → TestSCN005_ExistingImpactQueriesKeepWorkingAfterMigrationOrCompatibilityWrapping
func TestSCN005_ExistingImpactQueriesKeepWorkingAfterMigrationOrCompatibilityWrapping(t *testing.T) {
	// Scenario: Existing impact queries keep working after migration or compatibility wrapping.
	result := RunImpactQueryWithIRSupport(ImpactQueryFixture{
		ChangedSubjectID: "subject-a",
		LegacyImpacts: []LegacyImpactRelationship{RehydrateLegacyImpactRelationship(LegacyImpactSnapshot{
			StableID:        "legacy-impact-a-c",
			ChangedSubject:  "subject-a",
			ImpactedSubject: "subject-c",
			ImpactedLabel:   "impacted subject C",
			SourceEvidence:  "subject A impacts subject C",
		})},
	})

	if !result.IncludesImpactedSubject("subject-c") {
		t.Fatalf("expected impacted subject C to remain returned, got %+v", result)
	}

	impact, ok := result.LegacyImpact("legacy-impact-a-c")
	if !ok {
		t.Fatalf("expected migrated legacy impact to preserve its stable identifier, got %+v", result)
	}
	if impact.ImpactedLbl != "impacted subject C" || impact.SourceEvidence != "subject A impacts subject C" {
		t.Fatalf("expected migrated legacy impact to preserve labels and source evidence, got %+v", impact)
	}
	if impact.Metadata.Unknowns["source_range"] == "" || impact.Metadata.Unknowns["extractor_version"] == "" {
		t.Fatalf("expected incomplete legacy metadata to be marked unknown, unavailable, or legacy-unavailable, got %+v", impact.Metadata)
	}
}

// REQ-005/REQ-013 → SCN-006 → TestSCN006_AnswersWithoutExplorationWhenRequiredCategoriesAreFreshHighConfidence
func TestSCN006_AnswersWithoutExplorationWhenRequiredCategoriesAreFreshHighConfidence(t *testing.T) {
	// Scenario: Vela answers without exploration only when required categories are fresh and high-confidence.
	answer := AnswerQueryWithConfidenceGate(QueryConfidenceFixture{
		RequiredCategories: []FactCategory{
			FactCategoryRoute,
			FactCategoryHandler,
			FactCategoryDependency,
			FactCategorySchema,
			FactCategoryTestCoverage,
		},
		Facts: []CategoryFact{
			freshHighConfidenceFact(FactCategoryRoute),
			freshHighConfidenceFact(FactCategoryHandler),
			freshHighConfidenceFact(FactCategoryDependency),
			freshHighConfidenceFact(FactCategorySchema),
			freshHighConfidenceFact(FactCategoryTestCoverage),
		},
	})

	if !answer.GraphBacked {
		t.Fatalf("expected answer to be presented as graph-backed, got %+v", answer)
	}
	if answer.ExplorationNeeded {
		t.Fatalf("expected exploration not to be needed, got %+v", answer)
	}
	if answer.ExplorationState != "not_needed" {
		t.Fatalf("expected answer to state exploration was not needed, got %q", answer.ExplorationState)
	}

	for _, category := range []FactCategory{
		FactCategoryRoute,
		FactCategoryHandler,
		FactCategoryDependency,
		FactCategorySchema,
		FactCategoryTestCoverage,
	} {
		if !answer.RequiredCategorySatisfied(category) {
			t.Fatalf("expected required category %q to be identified as satisfied, got %+v", category, answer)
		}
	}
}

// REQ-005/REQ-007/REQ-010 → SCN-007 → TestSCN007_MissingRequiredCategoryDoesNotOverclaimAndRequestsApproval
func TestSCN007_MissingRequiredCategoryDoesNotOverclaimAndRequestsApproval(t *testing.T) {
	// Scenario: Vela does not silently overclaim when required categories are missing.
	answer := AnswerQueryWithConfidenceGate(QueryConfidenceFixture{
		RequiredCategories: []FactCategory{
			FactCategoryRoute,
			FactCategoryHandler,
			FactCategoryDependency,
			FactCategorySchema,
			FactCategoryTestCoverage,
		},
		Facts: []CategoryFact{
			freshHighConfidenceFact(FactCategoryRoute),
			freshHighConfidenceFact(FactCategoryHandler),
			freshHighConfidenceFact(FactCategoryDependency),
			freshHighConfidenceFact(FactCategorySchema),
		},
	})

	if answer.GraphBacked {
		t.Fatalf("expected missing test coverage to prevent a strong complete graph-backed answer, got %+v", answer)
	}
	for _, category := range []FactCategory{
		FactCategoryRoute,
		FactCategoryHandler,
		FactCategoryDependency,
		FactCategorySchema,
	} {
		if !answer.KnownFactExplained(category) {
			t.Fatalf("expected known fact category %q to be explained, got %+v", category, answer)
		}
	}
	if !answer.MissingRequiredCategory(FactCategoryTestCoverage) {
		t.Fatalf("expected test coverage to be listed as a missing required category, got %+v", answer)
	}
	if answer.ExplorationState != "approval_required" || !answer.ExplorationApprovalRequired {
		t.Fatalf("expected answer to ask for approval before targeted exploration runs, got %+v", answer)
	}
}

// REQ-005/REQ-010 → SCN-008 → TestSCN008_NonCriticalMissingCategoryIsDisclosedWithoutBlockingGraphBackedAnswer
func TestSCN008_NonCriticalMissingCategoryIsDisclosedWithoutBlockingGraphBackedAnswer(t *testing.T) {
	// Scenario: Vela may answer with disclosed limits when missing categories are non-critical.
	answer := AnswerQueryWithConfidenceGate(QueryConfidenceFixture{
		RequiredCategories: []FactCategory{
			FactCategoryDependency,
		},
		NonCriticalCategories: []FactCategory{
			FactCategoryArtifact,
		},
		Facts: []CategoryFact{
			freshHighConfidenceFact(FactCategoryDependency),
		},
	})

	if !answer.GraphBacked {
		t.Fatalf("expected fresh high-confidence dependency facts to allow a graph-backed answer, got %+v", answer)
	}
	if !answer.MissingNonCriticalCategory(FactCategoryArtifact) {
		t.Fatalf("expected missing artifact facts to be disclosed as non-critical, got %+v", answer)
	}
	if answer.CategoryCheckedCompletely(FactCategoryArtifact) {
		t.Fatalf("answer must not imply artifact facts were checked completely, got %+v", answer)
	}
}

// REQ-006/REQ-010/REQ-014 → SCN-009 → TestSCN009_StaleFactsAreDisclosedAndDoNotSatisfyHighConfidenceGates
func TestSCN009_StaleFactsAreDisclosedAndDoNotSatisfyHighConfidenceGates(t *testing.T) {
	// Scenario: Stale facts are disclosed and do not satisfy high-confidence gates.
	answer := AnswerQueryWithConfidenceGate(QueryConfidenceFixture{
		RequiredCategories: []FactCategory{
			FactCategoryDependency,
		},
		Facts: []CategoryFact{{
			Category:         FactCategoryDependency,
			Fresh:            true,
			Confidence:       "high",
			SourceFile:       "internal/service/routes.go",
			RecordedFileHash: "sha256:recorded",
			CurrentFileHash:  "sha256:current",
			SourceRange:      "10:1-12:2",
			ExtractorName:    "common-ir-contract",
			ExtractorVersion: "v1",
		}},
	})

	if answer.GraphBacked {
		t.Fatalf("expected stale dependency fact not to be treated as fresh graph truth, got %+v", answer)
	}
	if !answer.StaleFact(FactCategoryDependency) {
		t.Fatalf("expected stale dependency fact to be reported as stale, got %+v", answer)
	}
	if !answer.AffectedEvidenceSource("internal/service/routes.go") {
		t.Fatalf("expected response to identify the affected source file, got %+v", answer)
	}
}

// REQ-007/REQ-013/REQ-014 → SCN-010 → TestSCN010_TargetedExplorationRequiresApprovalBeforeSpendingTokens
func TestSCN010_TargetedExplorationRequiresApprovalBeforeSpendingTokens(t *testing.T) {
	// Scenario: Targeted exploration requires approval before spending tokens.
	proposal := ProposeTargetedExploration(QueryConfidenceFixture{
		RequiredCategories: []FactCategory{
			FactCategoryRoute,
			FactCategoryDependency,
			FactCategorySchema,
		},
		Facts: []CategoryFact{
			freshHighConfidenceFact(FactCategoryRoute),
			{
				Category:         FactCategoryDependency,
				Fresh:            true,
				Confidence:       "high",
				SourceFile:       "internal/service/routes.go",
				RecordedFileHash: "sha256:recorded",
				CurrentFileHash:  "sha256:current",
			},
		},
	})

	if !proposal.KnownFactExplained(FactCategoryRoute) {
		t.Fatalf("expected proposal to explain graph facts already known, got %+v", proposal)
	}
	if proposal.NeedReason == "" {
		t.Fatalf("expected proposal to explain why exploration is needed, got %+v", proposal)
	}
	if proposal.TargetedScope == "" {
		t.Fatalf("expected proposal to describe the smallest targeted scope it can identify, got %+v", proposal)
	}
	if proposal.ExplorationRun || proposal.EnrichmentWriteOccurred {
		t.Fatalf("expected no exploration or enrichment write before explicit approval, got %+v", proposal)
	}
}

// REQ-007/REQ-008/REQ-013 → SCN-011 → TestSCN011_ApprovedExplorationPersistsTypedEnrichmentAndReusesFreshFact
func TestSCN011_ApprovedExplorationPersistsTypedEnrichmentAndReusesFreshFact(t *testing.T) {
	// Scenario: Approved exploration is persisted as typed enrichment and reused.
	store := NewEnrichmentFactStore()
	proposal := ProposeTargetedExploration(QueryConfidenceFixture{
		RequiredCategories: []FactCategory{FactCategorySideEffect},
	})

	persisted, ok := store.PersistApprovedExplorationFinding(ExplorationApproval{
		Approved: true,
		Scope:    proposal.TargetedScope,
	}, ExplorationFinding{
		Category: FactCategorySideEffect,
		Node: NodeFact{
			ID:    "side-effect-session-write",
			Kind:  NodeKindSideEffect,
			Label: "writes session cookie",
			Metadata: FactMetadata{
				Confidence:       "high",
				ExtractorName:    "targeted-exploration",
				ExtractorVersion: "v1",
				Freshness: FreshnessMetadata{
					SourceFileHash: "sha256:fresh",
					SourceRange:    "12:3-12:22",
				},
				SourceFile:      "internal/auth/session.go",
				EvidenceSnippet: "session.SetCookie(userID)",
				LastSeenAt:      "2026-06-28T00:00:00Z",
			},
		},
	})

	if !ok {
		t.Fatalf("expected approved exploration finding to be persisted, got ok=%v fact=%+v", ok, persisted)
	}
	if persisted.Kind != NodeKindSideEffect {
		t.Fatalf("expected persisted finding to be a typed SideEffect fact, got %+v", persisted)
	}
	if persisted.Metadata.Origin != "exploration_enriched" {
		t.Fatalf("expected persisted enrichment origin exploration_enriched, got %+v", persisted.Metadata)
	}
	assertCompleteTrustMetadata(t, persisted.Metadata)

	reuse := store.ReuseFreshEnrichment(EquivalentQuery{RequiredCategories: []FactCategory{FactCategorySideEffect}})
	if !reuse.ReusedFreshFact("side-effect-session-write") {
		t.Fatalf("expected later equivalent query to reuse fresh enrichment fact, got %+v", reuse)
	}
	if reuse.ExplorationRepeated {
		t.Fatalf("expected equivalent query not to repeat the same exploration, got %+v", reuse)
	}
}

// REQ-008/REQ-009/REQ-010 → SCN-012 → TestSCN012_UntypedExplorationFindingWithoutEvidenceIsNotPromotedToGraphTruth
func TestSCN012_UntypedExplorationFindingWithoutEvidenceIsNotPromotedToGraphTruth(t *testing.T) {
	// Scenario: Exploration findings without typed evidence are not promoted to graph truth.
	store := NewEnrichmentFactStore()

	persisted, ok := store.PersistApprovedExplorationFinding(ExplorationApproval{
		Approved: true,
		Scope:    "collect only side-effect evidence",
	}, ExplorationFinding{
		Category: FactCategorySideEffect,
		Note:     "this handler probably writes a session cookie",
	})

	if ok {
		t.Fatalf("expected untyped exploration statement without evidence not to be persisted as graph truth, got ok=%v fact=%+v", ok, persisted)
	}

	answer := store.RecordUnpromotedExplorationFinding(FactCategorySideEffect, "this handler probably writes a session cookie")
	if !answer.UnpersistedLimitationMentioned("this handler probably writes a session cookie") {
		t.Fatalf("expected answer to mention the statement only as an unpersisted limitation or note, got %+v", answer)
	}
	if answer.RequiredCategorySatisfied(FactCategorySideEffect) || answer.GraphBacked {
		t.Fatalf("expected unpromoted statement not to satisfy a high-confidence required category, got %+v", answer)
	}
}

// REQ-009/REQ-010/REQ-014 → SCN-013 → TestSCN013_DeterministicFactWinsOverConflictingEnrichment
func TestSCN013_DeterministicFactWinsOverConflictingEnrichment(t *testing.T) {
	// Scenario: Deterministic facts win over conflicting enrichment.
	answer := AnswerRouteHandlerOwnership(RouteHandlerOwnershipFixture{
		RouteID: "route-users-show",
		DeterministicHandler: EdgeFact{
			ID:       "deterministic-route-handler-a",
			SourceID: "route-users-show",
			TargetID: "HandlerA",
			Kind:     EdgeKindHandledBy,
			Metadata: validEdgeFactMetadata(EdgeKindHandledBy),
		},
		EnrichedHandler: EdgeFact{
			ID:       "enriched-route-handler-b",
			SourceID: "route-users-show",
			TargetID: "HandlerB",
			Kind:     EdgeKindHandledBy,
			Metadata: FactMetadata{
				Origin:           "exploration_enriched",
				Confidence:       "high",
				ExtractorName:    "targeted-exploration",
				ExtractorVersion: "v1",
				Freshness: FreshnessMetadata{
					SourceFileHash: "sha256:fresh",
					SourceRange:    "44:1-44:20",
				},
				SourceFile:      "internal/http/routes.go",
				EvidenceSnippet: "route.Handle(HandlerB)",
				LastSeenAt:      "2026-06-28T00:00:00Z",
			},
		},
	})

	if answer.AuthoritativeHandlerID != "HandlerA" {
		t.Fatalf("expected deterministic HandlerA to be authoritative, got %+v", answer)
	}
	conflict, ok := answer.ConflictingEnrichment("enriched-route-handler-b")
	if !ok {
		t.Fatalf("expected conflicting enrichment fact to remain visible, got %+v", answer)
	}
	if conflict.Status != "conflicting_non_authoritative" {
		t.Fatalf("expected conflicting enrichment status to be visible, got %+v", conflict)
	}
	if conflict.Fact.Metadata.Origin != "exploration_enriched" || conflict.Fact.Metadata.Confidence == "" || conflict.Fact.Metadata.Freshness.SourceFileHash == "" || conflict.Fact.Metadata.EvidenceSnippet == "" {
		t.Fatalf("expected conflicting enrichment to expose origin, confidence, freshness, and evidence, got %+v", conflict.Fact.Metadata)
	}
	if answer.EnrichmentDeleted || answer.EnrichmentTrustedAsAuthoritative {
		t.Fatalf("expected enrichment not to be silently trusted or deleted, got %+v", answer)
	}
}

// REQ-004/REQ-009/REQ-013 → SCN-014 → TestSCN014_InferredImpactFactsDoNotMasqueradeAsDeterministicFacts
func TestSCN014_InferredImpactFactsDoNotMasqueradeAsDeterministicFacts(t *testing.T) {
	// Scenario: Inferred impact facts do not masquerade as deterministic facts.
	answer := AnswerInferredImpactRelationship(InferredImpactFixture{
		DerivedImpact: EdgeFact{
			ID:       "inferred-impact-a-b",
			SourceID: "subject-a",
			TargetID: "subject-b",
			Kind:     EdgeKindImpacts,
			Metadata: validEdgeFactMetadata(EdgeKindDependsOn),
		},
		GraphEvidence: []EdgeFact{{
			ID:       "dependency-evidence-a-b",
			SourceID: "subject-a",
			TargetID: "subject-b",
			Kind:     EdgeKindDependsOn,
			Metadata: validEdgeFactMetadata(EdgeKindDependsOn),
		}},
		ConflictingDeterministicImpact: EdgeFact{
			ID:       "deterministic-impact-a-c",
			SourceID: "subject-a",
			TargetID: "subject-c",
			Kind:     EdgeKindImpacts,
			Metadata: validEdgeFactMetadata(EdgeKindDependsOn),
		},
	})

	if answer.Impact.Metadata.Origin != "inferred" {
		t.Fatalf("expected graph-reasoned impact relationship to be labeled inferred, got %+v", answer.Impact.Metadata)
	}
	if !answer.GraphEvidenceUsed("dependency-evidence-a-b") {
		t.Fatalf("expected answer to expose the graph evidence used to derive the impact, got %+v", answer)
	}
	if answer.AuthoritativeDeterministicImpact.ID != "deterministic-impact-a-c" || answer.DeterministicFactOverridden {
		t.Fatalf("expected conflicting fresh deterministic impact not to be overridden, got %+v", answer)
	}
}

// REQ-010/REQ-013/REQ-014 → SCN-015 → TestSCN015_EmptyResultsDistinguishKnownEmptyFromUnavailableOrUnsupportedData
func TestSCN015_EmptyResultsDistinguishKnownEmptyFromUnavailableOrUnsupportedData(t *testing.T) {
	// Scenario: Empty results distinguish known-empty from unavailable or unsupported data.
	answer := AnswerTestUsageCoverageForSchema(TestUsageCoverageFixture{
		SchemaID:              "schema-user-response",
		TestUsageFactsIndexed: false,
	})

	if !answer.ReportsCategoryAs(FactCategoryTestCoverage, "not_indexed") {
		t.Fatalf("expected TestUsage coverage to be reported as missing/unavailable/not indexed/unsupported, got %+v", answer)
	}
	if answer.NegativeClaimed {
		t.Fatalf("answer must not claim no tests exist without graph-contract support for that negative claim, got %+v", answer)
	}
	if answer.SuggestedNextAction == "" && answer.ExplorationApprovalGuidance == "" {
		t.Fatalf("expected suggested next action or exploration approval guidance, got %+v", answer)
	}
}

// REQ-015/REQ-011 → SCN-016 → TestSCN016_PriorRuntimeAndLowLevelGraphBehaviorCoexistsWithNewIR
func TestSCN016_PriorRuntimeAndLowLevelGraphBehaviorCoexistsWithNewIR(t *testing.T) {
	// Scenario: Prior runtime and low-level graph behavior coexists with the new IR.
	answer := AnswerCoexistingRuntimeAndIRFacts(CoexistenceQueryFixture{
		SubjectID: "subject-a",
		LegacyFacts: []LegacyDependencyRelationship{{
			SubjectID:     "subject-a",
			DependencyID:  "legacy-dependency-b",
			DependencyLbl: "legacy dependency B",
		}},
		IRFacts: []EdgeFact{{
			ID:       "ir-dependency-c",
			SourceID: "subject-a",
			TargetID: "ir-dependency-c",
			Kind:     EdgeKindDependsOn,
			Metadata: validEdgeFactMetadata(EdgeKindDependsOn),
		}},
	})

	if !answer.LegacyEvidenceLabeled("legacy-dependency-b") {
		t.Fatalf("expected legacy-backed evidence to be labeled separately, got %+v", answer)
	}
	if !answer.IREvidenceLabeled("ir-dependency-c") {
		t.Fatalf("expected IR-backed evidence to be labeled separately, got %+v", answer)
	}
	if !answer.LowLevelGraphToolsUsable {
		t.Fatalf("expected existing low-level graph tools to remain usable, got %+v", answer)
	}
	if answer.ClaimsFullReplacement {
		t.Fatalf("answer must not state Phase 1 completed full replacement of previous runtime behavior, got %+v", answer)
	}
}

func freshHighConfidenceFact(category FactCategory) CategoryFact {
	return CategoryFact{Category: category, Fresh: true, Confidence: "high"}
}

func assertCompleteTrustMetadata(t *testing.T, metadata FactMetadata) {
	t.Helper()

	if metadata.Origin == "" {
		t.Fatal("expected origin to be observable")
	}
	if metadata.Confidence == "" {
		t.Fatal("expected confidence to be observable")
	}
	if metadata.Freshness.SourceFileHash == "" {
		t.Fatal("expected source file hash freshness metadata to be observable")
	}
	if metadata.Freshness.SourceRange == "" {
		t.Fatal("expected source range freshness metadata to be observable")
	}
	if metadata.ExtractorName == "" {
		t.Fatal("expected extractor name to be observable")
	}
	if metadata.ExtractorVersion == "" {
		t.Fatal("expected extractor version to be observable")
	}
	if metadata.EvidenceSnippet == "" {
		t.Fatal("expected source evidence to be observable")
	}
	if metadata.LastSeenAt == "" {
		t.Fatal("expected last seen time to be observable")
	}
	if len(metadata.Unknowns) == 0 {
		t.Fatal("expected unknown or unavailable metadata to be explicit with a reason")
	}
	for field, reason := range metadata.Unknowns {
		if field == "" || reason == "" {
			t.Fatalf("expected unknown metadata field and reason to be explicit, got field=%q reason=%q", field, reason)
		}
	}
}
