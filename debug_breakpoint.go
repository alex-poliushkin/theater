package theater

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	debugBreakpointKindScenarioCall debugBreakpointKind = "scenario_call"
	debugBreakpointKindAct          debugBreakpointKind = "act"
	debugBreakpointKindAction       debugBreakpointKind = "action"
	debugBreakpointKindExpectation  debugBreakpointKind = "expectation"

	debugBreakpointWhenAlways          debugBreakpointWhen = "always"
	debugBreakpointWhenTerminalFailure debugBreakpointWhen = "terminal-failure"
	debugBreakpointWhenAttemptFailure  debugBreakpointWhen = "attempt-failure"
	debugBreakpointWhenRetryOnly       debugBreakpointWhen = "retry-only"

	debugBreakpointActionPause            debugBreakpointAction = "pause"
	debugBreakpointActionSnapshotContinue debugBreakpointAction = "snapshot-continue"

	debugBreakpointAttemptModeAny       debugBreakpointAttemptMode = "any"
	debugBreakpointAttemptModeFirst     debugBreakpointAttemptMode = "first"
	debugBreakpointAttemptModeRetryOnly debugBreakpointAttemptMode = "retry-only"
	debugBreakpointWildcardPath                                    = "**"
)

type debugCompiledBreakpoint struct {
	Name     string
	Boundary debugCompiledBoundary
	When     debugBreakpointWhen
	Attempt  debugBreakpointAttempt
	Reaction debugBreakpointAction
}

type debugCompiledBoundary struct {
	Path         string
	ScenarioPath string
	ActID        string
	NodeRef      string
	Kind         debugBoundaryKind
	Phase        debugBoundaryPhase
	RetryAware   bool
}

type debugBreakpointSpec struct {
	Name    string
	Path    string
	Kind    debugBreakpointKind
	Phase   debugBoundaryPhase
	When    debugBreakpointWhen
	Attempt debugBreakpointAttempt
	Action  debugBreakpointAction
}

type debugBreakpointAttempt struct {
	Mode  debugBreakpointAttemptMode
	Index int
}

type debugBreakpointAction string

type debugBreakpointAttemptMode string

type debugBreakpointKind string

type debugBreakpointWhen string

func compileDebugBreakpoints(stage *stagePlan, rawSpecs []string) ([]debugCompiledBreakpoint, error) {
	if len(rawSpecs) == 0 {
		return nil, nil
	}

	boundaries, err := collectDebugBoundaries(stage)
	if err != nil {
		return nil, err
	}

	compiled := make([]debugCompiledBreakpoint, 0, len(rawSpecs))
	for i := range rawSpecs {
		spec, err := parseDebugBreakpointSpec(rawSpecs[i])
		if err != nil {
			return nil, newPlanPreparationError(debugBreakpointSpecPath(i), err)
		}

		entries, err := compileDebugBreakpointSpec(spec, boundaries)
		if err != nil {
			return nil, newPlanPreparationError(debugBreakpointSpecPath(i), err)
		}

		compiled = append(compiled, entries...)
	}

	return compiled, nil
}

func parseDebugBreakpointSpec(raw string) (debugBreakpointSpec, error) {
	spec := debugBreakpointSpec{
		Path:    debugBreakpointWildcardPath,
		When:    debugBreakpointWhenAlways,
		Attempt: debugBreakpointAttempt{Mode: debugBreakpointAttemptModeAny},
		Action:  debugBreakpointActionPause,
	}

	if strings.TrimSpace(raw) == "" {
		return debugBreakpointSpec{}, errors.New("debug breakpoint spec must not be empty")
	}

	seen := make(map[string]struct{}, 7)
	parts := strings.Split(raw, ",")
	for _, part := range parts {
		key, value, err := parseDebugBreakpointField(part)
		if err != nil {
			return debugBreakpointSpec{}, err
		}
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			return debugBreakpointSpec{}, fmt.Errorf("debug breakpoint field %q is duplicated", key)
		}
		seen[key] = struct{}{}

		if err := applyDebugBreakpointField(&spec, key, value); err != nil {
			return debugBreakpointSpec{}, err
		}
	}

	if err := validateDebugBreakpointSpec(spec); err != nil {
		return debugBreakpointSpec{}, err
	}

	return spec, nil
}

