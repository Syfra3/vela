package types

import "testing"

func TestFactStableKey_NormalizesCodeTruthIdentity(t *testing.T) {
	fact := Fact{
		Repo:     "vela",
		Language: "go",
		Kind:     FactKindCalls,
		From:     "internal/query/query.go#Run",
		To:       "internal/graph/build.go#Merge",
	}

	got := fact.StableKey()
	want := "vela|go|calls|internal/query/query.go#Run|internal/graph/build.go#Merge"
	if got != want {
		t.Fatalf("Fact.StableKey() = %q, want %q", got, want)
	}
}

func TestBuildRequestNormalize_DefaultsPipelineAndDedupesHooks(t *testing.T) {
	req := BuildRequest{
		RepoRoot: "/repo",
		Drivers:  []string{"typescript", "go", "typescript", "go"},
		Patchers: []string{"lsp", "lsp", "manual"},
	}

	normalized := req.Normalize()

	if len(normalized.Stages) != 6 {
		t.Fatalf("Normalize() stages = %v, want 6 default stages", normalized.Stages)
	}
	if normalized.Stages[0] != BuildStageDetect || normalized.Stages[5] != BuildStagePersist {
		t.Fatalf("Normalize() stage boundaries = %v", normalized.Stages)
	}
	if got, want := normalized.Drivers, []string{"go", "typescript"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Normalize() drivers = %v, want %v", got, want)
	}
	if got, want := normalized.Patchers, []string{"lsp", "manual"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Normalize() patchers = %v, want %v", got, want)
	}
}

func TestQueryRequestValidate_RequiresKindSpecificFields(t *testing.T) {
	tests := []struct {
		name    string
		input   QueryRequest
		wantErr bool
	}{
		{
			name:    "dependency requires subject",
			input:   QueryRequest{Kind: QueryKindDependencies},
			wantErr: true,
		},
		{
			name:    "path requires subject and target",
			input:   QueryRequest{Kind: QueryKindPath, Subject: "a"},
			wantErr: true,
		},
		{
			name:    "impact requires subject",
			input:   QueryRequest{Kind: QueryKindImpact, Subject: "internal/query/query.go#Run", Limit: 25},
			wantErr: false,
		},
		{
			name:    "explain requires subject",
			input:   QueryRequest{Kind: QueryKindExplain, Subject: "internal/query/query.go#Run"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestQueryRequestNormalize_DefaultsExplainabilityFlags(t *testing.T) {
	req := QueryRequest{Kind: QueryKindReverseDependencies, Subject: "pkg/types/types.go#Config"}

	normalized := req.Normalize()

	if !normalized.IncludeProvenance {
		t.Fatal("Normalize() should enable provenance by default")
	}
	if normalized.Limit != DefaultQueryLimit {
		t.Fatalf("Normalize() limit = %d, want %d", normalized.Limit, DefaultQueryLimit)
	}
}
