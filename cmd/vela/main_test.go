package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Syfra3/vela/internal/export"
	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/internal/hooks"
	"github.com/Syfra3/vela/internal/query"
	"github.com/Syfra3/vela/pkg/types"
	mcppkg "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func TestRootCommandExposesReducedBuildAndQuerySurface(t *testing.T) {
	root := rootCmd()
	commands := map[string]bool{}
	for _, cmd := range root.Commands() {
		commands[cmd.Name()] = true
	}

	for _, want := range []string{"build", "update", "watch", "hooks", "extract", "status", "lookup", "search", "query", "serve", "tui", "version"} {
		if !commands[want] {
			t.Fatalf("expected command %q to be registered", want)
		}
	}
	for _, blocked := range []string{"hook", "doctor", "config"} {
		if commands[blocked] {
			t.Fatalf("did not expect legacy command %q to remain active", blocked)
		}
	}

	queryCommand, _, err := root.Find([]string{"query", "dependencies"})
	if err != nil {
		t.Fatalf("Find(query dependencies) error = %v", err)
	}
	if queryCommand == nil || queryCommand.Name() != "dependencies" {
		t.Fatalf("expected dependencies subcommand, got %#v", queryCommand)
	}
}

// REQ-001 → SCN-001 → TestSCN001_LanguageCompatibilityReportsScannerEvidence
func TestSCN001_LanguageCompatibilityReportsScannerEvidence(t *testing.T) {
	// Scenario: Language support is reported as scanner-level evidence
	stdout := runCompatibilityCommand(t)

	if !strings.Contains(stdout, "capability=scanner") {
		t.Fatalf("expected scanner capability in compatibility output, got %q", stdout)
	}
	if strings.Contains(strings.ToLower(stdout), "semantically supported") {
		t.Fatalf("compatibility output describes scanner evidence as semantically supported: %q", stdout)
	}
}

// REQ-001 → SCN-002 → TestSCN002_LanguageCompatibilityDistinguishesSemanticAndPatched
func TestSCN002_LanguageCompatibilityDistinguishesSemanticAndPatched(t *testing.T) {
	// Scenario: Semantic and patched language capabilities are distinguishable
	stdout := runCompatibilityCommand(t)

	for _, want := range []string{"go capability=semantic", "typescript capability=patched"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected compatibility output to contain %q, got %q", want, stdout)
		}
	}
}

func runCompatibilityCommand(t *testing.T) string {
	t.Helper()

	stdout := captureStdout(t, func() {
		root := rootCmd()
		root.SetErr(&bytes.Buffer{})
		root.SetArgs([]string{"compatibility"})
		if err := root.Execute(); err != nil {
			t.Fatalf("Execute(compatibility) error = %v", err)
		}
	})
	return stdout
}

// REQ-007/REQ-009 → SCN-011 → TestSCN011_CompatibilityCLIInstallInitializesProjectGraphAndOpenCode
func TestSCN011_CompatibilityCLIInstallInitializesProjectGraphAndOpenCode(t *testing.T) {
	// Scenario: CLI install initializes project graph and selected agent integration
	projectDir := t.TempDir()
	opencodeDir := filepath.Join(t.TempDir(), "opencode")

	var stdout bytes.Buffer
	root := rootCmd()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"install", "--project", projectDir, "--agent", "opencode", "--opencode-dir", opencodeDir})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(install) error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(projectDir, ".vela", "graph.db")); err != nil {
		t.Fatalf("expected project graph.db initialized: %v", err)
	}
	if _, err := os.Stat(filepath.Join(opencodeDir, "opencode.json")); err != nil {
		t.Fatalf("expected OpenCode config with MCP integration installed: %v", err)
	}
	for _, want := range []string{"initialized project graph", "installed OpenCode integration"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected install output to contain %q, got %q", want, stdout.String())
		}
	}
}

// REQ-007/REQ-012 → SCN-012 → TestSCN012_CompatibilityCLIUninstallRemovesClaudeIntegrationPreservesIndex
func TestSCN012_CompatibilityCLIUninstallRemovesClaudeIntegrationPreservesIndex(t *testing.T) {
	// Scenario: CLI uninstall does not delete indexes by default
	projectDir := t.TempDir()
	graphDB := filepath.Join(projectDir, ".vela", "graph.db")
	claudeDir := filepath.Join(t.TempDir(), "claude")
	if err := os.MkdirAll(filepath.Dir(graphDB), 0o755); err != nil {
		t.Fatalf("MkdirAll(.vela) error = %v", err)
	}
	if err := os.WriteFile(graphDB, []byte("SQLite format 3\x00"), 0o644); err != nil {
		t.Fatalf("WriteFile(graph.db) error = %v", err)
	}
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(claudeDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "vela-mcp.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(claude integration) error = %v", err)
	}

	var stdout bytes.Buffer
	root := rootCmd()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"uninstall", "--project", projectDir, "--agent", "claude", "--claude-dir", claudeDir})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(uninstall) error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(claudeDir, "vela-mcp.json")); !os.IsNotExist(err) {
		t.Fatalf("expected Claude Code integration removed, stat err = %v", err)
	}
	if _, err := os.Stat(graphDB); err != nil {
		t.Fatalf("expected graph.db index preserved: %v", err)
	}
	for _, want := range []string{"removed Claude Code integration", "index preserved"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected uninstall output to contain %q, got %q", want, stdout.String())
		}
	}
}

// REQ-008/REQ-011 → SCN-015 → TestSCN015_InstallerOffersOnlySupportedAgentTargets
func TestSCN015_InstallerOffersOnlySupportedAgentTargets(t *testing.T) {
	// Scenario: Installer offers only OpenCode and Claude Code as agent targets
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	for _, configPath := range []string{
		filepath.Join(homeDir, ".config", "opencode", "opencode.json"),
		filepath.Join(homeDir, ".claude", "settings.json"),
		filepath.Join(homeDir, ".cursor", "settings.json"),
		filepath.Join(homeDir, ".codex", "config.toml"),
		filepath.Join(homeDir, ".gemini", "settings.json"),
	} {
		if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(configPath), err)
		}
		if err := os.WriteFile(configPath, []byte("{}\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", configPath, err)
		}
	}

	var stdout bytes.Buffer
	root := rootCmd()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"install", "--project", projectDir})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(install) error = %v", err)
	}

	for _, want := range []string{"agent target available: OpenCode", "agent target available: Claude Code"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected install output to contain %q, got %q", want, stdout.String())
		}
	}
	for _, unwanted := range []string{"Cursor", "Codex", "Gemini"} {
		if strings.Contains(stdout.String(), unwanted) {
			t.Fatalf("expected install output not to offer %s, got %q", unwanted, stdout.String())
		}
	}
}

// REQ-008/REQ-009 → SCN-016 → TestSCN016_InstallerInitializesProjectWhenNoSupportedAgentDetected
func TestSCN016_InstallerInitializesProjectWhenNoSupportedAgentDetected(t *testing.T) {
	// Scenario: Installer allows project initialization when no supported agent is detected
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	var stdout bytes.Buffer
	root := rootCmd()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"install", "--project", projectDir})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(install) error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(projectDir, ".vela", "graph.db")); err != nil {
		t.Fatalf("expected project graph.db initialized without supported agents: %v", err)
	}
	for _, want := range []string{"no supported coding-agent target was detected", "initialized project graph"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected install output to contain %q, got %q", want, stdout.String())
		}
	}
}

// REQ-010 → SCN-017 → TestSCN017_OpenCodeInstallWritesMCPAndInstructionSnippet
func TestSCN017_OpenCodeInstallWritesMCPAndInstructionSnippet(t *testing.T) {
	// Scenario: Agent install writes MCP configuration and instruction snippet
	projectDir := t.TempDir()
	opencodeDir := filepath.Join(t.TempDir(), "opencode")
	unrelatedConfigPath := filepath.Join(opencodeDir, "opencode.json")
	if err := os.MkdirAll(opencodeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(opencodeDir) error = %v", err)
	}
	if err := os.WriteFile(unrelatedConfigPath, []byte(`{"theme":"dark"}`+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(unrelated opencode config) error = %v", err)
	}

	var stdout bytes.Buffer
	root := rootCmd()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"install", "--project", projectDir, "--agent", "opencode", "--opencode-dir", opencodeDir})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(install) error = %v", err)
	}

	mcpConfig, err := os.ReadFile(filepath.Join(opencodeDir, "opencode.json"))
	if err != nil {
		t.Fatalf("ReadFile(OpenCode config) error = %v", err)
	}
	var opencodeConfig map[string]any
	if err := json.Unmarshal(mcpConfig, &opencodeConfig); err != nil {
		t.Fatalf("OpenCode config is not valid JSON: %v\n%s", err, mcpConfig)
	}
	mcp := opencodeConfig["mcp"].(map[string]any)
	vela := mcp["vela"].(map[string]any)
	if vela["type"] != "local" || vela["enabled"] != true {
		t.Fatalf("expected OpenCode MCP config to register enabled local vela, got %#v", vela)
	}
	instructions, err := os.ReadFile(filepath.Join(opencodeDir, "instructions.md"))
	if err != nil {
		t.Fatalf("ReadFile(OpenCode instruction snippet) error = %v", err)
	}
	if !strings.Contains(string(instructions), "Vela") || !strings.Contains(string(instructions), "graph") {
		t.Fatalf("expected OpenCode instruction snippet to explain Vela graph usage, got %q", string(instructions))
	}
	unrelatedConfig, err := os.ReadFile(unrelatedConfigPath)
	if err != nil {
		t.Fatalf("ReadFile(unrelated opencode config) error = %v", err)
	}
	if !strings.Contains(string(unrelatedConfig), `"theme": "dark"`) {
		t.Fatalf("existing OpenCode config field was not preserved: %q", string(unrelatedConfig))
	}
}

// REQ-010 → SCN-018 → TestSCN018_OpenCodeInstallReportsUnsupportedPermissionWithoutFailingIntegration
func TestSCN018_OpenCodeInstallReportsUnsupportedPermissionWithoutFailingIntegration(t *testing.T) {
	// Scenario: Agent install reports unsupported permission settings without failing the integration
	projectDir := t.TempDir()
	opencodeDir := filepath.Join(t.TempDir(), "opencode")

	var stdout bytes.Buffer
	root := rootCmd()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{
		"install",
		"--project", projectDir,
		"--agent", "opencode",
		"--opencode-dir", opencodeDir,
		"--permission", "allow-network",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(install with unsupported permission) error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(opencodeDir, "opencode.json")); err != nil {
		t.Fatalf("expected OpenCode MCP config installed despite unsupported permission: %v", err)
	}
	if _, err := os.Stat(filepath.Join(opencodeDir, "instructions.md")); err != nil {
		t.Fatalf("expected OpenCode instruction snippet installed despite unsupported permission: %v", err)
	}
	for _, want := range []string{"installed OpenCode integration", "skipped unsupported permission setting: allow-network"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected install output to contain %q, got %q", want, stdout.String())
		}
	}
}

// REQ-010/REQ-012 → SCN-019 → TestSCN019_ClaudeInstallUpdatesManagedEntriesWithoutDuplication
func TestSCN019_ClaudeInstallUpdatesManagedEntriesWithoutDuplication(t *testing.T) {
	// Scenario: Re-running install updates Vela-managed entries without duplication
	projectDir := t.TempDir()
	claudeDir := filepath.Join(t.TempDir(), "claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(claudeDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "vela-mcp.json"), []byte(`{"mcpServers":{"vela":{"command":"old-vela"}}}`+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(existing Claude MCP config) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "vela-instructions.md"), []byte("Old Vela instruction snippet\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(existing Claude instruction snippet) error = %v", err)
	}

	for run := 1; run <= 2; run++ {
		root := rootCmd()
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		root.SetArgs([]string{"install", "--project", projectDir, "--agent", "claude", "--claude-dir", claudeDir})
		if err := root.Execute(); err != nil {
			t.Fatalf("Execute(install Claude run %d) error = %v", run, err)
		}
	}

	mcpConfig, err := os.ReadFile(filepath.Join(claudeDir, "vela-mcp.json"))
	if err != nil {
		t.Fatalf("ReadFile(Claude MCP config) error = %v", err)
	}
	if strings.Contains(string(mcpConfig), "old-vela") {
		t.Fatalf("expected Claude MCP Vela-managed entry updated, got %q", string(mcpConfig))
	}
	if got := strings.Count(string(mcpConfig), "vela"); got != 2 {
		t.Fatalf("Claude MCP config has %d vela occurrences, want one managed server entry without duplication: %q", got, string(mcpConfig))
	}

	instructions, err := os.ReadFile(filepath.Join(claudeDir, "vela-instructions.md"))
	if err != nil {
		t.Fatalf("ReadFile(Claude instruction snippet) error = %v", err)
	}
	if strings.Contains(string(instructions), "Old Vela instruction") {
		t.Fatalf("expected Claude instruction snippet updated, got %q", string(instructions))
	}
	if got := strings.Count(string(instructions), "Vela"); got != 1 {
		t.Fatalf("Claude instruction snippet has %d Vela occurrences, want one managed snippet without duplication: %q", got, string(instructions))
	}
}

