package httpclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"sync"
	"time"
)

const (
	defaultUserAgent              = "theater-client/1.0"
	defaultSessionName            = "default"
	sharedTransportMaxIdleConns   = 256
	sharedTransportMaxIdlePerHost = 32
)

const (
	SessionModeDefault SessionMode = iota
	SessionModeNamed
	SessionModeNone
)

type ClientFactory struct {
	transport *http.Transport
}

type Client struct {
	transport      *http.Transport
	manageSessions bool
	mu             sync.Mutex
	jars           map[string]http.CookieJar
}

type SessionMode uint8

type Request struct {
	Method      string
	URL         string
	Headers     map[string][]string
	Body        []byte
	Timeout     time.Duration
	Session     string
	SessionMode SessionMode
}

type Response struct {
	Body       []byte
	Header     http.Header
	Status     string
	StatusCode int
}

func NewClientFactory() *ClientFactory {
	return &ClientFactory{transport: newSharedTransport()}
}

func NewClient() *Client {
	return NewClientFactory().NewClient()
}

func NewStatelessClient() *Client {
	return NewClientFactory().NewStatelessClient()
}

func (f *ClientFactory) NewClient() *Client {
	return newClient(true, f.transportInstance())
}

func (f *ClientFactory) NewStatelessClient() *Client {
	return newClient(false, f.transportInstance())
}

func (f *ClientFactory) CloseIdleConnections() {
	if f == nil || f.transport == nil {
		return
	}

	f.transport.CloseIdleConnections()
}

func (c *Client) Do(ctx context.Context, request Request) (Response, error) {
	if c == nil {
		c = NewStatelessClient()
	}

	httpRequest, err := request.newHTTPRequest(ctx, c.manageSessions && request.SessionMode != SessionModeNone)
	if err != nil {
		return Response{}, err
	}

	httpClient, err := c.httpClient(request)
	if err != nil {
		return Response{}, err
	}

	response, err := httpClient.Do(httpRequest)
	if err != nil {
		return Response{}, err
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return Response{}, err
	}

	return Response{
		Body:       responseBody,
		Header:     response.Header.Clone(),
		Status:     response.Status,
		StatusCode: response.StatusCode,
	}, nil
}

func (c *Client) httpClient(request Request) (*http.Client, error) {
	client := &http.Client{
		Transport: c.transportInstance(),
	}
	if request.Timeout > 0 {
		client.Timeout = request.Timeout
	}

	if !c.manageSessions {
		if request.SessionMode == SessionModeNamed {
			return nil, errors.New("session requires scenario-local HTTP client")
		}

		return client, nil
	}

	if request.SessionMode == SessionModeNone {
		return client, nil
	}

	jar, err := c.jarForSession(request.Session)
	if err != nil {
		return nil, err
	}

	client.Jar = jar
	return client, nil
}

func (c *Client) jarForSession(name string) (http.CookieJar, error) {
	sessionName := normalizeSessionName(name)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.jars == nil {
		c.jars = make(map[string]http.CookieJar)
	}

	if jar, ok := c.jars[sessionName]; ok {
		return jar, nil
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	c.jars[sessionName] = jar
	return jar, nil
}

func newSharedTransport() *http.Transport {
	template, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		template = fallbackSharedTransport()
	}

	transport := template.Clone()
	transport.MaxIdleConns = sharedTransportMaxIdleConns
	transport.MaxIdleConnsPerHost = sharedTransportMaxIdlePerHost
	transport.MaxConnsPerHost = 0
	return transport
}

func fallbackSharedTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

func newClient(manageSessions bool, transport *http.Transport) *Client {
	client := &Client{
		transport:      transport,
		manageSessions: manageSessions,
	}

	if manageSessions {
		client.jars = make(map[string]http.CookieJar)
	}

	return client
}

func (f *ClientFactory) transportInstance() *http.Transport {
	if f == nil || f.transport == nil {
		return newSharedTransport()
	}

	return f.transport
}

func (c *Client) transportInstance() *http.Transport {
	if c == nil || c.transport == nil {
		return newSharedTransport()
	}

	return c.transport
}

func normalizeSessionName(name string) string {
	if name == "" {
		return defaultSessionName
	}

	return name
}

func (r Request) newHTTPRequest(ctx context.Context, managedSession bool) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, r.Method, r.URL, bytes.NewReader(r.Body))
	if err != nil {
		return nil, err
	}

	if managedSession && hasHeader(r.Headers, "Cookie") {
		return nil, errors.New("headers.Cookie is incompatible with managed HTTP sessions")
	}

	addHeaders(request, r.Headers)
	addDefaultUserAgent(request)
	return request, nil
}

func addDefaultUserAgent(request *http.Request) {
	if hasHeader(request.Header, "User-Agent") {
		return
	}

	request.Header.Set("User-Agent", defaultUserAgent)
}

func hasHeader(headers map[string][]string, name string) bool {
	canonical := http.CanonicalHeaderKey(name)
	for key := range headers {
		if http.CanonicalHeaderKey(key) == canonical {
			return true
		}
	}

	return false
}

func addHeaders(request *http.Request, headers map[string][]string) {
	for key, values := range headers {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
}
