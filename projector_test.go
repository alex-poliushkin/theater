package theater

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSafeProjectTextRedactsSecretPayload(t *testing.T) {
	t.Parallel()

	payload := SafeProjectText(
		"Authorization: Bearer super-secret-token\r\nnext-line",
		PayloadMetadata{
			Origin:      "action.http.response",
			Sensitivity: SensitivitySecret,
			Capture:     CaptureSummary,
			ContentType: "text/plain",
			SizeBytes:   50,
		},
		32,
	)

	if !payload.Metadata.Redacted {
		t.Fatal("expected redacted payload")
	}

	if strings.Contains(payload.Preview, "super-secret-token") {
		t.Fatalf("payload preview leaked secret: %q", payload.Preview)
	}

	if got, want := payload.Preview, "[redacted]"; got != want {
		t.Fatalf("payload preview mismatch: got %q want %q", got, want)
	}
}

func TestSafeProjectTextTruncatesNonSecretPayloadDeterministically(t *testing.T) {
	t.Parallel()

	payload := SafeProjectText(
		"line-1\r\nline-2\r\nline-3",
		PayloadMetadata{
			Origin:      "action.http.response",
			Sensitivity: SensitivityInternal,
			Capture:     CaptureSummary,
			ContentType: "text/plain",
			SizeBytes:   22,
		},
		10,
	)

	if !payload.Metadata.Truncated {
		t.Fatal("expected truncated payload")
	}

	if strings.Contains(payload.Preview, "\r") || strings.Contains(payload.Preview, "\n") {
		t.Fatalf("payload preview must be sanitized: %q", payload.Preview)
	}

	if got, want := payload.Preview, "line-1\\nli..."; got != want {
		t.Fatalf("payload preview mismatch: got %q want %q", got, want)
	}
}

func TestSafeProjectTextKeepsUTF8Boundaries(t *testing.T) {
	t.Parallel()

	payload := SafeProjectText(
		"абвгд",
		PayloadMetadata{
			Origin:      "action.command.stdout",
			Sensitivity: SensitivityInternal,
			Capture:     CaptureSummary,
			ContentType: "text/plain",
			SizeBytes:   10,
		},
		5,
	)

	if !payload.Metadata.Truncated {
		t.Fatal("expected truncated payload")
	}

	if got, want := payload.Preview, "аб..."; got != want {
		t.Fatalf("payload preview mismatch: got %q want %q", got, want)
	}

	if !utf8.ValidString(payload.Preview) {
		t.Fatalf("payload preview must keep UTF-8 valid: %q", payload.Preview)
	}
}

