package report

import (
	"errors"
	"fmt"
	"time"
)

// Public report schema version and enum values used by run documents.
const (
	RunDocumentSchemaVersion = "v1alpha1"

	DefaultScenarioLogPreviewLimitBytes = 4 * 1024
	DefaultScenarioLogRecordsPerAct     = 32
	DefaultScenarioLogRecordsPerRun     = 1024

	EventKindStageRunning        = "stage.running"
	EventKindStageFinished       = "stage.finished"
	EventKindScenarioRunning     = "scenario.running"
	EventKindScenarioFinished    = "scenario.finished"
	EventKindActRunning          = "act.running"
	EventKindActFinished         = "act.finished"
	EventKindActionRunning       = "action.running"
	EventKindActionFinished      = "action.finished"
	EventKindExpectationFinished = "expectation.finished"
	EventKindLogEmitted          = "log.emitted"

	NodeKindScenario    NodeKind = "scenario"
	NodeKindAct         NodeKind = "act"
	NodeKindAction      NodeKind = "action"
	NodeKindExpectation NodeKind = "expectation"
	NodeKindLog         NodeKind = "log"

	LogStatusEmitted LogStatus = "emitted"
	LogStatusOmitted LogStatus = "omitted"
	LogStatusError   LogStatus = "error"

	SensitivityNone     Sensitivity = "none"
	SensitivityInternal Sensitivity = "internal"
	SensitivityPersonal Sensitivity = "personal"
	SensitivitySecret   Sensitivity = "secret"

	CaptureOmit        Capture = "omit"
	CaptureSummary     Capture = "summary"
	CaptureArtifactRef Capture = "artifact_ref"

	TerminationReasonConverged        TerminationReason = "converged"
	TerminationReasonDeadlineExceeded TerminationReason = "deadline_exceeded"
	TerminationReasonTerminalFailure  TerminationReason = "terminal_failure"
	//nolint:misspell // public report contract freezes parent_cancelled spelling.
	TerminationReasonParentCanceled TerminationReason = "parent_cancelled"

	SkipReasonExplicit     SkipReason = "explicit"
	SkipReasonStageAborted SkipReason = "stage_aborted"
)

// Sensitivity classifies how a value should be treated in diagnostics and
// report payloads.
type Sensitivity string

// Capture defines how much of a value may appear in previews or artifact
// payloads.
type Capture string

// NodeKind identifies the logical node represented in final reports.
type NodeKind string

// LogStatus identifies how one scenario-authored log record was handled.
type LogStatus string

// TerminationReason explains why an eventually block stopped retrying.
type TerminationReason string

// SkipReason explains why a node or scenario finished as skipped.
type SkipReason string

// GenerationMetadata records the generator seed and base time captured for one run.
type GenerationMetadata struct {
	Seed     string    `json:"seed,omitempty"`
	BaseTime time.Time `json:"base_time,omitempty"`
}

// Preview is a report-safe preview of a captured value or payload.
type Preview struct {
	Kind          string `json:"kind,omitempty"`
	Text          string `json:"text,omitempty"`
	JSONValue     any    `json:"json_value,omitempty"`
	SizeHint      int64  `json:"size_hint,omitempty"`
	Truncated     bool   `json:"truncated,omitempty"`
	Redacted      bool   `json:"redacted,omitempty"`
	OmittedReason string `json:"omitted_reason,omitempty"`
	ContentType   string `json:"content_type,omitempty"`
}

// ArtifactRef points to payload content stored outside the report document.
type ArtifactRef struct {
	Name             string       `json:"name,omitempty"`
	Kind             string       `json:"kind,omitempty"`
	ContentType      string       `json:"content_type,omitempty"`
	Locator          string       `json:"locator"`
	SizeBytes        int64        `json:"size_bytes,omitempty"`
	CreatedByAddress *NodeAddress `json:"created_by_addr,omitempty"`
	Sensitive        bool         `json:"sensitive,omitempty"`
	Retention        string       `json:"retention,omitempty"`
	PreviewAvailable bool         `json:"preview_available,omitempty"`
}

