package security

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

const maxLabelLen = 256

// ValidatePath checks that target is contained within root (prevents path
// traversal). Both paths are resolved to absolute paths before comparison.
func ValidatePath(root, target string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolving root: %w", err)
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolving target: %w", err)
	}

	// Ensure absTarget starts with absRoot + separator (or equals absRoot)
	if absTarget != absRoot && !strings.HasPrefix(absTarget, absRoot+string(filepath.Separator)) {
		return fmt.Errorf("path %q escapes root %q", target, root)
	}
	return nil
}

// SanitizeLabel strips non-printable characters from label and truncates it
// to maxLabelLen runes.
func SanitizeLabel(label string) string {
	var b strings.Builder
	for _, r := range label {
		if unicode.IsPrint(r) {
			b.WriteRune(r)
		}
	}
	result := b.String()
	runes := []rune(result)
	if len(runes) > maxLabelLen {
		return string(runes[:maxLabelLen])
	}
	return result
}
