package inventory_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/alex-poliushkin/theater"
	builtinaction "github.com/alex-poliushkin/theater/builtin/action"
	"github.com/alex-poliushkin/theater/builtin/internal/builtinhttp"
	builtininventory "github.com/alex-poliushkin/theater/builtin/inventory"
	"github.com/alex-poliushkin/theater/internal/testkit"
)

type recordingInventoryRegistrar struct {
	inventories map[string]theater.Inventory
}

type recordingRuntimeInventoryRegistrar struct {
	recordingInventoryRegistrar
	initializers map[string]theater.ScenarioScopeInitializerFactory
}

func (r *recordingInventoryRegistrar) RegisterInventory(ref string, inventory theater.Inventory) error {
	if r.inventories == nil {
		r.inventories = make(map[string]theater.Inventory)
	}

	r.inventories[ref] = inventory
	return nil
}

func (r *recordingRuntimeInventoryRegistrar) RegisterScenarioScopeInitializer(
	ref string,
	factory theater.ScenarioScopeInitializerFactory,
) error {
	if r.initializers == nil {
		r.initializers = make(map[string]theater.ScenarioScopeInitializerFactory)
	}

	r.initializers[ref] = factory
	return nil
}

func TestRegisterAcceptsInventoryRegistrarPort(t *testing.T) {
	t.Parallel()

	registrar := &recordingInventoryRegistrar{}
	if err := builtininventory.Register(registrar); err != nil {
		t.Fatalf("register inventories failed: %v", err)
	}

	if _, ok := registrar.inventories[builtininventory.EnvRef]; !ok {
		t.Fatalf("expected %q to be registered", builtininventory.EnvRef)
	}

	if _, ok := registrar.inventories[builtininventory.FileRef]; !ok {
		t.Fatalf("expected %q to be registered", builtininventory.FileRef)
	}

	if _, ok := registrar.inventories[builtininventory.HTTPGetRef]; !ok {
		t.Fatalf("expected %q to be registered", builtininventory.HTTPGetRef)
	}
}

func TestRegisterRuntimeRegistersScenarioScopeInitializer(t *testing.T) {
	t.Parallel()

	registrar := &recordingRuntimeInventoryRegistrar{}
	if err := builtininventory.RegisterRuntime(registrar); err != nil {
		t.Fatalf("register inventories runtime failed: %v", err)
	}

	if _, ok := registrar.inventories[builtininventory.EnvRef]; !ok {
		t.Fatalf("expected %q to be registered", builtininventory.EnvRef)
	}

	if _, ok := registrar.inventories[builtininventory.FileRef]; !ok {
		t.Fatalf("expected %q to be registered", builtininventory.FileRef)
	}

	if _, ok := registrar.inventories[builtininventory.HTTPGetRef]; !ok {
		t.Fatalf("expected %q to be registered", builtininventory.HTTPGetRef)
	}

	if got, want := len(registrar.initializers), 1; got != want {
		t.Fatalf("initializer count mismatch: got %d want %d", got, want)
	}
}

func TestRegisterRegistersInventories(t *testing.T) {
	t.Parallel()

	catalog := theater.NewCatalog()
	if err := builtininventory.RegisterRuntime(catalog); err != nil {
		t.Fatalf("register inventories failed: %v", err)
	}

	if _, err := catalog.ResolveInventory(builtininventory.EnvRef); err != nil {
		t.Fatalf("resolve env inventory failed: %v", err)
	}

	if _, err := catalog.ResolveInventory(builtininventory.FileRef); err != nil {
		t.Fatalf("resolve file inventory failed: %v", err)
	}

	if _, err := catalog.ResolveInventory(builtininventory.HTTPGetRef); err != nil {
		t.Fatalf("resolve http inventory failed: %v", err)
	}
}

