package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Syfra3/vela/internal/config"
	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/internal/registry"
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
	snapshot igraph.StatusSnapshot
	registry igraph.RegistryStatusSnapshot
	env      igraph.ClusteringEnvironment
	vault    vaultStats
	loadErr  error
	global   bool
	repoName string
}

type GraphStatusModel struct {
	graphPath    string
	global       bool
	repoName     string
	metrics      igraph.HealthMetrics
	vault        vaultStats
	fresh        igraph.FreshnessStats
	projects     []igraph.ProjectStatus
	registryData igraph.RegistryStatusSnapshot
	env          igraph.ClusteringEnvironment
	loadErr      error
	loading      bool
	quitting     bool
	termWidth    int
	termHeight   int
	scrollOffset int
}

func NewGraphStatusModel() GraphStatusModel {
	return GraphStatusModel{loading: true, global: true, termWidth: 100, termHeight: 24}
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
		env := igraph.LoadClusteringEnvironment()
		resolvedGraphPath := graphPath
		entries, registryErr := registry.Load()
		if strings.TrimSpace(resolvedGraphPath) == "" && registryErr == nil && len(entries) > 0 {
			cfg, cfgErr := config.Load()
			if cfgErr != nil {
				return graphStatusLoadedMsg{env: env, global: true, loadErr: cfgErr}
			}
			vs := loadVaultStats(filepath.Join(config.ResolveVaultDir(cfg.Obsidian.VaultDir), "obsidian"))
			return graphStatusLoadedMsg{registry: igraph.LoadRegistryStatusSnapshot(entries, 5), env: env, vault: vs, global: true}
		}
		if strings.TrimSpace(resolvedGraphPath) == "" {
			var err error
			resolvedGraphPath, err = config.FindGraphFile(".")
			if err != nil {
				return graphStatusLoadedMsg{env: env, loadErr: err}
			}
		}
		snapshot, loadErr := igraph.LoadStatusSnapshot(resolvedGraphPath, 5)
		cfg, cfgErr := config.Load()
		if cfgErr != nil {
			return graphStatusLoadedMsg{snapshot: snapshot, env: env, loadErr: cfgErr}
		}
		vs := loadVaultStats(filepath.Join(config.ResolveVaultDir(cfg.Obsidian.VaultDir), "obsidian"))
		return graphStatusLoadedMsg{snapshot: snapshot, env: env, vault: vs, loadErr: loadErr, repoName: resolveSingleRepoName(resolvedGraphPath, entries, snapshot)}
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
		m.global = msg.global
		m.repoName = msg.repoName
		m.metrics = msg.snapshot.Metrics
		m.vault = msg.vault
		m.fresh = msg.snapshot.Freshness
		m.projects = msg.snapshot.Projects
		m.registryData = msg.registry
		m.env = msg.env
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
	if m.global {
		return m.renderGlobalContent()
	}
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
	fresh := m.fresh
	projects := append([]igraph.ProjectStatus(nil), m.projects...)
	sort.Slice(projects, func(i, j int) bool {
		if projects[i].Name == projects[j].Name {
			return projects[i].Path < projects[j].Path
		}
		return projects[i].Name < projects[j].Name
	})

	renderEnvironmentSection(&b, m.env, label, text, dim, ok, warn, errS, accent)

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

	b.WriteString(accent.Render("Freshness"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Graph path"), dim.Render(truncateMiddle(fresh.GraphPath, 64))))
	if !fresh.GraphUpdatedAt.IsZero() {
		b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Graph updated"), dim.Render(fresh.GraphUpdatedAt.Format("2006-01-02 15:04 UTC"))))
	}
	manifestState := warn.Render("missing")
	if fresh.ManifestPresent {
		manifestState = ok.Render("present")
	}
	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Manifest"), manifestState))
	if fresh.ManifestPresent {
		if !fresh.ManifestUpdatedAt.IsZero() {
			b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Manifest updated"), dim.Render(fresh.ManifestUpdatedAt.Format("2006-01-02 15:04 UTC"))))
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Tracked files"), text.Render(fmt.Sprintf("%d", fresh.TrackedFiles))))
		b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Last refresh mode"), renderBuildMode(fresh.BuildMode, ok, warn, dim)))
	}
	reportState := warn.Render("missing")
	if fresh.ReportPresent {
		reportState = ok.Render("present")
	}
	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("GRAPH_REPORT.md"), reportState))
	b.WriteString("\n")

	if len(projects) > 0 {
		b.WriteString(accent.Render("Projects"))
		b.WriteString("\n\n")
		for _, project := range projects {
			b.WriteString(fmt.Sprintf("  %s\n", text.Copy().Bold(true).Render(project.Name)))
			if project.Path != "" {
				b.WriteString(fmt.Sprintf("    %s %s\n", dim.Render("path"), dim.Render(truncateMiddle(project.Path, 72))))
			}
			if project.Remote != "" {
				b.WriteString(fmt.Sprintf("    %s %s\n", dim.Render("remote"), dim.Render(truncateMiddle(project.Remote, 72))))
			}
			b.WriteString(fmt.Sprintf("    %s nodes:%d  files:%d  symbols:%d  outgoing edges:%d\n\n",
				dim.Render("coverage"),
				project.Nodes,
				project.Files,
				project.Symbols,
				project.OutgoingEdges,
			))
		}
	}

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

func (m GraphStatusModel) renderGlobalContent() string {
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
	summary := m.registryData.Summary
	renderEnvironmentSection(&b, m.env, label, text, dim, ok, warn, errS, accent)
	b.WriteString(accent.Render("Global Summary"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Tracked repos"), text.Render(fmt.Sprintf("%d", summary.Repositories))))
	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Healthy graphs"), text.Render(fmt.Sprintf("%d", summary.HealthyGraphs))))
	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Missing graphs"), renderState(summary.MissingGraphs == 0, ok, warn, errS, fmt.Sprintf("%d", summary.MissingGraphs))))
	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Installed hooks"), text.Render(fmt.Sprintf("%d", summary.InstalledHooks))))
	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Missing hooks"), renderState(summary.MissingHooks == 0, ok, warn, errS, fmt.Sprintf("%d", summary.MissingHooks))))
	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Missing manifest"), renderState(summary.MissingManifest == 0, ok, warn, errS, fmt.Sprintf("%d", summary.MissingManifest))))
	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Missing report"), renderState(summary.MissingReport == 0, ok, warn, errS, fmt.Sprintf("%d", summary.MissingReport))))
	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Merged nodes"), text.Render(fmt.Sprintf("%d", summary.Nodes))))
	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Merged edges"), text.Render(fmt.Sprintf("%d", summary.Edges))))
	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Broken edges"), renderState(summary.BrokenEdges == 0, ok, warn, errS, fmt.Sprintf("%d", summary.BrokenEdges))))
	if !summary.LatestGraphUTC.IsZero() {
		b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Latest graph"), dim.Render(summary.LatestGraphUTC.Format("2006-01-02 15:04 UTC"))))
	}
	b.WriteString("\n")

	b.WriteString(accent.Render("Repositories"))
	b.WriteString("\n\n")
	for _, repo := range m.registryData.Repos {
		name := repo.Name
		if name == "" {
			name = filepath.Base(repo.RepoRoot)
		}
		b.WriteString(fmt.Sprintf("  %s\n", text.Copy().Bold(true).Render(name)))
		if repo.RepoRoot != "" {
			b.WriteString(fmt.Sprintf("    %s %s\n", dim.Render("path"), dim.Render(truncateMiddle(repo.RepoRoot, 72))))
		}
		if repo.Remote != "" {
			b.WriteString(fmt.Sprintf("    %s %s\n", dim.Render("remote"), dim.Render(truncateMiddle(repo.Remote, 72))))
		}
		if repo.GraphPath != "" {
			b.WriteString(fmt.Sprintf("    %s %s\n", dim.Render("graph"), dim.Render(truncateMiddle(repo.GraphPath, 72))))
		}
		if repo.ManifestPath != "" {
			b.WriteString(fmt.Sprintf("    %s %s\n", dim.Render("manifest file"), dim.Render(truncateMiddle(repo.ManifestPath, 72))))
		}
		if repo.ReportPath != "" {
			b.WriteString(fmt.Sprintf("    %s %s\n", dim.Render("report file"), dim.Render(truncateMiddle(repo.ReportPath, 72))))
		}
		b.WriteString(fmt.Sprintf("    %s %s\n", dim.Render("hooks"), renderHookState(repo.HookStatus, repo.HookInstalled, ok, warn, errS)))
		b.WriteString(fmt.Sprintf("    %s %s  %s %s\n", dim.Render("manifest"), renderBinaryState(repo.Snapshot.Freshness.ManifestPresent, ok, warn), dim.Render("report"), renderBinaryState(repo.Snapshot.Freshness.ReportPresent, ok, warn)))
		if repo.LoadError != "" {
			b.WriteString(fmt.Sprintf("    %s %s\n\n", dim.Render("status"), errS.Render(repo.LoadError)))
			continue
		}
		if !repo.Snapshot.Freshness.GraphUpdatedAt.IsZero() {
			b.WriteString(fmt.Sprintf("    %s %s\n", dim.Render("updated"), dim.Render(repo.Snapshot.Freshness.GraphUpdatedAt.Format("2006-01-02 15:04 UTC"))))
		}
		b.WriteString(fmt.Sprintf("    %s nodes:%d  files:%d  symbols:%d  edges:%d  broken:%d\n\n",
			dim.Render("coverage"),
			repo.Snapshot.Metrics.Nodes,
			totalProjectFiles(repo.Snapshot.Projects),
			totalProjectSymbols(repo.Snapshot.Projects),
			repo.Snapshot.Metrics.Edges,
			repo.Snapshot.Metrics.BrokenEdges,
		))
		if len(repo.Snapshot.Projects) > 0 {
			projects := append([]igraph.ProjectStatus(nil), repo.Snapshot.Projects...)
			sort.Slice(projects, func(i, j int) bool {
				if projects[i].Name == projects[j].Name {
					return projects[i].Path < projects[j].Path
				}
				return projects[i].Name < projects[j].Name
			})
			for _, project := range projects {
				b.WriteString(fmt.Sprintf("      %s\n", text.Render(project.Name)))
				if project.Path != "" {
					b.WriteString(fmt.Sprintf("        %s %s\n", dim.Render("path"), dim.Render(truncateMiddle(project.Path, 68))))
				}
				b.WriteString(fmt.Sprintf("        %s nodes:%d  files:%d  symbols:%d  outgoing edges:%d\n", dim.Render("project"), project.Nodes, project.Files, project.Symbols, project.OutgoingEdges))
			}
			b.WriteString("\n")
		}
	}
	if len(m.registryData.Repos) == 0 {
		b.WriteString(dim.Render("No tracked repositories in ~/.vela/registry.json."))
		b.WriteString("\n\n")
	}

	b.WriteString(accent.Render("Obsidian Vault"))
	b.WriteString("\n\n")
	if m.vault.Notes == 0 {
		b.WriteString(fmt.Sprintf("  %s\n", warn.Render("No vault found — run Export to Obsidian")))
	} else {
		b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Notes"), text.Render(fmt.Sprintf("%d", m.vault.Notes))))
		b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Wikilinks"), text.Render(fmt.Sprintf("%d", m.vault.Links))))
	}
	b.WriteString("\n")
	return b.String()
}

func renderEnvironmentSection(b *strings.Builder, env igraph.ClusteringEnvironment, label, text, dim, ok, warn, errS, accent lipgloss.Style) {
	b.WriteString(accent.Render("Environment"))
	b.WriteString("\n\n")

	pythonState := warn.Render("missing")
	if env.PythonFound {
		pythonState = ok.Render("found")
	}
	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Python"), pythonState))
	if env.PythonFound && strings.TrimSpace(env.PythonPath) != "" {
		b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Python path"), dim.Render(truncateMiddle(env.PythonPath, 64))))
	}
	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("networkx"), renderBinaryState(env.NetworkXAvailable, ok, warn)))
	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("graspologic"), renderBinaryState(env.GraspologicAvailable, ok, warn)))

	backendState := warn.Render("missing")
	if env.NetworkXAvailable || env.GraspologicAvailable {
		backendState = ok.Render("ready")
	}
	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Clustering"), backendState))

	installStyle := dim
	if !env.PythonFound || (!env.NetworkXAvailable && !env.GraspologicAvailable) {
		installStyle = errS
	}
	b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Install base"), installStyle.Render(env.BaseInstallCommand)))
	if !env.GraspologicAvailable {
		b.WriteString(fmt.Sprintf("  %s %s\n", label.Render("Install Leiden"), dim.Render(env.LeidenInstallCommand)))
	}
	b.WriteString("\n")
}

