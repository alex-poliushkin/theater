package theater

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alex-poliushkin/theater/observe"
	statemodel "github.com/alex-poliushkin/theater/state"
)

type actExecution struct {
	act           *actPlan
	actPath       string
	actionPath    string
	http          *HTTPSpec
	catalog       runtimeCatalog
	matchers      MatcherResolver
	identity      executionIdentity
	recorder      executionRecorder
	resources     ResourceScope
	live          observe.Sink
	scenarioScope *valueScope
	generation    *generationRuntime
	state         *statemodel.Manager
	logLimiter    *scenarioLogLimiter
	debug         *debugRuntime
	snapshot      debugSnapshotBuilder
}

type preparedActionExecution struct {
	runner            Action
	contract          ActionContract
	protectedArgs     Args
	inputObservations *ActionObservations
	scopeSection      debugSnapshotSection
	inputSection      debugSnapshotSection
}

type preparedExpectationExecution struct {
	actual       any
	matcher      Matcher
	scopeSection debugSnapshotSection
	inputSection debugSnapshotSection
}

func newActExecution(r *actRunner) actExecution {
	return actExecution{
		act:           r.act,
		actPath:       r.path(),
		actionPath:    r.recorder.action(r.act.ID).path,
		http:          r.http,
		catalog:       r.catalog,
		matchers:      r.matchers,
		identity:      r.identity,
		recorder:      r.recorder,
		resources:     r.resources,
		live:          r.live,
		scenarioScope: r.scenarioScope,
		generation:    r.generation,
		state:         r.state,
		logLimiter:    r.logLimiter,
		debug:         r.debug,
		snapshot:      newDebugSnapshotBuilder(),
	}
}

func (e actExecution) Run(ctx context.Context, attempt int) (actOutcome, error) {
	actScope := newValueScope(e.scenarioScope)
	failure := e.resolveProperties(ctx, actScope, attempt)
	if failure != nil {
		return actOutcome{status: StatusFailed, failure: failure}, nil
	}

	outcome, err := e.runAction(ctx, actScope, attempt)
	if err != nil || outcome.status != StatusPassed {
		return outcome, err
	}

	logs, terminal, err := e.runLogs(ctx, actScope, outcome.values, attempt)
	if err != nil {
		return actOutcome{}, err
	}
	if terminal != nil {
		return *terminal, nil
	}

	outcome, err = e.runExpectations(ctx, actScope, outcome.values, outcome.diagnostics, attempt)
	outcome.logs = logs
	if err == nil && outcome.status == StatusPassed {
		outcome.properties = currentActPropertyValues(e.act, actScope)
	}
	return outcome, err
}

func (e actExecution) actionResult(
	status Status,
	failure *Failure,
	observations *ActionObservations,
	diagnostics []NodeDiagnostic,
	startedAt time.Time,
	endedAt time.Time,
) executionNodeResult {
	return executionNodeResult{
		status:       status,
		failure:      failure,
		startedAt:    startedAt,
		endedAt:      endedAt,
		sourceSpan:   cloneSourceRef(e.act.Action.SourceSpan),
		observations: observations,
		diagnostics:  diagnostics,
	}
}

func (e actExecution) expectationResult(
	expectation expectationPlan,
	status Status,
	failure *Failure,
	diagnostics []NodeDiagnostic,
	startedAt time.Time,
	endedAt time.Time,
) executionNodeResult {
	return executionNodeResult{
		status:      status,
		failure:     failure,
		startedAt:   startedAt,
		endedAt:     endedAt,
		sourceSpan:  cloneSourceRef(expectation.SourceSpan),
		diagnostics: diagnostics,
	}
}

func (e actExecution) resolveProperties(ctx context.Context, actScope *valueScope, attempt int) *Failure {
	for i := range e.act.Properties {
		property := e.act.Properties[i]
		path := property.Path

		value, outputContract, err := e.resolvePropertyValue(ctx, actScope, property, attempt)
		if err != nil {
			var panicErr boundaryPanicError
			if errors.As(err, &panicErr) {
				return internalFailure(path, "inventory panicked", err)
			}

			return setupFailure(path, err)
		}

		if err := validateResolvedContract(path, outputContract, value); err != nil {
			return internalFailure(path, "property value failed contract validation", err)
		}

		value = protectValue(value, outputContract)

		for j := range property.Decorators {
			decorator := property.Decorators[j]
			if decorator.Transform == nil {
				return internalFailure(path, "decorator is not compiled", fmt.Errorf("decorator %q is not prepared", decorator.Use))
			}

			value, err = invokeBoundary("decorator", decorator.Use, func() (any, error) {
				return decorator.Transform(value)
			})
			if err != nil {
				var panicErr boundaryPanicError
				if errors.As(err, &panicErr) {
					return internalFailure(path, "decorator panicked", err)
				}

				return setupFailure(path, err)
			}

			if err := validateResolvedContract(path, decorator.Contract.Produces, value); err != nil {
				return internalFailure(path, "decorator output failed contract validation", err)
			}

			value = protectValue(value, decorator.Contract.Produces)
		}

		actScope.writeAll(Values{property.ID: value})
	}

	return nil
}

