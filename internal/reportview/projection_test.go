package reportview

import (
	"testing"
	"time"

	"github.com/alex-poliushkin/theater"
)

func TestProjectionBuildsScenarioViewsFromSharedReportState(t *testing.T) {
	t.Parallel()

	scenarioPath := "stage.main/call.login-user"
	now := time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC)
	failedScenario := theater.NodeReport{
		Kind:           theater.NodeKindScenario,
		Path:           scenarioPath,
		ScenarioID:     "auth/login",
		ScenarioCallID: "login-user",
		ScenarioPath:   scenarioPath,
		Status:         theater.StatusFailed,
		Failure: &theater.Failure{
			Kind:    theater.FailureKindExpectation,
			Phase:   theater.PhaseRun,
			At:      scenarioPath,
			Summary: "login failed",
		},
		StartedAt:  now,
		EndedAt:    now.Add(2 * time.Second),
		DurationMs: 2000,
		SourceSpan: &theater.SourceRef{File: "scenario.yaml", Line: 10},
	}
	expectationNode := theater.NodeReport{
		Kind:         theater.NodeKindExpectation,
		Path:         scenarioPath + "/act.submit/expectation.status",
		ScenarioPath: scenarioPath,
		Status:       theater.StatusFailed,
		Failure: &theater.Failure{
			Kind:    theater.FailureKindExpectation,
			Phase:   theater.PhaseRun,
			At:      scenarioPath + "/act.submit/expectation.status",
			Summary: "status mismatch",
		},
		Address:    &theater.NodeAddress{ActID: "submit"},
		SourceSpan: &theater.SourceRef{File: "expectation.yaml", Line: 21},
	}
	actionNode := theater.NodeReport{
		Kind:         theater.NodeKindAction,
		Path:         scenarioPath + "/act.submit/action",
		ScenarioPath: scenarioPath,
		Status:       theater.StatusFailed,
		Failure: &theater.Failure{
			Kind:    theater.FailureKindAction,
			Phase:   theater.PhaseRun,
			At:      scenarioPath + "/act.submit/action",
			Summary: "action failed",
		},
		Address: &theater.NodeAddress{ActID: "submit"},
	}
	eventuallyNode := theater.NodeReport{
		Kind:         theater.NodeKindAct,
		Path:         scenarioPath + "/act.submit",
		ScenarioPath: scenarioPath,
		Status:       theater.StatusFailed,
		Eventually: &theater.EventuallyReport{
			AttemptsTotal:     4,
			FinalOutcome:      theater.StatusFailed,
			TerminationReason: theater.TerminationReasonDeadlineExceeded,
			LastObservedFailure: &theater.Failure{
				Kind:    theater.FailureKindExpectation,
				Phase:   theater.PhaseRun,
				At:      scenarioPath + "/act.submit",
				Summary: "still failing",
			},
		},
	}
	skippedScenario := theater.NodeReport{
		Kind:           theater.NodeKindScenario,
		Path:           "stage.main/call.pending",
		ScenarioID:     "auth/pending",
		ScenarioCallID: "pending",
		ScenarioPath:   "stage.main/call.pending",
		Status:         theater.StatusSkipped,
		SkipReason:     theater.SkipReasonStageAborted,
	}

	projection := New(theater.RunDocument{
		ReportSchemaVersion: theater.RunDocumentSchemaVersion,
		Report: theater.Report{
			StageID:   "main",
			StagePath: "stage.main",
			Status:    theater.StatusFailed,
			Failure: &theater.Failure{
				Kind:    theater.FailureKindExpectation,
				Phase:   theater.PhaseRun,
				At:      scenarioPath,
				Summary: "stage failed",
			},
			Nodes: []theater.NodeReport{
				failedScenario,
				expectationNode,
				actionNode,
				eventuallyNode,
				skippedScenario,
			},
		},
	})

	if got, want := len(projection.Scenarios), 2; got != want {
		t.Fatalf("scenario count mismatch: got %d want %d", got, want)
	}

	failedView := projection.Scenarios[0]
	if got, want := failedView.Node.ScenarioCallID, "login-user"; got != want {
		t.Fatalf("scenario call mismatch: got %q want %q", got, want)
	}
	if failedView.PrimaryFailure == nil || failedView.PrimaryFailure.Kind != theater.NodeKindExpectation {
		t.Fatalf("primary failure must select expectation node: %#v", failedView.PrimaryFailure)
	}
	if failedView.TerminalFailure == nil || failedView.TerminalFailure.Path != expectationNode.Path {
		t.Fatalf("terminal failure mismatch: %#v", failedView.TerminalFailure)
	}
	if failedView.SourceSpan == nil || failedView.SourceSpan.File != "expectation.yaml" {
		t.Fatalf("source span must come from primary failure: %#v", failedView.SourceSpan)
	}
	if failedView.Eventually == nil || failedView.Eventually.AttemptsTotal != 4 {
		t.Fatalf("eventually summary mismatch: %#v", failedView.Eventually)
	}

	skippedView := projection.Scenarios[1]
	if got, want := skippedView.Node.SkipReason, theater.SkipReasonStageAborted; got != want {
		t.Fatalf("skip reason mismatch: got %q want %q", got, want)
	}

	if !projection.HasFailedScenario() {
		t.Fatal("projection must report failed scenario presence")
	}
}

