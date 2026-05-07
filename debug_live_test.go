package theater

import (
	"context"
	"testing"

	"github.com/alex-poliushkin/theater/observe"
)

func TestDebugLiveBridgeSnapshotKeepsRecentLaneEventsAndDroppedNotices(t *testing.T) {
	t.Parallel()

	bridge := newDebugLiveBridge(2)
	node := observe.NodeRef{
		ScenarioCallID: "login-user",
		Path:           "stage.main/call.login-user/act.submit/action",
		Attempt:        1,
	}

	bridge.record(observe.Envelope{
		Seq:  1,
		Kind: observe.KindProgress,
		Node: node,
		Progress: &observe.Progress{
			Message: "warming up",
		},
	})
	bridge.record(observe.Envelope{
		Seq:  2,
		Kind: observe.KindDropped,
		Node: node,
		Dropped: &observe.DroppedNotice{
			Count: 3,
		},
	})
	bridge.record(observe.Envelope{
		Seq:  3,
		Kind: observe.KindDiagnostic,
		Node: node,
		Diagnostic: &observe.Diagnostic{
			Message: "retrying",
		},
	})

	snapshot := bridge.Snapshot("login-user")
	if got, want := len(snapshot.Items), 2; got != want {
		t.Fatalf("item count mismatch: got %d want %d", got, want)
	}
	if got, want := snapshot.Omitted, 1; got != want {
		t.Fatalf("omitted count mismatch: got %d want %d", got, want)
	}

	if got, want := snapshot.Items[0].Kind, string(observe.KindDropped); got != want {
		t.Fatalf("item[0] kind mismatch: got %q want %q", got, want)
	}
	if got, want := snapshot.Items[0].Text, "dropped 3"; got != want {
		t.Fatalf("item[0] text mismatch: got %q want %q", got, want)
	}
	if got, want := snapshot.Items[1].Kind, string(observe.KindDiagnostic); got != want {
		t.Fatalf("item[1] kind mismatch: got %q want %q", got, want)
	}
	if got, want := snapshot.Items[1].Text, "retrying"; got != want {
		t.Fatalf("item[1] text mismatch: got %q want %q", got, want)
	}
}

