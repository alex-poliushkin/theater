package theater

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

func TestStatusTerminalBehavior(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		status   Status
		valid    bool
		terminal bool
	}{
		{name: "pending", status: StatusPending, valid: true},
		{name: "running", status: StatusRunning, valid: true},
		{name: "passed", status: StatusPassed, valid: true, terminal: true},
		{name: "failed", status: StatusFailed, valid: true, terminal: true},
		{name: "canceled", status: StatusCanceled, valid: true, terminal: true},
		{name: "skipped", status: StatusSkipped, valid: true, terminal: true},
		{name: "unknown", status: Status("unknown")},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.status.Valid(); got != testCase.valid {
				t.Fatalf("status validity mismatch: got %v want %v", got, testCase.valid)
			}

			if got := testCase.status.IsTerminal(); got != testCase.terminal {
				t.Fatalf("status terminal mismatch: got %v want %v", got, testCase.terminal)
			}
		})
	}
}

func TestFailureValidate(t *testing.T) {
	t.Parallel()

	failure := Failure{
		Kind:    FailureKindAction,
		Phase:   PhaseRun,
		At:      "stage.main/scenario.login/act.submit",
		Summary: "request failed",
		Cause:   errors.New("someErr"),
	}

	if err := failure.Validate(); err != nil {
		t.Fatalf("failure validation failed: %v", err)
	}

	if got, want := failure.Message(), "request failed: someErr"; got != want {
		t.Fatalf("failure error mismatch: got %q want %q", got, want)
	}
}

func TestValidateTerminalOutcome(t *testing.T) {
	t.Parallel()

	validFailure := &Failure{
		Kind:    FailureKindDefinition,
		Phase:   PhaseValidate,
		At:      "stage.main",
		Summary: "missing scenario ref",
	}

	testCases := []struct {
		name    string
		status  Status
		failure *Failure
		wantErr bool
	}{
		{name: "failed requires failure", status: StatusFailed, wantErr: true},
		{name: "failed with failure", status: StatusFailed, failure: validFailure},
		{name: "canceled with failure", status: StatusCanceled, failure: validFailure, wantErr: true},
		{name: "passed with failure", status: StatusPassed, failure: validFailure, wantErr: true},
		{name: "passed without failure", status: StatusPassed},
		{name: "running is not terminal", status: StatusRunning, wantErr: true},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateTerminalOutcome(testCase.status, testCase.failure)
			if testCase.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}

			if !testCase.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestPayloadMetadataValidate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		meta    PayloadMetadata
		wantErr bool
	}{
		{
			name: "artifact ref capture",
			meta: PayloadMetadata{
				Sensitivity: SensitivitySecret,
				Capture:     CaptureArtifactRef,
				ArtifactRef: "artifact://run/1",
			},
		},
		{
			name: "summary capture",
			meta: PayloadMetadata{
				Sensitivity: SensitivityInternal,
				Capture:     CaptureSummary,
			},
		},
		{
			name: "artifact ref missing",
			meta: PayloadMetadata{
				Sensitivity: SensitivitySecret,
				Capture:     CaptureArtifactRef,
			},
			wantErr: true,
		},
		{
			name: "artifact ref unexpected",
			meta: PayloadMetadata{
				Sensitivity: SensitivityPersonal,
				Capture:     CaptureSummary,
				ArtifactRef: "artifact://run/2",
			},
			wantErr: true,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := testCase.meta.Validate()
			if testCase.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}

			if !testCase.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestEventAndReportValidate(t *testing.T) {
	t.Parallel()

	failure := &Failure{
		Kind:    FailureKindTimeout,
		Phase:   PhaseRun,
		At:      "stage.main/scenario.confirm/act.wait",
		Summary: "request timed out",
	}

	event := Event{
		Kind:        "act.finished",
		Status:      StatusFailed,
		Failure:     failure,
		Attempt:     2,
		ScenarioSeq: 5,
		Payload: &PayloadMetadata{
			Sensitivity: SensitivitySecret,
			Capture:     CaptureArtifactRef,
			ArtifactRef: "artifact://run/3",
		},
	}

	if err := event.Validate(); err != nil {
		t.Fatalf("event validation failed: %v", err)
	}

	report := Report{
		StagePath: "stage.main",
		Status:    StatusFailed,
		Failure:   failure,
		Summary: Summary{
			TotalScenarios:  2,
			FailedScenarios: 1,
			PassedScenarios: 1,
		},
	}

	if err := report.Validate(); err != nil {
		t.Fatalf("report validation failed: %v", err)
	}
}

func TestLogEventAndReportValidationRejectInvalidShapes(t *testing.T) {
	t.Parallel()

	failure := &Failure{
		Kind:    FailureKindObservation,
		Phase:   PhaseRun,
		At:      "stage.main/call.login/act.submit/log.response",
		Summary: "log evaluation failed",
		Cause:   errors.New("missing field"),
	}
	validLogEvent := func() Event {
		address := &NodeAddress{
			ScenarioCallPath: "stage.main/call.login",
			ActID:            "submit",
			Kind:             NodeKindLog,
			NodeRef:          "response",
			Phase:            "log.evaluate",
			AttemptIndex:     1,
		}

		return Event{
			Kind:           EventKindLogEmitted,
			StageID:        "main",
			StagePath:      "stage.main",
			ScenarioID:     "login",
			ScenarioCallID: "login",
			ScenarioPath:   "stage.main/call.login",
			Path:           "stage.main/call.login/act.submit/log.response",
			Address:        address,
			Attempt:        1,
			ScenarioSeq:    1,
			Status:         StatusPassed,
			Log: &LogRecord{
				ID:             "response",
				Path:           "stage.main/call.login/act.submit/log.response",
				StageID:        "main",
				ScenarioID:     "login",
				ScenarioCallID: "login",
				ScenarioPath:   "stage.main/call.login",
				ActID:          "submit",
				Attempt:        1,
				ScenarioSeq:    1,
				Status:         LogStatusEmitted,
				Address:        address,
			},
		}
	}

	testCases := []struct {
		name   string
		event  func() Event
		report func() Report
	}{
		{
			name: "log event missing log record",
			event: func() Event {
				event := validLogEvent()
				event.Log = nil
				return event
			},
		},
		{
			name: "non-log event carries log record",
			event: func() Event {
				event := validLogEvent()
				event.Kind = EventKindActFinished
				return event
			},
		},
		{
			name: "log status invalid",
			event: func() Event {
				event := validLogEvent()
				event.Log.Status = LogStatus("unknown")
				return event
			},
		},
		{
			name: "error log missing failure",
			event: func() Event {
				event := validLogEvent()
				event.Status = StatusFailed
				event.Failure = failure
				event.Log.Status = LogStatusError
				return event
			},
		},
		{
			name: "emitted log carries failure",
			event: func() Event {
				event := validLogEvent()
				event.Log.Failure = failure
				return event
			},
		},
		{
			name: "log event path mismatches record",
			event: func() Event {
				event := validLogEvent()
				event.Log.Path = "stage.main/call.login/act.submit/log.other"
				return event
			},
		},
		{
			name: "log cannot validate as node report",
			report: func() Report {
				return Report{
					StagePath: "stage.main",
					Status:    StatusPassed,
					Nodes: []NodeReport{{
						Kind:   NodeKindLog,
						Path:   "stage.main/call.login/act.submit/log.response",
						Status: StatusPassed,
					}},
				}
			},
		},
		{
			name: "log record address mismatches identity",
			report: func() Report {
				event := validLogEvent()
				log := *event.Log
				address := *log.Address
				address.NodeRef = "other"
				log.Address = &address
				return Report{
					StagePath: "stage.main",
					Status:    StatusPassed,
					Logs:      []LogRecord{log},
				}
			},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			switch {
			case testCase.event != nil:
				if err := testCase.event().Validate(); err == nil {
					t.Fatal("expected event validation error")
				}
			case testCase.report != nil:
				if err := testCase.report().Validate(); err == nil {
					t.Fatal("expected report validation error")
				}
			}
		})
	}
}

