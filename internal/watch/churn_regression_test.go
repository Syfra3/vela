package watch

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWatchNewTreeTransientRemoval(t *testing.T) {
	root := t.TempDir()
	gone := filepath.Join(root, "gone")
	if err := os.Mkdir(gone, 0755); err != nil {
		t.Fatal(err)
	}
	w := &Watcher{exts: map[string]bool{".go": true}}
	// Simulate a directory disappearing between WalkDir and Add.
	err := w.watchNewTree(root, func(path string) error {
		if path == gone {
			return &os.PathError{Op: "add", Path: path, Err: os.ErrNotExist}
		}
		return nil
	}, func(string) {})
	if err != nil {
		t.Fatalf("transient add: %v", err)
	}
	// Remove after Add succeeds, before WalkDir reads the directory.
	err = w.watchNewTree(root, func(path string) error {
		if path == gone {
			return os.Remove(gone)
		}
		return nil
	}, func(string) {})
	if err != nil {
		t.Fatalf("transient readdir: %v", err)
	}
	// A create event's root can disappear before the walk starts.
	if err := w.watchNewTree(filepath.Join(root, "missing"), func(string) error { t.Fatal("unexpected add"); return nil }, func(string) {}); err != nil {
		t.Fatalf("transient walk: %v", err)
	}
	denied := errors.New("add failed")
	if err := w.watchNewTree(root, func(string) error { return denied }, func(string) {}); !errors.Is(err, denied) {
		t.Fatalf("non-ENOENT error lost: %v", err)
	}
}
