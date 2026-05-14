package thtr

import (
	"errors"
	"strconv"

	"github.com/alex-poliushkin/theater"
)

func newDiagnosticError(sourceFile, code, path string, err error) error {
	span := errorSpan(err)
	if path == "" {
		path = sourceFile
	}

	return &DiagnosticError{
		diagnostic: theater.Diagnostic{
			Code:     code,
			Path:     path,
			Severity: theater.SeverityError,
			Summary:  err.Error(),
			Span: theater.SourceRef{
				File:   sourceFile,
				Line:   span.Start.Line,
				Column: span.Start.Column,
			},
		},
	}
}

func nearestSyntaxPath(document *syntaxDocument, position sourceSpan) string {
	if document == nil {
		return ""
	}

	codec := sourcePathCodec{}
	stagePath := codec.Join("stage", document.Stage.ID)
	if document.Stage.Name != nil && spanContainsPosition(document.Stage.Name.Span, position.Start) {
		return stagePath + "/name"
	}
	if path := nearestStageSectionPath(document.HTTP, position, stagePath, httpEntryPath); path != "" {
		return path
	}
	if path := nearestStatePath(document.State, position, stagePath); path != "" {
		return path
	}
	if path := nearestScenarioPath(document.Scenarios, position, stagePath); path != "" {
		return path
	}
	if path := nearestCallPath(document.Calls, position, stagePath); path != "" {
		return path
	}

	return stagePath
}

func errorSpan(err error) sourceSpan {
	type spanProvider interface {
		Span() sourceSpan
	}

	var provider spanProvider
	if errors.As(err, &provider) {
		return provider.Span()
	}

	return sourceSpan{}
}

func spanContainsPosition(span sourceSpan, position sourcePosition) bool {
	if position.Offset < span.Start.Offset {
		return false
	}
	if position.Offset > span.End.Offset {
		return false
	}
	return true
}

func itoa(value int) string {
	return strconv.Itoa(value)
}

func nearestStageSectionPath(
	section *stageSectionSyntax,
	position sourceSpan,
	stagePath string,
	entryPath func(stagePath, kind, id string) string,
) string {
	if section == nil || !spanContainsPosition(section.Span, position.Start) {
		return ""
	}

	basePath := stagePath + "/" + section.Name
	for i := range section.Entries {
		entry := section.Entries[i]
		if spanContainsPosition(entry.Span, position.Start) {
			return entryPath(stagePath, entry.Kind, entry.ID)
		}
	}

	return basePath
}

func nearestStatePath(section *stageSectionSyntax, position sourceSpan, stagePath string) string {
	if section == nil || !spanContainsPosition(section.Span, position.Start) {
		return ""
	}

	basePath := stagePath + "/state"
	for i := range section.Entries {
		entry := section.Entries[i]
		if spanContainsPosition(entry.Span, position.Start) {
			return stateEntryPath(stagePath, entry.Kind, entry.ID)
		}
	}

	return basePath
}

func nearestScenarioPath(scenarios []scenarioSyntax, position sourceSpan, stagePath string) string {
	codec := sourcePathCodec{}
	for i := range scenarios {
		scenario := scenarios[i]
		if !spanContainsPosition(scenario.Span, position.Start) {
			continue
		}

		scenarioPath := codec.JoinChild(stagePath, "scenario", scenario.ID)
		if scenario.Name != nil && spanContainsPosition(scenario.Name.Span, position.Start) {
			return scenarioPath + "/name"
		}
		for j := range scenario.Inputs {
			if spanContainsPosition(scenario.Inputs[j].Span, position.Start) {
				return bindingPath(scenarioPath+"/input", scenario.Inputs[j].Name)
			}
		}
		for j := range scenario.AuthBindings {
			if path := nearestAuthBindingPath(scenario.AuthBindings[j], position, scenarioPath); path != "" {
				return path
			}
		}
		if path := nearestActPath(scenario.Acts, position, scenarioPath); path != "" {
			return path
		}

		return scenarioPath
	}

	return ""
}

func nearestAuthBindingPath(authBinding authBindingSyntax, position sourceSpan, scenarioPath string) string {
	if !spanContainsPosition(authBinding.Span, position.Start) {
		return ""
	}

	codec := sourcePathCodec{}
	authBindingPath := codec.JoinChild(scenarioPath, "auth_bindings", authBinding.Auth)
	for i := range authBinding.Slots {
		if spanContainsPosition(authBinding.Slots[i].Span, position.Start) {
			return codec.JoinChild(authBindingPath, "slot", authBinding.Slots[i].Name)
		}
	}

	return authBindingPath
}

