package builtinhttp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"mime"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/httpclient"
	"github.com/alex-poliushkin/theater/internal/secretvalue"
	"github.com/alex-poliushkin/theater/internal/streamtext"
)

const (
	httpDiagnosticPreviewKindBytes = "bytes"
	httpDiagnosticPreviewKindForm  = "form"
	httpDiagnosticPreviewKindJSON  = "json"
	httpDiagnosticPreviewKindText  = "text"

	httpDiagnosticOmittedBinary           = "binary"
	httpDiagnosticOmittedUnclassifiedText = "unclassified_text"
	httpDiagnosticPathSegmentMaxBytes     = 80
	httpDiagnosticPathSegmentID           = "id"
	httpDiagnosticPathSegmentOpaque       = "opaque"
	httpDiagnosticPathSegmentText         = "segment"
	httpDiagnosticPathSegmentUUID         = "uuid"
)

func newHTTPDiagnostic(request Request, response *httpclient.Response, duration time.Duration) theater.HTTPDiagnostic {
	return buildHTTPDiagnostic(request, response, duration, "")
}

func newHTTPDiagnosticForError(
	request Request,
	response *httpclient.Response,
	duration time.Duration,
	err error,
	fallback theater.HTTPDiagnosticFailureKind,
) theater.HTTPDiagnostic {
	failureKind := fallback
	if err != nil {
		failureKind = classifyHTTPDiagnosticFailure(err, fallback)
	}

	return buildHTTPDiagnostic(request, response, duration, failureKind)
}

func buildHTTPDiagnostic(
	request Request,
	response *httpclient.Response,
	duration time.Duration,
	failureKind theater.HTTPDiagnosticFailureKind,
) theater.HTTPDiagnostic {
	method := request.Method
	if method == "" {
		method = defaultMethod
	}
	redactedURL := redactedDiagnosticURL(request.URL)
	diagnostic := theater.HTTPDiagnostic{
		FailureKind:        failureKind,
		Method:             method,
		URL:                redactedURL,
		DurationMs:         duration.Milliseconds(),
		RequestFingerprint: diagnosticRequestFingerprint(request, method, redactedURL, duration),
	}

	if response == nil {
		return diagnostic
	}

	diagnostic.StatusCode = response.StatusCode
	diagnostic.Status = diagnosticStatusText(response.StatusCode, response.Status)
	diagnostic.ResponseHeaders = diagnosticResponseHeaders(response.Header)
	diagnostic.ResponsePreview = diagnosticResponsePreview(response.Body, response.Header)
	diagnostic.ResponseMetadata = diagnosticResponseMetadata(response, diagnostic.Status, diagnostic.ResponsePreview)
	return diagnostic
}

func classifyHTTPDiagnosticFailure(err error, fallback theater.HTTPDiagnosticFailureKind) theater.HTTPDiagnosticFailureKind {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return theater.HTTPDiagnosticFailureTimeout
	case isHTTPTimeoutError(err):
		return theater.HTTPDiagnosticFailureTimeout
	case isHTTPTLSError(err):
		return theater.HTTPDiagnosticFailureTLS
	case fallback != "":
		return fallback
	default:
		return theater.HTTPDiagnosticFailureNetwork
	}
}

func isHTTPTimeoutError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func isHTTPTLSError(err error) bool {
	var recordHeaderErr tls.RecordHeaderError
	if errors.As(err, &recordHeaderErr) {
		return true
	}

	var certVerificationErr *tls.CertificateVerificationError
	if errors.As(err, &certVerificationErr) {
		return true
	}

	var unknownAuthorityErr x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthorityErr) {
		return true
	}

	var hostnameErr x509.HostnameError
	if errors.As(err, &hostnameErr) {
		return true
	}

	var certificateInvalidErr x509.CertificateInvalidError
	return errors.As(err, &certificateInvalidErr)
}

