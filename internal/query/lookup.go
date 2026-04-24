package query

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/Syfra3/vela/pkg/types"
)

// LookupCandidate is a ranked node suggestion for broad discovery queries.
type LookupCandidate struct {
	Node  types.Node
	Score int
}

// Lookup returns ranked node candidates for a broad term without pretending to
// answer a structural graph question directly.
func (e *Engine) Lookup(term string, limit int) []LookupCandidate {
	if e == nil || e.graph == nil {
		return nil
	}
	term = strings.TrimSpace(term)
	if term == "" {
		return nil
	}
	if limit <= 0 {
		limit = 5
	}

	normalized := strings.ToLower(term)
	tokens := lookupTokens(normalized)
	if len(tokens) == 0 {
		return nil
	}

	results := make([]LookupCandidate, 0, limit)
	for _, node := range e.graph.Nodes {
		score := lookupScore(node, normalized, tokens)
		if score == 0 {
			continue
		}
		results = append(results, LookupCandidate{Node: node, Score: score})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			if results[i].Node.Label == results[j].Node.Label {
				return results[i].Node.ID < results[j].Node.ID
			}
			return results[i].Node.Label < results[j].Node.Label
		}
		return results[i].Score > results[j].Score
	})

	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

// RenderLookup formats ranked candidates with enough metadata to pick an exact
// subject for follow-up structural graph queries.
func (e *Engine) RenderLookup(term string, limit int) string {
	results := e.Lookup(term, limit)
	if len(results) == 0 {
		return fmt.Sprintf("No candidates found for %q.", term)
	}

	lines := []string{fmt.Sprintf("Candidates for %q:", term), ""}
	for i, candidate := range results {
		node := candidate.Node
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, describeNode(node)))
		lines = append(lines, fmt.Sprintf("   id: %s", node.ID))
		if file := strings.TrimSpace(node.SourceFile); file != "" && file != node.Label {
			lines = append(lines, fmt.Sprintf("   file: %s", file))
		}
	}

	best := results[0].Node.Label
	if strings.TrimSpace(best) == "" {
		best = results[0].Node.ID
	}
	lines = append(lines, "", "Next steps:")
	lines = append(lines, fmt.Sprintf("- vela search \"explain %s\"", best))
	lines = append(lines, fmt.Sprintf("- vela search \"who uses %s\"", best))
	return strings.Join(lines, "\n")
}

func lookupTokens(input string) []string {
	parts := strings.FieldsFunc(input, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '/' && r != '.' && r != '_' && r != '-'
	})
	unique := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		unique = append(unique, part)
	}
	return unique
}

func lookupScore(node types.Node, term string, tokens []string) int {
	label := strings.ToLower(strings.TrimSpace(node.Label))
	id := strings.ToLower(strings.TrimSpace(node.ID))
	file := strings.ToLower(strings.TrimSpace(node.SourceFile))
	description := strings.ToLower(strings.TrimSpace(node.Description))

	score := 0
	switch {
	case label == term || id == term:
		score += 100
	case file == term:
		score += 95
	case canonicalPathSuffix(file) == canonicalPathSuffix(term) && canonicalPathSuffix(term) != "":
		score += 85
	case strings.Contains(label, term):
		score += 70
	case strings.Contains(file, term):
		score += 65
	case strings.Contains(id, term):
		score += 60
	}

	matchedTokens := 0
	for _, token := range tokens {
		tokenMatched := false
		switch {
		case strings.Contains(label, token):
			score += 18
			tokenMatched = true
		case strings.Contains(file, token):
			score += 16
			tokenMatched = true
		case strings.Contains(id, token):
			score += 14
			tokenMatched = true
		case strings.Contains(description, token):
			score += 6
			tokenMatched = true
		}
		if tokenMatched {
			matchedTokens++
		}
	}
	if matchedTokens == len(tokens) && len(tokens) > 1 {
		score += 20
	}
	if matchedTokens == 0 {
		return 0
	}

	switch node.NodeType {
	case string(types.NodeTypeFile):
		if looksPathLike(term) {
			score += 12
		}
	default:
		if !looksPathLike(term) {
			score += 8
		}
	}

	if strings.Contains(file, "/test") || strings.Contains(file, ".test.") || strings.Contains(file, "_test.") {
		score -= 10
	}
	return score
}

func looksPathLike(input string) bool {
	return strings.Contains(input, "/") || strings.Contains(input, ".")
}

func canonicalPathSuffix(input string) string {
	input = filepath.ToSlash(strings.TrimSpace(input))
	if input == "" {
		return ""
	}
	parts := strings.Split(input, "/")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	return input
}