func TestHTTPInventoryContractDocumentsImplicitDefaultSession(t *testing.T) {
	t.Parallel()

	catalog := theater.NewCatalog()
	if err := builtininventory.RegisterRuntime(catalog); err != nil {
		t.Fatalf("register inventories failed: %v", err)
	}

	inventory, err := catalog.ResolveInventory(builtininventory.HTTPGetRef)
	if err != nil {
		t.Fatalf("resolve inventory failed: %v", err)
	}

	contract := inventory.Contract()
	var sessionArg *theater.ArgSpec
	for i := range contract.Args {
		arg := &contract.Args[i]
		if arg.Name == "session" {
			sessionArg = arg
			break
		}
	}

	if sessionArg == nil {
		t.Fatal("http inventory session arg must be declared")
	}

	if got, want := sessionArg.Description, builtinhttp.SessionArgDescription; got != want {
		t.Fatalf("session description mismatch: got %q want %q", got, want)
	}

	var authArg *theater.ArgSpec
	for i := range contract.Args {
		arg := &contract.Args[i]
		if arg.Name == "auth" {
			authArg = arg
			break
		}
	}

	if authArg == nil {
		t.Fatal("http inventory auth arg must be declared")
	}

	if got, want := authArg.Description, builtinhttp.AuthArgDescription; got != want {
		t.Fatalf("auth description mismatch: got %q want %q", got, want)
	}

	var identityArg *theater.ArgSpec
	for i := range contract.Args {
		arg := &contract.Args[i]
		if arg.Name == "identity" {
			identityArg = arg
			break
		}
	}

	if identityArg == nil {
		t.Fatal("http inventory identity arg must be declared")
	}

	if got, want := identityArg.Description, builtinhttp.IdentityArgDescription; got != want {
		t.Fatalf("identity description mismatch: got %q want %q", got, want)
	}

	var formArg *theater.ArgSpec
	for i := range contract.Args {
		arg := &contract.Args[i]
		if arg.Name == "form" {
			formArg = arg
			break
		}
	}

	if formArg == nil {
		t.Fatal("http inventory form arg must be declared")
	}

	if got, want := formArg.Description, builtinhttp.FormArgDescription; got != want {
		t.Fatalf("form description mismatch: got %q want %q", got, want)
	}
}

func TestHTTPInventoryRunnerSharesRunScopedTransportAcrossScenarioCalls(t *testing.T) {
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
						ID:     "call",
						Action: theater.ActionSpec{Use: "action.noop"},
						Properties: map[string]theater.PropertySpec{
							"response": {
								Inventory: &theater.InventoryCall{
									Use: builtininventory.HTTPGetRef,
									With: map[string]theater.BindingSpec{
										"url": {Kind: theater.BindingKindLiteral, Value: server.URL},
									},
								},
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
	if err := builtininventory.RegisterRuntime(catalog); err != nil {
		t.Fatalf("register inventories failed: %v", err)
	}
	if err := catalog.RegisterAction("action.noop", &testkit.ScriptedAction{}); err != nil {
		t.Fatalf("register noop action failed: %v", err)
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

func TestEnvInventoryReadsEnvironmentVariable(t *testing.T) {
	t.Setenv("THEATER_TEST_TOKEN", "issued-token")

	catalog := theater.NewCatalog()
	if err := builtininventory.Register(catalog); err != nil {
		t.Fatalf("register inventories failed: %v", err)
	}

	registeredInventory, err := catalog.ResolveInventory(builtininventory.EnvRef)
	if err != nil {
		t.Fatalf("resolve inventory failed: %v", err)
	}

	value, err := registeredInventory.Acquire(context.Background(), theater.InventoryRequest{
		Args: theater.Args{"name": "THEATER_TEST_TOKEN"},
	})
	if err != nil {
		t.Fatalf("acquire env failed: %v", err)
	}

	if got, want := value, "issued-token"; got != want {
		t.Fatalf("env value mismatch: got %v want %v", got, want)
	}
}

func TestFileInventoryReadsFileContents(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "payload.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	catalog := theater.NewCatalog()
	if err := builtininventory.Register(catalog); err != nil {
		t.Fatalf("register inventories failed: %v", err)
	}

	registeredInventory, err := catalog.ResolveInventory(builtininventory.FileRef)
	if err != nil {
		t.Fatalf("resolve inventory failed: %v", err)
	}

	value, err := registeredInventory.Acquire(context.Background(), theater.InventoryRequest{
		Args: theater.Args{"path": path},
	})
	if err != nil {
		t.Fatalf("acquire file failed: %v", err)
	}

	if got, want := string(value.([]byte)), "hello"; got != want {
		t.Fatalf("file value mismatch: got %q want %q", got, want)
	}
}

func TestHTTPInventoryFetchesResponseBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.Method, http.MethodGet; got != want {
			t.Fatalf("request method mismatch: got %q want %q", got, want)
		}

		if got, want := request.Header.Get("X-Token"), "issued-token"; got != want {
			t.Fatalf("request header mismatch: got %q want %q", got, want)
		}

		_, _ = writer.Write([]byte(`{"email":"alice@example.com"}`))
	}))
	defer server.Close()

	catalog := theater.NewCatalog()
	if err := builtininventory.Register(catalog); err != nil {
		t.Fatalf("register inventories failed: %v", err)
	}

	registeredInventory, err := catalog.ResolveInventory(builtininventory.HTTPGetRef)
	if err != nil {
		t.Fatalf("resolve inventory failed: %v", err)
	}

	value, err := registeredInventory.Acquire(context.Background(), theater.InventoryRequest{
		Args: theater.Args{
			"url": server.URL,
			"headers": map[string]any{
				"X-Token": "issued-token",
			},
		},
	})
	if err != nil {
		t.Fatalf("acquire http inventory failed: %v", err)
	}

	if got, want := string(value.([]byte)), `{"email":"alice@example.com"}`; got != want {
		t.Fatalf("inventory value mismatch: got %q want %q", got, want)
	}
}

