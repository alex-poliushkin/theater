package thtr

import (
	"testing"

	"github.com/alex-poliushkin/theater/internal/testkit/errtest"
)

func TestLexEmitsIndentAndDedentTokens(t *testing.T) {
	t.Parallel()

	tokens, err := lex([]byte("stage smoke\n  scenario login\n    act submit\n"))
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	requireTokenKinds(t, tokens,
		tokenIdentifier,
		tokenIdentifier,
		tokenNewline,
		tokenIndent,
		tokenIdentifier,
		tokenIdentifier,
		tokenNewline,
		tokenIndent,
		tokenIdentifier,
		tokenIdentifier,
		tokenNewline,
		tokenDedent,
		tokenDedent,
		tokenEOF,
	)
}

func TestLexCommentOnlyLineDoesNotChangeIndentation(t *testing.T) {
	t.Parallel()

	tokens, err := lex([]byte("stage smoke\n  # comment\n  scenario login\n"))
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	requireTokenKinds(t, tokens,
		tokenIdentifier,
		tokenIdentifier,
		tokenNewline,
		tokenComment,
		tokenNewline,
		tokenIndent,
		tokenIdentifier,
		tokenIdentifier,
		tokenNewline,
		tokenDedent,
		tokenEOF,
	)
}

func TestLexIgnoresIndentedNewlinesInsideGrouping(t *testing.T) {
	t.Parallel()

	tokens, err := lex([]byte("call run = login(\n  email: $user,\n)\n"))
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	requireNoTokenKind(t, tokens, tokenIndent)
	requireNoTokenKind(t, tokens, tokenDedent)
}

func TestLexRejectsLeadingTabs(t *testing.T) {
	t.Parallel()

	_, err := lex([]byte("stage smoke\n\tact bad\n"))

	errtest.RequireContains(t, err, "leading tabs are not allowed")
}

func TestLexEmitsStringTokens(t *testing.T) {
	t.Parallel()

	tokens, err := lex([]byte("\"plain\" r\"raw\" \"\"\"multi\nline\"\"\"\n"))
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	requireTokenKinds(t, tokens,
		tokenString,
		tokenRawString,
		tokenMultilineString,
		tokenNewline,
		tokenEOF,
	)
}

func TestLexEmitsDurationToken(t *testing.T) {
	t.Parallel()

	tokens, err := lex([]byte("eventually 30s every 1s\n"))
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	requireTokenKinds(t, tokens,
		tokenIdentifier,
		tokenDuration,
		tokenIdentifier,
		tokenDuration,
		tokenNewline,
		tokenEOF,
	)
}

func TestLexEmitsComparisonOperatorTokens(t *testing.T) {
	t.Parallel()

	tokens, err := lex([]byte("expect cmp: field(status_code) >= 500\nexpect low: field(count) < 10\n"))
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	requireTokenKinds(t, tokens,
		tokenIdentifier,
		tokenIdentifier,
		tokenColon,
		tokenIdentifier,
		tokenLParen,
		tokenIdentifier,
		tokenRParen,
		tokenGreater,
		tokenEqual,
		tokenNumber,
		tokenNewline,
		tokenIdentifier,
		tokenIdentifier,
		tokenColon,
		tokenIdentifier,
		tokenLParen,
		tokenIdentifier,
		tokenRParen,
		tokenLess,
		tokenNumber,
		tokenNewline,
		tokenEOF,
	)
}

func TestLexRejectsNonASCIIdentifiers(t *testing.T) {
	t.Parallel()

	_, err := lex([]byte("stage смоук\n"))

	errtest.RequireContains(t, err, "unexpected character")
}

func TestLexRejectsInvalidDurationLiteral(t *testing.T) {
	t.Parallel()

	_, err := lex([]byte("eventually 12abc every 1s\n"))

	errtest.RequireContains(t, err, `invalid duration literal "12abc"`)
}

func TestLexRejectsUnexpectedClosingBracket(t *testing.T) {
	t.Parallel()

	_, err := lex([]byte(")\n"))

	errtest.RequireContains(t, err, `unexpected closing bracket ")"`)
}

func TestLexPreservesCommentSpan(t *testing.T) {
	t.Parallel()

	tokens, err := lex([]byte("stage smoke # trailing\n"))
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	comment := findToken(t, tokens, tokenComment)
	if got, want := comment.Span.Start.Line, 1; got != want {
		t.Fatalf("comment start line mismatch: got %d want %d", got, want)
	}
	if got, want := comment.Span.Start.Column, 13; got != want {
		t.Fatalf("comment start column mismatch: got %d want %d", got, want)
	}
}

func requireTokenKinds(t *testing.T, tokens []token, want ...tokenKind) {
	t.Helper()

	if len(tokens) != len(want) {
		t.Fatalf("token count mismatch: got %d want %d", len(tokens), len(want))
	}

	for i := range want {
		if got := tokens[i].Kind; got != want[i] {
			t.Fatalf("token %d kind mismatch: got %s want %s", i, got, want[i])
		}
	}
}

func requireNoTokenKind(t *testing.T, tokens []token, want tokenKind) {
	t.Helper()

	for _, token := range tokens {
		if token.Kind == want {
			t.Fatalf("unexpected token kind %s present", want)
		}
	}
}

func findToken(t *testing.T, tokens []token, kind tokenKind) token {
	t.Helper()

	for _, token := range tokens {
		if token.Kind == kind {
			return token
		}
	}

	t.Fatalf("token %s not found", kind)
	return token{}
}
