package streamtext

import (
	"testing"
	"unicode/utf8"
)

func TestRenderPreservesUTF8AndEscapesUnsafeBytes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		data []byte
		want string
	}{
		{name: "utf8", data: []byte("cafe\u0301"), want: "cafe\u0301"},
		{name: "tab", data: []byte("a\tb"), want: "a\\tb"},
		{name: "controls", data: []byte{'a', 0x00, 'b', 0x7f}, want: "a\\x00b\\x7F"},
		{name: "invalid bytes", data: []byte{'o', 0xff, 0xc3}, want: "o\\xFF\\xC3"},
		{name: "replacement rune", data: []byte("�"), want: "�"},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := Render(tt.data); got != tt.want {
				t.Fatalf("render mismatch: got %q want %q", got, tt.want)
			}
		})
	}
}

func TestSafePrefixLenAvoidsSplittingUTF8WhenPossible(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		data  []byte
		limit int
		want  int
	}{
		{name: "full prefix", data: []byte("abcdef"), limit: 8, want: 6},
		{name: "before multibyte rune", data: []byte("ab€x"), limit: 4, want: 2},
		{name: "invalid byte still makes progress", data: []byte{0xff, 'a'}, limit: 1, want: 1},
		{name: "small limit forces one byte", data: []byte("éx"), limit: 1, want: 1},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := SafePrefixLen(tt.data, tt.limit); got != tt.want {
				t.Fatalf("prefix length mismatch: got %d want %d", got, tt.want)
			}
		})
	}
}

func TestTruncateSuffixPreservesUTF8Boundaries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		text      string
		limit     int
		want      string
		truncated bool
	}{
		{name: "no truncation", text: "abcdef", limit: 6, want: "abcdef"},
		{name: "cyrillic prefix", text: "абвгд", limit: 5, want: "аб...", truncated: true},
		{name: "emoji prefix", text: "🙂🙂🙂", limit: 5, want: "🙂...", truncated: true},
		{name: "zero limit", text: "абв", limit: 0, want: "...", truncated: true},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, truncated := TruncateSuffix(tt.text, tt.limit, "...")
			if got != tt.want || truncated != tt.truncated {
				t.Fatalf("truncate suffix mismatch: got (%q, %t) want (%q, %t)", got, truncated, tt.want, tt.truncated)
			}

			if !utf8.ValidString(got) {
				t.Fatalf("truncate suffix must keep UTF-8 valid: %q", got)
			}
		})
	}
}

func TestTruncateMiddlePreservesUTF8Boundaries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		text      string
		limit     int
		want      string
		truncated bool
	}{
		{name: "no truncation", text: "abcdef", limit: 6, want: "abcdef"},
		{name: "cyrillic middle", text: "абвгд", limit: 7, want: "а...д", truncated: true},
		{name: "ascii around multibyte", text: "ab🙂yz", limit: 6, want: "a...yz", truncated: true},
		{name: "marker only", text: "абв", limit: 2, want: "..", truncated: true},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, truncated := TruncateMiddle(tt.text, tt.limit, "...")
			if got != tt.want || truncated != tt.truncated {
				t.Fatalf("truncate middle mismatch: got (%q, %t) want (%q, %t)", got, truncated, tt.want, tt.truncated)
			}

			if !utf8.ValidString(got) {
				t.Fatalf("truncate middle must keep UTF-8 valid: %q", got)
			}
		})
	}
}
