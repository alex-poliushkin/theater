package theater

import (
	"context"
	"errors"
	"sync/atomic"
)

const (
	debugBoundaryPhaseBefore debugBoundaryPhase = "before"
	debugBoundaryPhaseAfter  debugBoundaryPhase = "after"

	debugBoundaryKindScenarioCall debugBoundaryKind = "scenario_call"
	debugBoundaryKindAct          debugBoundaryKind = "act"
	debugBoundaryKindAction       debugBoundaryKind = "action"
	debugBoundaryKindExpectation  debugBoundaryKind = "expectation"
)

type debugBoundaryPhase string

type debugBoundaryKind string

type debugBoundaryRef struct {
	StageID        string
	StagePath      string
	ScenarioID     string
	ScenarioCallID string
	ScenarioPath   string
	ActID          string
	NodeRef        string
	Path           string
	Kind           debugBoundaryKind
	Phase          debugBoundaryPhase
	Attempt        int
	SourceSpan     *SourceRef `json:"source,omitempty"`
}

type debugBoundaryState struct {
	Ref       debugBoundaryRef
	Status    Status
	Failure   *Failure
	Resources ResourceScope
	Scope     debugSnapshotSection
	Inputs    debugSnapshotSection
	Output    debugSnapshotSection
	State     debugStateSnapshot
	Recent    debugRecentSnapshot
	Scheduler debugSchedulerSummary
}

type debugBoundaryHook func(context.Context, debugBoundaryState) error

type debugRuntime struct {
	boundaryHook        debugBoundaryHook
	controller          *debugController
	breakpointSpecs     []string
	compiledBreakpoints []debugCompiledBreakpoint
	promptSession       *debugPromptSession
	stateRecorder       *debugStateRecorder
	liveBridge          *debugLiveBridge
	scheduler           *debugSchedulerState
	artifactPath        string
	artifactSink        *debugArtifactSink
	durableEventSeq     atomic.Uint64
}

func (d *debugRuntime) usesInteractiveSerialScheduling() bool {
	return d != nil && d.controller != nil && d.controller.usesInteractiveSerialScheduling()
}

func (d *debugRuntime) shouldEmitBoundaryKind(kind debugBoundaryKind) bool {
	if d == nil {
		return false
	}

	switch kind {
	case debugBoundaryKindScenarioCall, debugBoundaryKindAct:
		return d.hasCompiledBreakpointKind(kind)
	case debugBoundaryKindAction, debugBoundaryKindExpectation:
		return d.boundaryHook != nil || d.artifactSink != nil || d.controller != nil || len(d.compiledBreakpoints) > 0
	default:
		return false
	}
}

func (d *debugRuntime) atBoundary(ctx context.Context, state debugBoundaryState) error {
	if d == nil || (d.boundaryHook == nil && d.artifactSink == nil && d.controller == nil) {
		return nil
	}

	matches := d.matchingBreakpoints(state)
	if d.controller != nil {
		if !d.controller.ShouldEmitBoundary(state, matches) {
			return nil
		}

		var err error
		state, err = d.enrichBoundaryState(ctx, state)
		if err != nil {
			return err
		}

		decision, err := d.controller.DecideBoundary(ctx, state, matches, d.lastDurableEventSeq())
		if err != nil {
			return newContainedDebugBoundaryError(state, "debug controller failed", err)
		}

		return d.dispatchControlledBoundary(ctx, state, decision)
	}
	if len(d.compiledBreakpoints) > 0 && len(matches) == 0 {
		return nil
	}

	var err error
	state, err = d.enrichBoundaryState(ctx, state)
	if err != nil {
		return err
	}

	return d.dispatchBoundary(ctx, state, debugArtifactReasonFromBreakpoints(matches), debugBreakpointLabel(matches))
}

func (d *debugRuntime) atTerminalFailure(ctx context.Context, state debugBoundaryState) error {
	if d == nil || (d.boundaryHook == nil && d.artifactSink == nil && d.controller == nil) {
		return nil
	}

	matches := d.matchingTerminalBreakpoints(state)
	if d.controller != nil {
		if !d.controller.ShouldEmitTerminalFailure(matches) {
			return nil
		}

		var err error
		state, err = d.enrichBoundaryState(ctx, state)
		if err != nil {
			return err
		}

		decision, err := d.controller.DecideTerminalFailure(ctx, state, matches, d.lastDurableEventSeq())
		if err != nil {
			return newContainedDebugBoundaryError(state, "debug controller failed", err)
		}

		return d.dispatchControlledBoundary(ctx, state, decision)
	}
	if len(matches) == 0 {
		return nil
	}

	var err error
	state, err = d.enrichBoundaryState(ctx, state)
	if err != nil {
		return err
	}

	return d.dispatchBoundary(ctx, state, "terminal-failure", debugBreakpointLabel(matches))
}

