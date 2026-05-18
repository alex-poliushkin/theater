package theatercli

import (
	"strings"
	"testing"

	"github.com/alex-poliushkin/theater"
)

func TestRunTextViewRendersHTTPDiagnostics(t *testing.T) {
	t.Parallel()

	output := newRunTextView("run.json", httpDiagnosticFailureDocument()).String()

	for _, want := range []string{
		"http:",
		"request: GET https://api.example.test/redacted?token=redacted",
		"response: 502 Bad Gateway",
		"header.x-request-id: req-123",
		`body: {"message":"retry later","token":"[redacted]"} (redacted)`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("text output missing %q: %q", want, output)
		}
	}
	if strings.Contains(output, "credential-secret") {
		t.Fatalf("text output leaked secret: %q", output)
	}
}

func TestReportMarkdownRendererRendersHTTPDiagnostics(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	if err := newReportMarkdownRenderer().Write(&output, "run.json", httpDiagnosticFailureDocument()); err != nil {
		t.Fatalf("write markdown failed: %v", err)
	}

	for _, want := range []string{
		"- HTTP request: `GET https://api.example.test/redacted?token=redacted`",
		"- HTTP response: `502 Bad Gateway`",
		"- HTTP header `x-request-id`: `req-123`",
		"- HTTP body: `{\"message\":\"retry later\",\"token\":\"[redacted]\"}` (redacted)",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("markdown output missing %q: %q", want, output.String())
		}
	}
	if strings.Contains(output.String(), "credential-secret") {
		t.Fatalf("markdown output leaked secret: %q", output.String())
	}
}

func TestReportMarkdownRendererRendersHTTPTransportDiagnostics(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	if err := newReportMarkdownRenderer().Write(&output, "run.json", httpTransportDiagnosticFailureDocument()); err != nil {
		t.Fatalf("write markdown failed: %v", err)
	}

	for _, want := range []string{
		"- Action `fetch` failed",
		"- HTTP request: `GET https://api.example.test/redacted?token=redacted`",
		"- HTTP duration: `15ms`",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("markdown output missing %q: %q", want, output.String())
		}
	}
	for _, forbidden := range []string{
		"path-secret",
		"query-secret",
		"HTTP response:",
		"HTTP body:",
	} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("markdown output contains forbidden text %q: %q", forbidden, output.String())
		}
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

func httpTransportDiagnosticFailureDocument() theater.RunDocument {
	scenarioPath := "stage.main/call.probe"
	actionPath := scenarioPath + "/act.fetch/action"
	actPath := scenarioPath + "/act.fetch"
	failure := &theater.Failure{
		Kind:    theater.FailureKindAction,
		Phase:   theater.PhaseRun,
		At:      actionPath,
		Summary: "request failed",
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
					Kind:           theater.NodeKindAct,
					StageID:        "main",
					Path:           actPath,
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
						Kind:             theater.NodeKindAct,
					},
				},
				{
					Kind:           theater.NodeKindAction,
					StageID:        "main",
					Path:           actionPath,
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
						Kind:             theater.NodeKindAction,
					},
					Diagnostics: []theater.NodeDiagnostic{
						{
							Kind: theater.NodeDiagnosticKindHTTP,
							HTTP: &theater.HTTPDiagnostic{
								Method:     "GET",
								URL:        "https://api.example.test/redacted?token=redacted",
								DurationMs: 15,
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