// Contrast describes expected-versus-actual mismatch details.
type Contrast struct {
	Kind     string   `json:"kind,omitempty"`
	Summary  string   `json:"summary,omitempty"`
	Expected *Preview `json:"expected,omitempty"`
	Actual   *Preview `json:"actual,omitempty"`
	Excerpt  string   `json:"excerpt,omitempty"`
}

// NodeAddress identifies a logical runtime node within a scenario call.
type NodeAddress struct {
	ScenarioCallPath string   `json:"scenario_call_path,omitempty"`
	ActID            string   `json:"act_id,omitempty"`
	Kind             NodeKind `json:"kind,omitempty"`
	NodeRef          string   `json:"node_ref,omitempty"`
	Phase            string   `json:"phase,omitempty"`
	AttemptIndex     int      `json:"attempt_index,omitempty"`
}

// AttemptReport summarizes one eventually attempt.
type AttemptReport struct {
	Index          int           `json:"index"`
	StartedAt      time.Time     `json:"started_at,omitempty"`
	EndedAt        time.Time     `json:"ended_at,omitempty"`
	DurationMs     int64         `json:"duration_ms,omitempty"`
	Status         Status        `json:"status"`
	Retryable      bool          `json:"retryable,omitempty"`
	Failure        *Failure      `json:"failure,omitempty"`
	FailureSummary string        `json:"failure_summary,omitempty"`
	Artifacts      []ArtifactRef `json:"artifacts,omitempty"`
	Preview        *Preview      `json:"preview,omitempty"`
	Contrast       *Contrast     `json:"contrast,omitempty"`
}

// EventuallyReport summarizes convergence behavior for an act with eventually.
type EventuallyReport struct {
	Enabled             bool              `json:"enabled"`
	Timeout             string            `json:"timeout,omitempty"`
	Interval            string            `json:"interval,omitempty"`
	AttemptsTotal       int               `json:"attempts_total,omitempty"`
	ElapsedMs           int64             `json:"elapsed_ms,omitempty"`
	FinalOutcome        Status            `json:"final_outcome,omitempty"`
	TerminationReason   TerminationReason `json:"termination_reason,omitempty"`
	SuccessAttempt      int               `json:"success_attempt,omitempty"`
	FinalFailureReason  *Failure          `json:"final_failure_reason,omitempty"`
	LastObservedFailure *Failure          `json:"last_observed_failure,omitempty"`
	AttemptTimeline     []AttemptReport   `json:"attempt_timeline,omitempty"`
}

// PayloadMetadata describes how captured payload data was handled.
type PayloadMetadata struct {
	Origin      string      `json:"origin"`
	Sensitivity Sensitivity `json:"sensitivity"`
	Redacted    bool        `json:"redacted"`
	Truncated   bool        `json:"truncated"`
	ContentType string      `json:"content_type"`
	SizeBytes   int64       `json:"size_bytes"`
	Capture     Capture     `json:"capture"`
	ArtifactRef string      `json:"artifact_ref,omitempty"`
}

// LogRecord is one scenario-authored log materialized in the run report.
type LogRecord struct {
	ID             string           `json:"id"`
	Path           string           `json:"path"`
	StageID        string           `json:"stage_id,omitempty"`
	ScenarioID     string           `json:"scenario_id,omitempty"`
	ScenarioCallID string           `json:"scenario_call_id,omitempty"`
	ScenarioPath   string           `json:"scenario_path,omitempty"`
	ActID          string           `json:"act_id,omitempty"`
	Attempt        int              `json:"attempt"`
	ScenarioSeq    int              `json:"scenario_seq"`
	Status         LogStatus        `json:"status"`
	Format         string           `json:"format,omitempty"`
	SourceSpan     *SourceRef       `json:"source_span,omitempty"`
	Address        *NodeAddress     `json:"address,omitempty"`
	Preview        *Preview         `json:"preview,omitempty"`
	Payload        *PayloadMetadata `json:"payload,omitempty"`
	Failure        *Failure         `json:"failure,omitempty"`
	Dropped        bool             `json:"dropped,omitempty"`
	Truncated      bool             `json:"truncated,omitempty"`
}

