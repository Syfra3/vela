package ir

// NodeKind identifies a framework-neutral Phase 1 Common IR node category.
type NodeKind string

// EdgeKind identifies a framework-neutral Phase 1 Common IR relationship category.
type EdgeKind string

// FactCategory identifies the fact groups a query needs for conservative answer gating.
type FactCategory string

const (
	NodeKindRoute         NodeKind = "Route"
	NodeKindHTTPCall      NodeKind = "HttpCall"
	NodeKindSchema        NodeKind = "Schema"
	NodeKindSchemaField   NodeKind = "SchemaField"
	NodeKindSideEffect    NodeKind = "SideEffect"
	NodeKindTestUsage     NodeKind = "TestUsage"
	NodeKindConfigBinding NodeKind = "ConfigBinding"
	NodeKindArtifact      NodeKind = "Artifact"
)

const (
	EdgeKindHandledBy          EdgeKind = "HANDLED_BY"
	EdgeKindCalls              EdgeKind = "CALLS"
	EdgeKindUsesSchema         EdgeKind = "USES_SCHEMA"
	EdgeKindHasField           EdgeKind = "HAS_FIELD"
	EdgeKindProducesSideEffect EdgeKind = "PRODUCES_SIDE_EFFECT"
	EdgeKindCoveredByTest      EdgeKind = "COVERED_BY_TEST"
	EdgeKindReadsConfig        EdgeKind = "READS_CONFIG"
	EdgeKindGeneratesArtifact  EdgeKind = "GENERATES_ARTIFACT"
	EdgeKindDependsOn          EdgeKind = "DEPENDS_ON"
	EdgeKindImpacts            EdgeKind = "IMPACTS"
)

const (
	FactCategoryRoute        FactCategory = "route"
	FactCategoryHandler      FactCategory = "handler"
	FactCategoryDependency   FactCategory = "dependency"
	FactCategorySchema       FactCategory = "schema"
	FactCategorySideEffect   FactCategory = "side_effect"
	FactCategoryTestCoverage FactCategory = "test_coverage"
	FactCategoryArtifact     FactCategory = "artifact"
)

// NodeFact is the observable Common IR shape for a typed node fact.
type NodeFact struct {
	ID       string
	Kind     NodeKind
	Label    string
	Metadata FactMetadata
}

// EdgeFact is the observable Common IR shape for a typed relationship fact.
type EdgeFact struct {
	ID       string
	SourceID string
	TargetID string
	Kind     EdgeKind
	Metadata FactMetadata
}

// LegacyDependencyRelationship carries a pre-IR dependency edge through the IR compatibility boundary.
type LegacyDependencyRelationship struct {
	SubjectID     string
	DependencyID  string
	DependencyLbl string
	Hidden        bool
}

// LegacyImpactSnapshot carries the legacy fields available before IR migration or compatibility wrapping.
type LegacyImpactSnapshot struct {
	StableID        string
	ChangedSubject  string
	ImpactedSubject string
	ImpactedLabel   string
	SourceEvidence  string
}

// LegacyImpactRelationship carries a pre-IR impact edge through the IR compatibility boundary.
type LegacyImpactRelationship struct {
	StableID       string
	ChangedID      string
	ImpactedID     string
	ImpactedLbl    string
	SourceEvidence string
	Metadata       FactMetadata
	Hidden         bool
}

// DependencyQueryFixture describes the minimal legacy plus additive IR facts for a dependency query.
type DependencyQueryFixture struct {
	SubjectID           string
	LegacyRelationships []LegacyDependencyRelationship
	AdditiveIRFacts     []EdgeFact
}

// DependencyQueryResult is the observable compatibility result for dependency queries after IR support is enabled.
type DependencyQueryResult struct {
	LegacyRelationships []LegacyDependencyRelationship
	AdditiveIRFacts     []EdgeFact
}

// ImpactQueryFixture describes the minimal legacy facts for an impact query.
type ImpactQueryFixture struct {
	ChangedSubjectID string
	LegacyImpacts    []LegacyImpactRelationship
}

// ImpactQueryResult is the observable compatibility result for impact queries after IR support is enabled.
type ImpactQueryResult struct {
	LegacyImpacts []LegacyImpactRelationship
}