func TestProjectionTracksConvergedActsAndOrphanStageFailure(t *testing.T) {
	t.Parallel()

	scenarioPath := "stage.main/call.login-user"
	projection := New(theater.RunDocument{
		ReportSchemaVersion: theater.RunDocumentSchemaVersion,
		Report: theater.Report{
			StageID:   "main",
			StagePath: "stage.main",
			Status:    theater.StatusFailed,
			Failure: &theater.Failure{
				Kind:    theater.FailureKindSetup,
				Phase:   theater.PhaseRun,
				At:      "stage.main",
				Summary: "run aborted",
			},
			Nodes: []theater.NodeReport{
				{
					Kind:         theater.NodeKindAct,
					Path:         scenarioPath + "/act.submit",
					ScenarioPath: scenarioPath,
					Status:       theater.StatusPassed,
					Eventually: &theater.EventuallyReport{
						AttemptsTotal:     3,
						FinalOutcome:      theater.StatusPassed,
						TerminationReason: theater.TerminationReasonConverged,
					},
				},
				{
					Kind:           theater.NodeKindScenario,
					Path:           scenarioPath,
					ScenarioID:     "auth/login",
					ScenarioCallID: "login-user",
					ScenarioPath:   scenarioPath,
					Status:         theater.StatusPassed,
				},
			},
		},
	})

	if got, want := projection.ConvergedActs, 1; got != want {
		t.Fatalf("converged acts mismatch: got %d want %d", got, want)
	}
	if got, want := projection.ExtraAttempts, 2; got != want {
		t.Fatalf("extra attempts mismatch: got %d want %d", got, want)
	}
	if projection.HasFailedScenario() {
		t.Fatal("projection must not report failed scenarios when only stage failure exists")
	}
}

func TestProjectionPrefersLatestEventuallyFailure(t *testing.T) {
	t.Parallel()

	projection := New(eventuallyLatestFailureDocument())
	if got, want := len(projection.Scenarios), 1; got != want {
		t.Fatalf("scenario count mismatch: got %d want %d", got, want)
	}

	scenario := projection.Scenarios[0]
	if scenario.PrimaryFailure == nil {
		t.Fatal("primary failure must be selected")
	}
	if got, want := scenario.PrimaryFailure.Kind, theater.NodeKindAction; got != want {
		t.Fatalf("primary failure kind mismatch: got %s want %s", got, want)
	}
	if got, want := scenario.PrimaryFailure.Attempt, 3; got != want {
		t.Fatalf("primary failure attempt mismatch: got %d want %d", got, want)
	}
	if scenario.PrimaryFailure.Observations == nil {
		t.Fatal("primary failure observations must be preserved")
	}
	if got, want := scenario.PrimaryFailure.Observations.Outputs["response"].Preview.Text, "latest action payload"; got != want {
		t.Fatalf("primary failure output mismatch: got %q want %q", got, want)
	}
	if scenario.TerminalFailure == nil {
		t.Fatal("terminal failure must be selected")
	}
	if got, want := scenario.TerminalFailure.Kind, theater.NodeKindAct; got != want {
		t.Fatalf("terminal failure kind mismatch: got %s want %s", got, want)
	}
	if got, want := scenario.TerminalFailure.Failure.Kind, theater.FailureKindTimeout; got != want {
		t.Fatalf("terminal failure category mismatch: got %s want %s", got, want)
	}
	if scenario.SourceSpan == nil || scenario.SourceSpan.File != "action_latest.yaml" {
		t.Fatalf("source span must come from latest primary failure: %#v", scenario.SourceSpan)
	}
}

