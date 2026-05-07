package theater

import "sort"

const stageFailurePolicyFailAfterReadyBatch stageFailurePolicy = "fail_after_ready_batch"

type scenarioState struct {
	Status  Status
	Failure *Failure
	Exports Values
}

type stageFailurePolicy string

type stageExecutionPolicy struct {
	FailureBehavior stageFailurePolicy
}

func readyScenarioCalls(stage *stagePlan, states map[string]scenarioState) []scenarioCallPlan {
	ready := make([]scenarioCallPlan, 0, len(stage.ScenarioCalls))

	for _, scenarioCall := range stage.ScenarioCalls {
		if !isPendingCall(states, scenarioCall.ID) {
			continue
		}

		if !dependenciesSatisfied(scenarioCall.Dependencies, states) {
			continue
		}

		ready = append(ready, scenarioCall)
	}

	sort.Slice(ready, func(i, j int) bool {
		return ready[i].PlanOrdinal < ready[j].PlanOrdinal
	})

	return ready
}

func pendingScenarioCalls(stage *stagePlan, states map[string]scenarioState) []scenarioCallPlan {
	pending := make([]scenarioCallPlan, 0, len(stage.ScenarioCalls))

	for _, scenarioCall := range stage.ScenarioCalls {
		if !isPendingCall(states, scenarioCall.ID) {
			continue
		}

		pending = append(pending, scenarioCall)
	}

	return pending
}

func readyScenarioCallsAfterFailedBatch(ready []scenarioCallPlan) []scenarioCallPlan {
	filtered := make([]scenarioCallPlan, 0, len(ready))

	for _, scenarioCall := range ready {
		if !isAllowedAfterFailure(scenarioCall) {
			continue
		}

		filtered = append(filtered, scenarioCall)
	}

	return filtered
}

func defaultStageExecutionPolicy() stageExecutionPolicy {
	return stageExecutionPolicy{
		FailureBehavior: stageFailurePolicyFailAfterReadyBatch,
	}
}

func (p stageExecutionPolicy) filterReadyAfterFailure(ready []scenarioCallPlan) []scenarioCallPlan {
	switch p.FailureBehavior {
	case stageFailurePolicyFailAfterReadyBatch:
		return readyScenarioCallsAfterFailedBatch(ready)
	default:
		return ready
	}
}

func dependenciesSatisfied(dependencies []scenarioDependencyPlan, states map[string]scenarioState) bool {
	for _, dependency := range dependencies {
		state, ok := states[dependency.CallID]
		if !ok {
			return false
		}

		if !matchesPredicate(state.Status, dependency.When) {
			return false
		}
	}

	return true
}

func isPendingCall(states map[string]scenarioState, callID string) bool {
	if len(states) == 0 {
		return true
	}

	state, ok := states[callID]
	if !ok {
		return true
	}

	return state.Status == "" || state.Status == StatusPending
}

func matchesPredicate(status Status, predicate TriggerPredicate) bool {
	switch predicate {
	case TriggerPredicateSuccess:
		return status == StatusPassed
	case TriggerPredicateFailure:
		return status == StatusFailed
	case TriggerPredicateDone:
		return status.IsTerminal()
	default:
		return false
	}
}

func isAllowedAfterFailure(call scenarioCallPlan) bool {
	for _, dependency := range call.Dependencies {
		if dependency.When == TriggerPredicateFailure || dependency.When == TriggerPredicateDone {
			return true
		}
	}

	return false
}
