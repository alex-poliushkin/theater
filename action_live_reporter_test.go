package theater

import (
	"errors"
	"sync"
	"testing"

	"github.com/alex-poliushkin/theater/internal/liveobserve"
	"github.com/alex-poliushkin/theater/observe"
)

type recordingSink struct {
	mu        sync.Mutex
	dropped   uint64
	envelopes []observe.Envelope
}

func TestActionLiveReporterPublishesObserveEventsAndTracksStreamSummary(t *testing.T) {
	t.Parallel()

	sink := &recordingSink{}
	reporter := newActionLiveReporter(sink, observe.NodeRef{
		Kind: observe.NodeKindAction,
		Path: "stage.main/call.login/act.submit/action",
	}, 4)

	current := int64(1)
	total := int64(3)
	percent := 33.3
	reporter.Progress(observe.Progress{
		Phase:   "run",
		Message: "starting",
		Current: &current,
		Total:   &total,
		Percent: &percent,
	})
	reporter.Diagnostic(observe.Diagnostic{
		Message: "captured",
		Fields:  map[string]string{"source": "stdout"},
	})
	reporter.LogChunk(observe.LogChunk{
		Stream: "stdout",
		Data:   []byte("abcdef"),
	})

	envelopes := sink.Snapshot()
	if got, want := len(envelopes), 3; got != want {
		t.Fatalf("published envelope count mismatch: got %d want %d", got, want)
	}

	if got, want := envelopes[0].Kind, observe.KindProgress; got != want {
		t.Fatalf("progress envelope kind mismatch: got %q want %q", got, want)
	}

	if got, want := envelopes[1].Kind, observe.KindDiagnostic; got != want {
		t.Fatalf("diagnostic envelope kind mismatch: got %q want %q", got, want)
	}

	if got, want := envelopes[2].Kind, observe.KindLogChunk; got != want {
		t.Fatalf("log chunk envelope kind mismatch: got %q want %q", got, want)
	}

	if got, want := string(envelopes[2].LogChunk.Data), "abcdef"; got != want {
		t.Fatalf("log chunk mismatch: got %q want %q", got, want)
	}

	observations := buildStreamObservations(reporter.Snapshot())
	if observations == nil {
		t.Fatal("stream observations must be present")
	}

	stream, ok := observations.Streams["stdout"]
	if !ok {
		t.Fatal("stdout stream observation must be present")
	}

	if got, want := stream.Payload.SizeBytes, int64(6); got != want {
		t.Fatalf("stream size mismatch: got %d want %d", got, want)
	}

	if got, want := stream.Preview.Text, "cdef"; got != want {
		t.Fatalf("stream preview mismatch: got %q want %q", got, want)
	}

	if !stream.Preview.Truncated {
		t.Fatal("stream preview must be marked truncated")
	}

	if stream.DroppedChunks != 0 {
		t.Fatalf("dropped chunk count mismatch: got %d want 0", stream.DroppedChunks)
	}
}

func TestActionLiveReporterCountsDroppedLiveChunks(t *testing.T) {
	t.Parallel()

	sink := &recordingSink{dropped: 2}
	reporter := newActionLiveReporter(sink, observe.NodeRef{
		Kind: observe.NodeKindAction,
		Path: "stage.main/call.login/act.submit/action",
	}, 8)

	reporter.LogChunk(observe.LogChunk{
		Stream: "stderr",
		Data:   []byte("warn"),
	})

	observations := buildStreamObservations(reporter.Snapshot())
	if observations == nil {
		t.Fatal("stream observations must be present")
	}

	stream, ok := observations.Streams["stderr"]
	if !ok {
		t.Fatal("stderr stream observation must be present")
	}

	if got, want := stream.DroppedChunks, uint64(2); got != want {
		t.Fatalf("dropped chunk count mismatch: got %d want %d", got, want)
	}
}

