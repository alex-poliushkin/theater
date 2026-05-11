package theater

import (
	"context"
	"fmt"
	"time"

	"github.com/alex-poliushkin/theater/observe"
	statemodel "github.com/alex-poliushkin/theater/state"
)

type stageRunner struct {
	stage            *stagePlan
	sink             *stageEventSink
	planner          *scenarioBatchPlanner
	executor         *scenarioBatchExecutor
	generation       *generationRuntime
	skipper          *pendingScenarioSkipper
	scenarioScopeRun *scenarioScopeRun
	state            *statemodel.Manager
	startedAt        time.Time
}

type scenarioRunner struct {
	startedAt     time.Time
	identity      executionIdentity
	scenario      *scenarioPlan
	call          scenarioCallPlan
	http          *HTTPSpec
	catalog       runtimeCatalog
	matchers      MatcherResolver
	bindingSource Values
	generation    *generationRuntime
	recorder      executionRecorder
	scenarioNode  executionNode
	resources     ResourceScope
	live          observe.Sink
	scope         *valueScope
	state         *statemodel.Manager
	logLimiter    *scenarioLogLimiter
	debug         *debugRuntime
	snapshot      debugSnapshotBuilder
	inputSection  debugSnapshotSection
}

type actRunner struct {
	identity      executionIdentity
	act           *actPlan
	http          *HTTPSpec
	catalog       runtimeCatalog
	matchers      MatcherResolver
	resources     ResourceScope
	live          observe.Sink
	scenarioScope *valueScope
	generation    *generationRuntime
	recorder      executionRecorder
	actNode       executionNode
	state         *statemodel.Manager
	logLimiter    *scenarioLogLimiter
	debug         *debugRuntime
	snapshot      debugSnapshotBuilder
}

func newStageRunner(
	stage *stagePlan,
	catalog runtimeCatalog,
	matchers MatcherResolver,
	state *statemodel.Manager,
	live observe.Sink,
	recorder EventRecorder,
	debug *debugRuntime,
) *stageRunner {
	sink := newStageEventSink(live, recorder, debug)
	scenarioScopeRun := newScenarioScopeRun(catalog)
	executor := newScenarioBatchExecutor(stage.HTTP, catalog, matchers, sink.Record, live, scenarioScopeRun, state, debug)
	executor.logLimiter = newScenarioLogLimiter()
	return &stageRunner{
		stage:            stage,
		sink:             sink,
		planner:          newScenarioBatchPlanner(stage),
		executor:         executor,
		skipper:          newPendingScenarioSkipper(sink.Record),
		scenarioScopeRun: scenarioScopeRun,
		state:            state,
	}
}

func newScenarioRunner(
	identity executionIdentity,
	scenario *scenarioPlan,
	call scenarioCallPlan,
	http *HTTPSpec,
	catalog runtimeCatalog,
	matchers MatcherResolver,
	live observe.Sink,
	bindingSource Values,
	generation *generationRuntime,
	state *statemodel.Manager,
	record func(event Event) error,
	scopeRun *scenarioScopeRun,
	logLimiter *scenarioLogLimiter,
	debug *debugRuntime,
) *scenarioRunner {
	recorder := newExecutionRecorder(identity, record)
	return &scenarioRunner{
		identity:      identity,
		scenario:      scenario,
		call:          call,
		http:          http,
		catalog:       catalog,
		matchers:      matchers,
		bindingSource: bindingSource,
		generation:    generation,
		recorder:      recorder,
		scenarioNode:  recorder.scenario(),
		resources:     newScenarioResources(scopeRun),
		live:          live,
		scope:         newValueScope(nil),
		state:         state,
		logLimiter:    logLimiter,
		debug:         debug,
		snapshot:      newDebugSnapshotBuilder(),
	}
}

