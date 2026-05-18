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
		"failure: status_mismatch",
		"request: GET https://api.example.test/redacted?token=redacted",
		"request.host: api.example.test",
		"request.path_shape: /redacted",
		"request.query_keys: redacted",
		"response: 502 Bad Gateway",
		"response.content_type: application/json",
		"response.content_length_bytes: 96",
		"response.preview_kind: json",
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
		"- HTTP failure: `status_mismatch`",
		"- HTTP request: `GET https://api.example.test/redacted?token=redacted`",
		"- HTTP host: `api.example.test`",
		"- HTTP path shape: `/redacted`",
		"- HTTP query keys: `redacted`",
		"- HTTP response: `502 Bad Gateway`",
		"- HTTP content type: `application/json`",
		"- HTTP content length: `96`",
		"- HTTP preview kind: `json`",
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
		"- HTTP failure: `network_error`",
		"- HTTP request: `GET https://api.example.test/redacted?token=redacted`",
		"- HTTP host: `api.example.test`",
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

func TestRunTextViewRendersPreflightDiagnosticsWithoutValues(t *testing.T) {
	t.Parallel()

	output := newRunTextView("run.json", preflightDiagnosticFailureDocument()).String()

	for _, want := range []string{
		"preflight:",
		"guard: recipient-test-domain",
		"input: stage.main/call.send-prod/binding.recipient_email",
		"assert: expectation.matches",
		"reason: matcher_mismatch",
		"override: allow_non_test_recipient used=false",
		"source: flow.thtr:3:3",
		"binding_source: flow.thtr:9:31",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("text output missing %q: %q", want, output)
		}
	}
	if strings.Contains(output, "person@example.com") {
		t.Fatalf("text output leaked rejected input value: %q", output)
	}
}

func TestRunTextViewSanitizesPreflightDiagnosticFields(t *testing.T) {
	t.Parallel()

	document := preflightDiagnosticFailureDocument()
	diagnostic := document.Report.Nodes[0].Diagnostics[0].Preflight
	diagnostic.GuardID = "recipient\nnext"
	diagnostic.InputPath = strings.Repeat("input", preflightDiagnosticTextLimit)
	output := newRunTextView("run.json", document).String()

	if strings.Contains(output, "recipient\nnext") {
		t.Fatalf("text output must not preserve control characters: %q", output)
	}
	if !strings.Contains(output, "guard: recipient next") {
		t.Fatalf("text output missing sanitized guard id: %q", output)
	}
	if !strings.Contains(output, renderPreviewTruncatedSuffix) {
		t.Fatalf("text output missing preflight truncation marker: %q", output)
	}
}

func TestReportMarkdownRendererRendersPreflightDiagnosticsWithoutValues(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	if err := newReportMarkdownRenderer().Write(&output, "run.json", preflightDiagnosticFailureDocument()); err != nil {
		t.Fatalf("write markdown failed: %v", err)
	}

	for _, want := range []string{
		"- Preflight guard: `recipient-test-domain`",
		"- Preflight input: `stage.main/call.send-prod/binding.recipient_email`",
		"- Preflight assert: `expectation.matches`",
		"- Preflight reason: `matcher_mismatch`",
		"- Preflight override: `allow_non_test_recipient` used=`false`",
		"- Preflight source: `flow.thtr:3:3`",
		"- Preflight binding source: `flow.thtr:9:31`",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("markdown output missing %q: %q", want, output.String())
		}
	}
	if strings.Contains(output.String(), "person@example.com") {
		t.Fatalf("markdown output leaked rejected input value: %q", output.String())
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
								FailureKind: theater.HTTPDiagnosticFailureStatus,
								Method:      "GET",
								URL:         "https://api.example.test/redacted?token=redacted",
								StatusCode:  502,
								Status:      "Bad Gateway",
								DurationMs:  15,
								RequestFingerprint: &theater.HTTPRequestFingerprint{
									Method:     "GET",
									URL:        "https://api.example.test/redacted?token=redacted",
									Host:       "api.example.test",
									PathShape:  "/redacted",
									QueryKeys:  []string{"redacted"},
									DurationMs: 15,
								},
								ResponseMetadata: &theater.HTTPResponseMetadata{
									StatusCode:         502,
									Status:             "Bad Gateway",
									ContentType:        "application/json",
									ContentLengthBytes: 96,
									PreviewKind:        "json",
								},
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

func preflightDiagnosticFailureDocument() theater.RunDocument {
	scenarioPath := "stage.main/call.send-prod"
	failure := &theater.Failure{
		Kind:    theater.FailureKindSetup,
		Phase:   theater.PhaseRun,
		At:      "stage.main/scenario.send-email/preflight.recipient-test-domain",
		Summary: "preflight rejected scenario input",
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
					Kind:           theater.NodeKindScenario,
					StageID:        "main",
					Path:           scenarioPath,
					ScenarioID:     "send-email",
					ScenarioCallID: "send-prod",
					ScenarioPath:   scenarioPath,
					Attempt:        1,
					ScenarioSeq:    1,
					Status:         theater.StatusFailed,
					Failure:        failure,
					Diagnostics: []theater.NodeDiagnostic{
						{
							Kind: theater.NodeDiagnosticKindPreflight,
							Preflight: &theater.PreflightDiagnostic{
								GuardID:         "recipient-test-domain",
								InputRef:        "recipient_email",
								InputPath:       scenarioPath + "/binding.recipient_email",
								AssertRef:       "expectation.matches",
								ReasonCode:      "matcher_mismatch",
								OverrideRef:     "allow_non_test_recipient",
								OverridePresent: true,
								SourceSpan: &theater.SourceRef{
									File:   "flow.thtr",
									Line:   3,
									Column: 3,
								},
								BindingSourceSpan: &theater.SourceRef{
									File:   "flow.thtr",
									Line:   9,
									Column: 31,
								},
							},
						},
					},
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
								FailureKind: theater.HTTPDiagnosticFailureNetwork,
								Method:      "GET",
								URL:         "https://api.example.test/redacted?token=redacted",
								DurationMs:  15,
								RequestFingerprint: &theater.HTTPRequestFingerprint{
									Method:     "GET",
									URL:        "https://api.example.test/redacted?token=redacted",
									Host:       "api.example.test",
									PathShape:  "/redacted",
									QueryKeys:  []string{"redacted"},
									DurationMs: 15,
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
