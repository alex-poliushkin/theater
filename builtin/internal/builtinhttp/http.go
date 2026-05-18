package builtinhttp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/httpclient"
	"github.com/alex-poliushkin/theater/internal/runtimevalue"
	"github.com/alex-poliushkin/theater/internal/selectvalue"
)

const (
	defaultMethod               = http.MethodGet
	resourceNamespace           = "builtin/internal/builtinhttp"
	scenarioScopeInitializerRef = resourceNamespace
	formContentTypeError        = `headers.Content-Type must be "application/x-www-form-urlencoded" when form is used`
	SessionArgDescription       = "optional scenario-local HTTP session name; " +
		"when omitted, the request uses the implicit default session of the current scenario call; " +
		`use "none" to disable managed session reuse; named sessions must be declared in stage http.sessions`
	IdentityArgDescription = "optional named HTTP identity declared in stage http.identities; " +
		"identity provides default session/auth for the request; explicit session/auth overrides it"
	AuthArgDescription = "optional named HTTP auth config declared in stage http.auth; " +
		`use "none" to disable auth inherited from identity`
	FormArgDescription = "optional application/x-www-form-urlencoded request form; incompatible with raw body"
	JSONArgDescription = "optional application/json request body encoded from any bound runtime value; incompatible with raw body and form"

	httpDiagnosticPreviewLimitBytes = 4 * 1024
	httpDiagnosticQueryKeyLimit     = 32
	httpDiagnosticQueryKeyMaxBytes  = 128
	httpDiagnosticRedactedValue     = "redacted"
)

var clientFactoryScopeKey = theater.NewResourceKey(resourceNamespace, "client_factory")
var clientScopeKey = theater.NewResourceKey(resourceNamespace, "client")
var authStateScopeKey = theater.NewResourceKey(resourceNamespace, "auth_state")

type Request struct {
	Method      string
	URL         string
	Headers     map[string][]string
	Body        []byte
	Form        map[string]string
	JSON        any
	Timeout     time.Duration
	Session     string
	SessionMode httpclient.SessionMode
	Identity    string
	Auth        string
	AuthMode    authMode
}

type Response struct {
	Body       []byte
	Headers    http.Header
	Status     string
	StatusCode int
	Diagnostic theater.HTTPDiagnostic
}

func RegisterScenarioScopeInitializer(registrar theater.ScenarioScopeInitializerRegistrar) error {
	return registrar.RegisterScenarioScopeInitializer(scenarioScopeInitializerRef, newScenarioScopeInitializer)
}

func RequestFromArgs(args theater.Args) (Request, error) {
	method, err := optionalString(args["method"], "method", defaultMethod)
	if err != nil {
		return Request{}, err
	}

	requestURL, err := stringValue(args["url"], "url")
	if err != nil {
		return Request{}, err
	}

	_, bodyConfigured := args["body"]
	_, formConfigured := args["form"]
	jsonBody, jsonConfigured := args["json"]
	if bodyConfigured && formConfigured {
		return Request{}, errors.New("form is incompatible with body")
	}
	if bodyConfigured && jsonConfigured {
		return Request{}, errors.New("json is incompatible with body")
	}
	if formConfigured && jsonConfigured {
		return Request{}, errors.New("json is incompatible with form")
	}

	body, err := optionalBytes(args["body"], "body")
	if err != nil {
		return Request{}, err
	}

	form, err := optionalStringMap(args["form"], "form")
	if err != nil {
		return Request{}, err
	}

	headers, err := stringSliceMap(args["headers"], "headers")
	if err != nil {
		return Request{}, err
	}

	timeout, err := optionalDuration(args["timeout"], "timeout")
	if err != nil {
		return Request{}, err
	}

	session, sessionConfigured, err := optionalConfiguredString(args, "session")
	if err != nil {
		return Request{}, err
	}

	sessionMode, sessionName, err := sessionSelection(session, sessionConfigured)
	if err != nil {
		return Request{}, err
	}

	identity, identityConfigured, err := optionalConfiguredString(args, "identity")
	if err != nil {
		return Request{}, err
	}
	if identityConfigured && identity == "" {
		return Request{}, errors.New("identity must not be empty")
	}

	auth, authConfigured, err := optionalConfiguredString(args, "auth")
	if err != nil {
		return Request{}, err
	}
	authMode, authName, err := authSelection(auth, authConfigured)
	if err != nil {
		return Request{}, err
	}

	return Request{
		Method:      method,
		URL:         requestURL,
		Headers:     headers,
		Body:        body,
		Form:        form,
		JSON:        runtimevalue.Clone(jsonBody),
		Timeout:     timeout,
		Session:     sessionName,
		SessionMode: sessionMode,
		Identity:    identity,
		Auth:        authName,
		AuthMode:    authMode,
	}, nil
}

