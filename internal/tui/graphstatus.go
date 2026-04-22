package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Syfra3/vela/internal/config"
	igraph "github.com/Syfra3/vela/internal/graph"
)

// ---------------------------------------------------------------------------
// TUI-specific vault stats (not part of HealthMetrics)
// ---------------------------------------------------------------------------

type vaultStats struct {
	Notes   int
	Links   int
	Broken  int
	Healthy bool
	Path    string
}

// ---------------------------------------------------------------------------
// GraphStatusModel — the TUI screen
// ---------------------------------------------------------------------------

type graphStatusLoadedMsg struct {
	metrics igraph.HealthMetrics
	vault   vaultStats
	loadErr error
}

type GraphStatusModel struct {
	graphPath    string
	metrics      igraph.HealthMetrics
	vault        vaultStats
	loadErr      error
	loading      bool
	quitting     bool
	termWidth    int
	termHeight   int
	scrollOffset int
}

func NewGraphStatusModel() GraphStatusModel {
	return GraphStatusModel{loading: true, termWidth: 100, termHeight: 24}
}

func NewGraphStatusModelWithGraphPath(graphPath string) GraphStatusModel {
	return GraphStatusModel{graphPath: graphPath, loading: true, termWidth: 100, termHeight: 24}
}

func (m GraphStatusModel) Quitting() bool { return m.quitting }

func (m GraphStatusModel) Init() tea.Cmd {
	return loadMetricsCmd(m.graphPath)
}

func loadMetricsCmd(graphPath string) tea.Cmd {
	return func() tea.Msg {
		resolvedGraphPath := graphPath
		if strings.TrimSpace(resolvedGraphPath) == "" {
			var err error
			resolvedGraphPath, err = config.FindGraphFile(".")
			if err != nil {
				return graphStatusLoadedMsg{loadErr: err}
			}
		}
		mx, loadErr := igraph.LoadHealthMetrics(resolvedGraphPath, 5)
		cfg, cfgErr := config.Load()
		if cfgErr != nil {
			return graphStatusLoadedMsg{metrics: mx, loadErr: cfgErr}
		}
		vs := loadVaultStats(filepath.Join(config.ResolveVaultDir(cfg.Obsidian.VaultDir), "obsidian"))
		return graphStatusLoadedMsg{metrics: mx, vault: vs, loadErr: loadErr}
	}
}

func loadVaultStats(vaultPath string) vaultStats {
	vs := vaultStats{Path: vaultPath}
	entries, err := os.ReadDir(vaultPath)
	if err != nil {
		return vs
	}
	re := regexp.MustCompile(`\[\[(.+?)\]\]`)
	mdFiles := make(map[string]bool)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			mdFiles[strings.TrimSuffix(e.Name(), ".md")] = true
		}
	}
	vs.Notes = len(mdFiles)
	for name := range mdFiles {
		content, rerr := os.ReadFile(filepath.Join(vaultPath, name+".md"))
		if rerr != nil {
			continue
		}
		for _, match := range re.FindAllSubmatch(content, -1) {
			vs.Links++
			if !mdFiles[string(match[1])] {
				vs.Broken++
			}
		}
	}
	vs.Healthy = vs.Broken == 0 && vs.Notes > 0
	return vs
}

func (m GraphStatusModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case graphStatusLoadedMsg:
		m.metrics = msg.metrics
		m.vault = msg.vault
		m.loadErr = msg.loadErr
		m.loading = false
		m.clampScroll()
		return m, nil
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height
		m.clampScroll()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.quitting = true
		case "r":
			m.loading = true
			m.scrollOffset = 0
			return m, loadMetricsCmd(m.graphPath)
		case "up", "k":
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
		case "down", "j":
			if m.scrollOffset < m.maxScrollOffset() {
				m.scrollOffset++
			}
		case "pgup", "b", "ctrl+u":
			m.scrollOffset -= m.viewportHeight()
			m.clampScroll()
		case "pgdown", "f", "ctrl+d":
			m.scrollOffset += m.viewportHeight()
			m.clampScroll()
		}
	}
	return m, nil
}

func (m GraphStatusModel) ViewContent() string {
	content := m.renderContent()
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return ""
	}
	viewportHeight := m.viewportHeight()
	if viewportHeight <= 0 || len(lines) <= viewportHeight {
		return content
	}
	start := m.scrollOffset
	if start < 0 {
		start = 0
	}
	maxStart := len(lines) - viewportHeight
	if start > maxStart {
		start = maxStart
	}
	end := start + viewportHeight
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[start:end], "\n")
}

