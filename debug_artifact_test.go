package theater

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDebugArtifactSinkWritesNDJSONRecords(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "debug.ndjson")
	sink, err := openDebugArtifactSink(path)
	if err != nil {
		t.Fatalf("open artifact sink failed: %v", err)
	}
	defer func() {
		if err := sink.Close(); err != nil {
			t.Fatalf("close artifact sink failed: %v", err)
		}
	}()

	pauseSeq, err := sink.WritePause(context.Background(), "boundary", "bp.action", debugBoundaryState{
		Ref: debugBoundaryRef{
			ScenarioCallID: "first",
			ScenarioPath:   "stage.main/call.first",
			Path:           "stage.main/call.first/act.observe/action",
			Kind:           debugBoundaryKindAction,
			Phase:          debugBoundaryPhaseBefore,
			Attempt:        1,
			SourceSpan:     &SourceRef{File: "login.thtr", Line: 5, Column: 7},
		},
		Status: StatusFailed,
		Failure: &Failure{
			Kind:    FailureKindAction,
			Phase:   PhaseRun,
			At:      "stage.main/call.first/act.observe/action",
			Summary: "action failed",
			Cause:   errors.New("boom"),
		},
		Scope: debugSnapshotSection{
			Fields: []debugSnapshotField{{
				Key:        "token",
				Origin:     "scope.current",
				SourceSpan: &SourceRef{File: "login.thtr", Line: 6, Column: 9},
				Value: debugSafeValue{
					Kind: "string",
					Text: "issued-token",
				},
			}},
		},
		Scheduler: debugSchedulerSummary{
			FocusedLane: "stage.main/call.first",
			Active:      1,
			Ready:       1,
			Blocked:     0,
			ReadyPaths:  []string{"stage.main/call.first"},
		},
	})
	if err != nil {
		t.Fatalf("write pause failed: %v", err)
	}
	if got, want := pauseSeq, uint64(1); got != want {
		t.Fatalf("pause seq mismatch: got %d want %d", got, want)
	}

	resumeSeq, err := sink.WriteResume(context.Background(), pauseSeq, "continue")
	if err != nil {
		t.Fatalf("write resume failed: %v", err)
	}
	if got, want := resumeSeq, uint64(2); got != want {
		t.Fatalf("resume seq mismatch: got %d want %d", got, want)
	}

	summarySeq, err := sink.WriteSummary(context.Background(), debugArtifactSessionSummary{Records: 2})
	if err != nil {
		t.Fatalf("write summary failed: %v", err)
	}
	if got, want := summarySeq, uint64(3); got != want {
		t.Fatalf("summary seq mismatch: got %d want %d", got, want)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read artifact file failed: %v", err)
	}

	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	if got, want := len(lines), 3; got != want {
		t.Fatalf("line count mismatch: got %d want %d", got, want)
	}

	var pauseRecord debugArtifactRecord
	if err := json.Unmarshal(lines[0], &pauseRecord); err != nil {
		t.Fatalf("decode pause record failed: %v", err)
	}
	if got, want := pauseRecord.Kind, debugArtifactKindPause; got != want {
		t.Fatalf("pause kind mismatch: got %q want %q", got, want)
	}
	if got, want := pauseRecord.Pause.Reason, "boundary"; got != want {
		t.Fatalf("pause reason mismatch: got %q want %q", got, want)
	}
	if got, want := pauseRecord.Pause.Breakpoint, "bp.action"; got != want {
		t.Fatalf("pause breakpoint mismatch: got %q want %q", got, want)
	}
	if got, want := pauseRecord.Pause.Snapshot.Ref.Path, "stage.main/call.first/act.observe/action"; got != want {
		t.Fatalf("pause snapshot path mismatch: got %q want %q", got, want)
	}
	requireSourceRef(t, pauseRecord.Pause.Snapshot.Ref.SourceSpan, "login.thtr", 5, 7)
	requireSourceRef(t, pauseRecord.Pause.Snapshot.Scope.Fields[0].SourceSpan, "login.thtr", 6, 9)
	if pauseRecord.Pause.Snapshot.Failure == nil {
		t.Fatal("pause failure snapshot is nil")
	}
	if got, want := pauseRecord.Pause.Snapshot.Failure.Cause, "boom"; got != want {
		t.Fatalf("pause failure cause mismatch: got %q want %q", got, want)
	}

	var resumeRecord debugArtifactRecord
	if err := json.Unmarshal(lines[1], &resumeRecord); err != nil {
		t.Fatalf("decode resume record failed: %v", err)
	}
	if got, want := resumeRecord.Kind, debugArtifactKindResume; got != want {
		t.Fatalf("resume kind mismatch: got %q want %q", got, want)
	}
	if got, want := resumeRecord.Resume.PauseSeq, pauseSeq; got != want {
		t.Fatalf("resume pause seq mismatch: got %d want %d", got, want)
	}
	if got, want := resumeRecord.Resume.Command, "continue"; got != want {
		t.Fatalf("resume command mismatch: got %q want %q", got, want)
	}

	var summaryRecord debugArtifactRecord
	if err := json.Unmarshal(lines[2], &summaryRecord); err != nil {
		t.Fatalf("decode summary record failed: %v", err)
	}
	if got, want := summaryRecord.Kind, debugArtifactKindSummary; got != want {
		t.Fatalf("summary kind mismatch: got %q want %q", got, want)
	}
	if got, want := summaryRecord.Summary.Records, uint64(2); got != want {
		t.Fatalf("summary record count mismatch: got %d want %d", got, want)
	}
}

