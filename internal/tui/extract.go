package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Syfra3/vela/internal/app"
	"github.com/Syfra3/vela/internal/config"
	"github.com/Syfra3/vela/pkg/types"
)

type BuildRunRequest struct {
	RepoRoot string
	Observe  func(app.BuildEvent)
}

type BuildStageSummary struct {
	Name  string
	Count int
}

type BuildRunResult struct {
	GraphPath    string
	HTMLPath     string
	ObsidianPath string
	Files        int
	Facts        int
	Stages       []BuildStageSummary
	Warnings     []string
}

type buildFinishedMsg struct {
	result BuildRunResult
	err    error
}

type buildEventMsg struct{ event app.BuildEvent }

type extractTickMsg struct{}

type dirEntry struct {
	Name  string
	Path  string
	IsDir bool
}

type extractPhase int

const (
	extractPhaseBrowse extractPhase = iota
	extractPhaseConfirm
	extractPhaseRunning
	extractPhaseResult
)

var readDirEntries = func(root string) ([]dirEntry, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	result := make([]dirEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, dirEntry{Name: entry.Name(), Path: filepath.Join(root, entry.Name()), IsDir: entry.IsDir()})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDir != result[j].IsDir {
			return result[i].IsDir
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result, nil
}

var runTUIBuild = func(req BuildRunRequest) (BuildRunResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return BuildRunResult{}, err
	}
	result, err := app.BuildService{}.Run(context.Background(), app.BuildRequest{RepoRoot: req.RepoRoot, Obsidian: cfg.Obsidian, Observe: req.Observe})
	if err != nil {
		return BuildRunResult{}, err
	}
	stages := make([]BuildStageSummary, 0, len(result.StageReports))
	for _, stage := range result.StageReports {
		stages = append(stages, BuildStageSummary{Name: string(stage.Stage), Count: stage.Count})
	}
	return BuildRunResult{
		GraphPath:    result.GraphPath,
		HTMLPath:     result.HTMLPath,
		ObsidianPath: result.ObsidianPath,
		Files:        result.Files,
		Facts:        result.Facts,
		Stages:       stages,
		Warnings:     append([]string(nil), result.Warnings...),
	}, nil
}

type ExtractModel struct {
	currentDir string
	entries    []dirEntry
	cursor     int
	selected   string
	phase      extractPhase
	running    bool
	result     BuildRunResult
	err        error
	quitting   bool
	events     []app.BuildEvent
	startedAt  time.Time
	totalFiles int
	stage      types.BuildStage
	stageCount int
	eventCh    <-chan app.BuildEvent
	doneCh     <-chan buildFinishedMsg
}

func NewExtractModel() ExtractModel {
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}
	return NewExtractModelWithRoot(wd)
}

func NewExtractModelWithRoot(root string) ExtractModel {
	m := ExtractModel{currentDir: root, phase: extractPhaseBrowse}
	m.reloadEntries()
	return m
}

func (m ExtractModel) Init() tea.Cmd  { return nil }
func (m ExtractModel) Quitting() bool { return m.quitting }

func (m ExtractModel) StatusMessage() string {
	if m.err != nil {
		return "build failed: " + m.err.Error()
	}
	if m.result.GraphPath != "" {
		return "build complete: " + m.result.GraphPath
	}
	return ""
}

func (m ExtractModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case buildEventMsg:
		m.events = append(m.events, msg.event)
		m.applyBuildEvent(msg.event)
		if m.running && m.eventCh != nil {
			return m, waitForBuildEvent(m.eventCh)
		}
		return m, nil
	case extractTickMsg:
		if m.running {
			return m, tickExtractStatus()
		}
		return m, nil
	case buildFinishedMsg:
		m.running = false
		m.err = msg.err
		m.result = msg.result
		m.phase = extractPhaseResult
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, nil
		}
	}

	switch m.phase {
	case extractPhaseBrowse:
		return m.updateBrowse(msg)
	case extractPhaseConfirm:
		return m.updateConfirm(msg)
	case extractPhaseRunning:
		return m.updateRunning(msg)
	case extractPhaseResult:
		return m.updateResult(msg)
	default:
		return m, nil
	}
}

func (m ExtractModel) updateBrowse(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc", "q":
		m.quitting = true
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.entries)-1 {
			m.cursor++
		}
	case "backspace", "h", "left":
		parent := filepath.Dir(m.currentDir)
		if parent != m.currentDir {
			m.currentDir = parent
			m.cursor = 0
			m.reloadEntries()
		}
	case "enter", "l", "right":
		if len(m.entries) == 0 {
			return m, nil
		}
		entry := m.entries[m.cursor]
		if !entry.IsDir {
			return m, nil
		}
		m.currentDir = entry.Path
		m.cursor = 0
		m.reloadEntries()
	case "s", " ":
		m.selected = m.currentDir
		m.phase = extractPhaseConfirm
	}
	return m, nil
}

func (m ExtractModel) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc", "backspace":
		m.phase = extractPhaseBrowse
		return m, nil
	case "enter":
		m.running = true
		m.phase = extractPhaseRunning
		m.err = nil
		m.events = nil
		m.startedAt = time.Now()
		m.totalFiles = 0
		m.stage = ""
		m.stageCount = 0
		events := make(chan app.BuildEvent, 32)
		done := make(chan buildFinishedMsg, 1)
		m.eventCh = events
		m.doneCh = done
		startBuild(runTUIBuild, m.selected, events, done)
		return m, tea.Batch(waitForBuildEvent(m.eventCh), waitForBuildDone(m.doneCh), tickExtractStatus())
	}
	return m, nil
}

