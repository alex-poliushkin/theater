package thtrlsp

import (
	"strings"

	authoringthtr "github.com/alex-poliushkin/theater/internal/authoring/thtr"
)

var semanticTokenTypes = []string{
	"keyword",
	"comment",
	"string",
	"number",
	"operator",
}

type semanticSegment struct {
	line      int
	character int
	length    int
	tokenType int
}

func semanticTokensForDocument(text string) lspSemanticTokens {
	tokens, err := authoringthtr.Tokenize([]byte(text))
	if err != nil {
		return lspSemanticTokens{}
	}

	segments := make([]semanticSegment, 0, len(tokens))
	for i := range tokens {
		segmentType, ok := semanticTokenType(tokens[i])
		if !ok {
			continue
		}

		start := tokens[i].StartOffset
		end := tokens[i].EndOffset
		if start >= end {
			continue
		}

		segments = append(segments, semanticSegmentsForRange(text, start, end, segmentType)...)
	}

	data := make([]int, 0, len(segments)*5)
	prevLine := 0
	prevCharacter := 0
	for i := range segments {
		deltaLine := segments[i].line - prevLine
		deltaCharacter := segments[i].character
		if deltaLine == 0 {
			deltaCharacter -= prevCharacter
		}

		data = append(data,
			deltaLine,
			deltaCharacter,
			max(segments[i].length, 1),
			segments[i].tokenType,
			0,
		)

		prevLine = segments[i].line
		prevCharacter = segments[i].character
	}

	return lspSemanticTokens{Data: data}
}

func semanticSegmentsForRange(text string, start, end, tokenType int) []semanticSegment {
	startPos := positionForOffset(text, start)
	endPos := positionForOffset(text, end)
	if startPos.Line == endPos.Line {
		return []semanticSegment{{
			line:      startPos.Line,
			character: startPos.Character,
			length:    max(endPos.Character-startPos.Character, 1),
			tokenType: tokenType,
		}}
	}

	offsets := lineOffsets(text)
	segments := make([]semanticSegment, 0, endPos.Line-startPos.Line+1)
	for line := startPos.Line; line <= endPos.Line; line++ {
		segmentStart := offsets[line]
		segmentEnd := len(text)
		if line+1 < len(offsets) {
			segmentEnd = offsets[line+1]
		}
		for segmentEnd > segmentStart && (text[segmentEnd-1] == '\n' || text[segmentEnd-1] == '\r') {
			segmentEnd--
		}
		if line == startPos.Line {
			segmentStart = start
		}
		if line == endPos.Line {
			segmentEnd = end
		}
		if segmentEnd <= segmentStart {
			continue
		}

		segmentStartPos := positionForOffset(text, segmentStart)
		segmentEndPos := positionForOffset(text, segmentEnd)
		segments = append(segments, semanticSegment{
			line:      line,
			character: segmentStartPos.Character,
			length:    max(segmentEndPos.Character-segmentStartPos.Character, 1),
			tokenType: tokenType,
		})
	}

	return segments
}

func semanticTokenType(token authoringthtr.LexToken) (int, bool) {
	switch token.Kind {
	case "comment":
		return 1, true
	case "string", "raw_string", "multiline_string":
		return 2, true
	case "number", "duration":
		return 3, true
	case "(", ")", "{", "}", "[", "]", ",", ":", "|", "=", ">", "<", "!", "->", "$", ".", "/":
		return 4, true
	case "identifier":
		if isSemanticKeyword(token.Text) {
			return 0, true
		}
	}

	return 0, false
}

func isSemanticKeyword(text string) bool {
	for _, keyword := range keywordSet() {
		if strings.EqualFold(keyword, text) {
			return true
		}
	}

	return false
}
