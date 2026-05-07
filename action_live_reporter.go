package theater

import (
	"sync"

	"github.com/alex-poliushkin/theater/internal/liveobserve"
	"github.com/alex-poliushkin/theater/observe"
)

const actionLiveLogChunkBytes = liveobserve.DefaultLogChunkBytes

type actionLiveReporter struct {
	publisher *liveobserve.Publisher
	mu        sync.Mutex
	failure   error
}

type actionStreamSummary = liveobserve.StreamSummary

type debugActionCheckpointReporter struct {
	*actionLiveReporter
	handler func(DebugCheckpoint) error
}

func newActionLiveReporter(sink observe.Sink, node observe.NodeRef, tailLimit int) *actionLiveReporter {
	return &actionLiveReporter{
		publisher: liveobserve.NewPublisher(sink, node, actionLiveLogChunkBytes, tailLimit),
	}
}

func (r *actionLiveReporter) Progress(progress observe.Progress) {
	if r == nil {
		return
	}

	r.publisher.Progress(progress)
}

func (r *actionLiveReporter) Diagnostic(diagnostic observe.Diagnostic) {
	if r == nil {
		return
	}

	r.publisher.Diagnostic(diagnostic)
}

func (r *actionLiveReporter) LogChunk(chunk observe.LogChunk) {
	if r == nil {
		return
	}

	r.publisher.LogChunk(chunk)
}

func (r *actionLiveReporter) Snapshot() map[string]actionStreamSummary {
	if r == nil {
		return nil
	}

	return r.publisher.Snapshot()
}

func (r *debugActionCheckpointReporter) DebugCheckpoint(checkpoint DebugCheckpoint) {
	if r == nil {
		return
	}

	name := checkpoint.Name
	if name == "" {
		name = "checkpoint"
	}
	r.publisher.Diagnostic(observe.Diagnostic{
		Message: "debug checkpoint: " + name,
	})
	if r.handler == nil {
		return
	}

	if err := r.handler(DebugCheckpoint{
		Name:   name,
		Values: checkpoint.Values,
	}); err != nil {
		r.storeFailure(err)
	}
}

func (r *actionLiveReporter) checkpointReporter(
	handler func(DebugCheckpoint) error,
) *debugActionCheckpointReporter {
	if r == nil {
		return nil
	}

	return &debugActionCheckpointReporter{
		actionLiveReporter: r,
		handler:            handler,
	}
}

func (r *actionLiveReporter) Failure() error {
	if r == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	return r.failure
}

func (r *actionLiveReporter) storeFailure(err error) {
	if r == nil || err == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failure == nil {
		r.failure = err
	}
}
