package thtr

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/alex-poliushkin/theater"
	authoringthtr "github.com/alex-poliushkin/theater/internal/authoring/thtr"
)

// SourceMapVersion is the source-map schema version emitted by this package.
const SourceMapVersion = "v1alpha1"

const (
	// TokenComment identifies a comment token.
	TokenComment TokenKind = "comment"
	// TokenDedent identifies a layout dedent token.
	TokenDedent TokenKind = "dedent"
	// TokenDuration identifies a duration literal token.
	TokenDuration TokenKind = "duration"
	// TokenIdentifier identifies an identifier token.
	TokenIdentifier TokenKind = "identifier"
	// TokenIndent identifies a layout indent token.
	TokenIndent TokenKind = "indent"
	// TokenLBracket identifies a left bracket token.
	TokenLBracket TokenKind = "["
	// TokenLBrace identifies a left brace token.
	TokenLBrace TokenKind = "{"
	// TokenLParen identifies a left parenthesis token.
	TokenLParen TokenKind = "("
	// TokenMultilineString identifies a multiline string token.
	TokenMultilineString TokenKind = "multiline_string"
	// TokenNewline identifies a newline token.
	TokenNewline TokenKind = "newline"
	// TokenNumber identifies a number token.
	TokenNumber TokenKind = "number"
	// TokenArrow identifies an arrow token.
	TokenArrow TokenKind = "->"
	// TokenBang identifies a bang token.
	TokenBang TokenKind = "!"
	// TokenColon identifies a colon token.
	TokenColon TokenKind = ":"
	// TokenComma identifies a comma token.
	TokenComma TokenKind = ","
	// TokenDot identifies a dot token.
	TokenDot TokenKind = "."
	// TokenDollar identifies a dollar token.
	TokenDollar TokenKind = "$"
	// TokenEqual identifies an equals token.
	TokenEqual TokenKind = "="
	// TokenGreater identifies a greater-than token.
	TokenGreater TokenKind = ">"
	// TokenLess identifies a less-than token.
	TokenLess TokenKind = "<"
	// TokenPipe identifies a pipe token.
	TokenPipe TokenKind = "|"
	// TokenRBracket identifies a right bracket token.
	TokenRBracket TokenKind = "]"
	// TokenRBrace identifies a right brace token.
	TokenRBrace TokenKind = "}"
	// TokenRawString identifies a raw string token.
	TokenRawString TokenKind = "raw_string"
	// TokenRParen identifies a right parenthesis token.
	TokenRParen TokenKind = ")"
	// TokenSlash identifies a slash token.
	TokenSlash TokenKind = "/"
	// TokenString identifies a quoted string token.
	TokenString TokenKind = "string"
)

// AnalyzeOptions configures `.thtr` editor analysis.
type AnalyzeOptions struct {
	// Path is the source path used for diagnostics, source maps, and repo-aware
	// flow loading. Leave empty for standalone in-memory snippets. Existing
	// flow and library paths are canonicalized before repo-aware analysis.
	Path string
	// LibraryOverlay replaces existing repo-local library files during
	// repo-aware flow analysis. Keys are absolute or relative file paths and
	// values are unsaved editor buffer contents.
	LibraryOverlay map[string][]byte
}

// Analysis is the public editor-tooling result for one `.thtr` analysis pass.
type Analysis struct {
	Spec          theater.StageSpec
	CanonicalYAML []byte
	SourceMap     SourceMap
}

// DiagnosticError exposes parse/lower diagnostics from `.thtr` tooling APIs.
type DiagnosticError struct {
	diagnostic theater.Diagnostic
}

// TokenKind identifies a stable lexer token category exposed to editors.
type TokenKind string

// Token exposes one `.thtr` lexer token. Offsets are byte offsets into the
// input, line and column coordinates are 1-based, Start* identifies the first
// token position, and End* identifies the position immediately after the token.
type Token struct {
	Kind        TokenKind
	Text        string
	StartOffset int
	EndOffset   int
	StartLine   int
	StartColumn int
	EndLine     int
	EndColumn   int
}

// SourceMap maps lowered StageSpec paths back to `.thtr` source and canonical
// YAML ranges. Version is SourceMapVersion for maps produced by this package.
type SourceMap struct {
	Version string           `json:"version" yaml:"version"`
	Entries []SourceMapEntry `json:"entries" yaml:"entries"`
}