func newActRunner(
	identity executionIdentity,
	act *actPlan,
	http *HTTPSpec,
	catalog runtimeCatalog,
	matchers MatcherResolver,
	resources ResourceScope,
	live observe.Sink,
	scenarioScope *valueScope,
	generation *generationRuntime,
	state *statemodel.Manager,
	record func(event Event) error,
	logLimiter *scenarioLogLimiter,
	debug *debugRuntime,
) *actRunner {
	recorder := newExecutionRecorder(identity, record)
	return &actRunner{
		identity:      identity,
		act:           act,
		http:          http,
		catalog:       catalog,
		matchers:      matchers,
		resources:     resources,
		live:          live,
		scenarioScope: scenarioScope,
		generation:    generation,
		recorder:      recorder,
		actNode:       recorder.act(act.ID),
		state:         state,
		logLimiter:    logLimiter,
		debug:         debug,
		snapshot:      newDebugSnapshotBuilder(),
	}
}

func (r *stageRunner) Run(ctx context.Context) (Report, error) {
	if r.scenarioScopeRun != nil {
		defer r.scenarioScopeRun.Close()
	}

	r.startedAt = time.Now().UTC()
	r.generation = newGenerationRuntime(r.startedAt)
	r.executor.generation = r.generation
	if err := r.sink.Record(Event{
		Kind:       EventKindStageRunning,
		StageID:    r.stage.ID,
		StagePath:  r.stage.Path,
		Path:       r.stage.Path,
		Attempt:    1,
		Status:     StatusRunning,
		Generation: r.generation.Metadata(),
	}); err != nil {
		return r.finishContainedFailure(err)
	}

	for {
		ready, err := r.planner.NextReadyBatch()
		if err != nil {
			return Report{}, err
		}
		if len(ready) == 0 {
			break
		}

		scheduled := r.scheduleBatch(ready)
		r.updateDebugScheduler(ready, scheduled)
		results := r.executor.Execute(ctx, scheduled)
		if err := r.planner.ApplyResults(results); err != nil {
			return r.finishContainedFailure(err)
		}
	}

	if err := r.skipper.Skip(r.planner); err != nil {
		return r.finishContainedFailure(err)
	}

	stageStatus, stageFailure := r.planner.Summary()
	if err := r.sink.Record(completeEvent(Event{
		Kind:       EventKindStageFinished,
		StageID:    r.stage.ID,
		StagePath:  r.stage.Path,
		Path:       r.stage.Path,
		Attempt:    1,
		Status:     stageStatus,
		Failure:    stageFailure,
		Generation: r.generation.Metadata(),
	}, r.startedAt)); err != nil {
		return r.finishContainedFailure(err)
	}

	return r.sink.Report()
}

func (r *stageRunner) scheduleBatch(ready []scheduledScenarioRun) []scheduledScenarioRun {
	if len(ready) == 0 {
		return nil
	}
	if r == nil || r.executor == nil || !r.executor.debug.usesInteractiveSerialScheduling() {
		return ready
	}

	return ready[:1]
}

func newScenarioResources(scopeRun *scenarioScopeRun) ResourceScope {
	resources := NewResourceScope()
	if scopeRun != nil {
		scopeRun.Initialize(resources)
	}

	return resources
}

