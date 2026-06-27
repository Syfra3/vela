package watch

import (
	"sort"
	"sync"
	"time"
)

// Debouncer collapses bursts of changed files into serialized handler calls.
type Debouncer struct {
	delay   time.Duration
	handler Handler

	mu      sync.Mutex
	timer   *time.Timer
	pending map[string]struct{}
	running bool
	stopped bool
}

// NewDebouncer creates a debouncer for filesystem update cycles.
func NewDebouncer(delay time.Duration, handler Handler) *Debouncer {
	return &Debouncer{
		delay:   delay,
		handler: handler,
		pending: make(map[string]struct{}),
	}
}

// Trigger records a changed file and schedules one debounced update cycle.
func (d *Debouncer) Trigger(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return
	}
	d.pending[path] = struct{}{}
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.delay, d.flush)
}

// Stop cancels any pending timer. Running handlers are allowed to finish.
func (d *Debouncer) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stopped = true
	if d.timer != nil {
		d.timer.Stop()
	}
}

func (d *Debouncer) flush() {
	d.mu.Lock()
	if d.stopped {
		d.mu.Unlock()
		return
	}
	if d.running {
		d.mu.Unlock()
		return
	}
	changed := d.takePendingLocked()
	if len(changed) == 0 {
		d.mu.Unlock()
		return
	}
	d.running = true
	d.mu.Unlock()

	_ = d.handler(changed)

	d.mu.Lock()
	d.running = false
	shouldFlushAgain := !d.stopped && len(d.pending) > 0
	d.mu.Unlock()
	if shouldFlushAgain {
		d.flush()
	}
}

func (d *Debouncer) takePendingLocked() []string {
	changed := make([]string, 0, len(d.pending))
	for path := range d.pending {
		changed = append(changed, path)
	}
	d.pending = make(map[string]struct{})
	sort.Strings(changed)
	return changed
}
