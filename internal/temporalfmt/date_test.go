package temporalfmt

import (
	"strings"
	"testing"
	"time"
)

func TestFormatDateUsesUTCNamedFormats(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, time.March, 28, 0, 30, 0, 0, time.FixedZone("fixture", 2*60*60))

	tests := []struct {
		name   string
		format string
		want   string
	}{
		{name: "iso", format: DateFormatISO, want: "2026-03-27"},
		{name: "basic", format: DateFormatBasic, want: "20260327"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := FormatDate(base, test.format)
			if err != nil {
				t.Fatalf("format date failed: %v", err)
			}
			if got != test.want {
				t.Fatalf("date mismatch: got %q want %q", got, test.want)
			}
		})
	}
}

func TestValidateDateFormatRejectsUnsupportedFormat(t *testing.T) {
	t.Parallel()

	err := ValidateDateFormat("rfc3339")
	if err == nil {
		t.Fatal("expected unsupported format error")
	}
	if got, want := err.Error(), `format "rfc3339" is not supported`; !strings.Contains(got, want) {
		t.Fatalf("format error mismatch: got %q want substring %q", got, want)
	}
}
