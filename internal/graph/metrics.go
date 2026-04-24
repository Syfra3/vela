package graph

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
)

// NodeRank is a compact row used in metric leaderboards (top-N by degree).
type NodeRank struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	File   string `json:"file,omitempty"`
	Kind   string `json:"kind,omitempty"`
	OutDeg int    `json:"out_degree"`
	InDeg  int    `json:"in_degree"`
}

// HealthMetrics is the full set of graph-health signals computed from an
// on-disk graph.json. It is the single source of truth consumed by both
// `vela bench` and the TUI Graph Status screen.
type HealthMetrics struct {
	Path        string `json:"path"`
	GeneratedAt string `json:"generated_at,omitempty"`

	// Size
	Nodes int `json:"nodes"`
	Edges int `json:"edges"`

	// Coverage — breakdowns
	NodesByKind     map[string]int `json:"nodes_by_kind"`
	EdgesByRelation map[string]int `json:"edges_by_relation"`

	// Quality / resolution
	BrokenEdges    int     `json:"broken_edges"`
	SelfLoops      int     `json:"self_loops"`
	DuplicateEdges int     `json:"duplicate_edges"`
	ResolutionRate float64 `json:"resolution_rate"` // 0..1

	// Confidence
	ConfidenceDist map[string]int `json:"confidence_dist"`
	ExtractedRate  float64        `json:"extracted_rate"` // 0..1

	// Degree
	AvgDegree     float64 `json:"avg_degree"`
	MedianDegree  int     `json:"median_degree"`
	P95Degree     int     `json:"p95_degree"`
	MaxDegree     int     `json:"max_degree"`
	IsolatedNodes int     `json:"isolated_nodes"`
	HubNodes      int     `json:"hub_nodes"`  // total degree >= 10
	LeafNodes     int     `json:"leaf_nodes"` // total degree == 1
	ConnectedPct  int     `json:"connected_pct"`

	// Communities
	Communities          int     `json:"communities"`
	LargestCommunitySize int     `json:"largest_community_size"`
	SingletonCommunities int     `json:"singleton_communities"`
	Modularity           float64 `json:"modularity"`

	// Top nodes (length fixed via TopN when loading)
	TopByOutDegree []NodeRank `json:"top_by_out_degree"`
}

// rawGraph mirrors the on-disk JSON schema written by internal/export.WriteJSON.
type rawGraph struct {
	Nodes []struct {
		ID        string `json:"id"`
		Label     string `json:"label"`
		Kind      string `json:"kind"`
		File      string `json:"file,omitempty"`
		Community int    `json:"community,omitempty"`
	} `json:"nodes"`
	Edges []struct {
		From       string  `json:"from"`
		To         string  `json:"to"`
		Kind       string  `json:"kind"`
		Confidence string  `json:"confidence,omitempty"`
		Score      float64 `json:"score,omitempty"`
	} `json:"edges"`
	Meta struct {
		GeneratedAt string `json:"generatedAt"`
	} `json:"meta"`
}