// LogSummary reports the effective scenario-authored log limits and accounting.
type LogSummary struct {
	Records           int `json:"records,omitempty"`
	DroppedRecords    int `json:"dropped_records,omitempty"`
	TruncatedRecords  int `json:"truncated_records,omitempty"`
	PreviewLimitBytes int `json:"preview_limit_bytes,omitempty"`
	PerActLimit       int `json:"per_act_limit,omitempty"`
	PerRunLimit       int `json:"per_run_limit,omitempty"`
}

// Event is a low-level runtime event used to build reports and live mirrors.
type Event struct {
	Kind           string              `json:"kind"`
	StageID        string              `json:"stage_id,omitempty"`
	StagePath      string              `json:"stage_path"`
	ScenarioID     string              `json:"scenario_id,omitempty"`
	ScenarioCallID string              `json:"scenario_call_id,omitempty"`
	ScenarioPath   string              `json:"scenario_path,omitempty"`
	Path           string              `json:"path"`
	Attempt        int                 `json:"attempt"`
	ScenarioSeq    int                 `json:"scenario_seq"`
	Status         Status              `json:"status"`
	SkipReason     SkipReason          `json:"skip_reason,omitempty"`
	Failure        *Failure            `json:"failure,omitempty"`
	StartedAt      time.Time           `json:"started_at,omitempty"`
	EndedAt        time.Time           `json:"ended_at,omitempty"`
	DurationMs     int64               `json:"duration_ms,omitempty"`
	SourceSpan     *SourceRef          `json:"source_span,omitempty"`
	Preview        *Preview            `json:"preview,omitempty"`
	Contrast       *Contrast           `json:"contrast,omitempty"`
	Observations   *ActionObservations `json:"observations,omitempty"`
	Eventually     *EventuallyReport   `json:"eventually,omitempty"`
	Address        *NodeAddress        `json:"address,omitempty"`
	Payload        *PayloadMetadata    `json:"payload,omitempty"`
	Log            *LogRecord          `json:"log,omitempty"`
	Generation     *GenerationMetadata `json:"generation,omitempty"`
}

// Summary counts terminal scenario outcomes for a run.
type Summary struct {
	TotalScenarios    int `json:"total_scenarios"`
	PassedScenarios   int `json:"passed_scenarios"`
	FailedScenarios   int `json:"failed_scenarios"`
	CanceledScenarios int `json:"canceled_scenarios"`
	SkippedScenarios  int `json:"skipped_scenarios"`
}

// NodeReport is the final terminal snapshot of one logical node.
type NodeReport struct {
	Kind           NodeKind            `json:"kind"`
	StageID        string              `json:"stage_id,omitempty"`
	Path           string              `json:"path"`
	ScenarioID     string              `json:"scenario_id,omitempty"`
	ScenarioCallID string              `json:"scenario_call_id,omitempty"`
	ScenarioPath   string              `json:"scenario_path,omitempty"`
	Attempt        int                 `json:"attempt"`
	ScenarioSeq    int                 `json:"scenario_seq"`
	Status         Status              `json:"status"`
	SkipReason     SkipReason          `json:"skip_reason,omitempty"`
	Failure        *Failure            `json:"failure,omitempty"`
	StartedAt      time.Time           `json:"started_at,omitempty"`
	EndedAt        time.Time           `json:"ended_at,omitempty"`
	DurationMs     int64               `json:"duration_ms,omitempty"`
	Address        *NodeAddress        `json:"address,omitempty"`
	SourceSpan     *SourceRef          `json:"source_span,omitempty"`
	Preview        *Preview            `json:"preview,omitempty"`
	Artifacts      []ArtifactRef       `json:"artifacts,omitempty"`
	Contrast       *Contrast           `json:"contrast,omitempty"`
	Observations   *ActionObservations `json:"observations,omitempty"`
	Eventually     *EventuallyReport   `json:"eventually,omitempty"`
	Payload        *PayloadMetadata    `json:"payload,omitempty"`
}

// FailureIndexEntry points to one failed node inside Report.Failures.
type FailureIndexEntry struct {
	Path       string       `json:"path"`
	Address    *NodeAddress `json:"address,omitempty"`
	SourceSpan *SourceRef   `json:"source_span,omitempty"`
	Failure    *Failure     `json:"failure"`
}

