package main

import (
	"github.com/Syfra3/vela/internal/generation"
	"github.com/Syfra3/vela/pkg/types"
	"testing"
)

// Bind fixture provenance to the same closed inputs verified by runtime freshness.
func bindFreshFixtureManifest(t *testing.T, m *types.Manifest) {
	t.Helper()
	c, err := generation.ClosedInputs(m.RepoRoot)
	if err != nil {
		t.Fatal(err)
	}
	m.SourceBinding = generation.BoundGoProfile
	m.ConsumedFiles = c.Files
	m.DirectoryFingerprint = c.DirectoryFingerprint
	m.DependencyFingerprint = c.DependencyFingerprint
	m.ClosureConfigFingerprint = c.ConfigFingerprint
}
