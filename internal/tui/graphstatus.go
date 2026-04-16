package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// GraphMetrics — computed from graph.json + obsidian vault
// ---------------------------------------------------------------------------

type GraphMetrics struct {
	// Graph
	Nodes         int
	Edges         int
	BrokenEdges   int
	IsolatedNodes int // nodes with no edges at all
	ConnectedPct  int // % nodes with at least one edge

	// Connectivity buckets
	HubNodes  int // degree >= 10
	LeafNodes int // degree == 1

	// Top 5 nodes by out-degree
	TopNodes []nodeRank

	// Obsidian vault
	VaultNotes   int
	VaultLinks   int
	BrokenLinks  int
	VaultHealthy bool

	// Meta
	GeneratedAt string
	GraphPath   string
	VaultPath   string
	LoadErr     error
}

type nodeRank struct {
	Label  string
	File   string
	Kind   string
	OutDeg int
	InDeg  int
}

// loadGraphMetrics reads graph.json and the obsidian vault and computes metrics.
func loadGraphMetrics(outDir string) GraphMetrics {
	m := GraphMetrics{
		GraphPath: filepath.Join(outDir, "graph.json"),
		VaultPath: filepath.Join(outDir, "obsidian"),
	}

	// ── graph.json ──────────────────────────────────────────────────────────
	data, err := os.ReadFile(m.GraphPath)
	if err != nil {
		m.LoadErr = fmt.Errorf("graph.json not found — run Extract first")
		return m
	}

	var raw struct {
		Nodes []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
			Kind  string `json:"kind"`
			File  string `json:"file"`
		} `json:"nodes"`
		Edges []struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"edges"`
		Meta struct {
			GeneratedAt string `json:"generatedAt"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		m.LoadErr = fmt.Errorf("corrupt graph.json: %w", err)
		return m
	}

	m.Nodes = len(raw.Nodes)
	m.Edges = len(raw.Edges)
	m.GeneratedAt = raw.Meta.GeneratedAt

	// Build label set and degree maps
	labelSet := make(map[string]bool, m.Nodes)
	outDeg := make(map[string]int, m.Nodes)
	inDeg := make(map[string]int, m.Nodes)
	for _, n := range raw.Nodes {
		labelSet[n.Label] = true
	}
	for _, e := range raw.Edges {
		if !labelSet[e.To] {
			m.BrokenEdges++
		}
		outDeg[e.From]++
		// resolve target id for in-degree
		for _, n := range raw.Nodes {
			if n.Label == e.To {
				inDeg[n.ID]++
				break
			}
		}
	}

	// Per-node stats
	for _, n := range raw.Nodes {
		od := outDeg[n.ID]
		id := inDeg[n.ID]
		total := od + id
		if total == 0 {
			m.IsolatedNodes++
		}
		if total >= 10 {
			m.HubNodes++
		}
		if total == 1 {
			m.LeafNodes++
		}
	}
	if m.Nodes > 0 {
		m.ConnectedPct = 100 * (m.Nodes - m.IsolatedNodes) / m.Nodes
	}

	// Top 5 by out-degree
	type ranked struct {
		id    string
		label string
		file  string
		kind  string
		out   int
		in    int
	}
	var ranked5 []ranked
	for _, n := range raw.Nodes {
		ranked5 = append(ranked5, ranked{n.ID, n.Label, n.File, n.Kind, outDeg[n.ID], inDeg[n.ID]})
	}
	// simple top-5 selection (avoid sorting the whole slice)
	for i := 0; i < 5 && len(ranked5) > 0; i++ {
		best := 0
		for j := 1; j < len(ranked5); j++ {
			if ranked5[j].out > ranked5[best].out {
				best = j
			}
		}
		r := ranked5[best]
		m.TopNodes = append(m.TopNodes, nodeRank{r.label, r.file, r.kind, r.out, r.in})
		ranked5 = append(ranked5[:best], ranked5[best+1:]...)
	}

	// ── Obsidian vault ───────────────────────────────────────────────────────
	entries, err := os.ReadDir(m.VaultPath)
	if err == nil {
		re := regexp.MustCompile(`\[\[(.+?)\]\]`)
		mdFiles := make(map[string]bool)
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				mdFiles[strings.TrimSuffix(e.Name(), ".md")] = true
			}
		}
		m.VaultNotes = len(mdFiles)

		for name := range mdFiles {
			content, rerr := os.ReadFile(filepath.Join(m.VaultPath, name+".md"))
			if rerr != nil {
				continue
			}
			for _, match := range re.FindAllSubmatch(content, -1) {
				m.VaultLinks++
				if !mdFiles[string(match[1])] {
					m.BrokenLinks++
				}
			}
		}
		m.VaultHealthy = m.BrokenLinks == 0 && m.VaultNotes > 0
	}

	return m
}

// ---------------------------------------------------------------------------
// GraphStatusModel — the TUI screen
// ---------------------------------------------------------------------------

type graphStatusLoadedMsg struct {
	metrics GraphMetrics
}

type GraphStatusModel struct {
	metrics  GraphMetrics
	loading  bool
	quitting bool
}

func NewGraphStatusModel() GraphStatusModel {
	return GraphStatusModel{loading: true}
}

func (m GraphStatusModel) Quitting() bool { return m.quitting }

func (m GraphStatusModel) Init() tea.Cmd {
	return loadMetricsCmd()
}

func loadMetricsCmd() tea.Cmd {
	return func() tea.Msg {
		return graphStatusLoadedMsg{metrics: loadGraphMetrics("vela-out")}
	}
}

