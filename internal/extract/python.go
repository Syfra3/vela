package extract

import (
	"os"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"

	"github.com/Syfra3/vela/pkg/types"
)

var pyLang = python.GetLanguage()

// ParsePythonFile parses a .py source file and returns the root AST node.
func ParsePythonFile(path string) (*sitter.Node, []byte, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	root := sitter.Parse(src, pyLang)
	if root == nil {
		return nil, nil, nil
	}
	return root, src, nil
}

// ExtractPythonNodes extracts function_definition → function nodes and
// class_definition → struct nodes from a Python AST.
func ExtractPythonNodes(root *sitter.Node, src []byte, relFile string) []types.Node {
	var nodes []types.Node

	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Type() {
		case "function_definition":
			name := childFieldText(n, src, "name")
			if name != "" {
				nodes = append(nodes, types.Node{
					ID:         relFile + ":" + name,
					Label:      name,
					NodeType:   "function",
					SourceFile: relFile,
				})
			}
		case "class_definition":
			name := childFieldText(n, src, "name")
			if name != "" {
				nodes = append(nodes, types.Node{
					ID:         relFile + ":" + name,
					Label:      name,
					NodeType:   "struct", // class → struct in our model
					SourceFile: relFile,
				})
			}
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}

	walk(root)
	return nodes
}

// ExtractPythonEdges extracts call edges from Python function bodies.
func ExtractPythonEdges(root *sitter.Node, src []byte, relFile string) []types.Edge {
	var edges []types.Edge
	var currentFunc string

	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Type() {
		case "function_definition":
			prev := currentFunc
			currentFunc = childFieldText(n, src, "name")
			for i := 0; i < int(n.ChildCount()); i++ {
				walk(n.Child(i))
			}
			currentFunc = prev
			return

		case "call":
			if currentFunc != "" {
				callee := pythonCalleeLabel(n, src)
				if callee != "" && callee != currentFunc {
					edges = append(edges, types.Edge{
						Source:     relFile + ":" + currentFunc,
						Target:     callee,
						Relation:   "calls",
						Confidence: "EXTRACTED",
						SourceFile: relFile,
					})
				}
			}
		}

		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}

	walk(root)
	return edges
}

// pythonCalleeLabel extracts the function name from a Python call node.
// Handles: identifier, attribute (obj.method)
func pythonCalleeLabel(callNode *sitter.Node, src []byte) string {
	fn := callNode.ChildByFieldName("function")
	if fn == nil {
		return ""
	}
	switch fn.Type() {
	case "identifier":
		return string(src[fn.StartByte():fn.EndByte()])
	case "attribute":
		attr := fn.ChildByFieldName("attribute")
		if attr != nil {
			return string(src[attr.StartByte():attr.EndByte()])
		}
	}
	return ""
}
