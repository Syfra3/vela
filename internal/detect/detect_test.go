package detect

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestCollect_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	files, err := Collect(dir, []string{".go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestCollect_SingleMatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.go"), "package main")
	writeFile(t, filepath.Join(dir, "readme.md"), "# readme")

	files, err := Collect(dir, []string{".go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d: %v", len(files), files)
	}
	if filepath.Base(files[0]) != "main.go" {
		t.Errorf("expected main.go, got %s", files[0])
	}
}

func TestCollect_VelignoreExclusion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.go"), "package main")
	writeFile(t, filepath.Join(dir, "generated.go"), "package main")
	writeFile(t, filepath.Join(dir, ".velignore"), "generated.go\n")

	files, err := Collect(dir, []string{".go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d: %v", len(files), files)
	}
	if filepath.Base(files[0]) != "main.go" {
		t.Errorf("expected main.go, got %s", files[0])
	}
}

func TestCollect_NestedDirs(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "internal", "pkg")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "a.go"), "package a")
	writeFile(t, filepath.Join(sub, "b.go"), "package b")
	writeFile(t, filepath.Join(sub, "c.md"), "# doc")

	files, err := Collect(dir, []string{".go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}
	sort.Strings(files)
	if filepath.Base(files[0]) != "a.go" && filepath.Base(files[1]) != "b.go" {
		t.Errorf("unexpected files: %v", files)
	}
}

func TestCollect_VelignoreDirectory(t *testing.T) {
	dir := t.TempDir()
	vendorDir := filepath.Join(dir, "vendor")
	if err := os.MkdirAll(vendorDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "main.go"), "package main")
	writeFile(t, filepath.Join(vendorDir, "dep.go"), "package dep")
	writeFile(t, filepath.Join(dir, ".velignore"), "vendor/\n")

	files, err := Collect(dir, []string{".go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d: %v", len(files), files)
	}
	if filepath.Base(files[0]) != "main.go" {
		t.Errorf("expected main.go, got %s", files[0])
	}
}

func TestCollect_NoExtFilter(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.go"), "package main")
	writeFile(t, filepath.Join(dir, "readme.md"), "# doc")

	files, err := Collect(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
}

func TestCollect_GlobPattern(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "foo.pb.go"), "package pb")
	writeFile(t, filepath.Join(dir, "main.go"), "package main")
	writeFile(t, filepath.Join(dir, ".velignore"), "*.pb.go\n")

	files, err := Collect(dir, []string{".go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d: %v", len(files), files)
	}
	if filepath.Base(files[0]) != "main.go" {
		t.Errorf("expected main.go, got %s", files[0])
	}
}

// helper
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
