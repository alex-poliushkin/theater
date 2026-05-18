package builtinhttp

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/httpclient"
)

func TestRequestFromArgsBuildsHTTPClientRequest(t *testing.T) {
	t.Parallel()

	request, err := RequestFromArgs(theater.Args{
		"url":     "https://example.test/resource",
		"body":    `{"ok":true}`,
		"timeout": "2s",
		"session": "auth",
		"headers": map[string]any{
			"X-Test": []any{"one", "two"},
		},
	})
	if err != nil {
		t.Fatalf("request from args failed: %v", err)
	}

	if got, want := request.Method, http.MethodGet; got != want {
		t.Fatalf("method mismatch: got %q want %q", got, want)
	}

	if got, want := request.URL, "https://example.test/resource"; got != want {
		t.Fatalf("url mismatch: got %q want %q", got, want)
	}

	if got, want := string(request.Body), `{"ok":true}`; got != want {
		t.Fatalf("body mismatch: got %q want %q", got, want)
	}

	if got, want := request.Timeout, 2*time.Second; got != want {
		t.Fatalf("timeout mismatch: got %s want %s", got, want)
	}

	if got, want := request.Session, "auth"; got != want {
		t.Fatalf("session mismatch: got %q want %q", got, want)
	}

	if got, want := request.SessionMode, httpclient.SessionModeNamed; got != want {
		t.Fatalf("session mode mismatch: got %d want %d", got, want)
	}

	if got, want := request.Headers["X-Test"], []string{"one", "two"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("headers mismatch: got %#v want %#v", got, want)
	}
}

func TestOutputsCloneHeaders(t *testing.T) {
	t.Parallel()

	response := Response{
		StatusCode: http.StatusCreated,
		Status:     "201 Created",
		Body:       []byte(`{"ok":true}`),
		Headers: http.Header{
			"X-Test": []string{"one", "two"},
		},
	}

	values := Outputs(response)
	gotHeaders, ok := values["headers"].(map[string]any)
	if !ok {
		t.Fatalf("headers output type mismatch: got %T", values["headers"])
	}

	got, ok := gotHeaders["X-Test"].([]any)
	if !ok {
		t.Fatalf("header value type mismatch: got %T", gotHeaders["X-Test"])
	}

	response.Headers["X-Test"][0] = "changed"

	if want := []any{"one", "two"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("header values should be cloned: got %#v want %#v", got, want)
	}
}

func TestRequestFromArgsAcceptsStringHeaderLists(t *testing.T) {
	t.Parallel()

	request, err := RequestFromArgs(theater.Args{
		"url": "https://example.test/resource",
		"headers": map[string]any{
			"X-Test": []string{"one", "two"},
		},
	})
	if err != nil {
		t.Fatalf("request from args failed: %v", err)
	}

	if got, want := request.Headers["X-Test"], []string{"one", "two"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("headers mismatch: got %#v want %#v", got, want)
	}
}

func TestRequestFromArgsParsesSessionNoneAndAuth(t *testing.T) {
	t.Parallel()

	request, err := RequestFromArgs(theater.Args{
		"url":     "https://example.test/resource",
		"session": theater.HTTPSessionNone,
		"auth":    "ci_api",
	})
	if err != nil {
		t.Fatalf("request from args failed: %v", err)
	}

	if got, want := request.SessionMode, httpclient.SessionModeNone; got != want {
		t.Fatalf("session mode mismatch: got %d want %d", got, want)
	}
	if got, want := request.Auth, "ci_api"; got != want {
		t.Fatalf("auth mismatch: got %q want %q", got, want)
	}
}

func TestRequestFromArgsParsesIdentityAuthNoneAndForm(t *testing.T) {
	t.Parallel()

	request, err := RequestFromArgs(theater.Args{
		"url":      "https://example.test/resource",
		"identity": "user",
		"auth":     theater.HTTPAuthNone,
		"form": map[string]any{
			"username": "demo",
		},
	})
	if err != nil {
		t.Fatalf("request from args failed: %v", err)
	}

	if got, want := request.Identity, "user"; got != want {
		t.Fatalf("identity mismatch: got %q want %q", got, want)
	}
	if got, want := request.AuthMode, authModeNone; got != want {
		t.Fatalf("auth mode mismatch: got %d want %d", got, want)
	}
	if got, want := request.Form["username"], "demo"; got != want {
		t.Fatalf("form value mismatch: got %q want %q", got, want)
	}
}