func TestProjectReportAggregatesScenarioOutcomesIntoReport(t *testing.T) {
	t.Parallel()

	stageFailure := &Failure{
		Kind:    FailureKindAction,
		Phase:   PhaseRun,
		At:      "stage.main/call.login-user",
		Summary: "login failed",
	}

	report, err := NewProjector().Project([]Event{
		{
			Kind:         "scenario.finished",
			StagePath:    "stage.main",
			ScenarioPath: "stage.main/call.prepare-user",
			Path:         "stage.main/call.prepare-user",
			Attempt:      1,
			Status:       StatusPassed,
			ScenarioSeq:  1,
		},
		{
			Kind:         "action.finished",
			StagePath:    "stage.main",
			ScenarioPath: "stage.main/call.login-user",
			Path:         "stage.main/call.login-user/act.submit/action",
			Attempt:      1,
			Status:       StatusPassed,
			ScenarioSeq:  2,
			Payload: &PayloadMetadata{
				Sensitivity: SensitivitySecret,
				Capture:     CaptureArtifactRef,
				ArtifactRef: "artifact://run/1/action/submit",
			},
		},
		{
			Kind:         "expectation.finished",
			StagePath:    "stage.main",
			ScenarioPath: "stage.main/call.login-user",
			Path:         "stage.main/call.login-user/act.submit/expectation.token",
			Attempt:      1,
			Status:       StatusPassed,
			ScenarioSeq:  2,
		},
		{
			Kind:         "act.finished",
			StagePath:    "stage.main",
			ScenarioPath: "stage.main/call.login-user",
			Path:         "stage.main/call.login-user/act.submit",
			Attempt:      1,
			Status:       StatusPassed,
			ScenarioSeq:  2,
		},
		{
			Kind:         "scenario.finished",
			StagePath:    "stage.main",
			ScenarioPath: "stage.main/call.login-user",
			Path:         "stage.main/call.login-user",
			Attempt:      1,
			Status:       StatusFailed,
			Failure:      stageFailure,
			ScenarioSeq:  2,
		},
		{
			Kind:      "stage.finished",
			StagePath: "stage.main",
			Status:    StatusFailed,
			Failure:   stageFailure,
		},
	})
	if err != nil {
		t.Fatalf("project report failed: %v", err)
	}

	if got, want := report.StagePath, "stage.main"; got != want {
		t.Fatalf("stage path mismatch: got %q want %q", got, want)
	}

	if got, want := report.Status, StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	if got, want := report.Summary.TotalScenarios, 2; got != want {
		t.Fatalf("total scenarios mismatch: got %d want %d", got, want)
	}

	if got, want := report.Summary.PassedScenarios, 1; got != want {
		t.Fatalf("passed scenarios mismatch: got %d want %d", got, want)
	}

	if got, want := report.Summary.FailedScenarios, 1; got != want {
		t.Fatalf("failed scenarios mismatch: got %d want %d", got, want)
	}

	if got, want := len(report.Nodes), 5; got != want {
		t.Fatalf("node count mismatch: got %d want %d", got, want)
	}

	if got, want := report.Nodes[0].Kind, NodeKindScenario; got != want {
		t.Fatalf("first node kind mismatch: got %q want %q", got, want)
	}

	if got, want := report.Nodes[0].Path, "stage.main/call.prepare-user"; got != want {
		t.Fatalf("first node path mismatch: got %q want %q", got, want)
	}

	if got, want := report.Nodes[1].Kind, NodeKindScenario; got != want {
		t.Fatalf("second node kind mismatch: got %q want %q", got, want)
	}

	actionNode := findReportNodeByKindAndPath(t, report.Nodes, NodeKindAction, "stage.main/call.login-user/act.submit/action")
	if actionNode.Payload == nil {
		t.Fatal("action node payload must be preserved")
	}

	if got, want := actionNode.Payload.ArtifactRef, "artifact://run/1/action/submit"; got != want {
		t.Fatalf("action node artifact ref mismatch: got %q want %q", got, want)
	}

	findReportNodeByKindAndPath(t, report.Nodes, NodeKindAct, "stage.main/call.login-user/act.submit")
	findReportNodeByKindAndPath(t, report.Nodes, NodeKindExpectation, "stage.main/call.login-user/act.submit/expectation.token")
}

