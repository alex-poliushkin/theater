package liveobserve

import (
	"testing"

	"github.com/alex-poliushkin/theater/observe"
)

type recordingSink struct {
	dropped   uint64
	envelopes []observe.Envelope
}

func TestPublisherPublishesClonedEventsAndTracksSummary(t *testing.T) {
	t.Parallel()

	node := observe.NodeRef{Kind: observe.NodeKindAction, Path: "stage.main/call.login/act.submit/action"}
	sink := &recordingSink{}
	publisher := NewPublisher(sink, node, 4, 4)

	current := int64(1)
	total := int64(3)
	percent := 33.3
	progress := observe.Progress{
		Phase:   "run",
		Message: "starting",
		Current: &current,
		Total:   &total,
		Percent: &percent,
	}
	diagnostic := observe.Diagnostic{
		Message: "captured",
		Fields:  map[string]string{"source": "stdout"},
	}
	chunk := observe.LogChunk{
		Stream: "stdout",
		Data:   []byte("abcdef"),
	}

	publisher.Progress(progress)
	publisher.Diagnostic(diagnostic)
	publisher.LogChunk(chunk)

	*progress.Current = 99
	diagnostic.Fields["source"] = "stderr"
	chunk.Data[0] = 'z'

	envelopes := sink.Snapshot()
	if got, want := len(envelopes), 4; got != want {
		t.Fatalf("published envelope count mismatch: got %d want %d", got, want)
	}
	if got, want := envelopes[0].Progress.Message, "starting"; got != want {
		t.Fatalf("progress message mismatch: got %q want %q", got, want)
	}
	if got, want := *envelopes[0].Progress.Current, int64(1); got != want {
		t.Fatalf("progress current mismatch: got %d want %d", got, want)
	}
	if got, want := envelopes[1].Diagnostic.Fields["source"], "stdout"; got != want {
		t.Fatalf("diagnostic field mismatch: got %q want %q", got, want)
	}
	if got, want := string(envelopes[2].LogChunk.Data), "abcd"; got != want {
		t.Fatalf("first log chunk mismatch: got %q want %q", got, want)
	}
	if got, want := string(envelopes[3].LogChunk.Data), "ef"; got != want {
		t.Fatalf("second log chunk mismatch: got %q want %q", got, want)
	}

	summary := publisher.Snapshot()["stdout"]
	if got, want := summary.SizeBytes, int64(6); got != want {
		t.Fatalf("summary size mismatch: got %d want %d", got, want)
	}
	if got, want := string(summary.Tail), "cdef"; got != want {
		t.Fatalf("summary tail mismatch: got %q want %q", got, want)
	}
}

func TestPublisherCountsDroppedChunksAcrossChunkedPublish(t *testing.T) {
	t.Parallel()

	node := observe.NodeRef{Kind: observe.NodeKindAction, Path: "stage.main/call.login/act.submit/action"}
	sink := &recordingSink{dropped: 2}
	publisher := NewPublisher(sink, node, 4, 8)

	publisher.LogChunk(observe.LogChunk{
		Stream: "stderr",
		Data:   []byte("abcdefgh"),
	})

	summary := publisher.Snapshot()["stderr"]
	if got, want := summary.SizeBytes, int64(8); got != want {
		t.Fatalf("summary size mismatch: got %d want %d", got, want)
	}
	if got, want := summary.DroppedChunks, uint64(4); got != want {
		t.Fatalf("dropped chunks mismatch: got %d want %d", got, want)
	}
	if got, want := string(summary.Tail), "abcdefgh"; got != want {
		t.Fatalf("summary tail mismatch: got %q want %q", got, want)
	}
}

func (s *recordingSink) Publish(env observe.Envelope) uint64 {
	s.envelopes = append(s.envelopes, CloneEnvelope(env))
	return s.dropped
}

func (s *recordingSink) Snapshot() []observe.Envelope {
	cloned := make([]observe.Envelope, len(s.envelopes))
	for i := range s.envelopes {
		cloned[i] = CloneEnvelope(s.envelopes[i])
	}

	return cloned
}
