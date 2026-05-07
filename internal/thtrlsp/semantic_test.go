package thtrlsp

import (
	"strings"
	"testing"

	authoringthtr "github.com/alex-poliushkin/theater/internal/authoring/thtr"
)

func TestSemanticTokenTypeRecognizesScalarUnaryExpectationSurface(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		token     authoringthtr.LexToken
		wantType  int
		wantMatch bool
	}{
		{
			name:      "contains keyword",
			token:     authoringthtr.LexToken{Kind: "identifier", Text: "contains"},
			wantType:  0,
			wantMatch: true,
		},
		{
			name:      "not keyword",
			token:     authoringthtr.LexToken{Kind: "identifier", Text: "not"},
			wantType:  0,
			wantMatch: true,
		},
		{
			name:      "has keyword",
			token:     authoringthtr.LexToken{Kind: "identifier", Text: "has"},
			wantType:  0,
			wantMatch: true,
		},
		{
			name:      "key keyword",
			token:     authoringthtr.LexToken{Kind: "identifier", Text: "key"},
			wantType:  0,
			wantMatch: true,
		},
		{
			name:      "greater operator",
			token:     authoringthtr.LexToken{Kind: ">", Text: ">"},
			wantType:  4,
			wantMatch: true,
		},
		{
			name:      "less operator",
			token:     authoringthtr.LexToken{Kind: "<", Text: "<"},
			wantType:  4,
			wantMatch: true,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			gotType, gotMatch := semanticTokenType(testCase.token)
			if gotMatch != testCase.wantMatch {
				t.Fatalf("semantic token match mismatch: got %t want %t", gotMatch, testCase.wantMatch)
			}
			if gotType != testCase.wantType {
				t.Fatalf("semantic token type mismatch: got %d want %d", gotType, testCase.wantType)
			}
		})
	}
}

func TestSemanticTokenTypeRecognizesStateErgonomicsSurface(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		token     authoringthtr.LexToken
		wantType  int
		wantMatch bool
	}{
		{name: "backend keyword", token: authoringthtr.LexToken{Kind: "identifier", Text: "backend"}, wantType: 0, wantMatch: true},
		{name: "record keyword", token: authoringthtr.LexToken{Kind: "identifier", Text: "record"}, wantType: 0, wantMatch: true},
		{name: "pool keyword", token: authoringthtr.LexToken{Kind: "identifier", Text: "pool"}, wantType: 0, wantMatch: true},
		{name: "read keyword", token: authoringthtr.LexToken{Kind: "identifier", Text: "read"}, wantType: 0, wantMatch: true},
		{name: "update keyword", token: authoringthtr.LexToken{Kind: "identifier", Text: "update"}, wantType: 0, wantMatch: true},
		{name: "claim keyword", token: authoringthtr.LexToken{Kind: "identifier", Text: "claim"}, wantType: 0, wantMatch: true},
		{name: "renew keyword", token: authoringthtr.LexToken{Kind: "identifier", Text: "renew"}, wantType: 0, wantMatch: true},
		{name: "release keyword", token: authoringthtr.LexToken{Kind: "identifier", Text: "release"}, wantType: 0, wantMatch: true},
		{name: "consume keyword", token: authoringthtr.LexToken{Kind: "identifier", Text: "consume"}, wantType: 0, wantMatch: true},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			gotType, gotMatch := semanticTokenType(testCase.token)
			if gotMatch != testCase.wantMatch {
				t.Fatalf("semantic token match mismatch: got %t want %t", gotMatch, testCase.wantMatch)
			}
			if gotType != testCase.wantType {
				t.Fatalf("semantic token type mismatch: got %d want %d", gotType, testCase.wantType)
			}
		})
	}
}

func TestCapabilityCompletionsIncludeExpectationNot(t *testing.T) {
	t.Parallel()

	completions := testCapabilityCompletions(t)
	for i := range completions {
		if completions[i].Label == "expectation.not" {
			return
		}
	}

	t.Fatal("capability completions must include expectation.not")
}

