package theater

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/alex-poliushkin/theater/observe"
	statemodel "github.com/alex-poliushkin/theater/state"
)

type stageEventSink struct {
	report      *reportAccumulator
	recorder    EventRecorder
	live        observe.Sink
	debug       *debugRuntime
	identity    runDocumentIdentity
	mu          sync.Mutex
	callbackSeq uint64
	callbacks   *stageCallbackTurn
}

type scenarioBatchPlanner struct {
	stage       *stagePlan
	scenarios   scenarioAddressRegistry
	states      map[string]scenarioState
	scenarioSeq int
	policy      stageExecutionPolicy
	failedBatch bool
}

type scenarioBatchExecutor struct {
	http             *HTTPSpec
	catalog          runtimeCatalog
	matchers         MatcherResolver
	record           func(Event) error
	live             observe.Sink
	limit            int
	logLimiter       *scenarioLogLimiter
	generation       *generationRuntime
	scenarioScopeRun *scenarioScopeRun
	state            *statemodel.Manager
	debug            *debugRuntime
}

type pendingScenarioSkipper struct {
	record func(Event) error
}

func newStageEventSink(identity runDocumentIdentity, live observe.Sink, recorder EventRecorder, debug ...*debugRuntime) *stageEventSink {
	var runtimeDebug *debugRuntime
	if len(debug) != 0 {
		runtimeDebug = debug[0]
	}

	return &stageEventSink{
		report:    newReportAccumulator(),
		recorder:  recorder,
		live:      live,
		debug:     runtimeDebug,
		identity:  identity,
		callbacks: newStageCallbackTurn(),
	}
}

func newScenarioBatchPlanner(stage *stagePlan) *scenarioBatchPlanner {
	return newScenarioBatchPlannerWithPolicy(stage, defaultStageExecutionPolicy())
}

func newScenarioBatchPlannerWithPolicy(stage *stagePlan, policy stageExecutionPolicy) *scenarioBatchPlanner {
	return &scenarioBatchPlanner{
		stage:     stage,
		scenarios: newScenarioAddressRegistry(stage.Scenarios),
		states:    make(map[string]scenarioState, len(stage.ScenarioCalls)),
		policy:    policy,
	}
}

func newScenarioBatchExecutor(
	http *HTTPSpec,
	catalog runtimeCatalog,
	matchers MatcherResolver,
	record func(Event) error,
	live observe.Sink,
	scopeRun *scenarioScopeRun,
	state *statemodel.Manager,
	debug *debugRuntime,
) *scenarioBatchExecutor {
	return newScenarioBatchExecutorWithLimit(http, catalog, matchers, record, live, scopeRun, state, debug, defaultStageConcurrencyLimit())
}

func newScenarioBatchExecutorWithLimit(
	http *HTTPSpec,
	catalog runtimeCatalog,
	matchers MatcherResolver,
	record func(Event) error,
	live observe.Sink,
	scopeRun *scenarioScopeRun,
	state *statemodel.Manager,
	debug *debugRuntime,
	limit int,
) *scenarioBatchExecutor {
	return &scenarioBatchExecutor{
		http:             http,
		catalog:          catalog,
		matchers:         matchers,
		record:           record,
		live:             live,
		limit:            normalizeStageConcurrencyLimit(limit),
		scenarioScopeRun: scopeRun,
		state:            state,
		debug:            debug,
	}
}

func newPendingScenarioSkipper(record func(Event) error) *pendingScenarioSkipper {
	return &pendingScenarioSkipper{record: record}
}