func (r *scenarioRunner) Run(ctx context.Context) (scenarioState, error) {
	if len(r.scenario.Acts) == 0 {
		return scenarioState{}, fmt.Errorf("scenario %q has no acts", r.scenario.ID)
	}

	r.startedAt = time.Now().UTC()
	if err := r.scenarioNode.running(1); err != nil {
		return scenarioState{}, err
	}

	terminalState, terminated, err := r.applyBindings(ctx)
	if err != nil {
		return scenarioState{}, err
	}

	if terminated {
		return terminalState, nil
	}

	if r.debug.shouldEmitBoundaryKind(debugBoundaryKindScenarioCall) {
		if err := r.atBoundary(
			ctx,
			r.scenarioNode,
			debugBoundaryKindScenarioCall,
			debugBoundaryPhaseBefore,
			1,
			StatusRunning,
			nil,
			r.snapshot.scopeSection(r.scope),
			r.inputSection,
			debugSnapshotSection{},
		); err != nil {
			return scenarioState{}, err
		}
	}
	acts := indexActs(r.scenario.Acts)
	currentActID := r.scenario.Acts[0].ID
	steps := 0

	for currentActID != "" {
		act, ok := acts[currentActID]
		if !ok {
			return scenarioState{}, fmt.Errorf("act %q is missing in scenario %q", currentActID, r.scenario.ID)
		}

		steps++
		if steps > len(acts)+1 {
			return scenarioState{}, fmt.Errorf("act graph for scenario %q is not terminating", r.scenario.ID)
		}

		outcome, err := newActRunner(
			r.identity,
			act,
			r.http,
			r.catalog,
			r.matchers,
			r.resources,
			r.live,
			r.scope,
			r.generation,
			r.state,
			r.recorder.record,
			r.logLimiter,
			r.debug,
		).Run(ctx)
		if err != nil {
			return scenarioState{}, err
		}

		state, nextActID, terminated, err := r.advanceAfterAct(ctx, act, outcome)
		if err != nil {
			return scenarioState{}, err
		}

		if terminated {
			return state, nil
		}

		currentActID = nextActID
	}

	exported, err := newReferenceResolver(r.scope).
		withDecorators(r.catalog).
		withGeneration(r.catalog, r.generation, r.identity).
		withMatchers(r.matchers).
		ExportValuesContext(ctx, r.call.Exports)
	if err != nil {
		return r.finish(ctx, StatusFailed, internalFailure(r.call.Path, "scenario export failed", err))
	}

	if err := r.recordScenarioFinished(ctx, StatusPassed, nil, exported); err != nil {
		return scenarioState{}, err
	}

	return scenarioState{Status: StatusPassed, Exports: exported}, nil
}

func (r *scenarioRunner) applyBindings(ctx context.Context) (scenarioState, bool, error) {
	bindings, err := newReferenceResolver(r.bindingSource).
		withDecorators(r.catalog).
		withGeneration(r.catalog, r.generation, r.identity).
		withMatchers(r.matchers).
		ResolveBindingsContext(ctx, r.call.Bindings)
	if err != nil {
		state, finishErr := r.finish(ctx, StatusFailed, internalFailure(r.call.Path, "scenario binding failed", err))
		return state, true, finishErr
	}

	protectedBindings := protectValues(bindings, r.scenario.Inputs)
	r.scope.writeAll(protectedBindings)
	r.inputSection = r.snapshot.valuesSectionWithSources(
		protectedBindings,
		r.scenario.Inputs,
		"scenario.input",
		bindingSourceSpans(r.call.Bindings),
	)
	return scenarioState{}, false, nil
}

func (r *scenarioRunner) scenarioResult(
	status Status,
	failure *Failure,
	startedAt time.Time,
	endedAt time.Time,
) executionNodeResult {
	return executionNodeResult{
		status:     status,
		failure:    failure,
		startedAt:  startedAt,
		endedAt:    endedAt,
		sourceSpan: cloneSourceRef(r.call.SourceSpan),
	}
}

func (r *scenarioRunner) recordScenarioFinished(ctx context.Context, status Status, failure *Failure, exported Values) error {
	endedAt := time.Now().UTC()
	if !r.debug.shouldEmitBoundaryKind(debugBoundaryKindScenarioCall) {
		return r.scenarioNode.finished(1, r.scenarioResult(status, failure, r.startedAt, endedAt))
	}

	outputSection := r.snapshot.valuesSection(exported, nil, "scenario.export")
	boundaryState := r.boundaryState(
		r.scenarioNode,
		debugBoundaryKindScenarioCall,
		debugBoundaryPhaseAfter,
		1,
		status,
		failure,
		r.snapshot.scopeSection(r.scope),
		r.inputSection,
		outputSection,
	)
	if err := r.debug.atBoundary(ctx, boundaryState); err != nil {
		return err
	}
	if failure == nil {
		return r.scenarioNode.finished(1, r.scenarioResult(status, failure, r.startedAt, endedAt))
	}
	if err := r.debug.atTerminalFailure(ctx, boundaryState); err != nil {
		return err
	}

	return r.scenarioNode.finished(1, r.scenarioResult(status, failure, r.startedAt, endedAt))
}

