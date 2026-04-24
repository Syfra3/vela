package types

import (
	"errors"
	"sort"
	"strings"
)

// This file defines the shared, architecture-facing types that the rest of
// Vela's layered retrieval system builds on. The topology modeled here is:
//
//   organization -> workspace -> repo -> subsystem -> node/chunk
//
// Memory entities (observations, decisions, bugfixes, preferences) live in a
// separate memory layer and reference repo-layer entities rather than being
// embedded inside them.
//
// These types stabilize contracts before routing, fusion, and orchestration
// logic is wired up, so downstream layers can reason about identity, evidence,
// and routing without inventing their own conventions.

// Layer discriminates the four architectural layers in Vela's retrieval stack.
type Layer string

const (
	// LayerRepo is repo-local code, files, symbols, and chunks — the correctness
	// path for deep code retrieval. Repo-local SQLite, FTS, and vector indexes
	// live in this layer.
	LayerRepo Layer = "repo"

	// LayerContract is declared service and interface truth derived from
	// artifacts such as OpenAPI specs, proto files, manifests, and schema files.
	// Contract evidence outranks derived signals during fusion.
	LayerContract Layer = "contract"

	// LayerWorkspace is the lightweight routing graph over repos, services,
	// domains, packages, and dependencies. It answers "which repos should we
	// search" — it is not a deep code graph.
	LayerWorkspace Layer = "workspace"

	// LayerMemory is the structured layer over Ancora-backed observations,
	// decisions, bugfixes, and preferences. It links to repo entities by
	// reference rather than duplicating code structure.
	LayerMemory Layer = "memory"
)

// Topology node types for the organization -> workspace -> repo -> subsystem
// hierarchy. Chunk is the smallest retrievable unit under a file/symbol.
const (
	NodeTypeRepo      NodeType = "repo"
	NodeTypeSubsystem NodeType = "subsystem"
	NodeTypeChunk     NodeType = "chunk"

	// NodeTypeService and NodeTypeContract live in the contract layer.
	NodeTypeService  NodeType = "service"
	NodeTypeContract NodeType = "contract"

	// NodeTypeDomain is a workspace-layer tag node (e.g. "billing", "auth")
	// used for coarse routing. It is not a code entity.
	NodeTypeDomain NodeType = "domain"
)

// Confidence classifies how strongly a piece of evidence should be trusted.
// Higher values beat lower values when two signals disagree during fusion.
type Confidence string

const (
	// ConfidenceDeclared is evidence pulled directly from a declared artifact
	// (OpenAPI, proto, manifest). This is the strongest signal.
	ConfidenceDeclared Confidence = "declared"

	// ConfidenceExtracted is evidence pulled from structured parsing of code
	// (AST, import graph). Strong but weaker than declared contracts.
	ConfidenceExtracted Confidence = "extracted"

	// ConfidenceInferred is evidence derived from heuristics, embeddings, or
	// cross-reference resolution.
	ConfidenceInferred Confidence = "inferred"

	// ConfidenceAmbiguous marks evidence whose target could not be uniquely
	// resolved. Consumers should surface this rather than silently picking one.
	ConfidenceAmbiguous Confidence = "ambiguous"
)

// VerificationState tracks whether a memory reference still binds to a live
// code entity. The binder transitions references between these states as code
// moves or gets deleted.
type VerificationState string

const (
	// VerificationCurrent means the reference resolves to the same entity it
	// originally pointed at.
	VerificationCurrent VerificationState = "current"

	// VerificationRedirected means the entity was renamed or moved and the
	// binder has a high-confidence new target.
	VerificationRedirected VerificationState = "redirected"

	// VerificationStale means the entity no longer exists and no replacement
	// was found.
	VerificationStale VerificationState = "stale"

	// VerificationAmbiguous means multiple plausible targets exist and the
	// binder refused to silently pick one.
	VerificationAmbiguous VerificationState = "ambiguous"
)

