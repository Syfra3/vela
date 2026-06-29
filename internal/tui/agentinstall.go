package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Syfra3/vela/internal/agentinstall"
)

type AgentInstallTarget = agentinstall.Target
type AgentInstallRequest = agentinstall.Request
type AgentInstallResult = agentinstall.Result

type agentInstallStep int

const (
	agentInstallStepTargets agentInstallStep = iota
	agentInstallStepPreview
	agentInstallStepResult
)

type AgentInstallModel struct {
	step       agentInstallStep
	cursor     int
	projectDir string
	targets    []AgentInstallTarget
	selected   AgentInstallTarget
	message    string
	result     AgentInstallResult
	err        error
	runInstall func(AgentInstallRequest) (AgentInstallResult, error)
	quitting   bool
}

type agentInstallDoneMsg struct {
	result AgentInstallResult
	err    error
}

var detectAgentInstallTargetsFunc = agentinstall.DetectTargets
var runAgentInstallFunc = agentinstall.Install

func NewAgentInstallModel() AgentInstallModel {
	projectDir, err := os.Getwd()
	if err != nil || strings.TrimSpace(projectDir) == "" {
		projectDir = "."
	}
	targets := detectAgentInstallTargetsFunc()
	if len(targets) == 0 {
		home, _ := os.UserHomeDir()
		targets = []AgentInstallTarget{
			{Name: "OpenCode", Agent: "opencode", ConfigDir: filepath.Join(home, ".config", "opencode"), Supported: true},
			{Name: "Claude Code", Agent: "claude", ConfigDir: filepath.Join(home, ".claude"), Supported: true},
		}
	}
	return AgentInstallModel{projectDir: projectDir, targets: targets, runInstall: runAgentInstallFunc}
}

func (m AgentInstallModel) Init() tea.Cmd { return nil }

func (m AgentInstallModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.runInstall == nil {
		m.runInstall = runAgentInstallFunc
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			m.quitting = true
			return m, nil
		case "up", "k":
			if m.step == agentInstallStepTargets && m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.step == agentInstallStepTargets && m.cursor < len(m.targets)-1 {
				m.cursor++
			}
		case "enter", " ":
			return m.handleEnter()
		}
	case agentInstallDoneMsg:
		m.step = agentInstallStepResult
		m.result = msg.result
		m.err = msg.err
	}
	return m, nil
}

func (m AgentInstallModel) handleEnter() (tea.Model, tea.Cmd) {
	switch m.step {
	case agentInstallStepTargets:
		if len(m.targets) == 0 {
			m.message = "No coding-agent targets are configured yet. Use `vela install --agent opencode|claude` with an explicit config directory."
			return m, nil
		}
		target := m.targets[m.cursor]
		if !target.Supported {
			m.message = target.Name + " is guidance-only and not installable by this wizard yet."
			return m, nil
		}
		m.selected = target
		m.step = agentInstallStepPreview
		m.message = ""
		return m, nil
	case agentInstallStepPreview:
		req := AgentInstallRequest{ProjectDir: m.projectDir, Agent: m.selected.Agent, ConfigDir: m.selected.ConfigDir}
		run := m.runInstall
		return m, func() tea.Msg {
			result, err := run(req)
			return agentInstallDoneMsg{result: result, err: err}
		}
	case agentInstallStepResult:
		m.quitting = true
	}
	return m, nil
}

func (m AgentInstallModel) View() string { return m.ViewContent() }

func (m AgentInstallModel) ViewContent() string {
	var b strings.Builder
	b.WriteString("Agent Integration Installer\n")
	b.WriteString("configure explore for coding agents through Vela MCP\n\n")
	if strings.TrimSpace(m.message) != "" {
		b.WriteString(m.message)
		b.WriteString("\n\n")
	}
	switch m.step {
	case agentInstallStepTargets:
		b.WriteString("Select coding agent target:\n")
		for i, target := range m.targets {
			cursor := "  "
			if i == m.cursor {
				cursor = "▸ "
			}
			state := "guidance-only"
			if target.Supported {
				state = "installable"
			}
			b.WriteString(fmt.Sprintf("%s%s — %s — %s\n", cursor, target.Name, state, target.ConfigDir))
		}
	case agentInstallStepPreview:
		preview := agentinstall.Preview(AgentInstallRequest{ProjectDir: m.projectDir, Agent: m.selected.Agent, ConfigDir: m.selected.ConfigDir})
		b.WriteString("Preview\n")
		b.WriteString(fmt.Sprintf("Project: %s\n", m.projectDir))
		b.WriteString(fmt.Sprintf("Agent: %s\n", m.selected.Name))
		b.WriteString(fmt.Sprintf("MCP config: %s\n", preview.MCPConfigPath))
		b.WriteString(fmt.Sprintf("Instructions: %s\n", preview.InstructionPath))
		b.WriteString(fmt.Sprintf("Graph DB: %s\n", preview.GraphDBPath))
		b.WriteString("unrelated config is preserved; reruns are idempotent\n")
	case agentInstallStepResult:
		if m.err != nil {
			b.WriteString("Installation failed: ")
			b.WriteString(m.err.Error())
			b.WriteString("\n")
			break
		}
		b.WriteString("Installation complete\n")
		b.WriteString("MCP config written: ")
		b.WriteString(m.result.MCPConfigPath)
		b.WriteString("\n")
		b.WriteString("instructions mention explore first: ")
		b.WriteString(m.result.InstructionPath)
		b.WriteString("\n")
		if m.result.GraphReady {
			b.WriteString(".vela/graph.db is present: ")
			b.WriteString(m.result.GraphDBPath)
			b.WriteString("\n")
		} else {
			b.WriteString("Graph DB needs attention: run vela build, vela update, or vela status\n")
		}
		if m.result.Idempotent {
			b.WriteString("Result is idempotent; reruns update Vela-managed files without duplicates.\n")
		}
		b.WriteString("Try asking your coding agent: How does Vela resolve graph search queries?\n")
	}
	if m.step == agentInstallStepResult {
		b.WriteString("\nenter/esc back to menu\n")
	} else {
		b.WriteString("\n↑/↓ target • enter continue/confirm • esc cancel\n")
	}
	return b.String()
}

func (m AgentInstallModel) Quitting() bool { return m.quitting }
