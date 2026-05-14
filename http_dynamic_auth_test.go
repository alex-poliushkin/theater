package theater_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alex-poliushkin/theater"
	builtinaction "github.com/alex-poliushkin/theater/builtin/action"
	builtinexpectation "github.com/alex-poliushkin/theater/builtin/expectation"
	"github.com/alex-poliushkin/theater/thtr"
)

func TestValidateStageSpecAllowsDynamicBearerAuthBinding(t *testing.T) {
	t.Parallel()

	spec := dynamicBearerStageSpec("https://example.test/customer", "issued-token")

	if diagnostics := validateStage(spec, nil, matcherCatalog(t, builtinexpectation.Descriptors()...)); len(diagnostics) != 0 {
		t.Fatalf("dynamic bearer auth binding must validate cleanly, got %#v", diagnostics)
	}
}

func TestValidateStageSpecRejectsInvalidDynamicBearerAuthBindings(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		mutate   func(*theater.StageSpec)
		wantCode string
		wantPath string
	}{
		{
			name: "bearer declares token and token slot",
			mutate: func(spec *theater.StageSpec) {
				spec.HTTP.Auth["mobile_api"] = theater.HTTPAuthSpec{Attach: []theater.HTTPAuthAttachmentSpec{{
					Bearer: &theater.HTTPBearerAuthSpec{Token: "static", TokenSlot: "access_token"},
				}}}
			},
			wantCode: "invalid_http_auth_bearer_token_source",
			wantPath: "stage.main/http/auth.mobile_api/attach[0]",
		},
		{
			name: "bearer declares neither token nor token slot",
			mutate: func(spec *theater.StageSpec) {
				spec.HTTP.Auth["mobile_api"] = theater.HTTPAuthSpec{Attach: []theater.HTTPAuthAttachmentSpec{{
					Bearer: &theater.HTTPBearerAuthSpec{},
				}}}
			},
			wantCode: "invalid_http_auth_bearer_token_source",
			wantPath: "stage.main/http/auth.mobile_api/attach[0]",
		},
		{
			name: "auth binding targets unknown auth",
			mutate: func(spec *theater.StageSpec) {
				spec.Scenarios[0].AuthBindings["missing"] = spec.Scenarios[0].AuthBindings["mobile_api"]
				delete(spec.Scenarios[0].AuthBindings, "mobile_api")
			},
			wantCode: "unknown_http_auth_binding_ref",
			wantPath: "stage.main/scenario.mobile~1dashboard-ready/auth_bindings.missing",
		},
		{
			name: "auth binding declares no slots",
			mutate: func(spec *theater.StageSpec) {
				spec.Scenarios[0].AuthBindings["mobile_api"] = theater.HTTPAuthBindingSpec{}
			},
			wantCode: "missing_http_auth_binding_slots",
			wantPath: "stage.main/scenario.mobile~1dashboard-ready/auth_bindings.mobile_api",
		},
		{
			name: "auth binding targets undeclared slot",
			mutate: func(spec *theater.StageSpec) {
				spec.Scenarios[0].AuthBindings["mobile_api"] = theater.HTTPAuthBindingSpec{
					Slots: map[string]theater.BindingSpec{
						"refresh_token": {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "access_token"}},
					},
				}
			},
			wantCode: "unknown_http_auth_binding_slot",
			wantPath: "stage.main/scenario.mobile~1dashboard-ready/auth_bindings.mobile_api/slot.refresh_token",
		},
		{
			name: "auth binding reads unknown scenario input",
			mutate: func(spec *theater.StageSpec) {
				spec.Scenarios[0].AuthBindings["mobile_api"] = theater.HTTPAuthBindingSpec{
					Slots: map[string]theater.BindingSpec{
						"access_token": {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "missing_token"}},
					},
				}
			},
			wantCode: "unresolved_binding_ref",
			wantPath: "stage.main/scenario.mobile~1dashboard-ready/auth_bindings.mobile_api/slot.access_token",
		},
		{
			name: "auth binding slot has incompatible static type",
			mutate: func(spec *theater.StageSpec) {
				spec.Scenarios[0].AuthBindings["mobile_api"] = theater.HTTPAuthBindingSpec{
					Slots: map[string]theater.BindingSpec{
						"access_token": {Kind: theater.BindingKindLiteral, Value: 42},
					},
				}
			},
			wantCode: "incompatible_http_auth_binding_slot",
			wantPath: "stage.main/scenario.mobile~1dashboard-ready/auth_bindings.mobile_api/slot.access_token",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			spec := dynamicBearerStageSpec("https://example.test/customer", "issued-token")
			tc.mutate(&spec)

			diagnostics := validateStage(spec, nil, matcherCatalog(t, builtinexpectation.Descriptors()...))
			diagnostic := findDynamicAuthDiagnosticByCode(diagnostics, tc.wantCode)
			if diagnostic == nil {
				t.Fatalf("expected %s diagnostic, got %#v", tc.wantCode, diagnostics)
			}
			if got := diagnostic.Path; got != tc.wantPath {
				t.Fatalf("diagnostic path mismatch: got %q want %q", got, tc.wantPath)
			}
		})
	}
}

