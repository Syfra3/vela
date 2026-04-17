package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Syfra3/vela/internal/config"
)

// extractStep tracks which sub-screen is active inside the Extract screen.
type extractStep int

const (
	stepSource     extractStep = iota // choose source: path or ancora
	stepBrowse                        // filesystem navigator
	stepMode                          // Update vs Regenerate (only when graph.json exists)
	stepExtracting                    // extraction running
)

// ExtractSource selects where to read data from.
type ExtractSource int

const (
	SourcePath   ExtractSource = iota // browse filesystem
	SourceAncora                      // ancora SQLite snapshot
)

// ExtractionMode controls whether the cache is honoured.
type ExtractionMode int

const (
	ModeUpdate     ExtractionMode = iota // incremental — respect cache
	ModeRegenerate                       // full — ignore cache
)

// browseEntry is one row in the directory listing.
type browseEntry struct {
	name  string
	isDir bool
}

// ExtractModel is the TUI model for extraction configuration.
type ExtractModel struct {
	step   extractStep
	source ExtractSource

	// Source selection cursor
	sourceCursor int

	// Browse state
	browseDir     string
	browseEntries []browseEntry
	browseCursor  int
	browseOffset  int // scroll offset for long listings
	browseErr     error

	// Final selected directory (path source)
	selectedDir string

	// Mode selection
	modeCursor int
	mode       ExtractionMode

	quitting   bool
	starting   bool
	extracting bool
	err        error

	// Progress tracking
	progress       progress.Model
	currentFile    string
	processedFiles int
	totalFiles     int
}

const browsePageSize = 12 // rows visible in the listing

func NewExtractModel() ExtractModel {
	cwd, _ := os.Getwd()
	prog := progress.New(progress.WithDefaultGradient())
	m := ExtractModel{
		step:         stepSource,
		sourceCursor: 0,
		browseDir:    cwd,
		selectedDir:  cwd,
		modeCursor:   0,
		mode:         ModeUpdate,
		progress:     prog,
	}
	m.loadBrowseEntries()
	return m
}

// ── Public accessors ──────────────────────────────────────────────────────────

func (m ExtractModel) Init() tea.Cmd         { return nil }
func (m ExtractModel) Quitting() bool        { return m.quitting }
func (m ExtractModel) Starting() bool        { return m.starting }
func (m ExtractModel) Directory() string     { return m.selectedDir }
func (m ExtractModel) Mode() ExtractionMode  { return m.mode }
func (m ExtractModel) Source() ExtractSource { return m.source }

// ── Update ────────────────────────────────────────────────────────────────────

func (m ExtractModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.extracting {
			if msg.String() == "ctrl+c" || msg.String() == "esc" {
				m.quitting = true
			}
			return m, nil
		}
		switch m.step {
		case stepSource:
			return m.updateSource(msg)
		case stepBrowse:
			return m.updateBrowse(msg)
		case stepMode:
			return m.updateMode(msg)
		}
	}
	return m, nil
}

func (m ExtractModel) updateSource(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.quitting = true

	case "up", "k":
		if m.sourceCursor > 0 {
			m.sourceCursor--
		}
	case "down", "j":
		if m.sourceCursor < 1 {
			m.sourceCursor++
		}

	case "enter", " ":
		if m.sourceCursor == int(SourcePath) {
			m.source = SourcePath
			m.step = stepBrowse
		} else {
			m.source = SourceAncora
			// Ancora: skip browse, go straight to mode (or start immediately)
			if graphExists("") {
				m.step = stepMode
			} else {
				m.mode = ModeRegenerate
				m.starting = true
			}
		}
	}
	return m, nil
}

