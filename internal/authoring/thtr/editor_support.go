package thtr

import (
	"path/filepath"
	"strings"

	"github.com/alex-poliushkin/theater"
	authoringyaml "github.com/alex-poliushkin/theater/internal/authoring/yaml"
)

// LexToken exposes one `.thtr` token for editor tooling.
type LexToken struct {
	Kind        string
	Text        string
	StartOffset int
	EndOffset   int
	StartLine   int
	StartColumn int
	EndLine     int
	EndColumn   int
}

// AnalyzePathDetailed lowers one `.thtr` buffer using the same path semantics
// as CLI loading. Flow files under `theater/flows` reuse repo-local library
// files while the current buffer content comes from memory.
func AnalyzePathDetailed(path string, data []byte, matchers theater.MatcherSugarResolver) (LoadResult, error) {
	return AnalyzePathDetailedWithLibraryOverlay(path, data, matchers, nil)
}

// AnalyzePathDetailedWithLibraryOverlay is the editor-analysis variant of
// AnalyzePathDetailed. Flow files still load through repo-aware semantics, but
// open library buffers may override their on-disk source for the analysis pass.
func AnalyzePathDetailedWithLibraryOverlay(
	path string,
	data []byte,
	matchers theater.MatcherSugarResolver,
	libraryOverlay map[string][]byte,
) (LoadResult, error) {
	location, err := authoringyaml.ResolveFlowFileLocation(path)
	if err == nil {
		if location.InFlowRoot {
			return LoadFlowSourceDetailedWithLibraryOverlay(location.Path, data, matchers, libraryOverlay)
		}

		return ParseDetailed(data, location.Path, matchers)
	}

	absPath, absErr := filepath.Abs(path)
	if absErr == nil {
		path = absPath
	}

	return ParseDetailed(data, path, matchers)
}

// LibraryChangeAffectsFlowDocument reports whether a changed file belongs to
// the repo-local `.thtr` library tree that can contribute scenarios to the
// provided flow file. It intentionally models existing repo files; unsaved new
// library files are outside the current overlay contract because repo discovery
// is still convention-based and disk-indexed.
func LibraryChangeAffectsFlowDocument(flowPath, changedPath string) bool {
	flowLocation, err := authoringyaml.ResolveFlowFileLocation(flowPath)
	if err != nil || !flowLocation.RepoFound || !flowLocation.InFlowRoot {
		return false
	}

	changedLocation, err := authoringyaml.ResolveFlowFileLocation(changedPath)
	if err != nil || !changedLocation.RepoFound {
		return false
	}
	if filepath.Clean(flowLocation.Layout.LibraryRoot) != filepath.Clean(changedLocation.Layout.LibraryRoot) {
		return false
	}

	return pathWithinRoot(changedLocation.Path, flowLocation.Layout.LibraryRoot)
}

// LoadFlowSourceDetailed lowers one flow file from memory while still loading
// referenced library scenarios from disk.
func LoadFlowSourceDetailed(path string, data []byte, matchers theater.MatcherSugarResolver) (LoadResult, error) {
	return LoadFlowSourceDetailedWithLibraryOverlay(path, data, matchers, nil)
}

// LoadFlowSourceDetailedWithLibraryOverlay lowers one flow file from memory
// while loading referenced library scenarios from disk unless an editor overlay
// provides newer in-memory content for an existing library file discovered from
// the repo-local library tree.
func LoadFlowSourceDetailedWithLibraryOverlay(
	path string,
	data []byte,
	matchers theater.MatcherSugarResolver,
	libraryOverlay map[string][]byte,
) (LoadResult, error) {
	location, err := authoringyaml.ResolveFlowFileLocation(path)
	if err != nil {
		return LoadResult{}, err
	}
	if !location.RepoFound {
		return LoadResult{}, errFlowRepoNotFound(location.Path)
	}
	if !location.InFlowRoot {
		return LoadResult{}, errFlowOutsideRoot(location.Path, location.Layout.FlowRoot)
	}

	flowResult, err := ParseDetailed(data, location.Path, matchers)
	if err != nil {
		return LoadResult{}, err
	}

	neededScenarioIDs := unresolvedFlowScenarioIDs(flowResult.Spec)
	if len(neededScenarioIDs) == 0 {
		return LoadResult{
			Spec:      cloneFlowStage(flowResult.Spec),
			sourceMap: cloneSourceMap(flowResult.sourceMap),
		}, nil
	}

	libraryFiles, err := collectFlowLibraryFiles(location.Layout.LibraryRoot)
	if err != nil {
		return LoadResult{}, err
	}

	overlay := normalizeSourceOverlay(libraryOverlay)
	index, err := buildFlowLibraryIndexWithOverlay(libraryFiles, overlay)
	if err != nil {
		return LoadResult{}, err
	}

	return assembleFlowStageDetailed(flowResult, neededScenarioIDs, index, matchers, overlay)
}

func pathWithinRoot(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}

	return relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// Tokenize exposes `.thtr` lexer output for editor tooling.
func Tokenize(data []byte) ([]LexToken, error) {
	return TokenizeSource(data, "")
}

// TokenizeSource exposes `.thtr` lexer output and attaches sourceFile to
// structured lexer diagnostics.
func TokenizeSource(data []byte, sourceFile string) ([]LexToken, error) {
	tokens, err := lex(data)
	if err != nil {
		return nil, newDiagnosticError(sourceFile, "thtr_lex_error", "", err)
	}

	result := make([]LexToken, 0, len(tokens))
	for i := range tokens {
		token := tokens[i]
		if token.Kind == tokenEOF {
			continue
		}

		result = append(result, LexToken{
			Kind:        string(token.Kind),
			Text:        token.Text,
			StartOffset: token.Span.Start.Offset,
			EndOffset:   token.Span.End.Offset,
			StartLine:   token.Span.Start.Line,
			StartColumn: token.Span.Start.Column,
			EndLine:     token.Span.End.Line,
			EndColumn:   token.Span.End.Column,
		})
	}

	return result, nil
}
