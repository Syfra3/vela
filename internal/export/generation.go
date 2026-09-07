package export

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/Syfra3/vela/internal/generation"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/Syfra3/vela/pkg/types"
)

// Snapshot owns an immutable, validated graph/manifest pair. Generations are
// deliberately never garbage collected yet: process-independent pinned readers
// and copied aggregate provenance are safe without a reader lease protocol.
type Snapshot struct {
	ID       string
	Dir      string
	Graph    *types.Graph
	Manifest *types.Manifest
	Sequence uint64
}

type generationSeal struct {
	ID       string            `json:"id"`
	Previous string            `json:"previous,omitempty"`
	Sequence uint64            `json:"sequence"`
	Hashes   map[string]string `json:"hashes"`
}

type Writer struct {
	Dir  string
	lock *generation.Lock
	// Per-instance failure injection; never global and never configured by CLI.
	Fault func(string) error
}

var PublicationBoundaries = []string{
	"generation_directory", "output_directory_sync", "stage_directory", "json_write", "json_sync",
	"sqlite_write", "sqlite_sync", "manifest_write", "manifest_sync", "validate_artifacts",
	"seal_write", "seal_sync", "stage_directory_sync", "generation_rename", "generations_directory_sync", "validate_generation",
	"aliases", "aliases_directory_sync", "selection_create", "selection_directory_sync", "selection_rename", "selected_directory_sync",
}

// AcquireWriter serializes all cooperating processes targeting the canonical
// output directory. The stable lock inode is NEVER unlinked (including restart).
func AcquireWriter(ctx context.Context, out string) (*Writer, error) {
	l, err := generation.Acquire(ctx, out)
	if err != nil {
		return nil, err
	}
	return &Writer{Dir: l.Dir, lock: l}, nil
}

