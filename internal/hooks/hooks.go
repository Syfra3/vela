package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	startMarker = "# >>> vela hooks >>>"
	endMarker   = "# <<< vela hooks <<<"
)

var managedHooks = []string{"post-commit", "post-checkout"}

type Status struct {
	RepoRoot string
	Hooks    map[string]bool
}

func Install(repoRoot, executablePath string) error {
	if strings.TrimSpace(repoRoot) == "" {
		return fmt.Errorf("repo root is required")
	}
	if strings.TrimSpace(executablePath) == "" {
		return fmt.Errorf("executable path is required")
	}
	hooksDir := filepath.Join(repoRoot, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("creating hooks dir: %w", err)
	}
	for _, hookName := range managedHooks {
		hookPath := filepath.Join(hooksDir, hookName)
		existing, err := os.ReadFile(hookPath)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("reading %s: %w", hookName, err)
		}
		updated := upsertManagedBlock(string(existing), managedBlock(repoRoot, executablePath))
		if err := os.WriteFile(hookPath, []byte(updated), 0o755); err != nil {
			return fmt.Errorf("writing %s: %w", hookName, err)
		}
	}
	return nil
}

func Uninstall(repoRoot string) error {
	if strings.TrimSpace(repoRoot) == "" {
		return fmt.Errorf("repo root is required")
	}
	for _, hookName := range managedHooks {
		hookPath := filepath.Join(repoRoot, ".git", "hooks", hookName)
		existing, err := os.ReadFile(hookPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("reading %s: %w", hookName, err)
		}
		updated := removeManagedBlock(string(existing))
		if strings.TrimSpace(updated) == "#!/bin/sh" || strings.TrimSpace(updated) == "" {
			if err := os.Remove(hookPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("removing %s: %w", hookName, err)
			}
			continue
		}
		if err := os.WriteFile(hookPath, []byte(updated), 0o755); err != nil {
			return fmt.Errorf("writing %s: %w", hookName, err)
		}
	}
	return nil
}

func Inspect(repoRoot string) (Status, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return Status{}, fmt.Errorf("repo root is required")
	}
	status := Status{RepoRoot: repoRoot, Hooks: make(map[string]bool, len(managedHooks))}
	for _, hookName := range managedHooks {
		data, err := os.ReadFile(filepath.Join(repoRoot, ".git", "hooks", hookName))
		if err != nil {
			if os.IsNotExist(err) {
				status.Hooks[hookName] = false
				continue
			}
			return Status{}, fmt.Errorf("reading %s: %w", hookName, err)
		}
		status.Hooks[hookName] = strings.Contains(string(data), startMarker) && strings.Contains(string(data), endMarker)
	}
	return status, nil
}

func managedBlock(repoRoot, executablePath string) string {
	repoRoot = filepath.Clean(repoRoot)
	executablePath = filepath.Clean(executablePath)
	return fmt.Sprintf("%s\n\"%s\" update \"%s\" >/dev/null 2>&1 || true\n%s", startMarker, executablePath, repoRoot, endMarker)
}

func upsertManagedBlock(existing, block string) string {
	trimmed := strings.TrimSpace(existing)
	if trimmed == "" {
		return "#!/bin/sh\n\n" + block + "\n"
	}
	if !strings.HasPrefix(existing, "#!/bin/sh") {
		existing = "#!/bin/sh\n\n" + strings.TrimLeft(existing, "\n")
	}
	if strings.Contains(existing, startMarker) && strings.Contains(existing, endMarker) {
		return replaceManagedBlock(existing, block)
	}
	if !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	return existing + "\n" + block + "\n"
}

func removeManagedBlock(existing string) string {
	if !strings.Contains(existing, startMarker) || !strings.Contains(existing, endMarker) {
		return existing
	}
	start := strings.Index(existing, startMarker)
	end := strings.Index(existing[start:], endMarker)
	if start < 0 || end < 0 {
		return existing
	}
	end += start + len(endMarker)
	remaining := existing[:start] + existing[end:]
	remaining = strings.ReplaceAll(remaining, "\n\n\n", "\n\n")
	return strings.TrimRight(remaining, "\n") + "\n"
}

func replaceManagedBlock(existing, block string) string {
	start := strings.Index(existing, startMarker)
	end := strings.Index(existing[start:], endMarker)
	if start < 0 || end < 0 {
		return existing
	}
	end += start + len(endMarker)
	return existing[:start] + block + existing[end:]
}