func TestRequestFromArgsParsesJSON(t *testing.T) {
	t.Parallel()

	request, err := RequestFromArgs(theater.Args{
		"url": "https://example.test/resource",
		"json": map[string]any{
			"email": "demo@example.test",
		},
	})
	if err != nil {
		t.Fatalf("request from args failed: %v", err)
	}

	if got, want := request.JSON.(map[string]any)["email"], "demo@example.test"; got != want {
		t.Fatalf("json value mismatch: got %v want %v", got, want)
	}
}

func TestRequestFromArgsRejectsJSONConflicts(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args theater.Args
		want string
	}{
		{
			name: "body",
			args: theater.Args{
				"url":  "https://example.test/resource",
				"body": "raw",
				"json": map[string]any{"ok": true},
			},
			want: "json is incompatible with body",
		},
		{
			name: "form",
			args: theater.Args{
				"url":  "https://example.test/resource",
				"form": map[string]any{"ok": true},
				"json": map[string]any{"ok": true},
			},
			want: "json is incompatible with form",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := RequestFromArgs(tc.args)
			if err == nil {
				t.Fatal("expected request error")
			}
			if got := err.Error(); got != tc.want {
				t.Fatalf("error mismatch: got %q want %q", got, tc.want)
			}
		})
	}
}

func TestDoReusesManagedClientWithinRuntimeScope(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/bootstrap":
			http.SetCookie(writer, &http.Cookie{Name: "sid", Value: "issued"})
			writer.WriteHeader(http.StatusNoContent)
		case "/protected":
			cookie, err := request.Cookie("sid")
			if err != nil || cookie.Value != "issued" {
				writer.WriteHeader(http.StatusUnauthorized)
				return
			}

			writer.WriteHeader(http.StatusOK)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	resources := theater.NewResourceScope()
	httpSpec := &theater.HTTPSpec{
		Sessions: map[string]theater.HTTPSessionSpec{
			"auth": {},
		},
	}

	if _, err := Do(context.Background(), resources, httpSpec, Request{
		URL:         server.URL + "/bootstrap",
		Session:     "auth",
		SessionMode: httpclient.SessionModeNamed,
	}); err != nil {
		t.Fatalf("bootstrap request failed: %v", err)
	}

	response, err := Do(context.Background(), resources, httpSpec, Request{
		URL:         server.URL + "/protected",
		Session:     "auth",
		SessionMode: httpclient.SessionModeNamed,
	})
	if err != nil {
		t.Fatalf("protected request failed: %v", err)
	}

	if got, want := response.StatusCode, http.StatusOK; got != want {
		t.Fatalf("status code mismatch: got %d want %d", got, want)
	}
}

func TestDoFallsBackToStatelessExecutionWithoutRuntimeResources(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.Header.Get("Cookie"), "sid=manual"; got != want {
			t.Fatalf("cookie header mismatch: got %q want %q", got, want)
		}

		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	response, err := Do(context.Background(), nil, nil, Request{
		URL: server.URL,
		Headers: map[string][]string{
			"Cookie": {"sid=manual"},
		},
	})
	if err != nil {
		t.Fatalf("stateless request failed: %v", err)
	}

	if got, want := response.StatusCode, http.StatusOK; got != want {
		t.Fatalf("status code mismatch: got %d want %d", got, want)
	}
}

func TestDoRejectsConfiguredSessionWithoutRuntimeScope(t *testing.T) {
	t.Parallel()

	_, err := Do(context.Background(), nil, &theater.HTTPSpec{
		Sessions: map[string]theater.HTTPSessionSpec{
			"auth": {},
		},
	}, Request{
		URL:         "http://example.invalid",
		Session:     "auth",
		SessionMode: httpclient.SessionModeNamed,
	})
	if err == nil {
		t.Fatal("expected stateless session error")
	}

	if got, want := err.Error(), "session requires scenario-local HTTP client"; got != want {
		t.Fatalf("error mismatch: got %q want %q", got, want)
	}
}