// CanonicalKey is the layer-aware identity used for cross-graph joins. It is
// produced by the identity resolver so that no individual layer invents its
// own identity scheme.
//
// The canonical string form is "<layer>:<kind>:<key>", e.g.
// "repo:function:internal/query/search.go#Run" or
// "memory:observation:ancora:obs:42".
type CanonicalKey struct {
	Layer Layer  `json:"layer"`
	Kind  string `json:"kind"`
	Key   string `json:"key"`
}

// String renders a CanonicalKey in its canonical "<layer>:<kind>:<key>" form.
func (c CanonicalKey) String() string {
	return string(c.Layer) + ":" + c.Kind + ":" + c.Key
}

// IsZero reports whether the key is unset.
func (c CanonicalKey) IsZero() bool {
	return c.Layer == "" && c.Kind == "" && c.Key == ""
}

// ParseCanonicalKey parses the canonical "<layer>:<kind>:<key>" wire form.
func ParseCanonicalKey(raw string) (CanonicalKey, bool) {
	raw = strings.TrimSpace(raw)
	parts := strings.SplitN(raw, ":", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return CanonicalKey{}, false
	}
	key := CanonicalKey{Layer: Layer(parts[0]), Kind: parts[1], Key: parts[2]}
	switch key.Layer {
	case LayerRepo, LayerContract, LayerWorkspace, LayerMemory:
		return key, true
	default:
		return CanonicalKey{}, false
	}
}

// CanonicalKeyForNode resolves a graph node into the stable cross-layer key used
// for joins. It first honors explicit metadata, then falls back to the shared
// layer naming conventions.
func CanonicalKeyForNode(n Node) CanonicalKey {
	if n.Metadata != nil {
		if raw, ok := n.Metadata["canonical_key"].(string); ok {
			if key, ok := ParseCanonicalKey(raw); ok {
				return key
			}
		}
	}
	return CanonicalKeyForID(n.ID, n.NodeType, n.Source)
}

// CanonicalJoinKey returns the stable key callers should use when merging or
// comparing cross-layer entities. It prefers canonical identity and only falls
// back to the trimmed raw ID when the value cannot be canonicalized.
func CanonicalJoinKey(id, kind string, src *Source) string {
	if key := CanonicalKeyForID(id, kind, src); !key.IsZero() {
		return key.String()
	}
	return strings.TrimSpace(id)
}

