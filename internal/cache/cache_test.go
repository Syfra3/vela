package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCache_MissOnFirstCall(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.IsCached("file.go", "abc123") {
		t.Error("expected cache miss on first call")
	}
}

func TestCache_HitAfterMark(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	c.Mark("file.go", "abc123")
	if !c.IsCached("file.go", "abc123") {
		t.Error("expected cache hit after Mark")
	}
}

func TestCache_MissOnDifferentSHA(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	c.Mark("file.go", "abc123")
	if c.IsCached("file.go", "different") {
		t.Error("expected cache miss for different SHA")
	}
}

func TestCache_PersistsAcrossLoad(t *testing.T) {
	dir := t.TempDir()

	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	c.Mark("file.go", "abc123")
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	// Load again from same dir
	c2, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !c2.IsCached("file.go", "abc123") {
		t.Error("expected cache hit after reload from disk")
	}
}

func TestSHA256File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	if err := os.WriteFile(path, []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	sha1, err := SHA256File(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(sha1) != 64 {
		t.Errorf("expected 64-char hex SHA, got %d chars", len(sha1))
	}

	// Same content → same hash
	sha2, _ := SHA256File(path)
	if sha1 != sha2 {
		t.Error("expected same SHA for same content")
	}

	// Different content → different hash
	if err := os.WriteFile(path, []byte("package other"), 0644); err != nil {
		t.Fatal(err)
	}
	sha3, _ := SHA256File(path)
	if sha1 == sha3 {
		t.Error("expected different SHA for different content")
	}
}

func TestCache_DeletePrefix(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	c.Mark(filepath.Join(dir, "repo", "main.go"), "a")
	c.Mark(filepath.Join(dir, "repo", "pkg", "util.go"), "b")
	c.Mark(filepath.Join(dir, "other", "main.go"), "c")

	if !c.DeletePrefix(filepath.Join(dir, "repo")) {
		t.Fatal("expected DeletePrefix to report removals")
	}
	if c.IsCached(filepath.Join(dir, "repo", "main.go"), "a") {
		t.Fatal("expected repo cache entry removed")
	}
	if c.IsCached(filepath.Join(dir, "repo", "pkg", "util.go"), "b") {
		t.Fatal("expected nested repo cache entry removed")
	}
	if !c.IsCached(filepath.Join(dir, "other", "main.go"), "c") {
		t.Fatal("expected unrelated cache entry preserved")
	}
}