func TestRunDebugArtifactSnapshotsIncludeSourceSpans(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "runtime.debug.ndjson")
	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{{
			ID: "login",
			Inputs: map[string]ValueContract{
				"username": {Kind: ValueKindString},
			},
			Acts: []ActSpec{{
				ID: "submit",
				Action: ActionSpec{
					Use: "action.login",
					With: map[string]BindingSpec{
						"username": {
							Kind:       BindingKindRef,
							Ref:        &RefSpec{Name: "username"},
							SourceSpan: &SourceRef{File: "login.thtr", Line: 6, Column: 11},
						},
					},
					SourceSpan: &SourceRef{File: "login.thtr", Line: 5, Column: 5},
				},
				Expectations: []ExpectationSpec{{
					ID:      "token",
					Subject: SubjectSpec{Field: "token"},
					Assert: AssertSpec{
						Ref: "expectation.token",
						Args: map[string]BindingSpec{
							"expected": {
								Kind:       BindingKindLiteral,
								Value:      "issued-token",
								SourceSpan: &SourceRef{File: "login.thtr", Line: 8, Column: 37},
							},
						},
					},
					SourceSpan: &SourceRef{File: "login.thtr", Line: 7, Column: 5},
				}},
				SourceSpan: &SourceRef{File: "login.thtr", Line: 4, Column: 3},
			}},
		}},
		ScenarioCalls: []ScenarioCallSpec{{
			ID:         "login-user",
			ScenarioID: "login",
			Bindings: map[string]BindingSpec{
				"username": {
					Kind:       BindingKindLiteral,
					Value:      "alex",
					SourceSpan: &SourceRef{File: "login.thtr", Line: 11, Column: 23},
				},
			},
			SourceSpan: &SourceRef{File: "login.thtr", Line: 11, Column: 1},
		}},
	}

	catalog := NewCatalog()
	if err := catalog.RegisterAction("action.login", debugBoundaryTestAction{
		ContractFunc: func() ActionContract {
			return ActionContract{
				Inputs: map[string]ValueContract{
					"username": {Kind: ValueKindString},
				},
				Outputs: map[string]ValueContract{
					"token": {Kind: ValueKindString},
				},
			}
		},
		RunFunc: func(request ActionRequest) (Outputs, error) {
			if got, want := request.Args["username"], "alex"; got != want {
				t.Fatalf("username arg mismatch: got %#v want %#v", got, want)
			}

			return Outputs{"token": "issued-token"}, nil
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers, err := NewMatcherCatalog(debugBoundaryTestExpectation{
		CheckFunc: func(actual any) error { return nil },
		Args: []MatcherArg{{
			Name:    "expected",
			Accepts: ValueContract{Kind: ValueKindString},
		}},
	}.Descriptor("expectation.token"))
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	result, err := NewRunner(catalog, matchers).runWithDebugRuntime(
		context.Background(),
		spec,
		RunOptions{},
		&debugRuntime{
			breakpointSpecs: []string{
				"name=before-call,path=**,kind=scenario_call,phase=before",
				"name=before-act,path=**/act.submit,kind=act,phase=before",
				"name=before-action,path=**/action,kind=action,phase=before",
				"name=before-expectation,path=**/expectation.token,kind=expectation,phase=before",
			},
			artifactPath: path,
		},
	)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := result.Report.Status, StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	records := readDebugArtifactRecordStructs(t, path)
	scenarioPause := requireDebugArtifactPause(t, records, "before-call")
	requireSourceRef(t, scenarioPause.Snapshot.Ref.SourceSpan, "login.thtr", 11, 1)
	if got, want := scenarioPause.Snapshot.Ref.Kind, debugBoundaryKindScenarioCall; got != want {
		t.Fatalf("scenario boundary kind mismatch: got %q want %q", got, want)
	}
	scenarioUsernameField := requireDebugSnapshotField(t, scenarioPause.Snapshot.Inputs.Fields, "username")
	requireSourceRef(t, scenarioUsernameField.SourceSpan, "login.thtr", 11, 23)

	actPause := requireDebugArtifactPause(t, records, "before-act")
	requireSourceRef(t, actPause.Snapshot.Ref.SourceSpan, "login.thtr", 4, 3)
	if got, want := actPause.Snapshot.Ref.Kind, debugBoundaryKindAct; got != want {
		t.Fatalf("act boundary kind mismatch: got %q want %q", got, want)
	}

	actionPause := requireDebugArtifactPause(t, records, "before-action")
	requireSourceRef(t, actionPause.Snapshot.Ref.SourceSpan, "login.thtr", 5, 5)
	if got, want := actionPause.Snapshot.Ref.Kind, debugBoundaryKindAction; got != want {
		t.Fatalf("action boundary kind mismatch: got %q want %q", got, want)
	}
	usernameField := requireDebugSnapshotField(t, actionPause.Snapshot.Inputs.Fields, "username")
	requireSourceRef(t, usernameField.SourceSpan, "login.thtr", 6, 11)

	expectationPause := requireDebugArtifactPause(t, records, "before-expectation")
	requireSourceRef(t, expectationPause.Snapshot.Ref.SourceSpan, "login.thtr", 7, 5)
	if got, want := expectationPause.Snapshot.Ref.Kind, debugBoundaryKindExpectation; got != want {
		t.Fatalf("expectation boundary kind mismatch: got %q want %q", got, want)
	}
	expectedField := requireDebugSnapshotField(t, expectationPause.Snapshot.Inputs.Fields, "arg.expected")
	requireSourceRef(t, expectedField.SourceSpan, "login.thtr", 8, 37)
}

func TestDebugArtifactSinkCreatesPrivateFreshFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "debug.ndjson")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create debug dir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte("stale\n"), 0o666); err != nil {
		t.Fatalf("write stale artifact failed: %v", err)
	}

	sink, err := openDebugArtifactSink(path)
	if err != nil {
		t.Fatalf("open artifact sink failed: %v", err)
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("close artifact sink failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read artifact file failed: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("artifact file must be truncated on open: got %q", string(data))
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat artifact file failed: %v", err)
	}
	if got := info.Mode().Perm(); got&0o077 != 0 {
		t.Fatalf("artifact file must not be group/world readable or writable: mode %o", got)
	}
}

