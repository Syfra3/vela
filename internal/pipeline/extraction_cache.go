package pipeline

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Syfra3/vela/internal/extract"
	"github.com/Syfra3/vela/pkg/types"
)

type ownedExtraction struct {
	Owner string
	Nodes []types.Node
	Edges []types.Edge
}
type extractionRecord struct {
	Key     string
	Digest  string
	Payload json.RawMessage
}

func cacheDigest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// Cache only pre-resolution per-file artifacts for the closed default Go
// profile. Directory/config changes conservatively invalidate every owner.
// Global MergeFacts/Build/projection run after both cached and uncached scans.
func scanCaptured(s *sourceSnapshot, source *types.Source, out string, disabled bool) ([]types.Node, []types.Edge, int, error) {
	if disabled {
		nodes, edges, err := extract.ExtractAll(s.Root, s.Files, nil, source)
		return nodes, edges, 0, err
	}
	nodes, edges, err := extract.ExtractAll(s.Root, nil, nil, source)
	if err != nil {
		return nil, nil, 0, err
	}
	hits := 0
	for _, file := range s.Files {
		rel, _ := filepath.Rel(s.Root, file)
		rel = filepath.ToSlash(rel)
		identity, _ := json.Marshal(struct {
			Root, Owner, Content, Extractor, Config, Dependency string
			Source                                              *types.Source
		}{s.Manifest.RepoRoot, rel, cacheDigest(s.Closure.Bytes[rel]), s.Manifest.ExtractorFingerprint, s.Closure.ConfigFingerprint, s.Closure.DependencyFingerprint, source})
		key := cacheDigest(identity)
		path := filepath.Join(out, ".extraction-cache", key+".json")
		var owned ownedExtraction
		hit := false
		if !disabled {
			data, e := os.ReadFile(path)
			if e == nil {
				var record extractionRecord
				if json.Unmarshal(data, &record) == nil && record.Key == key && record.Digest == cacheDigest(record.Payload) && json.Unmarshal(record.Payload, &owned) == nil && validOwnership(owned, rel, source) && !bytes.Contains(record.Payload, []byte(s.Root)) {
					hit = true
				}
			}
		}
		if hit {
			hits++
		} else {
			n, e, err := extract.ExtractAll(s.Root, []string{file}, nil, source)
			if err != nil {
				return nil, nil, hits, err
			}
			owned = ownedExtraction{Owner: rel, Nodes: n, Edges: e}
			payload, err := json.Marshal(owned)
			if err != nil {
				return nil, nil, hits, err
			}
			if !disabled && validOwnership(owned, rel, source) && !bytes.Contains(payload, []byte(s.Root)) {
				data, _ := json.Marshal(extractionRecord{Key: key, Digest: cacheDigest(payload), Payload: payload})
				// Cache persistence is best effort. Corruption and interrupted writes
				// are misses; only sealed generations are graph truth.
				if os.MkdirAll(filepath.Dir(path), 0700) == nil {
					if f, e := os.CreateTemp(filepath.Dir(path), ".cache-"); e == nil {
						tmp := f.Name()
						_, e = f.Write(data)
						if closeErr := f.Close(); e == nil {
							e = closeErr
						}
						if e == nil {
							_ = os.Rename(tmp, path)
						}
						_ = os.Remove(tmp)
					}
				}
			}
		}
		nodes = append(nodes, owned.Nodes...)
		edges = append(edges, owned.Edges...)
	}
	return nodes, edges, hits, nil
}

func validOwnership(o ownedExtraction, owner string, source *types.Source) bool {
	if o.Owner != owner {
		return false
	}
	project := extract.CreateProjectNode(source).ID
	ownedIDs := map[string]bool{}
	for _, n := range o.Nodes {
		if n.ID != project && n.SourceFile != owner {
			return false
		}
		ownedIDs[n.ID] = true
		if n.Source == nil || n.Source.Path != source.Path || n.Source.ID != source.ID {
			return false
		}
	}
	for _, e := range o.Edges {
		artifact, _ := e.Metadata["evidence_source_artifact"].(string)
		if !ownedIDs[e.Source] || (e.SourceFile != owner && !(e.SourceFile == "" && artifact == owner)) {
			return false
		}
	}
	return true
}
