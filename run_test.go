package theater_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alex-poliushkin/theater"
	builtinaction "github.com/alex-poliushkin/theater/builtin/action"
	builtindecorator "github.com/alex-poliushkin/theater/builtin/decorator"
	builtinexpectation "github.com/alex-poliushkin/theater/builtin/expectation"
	builtininventory "github.com/alex-poliushkin/theater/builtin/inventory"
	thtrsyntax "github.com/alex-poliushkin/theater/internal/authoring/thtr"
	"github.com/alex-poliushkin/theater/internal/testkit"
	"github.com/alex-poliushkin/theater/observe"
)

const (
	expectedCommandCaptureLimitBytes = 1 << 20
	expectedCommandTailLimitBytes    = 4 * 1024
)

func matcherCatalog(t *testing.T, descriptors ...theater.MatcherDescriptor) *theater.MatcherCatalog {
	t.Helper()

	catalog, err := newMatcherCatalog(descriptors...)
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	return catalog
}

func TestRunExecutesSingleScenarioHappyPath(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "token",
								Subject: theater.SubjectSpec{Field: "token"},
								Assert:  theater.AssertSpec{Ref: "expectation.token"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	action := &testkit.ScriptedAction{
		Output: theater.Outputs{"token": "issued-token"},
	}
	expectation := &testkit.ScriptedExpectation{
		CheckFunc: func(actual any) error {
			if got, want := actual, "issued-token"; got != want {
				t.Fatalf("token mismatch: got %v want %v", got, want)
			}

			return nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	recorder := &testkit.EventRecorder{}
	result, err := runStageWithOptions(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, expectation.Descriptor("expectation.token")),
		theater.RunOptions{Events: recorder},
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := len(result.Diagnostics), 0; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	if got, want := result.Report.Summary.TotalScenarios, 1; got != want {
		t.Fatalf("total scenarios mismatch: got %d want %d", got, want)
	}

	if got, want := result.Report.Summary.PassedScenarios, 1; got != want {
		t.Fatalf("passed scenarios mismatch: got %d want %d", got, want)
	}

	if got, want := len(result.Report.Nodes), 4; got != want {
		t.Fatalf("report node count mismatch: got %d want %d", got, want)
	}

	events := recorder.Events()
	if got, want := len(events), 9; got != want {
		t.Fatalf("event count mismatch: got %d want %d", got, want)
	}

	if got, want := len(expectation.CompileCalls), 2; got != want {
		t.Fatalf("matcher compile count mismatch: got %d want %d", got, want)
	}

	if got, want := events[0].Kind, theater.EventKindStageRunning; got != want {
		t.Fatalf("first event kind mismatch: got %q want %q", got, want)
	}

	if got, want := events[len(events)-1].Kind, theater.EventKindStageFinished; got != want {
		t.Fatalf("last event kind mismatch: got %q want %q", got, want)
	}

	if got, want := events[3].Kind, theater.EventKindActionRunning; got != want {
		t.Fatalf("action running kind mismatch: got %q want %q", got, want)
	}

	if got, want := events[4].Kind, theater.EventKindActionFinished; got != want {
		t.Fatalf("action finished kind mismatch: got %q want %q", got, want)
	}

	for i, event := range events {
		if got, want := event.Attempt, 1; got != want {
			t.Fatalf("event[%d] attempt mismatch: got %d want %d", i, got, want)
		}
	}
}

func TestRunRequiredLogFailureStopsBeforeExpectations(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
						Logs: []theater.LogSpec{{
							ID:         "response",
							Value:      theater.LogValueSpec{Field: "missing_body"},
							Required:   true,
							SourceSpan: &theater.SourceRef{File: "flows/login.yaml", Line: 12, Column: 11},
						}},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "token",
								Subject: theater.SubjectSpec{Field: "token"},
								Assert:  theater.AssertSpec{Ref: "expectation.token"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"token":        {Kind: theater.ValueKindString},
				"missing_body": {Kind: theater.ValueKindString},
			},
		},
		Output: theater.Outputs{"token": "issued-token"},
	}
	checked := false
	expectation := &testkit.ScriptedExpectation{
		CheckFunc: func(any) error {
			checked = true
			return nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStageWithOptions(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, expectation.Descriptor("expectation.token")),
		theater.RunOptions{},
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if checked {
		t.Fatal("expectation must not run after required log failure")
	}
	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if result.Report.Failure == nil {
		t.Fatal("expected report failure")
	}
	if got, want := result.Report.Failure.Kind, theater.FailureKindObservation; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}
	if got, want := result.Report.Failure.At, "stage.main/call.login-user/act.submit/log.response"; got != want {
		t.Fatalf("failure path mismatch: got %q want %q", got, want)
	}
	if got, want := result.Report.Failure.Summary, "log evaluation failed"; got != want {
		t.Fatalf("failure summary mismatch: got %q want %q", got, want)
	}

	if got, want := len(result.Report.Logs), 1; got != want {
		t.Fatalf("report log count mismatch: got %d want %d", got, want)
	}

	log := result.Report.Logs[0]
	if got, want := log.ID, "response"; got != want {
		t.Fatalf("log id mismatch: got %q want %q", got, want)
	}
	if got, want := log.Path, "stage.main/call.login-user/act.submit/log.response"; got != want {
		t.Fatalf("log path mismatch: got %q want %q", got, want)
	}
	if got, want := log.Status, theater.LogStatusError; got != want {
		t.Fatalf("log status mismatch: got %q want %q", got, want)
	}
	if got, want := log.ScenarioID, "login"; got != want {
		t.Fatalf("log scenario id mismatch: got %q want %q", got, want)
	}
	if got, want := log.ScenarioCallID, "login-user"; got != want {
		t.Fatalf("log scenario call id mismatch: got %q want %q", got, want)
	}
	if got, want := log.ActID, "submit"; got != want {
		t.Fatalf("log act id mismatch: got %q want %q", got, want)
	}
	if got, want := log.Attempt, 1; got != want {
		t.Fatalf("log attempt mismatch: got %d want %d", got, want)
	}
	if got, want := log.ScenarioSeq, 1; got != want {
		t.Fatalf("log scenario sequence mismatch: got %d want %d", got, want)
	}
	if log.Address == nil {
		t.Fatal("log address must be present")
	}
	if got, want := log.Address.Kind, theater.NodeKindLog; got != want {
		t.Fatalf("log address kind mismatch: got %q want %q", got, want)
	}
	if got, want := log.Address.ActID, "submit"; got != want {
		t.Fatalf("log address act id mismatch: got %q want %q", got, want)
	}
	if got, want := log.Address.NodeRef, "response"; got != want {
		t.Fatalf("log address node ref mismatch: got %q want %q", got, want)
	}
	if got, want := log.Address.Phase, "log.evaluate"; got != want {
		t.Fatalf("log address phase mismatch: got %q want %q", got, want)
	}
	if got, want := log.Address.AttemptIndex, 1; got != want {
		t.Fatalf("log address attempt mismatch: got %d want %d", got, want)
	}
	if log.SourceSpan == nil {
		t.Fatal("log source span must be present")
	}
	if got, want := log.SourceSpan.Line, 12; got != want {
		t.Fatalf("log source span line mismatch: got %d want %d", got, want)
	}
	if log.Failure == nil {
		t.Fatal("error log must carry failure")
	}

	for _, node := range result.Report.Nodes {
		if node.Kind == theater.NodeKindLog {
			t.Fatalf("log must not become node report: %#v", node)
		}
	}
}

func TestRunNonRequiredLogErrorPreservesControlFlowAndScope(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Inputs: map[string]theater.ValueContract{
					"request_id": {Kind: theater.ValueKindString, Required: true},
				},
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
						Logs: []theater.LogSpec{
							{
								ID:    "missing",
								Value: theater.LogValueSpec{Field: "missing_body"},
							},
							{
								ID:    "request_id",
								Value: theater.LogValueSpec{Field: "token"},
							},
						},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "token",
								Subject: theater.SubjectSpec{Field: "token"},
								Assert:  theater.AssertSpec{Ref: "expectation.token"},
							},
						},
						Exports: []theater.ExportSpec{
							{As: "issued_token", Field: "token"},
						},
						Transitions: []theater.TransitionSpec{
							{On: theater.TransitionOnPass, To: "consume"},
						},
					},
					{
						ID: "consume",
						Action: theater.ActionSpec{
							Use: "action.consume",
							With: map[string]theater.BindingSpec{
								"from_export": {
									Kind: theater.BindingKindRef,
									Ref:  &theater.RefSpec{Name: "issued_token"},
								},
								"from_input": {
									Kind: theater.BindingKindRef,
									Ref:  &theater.RefSpec{Name: "request_id"},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{
				ID:         "login-user",
				ScenarioID: "login",
				Bindings: map[string]theater.BindingSpec{
					"request_id": {Kind: theater.BindingKindLiteral, Value: "original-request"},
				},
			},
		},
	}

	login := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"token":        {Kind: theater.ValueKindString},
				"missing_body": {Kind: theater.ValueKindString},
			},
		},
		Output: theater.Outputs{"token": "issued-token"},
	}
	consume := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Inputs: map[string]theater.ValueContract{
				"from_export": {Kind: theater.ValueKindString, Required: true},
				"from_input":  {Kind: theater.ValueKindString, Required: true},
			},
		},
		CheckFunc: func(args theater.Args) error {
			if got, want := args["from_export"], "issued-token"; got != want {
				t.Fatalf("export arg mismatch: got %#v want %#v", got, want)
			}
			if got, want := args["from_input"], "original-request"; got != want {
				t.Fatalf("input arg mismatch: got %#v want %#v", got, want)
			}
			return nil
		},
	}
	expectation := &testkit.ScriptedExpectation{
		CheckFunc: func(actual any) error {
			if got, want := actual, "issued-token"; got != want {
				t.Fatalf("token mismatch: got %#v want %#v", got, want)
			}
			return nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", login); err != nil {
		t.Fatalf("register login action failed: %v", err)
	}
	if err := catalog.RegisterAction("action.consume", consume); err != nil {
		t.Fatalf("register consume action failed: %v", err)
	}

	result, err := runStageWithOptions(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, expectation.Descriptor("expectation.token")),
		theater.RunOptions{},
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if got, want := len(expectation.Calls), 1; got != want {
		t.Fatalf("expectation call count mismatch: got %d want %d", got, want)
	}
	if got, want := len(consume.Calls), 1; got != want {
		t.Fatalf("consume call count mismatch: got %d want %d", got, want)
	}

	missingLog := findLogRecord(t, result.Report, "missing")
	if got, want := missingLog.Status, theater.LogStatusError; got != want {
		t.Fatalf("missing log status mismatch: got %q want %q", got, want)
	}
	if missingLog.Failure == nil {
		t.Fatal("missing log must carry failure details")
	}

	requestLog := findLogRecord(t, result.Report, "request_id")
	if got, want := requestLog.Status, theater.LogStatusOmitted; got != want {
		t.Fatalf("request log status mismatch: got %q want %q", got, want)
	}
	if requestLog.Preview == nil {
		t.Fatal("request log preview must be present")
	}
	if got, want := requestLog.Preview.OmittedReason, "not_visible"; got != want {
		t.Fatalf("request log omitted reason mismatch: got %q want %q", got, want)
	}
	if requestLog.Payload != nil {
		t.Fatalf("omitted log must not carry payload metadata: %#v", requestLog.Payload)
	}
	if got, want := result.Report.Summary.TotalScenarios, 1; got != want {
		t.Fatalf("summary total mismatch: got %d want %d", got, want)
	}
	if got, want := result.Report.Summary.PassedScenarios, 1; got != want {
		t.Fatalf("summary passed mismatch: got %d want %d", got, want)
	}
}

func TestRunScenarioLogSummaryPreviewIsReportSafe(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "login",
			Acts: []theater.ActSpec{{
				ID:     "submit",
				Action: theater.ActionSpec{Use: "action.login"},
				Logs: []theater.LogSpec{{
					ID:      "token",
					Value:   theater.LogValueSpec{Field: "token"},
					Capture: theater.CaptureSummary,
				}},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "login-user", ScenarioID: "login"}},
	}

	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString},
			},
		},
		Output: theater.Outputs{"token": theater.NewSecret("issued-token")},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	log := findLogRecord(t, result.Report, "token")
	if got, want := log.Status, theater.LogStatusEmitted; got != want {
		t.Fatalf("log status mismatch: got %q want %q", got, want)
	}
	if log.Preview == nil {
		t.Fatal("log preview must be present")
	}
	if !log.Preview.Redacted {
		t.Fatal("log preview must be redacted")
	}
	if strings.Contains(log.Preview.Text, "issued-token") {
		t.Fatalf("log preview leaked secret: %q", log.Preview.Text)
	}
	if log.Payload == nil {
		t.Fatal("summary log must carry payload metadata")
	}
	if got, want := log.Payload.Sensitivity, theater.SensitivitySecret; got != want {
		t.Fatalf("payload sensitivity mismatch: got %q want %q", got, want)
	}
	if got, want := log.Payload.Capture, theater.CaptureSummary; got != want {
		t.Fatalf("payload capture mismatch: got %q want %q", got, want)
	}
	if !log.Payload.Redacted {
		t.Fatal("payload metadata must record redaction")
	}

	encoded, err := json.Marshal(result.Report)
	if err != nil {
		t.Fatalf("marshal report failed: %v", err)
	}
	if !strings.Contains(string(encoded), `"logs"`) {
		t.Fatalf("json report must include logs: %s", encoded)
	}
	if strings.Contains(string(encoded), "issued-token") {
		t.Fatalf("json report leaked secret: %s", encoded)
	}
}

func TestRunScenarioLogSummaryMarksTruncatedPreview(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "login",
			Acts: []theater.ActSpec{{
				ID:     "submit",
				Action: theater.ActionSpec{Use: "action.login"},
				Logs: []theater.LogSpec{{
					ID:          "body",
					Value:       theater.LogValueSpec{Field: "body"},
					Capture:     theater.CaptureSummary,
					Sensitivity: theater.SensitivityInternal,
				}},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "login-user", ScenarioID: "login"}},
	}

	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"body": {Kind: theater.ValueKindString},
			},
		},
		Output: theater.Outputs{"body": strings.Repeat("x", theater.DefaultScenarioLogPreviewLimitBytes+1)},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	log := findLogRecord(t, result.Report, "body")
	if !log.Truncated {
		t.Fatal("log record must mark truncated preview")
	}
	if log.Preview == nil || !log.Preview.Truncated {
		t.Fatalf("log preview must be truncated: %#v", log.Preview)
	}
	if log.Payload == nil || !log.Payload.Truncated {
		t.Fatalf("log payload metadata must mark truncation: %#v", log.Payload)
	}
	if result.Report.LogSummary == nil {
		t.Fatal("report log summary must be present")
	}
	if got, want := result.Report.LogSummary.TruncatedRecords, 1; got != want {
		t.Fatalf("truncated log count mismatch: got %d want %d", got, want)
	}
	if got, want := result.Report.LogSummary.PreviewLimitBytes, theater.DefaultScenarioLogPreviewLimitBytes; got != want {
		t.Fatalf("preview limit mismatch: got %d want %d", got, want)
	}
}

func TestRunScenarioLogsEnforcePerActReportLimit(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "probe",
			Acts: []theater.ActSpec{{
				ID:         "wait-ready",
				Eventually: &theater.EventuallySpec{Timeout: "1s", Interval: "1ms"},
				Action:     theater.ActionSpec{Use: "action.probe", Repeatable: true},
				Logs: []theater.LogSpec{{
					ID:      "status",
					Value:   theater.LogValueSpec{Field: "status"},
					Capture: theater.CaptureSummary,
				}},
				Expectations: []theater.ExpectationSpec{{
					ID:      "ready",
					Subject: theater.SubjectSpec{Field: "status"},
					Assert:  theater.AssertSpec{Ref: "expectation.ready"},
				}},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "probe-server", ScenarioID: "probe"}},
	}

	actionCalls := 0
	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"status": {Kind: theater.ValueKindString},
			},
		},
		RunFunc: func(args theater.Args) (theater.Outputs, error) {
			actionCalls++
			if actionCalls < theater.DefaultScenarioLogRecordsPerAct+2 {
				return theater.Outputs{"status": "PENDING"}, nil
			}

			return theater.Outputs{"status": "READY"}, nil
		},
	}
	expectation := &testkit.ScriptedExpectation{
		CheckFunc: func(actual any) error {
			if got, want := actual, "READY"; got != want {
				return errors.New("status is not ready")
			}

			return nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.probe", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	recorder := &testkit.EventRecorder{}
	result, err := runStageWithOptions(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, expectation.Descriptor("expectation.ready")),
		theater.RunOptions{Events: recorder},
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}
	if got, want := actionCalls, theater.DefaultScenarioLogRecordsPerAct+2; got != want {
		t.Fatalf("action call count mismatch: got %d want %d", got, want)
	}

	if got, want := len(result.Report.Logs), theater.DefaultScenarioLogRecordsPerAct; got != want {
		t.Fatalf("retained log count mismatch: got %d want %d", got, want)
	}
	if result.Report.LogSummary == nil {
		t.Fatal("report log summary must be present")
	}
	if got, want := result.Report.LogSummary.Records, theater.DefaultScenarioLogRecordsPerAct; got != want {
		t.Fatalf("log summary records mismatch: got %d want %d", got, want)
	}
	if got, want := result.Report.LogSummary.DroppedRecords, 2; got != want {
		t.Fatalf("dropped log count mismatch: got %d want %d", got, want)
	}
	if got, want := result.Report.LogSummary.PerActLimit, theater.DefaultScenarioLogRecordsPerAct; got != want {
		t.Fatalf("per-act limit mismatch: got %d want %d", got, want)
	}
	if got, want := result.Report.LogSummary.PerRunLimit, theater.DefaultScenarioLogRecordsPerRun; got != want {
		t.Fatalf("per-run limit mismatch: got %d want %d", got, want)
	}

	var droppedEvents int
	for _, event := range recorder.Events() {
		if event.Kind == theater.EventKindLogEmitted && event.Log != nil && event.Log.Dropped {
			droppedEvents++
			if event.Log.Preview == nil || event.Log.Preview.OmittedReason != "log_limit" {
				t.Fatalf("dropped event preview mismatch: %#v", event.Log.Preview)
			}
			if event.Log.Payload != nil {
				t.Fatalf("dropped event must not carry payload metadata: %#v", event.Log.Payload)
			}
		}
	}
	if got, want := droppedEvents, 2; got != want {
		t.Fatalf("dropped log event count mismatch: got %d want %d", got, want)
	}

	replayed, err := theater.NewProjector().Project(recorder.Events())
	if err != nil {
		t.Fatalf("replay report failed: %v", err)
	}
	if replayed.LogSummary == nil {
		t.Fatal("replay log summary must be present")
	}
	if got, want := replayed.LogSummary.DroppedRecords, 2; got != want {
		t.Fatalf("replay dropped log count mismatch: got %d want %d", got, want)
	}
}