func (w *Writer) Close() error {
	if w == nil || w.lock == nil {
		return nil
	}
	f := w.lock
	w.lock = nil
	return f.Close()
}
func (w *Writer) step(name string, action func() error) error {
	if w == nil || w.lock == nil {
		return fmt.Errorf("generation writer is not locked")
	}
	if w.Fault != nil {
		if err := w.Fault(name); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	if err := action(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}
func syncPath(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	err = f.Sync()
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// Publish stages all artifacts, validates them, seals their exact bytes, fsyncs
// artifacts and directory entries, then atomically renames ONE selection link.
// A failure after selection rename may expose the complete new generation. It
// must never be "rolled back" using an earlier CLI byte snapshot.
func (w *Writer) Publish(g *types.Graph, manifest *types.Manifest, persist func(*types.Graph, string) error) (*Snapshot, error) {
	if g == nil || manifest == nil {
		return nil, fmt.Errorf("graph and manifest required")
	}
	g = canonicalGraph(g)
	if w == nil || w.lock == nil {
		return nil, fmt.Errorf("generation writer is not locked")
	}
	previous := ""
	if current, err := ResolveGeneration(w.Dir); err == nil && current != nil {
		previous = current.ID
	} else if err != nil {
		// Existing invalid selection is never silently repaired by building.
		return nil, err
	}
	gens := filepath.Join(w.Dir, ".generations")
	if err := w.step("generation_directory", func() error { return os.MkdirAll(gens, 0755) }); err != nil {
		return nil, err
	}
	if err := w.step("output_directory_sync", func() error {
		if err := syncPath(w.Dir); err != nil {
			return err
		}
		return syncPath(filepath.Dir(w.Dir))
	}); err != nil {
		return nil, err
	}
	var stage string
	if err := w.step("stage_directory", func() error { var err error; stage, err = os.MkdirTemp(gens, ".stage-"); return err }); err != nil {
		return nil, err
	}
	// Incomplete stages remain diagnostically available, but are never candidates.
	id := "g-" + strings.TrimPrefix(filepath.Base(stage), ".stage-")
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	var m types.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	m.Generation = id
	if err := validateManifest(&m); err != nil {
		return nil, err
	}
	steps := []struct {
		name   string
		action func() error
	}{
		{"json_write", func() error { return serializeGenerationJSON(g, stage, persist) }},
		{"json_sync", func() error { return syncPath(filepath.Join(stage, "graph.json")) }},
		{"sqlite_write", func() error { return writeSQLiteGraphAtomic(g, stage) }},
		{"sqlite_sync", func() error { return syncPath(filepath.Join(stage, "graph.db")) }},
		{"manifest_write", func() error { return writeManifestAtomic(&m, stage) }},
		{"manifest_sync", func() error { return syncPath(filepath.Join(stage, "manifest.json")) }},
		{"validate_artifacts", func() error { _, err := validateArtifacts(stage); return err }},
	}
	for _, step := range steps {
		if err := w.step(step.name, step.action); err != nil {
			return nil, err
		}
	}
	seal := generationSeal{ID: id, Previous: previous, Sequence: 1, Hashes: map[string]string{}}
	for _, candidate := range validatedGenerations(gens) {
		if candidate.Sequence == ^uint64(0) {
			return nil, fmt.Errorf("generation sequence exhausted")
		}
		if candidate.Sequence >= seal.Sequence {
			seal.Sequence = candidate.Sequence + 1
		}
	}
	for _, name := range []string{"graph.json", "graph.db", "manifest.json"} {
		bytes, err := regularFile(filepath.Join(stage, name))
		if err != nil {
			return nil, err
		}
		seal.Hashes[name] = digestBytes(bytes)
	}
	sealData, err := json.Marshal(seal)
	if err != nil {
		return nil, err
	}
	final := filepath.Join(gens, id)
	steps = []struct {
		name   string
		action func() error
	}{
		{"seal_write", func() error { return os.WriteFile(filepath.Join(stage, "seal.json"), sealData, 0600) }},
		{"seal_sync", func() error { return syncPath(filepath.Join(stage, "seal.json")) }},
		{"stage_directory_sync", func() error { return syncPath(stage) }},
		{"generation_rename", func() error { return os.Rename(stage, final) }},
		{"generations_directory_sync", func() error { return syncPath(gens) }},
		{"validate_generation", func() error { _, err := ValidateGeneration(final); return err }},
	}
	for _, step := range steps {
		if err := w.step(step.name, step.action); err != nil {
			return nil, err
		}
	}
	if err := w.selectGeneration(id); err != nil {
		return nil, err
	}
	return ValidateGeneration(final)
}

// User callbacks serialize into private scratch outside sealed ancestry. Only
// the publisher copies regular bytes into staging; no global staging privilege
// or goroutine flag is granted to raw writers. Agreement is validated below.
func serializeGenerationJSON(g *types.Graph, stage string, persist func(*types.Graph, string) error) error {
	if persist == nil {
		return writeJSONAtomic(g, stage)
	}
	scratch, err := os.MkdirTemp("", "vela-serialize-")
	if err != nil {
		return err
	}
	// Retain scratch unconditionally. The callback may remove/move it, so any
	// cleanup could escalate to an ancestor EX lock already held SH by this
	// writer. No reclamation or lock upgrade is attempted inside publication.
	expected, err := marshalGraph(g)
	if err != nil {
		return err
	}
	data, err := json.Marshal(g)
	if err != nil {
		return err
	}
	var callbackGraph types.Graph
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&callbackGraph); err != nil {
		return err
	}
	if err := persist(&callbackGraph, scratch); err != nil {
		return err
	}
	data, err = regularFile(filepath.Join(scratch, "graph.json"))
	if err != nil {
		return err
	}
	if err := validateFullExport(expected, data); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stage, "graph.json"), data, 0600)
}

// Normalize through the actual exported schema, not the smaller SQLite column
// projection. Only export wall-clock time is excluded; counts and every node,
// edge, source and metadata field remain checked. Unknown/trailing data fails.
func normalizedExport(data []byte) ([]byte, error) {
	var raw graphJSON
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return nil, err
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("trailing export data")
	}
	raw.Meta.GeneratedAt = ""
	if raw.Nodes == nil {
		raw.Nodes = []nodeJSON{}
	}
	if raw.Edges == nil {
		raw.Edges = []edgeJSON{}
	}
	// The encoder's omitempty behavior normalizes nil/empty optional metadata.
	return json.Marshal(raw)
}
func validateFullExport(expected, actual []byte) error {
	want, err := normalizedExport(expected)
	if err != nil {
		return err
	}
	got, err := normalizedExport(actual)
	if err != nil {
		return fmt.Errorf("invalid callback graph export: %w", err)
	}
	if !bytes.Equal(want, got) {
		return fmt.Errorf("callback changed full exported graph semantics")
	}
	return nil
}

