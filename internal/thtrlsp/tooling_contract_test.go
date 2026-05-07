package thtrlsp

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAnalyzeDocumentReportsToolingContractParseFixtures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		wantLine           int
		wantStartCharacter int
		wantMessage        string
	}{
		{
			name:               "parse-error-bad-indentation.thtr",
			wantLine:           1,
			wantStartCharacter: 0,
			wantMessage:        "expected scenario, call, or end of file",
		},
		{
			name:               "parse-error-incomplete-paren.thtr",
			wantLine:           3,
			wantStartCharacter: 47,
			wantMessage:        "expected )",
		},
		{
			name:               "parse-error-malformed-clause.thtr",
			wantLine:           6,
			wantStartCharacter: 6,
			wantMessage:        `expected "," or ")" after relative clause`,
		},
		{
			name:               "parse-error-quoted-core-id.thtr",
			wantLine:           0,
			wantStartCharacter: 6,
			wantMessage:        "quoted core identifiers are not supported; use an unquoted identifier",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			path := toolingContractFixturePath(t, test.name)
			text := string(readToolingContractFixture(t, test.name))
			grouped := testAnalyzeDocument(t, path, text)
			diagnostics := grouped[path]
			if got, want := len(diagnostics), 1; got != want {
				t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
			}
			if got, want := diagnostics[0].Code, "thtr_parse_error"; got != want {
				t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
			}
			if got, want := diagnostics[0].Message, test.wantMessage; got != want {
				t.Fatalf("diagnostic message mismatch: got %q want %q", got, want)
			}
			if got, want := diagnostics[0].Range.Start.Line, test.wantLine; got != want {
				t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
			}
			if got, want := diagnostics[0].Range.Start.Character, test.wantStartCharacter; got != want {
				t.Fatalf("diagnostic start character mismatch: got %d want %d", got, want)
			}
			if !lspPositionAfter(diagnostics[0].Range.End, diagnostics[0].Range.Start) {
				t.Fatalf("diagnostic range must be non-empty: %#v", diagnostics[0].Range)
			}
		})
	}
}

func lspPositionAfter(right, left lspPosition) bool {
	return right.Line > left.Line ||
		right.Line == left.Line && right.Character > left.Character
}

func toolingContractFixturePath(t *testing.T, name string) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve fixture path failed")
	}

	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "thtr-tooling-contract", name)
}

func readToolingContractFixture(t *testing.T, name string) []byte {
	t.Helper()

	data, err := os.ReadFile(toolingContractFixturePath(t, name))
	if err != nil {
		t.Fatalf("read fixture %s failed: %v", name, err)
	}

	return data
}
