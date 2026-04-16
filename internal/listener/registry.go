package listener

import (
	"context"
	"fmt"
	"sync"
)

// Registry manages a set of EventSources and fans their events out onto a
// single merged channel. Sources can be added or removed at runtime.
type Registry struct {
	mu      sync.RWMutex
	sources map[string]EventSource

	merged  chan Event
	closeCh chan struct{}

	wg sync.WaitGroup
}

// NewRegistry creates an empty registry. Call Run to start the fan-in loop.
func NewRegistry() *Registry {
	return &Registry{
		sources: make(map[string]EventSource),
		merged:  make(chan Event, 512),
		closeCh: make(chan struct{}),
	}
}

// Add registers an EventSource. If a source with the same name already exists
// it is replaced (the old one is closed first).
// Add is safe to call concurrently and after Run has started.
func (r *Registry) Add(ctx context.Context, src EventSource) error {
	r.mu.Lock()
	if old, exists := r.sources[src.Name()]; exists {
		_ = old.Close()
	}
	r.sources[src.Name()] = src
	r.mu.Unlock()

	if err := src.Connect(ctx); err != nil {
		return fmt.Errorf("connect %s: %w", src.Name(), err)
	}

	// Fan the source's events into the merged channel.
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for {
			select {
			case ev, ok := <-src.Events():
				if !ok {
					return
				}
				select {
				case r.merged <- ev:
				case <-r.closeCh:
					return
				}
			case <-r.closeCh:
				return
			}
		}
	}()

	return nil
}

// Remove disconnects and removes the named source.
func (r *Registry) Remove(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	src, ok := r.sources[name]
	if !ok {
		return fmt.Errorf("source %q not found", name)
	}
	delete(r.sources, name)
	return src.Close()
}

// Events returns the merged event channel (all sources combined).
func (r *Registry) Events() <-chan Event {
	return r.merged
}

// Statuses returns a snapshot of all source statuses keyed by name.
func (r *Registry) Statuses() map[string]SourceStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]SourceStatus, len(r.sources))
	for name, src := range r.sources {
		out[name] = src.Status()
	}
	return out
}

// Names returns the names of all registered sources (order not guaranteed).
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.sources))
	for n := range r.sources {
		names = append(names, n)
	}
	return names
}

// Close shuts down all sources and the merged channel.
func (r *Registry) Close() {
	close(r.closeCh)
	r.mu.RLock()
	for _, src := range r.sources {
		_ = src.Close()
	}
	r.mu.RUnlock()
	r.wg.Wait()
}
