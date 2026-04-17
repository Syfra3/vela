package extract

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Syfra3/vela/internal/ancora"
	"github.com/Syfra3/vela/pkg/types"
)

// ─── Node ID helpers ─────────────────────────────────────────────────────────

// memorySrc is the Source stamped on all ancora-derived nodes.
// Type=memory, Name="ancora" — distinct from the ancora codebase which would
// be Type=codebase, Name="ancora" when extracted via path.
var memorySrc = &types.Source{
	Type: types.SourceTypeMemory,
	Name: "ancora",
}

// memoryRootNodeID is the single root node for all ancora memory.
const memoryRootNodeID = "memory:ancora"

func obsNodeID(id int64) string        { return fmt.Sprintf("ancora:obs:%d", id) }
func workspaceNodeID(ws string) string { return fmt.Sprintf("ancora:workspace:%s", ws) }
func visNodeID(vis string) string      { return fmt.Sprintf("ancora:visibility:%s", vis) }
func orgNodeID(org string) string      { return fmt.Sprintf("ancora:org:%s", org) }

// ─── Reference wire type (mirrors ancora IPC wire format) ────────────────────

type ancRef struct {
	Type   string `json:"type"`
	Target string `json:"target"`
}

// ─── Main extractor ──────────────────────────────────────────────────────────

// ExtractAncora reads all observations from the ancora SQLite database and
// returns nodes + edges for the knowledge graph.
//
// Graph structure:
//
//	Hierarchy nodes:  Workspace → Visibility → Organization (when present)
//	Observation nodes: one per observation, linked into the hierarchy
//	Reference edges:  explicit references stored in observations.references
//	LLM edges:        semantic relations inferred by the LLM provider (optional)
//
// provider may be nil — in that case only structural edges are produced.
func ExtractAncora(
	dbPath string,
	provider types.LLMProvider,
	maxTokens int,
	progress func(done, total int, current string),
) ([]types.Node, []types.Edge, error) {

	r, err := ancora.Open(dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open ancora db: %w", err)
	}
	defer r.Close()

	obs, err := r.AllObservations("", "")
	if err != nil {
		return nil, nil, fmt.Errorf("read observations: %w", err)
	}
	if len(obs) == 0 {
		return nil, nil, nil
	}

	// ── Memory root node ─────────────────────────────────────────────────
	// Single root for all ancora memory — distinct from the ancora codebase
	// which would be a project:ancora node extracted from the filesystem.
	memRoot := types.Node{
		ID:          memoryRootNodeID,
		Label:       "Ancora Memory",
		NodeType:    string(types.NodeTypeMemorySource),
		Description: "Persistent memory: decisions, bugs, architecture observations",
		Source:      memorySrc,
	}

	var nodes []types.Node
	var edges []types.Edge

	nodes = append(nodes, memRoot)

	// ── Hierarchy nodes ───────────────────────────────────────────────────
	// Track unique values and per-workspace visibility pairs to avoid
	// the cartesian-product bug (every workspace linked to every visibility).
	workspaces := make(map[string]bool)
	visibilities := make(map[string]bool)
	orgs := make(map[string]bool)
	// wsVisPairs: set of "workspace\x00visibility" seen together in actual obs.
	wsVisPairs := make(map[string]bool)

	for _, o := range obs {
		if o.Workspace != "" {
			workspaces[o.Workspace] = true
		}
		if o.Visibility != "" {
			visibilities[o.Visibility] = true
		}
		if o.Organization != "" {
			orgs[o.Organization] = true
		}
		if o.Workspace != "" && o.Visibility != "" {
			wsVisPairs[o.Workspace+"\x00"+o.Visibility] = true
		}
	}

	for ws := range workspaces {
		nodes = append(nodes, types.Node{
			ID:       workspaceNodeID(ws),
			Label:    ws,
			NodeType: string(types.NodeTypeWorkspace),
			Source:   memorySrc,
		})
		// memory root → workspace
		edges = append(edges, types.Edge{
			Source:   memoryRootNodeID,
			Target:   workspaceNodeID(ws),
			Relation: "contains",
		})
	}
	for vis := range visibilities {
		nodes = append(nodes, types.Node{
			ID:       visNodeID(vis),
			Label:    vis,
			NodeType: string(types.NodeTypeVisibility),
			Source:   memorySrc,
		})
	}
	for org := range orgs {
		nodes = append(nodes, types.Node{
			ID:       orgNodeID(org),
			Label:    org,
			NodeType: string(types.NodeTypeOrganization),
			Source:   memorySrc,
		})
	}

	// Hierarchy structural edges: workspace → visibility (only actual pairs).
	for pair := range wsVisPairs {
		parts := strings.SplitN(pair, "\x00", 2)
		edges = append(edges, types.Edge{
			Source:   workspaceNodeID(parts[0]),
			Target:   visNodeID(parts[1]),
			Relation: "contains",
		})
	}

	// ── Observation nodes + structural edges ──────────────────────────────
	for i, o := range obs {
		if progress != nil {
			progress(i, len(obs), o.Title)
		}

		label := o.Title
		if label == "" {
			label = fmt.Sprintf("obs-%d", o.ID)
		}

		node := types.Node{
			ID:          obsNodeID(o.ID),
			Label:       label,
			NodeType:    string(types.NodeTypeObservation),
			SourceFile:  fmt.Sprintf("ancora:obs:%d", o.ID),
			Description: truncateDescription(o.Content, 300),
			Source:      memorySrc,
			Metadata: map[string]interface{}{
				"ancora_id":    o.ID,
				"obs_type":     o.Type,
				"workspace":    o.Workspace,
				"visibility":   o.Visibility,
				"organization": o.Organization,
				"topic_key":    o.TopicKey,
				"created_at":   o.CreatedAt.Format("2006-01-02"),
			},
		}
		nodes = append(nodes, node)

		// obs → workspace
		if o.Workspace != "" {
			edges = append(edges, types.Edge{
				Source:     obsNodeID(o.ID),
				Target:     workspaceNodeID(o.Workspace),
				Relation:   "belongs_to",
				SourceFile: node.SourceFile,
			})
		}

		// obs → visibility
		if o.Visibility != "" {
			edges = append(edges, types.Edge{
				Source:     obsNodeID(o.ID),
				Target:     visNodeID(o.Visibility),
				Relation:   "scoped_to",
				SourceFile: node.SourceFile,
			})
		}

		// obs → organization
		if o.Organization != "" {
			edges = append(edges, types.Edge{
				Source:     obsNodeID(o.ID),
				Target:     orgNodeID(o.Organization),
				Relation:   "belongs_to",
				SourceFile: node.SourceFile,
			})
		}

		// Explicit references stored in the observation
		refEdges := parseObsReferences(o.ID, o.Type, o.References)
		if len(refEdges) == 0 {
			// Fallback: extract file paths from the **Where**: section of content
			refEdges = parseWhereReferences(o.ID, o.Type, o.Content)
		}
		edges = append(edges, refEdges...)
	}

	// ── LLM-inferred semantic edges ───────────────────────────────────────
	if provider != nil {
		llmEdges, err := inferAncoraRelations(obs, provider, maxTokens)
		if err == nil {
			edges = append(edges, llmEdges...)
		}
		// LLM errors are non-fatal — structural graph is still useful.
	}

	return nodes, edges, nil
}