func diagnosticRequestFingerprint(
	request Request,
	method string,
	redactedURL string,
	duration time.Duration,
) *theater.HTTPRequestFingerprint {
	fingerprint := &theater.HTTPRequestFingerprint{
		Method:     method,
		URL:        redactedURL,
		DurationMs: duration.Milliseconds(),
	}

	parsed, err := url.Parse(request.URL)
	if err != nil {
		return fingerprint
	}

	fingerprint.Host = parsed.Hostname()
	fingerprint.PathShape = redactedDiagnosticPath(parsed.EscapedPath())
	fingerprint.QueryKeys = diagnosticQueryKeys(parsed.Query())
	return fingerprint
}

func diagnosticQueryKeys(query url.Values) []string {
	if len(query) == 0 {
		return nil
	}

	rawKeys := make([]string, 0, len(query))
	for key := range query {
		rawKeys = append(rawKeys, key)
	}
	sort.Strings(rawKeys)

	seen := make(map[string]struct{}, len(rawKeys))
	keys := make([]string, 0, len(rawKeys))
	for _, key := range rawKeys {
		projected := diagnosticQueryKey(key)
		if _, ok := seen[projected]; ok {
			continue
		}
		seen[projected] = struct{}{}
		keys = append(keys, projected)
		if len(keys) == httpDiagnosticQueryKeyLimit {
			break
		}
	}
	return keys
}

func diagnosticQueryKey(key string) string {
	if isSensitiveDiagnosticName(key) || isCredentialLikeValue(key) || isPersonalLikeValue(key) {
		return httpDiagnosticRedactedValue
	}

	rendered := streamtext.Render([]byte(key))
	truncated, _ := streamtext.TruncateSuffix(rendered, httpDiagnosticQueryKeyMaxBytes-len("..."), "...")
	return truncated
}

func diagnosticResponseMetadata(
	response *httpclient.Response,
	status string,
	preview *theater.Preview,
) *theater.HTTPResponseMetadata {
	metadata := &theater.HTTPResponseMetadata{
		StatusCode:         response.StatusCode,
		Status:             status,
		ContentType:        diagnosticContentType(response.Header),
		ContentLengthBytes: int64(len(response.Body)),
	}
	if preview != nil {
		metadata.PreviewKind = preview.Kind
		metadata.PreviewOmittedReason = preview.OmittedReason
	}

	return metadata
}

func diagnosticStatusText(code int, status string) string {
	text := strings.TrimSpace(status)
	if code == 0 {
		return text
	}

	codeText := strconv.Itoa(code)
	switch {
	case text == codeText:
		return ""
	case strings.HasPrefix(text, codeText+" "):
		return strings.TrimSpace(strings.TrimPrefix(text, codeText))
	case text != "":
		return text
	default:
		return http.StatusText(code)
	}
}

func redactedDiagnosticURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return secretvalue.RedactedText
	}

	parsed.User = nil
	parsed.Fragment = ""
	parsed.RawFragment = ""
	if parsed.Opaque != "" {
		parsed.Opaque = httpDiagnosticRedactedValue
	}
	parsed.Path = redactedDiagnosticPath(parsed.EscapedPath())
	parsed.RawPath = ""

	query := parsed.Query()
	for key, values := range query {
		redacted := make([]string, len(values))
		for i := range redacted {
			redacted[i] = httpDiagnosticRedactedValue
		}
		query[key] = redacted
	}
	parsed.RawQuery = query.Encode()

	return parsed.String()
}

func redactedDiagnosticPath(path string) string {
	if path == "" {
		return ""
	}

	segments := strings.Split(path, "/")
	for i := range segments {
		if segments[i] != "" {
			segments[i] = diagnosticPathSegment(segments[i])
		}
	}

	return strings.Join(segments, "/")
}

func diagnosticPathSegment(escaped string) string {
	segment, err := url.PathUnescape(escaped)
	if err != nil {
		return httpDiagnosticRedactedValue
	}
	if !isSafeDiagnosticPathSegmentShape(segment) {
		return httpDiagnosticRedactedValue
	}
	if isSensitiveDiagnosticPathSegmentName(segment) ||
		isCredentialLikeValue(segment) ||
		isPersonalLikeValue(segment) {
		return httpDiagnosticRedactedValue
	}
	switch {
	case isGenericDiagnosticPathSegment(segment):
		return strings.ToLower(segment)
	case isAPIVersionPathSegment(segment):
		return strings.ToLower(segment)
	case isAllASCIIDigits(segment):
		return httpDiagnosticPathSegmentID
	case isUUIDLikePathSegment(segment):
		return httpDiagnosticPathSegmentUUID
	case isLongHexPathSegment(segment),
		isULIDLikePathSegment(segment),
		isOpaqueIdentifierPathSegment(segment),
		containsASCIIDigit(segment):
		return httpDiagnosticPathSegmentOpaque
	default:
		return httpDiagnosticPathSegmentText
	}
}