func TestRunReturnsDefinitionFailureReportForTooManyScenarioLogs(t *testing.T) {
	t.Parallel()

	logs := make([]theater.LogSpec, 0, theater.DefaultScenarioLogRecordsPerAct+1)
	for i := 0; i < theater.DefaultScenarioLogRecordsPerAct+1; i++ {
		logs = append(logs, theater.LogSpec{
			ID:    "log-" + strconv.Itoa(i),
			Value: theater.LogValueSpec{Field: "status"},
		})
	}

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "probe",
			Acts: []theater.ActSpec{{
				ID:     "submit",
				Action: theater.ActionSpec{Use: "action.probe"},
				Logs:   logs,
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "probe-server", ScenarioID: "probe"}},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.probe", &testkit.ScriptedAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}
	if got, want := len(result.Diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}
	diagnostic := result.Diagnostics[0]
	if got, want := diagnostic.Code, "too_many_logs"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Path, "stage.main/scenario.probe/act.submit/logs"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func TestRunNormalizesNativeActionOutputContainers(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
						Expectations: []theater.ExpectationSpec{
							{
								ID: "header",
								Subject: theater.SubjectSpec{
									Field: "headers",
									Path:  "/X-Test",
								},
								Assert: theater.AssertSpec{
									Ref:  builtinexpectation.EqualRef,
									Args: map[string]theater.BindingSpec{"expected": {Kind: theater.BindingKindLiteral, Value: "issued-token"}},
								},
							},
							{
								ID: "item",
								Subject: theater.SubjectSpec{
									Field: "items",
									Path:  "/1",
								},
								Assert: theater.AssertSpec{
									Ref:  builtinexpectation.EqualRef,
									Args: map[string]theater.BindingSpec{"expected": {Kind: theater.BindingKindLiteral, Value: "second"}},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"headers": {Kind: theater.ValueKindObject, Elem: &theater.ValueContract{Kind: theater.ValueKindString}},
				"items":   {Kind: theater.ValueKindList, Elem: &theater.ValueContract{Kind: theater.ValueKindString}},
			},
		},
		Output: theater.Outputs{
			"headers": map[string]string{"X-Test": "issued-token"},
			"items":   []string{"first", "second"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, builtinexpectation.Descriptors()...),
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func TestRunFailsLargeIntegerExpectationWithoutFloat64PrecisionLoss(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "probe",
				Acts: []theater.ActSpec{
					{
						ID:     "check",
						Action: theater.ActionSpec{Use: "action.counter"},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "count",
								Subject: theater.SubjectSpec{Field: "count"},
								Assert: theater.AssertSpec{
									Ref: builtinexpectation.EqualRef,
									Args: map[string]theater.BindingSpec{
										"expected": {Kind: theater.BindingKindLiteral, Value: uint64(9007199254740993)},
									},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "probe-run", ScenarioID: "probe"},
		},
	}

	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"count": {Kind: theater.ValueKindNumber},
			},
		},
		Output: theater.Outputs{
			"count": json.Number("9007199254740992"),
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.counter", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, builtinexpectation.Descriptors()...),
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if result.Report.Failure == nil || result.Report.Failure.Kind != theater.FailureKindExpectation {
		t.Fatalf("report failure kind mismatch: %#v", result.Report.Failure)
	}
}

func TestRunNormalizesNativeInventoryContainers(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID: "submit",
						Action: theater.ActionSpec{
							Use: "action.login",
							With: map[string]theater.BindingSpec{
								"payload": {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "payload"}},
							},
						},
						Properties: map[string]theater.PropertySpec{
							"payload": {
								Inventory: &theater.InventoryCall{Use: "inventory.payload"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Inputs: map[string]theater.ValueContract{
				"payload": {
					Kind: theater.ValueKindObject,
					Fields: map[string]theater.ValueContract{
						"items": {Kind: theater.ValueKindList, Elem: &theater.ValueContract{Kind: theater.ValueKindString}},
					},
				},
			},
			Outputs: map[string]theater.ValueContract{
				"status": {Kind: theater.ValueKindString},
			},
		},
		RunFunc: func(args theater.Args) (theater.Outputs, error) {
			payload, ok := args["payload"].(map[string]any)
			if !ok {
				t.Fatalf("payload type mismatch: got %T", args["payload"])
			}

			items, ok := payload["items"].([]any)
			if !ok {
				t.Fatalf("payload.items type mismatch: got %T", payload["items"])
			}

			if got, want := items, []any{"first", "second"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("payload items mismatch: got %#v want %#v", got, want)
			}

			return theater.Outputs{"status": "ok"}, nil
		},
	}
	inventory := &testkit.ScriptedInventory{
		ContractValue: theater.InventoryContract{
			Produces: theater.ValueContract{
				Kind: theater.ValueKindObject,
				Fields: map[string]theater.ValueContract{
					"items": {Kind: theater.ValueKindList, Elem: &theater.ValueContract{Kind: theater.ValueKindString}},
				},
			},
		},
		Output: map[string][]string{
			"items": {"first", "second"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterInventory("inventory.payload", inventory); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers, err := theater.NewMatcherCatalog()
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matchers)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func TestRunResolvesPickWhereActExport(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "mail/wait",
			Inputs: map[string]theater.ValueContract{
				"email": {Kind: theater.ValueKindString, Required: true},
			},
			Acts: []theater.ActSpec{
				{
					ID:     "poll",
					Action: theater.ActionSpec{Use: "action.poll"},
					Transitions: []theater.TransitionSpec{
						{On: theater.TransitionOnPass, To: "use-otp"},
					},
					Exports: []theater.ExportSpec{{
						As:    "otp",
						Field: "notifications",
						Through: []theater.ThroughStepSpec{
							{
								Pick: &theater.PickStepSpec{
									Where: []theater.PickWhereClauseSpec{
										{
											Subject: theater.RelativeSubjectSpec{Path: theater.JSONPointer("/receiverAddress")},
											Assert: theater.AssertSpec{
												Ref: builtinexpectation.EqualRef,
												Args: map[string]theater.BindingSpec{
													"expected": {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "email"}},
												},
											},
										},
										{
											Subject: theater.RelativeSubjectSpec{Path: theater.JSONPointer("/subject")},
											Assert: theater.AssertSpec{
												Ref: builtinexpectation.ContainsRef,
												Args: map[string]theater.BindingSpec{
													"expected": {Kind: theater.BindingKindLiteral, Value: "Verification"},
												},
											},
										},
									},
								},
							},
							{Path: theater.JSONPointer("/body")},
						},
					}},
				},
				{
					ID: "use-otp",
					Action: theater.ActionSpec{
						Use: "action.use-otp",
						With: map[string]theater.BindingSpec{
							"otp": {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "otp"}},
						},
					},
				},
			},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{
			ID:         "wait-for-mail",
			ScenarioID: "mail/wait",
			Bindings: map[string]theater.BindingSpec{
				"email": {Kind: theater.BindingKindLiteral, Value: "demo@example.test"},
			},
		}},
	}

	poll := &testkit.ScriptedAction{
		Output: theater.Outputs{
			"notifications": []any{
				map[string]any{"subject": "Verification Code", "body": "000000"},
				map[string]any{"receiverAddress": "other@example.test", "subject": "Verification Code", "body": "111111"},
				map[string]any{"receiverAddress": "demo@example.test", "subject": "Verification Code", "body": "654321"},
			},
		},
	}
	useOTP := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Inputs: map[string]theater.ValueContract{
				"otp": {Kind: theater.ValueKindString, Required: true},
			},
		},
		CheckFunc: func(args theater.Args) error {
			if got, want := args["otp"], "654321"; got != want {
				t.Fatalf("otp arg mismatch: got %v want %v", got, want)
			}

			return nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.poll", poll); err != nil {
		t.Fatalf("register poll action failed: %v", err)
	}
	if err := catalog.RegisterAction("action.use-otp", useOTP); err != nil {
		t.Fatalf("register use otp action failed: %v", err)
	}
	matchers, err := newMatcherCatalog(builtinexpectation.Descriptors()...)
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matchers)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}
	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if got, want := len(useOTP.Calls), 1; got != want {
		t.Fatalf("use otp call count mismatch: got %d want %d", got, want)
	}
}

func TestRunResultDocumentMatchesReplayAndHidesEventsInJSON(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
						Logs: []theater.LogSpec{{
							ID:      "token",
							Value:   theater.LogValueSpec{Field: "token"},
							Capture: theater.CaptureSummary,
						}},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "token",
								Subject: theater.SubjectSpec{Field: "token"},
								Assert:  theater.AssertSpec{Ref: "expectation.token"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString},
			},
		},
		Output: theater.Outputs{"token": "issued-token"},
	}
	expectation := &testkit.ScriptedExpectation{
		CheckFunc: func(actual any) error {
			if got, want := actual, "issued-token"; got != want {
				t.Fatalf("token mismatch: got %v want %v", got, want)
			}

			return nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	recorder := &testkit.EventRecorder{}
	result, err := runStageWithOptions(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, expectation.Descriptor("expectation.token")),
		theater.RunOptions{Events: recorder},
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	doc := result.Document()
	replayed, err := projectRunDocument(recorder.Events())
	if err != nil {
		t.Fatalf("project replayed run document failed: %v", err)
	}

	if got, want := doc.SchemaVersion, theater.RunDocumentSchemaVersion; got != want {
		t.Fatalf("schema version mismatch: got %q want %q", got, want)
	}

	if got, want := replayed.SchemaVersion, theater.RunDocumentSchemaVersion; got != want {
		t.Fatalf("replayed schema version mismatch: got %q want %q", got, want)
	}

	if got, want := doc.Report, replayed.Report; !reflect.DeepEqual(got, want) {
		t.Fatalf("live and replay report mismatch: got %#v want %#v", got, want)
	}
	if got, want := result.Report.Logs, replayed.Report.Logs; !reflect.DeepEqual(got, want) {
		t.Fatalf("live and replay logs mismatch: got %#v want %#v", got, want)
	}
	if got, want := len(result.Report.Logs), 1; got != want {
		t.Fatalf("report log count mismatch: got %d want %d", got, want)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal run result failed: %v", err)
	}

	jsonText := string(encoded)
	if !strings.Contains(jsonText, `"schema_version":"`+theater.RunDocumentSchemaVersion+`"`) {
		t.Fatalf("json output must include schema version: %s", jsonText)
	}

	if strings.Contains(jsonText, `"events":`) {
		t.Fatalf("json output must not expose raw events: %s", jsonText)
	}
}

func TestRunProjectsExplicitScenarioIdentityAndTiming(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "auth/login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "token",
								Subject: theater.SubjectSpec{Field: "token"},
								Assert:  theater.AssertSpec{Ref: "expectation.token"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login_user", ScenarioID: "auth/login"},
		},
	}

	action := &testkit.ScriptedAction{
		Output: theater.Outputs{"token": "issued-token"},
	}
	expectation := &testkit.ScriptedExpectation{}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	recorder := &testkit.EventRecorder{}
	result, err := runStageWithOptions(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, expectation.Descriptor("expectation.token")),
		theater.RunOptions{Events: recorder},
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.StageID, "main"; got != want {
		t.Fatalf("stage id mismatch: got %q want %q", got, want)
	}

	if result.Report.StartedAt.IsZero() || result.Report.EndedAt.IsZero() {
		t.Fatalf("report timing must be populated: %#v", result.Report)
	}

	var scenarioNode theater.NodeReport
	var actionNode theater.NodeReport
	for _, node := range result.Report.Nodes {
		switch node.Kind {
		case theater.NodeKindScenario:
			scenarioNode = node
		case theater.NodeKindAction:
			actionNode = node
		}
	}

	if scenarioNode.Kind == "" {
		t.Fatal("scenario node not found")
	}

	if got, want := scenarioNode.ScenarioID, "auth/login"; got != want {
		t.Fatalf("scenario id mismatch: got %q want %q", got, want)
	}

	if got, want := scenarioNode.ScenarioCallID, "login_user"; got != want {
		t.Fatalf("scenario call id mismatch: got %q want %q", got, want)
	}

	if scenarioNode.StartedAt.IsZero() || scenarioNode.EndedAt.IsZero() {
		t.Fatalf("scenario timing must be populated: %#v", scenarioNode)
	}

	if got, want := actionNode.ScenarioID, "auth/login"; got != want {
		t.Fatalf("action scenario id mismatch: got %q want %q", got, want)
	}

	if got, want := actionNode.ScenarioCallID, "login_user"; got != want {
		t.Fatalf("action scenario call id mismatch: got %q want %q", got, want)
	}
}

func TestRunKeepsHTTPSessionsScenarioLocalAndReusesThemAcrossActs(t *testing.T) {
	t.Parallel()

	var protectedCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/bootstrap":
			http.SetCookie(writer, &http.Cookie{Name: "sid", Value: "issued"})
			writer.WriteHeader(http.StatusNoContent)
		case "/protected":
			cookie, err := request.Cookie("sid")
			if err != nil || cookie.Value != "issued" {
				writer.WriteHeader(http.StatusUnauthorized)
				return
			}

			protectedCalls++
			if protectedCalls == 1 {
				writer.WriteHeader(http.StatusServiceUnavailable)
				return
			}

			writer.WriteHeader(http.StatusOK)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	spec := theater.StageSpec{
		ID: "main",
		HTTP: &theater.HTTPSpec{
			Sessions: map[string]theater.HTTPSessionSpec{
				"auth": {},
			},
		},
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "auth/bootstrap_then_check",
				Acts: []theater.ActSpec{
					{
						ID: "bootstrap",
						Action: theater.ActionSpec{
							Use: "action.http",
							With: map[string]theater.BindingSpec{
								"url":     {Kind: theater.BindingKindLiteral, Value: server.URL + "/bootstrap"},
								"session": {Kind: theater.BindingKindLiteral, Value: "auth"},
							},
						},
						Transitions: []theater.TransitionSpec{
							{On: theater.TransitionOnPass, To: "protected"},
						},
					},
					{
						ID: "protected",
						Eventually: &theater.EventuallySpec{
							Timeout:  "2s",
							Interval: "10ms",
						},
						Action: theater.ActionSpec{
							Use:        "action.http",
							Repeatable: true,
							With: map[string]theater.BindingSpec{
								"url":     {Kind: theater.BindingKindLiteral, Value: server.URL + "/protected"},
								"session": {Kind: theater.BindingKindLiteral, Value: "auth"},
							},
						},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "status",
								Subject: theater.SubjectSpec{Field: "status_code"},
								Assert: theater.AssertSpec{
									Ref:  builtinexpectation.EqualRef,
									Args: map[string]theater.BindingSpec{"expected": {Kind: theater.BindingKindLiteral, Value: http.StatusOK}},
								},
							},
						},
					},
				},
			},
			{
				ID: "auth/check_only",
				Acts: []theater.ActSpec{
					{
						ID: "protected",
						Action: theater.ActionSpec{
							Use: "action.http",
							With: map[string]theater.BindingSpec{
								"url":     {Kind: theater.BindingKindLiteral, Value: server.URL + "/protected"},
								"session": {Kind: theater.BindingKindLiteral, Value: "auth"},
							},
						},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "status",
								Subject: theater.SubjectSpec{Field: "status_code"},
								Assert: theater.AssertSpec{
									Ref:  builtinexpectation.EqualRef,
									Args: map[string]theater.BindingSpec{"expected": {Kind: theater.BindingKindLiteral, Value: http.StatusUnauthorized}},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "bootstrap_flow", ScenarioID: "auth/bootstrap_then_check"},
			{
				ID:         "isolated_check",
				ScenarioID: "auth/check_only",
				Dependencies: []theater.ScenarioDependencySpec{
					{CallID: "bootstrap_flow", When: theater.TriggerPredicateDone},
				},
			},
		},
	}

	catalog := theater.NewCatalog()
	if err := builtinaction.Register(catalog); err != nil {
		t.Fatalf("register actions failed: %v", err)
	}

	result, err := runStage(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, builtinexpectation.Descriptors()...),
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	if got, want := result.Report.Summary.PassedScenarios, 2; got != want {
		t.Fatalf("passed scenarios mismatch: got %d want %d", got, want)
	}

	var eventually *theater.EventuallyReport
	for i := range result.Report.Nodes {
		node := result.Report.Nodes[i]
		if node.Kind == theater.NodeKindAct && node.ScenarioCallID == "bootstrap_flow" && node.Address != nil && node.Address.ActID == "protected" {
			eventually = node.Eventually
			break
		}
	}

	if eventually == nil {
		t.Fatalf("protected act eventually summary is missing")
	}

	if got, want := eventually.AttemptsTotal, 2; got != want {
		t.Fatalf("attempt count mismatch: got %d want %d", got, want)
	}

	if got, want := eventually.TerminationReason, theater.TerminationReasonConverged; got != want {
		t.Fatalf("termination reason mismatch: got %q want %q", got, want)
	}

	var actionNode *theater.NodeReport
	for i := range result.Report.Nodes {
		node := &result.Report.Nodes[i]
		if node.Kind == theater.NodeKindAction && node.ScenarioCallID == "bootstrap_flow" && node.Address != nil && node.Address.ActID == "protected" {
			actionNode = node
			break
		}
	}

	if actionNode == nil {
		t.Fatal("protected action node must be present")
	}

	if actionNode.Observations == nil {
		t.Fatal("action observations must be present")
	}

	sessionInput := actionNode.Observations.Inputs["session"]
	if sessionInput.Preview == nil {
		t.Fatal("session preview must be present")
	}

	if !sessionInput.Preview.Redacted {
		t.Fatal("session preview must be redacted")
	}

	if got, want := sessionInput.Preview.Text, "[redacted]"; got != want {
		t.Fatalf("session preview mismatch: got %q want %q", got, want)
	}
}