func TestRunBindsDynamicBearerAuthFromScenarioInput(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.Header.Get("Authorization"), "Bearer issued-token"; got != want {
			t.Fatalf("authorization header mismatch: got %q want %q", got, want)
		}

		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	catalog := theater.NewCatalog()
	if err := builtinaction.RegisterRuntime(catalog); err != nil {
		t.Fatalf("register actions failed: %v", err)
	}

	result, err := runStage(
		context.Background(),
		dynamicBearerStageSpec(server.URL+"/customer", "issued-token"),
		catalog,
		matcherCatalog(t, builtinexpectation.Descriptors()...),
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q failure=%#v cause=%v", got, want, result.Report.Failure, result.Report.Failure.Cause)
	}
}

func TestRunRejectsEmptyDynamicBearerTokenBeforeHTTPRequest(t *testing.T) {
	t.Parallel()

	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		atomic.AddInt32(&requests, 1)
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	catalog := theater.NewCatalog()
	if err := builtinaction.RegisterRuntime(catalog); err != nil {
		t.Fatalf("register actions failed: %v", err)
	}

	result, err := runStage(
		context.Background(),
		dynamicBearerStageSpec(server.URL+"/customer", ""),
		catalog,
		matcherCatalog(t, builtinexpectation.Descriptors()...),
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q failure=%#v", got, want, result.Report.Failure)
	}
	if got := atomic.LoadInt32(&requests); got != 0 {
		t.Fatalf("dynamic bearer auth must fail before HTTP request, got %d requests", got)
	}
}

func TestRunRejectsMissingDynamicBearerTokenBeforeHTTPRequest(t *testing.T) {
	t.Parallel()

	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		atomic.AddInt32(&requests, 1)
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	catalog := theater.NewCatalog()
	if err := builtinaction.RegisterRuntime(catalog); err != nil {
		t.Fatalf("register actions failed: %v", err)
	}

	spec := dynamicBearerStageSpec(server.URL+"/customer", "issued-token")
	spec.Scenarios[0].AuthBindings = nil

	result, err := runStage(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, builtinexpectation.Descriptors()...),
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q failure=%#v", got, want, result.Report.Failure)
	}
	if got := atomic.LoadInt32(&requests); got != 0 {
		t.Fatalf("dynamic bearer auth must fail before HTTP request, got %d requests", got)
	}
}

func TestRunRejectsTypedMissingDynamicBearerTokenBeforeHTTPRequest(t *testing.T) {
	t.Parallel()

	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		atomic.AddInt32(&requests, 1)
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	catalog := theater.NewCatalog()
	if err := builtinaction.RegisterRuntime(catalog); err != nil {
		t.Fatalf("register actions failed: %v", err)
	}

	spec := dynamicBearerStageSpec(server.URL+"/customer", "issued-token")
	spec.Scenarios[0].Inputs["access_token"] = theater.ValueContract{Kind: theater.ValueKindString}
	delete(spec.ScenarioCalls[0].Bindings, "access_token")

	result, err := runStage(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, builtinexpectation.Descriptors()...),
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q failure=%#v", got, want, result.Report.Failure)
	}
	if got := atomic.LoadInt32(&requests); got != 0 {
		t.Fatalf("dynamic bearer auth must fail before HTTP request, got %d requests", got)
	}
}

func TestRunRejectsNonStringDynamicBearerTokenBeforeHTTPRequest(t *testing.T) {
	t.Parallel()

	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		atomic.AddInt32(&requests, 1)
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	catalog := theater.NewCatalog()
	if err := builtinaction.RegisterRuntime(catalog); err != nil {
		t.Fatalf("register actions failed: %v", err)
	}

	spec := dynamicBearerStageSpec(server.URL+"/customer", 42)
	spec.Scenarios[0].Inputs["access_token"] = theater.ValueContract{Kind: theater.ValueKindAny, Required: true}

	result, err := runStage(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, builtinexpectation.Descriptors()...),
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q failure=%#v", got, want, result.Report.Failure)
	}
	if got := atomic.LoadInt32(&requests); got != 0 {
		t.Fatalf("dynamic bearer auth must fail before HTTP request, got %d requests", got)
	}
	if result.Report.Failure == nil || result.Report.Failure.Cause == nil {
		t.Fatal("dynamic bearer auth failure must include a cause")
	}
	if cause := result.Report.Failure.Cause.Error(); strings.Contains(cause, "42") {
		t.Fatalf("dynamic bearer auth failure leaked token value: %q", cause)
	}
}