func TestRunIncludesRecentLiveTailAndSchedulerSummaryInDebugBoundarySnapshots(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{{
			ID: "emit",
			Acts: []ActSpec{{
				ID:     "observe",
				Action: ActionSpec{Use: "action.observe"},
			}},
		}},
		ScenarioCalls: []ScenarioCallSpec{
			{
				ID:         "first",
				ScenarioID: "emit",
			},
			{
				ID:         "second",
				ScenarioID: "emit",
				Dependencies: []ScenarioDependencySpec{
					{CallID: "first", When: TriggerPredicateSuccess},
				},
			},
		},
	}

	catalog := NewCatalog()
	if err := catalog.RegisterAction("action.observe", debugObserveAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers, err := NewMatcherCatalog()
	if err != nil {
		t.Fatalf("register matcher catalog failed: %v", err)
	}

	hits := make([]debugBoundaryState, 0, 4)
	result, err := NewRunner(catalog, matchers).runWithDebugRuntime(
		context.Background(),
		spec,
		RunOptions{},
		&debugRuntime{
			boundaryHook: func(_ context.Context, state debugBoundaryState) error {
				hits = append(hits, state)
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if got, want := result.Report.Status, StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	var firstAfter debugBoundaryState
	found := false
	for i := range hits {
		if hits[i].Ref.ScenarioCallID != "first" {
			continue
		}
		if hits[i].Ref.Kind != debugBoundaryKindAction || hits[i].Ref.Phase != debugBoundaryPhaseAfter {
			continue
		}

		firstAfter = hits[i]
		found = true
		break
	}
	if !found {
		t.Fatal("first action after-boundary snapshot not found")
	}

	if len(firstAfter.Recent.Items) == 0 {
		t.Fatal("recent live snapshot is empty")
	}
	if !hasDebugRecentKind(firstAfter.Recent, observe.KindProgress) {
		t.Fatal("recent live snapshot does not include progress event")
	}
	if !hasDebugRecentKind(firstAfter.Recent, observe.KindDiagnostic) {
		t.Fatal("recent live snapshot does not include diagnostic event")
	}
	if !hasDebugRecentKind(firstAfter.Recent, observe.KindLogChunk) {
		t.Fatal("recent live snapshot does not include log chunk event")
	}

	if got, want := firstAfter.Scheduler.FocusedLane, "stage.main/call.first"; got != want {
		t.Fatalf("focused lane mismatch: got %q want %q", got, want)
	}
	if got, want := firstAfter.Scheduler.Active, 1; got != want {
		t.Fatalf("scheduler active mismatch: got %d want %d", got, want)
	}
	if got, want := firstAfter.Scheduler.Ready, 1; got != want {
		t.Fatalf("scheduler ready mismatch: got %d want %d", got, want)
	}
	if got, want := firstAfter.Scheduler.Blocked, 1; got != want {
		t.Fatalf("scheduler blocked mismatch: got %d want %d", got, want)
	}
	if got, want := len(firstAfter.Scheduler.ReadyPaths), 1; got != want {
		t.Fatalf("scheduler ready path count mismatch: got %d want %d", got, want)
	}
	if got, want := firstAfter.Scheduler.ReadyPaths[0], "stage.main/call.first"; got != want {
		t.Fatalf("scheduler ready path mismatch: got %q want %q", got, want)
	}
}

func TestDebugRuntimeCloseClearsPerRunState(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/debug.ndjson"
	sink, err := openDebugArtifactSink(path)
	if err != nil {
		t.Fatalf("open artifact sink failed: %v", err)
	}

	debug := &debugRuntime{
		artifactSink:  sink,
		liveBridge:    newDebugLiveBridge(2),
		scheduler:     newDebugSchedulerState(),
		stateRecorder: &debugStateRecorder{},
	}

	if err := debug.close(context.Background()); err != nil {
		t.Fatalf("close debug runtime failed: %v", err)
	}

	if debug.artifactSink != nil {
		t.Fatal("artifact sink must be cleared after close")
	}
	if debug.liveBridge != nil {
		t.Fatal("live bridge must be cleared after close")
	}
	if debug.scheduler != nil {
		t.Fatal("scheduler state must be cleared after close")
	}
	if debug.stateRecorder != nil {
		t.Fatal("state recorder must be cleared after close")
	}
}

func TestDebugRuntimePrepareRunReopensArtifactSinkAfterClose(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/debug.ndjson"
	debug := &debugRuntime{artifactPath: path}

	_, closeFirst, err := debug.prepareRun(nil)
	if err != nil {
		t.Fatalf("first prepare run failed: %v", err)
	}
	if debug.artifactSink == nil {
		t.Fatal("first artifact sink is nil")
	}
	if _, err := debug.artifactSink.WritePause(context.Background(), "boundary", "", debugBoundaryState{}); err != nil {
		t.Fatalf("first write failed: %v", err)
	}
	if err := closeFirst(context.Background()); err != nil {
		t.Fatalf("first close failed: %v", err)
	}

	_, closeSecond, err := debug.prepareRun(nil)
	if err != nil {
		t.Fatalf("second prepare run failed: %v", err)
	}
	if debug.artifactSink == nil {
		t.Fatal("second artifact sink is nil")
	}
	if _, err := debug.artifactSink.WritePause(context.Background(), "boundary", "", debugBoundaryState{}); err != nil {
		t.Fatalf("second write failed: %v", err)
	}
	if err := closeSecond(context.Background()); err != nil {
		t.Fatalf("second close failed: %v", err)
	}
}

type debugObserveAction struct{}

func (debugObserveAction) Contract() ActionContract {
	return ActionContract{}
}

func (debugObserveAction) Run(_ context.Context, request ActionRequest) (Outputs, error) {
	request.Reporter.Progress(observe.Progress{
		Phase:   "prepare",
		Message: "warming up",
	})
	request.Reporter.Diagnostic(observe.Diagnostic{
		Message: "inventory ready",
	})
	request.Reporter.LogChunk(observe.LogChunk{
		Stream: "stdout",
		Data:   []byte("hello from action"),
	})

	return Outputs{}, nil
}

func hasDebugRecentKind(snapshot debugRecentSnapshot, kind observe.Kind) bool {
	for i := range snapshot.Items {
		if snapshot.Items[i].Kind == string(kind) {
			return true
		}
	}

	return false
}