func (w *Writer) selectGeneration(id string) error {
	if err := w.step("aliases", func() error {
		for _, name := range []string{"graph.json", "graph.db", "manifest.json"} {
			path := filepath.Join(w.Dir, name)
			if target, err := os.Readlink(path); err == nil && target == ".current/"+name {
				continue
			}
			tmp := path + ".generation-link"
			if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
				return err
			}
			if err := os.Symlink(".current/"+name, tmp); err != nil {
				return err
			}
			if err := os.Rename(tmp, path); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := w.step("aliases_directory_sync", func() error { return syncPath(w.Dir) }); err != nil {
		return err
	}
	tmp := filepath.Join(w.Dir, ".current.next")
	if err := w.step("selection_create", func() error {
		if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
			return err
		}
		return os.Symlink(filepath.Join(".generations", id), tmp)
	}); err != nil {
		return err
	}
	if err := w.step("selection_directory_sync", func() error { return syncPath(w.Dir) }); err != nil {
		return err
	}
	if err := w.step("selection_rename", func() error { return os.Rename(tmp, filepath.Join(w.Dir, ".current")) }); err != nil {
		return err
	}
	return w.step("selected_directory_sync", func() error { return syncPath(w.Dir) })
}

// ResolveGeneration reads selection once. An absent pointer in an adopted
// layout, invalid pointer, or corrupt generation is unavailable, never legacy.
func ResolveGeneration(out string) (*Snapshot, error) {
	pin, err := generation.Resolve(out)
	if err != nil || pin == nil {
		return nil, err
	}
	return ValidateGeneration(pin.Dir)
}
func validGenerationID(id string) bool {
	return generation.ValidID(id)
}

func ValidateGeneration(dir string) (*Snapshot, error) {
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("generation is not a directory")
	}
	data, err := regularFile(filepath.Join(dir, "seal.json"))
	if err != nil {
		return nil, err
	}
	var seal generationSeal
	if err := json.Unmarshal(data, &seal); err != nil {
		return nil, err
	}
	if !validGenerationID(seal.ID) || seal.ID != filepath.Base(dir) || seal.Sequence == 0 || len(seal.Hashes) != 3 {
		return nil, fmt.Errorf("invalid generation seal")
	}
	for _, name := range []string{"graph.json", "graph.db", "manifest.json"} {
		data, err := regularFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		if digestBytes(data) != seal.Hashes[name] {
			return nil, fmt.Errorf("generation hash mismatch: %s", name)
		}
	}
	s, err := validateArtifacts(dir)
	if err != nil {
		return nil, err
	}
	if s.Manifest.Generation != seal.ID {
		return nil, fmt.Errorf("manifest generation mismatch")
	}
	s.ID, s.Dir, s.Sequence = seal.ID, dir, seal.Sequence
	return s, nil
}

func validateArtifacts(dir string) (*Snapshot, error) {
	m, err := LoadManifest(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	if err := validateManifest(m); err != nil {
		return nil, err
	}
	g, err := LoadJSON(filepath.Join(dir, "graph.json"))
	if err != nil {
		return nil, err
	}
	if err := validateSQLiteAgreement(filepath.Join(dir, "graph.db"), g); err != nil {
		return nil, err
	}
	if m.Aggregate != nil {
		if err := validateAggregateGraph(g, m.Aggregate); err != nil {
			return nil, err
		}
	}
	return &Snapshot{Graph: g, Manifest: m}, nil
}

func validateManifest(m *types.Manifest) error {
	if m == nil || m.Version != InventoryVersion || !m.InventoryComplete || !filepath.IsAbs(m.RepoRoot) || filepath.Clean(m.RepoRoot) != m.RepoRoot || !validGenerationID(m.Generation) || m.ExtractorFingerprint == "" || m.DiscoveryFingerprint == "" || m.RequestFingerprint != RequestFingerprint(m.Request) {
		return fmt.Errorf("incomplete generation provenance")
	}
	seen := map[string]bool{}
	for _, f := range m.Files {
		if f.Path == "" || filepath.IsAbs(f.Path) || filepath.ToSlash(filepath.Clean(f.Path)) != f.Path || f.Path == ".." || strings.HasPrefix(f.Path, "../") || len(f.SHA256) != 64 || seen[f.Path] {
			return fmt.Errorf("invalid inventory entry %q", f.Path)
		}
		seen[f.Path] = true
	}
	if m.Aggregate == nil {
		if m.ConfigFingerprint == "" {
			return fmt.Errorf("missing configuration fingerprint")
		}
		return nil
	}
	a := m.Aggregate
	if a.ParentRoot != m.RepoRoot || a.DiscoveryFingerprint == "" || len(a.Children) == 0 || len(a.Children) != len(a.ChildRoots) {
		return fmt.Errorf("incomplete aggregate provenance")
	}
	seen = map[string]bool{}
	for i, child := range a.Children {
		rel, err := filepath.Rel(m.RepoRoot, child.Root)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("child outside recorded discovery scope")
		}
		if child.Root != a.ChildRoots[i] || (i > 0 && a.ChildRoots[i-1] >= child.Root) || seen[child.Root] || child.Root != child.Manifest.RepoRoot || child.Generation != child.Manifest.Generation || child.Manifest.RequestFingerprint != m.RequestFingerprint || len(child.GraphDigest) != 64 || child.Manifest.Aggregate != nil {
			return fmt.Errorf("invalid child binding")
		}
		if err := validateManifest(&child.Manifest); err != nil {
			return err
		}
		seen[child.Root] = true
	}
	return nil
}