func TestRunRedactsDynamicBearerAuthSourceInDebugSnapshots(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.Header.Get("Authorization"), "Bearer issued-token"; got != want {
			t.Fatalf("authorization header mismatch: got %q want %q", got, want)
		}

		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	spec, err := thtr.Parse([]byte(fmt.Sprintf(`stage mobile-dashboard
http
  auth mobile_api = http.auth(
    attach: list [
      object { bearer: object { token_slot: "access_token" } },
    ],
  )

scenario mobile/dashboard-ready(access_token: string!, endpoint: string!)
  bind auth mobile_api
    access_token: $access_token
  act wait-customer
    do action.http
      method: "GET"
      url: $endpoint
      session: "none"
      auth: "mobile_api"

call run-dashboard = mobile/dashboard-ready(access_token: "issued-token", endpoint: %q)
`, server.URL+"/customer")), nil)
	if err != nil {
		t.Fatalf("parse thtr failed: %v", err)
	}

	catalog := theater.NewCatalog()
	if err := builtinaction.RegisterRuntime(catalog); err != nil {
		t.Fatalf("register actions failed: %v", err)
	}

	dumpPath := t.TempDir() + "/debug.ndjson"
	result, err := runStageWithOptions(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, builtinexpectation.Descriptors()...),
		theater.RunOptions{
			Debug: &theater.DebugOptions{
				Mode:        theater.DebugModeDump,
				DumpPath:    dumpPath,
				Breakpoints: []string{"path=**,kind=scenario_call,action=snapshot-continue"},
			},
		},
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}
	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q failure=%#v", got, want, result.Report.Failure)
	}

	dump, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read debug dump failed: %v", err)
	}
	if bytes.Contains(dump, []byte("issued-token")) {
		t.Fatalf("debug dump leaked dynamic bearer source value:\n%s", dump)
	}
	if !bytes.Contains(dump, []byte("[redacted]")) {
		t.Fatalf("debug dump must redact dynamic bearer source value:\n%s", dump)
	}
}

func dynamicBearerStageSpec(url string, token any) theater.StageSpec {
	return theater.StageSpec{
		ID: "main",
		HTTP: &theater.HTTPSpec{
			Auth: map[string]theater.HTTPAuthSpec{
				"mobile_api": {Attach: []theater.HTTPAuthAttachmentSpec{{
					Bearer: &theater.HTTPBearerAuthSpec{TokenSlot: "access_token"},
				}}},
			},
		},
		Scenarios: []theater.ScenarioSpec{{
			ID: "mobile/dashboard-ready",
			Inputs: map[string]theater.ValueContract{
				"access_token": {
					Kind:        theater.ValueKindString,
					Required:    true,
					Sensitivity: theater.SensitivitySecret,
					Capture:     theater.CaptureOmit,
				},
			},
			AuthBindings: map[string]theater.HTTPAuthBindingSpec{
				"mobile_api": {
					Slots: map[string]theater.BindingSpec{
						"access_token": {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "access_token"}},
					},
				},
			},
			Acts: []theater.ActSpec{{
				ID: "wait-customer",
				Action: theater.ActionSpec{
					Use: builtinaction.HTTPRef,
					With: map[string]theater.BindingSpec{
						"method":  {Kind: theater.BindingKindLiteral, Value: http.MethodGet},
						"url":     {Kind: theater.BindingKindLiteral, Value: url},
						"session": {Kind: theater.BindingKindLiteral, Value: theater.HTTPSessionNone},
						"auth":    {Kind: theater.BindingKindLiteral, Value: "mobile_api"},
					},
				},
				Expectations: []theater.ExpectationSpec{{
					ID:      "status",
					Subject: theater.SubjectSpec{Field: "status_code"},
					Assert: theater.AssertSpec{
						Ref: builtinexpectation.BetweenRef,
						Args: map[string]theater.BindingSpec{
							"min": {Kind: theater.BindingKindLiteral, Value: 200},
							"max": {Kind: theater.BindingKindLiteral, Value: 299},
						},
					},
				}},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{
			ID:         "mobile-dashboard-ready",
			ScenarioID: "mobile/dashboard-ready",
			Bindings: map[string]theater.BindingSpec{
				"access_token": {Kind: theater.BindingKindLiteral, Value: token},
			},
		}},
	}
}

func findDynamicAuthDiagnosticByCode(diagnostics []theater.Diagnostic, code string) *theater.Diagnostic {
	for i := range diagnostics {
		if diagnostics[i].Code == code {
			return &diagnostics[i]
		}
	}

	return nil
}
