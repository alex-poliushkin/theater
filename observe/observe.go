package observe

import (
	"time"
)

// Kind identifies the type of a live observation envelope.
type Kind string

// NodeKind identifies the logical runtime node referenced by an envelope.
type NodeKind string

// Live observation envelope kinds.
const (
	KindTransition Kind = "transition"
	KindProgress   Kind = "progress"
	KindDiagnostic Kind = "diagnostic"
	KindLogChunk   Kind = "log_chunk"
	KindDropped    Kind = "dropped"
)

// Live observation node kinds.
const (
	NodeKindStage    NodeKind = "stage"
	NodeKindScenario NodeKind = "scenario"
	NodeKindAct      NodeKind = "act"
	NodeKindAction   NodeKind = "action"
	NodeKindLog      NodeKind = "log"
)

// Envelope is one live observation event emitted during a run.
type Envelope struct {
	RunID         string
	Seq           uint64
	ObservedAt    time.Time
	SourceAt      *time.Time
	Kind          Kind
	DurableMirror bool
	Node          NodeRef
	Transition    *Transition
	Progress      *Progress
	Diagnostic    *Diagnostic
	LogChunk      *LogChunk
	Dropped       *DroppedNotice
}

// NodeRef identifies the logical runtime node associated with an envelope.
type NodeRef struct {
	Kind           NodeKind
	StageID        string
	ScenarioCallID string
	ScenarioID     string
	Path           string
	Attempt        int
}

// Transition describes a node status transition.
type Transition struct {
	EventKind      string
	Status         string
	FailureKind    string
	FailureAt      string
	FailureSummary string
}

// Progress carries a live progress update.
type Progress struct {
	Phase         string
	Message       string
	Current       *int64
	Total         *int64
	Unit          string
	Percent       *float64
	Indeterminate bool
}

// Diagnostic carries a live diagnostic message.
type Diagnostic struct {
	Message string
	Fields  map[string]string
}

// LogChunk carries one chunk of streamed log output.
type LogChunk struct {
	Stream string
	Data   []byte
}

// DroppedNotice reports how many envelopes were dropped for one node.
type DroppedNotice struct {
	Count uint64
}

// Subscription exposes a live event stream and its shutdown hook.
type Subscription interface {
	Events() <-chan Envelope
	Close()
}

// Sink receives live envelopes and returns a delivery sequence hint.
type Sink interface {
	Publish(Envelope) uint64
}

// Reporter is the action-facing helper used to emit live observations.
type Reporter interface {
	Progress(Progress)
	Diagnostic(Diagnostic)
	LogChunk(LogChunk)
}