// REQ-011/REQ-012 → SCN-020 → TestSCN020_UninstallIgnoresUnsupportedFutureAgents
func TestSCN020_UninstallIgnoresUnsupportedFutureAgents(t *testing.T) {
	// Scenario: Uninstall ignores unsupported future agents
	projectDir := t.TempDir()
	opencodeDir := filepath.Join(t.TempDir(), "opencode")
	futureAgentDir := filepath.Join(t.TempDir(), "future-agent")
	graphDB := filepath.Join(projectDir, ".vela", "graph.db")
	futureAgentConfig := filepath.Join(futureAgentDir, "vela-mcp.json")
	futureAgentInstructions := filepath.Join(futureAgentDir, "vela-instructions.md")

	for _, dir := range []string{filepath.Dir(graphDB), opencodeDir, futureAgentDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", dir, err)
		}
	}
	if err := os.WriteFile(graphDB, []byte("SQLite format 3\x00"), 0o644); err != nil {
		t.Fatalf("WriteFile(graph.db) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(opencodeDir, "mcp.json"), []byte("{\"mcpServers\":{\"vela\":{}}}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(OpenCode MCP config) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(opencodeDir, "instructions.md"), []byte("Use Vela graph queries.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(OpenCode instructions) error = %v", err)
	}
	futureMCP := []byte("future-agent Vela MCP config must remain untouched\n")
	futureInstructions := []byte("future-agent Vela instructions must remain untouched\n")
	if err := os.WriteFile(futureAgentConfig, futureMCP, 0o644); err != nil {
		t.Fatalf("WriteFile(future agent config) error = %v", err)
	}
	if err := os.WriteFile(futureAgentInstructions, futureInstructions, 0o644); err != nil {
		t.Fatalf("WriteFile(future agent instructions) error = %v", err)
	}

	var stdout bytes.Buffer
	root := rootCmd()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"uninstall", "--project", projectDir, "--agent", "opencode", "--opencode-dir", opencodeDir})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(uninstall OpenCode) error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(opencodeDir, "mcp.json")); !os.IsNotExist(err) {
		t.Fatalf("expected OpenCode MCP integration removed, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(opencodeDir, "instructions.md")); !os.IsNotExist(err) {
		t.Fatalf("expected OpenCode instruction snippet removed, stat err = %v", err)
	}
	if got, err := os.ReadFile(futureAgentConfig); err != nil || string(got) != string(futureMCP) {
		t.Fatalf("future agent config = %q, %v; want unchanged %q", string(got), err, string(futureMCP))
	}
	if got, err := os.ReadFile(futureAgentInstructions); err != nil || string(got) != string(futureInstructions) {
		t.Fatalf("future agent instructions = %q, %v; want unchanged %q", string(got), err, string(futureInstructions))
	}
	for _, want := range []string{"removed OpenCode integration", "index preserved"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected uninstall output to contain %q, got %q", want, stdout.String())
		}
	}
}

// REQ-007/REQ-012 → SCN-013 → TestSCN013_CompatibilityCLIPurgeRequiresDestructiveApproval
func TestSCN013_CompatibilityCLIPurgeRequiresDestructiveApproval(t *testing.T) {
	// Scenario: CLI purge requires explicit destructive approval
	projectDir := t.TempDir()
	graphDB := filepath.Join(projectDir, ".vela", "graph.db")
	if err := os.MkdirAll(filepath.Dir(graphDB), 0o755); err != nil {
		t.Fatalf("MkdirAll(.vela) error = %v", err)
	}
	if err := os.WriteFile(graphDB, []byte("SQLite format 3\x00"), 0o644); err != nil {
		t.Fatalf("WriteFile(graph.db) error = %v", err)
	}

	var stdout bytes.Buffer
	root := rootCmd()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"purge", "--project", projectDir})
	err := root.Execute()

	if err == nil {
		t.Fatal("Execute(purge) error = nil, want destructive confirmation required")
	}
	if _, statErr := os.Stat(graphDB); statErr != nil {
		t.Fatalf("expected graph.db index preserved without confirmation: %v", statErr)
	}
	for _, want := range []string{"destructive confirmation is required", "index preserved"} {
		if !strings.Contains(err.Error()+stdout.String(), want) {
			t.Fatalf("expected purge output/error to contain %q, got error %q stdout %q", want, err.Error(), stdout.String())
		}
	}
}

// REQ-007/REQ-005 → SCN-014 → TestSCN014_CompatibilityCLIForcePurgeAllReportsPartialFailures
func TestSCN014_CompatibilityCLIForcePurgeAllReportsPartialFailures(t *testing.T) {
	// Scenario: CLI force purge all reports partial failures
	home := t.TempDir()
	t.Setenv("HOME", home)
	alpha := filepath.Join(t.TempDir(), "alpha")
	beta := filepath.Join(t.TempDir(), "beta")
	alphaGraphDB := filepath.Join(alpha, ".vela", "graph.db")
	betaGraphDB := filepath.Join(beta, ".vela", "graph.db")
	writeGraphDBForPurgeTest(t, alphaGraphDB)
	writeGraphDBForPurgeTest(t, betaGraphDB)
	if err := os.Chmod(filepath.Dir(betaGraphDB), 0o500); err != nil {
		t.Fatalf("Chmod(beta .vela) error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(betaGraphDB), 0o700) })

	registryPath := filepath.Join(home, ".vela", "registry.json")
	if err := os.MkdirAll(filepath.Dir(registryPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(registry) error = %v", err)
	}
	registryJSON := fmt.Sprintf(`{"version":1,"entries":[{"repo_root":%q,"name":"alpha","graph_path":%q},{"repo_root":%q,"name":"beta","graph_path":%q}]}`+"\n", alpha, alphaGraphDB, beta, betaGraphDB)
	if err := os.WriteFile(registryPath, []byte(registryJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(registry) error = %v", err)
	}

	var stdout bytes.Buffer
	root := rootCmd()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"purge", "--all", "--force"})
	err := root.Execute()

	if err == nil {
		t.Fatal("Execute(purge --all --force) error = nil, want partial failure")
	}
	if _, statErr := os.Stat(alphaGraphDB); !os.IsNotExist(statErr) {
		t.Fatalf("alpha graph.db stat err = %v, want deleted", statErr)
	}
	if _, statErr := os.Stat(betaGraphDB); statErr != nil {
		t.Fatalf("beta graph.db stat err = %v, want preserved after failed delete", statErr)
	}
	combined := err.Error() + stdout.String()
	for _, want := range []string{"deleted index for alpha", "failed to delete index for beta", "partial failure"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("expected purge output/error to contain %q, got error %q stdout %q", want, err.Error(), stdout.String())
		}
	}
}

func writeGraphDBForPurgeTest(t *testing.T, graphDB string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(graphDB), 0o755); err != nil {
		t.Fatalf("MkdirAll(.vela) error = %v", err)
	}
	if err := os.WriteFile(graphDB, []byte("SQLite format 3\x00"), 0o644); err != nil {
		t.Fatalf("WriteFile(graph.db) error = %v", err)
	}
}

// REQ-008 → SCN-011 → TestSCN011_CLIExposesRequiredV04CommandSurface
func TestSCN011_CLIExposesRequiredV04CommandSurface(t *testing.T) {
	// Scenario: CLI provides required v0.4 command surface
	root := rootCmd()
	commands := map[string]bool{}
	for _, cmd := range root.Commands() {
		commands[cmd.Name()] = true
	}

	for _, want := range []string{"explore", "lookup", "status", "build", "update", "watch", "serve", "explain", "impact", "path"} {
		if !commands[want] {
			t.Fatalf("expected v0.4 command %q to be registered", want)
		}
	}

	serve := commands["serve"]
	if !serve {
		t.Fatal("expected serve command to be registered")
	}
	serveCmd, _, err := root.Find([]string{"serve", "--mcp"})
	if err != nil {
		t.Fatalf("Find(serve --mcp) error = %v", err)
	}
	if serveCmd == nil || serveCmd.Name() != "serve" {
		t.Fatalf("expected serve --mcp to resolve to serve command, got %#v", serveCmd)
	}
}