func Do(ctx context.Context, resources theater.ResourceScope, httpSpec *theater.HTTPSpec, request Request) (Response, error) {
	return defaultResolver.Resolve(resources).Do(ctx, httpSpec, request)
}

func HTTPDiagnosticFromError(err error) (theater.HTTPDiagnostic, bool) {
	var diagnosticErr diagnosticError
	if !errors.As(err, &diagnosticErr) {
		return theater.HTTPDiagnostic{}, false
	}

	return diagnosticErr.diagnostic, true
}

func Outputs(response Response) theater.Outputs {
	return theater.Outputs{
		"status_code": response.StatusCode,
		"status":      response.Status,
		"headers":     headerValues(response.Headers),
		"body":        string(response.Body),
	}
}

type requestExecutor interface {
	Do(ctx context.Context, httpSpec *theater.HTTPSpec, request Request) (Response, error)
}

type clientResolver interface {
	Resolve(resources theater.ResourceScope) requestExecutor
}

type transportClient interface {
	Do(ctx context.Context, request httpclient.Request) (httpclient.Response, error)
}

type defaultClientResolver struct{}

type scenarioScopeInitializer struct {
	factory *httpclient.ClientFactory
}

type transportExecutor struct {
	client    transportClient
	cleanup   func()
	resources theater.ResourceScope
}

type authMode uint8

type authStateStore struct {
	mu    sync.Mutex
	slots map[string]map[string]theater.Secret
}

type diagnosticError struct {
	cause      error
	diagnostic theater.HTTPDiagnostic
}

var defaultResolver clientResolver = defaultClientResolver{}

const (
	authModeDefault authMode = iota
	authModeNamed
	authModeNone
)

func (defaultClientResolver) Resolve(resources theater.ResourceScope) requestExecutor {
	client := clientFromResources(resources)
	if client == nil {
		factory := httpclient.NewClientFactory()
		return transportExecutor{
			client:    factory.NewStatelessClient(),
			cleanup:   factory.CloseIdleConnections,
			resources: resources,
		}
	}

	return transportExecutor{client: client, resources: resources}
}

func (e diagnosticError) Error() string {
	if e.cause == nil {
		return "http diagnostic error"
	}

	return e.cause.Error()
}

func (e diagnosticError) Unwrap() error {
	return e.cause
}

func (e transportExecutor) Do(ctx context.Context, httpSpec *theater.HTTPSpec, request Request) (Response, error) {
	if e.cleanup != nil {
		defer e.cleanup()
	}

	startedAt := time.Now()
	resolved, err := resolveRequest(e.resources, httpSpec, request)
	if err != nil {
		return Response{}, diagnosticError{
			cause:      err,
			diagnostic: newHTTPDiagnosticForError(request, nil, time.Since(startedAt), err, theater.HTTPDiagnosticFailureRequest),
		}
	}

	response, err := e.client.Do(ctx, transportRequest(resolved))
	if err != nil {
		return Response{}, diagnosticError{
			cause:      err,
			diagnostic: newHTTPDiagnosticForError(resolved, nil, time.Since(startedAt), err, theater.HTTPDiagnosticFailureNetwork),
		}
	}
	diagnostic := newHTTPDiagnostic(resolved, &response, time.Since(startedAt))

	return Response{
		Body:       bytes.Clone(response.Body),
		Headers:    response.Header.Clone(),
		Status:     response.Status,
		StatusCode: response.StatusCode,
		Diagnostic: diagnostic,
	}, nil
}

func transportRequest(request Request) httpclient.Request {
	return httpclient.Request{
		Method:      request.Method,
		URL:         request.URL,
		Headers:     cloneHeaderListMap(request.Headers),
		Body:        bytes.Clone(request.Body),
		Timeout:     request.Timeout,
		Session:     request.Session,
		SessionMode: request.SessionMode,
	}
}