func compileDebugBreakpointSpec(
	spec debugBreakpointSpec,
	boundaries []debugCompiledBoundary,
) ([]debugCompiledBreakpoint, error) {
	if err := validateDebugBreakpointKindAvailability(spec.Kind); err != nil {
		return nil, err
	}

	compiled := make([]debugCompiledBreakpoint, 0, len(boundaries))
	for i := range boundaries {
		if !debugBreakpointSpecMatchesBoundary(spec, boundaries[i]) {
			continue
		}

		compiled = append(compiled, debugCompiledBreakpoint{
			Name:     spec.Name,
			Boundary: boundaries[i],
			When:     spec.When,
			Attempt:  spec.Attempt,
			Reaction: spec.Action,
		})
	}

	if len(compiled) == 0 {
		return nil, fmt.Errorf("debug breakpoint %q matched no debuggable boundaries", debugBreakpointDisplayName(spec))
	}

	return compiled, nil
}

func collectDebugBoundaries(stage *stagePlan) ([]debugCompiledBoundary, error) {
	if stage == nil {
		return nil, errors.New("stage plan is required")
	}

	scenarios := make(map[string]scenarioPlan, len(stage.Scenarios))
	for i := range stage.Scenarios {
		scenarios[stage.Scenarios[i].ID] = stage.Scenarios[i]
	}

	boundaries := make([]debugCompiledBoundary, 0, len(stage.ScenarioCalls))
	for i := range stage.ScenarioCalls {
		call := stage.ScenarioCalls[i]
		scenario, ok := scenarios[call.ScenarioID]
		if !ok {
			return nil, fmt.Errorf("scenario call %q targets unknown scenario %q", call.ID, call.ScenarioID)
		}

		addresses := newExecutionAddressSpace(call.Path)
		boundaries = append(boundaries,
			debugCompiledBoundary{
				Path:         addresses.scenarioPath(),
				ScenarioPath: call.Path,
				Kind:         debugBoundaryKindScenarioCall,
				Phase:        debugBoundaryPhaseBefore,
			},
			debugCompiledBoundary{
				Path:         addresses.scenarioPath(),
				ScenarioPath: call.Path,
				Kind:         debugBoundaryKindScenarioCall,
				Phase:        debugBoundaryPhaseAfter,
			},
		)

		for j := range scenario.Acts {
			act := scenario.Acts[j]
			retryAware := act.Eventually != nil
			boundaries = append(boundaries,
				debugCompiledBoundary{
					Path:         addresses.actPath(act.ID),
					ScenarioPath: call.Path,
					ActID:        act.ID,
					Kind:         debugBoundaryKindAct,
					Phase:        debugBoundaryPhaseBefore,
				},
				debugCompiledBoundary{
					Path:         addresses.actPath(act.ID),
					ScenarioPath: call.Path,
					ActID:        act.ID,
					Kind:         debugBoundaryKindAct,
					Phase:        debugBoundaryPhaseAfter,
					RetryAware:   retryAware,
				},
				debugCompiledBoundary{
					Path:         addresses.actionPath(act.ID),
					ScenarioPath: call.Path,
					ActID:        act.ID,
					NodeRef:      "action",
					Kind:         debugBoundaryKindAction,
					Phase:        debugBoundaryPhaseBefore,
					RetryAware:   retryAware,
				},
				debugCompiledBoundary{
					Path:         addresses.actionPath(act.ID),
					ScenarioPath: call.Path,
					ActID:        act.ID,
					NodeRef:      "action",
					Kind:         debugBoundaryKindAction,
					Phase:        debugBoundaryPhaseAfter,
					RetryAware:   retryAware,
				},
			)

			for k := range act.Expectations {
				expectation := act.Expectations[k]
				path := addresses.expectationPath(act.ID, expectation.ID)
				boundaries = append(boundaries,
					debugCompiledBoundary{
						Path:         path,
						ScenarioPath: call.Path,
						ActID:        act.ID,
						NodeRef:      expectation.ID,
						Kind:         debugBoundaryKindExpectation,
						Phase:        debugBoundaryPhaseBefore,
						RetryAware:   retryAware,
					},
					debugCompiledBoundary{
						Path:         path,
						ScenarioPath: call.Path,
						ActID:        act.ID,
						NodeRef:      expectation.ID,
						Kind:         debugBoundaryKindExpectation,
						Phase:        debugBoundaryPhaseAfter,
						RetryAware:   retryAware,
					},
				)
			}
		}
	}

	return boundaries, nil
}