func TestDebugArtifactSinkHardensSharedDirectory(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "shared")
	if err := os.Mkdir(dir, 0o777); err != nil {
		t.Fatalf("create shared dir failed: %v", err)
	}
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatalf("chmod shared dir failed: %v", err)
	}

	sink, err := openDebugArtifactSink(filepath.Join(dir, "debug.ndjson"))
	if err != nil {
		t.Fatalf("open artifact sink failed: %v", err)
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("close artifact sink failed: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat debug dir failed: %v", err)
	}
	if got := info.Mode().Perm(); got&0o022 != 0 {
		t.Fatalf("artifact directory must not remain group/world writable: mode %o", got)
	}
}

func TestDebugArtifactSinkRejectsLinkedArtifactPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "target.ndjson")
	if err := os.WriteFile(target, []byte("target\n"), 0o600); err != nil {
		t.Fatalf("write target file failed: %v", err)
	}

	symlinkPath := filepath.Join(dir, "debug-symlink.ndjson")
	if err := os.Symlink(target, symlinkPath); err != nil {
		t.Skipf("symlink is not available: %v", err)
	}
	if _, err := openDebugArtifactSink(symlinkPath); err == nil {
		t.Fatal("expected symlink artifact path to be rejected")
	} else if got, want := err.Error(), "must not be a symlink"; !strings.Contains(got, want) {
		t.Fatalf("symlink error mismatch: got %q want fragment %q", got, want)
	}

	hardlinkPath := filepath.Join(dir, "debug-hardlink.ndjson")
	if err := os.Link(target, hardlinkPath); err != nil {
		t.Skipf("hardlink is not available: %v", err)
	}
	if _, err := openDebugArtifactSink(hardlinkPath); err == nil {
		t.Fatal("expected hardlink artifact path to be rejected")
	} else if got, want := err.Error(), "multiple hard links"; !strings.Contains(got, want) {
		t.Fatalf("hardlink error mismatch: got %q want fragment %q", got, want)
	}
}

func TestDebugArtifactSinkHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "debug.ndjson")
	sink, err := openDebugArtifactSink(path)
	if err != nil {
		t.Fatalf("open artifact sink failed: %v", err)
	}
	defer func() {
		if err := sink.Close(); err != nil {
			t.Fatalf("close artifact sink failed: %v", err)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := sink.WritePause(ctx, "boundary", "", debugBoundaryState{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("write pause error mismatch: got %v want %v", err, context.Canceled)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read artifact file failed: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("artifact file must stay empty after canceled write: got %q", string(data))
	}
}

func TestRunWritesDebugArtifactSnapshotsWhenArtifactPathConfigured(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "runtime.debug.ndjson")
	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{{
			ID: "login",
			Acts: []ActSpec{{
				ID:     "submit",
				Action: ActionSpec{Use: "action.login"},
			}},
		}},
		ScenarioCalls: []ScenarioCallSpec{{
			ID:         "login-user",
			ScenarioID: "login",
		}},
	}

	catalog := NewCatalog()
	if err := catalog.RegisterAction("action.login", debugBoundaryTestAction{
		RunFunc: func(request ActionRequest) (Outputs, error) {
			return Outputs{"token": "issued-token"}, nil
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers, err := NewMatcherCatalog()
	if err != nil {
		t.Fatalf("register matcher catalog failed: %v", err)
	}

	result, err := NewRunner(catalog, matchers).runWithDebugRuntime(
		context.Background(),
		spec,
		RunOptions{},
		&debugRuntime{
			breakpointSpecs: []string{"path=**/action,kind=action"},
			artifactPath:    path,
		},
	)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := result.Report.Status, StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read artifact file failed: %v", err)
	}

	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	if got, want := len(lines), 2; got != want {
		t.Fatalf("runtime artifact line count mismatch: got %d want %d", got, want)
	}

	var beforeRecord debugArtifactRecord
	if err := json.Unmarshal(lines[0], &beforeRecord); err != nil {
		t.Fatalf("decode before record failed: %v", err)
	}
	if got, want := beforeRecord.Kind, debugArtifactKindPause; got != want {
		t.Fatalf("before record kind mismatch: got %q want %q", got, want)
	}
	if got, want := beforeRecord.Pause.Snapshot.Ref.Kind, debugBoundaryKindAction; got != want {
		t.Fatalf("before record kind mismatch: got %q want %q", got, want)
	}
	if got, want := beforeRecord.Pause.Snapshot.Ref.Phase, debugBoundaryPhaseBefore; got != want {
		t.Fatalf("before phase mismatch: got %q want %q", got, want)
	}

	var afterRecord debugArtifactRecord
	if err := json.Unmarshal(lines[len(lines)-1], &afterRecord); err != nil {
		t.Fatalf("decode after record failed: %v", err)
	}
	if got, want := afterRecord.Kind, debugArtifactKindPause; got != want {
		t.Fatalf("after record kind mismatch: got %q want %q", got, want)
	}
	if got, want := afterRecord.Pause.Snapshot.Ref.Kind, debugBoundaryKindAction; got != want {
		t.Fatalf("after record kind mismatch: got %q want %q", got, want)
	}
	if got, want := afterRecord.Pause.Snapshot.Ref.Phase, debugBoundaryPhaseAfter; got != want {
		t.Fatalf("after phase mismatch: got %q want %q", got, want)
	}
	if got, want := afterRecord.Pause.Snapshot.Status, StatusPassed; got != want {
		t.Fatalf("after status mismatch: got %q want %q", got, want)
	}
}

func TestRunWritesActionCheckpointDebugArtifactSnapshot(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "runtime.debug.ndjson")
	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{{
			ID: "login",
			Acts: []ActSpec{{
				ID:     "submit",
				Action: ActionSpec{Use: "action.login"},
			}},
		}},
		ScenarioCalls: []ScenarioCallSpec{{
			ID:         "login-user",
			ScenarioID: "login",
		}},
	}

	catalog := NewCatalog()
	if err := catalog.RegisterAction("action.login", debugBoundaryTestAction{
		RunFunc: func(request ActionRequest) (Outputs, error) {
			if reporter, ok := request.Reporter.(DebugCheckpointReporter); ok {
				reporter.DebugCheckpoint(DebugCheckpoint{
					Name: "mid-action",
					Values: Values{
						"step": "halfway",
					},
				})
			}

			return Outputs{"token": "issued-token"}, nil
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers, err := NewMatcherCatalog()
	if err != nil {
		t.Fatalf("register matcher catalog failed: %v", err)
	}

	result, err := NewRunner(catalog, matchers).runWithDebugRuntime(
		context.Background(),
		spec,
		RunOptions{},
		&debugRuntime{
			artifactPath: path,
		},
	)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := result.Report.Status, StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read artifact file failed: %v", err)
	}

	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	found := false
	for i := range lines {
		var record debugArtifactRecord
		if err := json.Unmarshal(lines[i], &record); err != nil {
			t.Fatalf("decode artifact record failed: %v", err)
		}
		if record.Kind != debugArtifactKindPause || record.Pause == nil {
			continue
		}
		if got, want := record.Pause.Reason, "checkpoint"; got != want {
			continue
		}
		found = true
		if got, want := record.Pause.Breakpoint, "mid-action"; got != want {
			t.Fatalf("checkpoint label mismatch: got %q want %q", got, want)
		}
		if got, want := record.Pause.Snapshot.Ref.Path, "stage.main/call.login-user/act.submit/action"; got != want {
			t.Fatalf("checkpoint path mismatch: got %q want %q", got, want)
		}
		if got, want := record.Pause.Snapshot.Output.Fields[0].Key, "checkpoint"; got != want {
			t.Fatalf("checkpoint output key mismatch: got %q want %q", got, want)
		}
		if got, want := record.Pause.Snapshot.Output.Fields[0].Value.Text, "mid-action"; got != want {
			t.Fatalf("checkpoint output text mismatch: got %q want %q", got, want)
		}
	}
	if !found {
		t.Fatalf("checkpoint artifact record not found in %q", string(data))
	}
}

func TestDebugArtifactSinkDoesNotReuseSequenceAfterWriteError(t *testing.T) {
	t.Parallel()

	writer := &debugArtifactFailWriter{failFirst: true}
	sink := &debugArtifactSink{
		encoder: json.NewEncoder(writer),
	}

	if _, err := sink.WritePause(context.Background(), "boundary", "", debugBoundaryState{}); err == nil {
		t.Fatal("first write error = nil, want failure")
	}
	sink.encoder = json.NewEncoder(bytes.NewBuffer(nil))

	seq, err := sink.WritePause(context.Background(), "boundary", "", debugBoundaryState{})
	if err != nil {
		t.Fatalf("second write failed: %v", err)
	}
	if got, want := seq, uint64(2); got != want {
		t.Fatalf("second write seq mismatch: got %d want %d", got, want)
	}
}

func readDebugArtifactRecordStructs(t *testing.T, path string) []debugArtifactRecord {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read artifact file failed: %v", err)
	}

	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	records := make([]debugArtifactRecord, 0, len(lines))
	for i := range lines {
		if len(lines[i]) == 0 {
			continue
		}

		var record debugArtifactRecord
		if err := json.Unmarshal(lines[i], &record); err != nil {
			t.Fatalf("decode artifact record failed: %v", err)
		}
		records = append(records, record)
	}

	return records
}

func requireDebugArtifactPause(
	t *testing.T,
	records []debugArtifactRecord,
	breakpoint string,
) debugArtifactPauseRecord {
	t.Helper()

	for i := range records {
		if records[i].Kind != debugArtifactKindPause || records[i].Pause == nil {
			continue
		}
		if records[i].Pause.Breakpoint == breakpoint {
			return *records[i].Pause
		}
	}

	t.Fatalf("debug artifact pause with breakpoint %q not found", breakpoint)
	return debugArtifactPauseRecord{}
}

func requireDebugSnapshotField(
	t *testing.T,
	fields []debugSnapshotField,
	key string,
) debugSnapshotField {
	t.Helper()

	for i := range fields {
		if fields[i].Key == key {
			return fields[i]
		}
	}

	t.Fatalf("debug snapshot field %q not found", key)
	return debugSnapshotField{}
}

func requireSourceRef(t *testing.T, source *SourceRef, file string, line int, column int) {
	t.Helper()

	if source == nil {
		t.Fatalf("source ref is nil, want %s:%d:%d", file, line, column)
	}
	if got, want := source.File, file; got != want {
		t.Fatalf("source file mismatch: got %q want %q", got, want)
	}
	if got, want := source.Line, line; got != want {
		t.Fatalf("source line mismatch: got %d want %d", got, want)
	}
	if got, want := source.Column, column; got != want {
		t.Fatalf("source column mismatch: got %d want %d", got, want)
	}
}

type debugArtifactFailWriter struct {
	failFirst bool
}

func (w *debugArtifactFailWriter) Write(p []byte) (int, error) {
	if w.failFirst {
		w.failFirst = false
		return 0, errors.New("write failed")
	}

	return len(p), nil
}
