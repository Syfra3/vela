package extract

import (
	"os"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"
	tstypescript "github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/Syfra3/vela/pkg/types"
)

var tsLang = tstypescript.GetLanguage()
var jsLang = javascript.GetLanguage()

// ParseTSFile parses a TypeScript or JavaScript file using tree-sitter.
// Pass the appropriate language based on the file extension.
func ParseTSFile(path string) (*sitter.Node, []byte, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	lang := tsLang
	if isJS(path) {
		lang = jsLang
	}

	root := sitter.Parse(src, lang)
	if root == nil {
		return nil, nil, nil
	}
	return root, src, nil
}

// ExtractTSNodes extracts functions, arrow functions, classes and interfaces.
func ExtractTSNodes(root *sitter.Node, src []byte, relFile string) []types.Node {
	var nodes []types.Node

	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Type() {
		case "function_declaration":
			name := childFieldText(n, src, "name")
			if name != "" {
				nodes = append(nodes, types.Node{
					ID: relFile + ":" + name, Label: name,
					NodeType: "function", SourceFile: relFile,
				})
			}

		case "method_definition":
			name := childFieldText(n, src, "name")
			if name != "" && name != "constructor" {
				nodes = append(nodes, types.Node{
					ID: relFile + ":" + name, Label: name,
					NodeType: "method", SourceFile: relFile,
				})
			}

		case "lexical_declaration", "variable_declaration":
			// Capture: const foo = () => {} or const foo = function() {}
			for i := 0; i < int(n.ChildCount()); i++ {
				decl := n.Child(i)
				if decl.Type() != "variable_declarator" {
					continue
				}
				name := childFieldText(decl, src, "name")
				val := decl.ChildByFieldName("value")
				if name == "" || val == nil {
					continue
				}
				if val.Type() == "arrow_function" || val.Type() == "function" || val.Type() == "function_expression" {
					nodes = append(nodes, types.Node{
						ID: relFile + ":" + name, Label: name,
						NodeType: "function", SourceFile: relFile,
					})
				}
			}

		case "class_declaration":
			name := childFieldText(n, src, "name")
			if name != "" {
				nodes = append(nodes, types.Node{
					ID: relFile + ":" + name, Label: name,
					NodeType: "struct", SourceFile: relFile,
				})
			}

		case "interface_declaration":
			name := childFieldText(n, src, "name")
			if name != "" {
				nodes = append(nodes, types.Node{
					ID: relFile + ":" + name, Label: name,
					NodeType: "interface", SourceFile: relFile,
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

// ExtractTSEdges extracts call edges from TypeScript/JavaScript files.
func ExtractTSEdges(root *sitter.Node, src []byte, relFile string) []types.Edge {
	var edges []types.Edge
	var currentFunc string

	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Type() {
		case "function_declaration", "method_definition":
			prev := currentFunc
			currentFunc = childFieldText(n, src, "name")
			for i := 0; i < int(n.ChildCount()); i++ {
				walk(n.Child(i))
			}
			currentFunc = prev
			return

		case "call_expression":
			if currentFunc != "" {
				callee := tsCalleeLabel(n, src)
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

// tsCalleeLabel extracts a readable name from a TS/JS call_expression.
func tsCalleeLabel(callExpr *sitter.Node, src []byte) string {
	fn := callExpr.ChildByFieldName("function")
	if fn == nil {
		return ""
	}
	switch fn.Type() {
	case "identifier":
		return string(src[fn.StartByte():fn.EndByte()])
	case "member_expression":
		prop := fn.ChildByFieldName("property")
		if prop != nil {
			return string(src[prop.StartByte():prop.EndByte()])
		}
	}
	raw := strings.TrimSpace(string(src[fn.StartByte():fn.EndByte()]))
	if idx := strings.LastIndex(raw, "."); idx >= 0 {
		return raw[idx+1:]
	}
	return raw
}

func isJS(path string) bool {
	return strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".jsx")
}