func clientFromResources(resources theater.ResourceScope) transportClient {
	if resources == nil {
		return nil
	}

	factory := clientFactoryFromServices(resources)
	if factory == nil {
		return nil
	}

	client, _ := resources.GetOrCreate(clientScopeKey, func() any {
		return factory.NewClient()
	}).(*httpclient.Client)
	return client
}

func clientFactoryFromServices(resources theater.ResourceScope) *httpclient.ClientFactory {
	if resources == nil {
		return nil
	}

	factory, _ := resources.GetOrCreate(clientFactoryScopeKey, func() any {
		return httpclient.NewClientFactory()
	}).(*httpclient.ClientFactory)
	return factory
}

func newScenarioScopeInitializer() theater.ScenarioScopeInitializer {
	return &scenarioScopeInitializer{factory: httpclient.NewClientFactory()}
}

func (i *scenarioScopeInitializer) InitializeScenarioScope(resources theater.ResourceScope) {
	if i == nil || resources == nil {
		return
	}

	resources.GetOrCreate(clientFactoryScopeKey, func() any {
		return i.factory
	})
}

func (i *scenarioScopeInitializer) InitializeHTTPAuthSlots(
	resources theater.ResourceScope,
	bindings map[string]theater.Values,
) error {
	state := authStateFromResources(resources)
	if state == nil {
		return errors.New("http auth bindings require scenario-local resources")
	}

	for authName, values := range bindings {
		state.storeValues(authName, values)
	}

	return nil
}

func (i *scenarioScopeInitializer) Close() {
	if i == nil || i.factory == nil {
		return
	}

	i.factory.CloseIdleConnections()
}

func headerValues(headers http.Header) map[string]any {
	if len(headers) == 0 {
		return map[string]any{}
	}

	values := make(map[string]any, len(headers))
	for key, list := range headers {
		cloned := make([]any, len(list))
		for i := range list {
			cloned[i] = list[i]
		}
		values[key] = cloned
	}

	return values
}

func cloneHeaderListMap(headers map[string][]string) map[string][]string {
	if len(headers) == 0 {
		return nil
	}

	cloned := make(map[string][]string, len(headers))
	for key, values := range headers {
		cloned[key] = append([]string(nil), values...)
	}

	return cloned
}

func resolveRequest(resources theater.ResourceScope, httpSpec *theater.HTTPSpec, request Request) (Request, error) {
	resolved := Request{
		Method:      request.Method,
		URL:         request.URL,
		Headers:     cloneHeaderListMap(request.Headers),
		Body:        bytes.Clone(request.Body),
		Form:        cloneStringMap(request.Form),
		JSON:        runtimevalue.Clone(request.JSON),
		Timeout:     request.Timeout,
		Session:     request.Session,
		SessionMode: request.SessionMode,
		Identity:    request.Identity,
		Auth:        request.Auth,
		AuthMode:    request.AuthMode,
	}
	if resolved.AuthMode == authModeDefault && resolved.Auth != "" {
		resolved.AuthMode = authModeNamed
	}

	identity, err := resolveNamedIdentitySpec(httpSpec, request.Identity)
	if err != nil {
		return Request{}, err
	}

	if request.SessionMode == httpclient.SessionModeDefault && identity.Session != "" {
		resolved.SessionMode = httpclient.SessionModeNamed
		resolved.Session = identity.Session
	}
	if request.AuthMode == authModeDefault && identity.Auth != "" {
		resolved.AuthMode = authModeNamed
		resolved.Auth = identity.Auth
	}
	if resolved.AuthMode == authModeNone {
		resolved.Auth = ""
	}

	if err := validateNamedSession(httpSpec, resolved); err != nil {
		return Request{}, err
	}

	if resolved.AuthMode == authModeNamed {
		authSpec, err := resolveNamedAuthSpec(httpSpec, resolved.Auth)
		if err != nil {
			return Request{}, err
		}

		if err := applyAuthAttachments(resources, &resolved, resolved.Auth, authSpec); err != nil {
			return Request{}, err
		}
	}

	if err := materializeForm(&resolved); err != nil {
		return Request{}, err
	}
	if err := materializeJSON(&resolved); err != nil {
		return Request{}, err
	}

	return resolved, nil
}

