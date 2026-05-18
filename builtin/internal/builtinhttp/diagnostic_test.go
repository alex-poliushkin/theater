package builtinhttp

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/httpclient"
)

func TestHTTPDiagnosticBuildsSafeRequestFingerprint(t *testing.T) {
	t.Parallel()

	diagnostic := newHTTPDiagnostic(Request{
		Method: http.MethodPost,
		URL:    "https://user:pass@api.example.test:9443/v1/users/123?token=query-secret&search=widgets&api_key=issued#fragment",
	}, nil, 1500*time.Millisecond)

	wantURL := "https://api.example.test:9443/redacted/redacted/redacted?api_key=redacted&search=redacted&token=redacted"
	if got := diagnostic.URL; got != wantURL {
		t.Fatalf("diagnostic url mismatch: got %q want %q", got, wantURL)
	}
	if diagnostic.RequestFingerprint == nil {
		t.Fatal("request fingerprint is missing")
	}
	if got := diagnostic.RequestFingerprint; !reflect.DeepEqual(got, &theater.HTTPRequestFingerprint{
		Method:     http.MethodPost,
		URL:        wantURL,
		Host:       "api.example.test",
		PathShape:  "/redacted/redacted/redacted",
		QueryKeys:  []string{"redacted", "search"},
		DurationMs: 1500,
	}) {
		t.Fatalf("request fingerprint mismatch: got %#v", got)
	}

	rendered := diagnostic.URL + " " + diagnostic.RequestFingerprint.URL + " " + diagnostic.RequestFingerprint.PathShape
	for _, forbidden := range []string{"user", "pass", "query-secret", "widgets", "issued", "123", "fragment"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("diagnostic leaked %q: %#v", forbidden, diagnostic)
		}
	}
}

func TestHTTPDiagnosticBuildsSafeResponseMetadata(t *testing.T) {
	t.Parallel()

	body := []byte(`{"message":"retry later","token":"credential-secret","email":"person@example.test"}`)
	diagnostic := newHTTPDiagnostic(
		Request{URL: "https://api.example.test/probe"},
		&httpclient.Response{
			StatusCode: http.StatusBadGateway,
			Status:     "502 Bad Gateway",
			Header: http.Header{
				"Authorization":  {"Bearer credential-secret"},
				"Content-Length": {"96"},
				"Content-Type":   {"application/json; token=credential-secret"},
				"Cookie":         {"sid=credential-secret"},
				"Set-Cookie":     {"sid=credential-secret; Path=/"},
				"X-Request-Id":   {"req-123"},
			},
			Body: body,
		},
		15*time.Millisecond,
	)

	if diagnostic.ResponseMetadata == nil {
		t.Fatal("response metadata is missing")
	}
	if got := diagnostic.ResponseMetadata; !reflect.DeepEqual(got, &theater.HTTPResponseMetadata{
		StatusCode:         http.StatusBadGateway,
		Status:             "Bad Gateway",
		ContentType:        "application/json",
		ContentLengthBytes: int64(len(body)),
		PreviewKind:        "json",
	}) {
		t.Fatalf("response metadata mismatch: got %#v", got)
	}
	if got := diagnostic.ResponseHeaders; !reflect.DeepEqual(got, map[string][]string{
		"content-length": {"96"},
		"x-request-id":   {"req-123"},
	}) {
		t.Fatalf("response headers mismatch: got %#v", got)
	}

	if diagnostic.ResponsePreview == nil || !diagnostic.ResponsePreview.Redacted {
		t.Fatalf("response preview should be redacted: %#v", diagnostic.ResponsePreview)
	}
	rendered := diagnostic.ResponseMetadata.ContentType + " " + diagnostic.ResponsePreview.ContentType + " " + diagnostic.ResponsePreview.Text
	for _, forbidden := range []string{"credential-secret", "person@example.test", "Bearer", "Set-Cookie"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("response preview leaked %q: %q", forbidden, rendered)
		}
	}
}

func TestHTTPDiagnosticPreviewClassifiesBodiesSafely(t *testing.T) {
	t.Parallel()

	jsonBody := []byte(`{"message":"` + strings.Repeat("a", httpDiagnosticPreviewLimitBytes+128) + `"}`)
	jsonDiagnostic := newHTTPDiagnostic(
		Request{URL: "https://api.example.test/probe"},
		&httpclient.Response{Header: http.Header{"Content-Type": {"application/json"}}, Body: jsonBody},
		time.Millisecond,
	)
	if got := jsonDiagnostic.ResponsePreview; got == nil || got.Kind != "json" || !got.Truncated || len(got.Text) > httpDiagnosticPreviewLimitBytes {
		t.Fatalf("json preview should be bounded and truncated: %#v", got)
	}

	binaryDiagnostic := newHTTPDiagnostic(
		Request{URL: "https://api.example.test/probe"},
		&httpclient.Response{Header: http.Header{"Content-Type": {"application/octet-stream"}}, Body: []byte{0xff, 0x00, 0x01}},
		time.Millisecond,
	)
	if got := binaryDiagnostic.ResponsePreview; got == nil || got.Kind != "bytes" || got.OmittedReason != "binary" || got.Text != "" {
		t.Fatalf("binary preview should be omitted: %#v", got)
	}

	textDiagnostic := newHTTPDiagnostic(
		Request{URL: "https://api.example.test/probe"},
		&httpclient.Response{
			Header: http.Header{"Content-Type": {"text/plain; boundary=person@example.test"}},
			Body:   []byte("plain textual body"),
		},
		time.Millisecond,
	)
	if got := textDiagnostic.ResponsePreview; got == nil || got.Kind != "text" || got.OmittedReason != "unclassified_text" || got.Text != "" {
		t.Fatalf("unclassified text preview should be omitted: %#v", got)
	}
	if got, want := textDiagnostic.ResponseMetadata.ContentType, "text/plain"; got != want {
		t.Fatalf("content-type metadata mismatch: got %q want %q", got, want)
	}
	if got := textDiagnostic.ResponseHeaders["content-type"]; len(got) != 0 {
		t.Fatalf("personal content-type header value must be omitted, got %#v", got)
	}
}

func TestHTTPDiagnosticClassifiesTransportFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		err  error
		want theater.HTTPDiagnosticFailureKind
	}{
		{
			name: "context deadline",
			err:  context.DeadlineExceeded,
			want: theater.HTTPDiagnosticFailureTimeout,
		},
		{
			name: "url timeout",
			err:  &url.Error{Op: "Get", URL: "https://api.example.test", Err: timeoutError{}},
			want: theater.HTTPDiagnosticFailureTimeout,
		},
		{
			name: "tls",
			err:  tls.RecordHeaderError{},
			want: theater.HTTPDiagnosticFailureTLS,
		},
		{
			name: "network",
			err:  errors.New("dial tcp: connection refused"),
			want: theater.HTTPDiagnosticFailureNetwork,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			diagnostic := newHTTPDiagnosticForError(
				Request{URL: "https://api.example.test/probe"},
				nil,
				time.Millisecond,
				tc.err,
				theater.HTTPDiagnosticFailureNetwork,
			)
			if got := diagnostic.FailureKind; got != tc.want {
				t.Fatalf("failure kind mismatch: got %q want %q", got, tc.want)
			}
		})
	}
}

type timeoutError struct{}

func (timeoutError) Error() string {
	return "timeout"
}

func (timeoutError) Timeout() bool {
	return true
}