func TestRunKeepsImplicitDefaultHTTPSessionScenarioLocalAcrossActs(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/bootstrap":
			http.SetCookie(writer, &http.Cookie{Name: "sid", Value: "issued"})
			writer.WriteHeader(http.StatusNoContent)
		case "/protected":
			cookie, err := request.Cookie("sid")
			if err != nil || cookie.Value != "issued" {
				writer.WriteHeader(http.StatusUnauthorized)
				return
			}

			writer.WriteHeader(http.StatusOK)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "auth/bootstrap_then_check",
				Acts: []theater.ActSpec{
					{
						ID: "bootstrap",
						Action: theater.ActionSpec{
							Use: "action.http",
							With: map[string]theater.BindingSpec{
								"url": {Kind: theater.BindingKindLiteral, Value: server.URL + "/bootstrap"},
							},
						},
						Transitions: []theater.TransitionSpec{
							{On: theater.TransitionOnPass, To: "protected"},
						},
					},
					{
						ID: "protected",
						Action: theater.ActionSpec{
							Use: "action.http",
							With: map[string]theater.BindingSpec{
								"url": {Kind: theater.BindingKindLiteral, Value: server.URL + "/protected"},
							},
						},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "status",
								Subject: theater.SubjectSpec{Field: "status_code"},
								Assert: theater.AssertSpec{
									Ref:  builtinexpectation.EqualRef,
									Args: map[string]theater.BindingSpec{"expected": {Kind: theater.BindingKindLiteral, Value: http.StatusOK}},
								},
							},
						},
					},
				},
			},
			{
				ID: "auth/check_only",
				Acts: []theater.ActSpec{
					{
						ID: "protected",
						Action: theater.ActionSpec{
							Use: "action.http",
							With: map[string]theater.BindingSpec{
								"url": {Kind: theater.BindingKindLiteral, Value: server.URL + "/protected"},
							},
						},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "status",
								Subject: theater.SubjectSpec{Field: "status_code"},
								Assert: theater.AssertSpec{
									Ref:  builtinexpectation.EqualRef,
									Args: map[string]theater.BindingSpec{"expected": {Kind: theater.BindingKindLiteral, Value: http.StatusUnauthorized}},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "bootstrap_flow", ScenarioID: "auth/bootstrap_then_check"},
			{
				ID:         "isolated_check",
				ScenarioID: "auth/check_only",
				Dependencies: []theater.ScenarioDependencySpec{
					{CallID: "bootstrap_flow", When: theater.TriggerPredicateDone},
				},
			},
		},
	}

	catalog := theater.NewCatalog()
	if err := builtinaction.Register(catalog); err != nil {
		t.Fatalf("register actions failed: %v", err)
	}

	result, err := runStage(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, builtinexpectation.Descriptors()...),
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	if got, want := result.Report.Summary.PassedScenarios, 2; got != want {
		t.Fatalf("passed scenarios mismatch: got %d want %d", got, want)
	}
}

func TestRunCapturesHTTPAuthSlotsAndAppliesIdentityDefaults(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/login":
			http.SetCookie(writer, &http.Cookie{Name: "sid", Value: "issued"})
			writer.Header().Set("X-CSRF-Token", "issued-csrf")
			writer.WriteHeader(http.StatusOK)
		case "/submit":
			cookie, err := request.Cookie("sid")
			if err != nil || cookie.Value != "issued" {
				writer.WriteHeader(http.StatusUnauthorized)
				return
			}
			if got, want := request.Header.Get("X-CSRF-Token"), "issued-csrf"; got != want {
				writer.WriteHeader(http.StatusForbidden)
				return
			}
			writer.WriteHeader(http.StatusOK)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	spec := theater.StageSpec{
		ID: "main",
		HTTP: &theater.HTTPSpec{
			Sessions: map[string]theater.HTTPSessionSpec{
				"web": {},
			},
			Auth: map[string]theater.HTTPAuthSpec{
				"web_csrf": {Attach: []theater.HTTPAuthAttachmentSpec{{HeaderSlot: &theater.HTTPHeaderSlotAuthSpec{Name: "X-CSRF-Token", Slot: "csrf"}}}},
			},
			Identities: map[string]theater.HTTPIdentitySpec{
				"user": {
					Session: "web",
					Auth:    "web_csrf",
				},
			},
		},
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "auth/login_then_submit",
				Acts: []theater.ActSpec{
					{
						ID: "login",
						Action: theater.ActionSpec{
							Use: "action.http",
							With: map[string]theater.BindingSpec{
								"url":      {Kind: theater.BindingKindLiteral, Value: server.URL + "/login"},
								"identity": {Kind: theater.BindingKindLiteral, Value: "user"},
								"auth":     {Kind: theater.BindingKindLiteral, Value: theater.HTTPAuthNone},
							},
						},
						CaptureAuth: &theater.HTTPAuthCaptureSpec{
							Auth: "web_csrf",
							Slots: map[string]theater.HTTPCaptureSourceSpec{
								"csrf": {ResponseHeader: "X-CSRF-Token"},
							},
						},
						Transitions: []theater.TransitionSpec{
							{On: theater.TransitionOnPass, To: "submit"},
						},
					},
					{
						ID: "submit",
						Action: theater.ActionSpec{
							Use: "action.http",
							With: map[string]theater.BindingSpec{
								"url":      {Kind: theater.BindingKindLiteral, Value: server.URL + "/submit"},
								"identity": {Kind: theater.BindingKindLiteral, Value: "user"},
							},
						},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "status",
								Subject: theater.SubjectSpec{Field: "status_code"},
								Assert: theater.AssertSpec{
									Ref:  builtinexpectation.EqualRef,
									Args: map[string]theater.BindingSpec{"expected": {Kind: theater.BindingKindLiteral, Value: http.StatusOK}},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "auth/login_then_submit"},
		},
	}

	catalog := theater.NewCatalog()
	if err := builtinaction.Register(catalog); err != nil {
		t.Fatalf("register actions failed: %v", err)
	}

	result, err := runStage(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, builtinexpectation.Descriptors()...),
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func TestRunCommandAllowsExpectationOnNonZeroExit(t *testing.T) {
	t.Parallel()

	helper := testkit.BuildCommandHelper(t)
	catalog, matchers, err := newBuiltins()
	if err != nil {
		t.Fatalf("new builtins failed: %v", err)
	}

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "command",
				Acts: []theater.ActSpec{
					{
						ID: "run",
						Action: theater.ActionSpec{
							Use: "action.command",
							With: map[string]theater.BindingSpec{
								"executable": {Kind: theater.BindingKindLiteral, Value: helper},
								"args": {
									Kind: theater.BindingKindList,
									List: []theater.BindingSpec{
										{Kind: theater.BindingKindLiteral, Value: "emit"},
										{Kind: theater.BindingKindLiteral, Value: "--exit-code"},
										{Kind: theater.BindingKindLiteral, Value: "7"},
									},
								},
							},
						},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "exit-code",
								Subject: theater.SubjectSpec{Field: "exit_code"},
								Assert: theater.AssertSpec{
									Ref: builtinexpectation.EqualRef,
									Args: map[string]theater.BindingSpec{
										"expected": {Kind: theater.BindingKindLiteral, Value: 7},
									},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "run-command", ScenarioID: "command"},
		},
	}

	result, err := runStage(context.Background(), spec, catalog, matchers)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func TestRunCommandActionReportIncludesStructuredObservations(t *testing.T) {
	t.Parallel()

	helper := testkit.BuildCommandHelper(t)
	catalog, matchers, err := newBuiltins()
	if err != nil {
		t.Fatalf("new builtins failed: %v", err)
	}

	actPath := "stage.main/call.run-command/act.run"
	actionPath := actPath + "/action"
	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "command",
				Acts: []theater.ActSpec{
					{
						ID: "run",
						Action: theater.ActionSpec{
							Use: "action.command",
							With: map[string]theater.BindingSpec{
								"executable":  {Kind: theater.BindingKindLiteral, Value: helper},
								"working_dir": {Kind: theater.BindingKindLiteral, Value: filepath.Dir(helper)},
								"stdin":       {Kind: theater.BindingKindLiteral, Value: "hidden-stdin"},
								"env": {
									Kind: theater.BindingKindObject,
									Object: map[string]theater.BindingSpec{
										"COMMAND_TEST_TOKEN": {Kind: theater.BindingKindLiteral, Value: "secret-value"},
									},
								},
								"args": {
									Kind: theater.BindingKindList,
									List: []theater.BindingSpec{
										{Kind: theater.BindingKindLiteral, Value: "emit"},
										{Kind: theater.BindingKindLiteral, Value: "--stdout"},
										{Kind: theater.BindingKindLiteral, Value: "out"},
										{Kind: theater.BindingKindLiteral, Value: "--stderr"},
										{Kind: theater.BindingKindLiteral, Value: "warn"},
									},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "run-command", ScenarioID: "command"},
		},
	}

	result, err := runStage(context.Background(), spec, catalog, matchers)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	actionNode := findNodeReport(t, result.Report, theater.NodeKindAction, actionPath)
	if actionNode.Observations == nil {
		t.Fatal("action observations must be present")
	}

	if actionNode.Address == nil {
		t.Fatal("action address must be present")
	}

	if got, want := actionNode.Address.ScenarioCallPath, "stage.main/call.run-command"; got != want {
		t.Fatalf("action address scenario path mismatch: got %q want %q", got, want)
	}

	if got, want := actionNode.Address.ActID, "run"; got != want {
		t.Fatalf("action address act id mismatch: got %q want %q", got, want)
	}

	if got, want := actionNode.Address.NodeRef, "action"; got != want {
		t.Fatalf("action address node ref mismatch: got %q want %q", got, want)
	}

	if got, want := actionNode.Observations.Inputs["executable"].Preview.Text, helper; got != want {
		t.Fatalf("executable preview mismatch: got %q want %q", got, want)
	}

	if !actionNode.Observations.Inputs["env"].Preview.Redacted {
		t.Fatal("env preview must be redacted")
	}

	if got, want := actionNode.Observations.Inputs["stdin"].Preview.OmittedReason, "not_visible"; got != want {
		t.Fatalf("stdin omitted reason mismatch: got %q want %q", got, want)
	}

	if got, want := actionNode.Observations.Outputs["stdout"].Preview.Text, "out"; got != want {
		t.Fatalf("stdout preview mismatch: got %q want %q", got, want)
	}

	if got, want := actionNode.Observations.Outputs["stderr"].Preview.Text, "warn"; got != want {
		t.Fatalf("stderr preview mismatch: got %q want %q", got, want)
	}

	if got, want := actionNode.Observations.Streams["stdout"].Preview.Text, "out"; got != want {
		t.Fatalf("stdout stream preview mismatch: got %q want %q", got, want)
	}

	if got, want := actionNode.Observations.Streams["stderr"].Preview.Text, "warn"; got != want {
		t.Fatalf("stderr stream preview mismatch: got %q want %q", got, want)
	}

	if got, want := actionNode.Observations.Streams["stdout"].Payload.SizeBytes, int64(3); got != want {
		t.Fatalf("stdout stream size mismatch: got %d want %d", got, want)
	}

	if got, want := actionNode.Observations.Streams["stderr"].Payload.SizeBytes, int64(4); got != want {
		t.Fatalf("stderr stream size mismatch: got %d want %d", got, want)
	}
}

func TestRunEscapesSlashSeparatedActIDsInRuntimePaths(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "wait/ready",
						Action: theater.ActionSpec{Use: "action.login"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login_user", ScenarioID: "login"},
		},
	}

	action := &testkit.ScriptedAction{}
	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	actNode := findNodeReport(t, result.Report, theater.NodeKindAct, "stage.main/call.login_user/act.wait~1ready")
	if actNode.Address == nil {
		t.Fatal("act address must be present")
	}

	if got, want := actNode.Address.ActID, "wait/ready"; got != want {
		t.Fatalf("act address id mismatch: got %q want %q", got, want)
	}
}

func TestRunEscapesDottedActIDsInRuntimePaths(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "wait.ready",
						Action: theater.ActionSpec{Use: "action.login"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login_user", ScenarioID: "login"},
		},
	}

	action := &testkit.ScriptedAction{}
	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	actNode := findNodeReport(t, result.Report, theater.NodeKindAct, "stage.main/call.login_user/act.wait~2ready")
	if actNode.Address == nil {
		t.Fatal("act address must be present")
	}

	if got, want := actNode.Address.ActID, "wait.ready"; got != want {
		t.Fatalf("act address id mismatch: got %q want %q", got, want)
	}
}

func TestRunReportsBoundedCommandOverflowTail(t *testing.T) {
	t.Parallel()

	helper := testkit.BuildCommandHelper(t)
	pattern := "abcdefg"
	captured := repeatCommandPattern(pattern, expectedCommandCaptureLimitBytes)
	expectedTail := captured[len(captured)-expectedCommandTailLimitBytes:]
	actionPath := "stage.main/call.run-command/act.run/action"

	catalog := theater.NewCatalog()
	if err := builtinaction.Register(catalog); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "command",
				Acts: []theater.ActSpec{
					{
						ID: "run",
						Action: theater.ActionSpec{
							Use: builtinaction.CommandRef,
							With: map[string]theater.BindingSpec{
								"executable": {Kind: theater.BindingKindLiteral, Value: helper},
								"args": {
									Kind: theater.BindingKindList,
									List: []theater.BindingSpec{
										{Kind: theater.BindingKindLiteral, Value: "spam"},
										{Kind: theater.BindingKindLiteral, Value: "--stream"},
										{Kind: theater.BindingKindLiteral, Value: "stdout"},
										{Kind: theater.BindingKindLiteral, Value: "--bytes"},
										{Kind: theater.BindingKindLiteral, Value: "2000000"},
										{Kind: theater.BindingKindLiteral, Value: "--pattern"},
										{Kind: theater.BindingKindLiteral, Value: pattern},
									},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "run-command", ScenarioID: "command"},
		},
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}
	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	actionNode := findNodeReport(t, result.Report, theater.NodeKindAction, actionPath)
	if actionNode.Failure == nil {
		t.Fatal("action failure must be present")
	}
	if got, want := actionNode.Failure.Summary, "command output exceeded capture limit"; got != want {
		t.Fatalf("failure summary mismatch: got %q want %q", got, want)
	}
	if actionNode.Observations == nil {
		t.Fatal("action observations must be present")
	}

	if got, want := actionNode.Observations.Outputs["stdout"].Preview.Text, expectedTail; got != want {
		t.Fatalf("stdout partial preview mismatch: got %q want %q", got, want)
	}
	if got, want := actionNode.Observations.Streams["stdout"].Preview.Text, expectedTail; got != want {
		t.Fatalf("stdout stream preview mismatch: got %q want %q", got, want)
	}
	if got, want := actionNode.Observations.Streams["stdout"].Payload.SizeBytes, int64(expectedCommandCaptureLimitBytes); got != want {
		t.Fatalf("stdout stream size mismatch: got %d want %d", got, want)
	}
	if !actionNode.Observations.Streams["stdout"].Preview.Truncated {
		t.Fatal("stdout stream preview must be marked truncated")
	}
}

func repeatCommandPattern(pattern string, size int) string {
	if size <= 0 || pattern == "" {
		return ""
	}

	var builder strings.Builder
	builder.Grow(size)
	for builder.Len() < size {
		remaining := size - builder.Len()
		if remaining >= len(pattern) {
			builder.WriteString(pattern)
			continue
		}

		builder.WriteString(pattern[:remaining])
	}

	return builder.String()
}

func TestRunEventuallyRetriesActionFailureBeforeSuccess(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:         "submit",
						Eventually: &theater.EventuallySpec{Timeout: "1s", Interval: "1ms"},
						Action:     theater.ActionSpec{Use: "action.login", Repeatable: true},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "token",
								Subject: theater.SubjectSpec{Field: "token"},
								Assert:  theater.AssertSpec{Ref: "expectation.token"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	actionCalls := 0
	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString},
			},
		},
		RunFunc: func(args theater.Args) (theater.Outputs, error) {
			actionCalls++
			if actionCalls == 1 {
				return theater.Outputs{}, errors.New("temporary failure")
			}

			return theater.Outputs{"token": "issued-token"}, nil
		},
	}
	expectation := &testkit.ScriptedExpectation{
		CheckFunc: func(actual any) error {
			if got, want := actual, "issued-token"; got != want {
				t.Fatalf("token mismatch: got %v want %v", got, want)
			}

			return nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	recorder := &testkit.EventRecorder{}
	result, err := runStageWithOptions(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, expectation.Descriptor("expectation.token")),
		theater.RunOptions{Events: recorder},
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := len(action.Calls), 2; got != want {
		t.Fatalf("action call count mismatch: got %d want %d", got, want)
	}

	if got, want := len(expectation.Calls), 1; got != want {
		t.Fatalf("expectation call count mismatch: got %d want %d", got, want)
	}

	if got, want := len(expectation.CompileCalls), 2; got != want {
		t.Fatalf("matcher compile count mismatch: got %d want %d", got, want)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	actionAttempts := make([]int, 0, 2)
	for _, event := range recorder.Events() {
		if event.Kind == theater.EventKindActionFinished {
			actionAttempts = append(actionAttempts, event.Attempt)
		}
	}

	if got, want := len(actionAttempts), 2; got != want {
		t.Fatalf("action event count mismatch: got %d want %d", got, want)
	}

	if got, want := actionAttempts[0], 1; got != want {
		t.Fatalf("first action attempt mismatch: got %d want %d", got, want)
	}

	if got, want := actionAttempts[1], 2; got != want {
		t.Fatalf("second action attempt mismatch: got %d want %d", got, want)
	}

	actNode := findNodeReport(t, result.Report, theater.NodeKindAct, "stage.main/call.login-user/act.submit")
	if actNode.Eventually == nil {
		t.Fatal("eventually report is nil")
	}

	if got, want := actNode.Eventually.AttemptsTotal, 2; got != want {
		t.Fatalf("attempts total mismatch: got %d want %d", got, want)
	}

	if got, want := actNode.Eventually.TerminationReason, theater.TerminationReasonConverged; got != want {
		t.Fatalf("termination reason mismatch: got %q want %q", got, want)
	}

	if got, want := actNode.Eventually.SuccessAttempt, 2; got != want {
		t.Fatalf("success attempt mismatch: got %d want %d", got, want)
	}
}

func TestRunFailsWhenActionReturnsUndeclaredOutput(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString},
			},
		},
		RunFunc: func(args theater.Args) (theater.Outputs, error) {
			return theater.Outputs{"unexpected": "value"}, nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	if result.Report.Failure == nil {
		t.Fatal("expected report failure")
	}

	if got, want := result.Report.Failure.Kind, theater.FailureKindInternal; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}
}

func TestRunAllowsActionToOmitOptionalOutput(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"token":    {Kind: theater.ValueKindString, Required: true},
				"metadata": {Kind: theater.ValueKindObject},
			},
		},
		RunFunc: func(args theater.Args) (theater.Outputs, error) {
			return theater.Outputs{"token": "issued-token"}, nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	if result.Report.Failure != nil {
		t.Fatalf("expected no report failure, got %v", result.Report.Failure)
	}
}

func TestRunFailsWhenActionOmitsRequiredOutput(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString, Required: true},
			},
		},
		RunFunc: func(args theater.Args) (theater.Outputs, error) {
			return theater.Outputs{}, nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	if result.Report.Failure == nil {
		t.Fatal("expected report failure")
	}

	if got, want := result.Report.Failure.Kind, theater.FailureKindInternal; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}

	if got, want := result.Report.Failure.Summary, "action outputs failed contract validation"; got != want {
		t.Fatalf("failure summary mismatch: got %q want %q", got, want)
	}

	if result.Report.Failure.Cause == nil {
		t.Fatal("expected failure cause")
	}

	if got := result.Report.Failure.Cause.Error(); got != "action output \"token\" is required" {
		t.Fatalf("failure cause mismatch: got %q", got)
	}
}