// CategoryFact is the minimal observable category status used for answer confidence gating.
type CategoryFact struct {
	Category         FactCategory
	Fresh            bool
	Confidence       string
	SourceFile       string
	RecordedFileHash string
	CurrentFileHash  string
	SourceRange      string
	ExtractorName    string
	ExtractorVersion string
}

// QueryConfidenceFixture describes the required category facts for a query answer.
type QueryConfidenceFixture struct {
	RequiredCategories    []FactCategory
	NonCriticalCategories []FactCategory
	Facts                 []CategoryFact
}

// QueryConfidenceAnswer is the observable result of conservative answer gating.
type QueryConfidenceAnswer struct {
	GraphBacked                  bool
	ExplorationNeeded            bool
	ExplorationApprovalRequired  bool
	ExplorationState             string
	SatisfiedCategories          []FactCategory
	KnownCategories              []FactCategory
	MissingRequiredCategories    []FactCategory
	MissingNonCriticalCategories []FactCategory
	CompletelyCheckedCategories  []FactCategory
	StaleCategories              []FactCategory
	AffectedEvidenceSources      []string
}

// TargetedExplorationProposal is the pre-approval explanation for token-spending exploration.
type TargetedExplorationProposal struct {
	KnownCategories         []FactCategory
	MissingCategories       []FactCategory
	StaleCategories         []FactCategory
	NeedReason              string
	TargetedScope           string
	ExplorationRun          bool
	EnrichmentWriteOccurred bool
}

// ExplorationApproval records explicit user approval for a bounded targeted exploration scope.
type ExplorationApproval struct {
	Approved bool
	Scope    string
}

// ExplorationFinding carries a typed fact discovered by approved targeted exploration.
type ExplorationFinding struct {
	Category FactCategory
	Node     NodeFact
	Note     string
}

// EnrichmentFactStore is the minimal graph-boundary store for exploration-enriched Common IR facts.
type EnrichmentFactStore struct {
	nodes []categorizedNodeFact
}

// EquivalentQuery describes a later query whose required categories may be answered from fresh enrichment.
type EquivalentQuery struct {
	RequiredCategories []FactCategory
}

// EnrichmentReuseResult reports whether a later query reused fresh enrichment instead of repeating exploration.
type EnrichmentReuseResult struct {
	ReusedFacts         []NodeFact
	ExplorationRepeated bool
}

// UnpromotedExplorationAnswer reports an approved exploration statement that was not promoted to graph truth.
type UnpromotedExplorationAnswer struct {
	Category               FactCategory
	UnpersistedLimitations []string
	GraphBacked            bool
	SatisfiedCategories    []FactCategory
}

// RouteHandlerOwnershipFixture carries competing handler facts for one route ownership answer.
type RouteHandlerOwnershipFixture struct {
	RouteID              string
	DeterministicHandler EdgeFact
	EnrichedHandler      EdgeFact
}

// ConflictingEnrichmentFact keeps a non-authoritative enrichment conflict visible to callers.
type ConflictingEnrichmentFact struct {
	Fact   EdgeFact
	Status string
}

// RouteHandlerOwnershipAnswer reports the authoritative handler and any visible enrichment conflicts.
type RouteHandlerOwnershipAnswer struct {
	AuthoritativeHandlerID           string
	ConflictingEnrichments           []ConflictingEnrichmentFact
	EnrichmentDeleted                bool
	EnrichmentTrustedAsAuthoritative bool
}

// InferredImpactFixture carries a graph-reasoned impact and the evidence used to derive it.
type InferredImpactFixture struct {
	DerivedImpact                  EdgeFact
	GraphEvidence                  []EdgeFact
	ConflictingDeterministicImpact EdgeFact
}

// InferredImpactAnswer reports an inferred impact without allowing it to override deterministic facts.
type InferredImpactAnswer struct {
	Impact                           EdgeFact
	GraphEvidence                    []EdgeFact
	AuthoritativeDeterministicImpact EdgeFact
	DeterministicFactOverridden      bool
}

// TestUsageCoverageFixture describes a schema coverage query over indexed TestUsage facts.
type TestUsageCoverageFixture struct {
	SchemaID              string
	TestUsageFactsIndexed bool
}