func (s *stageEventSink) Record(event Event) error {
	event = s.identify(event)
	if err := event.Validate(); err != nil {
		return err
	}

	var (
		recorder    EventRecorder
		live        observe.Sink
		envelope    observe.Envelope
		mirrored    bool
		callbackSeq uint64
	)

	s.mu.Lock()
	s.report.applyValidated(event)
	callbackSeq = s.callbackSeq
	s.callbackSeq++
	if s.debug != nil {
		s.debug.storeDurableEventSeq(callbackSeq + 1)
	}
	recorder = s.recorder
	live = s.live
	if live != nil {
		envelope, mirrored = mirroredEnvelopeFromEvent(event)
	}
	s.mu.Unlock()

	return s.callbacks.Run(callbackSeq, func() error {
		if recorder != nil {
			if err := invokeBoundaryError("event recorder", "", func() error {
				return recorder.Record(event)
			}); err != nil {
				var panicErr boundaryPanicError
				if errors.As(err, &panicErr) {
					return newContainedObserverError(event, "event recorder panicked", err)
				}

				return err
			}
		}

		if live != nil && mirrored {
			if err := invokeBoundaryError("live sink", event.Path, func() error {
				live.Publish(envelope)
				return nil
			}); err != nil {
				var panicErr boundaryPanicError
				if errors.As(err, &panicErr) {
					return newContainedObserverError(event, "live sink panicked", err)
				}

				return err
			}
		}

		return nil
	})
}

func (s *stageEventSink) Report() (Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.report.Report()
}

func (s *stageEventSink) recordLocal(event Event) error {
	event = s.identify(event)
	if err := event.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.report.applyValidated(event)
	return nil
}

func (s *stageEventSink) identify(event Event) Event {
	event.RunID = s.identity.runID
	event.TheaterVersion = s.identity.theaterVersion
	return event
}

type stageCallbackTurn struct {
	mu   sync.Mutex
	cond *sync.Cond
	next uint64
}

func newStageCallbackTurn() *stageCallbackTurn {
	turn := &stageCallbackTurn{}
	turn.cond = sync.NewCond(&turn.mu)
	return turn
}

func (t *stageCallbackTurn) Run(seq uint64, fn func() error) error {
	t.mu.Lock()
	for seq != t.next {
		t.cond.Wait()
	}
	// Recorder and mirrored live callbacks must observe the same sink order,
	// but that ordering should not rely on the main report mutex.
	defer func() {
		t.next++
		t.cond.Broadcast()
		t.mu.Unlock()
	}()

	return fn()
}

func (p *scenarioBatchPlanner) NextReadyBatch() ([]scheduledScenarioRun, error) {
	ready := readyScenarioCalls(p.stage, p.states)
	if p.failedBatch {
		ready = p.policy.filterReadyAfterFailure(ready)
	}
	if len(ready) == 0 {
		return nil, nil
	}

	scheduled := make([]scheduledScenarioRun, 0, len(ready))
	for _, scenarioCall := range ready {
		scenario, ok := p.scenarios.Resolve(scenarioAddress(scenarioCall.ScenarioID))
		if !ok {
			return nil, fmt.Errorf("scenario %q is missing in plan", scenarioCall.ScenarioID)
		}

		scheduled = append(scheduled, scheduledScenarioRun{
			call:          scenarioCall,
			scenario:      scenario,
			bindingSource: scenarioBindingSource(scenarioCall.Dependencies, p.states),
			identity:      p.nextIdentity(scenarioCall, scenario.ID),
		})
	}

	return scheduled, nil
}

func (p *scenarioBatchPlanner) ApplyResults(results []scenarioBatchResult) error {
	failed := false
	for i := range results {
		if results[i].err != nil {
			return results[i].err
		}

		p.states[results[i].callID] = results[i].state
		if results[i].state.Status == StatusFailed || results[i].state.Status == StatusCanceled {
			failed = true
		}
	}

	if failed {
		p.failedBatch = true
	}

	return nil
}

func (p *scenarioBatchPlanner) PendingCalls() []scenarioCallPlan {
	return pendingScenarioCalls(p.stage, p.states)
}

func (p *scenarioBatchPlanner) MarkSkipped(callID string) {
	p.states[callID] = scenarioState{Status: StatusSkipped}
}

func (p *scenarioBatchPlanner) Summary() (status Status, failure *Failure) {
	return summarizeStage(p.stage, p.states)
}

