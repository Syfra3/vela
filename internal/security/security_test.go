package security

import (
	"strings"
	"testing"
)

func TestValidatePath_Valid(t *testing.T) {
	if err := ValidatePath("/tmp/root", "/tmp/root/subdir/file.go"); err != nil {
		t.Errorf("expected nil for valid path, got: %v", err)
	}
}

func TestValidatePath_Traversal(t *testing.T) {
	if err := ValidatePath("/tmp/root", "/tmp/other"); err == nil {
		t.Error("expected error for path outside root, got nil")
	}
}

func TestValidatePath_DotDot(t *testing.T) {
	if err := ValidatePath("/tmp/root", "/tmp/root/../other"); err == nil {
		t.Error("expected error for ../ traversal, got nil")
	}
}

func TestValidatePath_SameAsRoot(t *testing.T) {
	if err := ValidatePath("/tmp/root", "/tmp/root"); err != nil {
		t.Errorf("expected nil for root==target, got: %v", err)
	}
}

func TestSanitizeLabel_NonPrintable(t *testing.T) {
	input := "hello\x00world\x01\x1b"
	result := SanitizeLabel(input)
	if result != "helloworld" {
		t.Errorf("expected 'helloworld', got %q", result)
	}
}

func TestSanitizeLabel_TruncatesLongLabel(t *testing.T) {
	long := strings.Repeat("a", 300)
	result := SanitizeLabel(long)
	if len([]rune(result)) != maxLabelLen {
		t.Errorf("expected length %d, got %d", maxLabelLen, len([]rune(result)))
	}
}

func TestSanitizeLabel_NormalString(t *testing.T) {
	input := "MyFunction"
	result := SanitizeLabel(input)
	if result != input {
		t.Errorf("expected %q unchanged, got %q", input, result)
	}
}