func TestDoRejectsUndeclaredNamedSession(t *testing.T) {
	t.Parallel()

	_, err := Do(context.Background(), theater.NewResourceScope(), nil, Request{
		URL:         "https://example.test",
		Session:     "auth",
		SessionMode: httpclient.SessionModeNamed,
	})
	if err == nil {
		t.Fatal("expected undeclared session error")
	}
	if got, want := err.Error(), `session "auth" is not declared`; got != want {
		t.Fatalf("error mismatch: got %q want %q", got, want)
	}
}

func TestDoSendsJSONBodyAndSetsContentType(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.Header.Get("Content-Type"), "application/json"; got != want {
			t.Fatalf("content type mismatch: got %q want %q", got, want)
		}

		defer request.Body.Close()

		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body failed: %v", err)
		}

		if got, want := body["email"], "demo@example.test"; got != want {
			t.Fatalf("email mismatch: got %v want %v", got, want)
		}

		writer.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	response, err := Do(context.Background(), nil, nil, Request{
		URL:  server.URL,
		JSON: map[string]any{"email": "demo@example.test"},
	})
	if err != nil {
		t.Fatalf("json request failed: %v", err)
	}

	if got, want := response.StatusCode, http.StatusAccepted; got != want {
		t.Fatalf("status code mismatch: got %d want %d", got, want)
	}
}

func TestDoRejectsConflictingJSONContentType(t *testing.T) {
	t.Parallel()

	_, err := Do(context.Background(), nil, nil, Request{
		URL:  "https://example.test",
		JSON: map[string]any{"email": "demo@example.test"},
		Headers: map[string][]string{
			"Content-Type": {"text/plain"},
		},
	})
	if err == nil {
		t.Fatal("expected content type conflict")
	}

	if got, want := err.Error(), `json request body conflicts with Content-Type "text/plain"`; got != want {
		t.Fatalf("error mismatch: got %q want %q", got, want)
	}
}

