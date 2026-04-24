package app

import (
	"strings"
	"testing"

	"github.com/Syfra3/vela/pkg/types"
)

func TestParseSearchQuery_RoutesStructuralPrompts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want types.QueryRequest
	}{
		{
			name: "reverse dependencies by usage wording",
			raw:  "who uses rootCmd",
			want: types.QueryRequest{Kind: types.QueryKindReverseDependencies, Subject: "rootCmd", Limit: types.DefaultQueryLimit, IncludeProvenance: true},
		},
		{
			name: "reverse dependencies by where used wording",
			raw:  "where is internal/query/query.go used?",
			want: types.QueryRequest{Kind: types.QueryKindReverseDependencies, Subject: "internal/query/query.go", Limit: types.DefaultQueryLimit, IncludeProvenance: true},
		},
		{
			name: "dependencies wording",
			raw:  "what does loadEngine depend on",
			want: types.QueryRequest{Kind: types.QueryKindDependencies, Subject: "loadEngine", Limit: types.DefaultQueryLimit, IncludeProvenance: true},
		},
		{
			name: "impact wording",
			raw:  "what breaks if loadEngine changes?",
			want: types.QueryRequest{Kind: types.QueryKindImpact, Subject: "loadEngine", Limit: types.DefaultQueryLimit, IncludeProvenance: true},
		},
		{
			name: "path wording",
			raw:  "path rootCmd -> graphQueryCmd",
			want: types.QueryRequest{Kind: types.QueryKindPath, Subject: "rootCmd", Target: "graphQueryCmd", Limit: types.DefaultQueryLimit, IncludeProvenance: true},
		},
		{
			name: "how does wording",
			raw:  "how does rootCmd reach graphQueryCmd?",
			want: types.QueryRequest{Kind: types.QueryKindPath, Subject: "rootCmd", Target: "graphQueryCmd", Limit: types.DefaultQueryLimit, IncludeProvenance: true},
		},
		{
			name: "explain wording",
			raw:  "explain `rootCmd`",
			want: types.QueryRequest{Kind: types.QueryKindExplain, Subject: "rootCmd", Limit: types.DefaultQueryLimit, IncludeProvenance: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSearchQuery(tt.raw)
			if err != nil {
				t.Fatalf("ParseSearchQuery(%q) error = %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("ParseSearchQuery(%q) = %+v, want %+v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestParseSearchQuery_RejectsUnsupportedPrompt(t *testing.T) {
	t.Parallel()

	_, err := ParseSearchQuery("find the TODO comment")
	if err == nil {
		t.Fatal("ParseSearchQuery() error = nil, want unsupported query error")
	}
	if !strings.Contains(err.Error(), "unsupported structural search") {
		t.Fatalf("unexpected error %q", err)
	}
}