func TestHTTPInventoryPersistsCookiesWithinNamedSession(t *testing.T) {
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
				_, _ = writer.Write([]byte("missing"))
				return
			}

			_, _ = writer.Write([]byte("ok"))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	catalog := theater.NewCatalog()
	if err := builtininventory.Register(catalog); err != nil {
		t.Fatalf("register inventories failed: %v", err)
	}

	inventory, err := catalog.ResolveInventory(builtininventory.HTTPGetRef)
	if err != nil {
		t.Fatalf("resolve inventory failed: %v", err)
	}

	resources := theater.NewResourceScope()
	httpSpec := &theater.HTTPSpec{
		Sessions: map[string]theater.HTTPSessionSpec{
			"auth": {},
		},
	}

	if _, err := inventory.Acquire(context.Background(), theater.InventoryRequest{
		Args: theater.Args{
			"url":     server.URL + "/bootstrap",
			"session": "auth",
		},
		HTTP:      httpSpec,
		Resources: resources,
	}); err != nil {
		t.Fatalf("bootstrap inventory failed: %v", err)
	}

	value, err := inventory.Acquire(context.Background(), theater.InventoryRequest{
		Args: theater.Args{
			"url":     server.URL + "/protected",
			"session": "auth",
		},
		HTTP:      httpSpec,
		Resources: resources,
	})
	if err != nil {
		t.Fatalf("protected inventory failed: %v", err)
	}

	if got, want := string(value.([]byte)), "ok"; got != want {
		t.Fatalf("inventory value mismatch: got %q want %q", got, want)
	}
}

func TestHTTPInventoryUsesImplicitDefaultSessionWhenSessionIsOmitted(t *testing.T) {
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
				_, _ = writer.Write([]byte("missing"))
				return
			}

			_, _ = writer.Write([]byte("ok"))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	catalog := theater.NewCatalog()
	if err := builtininventory.Register(catalog); err != nil {
		t.Fatalf("register inventories failed: %v", err)
	}

	inventory, err := catalog.ResolveInventory(builtininventory.HTTPGetRef)
	if err != nil {
		t.Fatalf("resolve inventory failed: %v", err)
	}

	resources := theater.NewResourceScope()

	if _, err := inventory.Acquire(context.Background(), theater.InventoryRequest{
		Args:      theater.Args{"url": server.URL + "/bootstrap"},
		Resources: resources,
	}); err != nil {
		t.Fatalf("bootstrap inventory failed: %v", err)
	}

	value, err := inventory.Acquire(context.Background(), theater.InventoryRequest{
		Args:      theater.Args{"url": server.URL + "/protected"},
		Resources: resources,
	})
	if err != nil {
		t.Fatalf("protected inventory failed: %v", err)
	}

	if got, want := string(value.([]byte)), "ok"; got != want {
		t.Fatalf("inventory value mismatch: got %q want %q", got, want)
	}
}

func TestHTTPActionAndInventoryShareImplicitDefaultScenarioLocalSession(t *testing.T) {
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
				_, _ = writer.Write([]byte("missing"))
				return
			}

			_, _ = writer.Write([]byte("ok"))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	catalog := theater.NewCatalog()
	if err := builtinaction.RegisterRuntime(catalog); err != nil {
		t.Fatalf("register actions failed: %v", err)
	}

	if err := builtininventory.RegisterRuntime(catalog); err != nil {
		t.Fatalf("register inventories failed: %v", err)
	}

	action, err := catalog.ResolveAction(builtinaction.HTTPRef)
	if err != nil {
		t.Fatalf("resolve action failed: %v", err)
	}

	inventory, err := catalog.ResolveInventory(builtininventory.HTTPGetRef)
	if err != nil {
		t.Fatalf("resolve inventory failed: %v", err)
	}

	resources := theater.NewResourceScope()

	if _, err := action.Run(context.Background(), theater.ActionRequest{
		Args:      theater.Args{"url": server.URL + "/bootstrap"},
		Resources: resources,
	}); err != nil {
		t.Fatalf("bootstrap action failed: %v", err)
	}

	value, err := inventory.Acquire(context.Background(), theater.InventoryRequest{
		Args:      theater.Args{"url": server.URL + "/protected"},
		Resources: resources,
	})
	if err != nil {
		t.Fatalf("protected inventory failed: %v", err)
	}

	if got, want := string(value.([]byte)), "ok"; got != want {
		t.Fatalf("inventory value mismatch: got %q want %q", got, want)
	}
}

