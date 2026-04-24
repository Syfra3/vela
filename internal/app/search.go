package app

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Syfra3/vela/pkg/types"
)

var pathArrowPattern = regexp.MustCompile(`(?i)^path\s+(.+?)\s*(?:->|=>|to)\s+(.+)$`)
var pathFromPattern = regexp.MustCompile(`(?i)^path\s+from\s+(.+?)\s+to\s+(.+)$`)
var pathHowPattern = regexp.MustCompile(`(?i)^how does\s+(.+?)\s+(?:reach|connect to|get to)\s+(.+?)(?:\?)?$`)
var explainPattern = regexp.MustCompile(`(?i)^explain\s+(.+?)(?:\?)?$`)
var reverseUsedPattern = regexp.MustCompile(`(?i)^where is\s+(.+?)\s+used(?:\?)?$`)
var reverseWhoUsesPattern = regexp.MustCompile(`(?i)^(?:who|what)\s+(?:uses|calls|depends on)\s+(.+?)(?:\?)?$`)
var reverseDepsPattern = regexp.MustCompile(`(?i)^reverse dependencies of\s+(.+?)(?:\?)?$`)
var reverseWhatDependsPattern = regexp.MustCompile(`(?i)^what depends on\s+(.+?)(?:\?)?$`)
var depsWhatDoesPattern = regexp.MustCompile(`(?i)^what does\s+(.+?)\s+depend on(?:\?)?$`)
var depsOfPattern = regexp.MustCompile(`(?i)^dependencies of\s+(.+?)(?:\?)?$`)
var impactOfPattern = regexp.MustCompile(`(?i)^impact of\s+(.+?)(?:\?)?$`)
var impactBreaksPattern = regexp.MustCompile(`(?i)^what breaks if\s+(.+?)\s+changes(?:\?)?$`)
var impactAffectedPattern = regexp.MustCompile(`(?i)^what is affected by\s+(.+?)(?:\?)?$`)

// ParseSearchQuery routes a freeform structural question into the canonical
// graph-truth query contract used by the CLI and MCP surfaces.
func ParseSearchQuery(raw string) (types.QueryRequest, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return types.QueryRequest{}, fmt.Errorf("search query is required")
	}

	if req, ok := parsePathQuery(raw); ok {
		return req.Normalize(), nil
	}
	if req, ok := parseSingleSubjectQuery(raw); ok {
		return req.Normalize(), nil
	}

	return types.QueryRequest{}, fmt.Errorf("unsupported structural search %q; try forms like 'who uses X', 'what does X depend on', 'impact of X', 'path A -> B', or 'explain X'", raw)
}

func parsePathQuery(raw string) (types.QueryRequest, bool) {
	for _, pattern := range []*regexp.Regexp{pathArrowPattern, pathFromPattern, pathHowPattern} {
		match := pattern.FindStringSubmatch(raw)
		if len(match) != 3 {
			continue
		}
		subject := normalizeSearchTerm(match[1])
		target := normalizeSearchTerm(match[2])
		if subject == "" || target == "" {
			return types.QueryRequest{}, false
		}
		return types.QueryRequest{Kind: types.QueryKindPath, Subject: subject, Target: target}, true
	}
	return types.QueryRequest{}, false
}

func parseSingleSubjectQuery(raw string) (types.QueryRequest, bool) {
	patterns := []struct {
		pattern *regexp.Regexp
		kind    types.QueryKind
	}{
		{pattern: explainPattern, kind: types.QueryKindExplain},
		{pattern: reverseUsedPattern, kind: types.QueryKindReverseDependencies},
		{pattern: reverseWhoUsesPattern, kind: types.QueryKindReverseDependencies},
		{pattern: reverseDepsPattern, kind: types.QueryKindReverseDependencies},
		{pattern: reverseWhatDependsPattern, kind: types.QueryKindReverseDependencies},
		{pattern: depsWhatDoesPattern, kind: types.QueryKindDependencies},
		{pattern: depsOfPattern, kind: types.QueryKindDependencies},
		{pattern: impactOfPattern, kind: types.QueryKindImpact},
		{pattern: impactBreaksPattern, kind: types.QueryKindImpact},
		{pattern: impactAffectedPattern, kind: types.QueryKindImpact},
	}

	for _, candidate := range patterns {
		match := candidate.pattern.FindStringSubmatch(raw)
		if len(match) != 2 {
			continue
		}
		subject := normalizeSearchTerm(match[1])
		if subject == "" {
			return types.QueryRequest{}, false
		}
		return types.QueryRequest{Kind: candidate.kind, Subject: subject}, true
	}

	return types.QueryRequest{}, false
}

func normalizeSearchTerm(term string) string {
	term = strings.TrimSpace(term)
	term = strings.TrimRight(term, "?.! ")
	if len(term) >= 2 {
		if (strings.HasPrefix(term, `"`) && strings.HasSuffix(term, `"`)) || (strings.HasPrefix(term, "'") && strings.HasSuffix(term, "'")) || (strings.HasPrefix(term, "`") && strings.HasSuffix(term, "`")) {
			term = term[1 : len(term)-1]
		}
	}
	return strings.TrimSpace(term)
}