func validateDebugBreakpointSpec(spec debugBreakpointSpec) error {
	if spec.When == debugBreakpointWhenRetryOnly {
		switch spec.Attempt.Mode {
		case debugBreakpointAttemptModeFirst:
			return fmt.Errorf("debug breakpoint attempt %q contradicts when %q", spec.Attempt.Mode, spec.When)
		case debugBreakpointAttemptModeAny:
			return nil
		case debugBreakpointAttemptModeRetryOnly:
			return nil
		default:
			if spec.Attempt.Index <= 1 {
				return fmt.Errorf("debug breakpoint attempt %q contradicts when %q", debugBreakpointAttemptDisplay(spec.Attempt), spec.When)
			}
		}
	}

	return nil
}

func validateDebugBreakpointKindAvailability(kind debugBreakpointKind) error {
	switch kind {
	case "", debugBreakpointKindScenarioCall, debugBreakpointKindAct, debugBreakpointKindAction, debugBreakpointKindExpectation:
		return nil
	default:
		return fmt.Errorf("debug breakpoint kind %q is invalid", kind)
	}
}

func validateDebugPathPattern(pattern string) error {
	if pattern == "" {
		return errors.New("debug breakpoint path is required")
	}

	segments := strings.Split(pattern, "/")
	for _, segment := range segments {
		if segment == "" {
			return fmt.Errorf("debug breakpoint path %q is invalid: empty path segment", pattern)
		}
		if strings.Contains(segment, string(debugBreakpointWildcardPath)) && segment != string(debugBreakpointWildcardPath) {
			return fmt.Errorf("debug breakpoint path %q is invalid: ** must occupy a full path segment", pattern)
		}
		if strings.ContainsAny(segment, "?[]") {
			return fmt.Errorf("debug breakpoint path %q is invalid: only * and ** wildcards are supported", pattern)
		}
	}

	return nil
}

func debugBreakpointSpecMatchesBoundary(
	spec debugBreakpointSpec,
	boundary debugCompiledBoundary,
) bool {
	if !debugBreakpointSpecKindMatchesBoundary(spec.Kind, boundary.Kind) {
		return false
	}
	if spec.Phase != "" && spec.Phase != boundary.Phase {
		return false
	}
	if !debugPathPatternMatches(spec.Path, boundary.Path) {
		return false
	}
	if !debugBreakpointWhenMatchesBoundary(spec.When, boundary) {
		return false
	}
	if !debugBreakpointAttemptMatchesBoundary(spec.Attempt, boundary) {
		return false
	}

	return true
}

func debugBreakpointSpecKindMatchesBoundary(kind debugBreakpointKind, boundary debugBoundaryKind) bool {
	if kind == "" {
		return boundary == debugBoundaryKindAction || boundary == debugBoundaryKindExpectation
	}

	return debugBreakpointKindMatchesBoundary(kind, boundary)
}

func debugBreakpointAttemptMatchesBoundary(
	attempt debugBreakpointAttempt,
	boundary debugCompiledBoundary,
) bool {
	switch attempt.Mode {
	case debugBreakpointAttemptModeAny, "":
		return true
	case debugBreakpointAttemptModeFirst:
		return true
	case debugBreakpointAttemptModeRetryOnly:
		return boundary.RetryAware
	default:
		if attempt.Index <= 1 {
			return true
		}

		return boundary.RetryAware
	}
}

func debugBreakpointWhenMatchesBoundary(
	when debugBreakpointWhen,
	boundary debugCompiledBoundary,
) bool {
	switch when {
	case "", debugBreakpointWhenAlways:
		return true
	case debugBreakpointWhenRetryOnly:
		return boundary.RetryAware
	case debugBreakpointWhenAttemptFailure:
		return boundary.Phase == debugBoundaryPhaseAfter &&
			boundary.RetryAware &&
			(boundary.Kind == debugBoundaryKindAction || boundary.Kind == debugBoundaryKindExpectation)
	case debugBreakpointWhenTerminalFailure:
		return boundary.Phase == debugBoundaryPhaseAfter
	default:
		return false
	}
}

func debugBreakpointKindMatchesBoundary(kind debugBreakpointKind, boundary debugBoundaryKind) bool {
	switch kind {
	case debugBreakpointKindScenarioCall:
		return boundary == debugBoundaryKindScenarioCall
	case debugBreakpointKindAct:
		return boundary == debugBoundaryKindAct
	case debugBreakpointKindAction:
		return boundary == debugBoundaryKindAction
	case debugBreakpointKindExpectation:
		return boundary == debugBoundaryKindExpectation
	default:
		return false
	}
}

func debugPathPatternMatches(pattern, candidate string) bool {
	return debugPathPatternMatchSegments(
		strings.Split(pattern, "/"),
		strings.Split(candidate, "/"),
	)
}

