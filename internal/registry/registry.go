package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Syfra3/vela/internal/config"
	"github.com/Syfra3/vela/internal/extract"
)

const fileVersion = 1

type Entry struct {
	RepoRoot     string    `json:"repo_root"`
	Name         string    `json:"name"`
	Remote       string    `json:"remote,omitempty"`
	GraphPath    string    `json:"graph_path,omitempty"`
	ManifestPath string    `json:"manifest_path,omitempty"`
	ReportPath   string    `json:"report_path,omitempty"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
}

type file struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

func Load() ([]Entry, error) {
	path := config.RegistryFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read registry: %w", err)
	}
	var stored file
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("parse registry: %w", err)
	}
	entries := make([]Entry, 0, len(stored.Entries))
	for _, entry := range stored.Entries {
		if normalized, ok := normalize(entry); ok {
			entries = append(entries, normalized)
		}
	}
	sortEntries(entries)
	return entries, nil
}

func Save(entries []Entry) error {
	normalized := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if clean, ok := normalize(entry); ok {
			normalized = append(normalized, clean)
		}
	}
	sortEntries(normalized)
	path := config.RegistryFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create registry dir: %w", err)
	}
	data, err := json.MarshalIndent(file{Version: fileVersion, Entries: normalized}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write registry: %w", err)
	}
	return nil
}

func UpsertTrackedRepo(repoRoot, graphPath, reportPath string) error {
	if strings.TrimSpace(repoRoot) == "" {
		return fmt.Errorf("repo root is required")
	}
	source := extract.DetectProject(repoRoot)
	entry := Entry{
		RepoRoot:     source.Path,
		Name:         source.Name,
		Remote:       source.Remote,
		GraphPath:    strings.TrimSpace(graphPath),
		ManifestPath: filepath.Join(filepath.Dir(strings.TrimSpace(graphPath)), "manifest.json"),
		ReportPath:   strings.TrimSpace(reportPath),
		UpdatedAt:    time.Now().UTC(),
	}
	entries, err := Load()
	if err != nil {
		return err
	}
	updated := false
	for i := range entries {
		if entries[i].RepoRoot == entry.RepoRoot {
			entries[i] = entry
			updated = true
			break
		}
	}
	if !updated {
		entries = append(entries, entry)
	}
	return Save(entries)
}

func RemoveTrackedRepo(repoRoot string) error {
	cleanRoot := cleanPath(repoRoot)
	entries, err := Load()
	if err != nil {
		return err
	}
	filtered := entries[:0]
	for _, entry := range entries {
		if entry.RepoRoot == cleanRoot {
			continue
		}
		filtered = append(filtered, entry)
	}
	return Save(filtered)
}

func normalize(entry Entry) (Entry, bool) {
	entry.RepoRoot = cleanPath(entry.RepoRoot)
	entry.GraphPath = cleanPath(entry.GraphPath)
	entry.ManifestPath = cleanPath(entry.ManifestPath)
	entry.ReportPath = cleanPath(entry.ReportPath)
	entry.Name = strings.TrimSpace(entry.Name)
	entry.Remote = strings.TrimSpace(entry.Remote)
	if entry.RepoRoot == "" {
		return Entry{}, false
	}
	if entry.Name == "" {
		entry.Name = filepath.Base(entry.RepoRoot)
	}
	return entry, true
}

func cleanPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	if abs, err := filepath.Abs(trimmed); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(trimmed)
}

func sortEntries(entries []Entry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name == entries[j].Name {
			return entries[i].RepoRoot < entries[j].RepoRoot
		}
		return entries[i].Name < entries[j].Name
	})
}
