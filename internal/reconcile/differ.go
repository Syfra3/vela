package reconcile

import (
	"encoding/json"
	"fmt"

	"github.com/Syfra3/vela/internal/listener"
	"github.com/Syfra3/vela/pkg/types"
)

// ChangeSet describes the minimal set of mutations needed to bring the graph
// up to date with a batch of incoming events.
type ChangeSet struct {
	Added   []types.ObservationNode
	Updated []types.ObservationNode
	Deleted []int64 // Ancora IDs of deleted observations
}

// IsEmpty reports whether the ChangeSet contains no mutations.
func (cs *ChangeSet) IsEmpty() bool {
	return len(cs.Added) == 0 && len(cs.Updated) == 0 && len(cs.Deleted) == 0
}

// Differ computes a ChangeSet from a batch of raw events. It is stateless —
// state tracking is the responsibility of the Reconciler / Patcher.
type Differ struct{}

// NewDiffer creates a Differ.
func NewDiffer() *Differ { return &Differ{} }

// Diff processes a batch of events and returns the ChangeSet to apply.
// Events for the same entity within the batch are collapsed: the last event
// for a given ID wins (which is guaranteed by Queue deduplication).
func (d *Differ) Diff(events []listener.Event) (ChangeSet, error) {
	var cs ChangeSet

	for _, ev := range events {
		switch ev.Type {
		case listener.EventObservationCreated:
			node, err := parseObservationNode(ev)
			if err != nil {
				return cs, fmt.Errorf("parsing created payload: %w", err)
			}
			cs.Added = append(cs.Added, node)

		case listener.EventObservationUpdated:
			node, err := parseObservationNode(ev)
			if err != nil {
				return cs, fmt.Errorf("parsing updated payload: %w", err)
			}
			cs.Updated = append(cs.Updated, node)

		case listener.EventObservationDeleted:
			var del listener.ObservationDeletedPayload
			if err := json.Unmarshal(ev.Payload, &del); err != nil {
				return cs, fmt.Errorf("parsing deleted payload: %w", err)
			}
			cs.Deleted = append(cs.Deleted, del.ID)

		// Session events are informational — no graph mutation needed.
		case listener.EventSessionCreated, listener.EventSessionEnded:
			continue

		default:
			// Unknown event type — skip without error.
		}
	}

	return cs, nil
}

// parseObservationNode unmarshals an observation payload into an ObservationNode.
func parseObservationNode(ev listener.Event) (types.ObservationNode, error) {
	var payload listener.ObservationPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return types.ObservationNode{}, err
	}

	refs := make([]types.ObsReference, len(payload.References))
	for i, r := range payload.References {
		refs[i] = types.ObsReference{Type: r.Type, Target: r.Target}
	}

	return types.ObservationNode{
		ID:         fmt.Sprintf("ancora:obs:%d", payload.ID),
		NodeType:   types.NodeTypeObservation,
		AncoraID:   payload.ID,
		Title:      payload.Title,
		Content:    payload.Content,
		ObsType:    payload.Type,
		Workspace:  payload.Workspace,
		References: refs,
		CreatedAt:  payload.CreatedAt,
		UpdatedAt:  payload.UpdatedAt,
	}, nil
}

// EntityKey returns a stable deduplication key for an observation event.
// For session events it returns the session ID; for unknown types, the raw type.
func EntityKey(ev listener.Event) string {
	switch ev.Type {
	case listener.EventObservationCreated,
		listener.EventObservationUpdated,
		listener.EventObservationDeleted:
		var p struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(ev.Payload, &p); err == nil && p.ID != 0 {
			return fmt.Sprintf("obs:%d", p.ID)
		}
	case listener.EventSessionCreated, listener.EventSessionEnded:
		var p struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(ev.Payload, &p); err == nil {
			return fmt.Sprintf("sess:%s", p.SessionID)
		}
	}
	return string(ev.Type)
}
