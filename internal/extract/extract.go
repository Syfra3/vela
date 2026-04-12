package extract

import (
	"log"
	"path/filepath"

	"github.com/Syfra3/vela/pkg/types"
)

// codeExtensions are file extensions handled by AST extractors (no LLM needed).
var codeExtensions = map[string]bool{
	".go": true,
}

// docExtensions are file extensions handled by LLM-based extraction.
var docExtensions = map[string]bool{
	".md":  true,
	".txt": true,
}

// ExtractAll dispatches each file to the appropriate extractor and returns the
// merged set of nodes and edges. provider may be nil; if so, doc files are
// skipped (Phase 0 behaviour).
func ExtractAll(root string, files []string, provider types.LLMProvider) ([]types.Node, []types.Edge, error) {
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
			// Phase 1+: LLM-based doc extraction (stub for Phase 0)
			log.Printf("[debug] skipping doc %s (LLM doc extraction not yet wired)", rel)

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
		nodes := ExtractGoNodes(root, src, rel)
		edges := ExtractGoEdges(root, src, rel)
		return nodes, edges, nil
	}
	return nil, nil, nil
}