func TestHTTPActionAndInventoryShareScenarioLocalSessionRegistry(t *testing.T) {
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
				_, _ = writer.Write([]byte("missing"))
				return
			}

			_, _ = writer.Write([]byte("ok"))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	catalog := theater.NewCatalog()
	if err := builtinaction.RegisterRuntime(catalog); err != nil {
		t.Fatalf("register actions failed: %v", err)
	}

	if err := builtininventory.RegisterRuntime(catalog); err != nil {
		t.Fatalf("register inventories failed: %v", err)
	}

	action, err := catalog.ResolveAction(builtinaction.HTTPRef)
	if err != nil {
		t.Fatalf("resolve action failed: %v", err)
	}

	inventory, err := catalog.ResolveInventory(builtininventory.HTTPGetRef)
	if err != nil {
		t.Fatalf("resolve inventory failed: %v", err)
	}

	resources := theater.NewResourceScope()
	httpSpec := &theater.HTTPSpec{
		Sessions: map[string]theater.HTTPSessionSpec{
			"auth": {},
		},
	}

	if _, err := action.Run(context.Background(), theater.ActionRequest{
		Args: theater.Args{
			"url":     server.URL + "/bootstrap",
			"session": "auth",
		},
		HTTP:      httpSpec,
		Resources: resources,
	}); err != nil {
		t.Fatalf("bootstrap action failed: %v", err)
	}

	value, err := inventory.Acquire(context.Background(), theater.InventoryRequest{
		Args: theater.Args{
			"url":     server.URL + "/protected",
			"session": "auth",
		},
		HTTP:      httpSpec,
		Resources: resources,
	})
	if err != nil {
		t.Fatalf("protected inventory failed: %v", err)
	}

	if got, want := string(value.([]byte)), "ok"; got != want {
		t.Fatalf("inventory value mismatch: got %q want %q", got, want)
	}
}

func TestHTTPInventoryAppliesNamedBasicAuth(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		user, password, ok := request.BasicAuth()
		if !ok {
			t.Fatal("basic auth is missing")
		}
		if got, want := user, "user"; got != want {
			t.Fatalf("basic auth username mismatch: got %q want %q", got, want)
		}
		if got, want := password, "pass"; got != want {
			t.Fatalf("basic auth password mismatch: got %q want %q", got, want)
		}

		_, _ = writer.Write([]byte("ok"))
	}))
	defer server.Close()

	catalog := theater.NewCatalog()
	if err := builtininventory.Register(catalog); err != nil {
		t.Fatalf("register inventories failed: %v", err)
	}

	inventory, err := catalog.ResolveInventory(builtininventory.HTTPGetRef)
	if err != nil {
		t.Fatalf("resolve inventory failed: %v", err)
	}

	value, err := inventory.Acquire(context.Background(), theater.InventoryRequest{
		Args: theater.Args{
			"url":  server.URL,
			"auth": "partner",
		},
		HTTP: &theater.HTTPSpec{
			Auth: map[string]theater.HTTPAuthSpec{
				"partner": {Attach: []theater.HTTPAuthAttachmentSpec{{Basic: &theater.HTTPBasicAuthSpec{Username: "user", Password: "pass"}}}},
			},
		},
	})
	if err != nil {
		t.Fatalf("http inventory failed: %v", err)
	}

	if got, want := string(value.([]byte)), "ok"; got != want {
		t.Fatalf("inventory value mismatch: got %q want %q", got, want)
	}
}

func TestHTTPInventoryAppliesIdentityDefaults(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.Header.Get("Authorization"), "Bearer issued-token"; got != want {
			t.Fatalf("authorization header mismatch: got %q want %q", got, want)
		}

		_, _ = writer.Write([]byte("ok"))
	}))
	defer server.Close()

	catalog := theater.NewCatalog()
	if err := builtininventory.Register(catalog); err != nil {
		t.Fatalf("register inventories failed: %v", err)
	}

	inventory, err := catalog.ResolveInventory(builtininventory.HTTPGetRef)
	if err != nil {
		t.Fatalf("resolve inventory failed: %v", err)
	}

	value, err := inventory.Acquire(context.Background(), theater.InventoryRequest{
		Args: theater.Args{
			"url":      server.URL,
			"identity": "user",
		},
		HTTP: &theater.HTTPSpec{
			Auth: map[string]theater.HTTPAuthSpec{
				"ci_api": {Attach: []theater.HTTPAuthAttachmentSpec{{Bearer: &theater.HTTPBearerAuthSpec{Token: "issued-token"}}}},
			},
			Identities: map[string]theater.HTTPIdentitySpec{
				"user": {Auth: "ci_api"},
			},
		},
	})
	if err != nil {
		t.Fatalf("http inventory failed: %v", err)
	}

	if got, want := string(value.([]byte)), "ok"; got != want {
		t.Fatalf("inventory value mismatch: got %q want %q", got, want)
	}
}
