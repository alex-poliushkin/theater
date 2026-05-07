package theater

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alex-poliushkin/theater/observe"
)

type failingEventRecorder struct {
	err error
}

func (r failingEventRecorder) Record(Event) error {
	return r.err
}

type executorConcurrencyProbe struct {
	started chan string
	current atomic.Int32
	max     atomic.Int32
}

type executorTestAction struct {
	contract ActionContract
	run      func(Args) (Outputs, error)
}

type scenarioRunningRecorder struct {
	mu      sync.Mutex
	events  []Event
	started chan string
}

func (p *executorConcurrencyProbe) Start(name string) {
	current := p.current.Add(1)
	for {
		max := p.max.Load()
		if current <= max || p.max.CompareAndSwap(max, current) {
			break
		}
	}

	p.started <- name
}

func (p *executorConcurrencyProbe) Finish() {
	p.current.Add(-1)
}

func (a executorTestAction) Contract() ActionContract {
	return a.contract
}

func (a executorTestAction) Run(_ context.Context, request ActionRequest) (Outputs, error) {
	if a.run == nil {
		return Outputs{}, nil
	}

	return a.run(request.Args)
}

func (r *scenarioRunningRecorder) Record(event Event) error {
	r.mu.Lock()
	r.events = append(r.events, event)
	r.mu.Unlock()

	if event.Kind == EventKindScenarioRunning && r.started != nil {
		r.started <- event.ScenarioCallID
	}

	return nil
}

func (r *scenarioRunningRecorder) CountScenarioRunning() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	count := 0
	for i := range r.events {
		if r.events[i].Kind == EventKindScenarioRunning {
			count++
		}
	}

	return count
}

func TestScenarioBatchPlannerReturnsReadyBatchInPlanOrder(t *testing.T) {
	t.Parallel()

	planner := newScenarioBatchPlanner(&stagePlan{
		ID:   "main",
		Path: "stage.main",
		Scenarios: []scenarioPlan{
			{ID: "scenario/first"},
			{ID: "scenario/second"},
			{ID: "scenario/third"},
		},
		ScenarioCalls: []scenarioCallPlan{
			{ID: "third", Path: "stage.main/call.third", ScenarioID: "scenario/third", PlanOrdinal: 3},
			{ID: "first", Path: "stage.main/call.first", ScenarioID: "scenario/first", PlanOrdinal: 1},
			{
				ID:          "second",
				Path:        "stage.main/call.second",
				ScenarioID:  "scenario/second",
				PlanOrdinal: 2,
				Dependencies: []scenarioDependencyPlan{
					{CallID: "first", When: TriggerPredicateFailure},
				},
			},
		},
	})

	if got, want := planner.FailurePolicy(), stageFailurePolicyFailAfterReadyBatch; got != want {
		t.Fatalf("planner failure policy mismatch: got %q want %q", got, want)
	}

	scheduled, err := planner.NextReadyBatch()
	if err != nil {
		t.Fatalf("next ready batch failed: %v", err)
	}

	if got, want := len(scheduled), 2; got != want {
		t.Fatalf("scheduled count mismatch: got %d want %d", got, want)
	}
	if got, want := scheduled[0].call.ID, "first"; got != want {
		t.Fatalf("first scheduled call mismatch: got %q want %q", got, want)
	}
	if got, want := scheduled[1].call.ID, "third"; got != want {
		t.Fatalf("second scheduled call mismatch: got %q want %q", got, want)
	}
}

func TestStageRunnerSchedulesSingleReadyScenarioCallInInteractiveDebugMode(t *testing.T) {
	t.Parallel()

	ready := []scheduledScenarioRun{
		newScheduledScenarioRunForAction("alpha-user", "action.alpha", 1),
		newScheduledScenarioRunForAction("beta-user", "action.beta", 2),
	}

	runner := &stageRunner{
		executor: &scenarioBatchExecutor{
			debug: &debugRuntime{
				controller: newTestDebugController(debugModeInteractive, false, nil),
			},
		},
	}

	scheduled := runner.scheduleBatch(ready)
	if got, want := len(scheduled), 1; got != want {
		t.Fatalf("scheduled count mismatch: got %d want %d", got, want)
	}
	if got, want := scheduled[0].call.ID, "alpha-user"; got != want {
		t.Fatalf("scheduled call mismatch: got %q want %q", got, want)
	}
}