// CanonicalKeyForID resolves an ID produced by any layer into the stable
// canonical key used for joins.
func CanonicalKeyForID(id, kind string, src *Source) CanonicalKey {
	id = strings.TrimSpace(id)
	if id == "" {
		return CanonicalKey{}
	}
	if strings.HasPrefix(id, "memory:observation:") {
		return CanonicalKey{Layer: LayerMemory, Kind: "observation", Key: "ancora:obs:" + strings.TrimPrefix(id, "memory:observation:")}
	}
	if key, ok := ParseCanonicalKey(id); ok {
		return key
	}

	switch {
	case strings.HasPrefix(id, "ancora:obs:"):
		return CanonicalKey{Layer: LayerMemory, Kind: "observation", Key: id}
	case strings.HasPrefix(id, "memory:concept:"):
		return CanonicalKey{Layer: LayerMemory, Kind: "concept", Key: strings.TrimPrefix(id, "memory:concept:")}
	case strings.HasPrefix(id, "memory:workspace:"):
		return CanonicalKey{Layer: LayerMemory, Kind: "workspace", Key: strings.TrimPrefix(id, "memory:workspace:")}
	case strings.HasPrefix(id, "memory:organization:"):
		return CanonicalKey{Layer: LayerMemory, Kind: "organization", Key: strings.TrimPrefix(id, "memory:organization:")}
	case strings.HasPrefix(id, "memory:visibility:"):
		return CanonicalKey{Layer: LayerMemory, Kind: "visibility", Key: strings.TrimPrefix(id, "memory:visibility:")}
	case strings.HasPrefix(id, "workspace:repo:"):
		return CanonicalKey{Layer: LayerWorkspace, Kind: "repo", Key: strings.TrimPrefix(id, "workspace:repo:")}
	case strings.HasPrefix(id, "workspace:service:"):
		return CanonicalKey{Layer: LayerWorkspace, Kind: "service", Key: strings.TrimPrefix(id, "workspace:service:")}
	case strings.HasPrefix(id, "workspace:package:"):
		return CanonicalKey{Layer: LayerWorkspace, Kind: "package", Key: strings.TrimPrefix(id, "workspace:package:")}
	case strings.HasPrefix(id, "workspace:domain:"):
		return CanonicalKey{Layer: LayerWorkspace, Kind: "domain", Key: strings.TrimPrefix(id, "workspace:domain:")}
	case strings.HasPrefix(id, "contract:service:"):
		return CanonicalKey{Layer: LayerContract, Kind: "service", Key: strings.TrimPrefix(id, "contract:service:")}
	case strings.HasPrefix(id, "contract:endpoint:"):
		return CanonicalKey{Layer: LayerContract, Kind: "endpoint", Key: strings.TrimPrefix(id, "contract:endpoint:")}
	case strings.HasPrefix(id, "contract:rpc:"):
		return CanonicalKey{Layer: LayerContract, Kind: "rpc", Key: strings.TrimPrefix(id, "contract:rpc:")}
	case strings.HasPrefix(id, "project:"):
		return CanonicalKey{Layer: LayerRepo, Kind: "repo", Key: strings.TrimPrefix(id, "project:")}
	}

	if idx := strings.Index(id, ":file:"); idx > 0 {
		return CanonicalKey{Layer: LayerRepo, Kind: "file", Key: id[:idx] + "/" + strings.TrimPrefix(id[idx:], ":file:")}
	}
	if src != nil && src.Name != "" {
		kind = strings.TrimSpace(kind)
		if kind == "" {
			kind = "node"
		}
		return CanonicalKey{Layer: LayerRepo, Kind: kind, Key: src.Name + "/" + id}
	}
	if kind != "" {
		return CanonicalKey{Layer: LayerRepo, Kind: kind, Key: id}
	}
	return CanonicalKey{}
}

// Evidence describes a typed piece of provenance attached to a node or edge.
// Evidence is what makes retrieval explainable and ranking non-arbitrary:
// every relationship should be able to answer "why do you believe this?".
type Evidence struct {
	// Layer is the architectural layer the evidence came from.
	Layer Layer `json:"layer"`

	// Type is the evidence category, e.g. "openapi", "proto", "import",
	// "call", "observation", "embedding".
	Type string `json:"type"`

	// SourceArtifact identifies the file, observation ID, or declared artifact
	// that produced the evidence.
	SourceArtifact string `json:"source_artifact,omitempty"`

	// Confidence ranks this evidence against competing signals.
	Confidence Confidence `json:"confidence"`

	// Verification tracks binding health for memory references; empty for
	// non-memory evidence.
	Verification VerificationState `json:"verification,omitempty"`

	// Score is an optional numeric weight in [0, 1] used by rankers. Absence
	// means "no numeric preference".
	Score float64 `json:"score,omitempty"`
}

// Weight collapses typed evidence into a small ordering weight used when two
// edges claim the same triple or when a caller needs a coarse trust ordering.
func (e Evidence) Weight() float64 {
	weight := 0.0
	switch e.Confidence {
	case ConfidenceDeclared:
		weight = 4.0
	case ConfidenceExtracted:
		weight = 3.0
	case ConfidenceInferred:
		weight = 2.0
	case ConfidenceAmbiguous:
		weight = 1.0
	}

	switch e.Verification {
	case VerificationCurrent:
		weight += 0.30
	case VerificationRedirected:
		weight += 0.15
	case VerificationAmbiguous:
		weight -= 0.20
	case VerificationStale:
		weight -= 0.50
	}

	if e.Score > 0 {
		weight += e.Score / 100.0
	}
	return weight
}

