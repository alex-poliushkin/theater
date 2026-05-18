package theater_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alex-poliushkin/theater"
	builtinaction "github.com/alex-poliushkin/theater/builtin/action"
	builtinexpectation "github.com/alex-poliushkin/theater/builtin/expectation"
)

func TestHTTPExpectationFailureReportsSafeDiagnostics(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Request-Id", "req-123")
		writer.Header().Set("X-Correlation-Id", "Bearer header-secret")
		writer.Header().Set("Set-Cookie", "session=header-cookie-secret")
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte(`{"message":"upstream unavailable","detail":"upstream rejected key sk_live_embedded_secret and AKIAEMBEDDEDSECRET","access_token":"body-token-secret","csrf_token":"csrf-secret","key":"sk_live_body_secret","access_key":"AKIAIOSFODNN7EXAMPLE","private_key":"-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----","profile":{"email":"person@example.test","ssn":"123-45-6789","dob":"1990-01-01","name":"Alice Example"}}`))
	}))
	defer server.Close()

	result := runHTTPDiagnosticStage(
		t,
		server.URL+"/reset/path-secret?token=query-secret&visible=yes#debug",
	)

	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	actionPath := "stage.main/call.probe/act.fetch/action"
	actionNode := findNodeReport(t, result.Report, theater.NodeKindAction, actionPath)
	if got, want := actionNode.Status, theater.StatusPassed; got != want {
		t.Fatalf("action status mismatch: got %q want %q", got, want)
	}
	if len(actionNode.Diagnostics) != 0 {
		t.Fatalf("passed action node must not carry failure diagnostics: %#v", actionNode.Diagnostics)
	}

	expectationNode := findNodeReport(
		t,
		result.Report,
		theater.NodeKindExpectation,
		"stage.main/call.probe/act.fetch/expectation.status",
	)
	diagnostic := requireHTTPDiagnostic(t, expectationNode)

	if diagnostic.ActionAddress == nil {
		t.Fatal("http diagnostic must identify the source action address")
	}
	if got, want := diagnostic.ActionAddress.Kind, theater.NodeKindAction; got != want {
		t.Fatalf("action address kind mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.ActionAddress.ActID, "fetch"; got != want {
		t.Fatalf("action address act id mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.FailureKind, theater.HTTPDiagnosticFailureStatus; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Method, http.MethodGet; got != want {
		t.Fatalf("method mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.StatusCode, http.StatusInternalServerError; got != want {
		t.Fatalf("status code mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostic.Status, http.StatusText(http.StatusInternalServerError); got != want {
		t.Fatalf("status text mismatch: got %q want %q", got, want)
	}
	if diagnostic.DurationMs < 0 {
		t.Fatalf("duration must not be negative: %d", diagnostic.DurationMs)
	}

	assertNotContains(t, diagnostic.URL, "path-secret")
	assertNotContains(t, diagnostic.URL, "query-secret")
	assertNotContains(t, diagnostic.URL, "#debug")
	if !strings.Contains(diagnostic.URL, "token=redacted") {
		t.Fatalf("redacted URL must keep query names with redacted values: %q", diagnostic.URL)
	}

	if diagnostic.RequestFingerprint == nil {
		t.Fatal("http diagnostic must include request fingerprint")
	}
	if got, want := diagnostic.RequestFingerprint.Method, http.MethodGet; got != want {
		t.Fatalf("fingerprint method mismatch: got %q want %q", got, want)
	}
	if diagnostic.RequestFingerprint.Host == "" {
		t.Fatalf("fingerprint host is missing: %#v", diagnostic.RequestFingerprint)
	}
	if got, want := diagnostic.RequestFingerprint.PathShape, "/segment/redacted"; got != want {
		t.Fatalf("fingerprint path shape mismatch: got %q want %q", got, want)
	}
	if got := strings.Join(diagnostic.RequestFingerprint.QueryKeys, ","); got != "redacted,visible" {
		t.Fatalf("fingerprint query keys mismatch: %q", got)
	}
	assertNotContains(t, diagnostic.RequestFingerprint.URL, "path-secret")
	assertNotContains(t, diagnostic.RequestFingerprint.URL, "query-secret")

	if diagnostic.ResponseMetadata == nil {
		t.Fatal("http diagnostic must include response metadata")
	}
	if got, want := diagnostic.ResponseMetadata.StatusCode, http.StatusInternalServerError; got != want {
		t.Fatalf("metadata status code mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostic.ResponseMetadata.ContentType, "application/json"; got != want {
		t.Fatalf("metadata content type mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.ResponseMetadata.PreviewKind, "json"; got != want {
		t.Fatalf("metadata preview kind mismatch: got %q want %q", got, want)
	}

	if got := diagnostic.ResponseHeaders["x-request-id"]; len(got) != 1 || got[0] != "req-123" {
		t.Fatalf("request id header mismatch: %#v", diagnostic.ResponseHeaders)
	}
	if _, ok := diagnostic.ResponseHeaders["set-cookie"]; ok {
		t.Fatalf("set-cookie must not be reported: %#v", diagnostic.ResponseHeaders)
	}
	if _, ok := diagnostic.ResponseHeaders["x-correlation-id"]; ok {
		t.Fatalf("credential-like allowlisted header value must not be reported: %#v", diagnostic.ResponseHeaders)
	}

	if diagnostic.ResponsePreview == nil {
		t.Fatal("http diagnostic must include response preview metadata")
	}
	preview := diagnostic.ResponsePreview
	if preview.ContentType != "application/json" {
		t.Fatalf("preview content type mismatch: %#v", preview)
	}
	if !strings.Contains(preview.Text, "upstream unavailable") {
		t.Fatalf("preview must retain safe error context: %#v", preview)
	}
	if !preview.Redacted {
		t.Fatalf("preview must record redaction when sensitive fields are removed: %#v", preview)
	}
	assertNotContains(t, preview.Text, "body-token-secret")
	assertNotContains(t, preview.Text, "csrf-secret")
	assertNotContains(t, preview.Text, "sk_live_body_secret")
	assertNotContains(t, preview.Text, "sk_live_embedded_secret")
	assertNotContains(t, preview.Text, "AKIAIOSFODNN7EXAMPLE")
	assertNotContains(t, preview.Text, "AKIAEMBEDDEDSECRET")
	assertNotContains(t, preview.Text, "BEGIN PRIVATE KEY")
	assertNotContains(t, preview.Text, "person@example.test")
	assertNotContains(t, preview.Text, "123-45-6789")
	assertNotContains(t, preview.Text, "1990-01-01")
	assertNotContains(t, preview.Text, "Alice Example")

	reportJSON, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal run result failed: %v", err)
	}
	assertNotContains(t, string(reportJSON), "path-secret")
	assertNotContains(t, string(reportJSON), "query-secret")
	assertNotContains(t, string(reportJSON), "body-token-secret")
	assertNotContains(t, string(reportJSON), "csrf-secret")
	assertNotContains(t, string(reportJSON), "sk_live_body_secret")
	assertNotContains(t, string(reportJSON), "sk_live_embedded_secret")
	assertNotContains(t, string(reportJSON), "AKIAIOSFODNN7EXAMPLE")
	assertNotContains(t, string(reportJSON), "AKIAEMBEDDEDSECRET")
	assertNotContains(t, string(reportJSON), "BEGIN PRIVATE KEY")
	assertNotContains(t, string(reportJSON), "person@example.test")
	assertNotContains(t, string(reportJSON), "123-45-6789")
	assertNotContains(t, string(reportJSON), "1990-01-01")
	assertNotContains(t, string(reportJSON), "Alice Example")
}

func TestHTTPActionTransportFailureReportsRequestOnlyDiagnostics(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("closed server must not receive requests")
	}))
	url := server.URL + "/reset/path-secret?api_key=query-secret"
	server.Close()

	result := runHTTPStageWithoutExpectations(t, url)
	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	actionNode := findNodeReport(t, result.Report, theater.NodeKindAction, "stage.main/call.probe/act.fetch/action")
	diagnostic := requireHTTPDiagnostic(t, actionNode)
	if got, want := diagnostic.FailureKind, theater.HTTPDiagnosticFailureNetwork; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Method, http.MethodGet; got != want {
		t.Fatalf("method mismatch: got %q want %q", got, want)
	}
	assertNotContains(t, diagnostic.URL, "path-secret")
	assertNotContains(t, diagnostic.URL, "query-secret")
	if diagnostic.RequestFingerprint == nil {
		t.Fatal("transport failure must carry request fingerprint")
	}
	assertNotContains(t, diagnostic.RequestFingerprint.URL, "path-secret")
	assertNotContains(t, diagnostic.RequestFingerprint.URL, "query-secret")
	if diagnostic.StatusCode != 0 || diagnostic.Status != "" {
		t.Fatalf("transport failure must not carry response status: %#v", diagnostic)
	}
	if diagnostic.ResponseMetadata != nil {
		t.Fatalf("transport failure must not carry response metadata: %#v", diagnostic.ResponseMetadata)
	}
	if len(diagnostic.ResponseHeaders) != 0 {
		t.Fatalf("transport failure must not carry response headers: %#v", diagnostic.ResponseHeaders)
	}
	if diagnostic.ResponsePreview != nil {
		t.Fatalf("transport failure must not carry response preview: %#v", diagnostic.ResponsePreview)
	}
}

func TestHTTPActionTransportFailureRedactsOpaqueURL(t *testing.T) {
	t.Parallel()

	result := runHTTPStageWithoutExpectations(t, "mailto:person@example.test?token=query-secret")
	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	actionNode := findNodeReport(t, result.Report, theater.NodeKindAction, "stage.main/call.probe/act.fetch/action")
	diagnostic := requireHTTPDiagnostic(t, actionNode)
	assertNotContains(t, diagnostic.URL, "person@example.test")
	assertNotContains(t, diagnostic.URL, "query-secret")
	if !strings.Contains(diagnostic.URL, "mailto:redacted") {
		t.Fatalf("opaque URL must redact opaque value: %q", diagnostic.URL)
	}
	if !strings.Contains(diagnostic.URL, "token=redacted") {
		t.Fatalf("opaque URL must keep query names with redacted values: %q", diagnostic.URL)
	}
}

func TestHTTPDiagnosticsUseMetadataOnlyPreviewForUnclassifiedText(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/plain")
		writer.WriteHeader(http.StatusBadGateway)
		_, _ = writer.Write([]byte("token=plain-text-secret"))
	}))
	defer server.Close()

	result := runHTTPDiagnosticStage(t, server.URL+"/sample")
	expectationNode := findNodeReport(
		t,
		result.Report,
		theater.NodeKindExpectation,
		"stage.main/call.probe/act.fetch/expectation.status",
	)
	diagnostic := requireHTTPDiagnostic(t, expectationNode)
	if diagnostic.ResponsePreview == nil {
		t.Fatal("plain text response must still carry preview metadata")
	}
	preview := diagnostic.ResponsePreview
	if preview.Text != "" {
		t.Fatalf("unclassified text preview must not expose body text: %#v", preview)
	}
	if got, want := preview.OmittedReason, "unclassified_text"; got != want {
		t.Fatalf("omitted reason mismatch: got %q want %q", got, want)
	}
	if diagnostic.ResponseMetadata == nil || diagnostic.ResponseMetadata.PreviewOmittedReason != "unclassified_text" {
		t.Fatalf("response metadata must classify omitted preview: %#v", diagnostic.ResponseMetadata)
	}
	assertNotContains(t, fmt.Sprintf("%#v", diagnostic), "plain-text-secret")
}

