// Package generation is the dependency-neutral generation I/O boundary. It
// authenticates publisher-validated artifacts by seal; semantic validation stays
// in export. It never imports graph, query, export, or an SQL driver.
package generation

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Syfra3/vela/pkg/types"
)

type Selection struct {
	ID, Dir  string
	Sequence uint64
	Manifest *types.Manifest
}
type Seal struct {
	ID       string            `json:"id"`
	Previous string            `json:"previous,omitempty"`
	Sequence uint64            `json:"sequence"`
	Hashes   map[string]string `json:"hashes"`
}

func ValidID(id string) bool {
	return strings.HasPrefix(id, "g-") && len(id) > 2 && !strings.ContainsAny(id, "/\\.")
}
func RegularFile(path string) ([]byte, error) { return regularFile(path) }

func Validate(dir string) (*Selection, error) {
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
	var seal Seal
	if err := json.Unmarshal(data, &seal); err != nil {
		return nil, err
	}
	if !ValidID(seal.ID) || seal.ID != filepath.Base(dir) || seal.Sequence == 0 || len(seal.Hashes) != 3 {
		return nil, fmt.Errorf("invalid generation seal")
	}
	var m types.Manifest
	for _, name := range []string{"graph.json", "graph.db", "manifest.json"} {
		data, err := regularFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		if digestBytes(data) != seal.Hashes[name] {
			return nil, fmt.Errorf("generation hash mismatch: %s", name)
		}
		if name == "manifest.json" {
			if err := json.Unmarshal(data, &m); err != nil {
				return nil, err
			}
		}
	}
	if m.Generation != seal.ID {
		return nil, fmt.Errorf("manifest generation mismatch")
	}
	return &Selection{ID: seal.ID, Dir: dir, Sequence: seal.Sequence, Manifest: &m}, nil
}

func Adopted(out string) bool {
	info, err := os.Lstat(out)
	if err != nil || !info.IsDir() {
		return false
	}
	for _, name := range []string{".current", ".generations", "seal.json"} {
		if _, err := os.Lstat(filepath.Join(out, name)); !os.IsNotExist(err) {
			return true
		}
	}
	return false
}

func Resolve(out string) (*Selection, error) {
	abs, err := filepath.Abs(out)
	if err != nil {
		return nil, err
	}
	// .current is an output-directory alias, not a separate legacy layout.
	if filepath.Base(abs) == ".current" {
		return Resolve(filepath.Dir(abs))
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	} else if !os.IsNotExist(err) {
		return nil, err
	} else if _, e := os.Lstat(abs); e == nil {
		return nil, fmt.Errorf("runtime graph unavailable: dangling output alias")
	}
	if filepath.Base(filepath.Dir(abs)) == ".generations" || filePresent(filepath.Join(abs, "seal.json")) {
		return Validate(abs)
	}
	target, err := os.Readlink(filepath.Join(abs, ".current"))
	if err != nil {
		if !Adopted(abs) {
			return nil, nil
		}
		return nil, fmt.Errorf("runtime graph unavailable: invalid generation selection: %w", err)
	}
	id := filepath.Base(target)
	if !ValidID(id) || target != filepath.Join(".generations", id) {
		return nil, fmt.Errorf("runtime graph unavailable: invalid selection target")
	}
	return Validate(filepath.Join(abs, target))
}

// Pin accepts artifact entrypoints, output-directory aliases and explicit
// generation paths. Related reads must use returned Dir, never resolve again.
func Pin(path string) (*Selection, error) {
	return pinArtifact(path, 0)
}
func pinArtifact(path string, depth int) (*Selection, error) {
	if depth >= 40 {
		return nil, fmt.Errorf("runtime graph unavailable: artifact alias cycle")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	// Directory entrypoints own their selection, even when nested inside another
	// output. .current is explicitly an alias owned by its immediate directory.
	if filepath.Base(abs) == ".current" {
		return Resolve(abs)
	}
	info, err := os.Lstat(abs)
	if err == nil && info.IsDir() {
		return Resolve(abs)
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		resolvedInfo, statErr := os.Stat(abs)
		if statErr == nil && resolvedInfo.IsDir() {
			return Resolve(abs)
		}
		if statErr != nil && !os.IsNotExist(statErr) {
			return nil, statErr
		}
		// Only proven file aliases or reserved artifact names inherit parent
		// authority. A dangling directory alias must not fall into a parent corpus.
		if statErr == nil || artifactName(filepath.Base(abs)) {
			if selected, err := Resolve(filepath.Dir(abs)); err != nil || selected != nil {
				return selected, err
			}
		}
		target, err := os.Readlink(abs)
		if err != nil {
			return nil, err
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(abs), target)
		}
		selected, err := pinArtifact(target, depth+1)
		if statErr != nil && err == nil && selected == nil {
			return nil, fmt.Errorf("runtime graph unavailable: dangling entrypoint alias")
		}
		return selected, err
	}
	// Artifact parents remain authoritative for regular or absent artifacts.
	if err == nil || artifactName(filepath.Base(abs)) {
		return Resolve(filepath.Dir(abs))
	}
	return Resolve(abs)
}
func artifactName(name string) bool {
	return name == "graph.json" || name == "graph.db" || name == "manifest.json"
}
func filePresent(path string) bool { _, err := os.Lstat(path); return err == nil }

// Candidate preserves selected unavailable corpora instead of skipping their
// dangling compatibility path in favor of a different repository/global graph.
func Candidate(path string) (bool, error) {
	s, err := Pin(path)
	if err != nil {
		return true, err
	}
	if s != nil {
		return true, nil
	}
	_, err = os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}
func ReadOnlyURI(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	u := url.URL{Scheme: "file", Path: abs, RawQuery: "mode=ro"}
	return u.String(), nil
}
