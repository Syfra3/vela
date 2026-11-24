package export

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSequenceIgnoresInvalidHigherSeal(t *testing.T) {
	root := t.TempDir()
	w, err := AcquireWriter(context.Background(), filepath.Join(root, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	a, err := w.Publish(harnessGraph("a"), harnessManifest(t, root), nil)
	if err != nil {
		t.Fatal(err)
	}
	bogus := filepath.Join(w.Dir, ".generations", "g-bogus")
	if err := os.Mkdir(bogus, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bogus, "seal.json"), []byte(`{"id":"g-bogus","sequence":999999}`), 0600); err != nil {
		t.Fatal(err)
	}
	b, err := w.Publish(harnessGraph("b"), harnessManifest(t, root), nil)
	if err != nil {
		t.Fatal(err)
	}
	if b.Sequence != a.Sequence+1 {
		t.Fatalf("sequence %d after %d", b.Sequence, a.Sequence)
	}
}
