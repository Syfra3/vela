package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// REQ-013 → SCN-016 → TestSCN016_TUIMainMenuExposesAgentInstallerWizard
func TestSCN016_TUIMainMenuExposesAgentInstallerWizard(t *testing.T) {
	m := NewMenuModel()
	view := m.View()
	for _, want := range []string{"Install Agent Integration", "Configure Vela MCP for coding agents"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected main menu to contain %q, got %q", want, view)
		}
	}

	installIndex := -1
	for i, item := range m.items {
		if item.key == "agentinstall" {
			installIndex = i
			break
		}
	}
	if installIndex == -1 {
		t.Fatal("agent installer menu item not found")
	}
	m.cursor = installIndex
	updated, _ := m.handleMenuSelect()
	menu := updated.(MenuModel)
	if menu.screen != screenAgentInstall {
		t.Fatalf("screen = %v, want %v", menu.screen, screenAgentInstall)
	}
	if got := menu.viewAgentInstall(); !strings.Contains(got, "configure explore") {
		t.Fatalf("expected wizard to explain explore setup, got %q", got)
	}
}

func TestAgentInstallerMenuConfirmWritesOnceAndStaysOnResult(t *testing.T) {
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project")
	configDir := filepath.Join(tmp, "opencode")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}

	var calls int
	menu := NewMenuModel()
	menu.screen = screenAgentInstall
	menu.agentInstallModel = AgentInstallModel{
		projectDir: projectDir,
		targets:    []AgentInstallTarget{{Name: "OpenCode", Agent: "opencode", ConfigDir: configDir, Supported: true}},
		runInstall: func(req AgentInstallRequest) (AgentInstallResult, error) {
			calls++
			return runAgentInstallFunc(req)
		},
	}

	updated, cmd := menu.updateAgentInstall(tea.KeyMsg{Type: tea.KeyEnter})
	menu = updated.(MenuModel)
	if cmd != nil || menu.agentInstallModel.step != agentInstallStepPreview {
		t.Fatalf("select should show preview without command, step=%v cmd=%v", menu.agentInstallModel.step, cmd)
	}

	updated, cmd = menu.updateAgentInstall(tea.KeyMsg{Type: tea.KeyEnter})
	menu = updated.(MenuModel)
	if cmd == nil {
		t.Fatal("confirm should return install command")
	}
	updated, cmd = menu.Update(cmd())
	menu = updated.(MenuModel)
	if cmd != nil {
		t.Fatal("install completion should not schedule another install command")
	}
	if calls != 1 {
		t.Fatalf("installer calls = %d, want 1", calls)
	}
	if menu.screen != screenAgentInstall || menu.agentInstallModel.step != agentInstallStepResult {
		t.Fatalf("screen/step = %v/%v, want agent install result", menu.screen, menu.agentInstallModel.step)
	}
	for _, path := range []string{filepath.Join(configDir, "opencode.json"), filepath.Join(configDir, "instructions.md"), filepath.Join(projectDir, ".vela", "graph.db")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected installer to write %s: %v", path, err)
		}
	}
	view := menu.View()
	if !strings.Contains(view, "Installation complete") || !strings.Contains(view, "esc back to menu") {
		t.Fatalf("expected completion screen with explicit exit guidance, got %q", view)
	}
}

// REQ-013 → SCN-017 → TestSCN017_TUIInstallerListsSupportedAndUnsupportedTargetsSafely
func TestSCN017_TUIInstallerListsSupportedAndUnsupportedTargetsSafely(t *testing.T) {
	model := AgentInstallModel{
		projectDir: "/work/vela",
		targets: []AgentInstallTarget{
			{Name: "OpenCode", Agent: "opencode", ConfigDir: "/tmp/opencode", Supported: true},
			{Name: "Claude Code", Agent: "claude", ConfigDir: "/tmp/claude", Supported: true},
			{Name: "Cursor", Agent: "cursor", ConfigDir: "/tmp/cursor", Supported: false},
		},
	}

	view := model.ViewContent()
	for _, want := range []string{"OpenCode", "installable", "Claude Code", "Cursor", "guidance-only"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected target selection view to contain %q, got %q", want, view)
		}
	}

	model.cursor = 2
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(AgentInstallModel)
	if cmd != nil {
		t.Fatal("unsupported target should not start a command")
	}
	if model.step != agentInstallStepTargets {
		t.Fatalf("step = %v, want target selection", model.step)
	}
	if !strings.Contains(model.message, "not installable") {
		t.Fatalf("expected unsupported target guidance, got %q", model.message)
	}
}