func nearestActPath(acts []actSyntax, position sourceSpan, scenarioPath string) string {
	codec := sourcePathCodec{}
	for i := range acts {
		act := acts[i]
		if !spanContainsPosition(act.Span, position.Start) {
			continue
		}

		actPath := codec.JoinChild(scenarioPath, "act", act.ID)
		if path := nearestActMetadataPath(act, position, actPath); path != "" {
			return path
		}
		if path := nearestPropertySyntaxPath(act.Properties, position, actPath); path != "" {
			return path
		}
		if path := nearestLogSyntaxPath(act.Logs, position, actPath); path != "" {
			return path
		}
		if path := nearestExpectationSyntaxPath(act.Expectations, position, actPath); path != "" {
			return path
		}
		if path := nearestExportSyntaxPath(act.Exports, position, actPath); path != "" {
			return path
		}
		if path := nearestTransitionSyntaxPath(act.Transitions, position, actPath); path != "" {
			return path
		}

		return actPath
	}

	return ""
}

func nearestActMetadataPath(act actSyntax, position sourceSpan, actPath string) string {
	if act.Name != nil && spanContainsPosition(act.Name.Span, position.Start) {
		return actPath + "/name"
	}
	if act.Eventually != nil && spanContainsPosition(act.Eventually.Span, position.Start) {
		return actPath + "/eventually"
	}
	if act.Action != nil && spanContainsPosition(act.Action.Span, position.Start) {
		return actPath + "/action"
	}
	if act.CaptureAuth != nil && spanContainsPosition(act.CaptureAuth.Span, position.Start) {
		return nearestCaptureAuthPath(*act.CaptureAuth, position, actPath+"/capture_auth")
	}
	return ""
}

func nearestCaptureAuthPath(captureAuth captureAuthSyntax, position sourceSpan, capturePath string) string {
	for j := range captureAuth.Slots {
		if spanContainsPosition(captureAuth.Slots[j].Span, position.Start) {
			return joinChildPath(capturePath, "slot", captureAuth.Slots[j].Name)
		}
	}
	return capturePath
}

func nearestPropertySyntaxPath(properties []propertySyntax, position sourceSpan, actPath string) string {
	codec := sourcePathCodec{}
	for j := range properties {
		if spanContainsPosition(properties[j].Span, position.Start) {
			return codec.JoinChild(actPath, "property", properties[j].Name)
		}
	}
	return ""
}

func nearestLogSyntaxPath(logs []logSyntax, position sourceSpan, actPath string) string {
	for j := range logs {
		if spanContainsPosition(logs[j].Span, position.Start) {
			return joinChildPath(actPath, "log", logs[j].ID)
		}
	}
	return ""
}

func nearestExpectationSyntaxPath(expectations []expectationSyntax, position sourceSpan, actPath string) string {
	codec := sourcePathCodec{}
	for j := range expectations {
		if !spanContainsPosition(expectations[j].Span, position.Start) {
			continue
		}

		expectationPath := codec.JoinChild(actPath, "expectation", expectations[j].ID)
		if path := nearestAssertionSyntaxPath(expectations[j].Assert, position, expectationPath+"/assert"); path != "" {
			return path
		}
		return expectationPath
	}
	return ""
}

func nearestAssertionSyntaxPath(assertion assertionSyntax, position sourceSpan, assertPath string) string {
	if !spanContainsPosition(assertion.Span, position.Start) {
		return ""
	}

	for i := range assertion.Clauses {
		clausePath := assertPath + "/clause[" + itoa(i) + "]"
		if !spanContainsPosition(assertion.Clauses[i].Span, position.Start) {
			continue
		}
		if spanContainsPosition(assertion.Clauses[i].Subject.ExpressionSpan(), position.Start) {
			return clausePath + "/subject"
		}
		if path := nearestAssertionSyntaxPath(assertion.Clauses[i].Assert, position, clausePath+"/assert"); path != "" {
			return path
		}
		return clausePath
	}

	return assertPath
}

func nearestExportSyntaxPath(exports []exportSyntax, position sourceSpan, actPath string) string {
	for j := range exports {
		if spanContainsPosition(exports[j].Span, position.Start) {
			return exportPath(actPath, exports[j].Name)
		}
	}
	return ""
}

func nearestTransitionSyntaxPath(transitions []transitionSyntax, position sourceSpan, actPath string) string {
	for j := range transitions {
		if spanContainsPosition(transitions[j].Span, position.Start) {
			return actPath + "/transition[" + itoa(j) + "]"
		}
	}
	return ""
}

func nearestCallPath(calls []scenarioCallSyntax, position sourceSpan, stagePath string) string {
	codec := sourcePathCodec{}
	for i := range calls {
		call := calls[i]
		if !spanContainsPosition(call.Span, position.Start) {
			continue
		}

		callPath := codec.JoinChild(stagePath, "call", call.ID)
		if call.Name != nil && spanContainsPosition(call.Name.Span, position.Start) {
			return callPath + "/name"
		}
		for j := range call.Bindings {
			if spanContainsPosition(call.Bindings[j].Span, position.Start) {
				return bindingPath(callPath, call.Bindings[j].Name)
			}
		}
		for j := range call.Dependencies {
			if spanContainsPosition(call.Dependencies[j].Span, position.Start) {
				return callPath + "/dependency[" + itoa(j) + "]"
			}
		}
		for j := range call.Exports {
			if spanContainsPosition(call.Exports[j].Span, position.Start) {
				return exportPath(callPath, call.Exports[j].Name)
			}
		}

		return callPath
	}

	return ""
}
