package theatercli

import (
	"strings"
	"testing"
	"time"

	"github.com/alex-poliushkin/theater"
)

func TestRunTextViewPrefersLatestEventuallyFailure(t *testing.T) {
	t.Parallel()

	output := newRunTextView("run.json", eventuallyLatestFailureDocumentForText()).String()
	if !strings.Contains(output, "summary: latest action failed") {
		t.Fatalf("text output must show latest action summary: %s", output)
	}
	if strings.Contains(output, "summary: stale expectation mismatch") {
		t.Fatalf("text output must not show stale expectation summary: %s", output)
	}
	if !strings.Contains(output, "response: latest action payload") {
		t.Fatalf("text output must include latest observations: %s", output)
	}
	if strings.Contains(output, "response: older action payload") {
		t.Fatalf("text output must not include stale observations: %s", output)
	}
}

func eventuallyLatestFailureDocumentForText() theater.RunDocument {
	scenarioPath := "stage.main/call.wait_ready"
	actionPath := scenarioPath + "/act.wait_ready/action"
	actPath := scenarioPath + "/act.wait_ready"
	base := time.Date(2026, 3, 27, 9, 0, 0, 0, time.UTC)

	staleExpectationFailure := &theater.Failure{
		Kind:    theater.FailureKindExpectation,
		Phase:   theater.PhaseRun,
		At:      scenarioPath + "/act.wait_ready/expectation.status",
		Summary: "stale expectation mismatch",
	}
	olderActionFailure := &theater.Failure{
		Kind:    theater.FailureKindAction,
		Phase:   theater.PhaseRun,
		At:      actionPath,
		Summary: "older action failed",
	}
	latestActionFailure := &theater.Failure{
		Kind:    theater.FailureKindAction,
		Phase:   theater.PhaseRun,
		At:      actionPath,
		Summary: "latest action failed",
	}
	timeoutFailure := &theater.Failure{
		Kind:    theater.FailureKindTimeout,
		Phase:   theater.PhaseRun,
		At:      actPath,
		Summary: "eventually deadline exceeded",
	}

	return theater.RunDocument{
		SchemaVersion: theater.RunDocumentSchemaVersion,
		Report: theater.Report{
			StageID:   "main",
			StagePath: "stage.main",
			Status:    theater.StatusFailed,
			Failure:   timeoutFailure,
			Summary: theater.Summary{
				TotalScenarios:  1,
				FailedScenarios: 1,
			},
			Nodes: []theater.NodeReport{
				{
					Kind:         theater.NodeKindExpectation,
					Path:         scenarioPath + "/act.wait_ready/expectation.status",
					ScenarioPath: scenarioPath,
					Attempt:      1,
					ScenarioSeq:  1,
					Status:       theater.StatusFailed,
					Failure:      staleExpectationFailure,
					EndedAt:      base.Add(3 * time.Second),
					Address: &theater.NodeAddress{
						ActID:        "wait_ready",
						Kind:         theater.NodeKindExpectation,
						AttemptIndex: 1,
					},
				},
				{
					Kind:         theater.NodeKindAction,
					Path:         actionPath,
					ScenarioPath: scenarioPath,
					Attempt:      2,
					ScenarioSeq:  1,
					Status:       theater.StatusFailed,
					Failure:      olderActionFailure,
					EndedAt:      base.Add(11 * time.Second),
					Address: &theater.NodeAddress{
						ActID:        "wait_ready",
						Kind:         theater.NodeKindAction,
						AttemptIndex: 2,
					},
					Observations: &theater.ActionObservations{
						Outputs: map[string]theater.ObservedValue{
							"response": {Preview: &theater.Preview{Kind: "string", Text: "older action payload"}},
						},
					},
				},
				{
					Kind:         theater.NodeKindAction,
					Path:         actionPath,
					ScenarioPath: scenarioPath,
					Attempt:      3,
					ScenarioSeq:  1,
					Status:       theater.StatusFailed,
					Failure:      latestActionFailure,
					EndedAt:      base.Add(21 * time.Second),
					Address: &theater.NodeAddress{
						ActID:        "wait_ready",
						Kind:         theater.NodeKindAction,
						AttemptIndex: 3,
					},
					Observations: &theater.ActionObservations{
						Outputs: map[string]theater.ObservedValue{
							"response": {Preview: &theater.Preview{Kind: "string", Text: "latest action payload"}},
						},
					},
				},
				{
					Kind:         theater.NodeKindAct,
					Path:         actPath,
					ScenarioPath: scenarioPath,
					Attempt:      3,
					ScenarioSeq:  1,
					Status:       theater.StatusFailed,
					Failure:      timeoutFailure,
					EndedAt:      base.Add(30 * time.Second),
					Address: &theater.NodeAddress{
						ActID:        "wait_ready",
						Kind:         theater.NodeKindAct,
						AttemptIndex: 3,
					},
					Eventually: &theater.EventuallyReport{
						Enabled:             true,
						AttemptsTotal:       3,
						FinalOutcome:        theater.StatusFailed,
						TerminationReason:   theater.TerminationReasonDeadlineExceeded,
						LastObservedFailure: latestActionFailure,
					},
				},
				{
					Kind:           theater.NodeKindScenario,
					Path:           scenarioPath,
					ScenarioID:     "orders/wait_ready",
					ScenarioCallID: "wait_ready",
					ScenarioPath:   scenarioPath,
					Attempt:        1,
					ScenarioSeq:    1,
					Status:         theater.StatusFailed,
					Failure:        timeoutFailure,
				},
			},
		},
	}
}
