package pluginhost

import (
	"testing"

	"github.com/alex-poliushkin/theater/plugin/protocol"
)

func TestCallErrorRedactProtectsPartialOutputs(t *testing.T) {
	t.Parallel()

	callErr := &CallError{Response: protocol.ResponseError{
		Message: "failed with host-secret-value",
		Data: protocol.ErrorData{
			PartialOutputs: map[string]any{
				"echo": "host-secret-value",
				"nested": map[string]any{
					"token": "host-secret-value",
				},
				"host-secret-value": "key-secret",
				"items":             []any{"host-secret-value"},
			},
		},
	}}

	redacted := callErr.Redact(func(text string) string {
		if text == "host-secret-value" {
			return "[redacted]"
		}
		if text == "failed with host-secret-value" {
			return "failed with [redacted]"
		}
		return text
	})

	if got, want := redacted.Error(), "failed with [redacted]"; got != want {
		t.Fatalf("message mismatch: got %q want %q", got, want)
	}
	outputs := redacted.PartialOutputs()
	if got, want := outputs["echo"], "[redacted]"; got != want {
		t.Fatalf("partial output mismatch: got %#v want %#v", got, want)
	}
	nested := outputs["nested"].(map[string]any)
	if got, want := nested["token"], "[redacted]"; got != want {
		t.Fatalf("nested partial output mismatch: got %#v want %#v", got, want)
	}
	if got, want := outputs["[redacted]"], "key-secret"; got != want {
		t.Fatalf("partial output key mismatch: got %#v want %#v", got, want)
	}
	items := outputs["items"].([]any)
	if got, want := items[0], "[redacted]"; got != want {
		t.Fatalf("list partial output mismatch: got %#v want %#v", got, want)
	}
}