func (e actExecution) resolvePropertyValue(
	ctx context.Context,
	actScope *valueScope,
	property propertyPlan,
	attempt int,
) (value any, outputContract ValueContract, err error) {
	resolver := newReferenceResolver(actScope).
		withDecorators(e.catalog).
		withGeneration(e.catalog, e.generation, e.identity).
		withMatchers(e.matchers)

	if property.Value != nil {
		value, err := resolver.ResolveBindingContext(ctx, *property.Value)
		return value, inferredBindingContract(e.catalog, property.Value), err
	}

	args, err := resolver.ResolveBindingsContext(ctx, property.Inventory.With)
	if err != nil {
		return nil, ValueContract{}, err
	}

	inventory, err := e.catalog.ResolveInventory(property.Inventory.Use)
	if err != nil {
		return nil, ValueContract{}, err
	}

	contract := inventory.Contract()
	protectedArgs := protectInventoryArgs(Args(args), contract.Args)
	request := InventoryRequest{
		Args:  protectedArgs,
		HTTP:  e.http,
		State: e.state,
		Paths: PathContext{
			StagePath:    e.identity.stagePath,
			ScenarioPath: e.identity.scenarioPath,
			ActPath:      e.actPath,
			PropertyPath: property.Path,
		},
		Attempt:   attempt,
		Resources: e.resources,
	}

	value, err = invokeBoundary("inventory", property.Inventory.Use, func() (any, error) {
		return inventory.Acquire(ctx, request)
	})
	return value, contract.Produces, err
}

func (e actExecution) prepareActionExecution(
	ctx context.Context,
	actScope *valueScope,
	actionNode executionNode,
	attempt int,
	startedAt time.Time,
) (preparedActionExecution, *actOutcome, error) {
	scopeSection := e.snapshot.scopeSection(actScope)

	runner, err := e.catalog.ResolveAction(e.act.Action.Use)
	if err != nil {
		failure := setupFailure(e.actionPath, err)
		boundaryState, err := e.recordActionFinished(
			ctx,
			actionNode,
			attempt,
			StatusFailed,
			failure,
			nil,
			nil,
			startedAt,
			scopeSection,
			debugSnapshotSection{},
			debugSnapshotSection{},
		)
		if err != nil {
			return preparedActionExecution{}, nil, err
		}

		outcome := actOutcome{status: StatusFailed, failure: failure, debugTerminalBoundary: debugTerminalBoundary(boundaryState, failure)}
		return preparedActionExecution{}, &outcome, nil
	}

	args, err := newReferenceResolver(actScope).
		withDecorators(e.catalog).
		withGeneration(e.catalog, e.generation, e.identity).
		withMatchers(e.matchers).
		ResolveBindingsContext(ctx, e.act.Action.With)
	if err != nil {
		failure := setupFailure(e.actionPath, err)
		boundaryState, err := e.recordActionFinished(
			ctx,
			actionNode,
			attempt,
			StatusFailed,
			failure,
			nil,
			nil,
			startedAt,
			scopeSection,
			debugSnapshotSection{},
			debugSnapshotSection{},
		)
		if err != nil {
			return preparedActionExecution{}, nil, err
		}

		outcome := actOutcome{status: StatusFailed, failure: failure, debugTerminalBoundary: debugTerminalBoundary(boundaryState, failure)}
		return preparedActionExecution{}, &outcome, nil
	}

	contract := runner.Contract()
	protectedArgs := protectActionArgs(Args(args), contract.Inputs)
	inputSection := e.snapshot.actionInputsSection(protectedArgs, contract, e.act.Action.With)
	if err := validateResolvedArgs(contract, Args(args)); err != nil {
		failure := setupFailure(e.actionPath, err)
		observations := buildActionObservations(protectedArgs, nil, contract)
		boundaryState, err := e.recordActionFinished(
			ctx,
			actionNode,
			attempt,
			StatusFailed,
			failure,
			observations,
			nil,
			startedAt,
			scopeSection,
			inputSection,
			debugSnapshotSection{},
		)
		if err != nil {
			return preparedActionExecution{}, nil, err
		}

		outcome := actOutcome{status: StatusFailed, failure: failure, debugTerminalBoundary: debugTerminalBoundary(boundaryState, failure)}
		return preparedActionExecution{}, &outcome, nil
	}

	inputObservations := buildActionObservations(protectedArgs, nil, contract)
	if err := e.atBoundary(
		ctx,
		actionNode,
		debugBoundaryKindAction,
		debugBoundaryPhaseBefore,
		attempt,
		StatusRunning,
		nil,
		scopeSection,
		inputSection,
		debugSnapshotSection{},
	); err != nil {
		return preparedActionExecution{}, nil, err
	}
	if err := actionNode.running(attempt); err != nil {
		return preparedActionExecution{}, nil, err
	}

	return preparedActionExecution{
		runner:            runner,
		contract:          contract,
		protectedArgs:     protectedArgs,
		inputObservations: inputObservations,
		scopeSection:      scopeSection,
		inputSection:      inputSection,
	}, nil, nil
}

