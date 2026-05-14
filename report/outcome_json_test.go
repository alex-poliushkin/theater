package report

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestFailureJSONOmitsRawCause(t *testing.T) {
	t.Parallel()

	failure := Failure{
		Kind:    FailureKindExpectation,
		Phase:   PhaseRun,
		At:      "stage.example/call.run/act.check/expectation.status",
		Summary: "expectation failed",
		Cause:   errors.New("actual secret-token does not equal expected ok"),
	}

	data, err := json.Marshal(failure)
	if err != nil {
		t.Fatalf("marshal failure failed: %v", err)
	}
	if strings.Contains(string(data), "secret-token") || strings.Contains(string(data), `"cause"`) {
		t.Fatalf("failure JSON must not include raw cause: %s", data)
	}
}

func TestFailureJSONIgnoresLegacyCauseShapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "string",
			raw:  `{"kind":"expectation","phase":"run","at":"stage.example","summary":"failed","cause":"raw cause"}`,
		},
		{
			name: "object",
			raw:  `{"kind":"expectation","phase":"run","at":"stage.example","summary":"failed","cause":{}}`,
		},
		{
			name: "null",
			raw:  `{"kind":"expectation","phase":"run","at":"stage.example","summary":"failed","cause":null}`,
		},
		{
			name: "missing",
			raw:  `{"kind":"expectation","phase":"run","at":"stage.example","summary":"failed"}`,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var failure Failure
			if err := json.Unmarshal([]byte(test.raw), &failure); err != nil {
				t.Fatalf("unmarshal failure failed: %v", err)
			}
			if failure.Cause != nil {
				t.Fatalf("failure cause must not be restored from saved JSON: %v", failure.Cause)
			}
			if got, want := failure.Summary, "failed"; got != want {
				t.Fatalf("summary mismatch: got %q want %q", got, want)
			}
		})
	}
}