func (r *scenarioRunner) atBoundary(
	ctx context.Context,
	node executionNode,
	kind debugBoundaryKind,
	phase debugBoundaryPhase,
	attempt int,
	status Status,
	failure *Failure,
	scope debugSnapshotSection,
	inputs debugSnapshotSection,
	output debugSnapshotSection,
) error {
	return r.debug.atBoundary(ctx, r.boundaryState(node, kind, phase, attempt, status, failure, scope, inputs, output))
}

func (r *scenarioRunner) boundaryState(
	node executionNode,
	kind debugBoundaryKind,
	phase debugBoundaryPhase,
	attempt int,
	status Status,
	failure *Failure,
	scope debugSnapshotSection,
	inputs debugSnapshotSection,
	output debugSnapshotSection,
) debugBoundaryState {
	return debugBoundaryState{
		Ref: debugBoundaryRef{
			StageID:        r.identity.stageID,
			StagePath:      r.identity.stagePath,
			ScenarioID:     r.identity.scenarioID,
			ScenarioCallID: r.identity.scenarioCallID,
			ScenarioPath:   r.identity.scenarioPath,
			ActID:          node.address.actID,
			NodeRef:        node.address.nodeRef,
			Path:           node.path,
			Kind:           kind,
			Phase:          phase,
			Attempt:        attempt,
			SourceSpan:     cloneSourceRef(r.call.SourceSpan),
		},
		Status:    status,
		Failure:   failure,
		Resources: r.resources,
		Scope:     scope,
		Inputs:    inputs,
		Output:    output,
	}
}

func (r *scenarioRunner) advanceAfterAct(
	ctx context.Context,
	act *actPlan,
	outcome actOutcome,
) (state scenarioState, nextActID string, terminated bool, err error) {
	switch outcome.status {
	case StatusPassed:
		r.scope.writeAll(outcome.values)
		nextActID, ok := nextTransitionID(act, TransitionOnPass)
		if !ok {
			return scenarioState{Status: StatusRunning}, "", false, nil
		}

		return scenarioState{Status: StatusRunning}, nextActID, false, nil
	case StatusCanceled:
		nextID, ok := nextTransitionID(act, TransitionOnCancel)
		if ok {
			return scenarioState{Status: StatusRunning}, nextID, false, nil
		}

		state, err := r.finish(ctx, StatusCanceled, nil)
		return state, "", true, err
	case StatusFailed:
		nextID, ok := nextFailureTransitionID(act, outcome.failure)
		if ok {
			return scenarioState{Status: StatusRunning}, nextID, false, nil
		}

		state, err := r.finish(ctx, StatusFailed, outcome.failure)
		return state, "", true, err
	default:
		return scenarioState{}, "", false, fmt.Errorf("act %q produced unsupported status %q", act.ID, outcome.status)
	}
}

func (r *scenarioRunner) finish(ctx context.Context, status Status, failure *Failure) (scenarioState, error) {
	state := scenarioState{
		Status:  status,
		Failure: failure,
	}

	if err := r.recordScenarioFinished(ctx, status, failure, nil); err != nil {
		return scenarioState{}, err
	}

	return state, nil
}

func (r *actRunner) Run(ctx context.Context) (actOutcome, error) {
	if r.act.Eventually != nil {
		return r.runEventually(ctx)
	}

	return r.runSingle(ctx)
}

func (r *actRunner) path() string {
	return r.actNode.path
}

func (r *actRunner) recordRunning(attempt int) error {
	return r.actNode.running(attempt)
}

func (r *actRunner) actResult(
	status Status,
	failure *Failure,
	eventually *EventuallyReport,
	startedAt time.Time,
	endedAt time.Time,
) executionNodeResult {
	return executionNodeResult{
		status:     status,
		failure:    failure,
		startedAt:  startedAt,
		endedAt:    endedAt,
		sourceSpan: cloneSourceRef(r.act.SourceSpan),
		eventually: eventually,
	}
}

func (r *actRunner) recordBeforeBoundary(ctx context.Context) error {
	if !r.debug.shouldEmitBoundaryKind(debugBoundaryKindAct) {
		return nil
	}

	return r.debug.atBoundary(ctx, r.boundaryState(
		debugBoundaryPhaseBefore,
		1,
		StatusRunning,
		nil,
		r.snapshot.scopeSection(r.scenarioScope),
		debugSnapshotSection{},
		debugSnapshotSection{},
	))
}