func (e actExecution) prepareExpectationExecution(
	ctx context.Context,
	expectation expectationPlan,
	actScope *valueScope,
	actionOutputs Values,
	actionDiagnostics []NodeDiagnostic,
	expectationNode executionNode,
	attempt int,
	startedAt time.Time,
) (preparedExpectationExecution, *actOutcome, error) {
	scopeSection := e.snapshot.scopeSection(actScope)
	path := expectationNode.path
	if expectation.Matcher.Ref == "" {
		failure := setupFailure(path, errors.New("matcher descriptor is not prepared"))
		return e.finishExpectationPreparationFailure(
			ctx,
			expectation,
			expectationNode,
			attempt,
			failure,
			nil,
			startedAt,
			scopeSection,
		)
	}

	actual, err := e.expectationSubjectResolver(actionOutputs, actScope).
		ResolveSubjectContext(ctx, expectation.Subject)
	if err != nil {
		failure := expectationObservationFailure(path, err)
		return e.finishExpectationPreparationFailure(
			ctx,
			expectation,
			expectationNode,
			attempt,
			failure,
			httpDiagnosticsForExpectationFailure(actionDiagnostics, expectation, failure),
			startedAt,
			scopeSection,
		)
	}

	args, err := e.expectationBindingResolver(actionOutputs, actScope).
		ResolveBindingsContext(ctx, expectation.Assert.Args)
	if err != nil {
		failure := setupFailure(path, err)
		return e.finishExpectationPreparationFailure(
			ctx,
			expectation,
			expectationNode,
			attempt,
			failure,
			nil,
			startedAt,
			scopeSection,
		)
	}

	matcher, err := invokeBoundary("matcher", expectation.Matcher.Ref, func() (Matcher, error) {
		return expectation.Matcher.Compile(
			newMatcherCompileResolver(e.matchers),
			protectMatcherArgs(args, expectation.Matcher.Args),
		)
	})
	if err != nil {
		failure := e.matcherFailure(path, err)
		if failure == nil {
			failure = setupFailure(path, err)
		}

		outcome, err := e.finishExpectationFailure(
			ctx,
			expectation,
			expectationNode,
			attempt,
			failure,
			nil,
			startedAt,
			scopeSection,
			debugSnapshotSection{},
			debugSnapshotSection{},
		)
		if err != nil {
			return preparedExpectationExecution{}, nil, err
		}

		return preparedExpectationExecution{}, &outcome, nil
	}

	inputSection := e.snapshot.expectationInputsSection(
		actual,
		expectation.Matcher.Actual,
		args,
		expectation.Matcher.Args,
		expectation.Assert.Args,
	)
	if err := e.atBoundary(
		ctx,
		expectationNode,
		debugBoundaryKindExpectation,
		debugBoundaryPhaseBefore,
		attempt,
		StatusRunning,
		nil,
		scopeSection,
		inputSection,
		debugSnapshotSection{},
	); err != nil {
		return preparedExpectationExecution{}, nil, err
	}

	return preparedExpectationExecution{
		actual:       actual,
		matcher:      matcher,
		scopeSection: scopeSection,
		inputSection: inputSection,
	}, nil, nil
}

func (e actExecution) expectationSubjectResolver(actionOutputs Values, actScope *valueScope) referenceResolver {
	return newSubjectResolver(
		actionOutputs,
		currentActPropertyValues(e.act, actScope),
		layeredValueLookup{
			primary:  mapValueLookup(actionOutputs),
			fallback: actScope,
		},
	).
		withDecorators(e.catalog).
		withGeneration(e.catalog, e.generation, e.identity).
		withMatchers(e.matchers)
}

