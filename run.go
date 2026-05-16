package theater

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/alex-poliushkin/theater/internal/runtimevalue"
)

// RunResult is the public result returned by Runner.Run.
type RunResult struct {
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	Report      Report       `json:"report"`
}

type actOutcome struct {
	status                Status
	failure               *Failure
	values                Values
	properties            Values
	diagnostics           []NodeDiagnostic
	logs                  []logEvaluationRecord
	eventually            *EventuallyReport
	debugTerminalBoundary *debugBoundaryState
}

func definitionFailureReport(stageID, stagePath string, diagnostics []Diagnostic) Report {
	failure := &Failure{
		Kind:    FailureKindDefinition,
		Phase:   PhaseValidate,
		At:      stagePath,
		Summary: fmt.Sprintf("validation failed with %d diagnostic(s)", len(diagnostics)),
	}

	return Report{
		StageID:   stageID,
		StagePath: stagePath,
		Status:    StatusFailed,
		Failure:   failure,
	}
}

func setupFailureReport(stageID, stagePath string, failure *Failure) Report {
	return Report{
		StageID:   stageID,
		StagePath: stagePath,
		Status:    StatusFailed,
		Failure:   failure,
	}
}

func indexActs(acts []actPlan) map[string]*actPlan {
	indexed := make(map[string]*actPlan, len(acts))
	for i := range acts {
		indexed[acts[i].ID] = &acts[i]
	}

	return indexed
}

func nextTransitionID(act *actPlan, outcome TransitionOutcome) (string, bool) {
	if act == nil {
		return "", false
	}

	for _, transition := range act.Transitions {
		if transition.On == outcome {
			return transition.To, true
		}
	}

	return "", false
}

func scenarioBindingSource(dependencies []scenarioDependencyPlan, states map[string]scenarioState) Values {
	if len(dependencies) == 0 {
		return Values{}
	}

	source := make(Values)
	for _, dependency := range dependencies {
		state, ok := states[dependency.CallID]
		if !ok || len(state.Exports) == 0 {
			continue
		}

		for key, value := range state.Exports {
			source[key] = runtimevalue.Clone(value)
		}
	}

	return source
}

func internalFailure(path, summary string, err error) *Failure {
	return &Failure{
		Kind:    FailureKindInternal,
		Phase:   PhaseRun,
		At:      path,
		Summary: summary,
		Cause:   err,
	}
}

func classifyExecutionError(
	path string,
	err error,
	kind FailureKind,
	failureSummary string,
	timeoutSummary string,
) (status Status, failure *Failure) {
	switch {
	case errors.Is(err, context.Canceled):
		return StatusCanceled, nil
	case errors.Is(err, context.DeadlineExceeded):
		return StatusFailed, &Failure{
			Kind:    FailureKindTimeout,
			Phase:   PhaseRun,
			At:      path,
			Summary: timeoutSummary,
			Cause:   err,
		}
	default:
		return StatusFailed, &Failure{
			Kind:    kind,
			Phase:   PhaseRun,
			At:      path,
			Summary: failureSummary,
			Cause:   err,
		}
	}
}

func nextFailureTransitionID(act *actPlan, failure *Failure) (string, bool) {
	if failure != nil && failure.Kind == FailureKindTimeout {
		if nextID, ok := nextTransitionID(act, TransitionOnTimeout); ok {
			return nextID, true
		}
	}

	return nextTransitionID(act, TransitionOnFail)
}

func setupFailure(path string, err error) *Failure {
	return &Failure{
		Kind:    FailureKindSetup,
		Phase:   PhaseRun,
		At:      path,
		Summary: "runtime setup failed",
		Cause:   err,
	}
}

func summarizeStage(stage *stagePlan, states map[string]scenarioState) (status Status, failure *Failure) {
	allSkipped := len(states) > 0
	sawCanceled := false

	for _, scenarioCall := range orderedScenarioCalls(stage, states) {
		state := states[scenarioCall.ID]
		if state.Status == StatusFailed {
			return StatusFailed, state.Failure
		}

		if state.Status != StatusSkipped {
			allSkipped = false
		}

		if state.Status == StatusCanceled {
			sawCanceled = true
		}
	}

	if sawCanceled {
		return StatusCanceled, nil
	}

	if allSkipped {
		return StatusSkipped, nil
	}

	return StatusPassed, nil
}