func isSafeDiagnosticPathSegmentShape(segment string) bool {
	if segment == "" || len(segment) > httpDiagnosticPathSegmentMaxBytes {
		return false
	}
	for _, r := range segment {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-',
			r == '_',
			r == '.',
			r == '~':
		default:
			return false
		}
	}
	return true
}

func isSensitiveDiagnosticPathSegmentName(segment string) bool {
	normalized := normalizedDiagnosticName(segment)
	switch {
	case normalized == "":
		return false
	case strings.Contains(normalized, "password"),
		strings.Contains(normalized, "secret"),
		strings.Contains(normalized, "credential"),
		strings.Contains(normalized, "authorization"),
		strings.Contains(normalized, "api_key"),
		strings.Contains(normalized, "apikey"),
		strings.Contains(normalized, "access_key"),
		strings.Contains(normalized, "private_key"),
		strings.Contains(normalized, "csrf"),
		strings.Contains(normalized, "cookie"):
		return true
	default:
		return false
	}
}

func isGenericDiagnosticPathSegment(segment string) bool {
	switch strings.ToLower(segment) {
	case "api", "apis", "rest", "rpc", "graphql":
		return true
	default:
		return false
	}
}

func isAPIVersionPathSegment(segment string) bool {
	normalized := strings.ToLower(segment)
	if len(normalized) < 2 || normalized[0] != 'v' {
		return false
	}
	return isAllASCIIDigits(normalized[1:])
}

func isAllASCIIDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isUUIDLikePathSegment(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, r := range strings.ToLower(value) {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !isLowerHexRune(r) {
				return false
			}
		}
	}
	return true
}

func isLongHexPathSegment(value string) bool {
	if len(value) < 12 {
		return false
	}
	for _, r := range strings.ToLower(value) {
		if !isLowerHexRune(r) {
			return false
		}
	}
	return true
}

func isULIDLikePathSegment(value string) bool {
	if len(value) != 26 {
		return false
	}
	for _, r := range strings.ToUpper(value) {
		if !strings.ContainsRune("0123456789ABCDEFGHJKMNPQRSTVWXYZ", r) {
			return false
		}
	}
	return true
}

func isOpaqueIdentifierPathSegment(value string) bool {
	if len(value) < 24 {
		return false
	}

	hasDigit := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-',
			r == '_':
			if r >= '0' && r <= '9' {
				hasDigit = true
			}
		default:
			return false
		}
	}
	return hasDigit
}