// EdgeEvidence resolves the typed evidence attached to an edge from the shared
// metadata keys plus the legacy wire-level fields that older code still uses.
func EdgeEvidence(e Edge) Evidence {
	ev := Evidence{}
	if e.Metadata != nil {
		if v, ok := e.Metadata["layer"].(string); ok {
			ev.Layer = Layer(v)
		}
		if v, ok := e.Metadata["evidence_type"].(string); ok {
			ev.Type = v
		}
		if v, ok := e.Metadata["evidence_source_artifact"].(string); ok {
			ev.SourceArtifact = v
		}
		if v, ok := e.Metadata["evidence_confidence"].(string); ok {
			ev.Confidence = Confidence(v)
		}
		if v, ok := e.Metadata["verification"].(string); ok {
			ev.Verification = VerificationState(v)
		}
	}
	if ev.Confidence == "" && e.Confidence != "" {
		ev.Confidence = Confidence(strings.ToLower(e.Confidence))
	}
	if ev.Score == 0 {
		ev.Score = e.Score
	}
	return ev
}

// PreferEdgeEvidence reports whether candidate carries stronger typed evidence
// than current for the same logical relationship.
func PreferEdgeEvidence(candidate, current Edge) bool {
	cand := EdgeEvidence(candidate)
	cur := EdgeEvidence(current)
	if cand.Weight() != cur.Weight() {
		return cand.Weight() > cur.Weight()
	}
	if candidate.Score != current.Score {
		return candidate.Score > current.Score
	}
	return CanonicalJoinKey(candidate.Source, "", nil)+candidate.Relation+CanonicalJoinKey(candidate.Target, "", nil) <
		CanonicalJoinKey(current.Source, "", nil)+current.Relation+CanonicalJoinKey(current.Target, "", nil)
}

// RoutingMetadata is carried on workspace-layer nodes and edges so that the
// retrieval orchestrator can pick repos before running deep retrieval. It is
// intentionally small — workspace routing must stay lightweight.
type RoutingMetadata struct {
	// Repos are the repo canonical keys this routing entry suggests.
	Repos []string `json:"repos,omitempty"`

	// Services names declared services involved in the route (contract layer
	// entities, referenced by canonical key).
	Services []string `json:"services,omitempty"`

	// Domains are coarse subject tags such as "billing" or "auth".
	Domains []string `json:"domains,omitempty"`

	// Packages are language-level package names (Go modules, npm packages).
	Packages []string `json:"packages,omitempty"`

	// Dependencies expresses declared dependencies between repos/services.
	Dependencies []string `json:"dependencies,omitempty"`
}

// LayerOf reports the architectural layer a SourceType belongs to. Codebase
// sources belong to the repo layer; memory sources belong to the memory
// layer; webhook sources currently feed the memory layer as timestamped
// observations. Contract and workspace layers are derived rather than sourced
// directly, so they have no default SourceType mapping.
func LayerOf(s SourceType) Layer {
	switch s {
	case SourceTypeCodebase:
		return LayerRepo
	case SourceTypeMemory:
		return LayerMemory
	case SourceTypeWebhook:
		return LayerMemory
	}
	return ""
}

// FactKind classifies the canonical graph-truth relationship carried through the
// scanner -> semantic -> patch -> merge pipeline.
type FactKind string

const (
	FactKindContains   FactKind = "contains"
	FactKindDefines    FactKind = "defines"
	FactKindImports    FactKind = "imports"
	FactKindCalls      FactKind = "calls"
	FactKindReferences FactKind = "references"
	FactKindImplements FactKind = "implements"
	FactKindDependsOn  FactKind = "depends_on"
)

// Provenance records where a canonical fact came from and how trustworthy it is.
// Unlike the legacy memory-oriented contracts, this is strictly code-truth
// oriented: scanner, semantic drivers, patchers, and merge artifacts.
type Provenance struct {
	Stage       string     `json:"stage,omitempty"`
	Driver      string     `json:"driver,omitempty"`
	Source      string     `json:"source,omitempty"`
	Confidence  Confidence `json:"confidence,omitempty"`
	Artifact    string     `json:"artifact,omitempty"`
	Explanation string     `json:"explanation,omitempty"`
}