func (r *actRunner) recordTerminal(
	ctx context.Context,
	attempt int,
	status Status,
	failure *Failure,
	eventually *EventuallyReport,
	startedAt time.Time,
	endedAt time.Time,
	outputs Values,
) error {
	if !r.debug.shouldEmitBoundaryKind(debugBoundaryKindAct) {
		return r.actNode.finished(attempt, r.actResult(status, failure, eventually, startedAt, endedAt))
	}

	boundaryState := r.boundaryState(
		debugBoundaryPhaseAfter,
		attempt,
		status,
		failure,
		r.snapshot.scopeSection(r.scenarioScope),
		debugSnapshotSection{},
		r.snapshot.valuesSection(outputs, nil, "act.output"),
	)
	if err := r.debug.atBoundary(ctx, boundaryState); err != nil {
		return err
	}
	if failure != nil {
		if err := r.debug.atTerminalFailure(ctx, boundaryState); err != nil {
			return err
		}
	}

	return r.actNode.finished(attempt, r.actResult(status, failure, eventually, startedAt, endedAt))
}

func (r *actRunner) boundaryState(
	phase debugBoundaryPhase,
	attempt int,
	status Status,
	failure *Failure,
	scope debugSnapshotSection,
	inputs debugSnapshotSection,
	output debugSnapshotSection,
) debugBoundaryState {
	return debugBoundaryState{
		Ref: debugBoundaryRef{
			StageID:        r.identity.stageID,
			StagePath:      r.identity.stagePath,
			ScenarioID:     r.identity.scenarioID,
			ScenarioCallID: r.identity.scenarioCallID,
			ScenarioPath:   r.identity.scenarioPath,
			ActID:          r.actNode.address.actID,
			NodeRef:        r.actNode.address.nodeRef,
			Path:           r.actNode.path,
			Kind:           debugBoundaryKindAct,
			Phase:          phase,
			Attempt:        attempt,
			SourceSpan:     cloneSourceRef(r.act.SourceSpan),
		},
		Status:    status,
		Failure:   failure,
		Resources: r.resources,
		Scope:     scope,
		Inputs:    inputs,
		Output:    output,
	}
}

func (r *actRunner) runAttempt(ctx context.Context, attempt int) (actOutcome, AttemptReport, error) {
	attemptReport := AttemptReport{
		Index:     attempt,
		StartedAt: time.Now().UTC(),
	}

	if err := r.recordRunning(attempt); err != nil {
		return actOutcome{}, AttemptReport{}, err
	}

	outcome, err := newActExecution(r).Run(ctx, attempt)
	if err != nil {
		return actOutcome{}, AttemptReport{}, err
	}

	return outcome, finalizeAttemptReport(attemptReport, outcome), nil
}

func (r *actRunner) finishPassed(
	ctx context.Context,
	attempt int,
	actionOutputs Values,
	propertyValues Values,
	startedAt time.Time,
	endedAt time.Time,
	eventually *EventuallyReport,
) (actOutcome, error) {
	refSource := layeredValueLookup{
		primary:  mapValueLookup(propertyValues),
		fallback: r.scenarioScope,
	}
	selectorSource := layeredValueLookup{
		primary:  mapValueLookup(actionOutputs),
		fallback: refSource,
	}
	exported, err := newReferenceResolver(mapValueLookup(actionOutputs)).
		withBindingSource(selectorSource).
		withExportRefSource(refSource).
		withDecorators(r.catalog).
		withMatchers(r.matchers).
		ExportValuesContext(ctx, r.act.Exports)
	if err != nil {
		failure := internalFailure(r.path(), "act export failed", err)
		return actOutcome{status: StatusFailed, failure: failure}, r.recordTerminal(
			ctx,
			attempt,
			StatusFailed,
			failure,
			eventually,
			startedAt,
			endedAt,
			nil,
		)
	}

	if err := r.recordTerminal(ctx, attempt, StatusPassed, nil, eventually, startedAt, endedAt, exported); err != nil {
		return actOutcome{}, err
	}

	return actOutcome{status: StatusPassed, values: exported, properties: propertyValues, eventually: eventually}, nil
}