// Report is the final materialized report for one stage run.
type Report struct {
	StageID    string              `json:"stage_id,omitempty"`
	StagePath  string              `json:"stage_path"`
	Status     Status              `json:"status"`
	Failure    *Failure            `json:"failure,omitempty"`
	StartedAt  time.Time           `json:"started_at,omitempty"`
	EndedAt    time.Time           `json:"ended_at,omitempty"`
	DurationMs int64               `json:"duration_ms,omitempty"`
	Generation *GenerationMetadata `json:"generation,omitempty"`
	Nodes      []NodeReport        `json:"nodes,omitempty"`
	Logs       []LogRecord         `json:"logs,omitempty"`
	LogSummary *LogSummary         `json:"log_summary,omitempty"`
	Failures   []FailureIndexEntry `json:"failures,omitempty"`
	Summary    Summary             `json:"summary"`
}

func (m GenerationMetadata) Validate() error {
	if m.Seed == "" {
		return errors.New("generation seed is required")
	}

	if m.BaseTime.IsZero() {
		return errors.New("generation base_time is required")
	}

	return nil
}

func (s Sensitivity) Valid() bool {
	switch s {
	case SensitivityNone, SensitivityInternal, SensitivityPersonal, SensitivitySecret:
		return true
	default:
		return false
	}
}

func (c Capture) Valid() bool {
	switch c {
	case CaptureOmit, CaptureSummary, CaptureArtifactRef:
		return true
	default:
		return false
	}
}

func (k NodeKind) Valid() bool {
	switch k {
	case NodeKindScenario, NodeKindAct, NodeKindAction, NodeKindExpectation, NodeKindLog:
		return true
	default:
		return false
	}
}

func (s LogStatus) Valid() bool {
	switch s {
	case LogStatusEmitted, LogStatusOmitted, LogStatusError:
		return true
	default:
		return false
	}
}

func (r TerminationReason) Valid() bool {
	switch r {
	case "",
		TerminationReasonConverged,
		TerminationReasonDeadlineExceeded,
		TerminationReasonTerminalFailure,
		TerminationReasonParentCanceled:
		return true
	default:
		return false
	}
}

func (r SkipReason) Valid() bool {
	switch r {
	case "", SkipReasonExplicit, SkipReasonStageAborted:
		return true
	default:
		return false
	}
}

func (a NodeAddress) Validate() error {
	if a.Kind != "" && !a.Kind.Valid() {
		return fmt.Errorf("address kind %q is invalid", a.Kind)
	}

	if a.AttemptIndex < 0 {
		return fmt.Errorf("attempt_index %d is invalid", a.AttemptIndex)
	}

	return nil
}

func (m PayloadMetadata) Validate() error {
	if !m.Sensitivity.Valid() {
		return fmt.Errorf("payload sensitivity %q is invalid", m.Sensitivity)
	}

	if !m.Capture.Valid() {
		return fmt.Errorf("payload capture %q is invalid", m.Capture)
	}

	if m.SizeBytes < 0 {
		return fmt.Errorf("payload size %d is invalid", m.SizeBytes)
	}

	if m.Capture == CaptureArtifactRef && m.ArtifactRef == "" {
		return fmt.Errorf("payload capture %q requires artifact ref", m.Capture)
	}

	if m.Capture != CaptureArtifactRef && m.ArtifactRef != "" {
		return fmt.Errorf("payload capture %q must not carry artifact ref", m.Capture)
	}

	return nil
}

