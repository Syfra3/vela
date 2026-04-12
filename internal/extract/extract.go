package extract

import (
	"log"
	"path/filepath"

	"github.com/Syfra3/vela/pkg/types"
)

// codeExtensions are file extensions handled by AST extractors (no LLM needed).
var codeExtensions = map[string]bool{
	".go":  true,
	".py":  true,
	".ts":  true,
	".tsx": true,
	".js":  true,
	".jsx": true,
}

// docExtensions are file extensions handled by LLM-based extraction.
var docExtensions = map[string]bool{
	".md":  true,
	".txt": true,
	".pdf": true,
}

// ExtractAll dispatches each file to the appropriate extractor and returns the
// merged set of nodes and edges. provider may be nil; if so, doc files are
// skipped. maxChunkTokens controls chunking for local LLMs (0 = default 8000).
func ExtractAll(root string, files []string, provider types.LLMProvider, maxChunkTokens ...int) ([]types.Node, []types.Edge, error) {
	chunkTokens := 8000
	if len(maxChunkTokens) > 0 && maxChunkTokens[0] > 0 {
		chunkTokens = maxChunkTokens[0]
	}

	var allNodes []types.Node
	var allEdges []types.Edge

	for _, path := range files {
		ext := filepath.Ext(path)
		rel := RelPath(root, path)

		switch {
		case codeExtensions[ext]:
			nodes, edges, err := extractCode(path, rel)
			if err != nil {
				log.Printf("[debug] skipping %s: %v", rel, err)
				continue
			}
			allNodes = append(allNodes, nodes...)
			allEdges = append(allEdges, edges...)

		case docExtensions[ext]:
			if provider == nil {
				log.Printf("[debug] skipping doc %s (no LLM provider configured)", rel)
				continue
			}
			var nodes []types.Node
			var edges []types.Edge
			var docErr error
			if ext == ".pdf" {
				nodes, edges, docErr = ExtractPDF(path, rel, provider, chunkTokens)
			} else {
				nodes, edges, docErr = ExtractDoc(path, rel, provider, chunkTokens)
			}
			if docErr != nil {
				log.Printf("[debug] skipping doc %s: %v", rel, docErr)
				continue
			}
			allNodes = append(allNodes, nodes...)
			allEdges = append(allEdges, edges...)

		default:
			log.Printf("[debug] skipping unsupported extension: %s", rel)
		}
	}

	return allNodes, allEdges, nil
}

func extractCode(path, rel string) ([]types.Node, []types.Edge, error) {
	ext := filepath.Ext(path)
	switch ext {
	case ".go":
		root, src, err := ParseGoFile(path)
		if err != nil {
			return nil, nil, err
		}
		return ExtractGoNodes(root, src, rel), ExtractGoEdges(root, src, rel), nil

	case ".py":
		root, src, err := ParsePythonFile(path)
		if err != nil || root == nil {
			return nil, nil, err
		}
		return ExtractPythonNodes(root, src, rel), ExtractPythonEdges(root, src, rel), nil

	case ".ts", ".tsx", ".js", ".jsx":
		root, src, err := ParseTSFile(path)
		if err != nil || root == nil {
			return nil, nil, err
		}
		return ExtractTSNodes(root, src, rel), ExtractTSEdges(root, src, rel), nil
	}
	return nil, nil, nil
}
