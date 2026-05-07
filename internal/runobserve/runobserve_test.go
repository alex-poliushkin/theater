package runobserve

import (
	"testing"

	"github.com/alex-poliushkin/theater/observe"
)

func TestBusPublishesMonotonicEnvelopesToSubscribers(t *testing.T) {
	t.Parallel()

	bus := NewBus("run-1", 2)
	subscription := bus.Subscribe()

	bus.Publish(observe.Envelope{
		Kind:       observe.KindTransition,
		Node:       observe.NodeRef{Kind: observe.NodeKindStage, Path: "stage.main"},
		Transition: &observe.Transition{EventKind: "stage.running", Status: "running"},
	})
	bus.Publish(observe.Envelope{
		Kind:       observe.KindTransition,
		Node:       observe.NodeRef{Kind: observe.NodeKindStage, Path: "stage.main"},
		Transition: &observe.Transition{EventKind: "stage.finished", Status: "passed"},
	})
	subscription.Close()

	var envelopes []observe.Envelope
	for envelope := range subscription.Events() {
		envelopes = append(envelopes, envelope)
	}

	if got, want := len(envelopes), 2; got != want {
		t.Fatalf("envelope count mismatch: got %d want %d", got, want)
	}

	if got, want := envelopes[0].RunID, "run-1"; got != want {
		t.Fatalf("first run id mismatch: got %q want %q", got, want)
	}

	if got, want := envelopes[0].Seq, uint64(1); got != want {
		t.Fatalf("first sequence mismatch: got %d want %d", got, want)
	}

	if got, want := envelopes[1].Seq, uint64(2); got != want {
		t.Fatalf("second sequence mismatch: got %d want %d", got, want)
	}

	if envelopes[0].ObservedAt.IsZero() || envelopes[1].ObservedAt.IsZero() {
		t.Fatal("observed timestamps must be populated")
	}
}

func TestBusPublishesDroppedNoticeBeforeNextDeliveredEvent(t *testing.T) {
	t.Parallel()

	bus := NewBus("run-1", 1)
	subscription := bus.Subscribe()

	bus.Publish(observe.Envelope{
		Kind:     observe.KindLogChunk,
		Node:     observe.NodeRef{Kind: observe.NodeKindAction, Path: "stage.main/call.run/act.run/action"},
		LogChunk: &observe.LogChunk{Stream: "stdout", Data: []byte("first")},
	})
	bus.Publish(observe.Envelope{
		Kind:     observe.KindLogChunk,
		Node:     observe.NodeRef{Kind: observe.NodeKindAction, Path: "stage.main/call.run/act.run/action"},
		LogChunk: &observe.LogChunk{Stream: "stdout", Data: []byte("dropped")},
	})

	first := <-subscription.Events()
	if got, want := string(first.LogChunk.Data), "first"; got != want {
		t.Fatalf("first chunk mismatch: got %q want %q", got, want)
	}

	bus.Publish(observe.Envelope{
		Kind:     observe.KindLogChunk,
		Node:     observe.NodeRef{Kind: observe.NodeKindAction, Path: "stage.main/call.run/act.run/action"},
		LogChunk: &observe.LogChunk{Stream: "stdout", Data: []byte("third")},
	})

	second := <-subscription.Events()
	if got, want := second.Kind, observe.KindDropped; got != want {
		t.Fatalf("second envelope kind mismatch: got %q want %q", got, want)
	}
	if second.Dropped == nil || second.Dropped.Count != 1 {
		t.Fatalf("dropped count mismatch: got %#v want count=1", second.Dropped)
	}
	if second.Seq <= first.Seq {
		t.Fatalf("dropped notice sequence must increase: first=%d second=%d", first.Seq, second.Seq)
	}

	bus.Publish(observe.Envelope{
		Kind:     observe.KindLogChunk,
		Node:     observe.NodeRef{Kind: observe.NodeKindAction, Path: "stage.main/call.run/act.run/action"},
		LogChunk: &observe.LogChunk{Stream: "stdout", Data: []byte("fourth")},
	})

	third := <-subscription.Events()
	if got, want := third.Kind, observe.KindLogChunk; got != want {
		t.Fatalf("third envelope kind mismatch: got %q want %q", got, want)
	}
	if got, want := string(third.LogChunk.Data), "third"; got != want {
		t.Fatalf("third chunk mismatch: got %q want %q", got, want)
	}
	if third.Seq <= second.Seq {
		t.Fatalf("delivery sequence must stay monotonic: second=%d third=%d", second.Seq, third.Seq)
	}

	subscription.Close()
}