func TestProjectReportKeepsScenarioLogsAddressableAndOutsideNodes(t *testing.T) {
	t.Parallel()

	sourceSpan := &SourceRef{
		File:   "theater/flows/login.yaml",
		Line:   17,
		Column: 9,
	}
	address := &NodeAddress{
		ScenarioCallPath: "stage.main/call.login-user",
		ActID:            "submit",
		Kind:             NodeKindLog,
		NodeRef:          "response",
		Phase:            "log.evaluate",
		AttemptIndex:     2,
	}

	report, err := NewProjector().Project([]Event{
		{
			Kind:           EventKindLogEmitted,
			StageID:        "main",
			StagePath:      "stage.main",
			ScenarioID:     "login",
			ScenarioCallID: "login-user",
			ScenarioPath:   "stage.main/call.login-user",
			Path:           "stage.main/call.login-user/act.submit/log.response",
			Address:        address,
			Attempt:        2,
			ScenarioSeq:    1,
			Status:         StatusPassed,
			SourceSpan:     sourceSpan,
			Log: &LogRecord{
				ID:             "response",
				Path:           "stage.main/call.login-user/act.submit/log.response",
				StageID:        "main",
				ScenarioID:     "login",
				ScenarioCallID: "login-user",
				ScenarioPath:   "stage.main/call.login-user",
				ActID:          "submit",
				Attempt:        2,
				ScenarioSeq:    1,
				Status:         LogStatusEmitted,
				Format:         "json",
				SourceSpan:     sourceSpan,
				Address:        address,
				Preview: &Preview{
					Kind:        "object",
					Text:        `{"status":201}`,
					SizeHint:    14,
					ContentType: "application/json",
				},
				Payload: &PayloadMetadata{
					Origin:      "log.response",
					Sensitivity: SensitivityInternal,
					Capture:     CaptureSummary,
					ContentType: "application/json",
					SizeBytes:   14,
				},
			},
		},
		{
			Kind:         EventKindScenarioFinished,
			StageID:      "main",
			StagePath:    "stage.main",
			ScenarioPath: "stage.main/call.login-user",
			Path:         "stage.main/call.login-user",
			Attempt:      1,
			ScenarioSeq:  1,
			Status:       StatusPassed,
		},
		{
			Kind:      EventKindStageFinished,
			StageID:   "main",
			StagePath: "stage.main",
			Path:      "stage.main",
			Attempt:   1,
			Status:    StatusPassed,
		},
	})
	if err != nil {
		t.Fatalf("project report failed: %v", err)
	}

	if got, want := len(report.Logs), 1; got != want {
		t.Fatalf("log count mismatch: got %d want %d", got, want)
	}

	log := report.Logs[0]
	if got, want := log.ID, "response"; got != want {
		t.Fatalf("log id mismatch: got %q want %q", got, want)
	}
	if got, want := log.Address.Kind, NodeKindLog; got != want {
		t.Fatalf("log address kind mismatch: got %q want %q", got, want)
	}
	if got, want := log.Address.AttemptIndex, 2; got != want {
		t.Fatalf("log attempt mismatch: got %d want %d", got, want)
	}
	if got, want := log.SourceSpan.Line, 17; got != want {
		t.Fatalf("log source line mismatch: got %d want %d", got, want)
	}

	for _, node := range report.Nodes {
		if node.Kind == NodeKindLog {
			t.Fatalf("log must not be projected as a node: %#v", node)
		}
	}
	if got, want := report.Summary.TotalScenarios, 1; got != want {
		t.Fatalf("summary total mismatch: got %d want %d", got, want)
	}

	logOnlyRecord := log
	logOnlyReport, err := NewProjector().Project([]Event{
		{
			Kind:           EventKindLogEmitted,
			StageID:        "main",
			StagePath:      "stage.main",
			ScenarioID:     "login",
			ScenarioCallID: "login-user",
			ScenarioPath:   "stage.main/call.login-user",
			Path:           log.Path,
			Address:        log.Address,
			Attempt:        log.Attempt,
			ScenarioSeq:    log.ScenarioSeq,
			Status:         StatusPassed,
			SourceSpan:     log.SourceSpan,
			Log:            &logOnlyRecord,
		},
		{
			Kind:      EventKindStageFinished,
			StageID:   "main",
			StagePath: "stage.main",
			Path:      "stage.main",
			Attempt:   1,
			Status:    StatusPassed,
		},
	})
	if err != nil {
		t.Fatalf("project log-only report failed: %v", err)
	}
	if got, want := logOnlyReport.Summary.TotalScenarios, 0; got != want {
		t.Fatalf("log-only summary total mismatch: got %d want %d", got, want)
	}
}

