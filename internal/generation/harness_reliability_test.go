package generation_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/internal/generation"
	"github.com/Syfra3/vela/internal/query"
	"github.com/Syfra3/vela/pkg/types"
)

func TestHarnessReliabilitySharedAliasBoundary(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, ".vela")
	m, _, err := generation.Inventory(root, types.ManifestRequest{})
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
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(out, alias); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	for _, path := range []string{".vela/graph.json", ".vela/graph.db", ".vela/.current/graph.json", ".vela/.current", "alias/graph.json", s.Dir} {
		pin, err := generation.Pin(path)
		if err != nil || pin.ID != s.ID {
			t.Fatalf("%s: %v", path, err)
		}
	}
	for _, path := range []string{out, filepath.Join(out, ".current"), s.Dir, alias} {
		if err := generation.Initialize(path); err == nil {
			t.Fatalf("initializer wrote adopted %s", path)
		}
	}
	if err := generation.RefuseRemoval(root); err == nil {
		t.Fatal("nested layout removal allowed")
	}
	if err := os.Remove(filepath.Join(out, ".current")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".generations/g-missing", filepath.Join(out, ".current")); err != nil {
		t.Fatal(err)
	}
	if _, err := generation.Pin("alias/graph.json"); err == nil {
		t.Fatal("dangling adoption fell back")
	}
}

func TestHarnessReliabilityNestedOutputEntrypointsOwnTheirSelection(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "sub")
	publish := func(out, id string) *export.Snapshot {
		t.Helper()
		w, err := export.AcquireWriter(context.Background(), out)
		if err != nil {
			t.Fatal(err)
		}
		defer w.Close()
		m, _, err := generation.Inventory(out, types.ManifestRequest{})
		if err != nil {
			t.Fatal(err)
		}
		s, err := w.Publish(&types.Graph{Nodes: []types.Node{{ID: id, Label: id, NodeType: "file"}}}, m, nil)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	p := publish(parent, "parent")
	c := publish(child, "child")
	alias := filepath.Join(parent, "child-alias")
	if err := os.Symlink(child, alias); err != nil {
		t.Fatal(err)
	}
	paths := []string{child, alias, filepath.Join(child, ".current"), filepath.Join(alias, ".current"), filepath.Join(child, "graph.json"), filepath.Join(child, "graph.db"), filepath.Join(alias, "graph.json"), filepath.Join(alias, "graph.db"), c.Dir}
	for _, path := range paths {
		pin, err := generation.Pin(path)
		if err != nil || pin == nil || pin.ID != c.ID {
			t.Errorf("nested entrypoint selected wrong owner: %s %v %v", path, pin, err)
		}
		e, err := query.LoadFromFile(path)
		if err != nil || e.Freshness().Generation != c.ID {
			t.Errorf("query entrypoint selected wrong owner: %s %v", path, err)
		}
	}
	if err := os.Remove(filepath.Join(child, ".current")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".generations/g-missing", filepath.Join(child, ".current")); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths[:len(paths)-1] {
		if pin, err := generation.Pin(path); err == nil {
			t.Errorf("healthy parent masked invalid child %s: %v", path, pin)
		}
	}
	if pin, err := generation.Pin(parent); err != nil || pin.ID != p.ID {
		t.Fatal("child failure damaged parent selection")
	}
	if pin, err := generation.Pin(c.Dir); err != nil || pin.ID != c.ID {
		t.Fatal("explicit prior generation no longer readable")
	}
}

func TestHarnessReliabilityIndependentWritersRejectAdoption(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, ".vela")
	m, _, err := generation.Inventory(root, types.ManifestRequest{})
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
	for _, dir := range []string{out, filepath.Join(out, ".current"), s.Dir} {
		for _, write := range []func() error{func() error { return export.WriteJSON(&types.Graph{}, dir) }, func() error { return export.WriteJSONAtomic(&types.Graph{}, dir) }, func() error { return export.WriteSQLiteGraphAtomic(&types.Graph{}, dir) }, func() error { return export.WriteManifestAtomic(m, dir) }} {
			if err := write(); err == nil {
				t.Fatal("independent write accepted")
			}
		}
	}
	if _, err := generation.Validate(s.Dir); err != nil {
		t.Fatal(err)
	}
}

func TestHarnessReliabilityPermissionFailureIsUnknownNotDeletion(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks require unprivileged test runner")
	}
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	if err := os.WriteFile(path, []byte("package a"), 0600); err != nil {
		t.Fatal(err)
	}
	m, _, err := generation.Inventory(root, types.ManifestRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0600) })
	if stale, err := generation.CheckInventory(m); err == nil || len(stale) != 0 {
		t.Fatalf("inventory permission became deletion: %v %v", stale, err)
	}
	if stale, err := generation.CheckLegacy(m); err == nil || len(stale) != 0 {
		t.Fatalf("legacy permission became deletion: %v %v", stale, err)
	}
}

// Real filesystem locks, exclusively temporary fixtures. Both serial orders
// are controlled by held guards; no hook/registry integration is executed.
func TestHarnessReliabilityRemovalCoordinatesFirstWriter(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "legacy")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	err := generation.WithRemoval(context.Background(), []string{target}, func() error {
		fd, err := os.Open(target)
		if err != nil {
			return err
		}
		defer fd.Close()
		err = syscall.Flock(int(fd.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)
		if err == nil {
			_ = syscall.Flock(int(fd.Fd()), syscall.LOCK_UN)
			return errors.New("removal does not hold exclusive directory coordination")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		defer cancel()
		w, err := export.AcquireWriter(ctx, filepath.Join(target, "nested", ".vela"))
		if w != nil {
			w.Close()
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("first writer escaped removal guard: %v", err)
		}
		if _, err := os.Stat(filepath.Join(target, "nested")); !os.IsNotExist(err) {
			t.Error("writer created output before removal effects completed")
		}
		return os.RemoveAll(target)
	})
	if err != nil {
		t.Fatal(err)
	}
	// Writer first: removal waits for the writer, then refuses before effects.
	out := filepath.Join(target, "nested", ".vela")
	w, err := export.AcquireWriter(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(target, "registry-hooks-marker")
	if err := os.WriteFile(marker, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		close(started)
		result <- generation.WithRemoval(context.Background(), []string{target}, func() error { return os.WriteFile(marker, []byte("changed"), 0600) })
	}()
	<-started
	m, _, err := generation.Inventory(target, types.ManifestRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Publish(&types.Graph{}, m, nil); err != nil {
		t.Fatal(err)
	}
	w.Close()
	if err := <-result; err == nil {
		t.Fatal("removal accepted first adoption")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "unchanged" {
		t.Fatal("effects preceded adoption revalidation")
	}
	if _, err := os.Stat(filepath.Join(out, ".writer.lock")); err != nil {
		t.Fatal("stable writer inode removed")
	}
}
