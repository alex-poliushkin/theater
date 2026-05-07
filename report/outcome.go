package report

import (
	"errors"
	"fmt"
)

// Public status, failure-kind, and phase values used by runtime outcomes.
const (
	StatusPending  Status = "pending"
	StatusRunning  Status = "running"
	StatusPassed   Status = "passed"
	StatusFailed   Status = "failed"
	StatusCanceled Status = "canceled"
	StatusSkipped  Status = "skipped"

	FailureKindDefinition  FailureKind = "definition"
	FailureKindSetup       FailureKind = "setup"
	FailureKindObservation FailureKind = "observation"
	FailureKindAction      FailureKind = "action"
	FailureKindExpectation FailureKind = "expectation"
	FailureKindTimeout     FailureKind = "timeout"
	FailureKindInternal    FailureKind = "internal"

	PhaseCompile  Phase = "compile"
	PhaseValidate Phase = "validate"
	PhaseRun      Phase = "run"
)

// Status describes the lifecycle state of a runtime node or stage.
type Status string

// FailureKind classifies the logical source of a failure.
type FailureKind string

// Phase identifies the compile, validate, or run phase.
type Phase string

// Failure captures a user-facing failure summary and its optional underlying
// cause.
type Failure struct {
	Kind    FailureKind `json:"kind"`
	Phase   Phase       `json:"phase"`
	At      string      `json:"at"`
	Summary string      `json:"summary"`
	Cause   error       `json:"cause,omitempty"`
}

func (s Status) IsTerminal() bool {
	switch s {
	case StatusPassed, StatusFailed, StatusCanceled, StatusSkipped:
		return true
	default:
		return false
	}
}

func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusRunning, StatusPassed, StatusFailed, StatusCanceled, StatusSkipped:
		return true
	default:
		return false
	}
}

func (k FailureKind) Valid() bool {
	switch k {
	case FailureKindDefinition,
		FailureKindSetup,
		FailureKindObservation,
		FailureKindAction,
		FailureKindExpectation,
		FailureKindTimeout,
		FailureKindInternal:
		return true
	default:
		return false
	}
}

func (p Phase) Valid() bool {
	switch p {
	case PhaseCompile, PhaseValidate, PhaseRun:
		return true
	default:
		return false
	}
}

func (f Failure) Message() string {
	if f.Cause == nil {
		return f.Summary
	}

	return fmt.Sprintf("%s: %v", f.Summary, f.Cause)
}

func (f Failure) Validate() error {
	if !f.Kind.Valid() {
		return fmt.Errorf("failure kind %q is invalid", f.Kind)
	}

	if !f.Phase.Valid() {
		return fmt.Errorf("failure phase %q is invalid", f.Phase)
	}

	if f.At == "" {
		return errors.New("failure at is required")
	}

	if f.Summary == "" {
		return errors.New("failure summary is required")
	}

	return nil
}

// ValidateTerminalOutcome checks that a terminal status/failure pair is
// internally consistent.
func ValidateTerminalOutcome(status Status, failure *Failure) error {
	if !status.Valid() {
		return fmt.Errorf("status %q is invalid", status)
	}

	if !status.IsTerminal() {
		return fmt.Errorf("status %q is not terminal", status)
	}

	switch status {
	case StatusFailed:
		if failure == nil {
			return errors.New("failed outcome requires failure")
		}
	case StatusCanceled:
		if failure != nil {
			return errors.New("canceled outcome must not carry failure")
		}
	default:
		if failure != nil {
			return fmt.Errorf("%s outcome must not carry failure", status)
		}
	}

	if failure == nil {
		return nil
	}

	return failure.Validate()
}
