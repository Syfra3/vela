package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Syfra3/vela/internal/app"
	"github.com/Syfra3/vela/internal/config"
	vquery "github.com/Syfra3/vela/internal/query"
	"github.com/Syfra3/vela/pkg/types"
)

type queryRunner interface {
	RunRequest(req types.QueryRequest) (string, error)
}

type queryFinishedMsg struct {
	output string
	err    error
}

var queryKinds = []types.QueryKind{
	types.QueryKindDependencies,
	types.QueryKindReverseDependencies,
	types.QueryKindImpact,
	types.QueryKindPath,
	types.QueryKindExplain,
}

var queryLoadEngineFunc = func(graphPath string) (queryRunner, error) {
	if strings.TrimSpace(graphPath) == "" {
		resolved, err := config.FindGraphFile(".")
		if err != nil {
			return nil, err
		}
		graphPath = resolved
	}
	return vquery.LoadFromFile(graphPath)
}

var queryRunRequestFunc = func(r queryRunner, req types.QueryRequest) (string, error) {
	return r.RunRequest(req)
}

type queryField int

const (
	queryFieldKind queryField = iota
	queryFieldSubject
	queryFieldTarget
	queryFieldLimit
)

type QueryModel struct {
	kindIndex int
	subject   string
	target    string
	limit     string
	output    string
	err       error
	running   bool
	quitting  bool
	focus     queryField
}

func NewQueryModel() QueryModel {
	return QueryModel{limit: fmt.Sprintf("%d", types.DefaultQueryLimit), focus: queryFieldSubject}
}

func (m QueryModel) Init() tea.Cmd  { return nil }
func (m QueryModel) Quitting() bool { return m.quitting }

func (m QueryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case queryFinishedMsg:
		m.running = false
		m.output = msg.output
		m.err = msg.err
		return m, nil
	case tea.KeyMsg:
		if m.running {
			if msg.String() == "esc" || msg.String() == "ctrl+c" {
				m.quitting = true
			}
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c", "esc":
			m.quitting = true
			return m, nil
		case "tab":
			m.focus = (m.focus + 1) % 4
			return m, nil
		case "shift+tab":
			if m.focus == 0 {
				m.focus = 3
			} else {
				m.focus--
			}
			return m, nil
		case "up", "k":
			if m.focus == queryFieldKind && m.kindIndex > 0 {
				m.kindIndex--
			}
			return m, nil
		case "down", "j":
			if m.focus == queryFieldKind && m.kindIndex < len(queryKinds)-1 {
				m.kindIndex++
			}
			return m, nil
		case "enter":
			req, err := m.request()
			if err != nil {
				m.err = err
				return m, nil
			}
			m.running = true
			m.err = nil
			return m, runQueryCmd(req)
		}

		switch msg.Type {
		case tea.KeyBackspace, tea.KeyDelete:
			m.deleteRune()
		case tea.KeyRunes:
			m.appendText(msg.String())
		case tea.KeySpace:
			m.appendText(" ")
		}
	}
	return m, nil
}

func runQueryCmd(req types.QueryRequest) tea.Cmd {
	return func() tea.Msg {
		graphPath, err := config.FindGraphFile(".")
		if err != nil {
			return queryFinishedMsg{err: err}
		}
		output, _, err := app.QueryService{
			LoadEngine: func(path string) (app.QueryRunner, error) {
				return queryLoadEngineFunc(path)
			},
		}.Run(app.QueryRequestInput{
			GraphPath:         graphPath,
			Kind:              req.Kind,
			Subject:           req.Subject,
			Target:            req.Target,
			Limit:             req.Limit,
			IncludeProvenance: req.IncludeProvenance,
		})
		if err == nil && output == "" {
			engine, lerr := queryLoadEngineFunc(graphPath)
			if lerr != nil {
				return queryFinishedMsg{err: lerr}
			}
			output, err = queryRunRequestFunc(engine, req)
		}
		return queryFinishedMsg{output: output, err: err}
	}
}

func (m *QueryModel) appendText(text string) {
	switch m.focus {
	case queryFieldSubject:
		m.subject += text
	case queryFieldTarget:
		m.target += text
	case queryFieldLimit:
		if text == " " {
			return
		}
		m.limit += text
	}
}

func (m *QueryModel) deleteRune() {
	trim := func(input string) string {
		if len(input) == 0 {
			return input
		}
		runes := []rune(input)
		return string(runes[:len(runes)-1])
	}
	switch m.focus {
	case queryFieldSubject:
		m.subject = trim(m.subject)
	case queryFieldTarget:
		m.target = trim(m.target)
	case queryFieldLimit:
		m.limit = trim(m.limit)
	}
}

func (m QueryModel) request() (types.QueryRequest, error) {
	req := types.QueryRequest{Kind: queryKinds[m.kindIndex], Subject: strings.TrimSpace(m.subject)}
	if strings.TrimSpace(m.limit) != "" {
		fmt.Sscanf(m.limit, "%d", &req.Limit)
	}
	req.Target = strings.TrimSpace(m.target)
	req = req.Normalize()
	return req, req.Validate()
}

func (m QueryModel) View() string { return m.ViewContent() }

func (m QueryModel) ViewContent() string {
	var b strings.Builder
	b.WriteString("Kinds\n")
	for i, kind := range queryKinds {
		cursor := "  "
		if m.focus == queryFieldKind && i == m.kindIndex {
			cursor = "▸ "
		}
		b.WriteString(fmt.Sprintf("%s%s\n", cursor, kind))
	}
	b.WriteString("\n")
	b.WriteString(renderField("subject", m.subject, m.focus == queryFieldSubject))
	b.WriteString("\n")
	b.WriteString(renderField("target", m.target, m.focus == queryFieldTarget))
	b.WriteString("\n")
	b.WriteString(renderField("limit", m.limit, m.focus == queryFieldLimit))
	b.WriteString("\n\n")
	if m.running {
		b.WriteString("Running query...\n\n")
	}
	if m.err != nil {
		b.WriteString(errorStyle.Render("Error: " + m.err.Error()))
		b.WriteString("\n\n")
	}
	if strings.TrimSpace(m.output) != "" {
		b.WriteString(styleBorder.Render(m.output))
		b.WriteString("\n\n")
	}
	b.WriteString("Supported kinds: dependencies, reverse_dependencies, impact, path, explain.\n")
	return b.String()
}

func renderField(label, value string, focused bool) string {
	prefix := "  "
	if focused {
		prefix = "▸ "
	}
	return fmt.Sprintf("%s%s: %s", prefix, label, value)
}
