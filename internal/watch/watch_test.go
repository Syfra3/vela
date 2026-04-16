package watch

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestWatcher_DetectsChange(t *testing.T) {
	dir := t.TempDir()

	// Write an initial file
	goFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(goFile, []byte("package main"), 0644); err != nil {
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
	go func() {
		if err := w.Run(stop); err != nil {
			t.Errorf("watcher error: %v", err)
		}
	}()

	// Give the watcher time to start
	time.Sleep(50 * time.Millisecond)

	// Modify the file
	if err := os.WriteFile(goFile, []byte("package main\n// changed"), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for debounce + handler
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
		t.Error("expected handler to be called after file change, got nothing")
	}
}

func TestWatcher_IgnoresNonTrackedExtension(t *testing.T) {
	dir := t.TempDir()
	txtFile := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(txtFile, []byte("hello"), 0644); err != nil {
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

	// Only watch .go files — .txt changes should be ignored
	w, err := New(dir, []string{".go"}, handler)
	if err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	go func() { _ = w.Run(stop) }()

	time.Sleep(50 * time.Millisecond)

	// Modify the .txt file
	if err := os.WriteFile(txtFile, []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait past debounce window
	time.Sleep(debounceDelay + 100*time.Millisecond)
	close(stop)

	mu.Lock()
	defer mu.Unlock()
	if len(received) > 0 {
		t.Errorf("expected no handler calls for .txt file, got: %v", received)
	}
}

func TestWatcher_Debounce(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "app.go")
	if err := os.WriteFile(goFile, []byte("package app"), 0644); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	callCount := 0

	handler := func(changed []string) error {
		mu.Lock()
		callCount++
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

	// Rapid writes within debounce window → should be coalesced into 1 call
	for i := 0; i < 5; i++ {
		_ = os.WriteFile(goFile, []byte("package app // v"+string(rune('0'+i))), 0644)
		time.Sleep(20 * time.Millisecond) // less than debounceDelay
	}

	// Wait for debounce to fire
	time.Sleep(debounceDelay + 200*time.Millisecond)
	close(stop)

	mu.Lock()
	defer mu.Unlock()
	// Debounce should produce 1 call, not 5
	if callCount == 0 {
		t.Error("expected at least 1 handler call")
	}
	if callCount > 3 {
		t.Errorf("expected debouncing to coalesce rapid writes, got %d calls", callCount)
	}
}
