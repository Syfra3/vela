package query

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/pkg/types"
)

// Scenario: Source inventory determines freshness. Every artifact is temporary;
// no extraction, registry, CLI, external process, or live graph is used.
func TestHarnessReliabilityIncompleteInventory(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, ".vela")
	if err := export.WriteSQLiteGraphAtomic(&types.Graph{}, out); err != nil {
		t.Fatal(err)
	}
	if err := export.WriteManifestAtomic(&types.Manifest{Version: 1, RepoRoot: filepath.Join(root, "missing")}, out); err != nil {
		t.Fatal(err)
	}
	e, err := LoadFromFile(filepath.Join(out, "graph.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Freshness().Status; got == FreshnessFresh {
		t.Fatalf("incomplete inventory certified fresh: %s", got)
	}
}

func TestHarnessReliabilityConcurrentReadersWritersAndPinnedPrior(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, ".vela")
	publish := func(i int) error {
		m, _, err := export.Inventory(root, types.ManifestRequest{})
		if err != nil {
			return err
		}
		m.GeneratedAt = time.Unix(int64(i), 0).UTC()
		w, err := export.AcquireWriter(context.Background(), out)
		if err != nil {
			return err
		}
		defer w.Close()
		_, err = w.Publish(&types.Graph{Nodes: []types.Node{{ID: fmt.Sprint(i), Label: fmt.Sprint(i), NodeType: "file"}}}, m, nil)
		return err
	}
	if err := publish(1); err != nil {
		t.Fatal(err)
	}
	pinned, err := LoadFromFile(filepath.Join(out, "graph.json"))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 100)
	for i := 2; i <= 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := publish(i); err != nil {
				errs <- err
			}
		}(i)
	}
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				e, err := LoadFromFile(filepath.Join(out, "graph.json"))
				if err != nil {
					errs <- err
					return
				}
				stamp, err := time.Parse(time.RFC3339, e.Freshness().GraphUpdatedAt)
				if err != nil {
					errs <- err
					return
				}
				if e.graph.Nodes[0].ID != fmt.Sprint(stamp.Unix()) || e.Freshness().Generation == "" {
					errs <- fmt.Errorf("mixed graph/manifest")
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if pinned.graph.Nodes[0].ID != "1" || pinned.Freshness().GraphUpdatedAt != time.Unix(1, 0).UTC().Format(time.RFC3339) {
		t.Fatal("pinned prior changed")
	}
}

func TestHarnessReliabilitySelectedGenerationMissingSQLiteIsUnavailable(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, ".vela")
	m, _, err := export.Inventory(root, types.ManifestRequest{})
	if err != nil {
		t.Fatal(err)
	}
	w, err := export.AcquireWriter(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	s, err := w.Publish(&types.Graph{}, m, nil)
	w.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(s.Dir, "graph.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFromFile(filepath.Join(out, "graph.json")); err == nil {
		t.Fatal("selected JSON substituted for missing SQLite")
	}
}

func TestHarnessReliabilityUnavailableIdentity(t *testing.T) {
	for _, root := range []string{"", filepath.Join(t.TempDir(), "actual-project")} {
		e := NewUnavailableEngine(UnavailableDiagnostic{Workspace: root})
		if e.Freshness().Project == "stock-chef" {
			t.Fatal("fabricated unavailable project")
		}
	}
}

func TestHarnessReliabilityLegacySQLiteRequired(t *testing.T) {
	out := filepath.Join(t.TempDir(), ".vela")
	if err := export.WriteJSONAtomic(&types.Graph{}, out); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFromFile(filepath.Join(out, "graph.json")); err == nil {
		t.Fatal("JSON fallback accepted")
	}
	if err := export.WriteSQLiteGraphAtomic(&types.Graph{}, out); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFromFile(filepath.Join(out, "graph.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(out, "graph.db")); err != nil {
		t.Fatal(err)
	}
}

func TestHarnessReliabilityRelativeSQLiteEntrypoints(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo #?")
	out := filepath.Join(root, ".vela")
	if err := export.WriteSQLiteGraphAtomic(&types.Graph{}, out); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	for _, path := range []string{".vela/graph.json", ".vela/graph.db"} {
		if _, err := LoadFromFile(path); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
	m, _, err := export.Inventory(root, types.ManifestRequest{})
	if err != nil {
		t.Fatal(err)
	}
	w, err := export.AcquireWriter(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Publish(&types.Graph{}, m, nil)
	w.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".vela/graph.json", ".vela/graph.db", ".vela/.current/graph.json", ".vela/.current/graph.db", ".vela", ".vela/.current"} {
		if _, err := LoadFromFile(path); err != nil {
			t.Errorf("generation %s: %v", path, err)
		}
	}
}

func TestHarnessReliabilityStandaloneArtifactAliasesRequireSeal(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, ".vela")
	m, _, err := export.Inventory(root, types.ManifestRequest{})
	if err != nil {
		t.Fatal(err)
	}
	w, err := export.AcquireWriter(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	s, err := w.Publish(&types.Graph{}, m, nil)
	w.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"graph.json", "external.json", "external.db"} {
		alias := filepath.Join(t.TempDir(), name)
		target := "graph.json"
		if filepath.Ext(name) == ".db" {
			target = "graph.db"
		}
		if err := os.Symlink(filepath.Join(s.Dir, target), alias); err != nil {
			t.Fatal(err)
		}
		e, err := LoadFromFile(alias)
		if err != nil || e.Freshness().Generation != s.ID {
			t.Errorf("unvalidated alias %s: %v", name, err)
		}
	}
	alias := filepath.Join(t.TempDir(), "external.json")
	if err := os.Symlink(filepath.Join(s.Dir, "graph.json"), alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(s.Dir, "graph.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFromFile(alias); err == nil {
		t.Fatal("artifact alias fell back to JSON after SQLite disappeared")
	}
}
