package theater

import (
	"context"
	"errors"
	"time"
)

type terminalExecutionError interface {
	TheaterTerminal() bool
}

func recordEventuallyAttempt(
	outcome actOutcome,
	attemptReport AttemptReport,
	timeline []AttemptReport,
	lastObservedFailure *Failure,
	lastObservedBoundary *debugBoundaryState,
) (retryable bool, updatedTimeline []AttemptReport, observedFailure *Failure, observedBoundary *debugBoundaryState) {
	retryable = eventuallyRetryable(outcome)
	attemptReport.Retryable = retryable
	updatedTimeline = timeline
	updatedTimeline = append(updatedTimeline, attemptReport)
	observedFailure = lastObservedFailure
	observedBoundary = cloneDebugBoundaryState(lastObservedBoundary)
	if outcome.failure != nil {
		observedFailure = outcome.failure
	}
	if outcome.debugTerminalBoundary != nil {
		observedBoundary = cloneDebugBoundaryState(outcome.debugTerminalBoundary)
	}

	return retryable, updatedTimeline, observedFailure, observedBoundary
}

func finalizeAttemptReport(report AttemptReport, outcome actOutcome) AttemptReport {
	report.EndedAt = time.Now().UTC()
	report.DurationMs = report.EndedAt.Sub(report.StartedAt).Milliseconds()
	report.Status = outcome.status
	report.Failure = outcome.failure
	if outcome.failure != nil {
		report.FailureSummary = outcome.failure.Message()
	}

	return report
}

func eventuallyRetryable(outcome actOutcome) bool {
	if outcome.status != StatusFailed || outcome.failure == nil {
		return false
	}

	return !isTerminalExecutionError(outcome.failure.Cause)
}

func buildEventuallyReport(
	config *eventuallyPlan,
	startedAt time.Time,
	endedAt time.Time,
	finalOutcome Status,
	terminationReason TerminationReason,
	finalFailure *Failure,
	lastObservedFailure *Failure,
	timeline []AttemptReport,
	successAttempt int,
) *EventuallyReport {
	return &EventuallyReport{
		Enabled:             true,
		Timeout:             config.TimeoutText,
		Interval:            config.IntervalText,
		AttemptsTotal:       len(timeline),
		ElapsedMs:           endedAt.Sub(startedAt).Milliseconds(),
		FinalOutcome:        finalOutcome,
		TerminationReason:   terminationReason,
		SuccessAttempt:      successAttempt,
		FinalFailureReason:  finalFailure,
		LastObservedFailure: lastObservedFailure,
		AttemptTimeline:     cloneAttemptTimeline(timeline),
	}
}

func cloneAttemptTimeline(timeline []AttemptReport) []AttemptReport {
	if len(timeline) == 0 {
		return nil
	}

	cloned := make([]AttemptReport, len(timeline))
	copy(cloned, timeline)
	return cloned
}

func waitForEventuallyInterval(ctx context.Context, interval time.Duration) bool {
	timer := time.NewTimer(interval)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func isTerminalExecutionError(err error) bool {
	if err == nil {
		return false
	}

	var marker terminalExecutionError
	return errors.As(err, &marker) && marker.TheaterTerminal()
}

func cloneDebugBoundaryState(state *debugBoundaryState) *debugBoundaryState {
	if state == nil {
		return nil
	}

	cloned := *state
	return &cloned
}
