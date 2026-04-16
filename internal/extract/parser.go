package extract

import (
	"regexp"
	"strings"

	"github.com/Syfra3/vela/pkg/types"
)

// ---------------------------------------------------------------------------
// Explicit reference parser (spec §6.4 / task 14)
// ---------------------------------------------------------------------------
//
// ParseReferences extracts structured references from an observation's title
// and content without requiring an LLM. It uses lightweight heuristics:
//
//   - File paths:   tokens that look like path/to/file.ext
//   - Go identifiers in code-fence blocks (FunctionName, StructName, etc.)
//   - Concept names: [[concept]] wiki-link notation
//   - Observation IDs: "#123" or "obs:123" patterns
//
// Results are deduplicated before being returned.

var (
	// filePathRe matches strings that look like relative file paths with extensions.
	filePathRe = regexp.MustCompile(`(?:^|[\s(\["])([a-zA-Z0-9_.\-]+(?:/[a-zA-Z0-9_.\-]+)+\.[a-zA-Z]{1,6})`)

	// goIdentRe matches Go-style PascalCase or camelCase identifiers inside
	// backtick code spans (e.g. `MyFunc`, `myVar`).
	// Uses a regular string (not raw) because the pattern itself contains backticks.
	goIdentRe = regexp.MustCompile("`([A-Z][a-zA-Z0-9_]+(?:[.][A-Z][a-zA-Z0-9_]+)*)`")

	// wikiLinkRe matches [[ConceptName]] wiki-link notation.
	wikiLinkRe = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)

	// obsIDRe matches "#123" or "obs:123" patterns.
	obsIDRe = regexp.MustCompile(`(?:^|[\s,])(?:#|obs:)(\d+)`)
)

// ParseReferences extracts explicit references from observation text.
// It is intentionally conservative — false positives create noise in the graph.
func ParseReferences(title, content string) []types.ObsReference {
	text := title + "\n" + content
	seen := make(map[string]bool)
	var refs []types.ObsReference

	add := func(refType, target string) {
		target = strings.TrimSpace(target)
		if target == "" {
			return
		}
		key := refType + ":" + target
		if !seen[key] {
			seen[key] = true
			refs = append(refs, types.ObsReference{Type: refType, Target: target})
		}
	}

	// File paths
	for _, m := range filePathRe.FindAllStringSubmatch(text, -1) {
		if len(m) >= 2 {
			add("file", m[1])
		}
	}

	// Go identifiers in backtick spans
	for _, m := range goIdentRe.FindAllStringSubmatch(text, -1) {
		if len(m) >= 2 {
			add("function", m[1])
		}
	}

	// Wiki-link concepts
	for _, m := range wikiLinkRe.FindAllStringSubmatch(text, -1) {
		if len(m) >= 2 {
			add("concept", m[1])
		}
	}

	// Observation ID references
	for _, m := range obsIDRe.FindAllStringSubmatch(text, -1) {
		if len(m) >= 2 {
			add("observation", m[1])
		}
	}

	return refs
}
