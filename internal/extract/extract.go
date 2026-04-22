package extract

import (
	"path/filepath"
	"strings"

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

// ExtractAll dispatches each file to the appropriate extractor and returns the
// merged set of nodes and edges.
//
// src describes which project this extraction belongs to. If nil, DetectProject
// is called using root. All returned nodes carry src in Node.Source and have
// their IDs prefixed with "<project>:". A project root node and one file node
// per extracted file are also created and linked via "contains" edges.
//
// provider may be nil; if so, doc files are skipped.
// maxChunkTokens controls chunking for local LLMs (0 = default 8000).
func ExtractAll(
	root string,
	files []string,
	_ types.LLMProvider,
	src *types.Source,
	_ ...int,
) ([]types.Node, []types.Edge, error) {
	if src == nil {
		src = DetectProject(root)
	}

	// Project root node — one per extraction run. Stamp with repo-layer
	// project evidence so downstream consumers can attribute routing and
	// retrieval hits back to the repo rather than a specific artifact.
	projectNode := CreateProjectNode(src)
	stampRepoNode(&projectNode, EvidenceTypeProject, types.ConfidenceDeclared, src.Path)

	var allNodes []types.Node
	var allEdges []types.Edge

	// Track which file nodes we've already emitted (multiple symbols per file).
	emittedFiles := make(map[string]bool)

	for _, path := range files {
		ext := filepath.Ext(path)
		rel := RelPath(root, path)
		evType, evConfidence := evidenceForExt(ext)

		// Emit file nodes even when a file has no extracted symbols. Barrel/index
		// files still matter for file-level dependency graphs and reverse lookups.
		if !emittedFiles[rel] {
			emittedFiles[rel] = true
			fileNode := types.Node{
				ID:         fileNodeID(src.Name, rel),
				Label:      rel,
				NodeType:   string(types.NodeTypeFile),
				SourceFile: rel,
				Source:     src,
			}
			stampRepoNode(&fileNode, EvidenceTypeFilesystem, types.ConfidenceDeclared, rel)
			allNodes = append(allNodes, fileNode)
			containsEdge := types.Edge{
				Source:   projectNode.ID,
				Target:   fileNode.ID,
				Relation: "contains",
			}
			stampRepoEdge(&containsEdge, EvidenceTypeFilesystem, types.ConfidenceDeclared, rel)
			allEdges = append(allEdges, containsEdge)
		}

		var rawNodes []types.Node
		var rawEdges []types.Edge

		switch {
		case codeExtensions[ext]:
			n, e, err := extractCode(path, rel)
			if err != nil {
				// Individual parse failures should not spam the product UI when a
				// directory contains fixture or intentionally invalid source files.
				continue
			}
			rawNodes, rawEdges = n, e

		default:
			continue
		}

		for _, fileEdge := range extractFileEdges(path, root, src.Name, rel) {
			stampRepoEdge(&fileEdge, evType, evConfidence, rel)
			allEdges = append(allEdges, fileEdge)
		}

		if len(rawNodes) == 0 {
			continue
		}

		// Prefix all node IDs and stamp Source + evidence metadata.
		prefixed := make([]types.Node, 0, len(rawNodes))
		for _, n := range rawNodes {
			n.ID = prefixID(src.Name, n.ID)
			n.Source = src
			stampRepoNode(&n, evType, evConfidence, rel)
			prefixed = append(prefixed, n)
			// file → symbol
			fileContains := types.Edge{
				Source:     fileNodeID(src.Name, rel),
				Target:     n.ID,
				Relation:   "contains",
				SourceFile: rel,
			}
			stampRepoEdge(&fileContains, EvidenceTypeFilesystem, types.ConfidenceDeclared, rel)
			allEdges = append(allEdges, fileContains)
		}
		allNodes = append(allNodes, prefixed...)

		// Rewrite edge Sources (always file-qualified IDs); keep Targets bare
		// so Build() can resolve them — cross-file and cross-project calls resolve
		// by label in the label index.
		for _, e := range rawEdges {
			e.Source = prefixID(src.Name, e.Source)
			// Target stays as bare callee label — Build() resolves via labelIndex.
			stampRepoEdge(&e, evType, evConfidence, rel)
			allEdges = append(allEdges, e)
		}
	}

	// Prepend project node so it's always ID 1 in the graph (stable ordering).
	result := make([]types.Node, 0, 1+len(allNodes))
	result = append(result, projectNode)
	result = append(result, allNodes...)

	return result, allEdges, nil
}

// ── ID helpers ────────────────────────────────────────────────────────────────

// prefixID prepends "<project>:" to a raw node ID produced by an AST extractor.
// If the ID already has the prefix, it is returned unchanged.
func prefixID(project, id string) string {
	prefix := project + ":"
	if strings.HasPrefix(id, prefix) {
		return id
	}
	return prefix + id
}

// fileNodeID returns the canonical node ID for a file node.
func fileNodeID(project, relPath string) string {
	return project + ":file:" + relPath
}

// ── Raw code extraction (unchanged signatures — project prefix applied above) ──

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