func (m GraphStatusModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case graphStatusLoadedMsg:
		m.metrics = msg.metrics
		m.loading = false
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.quitting = true
		case "r":
			m.loading = true
			return m, loadMetricsCmd()
		}
	}
	return m, nil
}

func (m GraphStatusModel) ViewContent() string {
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

	mx := m.metrics

	if mx.LoadErr != nil {
		b.WriteString(errS.Render("✗ " + mx.LoadErr.Error()))
		return b.String()
	}

	// ── Graph section ────────────────────────────────────────────────────────
	b.WriteString(accent.Render("Graph"))
	b.WriteString("\n\n")

	// Generated at
	if mx.GeneratedAt != "" {
		t, err := time.Parse(time.RFC3339, mx.GeneratedAt)
		if err == nil {
			b.WriteString(fmt.Sprintf("  %s %s\n\n", label.Render("Generated"), dim.Render(t.Format("2006-01-02 15:04 UTC"))))
		}
	}

	// Nodes / Edges row
	b.WriteString(fmt.Sprintf("  %s %s\n",
		label.Render("Nodes"),
		text.Render(fmt.Sprintf("%d", mx.Nodes)),
	))
	b.WriteString(fmt.Sprintf("  %s %s\n",
		label.Render("Edges"),
		text.Render(fmt.Sprintf("%d", mx.Edges)),
	))

	// Broken edges
	brokenEdgeS := ok.Render("0 ✓")
	if mx.BrokenEdges > 0 {
		brokenEdgeS = errS.Render(fmt.Sprintf("%d ✗", mx.BrokenEdges))
	}
	b.WriteString(fmt.Sprintf("  %s %s\n",
		label.Render("Broken edges"),
		brokenEdgeS,
	))

	b.WriteString("\n")

	// Connectivity bar
	connColor := colorSuccess
	if mx.ConnectedPct < 50 {
		connColor = colorWarn
	}
	connBar := bar(mx.ConnectedPct, 24)
	connLabel := lipgloss.NewStyle().Foreground(connColor).Bold(true).
		Render(fmt.Sprintf("%d%%", mx.ConnectedPct))
	b.WriteString(fmt.Sprintf("  %s %s %s\n",
		label.Render("Connected nodes"),
		connBar,
		connLabel,
	))
	b.WriteString(fmt.Sprintf("  %s %s\n",
		label.Render("Isolated nodes"),
		dim.Render(fmt.Sprintf("%d", mx.IsolatedNodes)),
	))
	b.WriteString(fmt.Sprintf("  %s %s   %s %s\n",
		label.Render("Hub nodes (≥10 edges)"),
		text.Render(fmt.Sprintf("%d", mx.HubNodes)),
		dim.Render("leaf (1 edge)"),
		dim.Render(fmt.Sprintf("%d", mx.LeafNodes)),
	))

	// ── Top nodes ────────────────────────────────────────────────────────────
	if len(mx.TopNodes) > 0 {
		b.WriteString("\n")
		b.WriteString(accent.Render("Top Nodes by Out-Degree"))
		b.WriteString("\n\n")

		rankStyle := lipgloss.NewStyle().Foreground(colorSubtext).Width(3)
		degStyle := lipgloss.NewStyle().Foreground(colorText)
		fileStyle := lipgloss.NewStyle().Foreground(colorSubtext)

		for i, n := range mx.TopNodes {
			shortFile := filepath.Base(n.File)
			kindColor := nodeKindColor(n.Kind)
			nameStyle := lipgloss.NewStyle().Foreground(kindColor).Width(24)
			kindTag := lipgloss.NewStyle().Foreground(kindColor).Faint(true).Render("[" + n.Kind + "]")
			b.WriteString(fmt.Sprintf("  %s %s %s %s  %s\n",
				rankStyle.Render(fmt.Sprintf("%d.", i+1)),
				nameStyle.Render(n.Label),
				kindTag,
				degStyle.Render(fmt.Sprintf("out:%d in:%d", n.OutDeg, n.InDeg)),
				fileStyle.Render(shortFile),
			))
		}
	}

	// ── Obsidian vault section ───────────────────────────────────────────────
	b.WriteString("\n")
	b.WriteString(accent.Render("Obsidian Vault"))
	b.WriteString("\n\n")

	if mx.VaultNotes == 0 {
		b.WriteString(fmt.Sprintf("  %s\n", warn.Render("No vault found — run Export to Obsidian")))
	} else {
		b.WriteString(fmt.Sprintf("  %s %s\n",
			label.Render("Notes"),
			text.Render(fmt.Sprintf("%d", mx.VaultNotes)),
		))
		b.WriteString(fmt.Sprintf("  %s %s\n",
			label.Render("Wikilinks"),
			text.Render(fmt.Sprintf("%d", mx.VaultLinks)),
		))

		brokenLinkS := ok.Render("0 ✓")
		if mx.BrokenLinks > 0 {
			brokenLinkS = errS.Render(fmt.Sprintf("%d ✗", mx.BrokenLinks))
		}
		b.WriteString(fmt.Sprintf("  %s %s\n",
			label.Render("Broken links"),
			brokenLinkS,
		))

		healthS := ok.Render("healthy ✓")
		if !mx.VaultHealthy {
			healthS = warn.Render("degraded — re-export")
		}
		b.WriteString(fmt.Sprintf("  %s %s\n",
			label.Render("Vault status"),
			healthS,
		))
	}

	b.WriteString("\n")

	return b.String()
}

func (m GraphStatusModel) View() string { return m.ViewContent() }

func (m GraphStatusModel) FooterHelp() string {
	return "r refresh • esc back to menu"
}