func (r *actRunner) runSingle(ctx context.Context) (actOutcome, error) {
	if err := r.recordBeforeBoundary(ctx); err != nil {
		return actOutcome{}, err
	}

	outcome, attemptReport, err := r.runAttempt(ctx, 1)
	if err != nil {
		return actOutcome{}, err
	}

	if outcome.status == StatusPassed {
		return r.finishPassed(ctx, 1, outcome.values, outcome.properties, attemptReport.StartedAt, attemptReport.EndedAt, nil)
	}

	if err := r.emitDebugTerminalFailure(ctx, outcome); err != nil {
		return actOutcome{}, err
	}

	return outcome, r.recordTerminal(ctx, 1, outcome.status, outcome.failure, nil, attemptReport.StartedAt, attemptReport.EndedAt, nil)
}

func (r *actRunner) runEventually(ctx context.Context) (actOutcome, error) {
	startedAt := time.Now().UTC()
	actCtx, cancel := context.WithTimeout(ctx, r.act.Eventually.Timeout)
	defer cancel()

	if err := r.recordBeforeBoundary(ctx); err != nil {
		return actOutcome{}, err
	}

	timeline := make([]AttemptReport, 0, 1)
	var lastObservedFailure *Failure
	var lastObservedBoundary *debugBoundaryState

	for attempt := 1; ; attempt++ {
		outcome, attemptReport, err := r.runAttempt(actCtx, attempt)
		if err != nil {
			return actOutcome{}, err
		}

		retryable, updatedTimeline, observedFailure, observedBoundary := recordEventuallyAttempt(
			outcome,
			attemptReport,
			timeline,
			lastObservedFailure,
			lastObservedBoundary,
		)
		timeline = updatedTimeline
		lastObservedFailure = observedFailure
		lastObservedBoundary = observedBoundary

		if actCtx.Err() != nil {
			outcome, err := r.finishEventuallyByContext(
				ctx,
				actCtx.Err(),
				startedAt,
				attempt,
				lastObservedFailure,
				lastObservedBoundary,
				timeline,
			)
			if err != nil {
				return actOutcome{}, err
			}
			if err := r.emitDebugTerminalFailure(ctx, outcome); err != nil {
				return actOutcome{}, err
			}

			return outcome, nil
		}

		switch {
		case outcome.status == StatusPassed:
			return r.finishEventuallyPassed(ctx, attempt, startedAt, outcome.values, outcome.properties, lastObservedFailure, timeline)
		case outcome.status == StatusCanceled:
			return r.finishEventuallyCanceled(ctx, attempt, startedAt, attemptReport.EndedAt, lastObservedFailure, timeline)
		case !retryable:
			if err := r.emitDebugTerminalFailure(actCtx, outcome); err != nil {
				return actOutcome{}, err
			}
			return r.finishEventuallyTerminalFailure(ctx, attempt, startedAt, attemptReport.EndedAt, outcome.failure, lastObservedFailure, timeline)
		}

		if !waitForEventuallyInterval(actCtx, r.act.Eventually.Interval) {
			outcome, err := r.finishEventuallyByContext(
				ctx,
				actCtx.Err(),
				startedAt,
				attempt,
				lastObservedFailure,
				lastObservedBoundary,
				timeline,
			)
			if err != nil {
				return actOutcome{}, err
			}
			if err := r.emitDebugTerminalFailure(ctx, outcome); err != nil {
				return actOutcome{}, err
			}

			return outcome, nil
		}
	}
}

func (r *actRunner) emitDebugTerminalFailure(ctx context.Context, outcome actOutcome) error {
	if r == nil || r.debug == nil || outcome.debugTerminalBoundary == nil {
		return nil
	}

	return r.debug.atTerminalFailure(ctx, *outcome.debugTerminalBoundary)
}