func TestRunFailsWithInternalFailureWhenActionPanicsAndClosesScenarioScope(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "login",
			Acts: []theater.ActSpec{{
				ID:     "submit",
				Action: theater.ActionSpec{Use: "action.login"},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{
			ID:         "login-user",
			ScenarioID: "login",
		}},
	}

	action := &testkit.ScriptedAction{
		RunFunc: func(theater.Args) (theater.Outputs, error) {
			panic("action boom")
		},
	}

	var closed int32
	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	registerClosingScenarioScopeInitializer(t, catalog, &closed)

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := atomic.LoadInt32(&closed), int32(1); got != want {
		t.Fatalf("initializer close count mismatch: got %d want %d", got, want)
	}
	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if result.Report.Failure == nil {
		t.Fatal("report failure must be present")
	}
	if got, want := result.Report.Failure.Kind, theater.FailureKindInternal; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}
	if got, want := result.Report.Failure.Summary, "action panicked"; got != want {
		t.Fatalf("failure summary mismatch: got %q want %q", got, want)
	}
	if got, want := result.Report.Failure.Cause.Error(), `action "action.login" panicked: action boom`; got != want {
		t.Fatalf("failure cause mismatch: got %q want %q", got, want)
	}

	actionNode := findNodeReport(t, result.Report, theater.NodeKindAction, "stage.main/call.login-user/act.submit/action")
	if actionNode.Failure == nil {
		t.Fatal("action node failure must be present")
	}
	if got, want := actionNode.Failure.Kind, theater.FailureKindInternal; got != want {
		t.Fatalf("action node failure kind mismatch: got %q want %q", got, want)
	}
}

func TestRunFailsWithInternalFailureWhenInventoryPanics(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "login",
			Acts: []theater.ActSpec{{
				ID: "submit",
				Properties: map[string]theater.PropertySpec{
					"seed": {
						Inventory: &theater.InventoryCall{Use: "inventory.seed"},
					},
				},
				Action: theater.ActionSpec{Use: "action.login"},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{
			ID:         "login-user",
			ScenarioID: "login",
		}},
	}

	action := &testkit.ScriptedAction{}
	inventory := &testkit.ScriptedInventory{
		ContractValue: theater.InventoryContract{
			Produces: theater.ValueContract{Kind: theater.ValueKindString},
		},
		AcquireFunc: func(theater.InventoryRequest) (any, error) {
			panic("inventory boom")
		},
	}

	var closed int32
	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	if err := catalog.RegisterInventory("inventory.seed", inventory); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}
	registerClosingScenarioScopeInitializer(t, catalog, &closed)

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := atomic.LoadInt32(&closed), int32(1); got != want {
		t.Fatalf("initializer close count mismatch: got %d want %d", got, want)
	}
	if got, want := len(action.Calls), 0; got != want {
		t.Fatalf("action call count mismatch: got %d want %d", got, want)
	}
	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if result.Report.Failure == nil {
		t.Fatal("report failure must be present")
	}
	if got, want := result.Report.Failure.Kind, theater.FailureKindInternal; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}
	if got, want := result.Report.Failure.Summary, "inventory panicked"; got != want {
		t.Fatalf("failure summary mismatch: got %q want %q", got, want)
	}
	if got, want := result.Report.Failure.Cause.Error(), `inventory "inventory.seed" panicked: inventory boom`; got != want {
		t.Fatalf("failure cause mismatch: got %q want %q", got, want)
	}

	actNode := findNodeReport(t, result.Report, theater.NodeKindAct, "stage.main/call.login-user/act.submit")
	if actNode.Failure == nil {
		t.Fatal("act node failure must be present")
	}
	if got, want := actNode.Failure.Kind, theater.FailureKindInternal; got != want {
		t.Fatalf("act node failure kind mismatch: got %q want %q", got, want)
	}
}

func TestRunFailsWithInternalFailureWhenDecoratorPanics(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "login",
			Acts: []theater.ActSpec{{
				ID: "submit",
				Properties: map[string]theater.PropertySpec{
					"seed": {
						Inventory: &theater.InventoryCall{Use: "inventory.seed"},
						Decorators: []theater.DecoratorSpec{{
							Use: "decorator.normalize",
						}},
					},
				},
				Action: theater.ActionSpec{Use: "action.login"},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{
			ID:         "login-user",
			ScenarioID: "login",
		}},
	}

	action := &testkit.ScriptedAction{}
	inventory := &testkit.ScriptedInventory{
		ContractValue: theater.InventoryContract{
			Produces: theater.ValueContract{Kind: theater.ValueKindString},
		},
		Output: "seed",
	}
	decorator := &testkit.ScriptedDecorator{
		ContractValue: theater.DecoratorContract{
			Accepts:  theater.ValueContract{Kind: theater.ValueKindString},
			Produces: theater.ValueContract{Kind: theater.ValueKindString},
		},
		TransformFunc: func(any) (any, error) {
			panic("decorator boom")
		},
	}

	var closed int32
	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	if err := catalog.RegisterInventory("inventory.seed", inventory); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}
	if err := catalog.RegisterDecorator("decorator.normalize", decorator.Definition()); err != nil {
		t.Fatalf("register decorator failed: %v", err)
	}
	registerClosingScenarioScopeInitializer(t, catalog, &closed)

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := atomic.LoadInt32(&closed), int32(1); got != want {
		t.Fatalf("initializer close count mismatch: got %d want %d", got, want)
	}
	if got, want := len(action.Calls), 0; got != want {
		t.Fatalf("action call count mismatch: got %d want %d", got, want)
	}
	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if result.Report.Failure == nil {
		t.Fatal("report failure must be present")
	}
	if got, want := result.Report.Failure.Kind, theater.FailureKindInternal; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}
	if got, want := result.Report.Failure.Summary, "decorator panicked"; got != want {
		t.Fatalf("failure summary mismatch: got %q want %q", got, want)
	}
	if got, want := result.Report.Failure.Cause.Error(), `decorator "decorator.normalize" panicked: decorator boom`; got != want {
		t.Fatalf("failure cause mismatch: got %q want %q", got, want)
	}

	actNode := findNodeReport(t, result.Report, theater.NodeKindAct, "stage.main/call.login-user/act.submit")
	if actNode.Failure == nil {
		t.Fatal("act node failure must be present")
	}
	if got, want := actNode.Failure.Kind, theater.FailureKindInternal; got != want {
		t.Fatalf("act node failure kind mismatch: got %q want %q", got, want)
	}
}

func TestRunFailsWithInternalFailureWhenMatcherPanics(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "login",
			Acts: []theater.ActSpec{{
				ID:     "submit",
				Action: theater.ActionSpec{Use: "action.login"},
				Expectations: []theater.ExpectationSpec{{
					ID:      "token",
					Subject: theater.SubjectSpec{Field: "token"},
					Assert:  theater.AssertSpec{Ref: "expectation.token"},
				}},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{
			ID:         "login-user",
			ScenarioID: "login",
		}},
	}

	action := &testkit.ScriptedAction{
		Output: theater.Outputs{"token": "issued-token"},
	}
	expectation := &testkit.ScriptedExpectation{
		CheckFunc: func(any) error {
			panic("check boom")
		},
	}

	var closed int32
	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	registerClosingScenarioScopeInitializer(t, catalog, &closed)

	result, err := runStage(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, expectation.Descriptor("expectation.token")),
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := atomic.LoadInt32(&closed), int32(1); got != want {
		t.Fatalf("initializer close count mismatch: got %d want %d", got, want)
	}
	if got, want := len(action.Calls), 1; got != want {
		t.Fatalf("action call count mismatch: got %d want %d", got, want)
	}
	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if result.Report.Failure == nil {
		t.Fatal("report failure must be present")
	}
	if got, want := result.Report.Failure.Kind, theater.FailureKindInternal; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}
	if got, want := result.Report.Failure.Summary, "matcher panicked"; got != want {
		t.Fatalf("failure summary mismatch: got %q want %q", got, want)
	}
	if got, want := result.Report.Failure.Cause.Error(), `matcher "expectation.token" panicked: check boom`; got != want {
		t.Fatalf("failure cause mismatch: got %q want %q", got, want)
	}

	expectationNode := findNodeReport(t, result.Report, theater.NodeKindExpectation, "stage.main/call.login-user/act.submit/expectation.token")
	if expectationNode.Failure == nil {
		t.Fatal("expectation node failure must be present")
	}
	if got, want := expectationNode.Failure.Kind, theater.FailureKindInternal; got != want {
		t.Fatalf("expectation node failure kind mismatch: got %q want %q", got, want)
	}
}

func TestRunFailsWithInternalFailureWhenActionLiveSinkPanics(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "login",
			Acts: []theater.ActSpec{{
				ID:     "submit",
				Action: theater.ActionSpec{Use: "action.login"},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{
			ID:         "login-user",
			ScenarioID: "login",
		}},
	}

	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString},
			},
		},
		RunFunc: func(args theater.Args) (theater.Outputs, error) {
			return theater.Outputs{"token": "issued-token"}, nil
		},
	}

	var closed int32
	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", &actionReporterAction{delegate: action}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	registerClosingScenarioScopeInitializer(t, catalog, &closed)

	result, err := runStageWithOptions(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t),
		theater.RunOptions{
			Live: &panickingRunSink{panicOnKind: observe.KindProgress, message: "live boom"},
		},
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := atomic.LoadInt32(&closed), int32(1); got != want {
		t.Fatalf("initializer close count mismatch: got %d want %d", got, want)
	}
	if got, want := len(action.Calls), 1; got != want {
		t.Fatalf("action call count mismatch: got %d want %d", got, want)
	}
	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if result.Report.Failure == nil {
		t.Fatal("report failure must be present")
	}
	if got, want := result.Report.Failure.Kind, theater.FailureKindInternal; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}
	if got, want := result.Report.Failure.Summary, "live sink panicked"; got != want {
		t.Fatalf("failure summary mismatch: got %q want %q", got, want)
	}

	actionNode := findNodeReport(t, result.Report, theater.NodeKindAction, "stage.main/call.login-user/act.submit/action")
	if actionNode.Failure == nil {
		t.Fatal("action node failure must be present")
	}
	if got, want := actionNode.Failure.Cause.Error(), `live sink "stage.main/call.login-user/act.submit/action" panicked: live boom`; got != want {
		t.Fatalf("action node failure cause mismatch: got %q want %q", got, want)
	}
}

func TestRunReturnsFinalReportWhenEventRecorderPanics(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "login",
			Acts: []theater.ActSpec{{
				ID:     "submit",
				Action: theater.ActionSpec{Use: "action.login"},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{
			ID:         "login-user",
			ScenarioID: "login",
		}},
	}

	var closed int32
	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", &testkit.ScriptedAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	registerClosingScenarioScopeInitializer(t, catalog, &closed)

	result, err := runStageWithOptions(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t),
		theater.RunOptions{
			Events: panickingRunRecorder{panicOnKind: theater.EventKindActionFinished, message: "recorder boom"},
		},
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := atomic.LoadInt32(&closed), int32(1); got != want {
		t.Fatalf("initializer close count mismatch: got %d want %d", got, want)
	}
	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if result.Report.Failure == nil {
		t.Fatal("report failure must be present")
	}
	if got, want := result.Report.Failure.Kind, theater.FailureKindInternal; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}
	if got, want := result.Report.Failure.Summary, "event recorder panicked"; got != want {
		t.Fatalf("failure summary mismatch: got %q want %q", got, want)
	}
	if got, want := result.Report.Failure.Cause.Error(), "event recorder panicked: recorder boom"; got != want {
		t.Fatalf("failure cause mismatch: got %q want %q", got, want)
	}
}

func TestRunReturnsFinalReportWhenMirroredStageLiveSinkPanics(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "login",
			Acts: []theater.ActSpec{{
				ID:     "submit",
				Action: theater.ActionSpec{Use: "action.login"},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{
			ID:         "login-user",
			ScenarioID: "login",
		}},
	}

	var closed int32
	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", &testkit.ScriptedAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	registerClosingScenarioScopeInitializer(t, catalog, &closed)

	result, err := runStageWithOptions(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t),
		theater.RunOptions{
			Live: &panickingRunSink{panicOnTransition: theater.EventKindStageFinished, message: "stage live boom"},
		},
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := atomic.LoadInt32(&closed), int32(1); got != want {
		t.Fatalf("initializer close count mismatch: got %d want %d", got, want)
	}
	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if result.Report.Failure == nil {
		t.Fatal("report failure must be present")
	}
	if got, want := result.Report.Failure.Kind, theater.FailureKindInternal; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}
	if got, want := result.Report.Failure.Summary, "live sink panicked"; got != want {
		t.Fatalf("failure summary mismatch: got %q want %q", got, want)
	}
	if got, want := result.Report.Failure.Cause.Error(), `live sink "stage.main" panicked: stage live boom`; got != want {
		t.Fatalf("failure cause mismatch: got %q want %q", got, want)
	}
}

type actionReporterAction struct {
	delegate *testkit.ScriptedAction
}

type closingScenarioScopeInitializer struct {
	closed *int32
}

type panickingRunRecorder struct {
	panicOnKind string
	message     string
}

type panickingRunSink struct {
	panicOnKind       observe.Kind
	panicOnTransition string
	message           string
}

func (a *actionReporterAction) Contract() theater.ActionContract {
	return a.delegate.Contract()
}

func (a *actionReporterAction) Run(ctx context.Context, request theater.ActionRequest) (theater.Outputs, error) {
	request.Reporter.Progress(observe.Progress{
		Phase:   "run",
		Message: "emitting progress",
	})

	return a.delegate.Run(ctx, request)
}

func (*closingScenarioScopeInitializer) InitializeScenarioScope(theater.ResourceScope) {}

func registerClosingScenarioScopeInitializer(t *testing.T, catalog *theater.Catalog, closed *int32) {
	t.Helper()

	if err := catalog.RegisterScenarioScopeInitializer("test/runtime/close", func() theater.ScenarioScopeInitializer {
		return &closingScenarioScopeInitializer{closed: closed}
	}); err != nil {
		t.Fatalf("register initializer failed: %v", err)
	}
}

func (i *closingScenarioScopeInitializer) Close() {
	atomic.AddInt32(i.closed, 1)
}

func (r panickingRunRecorder) Record(event theater.Event) error {
	if event.Kind == r.panicOnKind {
		panic(r.message)
	}

	return nil
}

func (s *panickingRunSink) Publish(env observe.Envelope) uint64 {
	if s.panicOnKind != "" && env.Kind == s.panicOnKind {
		panic(s.message)
	}
	if s.panicOnTransition != "" && env.Transition != nil && env.Transition.EventKind == s.panicOnTransition {
		panic(s.message)
	}

	return 0
}

