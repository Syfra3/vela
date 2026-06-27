package watch

import (
	"sync"
	"testing"
	"time"
)

// REQ-013 → SCN-020 → TestSCN020_WatchDebouncesBurstsAndSerializesUpdates
func TestSCN020_WatchDebouncesBurstsAndSerializesUpdates(t *testing.T) {
	// Scenario: Watch debounces bursts of changes before updating
	started := make(chan struct{}, 2)
	release := make(chan struct{})

	var mu sync.Mutex
	var batches [][]string
	running := 0
	maxRunning := 0

	debouncer := NewDebouncer(10*time.Millisecond, func(changed []string) error {
		mu.Lock()
		running++
		if running > maxRunning {
			maxRunning = running
		}
		batches = append(batches, append([]string(nil), changed...))
		mu.Unlock()

		started <- struct{}{}
		<-release

		mu.Lock()
		running--
		mu.Unlock()
		return nil
	})
	defer debouncer.Stop()

	debouncer.Trigger("a.go")
	debouncer.Trigger("b.go")
	debouncer.Trigger("a.go")

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first debounced update did not start")
	}

	debouncer.Trigger("c.go")
	debouncer.Trigger("d.go")

	select {
	case <-started:
		t.Fatal("second update started while the first update was still running")
	case <-time.After(30 * time.Millisecond):
	}

	release <- struct{}{}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("queued debounced update did not start after first update finished")
	}
	release <- struct{}{}

	mu.Lock()
	defer mu.Unlock()
	if len(batches) != 2 {
		t.Fatalf("handler calls = %d, want 2 debounced update cycles: %v", len(batches), batches)
	}
	if got := len(batches[0]); got != 2 {
		t.Fatalf("first batch changed files = %v, want 2 unique files from burst", batches[0])
	}
	if got := len(batches[1]); got != 2 {
		t.Fatalf("second batch changed files = %v, want 2 unique files queued during first update", batches[1])
	}
	if maxRunning != 1 {
		t.Fatalf("max concurrent update handlers = %d, want serialized updates", maxRunning)
	}
}
