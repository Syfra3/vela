package extract

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"

	"github.com/Syfra3/vela/pkg/types"
)

var goLang = golang.GetLanguage()

// ParseGoFile parses a .go source file using tree-sitter and returns the root
// AST node along with the raw source bytes.
func ParseGoFile(path string) (*sitter.Node, []byte, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", path, err)
	}

	root := sitter.Parse(src, goLang)
	if root == nil {
		return nil, nil, fmt.Errorf("nil root node for %s", path)
	}
	return root, src, nil
}

// ExtractGoNodes extracts named function declarations, struct type declarations,
// and interface type declarations from the given tree-sitter AST root.
func ExtractGoNodes(root *sitter.Node, src []byte, relFile string) []types.Node {
	var nodes []types.Node

	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Type() {
		case "function_declaration", "method_declaration":
			name := childFieldText(n, src, "name")
			if name != "" {
				nodes = append(nodes, types.Node{
					ID:         relFile + ":" + name,
					Label:      name,
					NodeType:   nodeKindForFunc(n),
					SourceFile: relFile,
				})
			}
		case "type_declaration":
			// type_declaration → type_spec → (struct_type | interface_type)
			for i := 0; i < int(n.ChildCount()); i++ {
				spec := n.Child(i)
				if spec.Type() != "type_spec" {
					continue
				}
				name := childFieldText(spec, src, "name")
				typeNode := spec.ChildByFieldName("type")
				if name == "" || typeNode == nil {
					continue
				}
				var kind string
				switch typeNode.Type() {
				case "struct_type":
					kind = "struct"
				case "interface_type":
					kind = "interface"
				default:
					continue
				}
				nodes = append(nodes, types.Node{
					ID:         relFile + ":" + name,
					Label:      name,
					NodeType:   kind,
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

// ExtractGoEdges extracts function call edges from the tree-sitter AST.
// Edges represent caller → callee relationships within the file.
func ExtractGoEdges(root *sitter.Node, src []byte, relFile string) []types.Edge {
	var edges []types.Edge

	var currentFunc string

	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Type() {
		case "function_declaration", "method_declaration":
			prev := currentFunc
			currentFunc = childFieldText(n, src, "name")
			for i := 0; i < int(n.ChildCount()); i++ {
				walk(n.Child(i))
			}
			currentFunc = prev
			return

		case "call_expression":
			if currentFunc != "" {
				callee := calleeLabel(n, src)
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

// --- helpers ---

func nodeKindForFunc(n *sitter.Node) string {
	if n.Type() == "method_declaration" {
		return "method"
	}
	return "function"
}

// childFieldText returns the source text of the named field child of n.
func childFieldText(n *sitter.Node, src []byte, field string) string {
	child := n.ChildByFieldName(field)
	if child == nil {
		return ""
	}
	return string(src[child.StartByte():child.EndByte()])
}

// calleeLabel extracts a readable label for the called function.
// For selector expressions (pkg.Func or receiver.Method) returns "Func"/"Method".
// For plain identifiers returns the name.
func calleeLabel(callExpr *sitter.Node, src []byte) string {
	fn := callExpr.ChildByFieldName("function")
	if fn == nil {
		return ""
	}
	switch fn.Type() {
	case "identifier":
		return string(src[fn.StartByte():fn.EndByte()])
	case "selector_expression":
		field := fn.ChildByFieldName("field")
		if field != nil {
			return string(src[field.StartByte():field.EndByte()])
		}
	}
	// fallback: trim to last component
	raw := strings.TrimSpace(string(src[fn.StartByte():fn.EndByte()]))
	if idx := strings.LastIndex(raw, "."); idx >= 0 {
		return raw[idx+1:]
	}
	return raw
}

// RelPath returns path relative to root, or path if it cannot be made relative.
func RelPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}
