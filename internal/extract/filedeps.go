package extract

import (
	"bufio"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Syfra3/vela/pkg/types"
)

func extractFileEdges(absPath, root, project, rel string) []types.Edge {
	ext := strings.ToLower(filepath.Ext(absPath))
	var targets []string
	switch ext {
	case ".go":
		targets = extractGoFileDeps(absPath, root)
	case ".ts", ".tsx", ".js", ".jsx":
		targets = extractTSFileDeps(absPath, root)
	case ".py":
		targets = extractPythonFileDeps(absPath, root)
	default:
		return nil
	}
	if len(targets) == 0 {
		return nil
	}
	sort.Strings(targets)
	edges := make([]types.Edge, 0, len(targets))
	for _, target := range targets {
		if target == "" || target == rel {
			continue
		}
		edges = append(edges, types.Edge{
			Source:     fileNodeID(project, rel),
			Target:     fileNodeID(project, target),
			Relation:   string(types.FactKindDependsOn),
			Confidence: string(types.ConfidenceExtracted),
			SourceFile: rel,
			Metadata: map[string]interface{}{
				"projected_from": "static_import",
				"target_file":    target,
			},
		})
	}
	return edges
}

func extractGoFileDeps(absPath, root string) []string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, absPath, nil, parser.ImportsOnly)
	if err != nil {
		return nil
	}
	modulePath, moduleRoot := detectGoModule(root)
	if moduleRoot == "" {
		moduleRoot = root
	}
	deps := make([]string, 0, len(file.Imports))
	for _, spec := range file.Imports {
		importPath := strings.Trim(spec.Path.Value, "\"")
		if importPath == "" {
			continue
		}
		relDir := resolveGoImportDir(importPath, modulePath, moduleRoot)
		if relDir == "" {
			continue
		}
		if entry := representativeFileForDir(filepath.Join(moduleRoot, relDir), ".go"); entry != "" {
			deps = append(deps, filepath.ToSlash(filepath.Join(relDir, entry)))
		}
	}
	return uniqueStrings(deps)
}

func extractTSFileDeps(absPath, root string) []string {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil
	}
	deps := make([]string, 0)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.Contains(line, " from ") && !strings.HasPrefix(line, "import ") && !strings.HasPrefix(line, "export ") {
			continue
		}
		spec := quotedModuleSpecifier(line)
		if spec == "" || !strings.HasPrefix(spec, ".") {
			if target := resolveWorkspacePackage(root, spec); target != "" {
				deps = append(deps, target)
			}
			continue
		}
		if target := resolveTSModule(filepath.Dir(absPath), root, spec); target != "" {
			deps = append(deps, target)
		}
	}
	return uniqueStrings(deps)
}

func extractPythonFileDeps(absPath, root string) []string {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil
	}
	deps := make([]string, 0)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		var mod string
		switch {
		case strings.HasPrefix(line, "from "):
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				mod = parts[1]
			}
		case strings.HasPrefix(line, "import "):
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				mod = strings.Split(parts[1], ",")[0]
			}
		}
		if mod == "" || strings.HasPrefix(mod, ".") || strings.Contains(mod, ".") == false {
			continue
		}
		candidate := filepath.Join(root, filepath.FromSlash(strings.ReplaceAll(mod, ".", "/"))+".py")
		if rel := relIfFile(candidate, root); rel != "" {
			deps = append(deps, rel)
		}
	}
	return uniqueStrings(deps)
}

func detectGoModule(root string) (string, string) {
	path := filepath.Join(root, "go.mod")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), root
		}
	}
	return "", ""
}

func resolveGoImportDir(importPath, modulePath, moduleRoot string) string {
	if modulePath == "" {
		if strings.HasPrefix(importPath, "internal/") || strings.HasPrefix(importPath, "pkg/") || strings.HasPrefix(importPath, "cmd/") {
			return filepath.ToSlash(importPath)
		}
		return ""
	}
	if !strings.HasPrefix(importPath, modulePath) {
		return ""
	}
	rel := strings.TrimPrefix(importPath, modulePath)
	rel = strings.TrimPrefix(rel, "/")
	return filepath.ToSlash(rel)
}

func representativeFileForDir(absDir, ext string) string {
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return ""
	}
	best := ""
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.ToLower(filepath.Ext(name)) != ext {
			continue
		}
		if best == "" || name < best {
			best = name
		}
		base := filepath.Base(absDir) + ext
		if name == base || name == "index"+ext || name == "types"+ext || name == "config"+ext {
			return name
		}
	}
	return best
}

func quotedModuleSpecifier(line string) string {
	for _, quote := range []string{"\"", "'"} {
		parts := strings.Split(line, quote)
		if len(parts) >= 3 {
			candidate := strings.TrimSpace(parts[1])
			if candidate != "" {
				return candidate
			}
		}
	}
	return ""
}

func resolveTSModule(baseDir, root, spec string) string {
	resolved := filepath.Clean(filepath.Join(baseDir, filepath.FromSlash(spec)))
	candidates := []string{
		resolved,
		resolved + ".ts",
		resolved + ".tsx",
		resolved + ".js",
		resolved + ".jsx",
		filepath.Join(resolved, "index.ts"),
		filepath.Join(resolved, "index.tsx"),
		filepath.Join(resolved, "index.js"),
		filepath.Join(resolved, "index.jsx"),
	}
	for _, candidate := range candidates {
		if rel := relIfFile(candidate, root); rel != "" {
			return rel
		}
	}
	return ""
}

type packageJSON struct {
	Name        string `json:"name"`
	ReactNative string `json:"react-native"`
	Exports     map[string]struct {
		Source string `json:"source"`
		Next   string `json:"next"`
		Types  string `json:"types"`
	} `json:"exports"`
}

func resolveWorkspacePackage(root, spec string) string {
	packages := discoverWorkspacePackages(root)
	entry, ok := packages[strings.TrimSpace(spec)]
	if !ok {
		return ""
	}
	return entry
}

func discoverWorkspacePackages(root string) map[string]string {
	packages := make(map[string]string)
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == "node_modules" || name == ".git" || name == "dist" || name == "build" || strings.HasPrefix(name, ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "package.json" {
			return nil
		}
		entry := workspacePackageEntry(path, root)
		if entry.name != "" && entry.target != "" {
			packages[entry.name] = entry.target
		}
		return nil
	})
	return packages
}

type workspaceEntry struct {
	name   string
	target string
}

func workspacePackageEntry(packageJSONPath, root string) workspaceEntry {
	data, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return workspaceEntry{}
	}
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return workspaceEntry{}
	}
	if strings.TrimSpace(pkg.Name) == "" {
		return workspaceEntry{}
	}
	packageDir := filepath.Dir(packageJSONPath)
	candidates := []string{}
	if exp, ok := pkg.Exports["."]; ok {
		candidates = append(candidates, exp.Source, exp.Next, exp.Types)
	}
	candidates = append(candidates, pkg.ReactNative, "src/index.ts", "src/index.tsx", "src/index.js", "src/index.jsx")
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if rel := relIfFile(filepath.Join(packageDir, filepath.FromSlash(candidate)), root); rel != "" {
			return workspaceEntry{name: pkg.Name, target: rel}
		}
	}
	return workspaceEntry{}
}

func relIfFile(absPath, root string) string {
	info, err := os.Stat(absPath)
	if err != nil || info.IsDir() {
		return ""
	}
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(rel)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

var _ = fmt.Sprintf