// Recover is an explicit internal writer operation, never a read-side repair.
// Only fully sealed final directories qualify; staging is always ignored.
func (w *Writer) Recover() (*Snapshot, error) {
	if w == nil || w.lock == nil {
		return nil, fmt.Errorf("generation writer is not locked")
	}
	if s, err := ResolveGeneration(w.Dir); err == nil && s != nil {
		return s, nil
	}
	all := validatedGenerations(filepath.Join(w.Dir, ".generations"))
	if len(all) == 0 {
		return nil, fmt.Errorf("runtime graph unavailable: no validated recovery generation")
	}
	s := all[len(all)-1]
	if err := syncPath(filepath.Join(w.Dir, ".generations")); err != nil {
		return nil, err
	}
	if err := w.selectGeneration(s.ID); err != nil {
		return nil, err
	}
	return s, nil
}
func validatedGenerations(dir string) []*Snapshot {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var all []*Snapshot
	for _, entry := range entries {
		if entry.IsDir() && validGenerationID(entry.Name()) {
			if s, err := ValidateGeneration(filepath.Join(dir, entry.Name())); err == nil {
				all = append(all, s)
			}
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Sequence == all[j].Sequence {
			return all[i].ID < all[j].ID
		}
		return all[i].Sequence < all[j].Sequence
	})
	return all
}

// GraphDigest hashes the persisted JSON semantics, excluding wall-clock export
// timestamps. Used to bind pipeline return values and self-contained children.
func GraphDigest(g *types.Graph) string {
	data, err := marshalGraph(canonicalGraph(g))
	if err != nil {
		return ""
	}
	var raw graphJSON
	if json.Unmarshal(data, &raw) != nil {
		return ""
	}
	raw.Meta = metaJSON{}
	data, err = json.Marshal(raw)
	if err != nil {
		return ""
	}
	return digestBytes(data)
}

