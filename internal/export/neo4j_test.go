package export

import "testing"

func TestNeo4jLabel(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"function", "Function"},
		{"struct", "Struct"},
		{"interface", "Interface"},
		{"method", "Method"},
		{"concept", "Concept"},
		{"unknown", "Node"},
	}
	for _, tc := range cases {
		got := neo4jLabel(tc.input)
		if got != tc.want {
			t.Errorf("neo4jLabel(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestNeo4jRelType(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"calls", "CALLS"},
		{"imports", "IMPORTS"},
		{"uses", "USES"},
		{"implements", "IMPLEMENTS"},
		{"extends", "EXTENDS"},
		{"describes", "DESCRIBES"},
		{"related_to", "RELATED_TO"},
		{"unknown", "RELATED_TO"},
	}
	for _, tc := range cases {
		got := neo4jRelType(tc.input)
		if got != tc.want {
			t.Errorf("neo4jRelType(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
