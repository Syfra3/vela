package reconcile

import (
	"testing"
	"time"

	"github.com/Syfra3/vela/internal/listener"
)

func TestQueuePushDrainDeduplicatesSameKey(t *testing.T) {
	t.Parallel()

	q := NewQueue(0, 10)
	older := listener.Event{Source: "ancora", Type: listener.EventObservationUpdated, Timestamp: time.Unix(1, 0)}
	newer := listener.Event{Source: "ancora", Type: listener.EventObservationUpdated, Timestamp: time.Unix(2, 0)}

	q.Push(older, "obs:42")
	q.Push(newer, "obs:42")

	batch := q.Drain()
	if len(batch) != 1 {
		t.Fatalf("Drain() len = %d, want 1", len(batch))
	}
	if batch[0].Timestamp != newer.Timestamp {
		t.Fatalf("Drain()[0] timestamp = %v, want %v", batch[0].Timestamp, newer.Timestamp)
	}
	if q.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", q.Len())
	}
}

func TestQueueDrainRespectsMaxBatchAndSignalsRemaining(t *testing.T) {
	t.Parallel()

	q := NewQueue(0, 1)
	q.Push(listener.Event{Source: "a", Type: listener.EventObservationCreated}, "obs:1")
	q.Push(listener.Event{Source: "a", Type: listener.EventObservationCreated}, "obs:2")

	batch := q.Drain()
	if len(batch) != 1 {
		t.Fatalf("first Drain() len = %d, want 1", len(batch))
	}
	if q.Len() != 1 {
		t.Fatalf("Len() after first drain = %d, want 1", q.Len())
	}

	select {
	case <-q.Ready():
	case <-time.After(time.Second):
		t.Fatal("queue should remain signalled when events are still pending")
	}

	batch = q.Drain()
	if len(batch) != 1 {
		t.Fatalf("second Drain() len = %d, want 1", len(batch))
	}
}

func TestQueueDebounceSignalsAfterWindow(t *testing.T) {
	t.Parallel()

	q := NewQueue(25*time.Millisecond, 10)
	q.Push(listener.Event{Source: "a", Type: listener.EventObservationCreated}, "obs:1")

	select {
	case <-q.Ready():
		t.Fatal("queue signalled before debounce window elapsed")
	case <-time.After(10 * time.Millisecond):
	}

	select {
	case <-q.Ready():
	case <-time.After(time.Second):
		t.Fatal("queue did not signal after debounce window")
	}
}
