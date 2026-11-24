package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewNestedDirectoryIsWatched(t *testing.T) {
	root := t.TempDir()
	changes := make(chan []string, 8)
	w, err := New(root, []string{".go"}, func(paths []string) error { changes <- paths; return nil })
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- w.Run(stop) }()
	defer func() {
		close(stop)
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	nested := filepath.Join(root, "new", "nested")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(nested, "file.go")
	if err := os.WriteFile(file, []byte("package nested"), 0644); err != nil {
		t.Fatal(err)
	}
	timer := time.NewTimer(4 * time.Second)
	defer timer.Stop()
	for {
		select {
		case paths := <-changes:
			for _, path := range paths {
				if path == file {
					return
				}
			}
		case <-timer.C:
			t.Fatal("nested file not reported")
		}
	}
}