func TestStageRunnerKeepsWholeReadyBatchOutsideInteractiveDebugMode(t *testing.T) {
	t.Parallel()

	ready := []scheduledScenarioRun{
		newScheduledScenarioRunForAction("alpha-user", "action.alpha", 1),
		newScheduledScenarioRunForAction("beta-user", "action.beta", 2),
	}

	runner := &stageRunner{
		executor: &scenarioBatchExecutor{
			debug: &debugRuntime{
				controller: newTestDebugController(debugModeDump, false, nil),
			},
		},
	}

	scheduled := runner.scheduleBatch(ready)
	if got, want := len(scheduled), len(ready); got != want {
		t.Fatalf("scheduled count mismatch: got %d want %d", got, want)
	}
	if got, want := scheduled[1].call.ID, "beta-user"; got != want {
		t.Fatalf("second scheduled call mismatch: got %q want %q", got, want)
	}
}

func TestDefaultStageConcurrencyLimitUsesGOMAXPROCS(t *testing.T) {
	t.Parallel()

	if got, want := defaultStageConcurrencyLimit(), runtime.GOMAXPROCS(0); got != want {
		t.Fatalf("default concurrency limit mismatch: got %d want %d", got, want)
	}
}

func TestScenarioBatchExecutorLimitOneRunsOneScenarioAtATime(t *testing.T) {
	t.Parallel()

	probe := &executorConcurrencyProbe{started: make(chan string, 2)}
	release := make(chan struct{})
	catalog := newExecutorTestCatalog(t, map[string]executorTestAction{
		"action.alpha": newBlockingExecutorAction("alpha", probe, release),
		"action.beta":  newBlockingExecutorAction("beta", probe, release),
	})
	executor := newScenarioBatchExecutorWithLimit(nil, catalog, nil, noopEventRecord, nil, nil, nil, nil, 1)
	scheduled := []scheduledScenarioRun{
		newScheduledScenarioRunForAction("alpha-user", "action.alpha", 1),
		newScheduledScenarioRunForAction("beta-user", "action.beta", 2),
	}

	done := make(chan []scenarioBatchResult, 1)
	go func() {
		done <- executor.Execute(context.Background(), scheduled)
	}()

	waitForStartedCount(t, probe.started, 1, 2*time.Second)
	ensureNoAdditionalStart(t, probe.started, 150*time.Millisecond)
	close(release)

	results := waitForBatchResults(t, done)
	waitForStartedCount(t, probe.started, 1, 2*time.Second)

	if got, want := probe.max.Load(), int32(1); got != want {
		t.Fatalf("max in-flight mismatch: got %d want %d", got, want)
	}
	assertBatchResultOrder(t, results, scheduled)
	assertBatchStatuses(t, results, StatusPassed, StatusPassed)
}

func TestScenarioBatchExecutorCapsLargerBatchAtLimit(t *testing.T) {
	t.Parallel()

	probe := &executorConcurrencyProbe{started: make(chan string, 3)}
	release := make(chan struct{})
	catalog := newExecutorTestCatalog(t, map[string]executorTestAction{
		"action.alpha": newBlockingExecutorAction("alpha", probe, release),
		"action.beta":  newBlockingExecutorAction("beta", probe, release),
		"action.gamma": newBlockingExecutorAction("gamma", probe, release),
	})
	executor := newScenarioBatchExecutorWithLimit(nil, catalog, nil, noopEventRecord, nil, nil, nil, nil, 2)
	scheduled := []scheduledScenarioRun{
		newScheduledScenarioRunForAction("alpha-user", "action.alpha", 1),
		newScheduledScenarioRunForAction("beta-user", "action.beta", 2),
		newScheduledScenarioRunForAction("gamma-user", "action.gamma", 3),
	}

	done := make(chan []scenarioBatchResult, 1)
	go func() {
		done <- executor.Execute(context.Background(), scheduled)
	}()

	waitForStartedCount(t, probe.started, 2, 2*time.Second)
	ensureNoAdditionalStart(t, probe.started, 150*time.Millisecond)
	close(release)

	results := waitForBatchResults(t, done)
	waitForStartedCount(t, probe.started, 1, 2*time.Second)

	if got, want := probe.max.Load(), int32(2); got != want {
		t.Fatalf("max in-flight mismatch: got %d want %d", got, want)
	}
	assertBatchResultOrder(t, results, scheduled)
	assertBatchStatuses(t, results, StatusPassed, StatusPassed, StatusPassed)
}

func TestScenarioBatchExecutorForcesSingleWorkerInInteractiveDebugMode(t *testing.T) {
	t.Parallel()

	executor := newScenarioBatchExecutorWithLimit(
		nil,
		nil,
		nil,
		noopEventRecord,
		nil,
		nil,
		nil,
		&debugRuntime{
			controller: newTestDebugController(debugModeInteractive, false, nil),
		},
		4,
	)

	if got, want := executor.workerCount(3), 1; got != want {
		t.Fatalf("worker count mismatch: got %d want %d", got, want)
	}
}

