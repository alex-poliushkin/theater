package junit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/junit"
)

func TestExporterMarshal(t *testing.T) {
	t.Parallel()

	exporter := junit.NewExporter()
	for _, testcase := range []struct {
		name string
		doc  theater.RunDocument
	}{
		{name: "passed", doc: passedDocument()},
		{name: "expectation_failure", doc: expectationFailureDocument()},
		{name: "eventually_timeout", doc: eventuallyTimeoutDocument()},
		{name: "canceled", doc: canceledDocument()},
		{name: "validation_failure", doc: validationFailureDocument()},
	} {
		testcase := testcase
		t.Run(testcase.name, func(t *testing.T) {
			t.Parallel()

			got, err := exporter.Marshal(completeRunDocument(testcase.doc))
			if err != nil {
				t.Fatalf("marshal junit failed: %v", err)
			}

			want := readGolden(t, testcase.name+".xml")
			if diff := compareXML(string(got), want); diff != "" {
				t.Fatalf("golden mismatch:\n%s", diff)
			}

			if strings.Contains(string(got), "<system-out>") || strings.Contains(string(got), "<system-err>") {
				t.Fatalf("minimal exporter must not emit system-out/system-err: %s", got)
			}
		})
	}
}

func TestExporterUsesExplicitScenarioIdentityNotInternalPath(t *testing.T) {
	t.Parallel()

	exporter := junit.NewExporter()
	got, err := exporter.Marshal(completeRunDocument(passedDocument()))
	if err != nil {
		t.Fatalf("marshal junit failed: %v", err)
	}

	xmlText := string(got)
	if !strings.Contains(xmlText, `classname="auth/login"`) {
		t.Fatalf("expected scenario classname in XML: %s", xmlText)
	}

	if !strings.Contains(xmlText, `name="login_user"`) {
		t.Fatalf("expected scenario call name in XML: %s", xmlText)
	}

	if strings.Contains(xmlText, `classname="stage.main/call.login_user"`) {
		t.Fatalf("classname must not fall back to internal call path: %s", xmlText)
	}
}

func TestExporterOmitsStageAbortedPendingScenarios(t *testing.T) {
	t.Parallel()

	exporter := junit.NewExporter()
	got, err := exporter.Marshal(completeRunDocument(stageAbortDocument()))
	if err != nil {
		t.Fatalf("marshal junit failed: %v", err)
	}

	xmlText := string(got)
	if strings.Contains(xmlText, `name="notify_user"`) {
		t.Fatalf("stage-aborted pending scenario must not become testcase: %s", xmlText)
	}

	if !strings.Contains(xmlText, `name="register_user"`) {
		t.Fatalf("failing started scenario testcase must stay present: %s", xmlText)
	}

	if strings.Contains(xmlText, "<skipped") {
		t.Fatalf("stage-aborted pending scenario must not be exported as skipped testcase: %s", xmlText)
	}
}

func TestExporterPrefersLatestEventuallyFailureObservations(t *testing.T) {
	t.Parallel()

	exporter := junit.NewExporter()
	got, err := exporter.Marshal(completeRunDocument(eventuallyLatestFailureDocument()))
	if err != nil {
		t.Fatalf("marshal junit failed: %v", err)
	}

	xmlText := string(got)
	if !strings.Contains(xmlText, "output.response: latest action payload") {
		t.Fatalf("junit output must include latest observed payload: %s", xmlText)
	}
	if strings.Contains(xmlText, "output.response: older action payload") {
		t.Fatalf("junit output must not include stale observed payload: %s", xmlText)
	}
	if !strings.Contains(xmlText, "source: action_latest.yaml") {
		t.Fatalf("junit output must point to latest failure source: %s", xmlText)
	}
}

func TestExporterRendersHTTPDiagnostics(t *testing.T) {
	t.Parallel()

	exporter := junit.NewExporter()
	got, err := exporter.Marshal(completeRunDocument(httpDiagnosticFailureDocument()))
	if err != nil {
		t.Fatalf("marshal junit failed: %v", err)
	}

	xmlText := string(got)
	for _, want := range []string{
		"http.request: GET https://api.example.test/redacted?token=redacted",
		"http.response: 502 Bad Gateway",
		"http.header.x-request-id: req-123",
		"http.body:",
		"retry later",
		"[redacted]",
		"(redacted)",
	} {
		if !strings.Contains(xmlText, want) {
			t.Fatalf("junit output missing %q: %s", want, xmlText)
		}
	}
	if strings.Contains(xmlText, "credential-secret") {
		t.Fatalf("junit output leaked secret: %s", xmlText)
	}
}