func TestProjectReportSortsScenarioLogsDeterministically(t *testing.T) {
	t.Parallel()

	logEvent := func(scenarioSeq int, actID string, logID string, attempt int) Event {
		path := "stage.main/call.login-user/act." + actID + "/log." + logID
		address := &NodeAddress{
			ScenarioCallPath: "stage.main/call.login-user",
			ActID:            actID,
			Kind:             NodeKindLog,
			NodeRef:          logID,
			Phase:            "log.evaluate",
			AttemptIndex:     attempt,
		}

		return Event{
			Kind:           EventKindLogEmitted,
			StageID:        "main",
			StagePath:      "stage.main",
			ScenarioID:     "login",
			ScenarioCallID: "login-user",
			ScenarioPath:   "stage.main/call.login-user",
			Path:           path,
			Address:        address,
			Attempt:        attempt,
			ScenarioSeq:    scenarioSeq,
			Status:         StatusPassed,
			Log: &LogRecord{
				ID:             logID,
				Path:           path,
				StageID:        "main",
				ScenarioID:     "login",
				ScenarioCallID: "login-user",
				ScenarioPath:   "stage.main/call.login-user",
				ActID:          actID,
				Attempt:        attempt,
				ScenarioSeq:    scenarioSeq,
				Status:         LogStatusOmitted,
				Address:        address,
				Preview: &Preview{
					Kind:          "string",
					OmittedReason: "not_visible",
				},
			},
		}
	}

	report, err := NewProjector().Project([]Event{
		logEvent(2, "audit", "response", 1),
		logEvent(1, "submit", "response", 2),
		logEvent(1, "submit", "response", 1),
		{
			Kind:      EventKindStageFinished,
			StageID:   "main",
			StagePath: "stage.main",
			Path:      "stage.main",
			Attempt:   1,
			Status:    StatusPassed,
		},
	})
	if err != nil {
		t.Fatalf("project report failed: %v", err)
	}

	if got, want := len(report.Logs), 3; got != want {
		t.Fatalf("log count mismatch: got %d want %d", got, want)
	}

	want := []struct {
		scenarioSeq int
		actID       string
		attempt     int
	}{
		{scenarioSeq: 1, actID: "submit", attempt: 1},
		{scenarioSeq: 1, actID: "submit", attempt: 2},
		{scenarioSeq: 2, actID: "audit", attempt: 1},
	}
	for i := range want {
		if got := report.Logs[i].ScenarioSeq; got != want[i].scenarioSeq {
			t.Fatalf("log[%d] scenario sequence mismatch: got %d want %d", i, got, want[i].scenarioSeq)
		}
		if got := report.Logs[i].ActID; got != want[i].actID {
			t.Fatalf("log[%d] act id mismatch: got %q want %q", i, got, want[i].actID)
		}
		if got := report.Logs[i].Attempt; got != want[i].attempt {
			t.Fatalf("log[%d] attempt mismatch: got %d want %d", i, got, want[i].attempt)
		}
	}
}

