package generation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/Syfra3/vela/internal/detect"
	"github.com/Syfra3/vela/pkg/types"
)

const InventoryVersion = 2
const ExtractorFingerprint = "pipeline-build-v3-closed-go-v1"
const DiscoveryFingerprint = "detect-ignore-v1-strict-inventory-v1"
const ChildDiscoveryFingerprint = "nested-git-roots-v1-strict"

func CanonicalRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("empty source root")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

func digestBytes(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func RequestFingerprint(req types.ManifestRequest) string {
	data, _ := json.Marshal(req)
	return digestBytes(data)
}

func sourceExtension(path string) bool {
	switch filepath.Ext(path) {
	case ".go", ".py", ".ts", ".tsx", ".js", ".jsx":
		return true
	}
	return false
}

func sourceLanguage(path string) string {
	switch filepath.Ext(path) {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".js":
		return "javascript"
	case ".jsx":
		return "jsx"
	default:
		return strings.TrimPrefix(filepath.Ext(path), ".")
	}
}

func configurationName(name string) bool {
	switch name {
	case ".gitignore", ".velignore", "go.mod", "go.sum", "go.work", "go.work.sum", "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "pyproject.toml", "requirements.txt", "setup.py", "poetry.lock", "Cargo.toml", "Cargo.lock", "pom.xml", "build.gradle", "build.gradle.kts":
		return true
	}
	return strings.HasPrefix(name, "tsconfig")
}

// Inventory uses detect's actual rule matcher and ordering (including its
// ANY-list semantics). Unlike detect.Walk it never swallows traversal failures.
// The same function supplies default build discovery and reader verification.
func Inventory(root string, req types.ManifestRequest) (*types.Manifest, []string, error) {
	root, err := CanonicalRoot(root)
	if err != nil {
		return nil, nil, err
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, nil, fmt.Errorf("inventory root is not a directory: %s", root)
	}
	states := map[string][]*detect.IgnoreList{}
	configs := map[string]string{}
	var paths []string
	m := &types.Manifest{Version: InventoryVersion, RepoRoot: root, ExtractorFingerprint: ExtractorFingerprint, DiscoveryFingerprint: DiscoveryFingerprint, Request: req, RequestFingerprint: RequestFingerprint(req), Coverage: "unknown"}
	load := func(dir string, parent []*detect.IgnoreList) ([]*detect.IgnoreList, error) {
		lists := append([]*detect.IgnoreList(nil), parent...)
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !configurationName(entry.Name()) {
				continue
			}
			p := filepath.Join(dir, entry.Name())
			data, err := regularFile(p)
			if err != nil {
				return nil, err
			}
			rel, _ := filepath.Rel(root, p)
			configs[filepath.ToSlash(rel)] = digestBytes(data)
		}
		if tech := detect.DetectTech(dir); tech != detect.TechUnknown {
			lists = append(lists, detect.NewIgnoreList(dir, detect.DefaultIgnorePatterns(tech)))
		}
		for _, name := range []string{".gitignore", ".velignore"} {
			list, err := detect.LoadIgnoreFile(dir, name)
			if err != nil {
				return nil, err
			}
			if list != nil {
				lists = append(lists, list)
			}
		}
		return lists, nil
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// Generated artifacts have no source ownership. This exclusion is shared
		// by builds and readers and prevents recursively inspecting generations.
		if d.IsDir() && d.Name() == ".generations" && filepath.Base(filepath.Dir(path)) == ".vela" {
			return filepath.SkipDir
		}
		dir := path
		if !d.IsDir() {
			dir = filepath.Dir(path)
		}
		lists, ok := states[dir]
		if !ok {
			var err error
			lists, err = load(dir, states[filepath.Dir(dir)])
			if err != nil {
				return err
			}
			states[dir] = lists
		}
		if path == root {
			return nil
		}
		for _, list := range lists {
			if list.Ignored(path, d.IsDir()) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if d.IsDir() || !sourceExtension(path) {
			return nil
		}
		data, err := regularFile(path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		m.Files = append(m.Files, types.ManifestFile{Path: filepath.ToSlash(rel), SHA256: digestBytes(data), Size: int64(len(data)), ModTimeUTC: info.ModTime().UTC(), Language: sourceLanguage(path), Status: "active"})
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return m, paths, err
	}
	workspace := filepath.Join(root, ".vela", "workspace.yaml")
	if _, err := os.Lstat(workspace); err == nil {
		data, err := regularFile(workspace)
		if err != nil {
			return m, paths, err
		}
		configs[".vela/workspace.yaml"] = digestBytes(data)
	} else if !os.IsNotExist(err) {
		return m, paths, err
	}
	data, _ := json.Marshal(configs)
	m.ConfigFingerprint = digestBytes(data)
	m.InventoryComplete = true
	return m, paths, nil
}

func regularFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("inventory requires regular file: %s", path)
	}
	return os.ReadFile(path)
}