func (e actExecution) expectationBindingResolver(actionOutputs Values, actScope *valueScope) referenceResolver {
	return newReferenceResolver(layeredValueLookup{
		primary:  mapValueLookup(actionOutputs),
		fallback: actScope,
	}).
		withDecorators(e.catalog).
		withGeneration(e.catalog, e.generation, e.identity).
		withMatchers(e.matchers)
}

func (e actExecution) runAction(ctx context.Context, actScope *valueScope, attempt int) (actOutcome, error) {
	startedAt := time.Now().UTC()
	actionNode := e.recorder.action(e.act.ID)
	prepared, terminal, err := e.prepareActionExecution(ctx, actScope, actionNode, attempt, startedAt)
	if err != nil {
		return actOutcome{}, err
	}
	if terminal != nil {
		return *terminal, nil
	}

	live := newPanicCapturingSink(e.live, e.actionPath)
	publisher, requestReporter := e.newActionReporter(ctx, live, actionNode, attempt, prepared)
	diagnosticCollector := &actionDiagnosticCollector{}
	outputs, err := invokeBoundary("action", e.act.Action.Use, func() (Outputs, error) {
		return prepared.runner.Run(ctx, ActionRequest{
			Args:        prepared.protectedArgs,
			HTTP:        e.http,
			State:       e.state,
			HTTPCapture: compiledHTTPAuthCapture(e.act.Action.Use, e.act.CaptureAuth),
			Reporter:    requestReporter,
			Diagnostics: diagnosticCollector,
			Paths: PathContext{
				StagePath:    e.identity.stagePath,
				ScenarioPath: e.identity.scenarioPath,
				ActPath:      e.actPath,
			},
			Attempt:   attempt,
			Resources: e.resources,
		})
	})
	if checkpointErr := publisher.Failure(); checkpointErr != nil {
		return actOutcome{}, checkpointErr
	}
	actionDiagnostics := actionDiagnosticsForNode(diagnosticCollector.Snapshot(), actionNode, attempt)
	streamObservations := buildStreamObservations(publisher.Snapshot())
	if outcome, handled, err := e.handleActionLiveSinkFailure(
		ctx,
		actionNode,
		attempt,
		startedAt,
		prepared.scopeSection,
		prepared.inputSection,
		prepared.contract,
		prepared.protectedArgs,
		prepared.inputObservations,
		outputs,
		err,
		streamObservations,
		nil,
		live.Failure(),
	); handled {
		return outcome, err
	}
	if err != nil {
		return e.handleActionExecutionError(
			ctx,
			actionNode,
			attempt,
			startedAt,
			prepared.scopeSection,
			prepared.inputSection,
			prepared.contract,
			prepared.inputObservations,
			streamObservations,
			actionDiagnostics,
			err,
		)
	}

	if err := validateResolvedOutputs(prepared.contract, outputs); err != nil {
		protectedOutputs := protectActionOutputs(outputs, prepared.contract.Outputs)
		outputSection := e.snapshot.actionOutputsSection(protectedOutputs, prepared.contract)
		observations := buildActionObservations(prepared.protectedArgs, protectedOutputs, prepared.contract)
		observations = mergeActionObservations(observations, streamObservations)
		failure := internalFailure(e.actionPath, "action outputs failed contract validation", err)
		boundaryState, err := e.recordActionFinished(
			ctx,
			actionNode,
			attempt,
			StatusFailed,
			failure,
			observations,
			actionDiagnostics,
			startedAt,
			prepared.scopeSection,
			prepared.inputSection,
			outputSection,
		)
		if err != nil {
			return actOutcome{}, err
		}

		return actOutcome{status: StatusFailed, failure: failure, debugTerminalBoundary: debugTerminalBoundary(boundaryState, failure)}, nil
	}

	protectedOutputs := protectActionOutputs(outputs, prepared.contract.Outputs)
	outputSection := e.snapshot.actionOutputsSection(protectedOutputs, prepared.contract)
	observations := buildActionObservations(prepared.protectedArgs, protectedOutputs, prepared.contract)
	observations = mergeActionObservations(observations, streamObservations)
	if _, err := e.recordActionFinished(
		ctx,
		actionNode,
		attempt,
		StatusPassed,
		nil,
		observations,
		nil,
		startedAt,
		prepared.scopeSection,
		prepared.inputSection,
		outputSection,
	); err != nil {
		return actOutcome{}, err
	}

	return actOutcome{status: StatusPassed, values: Values(protectedOutputs), diagnostics: actionDiagnostics}, nil
}