func (r *actRunner) finishEventuallyPassed(
	ctx context.Context,
	attempt int,
	startedAt time.Time,
	actionOutputs Values,
	propertyValues Values,
	lastObservedFailure *Failure,
	timeline []AttemptReport,
) (actOutcome, error) {
	eventually := buildEventuallyReport(
		r.act.Eventually,
		startedAt,
		timeline[len(timeline)-1].EndedAt,
		StatusPassed,
		TerminationReasonConverged,
		nil,
		lastObservedFailure,
		timeline,
		attempt,
	)

	return r.finishPassed(
		ctx,
		attempt,
		actionOutputs,
		propertyValues,
		startedAt,
		timeline[len(timeline)-1].EndedAt,
		eventually,
	)
}

func (r *actRunner) finishEventuallyCanceled(
	ctx context.Context,
	attempt int,
	startedAt time.Time,
	endedAt time.Time,
	lastObservedFailure *Failure,
	timeline []AttemptReport,
) (actOutcome, error) {
	eventually := buildEventuallyReport(
		r.act.Eventually,
		startedAt,
		endedAt,
		StatusCanceled,
		TerminationReasonParentCanceled,
		nil,
		lastObservedFailure,
		timeline,
		0,
	)

	return actOutcome{
		status:     StatusCanceled,
		eventually: eventually,
	}, r.recordTerminal(ctx, attempt, StatusCanceled, nil, eventually, startedAt, endedAt, nil)
}

func (r *actRunner) finishEventuallyTerminalFailure(
	ctx context.Context,
	attempt int,
	startedAt time.Time,
	endedAt time.Time,
	failure *Failure,
	lastObservedFailure *Failure,
	timeline []AttemptReport,
) (actOutcome, error) {
	eventually := buildEventuallyReport(
		r.act.Eventually,
		startedAt,
		endedAt,
		StatusFailed,
		TerminationReasonTerminalFailure,
		failure,
		lastObservedFailure,
		timeline,
		0,
	)

	return actOutcome{
		status:     StatusFailed,
		failure:    failure,
		eventually: eventually,
	}, r.recordTerminal(ctx, attempt, StatusFailed, failure, eventually, startedAt, endedAt, nil)
}

func (r *actRunner) finishEventuallyByContext(
	parentCtx context.Context,
	err error,
	startedAt time.Time,
	attempt int,
	lastObservedFailure *Failure,
	lastObservedBoundary *debugBoundaryState,
	timeline []AttemptReport,
) (actOutcome, error) {
	endedAt := time.Now().UTC()
	if parentCtx.Err() != nil {
		eventually := buildEventuallyReport(
			r.act.Eventually,
			startedAt,
			endedAt,
			StatusCanceled,
			TerminationReasonParentCanceled,
			nil,
			lastObservedFailure,
			timeline,
			0,
		)

		return actOutcome{status: StatusCanceled, eventually: eventually}, r.recordTerminal(
			parentCtx,
			attempt,
			StatusCanceled,
			nil,
			eventually,
			startedAt,
			endedAt,
			nil,
		)
	}

	failure := &Failure{
		Kind:    FailureKindTimeout,
		Phase:   PhaseRun,
		At:      r.path(),
		Summary: "eventually deadline exceeded",
		Cause:   err,
	}

	eventually := buildEventuallyReport(
		r.act.Eventually,
		startedAt,
		endedAt,
		StatusFailed,
		TerminationReasonDeadlineExceeded,
		failure,
		lastObservedFailure,
		timeline,
		0,
	)

	return actOutcome{
		status:                StatusFailed,
		failure:               failure,
		eventually:            eventually,
		debugTerminalBoundary: debugTerminalBoundaryForTimeout(lastObservedBoundary, failure),
	}, r.recordTerminal(parentCtx, attempt, StatusFailed, failure, eventually, startedAt, endedAt, nil)
}

func debugTerminalBoundaryForTimeout(state *debugBoundaryState, failure *Failure) *debugBoundaryState {
	if state == nil {
		return nil
	}

	return debugTerminalBoundary(*state, failure)
}

func completeEvent(event Event, startedAt time.Time) Event {
	return timedEvent(event, startedAt, time.Now().UTC())
}

func timedEvent(event Event, startedAt, endedAt time.Time) Event {
	event.StartedAt = startedAt
	event.EndedAt = endedAt
	event.DurationMs = endedAt.Sub(startedAt).Milliseconds()
	return event
}
