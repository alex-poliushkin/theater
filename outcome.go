package theater

import reportmodel "github.com/alex-poliushkin/theater/report"

// Public status, failure-kind, and phase values used by runtime outcomes.
const (
	StatusPending  Status = reportmodel.StatusPending
	StatusRunning  Status = reportmodel.StatusRunning
	StatusPassed   Status = reportmodel.StatusPassed
	StatusFailed   Status = reportmodel.StatusFailed
	StatusCanceled Status = reportmodel.StatusCanceled
	StatusSkipped  Status = reportmodel.StatusSkipped

	FailureKindDefinition  FailureKind = reportmodel.FailureKindDefinition
	FailureKindSetup       FailureKind = reportmodel.FailureKindSetup
	FailureKindObservation FailureKind = reportmodel.FailureKindObservation
	FailureKindAction      FailureKind = reportmodel.FailureKindAction
	FailureKindExpectation FailureKind = reportmodel.FailureKindExpectation
	FailureKindTimeout     FailureKind = reportmodel.FailureKindTimeout
	FailureKindInternal    FailureKind = reportmodel.FailureKindInternal

	PhaseCompile  Phase = reportmodel.PhaseCompile
	PhaseValidate Phase = reportmodel.PhaseValidate
	PhaseRun      Phase = reportmodel.PhaseRun
)

// Status describes the lifecycle state of a runtime node or stage.
type Status = reportmodel.Status

// FailureKind classifies the logical source of a failure.
type FailureKind = reportmodel.FailureKind

// Phase identifies the compile, validate, or run phase.
type Phase = reportmodel.Phase

// Failure captures a user-facing failure summary and its optional underlying
// cause.
type Failure = reportmodel.Failure

// ValidateTerminalOutcome checks that a terminal status/failure pair is
// internally consistent.
func ValidateTerminalOutcome(status Status, failure *Failure) error {
	return reportmodel.ValidateTerminalOutcome(status, failure)
}