func (e actExecution) newActionReporter(
	ctx context.Context,
	live observe.Sink,
	actionNode executionNode,
	attempt int,
	prepared preparedActionExecution,
) (*actionLiveReporter, observe.Reporter) {
	reporter := newActionLiveReporter(live, actionNode.observeNodeRef(attempt), 0)
	if e.debug == nil {
		return reporter, reporter
	}

	checkpointReporter := reporter.checkpointReporter(func(checkpoint DebugCheckpoint) error {
		return e.emitActionDebugCheckpoint(ctx, actionNode, attempt, prepared.scopeSection, prepared.inputSection, checkpoint)
	})
	return reporter, checkpointReporter
}

func (e actExecution) emitActionDebugCheckpoint(
	ctx context.Context,
	actionNode executionNode,
	attempt int,
	scope debugSnapshotSection,
	inputs debugSnapshotSection,
	checkpoint DebugCheckpoint,
) error {
	if e.debug == nil {
		return nil
	}

	values := Values{
		"checkpoint": checkpoint.Name,
	}
	if len(checkpoint.Values) != 0 {
		values["values"] = checkpoint.Values
	}
	origins := map[string]string{
		"checkpoint": "action.checkpoint.name",
		"values":     "action.checkpoint.values",
	}

	return e.debug.atCheckpoint(ctx, debugBoundaryState{
		Ref: debugBoundaryRef{
			StageID:        e.identity.stageID,
			StagePath:      e.identity.stagePath,
			ScenarioID:     e.identity.scenarioID,
			ScenarioCallID: e.identity.scenarioCallID,
			ScenarioPath:   e.identity.scenarioPath,
			ActID:          e.act.ID,
			NodeRef:        "action",
			Path:           actionNode.path,
			Kind:           debugBoundaryKindAction,
			Phase:          debugBoundaryPhaseBefore,
			Attempt:        attempt,
		},
		Status:    StatusRunning,
		Resources: e.resources,
		Scope:     scope,
		Inputs:    inputs,
		Output:    e.snapshot.sectionFromValues(values, nil, origins),
	}, checkpoint.Name)
}

func (e actExecution) runExpectation(
	ctx context.Context,
	expectation expectationPlan,
	actScope *valueScope,
	actionOutputs Values,
	actionDiagnostics []NodeDiagnostic,
	attempt int,
) (actOutcome, error) {
	expectationNode := e.recorder.expectation(e.act.ID, expectation.ID)
	path := expectationNode.path
	startedAt := time.Now().UTC()
	prepared, terminal, err := e.prepareExpectationExecution(
		ctx,
		expectation,
		actScope,
		actionOutputs,
		actionDiagnostics,
		expectationNode,
		attempt,
		startedAt,
	)
	if err != nil {
		return actOutcome{}, err
	}
	if terminal != nil {
		return *terminal, nil
	}

	err = invokeBoundaryError("matcher", expectation.Matcher.Ref, func() error {
		return prepared.matcher.Check(ctx, prepared.actual)
	})
	if err != nil {
		if failure := e.matcherFailure(path, err); failure != nil {
			return e.finishExpectationFailure(
				ctx,
				expectation,
				expectationNode,
				attempt,
				failure,
				httpDiagnosticsForExpectationFailure(actionDiagnostics, expectation, failure),
				startedAt,
				prepared.scopeSection,
				prepared.inputSection,
				debugSnapshotSection{},
			)
		}

		status, failure := classifyExecutionError(path, err, FailureKindExpectation, "expectation failed", "expectation timed out")
		boundaryState, err := e.recordExpectationFinished(
			ctx,
			expectation,
			expectationNode,
			attempt,
			status,
			failure,
			httpDiagnosticsForExpectationFailure(actionDiagnostics, expectation, failure),
			startedAt,
			prepared.scopeSection,
			prepared.inputSection,
			debugSnapshotSection{},
		)
		if err != nil {
			return actOutcome{}, err
		}

		return actOutcome{status: status, failure: failure, debugTerminalBoundary: debugTerminalBoundary(boundaryState, failure)}, nil
	}

	if _, err := e.recordExpectationFinished(
		ctx,
		expectation,
		expectationNode,
		attempt,
		StatusPassed,
		nil,
		nil,
		startedAt,
		prepared.scopeSection,
		prepared.inputSection,
		debugSnapshotSection{},
	); err != nil {
		return actOutcome{}, err
	}

	return actOutcome{status: StatusPassed}, nil
}

