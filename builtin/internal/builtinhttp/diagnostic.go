package builtinhttp

import (
	"encoding/json"
	"mime"
	"net/http"
	"net/url"
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
)

func newHTTPDiagnostic(request Request, response *httpclient.Response, duration time.Duration) theater.HTTPDiagnostic {
	diagnostic := theater.HTTPDiagnostic{
		Method:     request.Method,
		URL:        redactedDiagnosticURL(request.URL),
		DurationMs: duration.Milliseconds(),
	}
	if diagnostic.Method == "" {
		diagnostic.Method = defaultMethod
	}

	if response == nil {
		return diagnostic
	}

	diagnostic.StatusCode = response.StatusCode
	diagnostic.Status = diagnosticStatusText(response.StatusCode, response.Status)
	diagnostic.ResponseHeaders = diagnosticResponseHeaders(response.Header)
	diagnostic.ResponsePreview = diagnosticResponsePreview(response.Body, response.Header)
	return diagnostic
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
			segments[i] = httpDiagnosticRedactedValue
		}
	}

	return strings.Join(segments, "/")
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
			if isCredentialLikeValue(value) {
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
	return value
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

	truncated, _ := streamtext.TruncateSuffix(value, httpDiagnosticPreviewLimitBytes, "...")
	return truncated, true
}