// DiscoverChildRoots follows the existing nested-repository discovery shape,
// but treats inaccessible .git probes as errors rather than missing children.
func DiscoverChildRoots(root string) ([]string, error) {
	root, err := CanonicalRoot(root)
	if err != nil {
		return nil, err
	}
	var roots []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != root {
			info, err := os.Stat(filepath.Join(path, ".git"))
			if err == nil && (info.IsDir() || info.Mode().IsRegular()) {
				roots = append(roots, path)
				return filepath.SkipDir
			}
			if err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		if d.Name() == ".git" || d.Name() == ".vela" {
			return filepath.SkipDir
		}
		return nil
	})
	sort.Strings(roots)
	return roots, err
}

// CheckInventory does not consult mutable child selections. Aggregate manifests
// embed everything needed for freshness after child generation reclamation.
func CheckInventory(m *types.Manifest) (stale []string, err error) {
	if m == nil || !m.InventoryComplete || m.Version != InventoryVersion {
		return nil, fmt.Errorf("inventory provenance unavailable")
	}
	if m.ExtractorFingerprint != ExtractorFingerprint || m.DiscoveryFingerprint != DiscoveryFingerprint || m.RequestFingerprint != RequestFingerprint(m.Request) {
		stale = append(stale, "fingerprint")
	}
	if m.Aggregate != nil {
		a := m.Aggregate
		if root, e := CanonicalRoot(m.RepoRoot); e != nil {
			err = e
		} else if root != m.RepoRoot {
			stale = append(stale, "source_identity")
		}
		if a.ParentRoot != m.RepoRoot || a.DiscoveryFingerprint != ChildDiscoveryFingerprint {
			stale = append(stale, "discovery_scope")
		}
		roots, e := DiscoverChildRoots(m.RepoRoot)
		if e != nil {
			err = e
		} else if !reflect.DeepEqual(roots, a.ChildRoots) {
			stale = append(stale, "child_set")
		}
		for _, child := range a.Children {
			changed, e := CheckInventory(&child.Manifest)
			for _, path := range changed {
				stale = append(stale, child.Root+":"+path)
			}
			if e != nil {
				err = e
			}
		}
		return stale, err
	}
	current, _, e := Inventory(m.RepoRoot, m.Request)
	if current == nil {
		return stale, e
	}
	if e == nil && current.ConfigFingerprint != m.ConfigFingerprint {
		stale = append(stale, "configuration")
	}
	if current.RepoRoot != m.RepoRoot {
		stale = append(stale, "source_identity")
	}
	before := map[string]string{}
	for _, f := range m.Files {
		before[f.Path] = f.SHA256
	}
	for _, f := range current.Files {
		if before[f.Path] != f.SHA256 {
			stale = append(stale, f.Path)
		}
		delete(before, f.Path)
	}
	for path := range before {
		if e == nil {
			stale = append(stale, path)
		}
	}
	sort.Strings(stale)
	return stale, e
}

// Legacy verification may establish a changed/deleted file, never completeness.
// Permission errors and nonregular paths are uncertainty, not deletion.
func CheckLegacy(m *types.Manifest) (stale []string, err error) {
	if m == nil || m.RepoRoot == "" {
		return nil, fmt.Errorf("missing legacy inventory")
	}
	for _, f := range m.Files {
		if f.Path == "" || f.SHA256 == "" {
			continue
		}
		data, e := regularFile(filepath.Join(m.RepoRoot, filepath.FromSlash(f.Path)))
		if e == nil {
			if digestBytes(data) != f.SHA256 {
				stale = append(stale, f.Path)
			}
		} else if os.IsNotExist(e) {
			stale = append(stale, f.Path)
		} else {
			err = e
		}
	}
	sort.Strings(stale)
	return stale, err
}

const BoundGoProfile = "captured-go-structural-v1"

// ClosedInputs records the filesystem profile the default Go structural
// extractor can inspect: all Go files (including ignored candidates), config,
// and directory existence. Generated/VCS stores cannot be import targets in
// this profile; pipeline validates imports before running on the private tree.
type Closure struct {
	Bytes                                                          map[string][]byte
	Dirs                                                           []string
	Files                                                          []types.ManifestFile
	DirectoryFingerprint, ConfigFingerprint, DependencyFingerprint string
}

