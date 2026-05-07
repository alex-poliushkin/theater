package theatercli

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/alex-poliushkin/theater"
	authoringthtr "github.com/alex-poliushkin/theater/internal/authoring/thtr"
	authoringyaml "github.com/alex-poliushkin/theater/internal/authoring/yaml"
	theateryaml "github.com/alex-poliushkin/theater/yaml"
)

const (
	thtrFileExtension = ".thtr"
	yamlFileExtension = ".yaml"
	ymlFileExtension  = ".yml"
)

type stageFileLoader struct {
	matchers theater.MatcherSugarResolver
}

type stageLoadResult struct {
	Spec                 theater.StageSpec
	AuthoringDiagnostics []theater.Diagnostic
	ValidationHints      []theater.Diagnostic
	rewriteDiagnostics   func([]theater.Diagnostic) []theater.Diagnostic
}

func newStageFileLoader(matchers theater.MatcherSugarResolver) *stageFileLoader {
	return &stageFileLoader{matchers: matchers}
}

func (r stageLoadResult) RewriteDiagnostics(diagnostics []theater.Diagnostic) []theater.Diagnostic {
	if r.rewriteDiagnostics == nil {
		return diagnostics
	}

	return r.rewriteDiagnostics(diagnostics)
}

func (l *stageFileLoader) Load(path string) (stageLoadResult, error) {
	location, err := authoringyaml.ResolveFlowFileLocation(path)
	if err != nil {
		return stageLoadResult{}, err
	}

	switch strings.ToLower(filepath.Ext(location.Path)) {
	case yamlFileExtension, ymlFileExtension:
		return l.loadYAML(location)
	case thtrFileExtension:
		return l.loadTHTR(location)
	default:
		return stageLoadResult{}, errors.New("unsupported stage file extension")
	}
}

func (l *stageFileLoader) loadYAML(location authoringyaml.FlowFileLocation) (stageLoadResult, error) {
	spec, err := loadYAMLStageSpec(location, l.matchers)
	if err != nil {
		return stageLoadResult{}, err
	}

	hints, err := authoringyaml.LiteralWrapperHintsForLocation(location, l.matchers)
	if err != nil {
		return stageLoadResult{}, err
	}

	return stageLoadResult{
		Spec:               spec,
		ValidationHints:    hints,
		rewriteDiagnostics: identityDiagnostics,
	}, nil
}

func loadYAMLStageSpec(location authoringyaml.FlowFileLocation, matchers theater.MatcherSugarResolver) (theater.StageSpec, error) {
	switch strings.ToLower(filepath.Ext(location.Path)) {
	case yamlFileExtension, ymlFileExtension:
	default:
		return theater.StageSpec{}, errors.New("unsupported stage file extension")
	}

	if location.InFlowRoot {
		return theateryaml.LoadFlowFile(location.Path, matchers)
	}

	return theateryaml.LoadFile(location.Path, matchers)
}

func (l *stageFileLoader) loadTHTR(location authoringyaml.FlowFileLocation) (stageLoadResult, error) {
	if location.InFlowRoot {
		result, err := authoringthtr.LoadFlowFileDetailed(location.Path, l.matchers)
		if err != nil {
			return stageLoadResult{}, mapTHTRLoadError(err)
		}
		return stageLoadResult{
			Spec:               result.Spec,
			rewriteDiagnostics: result.RewriteDiagnostics,
		}, nil
	}

	result, err := authoringthtr.LoadFileDetailed(location.Path, l.matchers)
	if err != nil {
		return stageLoadResult{}, mapTHTRLoadError(err)
	}

	return stageLoadResult{
		Spec:               result.Spec,
		rewriteDiagnostics: result.RewriteDiagnostics,
	}, nil
}

func mapTHTRLoadError(err error) error {
	var diagnosticError *authoringthtr.DiagnosticError
	if errors.As(err, &diagnosticError) {
		return diagnosticError
	}

	return err
}

func identityDiagnostics(diagnostics []theater.Diagnostic) []theater.Diagnostic {
	return diagnostics
}
