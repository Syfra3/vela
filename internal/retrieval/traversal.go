package retrieval

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

// TraversalOptions bounds structural graph expansion.
type TraversalOptions struct {
	MaxHops          int
	MaxExpansions    int
	Limit            int
	AllowedRelations []string
}

// EdgeContext is compact support for why a structural result was included.
type EdgeContext struct {
	FromID    string
	FromLabel string
	ToID      string
	ToLabel   string
	Relation  string
	Direction string
	Hop       int
}

// StructuralResult is one graph-structural candidate.
type StructuralResult struct {
	Result
	Context []EdgeContext
}

type traversalNode struct {
	ID           string
	Label        string
	Kind         string
	Path         string
	Description  string
	MetadataText string
}

type adjacencyEdge struct {
	Neighbor  traversalNode
	Relation  string
	Direction string
}

type traversalState struct {
	Node  traversalNode
	Hop   int
	Score float64
}

type structuralAccumulator struct {
	result   StructuralResult
	contexts map[string]struct{}
}

// ExpandStructural walks the persisted graph outward from lexical seeds.
func ExpandStructural(dbPath string, seeds []Result, opts TraversalOptions) ([]StructuralResult, int, error) {
	if len(seeds) == 0 {
		return nil, 0, nil
	}
	if opts.MaxHops <= 0 {
		opts.MaxHops = 2
	}
	if opts.MaxExpansions <= 0 {
		opts.MaxExpansions = 24
	}
	if opts.Limit <= 0 {
		opts.Limit = 5
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, 0, fmt.Errorf("open retrieval db: %w", err)
	}
	defer db.Close()

	adjacency, err := loadAdjacency(db, opts.AllowedRelations)
	if err != nil {
		return nil, 0, err
	}

	seedIDs := make(map[string]struct{}, len(seeds))
	bestHop := make(map[string]int, len(seeds))
	queue := make([]traversalState, 0, len(seeds))
	for _, seed := range seeds {
		if seed.ID == "" {
			continue
		}
		seedIDs[seed.ID] = struct{}{}
		bestHop[seed.ID] = 0
		queue = append(queue, traversalState{
			Node: traversalNode{
				ID:           seed.ID,
				Label:        seed.Label,
				Kind:         seed.Kind,
				Path:         seed.Path,
				Description:  seed.Description,
				MetadataText: seed.MetadataText,
			},
			Hop:   0,
			Score: seed.Score,
		})
	}

	accumulators := map[string]*structuralAccumulator{}
	expansions := 0
	for len(queue) > 0 && expansions < opts.MaxExpansions {
		current := queue[0]
		queue = queue[1:]
		if current.Hop >= opts.MaxHops {
			continue
		}
		for _, edge := range adjacency[current.Node.ID] {
			if expansions >= opts.MaxExpansions {
				break
			}
			expansions++
			nextHop := current.Hop + 1
			score := decayStructuralScore(current.Score, nextHop)
			neighbor := edge.Neighbor

			if _, isSeed := seedIDs[neighbor.ID]; !isSeed && neighbor.ID != "" {
				acc := accumulators[neighbor.ID]
				if acc == nil {
					acc = &structuralAccumulator{
						result: StructuralResult{Result: Result{
							ID:           neighbor.ID,
							Label:        neighbor.Label,
							Kind:         neighbor.Kind,
							Path:         neighbor.Path,
							Description:  neighbor.Description,
							MetadataText: neighbor.MetadataText,
						}},
						contexts: map[string]struct{}{},
					}
					accumulators[neighbor.ID] = acc
				}
				acc.result.Score += score
				context := EdgeContext{
					FromID:    current.Node.ID,
					FromLabel: current.Node.Label,
					ToID:      neighbor.ID,
					ToLabel:   neighbor.Label,
					Relation:  edge.Relation,
					Direction: edge.Direction,
					Hop:       nextHop,
				}
				key := fmt.Sprintf("%s|%s|%s|%d", context.FromID, context.ToID, context.Relation, context.Hop)
				if _, seen := acc.contexts[key]; !seen {
					acc.contexts[key] = struct{}{}
					acc.result.Context = append(acc.result.Context, context)
				}
			}

			if neighbor.ID == "" || nextHop >= opts.MaxHops {
				continue
			}
			if priorHop, seen := bestHop[neighbor.ID]; seen && priorHop <= nextHop {
				continue
			}
			bestHop[neighbor.ID] = nextHop
			queue = append(queue, traversalState{Node: neighbor, Hop: nextHop, Score: score})
		}
	}

	results := make([]StructuralResult, 0, len(accumulators))
	for _, acc := range accumulators {
		sort.Slice(acc.result.Context, func(i, j int) bool {
			if acc.result.Context[i].Hop == acc.result.Context[j].Hop {
				return acc.result.Context[i].Relation < acc.result.Context[j].Relation
			}
			return acc.result.Context[i].Hop < acc.result.Context[j].Hop
		})
		results = append(results, acc.result)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return strings.ToLower(results[i].Label) < strings.ToLower(results[j].Label)
		}
		return results[i].Score > results[j].Score
	})

	candidates := len(results)
	if opts.Limit < len(results) {
		results = results[:opts.Limit]
	}
	return results, candidates, nil
}