func (m ExtractModel) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true

	case "esc":
		// Back to source selection
		m.step = stepSource

	case "up", "k":
		if m.browseCursor > 0 {
			m.browseCursor--
			if m.browseCursor < m.browseOffset {
				m.browseOffset = m.browseCursor
			}
		}

	case "down", "j":
		if m.browseCursor < len(m.browseEntries)-1 {
			m.browseCursor++
			if m.browseCursor >= m.browseOffset+browsePageSize {
				m.browseOffset = m.browseCursor - browsePageSize + 1
			}
		}

	case "enter", " ":
		if len(m.browseEntries) == 0 {
			break
		}
		entry := m.browseEntries[m.browseCursor]
		if entry.isDir {
			// Descend into dir
			newDir := filepath.Join(m.browseDir, entry.name)
			if entry.name == ".." {
				newDir = filepath.Dir(m.browseDir)
			}
			m.browseDir = newDir
			m.browseCursor = 0
			m.browseOffset = 0
			m.loadBrowseEntries()
		} else {
			// File selected — use parent directory
			m.selectedDir = m.browseDir
			m.confirmBrowseSelection()
		}

	case "tab", "right", "l":
		// Descend if on a dir, confirm if at a leaf
		if len(m.browseEntries) == 0 {
			break
		}
		entry := m.browseEntries[m.browseCursor]
		if entry.isDir && entry.name != ".." {
			m.browseDir = filepath.Join(m.browseDir, entry.name)
			m.browseCursor = 0
			m.browseOffset = 0
			m.loadBrowseEntries()
		}

	case "backspace", "left", "h":
		// Go up one level
		parent := filepath.Dir(m.browseDir)
		if parent != m.browseDir {
			m.browseDir = parent
			m.browseCursor = 0
			m.browseOffset = 0
			m.loadBrowseEntries()
		}

	case "s", "S":
		// Select current directory without descending
		m.selectedDir = m.browseDir
		m.confirmBrowseSelection()
	}
	return m, nil
}

// confirmBrowseSelection transitions after a directory is chosen.
func (m *ExtractModel) confirmBrowseSelection() {
	if graphExists("") {
		m.step = stepMode
	} else {
		m.mode = ModeRegenerate
		m.starting = true
	}
}

// loadBrowseEntries reads the current browseDir and populates browseEntries.
func (m *ExtractModel) loadBrowseEntries() {
	m.browseErr = nil
	entries, err := os.ReadDir(m.browseDir)
	if err != nil {
		m.browseErr = err
		m.browseEntries = nil
		return
	}

	var result []browseEntry
	// Always offer ".." unless at filesystem root
	if filepath.Dir(m.browseDir) != m.browseDir {
		result = append(result, browseEntry{name: "..", isDir: true})
	}

	// Directories first, then files — both sorted alphabetically.
	var dirs, files []browseEntry
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue // skip hidden
		}
		if e.IsDir() {
			dirs = append(dirs, browseEntry{name: name, isDir: true})
		} else {
			files = append(files, browseEntry{name: name, isDir: false})
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name })
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })

	result = append(result, dirs...)
	result = append(result, files...)
	m.browseEntries = result

	// Keep cursor in bounds after reload.
	if m.browseCursor >= len(m.browseEntries) {
		m.browseCursor = len(m.browseEntries) - 1
	}
	if m.browseCursor < 0 {
		m.browseCursor = 0
	}
}

func (m ExtractModel) updateMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		// Back to browse (or source if ancora)
		if m.source == SourceAncora {
			m.step = stepSource
		} else {
			m.step = stepBrowse
		}

	case "up", "k":
		if m.modeCursor > 0 {
			m.modeCursor--
		}
	case "down", "j":
		if m.modeCursor < 1 {
			m.modeCursor++
		}

	case "enter", " ":
		if m.modeCursor == 0 {
			m.mode = ModeUpdate
		} else {
			m.mode = ModeRegenerate
		}
		m.starting = true
	}
	return m, nil
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m ExtractModel) View() string { return m.ViewContent() }