func validateNamedSession(httpSpec *theater.HTTPSpec, request Request) error {
	if request.SessionMode != httpclient.SessionModeNamed {
		return nil
	}
	if httpSpec == nil {
		return fmt.Errorf("session %q is not declared", request.Session)
	}
	if _, ok := httpSpec.Sessions[request.Session]; ok {
		return nil
	}

	return fmt.Errorf("session %q is not declared", request.Session)
}

func resolveNamedAuthSpec(httpSpec *theater.HTTPSpec, name string) (theater.HTTPAuthSpec, error) {
	if httpSpec == nil {
		return theater.HTTPAuthSpec{}, fmt.Errorf("auth %q is not declared", name)
	}

	authSpec, ok := httpSpec.Auth[name]
	if !ok {
		return theater.HTTPAuthSpec{}, fmt.Errorf("auth %q is not declared", name)
	}

	return authSpec, nil
}

func resolveNamedIdentitySpec(httpSpec *theater.HTTPSpec, name string) (theater.HTTPIdentitySpec, error) {
	if name == "" {
		return theater.HTTPIdentitySpec{}, nil
	}
	if httpSpec == nil {
		return theater.HTTPIdentitySpec{}, fmt.Errorf("identity %q is not declared", name)
	}

	identity, ok := httpSpec.Identities[name]
	if !ok {
		return theater.HTTPIdentitySpec{}, fmt.Errorf("identity %q is not declared", name)
	}

	return identity, nil
}

func applyAuthAttachments(
	resources theater.ResourceScope,
	request *Request,
	authName string,
	authSpec theater.HTTPAuthSpec,
) error {
	if request.Headers == nil {
		request.Headers = make(map[string][]string)
	}
	if request.Form == nil {
		request.Form = make(map[string]string)
	}

	parsedURL, err := url.Parse(request.URL)
	if err != nil {
		return err
	}

	query := parsedURL.Query()
	injectedHeaders := make(map[string]struct{}, len(authSpec.Attach))
	injectedQuery := make(map[string]struct{}, len(authSpec.Attach))
	injectedForm := make(map[string]struct{}, len(authSpec.Attach))
	state := authStateFromResources(resources)
	for _, attachment := range authSpec.Attach {
		if err := applyAuthAttachment(
			state,
			request.Headers,
			query,
			request.Form,
			injectedHeaders,
			injectedQuery,
			injectedForm,
			authName,
			len(request.Body) != 0,
			attachment,
		); err != nil {
			return err
		}
	}

	parsedURL.RawQuery = query.Encode()
	request.URL = parsedURL.String()
	return nil
}

func applyAuthAttachment(
	state *authStateStore,
	headers map[string][]string,
	query url.Values,
	form map[string]string,
	injectedHeaders map[string]struct{},
	injectedQuery map[string]struct{},
	injectedForm map[string]struct{},
	authName string,
	hasBody bool,
	attachment theater.HTTPAuthAttachmentSpec,
) error {
	switch {
	case attachment.Bearer != nil:
		token, err := bearerToken(state, authName, *attachment.Bearer)
		if err != nil {
			return err
		}
		return injectHeader(headers, injectedHeaders, "Authorization", "Bearer "+token)
	case attachment.Basic != nil:
		return injectBasicAuth(headers, injectedHeaders, *attachment.Basic)
	case attachment.APIKey != nil:
		return injectAPIKey(headers, query, injectedHeaders, injectedQuery, *attachment.APIKey)
	case attachment.HeaderSlot != nil:
		value, err := resolveAuthSlot(state, authName, attachment.HeaderSlot.Slot)
		if err != nil {
			return err
		}
		return injectHeader(headers, injectedHeaders, attachment.HeaderSlot.Name, value)
	case attachment.QuerySlot != nil:
		value, err := resolveAuthSlot(state, authName, attachment.QuerySlot.Slot)
		if err != nil {
			return err
		}
		return injectQuery(query, injectedQuery, attachment.QuerySlot.Name, value)
	case attachment.FormSlot != nil:
		if hasBody {
			return fmt.Errorf("auth form field %q conflicts with request body", attachment.FormSlot.Name)
		}
		value, err := resolveAuthSlot(state, authName, attachment.FormSlot.Slot)
		if err != nil {
			return err
		}
		return injectForm(form, injectedForm, attachment.FormSlot.Name, value)
	default:
		return errors.New("auth attachment is invalid")
	}
}