func containsASCIIDigit(value string) bool {
	for _, r := range value {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

func isLowerHexRune(r rune) bool {
	return r >= '0' && r <= '9' || r >= 'a' && r <= 'f'
}

func diagnosticResponseHeaders(headers http.Header) map[string][]string {
	if len(headers) == 0 {
		return nil
	}

	projected := make(map[string][]string)
	for name, values := range headers {
		key := strings.ToLower(name)
		if !isDiagnosticHeaderAllowed(key) {
			continue
		}

		for _, value := range values {
			if isCredentialLikeValue(value) || isPersonalLikeValue(value) {
				continue
			}
			projected[key] = append(projected[key], value)
		}
	}

	if len(projected) == 0 {
		return nil
	}
	return projected
}

func isDiagnosticHeaderAllowed(name string) bool {
	switch strings.ToLower(name) {
	case "content-type",
		"content-length",
		"x-request-id",
		"x-correlation-id",
		"request-id",
		"traceparent":
		return true
	default:
		return false
	}
}

func diagnosticResponsePreview(body []byte, headers http.Header) *theater.Preview {
	contentType := diagnosticContentType(headers)
	preview := &theater.Preview{
		Kind:        httpDiagnosticPreviewKindBytes,
		SizeHint:    int64(len(body)),
		ContentType: contentType,
	}

	if len(body) == 0 {
		return preview
	}

	mediaType := diagnosticMediaType(contentType)
	switch {
	case isJSONMediaType(mediaType):
		return diagnosticJSONPreview(body, preview)
	case mediaType == "application/x-www-form-urlencoded":
		return diagnosticFormPreview(body, preview)
	case isTextMediaType(mediaType) || utf8.Valid(body):
		preview.Kind = httpDiagnosticPreviewKindText
		preview.OmittedReason = httpDiagnosticOmittedUnclassifiedText
		return preview
	default:
		preview.OmittedReason = httpDiagnosticOmittedBinary
		return preview
	}
}

func diagnosticJSONPreview(body []byte, base *theater.Preview) *theater.Preview {
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		base.Kind = httpDiagnosticPreviewKindJSON
		base.OmittedReason = httpDiagnosticOmittedUnclassifiedText
		return base
	}

	sanitized, redacted := sanitizeDiagnosticValue(decoded, "")
	encoded, err := json.Marshal(sanitized)
	if err != nil {
		base.Kind = httpDiagnosticPreviewKindJSON
		base.OmittedReason = httpDiagnosticOmittedUnclassifiedText
		return base
	}

	text, truncated := truncateDiagnosticText(string(encoded))
	return &theater.Preview{
		Kind:        httpDiagnosticPreviewKindJSON,
		Text:        text,
		JSONValue:   sanitized,
		SizeHint:    int64(len(body)),
		Truncated:   truncated,
		Redacted:    redacted,
		ContentType: base.ContentType,
	}
}

func diagnosticFormPreview(body []byte, base *theater.Preview) *theater.Preview {
	values, err := url.ParseQuery(string(body))
	if err != nil {
		base.Kind = httpDiagnosticPreviewKindForm
		base.OmittedReason = httpDiagnosticOmittedUnclassifiedText
		return base
	}

	redacted := false
	for key, list := range values {
		if isSensitiveDiagnosticName(key) {
			for i := range list {
				list[i] = secretvalue.RedactedText
			}
			values[key] = list
			redacted = true
			continue
		}

		for i := range list {
			if isCredentialLikeValue(list[i]) || isPersonalLikeValue(list[i]) {
				list[i] = secretvalue.RedactedText
				redacted = true
			}
		}
		values[key] = list
	}

	text, truncated := truncateDiagnosticText(values.Encode())
	return &theater.Preview{
		Kind:        httpDiagnosticPreviewKindForm,
		Text:        text,
		SizeHint:    int64(len(body)),
		Truncated:   truncated,
		Redacted:    redacted,
		ContentType: base.ContentType,
	}
}

func diagnosticContentType(headers http.Header) string {
	value, ok := firstHeaderValue(headers, "Content-Type")
	if !ok {
		return ""
	}
	return safeDiagnosticContentType(value)
}

func safeDiagnosticContentType(value string) string {
	mediaType, params, err := mime.ParseMediaType(value)
	if err != nil {
		trimmed := strings.TrimSpace(value)
		if index := strings.Index(trimmed, ";"); index >= 0 {
			trimmed = strings.TrimSpace(trimmed[:index])
		}
		if isCredentialLikeValue(trimmed) || isPersonalLikeValue(trimmed) {
			return ""
		}
		return strings.ToLower(trimmed)
	}
	if isCredentialLikeValue(mediaType) || isPersonalLikeValue(mediaType) {
		return ""
	}

	safeParams := make(map[string]string)
	if charset, ok := params["charset"]; ok && !isCredentialLikeValue(charset) && !isPersonalLikeValue(charset) {
		safeParams["charset"] = charset
	}

	return mime.FormatMediaType(mediaType, safeParams)
}

func diagnosticMediaType(contentType string) string {
	if contentType == "" {
		return ""
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(contentType))
	}
	return strings.ToLower(mediaType)
}