func TestReportLogSummaryValidation(t *testing.T) {
	t.Parallel()

	validLog := validReportLogRecord()
	validSummary := func(records int) *LogSummary {
		return &LogSummary{
			Records:           records,
			PreviewLimitBytes: DefaultScenarioLogPreviewLimitBytes,
			PerActLimit:       DefaultScenarioLogRecordsPerAct,
			PerRunLimit:       DefaultScenarioLogRecordsPerRun,
		}
	}
	reportWith := func(logs []LogRecord, summary *LogSummary) Report {
		return Report{
			StagePath:  "stage.main",
			Status:     StatusPassed,
			Logs:       logs,
			LogSummary: summary,
		}
	}

	validDroppedOnly := validSummary(0)
	validDroppedOnly.DroppedRecords = 1
	if err := reportWith(nil, validDroppedOnly).Validate(); err != nil {
		t.Fatalf("dropped-only log summary validation failed: %v", err)
	}

	testCases := []struct {
		name    string
		report  Report
		wantErr string
	}{
		{
			name:    "logs require summary",
			report:  reportWith([]LogRecord{validLog}, nil),
			wantErr: "report logs require log summary",
		},
		{
			name: "summary records must match retained logs",
			report: func() Report {
				summary := validSummary(1)
				summary.Records = 2
				return reportWith([]LogRecord{validLog}, summary)
			}(),
			wantErr: "log records 2 does not match report logs 1",
		},
		{
			name: "summary records cannot be negative",
			report: func() Report {
				summary := validSummary(1)
				summary.Records = -1
				return reportWith([]LogRecord{validLog}, summary)
			}(),
			wantErr: "log records -1 is invalid",
		},
		{
			name: "summary dropped records cannot be negative",
			report: func() Report {
				summary := validSummary(1)
				summary.DroppedRecords = -1
				return reportWith([]LogRecord{validLog}, summary)
			}(),
			wantErr: "log dropped records -1 is invalid",
		},
		{
			name: "summary truncated records cannot be negative",
			report: func() Report {
				summary := validSummary(1)
				summary.TruncatedRecords = -1
				return reportWith([]LogRecord{validLog}, summary)
			}(),
			wantErr: "log truncated records -1 is invalid",
		},
		{
			name: "preview limit must be positive",
			report: func() Report {
				summary := validSummary(1)
				summary.PreviewLimitBytes = 0
				return reportWith([]LogRecord{validLog}, summary)
			}(),
			wantErr: "log preview limit 0 is invalid",
		},
		{
			name: "per-act limit must be positive",
			report: func() Report {
				summary := validSummary(1)
				summary.PerActLimit = 0
				return reportWith([]LogRecord{validLog}, summary)
			}(),
			wantErr: "log per-act limit 0 is invalid",
		},
		{
			name: "per-run limit must be positive",
			report: func() Report {
				summary := validSummary(1)
				summary.PerRunLimit = 0
				return reportWith([]LogRecord{validLog}, summary)
			}(),
			wantErr: "log per-run limit 0 is invalid",
		},
		{
			name: "retained logs cannot exceed per-run limit",
			report: func() Report {
				logs := validReportLogRecords(DefaultScenarioLogRecordsPerRun+1, func(i int) string {
					return "act-" + strconv.Itoa(i)
				})
				return reportWith(logs, validSummary(len(logs)))
			}(),
			wantErr: "exceeds per-run limit",
		},
		{
			name: "retained logs cannot exceed per-act limit",
			report: func() Report {
				logs := validReportLogRecords(DefaultScenarioLogRecordsPerAct+1, func(int) string {
					return "submit"
				})
				return reportWith(logs, validSummary(len(logs)))
			}(),
			wantErr: "exceed per-act limit",
		},
		{
			name: "summary truncated records must match retained logs",
			report: func() Report {
				log := validLog
				log.Truncated = true
				return reportWith([]LogRecord{log}, validSummary(1))
			}(),
			wantErr: "log truncated records 0 does not match report logs 1",
		},
		{
			name: "preview text cannot exceed preview limit",
			report: func() Report {
				log := validLog
				log.Preview = &Preview{Kind: "string", Text: strings.Repeat("x", DefaultScenarioLogPreviewLimitBytes+1)}
				return reportWith([]LogRecord{log}, validSummary(1))
			}(),
			wantErr: "preview exceeds preview limit",
		},
		{
			name: "preview json value cannot be retained",
			report: func() Report {
				log := validLog
				log.Preview = &Preview{Kind: "object", JSONValue: map[string]any{"payload": "value"}}
				return reportWith([]LogRecord{log}, validSummary(1))
			}(),
			wantErr: "preview must not carry json_value",
		},
		{
			name: "dropped records cannot be retained in report logs",
			report: func() Report {
				log := validLog
				log.Dropped = true
				return reportWith([]LogRecord{log}, validSummary(1))
			}(),
			wantErr: "dropped records belong in log summary",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := testCase.report.Validate()
			if err == nil {
				t.Fatal("expected report validation error")
			}
			if !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("validation error mismatch: got %q want substring %q", err.Error(), testCase.wantErr)
			}
		})
	}
}