func TestCapabilityCompletionsIncludeStateErgonomicVerbs(t *testing.T) {
	t.Parallel()

	wantLabels := map[string]struct{}{
		"state.read":    {},
		"state.update":  {},
		"state.claim":   {},
		"state.renew":   {},
		"state.release": {},
		"state.consume": {},
	}

	completions := testCapabilityCompletions(t)
	for i := range completions {
		delete(wantLabels, completions[i].Label)
	}

	if len(wantLabels) != 0 {
		t.Fatalf("capability completions missing state ergonomics labels: %#v", wantLabels)
	}
}

func TestSemanticTokensForDocumentRecognizeScalarUnaryExpectationSurface(t *testing.T) {
	t.Parallel()

	text := `stage smoke
scenario login
  act submit
    do action.http(method: "GET", url: "/health")
    expect page-text: field(body) contains "Example Domain"
    expect not-server-error: field(status_code) not >= 500
`

	segments := decodeSemanticSegments(semanticTokensForDocument(text))
	if !hasSemanticSegmentAt(text, segments, "contains", 0) {
		t.Fatal(`semantic tokens must classify "contains" as keyword`)
	}
	if !hasSemanticSegmentAt(text, segments, "not", 0) {
		t.Fatal(`semantic tokens must classify "not" as keyword`)
	}
	if !hasSemanticSegmentAt(text, segments, ">", 4) {
		t.Fatal(`semantic tokens must classify ">" as operator`)
	}
}

func TestSemanticTokensForDocumentRecognizeCollectionWhereKeywords(t *testing.T) {
	t.Parallel()

	text := `stage smoke
scenario login
  act submit
    do action.http(method: "GET", url: "/health")
    expect receiver-present: field(body) | decode(json) | path("/notifications") all items where path("/receiverAddress") contains "@example.test"
`

	segments := decodeSemanticSegments(semanticTokensForDocument(text))
	for _, keyword := range []string{"all", "items", "where"} {
		if !hasSemanticSegmentAt(text, segments, keyword, 0) {
			t.Fatalf("semantic tokens must classify %q as keyword", keyword)
		}
	}
}

func TestSemanticTokensForDocumentRecognizeStateErgonomicsSurface(t *testing.T) {
	t.Parallel()

	text := `stage smoke
state
  backend local = state.backend.file(root: "/tmp/theater-state")
  record shared_meta = state.record
    backend: local
    record: "env/shared-meta"
    min_guarantee: local-atomic
  pool otp_identities = state.pool
    backend: local
    pool: "otp-identities"
    min_guarantee: local-atomic
scenario verify-state
  act claim-item
    do state.claim
      pool: otp_identities
      lease:
        ttl: 5m
  act release-item
    do state.release(claim: $otp_claim)
`

	segments := decodeSemanticSegments(semanticTokensForDocument(text))
	for _, keyword := range []string{"backend", "record", "pool", "claim", "release"} {
		if !hasSemanticSegmentAt(text, segments, keyword, 0) {
			t.Fatalf("semantic tokens must classify %q as keyword", keyword)
		}
	}
}

type decodedSemanticSegment struct {
	line      int
	character int
	length    int
	tokenType int
}

func decodeSemanticSegments(tokens lspSemanticTokens) []decodedSemanticSegment {
	segments := make([]decodedSemanticSegment, 0, len(tokens.Data)/5)
	line := 0
	character := 0

	for i := 0; i+4 < len(tokens.Data); i += 5 {
		line += tokens.Data[i]
		if tokens.Data[i] == 0 {
			character += tokens.Data[i+1]
		} else {
			character = tokens.Data[i+1]
		}

		segments = append(segments, decodedSemanticSegment{
			line:      line,
			character: character,
			length:    tokens.Data[i+2],
			tokenType: tokens.Data[i+3],
		})
	}

	return segments
}

func hasSemanticSegmentAt(text string, segments []decodedSemanticSegment, needle string, wantType int) bool {
	lines := strings.Split(text, "\n")
	for lineIndex := range lines {
		line := lines[lineIndex]
		offset := 0
		for {
			column := strings.Index(line[offset:], needle)
			if column < 0 {
				break
			}
			column += offset
			for i := range segments {
				if segments[i].line == lineIndex && segments[i].character == column && segments[i].tokenType == wantType {
					return true
				}
			}
			offset = column + len(needle)
		}
	}

	return false
}