func TestRunDoesNotRetryExpectationFailureWithoutEventually(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "token",
								Subject: theater.SubjectSpec{Field: "token"},
								Assert:  theater.AssertSpec{Ref: "expectation.token"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	action := &testkit.ScriptedAction{
		Output: theater.Outputs{"token": "issued-token"},
	}
	expectation := &testkit.ScriptedExpectation{
		CheckFunc: func(any) error {
			return errors.New("expectation failed")
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	recorder := &testkit.EventRecorder{}
	result, err := runStageWithOptions(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, expectation.Descriptor("expectation.token")),
		theater.RunOptions{Events: recorder},
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := len(action.Calls), 1; got != want {
		t.Fatalf("action call count mismatch: got %d want %d", got, want)
	}

	if got, want := len(expectation.Calls), 1; got != want {
		t.Fatalf("expectation call count mismatch: got %d want %d", got, want)
	}

	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	for _, event := range recorder.Events() {
		if event.Kind != theater.EventKindActionFinished && event.Kind != theater.EventKindExpectationFinished && event.Kind != theater.EventKindActFinished {
			continue
		}

		if got, want := event.Attempt, 1; got != want {
			t.Fatalf("unexpected retry attempt on event %q: got %d want %d", event.Kind, got, want)
		}
	}
}

func TestRunEventuallyRetriesExpectationMismatchUntilSuccess(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "probe",
				Acts: []theater.ActSpec{
					{
						ID:         "wait-ready",
						Eventually: &theater.EventuallySpec{Timeout: "1s", Interval: "1ms"},
						Action:     theater.ActionSpec{Use: "action.probe", Repeatable: true},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "status",
								Subject: theater.SubjectSpec{Field: "status"},
								Assert:  theater.AssertSpec{Ref: "expectation.ready"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "probe-server", ScenarioID: "probe"},
		},
	}

	actionCalls := 0
	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"status": {Kind: theater.ValueKindString},
			},
		},
		Output: theater.Outputs{},
		RunFunc: func(args theater.Args) (theater.Outputs, error) {
			actionCalls++
			if actionCalls < 3 {
				return theater.Outputs{"status": "PENDING"}, nil
			}

			return theater.Outputs{"status": "READY"}, nil
		},
	}
	expectation := &testkit.ScriptedExpectation{
		CheckFunc: func(actual any) error {
			if got, want := actual, "READY"; got != want {
				return errors.New("status is not ready")
			}

			return nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.probe", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, expectation.Descriptor("expectation.ready")),
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := len(action.Calls), 3; got != want {
		t.Fatalf("action call count mismatch: got %d want %d (report status=%q diagnostics=%v failure=%v)", got, want, result.Report.Status, result.Diagnostics, result.Report.Failure)
	}

	if got, want := len(expectation.Calls), 3; got != want {
		t.Fatalf("expectation call count mismatch: got %d want %d", got, want)
	}

	actNode := findNodeReport(t, result.Report, theater.NodeKindAct, "stage.main/call.probe-server/act.wait-ready")
	if actNode.Eventually == nil {
		t.Fatal("eventually report is nil")
	}

	if got, want := actNode.Eventually.AttemptsTotal, 3; got != want {
		t.Fatalf("attempts total mismatch: got %d want %d", got, want)
	}

	if got, want := actNode.Eventually.FinalOutcome, theater.StatusPassed; got != want {
		t.Fatalf("final outcome mismatch: got %q want %q", got, want)
	}

	if got, want := actNode.Eventually.SuccessAttempt, 3; got != want {
		t.Fatalf("success attempt mismatch: got %d want %d", got, want)
	}

	if got, want := len(actNode.Eventually.AttemptTimeline), 3; got != want {
		t.Fatalf("attempt timeline length mismatch: got %d want %d", got, want)
	}

	if got, want := actNode.Eventually.AttemptTimeline[0].Status, theater.StatusFailed; got != want {
		t.Fatalf("first attempt status mismatch: got %q want %q", got, want)
	}

	if got, want := actNode.Eventually.AttemptTimeline[2].Status, theater.StatusPassed; got != want {
		t.Fatalf("final attempt status mismatch: got %q want %q", got, want)
	}
}

func TestRunEventuallyRetriesInventorySetupFailureUntilSuccess(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "probe",
				Acts: []theater.ActSpec{
					{
						ID:         "wait-ready",
						Eventually: &theater.EventuallySpec{Timeout: "1s", Interval: "1ms"},
						Action: theater.ActionSpec{
							Use:        "action.probe",
							Repeatable: true,
							With: map[string]theater.BindingSpec{
								"token": {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "token"}},
							},
						},
						Properties: map[string]theater.PropertySpec{
							"token": {
								Inventory: &theater.InventoryCall{Use: "inventory.seed"},
							},
						},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "status",
								Subject: theater.SubjectSpec{Field: "status"},
								Assert:  theater.AssertSpec{Ref: "expectation.ready"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "probe-server", ScenarioID: "probe"},
		},
	}

	inventoryCalls := 0
	inventory := &testkit.ScriptedInventory{
		ContractValue: theater.InventoryContract{
			Produces: theater.ValueContract{Kind: theater.ValueKindString},
		},
		AcquireFunc: func(request theater.InventoryRequest) (any, error) {
			inventoryCalls++
			if inventoryCalls < 3 {
				return nil, errors.New("inventory not ready")
			}

			return "issued-3", nil
		},
	}
	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Inputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString},
			},
			Outputs: map[string]theater.ValueContract{
				"status": {Kind: theater.ValueKindString},
			},
		},
		CheckFunc: func(args theater.Args) error {
			if got, want := args["token"], "issued-3"; got != want {
				t.Fatalf("token arg mismatch: got %v want %v", got, want)
			}

			return nil
		},
		Output: theater.Outputs{"status": "READY"},
	}
	expectation := &testkit.ScriptedExpectation{
		CheckFunc: func(actual any) error {
			if got, want := actual, "READY"; got != want {
				t.Fatalf("expectation actual mismatch: got %v want %v", got, want)
			}

			return nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterInventory("inventory.seed", inventory); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}
	if err := catalog.RegisterAction("action.probe", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, expectation.Descriptor("expectation.ready")),
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := len(inventory.Calls), 3; got != want {
		t.Fatalf("inventory call count mismatch: got %d want %d", got, want)
	}

	if got, want := len(action.Calls), 1; got != want {
		t.Fatalf("action call count mismatch: got %d want %d", got, want)
	}

	if got, want := len(expectation.Calls), 1; got != want {
		t.Fatalf("expectation call count mismatch: got %d want %d", got, want)
	}

	actNode := findNodeReport(t, result.Report, theater.NodeKindAct, "stage.main/call.probe-server/act.wait-ready")
	if actNode.Eventually == nil {
		t.Fatal("eventually report is nil")
	}

	if got, want := actNode.Eventually.AttemptsTotal, 3; got != want {
		t.Fatalf("attempts total mismatch: got %d want %d", got, want)
	}

	if got, want := actNode.Eventually.TerminationReason, theater.TerminationReasonConverged; got != want {
		t.Fatalf("termination reason mismatch: got %q want %q", got, want)
	}

	if got, want := actNode.Eventually.SuccessAttempt, 3; got != want {
		t.Fatalf("success attempt mismatch: got %d want %d", got, want)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func TestRunEventuallyRetriesActExportSelectorFailureUntilSuccess(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "select-item",
			Acts: []theater.ActSpec{
				{
					ID:         "wait-item",
					Eventually: &theater.EventuallySpec{Timeout: "1s", Interval: "1ms"},
					Action:     theater.ActionSpec{Use: "action.find", Repeatable: true},
					Exports: []theater.ExportSpec{{
						As:    "selected_status",
						Field: "items",
						Through: []theater.ThroughStepSpec{
							pickWhereIDEqualsRef("target"),
							{Path: theater.JSONPointer("/status")},
						},
					}},
					Expectations: []theater.ExpectationSpec{{
						ID:      "action-ready",
						Subject: theater.SubjectSpec{Field: "ready"},
						Assert: theater.AssertSpec{
							Ref: builtinexpectation.EqualRef,
							Args: map[string]theater.BindingSpec{
								"expected": {Kind: theater.BindingKindLiteral, Value: true},
							},
						},
					}},
					Transitions: []theater.TransitionSpec{{On: theater.TransitionOnPass, To: "verify"}},
				},
				{
					ID: "verify",
					Action: theater.ActionSpec{
						Use: "action.verify",
						With: map[string]theater.BindingSpec{
							"status": {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "selected_status"}},
						},
					},
				},
			},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "run", ScenarioID: "select-item"}},
	}

	actionCalls := 0
	findAction := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"target": {Kind: theater.ValueKindString},
				"items":  {Kind: theater.ValueKindAny},
				"ready":  {Kind: theater.ValueKindBool},
			},
		},
		RunFunc: func(theater.Args) (theater.Outputs, error) {
			actionCalls++
			item := map[string]any{"id": "item-100", "kind": "sample", "label": "alpha"}
			if actionCalls >= 2 {
				item["status"] = "ready"
			}

			return theater.Outputs{
				"target": "item-100",
				"items":  []any{item},
				"ready":  true,
			}, nil
		},
	}
	verifyAction := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Inputs: map[string]theater.ValueContract{
				"status": {Kind: theater.ValueKindString, Required: true},
			},
		},
		CheckFunc: func(args theater.Args) error {
			if got, want := args["status"], "ready"; got != want {
				t.Fatalf("status arg mismatch: got %v want %v", got, want)
			}

			return nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.find", findAction); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	if err := catalog.RegisterAction("action.verify", verifyAction); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t, builtinexpectation.Descriptors()...))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q diagnostics=%v failure=%v", got, want, result.Diagnostics, result.Report.Failure)
	}
	if got, want := len(findAction.Calls), 2; got != want {
		t.Fatalf("find action call count mismatch: got %d want %d", got, want)
	}
	if got, want := len(verifyAction.Calls), 1; got != want {
		t.Fatalf("verify action call count mismatch: got %d want %d", got, want)
	}

	actNode := findNodeReport(t, result.Report, theater.NodeKindAct, "stage.main/call.run/act.wait-item")
	if actNode.Eventually == nil {
		t.Fatal("eventually report is nil")
	}
	if got, want := actNode.Eventually.TerminationReason, theater.TerminationReasonConverged; got != want {
		t.Fatalf("termination reason mismatch: got %q want %q", got, want)
	}
	if got, want := actNode.Eventually.SuccessAttempt, 2; got != want {
		t.Fatalf("success attempt mismatch: got %d want %d", got, want)
	}
	if got, want := len(actNode.Eventually.AttemptTimeline), 2; got != want {
		t.Fatalf("attempt timeline length mismatch: got %d want %d", got, want)
	}
	firstAttempt := actNode.Eventually.AttemptTimeline[0]
	requireFailure(t, firstAttempt.Failure, theater.FailureKindObservation, "act export failed", `path "/status" target is missing`)
	if got, want := firstAttempt.Failure.At, "stage.main/call.run/act.wait-item/export.selected_status"; got != want {
		t.Fatalf("first attempt failure path mismatch: got %q want %q", got, want)
	}
	if got := firstAttempt.FailureSummary; !strings.Contains(got, `path "/status" target is missing`) {
		t.Fatalf("first attempt failure summary mismatch: got %q", got)
	}
	if got, want := firstAttempt.Status, theater.StatusFailed; got != want {
		t.Fatalf("first attempt status mismatch: got %q want %q", got, want)
	}
	if !firstAttempt.Retryable {
		t.Fatal("first attempt must be retryable")
	}
	if got, want := actNode.Eventually.AttemptTimeline[1].Status, theater.StatusPassed; got != want {
		t.Fatalf("second attempt status mismatch: got %q want %q", got, want)
	}
	requireFailure(t, actNode.Eventually.LastObservedFailure, theater.FailureKindObservation, "act export failed", `path "/status" target is missing`)
	if got, want := actNode.Eventually.LastObservedFailure.At, "stage.main/call.run/act.wait-item/export.selected_status"; got != want {
		t.Fatalf("last observed failure path mismatch: got %q want %q", got, want)
	}
}

func TestRunEventuallyTimeoutAfterExpectationMismatch(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "probe",
				Acts: []theater.ActSpec{
					{
						ID:         "wait-ready",
						Eventually: &theater.EventuallySpec{Timeout: "20ms", Interval: "5ms"},
						Action:     theater.ActionSpec{Use: "action.probe", Repeatable: true},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "status",
								Subject: theater.SubjectSpec{Field: "status"},
								Assert:  theater.AssertSpec{Ref: "expectation.ready"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "probe-server", ScenarioID: "probe"},
		},
	}

	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"status": {Kind: theater.ValueKindString},
			},
		},
		RunFunc: func(args theater.Args) (theater.Outputs, error) {
			return theater.Outputs{"status": "PENDING"}, nil
		},
	}
	expectation := &testkit.ScriptedExpectation{
		CheckFunc: func(actual any) error {
			return errors.New("status is not ready")
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.probe", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, expectation.Descriptor("expectation.ready")),
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	actNode := findNodeReport(t, result.Report, theater.NodeKindAct, "stage.main/call.probe-server/act.wait-ready")
	if actNode.Eventually == nil {
		t.Fatal("eventually report is nil")
	}

	if got, want := actNode.Eventually.TerminationReason, theater.TerminationReasonDeadlineExceeded; got != want {
		t.Fatalf("termination reason mismatch: got %q want %q", got, want)
	}

	if actNode.Eventually.FinalFailureReason == nil {
		t.Fatal("final failure reason is nil")
	}

	if got, want := actNode.Eventually.FinalFailureReason.Kind, theater.FailureKindTimeout; got != want {
		t.Fatalf("final failure kind mismatch: got %q want %q", got, want)
	}

	if actNode.Eventually.LastObservedFailure == nil {
		t.Fatal("last observed failure is nil")
	}

	if got, want := actNode.Eventually.LastObservedFailure.Kind, theater.FailureKindExpectation; got != want {
		t.Fatalf("last observed failure kind mismatch: got %q want %q", got, want)
	}
}

func TestRunEventuallyTimeoutAfterInventorySetupFailure(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "probe",
				Acts: []theater.ActSpec{
					{
						ID:         "wait-ready",
						Eventually: &theater.EventuallySpec{Timeout: "20ms", Interval: "1ms"},
						Action:     theater.ActionSpec{Use: "action.probe", Repeatable: true},
						Properties: map[string]theater.PropertySpec{
							"token": {
								Inventory: &theater.InventoryCall{Use: "inventory.seed"},
							},
						},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "status",
								Subject: theater.SubjectSpec{Field: "status"},
								Assert:  theater.AssertSpec{Ref: "expectation.ready"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "probe-server", ScenarioID: "probe"},
		},
	}

	inventory := &testkit.ScriptedInventory{
		ContractValue: theater.InventoryContract{
			Produces: theater.ValueContract{Kind: theater.ValueKindString},
		},
		Err: errors.New("inventory not ready"),
	}
	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"status": {Kind: theater.ValueKindString},
			},
		},
		Output: theater.Outputs{"status": "READY"},
	}
	expectation := &testkit.ScriptedExpectation{CheckFunc: func(actual any) error { return nil }}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterInventory("inventory.seed", inventory); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}
	if err := catalog.RegisterAction("action.probe", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, expectation.Descriptor("expectation.ready")),
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got := len(action.Calls); got != 0 {
		t.Fatalf("action must not run when setup keeps failing, got %d call(s)", got)
	}

	actNode := findNodeReport(t, result.Report, theater.NodeKindAct, "stage.main/call.probe-server/act.wait-ready")
	if actNode.Eventually == nil {
		t.Fatal("eventually report is nil")
	}

	if got, want := actNode.Eventually.TerminationReason, theater.TerminationReasonDeadlineExceeded; got != want {
		t.Fatalf("termination reason mismatch: got %q want %q", got, want)
	}

	if got := actNode.Eventually.AttemptsTotal; got < 2 {
		t.Fatalf("attempts total must reflect retries, got %d", got)
	}

	if actNode.Eventually.FinalFailureReason == nil {
		t.Fatal("final failure reason is nil")
	}

	if got, want := actNode.Eventually.FinalFailureReason.Kind, theater.FailureKindTimeout; got != want {
		t.Fatalf("final failure kind mismatch: got %q want %q", got, want)
	}

	if actNode.Eventually.LastObservedFailure == nil {
		t.Fatal("last observed failure is nil")
	}

	if got, want := actNode.Eventually.LastObservedFailure.Kind, theater.FailureKindSetup; got != want {
		t.Fatalf("last observed failure kind mismatch: got %q want %q", got, want)
	}
}

func TestRunEventuallyFailedAttemptsDoNotCommitExports(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "probe",
				Acts: []theater.ActSpec{
					{
						ID:         "fetch-token",
						Eventually: &theater.EventuallySpec{Timeout: "1s", Interval: "1ms"},
						Action:     theater.ActionSpec{Use: "action.fetch-token", Repeatable: true},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "ready",
								Subject: theater.SubjectSpec{Field: "ready"},
								Assert:  theater.AssertSpec{Ref: "expectation.ready"},
							},
						},
						Exports: []theater.ExportSpec{{Field: "token"}},
						Transitions: []theater.TransitionSpec{
							{On: theater.TransitionOnPass, To: "use-token"},
						},
					},
					{
						ID: "use-token",
						Action: theater.ActionSpec{
							Use: "action.use-token",
							With: map[string]theater.BindingSpec{
								"token": {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "token"}},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "probe-server", ScenarioID: "probe"},
		},
	}

	fetchCalls := 0
	fetch := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString},
				"ready": {Kind: theater.ValueKindBool},
			},
		},
		RunFunc: func(args theater.Args) (theater.Outputs, error) {
			fetchCalls++
			if fetchCalls == 1 {
				return theater.Outputs{"token": "issued-1", "ready": false}, nil
			}

			return theater.Outputs{"token": "issued-2", "ready": true}, nil
		},
	}
	useToken := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Inputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString},
			},
		},
		CheckFunc: func(args theater.Args) error {
			if got, want := args["token"], "issued-2"; got != want {
				t.Fatalf("token mismatch: got %v want %v", got, want)
			}

			return nil
		},
	}
	expectation := &testkit.ScriptedExpectation{
		CheckFunc: func(actual any) error {
			if got, ok := actual.(bool); ok && got {
				return nil
			}

			return errors.New("not ready")
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.fetch-token", fetch); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	if err := catalog.RegisterAction("action.use-token", useToken); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, expectation.Descriptor("expectation.ready")),
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := len(fetch.Calls), 2; got != want {
		t.Fatalf("fetch action call count mismatch: got %d want %d", got, want)
	}

	if got, want := len(useToken.Calls), 1; got != want {
		t.Fatalf("use-token action call count mismatch: got %d want %d", got, want)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func TestRunEventuallyParentCancellationStopsImmediately(t *testing.T) {
	t.Parallel()

	started := make(chan struct{}, 1)
	expectation := &testkit.ScriptedExpectation{CheckFunc: func(actual any) error { return nil }}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.wait", &testkit.ScriptedAction{
		RunFunc: func(args theater.Args) (theater.Outputs, error) {
			select {
			case started <- struct{}{}:
			default:
			}

			<-args["ctx_done"].(<-chan struct{})
			return theater.Outputs{}, context.Canceled
		},
		ContractValue: theater.ActionContract{
			Inputs: map[string]theater.ValueContract{
				"ctx_done": {Kind: theater.ValueKindAny},
			},
			Outputs: map[string]theater.ValueContract{
				"status": {Kind: theater.ValueKindString},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-started
		cancel()
	}()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "probe",
				Acts: []theater.ActSpec{
					{
						ID:         "wait",
						Eventually: &theater.EventuallySpec{Timeout: "1s", Interval: "1ms"},
						Action: theater.ActionSpec{
							Use:        "action.wait",
							Repeatable: true,
							With: map[string]theater.BindingSpec{
								"ctx_done": {Kind: theater.BindingKindLiteral, Value: ctx.Done()},
							},
						},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "ready",
								Subject: theater.SubjectSpec{Field: "status"},
								Assert:  theater.AssertSpec{Ref: "expectation.ready"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "probe-server", ScenarioID: "probe"},
		},
	}

	result, err := runStage(
		ctx,
		spec,
		catalog,
		matcherCatalog(t, expectation.Descriptor("expectation.ready")),
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusCanceled; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	actNode := findNodeReport(t, result.Report, theater.NodeKindAct, "stage.main/call.probe-server/act.wait")
	if actNode.Eventually == nil {
		t.Fatal("eventually report is nil")
	}

	if got, want := actNode.Eventually.TerminationReason, theater.TerminationReasonParentCanceled; got != want {
		t.Fatalf("termination reason mismatch: got %q want %q", got, want)
	}

	if got, want := actNode.Eventually.AttemptsTotal, 1; got != want {
		t.Fatalf("attempts total mismatch: got %d want %d", got, want)
	}
}

func TestRunEventuallyTerminalActionErrorStopsImmediately(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "probe",
				Acts: []theater.ActSpec{
					{
						ID:         "wait",
						Eventually: &theater.EventuallySpec{Timeout: "1s", Interval: "1ms"},
						Action:     theater.ActionSpec{Use: "action.wait", Repeatable: true},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "ready",
								Subject: theater.SubjectSpec{Field: "status"},
								Assert:  theater.AssertSpec{Ref: "expectation.ready"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "probe-server", ScenarioID: "probe"},
		},
	}

	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"status": {Kind: theater.ValueKindString},
			},
		},
		Err: testkit.TerminalError(errors.New("permanent failure")),
	}
	expectation := &testkit.ScriptedExpectation{CheckFunc: func(actual any) error { return nil }}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.wait", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, expectation.Descriptor("expectation.ready")),
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := len(action.Calls), 1; got != want {
		t.Fatalf("action call count mismatch: got %d want %d", got, want)
	}

	actNode := findNodeReport(t, result.Report, theater.NodeKindAct, "stage.main/call.probe-server/act.wait")
	if actNode.Eventually == nil {
		t.Fatal("eventually report is nil")
	}

	if got, want := actNode.Eventually.TerminationReason, theater.TerminationReasonTerminalFailure; got != want {
		t.Fatalf("termination reason mismatch: got %q want %q", got, want)
	}

	if got, want := actNode.Eventually.AttemptsTotal, 1; got != want {
		t.Fatalf("attempts total mismatch: got %d want %d", got, want)
	}
}

func TestRunEventuallyTerminalInventoryErrorStopsImmediately(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "probe",
				Acts: []theater.ActSpec{
					{
						ID:         "wait",
						Eventually: &theater.EventuallySpec{Timeout: "1s", Interval: "1ms"},
						Action:     theater.ActionSpec{Use: "action.wait", Repeatable: true},
						Properties: map[string]theater.PropertySpec{
							"token": {
								Inventory: &theater.InventoryCall{Use: "inventory.seed"},
							},
						},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "ready",
								Subject: theater.SubjectSpec{Field: "status"},
								Assert:  theater.AssertSpec{Ref: "expectation.ready"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "probe-server", ScenarioID: "probe"},
		},
	}

	inventory := &testkit.ScriptedInventory{
		ContractValue: theater.InventoryContract{
			Produces: theater.ValueContract{Kind: theater.ValueKindString},
		},
		Err: testkit.TerminalError(errors.New("permanent inventory failure")),
	}
	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"status": {Kind: theater.ValueKindString},
			},
		},
		Output: theater.Outputs{"status": "READY"},
	}
	expectation := &testkit.ScriptedExpectation{CheckFunc: func(actual any) error { return nil }}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterInventory("inventory.seed", inventory); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}
	if err := catalog.RegisterAction("action.wait", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, expectation.Descriptor("expectation.ready")),
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := len(inventory.Calls), 1; got != want {
		t.Fatalf("inventory call count mismatch: got %d want %d", got, want)
	}

	if got, want := len(action.Calls), 0; got != want {
		t.Fatalf("action call count mismatch: got %d want %d", got, want)
	}

	actNode := findNodeReport(t, result.Report, theater.NodeKindAct, "stage.main/call.probe-server/act.wait")
	if actNode.Eventually == nil {
		t.Fatal("eventually report is nil")
	}

	if got, want := actNode.Eventually.TerminationReason, theater.TerminationReasonTerminalFailure; got != want {
		t.Fatalf("termination reason mismatch: got %q want %q", got, want)
	}

	if got, want := actNode.Eventually.AttemptsTotal, 1; got != want {
		t.Fatalf("attempts total mismatch: got %d want %d", got, want)
	}

	if actNode.Eventually.FinalFailureReason == nil {
		t.Fatal("final failure reason is nil")
	}

	if got, want := actNode.Eventually.FinalFailureReason.Kind, theater.FailureKindSetup; got != want {
		t.Fatalf("final failure kind mismatch: got %q want %q", got, want)
	}
}