func TestBuildStreamObservationsEscapesInvalidBytes(t *testing.T) {
	t.Parallel()

	reporter := newActionLiveReporter(nil, observe.NodeRef{
		Kind: observe.NodeKindAction,
		Path: "stage.main/call.login/act.submit/action",
	}, 8)

	reporter.LogChunk(observe.LogChunk{
		Stream: "stdout",
		Data:   []byte{'o', 0x00, 'k', 0xff},
	})

	observations := buildStreamObservations(reporter.Snapshot())
	if observations == nil {
		t.Fatal("stream observations must be present")
	}

	stream, ok := observations.Streams["stdout"]
	if !ok {
		t.Fatal("stdout stream observation must be present")
	}

	if got, want := stream.Preview.Text, "o\\x00k\\xFF"; got != want {
		t.Fatalf("stream preview mismatch: got %q want %q", got, want)
	}
}

func TestBuildStreamObservationsEscapesMidRuneTailBytes(t *testing.T) {
	t.Parallel()

	reporter := newActionLiveReporter(nil, observe.NodeRef{
		Kind: observe.NodeKindAction,
		Path: "stage.main/call.login/act.submit/action",
	}, 2)

	reporter.LogChunk(observe.LogChunk{
		Stream: "stderr",
		Data:   []byte("éx"),
	})

	observations := buildStreamObservations(reporter.Snapshot())
	if observations == nil {
		t.Fatal("stream observations must be present")
	}

	stream, ok := observations.Streams["stderr"]
	if !ok {
		t.Fatal("stderr stream observation must be present")
	}

	if got, want := stream.Preview.Text, "\\xA9x"; got != want {
		t.Fatalf("stream preview mismatch: got %q want %q", got, want)
	}

	if !stream.Preview.Truncated {
		t.Fatal("stream preview must be marked truncated")
	}
}

func TestActionLiveReporterPublishesCheckpointDiagnosticAndStoresFailure(t *testing.T) {
	t.Parallel()

	sink := &recordingSink{}
	base := newActionLiveReporter(sink, observe.NodeRef{
		Kind: observe.NodeKindAction,
		Path: "stage.main/call.login/act.submit/action",
	}, 4)
	reporter := base.checkpointReporter(func(DebugCheckpoint) error {
		return errors.New("checkpoint failed")
	})

	reporter.DebugCheckpoint(DebugCheckpoint{
		Name:   "mid-action",
		Values: Values{"step": "halfway"},
	})

	envelopes := sink.Snapshot()
	if got, want := len(envelopes), 1; got != want {
		t.Fatalf("published envelope count mismatch: got %d want %d", got, want)
	}
	if got, want := envelopes[0].Kind, observe.KindDiagnostic; got != want {
		t.Fatalf("checkpoint envelope kind mismatch: got %q want %q", got, want)
	}
	if got, want := envelopes[0].Diagnostic.Message, "debug checkpoint: mid-action"; got != want {
		t.Fatalf("checkpoint message mismatch: got %q want %q", got, want)
	}
	if got, want := base.Failure().Error(), "checkpoint failed"; got != want {
		t.Fatalf("checkpoint failure mismatch: got %q want %q", got, want)
	}
}

func TestActionLiveReporterDoesNotExposeDebugCheckpointExtension(t *testing.T) {
	t.Parallel()

	var reporter observe.Reporter = newActionLiveReporter(&recordingSink{}, observe.NodeRef{
		Kind: observe.NodeKindAction,
		Path: "stage.main/call.login/act.submit/action",
	}, 4)

	if _, ok := reporter.(DebugCheckpointReporter); ok {
		t.Fatal("non-debug action reporter must not expose DebugCheckpointReporter")
	}
}

func (s *recordingSink) Publish(env observe.Envelope) uint64 {
	s.mu.Lock()
	s.envelopes = append(s.envelopes, cloneRecordedEnvelope(env))
	dropped := s.dropped
	s.mu.Unlock()

	return dropped
}

func (s *recordingSink) Snapshot() []observe.Envelope {
	s.mu.Lock()
	defer s.mu.Unlock()

	cloned := make([]observe.Envelope, len(s.envelopes))
	for i := range s.envelopes {
		cloned[i] = cloneRecordedEnvelope(s.envelopes[i])
	}

	return cloned
}

func cloneRecordedEnvelope(env observe.Envelope) observe.Envelope {
	return liveobserve.CloneEnvelope(env)
}
