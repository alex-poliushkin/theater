package theater

import "context"

// DebugCheckpoint carries one action-authored mid-action debug snapshot.
type DebugCheckpoint struct {
	Name   string
	Values Values
}

// DebugCheckpointReporter is an optional action-facing reporter extension for
// cooperative mid-action checkpoints.
type DebugCheckpointReporter interface {
	DebugCheckpoint(DebugCheckpoint)
}

// DebugStateSnapshotter is an optional state-backend extension that can enrich
// pause-time state snapshots with backend-authored fields.
type DebugStateSnapshotter interface {
	DebugStateSnapshot(context.Context) (Values, error)
}