func tickExtractStatus() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
		return extractTickMsg{}
	})
}

func (m ExtractModel) updateRunning(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if ok {
		switch key.String() {
		case "esc", "q":
			m.quitting = true
			return m, nil
		}
	}
	return m, nil
}

func (m ExtractModel) updateResult(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc", "q":
		m.quitting = true
	case "b":
		m.phase = extractPhaseBrowse
		m.err = nil
		m.result = BuildRunResult{}
	}
	return m, nil
}

func startBuild(runner func(BuildRunRequest) (BuildRunResult, error), selected string, events chan app.BuildEvent, done chan buildFinishedMsg) {
	go func() {
		result, err := runner(BuildRunRequest{RepoRoot: selected, Observe: func(event app.BuildEvent) { events <- event }})
		close(events)
		done <- buildFinishedMsg{result: result, err: err}
		close(done)
	}()
}

func waitForBuildEvent(ch <-chan app.BuildEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			return nil
		}
		return buildEventMsg{event: event}
	}
}

func waitForBuildDone(ch <-chan buildFinishedMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

func (m *ExtractModel) reloadEntries() {
	entries, err := readDirEntries(m.currentDir)
	m.entries = entries
	m.err = err
	if m.cursor >= len(m.entries) {
		m.cursor = 0
	}
}

func (m ExtractModel) View() string { return m.ViewContent() }

var buildStageOrder = []types.BuildStage{
	types.BuildStageDetect,
	types.BuildStageScan,
	types.BuildStageDrivers,
	types.BuildStagePatch,
	types.BuildStageMerge,
	types.BuildStagePersist,
}

func buildStageIndex(stage types.BuildStage) int {
	for i, candidate := range buildStageOrder {
		if stage == candidate {
			return i + 1
		}
	}
	return 0
}

func (m *ExtractModel) applyBuildEvent(event app.BuildEvent) {
	if event.Kind != app.BuildEventStage {
		return
	}
	m.stage = event.Stage
	m.stageCount = event.Count
	if event.Stage == types.BuildStageDetect {
		m.totalFiles = event.Count
	}
}

func (m ExtractModel) renderRunningProgress() string {
	progress := types.ExtractionProgress{
		TotalFiles:      m.totalFiles,
		ProcessedFiles:  buildStageIndex(m.stage),
		TotalChunks:     len(buildStageOrder),
		ProcessedChunks: buildStageIndex(m.stage),
		CurrentFile:     string(m.stage),
		StartTime:       m.startedAt,
	}
	if progress.StartTime.IsZero() {
		progress.StartTime = time.Now()
	}

	var b strings.Builder
	b.WriteString(RenderProgress(progress, 64))
	b.WriteString("\n\n")
	if m.totalFiles > 0 {
		b.WriteString(fmt.Sprintf("Files discovered: %d\n", m.totalFiles))
	}
	if m.stage != "" {
		b.WriteString(fmt.Sprintf("Current stage: %s", m.stage))
		if m.stageCount > 0 {
			b.WriteString(fmt.Sprintf(" (%d)", m.stageCount))
		}
		b.WriteString("\n")
	}
	if !m.startedAt.IsZero() {
		spinner := []string{"|", "/", "-", "\\"}
		frame := int(time.Since(m.startedAt)/(250*time.Millisecond)) % len(spinner)
		b.WriteString(fmt.Sprintf("Activity: %s working...\n", spinner[frame]))
	}
	if len(m.events) > 0 {
		b.WriteString("\nRecent events:\n")
		start := 0
		if len(m.events) > 4 {
			start = len(m.events) - 4
		}
		for _, event := range m.events[start:] {
			if event.Message == "" {
				continue
			}
			b.WriteString(fmt.Sprintf("- %s\n", event.Message))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m ExtractModel) ViewContent() string {
	var b strings.Builder
	switch m.phase {
	case extractPhaseBrowse:
		b.WriteString("Browse folders\n")
		b.WriteString("Current folder\n")
		b.WriteString(stylePrompt.Render(m.currentDir))
		b.WriteString("\n\n")
		for i, entry := range m.entries {
			cursor := "  "
			if i == m.cursor {
				cursor = "▸ "
			}
			name := entry.Name
			if entry.IsDir {
				name += "/"
			}
			b.WriteString(cursor + name + "\n")
		}
		b.WriteString("\nenter open folder • s select current folder • backspace parent\n")
	case extractPhaseConfirm:
		b.WriteString("Confirm extraction\n\n")
		b.WriteString("Selected folder:\n")
		b.WriteString(stylePrompt.Render(m.selected))
		b.WriteString("\n\nThis keeps the new backend but restores the classic guided flow.\n")
	case extractPhaseRunning:
		b.WriteString("Running extraction\n\n")
		b.WriteString(m.renderRunningProgress())
	case extractPhaseResult:
		if m.err != nil {
			b.WriteString(errorStyle.Render("Error: " + m.err.Error()))
			b.WriteString("\n")
			return b.String()
		}
		b.WriteString("Build summary\n\n")
		b.WriteString(RenderBuildSummary(m.result))
		if len(m.result.Warnings) > 0 {
			b.WriteString("\n\nWarnings:\n")
			for _, warning := range m.result.Warnings {
				b.WriteString("- " + warning + "\n")
			}
		}
	}
	return b.String()
}
