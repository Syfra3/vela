package reconcile

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Syfra3/vela/internal/listener"
	"github.com/Syfra3/vela/pkg/types"
)

func TestDifferDiffBuildsChangeSet(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	createdPayload, _ := json.Marshal(listener.ObservationPayload{
		ID:        1,
		Type:      "decision",
		Title:     "Create auth service",
		Content:   "content",
		Workspace: "vela",
		References: []listener.Reference{
			{Type: "file", Target: "internal/auth/service.go"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	})
	updatedPayload, _ := json.Marshal(listener.ObservationPayload{
		ID:        2,
		Type:      "bugfix",
		Title:     "Fix queue",
		Content:   "content",
		Workspace: "vela",
		CreatedAt: now,
		UpdatedAt: now,
	})
	deletedPayload, _ := json.Marshal(listener.ObservationDeletedPayload{ID: 3})
	sessionPayload := json.RawMessage(`{"session_id":"abc"}`)

	cs, err := NewDiffer().Diff([]listener.Event{
		{Type: listener.EventObservationCreated, Payload: createdPayload},
		{Type: listener.EventObservationUpdated, Payload: updatedPayload},
		{Type: listener.EventObservationDeleted, Payload: deletedPayload},
		{Type: listener.EventSessionCreated, Payload: sessionPayload},
		{Type: listener.EventType("unknown"), Payload: json.RawMessage(`{}`)},
	})
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}

	if len(cs.Added) != 1 || len(cs.Updated) != 1 || len(cs.Deleted) != 1 {
		t.Fatalf("ChangeSet sizes = added:%d updated:%d deleted:%d", len(cs.Added), len(cs.Updated), len(cs.Deleted))
	}
	if cs.Added[0].NodeType != types.NodeTypeObservation {
		t.Fatalf("added node type = %q, want %q", cs.Added[0].NodeType, types.NodeTypeObservation)
	}
	if cs.Added[0].ID != "ancora:obs:1" {
		t.Fatalf("added node ID = %q, want ancora:obs:1", cs.Added[0].ID)
	}
	if got := cs.Added[0].References; len(got) != 1 || got[0].Target != "internal/auth/service.go" {
		t.Fatalf("added references = %#v, want file reference", got)
	}
	if cs.Deleted[0] != 3 {
		t.Fatalf("deleted ID = %d, want 3", cs.Deleted[0])
	}
}

func TestEntityKeyUsesObservationIDAndSessionID(t *testing.T) {
	t.Parallel()

	obsPayload := json.RawMessage(`{"id":42}`)
	sessPayload := json.RawMessage(`{"session_id":"sess-1"}`)

	if got := EntityKey(listener.Event{Type: listener.EventObservationUpdated, Payload: obsPayload}); got != "obs:42" {
		t.Fatalf("EntityKey(observation) = %q, want obs:42", got)
	}
	if got := EntityKey(listener.Event{Type: listener.EventSessionEnded, Payload: sessPayload}); got != "sess:sess-1" {
		t.Fatalf("EntityKey(session) = %q, want sess:sess-1", got)
	}
}