func orderedScenarioCalls(stage *stagePlan, states map[string]scenarioState) []scenarioCallPlan {
	if stage == nil || len(stage.ScenarioCalls) == 0 || len(states) == 0 {
		return nil
	}

	ordered := make([]scenarioCallPlan, 0, len(states))
	for _, scenarioCall := range stage.ScenarioCalls {
		if _, ok := states[scenarioCall.ID]; !ok {
			continue
		}

		ordered = append(ordered, scenarioCall)
	}

	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].PlanOrdinal != ordered[j].PlanOrdinal {
			return ordered[i].PlanOrdinal < ordered[j].PlanOrdinal
		}

		return ordered[i].ID < ordered[j].ID
	})

	return ordered
}

func preflightCatalog(stage *stagePlan, catalog CatalogResolver) *Failure {
	if stage.State != nil {
		for name, backend := range stage.State.Backends {
			if _, err := catalog.ResolveStateBackend(backend.Use); err != nil {
				return setupFailure(stateBackendPath(stage.Path, name), err)
			}
		}
	}

	for i := range stage.Scenarios {
		scenario := &stage.Scenarios[i]
		for j := range scenario.Acts {
			act := &scenario.Acts[j]
			if _, err := catalog.ResolveAction(act.Action.Use); err != nil {
				return setupFailure(act.Path, err)
			}

			for k := range act.Properties {
				property := &act.Properties[k]
				propPath := property.Path
				if property.Inventory.Present {
					if _, err := catalog.ResolveInventory(property.Inventory.Use); err != nil {
						return setupFailure(propPath, err)
					}
				}

				for m := range property.Decorators {
					if property.Decorators[m].Use == "" {
						continue
					}

					if _, err := catalog.ResolveDecorator(property.Decorators[m].Use); err != nil {
						return setupFailure(propPath, err)
					}
				}
			}
		}
	}

	return nil
}

func clonePayloadMetadata(payload *PayloadMetadata) *PayloadMetadata {
	if payload == nil {
		return nil
	}

	cloned := *payload
	return &cloned
}

func cloneObservedValue(observed ObservedValue) ObservedValue {
	cloned := observed
	cloned.Preview = clonePreview(observed.Preview)
	cloned.Payload = clonePayloadMetadata(observed.Payload)
	if len(observed.Artifacts) != 0 {
		cloned.Artifacts = make([]ArtifactRef, len(observed.Artifacts))
		copy(cloned.Artifacts, observed.Artifacts)
	}

	return cloned
}

func cloneObservedStream(observed ObservedStream) ObservedStream {
	cloned := observed
	cloned.Preview = clonePreview(observed.Preview)
	cloned.Payload = clonePayloadMetadata(observed.Payload)
	return cloned
}

func cloneActionObservations(observations *ActionObservations) *ActionObservations {
	if observations == nil {
		return nil
	}

	cloned := &ActionObservations{}
	if len(observations.Inputs) != 0 {
		cloned.Inputs = make(map[string]ObservedValue, len(observations.Inputs))
		for key, value := range observations.Inputs {
			cloned.Inputs[key] = cloneObservedValue(value)
		}
	}

	if len(observations.Outputs) != 0 {
		cloned.Outputs = make(map[string]ObservedValue, len(observations.Outputs))
		for key, value := range observations.Outputs {
			cloned.Outputs[key] = cloneObservedValue(value)
		}
	}

	if len(observations.Streams) != 0 {
		cloned.Streams = make(map[string]ObservedStream, len(observations.Streams))
		for key, value := range observations.Streams {
			cloned.Streams[key] = cloneObservedStream(value)
		}
	}

	return cloned
}

type scheduledScenarioRun struct {
	call          scenarioCallPlan
	scenario      *scenarioPlan
	bindingSource Values
	identity      executionIdentity
}

type scenarioBatchResult struct {
	callID string
	state  scenarioState
	err    error
}