func (e Event) Validate() error {
	if e.Attempt < 0 {
		return fmt.Errorf("event attempt %d is invalid", e.Attempt)
	}

	if e.ScenarioSeq < 0 {
		return fmt.Errorf("event scenario sequence %d is invalid", e.ScenarioSeq)
	}

	if !e.Status.Valid() {
		return fmt.Errorf("status %q is invalid", e.Status)
	}

	if !e.SkipReason.Valid() {
		return fmt.Errorf("skip_reason %q is invalid", e.SkipReason)
	}

	if e.SkipReason != "" && e.Status != StatusSkipped {
		return fmt.Errorf("skip_reason %q requires skipped status", e.SkipReason)
	}

	if err := validateTimedWindow(e.StartedAt, e.EndedAt, e.DurationMs); err != nil {
		return fmt.Errorf("event timing is invalid: %w", err)
	}

	if e.Status.IsTerminal() {
		if err := ValidateTerminalOutcome(e.Status, e.Failure); err != nil {
			return err
		}
	} else if e.Failure != nil {
		return fmt.Errorf("non-terminal status %q must not carry failure", e.Status)
	}

	if err := validateEventPayload(e.Payload); err != nil {
		return err
	}

	if e.Generation != nil {
		if err := e.Generation.Validate(); err != nil {
			return fmt.Errorf("event generation is invalid: %w", err)
		}
	}

	if e.Observations != nil {
		if err := e.Observations.Validate(); err != nil {
			return fmt.Errorf("event observations are invalid: %w", err)
		}
	}

	if e.Address != nil {
		if err := e.Address.Validate(); err != nil {
			return fmt.Errorf("event address is invalid: %w", err)
		}
	}

	if err := validateEventLog(e); err != nil {
		return err
	}

	if e.Eventually == nil {
		return nil
	}

	return e.Eventually.Validate()
}

func (r LogRecord) Validate() error {
	if r.ID == "" {
		return errors.New("log id is required")
	}

	if r.Path == "" {
		return errors.New("log path is required")
	}

	if r.ScenarioID == "" {
		return errors.New("log scenario id is required")
	}

	if r.ScenarioCallID == "" {
		return errors.New("log scenario call id is required")
	}

	if r.ScenarioPath == "" {
		return errors.New("log scenario path is required")
	}

	if r.ActID == "" {
		return errors.New("log act id is required")
	}

	if r.Attempt < 0 {
		return fmt.Errorf("log attempt %d is invalid", r.Attempt)
	}

	if r.ScenarioSeq < 0 {
		return fmt.Errorf("log scenario sequence %d is invalid", r.ScenarioSeq)
	}

	if !r.Status.Valid() {
		return fmt.Errorf("log status %q is invalid", r.Status)
	}

	if r.Status == LogStatusError {
		if r.Failure == nil {
			return errors.New("error log requires failure")
		}
	} else if r.Failure != nil {
		return fmt.Errorf("%s log must not carry failure", r.Status)
	}

	if r.Address == nil {
		return errors.New("log address is required")
	}

	if err := r.Address.Validate(); err != nil {
		return fmt.Errorf("log address is invalid: %w", err)
	}

	if r.Address.Kind != NodeKindLog {
		return fmt.Errorf("log address kind %q is invalid", r.Address.Kind)
	}

	if err := validateLogRecordAddress(r); err != nil {
		return err
	}

	if r.Payload != nil {
		if err := r.Payload.Validate(); err != nil {
			return fmt.Errorf("log payload is invalid: %w", err)
		}
	}

	return nil
}

func (s LogSummary) Validate(recordCount int) error {
	if s.Records < 0 {
		return fmt.Errorf("log records %d is invalid", s.Records)
	}

	if s.DroppedRecords < 0 {
		return fmt.Errorf("log dropped records %d is invalid", s.DroppedRecords)
	}

	if s.TruncatedRecords < 0 {
		return fmt.Errorf("log truncated records %d is invalid", s.TruncatedRecords)
	}

	if s.PreviewLimitBytes <= 0 {
		return fmt.Errorf("log preview limit %d is invalid", s.PreviewLimitBytes)
	}

	if s.PerActLimit <= 0 {
		return fmt.Errorf("log per-act limit %d is invalid", s.PerActLimit)
	}

	if s.PerRunLimit <= 0 {
		return fmt.Errorf("log per-run limit %d is invalid", s.PerRunLimit)
	}

	if s.Records != recordCount {
		return fmt.Errorf("log records %d does not match report logs %d", s.Records, recordCount)
	}

	return nil
}