func TestEventValidateAllowsNonTerminalStatusWithoutFailure(t *testing.T) {
	t.Parallel()

	event := Event{
		Kind:        "act.running",
		Status:      StatusRunning,
		Attempt:     1,
		ScenarioSeq: 1,
	}

	if err := event.Validate(); err != nil {
		t.Fatalf("event validation failed: %v", err)
	}
}

func validReportLogRecord() LogRecord {
	address := &NodeAddress{
		ScenarioCallPath: "stage.main/call.login",
		ActID:            "submit",
		Kind:             NodeKindLog,
		NodeRef:          "response",
		Phase:            "log.evaluate",
		AttemptIndex:     1,
	}

	return LogRecord{
		ID:             "response",
		Path:           "stage.main/call.login/act.submit/log.response",
		StageID:        "main",
		ScenarioID:     "login",
		ScenarioCallID: "login",
		ScenarioPath:   "stage.main/call.login",
		ActID:          "submit",
		Attempt:        1,
		ScenarioSeq:    1,
		Status:         LogStatusEmitted,
		Address:        address,
	}
}

func validReportLogRecords(count int, actID func(int) string) []LogRecord {
	logs := make([]LogRecord, 0, count)
	for i := 0; i < count; i++ {
		logID := "response-" + strconv.Itoa(i)
		act := actID(i)
		log := validReportLogRecord()
		log.ID = logID
		log.Path = "stage.main/call.login/act." + act + "/log." + logID
		log.ActID = act
		address := *log.Address
		address.ActID = act
		address.NodeRef = logID
		log.Address = &address
		logs = append(logs, log)
	}

	return logs
}