func debugPathPatternMatchSegments(pattern, candidate []string) bool {
	if len(pattern) == 0 {
		return len(candidate) == 0
	}

	if pattern[0] == "**" {
		if len(pattern) == 1 {
			return true
		}
		if debugPathPatternMatchSegments(pattern[1:], candidate) {
			return true
		}
		if len(candidate) == 0 {
			return false
		}

		return debugPathPatternMatchSegments(pattern, candidate[1:])
	}

	if len(candidate) == 0 {
		return false
	}
	if !debugPathSegmentMatches(pattern[0], candidate[0]) {
		return false
	}

	return debugPathPatternMatchSegments(pattern[1:], candidate[1:])
}

func debugPathSegmentMatches(pattern, candidate string) bool {
	if pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return pattern == candidate
	}

	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == candidate
	}

	position := 0
	if parts[0] != "" {
		if !strings.HasPrefix(candidate, parts[0]) {
			return false
		}
		position = len(parts[0])
	}

	for i := 1; i < len(parts)-1; i++ {
		part := parts[i]
		if part == "" {
			continue
		}

		index := strings.Index(candidate[position:], part)
		if index < 0 {
			return false
		}
		position += index + len(part)
	}

	last := parts[len(parts)-1]
	if last == "" {
		return true
	}

	suffixIndex := strings.LastIndex(candidate, last)
	if suffixIndex < position {
		return false
	}

	return strings.HasSuffix(candidate, last)
}

func parseDebugBreakpointAction(raw string) (debugBreakpointAction, error) {
	action := debugBreakpointAction(strings.TrimSpace(raw))
	switch action {
	case debugBreakpointActionPause, debugBreakpointActionSnapshotContinue:
		return action, nil
	default:
		return "", fmt.Errorf("debug breakpoint action %q is invalid", raw)
	}
}

func parseDebugBreakpointAttempt(raw string) (debugBreakpointAttempt, error) {
	switch strings.TrimSpace(raw) {
	case string(debugBreakpointAttemptModeAny):
		return debugBreakpointAttempt{Mode: debugBreakpointAttemptModeAny}, nil
	case string(debugBreakpointAttemptModeFirst):
		return debugBreakpointAttempt{Mode: debugBreakpointAttemptModeFirst, Index: 1}, nil
	case string(debugBreakpointAttemptModeRetryOnly):
		return debugBreakpointAttempt{Mode: debugBreakpointAttemptModeRetryOnly}, nil
	}

	index, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || index <= 0 {
		return debugBreakpointAttempt{}, fmt.Errorf("debug breakpoint attempt %q is invalid", raw)
	}

	return debugBreakpointAttempt{
		Mode:  debugBreakpointAttemptMode(strconv.Itoa(index)),
		Index: index,
	}, nil
}

func applyDebugBreakpointField(spec *debugBreakpointSpec, key, value string) error {
	switch key {
	case "name":
		if value == "" {
			return errors.New("debug breakpoint name is required")
		}
		spec.Name = value
		return nil
	case "path":
		if err := validateDebugPathPattern(value); err != nil {
			return err
		}
		spec.Path = value
		return nil
	case "kind":
		kind, err := parseDebugBreakpointKind(value)
		if err != nil {
			return err
		}
		spec.Kind = kind
		return nil
	case "phase":
		phase, err := parseDebugBoundaryPhase(value)
		if err != nil {
			return err
		}
		spec.Phase = phase
		return nil
	case "when":
		when, err := parseDebugBreakpointWhen(value)
		if err != nil {
			return err
		}
		spec.When = when
		return nil
	case "attempt":
		attempt, err := parseDebugBreakpointAttempt(value)
		if err != nil {
			return err
		}
		spec.Attempt = attempt
		return nil
	case string(debugBoundaryKindAction):
		action, err := parseDebugBreakpointAction(value)
		if err != nil {
			return err
		}
		spec.Action = action
		return nil
	default:
		return fmt.Errorf("unknown debug breakpoint field %q", key)
	}
}

func parseDebugBreakpointField(raw string) (key, value string, err error) {
	part := strings.TrimSpace(raw)
	if part == "" {
		return "", "", nil
	}

	var ok bool
	key, value, ok = strings.Cut(part, "=")
	if !ok {
		return "", "", fmt.Errorf("debug breakpoint segment %q must use key=value", part)
	}

	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" {
		return "", "", errors.New("debug breakpoint field name is required")
	}

	return key, value, nil
}