func TestDoAllowsCookieHeaderWhenSessionIsDisabled(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.Header.Get("Cookie"), "sid=manual"; got != want {
			t.Fatalf("cookie header mismatch: got %q want %q", got, want)
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	response, err := Do(context.Background(), theater.NewResourceScope(), nil, Request{
		URL:         server.URL,
		SessionMode: httpclient.SessionModeNone,
		Headers: map[string][]string{
			"Cookie": {"sid=manual"},
		},
	})
	if err != nil {
		t.Fatalf("disabled-session request failed: %v", err)
	}
	if got, want := response.StatusCode, http.StatusOK; got != want {
		t.Fatalf("status code mismatch: got %d want %d", got, want)
	}
}

func TestDoAttachesNamedBearerAuth(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.Header.Get("Authorization"), "Bearer issued-token"; got != want {
			t.Fatalf("authorization header mismatch: got %q want %q", got, want)
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	response, err := Do(context.Background(), nil, &theater.HTTPSpec{
		Auth: map[string]theater.HTTPAuthSpec{
			"ci_api": {Attach: []theater.HTTPAuthAttachmentSpec{{Bearer: &theater.HTTPBearerAuthSpec{Token: "issued-token"}}}},
		},
	}, Request{
		URL:  server.URL,
		Auth: "ci_api",
	})
	if err != nil {
		t.Fatalf("bearer request failed: %v", err)
	}
	if got, want := response.StatusCode, http.StatusOK; got != want {
		t.Fatalf("status code mismatch: got %d want %d", got, want)
	}
}

func TestDoAttachesNamedAPIKeyQuery(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.URL.Query().Get("api_key"), "issued-token"; got != want {
			t.Fatalf("query token mismatch: got %q want %q", got, want)
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	response, err := Do(context.Background(), nil, &theater.HTTPSpec{
		Auth: map[string]theater.HTTPAuthSpec{
			"ci_api": {Attach: []theater.HTTPAuthAttachmentSpec{{
				APIKey: &theater.HTTPAPIKeyAuthSpec{In: theater.HTTPAPIKeyInQuery, Name: "api_key", Value: "issued-token"},
			}}},
		},
	}, Request{
		URL:  server.URL + "/resource",
		Auth: "ci_api",
	})
	if err != nil {
		t.Fatalf("api key request failed: %v", err)
	}
	if got, want := response.StatusCode, http.StatusOK; got != want {
		t.Fatalf("status code mismatch: got %d want %d", got, want)
	}
}

func TestDoRejectsUndeclaredNamedAuth(t *testing.T) {
	t.Parallel()

	_, err := Do(context.Background(), nil, &theater.HTTPSpec{}, Request{
		URL:  "https://user:pass@example.test/path-secret?token=query-secret",
		Auth: "missing",
	})
	if err == nil {
		t.Fatal("expected undeclared auth error")
	}
	if got, want := err.Error(), `auth "missing" is not declared`; got != want {
		t.Fatalf("error mismatch: got %q want %q", got, want)
	}

	diagnostic, ok := HTTPDiagnosticFromError(err)
	if !ok {
		t.Fatal("expected http diagnostic on request error")
	}
	if got, want := diagnostic.FailureKind, theater.HTTPDiagnosticFailureRequest; got != want {
		t.Fatalf("diagnostic failure kind mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Method, http.MethodGet; got != want {
		t.Fatalf("diagnostic method mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.URL, "https://example.test/redacted?token=redacted"; got != want {
		t.Fatalf("diagnostic url mismatch: got %q want %q", got, want)
	}
	if diagnostic.RequestFingerprint == nil {
		t.Fatal("request error diagnostic must include request fingerprint")
	}
	if got, want := diagnostic.RequestFingerprint.Host, "example.test"; got != want {
		t.Fatalf("fingerprint host mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.RequestFingerprint.PathShape, "/redacted"; got != want {
		t.Fatalf("fingerprint path shape mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.RequestFingerprint.QueryKeys, []string{"redacted"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fingerprint query keys mismatch: got %#v want %#v", got, want)
	}
	assertDiagnosticTextOmits(t, diagnostic, "user", "pass", "path-secret", "query-secret")
	if diagnostic.StatusCode != 0 || diagnostic.Status != "" || diagnostic.ResponseMetadata != nil || diagnostic.ResponsePreview != nil {
		t.Fatalf("request error diagnostic must not include response data: %#v", diagnostic)
	}
}

func TestDoAppliesIdentityAuthAndAllowsAuthNoneOverride(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/with-auth":
			if got, want := request.Header.Get("Authorization"), "Bearer issued-token"; got != want {
				t.Fatalf("authorization header mismatch: got %q want %q", got, want)
			}
		case "/without-auth":
			if got := request.Header.Get("Authorization"); got != "" {
				t.Fatalf("authorization header must be empty, got %q", got)
			}
		default:
			t.Fatalf("unexpected path %q", request.URL.Path)
		}

		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	httpSpec := &theater.HTTPSpec{
		Auth: map[string]theater.HTTPAuthSpec{
			"ci_api": {Attach: []theater.HTTPAuthAttachmentSpec{{Bearer: &theater.HTTPBearerAuthSpec{Token: "issued-token"}}}},
		},
		Identities: map[string]theater.HTTPIdentitySpec{
			"user": {Auth: "ci_api"},
		},
	}

	if _, err := Do(context.Background(), theater.NewResourceScope(), httpSpec, Request{
		URL:      server.URL + "/with-auth",
		Identity: "user",
	}); err != nil {
		t.Fatalf("identity request failed: %v", err)
	}

	if _, err := Do(context.Background(), theater.NewResourceScope(), httpSpec, Request{
		URL:      server.URL + "/without-auth",
		Identity: "user",
		AuthMode: authModeNone,
	}); err != nil {
		t.Fatalf("auth none request failed: %v", err)
	}
}

func TestCaptureAuthStoresHeaderSlotAndReplaysIt(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.Header.Get("X-CSRF-Token"), "issued-csrf"; got != want {
			t.Fatalf("csrf header mismatch: got %q want %q", got, want)
		}

		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	httpSpec := &theater.HTTPSpec{
		Auth: map[string]theater.HTTPAuthSpec{
			"web": {Attach: []theater.HTTPAuthAttachmentSpec{{HeaderSlot: &theater.HTTPHeaderSlotAuthSpec{Name: "X-CSRF-Token", Slot: "csrf"}}}},
		},
	}
	resources := theater.NewResourceScope()

	if err := CaptureAuth(resources, httpSpec, theater.HTTPAuthCaptureSpec{
		Auth: "web",
		Slots: map[string]theater.HTTPCaptureSourceSpec{
			"csrf": {ResponseHeader: "X-CSRF-Token"},
		},
	}, Response{
		Headers: http.Header{"X-CSRF-Token": []string{"issued-csrf"}},
	}); err != nil {
		t.Fatalf("capture auth failed: %v", err)
	}

	if _, err := Do(context.Background(), resources, httpSpec, Request{
		URL:  server.URL,
		Auth: "web",
	}); err != nil {
		t.Fatalf("request with captured header failed: %v", err)
	}
}

func TestDoAppliesCapturedFormSlotAndEncodesFormBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.Header.Get("Content-Type"), "application/x-www-form-urlencoded"; got != want {
			t.Fatalf("content-type mismatch: got %q want %q", got, want)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("parse form failed: %v", err)
		}
		if got, want := request.PostForm.Get("username"), "demo"; got != want {
			t.Fatalf("username mismatch: got %q want %q", got, want)
		}
		if got, want := request.PostForm.Get("csrf"), "issued-csrf"; got != want {
			t.Fatalf("csrf mismatch: got %q want %q", got, want)
		}

		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	httpSpec := &theater.HTTPSpec{
		Auth: map[string]theater.HTTPAuthSpec{
			"web": {Attach: []theater.HTTPAuthAttachmentSpec{{FormSlot: &theater.HTTPFormSlotAuthSpec{Name: "csrf", Slot: "csrf"}}}},
		},
	}
	resources := theater.NewResourceScope()

	if err := CaptureAuth(resources, httpSpec, theater.HTTPAuthCaptureSpec{
		Auth: "web",
		Slots: map[string]theater.HTTPCaptureSourceSpec{
			"csrf": {ResponseCookie: "csrf"},
		},
	}, Response{
		Headers: http.Header{"Set-Cookie": []string{"csrf=issued-csrf; Path=/"}},
	}); err != nil {
		t.Fatalf("capture auth failed: %v", err)
	}

	if _, err := Do(context.Background(), resources, httpSpec, Request{
		Method: http.MethodPost,
		URL:    server.URL,
		Form: map[string]string{
			"username": "demo",
		},
		Auth: "web",
	}); err != nil {
		t.Fatalf("form request failed: %v", err)
	}
}

func TestCaptureAuthSupportsJSONPointerAndFormFieldSources(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.Header.Get("X-JSON-Token"), "json-token"; got != want {
			t.Fatalf("json token header mismatch: got %q want %q", got, want)
		}
		if got, want := request.URL.Query().Get("form_token"), "form-token"; got != want {
			t.Fatalf("form token query mismatch: got %q want %q", got, want)
		}

		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	httpSpec := &theater.HTTPSpec{
		Auth: map[string]theater.HTTPAuthSpec{
			"web": {Attach: []theater.HTTPAuthAttachmentSpec{
				{HeaderSlot: &theater.HTTPHeaderSlotAuthSpec{Name: "X-JSON-Token", Slot: "json"}},
				{QuerySlot: &theater.HTTPQuerySlotAuthSpec{Name: "form_token", Slot: "form"}},
			}},
		},
	}
	resources := theater.NewResourceScope()

	if err := CaptureAuth(resources, httpSpec, theater.HTTPAuthCaptureSpec{
		Auth: "web",
		Slots: map[string]theater.HTTPCaptureSourceSpec{
			"json": {JSONPointer: theater.JSONPointer("/token")},
			"form": {FormField: "csrf"},
		},
	}, Response{
		Body: []byte(`csrf=form-token`),
	}); err == nil {
		t.Fatal("expected mixed-source capture to fail because JSON source body is not JSON")
	}

	if err := CaptureAuth(resources, httpSpec, theater.HTTPAuthCaptureSpec{
		Auth: "web",
		Slots: map[string]theater.HTTPCaptureSourceSpec{
			"json": {JSONPointer: theater.JSONPointer("/token")},
		},
	}, Response{
		Body: []byte(`{"token":"json-token"}`),
	}); err != nil {
		t.Fatalf("json capture failed: %v", err)
	}

	if err := CaptureAuth(resources, httpSpec, theater.HTTPAuthCaptureSpec{
		Auth: "web",
		Slots: map[string]theater.HTTPCaptureSourceSpec{
			"form": {FormField: "csrf"},
		},
	}, Response{
		Body: []byte(`csrf=form-token`),
	}); err != nil {
		t.Fatalf("form capture failed: %v", err)
	}

	if _, err := Do(context.Background(), resources, httpSpec, Request{
		URL:  server.URL,
		Auth: "web",
	}); err != nil {
		t.Fatalf("request with captured json/form slots failed: %v", err)
	}
}

func TestDoRejectsCookieHeaderWhenManagedClientIsAttached(t *testing.T) {
	t.Parallel()

	_, err := Do(context.Background(), theater.NewResourceScope(), nil, Request{
		URL: "http://example.invalid",
		Headers: map[string][]string{
			"Cookie": {"sid=manual"},
		},
	})
	if err == nil {
		t.Fatal("expected managed cookie header error")
	}

	if got, want := err.Error(), "headers.Cookie is incompatible with managed HTTP sessions"; got != want {
		t.Fatalf("error mismatch: got %q want %q", got, want)
	}
}

func TestClientFromResourcesSharesRunFactoryAcrossScopes(t *testing.T) {
	t.Parallel()

	var (
		mu             sync.Mutex
		newConnections int
	)

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	server.Config.ConnState = func(conn net.Conn, state http.ConnState) {
		if state != http.StateNew {
			return
		}

		mu.Lock()
		defer mu.Unlock()
		newConnections++
	}
	server.Start()
	defer server.Close()

	initializer, ok := newScenarioScopeInitializer().(*scenarioScopeInitializer)
	if !ok {
		t.Fatalf("initializer type mismatch: got %T", newScenarioScopeInitializer())
	}
	t.Cleanup(initializer.Close)

	firstResources := theater.NewResourceScope()
	secondResources := theater.NewResourceScope()

	initializer.InitializeScenarioScope(firstResources)
	initializer.InitializeScenarioScope(secondResources)

	firstClient, ok := clientFromResources(firstResources).(*httpclient.Client)
	if !ok {
		t.Fatalf("first client type mismatch: got %T", clientFromResources(firstResources))
	}

	secondClient, ok := clientFromResources(secondResources).(*httpclient.Client)
	if !ok {
		t.Fatalf("second client type mismatch: got %T", clientFromResources(secondResources))
	}

	if firstClient == secondClient {
		t.Fatal("expected different runtime scopes to get different managed clients")
	}

	if _, err := firstClient.Do(context.Background(), httpclient.Request{
		Method: http.MethodGet,
		URL:    server.URL,
	}); err != nil {
		t.Fatalf("first request failed: %v", err)
	}

	if _, err := secondClient.Do(context.Background(), httpclient.Request{
		Method: http.MethodGet,
		URL:    server.URL,
	}); err != nil {
		t.Fatalf("second request failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if got, want := newConnections, 1; got != want {
		t.Fatalf("new connection count mismatch: got %d want %d", got, want)
	}
}

func TestClientFromResourcesCachesFactoryWhenScopeIsUnseeded(t *testing.T) {
	t.Parallel()

	resources := theater.NewResourceScope()

	firstClient, ok := clientFromResources(resources).(*httpclient.Client)
	if !ok {
		t.Fatalf("first client type mismatch: got %T", clientFromResources(resources))
	}

	secondClient, ok := clientFromResources(resources).(*httpclient.Client)
	if !ok {
		t.Fatalf("second client type mismatch: got %T", clientFromResources(resources))
	}

	if firstClient != secondClient {
		t.Fatal("expected one runtime scope to reuse the same managed client")
	}

	factory, ok := resources.GetOrCreate(clientFactoryScopeKey, func() any {
		t.Fatal("client factory should already be cached")
		return nil
	}).(*httpclient.ClientFactory)
	if !ok || factory == nil {
		t.Fatalf("cached factory mismatch: got %T", factory)
	}

	t.Cleanup(factory.CloseIdleConnections)
}

func assertDiagnosticTextOmits(t *testing.T, diagnostic theater.HTTPDiagnostic, forbidden ...string) {
	t.Helper()

	data, err := json.Marshal(diagnostic)
	if err != nil {
		t.Fatalf("marshal diagnostic failed: %v", err)
	}
	for _, value := range forbidden {
		if strings.Contains(string(data), value) {
			t.Fatalf("diagnostic leaked %q: %s", value, data)
		}
	}
}
