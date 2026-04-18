// Package listener defines the EventSource interface and shared event types
// for Vela's real-time update system. Sources (Ancora, future modules) connect
// via IPC and emit events that drive incremental graph reconciliation.
package listener

import (
	"context"
	"encoding/json"
	"time"
)

// EventType identifies the kind of change emitted by an event source.
type EventType string

const (
	EventObservationCreated EventType = "observation.created"
	EventObservationUpdated EventType = "observation.updated"
	EventObservationDeleted EventType = "observation.deleted"
	EventSessionCreated     EventType = "session.created"
	EventSessionEnded       EventType = "session.ended"
)

// Reference describes a relationship from an observation to another artifact.
type Reference struct {
	// Type is one of: "file", "observation", "concept", "function"
	Type string `json:"type"`
	// Target is the referenced artifact (file path, observation ID, concept name, etc.)
	Target string `json:"target"`
}

// ObservationPayload is the full event payload for created/updated observations.
type ObservationPayload struct {
	ID           int64       `json:"id"`
	SyncID       string      `json:"sync_id"`
	SessionID    string      `json:"session_id"`
	Type         string      `json:"type"`
	Title        string      `json:"title"`
	Content      string      `json:"content"`
	Workspace    string      `json:"workspace,omitempty"`
	Visibility   string      `json:"visibility"`
	Organization string      `json:"organization,omitempty"`
	TopicKey     string      `json:"topic_key,omitempty"`
	References   []Reference `json:"references,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

// ObservationDeletedPayload is the slim payload for deleted observations.
type ObservationDeletedPayload struct {
	ID     int64  `json:"id"`
	SyncID string `json:"sync_id"`
}

// Event is a single change notification received from an event source.
// It is the internal Vela representation — decoupled from the wire format.
type Event struct {
	// Source is the name of the listener that produced this event (e.g. "ancora").
	Source    string
	Type      EventType
	Payload   json.RawMessage
	Timestamp time.Time
}

// SourceStatus holds the current health and statistics for an EventSource.
type SourceStatus struct {
	Connected  bool
	LastEvent  time.Time
	EventCount int64
	LastError  error
}

// EventSource is the interface that all event listeners must implement.
// Implementations must be safe for concurrent use.
type EventSource interface {
	// Name returns a stable, human-readable identifier (e.g. "ancora").
	Name() string

	// Connect establishes the connection to the source. Implementations should
	// start a background reader goroutine and return immediately. If the source
	// is unavailable, Connect may return an error — callers should retry.
	Connect(ctx context.Context) error

	// Events returns a read-only channel of incoming events. The channel is
	// closed when the source disconnects or Close is called.
	Events() <-chan Event

	// Close disconnects from the source and releases resources.
	Close() error

	// Status returns a snapshot of the current connection status.
	Status() SourceStatus
}