func (m GraphStatusModel) renderContent() string {
	var b strings.Builder

	text := lipgloss.NewStyle().Foreground(colorText)
	dim := lipgloss.NewStyle().Foreground(colorSubtext)
	ok := lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
	warn := lipgloss.NewStyle().Foreground(colorWarn).Bold(true)
	errS := lipgloss.NewStyle().Foreground(colorErr).Bold(true)
	accent := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	label := lipgloss.NewStyle().Foreground(colorSubtext).Width(22)

	if m.loading {
		b.WriteString(dim.Render("Loading metrics..."))
		return b.String()
	}

	if m.loadErr != nil {
		b.WriteString(errS.Render("✗ " + m.loadErr.Error()))
		return b.String()
	}

	mx := m.metrics
	vs := m.vault

	// ── Graph section ────────────────────────────────────────────────────────
	b.WriteString(accent.Render("Graph"))
	b.WriteString("\n\n")

	if mx.GeneratedAt != "" {
		t, err := time.Parse(time.RFC3339, mx.GeneratedAt)
		if err == nil {
			b.WriteString(fmt.Sprintf("  %s %s\n\n",
				label.Render("Generated"),
				dim.Render(t.Format("2006-01-02 15:04 UTC")),
			))
		}
	}

	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Nodes"), text.Render(fmt.Sprintf("%d", mx.Nodes))))
	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Edges"), text.Render(fmt.Sprintf("%d", mx.Edges))))

	brokenEdgeS := ok.Render("0 ✓")
	if mx.BrokenEdges > 0 {
		brokenEdgeS = errS.Render(fmt.Sprintf("%d ✗", mx.BrokenEdges))
	}
	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Broken edges"), brokenEdgeS))
	b.WriteString("\n")

	// ── Coverage ─────────────────────────────────────────────────────────────
	b.WriteString(accent.Render("Coverage"))
	b.WriteString("\n\n")

	// Connected bar
	connColor := colorSuccess
	if mx.ConnectedPct < 50 {
		connColor = colorWarn
	}
	connBar := bar(mx.ConnectedPct, 24)
	connLabel := lipgloss.NewStyle().Foreground(connColor).Bold(true).
		Render(fmt.Sprintf("%d%%", mx.ConnectedPct))
	b.WriteString(fmt.Sprintf("  %s %s %s\n", label.Render("Connected nodes"), connBar, connLabel))
	b.WriteString(fmt.Sprintf("  %s %s   %s %s\n",
		label.Render("Hub nodes (≥10 edges)"),
		text.Render(fmt.Sprintf("%d", mx.HubNodes)),
		dim.Render("leaf (1 edge)"),
		dim.Render(fmt.Sprintf("%d", mx.LeafNodes)),
	))
	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Isolated nodes"), dim.Render(fmt.Sprintf("%d", mx.IsolatedNodes))))
	b.WriteString("\n")

	// ── Quality ───────────────────────────────────────────────────────────────
	b.WriteString(accent.Render("Quality"))
	b.WriteString("\n\n")

	// Resolution rate bar
	resRate := int(mx.ResolutionRate * 100)
	resColor := colorSuccess
	if resRate < 85 {
		resColor = colorWarn
	}
	if resRate < 60 {
		resColor = colorErr
	}
	resBar := bar(resRate, 24)
	resLabel := lipgloss.NewStyle().Foreground(resColor).Bold(true).Render(fmt.Sprintf("%d%%", resRate))
	b.WriteString(fmt.Sprintf("  %s %s %s\n", label.Render("Resolution rate"), resBar, resLabel))

	// Confidence distribution
	total := mx.Edges - mx.BrokenEdges
	if total > 0 {
		extracted := mx.ConfidenceDist["EXTRACTED"]
		inferred := mx.ConfidenceDist["INFERRED"]
		ambiguous := mx.ConfidenceDist["AMBIGUOUS"]
		extRate := int(mx.ExtractedRate * 100)

		extColor := colorSuccess
		if extRate < 50 {
			extColor = colorWarn
		}
		extBar := bar(extRate, 24)
		extLabel := lipgloss.NewStyle().Foreground(extColor).Bold(true).Render(fmt.Sprintf("%d%%", extRate))
		b.WriteString(fmt.Sprintf("  %s %s %s\n", label.Render("EXTRACTED rate"), extBar, extLabel))

		if inferred > 0 || ambiguous > 0 {
			b.WriteString(fmt.Sprintf("  %s EXTRACTED:%d  INFERRED:%d  AMBIGUOUS:%d\n",
				label.Render(""),
				extracted, inferred, ambiguous,
			))
		}
	}

	// Degree stats
	b.WriteString(fmt.Sprintf("  %s avg:%.1f  median:%d  p95:%d  max:%d\n",
		label.Render("Degree stats"),
		mx.AvgDegree, mx.MedianDegree, mx.P95Degree, mx.MaxDegree,
	))
	b.WriteString("\n")

	// ── Communities ───────────────────────────────────────────────────────────
	b.WriteString(accent.Render("Communities"))
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Communities"), text.Render(fmt.Sprintf("%d", mx.Communities))))
	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Largest"), dim.Render(fmt.Sprintf("%d nodes", mx.LargestCommunitySize))))
	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Singletons"), dim.Render(fmt.Sprintf("%d", mx.SingletonCommunities))))

	modColor := colorWarn
	modNote := "none"
	if mx.Modularity > 0.3 {
		modColor = colorSuccess
		modNote = "strong ✓"
	} else if mx.Modularity > 0.1 {
		modColor = colorText
		modNote = "weak"
	}
	modStyle := lipgloss.NewStyle().Foreground(modColor).Bold(true)
	b.WriteString(fmt.Sprintf("  %s %s  %s\n",
		label.Render("Modularity (Q)"),
		text.Render(fmt.Sprintf("%.3f", mx.Modularity)),
		modStyle.Render(modNote),
	))
	b.WriteString("\n")

	// ── Top nodes ─────────────────────────────────────────────────────────────
	if len(mx.TopByOutDegree) > 0 {
		b.WriteString(accent.Render("Top Nodes by Out-Degree"))
		b.WriteString("\n\n")

		rankStyle := lipgloss.NewStyle().Foreground(colorSubtext).Width(3)
		degStyle := lipgloss.NewStyle().Foreground(colorText)
		fileStyle := lipgloss.NewStyle().Foreground(colorSubtext)
		contentWidth := m.contentWidth()
		nameWidth := 24
		if contentWidth > 72 {
			nameWidth = contentWidth - 46
		}
		if nameWidth < 18 {
			nameWidth = 18
		}
		if nameWidth > 48 {
			nameWidth = 48
		}

		for i, n := range mx.TopByOutDegree {
			shortFile := filepath.Base(n.File)
			kindColor := nodeKindColor(n.Kind)
			nameStyle := lipgloss.NewStyle().Foreground(kindColor).Width(nameWidth).MaxWidth(nameWidth)
			kindTag := lipgloss.NewStyle().Foreground(kindColor).Faint(true).Render("[" + n.Kind + "]")
			b.WriteString(fmt.Sprintf("  %s %s %s %s  %s\n",
				rankStyle.Render(fmt.Sprintf("%d.", i+1)),
				nameStyle.Render(truncateMiddle(n.Label, nameWidth)),
				kindTag,
				degStyle.Render(fmt.Sprintf("out:%d in:%d", n.OutDeg, n.InDeg)),
				fileStyle.Render(truncateMiddle(shortFile, 24)),
			))
		}
		b.WriteString("\n")
	}

	// ── Obsidian vault section ────────────────────────────────────────────────
	b.WriteString(accent.Render("Obsidian Vault"))
	b.WriteString("\n\n")

	if vs.Notes == 0 {
		b.WriteString(fmt.Sprintf("  %s\n", warn.Render("No vault found — run Export to Obsidian")))
	} else {
		b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Notes"), text.Render(fmt.Sprintf("%d", vs.Notes))))
		b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Wikilinks"), text.Render(fmt.Sprintf("%d", vs.Links))))

		brokenLinkS := ok.Render("0 ✓")
		if vs.Broken > 0 {
			brokenLinkS = errS.Render(fmt.Sprintf("%d ✗", vs.Broken))
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Broken links"), brokenLinkS))

		healthS := ok.Render("healthy ✓")
		if !vs.Healthy {
			healthS = warn.Render("degraded — re-export")
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Vault status"), healthS))
	}

	b.WriteString("\n")
	return b.String()
}

func (m GraphStatusModel) View() string { return m.ViewContent() }

func (m GraphStatusModel) FooterHelp() string {
	if m.maxScrollOffset() > 0 {
		return "↑↓ scroll • pgup/pgdn page • r reload • esc back"
	}
	return "r refresh • esc back"
}

func (m GraphStatusModel) contentWidth() int {
	width := m.termWidth - 8
	if width < 60 {
		width = 60
	}
	return width
}

func (m GraphStatusModel) viewportHeight() int {
	height := m.termHeight - 14
	if height < 8 {
		height = 8
	}
	return height
}

func (m *GraphStatusModel) clampScroll() {
	if m == nil {
		return
	}
	maxOffset := m.maxScrollOffset()
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
}

func (m GraphStatusModel) maxScrollOffset() int {
	if m.loading || m.loadErr != nil {
		return 0
	}
	lines := strings.Split(strings.TrimRight(m.renderContent(), "\n"), "\n")
	viewportHeight := m.viewportHeight()
	if len(lines) <= viewportHeight {
		return 0
	}
	return len(lines) - viewportHeight
}

func truncateMiddle(value string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	if max <= 3 {
		return string(runes[:max])
	}
	head := (max - 1) / 2
	tail := max - head - 1
	return string(runes[:head]) + "…" + string(runes[len(runes)-tail:])
}
