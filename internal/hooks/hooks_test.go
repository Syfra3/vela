package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallCreatesManagedHooks(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Install(repo, "/usr/local/bin/vela"); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	for _, hookName := range managedHooks {
		data, err := os.ReadFile(filepath.Join(repo, ".git", "hooks", hookName))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", hookName, err)
		}
		content := string(data)
		for _, want := range []string{"#!/bin/sh", startMarker, endMarker, "\"/usr/local/bin/vela\" update \"" + repo + "\""} {
			if !strings.Contains(content, want) {
				t.Fatalf("expected %q in %s, got %q", want, hookName, content)
			}
		}
	}
}

func TestInstallPreservesExistingHookContent(t *testing.T) {
	repo := t.TempDir()
	hooksDir := filepath.Join(repo, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	hookPath := filepath.Join(hooksDir, "post-commit")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\necho existing\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Install(repo, "/vela"); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "echo existing") {
		t.Fatalf("expected existing hook content to survive, got %q", content)
	}
	if strings.Count(content, startMarker) != 1 {
		t.Fatalf("expected one managed block, got %q", content)
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Install(repo, "/vela"); err != nil {
		t.Fatal(err)
	}
	if err := Install(repo, "/vela"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(repo, ".git", "hooks", "post-commit"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), startMarker) != 1 {
		t.Fatalf("expected one managed block, got %q", string(data))
	}
}

func TestUninstallRemovesOnlyManagedBlock(t *testing.T) {
	repo := t.TempDir()
	hooksDir := filepath.Join(repo, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	postCommit := filepath.Join(hooksDir, "post-commit")
	content := "#!/bin/sh\necho existing\n\n" + managedBlock(repo, "/vela") + "\n"
	if err := os.WriteFile(postCommit, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Uninstall(repo); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	data, err := os.ReadFile(postCommit)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if strings.Contains(got, startMarker) || strings.Contains(got, endMarker) {
		t.Fatalf("expected managed block removed, got %q", got)
	}
	if !strings.Contains(got, "echo existing") {
		t.Fatalf("expected existing content preserved, got %q", got)
	}
}

func TestInspectReportsHookState(t *testing.T) {
	repo := t.TempDir()
	hooksDir := filepath.Join(repo, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooksDir, "post-commit"), []byte("#!/bin/sh\n"+managedBlock(repo, "/vela")), 0o755); err != nil {
		t.Fatal(err)
	}
	status, err := Inspect(repo)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if !status.Hooks["post-commit"] {
		t.Fatal("expected post-commit installed")
	}
	if status.Hooks["post-checkout"] {
		t.Fatal("expected post-checkout not installed")
	}
}