func (e actExecution) runExpectations(
	ctx context.Context,
	actScope *valueScope,
	actionOutputs Values,
	actionDiagnostics []NodeDiagnostic,
	attempt int,
) (actOutcome, error) {
	for i := range e.act.Expectations {
		outcome, err := e.runExpectation(ctx, e.act.Expectations[i], actScope, actionOutputs, actionDiagnostics, attempt)
		if err != nil || outcome.status != StatusPassed {
			return outcome, err
		}
	}

	return actOutcome{status: StatusPassed, values: actionOutputs}, nil
}

func (e actExecution) recordActionFinished(
	ctx context.Context,
	actionNode executionNode,
	attempt int,
	status Status,
	failure *Failure,
	observations *ActionObservations,
	diagnostics []NodeDiagnostic,
	startedAt time.Time,
	scope debugSnapshotSection,
	inputs debugSnapshotSection,
	output debugSnapshotSection,
) (debugBoundaryState, error) {
	endedAt := time.Now().UTC()
	boundaryState := e.boundaryState(
		actionNode,
		debugBoundaryKindAction,
		debugBoundaryPhaseAfter,
		attempt,
		status,
		failure,
		scope,
		inputs,
		output,
	)
	if err := e.debug.atBoundary(ctx, boundaryState); err != nil {
		return debugBoundaryState{}, err
	}

	return boundaryState, actionNode.finished(
		attempt,
		e.actionResult(status, failure, observations, diagnostics, startedAt, endedAt),
	)
}

func (e actExecution) recordExpectationFinished(
	ctx context.Context,
	expectation expectationPlan,
	expectationNode executionNode,
	attempt int,
	status Status,
	failure *Failure,
	diagnostics []NodeDiagnostic,
	startedAt time.Time,
	scope debugSnapshotSection,
	inputs debugSnapshotSection,
	output debugSnapshotSection,
) (debugBoundaryState, error) {
	endedAt := time.Now().UTC()
	boundaryState := e.boundaryState(
		expectationNode,
		debugBoundaryKindExpectation,
		debugBoundaryPhaseAfter,
		attempt,
		status,
		failure,
		scope,
		inputs,
		output,
	)
	if err := e.debug.atBoundary(ctx, boundaryState); err != nil {
		return debugBoundaryState{}, err
	}

	return boundaryState, expectationNode.finished(
		attempt,
		e.expectationResult(expectation, status, failure, diagnostics, startedAt, endedAt),
	)
}

func (e actExecution) atBoundary(
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
	return e.debug.atBoundary(ctx, e.boundaryState(
		node,
		kind,
		phase,
		attempt,
		status,
		failure,
		scope,
		inputs,
		output,
	))
}

func (e actExecution) boundaryState(
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
			StageID:        e.identity.stageID,
			StagePath:      e.identity.stagePath,
			ScenarioID:     e.identity.scenarioID,
			ScenarioCallID: e.identity.scenarioCallID,
			ScenarioPath:   e.identity.scenarioPath,
			ActID:          node.address.actID,
			NodeRef:        node.address.nodeRef,
			Path:           node.path,
			Kind:           kind,
			Phase:          phase,
			Attempt:        attempt,
			SourceSpan:     cloneSourceRef(e.boundarySourceSpan(kind, node)),
		},
		Status:    status,
		Failure:   failure,
		Resources: e.resources,
		Scope:     scope,
		Inputs:    inputs,
		Output:    output,
	}
}

func (e actExecution) boundarySourceSpan(kind debugBoundaryKind, node executionNode) *SourceRef {
	switch kind {
	case debugBoundaryKindAction:
		return e.act.Action.SourceSpan
	case debugBoundaryKindExpectation:
		for i := range e.act.Expectations {
			if e.act.Expectations[i].ID == node.address.nodeRef {
				return e.act.Expectations[i].SourceSpan
			}
		}
	}

	return nil
}

func debugTerminalBoundary(state debugBoundaryState, failure *Failure) *debugBoundaryState {
	if failure == nil {
		return nil
	}

	candidate := state
	return &candidate
}

func actionDiagnosticsForNode(diagnostics []NodeDiagnostic, actionNode executionNode, attempt int) []NodeDiagnostic {
	if len(diagnostics) == 0 {
		return nil
	}

	cloned := cloneNodeDiagnostics(diagnostics)
	actionAddress := actionNode.address.finished(attempt, nil)
	for i := range cloned {
		if cloned[i].Kind == NodeDiagnosticKindHTTP && cloned[i].HTTP != nil && cloned[i].HTTP.ActionAddress == nil {
			cloned[i].HTTP.ActionAddress = cloneNodeAddress(actionAddress)
		}
	}

	return cloned
}