// LoadHealthMetrics reads graph.json at path and computes every signal.
// topN controls the length of TopByOutDegree (5 is a reasonable default).
func LoadHealthMetrics(path string, topN int) (HealthMetrics, error) {
	m := HealthMetrics{
		Path:            path,
		NodesByKind:     map[string]int{},
		EdgesByRelation: map[string]int{},
		ConfidenceDist:  map[string]int{},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return m, fmt.Errorf("reading %s: %w", path, err)
	}
	var raw rawGraph
	if err := json.Unmarshal(data, &raw); err != nil {
		return m, fmt.Errorf("parsing %s: %w", path, err)
	}

	m.GeneratedAt = raw.Meta.GeneratedAt
	m.Nodes = len(raw.Nodes)
	m.Edges = len(raw.Edges)

	// Build indexes once.
	labelToID := make(map[string]string, m.Nodes)
	idToIndex := make(map[string]int, m.Nodes)
	for i, n := range raw.Nodes {
		idToIndex[n.ID] = i
		if _, seen := labelToID[n.Label]; !seen {
			labelToID[n.Label] = n.ID
		}
		m.NodesByKind[n.Kind]++
	}

	outDeg := make(map[string]int, m.Nodes)
	inDeg := make(map[string]int, m.Nodes)
	seenEdge := make(map[string]struct{}, m.Edges)

	// Per-community aggregates for modularity (Newman, undirected form).
	type commAgg struct {
		internalEdges int
		degreeSum     int
	}
	comm := map[int]*commAgg{}

	for _, e := range raw.Edges {
		m.EdgesByRelation[e.Kind]++

		conf := e.Confidence
		if conf == "" {
			conf = "UNSET"
		}
		m.ConfidenceDist[conf]++

		// Resolve endpoints.
		fromIdx, fromOK := idToIndex[e.From]
		toID, toResolved := resolveTargetID(e.To, idToIndex, labelToID)
		toIdx, toOK := -1, false
		if toResolved {
			toIdx, toOK = idToIndex[toID]
		}

		if !fromOK || !toOK {
			m.BrokenEdges++
			continue
		}

		if fromIdx == toIdx {
			m.SelfLoops++
			// self-loops still count toward degree bookkeeping below
		}

		key := e.From + "→" + toID
		if _, dup := seenEdge[key]; dup {
			m.DuplicateEdges++
		} else {
			seenEdge[key] = struct{}{}
		}

		outDeg[e.From]++
		inDeg[toID]++

		// Modularity accumulation (undirected simplification).
		cFrom := raw.Nodes[fromIdx].Community
		cTo := raw.Nodes[toIdx].Community
		if _, ok := comm[cFrom]; !ok {
			comm[cFrom] = &commAgg{}
		}
		if _, ok := comm[cTo]; !ok {
			comm[cTo] = &commAgg{}
		}
		comm[cFrom].degreeSum++
		comm[cTo].degreeSum++
		if cFrom == cTo {
			comm[cFrom].internalEdges++
		}
	}

	if m.Edges > 0 {
		m.ResolutionRate = float64(m.Edges-m.BrokenEdges) / float64(m.Edges)
		m.ExtractedRate = float64(m.ConfidenceDist["EXTRACTED"]) / float64(m.Edges)
	}

	// Degree distribution.
	degrees := make([]int, 0, m.Nodes)
	for _, n := range raw.Nodes {
		d := outDeg[n.ID] + inDeg[n.ID]
		degrees = append(degrees, d)
		if d == 0 {
			m.IsolatedNodes++
		}
		if d >= 10 {
			m.HubNodes++
		}
		if d == 1 {
			m.LeafNodes++
		}
		if d > m.MaxDegree {
			m.MaxDegree = d
		}
	}
	if m.Nodes > 0 {
		sort.Ints(degrees)
		var sum int
		for _, d := range degrees {
			sum += d
		}
		m.AvgDegree = float64(sum) / float64(m.Nodes)
		m.MedianDegree = degrees[len(degrees)/2]
		p95idx := int(math.Ceil(0.95*float64(len(degrees)))) - 1
		if p95idx < 0 {
			p95idx = 0
		}
		m.P95Degree = degrees[p95idx]
		m.ConnectedPct = 100 * (m.Nodes - m.IsolatedNodes) / m.Nodes
	}

	// Community stats.
	commSize := map[int]int{}
	for _, n := range raw.Nodes {
		commSize[n.Community]++
	}
	if !(len(commSize) == 1 && commSize[0] > 0) {
		m.Communities = len(commSize)
		for _, sz := range commSize {
			if sz > m.LargestCommunitySize {
				m.LargestCommunitySize = sz
			}
			if sz == 1 {
				m.SingletonCommunities++
			}
		}

		// Newman modularity Q = Σ_c [ L_c/m − (D_c/2m)² ].
		resolved := m.Edges - m.BrokenEdges
		if resolved > 0 {
			twoM := 2.0 * float64(resolved)
			var q float64
			for _, agg := range comm {
				lc := float64(agg.internalEdges)
				dc := float64(agg.degreeSum)
				q += (lc / float64(resolved)) - math.Pow(dc/twoM, 2)
			}
			m.Modularity = q
		}
	}

	// Top-N by out-degree.
	if topN > 0 && m.Nodes > 0 {
		type scored struct {
			n   NodeRank
			out int
		}
		all := make([]scored, 0, m.Nodes)
		for _, n := range raw.Nodes {
			all = append(all, scored{
				n: NodeRank{
					ID: n.ID, Label: n.Label, File: n.File, Kind: n.Kind,
					OutDeg: outDeg[n.ID], InDeg: inDeg[n.ID],
				},
				out: outDeg[n.ID],
			})
		}
		sort.Slice(all, func(i, j int) bool { return all[i].out > all[j].out })
		if topN > len(all) {
			topN = len(all)
		}
		m.TopByOutDegree = make([]NodeRank, topN)
		for i := 0; i < topN; i++ {
			m.TopByOutDegree[i] = all[i].n
		}
	}

	return m, nil
}

// resolveTargetID mirrors Build()'s resolveTarget: exact-ID first, then label.
func resolveTargetID(target string, idToIndex map[string]int, labelToID map[string]string) (string, bool) {
	if _, ok := idToIndex[target]; ok {
		return target, true
	}
	if id, ok := labelToID[target]; ok {
		return id, true
	}
	return "", false
}
