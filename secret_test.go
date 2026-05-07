package theater

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestNewSecretRedactsFmtAndJSON(t *testing.T) {
	t.Parallel()

	secret := NewSecret("issued-token")

	if got, want := secret.Reveal(), any("issued-token"); got != want {
		t.Fatalf("secret reveal mismatch: got %#v want %#v", got, want)
	}

	formatted := fmt.Sprintf("%v|%s|%q|%#v", secret, secret, secret, secret)
	if strings.Contains(formatted, "issued-token") {
		t.Fatalf("formatted secret leaked raw value: %q", formatted)
	}

	encoded, err := json.Marshal(map[string]any{"token": secret})
	if err != nil {
		t.Fatalf("marshal secret failed: %v", err)
	}

	jsonText := string(encoded)
	if strings.Contains(jsonText, "issued-token") {
		t.Fatalf("json secret leaked raw value: %q", jsonText)
	}

	if !strings.Contains(jsonText, redactedPreview) {
		t.Fatalf("json secret must use redacted placeholder: %q", jsonText)
	}
}

func TestFailureMessageRedactsSecretFormattedCause(t *testing.T) {
	t.Parallel()

	failure := Failure{
		Kind:    FailureKindAction,
		Phase:   PhaseRun,
		At:      "stage.main/call.login/act.submit/action",
		Summary: "request failed",
		Cause:   fmt.Errorf("authorization header=%v", NewSecret("Bearer issued-token")),
	}

	message := failure.Message()
	if strings.Contains(message, "issued-token") {
		t.Fatalf("failure message leaked secret: %q", message)
	}

	if !strings.Contains(message, redactedPreview) {
		t.Fatalf("failure message must use redacted placeholder: %q", message)
	}
}

func TestObserveValueRedactsWrappedSecretWithoutSecretSensitivity(t *testing.T) {
	t.Parallel()

	observed := observeValue("action.output.token", NewSecret("issued-token"), ValueContract{
		Kind:        ValueKindString,
		Sensitivity: SensitivityInternal,
		Capture:     CaptureSummary,
	})

	if observed.Preview == nil {
		t.Fatal("observed preview is required")
	}

	if !observed.Preview.Redacted {
		t.Fatal("wrapped secret preview must be redacted")
	}

	if got, want := observed.Preview.Text, redactedPreview; got != want {
		t.Fatalf("preview text mismatch: got %q want %q", got, want)
	}
}
