package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Syfra3/vela/internal/ancora"
	"github.com/Syfra3/vela/internal/config"
	vquery "github.com/Syfra3/vela/internal/query"
)

const queryResultLimit = 5

type querySearcherLoadedMsg struct {
	searcher *vquery.Searcher
	err      error
}

type querySearchResultMsg struct {
	response vquery.SearchResponse
	err      error
}

var (
	queryLoadSearcherFunc = loadQuerySearcher
	querySearchFunc       = runFederatedSearch
)

// QueryModel is the TUI model for interactive federated search.
type QueryModel struct {
	input       string
	searcher    *vquery.Searcher
	response    vquery.SearchResponse
	hasSearched bool
	quitting    bool
	loading     bool
	loadingText string
	err         error
}

func NewQueryModel() QueryModel {
	return QueryModel{
		loading:     true,
		loadingText: "Loading SQLite search index...",
	}
}

func (m QueryModel) Init() tea.Cmd {
	return loadQuerySearcherCmd()
}

func (m QueryModel) Quitting() bool {
	return m.quitting
}

func (m QueryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case querySearcherLoadedMsg:
		m.searcher = msg.searcher
		m.loading = false
		m.loadingText = ""
		m.err = msg.err
		return m, nil

	case querySearchResultMsg:
		m.loading = false
		m.loadingText = ""
		m.hasSearched = msg.err == nil
		m.err = msg.err
		if msg.err == nil {
			m.response = msg.response
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.quitting = true
			return m, nil

		case "r":
			m.loading = true
			m.loadingText = "Loading SQLite search index..."
			m.err = nil
			return m, loadQuerySearcherCmd()
		}

		if m.loading {
			return m, nil
		}

		switch msg.Type {
		case tea.KeyEnter:
			input := strings.TrimSpace(m.input)
			if input == "" {
				m.err = fmt.Errorf("query cannot be empty")
				return m, nil
			}
			if m.searcher == nil {
				m.err = fmt.Errorf("search is not ready")
				return m, nil
			}
			m.loading = true
			m.loadingText = fmt.Sprintf("Searching for %q...", input)
			m.err = nil
			return m, runQuerySearchCmd(m.searcher, input)

		case tea.KeyBackspace, tea.KeyDelete:
			if len(m.input) > 0 {
				runes := []rune(m.input)
				m.input = string(runes[:len(runes)-1])
			}

		case tea.KeyRunes:
			m.input += msg.String()
		case tea.KeySpace:
			m.input += " "
		}
	}

	return m, nil
}

func (m QueryModel) View() string {
	return m.ViewContent()
}

func (m QueryModel) ViewContent() string {
	var b strings.Builder

	textStyle := lipgloss.NewStyle().Foreground(colorText)
	dimStyle := lipgloss.NewStyle().Foreground(colorSubtext)
	inputStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	resultStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorOverlay).
		Padding(0, 1)
	metaStyle := lipgloss.NewStyle().Foreground(colorSubtext)

	b.WriteString(textStyle.Render("Federated search combines Ancora memory with Vela's SQLite-backed graph index."))
	b.WriteString("\n\n")
	b.WriteString(inputStyle.Render("> " + m.input))
	if !m.loading {
		b.WriteString(dimStyle.Render("█"))
	}
	b.WriteString("\n\n")

	if m.loading {
		b.WriteString(dimStyle.Render(m.loadingText))
		b.WriteString("\n\n")
	}

	if m.err != nil {
		b.WriteString(errorStyle.Render("Error: " + m.err.Error()))
		b.WriteString("\n\n")
	}

	if m.hasSearched {
		if len(m.response.Hits) == 0 {
			b.WriteString(dimStyle.Render(fmt.Sprintf("No results for %q.", m.response.Query)))
			b.WriteString("\n")
		} else {
			for i, hit := range m.response.Hits {
				var lines []string
				title := fmt.Sprintf("%d. [%s] %s", i+1, sourceLabel(hit.PrimarySource), hit.Label)
				if hit.Kind != "" {
					title += " (" + hit.Kind + ")"
				}
				lines = append(lines, title)

				var meta []string
				meta = append(meta, fmt.Sprintf("score %.2f", hit.Score))
				if len(hit.Sources) > 1 {
					meta = append(meta, "sources: "+joinSourceLabels(hit.Sources))
				}
				if hit.Path != "" {
					meta = append(meta, truncate(hit.Path, 72))
				}
				if len(hit.Signals) > 1 {
					meta = append(meta, "signals: "+joinSignalLabels(hit.Signals))
				}
				if hit.SupportGraph != nil {
					meta = append(meta, fmt.Sprintf("support %dn/%de", len(hit.SupportGraph.Nodes), len(hit.SupportGraph.Edges)))
				}
				lines = append(lines, metaStyle.Render(strings.Join(meta, "  |  ")))

				if hit.Snippet != "" {
					lines = append(lines, textStyle.Render(hit.Snippet))
				}
				if len(hit.Support) > 0 {
					lines = append(lines, dimStyle.Render("context: "+hit.Support[0]))
				}

				b.WriteString(resultStyle.Render(strings.Join(lines, "\n")))
				b.WriteString("\n\n")
			}
		}

		b.WriteString(dimStyle.Render(searchMetricsSummary(m.response.Metrics)))
		b.WriteString("\n")
	}

	return b.String()
}

func (m QueryModel) ModeName() string {
	return "Search"
}

func loadQuerySearcherCmd() tea.Cmd {
	return func() tea.Msg {
		searcher, err := queryLoadSearcherFunc()
		return querySearcherLoadedMsg{searcher: searcher, err: err}
	}
}

func runQuerySearchCmd(searcher *vquery.Searcher, input string) tea.Cmd {
	return func() tea.Msg {
		response, err := querySearchFunc(searcher, input)
		return querySearchResultMsg{response: response, err: err}
	}
}

func loadQuerySearcher() (*vquery.Searcher, error) {
	graphPath, err := config.FindGraphFile(".")
	if err != nil {
		return nil, err
	}
	engine, err := vquery.LoadFromFile(graphPath)
	if err != nil {
		return nil, err
	}
	ancoraDBPath, err := ancora.DefaultDBPath()
	if err != nil {
		return nil, err
	}
	return vquery.NewSearcher(engine, ancoraDBPath), nil
}

func runFederatedSearch(searcher *vquery.Searcher, input string) (vquery.SearchResponse, error) {
	return searcher.Search(input, queryResultLimit)
}

func searchMetricsSummary(metrics vquery.SearchMetrics) string {
	return fmt.Sprintf(
		"Ancora %dms/%d  |  Federated %dms/%d  |  overlap@%d %d  |  added %d",
		metrics.AncoraOnly.LatencyMs,
		metrics.AncoraOnly.Returned,
		metrics.Federated.LatencyMs,
		metrics.Federated.Returned,
		metrics.Limit,
		metrics.Comparison.OverlapAtK,
		metrics.Comparison.AddedByFederated,
	)
}

func sourceLabel(source string) string {
	switch source {
	case "ancora":
		return "Ancora"
	case "vela_graph":
		return "Graph"
	case "hybrid":
		return "Hybrid"
	default:
		return source
	}
}

func joinSourceLabels(sources []string) string {
	labels := make([]string, 0, len(sources))
	for _, source := range sources {
		labels = append(labels, sourceLabel(source))
	}
	return strings.Join(labels, ", ")
}

func joinSignalLabels(signals map[string]float64) string {
	labels := make([]string, 0, len(signals))
	for signal := range signals {
		labels = append(labels, strings.Title(signal))
	}
	sort.Strings(labels)
	return strings.Join(labels, ", ")
}