func renderBuildMode(mode string, ok, warn, dim lipgloss.Style) string {
	switch strings.TrimSpace(mode) {
	case "full_rebuild":
		return ok.Render("full rebuild")
	case "deleted_only_prune":
		return warn.Render("deleted-file prune")
	case "":
		return dim.Render("unknown")
	default:
		return dim.Render(strings.ReplaceAll(mode, "_", " "))
	}
}

func renderBinaryState(value bool, ok, warn lipgloss.Style) string {
	if value {
		return ok.Render("present")
	}
	return warn.Render("missing")
}

func renderHookState(status string, installed bool, ok, warn, errS lipgloss.Style) string {
	if installed {
		return ok.Render("installed")
	}
	switch status {
	case "partial":
		return warn.Render(status)
	case "missing", "unavailable", "path unavailable":
		return errS.Render(status)
	default:
		return warn.Render(status)
	}
}

func renderState(healthy bool, ok, warn, errS lipgloss.Style, value string) string {
	if healthy {
		return ok.Render(value)
	}
	if value == "0" {
		return warn.Render(value)
	}
	return errS.Render(value)
}

func totalProjectFiles(projects []igraph.ProjectStatus) int {
	total := 0
	for _, project := range projects {
		total += project.Files
	}
	return total
}

func totalProjectSymbols(projects []igraph.ProjectStatus) int {
	total := 0
	for _, project := range projects {
		total += project.Symbols
	}
	return total
}

func resolveSingleRepoName(graphPath string, entries []registry.Entry, snapshot igraph.StatusSnapshot) string {
	cleanGraphPath := strings.TrimSpace(graphPath)
	for _, entry := range entries {
		if strings.TrimSpace(entry.GraphPath) == cleanGraphPath && strings.TrimSpace(entry.Name) != "" {
			return entry.Name
		}
	}
	if len(snapshot.Projects) == 1 && strings.TrimSpace(snapshot.Projects[0].Name) != "" {
		return snapshot.Projects[0].Name
	}
	if cleanGraphPath != "" {
		return filepath.Base(filepath.Dir(cleanGraphPath))
	}
	return ""
}

func (m GraphStatusModel) View() string { return m.ViewContent() }

func (m GraphStatusModel) FooterHelp() string {
	if m.maxScrollOffset() > 0 {
		return "↑↓ scroll • pgup/pgdn page • r reload • esc back"
	}
	return "r refresh • esc back"
}

func (m GraphStatusModel) Subtitle() string {
	if m.global {
		return "Global tracked repositories"
	}
	if strings.TrimSpace(m.repoName) != "" {
		return "Repository: " + m.repoName
	}
	return "Read-only"
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
