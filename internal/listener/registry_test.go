package listener

import (
	"context"
	"testing"
	"time"
)

type fakeSource struct {
	name       string
	events     chan Event
	status     SourceStatus
	connected  bool
	closed     bool
	connectErr error
}

func newFakeSource(name string) *fakeSource {
	return &fakeSource{
		name:   name,
		events: make(chan Event, 4),
	}
}

func (f *fakeSource) Name() string { return f.name }

func (f *fakeSource) Connect(context.Context) error {
	f.connected = true
	f.status.Connected = true
	return f.connectErr
}

func (f *fakeSource) Events() <-chan Event { return f.events }

func (f *fakeSource) Close() error {
	f.closed = true
	f.status.Connected = false
	close(f.events)
	return nil
}

func (f *fakeSource) Status() SourceStatus { return f.status }

func TestRegistryAddFansInEventsAndStatuses(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	defer r.Close()

	src := newFakeSource("ancora")
	if err := r.Add(context.Background(), src); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	want := Event{Source: "ancora", Type: EventObservationCreated, Timestamp: time.Now()}
	src.events <- want

	select {
	case got := <-r.Events():
		if got.Source != want.Source || got.Type != want.Type {
			t.Fatalf("merged event = %#v, want %#v", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for merged event")
	}

	statuses := r.Statuses()
	if !statuses["ancora"].Connected {
		t.Fatal("status for ancora source should report connected")
	}

	names := r.Names()
	if len(names) != 1 || names[0] != "ancora" {
		t.Fatalf("Names() = %v, want [ancora]", names)
	}
}

func TestRegistryAddReplacesExistingSource(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	defer r.Close()

	oldSrc := newFakeSource("ancora")
	if err := r.Add(context.Background(), oldSrc); err != nil {
		t.Fatalf("first Add() error = %v", err)
	}

	newSrc := newFakeSource("ancora")
	if err := r.Add(context.Background(), newSrc); err != nil {
		t.Fatalf("second Add() error = %v", err)
	}

	if !oldSrc.closed {
		t.Fatal("old source should be closed when replaced")
	}
}

func TestRegistryRemoveMissingSource(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	defer r.Close()

	err := r.Remove("missing")
	if err == nil {
		t.Fatal("Remove() error = nil, want missing source error")
	}
	if err.Error() == "" {
		t.Fatalf("Remove() returned unexpected error: %v", err)
	}
}
