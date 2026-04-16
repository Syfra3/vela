// Package reconcile implements a React-style reconciliation pipeline for
// incremental graph updates driven by IPC events from Ancora and other sources.
package reconcile

import (
	"sync"
	"time"

	"github.com/Syfra3/vela/internal/listener"
)

// Queue buffers incoming events with deduplication and optional debouncing.
// Multiple events for the same observation ID within the debounce window are
// collapsed so the reconciler only processes the most-recent state.
type Queue struct {
	mu       sync.Mutex
	pending  map[dedupeKey]listener.Event // newest event per key
	order    []dedupeKey                  // insertion order for stable drain
	ready    chan struct{}                // signalled when items are available
	debounce time.Duration
	timers   map[dedupeKey]*time.Timer
	maxBatch int
}

// dedupeKey identifies a unique logical entity: source + event type + entity ID.
// For observation events the ID is embedded in the payload via ObservationKey.
type dedupeKey struct {
	source    string
	eventType listener.EventType
	entityKey string // "obs:123" or "sess:abc"
}

// NewQueue creates a Queue with the given debounce window and max batch size.
func NewQueue(debounce time.Duration, maxBatch int) *Queue {
	if maxBatch <= 0 {
		maxBatch = 50
	}
	return &Queue{
		pending:  make(map[dedupeKey]listener.Event),
		ready:    make(chan struct{}, 1),
		debounce: debounce,
		timers:   make(map[dedupeKey]*time.Timer),
		maxBatch: maxBatch,
	}
}

// Push adds an event to the queue. If an event with the same deduplication key
// already exists, it is replaced with the newer event (last-write-wins).
func (q *Queue) Push(ev listener.Event, entityKey string) {
	k := dedupeKey{
		source:    ev.Source,
		eventType: ev.Type,
		entityKey: entityKey,
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	_, exists := q.pending[k]
	q.pending[k] = ev

	if !exists {
		q.order = append(q.order, k)
	}

	if q.debounce > 0 {
		// Reset the per-key debounce timer.
		if t, ok := q.timers[k]; ok {
			t.Reset(q.debounce)
		} else {
			q.timers[k] = time.AfterFunc(q.debounce, func() {
				q.signal()
			})
		}
	} else {
		q.signal()
	}
}

// signal sends to the ready channel without blocking.
func (q *Queue) signal() {
	select {
	case q.ready <- struct{}{}:
	default:
	}
}

// Ready returns a channel that receives a value when events are available.
func (q *Queue) Ready() <-chan struct{} {
	return q.ready
}

// Drain removes up to maxBatch events from the queue and returns them in
// insertion order. The caller is responsible for processing them.
func (q *Queue) Drain() []listener.Event {
	q.mu.Lock()
	defer q.mu.Unlock()

	n := len(q.order)
	if n > q.maxBatch {
		n = q.maxBatch
	}
	if n == 0 {
		return nil
	}

	batch := make([]listener.Event, 0, n)
	remaining := make([]dedupeKey, 0, len(q.order)-n)

	for i, k := range q.order {
		if i < n {
			if ev, ok := q.pending[k]; ok {
				batch = append(batch, ev)
				delete(q.pending, k)
				if t, ok := q.timers[k]; ok {
					t.Stop()
					delete(q.timers, k)
				}
			}
		} else {
			remaining = append(remaining, k)
		}
	}
	q.order = remaining

	// If more events remain, keep the ready channel signalled.
	if len(q.pending) > 0 {
		q.signal()
	}

	return batch
}

// Len returns the current number of unique pending events.
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}
