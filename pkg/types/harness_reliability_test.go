package types

import (
	"encoding/json"
	"testing"
)

// Scenario: Aggregate publication preserves child provenance. Additive metadata
// must not change legacy manifest meaning or conflate identical relative paths.
func TestHarnessReliabilityManifestCompatibility(t *testing.T) {
	var legacy Manifest
	if err := json.Unmarshal([]byte(`{"version":1,"repo_root":"/fixture","extractor_fingerprint":"old","files":[{"path":"a.go","sha256":"abc"}]}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.InventoryComplete || legacy.Aggregate != nil || legacy.Files[0].Path != "a.go" {
		t.Fatal("legacy provenance fabricated")
	}
	m := Manifest{Version: 2, Aggregate: &AggregateManifest{ParentRoot: "/parent", ChildRoots: []string{"/parent/x/repo", "/parent/y/repo"}, Children: []ChildSnapshot{{Root: "/parent/x/repo", Generation: "g-1", Manifest: legacy}, {Root: "/parent/y/repo", Generation: "g-2", Manifest: legacy}}}}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var restored Manifest
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if len(restored.Aggregate.Children) != 2 || restored.Aggregate.Children[0].Root == restored.Aggregate.Children[1].Root {
		t.Fatal("child identity conflated")
	}
}
