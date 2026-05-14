package report

import (
	"strings"
	"testing"
)

func TestRunDocumentValidateRejectsUnsupportedSchemaVersion(t *testing.T) {
	t.Parallel()

	document := RunDocument{
		SchemaVersion: "v9",
		Report: Report{
			StagePath: "stage.example",
			Status:    StatusPassed,
		},
	}

	err := document.Validate()
	if err == nil {
		t.Fatal("expected unsupported schema version error")
	}
	if !strings.Contains(err.Error(), "unsupported schema_version") {
		t.Fatalf("error mismatch: %v", err)
	}
}
