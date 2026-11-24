package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Syfra3/vela/internal/query"
)

func TestSelectionDiagnosticsNeverExposeGraphFacts(t *testing.T) {
	candidates := []query.CorpusCandidate{{Project: "p", Root: "/one", GraphPath: "/one/graph.json"}, {Project: "p", Root: "/two", GraphPath: "/two/graph.json"}}
	for _, tc := range []struct {
		name   string
		engine *query.Engine
		code   int
		want   string
	}{
		{"ambiguous", query.NewAmbiguousEngine(candidates), http.StatusConflict, "/one/graph.json"},
		{"invalid", query.NewUnavailableEngine(query.UnavailableDiagnostic{Message: "invalid selected graph"}), http.StatusServiceUnavailable, "invalid selected graph"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(tc.engine, 0)
			for _, path := range []string{"/graph", "/query?kind=dependencies&subject=A"} {
				rec := httptest.NewRecorder()
				s.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
				if rec.Code != tc.code || !strings.Contains(rec.Body.String(), tc.want) || strings.Contains(rec.Body.String(), "nodes") {
					t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
				}
				if tc.name == "ambiguous" && !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
					t.Fatal(rec.Header())
				}
			}
		})
	}
}