func TestStageRunnerInteractiveDebugDoesNotStartAnotherReadyCallWhilePaused(t *testing.T) {
	t.Parallel()

	catalog := newExecutorTestCatalog(t, map[string]executorTestAction{
		"action.alpha": {},
		"action.beta":  {},
	})
	ready := []scheduledScenarioRun{
		newScheduledScenarioRunForAction("alpha-user", "action.alpha", 1),
		newScheduledScenarioRunForAction("beta-user", "action.beta", 2),
	}
	stage := &stagePlan{
		ID:   "main",
		Path: "stage.main",
		Scenarios: []scenarioPlan{
			*ready[0].scenario,
			*ready[1].scenario,
		},
		ScenarioCalls: []scenarioCallPlan{
			ready[0].call,
			ready[1].call,
		},
	}

	recorder := &scenarioRunningRecorder{started: make(chan string, len(ready))}
	paused := make(chan debugPause, 1)
	release := make(chan struct{})
	debug := &debugRuntime{
		controller: newTestDebugController(debugModeInteractive, true, func(_ context.Context, pause debugPause) (debugResumeCommand, error) {
			select {
			case paused <- pause:
			default:
			}

			<-release
			return debugResumeContinue, nil
		}),
	}

	runner := newStageRunner(stage, catalog, nil, nil, nil, recorder, debug)
	runner.executor = newScenarioBatchExecutorWithLimit(
		nil,
		catalog,
		nil,
		runner.sink.Record,
		nil,
		runner.scenarioScopeRun,
		nil,
		debug,
		2,
	)

	type runResult struct {
		report Report
		err    error
	}

	done := make(chan runResult, 1)
	go func() {
		report, err := runner.Run(context.Background())
		done <- runResult{report: report, err: err}
	}()

	select {
	case pause := <-paused:
		if got, want := pause.Reason, debugPauseReasonStart; got != want {
			t.Fatalf("pause reason mismatch: got %q want %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for initial pause")
	}

	waitForStartedCount(t, recorder.started, 1, 2*time.Second)
	ensureNoAdditionalStart(t, recorder.started, 150*time.Millisecond)
	if got, want := recorder.CountScenarioRunning(), 1; got != want {
		t.Fatalf("scenario running count while paused mismatch: got %d want %d", got, want)
	}

	close(release)

	result := waitForStageRunResult(t, done)
	waitForStartedCount(t, recorder.started, 1, 2*time.Second)
	if got, want := recorder.CountScenarioRunning(), len(ready); got != want {
		t.Fatalf("final scenario running count mismatch: got %d want %d", got, want)
	}

	if result.err != nil {
		t.Fatalf("run failed: %v", result.err)
	}
	if got, want := result.report.Status, StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func TestScenarioBatchExecutorPreservesScheduledResultOrder(t *testing.T) {
	t.Parallel()

	started := make(chan string, 3)
	completed := make(chan string, 3)
	alphaRelease := make(chan struct{})
	betaRelease := make(chan struct{})
	gammaRelease := make(chan struct{})
	catalog := newExecutorTestCatalog(t, map[string]executorTestAction{
		"action.alpha": newReleasedExecutorAction("alpha", started, completed, alphaRelease),
		"action.beta":  newReleasedExecutorAction("beta", started, completed, betaRelease),
		"action.gamma": newReleasedExecutorAction("gamma", started, completed, gammaRelease),
	})
	executor := newScenarioBatchExecutorWithLimit(nil, catalog, nil, noopEventRecord, nil, nil, nil, nil, 3)
	scheduled := []scheduledScenarioRun{
		newScheduledScenarioRunForAction("alpha-user", "action.alpha", 1),
		newScheduledScenarioRunForAction("beta-user", "action.beta", 2),
		newScheduledScenarioRunForAction("gamma-user", "action.gamma", 3),
	}

	done := make(chan []scenarioBatchResult, 1)
	go func() {
		done <- executor.Execute(context.Background(), scheduled)
	}()

	waitForStartedCount(t, started, 3, 2*time.Second)
	close(betaRelease)
	completionOrder := []string{
		waitForSingleName(t, completed, 2*time.Second),
	}
	close(gammaRelease)
	completionOrder = append(completionOrder, waitForSingleName(t, completed, 2*time.Second))
	close(alphaRelease)

	results := waitForBatchResults(t, done)
	completionOrder = append(completionOrder, waitForSingleName(t, completed, 2*time.Second))
	wantCompletionOrder := []string{"beta", "gamma", "alpha"}
	if !reflect.DeepEqual(completionOrder, wantCompletionOrder) {
		t.Fatalf("completion order mismatch: got %#v want %#v", completionOrder, wantCompletionOrder)
	}

	assertBatchResultOrder(t, results, scheduled)
	assertBatchStatuses(t, results, StatusPassed, StatusPassed, StatusPassed)
}

func TestScenarioBatchExecutorDrainsWholeBatchAfterFailure(t *testing.T) {
	t.Parallel()

	started := make(chan string, 3)
	release := make(chan struct{})
	catalog := newExecutorTestCatalog(t, map[string]executorTestAction{
		"action.alpha": {
			run: func(Args) (Outputs, error) {
				started <- "alpha"
				return Outputs{}, errors.New("boom")
			},
		},
		"action.beta":  newBlockingExecutorAction("beta", &executorConcurrencyProbe{started: started}, release),
		"action.gamma": newBlockingExecutorAction("gamma", &executorConcurrencyProbe{started: started}, release),
	})
	executor := newScenarioBatchExecutorWithLimit(nil, catalog, nil, noopEventRecord, nil, nil, nil, nil, 2)
	scheduled := []scheduledScenarioRun{
		newScheduledScenarioRunForAction("alpha-user", "action.alpha", 1),
		newScheduledScenarioRunForAction("beta-user", "action.beta", 2),
		newScheduledScenarioRunForAction("gamma-user", "action.gamma", 3),
	}

	done := make(chan []scenarioBatchResult, 1)
	go func() {
		done <- executor.Execute(context.Background(), scheduled)
	}()

	startedNames := waitForNames(t, started, 3, 2*time.Second)
	close(release)

	results := waitForBatchResults(t, done)
	if got, want := len(startedNames), 3; got != want {
		t.Fatalf("started count mismatch: got %d want %d", got, want)
	}

	assertBatchResultOrder(t, results, scheduled)
	assertBatchStatuses(t, results, StatusFailed, StatusPassed, StatusPassed)
}

func TestScenarioBatchPlannerFiltersReadyCallsAfterFailedBatch(t *testing.T) {
	t.Parallel()

	planner := newScenarioBatchPlanner(&stagePlan{
		ID:   "main",
		Path: "stage.main",
		Scenarios: []scenarioPlan{
			{ID: "scenario/first"},
			{ID: "scenario/on-failure"},
			{ID: "scenario/unrelated"},
		},
		ScenarioCalls: []scenarioCallPlan{
			{ID: "first", Path: "stage.main/call.first", ScenarioID: "scenario/first", PlanOrdinal: 1},
			{
				ID:          "on-failure",
				Path:        "stage.main/call.on-failure",
				ScenarioID:  "scenario/on-failure",
				PlanOrdinal: 2,
				Dependencies: []scenarioDependencyPlan{
					{CallID: "first", When: TriggerPredicateFailure},
				},
			},
			{ID: "unrelated", Path: "stage.main/call.unrelated", ScenarioID: "scenario/unrelated", PlanOrdinal: 3},
		},
	})

	err := planner.ApplyResults([]scenarioBatchResult{
		{
			callID: "first",
			state: scenarioState{
				Status: StatusFailed,
				Failure: &Failure{
					Kind:    FailureKindAction,
					Phase:   PhaseRun,
					At:      "stage.main/call.first",
					Summary: "failed",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("apply results failed: %v", err)
	}

	scheduled, err := planner.NextReadyBatch()
	if err != nil {
		t.Fatalf("next ready batch failed: %v", err)
	}

	if got, want := len(scheduled), 1; got != want {
		t.Fatalf("scheduled count mismatch: got %d want %d", got, want)
	}
	if got, want := scheduled[0].call.ID, "on-failure"; got != want {
		t.Fatalf("scheduled call mismatch: got %q want %q", got, want)
	}
}

func newBlockingExecutorAction(
	name string,
	probe *executorConcurrencyProbe,
	release <-chan struct{},
) executorTestAction {
	return executorTestAction{
		run: func(Args) (Outputs, error) {
			probe.Start(name)
			defer probe.Finish()

			<-release
			return Outputs{}, nil
		},
	}
}

func newReleasedExecutorAction(
	name string,
	started chan<- string,
	completed chan<- string,
	release <-chan struct{},
) executorTestAction {
	return executorTestAction{
		run: func(Args) (Outputs, error) {
			started <- name
			<-release
			completed <- name
			return Outputs{}, nil
		},
	}
}

func newExecutorTestCatalog(t *testing.T, actions map[string]executorTestAction) *Catalog {
	t.Helper()

	catalog := NewCatalog()
	for ref, action := range actions {
		if err := catalog.RegisterAction(ref, action); err != nil {
			t.Fatalf("register action %q failed: %v", ref, err)
		}
	}

	return catalog
}

func newScheduledScenarioRunForAction(callID, actionRef string, sequence int) scheduledScenarioRun {
	scenarioID := "scenario/" + callID
	scenarioPath := "stage.main/call." + callID

	return scheduledScenarioRun{
		call: scenarioCallPlan{
			ID:          callID,
			Path:        scenarioPath,
			PlanOrdinal: sequence,
			ScenarioID:  scenarioID,
		},
		scenario: &scenarioPlan{
			ID:          scenarioID,
			Path:        "stage.main/scenario." + callID,
			PlanOrdinal: sequence,
			Acts: []actPlan{
				{
					ID:     "submit",
					Path:   scenarioPath + "/act.submit",
					Action: actionPlan{Use: actionRef},
				},
			},
		},
		identity: executionIdentity{
			stageID:        "main",
			stagePath:      "stage.main",
			scenarioID:     scenarioID,
			scenarioCallID: callID,
			scenarioPath:   scenarioPath,
			scenarioSeq:    sequence,
		},
	}
}

func noopEventRecord(Event) error {
	return nil
}

func waitForBatchResults(t *testing.T, done <-chan []scenarioBatchResult) []scenarioBatchResult {
	t.Helper()

	select {
	case results := <-done:
		return results
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for batch results")
		return nil
	}
}

func waitForStageRunResult[T any](t *testing.T, done <-chan T) T {
	t.Helper()

	select {
	case result := <-done:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stage run result")
		var zero T
		return zero
	}
}

func waitForStartedCount(t *testing.T, started <-chan string, want int, timeout time.Duration) []string {
	t.Helper()

	return waitForNames(t, started, want, timeout)
}

func waitForNames(t *testing.T, started <-chan string, want int, timeout time.Duration) []string {
	t.Helper()

	deadline := time.After(timeout)
	names := make([]string, 0, want)
	for len(names) < want {
		select {
		case name := <-started:
			names = append(names, name)
		case <-deadline:
			t.Fatalf("timed out waiting for %d names, got %d", want, len(names))
		}
	}

	return names
}

func waitForSingleName(t *testing.T, started <-chan string, timeout time.Duration) string {
	t.Helper()

	select {
	case name := <-started:
		return name
	case <-time.After(timeout):
		t.Fatal("timed out waiting for name")
		return ""
	}
}

func ensureNoAdditionalStart(t *testing.T, started <-chan string, duration time.Duration) {
	t.Helper()

	select {
	case name := <-started:
		t.Fatalf("unexpected additional start: %s", name)
	case <-time.After(duration):
	}
}

func assertBatchResultOrder(t *testing.T, results []scenarioBatchResult, scheduled []scheduledScenarioRun) {
	t.Helper()

	if got, want := len(results), len(scheduled); got != want {
		t.Fatalf("result count mismatch: got %d want %d", got, want)
	}

	for i := range scheduled {
		if got, want := results[i].callID, scheduled[i].call.ID; got != want {
			t.Fatalf("result[%d] call id mismatch: got %q want %q", i, got, want)
		}
	}
}

func assertBatchStatuses(t *testing.T, results []scenarioBatchResult, statuses ...Status) {
	t.Helper()

	if got, want := len(results), len(statuses); got != want {
		t.Fatalf("status count mismatch: got %d want %d", got, want)
	}

	for i, want := range statuses {
		if got := results[i].err; got != nil {
			t.Fatalf("result[%d] unexpected error: %v", i, got)
		}
		if got := results[i].state.Status; got != want {
			t.Fatalf("result[%d] status mismatch: got %q want %q", i, got, want)
		}
	}
}

func TestScenarioBatchPlannerKeepsAlreadyReadyCallsInCurrentBatchAfterFailure(t *testing.T) {
	t.Parallel()

	planner := newScenarioBatchPlanner(&stagePlan{
		ID:   "main",
		Path: "stage.main",
		Scenarios: []scenarioPlan{
			{ID: "scenario/first"},
			{ID: "scenario/independent"},
			{ID: "scenario/recovery"},
			{ID: "scenario/later"},
		},
		ScenarioCalls: []scenarioCallPlan{
			{ID: "first", Path: "stage.main/call.first", ScenarioID: "scenario/first", PlanOrdinal: 1},
			{ID: "independent", Path: "stage.main/call.independent", ScenarioID: "scenario/independent", PlanOrdinal: 2},
			{
				ID:          "recovery",
				Path:        "stage.main/call.recovery",
				ScenarioID:  "scenario/recovery",
				PlanOrdinal: 3,
				Dependencies: []scenarioDependencyPlan{
					{CallID: "first", When: TriggerPredicateFailure},
				},
			},
			{
				ID:          "later",
				Path:        "stage.main/call.later",
				ScenarioID:  "scenario/later",
				PlanOrdinal: 4,
				Dependencies: []scenarioDependencyPlan{
					{CallID: "independent", When: TriggerPredicateSuccess},
				},
			},
		},
	})

	scheduled, err := planner.NextReadyBatch()
	if err != nil {
		t.Fatalf("next ready batch failed: %v", err)
	}

	if got, want := len(scheduled), 2; got != want {
		t.Fatalf("scheduled count mismatch: got %d want %d", got, want)
	}
	if got, want := scheduled[0].call.ID, "first"; got != want {
		t.Fatalf("first scheduled call mismatch: got %q want %q", got, want)
	}
	if got, want := scheduled[1].call.ID, "independent"; got != want {
		t.Fatalf("second scheduled call mismatch: got %q want %q", got, want)
	}

	err = planner.ApplyResults([]scenarioBatchResult{
		{
			callID: "first",
			state: scenarioState{
				Status: StatusFailed,
				Failure: &Failure{
					Kind:    FailureKindAction,
					Phase:   PhaseRun,
					At:      "stage.main/call.first/action",
					Summary: "failed",
				},
			},
		},
		{
			callID: "independent",
			state:  scenarioState{Status: StatusPassed},
		},
	})
	if err != nil {
		t.Fatalf("apply results failed: %v", err)
	}

	scheduled, err = planner.NextReadyBatch()
	if err != nil {
		t.Fatalf("next ready batch failed: %v", err)
	}

	if got, want := len(scheduled), 1; got != want {
		t.Fatalf("scheduled count mismatch: got %d want %d", got, want)
	}
	if got, want := scheduled[0].call.ID, "recovery"; got != want {
		t.Fatalf("scheduled recovery call mismatch: got %q want %q", got, want)
	}
}

func TestPendingScenarioSkipperMarksStageAbortedAndRecordsEvent(t *testing.T) {
	t.Parallel()

	recorder := &recordingEventRecorder{}
	sink := newStageEventSink(nil, recorder)
	planner := newScenarioBatchPlanner(&stagePlan{
		ID:   "main",
		Path: "stage.main",
		Scenarios: []scenarioPlan{
			{ID: "scenario/pending"},
		},
		ScenarioCalls: []scenarioCallPlan{
			{
				ID:          "pending",
				Path:        "stage.main/call.pending",
				ScenarioID:  "scenario/pending",
				PlanOrdinal: 1,
				SourceSpan:  &SourceRef{File: "flow.yaml", Line: 12},
			},
		},
	})

	if err := newPendingScenarioSkipper(sink.Record).Skip(planner); err != nil {
		t.Fatalf("skip pending scenarios failed: %v", err)
	}

	state := planner.states["pending"]
	if got, want := state.Status, StatusSkipped; got != want {
		t.Fatalf("planner status mismatch: got %q want %q", got, want)
	}

	events := recorder.Snapshot()
	if got, want := len(events), 1; got != want {
		t.Fatalf("event count mismatch: got %d want %d", got, want)
	}
	if got, want := events[0].Kind, EventKindScenarioFinished; got != want {
		t.Fatalf("event kind mismatch: got %q want %q", got, want)
	}
	if got, want := events[0].SkipReason, SkipReasonStageAborted; got != want {
		t.Fatalf("skip reason mismatch: got %q want %q", got, want)
	}
	if got, want := events[0].Status, StatusSkipped; got != want {
		t.Fatalf("event status mismatch: got %q want %q", got, want)
	}
	if events[0].SourceSpan == nil || events[0].SourceSpan.File != "flow.yaml" {
		t.Fatalf("source span mismatch: %#v", events[0].SourceSpan)
	}
}

func TestStageEventSinkBuildsReportWithoutRawEventRetention(t *testing.T) {
	t.Parallel()

	sink := newStageEventSink(nil, nil)
	startedAt := time.Unix(1, 0).UTC()
	events := []Event{
		{
			Kind:      EventKindStageRunning,
			StageID:   "main",
			StagePath: "stage.main",
			Path:      "stage.main",
			Attempt:   1,
			Status:    StatusRunning,
		},
		{
			Kind:         EventKindScenarioFinished,
			StageID:      "main",
			StagePath:    "stage.main",
			ScenarioID:   "login",
			ScenarioPath: "stage.main/call.login-user",
			Path:         "stage.main/call.login-user",
			Attempt:      1,
			ScenarioSeq:  1,
			Status:       StatusPassed,
		},
		completeEvent(Event{
			Kind:      EventKindStageFinished,
			StageID:   "main",
			StagePath: "stage.main",
			Path:      "stage.main",
			Attempt:   1,
			Status:    StatusPassed,
		}, startedAt),
	}

	for _, event := range events {
		if err := sink.Record(event); err != nil {
			t.Fatalf("record event failed: %v", err)
		}
	}

	got, err := sink.Report()
	if err != nil {
		t.Fatalf("build report failed: %v", err)
	}

	want, err := NewProjector().Project(events)
	if err != nil {
		t.Fatalf("project replay report failed: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sink report mismatch:\n got=%#v\nwant=%#v", got, want)
	}
}

func TestStageEventSinkStopsWhenExplicitRecorderFails(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("recording failed")
	sink := newStageEventSink(nil, failingEventRecorder{err: wantErr})

	err := sink.Record(Event{
		Kind:      EventKindStageRunning,
		StageID:   "main",
		StagePath: "stage.main",
		Path:      "stage.main",
		Attempt:   1,
		Status:    StatusRunning,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("record error mismatch: got %v want %v", err, wantErr)
	}
}

func TestStageEventSinkAllowsReportWhileRecorderCallbackIsBlocked(t *testing.T) {
	t.Parallel()

	recorder := newBlockingEventRecorder()
	sink := newStageEventSink(nil, recorder)

	done := make(chan error, 1)
	go func() {
		done <- sink.Record(Event{
			Kind:      EventKindStageRunning,
			StageID:   "main",
			StagePath: "stage.main",
			Path:      "stage.main",
			Attempt:   1,
			Status:    StatusRunning,
		})
	}()

	recorder.WaitStarted(t)

	report, err := sink.Report()
	if err != nil {
		t.Fatalf("report while recorder blocked failed: %v", err)
	}
	if got, want := report.StagePath, "stage.main"; got != want {
		t.Fatalf("report stage path mismatch: got %q want %q", got, want)
	}

	recorder.Release()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("record event failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("record event did not finish")
	}
}

func TestStageEventSinkAllowsRecorderToReadReportReentrantly(t *testing.T) {
	t.Parallel()

	recorder := &reportReadingRecorder{}
	sink := newStageEventSink(nil, recorder)
	recorder.sink = sink

	err := sink.Record(Event{
		Kind:      EventKindStageRunning,
		StageID:   "main",
		StagePath: "stage.main",
		Path:      "stage.main",
		Attempt:   1,
		Status:    StatusRunning,
	})
	if err != nil {
		t.Fatalf("record event failed: %v", err)
	}

	report, reportErr := recorder.Result()
	if reportErr != nil {
		t.Fatalf("reentrant report failed: %v", reportErr)
	}
	if got, want := report.StagePath, "stage.main"; got != want {
		t.Fatalf("reentrant report stage path mismatch: got %q want %q", got, want)
	}
}

func TestStageRunnerReturnsFailedReportWhenRecorderPanicsDuringNonTerminalEvent(t *testing.T) {
	t.Parallel()

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
	if err := catalog.RegisterAction("action.login", executorTestAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	stage := prepareRuntimeStage(t, spec, catalog)
	report, err := newStageRunner(
		stage,
		catalog,
		nil,
		nil,
		nil,
		panickingEventRecorder{panicOnKind: EventKindActionRunning, message: "recorder boom"},
		nil,
	).Run(context.Background())
	if err != nil {
		t.Fatalf("stage run failed: %v", err)
	}

	if got, want := report.Status, StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if report.Failure == nil {
		t.Fatal("report failure must be present")
	}
	if got, want := report.Failure.Kind, FailureKindInternal; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}
	if got, want := report.Failure.Summary, "event recorder panicked"; got != want {
		t.Fatalf("failure summary mismatch: got %q want %q", got, want)
	}
	if got, want := report.Failure.Cause.Error(), "event recorder panicked: recorder boom"; got != want {
		t.Fatalf("failure cause mismatch: got %q want %q", got, want)
	}
}

func TestStageRunnerOverridesStageOutcomeWhenMirroredLiveSinkPanicsDuringStageFinished(t *testing.T) {
	t.Parallel()

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
	if err := catalog.RegisterAction("action.login", executorTestAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	stage := prepareRuntimeStage(t, spec, catalog)
	report, err := newStageRunner(
		stage,
		catalog,
		nil,
		nil,
		&panickingObserveSink{panicOnTransition: EventKindStageFinished, message: "live boom"},
		nil,
		nil,
	).Run(context.Background())
	if err != nil {
		t.Fatalf("stage run failed: %v", err)
	}

	if got, want := report.Status, StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if report.Failure == nil {
		t.Fatal("report failure must be present")
	}
	if got, want := report.Failure.Kind, FailureKindInternal; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}
	if got, want := report.Failure.Summary, "live sink panicked"; got != want {
		t.Fatalf("failure summary mismatch: got %q want %q", got, want)
	}
	if got, want := report.Failure.Cause.Error(), `live sink "stage.main" panicked: live boom`; got != want {
		t.Fatalf("failure cause mismatch: got %q want %q", got, want)
	}
	if got, want := report.Summary.TotalScenarios, 1; got != want {
		t.Fatalf("total scenarios mismatch: got %d want %d", got, want)
	}
	if got, want := report.Summary.PassedScenarios, 1; got != want {
		t.Fatalf("passed scenarios mismatch: got %d want %d", got, want)
	}
}

func TestScenarioBatchPlannerSummaryUsesPlanOrderForStageFailure(t *testing.T) {
	t.Parallel()

	planner := newScenarioBatchPlanner(&stagePlan{
		ID:   "main",
		Path: "stage.main",
		Scenarios: []scenarioPlan{
			{ID: "scenario/first"},
			{ID: "scenario/second"},
			{ID: "scenario/third"},
		},
		ScenarioCalls: []scenarioCallPlan{
			{ID: "third", Path: "stage.main/call.third", ScenarioID: "scenario/third", PlanOrdinal: 3},
			{ID: "first", Path: "stage.main/call.first", ScenarioID: "scenario/first", PlanOrdinal: 1},
			{ID: "second", Path: "stage.main/call.second", ScenarioID: "scenario/second", PlanOrdinal: 2},
		},
	})

	planner.states["third"] = scenarioState{
		Status: StatusFailed,
		Failure: &Failure{
			Kind:    FailureKindAction,
			Phase:   PhaseRun,
			At:      "stage.main/call.third/action",
			Summary: "third failed",
		},
	}
	planner.states["first"] = scenarioState{
		Status: StatusFailed,
		Failure: &Failure{
			Kind:    FailureKindExpectation,
			Phase:   PhaseRun,
			At:      "stage.main/call.first/act.verify",
			Summary: "first failed",
		},
	}
	planner.states["second"] = scenarioState{Status: StatusPassed}

	status, failure := planner.Summary()
	if got, want := status, StatusFailed; got != want {
		t.Fatalf("summary status mismatch: got %q want %q", got, want)
	}
	if failure == nil {
		t.Fatal("summary failure = nil, want first failed scenario call")
	}
	if got, want := failure.Summary, "first failed"; got != want {
		t.Fatalf("summary failure mismatch: got %q want %q", got, want)
	}
}

func prepareRuntimeStage(t *testing.T, spec StageSpec, catalog *Catalog) *stagePlan {
	t.Helper()

	stage := compileStageSpec(spec)
	prepared, err := planPreparer{catalog: catalog}.Prepare(stage)
	if err != nil {
		t.Fatalf("prepare stage failed: %v", err)
	}

	return prepared
}

type panickingEventRecorder struct {
	panicOnKind string
	message     string
}

type blockingEventRecorder struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

type reportReadingRecorder struct {
	sink *stageEventSink

	once   sync.Once
	report Report
	err    error
}

type panickingObserveSink struct {
	panicOnTransition string
	message           string
}

func (r panickingEventRecorder) Record(event Event) error {
	if event.Kind == r.panicOnKind {
		panic(r.message)
	}

	return nil
}

func newBlockingEventRecorder() *blockingEventRecorder {
	return &blockingEventRecorder{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (r *blockingEventRecorder) Record(Event) error {
	r.once.Do(func() {
		close(r.started)
	})
	<-r.release
	return nil
}

func (r *blockingEventRecorder) Release() {
	close(r.release)
}

func (r *blockingEventRecorder) WaitStarted(t *testing.T) {
	t.Helper()

	select {
	case <-r.started:
	case <-time.After(time.Second):
		t.Fatal("recorder callback did not start")
	}
}

func (r *reportReadingRecorder) Record(Event) error {
	report, err := r.sink.Report()
	r.once.Do(func() {
		r.report = report
		r.err = err
	})
	return nil
}

func (r *reportReadingRecorder) Result() (Report, error) {
	return r.report, r.err
}

func (s *panickingObserveSink) Publish(env observe.Envelope) uint64 {
	if env.Transition != nil && env.Transition.EventKind == s.panicOnTransition {
		panic(s.message)
	}

	return 0
}