func eventuallyLatestFailureDocument() theater.RunDocument {
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
		ReportSchemaVersion: theater.RunDocumentSchemaVersion,
		Report: theater.Report{
			StageID:    "main",
			StagePath:  "stage.main",
			Status:     theater.StatusFailed,
			StartedAt:  base,
			EndedAt:    base.Add(31 * time.Second),
			DurationMs: (31 * time.Second).Milliseconds(),
			Failure:    timeoutFailure,
			Summary: theater.Summary{
				TotalScenarios:  1,
				FailedScenarios: 1,
			},
			Nodes: []theater.NodeReport{
				{
					Kind:           theater.NodeKindExpectation,
					StageID:        "main",
					Path:           scenarioPath + "/act.wait_ready/expectation.status",
					ScenarioID:     "orders/wait_ready",
					ScenarioCallID: "wait_ready",
					ScenarioPath:   scenarioPath,
					Attempt:        1,
					ScenarioSeq:    1,
					Status:         theater.StatusFailed,
					Failure:        staleExpectationFailure,
					StartedAt:      base.Add(2 * time.Second),
					EndedAt:        base.Add(3 * time.Second),
					DurationMs:     time.Second.Milliseconds(),
					Address: &theater.NodeAddress{
						ScenarioCallPath: scenarioPath,
						ActID:            "wait_ready",
						Kind:             theater.NodeKindExpectation,
						NodeRef:          "status",
						Phase:            "assert.evaluate",
						AttemptIndex:     1,
					},
					SourceSpan: &theater.SourceRef{File: "expectation_attempt1.yaml", Line: 18},
				},
				{
					Kind:           theater.NodeKindAction,
					StageID:        "main",
					Path:           actionPath,
					ScenarioID:     "orders/wait_ready",
					ScenarioCallID: "wait_ready",
					ScenarioPath:   scenarioPath,
					Attempt:        2,
					ScenarioSeq:    1,
					Status:         theater.StatusFailed,
					Failure:        olderActionFailure,
					StartedAt:      base.Add(10 * time.Second),
					EndedAt:        base.Add(11 * time.Second),
					DurationMs:     time.Second.Milliseconds(),
					Address: &theater.NodeAddress{
						ScenarioCallPath: scenarioPath,
						ActID:            "wait_ready",
						Kind:             theater.NodeKindAction,
						NodeRef:          "action",
						Phase:            "action.execute",
						AttemptIndex:     2,
					},
					SourceSpan: &theater.SourceRef{File: "action_older.yaml", Line: 22},
					Observations: &theater.ActionObservations{
						Outputs: map[string]theater.ObservedValue{
							"response": {Preview: &theater.Preview{Kind: "string", Text: "older action payload"}},
						},
					},
				},
				{
					Kind:           theater.NodeKindAction,
					StageID:        "main",
					Path:           actionPath,
					ScenarioID:     "orders/wait_ready",
					ScenarioCallID: "wait_ready",
					ScenarioPath:   scenarioPath,
					Attempt:        3,
					ScenarioSeq:    1,
					Status:         theater.StatusFailed,
					Failure:        latestActionFailure,
					StartedAt:      base.Add(20 * time.Second),
					EndedAt:        base.Add(21 * time.Second),
					DurationMs:     time.Second.Milliseconds(),
					Address: &theater.NodeAddress{
						ScenarioCallPath: scenarioPath,
						ActID:            "wait_ready",
						Kind:             theater.NodeKindAction,
						NodeRef:          "action",
						Phase:            "action.execute",
						AttemptIndex:     3,
					},
					SourceSpan: &theater.SourceRef{File: "action_latest.yaml", Line: 27},
					Observations: &theater.ActionObservations{
						Outputs: map[string]theater.ObservedValue{
							"response": {Preview: &theater.Preview{Kind: "string", Text: "latest action payload"}},
						},
					},
					Contrast: &theater.Contrast{
						Summary: "latest response drift",
						Actual:  &theater.Preview{Kind: "string", Text: "HTTP 503"},
					},
				},
				{
					Kind:           theater.NodeKindAct,
					StageID:        "main",
					Path:           actPath,
					ScenarioID:     "orders/wait_ready",
					ScenarioCallID: "wait_ready",
					ScenarioPath:   scenarioPath,
					Attempt:        3,
					ScenarioSeq:    1,
					Status:         theater.StatusFailed,
					Failure:        timeoutFailure,
					StartedAt:      base.Add(time.Second),
					EndedAt:        base.Add(30 * time.Second),
					DurationMs:     (29 * time.Second).Milliseconds(),
					Address: &theater.NodeAddress{
						ScenarioCallPath: scenarioPath,
						ActID:            "wait_ready",
						Kind:             theater.NodeKindAct,
						Phase:            "act.execute",
						AttemptIndex:     3,
					},
					Eventually: &theater.EventuallyReport{
						Enabled:             true,
						Timeout:             "30s",
						Interval:            "2s",
						AttemptsTotal:       3,
						ElapsedMs:           (30 * time.Second).Milliseconds(),
						FinalOutcome:        theater.StatusFailed,
						TerminationReason:   theater.TerminationReasonDeadlineExceeded,
						FinalFailureReason:  timeoutFailure,
						LastObservedFailure: latestActionFailure,
						AttemptTimeline: []theater.AttemptReport{
							{Index: 1, Status: theater.StatusFailed, Failure: staleExpectationFailure},
							{Index: 2, Status: theater.StatusFailed, Failure: olderActionFailure},
							{Index: 3, Status: theater.StatusFailed, Failure: latestActionFailure},
						},
					},
				},
				{
					Kind:           theater.NodeKindScenario,
					StageID:        "main",
					Path:           scenarioPath,
					ScenarioID:     "orders/wait_ready",
					ScenarioCallID: "wait_ready",
					ScenarioPath:   scenarioPath,
					Attempt:        1,
					ScenarioSeq:    1,
					Status:         theater.StatusFailed,
					Failure:        timeoutFailure,
					StartedAt:      base,
					EndedAt:        base.Add(31 * time.Second),
					DurationMs:     (31 * time.Second).Milliseconds(),
				},
			},
		},
	}
}
