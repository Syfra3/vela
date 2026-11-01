package types

import "time"

// Manifest tracks file-level freshness for persisted graph outputs.
type Manifest struct {
	Version                  int                `json:"version"`
	RepoRoot                 string             `json:"repo_root,omitempty"`
	GeneratedAt              time.Time          `json:"generated_at"`
	ExtractorFingerprint     string             `json:"extractor_fingerprint"`
	BuildMode                string             `json:"build_mode,omitempty"`
	Files                    []ManifestFile     `json:"files,omitempty"`
	Generation               string             `json:"generation,omitempty"`
	InventoryComplete        bool               `json:"inventory_complete,omitempty"`
	DiscoveryFingerprint     string             `json:"discovery_fingerprint,omitempty"`
	ConfigFingerprint        string             `json:"config_fingerprint,omitempty"`
	RequestFingerprint       string             `json:"request_fingerprint,omitempty"`
	Request                  ManifestRequest    `json:"request,omitempty"`
	Coverage                 string             `json:"coverage,omitempty"`
	Gaps                     []string           `json:"gaps,omitempty"`
	Aggregate                *AggregateManifest `json:"aggregate,omitempty"`
	SourceBinding            string             `json:"source_binding,omitempty"`
	BindingReason            string             `json:"binding_reason,omitempty"`
	ReuseReason              string             `json:"reuse_reason,omitempty"`
	ConsumedFiles            []ManifestFile     `json:"consumed_files,omitempty"`
	DirectoryFingerprint     string             `json:"directory_fingerprint,omitempty"`
	DependencyFingerprint    string             `json:"dependency_fingerprint,omitempty"`
	ClosureConfigFingerprint string             `json:"closure_config_fingerprint,omitempty"`
}

// AggregateManifest embeds a vector of child snapshots, not a global instant.
// Canonical roots namespace both inventories and graph IDs (never basenames).
type AggregateManifest struct {
	ParentRoot           string          `json:"parent_root"`
	DiscoveryFingerprint string          `json:"discovery_fingerprint"`
	ChildRoots           []string        `json:"child_roots"`
	Children             []ChildSnapshot `json:"children"`
}

type ChildSnapshot struct {
	Root        string   `json:"root"`
	Generation  string   `json:"generation"`
	GraphDigest string   `json:"graph_digest"`
	Manifest    Manifest `json:"manifest"`
}

type ManifestRequest struct {
	Languages []string `json:"languages,omitempty"`
	Drivers   []string `json:"drivers,omitempty"`
	Patchers  []string `json:"patchers,omitempty"`
}

// ManifestFile describes one detected source file included in the build.
type ManifestFile struct {
	Path       string    `json:"path"`
	SHA256     string    `json:"sha256"`
	Size       int64     `json:"size,omitempty"`
	ModTimeUTC time.Time `json:"mod_time_utc,omitempty"`
	Language   string    `json:"language,omitempty"`
	Status     string    `json:"status,omitempty"`
}
