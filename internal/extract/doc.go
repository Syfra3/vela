package extract

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Syfra3/vela/pkg/types"
)

const (
	// charsPerToken is a rough estimate used to split docs for local LLMs.
	charsPerToken = 4
	// chunkOverlapRatio is the fraction of previous chunk prepended to next chunk.
	chunkOverlapRatio = 0.10
)

// ExtractDoc reads a markdown or text file, chunks it if necessary, and sends
// each chunk to the LLM provider for NER + RE extraction.
func ExtractDoc(path string, relFile string, provider types.LLMProvider, maxChunkTokens int) ([]types.Node, []types.Edge, error) {
	if provider == nil {
		return nil, nil, fmt.Errorf("no LLM provider configured")
	}

	src, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", path, err)
	}
	text := string(src)

	chunks := ChunkText(text, maxChunkTokens)

	var allNodes []types.Node
	var allEdges []types.Edge

	for _, chunk := range chunks {
		result, err := provider.ExtractGraph(context.Background(), chunk, extractionSchemaJSON())
		if err != nil {
			// Log and continue — one bad chunk should not abort the file
			continue
		}
		if result == nil {
			continue
		}

		// Tag all nodes/edges with this source file
		for i := range result.Nodes {
			if result.Nodes[i].SourceFile == "" {
				result.Nodes[i].SourceFile = relFile
			}
			if result.Nodes[i].ID == "" {
				result.Nodes[i].ID = relFile + ":" + result.Nodes[i].Label
			}
		}
		for i := range result.Edges {
			if result.Edges[i].SourceFile == "" {
				result.Edges[i].SourceFile = relFile
			}
		}

		allNodes = append(allNodes, result.Nodes...)
		allEdges = append(allEdges, result.Edges...)
	}

	return allNodes, allEdges, nil
}

// ChunkText splits text into chunks of at most maxTokens (estimated via
// charsPerToken). Each chunk overlaps with the previous by chunkOverlapRatio
// of the previous chunk's length to preserve context at boundaries.
// If the entire text fits within maxTokens, a single-element slice is returned.
func ChunkText(text string, maxTokens int) []string {
	if maxTokens <= 0 {
		maxTokens = 8000
	}
	maxChars := maxTokens * charsPerToken

	if len(text) <= maxChars {
		return []string{text}
	}

	var chunks []string
	var overlap string

	remaining := text
	for len(remaining) > 0 {
		// Prepend overlap from previous chunk
		candidate := overlap + remaining

		var chunk string
		if len(candidate) <= maxChars {
			chunk = candidate
			remaining = ""
		} else {
			// Split at a word boundary within maxChars
			splitAt := maxChars
			if splitAt < len(candidate) {
				// Walk back to the nearest space
				for splitAt > 0 && candidate[splitAt] != ' ' && candidate[splitAt] != '\n' {
					splitAt--
				}
				if splitAt == 0 {
					splitAt = maxChars // no space found, hard-cut
				}
			}
			chunk = candidate[:splitAt]
			// Advance remaining by the non-overlapping portion consumed
			consumed := splitAt - len(overlap)
			if consumed <= 0 {
				consumed = splitAt
			}
			remaining = remaining[min(consumed, len(remaining)):]
		}

		chunks = append(chunks, strings.TrimSpace(chunk))

		// Compute overlap: last chunkOverlapRatio of this chunk
		overlapLen := int(float64(len(chunk)) * chunkOverlapRatio)
		if overlapLen > 0 && len(chunk) > overlapLen {
			overlap = chunk[len(chunk)-overlapLen:]
		} else {
			overlap = ""
		}
	}

	return chunks
}

// extractionSchemaJSON returns the JSON schema string used in LLM prompts.
// Kept here so doc.go and llm/ share the same contract.
func extractionSchemaJSON() string {
	return `{
  "type": "object",
  "properties": {
    "nodes": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "id":    {"type": "string"},
          "label": {"type": "string"},
          "type":  {"type": "string", "enum": ["function","class","concept","file","module","interface","struct","constant"]},
          "description": {"type": "string"}
        },
        "required": ["id","label","type"]
      }
    },
    "edges": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "source":     {"type": "string"},
          "target":     {"type": "string"},
          "relation":   {"type": "string", "enum": ["calls","imports","uses","implements","extends","describes","related_to"]},
          "confidence": {"type": "string", "enum": ["EXTRACTED","INFERRED","AMBIGUOUS"]},
          "score":      {"type": "number", "minimum": 0, "maximum": 1}
        },
        "required": ["source","target","relation","confidence"]
      }
    }
  },
  "required": ["nodes","edges"]
}`
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
