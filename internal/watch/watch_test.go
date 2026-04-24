package watch

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestWatcherDetectsChange(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(goFile, []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var received []string
	handler := func(changed []string) error {
		mu.Lock()
		received = append(received, changed...)
		mu.Unlock()
		return nil
	}
	w, err := New(dir, []string{".go"}, handler)
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	go func() { _ = w.Run(stop) }()
	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(goFile, []byte("package main\n// changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(received)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	close(stop)
	mu.Lock()
	defer mu.Unlock()
	if len(received) == 0 {
		t.Fatal("expected handler to be called after file change")
	}
}

func TestWatcherIgnoresNonTrackedExtension(t *testing.T) {
	dir := t.TempDir()
	txtFile := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(txtFile, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var received []string
	handler := func(changed []string) error {
		mu.Lock()
		received = append(received, changed...)
		mu.Unlock()
		return nil
	}
	w, err := New(dir, []string{".go"}, handler)
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	go func() { _ = w.Run(stop) }()
	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(txtFile, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(debounceDelay + 100*time.Millisecond)
	close(stop)
	mu.Lock()
	defer mu.Unlock()
	if len(received) != 0 {
		t.Fatalf("expected no handler calls for .txt file, got %v", received)
	}
}