// parseObsReferences decodes the JSON references string stored in an
// observation and returns graph edges for each explicit reference.
// obsType is used to derive the semantic edge relation for code references.
func parseObsReferences(obsID int64, obsType, refsJSON string) []types.Edge {
	if refsJSON == "" {
		return nil
	}
	var refs []ancRef
	if err := json.Unmarshal([]byte(refsJSON), &refs); err != nil {
		return nil
	}
	var edges []types.Edge
	for _, r := range refs {
		relation := obsRefRelation(r.Type, obsType)
		edges = append(edges, types.Edge{
			Source:     obsNodeID(obsID),
			Target:     r.Target,
			Relation:   relation,
			SourceFile: fmt.Sprintf("ancora:obs:%d", obsID),
		})
	}
	return edges
}

// obsRefRelation maps a reference type + observation category to a semantic EdgeType.
func obsRefRelation(refType, obsType string) string {
	switch refType {
	case "observation":
		return string(types.EdgeTypeRelatedTo)
	case "concept":
		return string(types.EdgeTypeDefines)
	default:
		// file, function — derive from observation category
		switch obsType {
		case "decision":
			return string(types.EdgeTypeDecidesOn)
		case "bugfix":
			return string(types.EdgeTypeConstrains)
		case "pattern":
			return string(types.EdgeTypeExemplifies)
		case "architecture", "discovery", "learning":
			return string(types.EdgeTypeDocuments)
		default:
			return string(types.EdgeTypeReferences)
		}
	}
}

// knownCodeExts is the set of file extensions recognized as code/doc targets
// when parsing free-text **Where**: sections.
var knownCodeExts = map[string]bool{
	".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
	".py": true, ".rs": true, ".java": true, ".kt": true, ".swift": true,
	".md": true, ".yaml": true, ".yml": true, ".json": true, ".toml": true,
}

