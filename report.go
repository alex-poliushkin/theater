package theater

import reportmodel "github.com/alex-poliushkin/theater/report"

// Public report schema version and enum values used by run documents.
const (
	RunDocumentSchemaVersion = reportmodel.RunDocumentSchemaVersion

	DefaultScenarioLogPreviewLimitBytes = reportmodel.DefaultScenarioLogPreviewLimitBytes
	DefaultScenarioLogRecordsPerAct     = reportmodel.DefaultScenarioLogRecordsPerAct
	DefaultScenarioLogRecordsPerRun     = reportmodel.DefaultScenarioLogRecordsPerRun

	EventKindStageRunning        = reportmodel.EventKindStageRunning
	EventKindStageFinished       = reportmodel.EventKindStageFinished
	EventKindScenarioRunning     = reportmodel.EventKindScenarioRunning
	EventKindScenarioFinished    = reportmodel.EventKindScenarioFinished
	EventKindActRunning          = reportmodel.EventKindActRunning
	EventKindActFinished         = reportmodel.EventKindActFinished
	EventKindActionRunning       = reportmodel.EventKindActionRunning
	EventKindActionFinished      = reportmodel.EventKindActionFinished
	EventKindExpectationFinished = reportmodel.EventKindExpectationFinished
	EventKindLogEmitted          = reportmodel.EventKindLogEmitted

	NodeKindScenario    NodeKind = reportmodel.NodeKindScenario
	NodeKindAct         NodeKind = reportmodel.NodeKindAct
	NodeKindAction      NodeKind = reportmodel.NodeKindAction
	NodeKindExpectation NodeKind = reportmodel.NodeKindExpectation
	NodeKindLog         NodeKind = reportmodel.NodeKindLog

	NodeDiagnosticKindHTTP NodeDiagnosticKind = reportmodel.NodeDiagnosticKindHTTP

	LogStatusEmitted LogStatus = reportmodel.LogStatusEmitted
	LogStatusOmitted LogStatus = reportmodel.LogStatusOmitted
	LogStatusError   LogStatus = reportmodel.LogStatusError

	SensitivityNone     Sensitivity = reportmodel.SensitivityNone
	SensitivityInternal Sensitivity = reportmodel.SensitivityInternal
	SensitivityPersonal Sensitivity = reportmodel.SensitivityPersonal
	SensitivitySecret   Sensitivity = reportmodel.SensitivitySecret

	CaptureOmit        Capture = reportmodel.CaptureOmit
	CaptureSummary     Capture = reportmodel.CaptureSummary
	CaptureArtifactRef Capture = reportmodel.CaptureArtifactRef

	TerminationReasonConverged        TerminationReason = reportmodel.TerminationReasonConverged
	TerminationReasonDeadlineExceeded TerminationReason = reportmodel.TerminationReasonDeadlineExceeded
	TerminationReasonTerminalFailure  TerminationReason = reportmodel.TerminationReasonTerminalFailure
	TerminationReasonParentCanceled   TerminationReason = reportmodel.TerminationReasonParentCanceled

	SkipReasonExplicit     SkipReason = reportmodel.SkipReasonExplicit
	SkipReasonStageAborted SkipReason = reportmodel.SkipReasonStageAborted
)

// Sensitivity classifies how a value should be treated in diagnostics and
// report payloads.
type Sensitivity = reportmodel.Sensitivity

// Capture defines how much of a value may appear in previews or artifact
// payloads.
type Capture = reportmodel.Capture

// NodeKind identifies the logical node represented in final reports.
type NodeKind = reportmodel.NodeKind

// NodeDiagnosticKind identifies the typed diagnostic attached to a report node.
type NodeDiagnosticKind = reportmodel.NodeDiagnosticKind

// LogStatus identifies how one scenario-authored log record was handled.
type LogStatus = reportmodel.LogStatus

// TerminationReason explains why an eventually block stopped retrying.
type TerminationReason = reportmodel.TerminationReason

// SkipReason explains why a node or scenario finished as skipped.
type SkipReason = reportmodel.SkipReason

// GenerationMetadata records the generator seed and base time captured for one run.
type GenerationMetadata = reportmodel.GenerationMetadata

// Preview is a report-safe preview of a captured value or payload.
type Preview = reportmodel.Preview

// ArtifactRef points to payload content stored outside the report document.
type ArtifactRef = reportmodel.ArtifactRef

// Contrast describes expected-versus-actual mismatch details.
type Contrast = reportmodel.Contrast

// NodeAddress identifies a logical runtime node within a scenario call.
type NodeAddress = reportmodel.NodeAddress

// NodeDiagnostic is one typed report-safe diagnostic attached to a report node.
type NodeDiagnostic = reportmodel.NodeDiagnostic

// HTTPDiagnostic is the report-safe summary of one HTTP exchange.
type HTTPDiagnostic = reportmodel.HTTPDiagnostic

// AttemptReport summarizes one eventually attempt.
type AttemptReport = reportmodel.AttemptReport

// EventuallyReport summarizes convergence behavior for an act with eventually.
type EventuallyReport = reportmodel.EventuallyReport

// PayloadMetadata describes how captured payload data was handled.
type PayloadMetadata = reportmodel.PayloadMetadata

// Event is a low-level runtime event used to build reports and live mirrors.
type Event = reportmodel.Event

// Summary counts terminal scenario outcomes for a run.
type Summary = reportmodel.Summary

// NodeReport is the final terminal snapshot of one logical node.
type NodeReport = reportmodel.NodeReport

// LogRecord is one scenario-authored log materialized in the run report.
type LogRecord = reportmodel.LogRecord

// LogSummary reports the effective scenario-authored log limits and accounting.
type LogSummary = reportmodel.LogSummary

// FailureIndexEntry points to one failed node inside Report.Failures.
type FailureIndexEntry = reportmodel.FailureIndexEntry

// Report is the final materialized report for one stage run.
type Report = reportmodel.Report