func (n NodeReport) Validate() error {
	if !validReportNodeKind(n.Kind) {
		return fmt.Errorf("node kind %q is invalid", n.Kind)
	}

	if n.Path == "" {
		return errors.New("node path is required")
	}

	if !n.SkipReason.Valid() {
		return fmt.Errorf("node skip_reason %q is invalid", n.SkipReason)
	}

	if n.SkipReason != "" && n.Status != StatusSkipped {
		return fmt.Errorf("node skip_reason %q requires skipped status", n.SkipReason)
	}

	if n.Attempt < 0 {
		return fmt.Errorf("node attempt %d is invalid", n.Attempt)
	}

	if n.ScenarioSeq < 0 {
		return fmt.Errorf("node scenario sequence %d is invalid", n.ScenarioSeq)
	}

	if err := ValidateTerminalOutcome(n.Status, n.Failure); err != nil {
		return err
	}

	if err := validateTimedWindow(n.StartedAt, n.EndedAt, n.DurationMs); err != nil {
		return fmt.Errorf("node timing is invalid: %w", err)
	}

	if n.Eventually != nil {
		if err := n.Eventually.Validate(); err != nil {
			return fmt.Errorf("node eventually is invalid: %w", err)
		}
	}

	if n.Address != nil {
		if err := n.Address.Validate(); err != nil {
			return fmt.Errorf("node address is invalid: %w", err)
		}
	}

	if n.Observations != nil {
		if err := n.Observations.Validate(); err != nil {
			return fmt.Errorf("node observations are invalid: %w", err)
		}
	}

	if n.Payload == nil {
		return validateNodeArtifacts(n.Artifacts)
	}

	if err := n.Payload.Validate(); err != nil {
		return err
	}

	return validateNodeArtifacts(n.Artifacts)
}

func (r EventuallyReport) Validate() error {
	if !r.Enabled {
		return errors.New("eventually report must be enabled")
	}

	if r.AttemptsTotal < 0 {
		return fmt.Errorf("attempts_total %d is invalid", r.AttemptsTotal)
	}

	if r.ElapsedMs < 0 {
		return fmt.Errorf("elapsed_ms %d is invalid", r.ElapsedMs)
	}

	if !r.FinalOutcome.Valid() || !r.FinalOutcome.IsTerminal() {
		return fmt.Errorf("final_outcome %q is invalid", r.FinalOutcome)
	}

	if !r.TerminationReason.Valid() {
		return fmt.Errorf("termination_reason %q is invalid", r.TerminationReason)
	}

	for i := range r.AttemptTimeline {
		if err := r.AttemptTimeline[i].Validate(); err != nil {
			return fmt.Errorf("attempt_timeline %d is invalid: %w", i, err)
		}
	}

	return nil
}

func (r AttemptReport) Validate() error {
	if r.Index <= 0 {
		return fmt.Errorf("index %d is invalid", r.Index)
	}

	if r.DurationMs < 0 {
		return fmt.Errorf("duration_ms %d is invalid", r.DurationMs)
	}

	if !r.Status.Valid() || !r.Status.IsTerminal() {
		return fmt.Errorf("status %q is invalid", r.Status)
	}

	if err := ValidateTerminalOutcome(r.Status, r.Failure); err != nil {
		return err
	}

	return validateNodeArtifacts(r.Artifacts)
}

func (r Report) Validate() error {
	if err := ValidateTerminalOutcome(r.Status, r.Failure); err != nil {
		return err
	}

	if err := validateTimedWindow(r.StartedAt, r.EndedAt, r.DurationMs); err != nil {
		return fmt.Errorf("report timing is invalid: %w", err)
	}

	if r.Generation != nil {
		if err := r.Generation.Validate(); err != nil {
			return fmt.Errorf("report generation is invalid: %w", err)
		}
	}

	if r.Summary.TotalScenarios < 0 {
		return fmt.Errorf("summary total scenarios %d is invalid", r.Summary.TotalScenarios)
	}

	if r.Summary.PassedScenarios < 0 || r.Summary.FailedScenarios < 0 || r.Summary.CanceledScenarios < 0 || r.Summary.SkippedScenarios < 0 {
		return errors.New("summary counters must be non-negative")
	}

	for i := range r.Nodes {
		if err := r.Nodes[i].Validate(); err != nil {
			return fmt.Errorf("report node %d is invalid: %w", i, err)
		}
	}

	if err := validateReportLogs(r.Logs, r.LogSummary); err != nil {
		return err
	}

	for i := range r.Failures {
		if r.Failures[i].Path == "" {
			return fmt.Errorf("report failure index %d is invalid: path is required", i)
		}

		if r.Failures[i].Failure == nil {
			return fmt.Errorf("report failure index %d is invalid: failure is required", i)
		}
	}

	return nil
}

