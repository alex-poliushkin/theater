package thtrlsp

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

func pathFromURI(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}

	if parsed.Scheme == "" {
		return filepath.Abs(raw)
	}
	if !strings.EqualFold(parsed.Scheme, "file") {
		return "", fmt.Errorf("unsupported uri scheme %q", parsed.Scheme)
	}

	decodedPath, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return "", err
	}

	return filepath.Abs(filepath.FromSlash(decodedPath))
}

func uriFromPath(path string) string {
	return (&url.URL{
		Scheme: "file",
		Path:   filepath.ToSlash(path),
	}).String()
}

func fileText(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	return string(data)
}

func lineOffsets(text string) []int {
	offsets := []int{0}
	for index := 0; index < len(text); index++ {
		if text[index] == '\n' {
			offsets = append(offsets, index+1)
		}
	}

	return offsets
}

func clampOffset(text string, offset int) int {
	if offset < 0 {
		return 0
	}
	if offset > len(text) {
		return len(text)
	}
	return offset
}

func offsetForPosition(text string, position lspPosition) int {
	offsets := lineOffsets(text)
	if position.Line < 0 {
		return 0
	}
	if position.Line >= len(offsets) {
		return len(text)
	}

	start := offsets[position.Line]
	end := len(text)
	if position.Line+1 < len(offsets) {
		end = offsets[position.Line+1]
	}

	current := start
	consumed := 0
	for current < end {
		r, size := utf8.DecodeRuneInString(text[current:])
		if r == utf8.RuneError && size == 1 {
			size = 1
		}

		units := utf16RuneWidth(r)
		if consumed+units > position.Character {
			break
		}

		consumed += units
		current += size
	}

	return current
}

func positionForOffset(text string, offset int) lspPosition {
	offset = clampOffset(text, offset)
	offsets := lineOffsets(text)
	line := 0
	for line+1 < len(offsets) && offsets[line+1] <= offset {
		line++
	}

	start := offsets[line]
	character := 0
	for current := start; current < offset; {
		r, size := utf8.DecodeRuneInString(text[current:])
		if r == utf8.RuneError && size == 1 {
			size = 1
		}
		character += utf16RuneWidth(r)
		current += size
	}

	return lspPosition{Line: line, Character: character}
}

func rangeForOffsets(text string, start, end int) lspRange {
	return lspRange{
		Start: positionForOffset(text, start),
		End:   positionForOffset(text, end),
	}
}

func offsetForLineColumn(text string, line, column int) int {
	if line <= 0 || column <= 0 {
		return 0
	}

	offsets := lineOffsets(text)
	if line-1 >= len(offsets) {
		return len(text)
	}

	start := offsets[line-1]
	end := len(text)
	if line < len(offsets) {
		end = offsets[line]
	}

	current := start
	for count := 1; current < end && count < column; count++ {
		r, size := utf8.DecodeRuneInString(text[current:])
		if r == utf8.RuneError && size == 1 {
			size = 1
		}
		current += size
	}

	return current
}

func utf16RuneWidth(r rune) int {
	if r == utf8.RuneError {
		return 1
	}

	width := utf16.RuneLen(r)
	if width < 0 {
		return 1
	}

	return width
}