func httpDiagnosticsForExpectationFailure(
	diagnostics []NodeDiagnostic,
	expectation expectationPlan,
	failure *Failure,
) []NodeDiagnostic {
	failureKind := httpDiagnosticFailureKindForExpectation(expectation, failure)
	if failureKind == "" {
		return diagnostics
	}

	cloned := cloneNodeDiagnostics(diagnostics)
	for i := range cloned {
		if cloned[i].Kind != NodeDiagnosticKindHTTP || cloned[i].HTTP == nil {
			continue
		}
		if cloned[i].HTTP.FailureKind == "" {
			cloned[i].HTTP.FailureKind = failureKind
		}
	}

	return cloned
}

func httpDiagnosticFailureKindForExpectation(
	expectation expectationPlan,
	failure *Failure,
) HTTPDiagnosticFailureKind {
	if failure == nil || expectation.Subject.From == SubjectFromProperty {
		return ""
	}

	if failure.Kind == FailureKindObservation && expectation.Subject.Field == "body" && expectation.Subject.Decode != "" {
		return HTTPDiagnosticFailureBodyParse
	}

	if failure.Kind != FailureKindExpectation {
		return ""
	}

	switch expectation.Subject.Field {
	case "status", "status_code":
		return HTTPDiagnosticFailureStatus
	case "headers":
		return HTTPDiagnosticFailureHeader
	default:
		return HTTPDiagnosticFailureExpectation
	}
}

func (e actExecution) handleActionLiveSinkFailure(
	ctx context.Context,
	actionNode executionNode,
	attempt int,
	startedAt time.Time,
	scope debugSnapshotSection,
	inputs debugSnapshotSection,
	contract ActionContract,
	protectedArgs Args,
	inputObservations *ActionObservations,
	outputs Outputs,
	actionErr error,
	streamObservations *ActionObservations,
	diagnostics []NodeDiagnostic,
	publishErr error,
) (actOutcome, bool, error) {
	if publishErr == nil {
		return actOutcome{}, false, nil
	}

	observations := inputObservations
	if actionErr != nil {
		observations = mergeActionObservations(observations, partialOutputObservations(actionErr, contract))
	} else if validateResolvedOutputs(contract, outputs) == nil {
		observations = buildActionObservations(protectedArgs, protectActionOutputs(outputs, contract.Outputs), contract)
	}

	observations = mergeActionObservations(observations, streamObservations)
	failure := internalFailure(e.actionPath, "live sink panicked", publishErr)
	outputSection := debugSnapshotSection{}
	if validateResolvedOutputs(contract, outputs) == nil {
		outputSection = e.snapshot.actionOutputsSection(protectActionOutputs(outputs, contract.Outputs), contract)
	}

	outcome, err := e.finishActionFailure(
		ctx,
		actionNode,
		attempt,
		startedAt,
		failure,
		observations,
		diagnostics,
		scope,
		inputs,
		outputSection,
	)
	return outcome, true, err
}

func (e actExecution) handleActionExecutionError(
	ctx context.Context,
	actionNode executionNode,
	attempt int,
	startedAt time.Time,
	scope debugSnapshotSection,
	inputs debugSnapshotSection,
	contract ActionContract,
	inputObservations *ActionObservations,
	streamObservations *ActionObservations,
	diagnostics []NodeDiagnostic,
	actionErr error,
) (actOutcome, error) {
	var panicErr boundaryPanicError
	if errors.As(actionErr, &panicErr) {
		observations := mergeActionObservations(inputObservations, streamObservations)
		failure := internalFailure(e.actionPath, "action panicked", actionErr)
		return e.finishActionFailure(ctx, actionNode, attempt, startedAt, failure, observations, nil, scope, inputs, debugSnapshotSection{})
	}

	outputObservations := partialOutputObservations(actionErr, contract)
	observations := mergeActionObservations(inputObservations, outputObservations)
	observations = mergeActionObservations(observations, streamObservations)
	outputSection := debugSnapshotSection{}
	if details, ok := actionErrorDetails(actionErr); ok {
		outputSection = e.snapshot.actionOutputsSection(protectActionOutputs(details.PartialOutputs(), contract.Outputs), contract)
	}
	failureSummary := actionFailureSummary(actionErr, "action failed")
	timeoutSummary := actionFailureSummary(actionErr, "action timed out")
	status, failure := classifyExecutionError(e.actionPath, actionErr, FailureKindAction, failureSummary, timeoutSummary)
	boundaryState, err := e.recordActionFinished(
		ctx,
		actionNode,
		attempt,
		status,
		failure,
		observations,
		diagnostics,
		startedAt,
		scope,
		inputs,
		outputSection,
	)
	if err != nil {
		return actOutcome{}, err
	}

	return actOutcome{status: status, failure: failure, debugTerminalBoundary: debugTerminalBoundary(boundaryState, failure)}, nil
}