func TestProjectReportEnforcesPerRunLogLimit(t *testing.T) {
	t.Parallel()

	events := make([]Event, 0, DefaultScenarioLogRecordsPerRun+3)
	for i := 0; i < DefaultScenarioLogRecordsPerRun+2; i++ {
		actID := "act-" + strconv.Itoa(i)
		logID := "response"
		path := "stage.main/call.login-user/act." + actID + "/log." + logID
		address := &NodeAddress{
			ScenarioCallPath: "stage.main/call.login-user",
			ActID:            actID,
			Kind:             NodeKindLog,
			NodeRef:          logID,
			Phase:            "log.evaluate",
			AttemptIndex:     1,
		}
		events = append(events, Event{
			Kind:           EventKindLogEmitted,
			StageID:        "main",
			StagePath:      "stage.main",
			ScenarioID:     "login",
			ScenarioCallID: "login-user",
			ScenarioPath:   "stage.main/call.login-user",
			Path:           path,
			Address:        address,
			Attempt:        1,
			ScenarioSeq:    i + 1,
			Status:         StatusPassed,
			Log: &LogRecord{
				ID:             logID,
				Path:           path,
				StageID:        "main",
				ScenarioID:     "login",
				ScenarioCallID: "login-user",
				ScenarioPath:   "stage.main/call.login-user",
				ActID:          actID,
				Attempt:        1,
				ScenarioSeq:    i + 1,
				Status:         LogStatusOmitted,
				Address:        address,
				Preview: &Preview{
					Kind:          "string",
					OmittedReason: "not_visible",
				},
			},
		})
	}
	events = append(events, Event{
		Kind:      EventKindStageFinished,
		StageID:   "main",
		StagePath: "stage.main",
		Path:      "stage.main",
		Attempt:   1,
		Status:    StatusPassed,
	})

	report, err := NewProjector().Project(events)
	if err != nil {
		t.Fatalf("project report failed: %v", err)
	}

	if got, want := len(report.Logs), DefaultScenarioLogRecordsPerRun; got != want {
		t.Fatalf("retained log count mismatch: got %d want %d", got, want)
	}
	if report.LogSummary == nil {
		t.Fatal("report log summary must be present")
	}
	if got, want := report.LogSummary.Records, DefaultScenarioLogRecordsPerRun; got != want {
		t.Fatalf("log summary records mismatch: got %d want %d", got, want)
	}
	if got, want := report.LogSummary.DroppedRecords, 2; got != want {
		t.Fatalf("dropped log count mismatch: got %d want %d", got, want)
	}
	if got, want := report.LogSummary.PerRunLimit, DefaultScenarioLogRecordsPerRun; got != want {
		t.Fatalf("per-run limit mismatch: got %d want %d", got, want)
	}
}