// REQ-013 → SCN-018 → TestSCN018_StatusCommandReportsPendingStaleFiles
func TestSCN018_StatusCommandReportsPendingStaleFiles(t *testing.T) {
	// Scenario: Status reports pending stale files after source changes
	repoRoot := t.TempDir()
	outDir := filepath.Join(repoRoot, ".vela")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(.vela) error = %v", err)
	}
	sourcePath := filepath.Join(repoRoot, "main.go")
	if err := os.WriteFile(sourcePath, []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.go) error = %v", err)
	}
	graphPath := filepath.Join(outDir, "graph.json")
	if err := os.WriteFile(graphPath, []byte(`{"nodes":[],"edges":[],"meta":{"generatedAt":"2026-04-23T22:47:00Z"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile(graph.json) error = %v", err)
	}
	manifestJSON := `{
	  "version": 1,
	  "repo_root": ` + strconv.Quote(repoRoot) + `,
	  "generated_at": "2026-04-23T22:47:00Z",
	  "build_mode": "full_rebuild",
	  "files": [
	    {"path":"main.go", "sha256":` + strconv.Quote(testFileSHA256(t, sourcePath)) + `, "status":"active"}
	  ]
	}`
	if err := os.WriteFile(filepath.Join(outDir, "manifest.json"), []byte(manifestJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(manifest.json) error = %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("package main\nfunc main() { println(\"changed\") }\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.go changed) error = %v", err)
	}

	stdout := captureStdout(t, func() {
		root := rootCmd()
		root.SetErr(&bytes.Buffer{})
		root.SetArgs([]string{"status", "--graph", graphPath, "--baseline", ""})
		if err := root.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})
	for _, want := range []string{"freshness: stale", "stale files: main.go", "recommended: vela update, vela build"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected status output to contain %q, got %q", want, stdout)
		}
	}
}

// REQ-013 → SCN-019 → TestSCN019_UpdateFailurePreservesPreviousStaleGraphState
func TestSCN019_UpdateFailurePreservesPreviousStaleGraphState(t *testing.T) {
	// Scenario: Update safely refreshes stale graph state
	restore := runBuildService
	t.Cleanup(func() { runBuildService = restore })

	repoRoot := t.TempDir()
	outDir := filepath.Join(repoRoot, ".vela")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(.vela) error = %v", err)
	}
	sourcePath := filepath.Join(repoRoot, "main.go")
	if err := os.WriteFile(sourcePath, []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.go) error = %v", err)
	}
	graphPath := filepath.Join(outDir, "graph.json")
	previousGraph := []byte(`{"nodes":[],"edges":[],"meta":{"state":"previous-valid"}}`)
	if err := os.WriteFile(graphPath, previousGraph, 0o644); err != nil {
		t.Fatalf("WriteFile(graph.json) error = %v", err)
	}
	manifestPath := filepath.Join(outDir, "manifest.json")
	previousManifest := []byte(`{
	  "version": 1,
	  "repo_root": ` + strconv.Quote(repoRoot) + `,
	  "generated_at": "2026-04-23T22:47:00Z",
	  "build_mode": "full_rebuild",
	  "files": [
	    {"path":"main.go", "sha256":` + strconv.Quote(testFileSHA256(t, sourcePath)) + `, "status":"active"}
	  ]
	}`)
	if err := os.WriteFile(manifestPath, previousManifest, 0o644); err != nil {
		t.Fatalf("WriteFile(manifest.json) error = %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("package main\nfunc main() { println(\"changed\") }\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.go changed) error = %v", err)
	}

	runBuildService = func(_ context.Context, outDir string, req types.BuildRequest) (buildOutput, error) {
		if err := os.WriteFile(filepath.Join(outDir, "graph.json"), []byte(`{"corrupt":true}`), 0o644); err != nil {
			t.Fatalf("WriteFile(corrupt graph) error = %v", err)
		}
		freshManifest := []byte(`{
		  "version": 1,
		  "repo_root": ` + strconv.Quote(req.RepoRoot) + `,
		  "generated_at": "2026-04-23T22:48:00Z",
		  "build_mode": "full_rebuild",
		  "files": [
		    {"path":"main.go", "sha256":` + strconv.Quote(testFileSHA256(t, sourcePath)) + `, "status":"active"}
		  ]
		}`)
		if err := os.WriteFile(filepath.Join(outDir, "manifest.json"), freshManifest, 0o644); err != nil {
			t.Fatalf("WriteFile(fresh manifest) error = %v", err)
		}
		return buildOutput{}, fmt.Errorf("simulated interrupted update")
	}

	root := rootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"update", repoRoot, "--out-dir", outDir})
	if err := root.Execute(); err == nil {
		t.Fatal("Execute(update) error = nil, want interrupted update error")
	}

	graphBytes, err := os.ReadFile(graphPath)
	if err != nil {
		t.Fatalf("ReadFile(graph.json) error = %v", err)
	}
	if string(graphBytes) != string(previousGraph) {
		t.Fatalf("graph.json = %s, want previous valid graph", graphBytes)
	}
	snapshot, err := igraph.LoadStatusSnapshot(graphPath, 5)
	if err != nil {
		t.Fatalf("LoadStatusSnapshot() error = %v", err)
	}
	if snapshot.Freshness.Status != "stale" {
		t.Fatalf("freshness status after failed update = %q, want stale", snapshot.Freshness.Status)
	}
}

// REQ-014/REQ-006 → SCN-023 → TestSCN023_MCPFixtureServesAndCallsRequiredTools
func TestSCN023_MCPFixtureServesAndCallsRequiredTools(t *testing.T) {
	// Scenario: MCP fixture proves required tools can be served and called.
	restore := serveMCPStdio
	t.Cleanup(func() { serveMCPStdio = restore })

	graphPath := writeMCPFixtureGraph(t)
	var served *mcpserver.MCPServer
	serveMCPStdio = func(srv *mcpserver.MCPServer) error {
		served = srv
		return nil
	}

	root := rootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"serve", "--mcp", "--graph", graphPath})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(serve --mcp) error = %v", err)
	}
	if served == nil {
		t.Fatal("serve --mcp did not start an MCP server")
	}

	toolCalls := map[string]map[string]any{
		"explore": {"query": "AuthService", "limit": 5},
		"lookup":  {"term": "AuthService", "limit": 5},
		"explain": {"subject": "AuthService", "limit": 5},
		"impact":  {"subject": "Database", "limit": 5},
		"path":    {"subject": "AuthService", "target": "Database", "limit": 5},
		"status":  {},
	}
	for toolName, args := range toolCalls {
		tool := served.GetTool(toolName)
		if tool == nil {
			t.Fatalf("required MCP tool %q was not listed by served fixture", toolName)
		}
		res, err := tool.Handler(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: args}})
		if err != nil {
			t.Fatalf("%s handler error = %v", toolName, err)
		}
		core, ok := res.StructuredContent.(query.Result)
		if !ok {
			t.Fatalf("%s structured content = %T, want query.Result", toolName, res.StructuredContent)
		}
		if core.Status != query.ResultStatusOK && len(core.Diagnostics) == 0 {
			t.Fatalf("%s returned %q without structured diagnostic: %+v", toolName, core.Status, core)
		}
	}
}

// REQ-012 → SCN-014 → TestSCN014_SpecializedCLIMCPToolsRemainAvailableWithExploreAsDefaultSurface
func TestSCN014_SpecializedCLIMCPToolsRemainAvailableWithExploreAsDefaultSurface(t *testing.T) {
	// Scenario: Existing specialized CLI and MCP tools remain available.
	restore := serveMCPStdio
	t.Cleanup(func() { serveMCPStdio = restore })

	graphPath := writeMCPFixtureGraph(t)
	var served *mcpserver.MCPServer
	serveMCPStdio = func(srv *mcpserver.MCPServer) error {
		served = srv
		return nil
	}

	root := rootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"serve", "--mcp", "--graph", graphPath})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(serve --mcp) error = %v", err)
	}
	if served == nil {
		t.Fatal("serve --mcp did not start an MCP server")
	}

	for _, args := range [][]string{
		{"lookup"},
		{"search"},
		{"explain"},
		{"impact"},
		{"path"},
		{"build"},
		{"update"},
		{"status"},
	} {
		cmd, _, err := root.Find(args)
		if err != nil {
			t.Fatalf("Find(%v) error = %v", args, err)
		}
		if cmd == nil || cmd.Name() != args[0] {
			t.Fatalf("expected CLI command %q to remain available, got %#v", args[0], cmd)
		}
	}

	for _, toolName := range []string{"lookup", "explain", "impact", "path", "status"} {
		if tool := served.GetTool(toolName); tool == nil {
			t.Fatalf("expected specialized MCP tool %q to remain available", toolName)
		}
	}

	explore, _, err := root.Find([]string{"explore"})
	if err != nil {
		t.Fatalf("Find(explore) error = %v", err)
	}
	if explore == nil {
		t.Fatal("expected explore command to remain available")
	}
	if !strings.Contains(strings.ToLower(explore.Short), "default agent surface") {
		t.Fatalf("explore command should be presented as the default agent surface rather than the only graph capability; short help = %q", explore.Short)
	}
}

// REQ-014/REQ-004 → SCN-024 → TestSCN024_CLIMCPEquivalenceFixtureUsesSharedCoreSchema
func TestSCN024_CLIMCPEquivalenceFixtureUsesSharedCoreSchema(t *testing.T) {
	// Scenario: CLI and MCP equivalence fixture proves shared schema behavior.
	restore := serveMCPStdio
	t.Cleanup(func() { serveMCPStdio = restore })

	graphPath := writeMCPFixtureGraph(t)
	engine, err := query.LoadFromFile(graphPath)
	if err != nil {
		t.Fatalf("LoadFromFile(%s) error = %v", graphPath, err)
	}
	expected := engine.ExplainResult("AuthService")

	cliOut := &bytes.Buffer{}
	root := rootCmd()
	root.SetOut(cliOut)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"explain", "AuthService", "--graph", graphPath, "--format", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(CLI explain fixture) error = %v", err)
	}
	var cliCore query.Result
	if err := json.Unmarshal(cliOut.Bytes(), &cliCore); err != nil {
		t.Fatalf("CLI explain fixture output was not shared core JSON: %v\n%s", err, cliOut.String())
	}

	var served *mcpserver.MCPServer
	serveMCPStdio = func(srv *mcpserver.MCPServer) error {
		served = srv
		return nil
	}
	serve := rootCmd()
	serve.SetOut(&bytes.Buffer{})
	serve.SetErr(&bytes.Buffer{})
	serve.SetArgs([]string{"serve", "--mcp", "--graph", graphPath})
	if err := serve.Execute(); err != nil {
		t.Fatalf("Execute(serve --mcp fixture) error = %v", err)
	}
	if served == nil {
		t.Fatal("serve --mcp did not start an MCP server for equivalence fixture")
	}
	res, err := served.GetTool("explain").Handler(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: map[string]any{"subject": "AuthService", "limit": 5}}})
	if err != nil {
		t.Fatalf("MCP explain handler error = %v", err)
	}
	mcpCore, ok := res.StructuredContent.(query.Result)
	if !ok {
		t.Fatalf("MCP explain structured content = %T, want query.Result", res.StructuredContent)
	}

	assertEquivalentCoreResult(t, "CLI", cliCore, expected)
	assertEquivalentCoreResult(t, "MCP", mcpCore, expected)
	if len(cliCore.Facts) != len(mcpCore.Facts) || cliCore.Facts[0].Subject != mcpCore.Facts[0].Subject || cliCore.Facts[0].Predicate != mcpCore.Facts[0].Predicate || cliCore.Facts[0].Object != mcpCore.Facts[0].Object {
		t.Fatalf("CLI/MCP core facts diverged: CLI=%+v MCP=%+v", cliCore.Facts, mcpCore.Facts)
	}
}

// REQ-015/REQ-011 → SCN-016 → TestSCN016_CLIBoundaryLabelsLegacyAndIREvidence
func TestSCN016_CLIBoundaryLabelsLegacyAndIREvidence(t *testing.T) {
	// Scenario: Prior runtime and low-level graph behavior coexists with the new IR.
	graphPath := writeSCN016MixedRuntimeGraphForCLI(t)
	out := &bytes.Buffer{}
	root := rootCmd()
	root.SetOut(out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"query", "dependencies", "CheckoutService", "--graph", graphPath, "--limit", "5"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(query dependencies) error = %v", err)
	}

	stdout := out.String()
	for _, want := range []string{"LegacyGateway", "legacy-backed", "IRRepository", "IR-backed", "kind=DEPENDS_ON", "origin=deterministic"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected CLI output to contain %q, got:\n%s", want, stdout)
		}
	}
	for _, forbidden := range []string{"full replacement", "fully replaced", "completed full replacement", "Phase 1 replaced prior runtime"} {
		if strings.Contains(strings.ToLower(stdout), strings.ToLower(forbidden)) {
			t.Fatalf("CLI output must not claim Phase 1 fully replaced prior runtime behavior via %q, got:\n%s", forbidden, stdout)
		}
	}
}

func writeSCN016MixedRuntimeGraphForCLI(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	velaDir := filepath.Join(dir, ".vela")
	if err := os.Mkdir(velaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	graphJSON := filepath.Join(velaDir, "graph.json")
	if err := os.WriteFile(graphJSON, []byte(`{"nodes":[],"edges":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	graph := &types.Graph{
		Nodes: []types.Node{
			{ID: "checkout-service", Label: "CheckoutService", NodeType: "service", SourceFile: "checkout.go"},
			{ID: "legacy-gateway", Label: "LegacyGateway", NodeType: "client", SourceFile: "legacy_gateway.go"},
			{ID: "ir-repository", Label: "IRRepository", NodeType: "repository", SourceFile: "ir_repository.go"},
		},
		Edges: []types.Edge{
			{Source: "checkout-service", Target: "legacy-gateway", Relation: string(types.FactKindDependsOn), Metadata: map[string]interface{}{"evidence_type": "legacy-runtime", "evidence_source_artifact": "legacy_runtime.go", "evidence_confidence": "legacy"}},
			{Source: "checkout-service", Target: "ir-repository", Relation: string(types.FactKindDependsOn), Metadata: map[string]interface{}{"common_ir": true, "ir_kind": "DEPENDS_ON", "ir_origin": "deterministic", "freshness": "fresh", "evidence_type": "common-ir", "evidence_source_artifact": "ir_runtime.go", "evidence_confidence": "high"}},
		},
	}
	if err := export.WriteSQLiteGraphAtomic(graph, velaDir); err != nil {
		t.Fatalf("WriteSQLiteGraphAtomic error: %v", err)
	}
	return graphJSON
}

// REQ-014 → SCN-025 → TestSCN025_RealWorkspaceSmokeReportIsRedactedReleaseProof
func TestSCN025_RealWorkspaceSmokeReportIsRedactedReleaseProof(t *testing.T) {
	// Scenario: Real workspace smoke test proves release behavior outside toy fixtures.
	reportPath := filepath.Join("..", "..", "reports", "SCN-025-real-workspace-smoke.md")
	reportBytes, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", reportPath, err)
	}
	report := string(reportBytes)

	for _, want := range []string{
		"SCN-025 Real Workspace Smoke Report",
		"workspace: <REAL_WORKSPACE>",
		"redaction_policy: no secrets",
		"vela build <REAL_WORKSPACE>",
		"graph_db: present",
		"vela status --graph <REAL_WORKSPACE>/.vela/graph.json",
		"freshness:",
		"vela lookup",
		"vela explain",
		"MCP tool call: explain",
		"evidence-bearing: yes",
		"secret scan: pass",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("expected smoke report to contain %q, got:\n%s", want, report)
		}
	}
	for _, forbidden := range []string{"/home/geen/Documents/personal/stock-chef", "AIza", "sk-", "ghp_", "BEGIN PRIVATE KEY", "password="} {
		if strings.Contains(report, forbidden) {
			t.Fatalf("smoke report leaked forbidden content %q", forbidden)
		}
	}
}

// REQ-014 → SCN-025 → TestSCN025_RealWorkspaceSmokeHarness
func TestSCN025_RealWorkspaceSmokeHarness(t *testing.T) {
	// Scenario: Real workspace smoke test proves release behavior outside toy fixtures.
	workspace := os.Getenv("VELA_SCN025_WORKSPACE")
	if workspace == "" {
		t.Skip("set VELA_SCN025_WORKSPACE to run the external SCN-025 real workspace smoke")
	}
	restore := serveMCPStdio
	t.Cleanup(func() { serveMCPStdio = restore })

	buildOut := &bytes.Buffer{}
	build := rootCmd()
	build.SetOut(buildOut)
	build.SetErr(&bytes.Buffer{})
	build.SetArgs([]string{"build", workspace})
	if err := build.Execute(); err != nil {
		t.Fatalf("Execute(build real workspace) error = %v", err)
	}
	outDir := filepath.Join(workspace, ".vela")
	graphJSON := filepath.Join(outDir, "graph.json")
	if _, err := os.Stat(filepath.Join(outDir, "graph.db")); err != nil {
		t.Fatalf("graph.db after real workspace build: %v", err)
	}

	var statusErr error
	statusOut := captureStdout(t, func() {
		status := rootCmd()
		status.SetErr(&bytes.Buffer{})
		status.SetArgs([]string{"status", "--graph", graphJSON, "--baseline", ""})
		statusErr = status.Execute()
	})
	if statusErr != nil {
		t.Fatalf("Execute(status real workspace) error = %v", statusErr)
	}
	if !strings.Contains(statusOut, "freshness:") {
		t.Fatalf("status output missing freshness: %q", statusOut)
	}

	subject := "ExecuteAugusteToolUseCase"
	cliOut := &bytes.Buffer{}
	cliExplain := rootCmd()
	cliExplain.SetOut(cliOut)
	cliExplain.SetErr(&bytes.Buffer{})
	cliExplain.SetArgs([]string{"explain", subject, "--graph", graphJSON, "--format", "json"})
	if err := cliExplain.Execute(); err != nil {
		t.Fatalf("Execute(explain real workspace) error = %v", err)
	}
	var cliCore query.Result
	if err := json.Unmarshal(cliOut.Bytes(), &cliCore); err != nil {
		t.Fatalf("real workspace CLI explain was not core JSON: %v", err)
	}
	if cliCore.Status != query.ResultStatusOK || len(cliCore.Facts) == 0 || len(cliCore.Evidence) == 0 {
		t.Fatalf("real workspace CLI explain lacks evidence-bearing graph answer: %+v", cliCore)
	}

	var served *mcpserver.MCPServer
	serveMCPStdio = func(srv *mcpserver.MCPServer) error {
		served = srv
		return nil
	}
	serve := rootCmd()
	serve.SetOut(&bytes.Buffer{})
	serve.SetErr(&bytes.Buffer{})
	serve.SetArgs([]string{"serve", "--mcp", "--graph", graphJSON})
	if err := serve.Execute(); err != nil {
		t.Fatalf("Execute(serve --mcp real workspace) error = %v", err)
	}
	if served == nil {
		t.Fatal("serve --mcp did not start an MCP server for real workspace smoke")
	}
	res, err := served.GetTool("explain").Handler(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: map[string]any{"subject": subject, "limit": 5}}})
	if err != nil {
		t.Fatalf("MCP explain real workspace handler error = %v", err)
	}
	mcpCore, ok := res.StructuredContent.(query.Result)
	if !ok {
		t.Fatalf("MCP explain structured content = %T, want query.Result", res.StructuredContent)
	}
	if mcpCore.Status != query.ResultStatusOK || len(mcpCore.Facts) == 0 || len(mcpCore.Evidence) == 0 {
		t.Fatalf("real workspace MCP explain lacks evidence-bearing graph answer: %+v", mcpCore)
	}
}

