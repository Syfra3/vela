package extract

import "testing"

func TestParseReferencesExtractsAndDeduplicates(t *testing.T) {
	t.Parallel()

	title := "Auth decision touches `AuthService` and [[Security]]"
	content := "See internal/auth/service.go and internal/auth/service.go. Related to #42 and obs:42."

	refs := ParseReferences(title, content)
	got := map[string]string{}
	for _, ref := range refs {
		got[ref.Type] = ref.Target
	}

	if got["file"] != "internal/auth/service.go" {
		t.Fatalf("file ref = %q, want internal/auth/service.go", got["file"])
	}
	if got["function"] != "AuthService" {
		t.Fatalf("function ref = %q, want AuthService", got["function"])
	}
	if got["concept"] != "Security" {
		t.Fatalf("concept ref = %q, want Security", got["concept"])
	}
	if got["observation"] != "42" {
		t.Fatalf("observation ref = %q, want 42", got["observation"])
	}
	if len(refs) != 4 {
		t.Fatalf("len(refs) = %d, want 4 deduplicated refs", len(refs))
	}
}