func TestBusAttributesDroppedNoticeToDroppedSourceNode(t *testing.T) {
	t.Parallel()

	bus := NewBus("run-1", 1)
	subscription := bus.Subscribe()

	firstNode := observe.NodeRef{Kind: observe.NodeKindAction, Path: "stage.main/call.first/act.run/action"}
	secondNode := observe.NodeRef{Kind: observe.NodeKindAction, Path: "stage.main/call.second/act.run/action"}

	bus.Publish(observe.Envelope{
		Kind:     observe.KindLogChunk,
		Node:     firstNode,
		LogChunk: &observe.LogChunk{Stream: "stdout", Data: []byte("first")},
	})
	bus.Publish(observe.Envelope{
		Kind:     observe.KindLogChunk,
		Node:     firstNode,
		LogChunk: &observe.LogChunk{Stream: "stdout", Data: []byte("dropped")},
	})

	first := <-subscription.Events()
	if got, want := first.Node.Path, firstNode.Path; got != want {
		t.Fatalf("first envelope node mismatch: got %q want %q", got, want)
	}

	bus.Publish(observe.Envelope{
		Kind:     observe.KindLogChunk,
		Node:     secondNode,
		LogChunk: &observe.LogChunk{Stream: "stdout", Data: []byte("trigger")},
	})

	second := <-subscription.Events()
	if got, want := second.Kind, observe.KindDropped; got != want {
		t.Fatalf("second envelope kind mismatch: got %q want %q", got, want)
	}
	if second.Dropped == nil || second.Dropped.Count != 1 {
		t.Fatalf("dropped count mismatch: got %#v want count=1", second.Dropped)
	}
	if got, want := second.Node.Path, firstNode.Path; got != want {
		t.Fatalf("dropped notice node mismatch: got %q want %q", got, want)
	}

	subscription.Close()
}

func TestBusKeepsDroppedBucketsPerNodeInFIFOOrder(t *testing.T) {
	t.Parallel()

	bus := NewBus("run-1", 1)
	subscription := bus.Subscribe()

	seedNode := observe.NodeRef{Kind: observe.NodeKindAction, Path: "stage.main/call.seed/act.run/action"}
	firstDroppedNode := observe.NodeRef{Kind: observe.NodeKindAction, Path: "stage.main/call.first/act.run/action"}
	secondDroppedNode := observe.NodeRef{Kind: observe.NodeKindAction, Path: "stage.main/call.second/act.run/action"}
	triggerNode := observe.NodeRef{Kind: observe.NodeKindAction, Path: "stage.main/call.trigger/act.run/action"}

	bus.Publish(observe.Envelope{
		Kind:     observe.KindLogChunk,
		Node:     seedNode,
		LogChunk: &observe.LogChunk{Stream: "stdout", Data: []byte("seed")},
	})
	bus.Publish(observe.Envelope{
		Kind:     observe.KindLogChunk,
		Node:     firstDroppedNode,
		LogChunk: &observe.LogChunk{Stream: "stdout", Data: []byte("drop-1")},
	})
	bus.Publish(observe.Envelope{
		Kind:     observe.KindLogChunk,
		Node:     secondDroppedNode,
		LogChunk: &observe.LogChunk{Stream: "stdout", Data: []byte("drop-2")},
	})

	<-subscription.Events()

	bus.Publish(observe.Envelope{
		Kind:     observe.KindLogChunk,
		Node:     triggerNode,
		LogChunk: &observe.LogChunk{Stream: "stdout", Data: []byte("trigger-1")},
	})
	firstNotice := <-subscription.Events()
	if got, want := firstNotice.Kind, observe.KindDropped; got != want {
		t.Fatalf("first notice kind mismatch: got %q want %q", got, want)
	}
	if got, want := firstNotice.Node.Path, firstDroppedNode.Path; got != want {
		t.Fatalf("first notice node mismatch: got %q want %q", got, want)
	}
	if firstNotice.Dropped == nil || firstNotice.Dropped.Count != 1 {
		t.Fatalf("first notice dropped count mismatch: got %#v want count=1", firstNotice.Dropped)
	}

	bus.Publish(observe.Envelope{
		Kind:     observe.KindLogChunk,
		Node:     triggerNode,
		LogChunk: &observe.LogChunk{Stream: "stdout", Data: []byte("trigger-2")},
	})
	secondNotice := <-subscription.Events()
	if got, want := secondNotice.Kind, observe.KindDropped; got != want {
		t.Fatalf("second notice kind mismatch: got %q want %q", got, want)
	}
	if got, want := secondNotice.Node.Path, secondDroppedNode.Path; got != want {
		t.Fatalf("second notice node mismatch: got %q want %q", got, want)
	}
	if secondNotice.Dropped == nil || secondNotice.Dropped.Count != 1 {
		t.Fatalf("second notice dropped count mismatch: got %#v want count=1", secondNotice.Dropped)
	}

	subscription.Close()
}

func TestPublisherTracksTailAndDroppedChunks(t *testing.T) {
	t.Parallel()

	bus := NewBus("run-1", 1)
	slow := bus.Subscribe()
	publisher := NewPublisher(bus, observe.NodeRef{Kind: observe.NodeKindAction, Path: "stage.main/call.run/act.run/action"}, 4)

	publisher.LogChunk(observe.LogChunk{Stream: "stdout", Data: []byte("ab")})
	publisher.LogChunk(observe.LogChunk{Stream: "stdout", Data: []byte("cdef")})
	publisher.LogChunk(observe.LogChunk{Stream: "stdout", Data: []byte("gh")})

	snapshot := publisher.Snapshot()
	summary, ok := snapshot["stdout"]
	if !ok {
		t.Fatal("stdout summary must be present")
	}

	if got, want := summary.SizeBytes, int64(8); got != want {
		t.Fatalf("size mismatch: got %d want %d", got, want)
	}

	if got, want := string(summary.Tail), "efgh"; got != want {
		t.Fatalf("tail mismatch: got %q want %q", got, want)
	}

	if summary.DroppedChunks == 0 {
		t.Fatal("publisher must record dropped live chunks when subscriber backpressure occurs")
	}

	slow.Close()
}
