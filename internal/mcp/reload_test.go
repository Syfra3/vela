package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Syfra3/vela/internal/query"
	mcppkg "github.com/mark3labs/mcp-go/mcp"
)

func TestProviderReloadsAndRejectsInvalidGraph(t *testing.T) {
	path := filepath.Join(t.TempDir(), "graph.json")
	provider := func() *query.Engine {
		e, err := query.LoadFromFile(path)
		if err != nil {
			return query.NewUnavailableEngine(query.UnavailableDiagnostic{Message: err.Error(), GraphPath: path})
		}
		return e
	}
	srv := NewServerWithProvider(provider)
	if len(srv.ListTools()) != 11 {
		t.Fatal("tools unavailable without graph")
	}
	call := func() string {
		r, err := srv.GetTool("dependencies").Handler(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: map[string]any{"subject": "A"}}})
		if err != nil {
			t.Fatal(err)
		}
		return callResultText(t, r)
	}
	if got := call(); !strings.Contains(got, "unavailable") {
		t.Fatal(got)
	}
	write := func(data string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b","kind":"uses"}]}`)
	if got := call(); !strings.Contains(got, "B") {
		t.Fatal(got)
	}
	write(`{"nodes":[{"id":"a","label":"A"},{"id":"c","label":"C"}],"edges":[{"from":"a","to":"c","kind":"uses"}]}`)
	if got := call(); !strings.Contains(got, "C") || strings.Contains(got, " B") {
		t.Fatal(got)
	}
	write(`broken`)
	if got := call(); !strings.Contains(got, "unavailable") || strings.Contains(got, "Dependencies for") {
		t.Fatal(got)
	}
}