func ClosedInputs(root string) (*Closure, error) {
	c := &Closure{Bytes: map[string][]byte{}}
	var total int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		rel = filepath.ToSlash(rel)
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("unclosed symlink: %s", rel)
		}
		if d.IsDir() {
			if path != root && (d.Name() == ".git" || d.Name() == ".vela" || d.Name() == ".generations" || d.Name() == ".extraction-cache" || strings.HasPrefix(d.Name(), ".scip-build-")) {
				return filepath.SkipDir
			}
			c.Dirs = append(c.Dirs, rel)
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".go") && !configurationName(d.Name()) {
			return nil
		}
		data, e := regularFile(path)
		if e != nil {
			return e
		}
		total += int64(len(data))
		if total > 64<<20 || len(c.Bytes) >= 20000 {
			return fmt.Errorf("closed input budget exceeded")
		}
		c.Bytes[rel] = data
		return nil
	})
	if err != nil {
		return c, err
	}
	workspace := filepath.Join(root, ".vela", "workspace.yaml")
	if _, e := os.Lstat(workspace); e == nil {
		data, e := regularFile(workspace)
		if e != nil {
			return c, e
		}
		c.Bytes[".vela/workspace.yaml"] = data
	} else if !os.IsNotExist(e) {
		return c, e
	}
	var paths []string
	configs := map[string]string{}
	for p, data := range c.Bytes {
		paths = append(paths, p)
		if configurationName(filepath.Base(p)) || p == ".vela/workspace.yaml" {
			configs[p] = digestBytes(data)
		}
	}
	sort.Strings(paths)
	sort.Strings(c.Dirs)
	for _, p := range paths {
		c.Files = append(c.Files, types.ManifestFile{Path: p, SHA256: digestBytes(c.Bytes[p]), Size: int64(len(c.Bytes[p]))})
	}
	data, _ := json.Marshal(c.Dirs)
	c.DirectoryFingerprint = digestBytes(data)
	data, _ = json.Marshal(configs)
	c.ConfigFingerprint = digestBytes(data)
	data, _ = json.Marshal(struct {
		Paths, Dirs []string
		Config      string
	}{paths, c.Dirs, c.ConfigFingerprint})
	c.DependencyFingerprint = digestBytes(data)
	return c, nil
}

// VerifySources keeps binding, inventory freshness and extraction coverage
// separate. Equal live hashes alone cannot attest an opaque extractor's input.
func VerifySources(m *types.Manifest) (stale []string, err error) {
	defer func() {
		seen := map[string]bool{}
		unique := make([]string, 0, len(stale))
		for _, p := range stale {
			if !seen[p] {
				seen[p] = true
				unique = append(unique, p)
			}
		}
		sort.Strings(unique)
		stale = unique
	}()
	stale, err = CheckInventory(m)
	if m == nil {
		return stale, err
	}
	if m.Aggregate != nil {
		for _, child := range m.Aggregate.Children {
			s, e := VerifySources(&child.Manifest)
			for _, p := range s {
				stale = append(stale, child.Root+":"+p)
			}
			if e != nil {
				err = e
			}
		}
		return stale, err
	}
	if m.SourceBinding != BoundGoProfile {
		return stale, fmt.Errorf("source binding unknown: %s", m.BindingReason)
	}
	c, e := ClosedInputs(m.RepoRoot)
	if e != nil {
		if c != nil {
			before := map[string]string{}
			for _, f := range m.ConsumedFiles {
				before[f.Path] = f.SHA256
			}
			for p, data := range c.Bytes {
				if before[p] != digestBytes(data) {
					stale = append(stale, p)
				}
			}
		}
		return stale, e
	}
	if c.DirectoryFingerprint != m.DirectoryFingerprint || c.DependencyFingerprint != m.DependencyFingerprint || c.ConfigFingerprint != m.ClosureConfigFingerprint {
		stale = append(stale, "dependency_discovery")
	}
	before := map[string]string{}
	for _, f := range m.ConsumedFiles {
		before[f.Path] = f.SHA256
	}
	for _, f := range c.Files {
		if before[f.Path] != f.SHA256 {
			stale = append(stale, f.Path)
		}
		delete(before, f.Path)
	}
	for p := range before {
		stale = append(stale, p)
	}
	sort.Strings(stale)
	return stale, err
}