func isJSONMediaType(mediaType string) bool {
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func isTextMediaType(mediaType string) bool {
	return strings.HasPrefix(mediaType, "text/")
}

func sanitizeDiagnosticValue(value any, name string) (any, bool) {
	if isSensitiveDiagnosticName(name) {
		return secretvalue.RedactedText, true
	}

	switch typed := value.(type) {
	case map[string]any:
		redacted := false
		sanitized := make(map[string]any, len(typed))
		for key, item := range typed {
			sanitizedItem, itemRedacted := sanitizeDiagnosticValue(item, key)
			sanitized[key] = sanitizedItem
			redacted = redacted || itemRedacted
		}
		return sanitized, redacted
	case []any:
		redacted := false
		sanitized := make([]any, len(typed))
		for i, item := range typed {
			sanitizedItem, itemRedacted := sanitizeDiagnosticValue(item, name)
			sanitized[i] = sanitizedItem
			redacted = redacted || itemRedacted
		}
		return sanitized, redacted
	case string:
		if isCredentialLikeValue(typed) || isPersonalLikeValue(typed) {
			return secretvalue.RedactedText, true
		}
		return typed, false
	default:
		return typed, false
	}
}

func isSensitiveDiagnosticName(name string) bool {
	normalized := normalizedDiagnosticName(name)
	switch {
	case normalized == "":
		return false
	case strings.Contains(normalized, "token"),
		strings.Contains(normalized, "password"),
		strings.Contains(normalized, "secret"),
		normalized == "key",
		strings.HasSuffix(normalized, "_key"),
		strings.Contains(normalized, "access_key"),
		strings.Contains(normalized, "private_key"),
		strings.Contains(normalized, "csrf"),
		strings.Contains(normalized, "cookie"),
		strings.Contains(normalized, "session"),
		strings.Contains(normalized, "credential"),
		strings.Contains(normalized, "authorization"),
		strings.Contains(normalized, "api_key"),
		strings.Contains(normalized, "apikey"),
		strings.Contains(normalized, "email"),
		strings.Contains(normalized, "phone"),
		strings.Contains(normalized, "ssn"),
		strings.Contains(normalized, "social_security"),
		strings.Contains(normalized, "birth"),
		strings.Contains(normalized, "dob"),
		strings.Contains(normalized, "name"),
		strings.Contains(normalized, "address"),
		strings.Contains(normalized, "passport"),
		strings.Contains(normalized, "national_id"),
		strings.Contains(normalized, "tax_id"),
		strings.Contains(normalized, "card"),
		strings.Contains(normalized, "account"),
		strings.Contains(normalized, "iban"),
		strings.Contains(normalized, "routing"):
		return true
	default:
		return false
	}
}

func normalizedDiagnosticName(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	normalized = strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(normalized)
	return normalized
}

func isCredentialLikeValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	normalized := strings.ToLower(trimmed)
	switch {
	case normalized == "":
		return false
	case strings.Contains(normalized, "bearer "),
		strings.Contains(normalized, "basic "),
		strings.Contains(normalized, "token="),
		strings.Contains(normalized, "password="),
		strings.Contains(normalized, "secret="),
		strings.Contains(normalized, "session="),
		strings.Contains(normalized, "cookie="),
		strings.Contains(normalized, "sk_live_"),
		strings.Contains(normalized, "sk_test_"),
		strings.Contains(normalized, "sk-proj-"),
		strings.Contains(trimmed, "AKIA"),
		strings.Contains(trimmed, "-----BEGIN "),
		strings.Count(trimmed, ".") >= 2 && strings.Contains(trimmed, "eyJ"):
		return true
	default:
		return false
	}
}

func isPersonalLikeValue(value string) bool {
	return strings.Contains(value, "@") || containsSSNLikeValue(value)
}

func containsSSNLikeValue(value string) bool {
	for i := 0; i+11 <= len(value); i++ {
		if isSSNLikeWindow(value[i : i+11]) {
			return true
		}
	}

	return false
}

func isSSNLikeWindow(value string) bool {
	if value[3] != '-' || value[6] != '-' {
		return false
	}
	for i, r := range value {
		if i == 3 || i == 6 {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func truncateDiagnosticText(value string) (string, bool) {
	if len(value) <= httpDiagnosticPreviewLimitBytes {
		return value, false
	}

	truncated, _ := streamtext.TruncateSuffix(value, httpDiagnosticPreviewLimitBytes-len("..."), "...")
	return truncated, true
}
