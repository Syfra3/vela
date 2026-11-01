package tui

import (
	"context"
	"encoding/json"
	"github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/internal/registry"
	"github.com/Syfra3/vela/pkg/types"
	"os"
	"path/filepath"
	"testing"
)

// Only refusal paths run: no hooks, registry, real uninstall or GUI is invoked.
func TestHarnessReliabilityDeletionRefusesBeforeAnySideEffect(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "nested", ".vela")
	if err := os.MkdirAll(filepath.Join(out, ".generations"), 0755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "keep")
	if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	p := trackedProject{Path: filepath.Dir(out), GraphPath: filepath.Join(out, "graph.json")}
	if _, err := deleteTrackedProjects(p.GraphPath, []trackedProject{p}); err == nil {
		t.Fatal("project deletion accepted")
	}
	oldTargets, oldRepos := uninstallTargetsFunc, uninstallTrackedReposFunc
	t.Cleanup(func() { uninstallTargetsFunc = oldTargets; uninstallTrackedReposFunc = oldRepos })
	uninstallTargetsFunc = func() ([]string, error) { return []string{root}, nil }
	uninstallTrackedReposFunc = func() ([]string, error) { return []string{root}, nil }
	if _, err := uninstallAll(); err == nil {
		t.Fatal("nested uninstall accepted")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep" {
		t.Fatal("partial removal")
	}
}

func TestHarnessReliabilityFirstAdoptionPreservesHookAndRegistryBytes(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	out := filepath.Join(root, ".vela")
	hook := filepath.Join(root, ".git", "hooks", "post-commit")
	if err := os.MkdirAll(filepath.Dir(hook), 0700); err != nil {
		t.Fatal(err)
	}
	hookBytes := []byte("#!/bin/sh\n# >>> vela hooks >>>\nfixture-only\n# <<< vela hooks <<<\n")
	if err := os.WriteFile(hook, hookBytes, 0600); err != nil {
		t.Fatal(err)
	}
	registryPath := filepath.Join(home, ".vela", "registry.json")
	if err := os.MkdirAll(filepath.Dir(registryPath), 0700); err != nil {
		t.Fatal(err)
	}
	registryBytes, _ := json.Marshal(map[string]interface{}{"version": 1, "entries": []registry.Entry{{RepoRoot: root, Name: "fixture", GraphPath: filepath.Join(out, "graph.json")}}})
	if err := os.WriteFile(registryPath, registryBytes, 0600); err != nil {
		t.Fatal(err)
	}
	w, err := export.AcquireWriter(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(filepath.Join(out, ".writer.lock"))
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		close(started)
		_, err := deleteTrackedProjects(filepath.Join(out, "graph.json"), []trackedProject{{Path: root, GraphPath: filepath.Join(out, "graph.json")}})
		result <- err
	}()
	<-started
	m, _, err := export.Inventory(root, types.ManifestRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Publish(&types.Graph{}, m, nil); err != nil {
		t.Fatal(err)
	}
	w.Close()
	if err := <-result; err == nil {
		t.Fatal("first-adoption purge succeeded")
	}
	origTargets, origRepos := uninstallTargetsFunc, uninstallTrackedReposFunc
	t.Cleanup(func() { uninstallTargetsFunc = origTargets; uninstallTrackedReposFunc = origRepos })
	uninstallTargetsFunc = func() ([]string, error) { return []string{root}, nil }
	uninstallTrackedReposFunc = func() ([]string, error) { return []string{root}, nil }
	if _, err := uninstallAll(); err == nil {
		t.Fatal("uninstall accepted nested adoption")
	}
	for path, want := range map[string][]byte{hook: hookBytes, registryPath: registryBytes} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != string(want) {
			t.Fatalf("side effects preceded refusal: %s", path)
		}
	}
	after, err := os.Stat(filepath.Join(out, ".writer.lock"))
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("stable lock inode replaced")
	}
}