func (d *debugRuntime) atCheckpoint(
	ctx context.Context,
	state debugBoundaryState,
	label string,
) error {
	if d == nil || (d.boundaryHook == nil && d.artifactSink == nil) {
		return nil
	}

	var err error
	state, err = d.enrichBoundaryState(ctx, state)
	if err != nil {
		return err
	}
	return d.dispatchBoundary(ctx, state, "checkpoint", label)
}

func (d *debugRuntime) dispatchControlledBoundary(
	ctx context.Context,
	state debugBoundaryState,
	decision debugControlDecision,
) error {
	if !decision.Emit {
		return nil
	}

	if err := d.dispatchBoundary(ctx, state, string(decision.Reason), decision.Breakpoint); err != nil {
		return err
	}
	if d.artifactSink == nil || decision.PauseSeq == 0 || decision.Resume == "" {
		if decision.Resume == debugResumeQuit {
			return newContainedDebugBoundaryError(state, "debug session canceled run", context.Canceled)
		}

		return nil
	}
	if _, err := d.artifactSink.WriteResume(ctx, decision.PauseSeq, string(decision.Resume)); err != nil {
		return newContainedDebugBoundaryError(state, "debug artifact sink failed", err)
	}
	if decision.Resume == debugResumeQuit {
		return newContainedDebugBoundaryError(state, "debug session canceled run", context.Canceled)
	}

	return nil
}

func (d *debugRuntime) dispatchBoundary(
	ctx context.Context,
	state debugBoundaryState,
	reason string,
	breakpoint string,
) error {
	if d.artifactSink != nil {
		if _, err := d.artifactSink.WritePause(ctx, reason, breakpoint, state); err != nil {
			return newContainedDebugBoundaryError(state, "debug artifact sink failed", err)
		}
	}
	if d.boundaryHook == nil {
		return nil
	}

	if err := invokeBoundaryError("debug boundary hook", state.Ref.Path, func() error {
		return d.boundaryHook(ctx, state)
	}); err != nil {
		var panicErr boundaryPanicError
		if errors.As(err, &panicErr) {
			return newContainedDebugBoundaryError(state, "debug boundary hook panicked", err)
		}

		return newContainedDebugBoundaryError(state, "debug boundary hook failed", err)
	}

	return nil
}

func (d *debugRuntime) prepareBreakpoints(stage *stagePlan) error {
	if d == nil {
		return nil
	}

	d.compiledBreakpoints = nil
	if len(d.breakpointSpecs) == 0 {
		return nil
	}

	compiled, err := compileDebugBreakpoints(stage, d.breakpointSpecs)
	if err != nil {
		return err
	}

	d.compiledBreakpoints = compiled
	return nil
}

func (d *debugRuntime) matchingBreakpoints(state debugBoundaryState) []debugCompiledBreakpoint {
	if d == nil || len(d.compiledBreakpoints) == 0 {
		return nil
	}

	matches := make([]debugCompiledBreakpoint, 0, len(d.compiledBreakpoints))
	for i := range d.compiledBreakpoints {
		if debugBreakpointMatchesState(d.compiledBreakpoints[i], state) {
			matches = append(matches, d.compiledBreakpoints[i])
		}
	}

	return matches
}

func (d *debugRuntime) matchingTerminalBreakpoints(state debugBoundaryState) []debugCompiledBreakpoint {
	if d == nil || len(d.compiledBreakpoints) == 0 {
		return nil
	}

	matches := make([]debugCompiledBreakpoint, 0, len(d.compiledBreakpoints))
	for i := range d.compiledBreakpoints {
		if d.compiledBreakpoints[i].When != debugBreakpointWhenTerminalFailure {
			continue
		}
		if debugBreakpointMatchesTerminalState(d.compiledBreakpoints[i], state, true) {
			matches = append(matches, d.compiledBreakpoints[i])
		}
	}

	return matches
}

func (d *debugRuntime) storeDurableEventSeq(seq uint64) {
	if d == nil {
		return
	}

	d.durableEventSeq.Store(seq)
}

func (d *debugRuntime) lastDurableEventSeq() uint64 {
	if d == nil {
		return 0
	}

	return d.durableEventSeq.Load()
}

func (d *debugRuntime) hasCompiledBreakpointKind(kind debugBoundaryKind) bool {
	if d == nil {
		return false
	}

	for i := range d.compiledBreakpoints {
		if d.compiledBreakpoints[i].Boundary.Kind == kind {
			return true
		}
	}

	return false
}

func debugArtifactReasonFromBreakpoints(matches []debugCompiledBreakpoint) string {
	switch debugPauseReasonFromBreakpoints(matches) {
	case debugPauseReasonAttemptFailure:
		return string(debugPauseReasonAttemptFailure)
	case debugPauseReasonRetryOnly:
		return string(debugPauseReasonRetryOnly)
	default:
		return "boundary"
	}
}