func TestProjectReportBoundsScenarioLogPreview(t *testing.T) {
	t.Parallel()

	address := &NodeAddress{
		ScenarioCallPath: "stage.main/call.login-user",
		ActID:            "submit",
		Kind:             NodeKindLog,
		NodeRef:          "response",
		Phase:            "log.evaluate",
		AttemptIndex:     1,
	}
	path := "stage.main/call.login-user/act.submit/log.response"
	report, err := NewProjector().Project([]Event{
		{
			Kind:           EventKindLogEmitted,
			StageID:        "main",
			StagePath:      "stage.main",
			ScenarioID:     "login",
			ScenarioCallID: "login-user",
			ScenarioPath:   "stage.main/call.login-user",
			Path:           path,
			Address:        address,
			Attempt:        1,
			ScenarioSeq:    1,
			Status:         StatusPassed,
			Log: &LogRecord{
				ID:             "response",
				Path:           path,
				StageID:        "main",
				ScenarioID:     "login",
				ScenarioCallID: "login-user",
				ScenarioPath:   "stage.main/call.login-user",
				ActID:          "submit",
				Attempt:        1,
				ScenarioSeq:    1,
				Status:         LogStatusEmitted,
				Address:        address,
				Preview: &Preview{
					Kind:      "string",
					Text:      strings.Repeat("x", DefaultScenarioLogPreviewLimitBytes+1),
					JSONValue: map[string]any{"payload": strings.Repeat("y", DefaultScenarioLogPreviewLimitBytes+1)},
				},
				Payload: &PayloadMetadata{
					Origin:      "log.response",
					Sensitivity: SensitivityInternal,
					ContentType: "text/plain",
					SizeBytes:   int64(DefaultScenarioLogPreviewLimitBytes + 1),
					Capture:     CaptureSummary,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("project report failed: %v", err)
	}
	if got, want := len(report.Logs), 1; got != want {
		t.Fatalf("retained log count mismatch: got %d want %d", got, want)
	}
	log := report.Logs[0]
	if len(log.Preview.Text) > DefaultScenarioLogPreviewLimitBytes {
		t.Fatalf("log preview exceeds limit: got %d want <= %d", len(log.Preview.Text), DefaultScenarioLogPreviewLimitBytes)
	}
	if !log.Truncated || !log.Preview.Truncated {
		t.Fatalf("bounded log must be marked truncated: %#v", log)
	}
	if log.Preview.JSONValue != nil {
		t.Fatalf("bounded log preview must not retain json_value: %#v", log.Preview.JSONValue)
	}
	if log.Payload == nil || !log.Payload.Truncated {
		t.Fatalf("bounded log payload must be marked truncated: %#v", log.Payload)
	}
	if report.LogSummary == nil {
		t.Fatal("report log summary must be present")
	}
	if got, want := report.LogSummary.TruncatedRecords, 1; got != want {
		t.Fatalf("truncated log count mismatch: got %d want %d", got, want)
	}
}

func TestProjectRunDocumentBuildsVersionedFailureIndexedReport(t *testing.T) {
	t.Parallel()

	expectationFailure := &Failure{
		Kind:    FailureKindExpectation,
		Phase:   PhaseRun,
		At:      "stage.main/call.login-user/act.submit/expectation.token",
		Summary: "token mismatch",
	}

	doc, err := NewProjector().Document([]Event{
		{
			Kind:         EventKindActionFinished,
			StagePath:    "stage.main",
			ScenarioPath: "stage.main/call.login-user",
			Path:         "stage.main/call.login-user/act.submit/action",
			Address: &NodeAddress{
				ScenarioCallPath: "stage.main/call.login-user",
				ActID:            "submit",
				Kind:             NodeKindAction,
				NodeRef:          "action",
				Phase:            "action.execute",
				AttemptIndex:     2,
			},
			Attempt:     2,
			ScenarioSeq: 1,
			Status:      StatusPassed,
			Payload: &PayloadMetadata{
				Sensitivity: SensitivityInternal,
				Capture:     CaptureArtifactRef,
				ArtifactRef: "artifact://run/1/action/stdout",
				ContentType: "text/plain",
				SizeBytes:   128,
			},
			SourceSpan: &SourceRef{
				File:   "theater/flows/login.yaml",
				Line:   12,
				Column: 3,
			},
		},
		{
			Kind:         EventKindExpectationFinished,
			StagePath:    "stage.main",
			ScenarioPath: "stage.main/call.login-user",
			Path:         "stage.main/call.login-user/act.submit/expectation.token",
			Address: &NodeAddress{
				ScenarioCallPath: "stage.main/call.login-user",
				ActID:            "submit",
				Kind:             NodeKindExpectation,
				NodeRef:          "token",
				Phase:            "assert.evaluate",
				AttemptIndex:     2,
			},
			Attempt:     2,
			ScenarioSeq: 1,
			Status:      StatusFailed,
			Failure:     expectationFailure,
			SourceSpan: &SourceRef{
				File:   "theater/flows/login.yaml",
				Line:   18,
				Column: 5,
			},
		},
		{
			Kind:         EventKindActFinished,
			StagePath:    "stage.main",
			ScenarioPath: "stage.main/call.login-user",
			Path:         "stage.main/call.login-user/act.submit",
			Address: &NodeAddress{
				ScenarioCallPath: "stage.main/call.login-user",
				ActID:            "submit",
				Kind:             NodeKindAct,
				Phase:            "act.execute",
				AttemptIndex:     2,
			},
			Attempt:     2,
			ScenarioSeq: 1,
			Status:      StatusFailed,
			Failure:     expectationFailure,
		},
		{
			Kind:         EventKindScenarioFinished,
			StagePath:    "stage.main",
			ScenarioPath: "stage.main/call.login-user",
			Path:         "stage.main/call.login-user",
			Address: &NodeAddress{
				ScenarioCallPath: "stage.main/call.login-user",
				Kind:             NodeKindScenario,
				Phase:            "scenario.execute",
			},
			Attempt:     1,
			ScenarioSeq: 1,
			Status:      StatusFailed,
			Failure:     expectationFailure,
		},
		{
			Kind:      EventKindStageFinished,
			StagePath: "stage.main",
			Path:      "stage.main",
			Attempt:   1,
			Status:    StatusFailed,
			Failure:   expectationFailure,
		},
	})
	if err != nil {
		t.Fatalf("project run document failed: %v", err)
	}

	if got, want := doc.SchemaVersion, RunDocumentSchemaVersion; got != want {
		t.Fatalf("schema version mismatch: got %q want %q", got, want)
	}

	if got, want := len(doc.Report.Failures), 1; got != want {
		t.Fatalf("failure index count mismatch: got %d want %d", got, want)
	}

	failureEntry := doc.Report.Failures[0]
	if got, want := failureEntry.Path, "stage.main/call.login-user/act.submit/expectation.token"; got != want {
		t.Fatalf("failure path mismatch: got %q want %q", got, want)
	}

	if failureEntry.Address == nil {
		t.Fatal("failure entry must include address")
	}

	if got, want := failureEntry.Address.Kind, NodeKindExpectation; got != want {
		t.Fatalf("failure address kind mismatch: got %q want %q", got, want)
	}

	if got, want := failureEntry.Address.ScenarioCallPath, "stage.main/call.login-user"; got != want {
		t.Fatalf("failure address scenario path mismatch: got %q want %q", got, want)
	}

	if got, want := failureEntry.Address.ActID, "submit"; got != want {
		t.Fatalf("failure address act id mismatch: got %q want %q", got, want)
	}

	if got, want := failureEntry.Address.NodeRef, "token"; got != want {
		t.Fatalf("failure address node ref mismatch: got %q want %q", got, want)
	}

	if got, want := failureEntry.Address.Phase, "assert.evaluate"; got != want {
		t.Fatalf("failure address phase mismatch: got %q want %q", got, want)
	}

	if got, want := failureEntry.Address.AttemptIndex, 2; got != want {
		t.Fatalf("failure address attempt mismatch: got %d want %d", got, want)
	}

	if failureEntry.SourceSpan == nil {
		t.Fatal("failure entry must include source span")
	}

	var actionNode *NodeReport
	for i := range doc.Report.Nodes {
		if doc.Report.Nodes[i].Kind == NodeKindAction {
			actionNode = &doc.Report.Nodes[i]
			break
		}
	}

	if actionNode == nil {
		t.Fatal("action node must be present in report")
	}

	if actionNode.Address == nil {
		t.Fatal("action node must include derived address")
	}

	if got, want := actionNode.Address.Kind, NodeKindAction; got != want {
		t.Fatalf("action address kind mismatch: got %q want %q", got, want)
	}

	if got, want := actionNode.Address.NodeRef, "action"; got != want {
		t.Fatalf("action address node ref mismatch: got %q want %q", got, want)
	}

	if got, want := actionNode.Address.Phase, "action.execute"; got != want {
		t.Fatalf("action address phase mismatch: got %q want %q", got, want)
	}

	if got, want := len(actionNode.Artifacts), 1; got != want {
		t.Fatalf("action artifact count mismatch: got %d want %d", got, want)
	}

	if got, want := actionNode.Artifacts[0].Locator, "artifact://run/1/action/stdout"; got != want {
		t.Fatalf("artifact locator mismatch: got %q want %q", got, want)
	}
}

func findReportNodeByKindAndPath(t *testing.T, nodes []NodeReport, kind NodeKind, path string) NodeReport {
	t.Helper()

	for _, node := range nodes {
		if node.Kind == kind && node.Path == path {
			return node
		}
	}

	t.Fatalf("missing node kind=%q path=%q", kind, path)
	return NodeReport{}
}