// parseWhereReferences extracts file-path references from the **Where**: section
// of an observation's content. Used as a fallback when explicit references[] is empty.
func parseWhereReferences(obsID int64, obsType, content string) []types.Edge {
	// Find the **Where**: section.
	const marker = "**Where**:"
	idx := strings.Index(content, marker)
	if idx == -1 {
		return nil
	}
	section := content[idx+len(marker):]

	// Take only text until the next **<field>**: header.
	if next := strings.Index(section, "**"); next != -1 {
		// skip if it's immediately after (e.g. inline bold text inside the value)
		if next > 0 {
			section = section[:next]
		}
	}

	relation := obsRefRelation("file", obsType)
	srcFile := fmt.Sprintf("ancora:obs:%d", obsID)

	var edges []types.Edge
	seen := make(map[string]bool)
	for _, token := range strings.Fields(section) {
		// Strip trailing punctuation/list markers.
		token = strings.Trim(token, ",-•*`\"'()")
		if token == "" || seen[token] {
			continue
		}
		ext := filepath.Ext(token)
		if ext == "" || !knownCodeExts[ext] {
			continue
		}
		seen[token] = true
		edges = append(edges, types.Edge{
			Source:     obsNodeID(obsID),
			Target:     token,
			Relation:   relation,
			Confidence: "INFERRED",
			SourceFile: srcFile,
		})
	}
	return edges
}

// inferAncoraRelations sends batches of observations to the LLM and extracts
// semantic relationships between them. Returns edges only — nodes already exist.
func inferAncoraRelations(
	obs []ancora.Observation,
	provider types.LLMProvider,
	maxTokens int,
) ([]types.Edge, error) {
	if len(obs) == 0 {
		return nil, nil
	}

	// Build a compact index of all observation IDs so the LLM can reference them.
	var indexLines []string
	for _, o := range obs {
		line := fmt.Sprintf("[%d] %s (%s/%s): %s",
			o.ID, o.Title, o.Workspace, o.Visibility,
			truncateDescription(o.Content, 100),
		)
		indexLines = append(indexLines, line)
	}
	fullIndex := strings.Join(indexLines, "\n")

	// Chunk the index to fit within maxTokens (rough 4-char-per-token estimate).
	chunkSize := maxTokens * 3 // conservative chars per chunk
	chunks := chunkText(fullIndex, chunkSize)

	schema := `{
  "relations": [
    {
      "source_id": <ancora observation ID (integer)>,
      "target_id": <ancora observation ID (integer)>,
      "relation":  "<one of: related_to | contradicts | extends | references | implements>"
    }
  ]
}`

	var allEdges []types.Edge
	ctx := context.Background()

	for _, chunk := range chunks {
		prompt := fmt.Sprintf(
			"Below is a list of memory observations from a developer's knowledge base.\n"+
				"Each line is [ID] title (workspace/visibility): content snippet.\n\n"+
				"%s\n\n"+
				"Identify meaningful semantic relationships between these observations.\n"+
				"Return ONLY a JSON object matching this schema — no explanation:\n%s",
			chunk, schema,
		)

		result, err := provider.ExtractGraph(ctx, prompt, schema)
		if err != nil || result == nil {
			continue
		}

		// The LLM returns nodes+edges but we only care about edges here.
		for _, e := range result.Edges {
			// Rewrite source/target to ancora node IDs if they look numeric.
			src := rewriteAncoraID(e.Source)
			tgt := rewriteAncoraID(e.Target)
			if src == "" || tgt == "" || src == tgt {
				continue
			}
			allEdges = append(allEdges, types.Edge{
				Source:     src,
				Target:     tgt,
				Relation:   e.Relation,
				Confidence: "INFERRED",
				SourceFile: "ancora:llm",
			})
		}
	}

	return allEdges, nil
}

// rewriteAncoraID converts a bare numeric ID string (e.g. "42") to the
// canonical ancora node ID format ("ancora:obs:42"). Returns "" if invalid.
func rewriteAncoraID(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "ancora:") {
		return s
	}
	// Try numeric — LLM often returns the raw ID integer
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return s // not numeric, pass through as-is
		}
	}
	if s == "" {
		return ""
	}
	return "ancora:obs:" + s
}

// truncateDescription trims content to maxLen runes for use in node descriptions.
func truncateDescription(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "…"
}

// chunkText splits text into chunks of at most chunkSize bytes.
func chunkText(text string, chunkSize int) []string {
	if len(text) <= chunkSize {
		return []string{text}
	}
	var chunks []string
	for len(text) > 0 {
		end := chunkSize
		if end > len(text) {
			end = len(text)
		}
		// Don't cut mid-line.
		if end < len(text) {
			if idx := strings.LastIndex(text[:end], "\n"); idx > 0 {
				end = idx + 1
			}
		}
		chunks = append(chunks, text[:end])
		text = text[end:]
	}
	return chunks
}
