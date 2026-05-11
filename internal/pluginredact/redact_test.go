package pluginredact

import (
	"testing"

	"github.com/alex-poliushkin/theater/internal/secretvalue"
)

func TestNewCollectsStringMaps(t *testing.T) {
	t.Parallel()

	redactor := New(map[string]string{
		"THEATER_TOKEN": "host-secret-value",
	})

	if got, want := redactor.RedactText("token=host-secret-value"), "token=[redacted]"; got != want {
		t.Fatalf("redacted text mismatch: got %q want %q", got, want)
	}
}

func TestRootPointersProtectAndCollectScalarValues(t *testing.T) {
	t.Parallel()

	values, err := StringsAtPointers("123456", []string{""})
	if err != nil {
		t.Fatalf("collect root scalar strings: %v", err)
	}
	if got, want := len(values), 1; got != want {
		t.Fatalf("root scalar strings length mismatch: got %d want %d", got, want)
	}
	if got, want := values[0], "123456"; got != want {
		t.Fatalf("root scalar string mismatch: got %q want %q", got, want)
	}

	protected, err := ProtectPointers("123456", []string{""})
	if err != nil {
		t.Fatalf("protect root scalar: %v", err)
	}
	protectedSecret, ok := protected.(secretvalue.Value)
	if !ok {
		t.Fatalf("root scalar protection type mismatch: got %T", protected)
	}
	if got, want := protectedSecret.Reveal(), any("123456"); got != want {
		t.Fatalf("root scalar protection value mismatch: got %q want %q", got, want)
	}
}

func TestRootPointersCollectNonStringScalarValues(t *testing.T) {
	t.Parallel()

	values, err := StringsAtPointers(map[string]any{
		"code":  123456,
		"valid": true,
	}, []string{""})
	if err != nil {
		t.Fatalf("collect root scalar values: %v", err)
	}

	redactor := FromStrings(values)
	if got, want := redactor.RedactText("code=123456 valid=true"), "code=[redacted] valid=[redacted]"; got != want {
		t.Fatalf("redacted text mismatch: got %q want %q", got, want)
	}
}