func injectBasicAuth(
	headers map[string][]string,
	injected map[string]struct{},
	spec theater.HTTPBasicAuthSpec,
) error {
	credentials := spec.Username + ":" + spec.Password
	token := "Basic " + base64.StdEncoding.EncodeToString([]byte(credentials))
	return injectHeader(headers, injected, "Authorization", token)
}

func injectAPIKey(
	headers map[string][]string,
	query url.Values,
	injectedHeaders map[string]struct{},
	injectedQuery map[string]struct{},
	spec theater.HTTPAPIKeyAuthSpec,
) error {
	switch spec.In {
	case theater.HTTPAPIKeyInHeader:
		return injectHeader(headers, injectedHeaders, spec.Name, spec.Value)
	case theater.HTTPAPIKeyInQuery:
		return injectQuery(query, injectedQuery, spec.Name, spec.Value)
	default:
		return fmt.Errorf("auth api_key in %q is invalid", spec.In)
	}
}

func injectQuery(query url.Values, injected map[string]struct{}, name, value string) error {
	if _, exists := query[name]; exists {
		return fmt.Errorf("auth query parameter %q conflicts with request URL", name)
	}
	if _, exists := injected[name]; exists {
		return fmt.Errorf("auth query parameter %q is duplicated", name)
	}

	injected[name] = struct{}{}
	query.Set(name, value)
	return nil
}

func injectForm(form map[string]string, injected map[string]struct{}, name, value string) error {
	if form == nil {
		return fmt.Errorf("auth form field %q requires form-enabled request", name)
	}
	if _, exists := form[name]; exists {
		return fmt.Errorf("auth form field %q conflicts with request form", name)
	}
	if _, exists := injected[name]; exists {
		return fmt.Errorf("auth form field %q is duplicated", name)
	}

	form[name] = value
	injected[name] = struct{}{}
	return nil
}

func injectHeader(headers map[string][]string, injected map[string]struct{}, name, value string) error {
	canonical := http.CanonicalHeaderKey(name)
	if hasHeader(headers, canonical) {
		return fmt.Errorf("auth header %q conflicts with request headers", canonical)
	}
	if _, exists := injected[canonical]; exists {
		return fmt.Errorf("auth header %q is duplicated", canonical)
	}
	headers[canonical] = []string{value}
	injected[canonical] = struct{}{}
	return nil
}

func hasHeader(headers map[string][]string, name string) bool {
	for key := range headers {
		if http.CanonicalHeaderKey(key) == name {
			return true
		}
	}

	return false
}

func sessionSelection(session string, configured bool) (httpclient.SessionMode, string, error) {
	if !configured {
		return httpclient.SessionModeDefault, "", nil
	}
	if session == "" {
		return httpclient.SessionModeDefault, "", errors.New("session must not be empty")
	}
	if session == theater.HTTPSessionNone {
		return httpclient.SessionModeNone, "", nil
	}

	return httpclient.SessionModeNamed, session, nil
}

func authSelection(auth string, configured bool) (authMode, string, error) {
	if !configured {
		return authModeDefault, "", nil
	}
	if auth == "" {
		return authModeDefault, "", errors.New("auth must not be empty")
	}
	if auth == theater.HTTPAuthNone {
		return authModeNone, "", nil
	}

	return authModeNamed, auth, nil
}

func optionalBytes(value any, field string) ([]byte, error) {
	if value == nil {
		return nil, nil
	}

	return runtimevalue.Bytes(value, field)
}

func optionalStringMap(value any, field string) (map[string]string, error) {
	if value == nil {
		return map[string]string{}, nil
	}

	return runtimevalue.StringMap(value, field)
}

func optionalConfiguredString(args theater.Args, field string) (text string, configured bool, err error) {
	value, ok := args[field]
	if !ok || value == nil {
		return "", false, nil
	}

	text, err = stringValue(value, field)
	if err != nil {
		return "", false, err
	}

	return text, true, nil
}

func optionalDuration(value any, field string) (time.Duration, error) {
	if value == nil {
		return 0, nil
	}

	text, err := stringValue(value, field)
	if err != nil {
		return 0, err
	}

	duration, err := time.ParseDuration(text)
	if err != nil {
		return 0, fmt.Errorf("%s must be valid duration, got %q", field, text)
	}

	return duration, nil
}

