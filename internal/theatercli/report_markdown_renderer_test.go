package theatercli

import (
	"strings"
	"testing"

	reportmodel "github.com/alex-poliushkin/theater/report"
)

func TestReportMarkdownRendererShowsPassedScenarioDetail(t *testing.T) {
	t.Parallel()

	document := reportmodel.RunDocument{
		SchemaVersion: reportmodel.RunDocumentSchemaVersion,
		Report: reportmodel.Report{
			StageID:    "mobile-dashboard",
			StagePath:  "stage.mobile-dashboard",
			Status:     reportmodel.StatusPassed,
			DurationMs: 4200,
			Summary: reportmodel.Summary{
				TotalScenarios:  1,
				PassedScenarios: 1,
			},
			Nodes: []reportmodel.NodeReport{
				{
					Kind:           reportmodel.NodeKindScenario,
					Path:           "stage.mobile-dashboard/call.ready",
					ScenarioID:     "mobile/dashboard-ready",
					ScenarioCallID: "ready",
					Status:         reportmodel.StatusPassed,
					DurationMs:     4200,
				},
				{
					Kind:   reportmodel.NodeKindAct,
					Path:   "stage.mobile-dashboard/call.ready/act.wait-customer",
					Status: reportmodel.StatusPassed,
					Address: &reportmodel.NodeAddress{
						ScenarioCallPath: "stage.mobile-dashboard/call.ready",
						ActID:            "wait-customer",
						Kind:             reportmodel.NodeKindAct,
					},
					Eventually: &reportmodel.EventuallyReport{
						Enabled:           true,
						AttemptsTotal:     3,
						ElapsedMs:         4100,
						FinalOutcome:      reportmodel.StatusPassed,
						TerminationReason: reportmodel.TerminationReasonConverged,
						SuccessAttempt:    3,
					},
				},
				{
					Kind:   reportmodel.NodeKindExpectation,
					Path:   "stage.mobile-dashboard/call.ready/act.wait-customer/expectation.status-ok",
					Status: reportmodel.StatusPassed,
					Address: &reportmodel.NodeAddress{
						ScenarioCallPath: "stage.mobile-dashboard/call.ready",
						ActID:            "wait-customer",
						Kind:             reportmodel.NodeKindExpectation,
						NodeRef:          "status-ok",
					},
				},
			},
			Logs: []reportmodel.LogRecord{
				{
					ID:             "customer-route",
					Path:           "stage.mobile-dashboard/call.ready/act.wait-customer/log.customer-route",
					ScenarioID:     "mobile/dashboard-ready",
					ScenarioCallID: "ready",
					ScenarioPath:   "stage.mobile-dashboard/call.ready",
					ActID:          "wait-customer",
					Status:         reportmodel.LogStatusEmitted,
					Preview:        &reportmodel.Preview{Text: `{"status":200}`},
					Address: &reportmodel.NodeAddress{
						ScenarioCallPath: "stage.mobile-dashboard/call.ready",
						ActID:            "wait-customer",
						Kind:             reportmodel.NodeKindLog,
						NodeRef:          "customer-route",
					},
				},
			},
			LogSummary: &reportmodel.LogSummary{
				Records:           1,
				PreviewLimitBytes: reportmodel.DefaultScenarioLogPreviewLimitBytes,
				PerActLimit:       reportmodel.DefaultScenarioLogRecordsPerAct,
				PerRunLimit:       reportmodel.DefaultScenarioLogRecordsPerRun,
			},
		},
	}

	var output strings.Builder
	if err := newReportMarkdownRenderer().Write(&output, "run.json", document); err != nil {
		t.Fatalf("write markdown failed: %v", err)
	}

	for _, want := range []string{
		"- Stage: `mobile-dashboard`",
		"- Duration: `4.2s`",
		"### Scenario `ready`",
		"- Scenario: `mobile/dashboard-ready`",
		"- Act `wait-customer` passed",
		"  - Eventually: attempts=3 termination=converged success_attempt=3 elapsed=4.1s",
		"  - Expectation `status-ok` passed",
		"  - Log `customer-route` emitted: `{\"status\":200}`",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("markdown output missing %q: %q", want, output.String())
		}
	}
}

func TestReportMarkdownRendererUsesReportSafeObservedValues(t *testing.T) {
	t.Parallel()

	document := reportmodel.RunDocument{
		SchemaVersion: reportmodel.RunDocumentSchemaVersion,
		Report: reportmodel.Report{
			StageID:   "redaction-check",
			StagePath: "stage.redaction-check",
			Status:    reportmodel.StatusFailed,
			Failure: &reportmodel.Failure{
				Kind:    reportmodel.FailureKindExpectation,
				Phase:   reportmodel.PhaseRun,
				At:      "stage.redaction-check/call.run/act.check/expectation.status",
				Summary: "expectation failed",
			},
			Summary: reportmodel.Summary{
				TotalScenarios:  1,
				FailedScenarios: 1,
			},
			Nodes: []reportmodel.NodeReport{
				{
					Kind:           reportmodel.NodeKindScenario,
					Path:           "stage.redaction-check/call.run",
					ScenarioID:     "auth/check",
					ScenarioCallID: "run",
					Status:         reportmodel.StatusFailed,
					Failure: &reportmodel.Failure{
						Kind:    reportmodel.FailureKindExpectation,
						Phase:   reportmodel.PhaseRun,
						At:      "stage.redaction-check/call.run/act.check/expectation.status",
						Summary: "expectation failed",
					},
				},
				{
					Kind:   reportmodel.NodeKindExpectation,
					Path:   "stage.redaction-check/call.run/act.check/expectation.status",
					Status: reportmodel.StatusFailed,
					Address: &reportmodel.NodeAddress{
						ScenarioCallPath: "stage.redaction-check/call.run",
						ActID:            "check",
						Kind:             reportmodel.NodeKindExpectation,
						NodeRef:          "status",
					},
					Failure: &reportmodel.Failure{
						Kind:    reportmodel.FailureKindExpectation,
						Phase:   reportmodel.PhaseRun,
						At:      "stage.redaction-check/call.run/act.check/expectation.status",
						Summary: "expectation failed",
					},
					Observations: &reportmodel.ActionObservations{
						Inputs: map[string]reportmodel.ObservedValue{
							"token": {
								Preview: &reportmodel.Preview{Redacted: true, Text: "secret-token"},
							},
						},
						Outputs: map[string]reportmodel.ObservedValue{
							"status": {
								Preview: &reportmodel.Preview{Text: "401"},
							},
						},
					},
				},
			},
		},
	}

	var output strings.Builder
	if err := newReportMarkdownRenderer().Write(&output, "run.json", document); err != nil {
		t.Fatalf("write markdown failed: %v", err)
	}

	for _, want := range []string{
		"Failure: expectation failed",
		"Kind: `expectation`",
		"At: `stage.redaction-check/call.run/act.check/expectation.status`",
		"  - Act: `check`",
		"  - Input `token`: <redacted>",
		"  - Output `status`: `401`",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("markdown output missing %q: %q", want, output.String())
		}
	}
	if strings.Contains(output.String(), "secret-token") {
		t.Fatalf("markdown output leaked redacted value: %q", output.String())
	}
}
