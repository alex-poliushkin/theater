package action_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/alex-poliushkin/theater"
	builtinaction "github.com/alex-poliushkin/theater/builtin/action"
	"github.com/alex-poliushkin/theater/builtin/internal/builtinhttp"
)

type recordingActionRegistrar struct {
	actions map[string]theater.Action
}

type recordingRuntimeActionRegistrar struct {
	recordingActionRegistrar
	initializers map[string]theater.ScenarioScopeInitializerFactory
}

func (r *recordingActionRegistrar) RegisterAction(ref string, action theater.Action) error {
	if r.actions == nil {
		r.actions = make(map[string]theater.Action)
	}

	r.actions[ref] = action
	return nil
}

func (r *recordingRuntimeActionRegistrar) RegisterScenarioScopeInitializer(
	ref string,
	factory theater.ScenarioScopeInitializerFactory,
) error {
	if r.initializers == nil {
		r.initializers = make(map[string]theater.ScenarioScopeInitializerFactory)
	}

	r.initializers[ref] = factory
	return nil
}

func TestRegisterAcceptsActionRegistrarPort(t *testing.T) {
	t.Parallel()

	registrar := &recordingActionRegistrar{}
	if err := builtinaction.Register(registrar); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	if _, ok := registrar.actions[builtinaction.HTTPRef]; !ok {
		t.Fatalf("expected %q to be registered", builtinaction.HTTPRef)
	}

	if _, ok := registrar.actions[builtinaction.CommandRef]; !ok {
		t.Fatalf("expected %q to be registered", builtinaction.CommandRef)
	}
}

func TestRegisterRuntimeRegistersScenarioScopeInitializer(t *testing.T) {
	t.Parallel()

	registrar := &recordingRuntimeActionRegistrar{}
	if err := builtinaction.RegisterRuntime(registrar); err != nil {
		t.Fatalf("register action runtime failed: %v", err)
	}

	if _, ok := registrar.actions[builtinaction.HTTPRef]; !ok {
		t.Fatalf("expected %q to be registered", builtinaction.HTTPRef)
	}

	if _, ok := registrar.actions[builtinaction.CommandRef]; !ok {
		t.Fatalf("expected %q to be registered", builtinaction.CommandRef)
	}

	if got, want := len(registrar.initializers), 1; got != want {
		t.Fatalf("initializer count mismatch: got %d want %d", got, want)
	}
}

func TestRegisterRegistersHTTPAction(t *testing.T) {
	t.Parallel()

	catalog := theater.NewCatalog()
	if err := builtinaction.RegisterRuntime(catalog); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	if _, err := catalog.ResolveAction(builtinaction.HTTPRef); err != nil {
		t.Fatalf("resolve action failed: %v", err)
	}
}

func TestHTTPActionContractDocumentsImplicitDefaultSession(t *testing.T) {
	t.Parallel()

	catalog := theater.NewCatalog()
	if err := builtinaction.RegisterRuntime(catalog); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	action, err := catalog.ResolveAction(builtinaction.HTTPRef)
	if err != nil {
		t.Fatalf("resolve action failed: %v", err)
	}

	session, ok := action.Contract().Inputs["session"]
	if !ok {
		t.Fatal("http action session input must be declared")
	}

	if got, want := session.Description, builtinhttp.SessionArgDescription; got != want {
		t.Fatalf("session description mismatch: got %q want %q", got, want)
	}

	auth, ok := action.Contract().Inputs["auth"]
	if !ok {
		t.Fatal("http action auth input must be declared")
	}

	if got, want := auth.Description, builtinhttp.AuthArgDescription; got != want {
		t.Fatalf("auth description mismatch: got %q want %q", got, want)
	}

	identity, ok := action.Contract().Inputs["identity"]
	if !ok {
		t.Fatal("http action identity input must be declared")
	}

	if got, want := identity.Description, builtinhttp.IdentityArgDescription; got != want {
		t.Fatalf("identity description mismatch: got %q want %q", got, want)
	}

	form, ok := action.Contract().Inputs["form"]
	if !ok {
		t.Fatal("http action form input must be declared")
	}

	if got, want := form.Description, builtinhttp.FormArgDescription; got != want {
		t.Fatalf("form description mismatch: got %q want %q", got, want)
	}
}

