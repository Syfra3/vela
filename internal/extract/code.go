package extract

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/Syfra3/vela/pkg/types"
)

// parsedGoFile holds the parsed AST for a Go file.
type parsedGoFile struct {
	file *ast.File
	fset *token.FileSet
}

// ParseGoFile parses a .go source file using go/ast (pure Go, no CGO).
func ParseGoFile(path string) (*parsedGoFile, []byte, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", path, err)
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	return &parsedGoFile{file: f, fset: fset}, src, nil
}

// ExtractGoNodes extracts named function declarations, struct type declarations,
// and interface type declarations from a parsed Go file.
func ExtractGoNodes(parsed *parsedGoFile, _ []byte, relFile string) []types.Node {
	var nodes []types.Node

	ast.Inspect(parsed.file, func(n ast.Node) bool {
		switch decl := n.(type) {
		case *ast.FuncDecl:
			if decl.Name == nil {
				return true
			}
			kind := "function"
			if decl.Recv != nil {
				kind = "method"
			}
			nodes = append(nodes, types.Node{
				ID:         relFile + ":" + decl.Name.Name,
				Label:      decl.Name.Name,
				NodeType:   kind,
				SourceFile: relFile,
			})

		case *ast.GenDecl:
			if decl.Tok != token.TYPE {
				return true
			}
			for _, spec := range decl.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || ts.Name == nil {
					continue
				}
				var kind string
				switch ts.Type.(type) {
				case *ast.StructType:
					kind = "struct"
				case *ast.InterfaceType:
					kind = "interface"
				default:
					continue
				}
				nodes = append(nodes, types.Node{
					ID:         relFile + ":" + ts.Name.Name,
					Label:      ts.Name.Name,
					NodeType:   kind,
					SourceFile: relFile,
				})
			}
		}
		return true
	})

	return nodes
}

// ExtractGoEdges extracts function call edges from a parsed Go AST.
func ExtractGoEdges(parsed *parsedGoFile, _ []byte, relFile string) []types.Edge {
	var edges []types.Edge

	for _, decl := range parsed.file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Body == nil {
			continue
		}
		caller := fn.Name.Name

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			callee := goCalleeLabel(call)
			if callee == "" || callee == caller {
				return true
			}
			edges = append(edges, types.Edge{
				Source:     relFile + ":" + caller,
				Target:     callee,
				Relation:   "calls",
				Confidence: "EXTRACTED",
				SourceFile: relFile,
			})
			return true
		})
	}

	return edges
}

func goCalleeLabel(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	}
	raw := fmt.Sprintf("%v", call.Fun)
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
