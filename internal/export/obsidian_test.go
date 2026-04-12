package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Syfra3/vela/pkg/types"
)

func TestWriteObsidian(t *testing.T) {
	g := &types.Graph{
		Nodes: []types.Node{
			{ID: "auth", Label: "AuthService", NodeType: "struct", SourceFile: "auth.go", Community: 0},
			{ID: "db", Label: "Database", NodeType: "struct", SourceFile: "db.go", Community: 1},
		},
		Edges: []types.Edge{
			{Source: "auth", Target: "Database", Relation: "uses", Confidence: "EXTRACTED"},
		},
		Communities: map[int][]string{},
		ExtractedAt: time.Now(),
	}

	outDir := t.TempDir()
	if err := WriteObsidian(g, outDir); err != nil {
		t.Fatalf("WriteObsidian error: %v", err)
	}

	vaultDir := filepath.Join(outDir, "obsidian")

	// Check AuthService.md exists
	authPath := filepath.Join(vaultDir, "AuthService.md")
	data, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("AuthService.md not created: %v", err)
	}
	content := string(data)

	// Must have frontmatter
	if !strings.Contains(content, "---") {
		t.Error("missing YAML frontmatter")
	}
	if !strings.Contains(content, `kind: "struct"`) {
		t.Errorf("missing kind in frontmatter, got:\n%s", content)
	}

	// Must have wikilink to Database
	if !strings.Contains(content, "[[Database]]") {
		t.Errorf("missing wikilink to Database, got:\n%s", content)
	}

	// Database.md must also exist
	dbPath := filepath.Join(vaultDir, "Database.md")
	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("Database.md not created: %v", err)
	}
}