func TestHTTPActionRunnerSharesRunScopedTransportAcrossScenarioCalls(t *testing.T) {
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

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "fetch",
				Acts: []theater.ActSpec{
					{
						ID: "call",
						Action: theater.ActionSpec{
							Use: builtinaction.HTTPRef,
							With: map[string]theater.BindingSpec{
								"url": {Kind: theater.BindingKindLiteral, Value: server.URL},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "first", ScenarioID: "fetch"},
			{
				ID:         "second",
				ScenarioID: "fetch",
				Dependencies: []theater.ScenarioDependencySpec{
					{CallID: "first", When: theater.TriggerPredicateDone},
				},
			},
		},
	}

	catalog := theater.NewCatalog()
	if err := builtinaction.RegisterRuntime(catalog); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers, err := theater.NewMatcherCatalog()
	if err != nil {
		t.Fatalf("create matcher catalog failed: %v", err)
	}

	result, err := theater.NewRunner(catalog, matchers).Run(context.Background(), spec, theater.RunOptions{})
	if err != nil {
		t.Fatalf("runner run failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("run status mismatch: got %s want %s", got, want)
	}

	mu.Lock()
	defer mu.Unlock()

	if got, want := newConnections, 1; got != want {
		t.Fatalf("new connection count mismatch: got %d want %d", got, want)
	}
}

func TestHTTPActionContractDeclaresTimeoutInput(t *testing.T) {
	t.Parallel()

	catalog := theater.NewCatalog()
	if err := builtinaction.Register(catalog); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	registeredAction, err := catalog.ResolveAction(builtinaction.HTTPRef)
	if err != nil {
		t.Fatalf("resolve action failed: %v", err)
	}

	timeout, ok := registeredAction.Contract().Inputs["timeout"]
	if !ok {
		t.Fatal("timeout input is missing from http action contract")
	}

	if got, want := timeout.Kind, theater.ValueKindString; got != want {
		t.Fatalf("timeout kind mismatch: got %q want %q", got, want)
	}
}

func TestHTTPActionExecutesRequestAndReturnsResponseValues(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.Method, http.MethodPost; got != want {
			t.Fatalf("request method mismatch: got %q want %q", got, want)
		}

		if got, want := request.Header.Get("X-Test"), "ok"; got != want {
			t.Fatalf("request header mismatch: got %q want %q", got, want)
		}

		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(`{"token":"issued-token"}`))
	}))
	defer server.Close()

	catalog := theater.NewCatalog()
	if err := builtinaction.Register(catalog); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	registeredAction, err := catalog.ResolveAction(builtinaction.HTTPRef)
	if err != nil {
		t.Fatalf("resolve action failed: %v", err)
	}

	result, err := registeredAction.Run(context.Background(), theater.ActionRequest{
		Args: theater.Args{
			"method": http.MethodPost,
			"url":    server.URL,
			"headers": map[string]any{
				"X-Test": "ok",
			},
			"body":    `{"request":"ok"}`,
			"timeout": "2s",
		},
	})
	if err != nil {
		t.Fatalf("run action failed: %v", err)
	}

	if got, want := result["status_code"], http.StatusCreated; got != want {
		t.Fatalf("status code mismatch: got %v want %v", got, want)
	}

	if got, want := result["body"], `{"token":"issued-token"}`; got != want {
		t.Fatalf("response body mismatch: got %v want %v", got, want)
	}
}

func TestHTTPActionAppliesNamedBearerAuth(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.Header.Get("Authorization"), "Bearer issued-token"; got != want {
			t.Fatalf("authorization header mismatch: got %q want %q", got, want)
		}

		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	catalog := theater.NewCatalog()
	if err := builtinaction.Register(catalog); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	registeredAction, err := catalog.ResolveAction(builtinaction.HTTPRef)
	if err != nil {
		t.Fatalf("resolve action failed: %v", err)
	}

	result, err := registeredAction.Run(context.Background(), theater.ActionRequest{
		Args: theater.Args{
			"url":  server.URL,
			"auth": "ci_api",
		},
		HTTP: &theater.HTTPSpec{
			Auth: map[string]theater.HTTPAuthSpec{
				"ci_api": {Attach: []theater.HTTPAuthAttachmentSpec{{Bearer: &theater.HTTPBearerAuthSpec{Token: "issued-token"}}}},
			},
		},
	})
	if err != nil {
		t.Fatalf("http action failed: %v", err)
	}

	if got, want := result["status_code"], http.StatusOK; got != want {
		t.Fatalf("status code mismatch: got %v want %v", got, want)
	}
}