func TestRunEventuallyTransitionsFireOnlyAfterFinalTimeout(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "probe",
				Acts: []theater.ActSpec{
					{
						ID:         "wait-ready",
						Eventually: &theater.EventuallySpec{Timeout: "20ms", Interval: "5ms"},
						Action:     theater.ActionSpec{Use: "action.probe", Repeatable: true},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "status",
								Subject: theater.SubjectSpec{Field: "status"},
								Assert:  theater.AssertSpec{Ref: "expectation.ready"},
							},
						},
						Transitions: []theater.TransitionSpec{
							{On: theater.TransitionOnTimeout, To: "fallback"},
						},
					},
					{
						ID:     "fallback",
						Action: theater.ActionSpec{Use: "action.fallback"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "probe-server", ScenarioID: "probe"},
		},
	}

	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"status": {Kind: theater.ValueKindString},
			},
		},
		RunFunc: func(args theater.Args) (theater.Outputs, error) {
			return theater.Outputs{"status": "PENDING"}, nil
		},
	}
	fallback := &testkit.ScriptedAction{}
	expectation := &testkit.ScriptedExpectation{
		CheckFunc: func(actual any) error {
			return errors.New("status is not ready")
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.probe", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	if err := catalog.RegisterAction("action.fallback", fallback); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, expectation.Descriptor("expectation.ready")),
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := len(fallback.Calls), 1; got != want {
		t.Fatalf("fallback call count mismatch: got %d want %d", got, want)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func TestRunReturnsObservationFailureWhenExpectationSubjectIsMissing(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "token",
								Subject: theater.SubjectSpec{Field: "missing"},
								Assert:  theater.AssertSpec{Ref: "expectation.token"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	action := &testkit.ScriptedAction{
		Output: theater.Outputs{"token": "issued-token"},
	}
	expectation := &testkit.ScriptedExpectation{}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(
		context.Background(),
		spec,
		catalog,
		matcherCatalog(t, expectation.Descriptor("expectation.token")),
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	if result.Report.Failure == nil {
		t.Fatal("expected report failure")
	}

	if got, want := result.Report.Failure.Kind, theater.FailureKindDefinition; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}

	if got, want := len(expectation.Calls), 0; got != want {
		t.Fatalf("expectation call count mismatch: got %d want %d", got, want)
	}

	if got, want := len(result.Diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := result.Diagnostics[0].Code, "unknown_expectation_subject_field"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
}

func TestRunReturnsDefinitionFailureReportOnValidationDiagnostics(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
						Transitions: []theater.TransitionSpec{
							{On: theater.TransitionOnPass, To: "missing"},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	catalog := theater.NewCatalog()
	for _, use := range []string{"action.start", "action.issue", "action.fallback"} {
		if err := catalog.RegisterAction(use, &testkit.ScriptedAction{}); err != nil {
			t.Fatalf("register action %q failed: %v", use, err)
		}
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := len(result.Diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	if result.Report.Failure == nil {
		t.Fatal("definition failure must be present")
	}

	if got, want := result.Report.Failure.Kind, theater.FailureKindDefinition; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}

	if got, want := result.Report.Failure.Phase, theater.PhaseValidate; got != want {
		t.Fatalf("failure phase mismatch: got %q want %q", got, want)
	}
}

func TestRunReturnsDefinitionFailureReportForEventuallyIntervalNotShorterThanTimeout(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:         "submit",
						Eventually: &theater.EventuallySpec{Timeout: "1s", Interval: "2s"},
						Action: theater.ActionSpec{
							Use:        "action.login",
							Repeatable: true,
						},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "token",
								Subject: theater.SubjectSpec{Field: "token"},
								Assert:  theater.AssertSpec{Ref: "expectation.token"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	result, err := runStage(context.Background(), spec, theater.NewCatalog(), matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := len(result.Diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := result.Diagnostics[0].Code, "invalid_eventually_interval"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := result.Diagnostics[0].Path, "stage.main/scenario.login/act.submit/eventually/interval"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}

	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	if result.Report.Failure == nil {
		t.Fatal("definition failure must be present")
	}

	if got, want := result.Report.Failure.Kind, theater.FailureKindDefinition; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}

	if got, want := result.Report.Failure.Phase, theater.PhaseValidate; got != want {
		t.Fatalf("failure phase mismatch: got %q want %q", got, want)
	}
}

func TestRunReturnsDefinitionFailureReportForIncompatibleActExportSelector(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
						Exports: []theater.ExportSpec{
							{
								As:    "issued_token",
								Field: "body",
								Path:  "/token",
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"body": {Kind: theater.ValueKindString},
			},
		},
		RunFunc: func(args theater.Args) (theater.Outputs, error) {
			t.Fatal("action must not run when export selector is invalid at validate time")
			return nil, nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := len(result.Diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := result.Diagnostics[0].Code, "incompatible_act_export_path"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := result.Diagnostics[0].Path, "stage.main/scenario.login/act.submit/export.issued_token"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}

	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	if result.Report.Failure == nil {
		t.Fatal("definition failure must be present")
	}

	if got, want := result.Report.Failure.Kind, theater.FailureKindDefinition; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}

	if got, want := result.Report.Failure.Phase, theater.PhaseValidate; got != want {
		t.Fatalf("failure phase mismatch: got %q want %q", got, want)
	}

	if got, want := len(action.Calls), 0; got != want {
		t.Fatalf("action call count mismatch: got %d want %d", got, want)
	}
}

func TestRunReturnsDefinitionFailureReportForScenarioWithoutActs(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	result, err := runStage(context.Background(), spec, theater.NewCatalog(), matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := len(result.Diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := result.Diagnostics[0].Code, "missing_scenario_acts"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := result.Diagnostics[0].Path, "stage.main/scenario.login"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}

	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	if result.Report.Failure == nil {
		t.Fatal("definition failure must be present")
	}

	if got, want := result.Report.Failure.Kind, theater.FailureKindDefinition; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}

	if got, want := result.Report.Failure.Phase, theater.PhaseValidate; got != want {
		t.Fatalf("failure phase mismatch: got %q want %q", got, want)
	}
}

func TestRunReturnsDefinitionFailureReportForMissingLocalBindingRef(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID: "submit",
						Action: theater.ActionSpec{
							Use: "action.login",
							With: map[string]theater.BindingSpec{
								"token": {
									Kind: theater.BindingKindRef,
									Ref:  &theater.RefSpec{Name: "missing_token"},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", &testkit.ScriptedAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := len(result.Diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := result.Diagnostics[0].Code, "unresolved_binding_ref"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := result.Diagnostics[0].Path, "stage.main/scenario.login/act.submit/action/binding.token"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}

	if result.Report.Failure == nil {
		t.Fatal("definition failure must be present")
	}

	if got, want := result.Report.Failure.Kind, theater.FailureKindDefinition; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}

	if got, want := result.Report.Failure.Phase, theater.PhaseValidate; got != want {
		t.Fatalf("failure phase mismatch: got %q want %q", got, want)
	}
}

func TestRunReturnsDefinitionFailureReportForCollidingActExportName(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Inputs: map[string]theater.ValueContract{
					"token": {Kind: theater.ValueKindString},
				},
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
						Exports: []theater.ExportSpec{
							{Field: "token", As: "token"},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString},
			},
		},
		Output: theater.Outputs{
			"token": "issued-token",
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := len(result.Diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := result.Diagnostics[0].Code, "colliding_act_export_name"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := result.Diagnostics[0].Path, "stage.main/scenario.login/act.submit/export.token"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}

	if got, want := len(action.Calls), 0; got != want {
		t.Fatalf("action call count mismatch: got %d want %d", got, want)
	}

	if result.Report.Failure == nil {
		t.Fatal("definition failure must be present")
	}

	if got, want := result.Report.Failure.Kind, theater.FailureKindDefinition; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}

	if got, want := result.Report.Failure.Phase, theater.PhaseValidate; got != want {
		t.Fatalf("failure phase mismatch: got %q want %q", got, want)
	}
}

func TestRunReturnsDefinitionFailureReportForScenarioCallExportOutsideFinalScope(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "start",
						Action: theater.ActionSpec{Use: "action.start"},
						Transitions: []theater.TransitionSpec{
							{On: theater.TransitionOnPass, To: "issue"},
							{On: theater.TransitionOnFail, To: "fallback"},
						},
					},
					{
						ID:     "issue",
						Action: theater.ActionSpec{Use: "action.issue"},
						Exports: []theater.ExportSpec{
							{Field: "token", As: "issued_token"},
						},
					},
					{
						ID:     "fallback",
						Action: theater.ActionSpec{Use: "action.fallback"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{
				ID:         "login-user",
				ScenarioID: "login",
				Exports: []theater.ExportSpec{
					{Ref: &theater.RefSpec{Name: "issued_token"}, As: "final_token"},
				},
			},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.start", &testkit.ScriptedAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	if err := catalog.RegisterAction("action.issue", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	if err := catalog.RegisterAction("action.fallback", &testkit.ScriptedAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	found := false
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "unresolved_scenario_call_export_ref" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing unresolved_scenario_call_export_ref diagnostic: %v", result.Diagnostics)
	}

	if result.Report.Failure == nil {
		t.Fatal("definition failure must be present")
	}

	if got, want := result.Report.Failure.Kind, theater.FailureKindDefinition; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}

	if got, want := result.Report.Failure.Phase, theater.PhaseValidate; got != want {
		t.Fatalf("failure phase mismatch: got %q want %q", got, want)
	}
}

func TestRunReturnsSetupFailureReportBeforeStageEventsWhenCatalogIsIncomplete(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	result, err := runStage(context.Background(), spec, theater.NewCatalog(), matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	if result.Report.Failure == nil {
		t.Fatal("definition failure must be present")
	}

	if got, want := result.Report.Failure.Kind, theater.FailureKindDefinition; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}
}

func TestRunUsesFailAfterBatchStagePolicyWithRecoveryAndLaterSkips(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "register",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.register"},
					},
				},
			},
			{
				ID: "cleanup",
				Acts: []theater.ActSpec{
					{
						ID:     "revoke",
						Action: theater.ActionSpec{Use: "action.cleanup"},
					},
				},
			},
			{
				ID: "prepare",
				Acts: []theater.ActSpec{
					{
						ID:     "seed",
						Action: theater.ActionSpec{Use: "action.prepare"},
					},
				},
			},
			{
				ID: "notify",
				Acts: []theater.ActSpec{
					{
						ID:     "send",
						Action: theater.ActionSpec{Use: "action.notify"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "register-user", ScenarioID: "register"},
			{ID: "prepare-user", ScenarioID: "prepare"},
			{
				ID:         "cleanup-user",
				ScenarioID: "cleanup",
				Dependencies: []theater.ScenarioDependencySpec{
					{CallID: "register-user", When: theater.TriggerPredicateFailure},
				},
			},
			{
				ID:         "notify-user",
				ScenarioID: "notify",
				Dependencies: []theater.ScenarioDependencySpec{
					{CallID: "prepare-user"},
				},
			},
		},
	}

	registerAction := &testkit.ScriptedAction{Err: errors.New("registration failed")}
	cleanupAction := &testkit.ScriptedAction{}
	prepareAction := &testkit.ScriptedAction{}
	notifyAction := &testkit.ScriptedAction{}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.register", registerAction); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	if err := catalog.RegisterAction("action.cleanup", cleanupAction); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	if err := catalog.RegisterAction("action.prepare", prepareAction); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	if err := catalog.RegisterAction("action.notify", notifyAction); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := len(registerAction.Calls), 1; got != want {
		t.Fatalf("register call count mismatch: got %d want %d", got, want)
	}

	if got, want := len(cleanupAction.Calls), 1; got != want {
		t.Fatalf("cleanup call count mismatch: got %d want %d", got, want)
	}

	if got, want := len(prepareAction.Calls), 1; got != want {
		t.Fatalf("prepare call count mismatch: got %d want %d", got, want)
	}

	if got, want := len(notifyAction.Calls), 0; got != want {
		t.Fatalf("notify call count mismatch: got %d want %d", got, want)
	}

	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	if got, want := result.Report.Summary.TotalScenarios, 4; got != want {
		t.Fatalf("total scenarios mismatch: got %d want %d", got, want)
	}

	if got, want := result.Report.Summary.PassedScenarios, 2; got != want {
		t.Fatalf("passed scenarios mismatch: got %d want %d", got, want)
	}

	if got, want := result.Report.Summary.FailedScenarios, 1; got != want {
		t.Fatalf("failed scenarios mismatch: got %d want %d", got, want)
	}

	if got, want := result.Report.Summary.SkippedScenarios, 1; got != want {
		t.Fatalf("skipped scenarios mismatch: got %d want %d", got, want)
	}

	var skipped *theater.NodeReport
	for i := range result.Report.Nodes {
		node := &result.Report.Nodes[i]
		if node.Kind == theater.NodeKindScenario && node.ScenarioCallID == "notify-user" {
			skipped = node
			break
		}
	}

	if skipped == nil {
		t.Fatal("skipped scenario node must be present")
	}

	if got, want := skipped.Status, theater.StatusSkipped; got != want {
		t.Fatalf("skipped scenario status mismatch: got %q want %q", got, want)
	}

	if got, want := skipped.SkipReason, theater.SkipReasonStageAborted; got != want {
		t.Fatalf("skipped scenario reason mismatch: got %q want %q", got, want)
	}
}

func TestRunExecutesIndependentScenarioCalls(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "alpha",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.alpha"},
					},
				},
			},
			{
				ID: "beta",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.beta"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "alpha-user", ScenarioID: "alpha"},
			{ID: "beta-user", ScenarioID: "beta"},
		},
	}

	started := make(chan string, 2)
	release := make(chan struct{})
	done := make(chan struct {
		result theater.RunResult
		err    error
	}, 1)

	newBlockingAction := func(name string) *testkit.ScriptedAction {
		return &testkit.ScriptedAction{
			ContractValue: theater.ActionContract{
				Outputs: map[string]theater.ValueContract{
					"call": {Kind: theater.ValueKindString},
				},
			},
			RunFunc: func(args theater.Args) (theater.Outputs, error) {
				started <- name
				<-release
				return theater.Outputs{"call": name}, nil
			},
		}
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.alpha", newBlockingAction("alpha")); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	if err := catalog.RegisterAction("action.beta", newBlockingAction("beta")); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	go func() {
		result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
		done <- struct {
			result theater.RunResult
			err    error
		}{result: result, err: err}
	}()

	seen := make(map[string]struct{}, 2)
	select {
	case name := <-started:
		seen[name] = struct{}{}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first independent scenario start")
	}

	close(release)

	select {
	case outcome := <-done:
		if outcome.err != nil {
			t.Fatalf("run stage failed: %v", outcome.err)
		}

		if got, want := outcome.result.Report.Status, theater.StatusPassed; got != want {
			t.Fatalf("report status mismatch: got %q want %q", got, want)
		}

		if got, want := outcome.result.Report.Summary.PassedScenarios, 2; got != want {
			t.Fatalf("passed scenarios mismatch: got %d want %d", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stage completion")
	}

	for len(seen) < 2 {
		select {
		case name := <-started:
			seen[name] = struct{}{}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for both independent scenario starts, got %d", len(seen))
		}
	}
}

func TestRunMarksScenarioAndStageCanceledWhenActionReturnsContextCanceled(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	action := &testkit.ScriptedAction{Err: context.Canceled}
	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusCanceled; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	if result.Report.Failure != nil {
		t.Fatal("canceled report must not carry failure")
	}

	if got, want := result.Report.Summary.TotalScenarios, 1; got != want {
		t.Fatalf("total scenarios mismatch: got %d want %d", got, want)
	}

	if got, want := result.Report.Summary.CanceledScenarios, 1; got != want {
		t.Fatalf("canceled scenarios mismatch: got %d want %d", got, want)
	}
}

func TestRunFollowsTimeoutTransition(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
						Transitions: []theater.TransitionSpec{
							{On: theater.TransitionOnTimeout, To: "fallback"},
						},
					},
					{
						ID:     "fallback",
						Action: theater.ActionSpec{Use: "action.fallback"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	loginAction := &testkit.ScriptedAction{Err: context.DeadlineExceeded}
	fallbackAction := &testkit.ScriptedAction{}
	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", loginAction); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	if err := catalog.RegisterAction("action.fallback", fallbackAction); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := len(fallbackAction.Calls), 1; got != want {
		t.Fatalf("fallback call count mismatch: got %d want %d", got, want)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func TestRunFollowsCancelTransition(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
						Transitions: []theater.TransitionSpec{
							{On: theater.TransitionOnCancel, To: "cleanup"},
						},
					},
					{
						ID:     "cleanup",
						Action: theater.ActionSpec{Use: "action.cleanup"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	loginAction := &testkit.ScriptedAction{Err: context.Canceled}
	cleanupAction := &testkit.ScriptedAction{}
	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", loginAction); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	if err := catalog.RegisterAction("action.cleanup", cleanupAction); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := len(cleanupAction.Calls), 1; got != want {
		t.Fatalf("cleanup call count mismatch: got %d want %d", got, want)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func TestRunCarriesExplicitActExportsIntoNextAct(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:      "submit",
						Action:  theater.ActionSpec{Use: "action.submit"},
						Exports: []theater.ExportSpec{{Field: "token"}},
						Transitions: []theater.TransitionSpec{
							{On: theater.TransitionOnPass, To: "verify"},
						},
					},
					{
						ID: "verify",
						Action: theater.ActionSpec{
							Use: "action.verify",
							With: map[string]theater.BindingSpec{
								"token": {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "token"}},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	submitAction := &testkit.ScriptedAction{
		Output: theater.Outputs{"token": "issued-token"},
	}
	verifyAction := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Inputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString, Required: true},
			},
		},
		CheckFunc: func(args theater.Args) error {
			if got, want := args["token"], "issued-token"; got != want {
				t.Fatalf("token mismatch: got %v want %v", got, want)
			}

			return nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.submit", submitAction); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	if err := catalog.RegisterAction("action.verify", verifyAction); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := len(verifyAction.Calls), 1; got != want {
		t.Fatalf("verify call count mismatch: got %d want %d", got, want)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func TestRunExportsActPropertyRefIntoNextAct(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID: "load-runtime",
						Properties: map[string]theater.PropertySpec{
							"token": {
								Inventory: &theater.InventoryCall{Use: "inventory.token"},
							},
						},
						Action:  theater.ActionSpec{Use: "action.noop"},
						Exports: []theater.ExportSpec{{Ref: &theater.RefSpec{Name: "token"}}},
						Transitions: []theater.TransitionSpec{
							{On: theater.TransitionOnPass, To: "verify"},
						},
					},
					{
						ID: "verify",
						Action: theater.ActionSpec{
							Use: "action.verify",
							With: map[string]theater.BindingSpec{
								"token": {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "token"}},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	inventory := &testkit.ScriptedInventory{
		Output: "issued-token",
	}
	verifyAction := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Inputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString, Required: true},
			},
		},
		CheckFunc: func(args theater.Args) error {
			if got, want := args["token"], "issued-token"; got != want {
				t.Fatalf("token mismatch: got %v want %v", got, want)
			}
			return nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterInventory("inventory.token", inventory); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}
	if err := catalog.RegisterAction("action.noop", &testkit.ScriptedAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	if err := catalog.RegisterAction("action.verify", verifyAction); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := len(verifyAction.Calls), 1; got != want {
		t.Fatalf("verify call count mismatch: got %d want %d", got, want)
	}
	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func TestRunExportsActPropertyRefWithTopLevelSelector(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID: "load-runtime",
						Properties: map[string]theater.PropertySpec{
							"profile": {
								Inventory: &theater.InventoryCall{Use: "inventory.profile"},
							},
						},
						Action: theater.ActionSpec{Use: "action.noop"},
						Exports: []theater.ExportSpec{{
							As:   "profile_id",
							Ref:  &theater.RefSpec{Name: "profile"},
							Path: theater.JSONPointer("/id"),
						}},
						Transitions: []theater.TransitionSpec{
							{On: theater.TransitionOnPass, To: "verify"},
						},
					},
					{
						ID: "verify",
						Action: theater.ActionSpec{
							Use: "action.verify",
							With: map[string]theater.BindingSpec{
								"profile_id": {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "profile_id"}},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	verifyAction := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Inputs: map[string]theater.ValueContract{
				"profile_id": {Kind: theater.ValueKindString, Required: true},
			},
		},
		CheckFunc: func(args theater.Args) error {
			if got, want := args["profile_id"], "user-123"; got != want {
				t.Fatalf("profile id mismatch: got %v want %v", got, want)
			}
			return nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterInventory("inventory.profile", &testkit.ScriptedInventory{
		ContractValue: theater.InventoryContract{
			Produces: theater.ValueContract{
				Kind: theater.ValueKindObject,
				Fields: map[string]theater.ValueContract{
					"id":     {Kind: theater.ValueKindString},
					"secret": {Kind: theater.ValueKindString},
				},
			},
		},
		Output: map[string]any{"id": "user-123", "secret": "not-exported"},
	}); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}
	if err := catalog.RegisterAction("action.noop", &testkit.ScriptedAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	if err := catalog.RegisterAction("action.verify", verifyAction); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q (diagnostics=%v failure=%v)", got, want, result.Diagnostics, result.Report.Failure)
	}
}

func TestRunTheaterDSLActPropertyRefExportSelector(t *testing.T) {
	t.Parallel()

	spec, err := thtrsyntax.Parse([]byte(`stage main
scenario login
  act load-runtime
    prop profile = inventory.profile()
    do action.noop
    export profile_id = $profile | path("/id")
    on pass -> verify

  act verify
    do action.verify(profile_id: $profile_id)

call login-user = login()
`), nil)
	if err != nil {
		t.Fatalf("parse thtr failed: %v", err)
	}

	verifyAction := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Inputs: map[string]theater.ValueContract{
				"profile_id": {Kind: theater.ValueKindString, Required: true},
			},
		},
		CheckFunc: func(args theater.Args) error {
			if got, want := args["profile_id"], "user-123"; got != want {
				t.Fatalf("profile id mismatch: got %v want %v", got, want)
			}
			return nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterInventory("inventory.profile", &testkit.ScriptedInventory{
		ContractValue: theater.InventoryContract{
			Produces: theater.ValueContract{
				Kind: theater.ValueKindObject,
				Fields: map[string]theater.ValueContract{
					"id": {Kind: theater.ValueKindString},
				},
			},
		},
		Output: map[string]any{"id": "user-123"},
	}); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}
	if err := catalog.RegisterAction("action.noop", &testkit.ScriptedAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	if err := catalog.RegisterAction("action.verify", verifyAction); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func TestRunFieldExportSelectorCanReferenceActionOutput(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "find",
				Acts: []theater.ActSpec{
					{
						ID:     "select",
						Action: theater.ActionSpec{Use: "action.find"},
						Exports: []theater.ExportSpec{{
							As:    "selected",
							Field: "items",
							Through: []theater.ThroughStepSpec{{
								Pick: &theater.PickStepSpec{
									At: theater.JSONPointer("/id"),
									Equals: theater.BindingSpec{
										Kind: theater.BindingKindRef,
										Ref:  &theater.RefSpec{Name: "target"},
									},
								},
							}},
						}},
						Transitions: []theater.TransitionSpec{
							{On: theater.TransitionOnPass, To: "verify"},
						},
					},
					{
						ID: "verify",
						Action: theater.ActionSpec{
							Use: "action.verify",
							With: map[string]theater.BindingSpec{
								"selected": {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "selected"}},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "run", ScenarioID: "find"},
		},
	}

	verifyAction := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Inputs: map[string]theater.ValueContract{
				"selected": {Kind: theater.ValueKindObject, Required: true},
			},
		},
		CheckFunc: func(args theater.Args) error {
			selected, ok := args["selected"].(map[string]any)
			if !ok {
				t.Fatalf("selected value type mismatch: got %T", args["selected"])
			}
			if got, want := selected["id"], "b"; got != want {
				t.Fatalf("selected id mismatch: got %v want %v", got, want)
			}
			return nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.find", &testkit.ScriptedAction{
		Output: theater.Outputs{
			"target": "b",
			"items": []any{
				map[string]any{"id": "a"},
				map[string]any{"id": "b"},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	if err := catalog.RegisterAction("action.verify", verifyAction); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q (diagnostics=%v failure=%v)", got, want, result.Diagnostics, result.Report.Failure)
	}
}

func TestRunReportsPickWhereExpectationSelectorFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		target    string
		items     []any
		wantCause string
	}{
		{
			name:   "no match",
			target: "item-404",
			items: []any{
				map[string]any{"id": "item-100", "kind": "sample", "status": "ready"},
			},
			wantCause: "pick matched no items",
		},
		{
			name:   "later selector step fails",
			target: "item-100",
			items: []any{
				map[string]any{"id": "item-100", "kind": "sample", "label": "alpha"},
			},
			wantCause: `path "/status" target is missing`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			spec := theater.StageSpec{
				ID: "main",
				Scenarios: []theater.ScenarioSpec{{
					ID: "select-item",
					Acts: []theater.ActSpec{{
						ID:     "fetch",
						Action: theater.ActionSpec{Use: "action.find"},
						Expectations: []theater.ExpectationSpec{{
							ID: "selected-ready",
							Subject: theater.SubjectSpec{
								Field: "items",
								Through: []theater.ThroughStepSpec{
									pickWhereIDEqualsRef("target"),
									{Path: theater.JSONPointer("/status")},
								},
							},
							Assert: theater.AssertSpec{
								Ref: builtinexpectation.EqualRef,
								Args: map[string]theater.BindingSpec{
									"expected": {Kind: theater.BindingKindLiteral, Value: "ready"},
								},
							},
						}},
					}},
				}},
				ScenarioCalls: []theater.ScenarioCallSpec{
					{ID: "run", ScenarioID: "select-item"},
				},
			}

			catalog := theater.NewCatalog()
			if err := catalog.RegisterAction("action.find", scriptedItemAction(testCase.target, testCase.items)); err != nil {
				t.Fatalf("register action failed: %v", err)
			}

			result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t, builtinexpectation.Descriptors()...))
			if err != nil {
				t.Fatalf("run stage failed: %v", err)
			}

			requireReportFailure(t, result.Report, theater.FailureKindObservation, "observation failed", "stage.main/call.run/act.fetch/expectation.selected-ready", testCase.wantCause)
			expectationNode := findNodeReport(t, result.Report, theater.NodeKindExpectation, "stage.main/call.run/act.fetch/expectation.selected-ready")
			requireNodeFailure(t, expectationNode, theater.FailureKindObservation, "observation failed", testCase.wantCause)
		})
	}
}

func TestRunReportsPickWhereActExportSelectorFailure(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "select-item",
			Acts: []theater.ActSpec{{
				ID:     "fetch",
				Action: theater.ActionSpec{Use: "action.find"},
				Exports: []theater.ExportSpec{{
					As:    "selected_status",
					Field: "items",
					Through: []theater.ThroughStepSpec{
						pickWhereIDEqualsRef("target"),
						{Path: theater.JSONPointer("/status")},
					},
				}},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "run", ScenarioID: "select-item"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.find", scriptedItemAction("item-100", []any{
		map[string]any{"id": "item-100", "kind": "sample", "status": "ready"},
		map[string]any{"id": "item-100", "kind": "sample", "status": "blocked"},
	})); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t, builtinexpectation.Descriptors()...))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	requireReportFailure(t, result.Report, theater.FailureKindObservation, "act export failed", "stage.main/call.run/act.fetch/export.selected_status", "pick matched multiple items")
	actionNode := findNodeReport(t, result.Report, theater.NodeKindAction, "stage.main/call.run/act.fetch/action")
	if got, want := actionNode.Status, theater.StatusPassed; got != want {
		t.Fatalf("action node status mismatch: got %q want %q", got, want)
	}
	actNode := findNodeReport(t, result.Report, theater.NodeKindAct, "stage.main/call.run/act.fetch")
	requireNodeFailure(t, actNode, theater.FailureKindObservation, "act export failed", `export "selected_status": pick matched multiple items`)
}

