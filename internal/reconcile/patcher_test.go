package reconcile

import (
	"testing"
	"time"

	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/pkg/types"
)

func TestPatcherApplyObservationLifecycle(t *testing.T) {
	t.Parallel()

	g, err := igraph.Build(nil, nil)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	p := NewPatcher(g, 4)
	now := time.Now().UTC()

	obs := types.ObservationNode{
		ID:           "ancora:obs:7",
		NodeType:     types.NodeTypeObservation,
		AncoraID:     7,
		Title:        "Initial title",
		Content:      "Initial content",
		ObsType:      "decision",
		Workspace:    "vela",
		Visibility:   "work",
		Organization: "glim",
		TopicKey:     "architecture/auth",
		References: []types.ObsReference{
			{Type: "file", Target: "internal/auth/service.go"},
			{Type: "concept", Target: "Auth Architecture"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := p.Apply(ChangeSet{Added: []types.ObservationNode{obs}}); err != nil {
		t.Fatalf("Apply(add) error = %v", err)
	}
	if g.NodeCount() != 5 {
		t.Fatalf("NodeCount() after add = %d, want 5", g.NodeCount())
	}
	if len(g.ResolvedEdges) != 2 {
		t.Fatalf("ResolvedEdges after add = %d, want 2", len(g.ResolvedEdges))
	}
	for _, nodeID := range []string{"memory:ancora", "ancora:workspace:vela", "ancora:visibility:work", "ancora:organization:glim", obs.ID} {
		if _, ok := g.NodeIndex[nodeID]; !ok {
			t.Fatalf("expected node %q in graph", nodeID)
		}
	}
	if queued := <-p.LLMQueue(); queued.AncoraID != 7 {
		t.Fatalf("LLM queue node ID = %d, want 7", queued.AncoraID)
	}

	updated := obs
	updated.Title = "Updated title"
	updated.Content = "Updated content"
	updated.References = []types.ObsReference{{Type: "observation", Target: "12"}}
	updated.UpdatedAt = now.Add(time.Minute)

	if err := p.Apply(ChangeSet{Updated: []types.ObservationNode{updated}}); err != nil {
		t.Fatalf("Apply(update) error = %v", err)
	}
	if len(g.ResolvedEdges) != 1 {
		t.Fatalf("ResolvedEdges after update = %d, want 1", len(g.ResolvedEdges))
	}
	if got := g.ResolvedEdges[0]; got.Relation != string(types.EdgeTypeRelatedTo) || got.Target != "12" {
		t.Fatalf("updated edge = %#v, want related_to observation edge", got)
	}

	gonumID := g.NodeIndex[obs.ID]
	updatedNode := g.NodeByID[gonumID]
	if updatedNode.Label != "Updated title" {
		t.Fatalf("updated node label = %q, want Updated title", updatedNode.Label)
	}
	if updatedNode.Metadata["content"] != "Updated content" {
		t.Fatalf("updated node content = %v, want Updated content", updatedNode.Metadata["content"])
	}
	if updatedNode.Metadata["visibility"] != "work" {
		t.Fatalf("updated node visibility = %v, want work", updatedNode.Metadata["visibility"])
	}
	if updatedNode.Metadata["organization"] != "glim" {
		t.Fatalf("updated node organization = %v, want glim", updatedNode.Metadata["organization"])
	}
	if updatedNode.Metadata["topic_key"] != "architecture/auth" {
		t.Fatalf("updated node topic_key = %v, want architecture/auth", updatedNode.Metadata["topic_key"])
	}

	if err := p.Apply(ChangeSet{Deleted: []int64{7}}); err != nil {
		t.Fatalf("Apply(delete) error = %v", err)
	}
	if g.NodeCount() != 4 {
		t.Fatalf("NodeCount() after delete = %d, want 4", g.NodeCount())
	}
	if len(g.ResolvedEdges) != 0 {
		t.Fatalf("ResolvedEdges after delete = %d, want 0", len(g.ResolvedEdges))
	}
}
