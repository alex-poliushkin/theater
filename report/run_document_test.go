package report

import (
	"strings"
	"testing"
)

func TestRunDocumentValidateRejectsUnsupportedSchemaVersion(t *testing.T) {
	t.Parallel()

	document := RunDocument{
		ReportSchemaVersion: "v9",
		TheaterVersion:      "dev",
		RunID:               "run-1",
		Report: Report{
			StagePath: "stage.example",
			Status:    StatusPassed,
		},
	}

	err := document.Validate()
	if err == nil {
		t.Fatal("expected unsupported schema version error")
	}
	if !strings.Contains(err.Error(), "unsupported report_schema_version") {
		t.Fatalf("error mismatch: %v", err)
	}
}

func TestRunDocumentValidateRequiresIdentity(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		document RunDocument
		want     string
	}{
		{
			name: "report schema version",
			document: RunDocument{
				TheaterVersion: "dev",
				RunID:          "run-1",
				Report: Report{
					StagePath: "stage.example",
					Status:    StatusPassed,
				},
			},
			want: "report_schema_version is required",
		},
		{
			name: "theater version",
			document: RunDocument{
				ReportSchemaVersion: RunDocumentSchemaVersion,
				RunID:               "run-1",
				Report: Report{
					StagePath: "stage.example",
					Status:    StatusPassed,
				},
			},
			want: "theater_version is required",
		},
		{
			name: "run id",
			document: RunDocument{
				ReportSchemaVersion: RunDocumentSchemaVersion,
				TheaterVersion:      "dev",
				Report: Report{
					StagePath: "stage.example",
					Status:    StatusPassed,
				},
			},
			want: "run_id is required",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := testCase.document.Validate()
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error mismatch: got %q want fragment %q", err, testCase.want)
			}
		})
	}
}