func optionalString(value any, field, fallback string) (string, error) {
	if value == nil {
		return fallback, nil
	}

	return stringValue(value, field)
}

func stringSliceMap(value any, field string) (map[string][]string, error) {
	return runtimevalue.StringSliceMap(value, field)
}

func stringValue(value any, field string) (string, error) {
	return runtimevalue.String(value, field)
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}

	return cloned
}

func materializeForm(request *Request) error {
	if request == nil || len(request.Form) == 0 {
		return nil
	}
	if len(request.Body) != 0 {
		return errors.New("form is incompatible with body")
	}
	if request.Headers == nil {
		request.Headers = make(map[string][]string)
	}
	if err := ensureFormContentType(request.Headers); err != nil {
		return err
	}

	values := url.Values{}
	for key, value := range request.Form {
		values.Set(key, value)
	}
	request.Body = []byte(values.Encode())
	return nil
}

func materializeJSON(request *Request) error {
	if request == nil || request.JSON == nil {
		return nil
	}
	if len(request.Form) != 0 {
		return errors.New("json is incompatible with form")
	}
	if len(request.Body) != 0 {
		return errors.New("json is incompatible with body")
	}
	if request.Headers == nil {
		request.Headers = make(map[string][]string)
	}
	if err := ensureJSONContentType(request.Headers); err != nil {
		return err
	}

	encoded, err := json.Marshal(runtimevalue.Reveal(request.JSON))
	if err != nil {
		return fmt.Errorf("json must be encodable: %w", err)
	}

	request.Body = encoded
	return nil
}

func ensureFormContentType(headers map[string][]string) error {
	contentType, ok := firstHeaderValue(headers, "Content-Type")
	if !ok || strings.TrimSpace(contentType) == "" {
		headers["Content-Type"] = []string{"application/x-www-form-urlencoded"}
		return nil
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return errors.New(formContentTypeError)
	}
	if !strings.EqualFold(mediaType, "application/x-www-form-urlencoded") {
		return errors.New(formContentTypeError)
	}

	return nil
}

func ensureJSONContentType(headers map[string][]string) error {
	contentType, ok := firstHeaderValue(headers, "Content-Type")
	if !ok || strings.TrimSpace(contentType) == "" {
		headers["Content-Type"] = []string{"application/json"}
		return nil
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("json request body conflicts with Content-Type %q", contentType)
	}
	if !strings.EqualFold(mediaType, "application/json") {
		return fmt.Errorf("json request body conflicts with Content-Type %q", contentType)
	}

	return nil
}

func firstHeaderValue(headers map[string][]string, name string) (string, bool) {
	canonical := http.CanonicalHeaderKey(name)
	for key, values := range headers {
		if http.CanonicalHeaderKey(key) != canonical || len(values) == 0 {
			continue
		}

		return values[0], true
	}

	return "", false
}

func authStateFromResources(resources theater.ResourceScope) *authStateStore {
	if resources == nil {
		return nil
	}

	state, _ := resources.GetOrCreate(authStateScopeKey, func() any {
		return &authStateStore{slots: make(map[string]map[string]theater.Secret)}
	}).(*authStateStore)
	return state
}

func bearerToken(state *authStateStore, authName string, spec theater.HTTPBearerAuthSpec) (string, error) {
	if spec.TokenSlot == "" {
		if spec.Token == "" {
			return "", errors.New("bearer attachment token is required")
		}

		return spec.Token, nil
	}

	token, err := resolveAuthSlot(state, authName, spec.TokenSlot)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(token) == "" {
		return "", fmt.Errorf("auth slot %q must not be empty", spec.TokenSlot)
	}

	return token, nil
}

func resolveAuthSlot(state *authStateStore, authName, slot string) (string, error) {
	if state == nil {
		return "", fmt.Errorf("auth slot %q is not set", slot)
	}

	return state.slot(authName, slot)
}

func (s *authStateStore) slot(authName, slot string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	authSlots, ok := s.slots[authName]
	if !ok {
		return "", fmt.Errorf("auth slot %q is not set", slot)
	}

	value, ok := authSlots[slot]
	if !ok {
		return "", fmt.Errorf("auth slot %q is not set", slot)
	}

	text, err := runtimevalue.String(value, "auth slot "+slot)
	if err != nil {
		return "", err
	}

	return text, nil
}

