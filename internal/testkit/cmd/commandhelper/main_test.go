package main

import (
	"bytes"
	"io"
	"testing"
)

func TestWritePatternedStreamProducesDeterministicOutput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		pattern string
		size    int
		want    string
	}{
		{name: "empty output", pattern: "abc", size: 0, want: ""},
		{name: "single byte pattern", pattern: "x", size: 5, want: "xxxxx"},
		{name: "partial tail", pattern: "abc", size: 8, want: "abcabcab"},
		{name: "crosses scratch boundary", pattern: "stdout-", size: spamChunkSize + 5, want: repeatedPattern("stdout-", spamChunkSize+5)},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buffer bytes.Buffer
			if err := writePatternedStream(&buffer, tt.pattern, tt.size); err != nil {
				t.Fatalf("write patterned stream failed: %v", err)
			}

			if got := buffer.String(); got != tt.want {
				t.Fatalf("output mismatch: got %q want %q", got, tt.want)
			}
		})
	}
}

func TestWritePatternedStreamRejectsNegativeByteCount(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	err := writePatternedStream(&buffer, "abc", -1)
	if err == nil {
		t.Fatal("expected negative byte count error")
	}

	if got, want := err.Error(), "bytes must be non-negative"; got != want {
		t.Fatalf("error mismatch: got %q want %q", got, want)
	}
}

func BenchmarkWritePatternedStreamLarge(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if err := writePatternedStream(io.Discard, "abcdefg", 2_000_000); err != nil {
			b.Fatalf("write patterned stream failed: %v", err)
		}
	}
}

func repeatedPattern(pattern string, size int) string {
	if size <= 0 || pattern == "" {
		return ""
	}

	output := make([]byte, size)
	fillPatternedChunk(output, pattern, 0)
	return string(output)
}
