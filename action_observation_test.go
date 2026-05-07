package theater

import (
	"testing"
	"unicode/utf8"
)

func TestObservedPreviewTextKeepsUTF8Boundaries(t *testing.T) {
	t.Parallel()

	preview, truncated := observedPreviewText("абвгд", 7)
	if !truncated {
		t.Fatal("expected observed preview to be truncated")
	}

	if got, want := preview, "а...д"; got != want {
		t.Fatalf("observed preview mismatch: got %q want %q", got, want)
	}

	if !utf8.ValidString(preview) {
		t.Fatalf("observed preview must keep UTF-8 valid: %q", preview)
	}
}