func passedDocument() theater.RunDocument {
	startedAt := fixedTime(0)
	endedAt := fixedTime(1000)
	scenarioStartedAt := fixedTime(100)
	scenarioEndedAt := fixedTime(350)

	return theater.RunDocument{
		ReportSchemaVersion: theater.RunDocumentSchemaVersion,
		Report: theater.Report{
			StageID:    "main",
			StagePath:  "stage.main",
			Status:     theater.StatusPassed,
			StartedAt:  startedAt,
			EndedAt:    endedAt,
			DurationMs: endedAt.Sub(startedAt).Milliseconds(),
			Summary: theater.Summary{
				TotalScenarios:  1,
				PassedScenarios: 1,
			},
			Nodes: []theater.NodeReport{
				{
					Kind:           theater.NodeKindScenario,
					StageID:        "main",
					Path:           "stage.main/call.login_user",
					ScenarioID:     "auth/login",
					ScenarioCallID: "login_user",
					ScenarioPath:   "stage.main/call.login_user",
					Attempt:        1,
					ScenarioSeq:    1,
					Status:         theater.StatusPassed,
					StartedAt:      scenarioStartedAt,
					EndedAt:        scenarioEndedAt,
					DurationMs:     scenarioEndedAt.Sub(scenarioStartedAt).Milliseconds(),
				},
			},
		},
	}
}

func expectationFailureDocument() theater.RunDocument {
	startedAt := fixedTime(0)
	endedAt := fixedTime(1500)
	scenarioStartedAt := fixedTime(100)
	scenarioEndedAt := fixedTime(850)
	failure := &theater.Failure{
		Kind:    theater.FailureKindExpectation,
		Phase:   theater.PhaseRun,
		At:      "stage.main/call.login_user/act.submit/expectation.token",
		Summary: "token mismatch",
	}

	return theater.RunDocument{
		ReportSchemaVersion: theater.RunDocumentSchemaVersion,
		Report: theater.Report{
			StageID:    "main",
			StagePath:  "stage.main",
			Status:     theater.StatusFailed,
			StartedAt:  startedAt,
			EndedAt:    endedAt,
			DurationMs: endedAt.Sub(startedAt).Milliseconds(),
			Failure:    failure,
			Summary: theater.Summary{
				TotalScenarios:  1,
				FailedScenarios: 1,
			},
			Nodes: []theater.NodeReport{
				{
					Kind:           theater.NodeKindExpectation,
					StageID:        "main",
					Path:           "stage.main/call.login_user/act.submit/expectation.token",
					ScenarioID:     "auth/login",
					ScenarioCallID: "login_user",
					ScenarioPath:   "stage.main/call.login_user",
					Attempt:        1,
					ScenarioSeq:    1,
					Status:         theater.StatusFailed,
					Failure:        failure,
					StartedAt:      fixedTime(400),
					EndedAt:        fixedTime(450),
					DurationMs:     50,
					Address: &theater.NodeAddress{
						ScenarioCallPath: "stage.main/call.login_user",
						ActID:            "submit",
						Kind:             theater.NodeKindExpectation,
						NodeRef:          "token",
						Phase:            "assert.evaluate",
					},
					SourceSpan: &theater.SourceRef{
						File:   "theater/flows/login.yaml",
						Line:   18,
						Column: 5,
					},
				},
				{
					Kind:           theater.NodeKindScenario,
					StageID:        "main",
					Path:           "stage.main/call.login_user",
					ScenarioID:     "auth/login",
					ScenarioCallID: "login_user",
					ScenarioPath:   "stage.main/call.login_user",
					Attempt:        1,
					ScenarioSeq:    1,
					Status:         theater.StatusFailed,
					Failure:        failure,
					StartedAt:      scenarioStartedAt,
					EndedAt:        scenarioEndedAt,
					DurationMs:     scenarioEndedAt.Sub(scenarioStartedAt).Milliseconds(),
				},
			},
		},
	}
}