func (m ExtractModel) ViewContent() string {
	var b strings.Builder

	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	accentStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(colorSubtext)
	selectedStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	normalStyle := lipgloss.NewStyle().Foreground(colorText)
	dimStyle := lipgloss.NewStyle().Foreground(colorMuted)

	switch m.step {

	// ── Source selection ─────────────────────────────────────────────────
	case stepSource:
		b.WriteString(textStyle.Render("Choose extraction source:"))
		b.WriteString("\n\n")

		sources := []struct {
			label string
			desc  string
		}{
			{"From path", "Browse filesystem and extract a codebase or documents"},
			{"From Ancora", "Snapshot your memory — observations, decisions, lessons learned"},
		}
		for i, s := range sources {
			cursor := "  "
			ls := normalStyle
			if i == m.sourceCursor {
				cursor = lipgloss.NewStyle().Foreground(colorAccent).Render("▸ ")
				ls = selectedStyle
			}
			b.WriteString(fmt.Sprintf("%s%s\n", cursor, ls.Render(s.label)))
			b.WriteString(fmt.Sprintf("    %s\n\n", mutedStyle.Render(s.desc)))
		}

	// ── Filesystem browser ───────────────────────────────────────────────
	case stepBrowse:
		// Current path bar
		b.WriteString(accentStyle.Render("Directory: "))
		b.WriteString(textStyle.Render(m.browseDir))
		b.WriteString("\n\n")

		if m.browseErr != nil {
			b.WriteString(errorStyle.Render(fmt.Sprintf("Error reading dir: %v", m.browseErr)))
			b.WriteString("\n")
			break
		}

		if len(m.browseEntries) == 0 {
			b.WriteString(mutedStyle.Render("(empty directory)"))
			b.WriteString("\n")
			break
		}

		// Visible window
		end := m.browseOffset + browsePageSize
		if end > len(m.browseEntries) {
			end = len(m.browseEntries)
		}
		visible := m.browseEntries[m.browseOffset:end]

		for i, entry := range visible {
			absIdx := m.browseOffset + i
			cursor := "  "
			nameStyle := normalStyle
			icon := "  "

			if entry.isDir {
				icon = "📁"
				nameStyle = accentStyle
			}
			if entry.name == ".." {
				icon = "↑ "
				nameStyle = mutedStyle
			}
			if absIdx == m.browseCursor {
				cursor = lipgloss.NewStyle().Foreground(colorAccent).Render("▸ ")
				if !entry.isDir {
					nameStyle = selectedStyle
				}
			}
			b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, icon, nameStyle.Render(entry.name)))
		}

		// Scroll hint
		if len(m.browseEntries) > browsePageSize {
			shown := fmt.Sprintf("%d–%d of %d", m.browseOffset+1, end, len(m.browseEntries))
			b.WriteString("\n")
			b.WriteString(dimStyle.Render(shown))
			b.WriteString("\n")
		}

		b.WriteString("\n")
		b.WriteString(dimStyle.Render("[s] select this dir  [Enter/→] descend  [←/backspace] up  [esc] back"))
		b.WriteString("\n")

	// ── Mode selection ───────────────────────────────────────────────────
	case stepMode:
		srcLabel := m.selectedDir
		if m.source == SourceAncora {
			srcLabel = "Ancora memory (~/.ancora/ancora.db)"
		}
		b.WriteString(accentStyle.Render("Previous extraction found."))
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render(fmt.Sprintf("Source: %s", srcLabel)))
		b.WriteString("\n\n")
		b.WriteString(textStyle.Render("Choose extraction mode:"))
		b.WriteString("\n\n")

		modeOptions := []struct {
			label string
			desc  string
		}{
			{"Update", "Incremental — only re-extract changed observations (fast)"},
			{"Regenerate", "Full re-extraction — rebuild from scratch"},
		}
		for i, opt := range modeOptions {
			cursor := "  "
			ls := normalStyle
			if i == m.modeCursor {
				cursor = lipgloss.NewStyle().Foreground(colorAccent).Render("▸ ")
				ls = selectedStyle
			}
			b.WriteString(fmt.Sprintf("%s%s\n", cursor, ls.Render(opt.label)))
			b.WriteString(fmt.Sprintf("    %s\n\n", mutedStyle.Render(opt.desc)))
		}

	// ── Extracting ───────────────────────────────────────────────────────
	case stepExtracting:
		srcLabel := m.selectedDir
		if m.source == SourceAncora {
			srcLabel = "Ancora memory"
		}
		b.WriteString(textStyle.Render(fmt.Sprintf("Extracting knowledge graph from %s...", srcLabel)))
		b.WriteString("\n\n")

		if m.totalFiles > 0 {
			percent := float64(m.processedFiles) / float64(m.totalFiles)
			b.WriteString(m.progress.ViewAs(percent))
			b.WriteString("\n\n")
			b.WriteString(textStyle.Render(fmt.Sprintf(
				"Progress: %d/%d  (%.0f%%)",
				m.processedFiles, m.totalFiles, percent*100,
			)))
			b.WriteString("\n")
			if m.currentFile != "" {
				b.WriteString(mutedStyle.Render(fmt.Sprintf("Current: %s", m.currentFile)))
			}
		} else {
			b.WriteString(textStyle.Render("Discovering observations..."))
		}
		b.WriteString("\n\n")
	}

	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		b.WriteString("\n\n")
	}

	return b.String()
}

// FooterHelp returns context-appropriate footer text for the current step.
func (m ExtractModel) FooterHelp() string {
	switch m.step {
	case stepSource:
		return "↑↓ select • Enter confirm • esc back to menu"
	case stepBrowse:
		return "↑↓ navigate • s select dir • → descend • ← up • esc back"
	case stepMode:
		return "↑↓ select mode • Enter confirm • esc back"
	default:
		return "esc cancel"
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// graphExists returns true when the global graph.json is present and non-empty.
func graphExists(_ string) bool {
	p, err := config.FindGraphFile(".")
	if err != nil {
		return false
	}
	info, err := os.Stat(p)
	if err != nil {
		return false
	}
	return info.Size() > 0
}
