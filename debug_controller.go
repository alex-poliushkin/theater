package theater

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

const (
	debugModeOff         debugMode = "off"
	debugModeDump        debugMode = "dump"
	debugModeInteractive debugMode = "interactive"

	debugPauseReasonStart           debugPauseReason = "start"
	debugPauseReasonBreakpoint      debugPauseReason = "breakpoint"
	debugPauseReasonAttemptFailure  debugPauseReason = "attempt-failure"
	debugPauseReasonRetryOnly       debugPauseReason = "retry-only"
	debugPauseReasonStep            debugPauseReason = "step"
	debugPauseReasonTerminalFailure debugPauseReason = "terminal-failure"

	debugResumeContinue debugResumeCommand = "continue"
	debugResumeQuit     debugResumeCommand = "quit"
	debugResumeStep     debugResumeCommand = "step"
)

type debugMode string

type debugPause struct {
	Seq             uint64
	Reason          debugPauseReason
	Breakpoint      string
	DurableEventSeq uint64
	State           debugBoundaryState
}

type debugPauseHandler func(context.Context, debugPause) (debugResumeCommand, error)

type debugPauseReason string

type debugResumeCommand string

type debugControlDecision struct {
	Emit       bool
	PauseSeq   uint64
	Reason     debugPauseReason
	Breakpoint string
	Resume     debugResumeCommand
}

type debugController struct {
	mode        debugMode
	startPaused bool
	pause       debugPauseHandler

	mu           sync.Mutex
	pendingStart bool
	stepArmed    bool
	focusedLane  string
	nextPauseSeq uint64
}

func (c *debugController) usesInteractiveSerialScheduling() bool {
	return c != nil && c.mode == debugModeInteractive
}

func (c *debugController) Reset() {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.pendingStart = c.startPaused
	c.stepArmed = false
	c.focusedLane = ""
	c.nextPauseSeq = 0
}

func (c *debugController) DecideBoundary(
	ctx context.Context,
	state debugBoundaryState,
	matches []debugCompiledBreakpoint,
	durableEventSeq uint64,
) (debugControlDecision, error) {
	if c == nil {
		return debugControlDecision{}, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.mode {
	case debugModeOff:
		return debugControlDecision{}, nil
	case debugModeDump:
		return c.dumpDecision(matches, debugPauseReasonFromBreakpoints(matches)), nil
	case debugModeInteractive:
		if c.pendingStart {
			c.pendingStart = false
			return c.pauseDecision(ctx, state, debugPauseReasonStart, "", durableEventSeq)
		}
		if c.stepArmed {
			if state.Ref.ScenarioPath != c.focusedLane {
				return debugControlDecision{}, nil
			}

			c.stepArmed = false
			return c.pauseDecision(ctx, state, debugPauseReasonStep, "", durableEventSeq)
		}

		pauseMatches := debugPauseReactionMatches(matches)
		if len(pauseMatches) != 0 {
			return c.pauseDecision(
				ctx,
				state,
				debugPauseReasonFromBreakpoints(pauseMatches),
				debugBreakpointLabel(pauseMatches),
				durableEventSeq,
			)
		}

		return c.dumpDecision(matches, debugPauseReasonFromBreakpoints(matches)), nil
	default:
		return debugControlDecision{}, fmt.Errorf("debug mode %q is invalid", c.mode)
	}
}

func (c *debugController) ShouldEmitBoundary(state debugBoundaryState, matches []debugCompiledBreakpoint) bool {
	if c == nil {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.mode {
	case debugModeOff:
		return false
	case debugModeDump:
		return len(matches) != 0
	case debugModeInteractive:
		if c.pendingStart {
			return true
		}
		if c.stepArmed {
			return state.Ref.ScenarioPath == c.focusedLane
		}
		if len(debugPauseReactionMatches(matches)) != 0 {
			return true
		}

		return len(matches) != 0
	default:
		return false
	}
}

func (c *debugController) DecideTerminalFailure(
	ctx context.Context,
	state debugBoundaryState,
	matches []debugCompiledBreakpoint,
	durableEventSeq uint64,
) (debugControlDecision, error) {
	if c == nil {
		return debugControlDecision{}, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.mode {
	case debugModeOff:
		return debugControlDecision{}, nil
	case debugModeDump:
		return c.dumpDecision(matches, debugPauseReasonTerminalFailure), nil
	case debugModeInteractive:
		pauseMatches := debugPauseReactionMatches(matches)
		if len(pauseMatches) != 0 {
			return c.pauseDecision(ctx, state, debugPauseReasonTerminalFailure, debugBreakpointLabel(pauseMatches), durableEventSeq)
		}

		return c.dumpDecision(matches, debugPauseReasonTerminalFailure), nil
	default:
		return debugControlDecision{}, fmt.Errorf("debug mode %q is invalid", c.mode)
	}
}

func (c *debugController) ShouldEmitTerminalFailure(matches []debugCompiledBreakpoint) bool {
	if c == nil {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.mode {
	case debugModeOff:
		return false
	case debugModeDump:
		return len(matches) != 0
	case debugModeInteractive:
		return len(matches) != 0
	default:
		return false
	}
}

func (c *debugController) dumpDecision(matches []debugCompiledBreakpoint, reason debugPauseReason) debugControlDecision {
	if len(matches) == 0 {
		return debugControlDecision{}
	}

	return debugControlDecision{
		Emit:       true,
		Reason:     reason,
		Breakpoint: debugBreakpointLabel(matches),
	}
}

func (c *debugController) pauseDecision(
	ctx context.Context,
	state debugBoundaryState,
	reason debugPauseReason,
	breakpoint string,
	durableEventSeq uint64,
) (debugControlDecision, error) {
	if c.pause == nil {
		return debugControlDecision{}, errors.New("debug pause handler is required")
	}

	c.nextPauseSeq++
	command, err := c.pause(ctx, debugPause{
		Seq:             c.nextPauseSeq,
		Reason:          reason,
		Breakpoint:      breakpoint,
		DurableEventSeq: durableEventSeq,
		State:           state,
	})
	if err != nil {
		return debugControlDecision{}, err
	}

	switch command {
	case "", debugResumeContinue:
		command = debugResumeContinue
		c.focusedLane = ""
		c.stepArmed = false
	case debugResumeQuit:
		c.focusedLane = ""
		c.stepArmed = false
	case debugResumeStep:
		c.focusedLane = state.Ref.ScenarioPath
		c.stepArmed = true
	default:
		return debugControlDecision{}, fmt.Errorf("debug resume command %q is invalid", command)
	}

	return debugControlDecision{
		Emit:       true,
		PauseSeq:   c.nextPauseSeq,
		Reason:     reason,
		Breakpoint: breakpoint,
		Resume:     command,
	}, nil
}

func debugPauseReactionMatches(matches []debugCompiledBreakpoint) []debugCompiledBreakpoint {
	if len(matches) == 0 {
		return nil
	}

	filtered := make([]debugCompiledBreakpoint, 0, len(matches))
	for i := range matches {
		if matches[i].Reaction == debugBreakpointActionPause {
			filtered = append(filtered, matches[i])
		}
	}

	return filtered
}

func debugPauseReasonFromBreakpoints(matches []debugCompiledBreakpoint) debugPauseReason {
	for i := range matches {
		switch matches[i].When {
		case debugBreakpointWhenAttemptFailure:
			return debugPauseReasonAttemptFailure
		case debugBreakpointWhenRetryOnly:
			return debugPauseReasonRetryOnly
		}
	}

	return debugPauseReasonBreakpoint
}