func parseDebugBreakpointKind(raw string) (debugBreakpointKind, error) {
	kind := debugBreakpointKind(strings.TrimSpace(raw))
	switch kind {
	case debugBreakpointKindScenarioCall, debugBreakpointKindAct, debugBreakpointKindAction, debugBreakpointKindExpectation:
		return kind, nil
	default:
		return "", fmt.Errorf("debug breakpoint kind %q is invalid", raw)
	}
}

func parseDebugBreakpointWhen(raw string) (debugBreakpointWhen, error) {
	when := debugBreakpointWhen(strings.TrimSpace(raw))
	switch when {
	case debugBreakpointWhenAlways, debugBreakpointWhenTerminalFailure, debugBreakpointWhenAttemptFailure, debugBreakpointWhenRetryOnly:
		return when, nil
	default:
		return "", fmt.Errorf("debug breakpoint when %q is invalid", raw)
	}
}

func parseDebugBoundaryPhase(raw string) (debugBoundaryPhase, error) {
	phase := debugBoundaryPhase(strings.TrimSpace(raw))
	switch phase {
	case debugBoundaryPhaseBefore, debugBoundaryPhaseAfter:
		return phase, nil
	default:
		return "", fmt.Errorf("debug breakpoint phase %q is invalid", raw)
	}
}

func debugBreakpointAttemptDisplay(attempt debugBreakpointAttempt) string {
	if attempt.Mode == "" || attempt.Mode == debugBreakpointAttemptModeAny {
		return "any"
	}
	if attempt.Mode == debugBreakpointAttemptModeFirst {
		return "first"
	}
	if attempt.Mode == debugBreakpointAttemptModeRetryOnly {
		return "retry-only"
	}
	if attempt.Index > 0 {
		return strconv.Itoa(attempt.Index)
	}

	return string(attempt.Mode)
}

func debugBreakpointDisplayName(spec debugBreakpointSpec) string {
	if spec.Name != "" {
		return spec.Name
	}
	if spec.Path != "" {
		return spec.Path
	}

	return "breakpoint"
}

func debugBreakpointSpecPath(index int) string {
	return fmt.Sprintf("debug/breakpoint[%d]", index)
}

func debugBreakpointLabel(matches []debugCompiledBreakpoint) string {
	for i := range matches {
		if matches[i].Name != "" {
			return matches[i].Name
		}
	}
	if len(matches) == 0 {
		return ""
	}

	return matches[0].Boundary.Path
}

func debugBreakpointMatchesState(
	breakpoint debugCompiledBreakpoint,
	state debugBoundaryState,
) bool {
	return debugBreakpointMatchesTerminalState(breakpoint, state, false)
}

func debugBreakpointMatchesTerminalState(
	breakpoint debugCompiledBreakpoint,
	state debugBoundaryState,
	terminal bool,
) bool {
	if !debugCompiledBoundaryMatchesRef(breakpoint.Boundary, state.Ref) {
		return false
	}
	if !debugBreakpointAttemptMatchesState(breakpoint.Attempt, state.Ref.Attempt) {
		return false
	}
	if !debugBreakpointWhenMatchesState(breakpoint.When, state, terminal) {
		return false
	}

	return true
}

func debugBreakpointAttemptMatchesState(attempt debugBreakpointAttempt, current int) bool {
	switch attempt.Mode {
	case "", debugBreakpointAttemptModeAny:
		return true
	case debugBreakpointAttemptModeFirst:
		return current == 1
	case debugBreakpointAttemptModeRetryOnly:
		return current > 1
	default:
		return current == attempt.Index
	}
}

func debugBreakpointWhenMatchesState(
	when debugBreakpointWhen,
	state debugBoundaryState,
	terminal bool,
) bool {
	switch when {
	case "", debugBreakpointWhenAlways:
		return true
	case debugBreakpointWhenRetryOnly:
		return state.Ref.Attempt > 1
	case debugBreakpointWhenAttemptFailure:
		return !terminal && state.Ref.Phase == debugBoundaryPhaseAfter && state.Failure != nil
	case debugBreakpointWhenTerminalFailure:
		return terminal && state.Ref.Phase == debugBoundaryPhaseAfter && state.Failure != nil
	default:
		return false
	}
}

func debugCompiledBoundaryMatchesRef(
	boundary debugCompiledBoundary,
	ref debugBoundaryRef,
) bool {
	return boundary.Path == ref.Path &&
		boundary.ScenarioPath == ref.ScenarioPath &&
		boundary.ActID == ref.ActID &&
		boundary.NodeRef == ref.NodeRef &&
		boundary.Kind == ref.Kind &&
		boundary.Phase == ref.Phase
}
