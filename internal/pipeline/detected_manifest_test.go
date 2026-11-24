package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Syfra3/vela/internal/generation"
	"github.com/Syfra3/vela/pkg/types"
)

func TestDetectedFilesFollowCapturedManifest(t *testing.T) {
	root := t.TempDir()
	canonical, err := generation.CanonicalRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	harnessPut(t, root, "go.mod", "module fixture\n")
	harnessPut(t, root, "a.go", "package fixture\nfunc A() {}\n")
	a, b := filepath.Join(canonical, "a.go"), filepath.Join(canonical, "b.go")
	calls := 0
	builder := NewBuilder(Config{Source: harnessSource, Detect: func(string) ([]string, error) {
		calls++
		if calls == 1 {
			// Detection has finished, but capture has not yet started.
			harnessPut(t, root, "b.go", "package fixture\nfunc B() {}\n")
			return []string{a}, nil
		}
		return []string{a, b}, nil
	}})
	for i := 0; i < 2; i++ {
		result, err := builder.Build(context.Background(), types.BuildRequest{RepoRoot: root})
		if err != nil {
			t.Fatal(err)
		}
		if result.Snapshot.Manifest.SourceBinding != generation.BoundGoProfile {
			t.Fatal("capture not bound")
		}
		if !reflect.DeepEqual(result.DetectedFiles, []string{a, b}) {
			t.Fatalf("build %d detected %v, want captured files", i, result.DetectedFiles)
		}
		if len(result.Snapshot.Manifest.Files) != 2 {
			t.Fatalf("manifest: %+v", result.Snapshot.Manifest.Files)
		}
		if i == 1 && result.CacheHits == 0 {
			t.Fatal("warm build did not reuse captured artifacts")
		}
		for _, file := range result.DetectedFiles {
			if _, err := os.Stat(file); err != nil {
				t.Fatalf("repository path %s: %v", file, err)
			}
		}
	}
}
