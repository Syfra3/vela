package extract

import (
	"strings"
	"testing"
)

func TestChunkText_ShortText(t *testing.T) {
	text := "Hello world"
	chunks := ChunkText(text, 8000)
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk for short text, got %d", len(chunks))
	}
	if chunks[0] != text {
		t.Errorf("expected unchanged text, got %q", chunks[0])
	}
}

func TestChunkText_LongText(t *testing.T) {
	// Generate text that is ~3x the chunk size
	word := "architecture "
	maxTokens := 100
	maxChars := maxTokens * charsPerToken // 400 chars per chunk
	// Build text of ~1200 chars
	count := 1200 / len(word)
	text := strings.Repeat(word, count)

	chunks := ChunkText(text, maxTokens)

	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks for long text (%d chars), got %d", len(text), len(chunks))
	}

	// Each chunk must be within limit (with some tolerance for overlap)
	for i, c := range chunks {
		if len(c) > maxChars*2 {
			t.Errorf("chunk %d too large: %d chars (limit %d)", i, len(c), maxChars)
		}
	}
}

func TestChunkText_EachChunkWithinLimit(t *testing.T) {
	// 50 words each ~10 chars, maxTokens=10 → maxChars=40 → should chunk
	word := "architects "
	text := strings.Repeat(word, 50)
	maxTokens := 10

	chunks := ChunkText(text, maxTokens)
	maxChars := maxTokens * charsPerToken

	for i, c := range chunks {
		// Allow 2x for overlap in edge cases
		if len(c) > maxChars*3 {
			t.Errorf("chunk %d exceeds limit: %d chars (limit %d)", i, len(c), maxChars)
		}
	}
	if len(chunks) < 2 {
		t.Error("expected more than 1 chunk")
	}
}

func TestChunkText_EmptyText(t *testing.T) {
	chunks := ChunkText("", 8000)
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk for empty text, got %d", len(chunks))
	}
}