func (p *scenarioBatchPlanner) HasFailedScenario() bool {
	return p.failedBatch
}

func (p *scenarioBatchPlanner) FailurePolicy() stageFailurePolicy {
	return p.policy.FailureBehavior
}

func (p *scenarioBatchPlanner) nextIdentity(call scenarioCallPlan, scenarioID string) executionIdentity {
	p.scenarioSeq++
	return executionIdentity{
		stageID:        p.stage.ID,
		stagePath:      p.stage.Path,
		scenarioID:     scenarioID,
		scenarioCallID: call.ID,
		scenarioPath:   call.Path,
		scenarioSeq:    p.scenarioSeq,
	}
}

func (e *scenarioBatchExecutor) Execute(ctx context.Context, scheduled []scheduledScenarioRun) []scenarioBatchResult {
	results := make([]scenarioBatchResult, len(scheduled))
	if len(scheduled) == 0 {
		return results
	}

	jobs := make(chan int)
	workerCount := e.workerCount(len(scheduled))
	var wg sync.WaitGroup
	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for i := range jobs {
				state, err := newScenarioRunner(
					scheduled[i].identity,
					scheduled[i].scenario,
					scheduled[i].call,
					e.http,
					e.catalog,
					e.matchers,
					e.live,
					scheduled[i].bindingSource,
					e.generation,
					e.state,
					e.record,
					e.scenarioScopeRun,
					e.logLimiter,
					e.debug,
				).Run(ctx)
				results[i] = scenarioBatchResult{
					callID: scheduled[i].call.ID,
					state:  state,
					err:    err,
				}
			}
		}()
	}

	for i := range scheduled {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	return results
}

func (e *scenarioBatchExecutor) workerCount(batchSize int) int {
	if e != nil && e.debug.usesInteractiveSerialScheduling() {
		return 1
	}

	workers := e.limit
	if batchSize < workers {
		return batchSize
	}

	return workers
}

func defaultStageConcurrencyLimit() int {
	return normalizeStageConcurrencyLimit(runtime.GOMAXPROCS(0))
}

func normalizeStageConcurrencyLimit(limit int) int {
	if limit < 1 {
		return 1
	}

	return limit
}

func (s *pendingScenarioSkipper) Skip(planner *scenarioBatchPlanner) error {
	for _, scenarioCall := range planner.PendingCalls() {
		planner.MarkSkipped(scenarioCall.ID)
		skippedAt := time.Now().UTC()
		recorder := newExecutionRecorder(planner.nextIdentity(scenarioCall, scenarioCall.ScenarioID), s.record)
		if err := recorder.scenario().finished(1, executionNodeResult{
			status:     StatusSkipped,
			skipReason: SkipReasonStageAborted,
			startedAt:  skippedAt,
			endedAt:    skippedAt,
			sourceSpan: cloneSourceRef(scenarioCall.SourceSpan),
		}); err != nil {
			return err
		}
	}

	return nil
}

func (r *stageRunner) finishContainedFailure(err error) (Report, error) {
	var contained containedStageFailure
	if !errors.As(err, &contained) {
		return Report{}, err
	}

	status := StatusFailed
	failure := contained.stageFailure()
	switch {
	case errors.Is(err, context.Canceled):
		status = StatusCanceled
		failure = nil
	case errors.Is(err, context.DeadlineExceeded):
		failure = &Failure{
			Kind:    FailureKindTimeout,
			Phase:   PhaseRun,
			At:      r.stage.Path,
			Summary: "stage run timed out",
			Cause:   err,
		}
	}

	if err := r.sink.recordLocal(completeEvent(Event{
		Kind:      EventKindStageFinished,
		StageID:   r.stage.ID,
		StagePath: r.stage.Path,
		Path:      r.stage.Path,
		Attempt:   1,
		Status:    status,
		Failure:   failure,
	}, r.startedAt)); err != nil {
		return Report{}, err
	}

	return r.sink.Report()
}
