package extract

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/Syfra3/vela/internal/llm"
	"github.com/Syfra3/vela/pkg/types"
)

// ---------------------------------------------------------------------------
// LLM-based reference extractor (spec §6.4 / task 15-16)
// ---------------------------------------------------------------------------

// LLMExtractor uses an LLM to find implicit references in an observation's
// content that are not captured by the explicit reference parser.
// It runs as a worker pool consuming from the Patcher's LLM queue.
type LLMExtractor struct {
	client      types.LLMProvider
	workerCount int

	// onRefsFound is called after extraction with the discovered references.
	// Implementations may write them back to Ancora or patch the graph directly.
	onRefsFound func(ancoraID int64, refs []types.ObsReference)
}

// NewLLMExtractor creates an extractor using the provided LLM client.
// workerCount controls how many parallel extraction goroutines are run.
func NewLLMExtractor(cfg *types.LLMConfig, workerCount int, onRefsFound func(int64, []types.ObsReference)) (*LLMExtractor, error) {
	if workerCount <= 0 {
		workerCount = 2
	}
	client, err := llm.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("llm client: %w", err)
	}
	return &LLMExtractor{
		client:      client,
		workerCount: workerCount,
		onRefsFound: onRefsFound,
	}, nil
}

// Start launches worker goroutines that consume from queue.
// It returns when ctx is cancelled or queue is closed.
func (e *LLMExtractor) Start(ctx context.Context, queue <-chan types.ObservationNode) {
	var wg sync.WaitGroup
	for i := 0; i < e.workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case obs, ok := <-queue:
					if !ok {
						return
					}
					refs, err := e.extract(ctx, obs)
					if err != nil {
						log.Printf("[llm_refs] extract obs %d: %v", obs.AncoraID, err)
						continue
					}
					if len(refs) > 0 && e.onRefsFound != nil {
						e.onRefsFound(obs.AncoraID, refs)
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	wg.Wait()
}

const extractionPromptTemplate = `Extract references from this observation. Return ONLY a JSON array.

Observation:
Title: %s
Content: %s

Extract:
- File paths (type: "file")
- Function or struct names (type: "function")
- Concept names (type: "concept")
- Related observation IDs if mentioned as #N (type: "observation")

Response format — JSON array only, no prose:
[{"type": "file", "target": "internal/store/store.go"}, ...]`

// extract calls the LLM and parses the resulting reference list.
func (e *LLMExtractor) extract(ctx context.Context, obs types.ObservationNode) ([]types.ObsReference, error) {
	prompt := fmt.Sprintf(extractionPromptTemplate, obs.Title, obs.Content)

	// Re-use the ExtractGraph interface by wrapping in a minimal schema call.
	// We ask for a graph but only care about the edges (references).
	schema := `{"type":"array","items":{"type":"object","properties":{"type":{"type":"string"},"target":{"type":"string"}}}}`
	result, err := e.client.ExtractGraph(ctx, prompt, schema)
	if err != nil {
		return nil, err
	}

	// The LLM returns nodes/edges but we repurpose the raw response.
	// Re-marshal nodes to extract the reference list from the LLM output.
	// In practice the LLM may return a raw JSON array — try direct parse.
	if len(result.Nodes) == 0 {
		return nil, nil
	}

	// Attempt to unmarshal the first node's description as a JSON array of refs.
	var refs []types.ObsReference
	if err := json.Unmarshal([]byte(result.Nodes[0].Description), &refs); err != nil {
		// Fallback: build refs from nodes returned.
		for _, n := range result.Nodes {
			refs = append(refs, types.ObsReference{
				Type:   n.NodeType,
				Target: n.Label,
			})
		}
	}

	// Merge with parser output to avoid duplicates.
	explicit := ParseReferences(obs.Title, obs.Content)
	return mergeRefs(refs, explicit), nil
}

// mergeRefs deduplicates two reference slices, preferring LLM results.
func mergeRefs(llmRefs, parsedRefs []types.ObsReference) []types.ObsReference {
	seen := make(map[string]bool, len(llmRefs)+len(parsedRefs))
	out := make([]types.ObsReference, 0, len(llmRefs)+len(parsedRefs))
	for _, r := range llmRefs {
		k := r.Type + ":" + r.Target
		if !seen[k] {
			seen[k] = true
			out = append(out, r)
		}
	}
	for _, r := range parsedRefs {
		k := r.Type + ":" + r.Target
		if !seen[k] {
			seen[k] = true
			out = append(out, r)
		}
	}
	return out
}