func validateReportLogs(logs []LogRecord, summary *LogSummary) error {
	actCounts := make(map[string]int)
	truncatedRecords := 0
	for i := range logs {
		if err := logs[i].Validate(); err != nil {
			return fmt.Errorf("report log %d is invalid: %w", i, err)
		}
		if logs[i].Dropped {
			return fmt.Errorf("report log %d is invalid: dropped records belong in log summary", i)
		}
		if logs[i].Truncated {
			truncatedRecords++
		}
		actCounts[reportLogActCountKey(logs[i])]++
	}

	if len(logs) != 0 && summary == nil {
		return errors.New("report logs require log summary")
	}

	if summary != nil {
		if err := summary.Validate(len(logs)); err != nil {
			return fmt.Errorf("report log summary is invalid: %w", err)
		}
		if err := validateReportLogSummaryBounds(logs, actCounts, truncatedRecords, *summary); err != nil {
			return err
		}
	}

	return nil
}

func validateReportLogSummaryBounds(
	logs []LogRecord,
	actCounts map[string]int,
	truncatedRecords int,
	summary LogSummary,
) error {
	if len(logs) > summary.PerRunLimit {
		return fmt.Errorf("report logs %d exceeds per-run limit %d", len(logs), summary.PerRunLimit)
	}

	for key, count := range actCounts {
		if count > summary.PerActLimit {
			return fmt.Errorf("report logs for act %q exceed per-act limit %d", key, summary.PerActLimit)
		}
	}

	for i := range logs {
		if logs[i].Preview != nil && len(logs[i].Preview.Text) > summary.PreviewLimitBytes {
			return fmt.Errorf("report log %d preview exceeds preview limit %d", i, summary.PreviewLimitBytes)
		}
		if logs[i].Preview != nil && logs[i].Preview.JSONValue != nil {
			return fmt.Errorf("report log %d preview must not carry json_value", i)
		}
	}

	if summary.TruncatedRecords != truncatedRecords {
		return fmt.Errorf("log truncated records %d does not match report logs %d", summary.TruncatedRecords, truncatedRecords)
	}

	return nil
}

func reportLogActCountKey(record LogRecord) string {
	return record.ScenarioPath + "\x00" + record.ActID
}

func validateEventLog(event Event) error {
	if event.Kind != EventKindLogEmitted {
		if event.Log == nil {
			return nil
		}

		return fmt.Errorf("event kind %q must not carry log record", event.Kind)
	}

	if event.Address == nil {
		return errors.New("log event requires address")
	}

	if event.Log == nil {
		return errors.New("log event requires log record")
	}

	if err := event.Log.Validate(); err != nil {
		return fmt.Errorf("event log is invalid: %w", err)
	}

	if err := validateEventLogOutcome(event, *event.Log); err != nil {
		return err
	}

	return validateEventLogIdentity(event, *event.Log)
}

func validReportNodeKind(kind NodeKind) bool {
	switch kind {
	case NodeKindScenario, NodeKindAct, NodeKindAction, NodeKindExpectation:
		return true
	default:
		return false
	}
}

func validateLogRecordAddress(record LogRecord) error {
	if record.Address.ScenarioCallPath != record.ScenarioPath {
		return fmt.Errorf(
			"log address scenario call path %q does not match log scenario path %q",
			record.Address.ScenarioCallPath,
			record.ScenarioPath,
		)
	}
	if record.Address.ActID != record.ActID {
		return fmt.Errorf("log address act id %q does not match log act id %q", record.Address.ActID, record.ActID)
	}
	if record.Address.NodeRef != record.ID {
		return fmt.Errorf("log address node ref %q does not match log id %q", record.Address.NodeRef, record.ID)
	}
	if record.Address.Phase != "log.evaluate" {
		return fmt.Errorf("log address phase %q is invalid", record.Address.Phase)
	}
	if record.Address.AttemptIndex != record.Attempt {
		return fmt.Errorf("log address attempt %d does not match log attempt %d", record.Address.AttemptIndex, record.Attempt)
	}

	return nil
}