// REQ-001/REQ-002 → SCN-001 → TestSCN001_MCPServeFromFreshWorkspaceReportsFreshSelectedGraph
func TestSCN001_MCPServeFromFreshWorkspaceReportsFreshSelectedGraph(t *testing.T) {
	// Scenario: MCP reports the same fresh active stock-chef graph as CLI status.
	restore := serveMCPStdio
	t.Cleanup(func() { serveMCPStdio = restore })

	home := t.TempDir()
	t.Setenv("HOME", home)
	writeRuntimeGraphForMCPSelection(t, filepath.Join(home, ".vela"), home, false)

	workspace := filepath.Join(t.TempDir(), "stock-chef")
	graphJSON := writeRuntimeGraphForMCPSelection(t, filepath.Join(workspace, ".vela"), workspace, true)
	t.Chdir(workspace)

	var served *mcpserver.MCPServer
	serveMCPStdio = func(srv *mcpserver.MCPServer) error {
		served = srv
		return nil
	}
	serve := rootCmd()
	serve.SetOut(&bytes.Buffer{})
	serve.SetErr(&bytes.Buffer{})
	serve.SetArgs([]string{"serve", "--mcp"})
	if err := serve.Execute(); err != nil {
		t.Fatalf("Execute(serve --mcp from stock-chef workspace) error = %v", err)
	}
	if served == nil {
		t.Fatal("serve --mcp did not start an MCP server")
	}

	res, err := served.GetTool("explain").Handler(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: map[string]any{"subject": "AuthService", "limit": 5}}})
	if err != nil {
		t.Fatalf("MCP explain handler error = %v", err)
	}
	core, ok := res.StructuredContent.(query.Result)
	if !ok {
		t.Fatalf("MCP explain structured content = %T, want query.Result", res.StructuredContent)
	}
	if core.Freshness.Status != query.FreshnessFresh {
		t.Fatalf("MCP freshness = %q, want fresh; diagnostics=%+v", core.Freshness.Status, core.Diagnostics)
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(callToolResultText(t, res)), &envelope); err != nil {
		t.Fatalf("MCP explain text fallback was not JSON: %v", err)
	}
	freshness, ok := envelope["freshness"].(map[string]any)
	if !ok {
		t.Fatalf("MCP response freshness = %#v, want object", envelope["freshness"])
	}
	if freshness["source"] != graphJSON {
		t.Fatalf("MCP graph source = %#v, want %q", freshness["source"], graphJSON)
	}
	if actions, ok := freshness["recommended_actions"].([]any); ok {
		for _, action := range actions {
			if strings.Contains(fmt.Sprint(action), "vela build") || strings.Contains(fmt.Sprint(action), "vela update") {
				t.Fatalf("MCP response recommended %q for fresh selected graph; freshness=%#v", action, freshness)
			}
		}
	}
}

// REQ-001/REQ-002 → SCN-002 → TestSCN002_MCPGraphQueryIncludesSelectedGraphSourceEvidence
func TestSCN002_MCPGraphQueryIncludesSelectedGraphSourceEvidence(t *testing.T) {
	// Scenario: MCP response includes graph source evidence for debugging selection.
	restore := serveMCPStdio
	t.Cleanup(func() { serveMCPStdio = restore })

	workspace := filepath.Clean("/home/geen/Documents/personal/stock-chef")
	updatedAt := time.Date(2026, 6, 29, 15, 30, 0, 0, time.UTC)
	graphJSON := writeRuntimeGraphWithSourceEvidence(t, filepath.Join(t.TempDir(), ".vela"), workspace, updatedAt)

	var served *mcpserver.MCPServer
	serveMCPStdio = func(srv *mcpserver.MCPServer) error {
		served = srv
		return nil
	}
	serve := rootCmd()
	serve.SetOut(&bytes.Buffer{})
	serve.SetErr(&bytes.Buffer{})
	serve.SetArgs([]string{"serve", "--mcp", "--graph", graphJSON})
	if err := serve.Execute(); err != nil {
		t.Fatalf("Execute(serve --mcp --graph) error = %v", err)
	}
	if served == nil {
		t.Fatal("serve --mcp did not start an MCP server")
	}

	res, err := served.GetTool("explain").Handler(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: map[string]any{"subject": "AuthService", "limit": 5}}})
	if err != nil {
		t.Fatalf("MCP explain handler error = %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(callToolResultText(t, res)), &envelope); err != nil {
		t.Fatalf("MCP explain text fallback was not JSON: %v", err)
	}
	freshness, ok := envelope["freshness"].(map[string]any)
	if !ok {
		t.Fatalf("MCP response freshness = %#v, want object", envelope["freshness"])
	}
	for key, want := range map[string]string{
		"selected_graph_path": graphJSON,
		"project":             "stock-chef",
		"workspace_root":      workspace,
		"graph_updated_at":    updatedAt.Format(time.RFC3339),
	} {
		if freshness[key] != want {
			t.Fatalf("freshness[%q] = %#v, want %q; freshness=%#v", key, freshness[key], want, freshness)
		}
	}
}

// REQ-002 → SCN-003 → TestSCN003_MCPGraphQueryExplainsUnknownFreshnessBuildRecommendation
func TestSCN003_MCPGraphQueryExplainsUnknownFreshnessBuildRecommendation(t *testing.T) {
	// Scenario: Stale or unknown freshness explains why build is recommended.
	restore := serveMCPStdio
	t.Cleanup(func() { serveMCPStdio = restore })

	workspace := filepath.Join(t.TempDir(), "stock-chef")
	graphJSON := writeRuntimeGraphForMCPSelection(t, filepath.Join(workspace, ".vela"), workspace, false)

	var served *mcpserver.MCPServer
	serveMCPStdio = func(srv *mcpserver.MCPServer) error {
		served = srv
		return nil
	}
	serve := rootCmd()
	serve.SetOut(&bytes.Buffer{})
	serve.SetErr(&bytes.Buffer{})
	serve.SetArgs([]string{"serve", "--mcp", "--graph", graphJSON})
	if err := serve.Execute(); err != nil {
		t.Fatalf("Execute(serve --mcp --graph unknown freshness) error = %v", err)
	}
	if served == nil {
		t.Fatal("serve --mcp did not start an MCP server")
	}

	res, err := served.GetTool("explain").Handler(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: map[string]any{"subject": "AuthService", "limit": 5}}})
	if err != nil {
		t.Fatalf("MCP explain handler error = %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(callToolResultText(t, res)), &envelope); err != nil {
		t.Fatalf("MCP explain text fallback was not JSON: %v", err)
	}
	freshness, ok := envelope["freshness"].(map[string]any)
	if !ok {
		t.Fatalf("MCP response freshness = %#v, want object", envelope["freshness"])
	}
	if freshness["status"] != string(query.FreshnessUnknown) {
		t.Fatalf("freshness status = %#v, want unknown; freshness=%#v", freshness["status"], freshness)
	}
	if freshness["selected_graph_path"] != graphJSON {
		t.Fatalf("freshness selected_graph_path = %#v, want %q; freshness=%#v", freshness["selected_graph_path"], graphJSON, freshness)
	}
	reason, ok := freshness["reason"].(string)
	if !ok || !strings.Contains(reason, "manifest") || !strings.Contains(reason, "vela build") {
		t.Fatalf("freshness reason = %#v, want manifest-specific build explanation; freshness=%#v", freshness["reason"], freshness)
	}
	actions, ok := freshness["recommended_actions"].([]any)
	if !ok {
		t.Fatalf("freshness recommended_actions = %#v, want actions including vela build", freshness["recommended_actions"])
	}
	foundBuild := false
	for _, action := range actions {
		if fmt.Sprint(action) == "vela build" {
			foundBuild = true
		}
	}
	if !foundBuild {
		t.Fatalf("freshness recommended_actions = %#v, want vela build", actions)
	}
}

// REQ-004 → SCN-005 → TestSCN005_MCPReverseDependenciesPreferActiveWorkspaceOverDepEvalCorpus
func TestSCN005_MCPReverseDependenciesPreferActiveWorkspaceOverDepEvalCorpus(t *testing.T) {
	// Scenario: Active real stock-chef workspace is preferred over dep-eval stock-chef corpus.
	restore := serveMCPStdio
	t.Cleanup(func() { serveMCPStdio = restore })

	home := t.TempDir()
	t.Setenv("HOME", home)
	writeStockChefSelectionRuntimeGraph(t, filepath.Join(home, ".vela"), filepath.Join(home, "dep-eval", "corpora", "workdirs", "stock-chef"), "dep-eval:corpora/workdirs/stock-chef/DepEvalCaller", "dep_eval.go")

	workspace := filepath.Join(t.TempDir(), "stock-chef")
	graphJSON := writeStockChefSelectionRuntimeGraph(t, filepath.Join(workspace, ".vela"), workspace, "github.com/Syfra3/stock-chef:LocalCaller", "local_stock_chef.go")
	t.Chdir(workspace)

	var served *mcpserver.MCPServer
	serveMCPStdio = func(srv *mcpserver.MCPServer) error {
		served = srv
		return nil
	}
	serve := rootCmd()
	serve.SetOut(&bytes.Buffer{})
	serve.SetErr(&bytes.Buffer{})
	serve.SetArgs([]string{"serve", "--mcp"})
	if err := serve.Execute(); err != nil {
		t.Fatalf("Execute(serve --mcp from active stock-chef workspace) error = %v", err)
	}
	if served == nil {
		t.Fatal("serve --mcp did not start an MCP server")
	}

	res, err := served.GetTool("reverse_dependencies").Handler(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: map[string]any{"subject": "SharedStockChefSymbol", "limit": 5}}})
	if err != nil {
		t.Fatalf("MCP reverse_dependencies handler error = %v", err)
	}
	text := callToolResultText(t, res)
	for _, want := range []string{graphJSON, workspace, "stock-chef", "github.com/Syfra3/stock-chef:LocalCaller"} {
		if !strings.Contains(text, want) {
			t.Fatalf("MCP response missing active workspace evidence %q; response:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{"dep-eval", "corpora/workdirs/stock-chef", "DepEvalCaller"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("MCP response included non-active dep-eval corpus evidence %q; response:\n%s", forbidden, text)
		}
	}
}