// EmptyResultAnswer reports why an empty result is not necessarily a known-empty claim.
type EmptyResultAnswer struct {
	CategoryStatus              map[FactCategory]string
	NegativeClaimed             bool
	SuggestedNextAction         string
	ExplorationApprovalGuidance string
}

// CoexistenceQueryFixture describes legacy runtime graph facts and additive Common IR facts relevant to one query.
type CoexistenceQueryFixture struct {
	SubjectID   string
	LegacyFacts []LegacyDependencyRelationship
	IRFacts     []EdgeFact
}

// CoexistenceQueryAnswer keeps prior runtime evidence and new IR evidence distinguishable in one answer.
type CoexistenceQueryAnswer struct {
	LegacyBackedEvidence     []LegacyDependencyRelationship
	IRBackedEvidence         []EdgeFact
	LowLevelGraphToolsUsable bool
	ClaimsFullReplacement    bool
}

type categorizedNodeFact struct {
	category FactCategory
	node     NodeFact
}

// FactMetadata exposes the trust metadata required for every Common IR fact.
type FactMetadata struct {
	Origin           string
	Confidence       string
	Freshness        FreshnessMetadata
	ExtractorName    string
	ExtractorVersion string
	SourceFile       string
	EvidenceSnippet  string
	LastSeenAt       string
	Language         string
	Framework        string
	Unknowns         map[string]string
}

// FreshnessMetadata carries the MVP freshness fields for a fact.
type FreshnessMetadata struct {
	SourceFileHash string
	SourceRange    string
}

// ValidFactForEachRequiredNodeKind returns one observable fact for every Phase 1 node kind.
func ValidFactForEachRequiredNodeKind() []NodeFact {
	kinds := []NodeKind{
		NodeKindRoute,
		NodeKindHTTPCall,
		NodeKindSchema,
		NodeKindSchemaField,
		NodeKindSideEffect,
		NodeKindTestUsage,
		NodeKindConfigBinding,
		NodeKindArtifact,
	}

	nodes := make([]NodeFact, 0, len(kinds))
	for _, kind := range kinds {
		nodes = append(nodes, NodeFact{
			ID:       "common-ir-node-" + string(kind),
			Kind:     kind,
			Label:    string(kind),
			Metadata: validNodeFactMetadata(),
		})
	}

	return nodes
}

// InspectCommonIRNodes returns the observable node facts for Common IR inspection.
func InspectCommonIRNodes(nodes []NodeFact) []NodeFact {
	return nodes
}