// SQLite remains runtime truth. Validate every persisted runtime column against
// its JSON projection, including duplicate-edge and label-resolution semantics.
func validateSQLiteAgreement(path string, g *types.Graph) error {
	uri, err := generation.ReadOnlyURI(path)
	if err != nil {
		return err
	}
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		return err
	}
	defer db.Close()
	var integrity, version string
	if err := db.QueryRow("PRAGMA quick_check").Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return fmt.Errorf("sqlite integrity: %s", integrity)
	}
	if err := db.QueryRow("SELECT schema_version FROM schema_meta").Scan(&version); err != nil {
		return err
	}
	if version != runtimeSchemaVersion {
		return fmt.Errorf("unsupported sqlite schema")
	}
	labels := map[string]string{}
	nodes := map[string]string{}
	for _, n := range g.Nodes {
		if _, ok := labels[n.Label]; !ok {
			labels[n.Label] = n.ID
		}
		addChildLabel(labels, n)
		md, err := persistenceMetadata(n.Metadata)
		if err != nil {
			return err
		}
		row, _ := json.Marshal([]string{n.ID, n.NodeType, n.Label, n.SourceFile, string(md)})
		nodes[n.ID] = string(row)
	}
	rows, err := db.Query("SELECT id,kind,label,COALESCE(file_path,''),COALESCE(metadata_json,'null') FROM nodes")
	if err != nil {
		return err
	}
	actual := map[string]string{}
	for rows.Next() {
		v := make([]string, 5)
		if err := rows.Scan(&v[0], &v[1], &v[2], &v[3], &v[4]); err != nil {
			rows.Close()
			return err
		}
		data, _ := json.Marshal(v)
		v[4], err = normalizeMetadataJSON(v[4])
		if err != nil {
			rows.Close()
			return err
		}
		data, _ = json.Marshal(v)
		actual[v[0]] = string(data)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, nodes) {
		return fmt.Errorf("SQLite/JSON node disagreement")
	}
	edges := map[string]string{}
	for _, e := range g.Edges {
		keyData, _ := json.Marshal([]string{sqliteNodeID(e.Source, labels), sqliteNodeID(e.Target, labels), e.Relation})
		key := string(keyData)
		if _, ok := edges[key]; ok {
			continue
		}
		md, err := persistenceMetadata(e.Metadata)
		if err != nil {
			return err
		}
		row, _ := json.Marshal([]string{sqliteNodeID(e.Source, labels), sqliteNodeID(e.Target, labels), e.Relation, e.Confidence, string(md)})
		edges[key] = string(row)
	}
	rows, err = db.Query("SELECT from_node_id,to_node_id,kind,COALESCE(confidence,''),COALESCE(metadata_json,'null') FROM edges")
	if err != nil {
		return err
	}
	actual = map[string]string{}
	for rows.Next() {
		v := make([]string, 5)
		if err := rows.Scan(&v[0], &v[1], &v[2], &v[3], &v[4]); err != nil {
			rows.Close()
			return err
		}
		v[4], err = normalizeMetadataJSON(v[4])
		if err != nil {
			rows.Close()
			return err
		}
		key, _ := json.Marshal(v[:3])
		data, _ := json.Marshal(v)
		actual[string(key)] = string(data)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, edges) {
		return fmt.Errorf("SQLite/JSON edge disagreement")
	}
	return nil
}

func persistenceMetadata(m map[string]interface{}) ([]byte, error) {
	if len(m) == 0 {
		return []byte("null"), nil
	}
	return json.Marshal(m)
}
func normalizeMetadataJSON(raw string) (string, error) {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return "", err
	}
	data, err := persistenceMetadata(m)
	return string(data), err
}

func ChildNamespace(root string) string { return "child:" + digestBytes([]byte(root)) + ":" }

// NamespaceChild keeps each child's graph independently reconstructable, even
// when basenames, relative file paths, labels, and original node IDs collide.
func NamespaceChild(root string, g *types.Graph) *types.Graph {
	prefix := ChildNamespace(root)
	out := &types.Graph{}
	for _, n := range g.Nodes {
		n.ID = prefix + n.ID
		out.Nodes = append(out.Nodes, n)
	}
	for _, e := range g.Edges {
		e.Source = prefix + e.Source
		e.Target = prefix + e.Target
		out.Edges = append(out.Edges, e)
	}
	return out
}
func validateAggregateGraph(g *types.Graph, a *types.AggregateManifest) error {
	parts := map[string]*types.Graph{}
	for _, child := range a.Children {
		parts[ChildNamespace(child.Root)] = &types.Graph{}
	}
	for _, n := range g.Nodes {
		found := false
		for prefix, part := range parts {
			if strings.HasPrefix(n.ID, prefix) {
				n.ID = strings.TrimPrefix(n.ID, prefix)
				part.Nodes = append(part.Nodes, n)
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unaccounted aggregate node")
		}
	}
	for _, e := range g.Edges {
		found := false
		for prefix, part := range parts {
			if strings.HasPrefix(e.Source, prefix) && strings.HasPrefix(e.Target, prefix) {
				e.Source = strings.TrimPrefix(e.Source, prefix)
				e.Target = strings.TrimPrefix(e.Target, prefix)
				part.Edges = append(part.Edges, e)
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unaccounted aggregate edge")
		}
	}
	for _, child := range a.Children {
		if GraphDigest(parts[ChildNamespace(child.Root)]) != child.GraphDigest {
			return fmt.Errorf("aggregate child graph mismatch: %s", child.Root)
		}
	}
	return nil
}