// SourceMapEntry describes one mapped StageSpec node.
type SourceMapEntry struct {
	NodeID   string      `json:"node_id" yaml:"node_id"`
	SpecPath string      `json:"spec_path" yaml:"spec_path"`
	Source   SourceRange `json:"source" yaml:"source"`
	YAML     *YAMLRange  `json:"yaml,omitempty" yaml:"yaml,omitempty"`
}

// SourceRange identifies a `.thtr` source range. Line and column coordinates
// are 1-based, Start* is inclusive, and End* is the position immediately after
// the source construct.
type SourceRange struct {
	File        string `json:"file" yaml:"file"`
	StartLine   int    `json:"start_line" yaml:"start_line"`
	StartColumn int    `json:"start_column" yaml:"start_column"`
	EndLine     int    `json:"end_line" yaml:"end_line"`
	EndColumn   int    `json:"end_column" yaml:"end_column"`
}

// YAMLRange identifies a canonical YAML range. Line and column coordinates are
// 1-based and both endpoints participate in YAML position lookups.
type YAMLRange struct {
	StartLine   int `json:"start_line" yaml:"start_line"`
	StartColumn int `json:"start_column" yaml:"start_column"`
	EndLine     int `json:"end_line" yaml:"end_line"`
	EndColumn   int `json:"end_column" yaml:"end_column"`
}

// Analyze lowers `.thtr` bytes and returns StageSpec, canonical YAML, and
// source-map data for editor integrations.
func Analyze(data []byte, options AnalyzeOptions) (Analysis, error) {
	var result authoringthtr.LoadResult
	var err error
	if options.Path == "" {
		result, err = authoringthtr.ParseDetailed(data, "", nil)
	} else {
		result, err = authoringthtr.AnalyzePathDetailedWithLibraryOverlay(
			options.Path,
			data,
			nil,
			options.LibraryOverlay,
		)
	}
	if err != nil {
		return Analysis{}, wrapDiagnosticError(err)
	}

	return newAnalysis(result)
}

// AnalyzeFile reads and analyzes one `.thtr` file.
func AnalyzeFile(path string, options AnalyzeOptions) (Analysis, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Analysis{}, err
	}
	if options.Path == "" {
		options.Path = path
	}

	return Analyze(data, options)
}

// Code returns the diagnostic code.
func (e *DiagnosticError) Code() string {
	return e.diagnostic.Code
}

// Diagnostic returns the structured diagnostic behind this error.
func (e *DiagnosticError) Diagnostic() theater.Diagnostic {
	return e.diagnostic
}

// Error returns a compact diagnostic string with code and source location when
// available.
func (e *DiagnosticError) Error() string {
	diagnostic := e.diagnostic
	if diagnostic.Code == "" {
		return diagnostic.Summary
	}

	location := ""
	if diagnostic.Span.File != "" && diagnostic.Span.Line > 0 {
		location = fmt.Sprintf(" at %s:%d:%d", diagnostic.Span.File, diagnostic.Span.Line, diagnostic.Span.Column)
	}
	if diagnostic.Summary == "" {
		return diagnostic.Code + location
	}
	return fmt.Sprintf("%s%s: %s", diagnostic.Code, location, diagnostic.Summary)
}

// Span returns the source location attached to the diagnostic.
func (e *DiagnosticError) Span() theater.SourceRef {
	return e.diagnostic.Span
}

// Format formats `.thtr` source bytes.
func Format(data []byte) ([]byte, error) {
	formatted, err := authoringthtr.FormatSource(data, "")
	if err != nil {
		return nil, wrapDiagnosticError(err)
	}
	return formatted, nil
}

// FormatFile reads and formats one `.thtr` file without writing it back.
func FormatFile(path string) ([]byte, error) {
	formatted, err := authoringthtr.FormatFile(path)
	if err != nil {
		return nil, wrapDiagnosticError(err)
	}
	return formatted, nil
}

// LookupExactSpecPath returns the exact source-map entry for a StageSpec path.
func (m SourceMap) LookupExactSpecPath(path string) (SourceMapEntry, bool) {
	for i := range m.Entries {
		if m.Entries[i].SpecPath == path {
			return m.Entries[i], true
		}
	}
	return SourceMapEntry{}, false
}

// LookupSpecPath returns the source-map entry for path, falling back to the
// nearest recorded ancestor when the exact path has no entry.
func (m SourceMap) LookupSpecPath(path string) (SourceMapEntry, bool) {
	for candidate := path; candidate != ""; candidate = fallbackSpecPath(candidate) {
		if entry, ok := m.LookupExactSpecPath(candidate); ok {
			return entry, true
		}
	}
	return SourceMapEntry{}, false
}