// ValidFactForEachRequiredEdgeKind returns one observable fact for every Phase 1 edge kind.
func ValidFactForEachRequiredEdgeKind() []EdgeFact {
	kinds := []EdgeKind{
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

	edges := make([]EdgeFact, 0, len(kinds))
	for _, kind := range kinds {
		edges = append(edges, EdgeFact{
			ID:       "common-ir-edge-" + string(kind),
			SourceID: "common-ir-source-" + string(kind),
			TargetID: "common-ir-target-" + string(kind),
			Kind:     kind,
			Metadata: validEdgeFactMetadata(kind),
		})
	}

	return edges
}

// InspectCommonIRRelationships returns the observable edge facts for Common IR inspection.
func InspectCommonIRRelationships(edges []EdgeFact) []EdgeFact {
	return edges
}

// RunDependencyQueryWithIRSupport preserves pre-IR dependency relationships and returns additive IR facts separately.
func RunDependencyQueryWithIRSupport(fixture DependencyQueryFixture) DependencyQueryResult {
	legacy := make([]LegacyDependencyRelationship, 0, len(fixture.LegacyRelationships))
	for _, relationship := range fixture.LegacyRelationships {
		if relationship.SubjectID == fixture.SubjectID {
			legacy = append(legacy, relationship)
		}
	}

	irFacts := make([]EdgeFact, 0, len(fixture.AdditiveIRFacts))
	for _, fact := range fixture.AdditiveIRFacts {
		if fact.SourceID == fixture.SubjectID {
			irFacts = append(irFacts, fact)
		}
	}

	return DependencyQueryResult{LegacyRelationships: legacy, AdditiveIRFacts: irFacts}
}

// RehydrateLegacyImpactRelationship wraps legacy impact data without dropping available identifiers, labels, evidence, or relationships.
func RehydrateLegacyImpactRelationship(snapshot LegacyImpactSnapshot) LegacyImpactRelationship {
	metadata := legacyUnavailableMetadata()
	metadata.EvidenceSnippet = snapshot.SourceEvidence

	return LegacyImpactRelationship{
		StableID:       snapshot.StableID,
		ChangedID:      snapshot.ChangedSubject,
		ImpactedID:     snapshot.ImpactedSubject,
		ImpactedLbl:    snapshot.ImpactedLabel,
		SourceEvidence: snapshot.SourceEvidence,
		Metadata:       metadata,
	}
}

// RunImpactQueryWithIRSupport preserves pre-IR impact relationships through migration or compatibility wrapping.
func RunImpactQueryWithIRSupport(fixture ImpactQueryFixture) ImpactQueryResult {
	legacy := make([]LegacyImpactRelationship, 0, len(fixture.LegacyImpacts))
	for _, impact := range fixture.LegacyImpacts {
		if impact.ChangedID == fixture.ChangedSubjectID {
			legacy = append(legacy, impact)
		}
	}

	return ImpactQueryResult{LegacyImpacts: legacy}
}

// AnswerQueryWithConfidenceGate presents a graph-backed answer only when all required categories are fresh and high-confidence.
func AnswerQueryWithConfidenceGate(fixture QueryConfidenceFixture) QueryConfidenceAnswer {
	factsByCategory := make(map[FactCategory]CategoryFact, len(fixture.Facts))
	for _, fact := range fixture.Facts {
		factsByCategory[fact.Category] = fact
	}

	satisfied := make([]FactCategory, 0, len(fixture.RequiredCategories))
	missing := make([]FactCategory, 0)
	for _, category := range fixture.RequiredCategories {
		fact, ok := factsByCategory[category]
		if !ok {
			missing = append(missing, category)
			continue
		}
		fresh := fact.Fresh && !fact.SourceFileHashChanged()
		if !fresh || fact.Confidence != "high" {
			answer := QueryConfidenceAnswer{ExplorationNeeded: true, KnownCategories: satisfied}
			if !fresh {
				answer.StaleCategories = []FactCategory{category}
				if fact.SourceFile != "" {
					answer.AffectedEvidenceSources = []string{fact.SourceFile}
				}
			}
			return answer
		}
		satisfied = append(satisfied, category)
	}

	if len(missing) > 0 {
		return QueryConfidenceAnswer{
			ExplorationNeeded:           true,
			ExplorationApprovalRequired: true,
			ExplorationState:            "approval_required",
			KnownCategories:             satisfied,
			MissingRequiredCategories:   missing,
		}
	}

	missingNonCritical := make([]FactCategory, 0)
	for _, category := range fixture.NonCriticalCategories {
		if _, ok := factsByCategory[category]; !ok {
			missingNonCritical = append(missingNonCritical, category)
		}
	}

	return QueryConfidenceAnswer{
		GraphBacked:                  true,
		ExplorationNeeded:            false,
		ExplorationState:             "not_needed",
		SatisfiedCategories:          satisfied,
		KnownCategories:              satisfied,
		MissingNonCriticalCategories: missingNonCritical,
		CompletelyCheckedCategories:  satisfied,
	}
}

// ProposeTargetedExploration explains the known graph facts and smallest visible gap without running exploration.
func ProposeTargetedExploration(fixture QueryConfidenceFixture) TargetedExplorationProposal {
	factsByCategory := make(map[FactCategory]CategoryFact, len(fixture.Facts))
	for _, fact := range fixture.Facts {
		factsByCategory[fact.Category] = fact
	}

	proposal := TargetedExplorationProposal{}
	for _, category := range fixture.RequiredCategories {
		fact, ok := factsByCategory[category]
		if !ok {
			proposal.MissingCategories = append(proposal.MissingCategories, category)
			continue
		}

		if !fact.Fresh || fact.SourceFileHashChanged() {
			proposal.StaleCategories = append(proposal.StaleCategories, category)
			continue
		}

		proposal.KnownCategories = append(proposal.KnownCategories, category)
	}

	proposal.NeedReason = "required graph facts are missing or stale"
	proposal.TargetedScope = targetedExplorationScope(proposal)
	return proposal
}

// NewEnrichmentFactStore returns an empty typed enrichment fact store.
func NewEnrichmentFactStore() *EnrichmentFactStore {
	return &EnrichmentFactStore{}
}

// PersistApprovedExplorationFinding stores an approved exploration finding as typed Common IR enrichment.
func (s *EnrichmentFactStore) PersistApprovedExplorationFinding(approval ExplorationApproval, finding ExplorationFinding) (NodeFact, bool) {
	if !approval.Approved || approval.Scope == "" || !isTypedEvidenceBackedFinding(finding.Node) {
		return NodeFact{}, false
	}

	node := finding.Node
	node.Metadata.Origin = "exploration_enriched"
	node.Metadata = fillExplicitMetadataGaps(node.Metadata)
	s.nodes = append(s.nodes, categorizedNodeFact{category: finding.Category, node: node})

	return node, true
}

// RecordUnpromotedExplorationFinding keeps an untyped or evidence-free exploration statement out of graph truth.
func (s *EnrichmentFactStore) RecordUnpromotedExplorationFinding(category FactCategory, note string) UnpromotedExplorationAnswer {
	return UnpromotedExplorationAnswer{
		Category:               category,
		UnpersistedLimitations: []string{note},
	}
}

// ReuseFreshEnrichment returns matching fresh enrichment facts for equivalent queries without rerunning exploration.
func (s *EnrichmentFactStore) ReuseFreshEnrichment(query EquivalentQuery) EnrichmentReuseResult {
	result := EnrichmentReuseResult{ExplorationRepeated: true}
	for _, required := range query.RequiredCategories {
		for _, stored := range s.nodes {
			if stored.category == required && isFreshReusableEnrichment(stored.node) {
				result.ReusedFacts = append(result.ReusedFacts, stored.node)
				result.ExplorationRepeated = false
			}
		}
	}
	return result
}

// AnswerRouteHandlerOwnership selects fresh deterministic route ownership over conflicting enrichment.
func AnswerRouteHandlerOwnership(fixture RouteHandlerOwnershipFixture) RouteHandlerOwnershipAnswer {
	answer := RouteHandlerOwnershipAnswer{}
	if isFreshDeterministicHandlerFact(fixture.RouteID, fixture.DeterministicHandler) {
		answer.AuthoritativeHandlerID = fixture.DeterministicHandler.TargetID
	}

	if isConflictingEnrichedHandlerFact(fixture.RouteID, fixture.EnrichedHandler, answer.AuthoritativeHandlerID) {
		answer.ConflictingEnrichments = append(answer.ConflictingEnrichments, ConflictingEnrichmentFact{
			Fact:   fixture.EnrichedHandler,
			Status: "conflicting_non_authoritative",
		})
	}

	return answer
}

// AnswerInferredImpactRelationship returns graph-reasoned impact facts as inferred and keeps conflicting deterministic facts authoritative.
func AnswerInferredImpactRelationship(fixture InferredImpactFixture) InferredImpactAnswer {
	impact := fixture.DerivedImpact
	impact.Metadata.Origin = "inferred"

	answer := InferredImpactAnswer{Impact: impact, GraphEvidence: fixture.GraphEvidence}
	if isFreshDeterministicImpactFact(fixture.ConflictingDeterministicImpact) {
		answer.AuthoritativeDeterministicImpact = fixture.ConflictingDeterministicImpact
	}

	return answer
}

// AnswerTestUsageCoverageForSchema distinguishes unavailable TestUsage coverage from a supported negative claim.
func AnswerTestUsageCoverageForSchema(fixture TestUsageCoverageFixture) EmptyResultAnswer {
	if fixture.TestUsageFactsIndexed {
		return EmptyResultAnswer{CategoryStatus: map[FactCategory]string{FactCategoryTestCoverage: "available"}}
	}

	return EmptyResultAnswer{
		CategoryStatus: map[FactCategory]string{
			FactCategoryTestCoverage: "not_indexed",
		},
		SuggestedNextAction:         "index TestUsage facts for the project before claiming schema test coverage is known-empty",
		ExplorationApprovalGuidance: "request approval for targeted exploration of tests covering the schema",
	}
}

// AnswerCoexistingRuntimeAndIRFacts labels legacy-backed and IR-backed evidence separately without replacing low-level graph behavior.
func AnswerCoexistingRuntimeAndIRFacts(fixture CoexistenceQueryFixture) CoexistenceQueryAnswer {
	answer := CoexistenceQueryAnswer{LowLevelGraphToolsUsable: true}
	for _, fact := range fixture.LegacyFacts {
		if fact.SubjectID == fixture.SubjectID && !fact.Hidden {
			answer.LegacyBackedEvidence = append(answer.LegacyBackedEvidence, fact)
		}
	}
	for _, fact := range fixture.IRFacts {
		if fact.SourceID == fixture.SubjectID {
			answer.IRBackedEvidence = append(answer.IRBackedEvidence, fact)
		}
	}
	return answer
}

// SourceFileHashChanged reports whether current source freshness invalidates the recorded fact hash.
func (f CategoryFact) SourceFileHashChanged() bool {
	return f.RecordedFileHash != "" && f.CurrentFileHash != "" && f.RecordedFileHash != f.CurrentFileHash
}

// IncludesDependency reports whether the compatibility result still returns a legacy dependency.
func (r DependencyQueryResult) IncludesDependency(dependencyID string) bool {
	for _, relationship := range r.LegacyRelationships {
		if relationship.DependencyID == dependencyID && !relationship.Hidden {
			return true
		}
	}
	return false
}

// LegacyRelationshipRemovedOrHidden reports whether a legacy dependency was absent from visible results.
func (r DependencyQueryResult) LegacyRelationshipRemovedOrHidden(dependencyID string) bool {
	return !r.IncludesDependency(dependencyID)
}

// IRFact returns an additive IR-backed fact by stable ID.
func (r DependencyQueryResult) IRFact(id string) (EdgeFact, bool) {
	for _, fact := range r.AdditiveIRFacts {
		if fact.ID == id {
			return fact, true
		}
	}
	return EdgeFact{}, false
}

// IncludesImpactedSubject reports whether the compatibility result still returns a legacy impacted subject.
func (r ImpactQueryResult) IncludesImpactedSubject(subjectID string) bool {
	for _, impact := range r.LegacyImpacts {
		if impact.ImpactedID == subjectID && !impact.Hidden {
			return true
		}
	}
	return false
}

// LegacyImpact returns a migrated or compatibility-backed legacy impact by stable ID.
func (r ImpactQueryResult) LegacyImpact(stableID string) (LegacyImpactRelationship, bool) {
	for _, impact := range r.LegacyImpacts {
		if impact.StableID == stableID {
			return impact, true
		}
	}
	return LegacyImpactRelationship{}, false
}

// RequiredCategorySatisfied reports whether an answer identified the required category as satisfied.
func (a QueryConfidenceAnswer) RequiredCategorySatisfied(category FactCategory) bool {
	for _, satisfied := range a.SatisfiedCategories {
		if satisfied == category {
			return true
		}
	}
	return false
}

// KnownFactExplained reports whether a partial answer disclosed an already-known fact category.
func (a QueryConfidenceAnswer) KnownFactExplained(category FactCategory) bool {
	for _, known := range a.KnownCategories {
		if known == category {
			return true
		}
	}
	return false
}

// MissingRequiredCategory reports whether a partial answer disclosed a missing required category.
func (a QueryConfidenceAnswer) MissingRequiredCategory(category FactCategory) bool {
	for _, missing := range a.MissingRequiredCategories {
		if missing == category {
			return true
		}
	}
	return false
}

// MissingNonCriticalCategory reports whether a graph-backed answer disclosed a missing non-critical category.
func (a QueryConfidenceAnswer) MissingNonCriticalCategory(category FactCategory) bool {
	for _, missing := range a.MissingNonCriticalCategories {
		if missing == category {
			return true
		}
	}
	return false
}

// CategoryCheckedCompletely reports whether the answer claims complete coverage for a category.
func (a QueryConfidenceAnswer) CategoryCheckedCompletely(category FactCategory) bool {
	for _, checked := range a.CompletelyCheckedCategories {
		if checked == category {
			return true
		}
	}
	return false
}

// StaleFact reports whether a required fact category was disclosed as stale.
func (a QueryConfidenceAnswer) StaleFact(category FactCategory) bool {
	for _, stale := range a.StaleCategories {
		if stale == category {
			return true
		}
	}
	return false
}

// AffectedEvidenceSource reports whether the answer identified the source file or evidence source affected by staleness.
func (a QueryConfidenceAnswer) AffectedEvidenceSource(source string) bool {
	for _, affected := range a.AffectedEvidenceSources {
		if affected == source {
			return true
		}
	}
	return false
}

// KnownFactExplained reports whether an exploration proposal disclosed an already-known fact category.
func (p TargetedExplorationProposal) KnownFactExplained(category FactCategory) bool {
	for _, known := range p.KnownCategories {
		if known == category {
			return true
		}
	}
	return false
}

// ReusedFreshFact reports whether a later equivalent query reused the named fresh enrichment fact.
func (r EnrichmentReuseResult) ReusedFreshFact(id string) bool {
	for _, fact := range r.ReusedFacts {
		if fact.ID == id {
			return true
		}
	}
	return false
}

// ConflictingEnrichment reports whether a conflicting enrichment fact remains visible by stable ID.
func (a RouteHandlerOwnershipAnswer) ConflictingEnrichment(id string) (ConflictingEnrichmentFact, bool) {
	for _, conflict := range a.ConflictingEnrichments {
		if conflict.Fact.ID == id {
			return conflict, true
		}
	}
	return ConflictingEnrichmentFact{}, false
}

// GraphEvidenceUsed reports whether a derived impact answer exposed the named graph evidence fact.
func (a InferredImpactAnswer) GraphEvidenceUsed(id string) bool {
	for _, evidence := range a.GraphEvidence {
		if evidence.ID == id {
			return true
		}
	}
	return false
}

// ReportsCategoryAs reports whether an empty result disclosed a category status explicitly.
func (a EmptyResultAnswer) ReportsCategoryAs(category FactCategory, status string) bool {
	return a.CategoryStatus[category] == status
}

// LegacyEvidenceLabeled reports whether legacy-backed evidence remains explicitly separated from IR evidence.
func (a CoexistenceQueryAnswer) LegacyEvidenceLabeled(dependencyID string) bool {
	for _, fact := range a.LegacyBackedEvidence {
		if fact.DependencyID == dependencyID {
			return true
		}
	}
	return false
}

// IREvidenceLabeled reports whether IR-backed evidence remains explicitly separated from legacy evidence.
func (a CoexistenceQueryAnswer) IREvidenceLabeled(id string) bool {
	for _, fact := range a.IRBackedEvidence {
		if fact.ID == id {
			return true
		}
	}
	return false
}

// RequiredCategorySatisfied reports whether an unpromoted statement satisfied a high-confidence required category.
func (a UnpromotedExplorationAnswer) RequiredCategorySatisfied(category FactCategory) bool {
	for _, satisfied := range a.SatisfiedCategories {
		if satisfied == category {
			return true
		}
	}
	return false
}

// UnpersistedLimitationMentioned reports whether an unpromoted statement is visible only as a limitation or note.
func (a UnpromotedExplorationAnswer) UnpersistedLimitationMentioned(note string) bool {
	for _, limitation := range a.UnpersistedLimitations {
		if limitation == note {
			return true
		}
	}
	return false
}

func isTypedEvidenceBackedFinding(node NodeFact) bool {
	return node.ID != "" &&
		node.Kind != "" &&
		node.Metadata.EvidenceSnippet != "" &&
		node.Metadata.Freshness.SourceFileHash != "" &&
		node.Metadata.Freshness.SourceRange != ""
}

func isFreshReusableEnrichment(node NodeFact) bool {
	return node.Metadata.Origin == "exploration_enriched" &&
		node.Metadata.Confidence == "high" &&
		node.Metadata.Freshness.SourceFileHash != "" &&
		node.Metadata.Freshness.SourceRange != "" &&
		node.Metadata.ExtractorVersion != "" &&
		node.Metadata.EvidenceSnippet != "" &&
		node.Metadata.LastSeenAt != ""
}

func isFreshDeterministicHandlerFact(routeID string, fact EdgeFact) bool {
	return fact.SourceID == routeID &&
		fact.Kind == EdgeKindHandledBy &&
		fact.Metadata.Origin == "deterministic" &&
		fact.Metadata.Confidence == "high" &&
		fact.Metadata.Freshness.SourceFileHash != "" &&
		fact.Metadata.Freshness.SourceRange != ""
}

func isConflictingEnrichedHandlerFact(routeID string, fact EdgeFact, authoritativeHandlerID string) bool {
	return authoritativeHandlerID != "" &&
		fact.SourceID == routeID &&
		fact.Kind == EdgeKindHandledBy &&
		fact.TargetID != authoritativeHandlerID &&
		fact.Metadata.Origin == "exploration_enriched" &&
		fact.Metadata.Confidence != "" &&
		fact.Metadata.Freshness.SourceFileHash != "" &&
		fact.Metadata.EvidenceSnippet != ""
}

func isFreshDeterministicImpactFact(fact EdgeFact) bool {
	return fact.Kind == EdgeKindImpacts &&
		fact.Metadata.Origin == "deterministic" &&
		fact.Metadata.Confidence == "high" &&
		fact.Metadata.Freshness.SourceFileHash != "" &&
		fact.Metadata.Freshness.SourceRange != ""
}

func fillExplicitMetadataGaps(metadata FactMetadata) FactMetadata {
	if metadata.Language == "" {
		metadata.Language = "unknown"
	}
	if metadata.Framework == "" {
		metadata.Framework = "unknown"
	}
	if metadata.Unknowns == nil {
		metadata.Unknowns = make(map[string]string)
	}
	if metadata.Unknowns["language"] == "" && metadata.Language == "unknown" {
		metadata.Unknowns["language"] = "language unavailable from targeted exploration finding"
	}
	if metadata.Unknowns["framework"] == "" && metadata.Framework == "unknown" {
		metadata.Unknowns["framework"] = "framework unavailable from targeted exploration finding"
	}
	return metadata
}

func targetedExplorationScope(proposal TargetedExplorationProposal) string {
	if len(proposal.MissingCategories) > 0 || len(proposal.StaleCategories) > 0 {
		return "revalidate or collect only the missing/stale required fact categories"
	}
	return "no targeted exploration required"
}

func validNodeFactMetadata() FactMetadata {
	return FactMetadata{
		Origin:     "deterministic",
		Confidence: "high",
		Freshness: FreshnessMetadata{
			SourceFileHash: "sha256:example",
			SourceRange:    "1:1-1:1",
		},
		ExtractorName:    "common-ir-contract",
		ExtractorVersion: "v1",
		SourceFile:       "internal/ir/facts.go",
		EvidenceSnippet:  "Phase 1 Common IR node fact",
		LastSeenAt:       "2026-06-28T00:00:00Z",
		Language:         "unknown",
		Framework:        "unknown",
		Unknowns: map[string]string{
			"framework": "framework is unknown for framework-neutral Phase 1 node facts",
		},
	}
}

func validEdgeFactMetadata(kind EdgeKind) FactMetadata {
	metadata := validNodeFactMetadata()
	metadata.EvidenceSnippet = "Phase 1 Common IR edge fact"
	if kind == EdgeKindImpacts {
		metadata.Origin = "inferred"
	}
	return metadata
}

func legacyUnavailableMetadata() FactMetadata {
	return FactMetadata{
		Origin:     "deterministic",
		Confidence: "medium",
		Freshness: FreshnessMetadata{
			SourceFileHash: "legacy-unavailable",
			SourceRange:    "legacy-unavailable",
		},
		ExtractorName:    "legacy-graph-compatibility",
		ExtractorVersion: "legacy-unavailable",
		SourceFile:       "legacy-unavailable",
		LastSeenAt:       "legacy-unavailable",
		Language:         "unknown",
		Framework:        "unknown",
		Unknowns: map[string]string{
			"source_file_hash":  "legacy-unavailable: legacy graph impact did not record a source file hash",
			"source_range":      "legacy-unavailable: legacy graph impact did not record a source range",
			"extractor_version": "legacy-unavailable: legacy graph impact did not record an extractor version",
		},
	}
}
