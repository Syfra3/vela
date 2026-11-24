package export

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFirstPublishRetryPreflight(t *testing.T) {
	for _, boundary := range []string{"output_directory_sync", "json_write", "generation_rename"} {
		t.Run(boundary, func(t *testing.T) {
			root := t.TempDir()
			out := filepath.Join(root, ".vela")
			w, err := AcquireWriter(context.Background(), out)
			if err != nil {
				t.Fatal(err)
			}
			w.Fault = func(s string) error {
				if s == boundary {
					return errors.New("injected")
				}
				return nil
			}
			if _, err := w.Publish(harnessGraph("a"), harnessManifest(t, root), nil); err == nil {
				t.Fatal("expected fault")
			}
			w.Close()
			if _, err := ResolveGeneration(out); err == nil {
				t.Fatal("reader accepted missing selection")
			}
			w, err = AcquireWriter(context.Background(), out)
			if err != nil {
				t.Fatal(err)
			}
			defer w.Close()
			if _, err := w.Publish(harnessGraph("b"), harnessManifest(t, root), nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFirstPublishPreflightRejectsUnknownAndHistory(t *testing.T) {
	for _, entry := range []string{"g-old", ".stage-", "unknown"} {
		t.Run(entry, func(t *testing.T) {
			root := t.TempDir()
			out := filepath.Join(root, ".vela")
			if err := os.MkdirAll(filepath.Join(out, ".generations", entry), 0755); err != nil {
				t.Fatal(err)
			}
			w, err := AcquireWriter(context.Background(), out)
			if err != nil {
				t.Fatal(err)
			}
			defer w.Close()
			if _, err := w.Publish(harnessGraph("b"), harnessManifest(t, root), nil); err == nil {
				t.Fatal("accepted unknown history")
			}
		})
	}
}