func validateEventLogOutcome(event Event, log LogRecord) error {
	if log.Status == LogStatusError {
		if event.Status != StatusFailed {
			return fmt.Errorf("error log event status %q is invalid", event.Status)
		}
		if event.Failure == nil {
			return errors.New("error log event requires failure")
		}
		if !failuresEqual(event.Failure, log.Failure) {
			return errors.New("error log event failure must match log failure")
		}

		return nil
	}

	if event.Status != StatusPassed {
		return fmt.Errorf("%s log event status %q is invalid", log.Status, event.Status)
	}
	if event.Failure != nil {
		return fmt.Errorf("%s log event must not carry failure", log.Status)
	}

	return nil
}

func validateEventLogIdentity(event Event, log LogRecord) error {
	if event.Path != log.Path {
		return fmt.Errorf("log event path %q does not match log path %q", event.Path, log.Path)
	}
	if event.StageID != log.StageID {
		return fmt.Errorf("log event stage id %q does not match log stage id %q", event.StageID, log.StageID)
	}
	if event.ScenarioID != log.ScenarioID {
		return fmt.Errorf("log event scenario id %q does not match log scenario id %q", event.ScenarioID, log.ScenarioID)
	}
	if event.ScenarioCallID != log.ScenarioCallID {
		return fmt.Errorf("log event scenario call id %q does not match log scenario call id %q", event.ScenarioCallID, log.ScenarioCallID)
	}
	if event.ScenarioPath != log.ScenarioPath {
		return fmt.Errorf("log event scenario path %q does not match log scenario path %q", event.ScenarioPath, log.ScenarioPath)
	}
	if event.Attempt != log.Attempt {
		return fmt.Errorf("log event attempt %d does not match log attempt %d", event.Attempt, log.Attempt)
	}
	if event.ScenarioSeq != log.ScenarioSeq {
		return fmt.Errorf("log event scenario sequence %d does not match log scenario sequence %d", event.ScenarioSeq, log.ScenarioSeq)
	}
	if !nodeAddressesEqual(event.Address, log.Address) {
		return errors.New("log event address must match log address")
	}
	if !sourceRefsEqual(event.SourceSpan, log.SourceSpan) {
		return errors.New("log event source span must match log source span")
	}

	return nil
}

func failuresEqual(left, right *Failure) bool {
	if left == nil || right == nil {
		return left == right
	}
	if left.Kind != right.Kind || left.Phase != right.Phase || left.At != right.At || left.Summary != right.Summary {
		return false
	}
	if left.Cause == nil && right.Cause == nil {
		return true
	}
	if left.Cause == nil || right.Cause == nil {
		return false
	}

	return left.Cause.Error() == right.Cause.Error()
}

func nodeAddressesEqual(left, right *NodeAddress) bool {
	if left == nil || right == nil {
		return left == right
	}

	return left.ScenarioCallPath == right.ScenarioCallPath &&
		left.ActID == right.ActID &&
		left.Kind == right.Kind &&
		left.NodeRef == right.NodeRef &&
		left.Phase == right.Phase &&
		left.AttemptIndex == right.AttemptIndex
}

func sourceRefsEqual(left, right *SourceRef) bool {
	if left == nil || right == nil {
		return left == right
	}

	return left.File == right.File &&
		left.Line == right.Line &&
		left.Column == right.Column
}

func validateEventPayload(payload *PayloadMetadata) error {
	if payload == nil {
		return nil
	}

	return payload.Validate()
}

func validateNodeArtifacts(artifacts []ArtifactRef) error {
	for i := range artifacts {
		if artifacts[i].Locator == "" {
			return fmt.Errorf("artifact %d locator is required", i)
		}

		if artifacts[i].SizeBytes < 0 {
			return fmt.Errorf("artifact %d size %d is invalid", i, artifacts[i].SizeBytes)
		}
	}

	return nil
}

func validateTimedWindow(startedAt, endedAt time.Time, durationMs int64) error {
	if durationMs < 0 {
		return fmt.Errorf("duration_ms %d is invalid", durationMs)
	}

	started := !startedAt.IsZero()
	ended := !endedAt.IsZero()
	if started != ended {
		return errors.New("started_at and ended_at must either both be set or both be empty")
	}

	if !started {
		return nil
	}

	if endedAt.Before(startedAt) {
		return errors.New("ended_at must not be before started_at")
	}

	return nil
}
