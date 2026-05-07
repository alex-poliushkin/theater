package thtr

import (
	"bytes"
	"encoding/json"
	"io"
	"os"

	"github.com/alex-poliushkin/theater"
)

type LoadResult struct {
	Spec theater.StageSpec

	sourceMap *sourceMap
	yamlData  []byte
}

func (r LoadResult) CanonicalYAML() []byte {
	if len(r.yamlData) == 0 {
		return nil
	}

	return append([]byte(nil), r.yamlData...)
}

func (r LoadResult) MarshalSourceMap() ([]byte, error) {
	if r.sourceMap == nil {
		return nil, nil
	}

	return json.MarshalIndent(r.sourceMap, "", "  ")
}

func (r LoadResult) RewriteDiagnostics(diagnostics []theater.Diagnostic) []theater.Diagnostic {
	if len(diagnostics) == 0 {
		return nil
	}

	rewritten := make([]theater.Diagnostic, len(diagnostics))
	copy(rewritten, diagnostics)
	if r.sourceMap == nil {
		return rewritten
	}

	for i := range rewritten {
		entry, ok := r.sourceMap.LookupSpecPath(rewritten[i].Path)
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

func Decode(reader io.Reader, matchers theater.MatcherSugarResolver) (theater.StageSpec, error) {
	result, err := DecodeDetailed(reader, "", matchers)
	if err != nil {
		return theater.StageSpec{}, err
	}

	return result.Spec, nil
}

func Parse(data []byte, matchers theater.MatcherSugarResolver) (theater.StageSpec, error) {
	result, err := ParseDetailed(data, "", matchers)
	if err != nil {
		return theater.StageSpec{}, err
	}

	return result.Spec, nil
}

func LoadFile(path string, matchers theater.MatcherSugarResolver) (theater.StageSpec, error) {
	result, err := LoadFileDetailed(path, matchers)
	if err != nil {
		return theater.StageSpec{}, err
	}

	return result.Spec, nil
}

func DecodeDetailed(reader io.Reader, sourceFile string, matchers theater.MatcherSugarResolver) (LoadResult, error) {
	_ = matchers
	return decodeWithSource(reader, sourceFile, matchers)
}

func ParseDetailed(data []byte, sourceFile string, matchers theater.MatcherSugarResolver) (LoadResult, error) {
	_ = matchers
	return decodeWithSource(bytes.NewReader(data), sourceFile, matchers)
}

func LoadFileDetailed(path string, matchers theater.MatcherSugarResolver) (LoadResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LoadResult{}, err
	}

	return decodeWithSource(bytes.NewReader(data), path, matchers)
}

func LoadFlowFileDetailed(path string, matchers theater.MatcherSugarResolver) (LoadResult, error) {
	loader := newFlowFileLoader(path, matchers)
	return loader.LoadDetailed()
}

func decodeWithSource(reader io.Reader, sourceFile string, matchers theater.MatcherSugarResolver) (LoadResult, error) {
	_ = matchers
	data, err := io.ReadAll(reader)
	if err != nil {
		return LoadResult{}, err
	}

	tokens, err := lex(data)
	if err != nil {
		return LoadResult{}, newDiagnosticError(sourceFile, "thtr_lex_error", "", err)
	}
	document, err := parseTokens(tokens)
	if err != nil {
		return LoadResult{}, newDiagnosticError(sourceFile, "thtr_parse_error", "", err)
	}

	lowered, err := lowerDocumentWithSourceMap(document, sourceFile)
	if err != nil {
		return LoadResult{}, newDiagnosticError(
			sourceFile,
			"thtr_lower_error",
			nearestSyntaxPath(document, errorSpan(err)),
			err,
		)
	}

	return LoadResult{
		Spec:      lowered.Spec,
		sourceMap: lowered.SourceMap,
		yamlData:  lowered.YAML,
	}, nil
}