func TestHTTPDiagnosticsRedactFormResponsePreview(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/x-www-form-urlencoded")
		writer.WriteHeader(http.StatusBadGateway)
		_, _ = writer.Write([]byte("message=retry&access_token=form-secret&note=Bearer+form-secret&detail=upstream+rejected+sk_live_form_embedded+and+AKIAFORMEMBEDDED&ssn=123-45-6789&name=Alice"))
	}))
	defer server.Close()

	result := runHTTPDiagnosticStage(t, server.URL+"/sample")
	expectationNode := findNodeReport(
		t,
		result.Report,
		theater.NodeKindExpectation,
		"stage.main/call.probe/act.fetch/expectation.status",
	)
	diagnostic := requireHTTPDiagnostic(t, expectationNode)
	if diagnostic.ResponsePreview == nil {
		t.Fatal("form response must include response preview metadata")
	}
	preview := diagnostic.ResponsePreview
	if got, want := preview.Kind, "form"; got != want {
		t.Fatalf("preview kind mismatch: got %q want %q", got, want)
	}
	if !strings.Contains(preview.Text, "message=retry") {
		t.Fatalf("form preview must retain safe context: %#v", preview)
	}
	if !preview.Redacted {
		t.Fatalf("form preview must record redaction: %#v", preview)
	}
	assertNotContains(t, preview.Text, "form-secret")
	assertNotContains(t, preview.Text, "sk_live_form_embedded")
	assertNotContains(t, preview.Text, "AKIAFORMEMBEDDED")
	assertNotContains(t, preview.Text, "123-45-6789")
	assertNotContains(t, preview.Text, "Alice")
}