func TestRunReportsPickWhereLogSelectorFailure(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "select-item",
			Acts: []theater.ActSpec{{
				ID:     "fetch",
				Action: theater.ActionSpec{Use: "action.find"},
				Logs: []theater.LogSpec{{
					ID:       "selected-status",
					Value:    theater.LogValueSpec{Field: "items", Through: []theater.ThroughStepSpec{pickWhereIDEqualsRef("target")}},
					Required: true,
				}},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "run", ScenarioID: "select-item"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.find", scriptedItemAction("item-100", map[string]any{"id": "item-100"})); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t, builtinexpectation.Descriptors()...))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	requireReportFailure(t, result.Report, theater.FailureKindObservation, "log evaluation failed", "stage.main/call.run/act.fetch/log.selected-status", "pick requires list input")
	log := findLogRecord(t, result.Report, "selected-status")
	if got, want := log.Status, theater.LogStatusError; got != want {
		t.Fatalf("log status mismatch: got %q want %q", got, want)
	}
	requireFailure(t, log.Failure, theater.FailureKindObservation, "log evaluation failed", "pick requires list input")
}

func TestRunCoalesceUsesFallbackForOmittedOptionalInput(t *testing.T) {
	t.Parallel()

	verifyAction := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Inputs: map[string]theater.ValueContract{
				"nickname": {Kind: theater.ValueKindString, Required: true},
			},
		},
		CheckFunc: func(args theater.Args) error {
			if got, want := args["nickname"], "guest"; got != want {
				t.Fatalf("nickname mismatch: got %v want %v", got, want)
			}
			return nil
		},
	}

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Inputs: map[string]theater.ValueContract{
					"nickname": {Kind: theater.ValueKindString},
				},
				Acts: []theater.ActSpec{
					{
						ID: "verify",
						Action: theater.ActionSpec{
							Use: "action.verify",
							With: map[string]theater.BindingSpec{
								"nickname": {
									Kind: theater.BindingKindCoalesce,
									Candidates: []theater.BindingSpec{
										{Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "nickname"}},
										{Kind: theater.BindingKindLiteral, Value: "guest"},
									},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.verify", verifyAction); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func TestRunEventuallyExportsPassingAttemptActPropertyRef(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:         "load-runtime",
						Eventually: &theater.EventuallySpec{Timeout: "100ms", Interval: "1ms"},
						Properties: map[string]theater.PropertySpec{
							"token": {
								Inventory: &theater.InventoryCall{Use: "inventory.token"},
							},
						},
						Action: theater.ActionSpec{Use: "action.status", Repeatable: true},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "ready",
								Subject: theater.SubjectSpec{Field: "ready"},
								Assert:  theater.AssertSpec{Ref: "expectation.ready"},
							},
						},
						Exports: []theater.ExportSpec{{Ref: &theater.RefSpec{Name: "token"}}},
						Transitions: []theater.TransitionSpec{
							{On: theater.TransitionOnPass, To: "verify"},
						},
					},
					{
						ID: "verify",
						Action: theater.ActionSpec{
							Use: "action.verify",
							With: map[string]theater.BindingSpec{
								"token": {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "token"}},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	var inventoryCalls int
	var statusCalls int
	inventory := &testkit.ScriptedInventory{
		ContractValue: theater.InventoryContract{
			Produces: theater.ValueContract{Kind: theater.ValueKindString},
		},
		AcquireFunc: func(theater.InventoryRequest) (any, error) {
			inventoryCalls++
			return "attempt-" + strconv.Itoa(inventoryCalls), nil
		},
	}
	statusAction := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"ready": {Kind: theater.ValueKindBool},
			},
		},
		RunFunc: func(theater.Args) (theater.Outputs, error) {
			statusCalls++
			return theater.Outputs{"ready": statusCalls >= 2}, nil
		},
	}
	verifyAction := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Inputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString, Required: true},
			},
		},
		CheckFunc: func(args theater.Args) error {
			if got, want := args["token"], "attempt-2"; got != want {
				t.Fatalf("token mismatch: got %v want %v", got, want)
			}
			return nil
		},
	}
	expectation := &testkit.ScriptedExpectation{
		CheckFunc: func(actual any) error {
			if got, ok := actual.(bool); ok && got {
				return nil
			}

			return errors.New("not ready")
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterInventory("inventory.token", inventory); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}
	if err := catalog.RegisterAction("action.status", statusAction); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	if err := catalog.RegisterAction("action.verify", verifyAction); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t, expectation.Descriptor("expectation.ready")))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf(
			"report status mismatch: got %q want %q (diagnostics=%v failure=%v inventory_calls=%d status_calls=%d)",
			got,
			want,
			result.Diagnostics,
			result.Report.Failure,
			inventoryCalls,
			statusCalls,
		)
	}
	if got, want := len(verifyAction.Calls), 1; got != want {
		t.Fatalf("verify call count mismatch: got %d want %d", got, want)
	}
}

