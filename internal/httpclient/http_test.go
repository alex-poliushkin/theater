package httpclient

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestClientFactoryCreatesClientsWithSharedTransportAndTunedPool(t *testing.T) {
	t.Parallel()

	factory := NewClientFactory()
	t.Cleanup(factory.CloseIdleConnections)

	managed := factory.NewClient()
	stateless := factory.NewStatelessClient()

	managedHTTPClient, err := managed.httpClient(Request{})
	if err != nil {
		t.Fatalf("managed http client failed: %v", err)
	}

	statelessHTTPClient, err := stateless.httpClient(Request{})
	if err != nil {
		t.Fatalf("stateless http client failed: %v", err)
	}

	managedTransport, ok := managedHTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("managed transport type mismatch: got %T", managedHTTPClient.Transport)
	}

	statelessTransport, ok := statelessHTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("stateless transport type mismatch: got %T", statelessHTTPClient.Transport)
	}

	if managedTransport != statelessTransport {
		t.Fatal("expected managed and stateless clients to share the same transport instance")
	}

	if got, want := managedTransport.MaxIdleConns, sharedTransportMaxIdleConns; got != want {
		t.Fatalf("max idle conns mismatch: got %d want %d", got, want)
	}

	if got, want := managedTransport.MaxIdleConnsPerHost, sharedTransportMaxIdlePerHost; got != want {
		t.Fatalf("max idle conns per host mismatch: got %d want %d", got, want)
	}

	if got := managedTransport.MaxConnsPerHost; got != 0 {
		t.Fatalf("max conns per host mismatch: got %d want 0", got)
	}
}

func TestClientFactoryIsolatesIndependentFactories(t *testing.T) {
	t.Parallel()

	firstFactory := NewClientFactory()
	t.Cleanup(firstFactory.CloseIdleConnections)

	secondFactory := NewClientFactory()
	t.Cleanup(secondFactory.CloseIdleConnections)

	firstHTTPClient, err := firstFactory.NewStatelessClient().httpClient(Request{})
	if err != nil {
		t.Fatalf("first http client failed: %v", err)
	}

	secondHTTPClient, err := secondFactory.NewStatelessClient().httpClient(Request{})
	if err != nil {
		t.Fatalf("second http client failed: %v", err)
	}

	if firstHTTPClient.Transport == secondHTTPClient.Transport {
		t.Fatal("expected independent factories to use different transport instances")
	}
}

func TestStandaloneClientConstructorsUseIsolatedTransports(t *testing.T) {
	t.Parallel()

	managedHTTPClient, err := NewClient().httpClient(Request{})
	if err != nil {
		t.Fatalf("managed http client failed: %v", err)
	}

	statelessHTTPClient, err := NewStatelessClient().httpClient(Request{})
	if err != nil {
		t.Fatalf("stateless http client failed: %v", err)
	}

	if managedHTTPClient.Transport == statelessHTTPClient.Transport {
		t.Fatal("expected standalone constructors to use isolated transports")
	}
}

func TestClientDoReusesTransportAcrossDistinctClientsFromSameFactory(t *testing.T) {
	t.Parallel()

	var (
		mu              sync.Mutex
		newConnections  int
		remoteAddresses = make(map[string]struct{})
	)

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("ok"))
	}))
	server.Config.ConnState = func(conn net.Conn, state http.ConnState) {
		if state != http.StateNew {
			return
		}

		mu.Lock()
		defer mu.Unlock()

		newConnections++
		remoteAddresses[conn.RemoteAddr().String()] = struct{}{}
	}
	server.Start()
	defer server.Close()

	factory := NewClientFactory()
	t.Cleanup(factory.CloseIdleConnections)

	firstClient := factory.NewStatelessClient()
	secondClient := factory.NewStatelessClient()

	if _, err := firstClient.Do(context.Background(), Request{
		Method: http.MethodGet,
		URL:    server.URL,
	}); err != nil {
		t.Fatalf("first request failed: %v", err)
	}

	if _, err := secondClient.Do(context.Background(), Request{
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

	if got, want := len(remoteAddresses), 1; got != want {
		t.Fatalf("remote address count mismatch: got %d want %d", got, want)
	}
}

func TestStatelessClientRejectsConfiguredSession(t *testing.T) {
	t.Parallel()

	client := NewStatelessClient()

	_, err := client.Do(context.Background(), Request{
		Method:      http.MethodGet,
		URL:         "http://example.invalid",
		Session:     "auth",
		SessionMode: SessionModeNamed,
	})
	if err == nil {
		t.Fatal("expected stateless session error")
	}

	if got, want := err.Error(), "session requires scenario-local HTTP client"; got != want {
		t.Fatalf("error mismatch: got %q want %q", got, want)
	}
}

func TestManagedRequestRejectsCookieHeader(t *testing.T) {
	t.Parallel()

	client := NewClient()

	_, err := client.Do(context.Background(), Request{
		Method: http.MethodGet,
		URL:    "http://example.invalid",
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

func TestClientDoSetsDefaultUserAgent(t *testing.T) {
	t.Parallel()

	userAgent := ""
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		userAgent = request.UserAgent()
		_, _ = writer.Write([]byte("ok"))
	}))
	defer server.Close()

	client := NewStatelessClient()
	_, err := client.Do(context.Background(), Request{
		Method: http.MethodGet,
		URL:    server.URL,
	})
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if got, want := userAgent, defaultUserAgent; got != want {
		t.Fatalf("user-agent mismatch: got %q want %q", got, want)
	}
}

func TestClientDoPreservesConfiguredUserAgent(t *testing.T) {
	t.Parallel()

	userAgent := ""
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		userAgent = request.UserAgent()
		_, _ = writer.Write([]byte("ok"))
	}))
	defer server.Close()

	client := NewStatelessClient()
	_, err := client.Do(context.Background(), Request{
		Method: http.MethodGet,
		URL:    server.URL,
		Headers: map[string][]string{
			"user-agent": {"custom-client/2.0"},
		},
	})
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if got, want := userAgent, "custom-client/2.0"; got != want {
		t.Fatalf("user-agent mismatch: got %q want %q", got, want)
	}
}

func TestJarForSessionKeepsNamedSessionsIsolated(t *testing.T) {
	t.Parallel()

	client := NewClient()

	first, err := client.jarForSession("first")
	if err != nil {
		t.Fatalf("first jar failed: %v", err)
	}

	firstAgain, err := client.jarForSession("first")
	if err != nil {
		t.Fatalf("first repeat jar failed: %v", err)
	}

	second, err := client.jarForSession("second")
	if err != nil {
		t.Fatalf("second jar failed: %v", err)
	}

	if first != firstAgain {
		t.Fatal("expected same named session to reuse the same jar")
	}

	if first == second {
		t.Fatal("expected different named sessions to use different jars")
	}
}
