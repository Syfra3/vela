package extract

import (
	"context"
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"

	"github.com/Syfra3/vela/pkg/types"
)

// ExtractPDF reads a PDF file, converts it to plain text, chunks it, and sends
// each chunk to the LLM provider for NER + RE extraction.
func ExtractPDF(path string, relFile string, provider types.LLMProvider, maxChunkTokens int) ([]types.Node, []types.Edge, error) {
	if provider == nil {
		return nil, nil, fmt.Errorf("no LLM provider configured for PDF extraction")
	}

	text, err := pdfToText(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading PDF %s: %w", path, err)
	}
	if strings.TrimSpace(text) == "" {
		return nil, nil, nil // empty or image-only PDF
	}

	chunks := ChunkText(text, maxChunkTokens)
	schema := extractionSchemaJSON()

	var allNodes []types.Node
	var allEdges []types.Edge

	for _, chunk := range chunks {
		result, err := provider.ExtractGraph(context.Background(), chunk, schema)
		if err != nil {
			// Log and continue — one bad chunk should not abort the PDF
			continue
		}
		if result == nil {
			continue
		}
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

// pdfToText extracts plain text from all pages of a PDF file.
func pdfToText(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var sb strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue // skip unreadable pages
		}
		sb.WriteString(text)
		sb.WriteString("\n")
	}
	return sb.String(), nil
}