func httpDiagnosticFailureDocument() theater.RunDocument {
	scenarioPath := "stage.main/call.probe"
	expectationPath := scenarioPath + "/act.fetch/expectation.status"
	failure := &theater.Failure{
		Kind:    theater.FailureKindExpectation,
		Phase:   theater.PhaseRun,
		At:      expectationPath,
		Summary: "status mismatch",
	}

	return theater.RunDocument{
		ReportSchemaVersion: theater.RunDocumentSchemaVersion,
		Report: theater.Report{
			StageID:   "main",
			StagePath: "stage.main",
			Status:    theater.StatusFailed,
			Failure:   failure,
			Summary: theater.Summary{
				TotalScenarios:  1,
				FailedScenarios: 1,
			},
			Nodes: []theater.NodeReport{
				{
					Kind:           theater.NodeKindExpectation,
					StageID:        "main",
					Path:           expectationPath,
					ScenarioID:     "http/probe",
					ScenarioCallID: "probe",
					ScenarioPath:   scenarioPath,
					Attempt:        1,
					ScenarioSeq:    1,
					Status:         theater.StatusFailed,
					Failure:        failure,
					Address: &theater.NodeAddress{
						ScenarioCallPath: scenarioPath,
						ActID:            "fetch",
						Kind:             theater.NodeKindExpectation,
						NodeRef:          "status",
					},
					Diagnostics: []theater.NodeDiagnostic{
						{
							Kind: theater.NodeDiagnosticKindHTTP,
							HTTP: &theater.HTTPDiagnostic{
								ActionAddress: &theater.NodeAddress{
									ScenarioCallPath: scenarioPath,
									ActID:            "fetch",
									Kind:             theater.NodeKindAction,
								},
								Method:     "GET",
								URL:        "https://api.example.test/redacted?token=redacted",
								StatusCode: 502,
								Status:     "Bad Gateway",
								DurationMs: 15,
								ResponseHeaders: map[string][]string{
									"x-request-id": {"req-123"},
								},
								ResponsePreview: &theater.Preview{
									Kind:        "json",
									Text:        `{"message":"retry later","token":"[redacted]"}`,
									SizeHint:    96,
									Redacted:    true,
									ContentType: "application/json",
								},
							},
						},
					},
				},
				{
					Kind:           theater.NodeKindScenario,
					StageID:        "main",
					Path:           scenarioPath,
					ScenarioID:     "http/probe",
					ScenarioCallID: "probe",
					ScenarioPath:   scenarioPath,
					Attempt:        1,
					ScenarioSeq:    1,
					Status:         theater.StatusFailed,
					Failure:        failure,
				},
			},
		},
	}
}