func TestHTTPActionPersistsCookiesWithinNamedSession(t *testing.T) {
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

	catalog := theater.NewCatalog()
	if err := builtinaction.Register(catalog); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	registeredAction, err := catalog.ResolveAction(builtinaction.HTTPRef)
	if err != nil {
		t.Fatalf("resolve action failed: %v", err)
	}

	resources := theater.NewResourceScope()
	httpSpec := &theater.HTTPSpec{
		Sessions: map[string]theater.HTTPSessionSpec{
			"auth": {},
		},
	}

	if _, err := registeredAction.Run(context.Background(), theater.ActionRequest{
		Args: theater.Args{
			"url":     server.URL + "/bootstrap",
			"session": "auth",
		},
		HTTP:      httpSpec,
		Resources: resources,
	}); err != nil {
		t.Fatalf("bootstrap action failed: %v", err)
	}

	result, err := registeredAction.Run(context.Background(), theater.ActionRequest{
		Args: theater.Args{
			"url":     server.URL + "/protected",
			"session": "auth",
		},
		HTTP:      httpSpec,
		Resources: resources,
	})
	if err != nil {
		t.Fatalf("protected action failed: %v", err)
	}

	if got, want := result["status_code"], http.StatusOK; got != want {
		t.Fatalf("status code mismatch: got %v want %v", got, want)
	}
}

func TestHTTPActionUsesImplicitDefaultSessionWhenSessionIsOmitted(t *testing.T) {
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

	catalog := theater.NewCatalog()
	if err := builtinaction.Register(catalog); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	registeredAction, err := catalog.ResolveAction(builtinaction.HTTPRef)
	if err != nil {
		t.Fatalf("resolve action failed: %v", err)
	}

	resources := theater.NewResourceScope()

	if _, err := registeredAction.Run(context.Background(), theater.ActionRequest{
		Args:      theater.Args{"url": server.URL + "/bootstrap"},
		Resources: resources,
	}); err != nil {
		t.Fatalf("bootstrap action failed: %v", err)
	}

	result, err := registeredAction.Run(context.Background(), theater.ActionRequest{
		Args:      theater.Args{"url": server.URL + "/protected"},
		Resources: resources,
	})
	if err != nil {
		t.Fatalf("protected action failed: %v", err)
	}

	if got, want := result["status_code"], http.StatusOK; got != want {
		t.Fatalf("status code mismatch: got %v want %v", got, want)
	}
}

func TestHTTPActionSessionNoneSkipsImplicitDefaultSessionReuse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/bootstrap":
			http.SetCookie(writer, &http.Cookie{Name: "sid", Value: "issued"})
			writer.WriteHeader(http.StatusNoContent)
		case "/protected":
			cookie, err := request.Cookie("sid")
			if err == nil && cookie.Value == "issued" {
				writer.WriteHeader(http.StatusOK)
				return
			}

			writer.WriteHeader(http.StatusUnauthorized)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	catalog := theater.NewCatalog()
	if err := builtinaction.Register(catalog); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	registeredAction, err := catalog.ResolveAction(builtinaction.HTTPRef)
	if err != nil {
		t.Fatalf("resolve action failed: %v", err)
	}

	resources := theater.NewResourceScope()

	if _, err := registeredAction.Run(context.Background(), theater.ActionRequest{
		Args:      theater.Args{"url": server.URL + "/bootstrap"},
		Resources: resources,
	}); err != nil {
		t.Fatalf("bootstrap action failed: %v", err)
	}

	result, err := registeredAction.Run(context.Background(), theater.ActionRequest{
		Args: theater.Args{
			"url":     server.URL + "/protected",
			"session": theater.HTTPSessionNone,
		},
		Resources: resources,
	})
	if err != nil {
		t.Fatalf("protected action failed: %v", err)
	}

	if got, want := result["status_code"], http.StatusUnauthorized; got != want {
		t.Fatalf("status code mismatch: got %v want %v", got, want)
	}
}