// LookupYAMLPosition returns the smallest source-map entry containing a
// canonical YAML position.
func (m SourceMap) LookupYAMLPosition(line, column int) (SourceMapEntry, bool) {
	best := -1
	for i := range m.Entries {
		entry := m.Entries[i]
		if entry.YAML == nil || !yamlRangeContains(*entry.YAML, line, column) {
			continue
		}
		if best == -1 || yamlRangeSpan(*entry.YAML) < yamlRangeSpan(*m.Entries[best].YAML) {
			best = i
		}
	}
	if best == -1 {
		return SourceMapEntry{}, false
	}
	return m.Entries[best], true
}

// RewriteDiagnostics maps validation diagnostics from canonical StageSpec paths
// back to `.thtr` source spans.
func (a Analysis) RewriteDiagnostics(diagnostics []theater.Diagnostic) []theater.Diagnostic {
	if len(diagnostics) == 0 {
		return nil
	}

	rewritten := make([]theater.Diagnostic, len(diagnostics))
	copy(rewritten, diagnostics)
	for i := range rewritten {
		entry, ok := a.SourceMap.LookupSpecPath(rewritten[i].Path)
		if !ok {
			continue
		}
		rewritten[i].Span = theater.SourceRef{
			File:   entry.Source.File,
			Line:   entry.Source.StartLine,
			Column: entry.Source.StartColumn,
		}
	}

	return rewritten
}

// Tokenize returns `.thtr` lexer tokens with source coordinates.
func Tokenize(data []byte) ([]Token, error) {
	tokens, err := authoringthtr.TokenizeSource(data, "")
	if err != nil {
		return nil, wrapDiagnosticError(err)
	}

	result := make([]Token, 0, len(tokens))
	for i := range tokens {
		result = append(result, Token{
			Kind:        TokenKind(tokens[i].Kind),
			Text:        tokens[i].Text,
			StartOffset: tokens[i].StartOffset,
			EndOffset:   tokens[i].EndOffset,
			StartLine:   tokens[i].StartLine,
			StartColumn: tokens[i].StartColumn,
			EndLine:     tokens[i].EndLine,
			EndColumn:   tokens[i].EndColumn,
		})
	}
	return result, nil
}

func newAnalysis(result authoringthtr.LoadResult) (Analysis, error) {
	sourceMap, err := decodeSourceMap(result)
	if err != nil {
		return Analysis{}, err
	}

	return Analysis{
		Spec:          result.Spec,
		CanonicalYAML: result.CanonicalYAML(),
		SourceMap:     sourceMap,
	}, nil
}

func decodeSourceMap(result authoringthtr.LoadResult) (SourceMap, error) {
	raw, err := result.MarshalSourceMap()
	if err != nil {
		return SourceMap{}, err
	}
	if len(raw) == 0 {
		return SourceMap{}, nil
	}

	var sourceMap SourceMap
	if err := json.Unmarshal(raw, &sourceMap); err != nil {
		return SourceMap{}, err
	}
	if sourceMap.Version != SourceMapVersion {
		return SourceMap{}, fmt.Errorf("unsupported thtr source map version %q", sourceMap.Version)
	}
	return sourceMap, nil
}

func wrapDiagnosticError(err error) error {
	var diagnosticError *authoringthtr.DiagnosticError
	if errors.As(err, &diagnosticError) {
		return &DiagnosticError{diagnostic: diagnosticError.Diagnostic()}
	}
	return err
}

func fallbackSpecPath(path string) string {
	if path == "" {
		return ""
	}

	if index := strings.LastIndex(path, "["); index != -1 && strings.HasSuffix(path, "]") {
		return path[:index]
	}

	lastSlash := strings.LastIndex(path, "/")
	if lastSlash == -1 {
		if index := strings.LastIndex(path, "."); index != -1 {
			return path[:index]
		}
		return ""
	}

	segment := path[lastSlash+1:]
	if index := strings.LastIndex(segment, "."); index != -1 {
		return path[:lastSlash+1] + segment[:index]
	}

	return path[:lastSlash]
}

func yamlRangeContains(r YAMLRange, line, column int) bool {
	if line < r.StartLine || line > r.EndLine {
		return false
	}
	if line == r.StartLine && column < r.StartColumn {
		return false
	}
	if line == r.EndLine && column > r.EndColumn {
		return false
	}
	return true
}

func yamlRangeSpan(r YAMLRange) int {
	return (r.EndLine-r.StartLine)*10_000 + (r.EndColumn - r.StartColumn)
}