func loadAdjacency(db *sql.DB, allowedRelations []string) (map[string][]adjacencyEdge, error) {
	filter := make(map[string]struct{}, len(allowedRelations))
	for _, relation := range allowedRelations {
		if relation == "" {
			continue
		}
		filter[strings.ToLower(relation)] = struct{}{}
	}
	allow := func(relation string) bool {
		if len(filter) == 0 {
			return true
		}
		_, ok := filter[strings.ToLower(relation)]
		return ok
	}

	rows, err := db.Query(`
		SELECT
			e.source_id,
			e.target_id,
			COALESCE(e.relation, ''),
			COALESCE(s.label, ''),
			COALESCE(s.kind, ''),
			COALESCE(s.path, ''),
			COALESCE(s.description, ''),
			COALESCE(s.metadata_text, ''),
			COALESCE(t.label, ''),
			COALESCE(t.kind, ''),
			COALESCE(t.path, ''),
			COALESCE(t.description, ''),
			COALESCE(t.metadata_text, '')
		FROM edges e
		JOIN nodes s ON s.id = e.source_id
		JOIN nodes t ON t.id = e.target_id`)
	if err != nil {
		return nil, fmt.Errorf("query edges: %w", err)
	}
	defer rows.Close()

	adjacency := map[string][]adjacencyEdge{}
	for rows.Next() {
		var sourceID, targetID, relation string
		var source traversalNode
		var target traversalNode
		if err := rows.Scan(
			&sourceID,
			&targetID,
			&relation,
			&source.Label,
			&source.Kind,
			&source.Path,
			&source.Description,
			&source.MetadataText,
			&target.Label,
			&target.Kind,
			&target.Path,
			&target.Description,
			&target.MetadataText,
		); err != nil {
			return nil, fmt.Errorf("scan edge: %w", err)
		}
		if !allow(relation) {
			continue
		}
		source.ID = sourceID
		target.ID = targetID
		adjacency[sourceID] = append(adjacency[sourceID], adjacencyEdge{Neighbor: target, Relation: relation, Direction: "out"})
		adjacency[targetID] = append(adjacency[targetID], adjacencyEdge{Neighbor: source, Relation: relation, Direction: "in"})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate edges: %w", err)
	}
	return adjacency, nil
}

func decayStructuralScore(seedScore float64, hop int) float64 {
	if hop <= 0 {
		return seedScore
	}
	decay := 0.35
	if hop > 1 {
		decay = 0.35 / float64(hop)
	}
	return seedScore * decay
}