func (s *authStateStore) store(authName string, values map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	authSlots, ok := s.slots[authName]
	if !ok {
		authSlots = make(map[string]theater.Secret)
		s.slots[authName] = authSlots
	}

	for slot, value := range values {
		authSlots[slot] = theater.NewSecret(value)
	}
}

func (s *authStateStore) storeValues(authName string, values theater.Values) {
	s.mu.Lock()
	defer s.mu.Unlock()

	authSlots, ok := s.slots[authName]
	if !ok {
		authSlots = make(map[string]theater.Secret)
		s.slots[authName] = authSlots
	}

	for slot, value := range values {
		authSlots[slot] = theater.NewSecret(value)
	}
}

func CaptureAuth(
	resources theater.ResourceScope,
	httpSpec *theater.HTTPSpec,
	capture theater.HTTPAuthCaptureSpec,
	response Response,
) error {
	if capture.Auth == "" {
		return errors.New("capture_auth auth is required")
	}
	if len(capture.Slots) == 0 {
		return errors.New("capture_auth must declare at least one slot")
	}

	authSpec, err := resolveNamedAuthSpec(httpSpec, capture.Auth)
	if err != nil {
		return err
	}

	state := authStateFromResources(resources)
	if state == nil {
		return errors.New("auth capture requires scenario-local resources")
	}

	values := make(map[string]string, len(capture.Slots))
	for slot, source := range capture.Slots {
		if !authSpecUsesSlot(authSpec, slot) {
			return fmt.Errorf("auth slot %q is not declared", slot)
		}

		value, err := captureSourceValue(response, source)
		if err != nil {
			return fmt.Errorf("capture auth slot %q failed: %w", slot, err)
		}

		values[slot] = value
	}

	state.store(capture.Auth, values)
	return nil
}

func authSpecUsesSlot(authSpec theater.HTTPAuthSpec, slot string) bool {
	for i := range authSpec.Attach {
		attachment := authSpec.Attach[i]
		switch {
		case attachment.Bearer != nil && attachment.Bearer.TokenSlot == slot:
			return true
		case attachment.HeaderSlot != nil && attachment.HeaderSlot.Slot == slot:
			return true
		case attachment.QuerySlot != nil && attachment.QuerySlot.Slot == slot:
			return true
		case attachment.FormSlot != nil && attachment.FormSlot.Slot == slot:
			return true
		}
	}

	return false
}

func captureSourceValue(response Response, source theater.HTTPCaptureSourceSpec) (string, error) {
	configured := 0
	if source.ResponseHeader != "" {
		configured++
	}
	if source.ResponseCookie != "" {
		configured++
	}
	if !source.JSONPointer.IsZero() {
		configured++
	}
	if source.FormField != "" {
		configured++
	}
	if configured != 1 {
		return "", errors.New("capture source must declare exactly one extractor")
	}

	switch {
	case source.ResponseHeader != "":
		value, ok := firstHeaderValue(response.Headers, source.ResponseHeader)
		if !ok {
			return "", fmt.Errorf("response header %q is missing", source.ResponseHeader)
		}
		return value, nil
	case source.ResponseCookie != "":
		httpResponse := &http.Response{Header: response.Headers.Clone()}
		for _, cookie := range httpResponse.Cookies() {
			if cookie.Name == source.ResponseCookie {
				return cookie.Value, nil
			}
		}
		return "", fmt.Errorf("response cookie %q is missing", source.ResponseCookie)
	case !source.JSONPointer.IsZero():
		value, err := selectvalue.Resolve(string(response.Body), theater.DecodeJSON, source.JSONPointer)
		if err != nil {
			return "", err
		}
		return runtimevalue.String(value, "captured JSON value")
	case source.FormField != "":
		values, err := url.ParseQuery(string(response.Body))
		if err != nil {
			return "", err
		}
		items, ok := values[source.FormField]
		if !ok || len(items) == 0 {
			return "", fmt.Errorf("response form field %q is missing", source.FormField)
		}
		return items[0], nil
	default:
		return "", errors.New("capture source is invalid")
	}
}