func eventuallyTimeoutDocument() theater.RunDocument {
	startedAt := fixedTime(0)
	endedAt := fixedTime(32000)
	scenarioStartedAt := fixedTime(100)
	scenarioEndedAt := fixedTime(30100)
	expectationFailure := &theater.Failure{
		Kind:    theater.FailureKindExpectation,
		Phase:   theater.PhaseRun,
		At:      "stage.main/call.login_user/act.wait_ready/expectation.status",
		Summary: "expected READY got PENDING",
	}
	timeoutFailure := &theater.Failure{
		Kind:    theater.FailureKindTimeout,
		Phase:   theater.PhaseRun,
		At:      "stage.main/call.login_user/act.wait_ready",
		Summary: "eventually deadline exceeded",
	}

	return theater.RunDocument{
		ReportSchemaVersion: theater.RunDocumentSchemaVersion,
		Report: theater.Report{
			StageID:    "main",
			StagePath:  "stage.main",
			Status:     theater.StatusFailed,
			StartedAt:  startedAt,
			EndedAt:    endedAt,
			DurationMs: endedAt.Sub(startedAt).Milliseconds(),
			Failure:    timeoutFailure,
			Summary: theater.Summary{
				TotalScenarios:  1,
				FailedScenarios: 1,
			},
			Nodes: []theater.NodeReport{
				{
					Kind:           theater.NodeKindExpectation,
					StageID:        "main",
					Path:           "stage.main/call.login_user/act.wait_ready/expectation.status",
					ScenarioID:     "orders/check_ready",
					ScenarioCallID: "login_user",
					ScenarioPath:   "stage.main/call.login_user",
					Attempt:        3,
					ScenarioSeq:    1,
					Status:         theater.StatusFailed,
					Failure:        expectationFailure,
					StartedAt:      fixedTime(28000),
					EndedAt:        fixedTime(28100),
					DurationMs:     100,
					Address: &theater.NodeAddress{
						ScenarioCallPath: "stage.main/call.login_user",
						ActID:            "wait_ready",
						Kind:             theater.NodeKindExpectation,
						NodeRef:          "status",
						Phase:            "assert.evaluate",
					},
				},
				{
					Kind:           theater.NodeKindAct,
					StageID:        "main",
					Path:           "stage.main/call.login_user/act.wait_ready",
					ScenarioID:     "orders/check_ready",
					ScenarioCallID: "login_user",
					ScenarioPath:   "stage.main/call.login_user",
					Attempt:        3,
					ScenarioSeq:    1,
					Status:         theater.StatusFailed,
					Failure:        timeoutFailure,
					StartedAt:      fixedTime(100),
					EndedAt:        fixedTime(30100),
					DurationMs:     30000,
					Address: &theater.NodeAddress{
						ScenarioCallPath: "stage.main/call.login_user",
						ActID:            "wait_ready",
						Kind:             theater.NodeKindAct,
						Phase:            "act.execute",
					},
					Eventually: &theater.EventuallyReport{
						Enabled:             true,
						Timeout:             "30s",
						Interval:            "2s",
						AttemptsTotal:       3,
						ElapsedMs:           30000,
						FinalOutcome:        theater.StatusFailed,
						TerminationReason:   theater.TerminationReasonDeadlineExceeded,
						FinalFailureReason:  timeoutFailure,
						LastObservedFailure: expectationFailure,
						AttemptTimeline: []theater.AttemptReport{
							{Index: 1, Status: theater.StatusFailed, Failure: expectationFailure},
							{Index: 2, Status: theater.StatusFailed, Failure: expectationFailure},
							{Index: 3, Status: theater.StatusFailed, Failure: expectationFailure},
						},
					},
				},
				{
					Kind:           theater.NodeKindScenario,
					StageID:        "main",
					Path:           "stage.main/call.login_user",
					ScenarioID:     "orders/check_ready",
					ScenarioCallID: "login_user",
					ScenarioPath:   "stage.main/call.login_user",
					Attempt:        1,
					ScenarioSeq:    1,
					Status:         theater.StatusFailed,
					Failure:        timeoutFailure,
					StartedAt:      scenarioStartedAt,
					EndedAt:        scenarioEndedAt,
					DurationMs:     scenarioEndedAt.Sub(scenarioStartedAt).Milliseconds(),
				},
			},
		},
	}
}

func canceledDocument() theater.RunDocument {
	startedAt := fixedTime(0)
	endedAt := fixedTime(900)
	scenarioStartedAt := fixedTime(100)
	scenarioEndedAt := fixedTime(400)

	return theater.RunDocument{
		ReportSchemaVersion: theater.RunDocumentSchemaVersion,
		Report: theater.Report{
			StageID:    "main",
			StagePath:  "stage.main",
			Status:     theater.StatusCanceled,
			StartedAt:  startedAt,
			EndedAt:    endedAt,
			DurationMs: endedAt.Sub(startedAt).Milliseconds(),
			Summary: theater.Summary{
				TotalScenarios:    1,
				CanceledScenarios: 1,
			},
			Nodes: []theater.NodeReport{
				{
					Kind:           theater.NodeKindScenario,
					StageID:        "main",
					Path:           "stage.main/call.login_user",
					ScenarioID:     "auth/login",
					ScenarioCallID: "login_user",
					ScenarioPath:   "stage.main/call.login_user",
					Attempt:        1,
					ScenarioSeq:    1,
					Status:         theater.StatusCanceled,
					StartedAt:      scenarioStartedAt,
					EndedAt:        scenarioEndedAt,
					DurationMs:     scenarioEndedAt.Sub(scenarioStartedAt).Milliseconds(),
				},
			},
		},
	}
}

