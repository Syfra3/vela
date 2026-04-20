package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/Syfra3/vela/internal/query"
	"github.com/Syfra3/vela/pkg/types"
)

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "empty", input: "", want: nil},
		{name: "single", input: "calls", want: []string{"calls"}},
		{name: "multiple with spaces", input: "calls, uses , references", want: []string{"calls", "uses", "references"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitCSV(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("splitCSV(%q) = %#v, want %#v", tt.input, got, tt.want)
			}
		})
	}
}

func TestWriteSearchResponse_ShowsRoutingAndLayerEvidence(t *testing.T) {
	var buf bytes.Buffer
	writeSearchResponse(&buf, query.SearchResponse{
		Routing: query.SearchRouting{
			RoutedRepos: []query.SearchRoute{{Repo: "billing-api", Score: 3.5, Reasons: []string{"service:billing"}}},
		},
		Hits: []query.SearchHit{{
			Label:         "Billing architecture note",
			Kind:          "architecture",
			PrimarySource: "ancora",
			PrimaryLayer:  "memory",
			Layers:        []string{"memory", "workspace", "contract", "repo"},
			Provenance: []query.SearchProvenance{
				{Layer: "memory", Source: "ancora"},
				{Layer: "workspace", Source: "vela_graph", Signal: "routing", Repo: "billing-api", Reasons: []string{"service:billing"}},
				{Layer: "contract", Source: "vela_graph", Signal: "lexical", Repo: "billing-api"},
				{Layer: "repo", Source: "vela_graph", Signal: "structural", Repo: "billing-api"},
			},
			Score:   9.2,
			Path:    "vela",
			Snippet: "Ancora memory plus routed repo evidence.",
		}},
	}, false)

	out := buf.String()
	for _, want := range []string{
		"Routing",
		"billing-api score=3.50 reasons=service:billing",
		"[memory/ancora] Billing architecture note",
		"layers: memory, workspace, contract, repo",
		"memory via ancora",
		"workspace/routing via vela_graph repo=billing-api reasons=service:billing",
		"contract/lexical via vela_graph repo=billing-api",
		"repo/structural via vela_graph repo=billing-api",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestFormatLayerSummary(t *testing.T) {
	summary := formatLayerSummary([]types.Node{
		{ID: "project:vela", Label: "vela", NodeType: string(types.NodeTypeProject), Source: &types.Source{Type: types.SourceTypeCodebase, Name: "vela"}},
		{ID: "contract:service:billing", Label: "billing", NodeType: string(types.NodeTypeService), Metadata: map[string]interface{}{"layer": "contract"}},
		{ID: "workspace:repo:vela", Label: "vela", NodeType: string(types.NodeTypeRepo), Metadata: map[string]interface{}{"layer": "workspace"}},
		{ID: "memory:observation:1", Label: "note", NodeType: string(types.NodeTypeObservation), Metadata: map[string]interface{}{"layer": "memory"}},
	})
	if summary != "repo=1, contract=1, workspace=1, memory=1" {
		t.Fatalf("formatLayerSummary() = %q", summary)
	}
}
