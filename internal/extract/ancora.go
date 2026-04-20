package extract

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Syfra3/vela/internal/ancora"
	mgraph "github.com/Syfra3/vela/internal/graph"
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
func orgNodeID(org string) string      { return fmt.Sprintf("ancora:organization:%s", org) }
func conceptNodeID(name string) string {
	return fmt.Sprintf("ancora:concept:%s", strings.ToLower(name))
}

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
	knownRepos := collectKnownRepos(obs)
	prepped := prepareMemoryObservations(obs)
	for i, o := range prepped {
		if progress != nil {
			progress(i, len(prepped), o.Title)
		}
	}

	mem := mgraph.BuildMemory(prepped, mgraph.MemoryOptions{KnownRepos: knownRepos})
	nodes, edges := adaptMemoryGraph(mem)

	// ── LLM-inferred semantic edges ───────────────────────────────────────
	if provider != nil {
		llmEdges, err := inferAncoraRelations(prepped, provider, maxTokens)
		if err == nil {
			edges = append(edges, llmEdges...)
		}
		// LLM errors are non-fatal — structural graph is still useful.
	}

	return nodes, edges, nil
}

func collectKnownRepos(obs []ancora.Observation) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(obs))
	for _, o := range obs {
		ws := strings.TrimSpace(o.Workspace)
		if ws == "" {
			continue
		}
		key := strings.ToLower(ws)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, ws)
	}
	return out
}

func prepareMemoryObservations(obs []ancora.Observation) []ancora.Observation {
	prepared := make([]ancora.Observation, 0, len(obs))
	for _, o := range obs {
		if strings.TrimSpace(o.References) != "" {
			prepared = append(prepared, o)
			continue
		}
		refs := inferWhereWireRefs(o.Content)
		if len(refs) == 0 {
			prepared = append(prepared, o)
			continue
		}
		payload, err := json.Marshal(refs)
		if err != nil {
			prepared = append(prepared, o)
			continue
		}
		o.References = string(payload)
		prepared = append(prepared, o)
	}
	return prepared
}

func adaptMemoryGraph(mem *mgraph.MemoryGraph) ([]types.Node, []types.Edge) {
	if mem == nil {
		return nil, nil
	}

	nodes := make([]types.Node, 0, len(mem.Nodes))
	for _, n := range mem.Nodes {
		n.Source = memorySrc
		if strings.HasPrefix(n.ID, "memory:observation:") {
			if ancoraID, ok := n.Metadata["ancora_id"].(int64); ok {
				n.SourceFile = fmt.Sprintf("ancora:obs:%d", ancoraID)
			}
		}
		nodes = append(nodes, n)
	}

	edges := make([]types.Edge, 0, len(mem.Edges))
	for _, e := range mem.Edges {
		edges = append(edges, e)
	}
	return nodes, edges
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
	refs := inferWhereWireRefs(content)
	if len(refs) == 0 {
		return nil
	}

	relation := obsRefRelation("file", obsType)
	srcFile := fmt.Sprintf("ancora:obs:%d", obsID)

	var edges []types.Edge
	for _, ref := range refs {
		edges = append(edges, types.Edge{
			Source:     obsNodeID(obsID),
			Target:     ref.Target,
			Relation:   relation,
			Confidence: "INFERRED",
			SourceFile: srcFile,
		})
	}
	return edges
}

func inferWhereWireRefs(content string) []ancRef {
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

	var refs []ancRef
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
		refs = append(refs, ancRef{Type: "file", Target: token})
	}
	return refs
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