func validationFailureDocument() theater.RunDocument {
	return theater.RunDocument{
		ReportSchemaVersion: theater.RunDocumentSchemaVersion,
		Diagnostics: []theater.Diagnostic{
			{
				Code:     "missing_scenario_id",
				Path:     "stage.main/call.login_user",
				Severity: theater.SeverityError,
				Summary:  "scenario call must reference a scenario",
				Span: theater.SourceRef{
					File:   "theater/flows/login.yaml",
					Line:   10,
					Column: 3,
				},
			},
		},
		Report: theater.Report{
			StageID:   "main",
			StagePath: "stage.main",
			Status:    theater.StatusFailed,
			Failure: &theater.Failure{
				Kind:    theater.FailureKindDefinition,
				Phase:   theater.PhaseValidate,
				At:      "stage.main",
				Summary: "validation failed with 1 diagnostic(s)",
			},
		},
	}
}

func stageAbortDocument() theater.RunDocument {
	failure := &theater.Failure{
		Kind:    theater.FailureKindAction,
		Phase:   theater.PhaseRun,
		At:      "stage.main/call.register_user/act.submit",
		Summary: "registration failed",
	}

	return theater.RunDocument{
		ReportSchemaVersion: theater.RunDocumentSchemaVersion,
		Report: theater.Report{
			StageID:   "main",
			StagePath: "stage.main",
			Status:    theater.StatusFailed,
			Failure:   failure,
			Summary: theater.Summary{
				TotalScenarios:   2,
				FailedScenarios:  1,
				SkippedScenarios: 1,
			},
			Nodes: []theater.NodeReport{
				{
					Kind:           theater.NodeKindAction,
					StageID:        "main",
					Path:           "stage.main/call.register_user/act.submit",
					ScenarioID:     "users/register",
					ScenarioCallID: "register_user",
					ScenarioPath:   "stage.main/call.register_user",
					Attempt:        1,
					ScenarioSeq:    1,
					Status:         theater.StatusFailed,
					Failure:        failure,
					Address: &theater.NodeAddress{
						ScenarioCallPath: "stage.main/call.register_user",
						ActID:            "submit",
						Kind:             theater.NodeKindAction,
						NodeRef:          "action",
						Phase:            "action.execute",
					},
				},
				{
					Kind:           theater.NodeKindScenario,
					StageID:        "main",
					Path:           "stage.main/call.register_user",
					ScenarioID:     "users/register",
					ScenarioCallID: "register_user",
					ScenarioPath:   "stage.main/call.register_user",
					Attempt:        1,
					ScenarioSeq:    1,
					Status:         theater.StatusFailed,
					Failure:        failure,
				},
				{
					Kind:           theater.NodeKindScenario,
					StageID:        "main",
					Path:           "stage.main/call.notify_user",
					ScenarioID:     "users/notify",
					ScenarioCallID: "notify_user",
					ScenarioPath:   "stage.main/call.notify_user",
					Attempt:        1,
					ScenarioSeq:    2,
					Status:         theater.StatusSkipped,
					SkipReason:     theater.SkipReasonStageAborted,
				},
			},
		},
	}
}

func eventuallyLatestFailureDocument() theater.RunDocument {
	scenarioPath := "stage.main/call.wait_ready"
	actionPath := scenarioPath + "/act.wait_ready/action"
	actPath := scenarioPath + "/act.wait_ready"
	base := fixedTime(0)

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

func fixedTime(offsetMs int64) time.Time {
	return time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC).Add(time.Duration(offsetMs) * time.Millisecond)
}

func completeRunDocument(doc theater.RunDocument) theater.RunDocument {
	doc.TheaterVersion = "dev"
	if doc.RunID == "" {
		doc.RunID = doc.Report.StageID + "/test"
	}
	for i := range doc.Report.Nodes {
		if doc.Report.Nodes[i].ID == "" {
			doc.Report.Nodes[i].ID = doc.Report.Nodes[i].Path
		}
	}
	return doc
}

func readGolden(t *testing.T, name string) string {
	t.Helper()

	bytes, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read golden failed: %v", err)
	}

	return string(bytes)
}

func compareXML(got, want string) string {
	got = strings.TrimSpace(got)
	want = strings.TrimSpace(want)
	if got == want {
		return ""
	}

	return "got:\n" + got + "\nwant:\n" + want
}
