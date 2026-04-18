package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Syfra3/vela/internal/config"
	vdoctor "github.com/Syfra3/vela/internal/doctor"
	"github.com/Syfra3/vela/internal/llm"
	"github.com/Syfra3/vela/pkg/types"
)

type healthCheckMsg struct {
	provider string
	ok       bool
	err      error
}

type doctorReportMsg struct {
	llmResults        []healthCheckMsg
	integrationChecks []vdoctor.Step
}

// DoctorModel is the TUI model for health check.
type DoctorModel struct {
	checking    bool
	results     []healthCheckMsg
	integration []vdoctor.Step
	quitting    bool
	cfg         *types.Config
}

func NewDoctorModel() DoctorModel {
	cfg, _ := config.Load()
	return DoctorModel{
		checking: true,
		cfg:      cfg,
	}
}

func (m DoctorModel) Init() tea.Cmd {
	return m.runHealthCheck()
}

func (m DoctorModel) Quitting() bool {
	return m.quitting
}

func (m DoctorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.quitting = true
			return m, nil

		case "r":
			// Re-run health check
			m.results = []healthCheckMsg{}
			m.checking = true
			return m, m.runHealthCheck()
		}

	case doctorReportMsg:
		m.results = msg.llmResults
		m.integration = msg.integrationChecks
		m.checking = false
		return m, nil
	}
	return m, nil
}

func (m DoctorModel) View() string {
	// Content only — menu will wrap with header/footer
	return m.ViewContent()
}

func (m DoctorModel) ViewContent() string {
	var b strings.Builder

	textStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	successStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("42"))

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196"))

	if m.checking {
		b.WriteString(textStyle.Render("Checking LLM providers..."))
		b.WriteString("\n\n")
	}

	for _, result := range m.results {
		status := "✓"
		statusStyle := successStyle
		msg := "OK"

		if !result.ok {
			status = "✗"
			statusStyle = errorStyle
			msg = fmt.Sprintf("FAIL: %v", result.err)
		}

		b.WriteString(fmt.Sprintf("%s %s: %s\n",
			statusStyle.Render(status),
			textStyle.Bold(true).Render(result.provider),
			textStyle.Render(msg)))
	}

	if len(m.integration) > 0 {
		if len(m.results) > 0 {
			b.WriteString("\n")
		}
		b.WriteString(textStyle.Bold(true).Render("Integration Path"))
		b.WriteString("\n")
		for _, step := range m.integration {
			status := "✓"
			statusStyle := successStyle
			if step.Status == vdoctor.StepWarn {
				status = "!"
				statusStyle = textStyle
			}
			if step.Status == vdoctor.StepFail {
				status = "✗"
				statusStyle = errorStyle
			}
			b.WriteString(fmt.Sprintf("%s %s: %s\n",
				statusStyle.Render(status),
				textStyle.Bold(true).Render(step.Name),
				textStyle.Render(step.Detail)))
		}
	}

	return b.String()
}

func (m DoctorModel) runHealthCheck() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Check all providers
		providers := []string{"local", "anthropic", "openai"}
		results := make(chan healthCheckMsg, len(providers))

		for _, p := range providers {
			go func(provider string) {
				cfg := &types.LLMConfig{
					Provider: provider,
					Timeout:  10 * time.Second,
				}

				// Copy settings from main config if available
				if m.cfg != nil {
					if provider == "local" {
						cfg.Endpoint = m.cfg.LLM.Endpoint
						cfg.Model = m.cfg.LLM.Model
					} else {
						cfg.APIKey = m.cfg.LLM.APIKey
					}
				}

				client, err := llm.NewClient(cfg)
				if err != nil {
					results <- healthCheckMsg{provider: provider, ok: false, err: err}
					return
				}

				err = client.Health(ctx)
				results <- healthCheckMsg{provider: provider, ok: err == nil, err: err}
			}(p)
		}

		collected := make([]healthCheckMsg, 0, len(providers))
		for range providers {
			select {
			case result := <-results:
				collected = append(collected, result)
			case <-ctx.Done():
				collected = append(collected, healthCheckMsg{provider: "unknown", ok: false, err: ctx.Err()})
				return doctorReportMsg{llmResults: collected, integrationChecks: vdoctor.IntegrationChecks(m.cfg, "")}
			}
		}

		return doctorReportMsg{llmResults: collected, integrationChecks: vdoctor.IntegrationChecks(m.cfg, "")}
	}
}