// REQ-005 → SCN-006 → TestSCN006_MCPReverseDependenciesReturnsAmbiguityForSameNameStockChefCorpora
func TestSCN006_MCPReverseDependenciesReturnsAmbiguityForSameNameStockChefCorpora(t *testing.T) {
	// Scenario: Ambiguous stock-chef corpora require explicit disambiguation.
	restore := serveMCPStdio
	t.Cleanup(func() { serveMCPStdio = restore })

	home := t.TempDir()
	t.Setenv("HOME", home)
	localRoot := filepath.Join(home, "work", "stock-chef")
	depEvalRoot := filepath.Join(home, "dep-eval", "corpora", "workdirs", "stock-chef")
	localGraph := writeStockChefSelectionRuntimeGraph(t, filepath.Join(localRoot, ".vela"), localRoot, "github.com/Syfra3/stock-chef:LocalCaller", "local_stock_chef.go")
	depEvalGraph := writeStockChefSelectionRuntimeGraph(t, filepath.Join(depEvalRoot, ".vela"), depEvalRoot, "dep-eval:corpora/workdirs/stock-chef/DepEvalCaller", "dep_eval.go")
	registryPath := filepath.Join(home, ".vela", "registry.json")
	if err := os.MkdirAll(filepath.Dir(registryPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(registry) error = %v", err)
	}
	registryJSON := fmt.Sprintf(`{"version":1,"entries":[{"repo_root":%q,"name":"stock-chef","graph_path":%q},{"repo_root":%q,"name":"stock-chef","graph_path":%q}]}`+"\n", localRoot, localGraph, depEvalRoot, depEvalGraph)
	if err := os.WriteFile(registryPath, []byte(registryJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(registry) error = %v", err)
	}
	t.Chdir(t.TempDir())

	var served *mcpserver.MCPServer
	serveMCPStdio = func(srv *mcpserver.MCPServer) error {
		served = srv
		return nil
	}
	serve := rootCmd()
	serve.SetOut(&bytes.Buffer{})
	serve.SetErr(&bytes.Buffer{})
	serve.SetArgs([]string{"serve", "--mcp"})
	if err := serve.Execute(); err != nil {
		t.Fatalf("Execute(serve --mcp with ambiguous stock-chef registry) error = %v", err)
	}
	if served == nil {
		t.Fatal("serve --mcp did not start an MCP server")
	}

	res, err := served.GetTool("reverse_dependencies").Handler(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: map[string]any{"subject": "stock-chef", "limit": 5}}})
	if err != nil {
		t.Fatalf("MCP reverse_dependencies handler error = %v", err)
	}
	text := callToolResultText(t, res)
	for _, want := range []string{"Status: ambiguous", "stock-chef", localGraph, localRoot, depEvalGraph, depEvalRoot, "choose an explicit corpus", "--graph"} {
		if !strings.Contains(text, want) {
			t.Fatalf("MCP ambiguity response missing %q; response:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{"LocalCaller", "DepEvalCaller", "Graph evidence:"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("MCP ambiguity response merged graph answer evidence %q; response:\n%s", forbidden, text)
		}
	}
}

// REQ-005 → SCN-006 → TestSCN006_MCPSymbolOnlyQueryReturnsAmbiguityForSameNameCorpora
func TestSCN006_MCPSymbolOnlyQueryReturnsAmbiguityForSameNameCorpora(t *testing.T) {
	// Scenario: Ambiguous stock-chef corpora require explicit disambiguation.
	restore := serveMCPStdio
	t.Cleanup(func() { serveMCPStdio = restore })

	home := t.TempDir()
	t.Setenv("HOME", home)
	localRoot := filepath.Join(home, "work", "stock-chef")
	depEvalRoot := filepath.Join(home, "dep-eval", "corpora", "workdirs", "stock-chef")
	localGraph := writeStockChefSelectionRuntimeGraph(t, filepath.Join(localRoot, ".vela"), localRoot, "github.com/Syfra3/stock-chef:LocalCaller", "local_stock_chef.go")
	depEvalGraph := writeStockChefSelectionRuntimeGraph(t, filepath.Join(depEvalRoot, ".vela"), depEvalRoot, "dep-eval:corpora/workdirs/stock-chef/DepEvalCaller", "dep_eval.go")
	registryPath := filepath.Join(home, ".vela", "registry.json")
	if err := os.MkdirAll(filepath.Dir(registryPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(registry) error = %v", err)
	}
	registryJSON := fmt.Sprintf(`{"version":1,"entries":[{"repo_root":%q,"name":"stock-chef","graph_path":%q},{"repo_root":%q,"name":"stock-chef","graph_path":%q}]}`+"\n", localRoot, localGraph, depEvalRoot, depEvalGraph)
	if err := os.WriteFile(registryPath, []byte(registryJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(registry) error = %v", err)
	}
	t.Chdir(t.TempDir())

	var served *mcpserver.MCPServer
	serveMCPStdio = func(srv *mcpserver.MCPServer) error {
		served = srv
		return nil
	}
	serve := rootCmd()
	serve.SetOut(&bytes.Buffer{})
	serve.SetErr(&bytes.Buffer{})
	serve.SetArgs([]string{"serve", "--mcp"})
	if err := serve.Execute(); err != nil {
		t.Fatalf("Execute(serve --mcp with ambiguous stock-chef registry) error = %v", err)
	}
	if served == nil {
		t.Fatal("serve --mcp did not start an MCP server")
	}

	res, err := served.GetTool("reverse_dependencies").Handler(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: map[string]any{"subject": "SharedStockChefSymbol", "limit": 5}}})
	if err != nil {
		t.Fatalf("MCP reverse_dependencies handler error = %v", err)
	}
	text := callToolResultText(t, res)
	for _, want := range []string{"Status: ambiguous", "SharedStockChefSymbol", localGraph, localRoot, depEvalGraph, depEvalRoot, "choose an explicit corpus", "--graph"} {
		if !strings.Contains(text, want) {
			t.Fatalf("MCP ambiguity response missing %q; response:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{"LocalCaller", "DepEvalCaller", "Graph evidence:"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("MCP symbol-only ambiguity response executed against one candidate %q; response:\n%s", forbidden, text)
		}
	}
}

// REQ-004/REQ-005 → SCN-007 → TestSCN007_MCPMissingActiveStockChefGraphDoesNotFallbackToDepEvalCorpus
func TestSCN007_MCPMissingActiveStockChefGraphDoesNotFallbackToDepEvalCorpus(t *testing.T) {
	// Scenario: Missing active graph does not silently fall back to dep-eval corpus.
	restore := serveMCPStdio
	t.Cleanup(func() { serveMCPStdio = restore })

	home := t.TempDir()
	t.Setenv("HOME", home)
	depEvalRoot := filepath.Join(home, "dep-eval", "corpora", "workdirs", "stock-chef")
	depEvalGraph := writeStockChefSelectionRuntimeGraph(t, filepath.Join(home, ".vela"), depEvalRoot, "dep-eval:corpora/workdirs/stock-chef/DepEvalCaller", "dep_eval.go")

	workspace := filepath.Join(t.TempDir(), "stock-chef")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "local_stock_chef.go"), []byte("package main\n\ntype SharedStockChefSymbol struct{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(workspace)

	var served *mcpserver.MCPServer
	serveMCPStdio = func(srv *mcpserver.MCPServer) error {
		served = srv
		return nil
	}
	serve := rootCmd()
	serve.SetOut(&bytes.Buffer{})
	serve.SetErr(&bytes.Buffer{})
	serve.SetArgs([]string{"serve", "--mcp"})
	if err := serve.Execute(); err != nil {
		t.Fatalf("Execute(serve --mcp from active stock-chef workspace with missing graph) error = %v", err)
	}
	if served == nil {
		t.Fatal("serve --mcp did not start an MCP server")
	}

	res, err := served.GetTool("reverse_dependencies").Handler(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: map[string]any{"subject": "SharedStockChefSymbol", "limit": 5}}})
	if err != nil {
		t.Fatalf("MCP reverse_dependencies handler error = %v", err)
	}
	text := callToolResultText(t, res)
	for _, want := range []string{"Status: unavailable", "active workspace graph", "missing", workspace, filepath.Join(workspace, ".vela", "graph.json"), depEvalGraph, depEvalRoot, "vela build", "choose an explicit corpus"} {
		if !strings.Contains(text, want) {
			t.Fatalf("MCP missing-active-graph response missing %q; response:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{"DepEvalCaller", "Graph evidence:"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("MCP response silently answered from dep-eval corpus evidence %q; response:\n%s", forbidden, text)
		}
	}
}

// REQ-004/REQ-005 → SCN-007 → TestSCN007_MCPInvalidActiveStockChefGraphDoesNotFailStartupOrFallbackToDepEvalCorpus
func TestSCN007_MCPInvalidActiveStockChefGraphDoesNotFailStartupOrFallbackToDepEvalCorpus(t *testing.T) {
	// Scenario: Missing active graph does not silently fall back to dep-eval corpus.
	restore := serveMCPStdio
	t.Cleanup(func() { serveMCPStdio = restore })

	home := t.TempDir()
	t.Setenv("HOME", home)
	depEvalRoot := filepath.Join(home, "dep-eval", "corpora", "workdirs", "stock-chef")
	depEvalGraph := writeStockChefSelectionRuntimeGraph(t, filepath.Join(depEvalRoot, ".vela"), depEvalRoot, "dep-eval:corpora/workdirs/stock-chef/DepEvalCaller", "dep_eval.go")
	registryPath := filepath.Join(home, ".vela", "registry.json")
	if err := os.MkdirAll(filepath.Dir(registryPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(registry) error = %v", err)
	}
	registryJSON := fmt.Sprintf(`{"version":1,"entries":[{"repo_root":%q,"name":"stock-chef","graph_path":%q}]}`+"\n", depEvalRoot, depEvalGraph)
	if err := os.WriteFile(registryPath, []byte(registryJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(registry) error = %v", err)
	}

	workspace := filepath.Join(t.TempDir(), "stock-chef")
	activeGraph := filepath.Join(workspace, ".vela", "graph.json")
	if err := os.MkdirAll(filepath.Dir(activeGraph), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "local_stock_chef.go"), []byte("package main\n\ntype SharedStockChefSymbol struct{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(activeGraph, []byte(`{"nodes":`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(workspace)

	var served *mcpserver.MCPServer
	serveMCPStdio = func(srv *mcpserver.MCPServer) error {
		served = srv
		return nil
	}
	serve := rootCmd()
	serve.SetOut(&bytes.Buffer{})
	serve.SetErr(&bytes.Buffer{})
	serve.SetArgs([]string{"serve", "--mcp"})
	if err := serve.Execute(); err != nil {
		t.Fatalf("Execute(serve --mcp from active stock-chef workspace with invalid graph) error = %v", err)
	}
	if served == nil {
		t.Fatal("serve --mcp did not start an MCP server")
	}

	res, err := served.GetTool("reverse_dependencies").Handler(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: map[string]any{"subject": "SharedStockChefSymbol", "limit": 5}}})
	if err != nil {
		t.Fatalf("MCP reverse_dependencies handler error = %v", err)
	}
	text := callToolResultText(t, res)
	for _, want := range []string{"Status: unavailable", "active workspace graph", "invalid", workspace, activeGraph, depEvalGraph, depEvalRoot, "vela build", "choose an explicit corpus"} {
		if !strings.Contains(text, want) {
			t.Fatalf("MCP invalid-active-graph response missing %q; response:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{"DepEvalCaller", "Graph evidence:"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("MCP response silently answered from dep-eval corpus evidence %q; response:\n%s", forbidden, text)
		}
	}
}

func writeRuntimeGraphForMCPSelection(t *testing.T, outDir, repoRoot string, withManifest bool) string {
	t.Helper()
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(repoRoot, "auth.go")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	source := []byte("package main\n\ntype AuthService struct{}\ntype Database struct{}\n")
	if err := os.WriteFile(sourcePath, source, 0o644); err != nil {
		t.Fatal(err)
	}
	graphJSON := filepath.Join(outDir, "graph.json")
	if err := os.WriteFile(graphJSON, []byte(`{"nodes":[],"edges":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	graph := &types.Graph{
		Nodes: []types.Node{
			{ID: "auth", Label: "AuthService", NodeType: "struct", SourceFile: "auth.go"},
			{ID: "db", Label: "Database", NodeType: "struct", SourceFile: "auth.go"},
		},
		Edges: []types.Edge{{Source: "auth", Target: "db", Relation: "uses", Metadata: map[string]interface{}{"evidence_confidence": "extracted"}}},
	}
	if err := export.WriteSQLiteGraphAtomic(graph, outDir); err != nil {
		t.Fatalf("WriteSQLiteGraphAtomic error = %v", err)
	}
	if withManifest {
		manifest := types.Manifest{
			Version:     1,
			RepoRoot:    repoRoot,
			GeneratedAt: time.Now().UTC(),
			BuildMode:   "full_rebuild",
			Files:       []types.ManifestFile{{Path: "auth.go", SHA256: hexSHA256(source)}},
		}
		manifestData, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(outDir, "manifest.json"), manifestData, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return graphJSON
}

func writeStockChefSelectionRuntimeGraph(t *testing.T, outDir, repoRoot, callerID, sourceFile string) string {
	t.Helper()
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	source := []byte("package main\n\ntype SharedStockChefSymbol struct{}\ntype LocalCaller struct{}\n")
	if err := os.WriteFile(filepath.Join(repoRoot, sourceFile), source, 0o644); err != nil {
		t.Fatal(err)
	}
	graphJSON := filepath.Join(outDir, "graph.json")
	if err := os.WriteFile(graphJSON, []byte(`{"nodes":[],"edges":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	graph := &types.Graph{
		Nodes: []types.Node{
			{ID: callerID, Label: callerID, NodeType: "struct", SourceFile: sourceFile},
			{ID: "github.com/Syfra3/stock-chef:SharedStockChefSymbol", Label: "SharedStockChefSymbol", NodeType: "struct", SourceFile: sourceFile},
		},
		Edges: []types.Edge{{Source: callerID, Target: "github.com/Syfra3/stock-chef:SharedStockChefSymbol", Relation: "uses", Metadata: map[string]interface{}{"evidence_confidence": "extracted"}}},
	}
	if err := export.WriteSQLiteGraphAtomic(graph, outDir); err != nil {
		t.Fatalf("WriteSQLiteGraphAtomic error = %v", err)
	}
	manifest := types.Manifest{
		Version:     1,
		RepoRoot:    repoRoot,
		GeneratedAt: time.Now().UTC(),
		BuildMode:   "full_rebuild",
		Files:       []types.ManifestFile{{Path: sourceFile, SHA256: hexSHA256(source)}},
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "manifest.json"), manifestData, 0o644); err != nil {
		t.Fatal(err)
	}
	return graphJSON
}

func writeRuntimeGraphWithSourceEvidence(t *testing.T, outDir, repoRoot string, generatedAt time.Time) string {
	t.Helper()
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	graphJSON := filepath.Join(outDir, "graph.json")
	if err := os.WriteFile(graphJSON, []byte(`{"nodes":[],"edges":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	graph := &types.Graph{
		Nodes: []types.Node{
			{ID: "auth", Label: "AuthService", NodeType: "struct", SourceFile: "auth.go"},
			{ID: "db", Label: "Database", NodeType: "struct", SourceFile: "auth.go"},
		},
		Edges: []types.Edge{{Source: "auth", Target: "db", Relation: "uses", Metadata: map[string]interface{}{"evidence_confidence": "extracted"}}},
	}
	if err := export.WriteSQLiteGraphAtomic(graph, outDir); err != nil {
		t.Fatalf("WriteSQLiteGraphAtomic error = %v", err)
	}
	manifest := types.Manifest{Version: 1, RepoRoot: repoRoot, GeneratedAt: generatedAt, BuildMode: "full_rebuild"}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "manifest.json"), manifestData, 0o644); err != nil {
		t.Fatal(err)
	}
	return graphJSON
}

func callToolResultText(t *testing.T, res *mcppkg.CallToolResult) string {
	t.Helper()
	if res == nil || len(res.Content) == 0 {
		t.Fatalf("expected non-empty MCP tool result")
	}
	text, ok := mcppkg.AsTextContent(res.Content[0])
	if !ok {
		t.Fatalf("expected text content in MCP tool result")
	}
	return text.Text
}

func assertEquivalentCoreResult(t *testing.T, adapter string, got, expected query.Result) {
	t.Helper()
	if got.SchemaVersion != expected.SchemaVersion || got.QueryKind != expected.QueryKind || got.Status != expected.Status {
		t.Fatalf("%s core header = (%q, %q, %q), want (%q, %q, %q)", adapter, got.SchemaVersion, got.QueryKind, got.Status, expected.SchemaVersion, expected.QueryKind, expected.Status)
	}
	if len(got.ResolvedSubjects) != len(expected.ResolvedSubjects) || got.ResolvedSubjects[0].ID != expected.ResolvedSubjects[0].ID {
		t.Fatalf("%s resolved subjects = %+v, want %+v", adapter, got.ResolvedSubjects, expected.ResolvedSubjects)
	}
	if len(got.Facts) != len(expected.Facts) || got.Facts[0].Subject != expected.Facts[0].Subject || got.Facts[0].Predicate != expected.Facts[0].Predicate || got.Facts[0].Object != expected.Facts[0].Object {
		t.Fatalf("%s facts = %+v, want %+v", adapter, got.Facts, expected.Facts)
	}
	if got.Facts[0].Confidence != expected.Facts[0].Confidence || got.Freshness.Status != expected.Freshness.Status {
		t.Fatalf("%s proof semantics = confidence %q freshness %q, want confidence %q freshness %q", adapter, got.Facts[0].Confidence, got.Freshness.Status, expected.Facts[0].Confidence, expected.Freshness.Status)
	}
}

func writeMCPFixtureGraph(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	graph := map[string]any{
		"nodes": []map[string]any{
			{"id": "auth", "label": "AuthService", "kind": "struct", "file": "auth.go"},
			{"id": "db", "label": "Database", "kind": "struct", "file": "db.go"},
		},
		"edges": []map[string]any{{"from": "auth", "to": "db", "kind": "uses", "metadata": map[string]any{"evidence_confidence": "extracted"}}},
	}
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent graph error = %v", err)
	}
	path := filepath.Join(dir, "graph.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(graph.json) error = %v", err)
	}
	return path
}

func testFileSHA256(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe() error = %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("Close(stdout pipe) error = %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll(stdout pipe) error = %v", err)
	}
	return string(data)
}

func TestBuildAndExtractCommandsRouteThroughSharedBuildService(t *testing.T) {
	restore := runBuildService
	t.Cleanup(func() { runBuildService = restore })

	tests := []struct {
		name    string
		args    []string
		wantUse string
	}{
		{name: "build", args: []string{"build", "/repo", "--language", "go", "--driver", "scip-go"}, wantUse: "build"},
		{name: "update", args: []string{"update", "/repo", "--language", "go", "--driver", "scip-go"}, wantUse: "update"},
		{name: "extract alias", args: []string{"extract", "/repo", "--language", "go", "--driver", "scip-go"}, wantUse: "extract"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured types.BuildRequest
			runBuildService = func(_ context.Context, outDir string, req types.BuildRequest) (buildOutput, error) {
				captured = req
				return buildOutput{GraphPath: outDir + "/graph.json", HTMLPath: outDir + "/graph.html", ReportPath: outDir + "/GRAPH_REPORT.md", ObsidianPath: "/vault/obsidian", Files: 1}, nil
			}

			root := rootCmd()
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			root.SetOut(stdout)
			root.SetErr(stderr)
			root.SetArgs(tt.args)

			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if captured.RepoRoot != "/repo" {
				t.Fatalf("RepoRoot = %q, want /repo", captured.RepoRoot)
			}
			if len(captured.Languages) != 1 || captured.Languages[0] != "go" {
				t.Fatalf("Languages = %v, want [go]", captured.Languages)
			}
			if len(captured.Drivers) != 1 || captured.Drivers[0] != "scip-go" {
				t.Fatalf("Drivers = %v, want [scip-go]", captured.Drivers)
			}
			for _, want := range []string{"graph.json", "graph.html", "GRAPH_REPORT.md", "/vault/obsidian"} {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("expected build output to mention %q, got %q", want, stdout.String())
				}
			}
		})
	}
}

func TestWatchCommandRoutesThroughSharedWatchService(t *testing.T) {
	restore := runWatchService
	t.Cleanup(func() { runWatchService = restore })

	var captured types.BuildRequest
	var capturedOutDir string
	runWatchService = func(ctx context.Context, outDir string, req types.BuildRequest, stdout, stderr io.Writer) error {
		captured = req
		capturedOutDir = outDir
		return nil
	}

	root := rootCmd()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs([]string{"watch", "/repo", "--language", "go", "--driver", "scip-go", "--out-dir", "/repo/.vela"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if captured.RepoRoot != "/repo" {
		t.Fatalf("RepoRoot = %q, want /repo", captured.RepoRoot)
	}
	if len(captured.Languages) != 1 || captured.Languages[0] != "go" {
		t.Fatalf("Languages = %v, want [go]", captured.Languages)
	}
	if len(captured.Drivers) != 1 || captured.Drivers[0] != "scip-go" {
		t.Fatalf("Drivers = %v, want [scip-go]", captured.Drivers)
	}
	if capturedOutDir != "/repo/.vela" {
		t.Fatalf("outDir = %q, want /repo/.vela", capturedOutDir)
	}
	if !strings.Contains(stdout.String(), "watching for changes") {
		t.Fatalf("expected watch startup message, got %q", stdout.String())
	}
}

func TestHooksInstallCommandRoutesThroughInstaller(t *testing.T) {
	restore := installRepoHooks
	t.Cleanup(func() { installRepoHooks = restore })

	called := false
	installRepoHooks = func(repoRoot, executablePath string) error {
		called = true
		if repoRoot != "/repo" {
			t.Fatalf("repoRoot = %q, want /repo", repoRoot)
		}
		if executablePath == "" {
			t.Fatal("expected executable path")
		}
		return nil
	}

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"hooks", "install", "/repo"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !called {
		t.Fatal("expected installRepoHooks to be called")
	}
	if !strings.Contains(stdout.String(), "installed Vela hooks") {
		t.Fatalf("expected install output, got %q", stdout.String())
	}
}

func TestHooksStatusCommandPrintsHookStates(t *testing.T) {
	restore := inspectRepoHooks
	t.Cleanup(func() { inspectRepoHooks = restore })

	inspectRepoHooks = func(repoRoot string) (hooks.Status, error) {
		return hooks.Status{RepoRoot: repoRoot, Hooks: map[string]bool{"post-commit": true, "post-checkout": false}}, nil
	}

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"hooks", "status", "/repo"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"repo: /repo", "post-commit: installed", "post-checkout: missing"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected %q in status output, got %q", want, stdout.String())
		}
	}
}

func TestHooksUninstallCommandRoutesThroughRemover(t *testing.T) {
	restore := uninstallRepoHooks
	t.Cleanup(func() { uninstallRepoHooks = restore })

	called := false
	uninstallRepoHooks = func(repoRoot string) error {
		called = true
		if repoRoot != "/repo" {
			t.Fatalf("repoRoot = %q, want /repo", repoRoot)
		}
		return nil
	}

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"hooks", "uninstall", "/repo"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !called {
		t.Fatal("expected uninstallRepoHooks to be called")
	}
	if !strings.Contains(stdout.String(), "removed Vela hooks") {
		t.Fatalf("expected uninstall output, got %q", stdout.String())
	}
}

func TestServeCommandOmitsLegacyAncoraFlag(t *testing.T) {
	cmd := serveCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	help := buf.String()
	if strings.Contains(help, "ancora-db") {
		t.Fatalf("expected serve help to omit legacy ancora-db flag, got %q", help)
	}
	for _, want := range []string{"--graph", "--http", "--port"} {
		if !strings.Contains(help, want) {
			t.Fatalf("expected serve help to contain %q, got %q", want, help)
		}
	}
}

func TestSearchCommandRoutesStructuralPromptToQueryService(t *testing.T) {
	graphPath := writeSearchTestGraph(t)

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"search", "who uses rootCmd", "--graph", graphPath, "--limit", "7"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"Reverse dependencies for \"rootCmd\":", "main [repo/function] via calls"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected stdout to contain %q, got %q", want, stdout.String())
		}
	}
}

func TestLookupCommandPrintsCandidateNodes(t *testing.T) {
	graphPath := writeSearchTestGraph(t)

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"lookup", "root", "--graph", graphPath, "--limit", "2"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"Candidates for \"root\":", "1. rootCmd", "id: cmd/vela/main.go:rootCmd", "Next steps:"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected stdout to contain %q, got %q", want, stdout.String())
		}
	}
}

// REQ-002/REQ-004 → SCN-001 → TestSCN001_CLIExploreAnswersKnownStructuralQuestionWithStableSections
func TestSCN001_CLIExploreAnswersKnownStructuralQuestionWithStableSections(t *testing.T) {
	// Scenario: CLI explore answers a known structural question with the stable sections.
	graphPath := writeFreshRefundServiceRuntimeGraph(t)

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"explore", "explain RefundService", "--graph", graphPath, "--limit", "3"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{
		"Answer",
		"Freshness",
		"Relevant source",
		"Paths and relationships",
		"Impact radius",
		"Layered evidence",
		"Confidence and limits",
		"Suggested next queries",
		"Interpreted intent: explain",
		"RefundService",
		"graph-backed evidence",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected stdout to contain %q, got %q", want, stdout.String())
		}
	}
}

// REQ-004/REQ-006 → SCN-003 → TestSCN003_CLIExploreIncludesRequiredSectionsWhenNotRelevant
func TestSCN003_CLIExploreIncludesRequiredSectionsWhenNotRelevant(t *testing.T) {
	// Scenario: Explore response includes every required section even when sections are not relevant.
	graphPath := writeFreshRefundStatusRuntimeGraph(t)

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"explore", "who uses RefundStatus?", "--graph", graphPath, "--limit", "3"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		"Freshness\n  status: fresh",
		"Relevant source\n  services/refund_status.go",
		"Paths and relationships",
		"RefundService [repo/struct] --[uses]--> RefundStatus [repo/enum]",
		"Impact radius\n  not relevant for this usage result",
		"Confidence and limits",
		"Source snippets are unavailable from this graph fact; missing graph families are reported as limits instead of omitted.",
		"vela search \"explain RefundStatus\"",
		"vela search \"who uses RefundStatus\"",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected stdout to contain %q, got %q", want, output)
		}
	}
}

// REQ-007/REQ-012 → SCN-004 → TestSCN004_PlannerRoutesCommonIntentFamiliesToExistingPrimitives
func TestSCN004_PlannerRoutesCommonIntentFamiliesToExistingPrimitives(t *testing.T) {
	// Scenario Outline: Planner routes common intent families to existing graph primitives.
	graphPath := writeExplorePlannerRuntimeGraph(t)

	cases := []struct {
		question  string
		intent    string
		primitive string
	}{
		{question: "explain RefundService", intent: "explain", primitive: "lookup/explain"},
		{question: "who uses RefundStatus?", intent: "usage", primitive: "reverse dependency / who uses"},
		{question: "what does WebhookHandler depend on?", intent: "dependency", primitive: "dependency / callee neighborhood"},
		{question: "how does StripeWebhook reach RefundService?", intent: "path", primitive: "path"},
		{question: "what breaks if RefundStatus changes?", intent: "impact", primitive: "impact / bounded reverse reach"},
	}

	for _, tc := range cases {
		t.Run(tc.intent, func(t *testing.T) {
			root := rootCmd()
			stdout := &bytes.Buffer{}
			root.SetOut(stdout)
			root.SetErr(&bytes.Buffer{})
			root.SetArgs([]string{"explore", tc.question, "--graph", graphPath, "--limit", "5"})

			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			output := stdout.String()
			for _, want := range []string{
				"Interpreted intent: " + tc.intent,
				"Derived primitive: " + tc.primitive,
				"Graph facts used:",
			} {
				if !strings.Contains(output, want) {
					t.Fatalf("expected stdout to contain %q, got %q", want, output)
				}
			}
		})
	}
}

// REQ-007/REQ-008 → SCN-005 → TestSCN005_AmbiguousExplainExploreQueryReturnsCandidates
func TestSCN005_AmbiguousExplainExploreQueryReturnsCandidates(t *testing.T) {
	// Scenario: Ambiguous explore query returns candidates instead of a strong claim.
	graphPath := writeAmbiguousExploreGraph(t)

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"explore", "explain auth", "--graph", graphPath, "--limit", "5"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		"Status: ambiguous",
		"Ambiguous explore query for \"explain auth\"",
		"AuthService",
		"AuthController",
		"file: services/auth/service.go",
		"file: services/auth/controller.go",
		"vela explore \"explain AuthService\"",
		"vela explore \"explain AuthController\"",
		"Refine the request",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected stdout to contain %q, got %q", want, output)
		}
	}
	if strings.Contains(output, "Graph facts used:") || strings.Contains(output, "graph-backed evidence:") {
		t.Fatalf("ambiguous explore output chose a graph-backed answer instead of asking for refinement: %q", output)
	}
}

// REQ-005/REQ-006 → SCN-006 → TestSCN006_CLIMissingRuntimeDBFailsWithActionableDiagnostics
func TestSCN006_CLIMissingRuntimeDBFailsWithActionableDiagnostics(t *testing.T) {
	// Scenario: Missing runtime DB fails with actionable diagnostics and no JSON fallback.
	graphPath := writeMissingRuntimeDBExploreGraph(t)

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"explore", "explain RefundService", "--graph", graphPath, "--limit", "3"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want missing runtime DB failure")
	}
	message := err.Error()
	for _, want := range []string{
		"freshness state: unavailable",
		".vela/graph.db is required for runtime graph answers",
		"vela build",
		"vela update",
		"vela status",
		".vela/graph.json is export/debug only and will not be used as runtime graph truth",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected error to contain %q, got %q", want, message)
		}
	}
	if strings.Contains(stdout.String(), "Graph facts used:") || strings.Contains(stdout.String(), "graph-backed evidence") {
		t.Fatalf("missing runtime DB used graph.json as graph-backed answer: %q", stdout.String())
	}
}

// REQ-006/REQ-008 → SCN-009 → TestSCN009_CLIExploreNamesKnownStaleAffectedFiles
func TestSCN009_CLIExploreNamesKnownStaleAffectedFiles(t *testing.T) {
	// Scenario: Stale or pending freshness names affected files when known.
	graphPath := writeStaleQueryEngineRuntimeGraph(t)

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"explore", "who uses QueryEngine?", "--graph", graphPath, "--limit", "3"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		"Freshness\n  status: stale",
		"affected files: internal/query/query.go",
		"exact latest source may require a direct file read",
		"vela update",
		"vela build",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected stdout to contain %q, got %q", want, output)
		}
	}
}

// REQ-001/REQ-011 → SCN-015 → TestSCN015_CLIExploreDefersWatcherAndDebounceForActiveSessionFreshness
func TestSCN015_CLIExploreDefersWatcherAndDebounceForActiveSessionFreshness(t *testing.T) {
	// Scenario: Phase 1 shell defers watcher and debounce implementation to later phases.
	graphPath := writeStaleQueryEngineRuntimeGraph(t)

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"explore", "how is active-session freshness handled?", "--graph", graphPath, "--limit", "3"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		"Active-session freshness",
		"known runtime freshness state: stale",
		"MCP-session file watching is deferred to a later phase",
		"debounced auto-sync is deferred to a later phase",
		"does not claim active-session watcher or debounced auto-sync is implemented by this Phase 1 shell",
		"vela update",
		"vela build",
		"vela status",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected stdout to contain %q, got %q", want, output)
		}
	}
}

// REQ-009 → SCN-010 → TestSCN010_CLIExploreSeparatesLayeredEvidenceLabels
func TestSCN010_CLIExploreSeparatesLayeredEvidenceLabels(t *testing.T) {
	// Scenario: Layered evidence labels separate code, workspace, and contract facts.
	graphPath := writeLayeredRefundEvidenceRuntimeGraph(t)

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"explore", "where is the refund API contract enforced?", "--graph", graphPath, "--limit", "5"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		"Layered evidence",
		"repo_code evidence:",
		"workspace evidence:",
		"contract evidence:",
		"Contract evidence is public-interface or behavior-contract context, not inferred executable code truth.",
		"resource evidence: unavailable",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected stdout to contain %q, got %q", want, output)
		}
	}
	if strings.Contains(output, "contract evidence: executable code truth") {
		t.Fatalf("contract evidence was presented as executable code truth: %q", output)
	}
}

// REQ-010 → SCN-011 → TestSCN011_NormalStructuralExploreOmitsMemoryEvidenceByDefault
func TestSCN011_NormalStructuralExploreOmitsMemoryEvidenceByDefault(t *testing.T) {
	// Scenario: Normal structural queries omit memory evidence by default.
	graphPath := writeRefundServiceRuntimeGraphWithMemoryObservation(t)

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"explore", "explain RefundService", "--graph", graphPath, "--limit", "3"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		"repo_code evidence:",
		"memory evidence: not requested",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected stdout to contain %q, got %q", want, output)
		}
	}
	for _, unwanted := range []string{
		"Prior refund decision [observation] --[documents]--> RefundService [repo/struct]",
		"memory evidence:\n    Prior refund decision",
	} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("normal structural explore included memory evidence %q in output %q", unwanted, output)
		}
	}
}

// REQ-010 → SCN-012 → TestSCN012_DecisionHistoryExploreIncludesSeparateMemoryEvidence
func TestSCN012_DecisionHistoryExploreIncludesSeparateMemoryEvidence(t *testing.T) {
	// Scenario: Decision-history queries include memory evidence as a separate layer.
	graphPath := writeStripeRefundDecisionRuntimeGraph(t)

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"explore", "what did we decide about Stripe refunds?", "--graph", graphPath, "--limit", "3"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		"Interpreted intent: memory",
		"memory evidence:",
		"Prior Stripe refund decision [memory/observation] --[documents]--> Stripe refunds [repo/service]",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected stdout to contain %q, got %q", want, output)
		}
	}
	if strings.Contains(output, "repo_code evidence:\n    Prior Stripe refund decision [memory/observation]") {
		t.Fatalf("memory evidence was merged into repo_code facts: %q", output)
	}
}

// REQ-005 → SCN-006 → TestSCN006_ExploreResolvesBroadRequestIntoGraphBackedContext
func TestSCN006_ExploreResolvesBroadRequestIntoGraphBackedContext(t *testing.T) {
	// Scenario: Explore resolves natural language into graph-backed structural context.
	graphPath := writeSearchTestGraph(t)

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"explore", "root command", "--graph", graphPath, "--limit", "2"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"Resolved candidates for \"root command\":", "rootCmd", "Graph facts used:", "main [repo/function] --[calls]--> rootCmd [repo/function]"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected stdout to contain %q, got %q", want, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "free-text proof") {
		t.Fatalf("explore output presented free-text matching as proof: %q", stdout.String())
	}
}

// REQ-005/REQ-015 → SCN-007 → TestSCN007_AmbiguousExploreQueryReturnsCandidates
func TestSCN007_AmbiguousExploreQueryReturnsCandidates(t *testing.T) {
	// Scenario: Ambiguous explore query returns candidates instead of choosing silently.
	graphPath := writeAmbiguousExploreGraph(t)

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"explore", "auth", "--graph", graphPath, "--limit", "5"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{
		"Ambiguous explore query for \"auth\"",
		"AuthService",
		"AuthController",
		"file: services/auth/service.go",
		"file: services/auth/controller.go",
		"Refine the request or run `vela lookup \"auth\"` before asking for a strong graph claim.",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected stdout to contain %q, got %q", want, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "Graph facts used:") {
		t.Fatalf("ambiguous explore output chose a graph-backed answer instead of asking for refinement: %q", stdout.String())
	}
}

// REQ-012 → SCN-016 → TestSCN016_MultiRepoExploreRoutesBeforeDeepRetrieval
func TestSCN016_MultiRepoExploreRoutesBeforeDeepRetrieval(t *testing.T) {
	// Scenario: Multi-repo exploration routes first and retrieves deeply second.
	graphPath := writeMultiRepoExploreGraph(t)

	root := rootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"explore", "billing checkout", "--graph", graphPath, "--limit", "5"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		"Workspace routes for \"billing checkout\":",
		"Route ambiguity: multiple workspace routes match",
		"billing-api score=",
		"checkout-web score=",
		"Workspace routing facts:",
		"billing-api [workspace/repo] --[exposes]--> billing [workspace/service]",
		"Selected workspace routes are routing/topology truth, not deep code truth.",
		"Deep graph retrieval candidates:",
		"BillingHandler",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected stdout to contain %q, got %q", want, output)
		}
	}
	if strings.Index(output, "Workspace routes for") > strings.Index(output, "Deep graph retrieval candidates:") {
		t.Fatalf("expected workspace routes before deep retrieval candidates, got %q", output)
	}
}

func TestQueryCommandSuggestsLookupWhenSubjectIsMissing(t *testing.T) {
	graphPath := writeSearchTestGraph(t)

	root := rootCmd()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs([]string{"query", "dependencies", "MissingNode", "--graph", graphPath})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected missing-node error")
	}
	for _, want := range []string{"node \"MissingNode\" not found", "hint: try `vela lookup \"MissingNode\"` to find candidate nodes"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %q", want, err.Error())
		}
	}
}

func writeFreshRefundServiceRuntimeGraph(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	t.Setenv("HOME", filepath.Join(repo, "home"))
	servicePath := filepath.Join(repo, "services", "refund.go")
	if err := os.MkdirAll(filepath.Dir(servicePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(service dir) error = %v", err)
	}
	serviceSource := []byte("package services\n\ntype RefundService struct{}\n")
	if err := os.WriteFile(servicePath, serviceSource, 0o644); err != nil {
		t.Fatalf("WriteFile(refund service) error = %v", err)
	}
	outDir := filepath.Join(repo, ".vela")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(.vela) error = %v", err)
	}
	graph := &types.Graph{
		Nodes: []types.Node{
			{ID: "services/refund.go:RefundService", Label: "RefundService", NodeType: "struct", SourceFile: "services/refund.go"},
			{ID: "services/refund.go:RefundRepository", Label: "RefundRepository", NodeType: "interface", SourceFile: "services/refund.go"},
		},
		Edges: []types.Edge{
			{
				Source:     "services/refund.go:RefundService",
				Target:     "services/refund.go:RefundRepository",
				Relation:   "uses",
				SourceFile: "services/refund.go",
				Confidence: string(types.ConfidenceExtracted),
				Metadata: map[string]interface{}{
					"layer":                    string(types.LayerRepo),
					"evidence_type":            "static-analysis",
					"evidence_source_artifact": "services/refund.go",
					"evidence_confidence":      string(types.ConfidenceExtracted),
				},
			},
		},
	}
	if err := export.WriteSQLiteGraphAtomic(graph, outDir); err != nil {
		t.Fatalf("WriteSQLiteGraphAtomic error = %v", err)
	}
	manifest := types.Manifest{
		Version:              1,
		RepoRoot:             repo,
		GeneratedAt:          time.Now().UTC(),
		ExtractorFingerprint: "test",
		BuildMode:            "test",
		Files: []types.ManifestFile{{
			Path:   "services/refund.go",
			SHA256: hexSHA256(serviceSource),
			Size:   int64(len(serviceSource)),
		}},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal(manifest) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "manifest.json"), manifestJSON, 0o644); err != nil {
		t.Fatalf("WriteFile(manifest.json) error = %v", err)
	}
	graphPath := filepath.Join(outDir, "graph.json")
	if err := os.WriteFile(graphPath, []byte(`{"nodes":[],"edges":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile(graph.json) error = %v", err)
	}
	return graphPath
}

func writeFreshRefundStatusRuntimeGraph(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	t.Setenv("HOME", filepath.Join(repo, "home"))
	statusPath := filepath.Join(repo, "services", "refund_status.go")
	if err := os.MkdirAll(filepath.Dir(statusPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(service dir) error = %v", err)
	}
	statusSource := []byte("package services\n\ntype RefundStatus string\ntype RefundService struct{}\n")
	if err := os.WriteFile(statusPath, statusSource, 0o644); err != nil {
		t.Fatalf("WriteFile(refund status) error = %v", err)
	}
	outDir := filepath.Join(repo, ".vela")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(.vela) error = %v", err)
	}
	graph := &types.Graph{
		Nodes: []types.Node{
			{ID: "services/refund_status.go:RefundService", Label: "RefundService", NodeType: "struct", SourceFile: "services/refund_status.go"},
			{ID: "services/refund_status.go:RefundStatus", Label: "RefundStatus", NodeType: "enum", SourceFile: "services/refund_status.go"},
		},
		Edges: []types.Edge{{
			Source:     "services/refund_status.go:RefundService",
			Target:     "services/refund_status.go:RefundStatus",
			Relation:   "uses",
			SourceFile: "services/refund_status.go",
			Confidence: string(types.ConfidenceExtracted),
			Metadata: map[string]interface{}{
				"layer":                    string(types.LayerRepo),
				"evidence_type":            "static-analysis",
				"evidence_source_artifact": "services/refund_status.go",
				"evidence_confidence":      string(types.ConfidenceExtracted),
			},
		}},
	}
	if err := export.WriteSQLiteGraphAtomic(graph, outDir); err != nil {
		t.Fatalf("WriteSQLiteGraphAtomic error = %v", err)
	}
	manifest := types.Manifest{
		Version:              1,
		RepoRoot:             repo,
		GeneratedAt:          time.Now().UTC(),
		ExtractorFingerprint: "test",
		BuildMode:            "test",
		Files: []types.ManifestFile{{
			Path:   "services/refund_status.go",
			SHA256: hexSHA256(statusSource),
			Size:   int64(len(statusSource)),
		}},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal(manifest) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "manifest.json"), manifestJSON, 0o644); err != nil {
		t.Fatalf("WriteFile(manifest.json) error = %v", err)
	}
	graphPath := filepath.Join(outDir, "graph.json")
	if err := os.WriteFile(graphPath, []byte(`{"nodes":[],"edges":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile(graph.json) error = %v", err)
	}
	return graphPath
}

func writeStaleQueryEngineRuntimeGraph(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	t.Setenv("HOME", filepath.Join(repo, "home"))
	queryPath := filepath.Join(repo, "internal", "query", "query.go")
	if err := os.MkdirAll(filepath.Dir(queryPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(query dir) error = %v", err)
	}
	querySource := []byte("package query\n\ntype QueryEngine struct{}\ntype QueryRunner struct{}\n")
	if err := os.WriteFile(queryPath, querySource, 0o644); err != nil {
		t.Fatalf("WriteFile(query source) error = %v", err)
	}
	outDir := filepath.Join(repo, ".vela")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(.vela) error = %v", err)
	}
	graph := &types.Graph{
		Nodes: []types.Node{
			{ID: "internal/query/query.go:QueryRunner", Label: "QueryRunner", NodeType: "struct", SourceFile: "internal/query/query.go"},
			{ID: "internal/query/query.go:QueryEngine", Label: "QueryEngine", NodeType: "struct", SourceFile: "internal/query/query.go"},
		},
		Edges: []types.Edge{{
			Source:     "internal/query/query.go:QueryRunner",
			Target:     "internal/query/query.go:QueryEngine",
			Relation:   "uses",
			SourceFile: "internal/query/query.go",
			Confidence: string(types.ConfidenceExtracted),
			Metadata: map[string]interface{}{
				"layer":                    string(types.LayerRepo),
				"evidence_type":            "static-analysis",
				"evidence_source_artifact": "internal/query/query.go",
				"evidence_confidence":      string(types.ConfidenceExtracted),
			},
		}},
	}
	if err := export.WriteSQLiteGraphAtomic(graph, outDir); err != nil {
		t.Fatalf("WriteSQLiteGraphAtomic error = %v", err)
	}
	manifest := types.Manifest{
		Version:              1,
		RepoRoot:             repo,
		GeneratedAt:          time.Now().UTC(),
		ExtractorFingerprint: "test",
		BuildMode:            "test",
		Files: []types.ManifestFile{{
			Path:   "internal/query/query.go",
			SHA256: strings.Repeat("0", 64),
			Size:   int64(len(querySource)),
		}},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal(manifest) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "manifest.json"), manifestJSON, 0o644); err != nil {
		t.Fatalf("WriteFile(manifest.json) error = %v", err)
	}
	graphPath := filepath.Join(outDir, "graph.json")
	if err := os.WriteFile(graphPath, []byte(`{"nodes":[],"edges":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile(graph.json) error = %v", err)
	}
	return graphPath
}

func writeExplorePlannerRuntimeGraph(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	t.Setenv("HOME", filepath.Join(repo, "home"))
	sourcePath := filepath.Join(repo, "services", "refund_flow.go")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(service dir) error = %v", err)
	}
	source := []byte("package services\n\ntype RefundStatus string\ntype RefundService struct{}\ntype WebhookHandler struct{}\ntype StripeWebhook struct{}\n")
	if err := os.WriteFile(sourcePath, source, 0o644); err != nil {
		t.Fatalf("WriteFile(refund flow) error = %v", err)
	}
	outDir := filepath.Join(repo, ".vela")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(.vela) error = %v", err)
	}
	graph := &types.Graph{
		Nodes: []types.Node{
			{ID: "services/refund_flow.go:RefundStatus", Label: "RefundStatus", NodeType: "enum", SourceFile: "services/refund_flow.go"},
			{ID: "services/refund_flow.go:RefundService", Label: "RefundService", NodeType: "struct", SourceFile: "services/refund_flow.go"},
			{ID: "services/refund_flow.go:WebhookHandler", Label: "WebhookHandler", NodeType: "struct", SourceFile: "services/refund_flow.go"},
			{ID: "services/refund_flow.go:StripeWebhook", Label: "StripeWebhook", NodeType: "struct", SourceFile: "services/refund_flow.go"},
		},
		Edges: []types.Edge{
			plannerRuntimeEdge("services/refund_flow.go:RefundService", "services/refund_flow.go:RefundStatus"),
			plannerRuntimeEdge("services/refund_flow.go:WebhookHandler", "services/refund_flow.go:RefundService"),
			plannerRuntimeEdge("services/refund_flow.go:StripeWebhook", "services/refund_flow.go:WebhookHandler"),
		},
	}
	if err := export.WriteSQLiteGraphAtomic(graph, outDir); err != nil {
		t.Fatalf("WriteSQLiteGraphAtomic error = %v", err)
	}
	manifest := types.Manifest{
		Version:              1,
		RepoRoot:             repo,
		GeneratedAt:          time.Now().UTC(),
		ExtractorFingerprint: "test",
		BuildMode:            "test",
		Files: []types.ManifestFile{{
			Path:   "services/refund_flow.go",
			SHA256: hexSHA256(source),
			Size:   int64(len(source)),
		}},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal(manifest) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "manifest.json"), manifestJSON, 0o644); err != nil {
		t.Fatalf("WriteFile(manifest.json) error = %v", err)
	}
	graphPath := filepath.Join(outDir, "graph.json")
	if err := os.WriteFile(graphPath, []byte(`{"nodes":[],"edges":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile(graph.json) error = %v", err)
	}
	return graphPath
}

func plannerRuntimeEdge(source, target string) types.Edge {
	return types.Edge{
		Source:     source,
		Target:     target,
		Relation:   "uses",
		SourceFile: "services/refund_flow.go",
		Confidence: string(types.ConfidenceExtracted),
		Metadata: map[string]interface{}{
			"layer":                    string(types.LayerRepo),
			"evidence_type":            "static-analysis",
			"evidence_source_artifact": "services/refund_flow.go",
			"evidence_confidence":      string(types.ConfidenceExtracted),
		},
	}
}

func writeLayeredRefundEvidenceRuntimeGraph(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	t.Setenv("HOME", filepath.Join(repo, "home"))
	sourcePath := filepath.Join(repo, "services", "payments", "handler.go")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(service dir) error = %v", err)
	}
	source := []byte("package payments\n\nfunc Handler() {}\n")
	if err := os.WriteFile(sourcePath, source, 0o644); err != nil {
		t.Fatalf("WriteFile(refund handler) error = %v", err)
	}
	outDir := filepath.Join(repo, ".vela")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(.vela) error = %v", err)
	}
	contractID := "contract:endpoint:refund-api-enforced"
	graph := &types.Graph{
		Nodes: []types.Node{
			{ID: "services/payments/handler.go:Handler", Label: "Handler", NodeType: "function", SourceFile: "services/payments/handler.go"},
			{ID: "workspace:service:payments", Label: "payments", NodeType: "service", SourceFile: ".vela/workspace.yaml", Metadata: map[string]interface{}{"layer": string(types.LayerWorkspace)}},
			{ID: contractID, Label: "where is the refund API contract enforced", NodeType: "endpoint", SourceFile: "openapi/refunds.yaml", Metadata: map[string]interface{}{"layer": string(types.LayerContract)}},
		},
		Edges: []types.Edge{
			layeredRuntimeEdge("services/payments/handler.go:Handler", contractID, "enforces", types.LayerRepo, "static-analysis", "services/payments/handler.go"),
			layeredRuntimeEdge("workspace:service:payments", contractID, "routes", types.LayerWorkspace, "routing", ".vela/workspace.yaml"),
			layeredRuntimeEdge(contractID, "services/payments/handler.go:Handler", "declares_behavior_for", types.LayerContract, "openapi", "openapi/refunds.yaml"),
		},
	}
	if err := export.WriteSQLiteGraphAtomic(graph, outDir); err != nil {
		t.Fatalf("WriteSQLiteGraphAtomic error = %v", err)
	}
	manifest := types.Manifest{
		Version:              1,
		RepoRoot:             repo,
		GeneratedAt:          time.Now().UTC(),
		ExtractorFingerprint: "test",
		BuildMode:            "test",
		Files: []types.ManifestFile{{
			Path:   "services/payments/handler.go",
			SHA256: hexSHA256(source),
			Size:   int64(len(source)),
		}},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal(manifest) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "manifest.json"), manifestJSON, 0o644); err != nil {
		t.Fatalf("WriteFile(manifest.json) error = %v", err)
	}
	graphPath := filepath.Join(outDir, "graph.json")
	if err := os.WriteFile(graphPath, []byte(`{"nodes":[],"edges":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile(graph.json) error = %v", err)
	}
	return graphPath
}

func layeredRuntimeEdge(source, target, relation string, layer types.Layer, evidenceType, artifact string) types.Edge {
	return types.Edge{
		Source:     source,
		Target:     target,
		Relation:   relation,
		SourceFile: artifact,
		Confidence: string(types.ConfidenceExtracted),
		Metadata: map[string]interface{}{
			"layer":                    string(layer),
			"evidence_type":            evidenceType,
			"evidence_source_artifact": artifact,
			"evidence_confidence":      string(types.ConfidenceExtracted),
		},
	}
}

func writeRefundServiceRuntimeGraphWithMemoryObservation(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	t.Setenv("HOME", filepath.Join(repo, "home"))
	servicePath := filepath.Join(repo, "services", "refund.go")
	if err := os.MkdirAll(filepath.Dir(servicePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(service dir) error = %v", err)
	}
	serviceSource := []byte("package services\n\ntype RefundService struct{}\ntype RefundRepository interface{}\n")
	if err := os.WriteFile(servicePath, serviceSource, 0o644); err != nil {
		t.Fatalf("WriteFile(refund service) error = %v", err)
	}
	outDir := filepath.Join(repo, ".vela")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(.vela) error = %v", err)
	}
	graph := &types.Graph{
		Nodes: []types.Node{
			{ID: "services/refund.go:RefundService", Label: "RefundService", NodeType: "struct", SourceFile: "services/refund.go"},
			{ID: "services/refund.go:RefundRepository", Label: "RefundRepository", NodeType: "interface", SourceFile: "services/refund.go"},
			{ID: "memory:observation:refund", Label: "Prior refund decision", NodeType: "observation", SourceFile: "ancora:obs:refund", Metadata: map[string]interface{}{"layer": string(types.LayerMemory)}},
		},
		Edges: []types.Edge{
			layeredRuntimeEdge("services/refund.go:RefundService", "services/refund.go:RefundRepository", "uses", types.LayerRepo, "static-analysis", "services/refund.go"),
			layeredRuntimeEdge("memory:observation:refund", "services/refund.go:RefundService", "documents", types.LayerMemory, "observation-reference", "ancora:obs:refund"),
		},
	}
	if err := export.WriteSQLiteGraphAtomic(graph, outDir); err != nil {
		t.Fatalf("WriteSQLiteGraphAtomic error = %v", err)
	}
	manifest := types.Manifest{
		Version:              1,
		RepoRoot:             repo,
		GeneratedAt:          time.Now().UTC(),
		ExtractorFingerprint: "test",
		BuildMode:            "test",
		Files: []types.ManifestFile{{
			Path:   "services/refund.go",
			SHA256: hexSHA256(serviceSource),
			Size:   int64(len(serviceSource)),
		}},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal(manifest) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "manifest.json"), manifestJSON, 0o644); err != nil {
		t.Fatalf("WriteFile(manifest.json) error = %v", err)
	}
	graphPath := filepath.Join(outDir, "graph.json")
	if err := os.WriteFile(graphPath, []byte(`{"nodes":[],"edges":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile(graph.json) error = %v", err)
	}
	return graphPath
}

func writeStripeRefundDecisionRuntimeGraph(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	t.Setenv("HOME", filepath.Join(repo, "home"))
	servicePath := filepath.Join(repo, "services", "stripe_refunds.go")
	if err := os.MkdirAll(filepath.Dir(servicePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(service dir) error = %v", err)
	}
	serviceSource := []byte("package services\n\ntype StripeRefunds struct{}\n")
	if err := os.WriteFile(servicePath, serviceSource, 0o644); err != nil {
		t.Fatalf("WriteFile(stripe refunds service) error = %v", err)
	}
	outDir := filepath.Join(repo, ".vela")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(.vela) error = %v", err)
	}
	graph := &types.Graph{
		Nodes: []types.Node{
			{ID: "services/stripe_refunds.go:StripeRefunds", Label: "Stripe refunds", NodeType: "service", SourceFile: "services/stripe_refunds.go"},
			{ID: "memory:observation:stripe-refunds", Label: "Prior Stripe refund decision", NodeType: "observation", SourceFile: "ancora:obs:stripe-refunds", Metadata: map[string]interface{}{"layer": string(types.LayerMemory)}},
		},
		Edges: []types.Edge{
			layeredRuntimeEdge("memory:observation:stripe-refunds", "services/stripe_refunds.go:StripeRefunds", "documents", types.LayerMemory, "observation-reference", "ancora:obs:stripe-refunds"),
		},
	}
	if err := export.WriteSQLiteGraphAtomic(graph, outDir); err != nil {
		t.Fatalf("WriteSQLiteGraphAtomic error = %v", err)
	}
	manifest := types.Manifest{
		Version:              1,
		RepoRoot:             repo,
		GeneratedAt:          time.Now().UTC(),
		ExtractorFingerprint: "test",
		BuildMode:            "test",
		Files: []types.ManifestFile{{
			Path:   "services/stripe_refunds.go",
			SHA256: hexSHA256(serviceSource),
			Size:   int64(len(serviceSource)),
		}},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal(manifest) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "manifest.json"), manifestJSON, 0o644); err != nil {
		t.Fatalf("WriteFile(manifest.json) error = %v", err)
	}
	graphPath := filepath.Join(outDir, "graph.json")
	if err := os.WriteFile(graphPath, []byte(`{"nodes":[],"edges":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile(graph.json) error = %v", err)
	}
	return graphPath
}

func hexSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func writeSearchTestGraph(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	graph := map[string]any{
		"nodes": []map[string]any{
			{"id": "cmd/vela/main.go:rootCmd", "label": "rootCmd", "kind": "function", "file": "cmd/vela/main.go"},
			{"id": "cmd/vela/main.go:main", "label": "main", "kind": "function", "file": "cmd/vela/main.go"},
		},
		"edges": []map[string]any{
			{"from": "cmd/vela/main.go:main", "to": "cmd/vela/main.go:rootCmd", "kind": "calls"},
		},
		"meta": map[string]any{"nodeCount": 2, "edgeCount": 1},
	}
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	path := filepath.Join(dir, "graph.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func writeAmbiguousExploreGraph(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	graph := map[string]any{
		"nodes": []map[string]any{
			{"id": "services/auth/service.go:AuthService", "label": "AuthService", "kind": "struct", "file": "services/auth/service.go"},
			{"id": "services/auth/controller.go:AuthController", "label": "AuthController", "kind": "struct", "file": "services/auth/controller.go"},
		},
		"edges": []map[string]any{},
		"meta":  map[string]any{"nodeCount": 2, "edgeCount": 0},
	}
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	path := filepath.Join(dir, "graph.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func writeMissingRuntimeDBExploreGraph(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	velaDir := filepath.Join(dir, ".vela")
	if err := os.MkdirAll(velaDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(.vela) error = %v", err)
	}
	graph := map[string]any{
		"nodes": []map[string]any{
			{"id": "services/refund.go:RefundService", "label": "RefundService", "kind": "struct", "file": "services/refund.go"},
		},
		"edges": []map[string]any{},
		"meta":  map[string]any{"nodeCount": 1, "edgeCount": 0},
	}
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	path := filepath.Join(velaDir, "graph.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(graph.json) error = %v", err)
	}
	return path
}

func writeMultiRepoExploreGraph(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	graph := map[string]any{
		"nodes": []map[string]any{
			{"id": "workspace:repo:billing-api", "label": "billing-api", "kind": "repo", "file": ".vela/workspace.yaml", "metadata": map[string]any{"layer": "workspace", "evidence_type": "routing", "evidence_confidence": "declared_hint"}},
			{"id": "workspace:service:billing", "label": "billing", "kind": "service", "file": ".vela/workspace.yaml", "metadata": map[string]any{"layer": "workspace", "evidence_type": "routing", "evidence_confidence": "declared_hint"}},
			{"id": "workspace:repo:checkout-web", "label": "checkout-web", "kind": "repo", "file": ".vela/workspace.yaml", "metadata": map[string]any{"layer": "workspace", "evidence_type": "routing", "evidence_confidence": "declared_hint"}},
			{"id": "workspace:service:checkout", "label": "checkout", "kind": "service", "file": ".vela/workspace.yaml", "metadata": map[string]any{"layer": "workspace", "evidence_type": "routing", "evidence_confidence": "declared_hint"}},
			{"id": "services/billing/handler.go:BillingHandler", "label": "BillingHandler", "kind": "function", "file": "services/billing/handler.go"},
		},
		"edges": []map[string]any{
			{"from": "workspace:repo:billing-api", "to": "workspace:service:billing", "kind": "exposes", "metadata": map[string]any{"layer": "workspace", "evidence_type": "routing", "evidence_confidence": "declared_hint", "evidence_source_artifact": ".vela/workspace.yaml"}},
			{"from": "workspace:repo:checkout-web", "to": "workspace:service:checkout", "kind": "exposes", "metadata": map[string]any{"layer": "workspace", "evidence_type": "routing", "evidence_confidence": "declared_hint", "evidence_source_artifact": ".vela/workspace.yaml"}},
		},
		"meta": map[string]any{"nodeCount": 5, "edgeCount": 2},
	}
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	path := filepath.Join(dir, "graph.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
