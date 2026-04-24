package types

import "time"

// Manifest tracks file-level freshness for persisted graph outputs.
type Manifest struct {
	Version              int            `json:"version"`
	RepoRoot             string         `json:"repo_root,omitempty"`
	GeneratedAt          time.Time      `json:"generated_at"`
	ExtractorFingerprint string         `json:"extractor_fingerprint"`
	BuildMode            string         `json:"build_mode,omitempty"`
	Files                []ManifestFile `json:"files,omitempty"`
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