func runHTTPDiagnosticStage(t *testing.T, url string) theater.RunResult {
	t.Helper()

	return runHTTPStage(t, url, []theater.ExpectationSpec{
		{
			ID:      "status",
			Subject: theater.SubjectSpec{Field: "status_code"},
			Assert: theater.AssertSpec{
				Ref: builtinexpectation.EqualRef,
				Args: map[string]theater.BindingSpec{
					"expected": {Kind: theater.BindingKindLiteral, Value: http.StatusOK},
				},
			},
		},
	})
}

func runHTTPStageWithoutExpectations(t *testing.T, url string) theater.RunResult {
	t.Helper()

	return runHTTPStage(t, url, nil)
}

func runHTTPStage(t *testing.T, url string, expectations []theater.ExpectationSpec) theater.RunResult {
	t.Helper()

	catalog, matchers, err := newBuiltins()
	if err != nil {
		t.Fatalf("new builtins failed: %v", err)
	}

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "http-probe",
				Acts: []theater.ActSpec{
					{
						ID: "fetch",
						Action: theater.ActionSpec{
							Use: builtinaction.HTTPRef,
							With: map[string]theater.BindingSpec{
								"url": {Kind: theater.BindingKindLiteral, Value: url},
							},
						},
						Expectations: expectations,
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "probe", ScenarioID: "http-probe"},
		},
	}

	result, err := runStage(context.Background(), spec, catalog, matchers)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	return result
}

func requireHTTPDiagnostic(t *testing.T, node theater.NodeReport) theater.HTTPDiagnostic {
	t.Helper()

	for i := range node.Diagnostics {
		diagnostic := node.Diagnostics[i]
		if diagnostic.Kind == theater.NodeDiagnosticKindHTTP && diagnostic.HTTP != nil {
			return *diagnostic.HTTP
		}
	}

	t.Fatalf("http diagnostic not found on node %s: %#v", node.Path, node.Diagnostics)
	return theater.HTTPDiagnostic{}
}

func assertNotContains(t *testing.T, value, forbidden string) {
	t.Helper()

	if strings.Contains(value, forbidden) {
		t.Fatalf("%q must not contain %q", value, forbidden)
	}
}