// Fact is the canonical unit merged into the code-truth graph.
type Fact struct {
	Repo       string                 `json:"repo,omitempty"`
	Language   string                 `json:"language,omitempty"`
	Kind       FactKind               `json:"kind"`
	From       string                 `json:"from"`
	To         string                 `json:"to,omitempty"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
	Provenance []Provenance           `json:"provenance,omitempty"`
}

// StableKey returns the deterministic identity used when deduplicating facts
// across scanner, semantic, and patch sources.
func (f Fact) StableKey() string {
	parts := []string{
		strings.TrimSpace(f.Repo),
		strings.ToLower(strings.TrimSpace(f.Language)),
		string(f.Kind),
		strings.TrimSpace(f.From),
		strings.TrimSpace(f.To),
	}
	return strings.Join(parts, "|")
}

// BuildStage identifies a step in the replacement extraction pipeline.
type BuildStage string

const (
	BuildStageDetect  BuildStage = "detect"
	BuildStageScan    BuildStage = "scan"
	BuildStageDrivers BuildStage = "drivers"
	BuildStagePatch   BuildStage = "patch"
	BuildStageMerge   BuildStage = "merge"
	BuildStagePersist BuildStage = "persist"
)

// BuildRequest defines the pipeline plan for producing a canonical graph-truth
// build. It intentionally models only code-oriented pipeline hooks.
type BuildRequest struct {
	RepoRoot  string       `json:"repo_root"`
	Languages []string     `json:"languages,omitempty"`
	Drivers   []string     `json:"drivers,omitempty"`
	Patchers  []string     `json:"patchers,omitempty"`
	Stages    []BuildStage `json:"stages,omitempty"`
}

// Normalize fills stable defaults and deduplicates pipeline hooks.
func (r BuildRequest) Normalize() BuildRequest {
	if len(r.Stages) == 0 {
		r.Stages = []BuildStage{
			BuildStageDetect,
			BuildStageScan,
			BuildStageDrivers,
			BuildStagePatch,
			BuildStageMerge,
			BuildStagePersist,
		}
	}
	r.Drivers = normalizeStrings(r.Drivers)
	r.Patchers = normalizeStrings(r.Patchers)
	r.Languages = normalizeStrings(r.Languages)
	return r
}

// QueryKind identifies the supported graph-truth query families.
type QueryKind string

const (
	QueryKindDependencies        QueryKind = "dependencies"
	QueryKindReverseDependencies QueryKind = "reverse_dependencies"
	QueryKindPath                QueryKind = "path"
	QueryKindImpact              QueryKind = "impact"
	QueryKindExplain             QueryKind = "explain"
)

const DefaultQueryLimit = 20

// QueryRequest is the canonical input contract for graph-truth queries.
type QueryRequest struct {
	Kind              QueryKind `json:"kind"`
	Subject           string    `json:"subject,omitempty"`
	Target            string    `json:"target,omitempty"`
	Limit             int       `json:"limit,omitempty"`
	IncludeProvenance bool      `json:"include_provenance,omitempty"`
}

// Normalize fills defaults that should be consistent across CLI, MCP, and TUI
// callers once the product is cut over to graph-truth-only queries.
func (r QueryRequest) Normalize() QueryRequest {
	if r.Limit <= 0 {
		r.Limit = DefaultQueryLimit
	}
	if !r.IncludeProvenance {
		r.IncludeProvenance = true
	}
	return r
}

// Validate enforces the minimal shape for each query family.
func (r QueryRequest) Validate() error {
	if r.Kind == "" {
		return errors.New("query kind is required")
	}
	if strings.TrimSpace(r.Subject) == "" {
		return errors.New("query subject is required")
	}
	switch r.Kind {
	case QueryKindDependencies, QueryKindReverseDependencies, QueryKindImpact, QueryKindExplain:
		return nil
	case QueryKindPath:
		if strings.TrimSpace(r.Target) == "" {
			return errors.New("query target is required for path queries")
		}
		return nil
	default:
		return errors.New("unsupported query kind")
	}
}

func normalizeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	sort.Strings(normalized)
	return normalized
}