func (e actExecution) finishActionFailure(
	ctx context.Context,
	actionNode executionNode,
	attempt int,
	startedAt time.Time,
	failure *Failure,
	observations *ActionObservations,
	diagnostics []NodeDiagnostic,
	scope debugSnapshotSection,
	inputs debugSnapshotSection,
	output debugSnapshotSection,
) (actOutcome, error) {
	boundaryState, err := e.recordActionFinished(
		ctx,
		actionNode,
		attempt,
		StatusFailed,
		failure,
		observations,
		diagnostics,
		startedAt,
		scope,
		inputs,
		output,
	)
	if err != nil {
		return actOutcome{}, err
	}

	return actOutcome{status: StatusFailed, failure: failure, debugTerminalBoundary: debugTerminalBoundary(boundaryState, failure)}, nil
}

func (e actExecution) finishExpectationPreparationFailure(
	ctx context.Context,
	expectation expectationPlan,
	expectationNode executionNode,
	attempt int,
	failure *Failure,
	diagnostics []NodeDiagnostic,
	startedAt time.Time,
	scope debugSnapshotSection,
) (preparedExpectationExecution, *actOutcome, error) {
	boundaryState, err := e.recordExpectationFinished(
		ctx,
		expectation,
		expectationNode,
		attempt,
		StatusFailed,
		failure,
		diagnostics,
		startedAt,
		scope,
		debugSnapshotSection{},
		debugSnapshotSection{},
	)
	if err != nil {
		return preparedExpectationExecution{}, nil, err
	}

	outcome := actOutcome{status: StatusFailed, failure: failure, debugTerminalBoundary: debugTerminalBoundary(boundaryState, failure)}
	return preparedExpectationExecution{}, &outcome, nil
}

func (e actExecution) finishExpectationFailure(
	ctx context.Context,
	expectation expectationPlan,
	expectationNode executionNode,
	attempt int,
	failure *Failure,
	diagnostics []NodeDiagnostic,
	startedAt time.Time,
	scope debugSnapshotSection,
	inputs debugSnapshotSection,
	output debugSnapshotSection,
) (actOutcome, error) {
	boundaryState, err := e.recordExpectationFinished(
		ctx,
		expectation,
		expectationNode,
		attempt,
		StatusFailed,
		failure,
		diagnostics,
		startedAt,
		scope,
		inputs,
		output,
	)
	if err != nil {
		return actOutcome{}, err
	}

	return actOutcome{status: StatusFailed, failure: failure, debugTerminalBoundary: debugTerminalBoundary(boundaryState, failure)}, nil
}

func (e actExecution) matcherFailure(path string, err error) *Failure {
	var panicErr boundaryPanicError
	if !errors.As(err, &panicErr) {
		return nil
	}

	return internalFailure(path, "matcher panicked", err)
}

func expectationObservationFailure(path string, err error) *Failure {
	return &Failure{
		Kind:    FailureKindObservation,
		Phase:   PhaseRun,
		At:      path,
		Summary: "observation failed",
		Cause:   err,
	}
}

func currentActPropertyValues(act *actPlan, actScope *valueScope) Values {
	if len(act.Properties) == 0 || actScope == nil || len(actScope.values) == 0 {
		return nil
	}

	values := make(Values)
	for i := range act.Properties {
		propertyID := act.Properties[i].ID
		if propertyID == "" {
			continue
		}

		value, ok := actScope.values[propertyID]
		if !ok {
			continue
		}

		values[propertyID] = value
	}

	if len(values) == 0 {
		return nil
	}

	return values
}

func compiledHTTPAuthCapture(actionRef string, capture *httpAuthCapturePlan) *HTTPAuthCaptureSpec {
	if actionRef != httpActionRef || capture == nil {
		return nil
	}

	spec := &HTTPAuthCaptureSpec{
		Auth:  capture.Auth,
		Slots: make(map[string]HTTPCaptureSourceSpec, len(capture.Slots)),
	}
	for name, source := range capture.Slots {
		spec.Slots[name] = source
	}
	if len(spec.Slots) == 0 {
		spec.Slots = nil
	}

	return spec
}