func TestHTTPActionKeepsNamedSessionsIsolated(t *testing.T) {
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

	catalog := theater.NewCatalog()
	if err := builtinaction.Register(catalog); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	registeredAction, err := catalog.ResolveAction(builtinaction.HTTPRef)
	if err != nil {
		t.Fatalf("resolve action failed: %v", err)
	}

	resources := theater.NewResourceScope()
	httpSpec := &theater.HTTPSpec{
		Sessions: map[string]theater.HTTPSessionSpec{
			"first":  {},
			"second": {},
		},
	}

	if _, err := registeredAction.Run(context.Background(), theater.ActionRequest{
		Args: theater.Args{
			"url":     server.URL + "/bootstrap",
			"session": "first",
		},
		HTTP:      httpSpec,
		Resources: resources,
	}); err != nil {
		t.Fatalf("bootstrap action failed: %v", err)
	}

	result, err := registeredAction.Run(context.Background(), theater.ActionRequest{
		Args: theater.Args{
			"url":     server.URL + "/protected",
			"session": "second",
		},
		HTTP:      httpSpec,
		Resources: resources,
	})
	if err != nil {
		t.Fatalf("protected action failed: %v", err)
	}

	if got, want := result["status_code"], http.StatusUnauthorized; got != want {
		t.Fatalf("status code mismatch: got %v want %v", got, want)
	}
}

func TestHTTPActionRejectsCookieHeaderWhenManagedClientIsAttached(t *testing.T) {
	t.Parallel()

	catalog := theater.NewCatalog()
	if err := builtinaction.Register(catalog); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	registeredAction, err := catalog.ResolveAction(builtinaction.HTTPRef)
	if err != nil {
		t.Fatalf("resolve action failed: %v", err)
	}

	_, err = registeredAction.Run(context.Background(), theater.ActionRequest{
		Args: theater.Args{
			"url": serverURLForInvalidCookieHeader(),
			"headers": map[string]any{
				"Cookie": "sid=manual",
			},
		},
		Resources: theater.NewResourceScope(),
	})
	if err == nil {
		t.Fatalf("run action unexpectedly succeeded")
	}

	if got, want := err.Error(), "headers.Cookie is incompatible with managed HTTP sessions"; got != want {
		t.Fatalf("error mismatch: got %q want %q", got, want)
	}
}

func TestHTTPActionCapturesAuthSlotsAfterSuccessfulResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/login":
			writer.Header().Set("X-CSRF-Token", "issued-csrf")
			writer.WriteHeader(http.StatusOK)
		case "/submit":
			if got, want := request.Header.Get("X-CSRF-Token"), "issued-csrf"; got != want {
				t.Fatalf("csrf header mismatch: got %q want %q", got, want)
			}
			writer.WriteHeader(http.StatusOK)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	catalog := theater.NewCatalog()
	if err := builtinaction.Register(catalog); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	registeredAction, err := catalog.ResolveAction(builtinaction.HTTPRef)
	if err != nil {
		t.Fatalf("resolve action failed: %v", err)
	}

	httpSpec := &theater.HTTPSpec{
		Auth: map[string]theater.HTTPAuthSpec{
			"web": {Attach: []theater.HTTPAuthAttachmentSpec{{HeaderSlot: &theater.HTTPHeaderSlotAuthSpec{Name: "X-CSRF-Token", Slot: "csrf"}}}},
		},
	}
	resources := theater.NewResourceScope()

	if _, err := registeredAction.Run(context.Background(), theater.ActionRequest{
		Args: theater.Args{
			"url": server.URL + "/login",
		},
		HTTP: httpSpec,
		HTTPCapture: &theater.HTTPAuthCaptureSpec{
			Auth: "web",
			Slots: map[string]theater.HTTPCaptureSourceSpec{
				"csrf": {ResponseHeader: "X-CSRF-Token"},
			},
		},
		Resources: resources,
	}); err != nil {
		t.Fatalf("login action failed: %v", err)
	}

	if _, err := registeredAction.Run(context.Background(), theater.ActionRequest{
		Args: theater.Args{
			"url":  server.URL + "/submit",
			"auth": "web",
		},
		HTTP:      httpSpec,
		Resources: resources,
	}); err != nil {
		t.Fatalf("submit action failed: %v", err)
	}
}

func serverURLForInvalidCookieHeader() string {
	return "http://example.invalid"
}