// REQ-014 → SCN-018 → TestSCN018_TUIInstallerPreviewsFilesBeforeWriting
func TestSCN018_TUIInstallerPreviewsFilesBeforeWriting(t *testing.T) {
	called := false
	model := AgentInstallModel{
		projectDir: "/work/vela",
		targets:    []AgentInstallTarget{{Name: "OpenCode", Agent: "opencode", ConfigDir: "/tmp/opencode", Supported: true}},
		runInstall: func(AgentInstallRequest) (AgentInstallResult, error) {
			called = true
			return AgentInstallResult{}, nil
		},
	}

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(AgentInstallModel)
	if cmd != nil || called {
		t.Fatal("preview selection must not write files")
	}
	view := model.ViewContent()
	for _, want := range []string{"Preview", "/tmp/opencode/opencode.json", "/tmp/opencode/instructions.md", "/work/vela/.vela/graph.db", "unrelated config is preserved"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected preview to contain %q, got %q", want, view)
		}
	}
}

// REQ-014/REQ-015 → SCN-019 → TestSCN019_TUIInstallerConfirmsOpenCodeThroughSharedInstaller
func TestSCN019_TUIInstallerConfirmsOpenCodeThroughSharedInstaller(t *testing.T) {
	var gotReq AgentInstallRequest
	model := AgentInstallModel{
		step:       agentInstallStepPreview,
		projectDir: "/work/vela",
		selected:   AgentInstallTarget{Name: "OpenCode", Agent: "opencode", ConfigDir: "/tmp/opencode", Supported: true},
		runInstall: func(req AgentInstallRequest) (AgentInstallResult, error) {
			gotReq = req
			return AgentInstallResult{MCPConfigPath: "/tmp/opencode/opencode.json", InstructionPath: "/tmp/opencode/instructions.md", GraphDBPath: "/work/vela/.vela/graph.db", GraphReady: true}, nil
		},
	}

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(AgentInstallModel)
	if cmd == nil {
		t.Fatal("expected confirm to start shared installer command")
	}
	msg := cmd()
	updated, _ = model.Update(msg)
	model = updated.(AgentInstallModel)
	if gotReq.Agent != "opencode" || gotReq.ProjectDir != "/work/vela" {
		t.Fatalf("installer request = %+v, want opencode /work/vela", gotReq)
	}
	view := model.ViewContent()
	for _, want := range []string{"MCP config written", "instructions mention explore first", ".vela/graph.db is present", "Try asking your coding agent"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected result to contain %q, got %q", want, view)
		}
	}
}

// REQ-014/REQ-015 → SCN-020 → TestSCN020_TUIInstallerConfirmsClaudeThroughSharedInstaller
func TestSCN020_TUIInstallerConfirmsClaudeThroughSharedInstaller(t *testing.T) {
	var calls int
	model := AgentInstallModel{
		step:       agentInstallStepPreview,
		projectDir: "/work/vela",
		selected:   AgentInstallTarget{Name: "Claude Code", Agent: "claude", ConfigDir: "/tmp/claude", Supported: true},
		runInstall: func(req AgentInstallRequest) (AgentInstallResult, error) {
			calls++
			if req.Agent != "claude" {
				t.Fatalf("agent = %q, want claude", req.Agent)
			}
			return AgentInstallResult{MCPConfigPath: "/tmp/claude/vela-mcp.json", InstructionPath: "/tmp/claude/vela-instructions.md", GraphDBPath: "/work/vela/.vela/graph.db", GraphReady: true, Idempotent: true}, nil
		},
	}

	for i := 0; i < 2; i++ {
		updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model = updated.(AgentInstallModel)
		if cmd == nil {
			t.Fatal("expected confirm command")
		}
		updated, _ = model.Update(cmd())
		model = updated.(AgentInstallModel)
		model.step = agentInstallStepPreview
	}
	if calls != 2 {
		t.Fatalf("installer calls = %d, want 2", calls)
	}
	if got := model.ViewContent(); !strings.Contains(got, "idempotent") {
		t.Fatalf("expected idempotent result, got %q", got)
	}
}

// REQ-014 → SCN-021 → TestSCN021_TUIInstallerCancelExitsWithoutWriting
func TestSCN021_TUIInstallerCancelExitsWithoutWriting(t *testing.T) {
	called := false
	menu := NewMenuModel()
	menu.screen = screenAgentInstall
	menu.agentInstallModel = AgentInstallModel{
		projectDir: "/work/vela",
		targets:    []AgentInstallTarget{{Name: "OpenCode", Agent: "opencode", ConfigDir: "/tmp/opencode", Supported: true}},
		runInstall: func(AgentInstallRequest) (AgentInstallResult, error) {
			called = true
			return AgentInstallResult{}, nil
		},
	}

	updated, cmd := menu.updateAgentInstall(tea.KeyMsg{Type: tea.KeyEsc})
	menu = updated.(MenuModel)
	if cmd != nil {
		t.Fatal("cancel should not start command")
	}
	if menu.screen != screenMain {
		t.Fatalf("screen = %v, want main", menu.screen)
	}
	if called {
		t.Fatal("cancel wrote files")
	}
}