func TestRunBindsAndExportsBetweenScenarioCalls(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "register",
				Acts: []theater.ActSpec{
					{
						ID:      "submit",
						Action:  theater.ActionSpec{Use: "action.register"},
						Exports: []theater.ExportSpec{{Field: "token"}},
					},
				},
			},
			{
				ID: "login",
				Inputs: map[string]theater.ValueContract{
					"token": {Kind: theater.ValueKindString, Required: true},
				},
				Acts: []theater.ActSpec{
					{
						ID: "verify",
						Action: theater.ActionSpec{
							Use: "action.login",
							With: map[string]theater.BindingSpec{
								"token": {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "token"}},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{
				ID:         "register-user",
				ScenarioID: "register",
				Exports: []theater.ExportSpec{
					{Ref: &theater.RefSpec{Name: "token"}, As: "issued_token"},
				},
			},
			{
				ID:         "login-user",
				ScenarioID: "login",
				Dependencies: []theater.ScenarioDependencySpec{
					{CallID: "register-user"},
				},
				Bindings: map[string]theater.BindingSpec{
					"token": {
						Kind: theater.BindingKindRef,
						Ref:  &theater.RefSpec{Name: "issued_token"},
					},
				},
			},
		},
	}

	registerAction := &testkit.ScriptedAction{
		Output: theater.Outputs{"token": "issued-token"},
	}
	loginAction := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Inputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString, Required: true},
			},
		},
		CheckFunc: func(args theater.Args) error {
			if got, want := args["token"], "issued-token"; got != want {
				t.Fatalf("token mismatch: got %v want %v", got, want)
			}

			return nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.register", registerAction); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	if err := catalog.RegisterAction("action.login", loginAction); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := len(loginAction.Calls), 1; got != want {
		t.Fatalf("login call count mismatch: got %d want %d", got, want)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func TestRunReportsPickWhereScenarioCallExportSelectorFailure(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "collect-items",
			Acts: []theater.ActSpec{{
				ID:     "fetch",
				Action: theater.ActionSpec{Use: "action.find"},
				Exports: []theater.ExportSpec{
					{Field: "target"},
					{Field: "items"},
				},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{
			ID:         "collect",
			ScenarioID: "collect-items",
			Exports: []theater.ExportSpec{{
				As:  "selected_status",
				Ref: &theater.RefSpec{Name: "items"},
				Through: []theater.ThroughStepSpec{
					pickWhereIDEqualsRef("target"),
					{Path: theater.JSONPointer("/status")},
				},
			}},
		}},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.find", scriptedItemAction("item-100", []any{
		map[string]any{"id": "item-100", "kind": "sample", "label": "alpha"},
	})); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t, builtinexpectation.Descriptors()...))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	requireReportFailure(t, result.Report, theater.FailureKindObservation, "scenario export failed", "stage.main/call.collect/export.selected_status", `path "/status" target is missing`)
	actionNode := findNodeReport(t, result.Report, theater.NodeKindAction, "stage.main/call.collect/act.fetch/action")
	if got, want := actionNode.Status, theater.StatusPassed; got != want {
		t.Fatalf("action node status mismatch: got %q want %q", got, want)
	}
	actNode := findNodeReport(t, result.Report, theater.NodeKindAct, "stage.main/call.collect/act.fetch")
	if got, want := actNode.Status, theater.StatusPassed; got != want {
		t.Fatalf("act node status mismatch: got %q want %q", got, want)
	}
	scenarioNode := findNodeReport(t, result.Report, theater.NodeKindScenario, "stage.main/call.collect")
	requireNodeFailure(t, scenarioNode, theater.FailureKindObservation, "scenario export failed", `export "selected_status": path "/status" target is missing`)
}

func TestRunResolvesActPropertiesBeforeAction(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID: "submit",
						Action: theater.ActionSpec{
							Use: "action.submit",
							With: map[string]theater.BindingSpec{
								"answer": {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "answer"}},
							},
						},
						Properties: map[string]theater.PropertySpec{
							"answer": {
								Inventory: &theater.InventoryCall{
									Use: "inventory.seed",
									With: map[string]theater.BindingSpec{
										"value": {Kind: theater.BindingKindLiteral, Value: 21},
									},
								},
								Decorators: []theater.DecoratorSpec{
									{Use: "math.double"},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Inputs: map[string]theater.ValueContract{
				"answer": {Kind: theater.ValueKindNumber, Required: true},
			},
		},
		CheckFunc: func(args theater.Args) error {
			if got, want := args["answer"], 42; got != want {
				t.Fatalf("answer mismatch: got %v want %v", got, want)
			}

			return nil
		},
	}
	inventory := &testkit.ScriptedInventory{
		ContractValue: theater.InventoryContract{
			Args: []theater.ArgSpec{
				{
					Name:     "value",
					Accepts:  theater.ValueContract{Kinds: theater.NewValueKindSet(theater.ValueKindNumber)},
					Required: true,
				},
			},
			Produces: theater.ValueContract{Kinds: theater.NewValueKindSet(theater.ValueKindNumber)},
		},
		AcquireFunc: func(request theater.InventoryRequest) (any, error) {
			if got, want := request.Paths.PropertyPath, "stage.main/scenario.login/act.submit/property.answer"; got != want {
				t.Fatalf("property path mismatch: got %q want %q", got, want)
			}

			if got, want := len(request.Args), 1; got != want {
				t.Fatalf("inventory arg count mismatch: got %d want %d", got, want)
			}

			return request.Args["value"], nil
		},
	}
	decorator := &testkit.ScriptedDecorator{
		ContractValue: theater.DecoratorContract{
			Accepts:  theater.ValueContract{Kind: theater.ValueKindNumber},
			Produces: theater.ValueContract{Kind: theater.ValueKindNumber},
		},
		TransformFunc: func(value any) (any, error) {
			typed, ok := value.(int)
			if !ok {
				t.Fatalf("decorator input type mismatch: got %T", value)
			}

			return typed * 2, nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.submit", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	if err := catalog.RegisterInventory("inventory.seed", inventory); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}

	if err := catalog.RegisterDecorator("math.double", decorator.Definition()); err != nil {
		t.Fatalf("register decorator failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := len(action.Calls), 1; got != want {
		t.Fatalf("action call count mismatch: got %d want %d", got, want)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func TestRunResolvesActPropertyValueBindingWithEnvCoalesce(t *testing.T) {
	unsetName := "THEATER_TEST_PROPERTY_VALUE_UNSET"
	emptyName := "THEATER_TEST_PROPERTY_VALUE_EMPTY"
	_ = os.Unsetenv(unsetName)
	t.Setenv(emptyName, "")

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "login",
			Acts: []theater.ActSpec{{
				ID: "submit",
				Properties: map[string]theater.PropertySpec{
					"fallback": {
						Value: &theater.BindingSpec{
							Kind: theater.BindingKindCoalesce,
							Candidates: []theater.BindingSpec{
								{Kind: theater.BindingKindEnv, Env: unsetName},
								{Kind: theater.BindingKindLiteral, Value: "generated@example.test"},
							},
						},
					},
					"empty": {
						Value: &theater.BindingSpec{
							Kind: theater.BindingKindCoalesce,
							Candidates: []theater.BindingSpec{
								{Kind: theater.BindingKindEnv, Env: emptyName},
								{Kind: theater.BindingKindLiteral, Value: "fallback"},
							},
						},
					},
				},
				Action: theater.ActionSpec{
					Use: "action.generate",
					With: map[string]theater.BindingSpec{
						"outputs": {
							Kind: theater.BindingKindObject,
							Object: map[string]theater.BindingSpec{
								"fallback": {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "fallback"}},
								"empty":    {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "empty"}},
							},
						},
					},
				},
				Expectations: []theater.ExpectationSpec{
					{
						ID:      "fallback",
						Subject: theater.SubjectSpec{Field: "values", Path: "/fallback"},
						Assert: theater.AssertSpec{
							Ref: builtinexpectation.EqualRef,
							Args: map[string]theater.BindingSpec{
								"expected": {Kind: theater.BindingKindLiteral, Value: "generated@example.test"},
							},
						},
					},
					{
						ID:      "empty",
						Subject: theater.SubjectSpec{Field: "values", Path: "/empty"},
						Assert: theater.AssertSpec{
							Ref: builtinexpectation.EqualRef,
							Args: map[string]theater.BindingSpec{
								"expected": {Kind: theater.BindingKindLiteral, Value: ""},
							},
						},
					},
				},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "login-user", ScenarioID: "login"}},
	}

	catalog, matchers, err := newBuiltins()
	if err != nil {
		t.Fatalf("new builtins failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matchers)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}
	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func TestRunTreatsEnvPropertyValueAsSecret(t *testing.T) {
	t.Setenv("THEATER_TEST_SECRET_TOKEN", "issued-token")

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "login",
			Acts: []theater.ActSpec{{
				ID: "submit",
				Properties: map[string]theater.PropertySpec{
					"token": {
						Value: &theater.BindingSpec{
							Kind: theater.BindingKindEnv,
							Env:  "THEATER_TEST_SECRET_TOKEN",
						},
					},
				},
				Action: theater.ActionSpec{
					Use: "action.submit",
					With: map[string]theater.BindingSpec{
						"token": {
							Kind: theater.BindingKindRef,
							Ref:  &theater.RefSpec{Name: "token"},
						},
					},
				},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "login-user", ScenarioID: "login"}},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Inputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString},
			},
		},
		RunFunc: func(args theater.Args) (theater.Outputs, error) {
			secret, ok := args["token"].(theater.Secret)
			if !ok {
				t.Fatalf("token arg must be protected as secret, got %T", args["token"])
			}
			if got, want := secret.Reveal(), any("issued-token"); got != want {
				t.Fatalf("secret token reveal mismatch: got %#v want %#v", got, want)
			}
			return theater.Outputs{}, nil
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}
	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func TestRunResolvesHTTPInventoryThroughJSONDecoratorBeforeAction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.Header.Get("X-Token"), "issued-token"; got != want {
			t.Fatalf("request header mismatch: got %q want %q", got, want)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"email":"alice@example.com","role":"admin"}`))
	}))
	defer server.Close()

	t.Setenv("THEATER_API_TOKEN", "issued-token")

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID: "submit",
						Action: theater.ActionSpec{
							Use: "action.submit",
							With: map[string]theater.BindingSpec{
								"profile":   {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "profile"}},
								"api_token": {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "api_token"}},
							},
						},
						Properties: map[string]theater.PropertySpec{
							"api_token": {
								Inventory: &theater.InventoryCall{
									Use: builtininventory.EnvRef,
									With: map[string]theater.BindingSpec{
										"name": {Kind: theater.BindingKindLiteral, Value: "THEATER_API_TOKEN"},
									},
								},
							},
							"profile": {
								Inventory: &theater.InventoryCall{
									Use: builtininventory.HTTPGetRef,
									With: map[string]theater.BindingSpec{
										"url": {Kind: theater.BindingKindLiteral, Value: server.URL},
										"headers": {
											Kind: theater.BindingKindObject,
											Object: map[string]theater.BindingSpec{
												"X-Token": {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "api_token"}},
											},
										},
									},
								},
								Decorators: []theater.DecoratorSpec{
									{Use: builtindecorator.JSONRef},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Inputs: map[string]theater.ValueContract{
				"profile":   {Kind: theater.ValueKindObject, Required: true},
				"api_token": {Kind: theater.ValueKindString, Required: true},
			},
		},
		CheckFunc: func(args theater.Args) error {
			profile, ok := args["profile"].(map[string]any)
			if !ok {
				t.Fatalf("profile type mismatch: got %T", args["profile"])
			}

			if got, want := profile["email"], "alice@example.com"; got != want {
				t.Fatalf("profile email mismatch: got %v want %v", got, want)
			}

			if got, want := profile["role"], "admin"; got != want {
				t.Fatalf("profile role mismatch: got %v want %v", got, want)
			}

			if got, want := args["api_token"], "issued-token"; got != want {
				t.Fatalf("api token mismatch: got %v want %v", got, want)
			}

			return nil
		},
	}

	catalog, matchers, err := newBuiltins()
	if err != nil {
		t.Fatalf("new built-ins failed: %v", err)
	}

	if err := catalog.RegisterAction("action.submit", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matchers)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := len(action.Calls), 1; got != want {
		t.Fatalf("action call count mismatch: got %d want %d", got, want)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func TestRunFailsWhenPropertyResolutionReturnsError(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.submit"},
						Properties: map[string]theater.PropertySpec{
							"answer": {
								Inventory: &theater.InventoryCall{Use: "inventory.seed"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	action := &testkit.ScriptedAction{}
	inventory := &testkit.ScriptedInventory{
		Err: errors.New("inventory failed"),
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.submit", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	if err := catalog.RegisterInventory("inventory.seed", inventory); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := len(action.Calls), 0; got != want {
		t.Fatalf("action call count mismatch: got %d want %d", got, want)
	}

	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	if result.Report.Failure == nil {
		t.Fatal("report failure must be present")
	}

	if got, want := result.Report.Failure.Kind, theater.FailureKindSetup; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}
}

func TestRunReusesSingleActionResponseForStatusAndDecodedBodyAssertions(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "notifications",
			Acts: []theater.ActSpec{{
				ID:     "fetch",
				Action: theater.ActionSpec{Use: "action.http"},
				Expectations: []theater.ExpectationSpec{
					{
						ID:      "status",
						Subject: theater.SubjectSpec{Field: "status_code"},
						Assert: theater.AssertSpec{
							Ref: builtinexpectation.EqualRef,
							Args: map[string]theater.BindingSpec{
								"expected": {Kind: theater.BindingKindLiteral, Value: 200},
							},
						},
					},
					{
						ID: "receiver-present",
						Subject: theater.SubjectSpec{
							Field:  "body",
							Decode: theater.DecodeJSON,
							Path:   "/data",
						},
						Assert: theater.AssertSpec{
							Ref: builtinexpectation.HasItemRef,
							Args: map[string]theater.BindingSpec{
								"where": {
									Kind: theater.BindingKindLiteral,
									Value: []any{
										map[string]any{
											"subject": map[string]any{"path": "/receiverAddress"},
											"assert": map[string]any{
												"ref": builtinexpectation.EqualRef,
												"args": map[string]any{
													"expected": "+13146235623",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "notifications-user", ScenarioID: "notifications"}},
	}

	var calls atomic.Int32
	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"status_code": {Kind: theater.ValueKindNumber},
				"body":        {Kind: theater.ValueKindString},
			},
		},
		RunFunc: func(theater.Args) (theater.Outputs, error) {
			calls.Add(1)
			return theater.Outputs{
				"status_code": 200,
				"body":        `{"data":[{"receiverAddress":"+13146235623"}]}`,
			}, nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.http", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t, builtinexpectation.Descriptors()...))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	if got, want := calls.Load(), int32(1); got != want {
		t.Fatalf("action call count mismatch: got %d want %d", got, want)
	}
}

func TestRunAllowsCollectionAssertionOverCurrentActPropertyValue(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "notifications",
			Acts: []theater.ActSpec{{
				ID: "fetch",
				Properties: map[string]theater.PropertySpec{
					"notifications_json": {
						Inventory: &theater.InventoryCall{Use: "inventory.notifications"},
						Decorators: []theater.DecoratorSpec{
							{Use: builtindecorator.JSONRef},
						},
					},
				},
				Action: theater.ActionSpec{Use: "action.noop"},
				Expectations: []theater.ExpectationSpec{
					{
						ID: "receiver-present",
						Subject: theater.SubjectSpec{
							From: theater.SubjectFromProperty,
							Ref:  "notifications_json",
							Path: "/data",
						},
						Assert: theater.AssertSpec{
							Ref: builtinexpectation.HasItemRef,
							Args: map[string]theater.BindingSpec{
								"where": {
									Kind: theater.BindingKindLiteral,
									Value: []any{
										map[string]any{
											"subject": map[string]any{"path": "/receiverAddress"},
											"assert": map[string]any{
												"ref": builtinexpectation.EqualRef,
												"args": map[string]any{
													"expected": "+13146235623",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "notifications-user", ScenarioID: "notifications"}},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.noop", &testkit.ScriptedAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	if err := catalog.RegisterInventory("inventory.notifications", &testkit.ScriptedInventory{
		ContractValue: theater.InventoryContract{
			Produces: theater.ValueContract{Kind: theater.ValueKindString},
		},
		Output: `{"data":[{"receiverAddress":"+13146235623"}]}`,
	}); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}

	if err := builtindecorator.Register(catalog); err != nil {
		t.Fatalf("register decorators failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t, builtinexpectation.Descriptors()...))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func TestRunSupportsCanonicalNotMatcher(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "notifications",
			Acts: []theater.ActSpec{{
				ID:     "fetch",
				Action: theater.ActionSpec{Use: "action.http"},
				Expectations: []theater.ExpectationSpec{{
					ID:      "not-server-error",
					Subject: theater.SubjectSpec{Field: "status_code"},
					Assert: theater.AssertSpec{
						Ref: builtinexpectation.NotRef,
						Args: map[string]theater.BindingSpec{
							"assert": {
								Kind: theater.BindingKindLiteral,
								Value: map[string]any{
									"ref": builtinexpectation.GTERef,
									"args": map[string]any{
										"expected": 500,
									},
								},
							},
						},
					},
				}},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "notifications-user", ScenarioID: "notifications"}},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.http", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"status_code": {Kind: theater.ValueKindNumber},
			},
		},
		Output: theater.Outputs{
			"status_code": 404,
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matcherCatalog(t, builtinexpectation.Descriptors()...))
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func findNodeReport(t *testing.T, report theater.Report, kind theater.NodeKind, path string) theater.NodeReport {
	t.Helper()

	for i := range report.Nodes {
		node := report.Nodes[i]
		if node.Kind == kind && node.Path == path {
			return node
		}
	}

	t.Fatalf("node %q at path %q not found", kind, path)
	return theater.NodeReport{}
}

func findLogRecord(t *testing.T, report theater.Report, id string) theater.LogRecord {
	t.Helper()

	for _, log := range report.Logs {
		if log.ID == id {
			return log
		}
	}

	t.Fatalf("log %q not found", id)
	return theater.LogRecord{}
}

func scriptedItemAction(target string, items any) *testkit.ScriptedAction {
	return &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"target": {Kind: theater.ValueKindString},
				"items":  {Kind: theater.ValueKindAny},
			},
		},
		Output: theater.Outputs{
			"target": target,
			"items":  items,
		},
	}
}

func pickWhereIDEqualsRef(ref string) theater.ThroughStepSpec {
	return theater.ThroughStepSpec{
		Pick: &theater.PickStepSpec{
			Where: []theater.PickWhereClauseSpec{{
				Subject: theater.RelativeSubjectSpec{Path: theater.JSONPointer("/id")},
				Assert: theater.AssertSpec{
					Ref: builtinexpectation.EqualRef,
					Args: map[string]theater.BindingSpec{
						"expected": {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: ref}},
					},
				},
			}},
		},
	}
}

func requireReportFailure(
	t *testing.T,
	report theater.Report,
	kind theater.FailureKind,
	summary string,
	at string,
	cause string,
) {
	t.Helper()

	if got, want := report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	requireFailure(t, report.Failure, kind, summary, cause)
	if got, want := report.Failure.At, at; got != want {
		t.Fatalf("failure path mismatch: got %q want %q", got, want)
	}
}

func requireNodeFailure(t *testing.T, node theater.NodeReport, kind theater.FailureKind, summary string, cause string) {
	t.Helper()

	if got, want := node.Status, theater.StatusFailed; got != want {
		t.Fatalf("node status mismatch: got %q want %q", got, want)
	}
	requireFailure(t, node.Failure, kind, summary, cause)
}

func requireFailure(t *testing.T, failure *theater.Failure, kind theater.FailureKind, summary string, cause string) {
	t.Helper()

	if failure == nil {
		t.Fatal("failure must be present")
	}
	if got, want := failure.Kind, kind; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}
	if got, want := failure.Summary, summary; got != want {
		t.Fatalf("failure summary mismatch: got %q want %q", got, want)
	}
	if got := failure.Message(); !strings.Contains(got, cause) {
		t.Fatalf("failure cause mismatch: got %q want contains %q", got, cause)
	}
}
