package theater

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alex-poliushkin/theater/observe"
)

type recordingEventRecorder struct {
	mu     sync.Mutex
	events []Event
}

func (r *recordingEventRecorder) Record(event Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = append(r.events, event)
	return nil
}

func (r *recordingEventRecorder) Snapshot() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]Event(nil), r.events...)
}

func TestMirroredEnvelopeFromEventIgnoresExpectationEvents(t *testing.T) {
	t.Parallel()

	_, ok := mirroredEnvelopeFromEvent(Event{
		Kind:         EventKindExpectationFinished,
		StagePath:    "stage.main",
		ScenarioPath: "stage.main/call.login",
		Path:         "stage.main/call.login/act.submit/expectation.token",
		Attempt:      1,
		ScenarioSeq:  1,
		Status:       StatusPassed,
	})
	if ok {
		t.Fatal("expectation events must not be mirrored")
	}
}

func TestMirroredEnvelopeFromEventMirrorsScenarioLogsAsLogChunks(t *testing.T) {
	t.Parallel()

	event := Event{
		Kind:           EventKindLogEmitted,
		StageID:        "main",
		StagePath:      "stage.main",
		ScenarioID:     "login",
		ScenarioCallID: "login-user",
		ScenarioPath:   "stage.main/call.login-user",
		Path:           "stage.main/call.login-user/act.submit/log.response",
		Attempt:        2,
		ScenarioSeq:    1,
		Status:         StatusPassed,
		Log: &LogRecord{
			ID:             "response",
			Path:           "stage.main/call.login-user/act.submit/log.response",
			StageID:        "main",
			ScenarioID:     "login",
			ScenarioCallID: "login-user",
			ScenarioPath:   "stage.main/call.login-user",
			ActID:          "submit",
			Attempt:        2,
			ScenarioSeq:    1,
			Status:         LogStatusEmitted,
			Preview:        &Preview{Kind: "string", Text: "created user", Truncated: true},
			Address: &NodeAddress{
				ScenarioCallPath: "stage.main/call.login-user",
				ActID:            "submit",
				Kind:             NodeKindLog,
				NodeRef:          "response",
				Phase:            "log.evaluate",
				AttemptIndex:     2,
			},
			Truncated: true,
		},
	}

	envelope, ok := mirroredEnvelopeFromEvent(event)
	if !ok {
		t.Fatal("scenario log event must be mirrored")
	}
	if got, want := envelope.Kind, observe.KindLogChunk; got != want {
		t.Fatalf("envelope kind mismatch: got %q want %q", got, want)
	}
	if !envelope.DurableMirror {
		t.Fatal("scenario log mirror must be marked as durable mirror")
	}
	if got, want := envelope.Node.Kind, observe.NodeKindLog; got != want {
		t.Fatalf("node kind mismatch: got %q want %q", got, want)
	}
	if got, want := envelope.Node.Path, event.Path; got != want {
		t.Fatalf("node path mismatch: got %q want %q", got, want)
	}
	if got, want := envelope.Node.Attempt, 2; got != want {
		t.Fatalf("node attempt mismatch: got %d want %d", got, want)
	}
	if envelope.LogChunk == nil {
		t.Fatal("scenario log mirror must carry a log chunk")
	}
	if got, want := envelope.LogChunk.Stream, liveScenarioLogStream; got != want {
		t.Fatalf("stream mismatch: got %q want %q", got, want)
	}
	if got, want := string(envelope.LogChunk.Data), "log response: created user [truncated]\n"; got != want {
		t.Fatalf("log chunk data mismatch: got %q want %q", got, want)
	}
}

func TestMirroredEnvelopeFromEventDoesNotMirrorLimitDroppedScenarioLogs(t *testing.T) {
	t.Parallel()

	_, ok := mirroredEnvelopeFromEvent(Event{
		Kind:   EventKindLogEmitted,
		Status: StatusPassed,
		Log: &LogRecord{
			ID:      "response",
			Status:  LogStatusOmitted,
			Dropped: true,
		},
	})
	if ok {
		t.Fatal("limit-dropped scenario logs must not be mirrored to live output")
	}
}

func TestMirroredEnvelopeFromEventUsesReportSafeScenarioLogText(t *testing.T) {
	t.Parallel()

	envelope, ok := mirroredEnvelopeFromEvent(Event{
		Kind:   EventKindLogEmitted,
		Status: StatusFailed,
		Path:   "stage.main/call.login-user/act.submit/log.token",
		Log: &LogRecord{
			ID:     "token",
			Status: LogStatusError,
			Failure: &Failure{
				Summary: "field token is missing",
				Cause:   errors.New("secret-token"),
			},
		},
	})
	if !ok {
		t.Fatal("scenario log error must be mirrored")
	}
	if envelope.LogChunk == nil {
		t.Fatal("scenario log error must carry a log chunk")
	}
	if got, want := string(envelope.LogChunk.Data), "log token error: field token is missing\n"; got != want {
		t.Fatalf("log chunk data mismatch: got %q want %q", got, want)
	}
	if strings.Contains(string(envelope.LogChunk.Data), "secret-token") {
		t.Fatalf("live scenario log must not include failure cause text: %q", string(envelope.LogChunk.Data))
	}
}

func TestStageEventSinkMirrorsRecordedOrder(t *testing.T) {
	t.Parallel()

	live := &recordingSink{}
	recorder := &recordingEventRecorder{}
	sink := newStageEventSink(runDocumentIdentity{}, live, recorder)

	events := []Event{
		{
			Kind:      EventKindStageRunning,
			StageID:   "main",
			StagePath: "stage.main",
			Path:      "stage.main",
			Attempt:   1,
			Status:    StatusRunning,
		},
		{
			Kind:           EventKindScenarioRunning,
			StageID:        "main",
			StagePath:      "stage.main",
			ScenarioID:     "login",
			ScenarioCallID: "login-user",
			ScenarioPath:   "stage.main/call.login-user",
			Path:           "stage.main/call.login-user",
			Attempt:        1,
			ScenarioSeq:    1,
			Status:         StatusRunning,
		},
		{
			Kind:           EventKindActionRunning,
			StageID:        "main",
			StagePath:      "stage.main",
			ScenarioID:     "login",
			ScenarioCallID: "login-user",
			ScenarioPath:   "stage.main/call.login-user",
			Path:           "stage.main/call.login-user/act.submit/action",
			Attempt:        1,
			ScenarioSeq:    1,
			Status:         StatusRunning,
		},
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range events {
		wg.Add(1)
		go func(event Event) {
			defer wg.Done()
			<-start
			if err := sink.Record(event); err != nil {
				t.Errorf("record event failed: %v", err)
			}
		}(events[i])
	}

	close(start)
	wg.Wait()

	mirrored := live.Snapshot()
	recorded := recorder.Snapshot()
	if got, want := len(mirrored), len(recorded); got != want {
		t.Fatalf("mirrored count mismatch: got %d want %d", got, want)
	}

	for i := range recorded {
		if got, want := mirrored[i].Transition.EventKind, recorded[i].Kind; got != want {
			t.Fatalf("mirrored[%d] kind mismatch: got %q want %q", i, got, want)
		}
		if got, want := mirrored[i].Node.Path, recorded[i].Path; got != want {
			t.Fatalf("mirrored[%d] path mismatch: got %q want %q", i, got, want)
		}
		if got, want := mirrored[i].Kind, observe.KindTransition; got != want {
			t.Fatalf("mirrored[%d] kind mismatch: got %q want %q", i, got, want)
		}
	}
}

func TestStageEventSinkSerializesObserverCallbacksInSinkOrder(t *testing.T) {
	t.Parallel()

	first := Event{
		Kind:      EventKindStageRunning,
		StageID:   "main",
		StagePath: "stage.main",
		Path:      "stage.main",
		Attempt:   1,
		Status:    StatusRunning,
	}
	second := Event{
		Kind:           EventKindScenarioRunning,
		StageID:        "main",
		StagePath:      "stage.main",
		ScenarioID:     "login",
		ScenarioCallID: "login-user",
		ScenarioPath:   "stage.main/call.login-user",
		Path:           "stage.main/call.login-user",
		Attempt:        1,
		ScenarioSeq:    1,
		Status:         StatusRunning,
	}

	live := &recordingSink{}
	recorder := newBlockingOrderRecorder(first.Path)
	sink := newStageEventSink(runDocumentIdentity{}, live, recorder)

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- sink.Record(first)
	}()

	if got := recorder.waitEntered(t); got != first.Path {
		t.Fatalf("first callback path mismatch: got %q want %q", got, first.Path)
	}

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- sink.Record(second)
	}()

	select {
	case err := <-secondDone:
		t.Fatalf("second record returned before first callback completed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	recorder.ReleaseFirst()

	for _, result := range []struct {
		name string
		done <-chan error
	}{
		{name: "first", done: firstDone},
		{name: "second", done: secondDone},
	} {
		select {
		case err := <-result.done:
			if err != nil {
				t.Fatalf("%s record failed: %v", result.name, err)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s record did not finish", result.name)
		}
	}

	recorded := recorder.Snapshot()
	if got, want := eventPaths(recorded), []string{first.Path, second.Path}; !equalStrings(got, want) {
		t.Fatalf("recorded callback order mismatch: got %v want %v", got, want)
	}

	mirrored := live.Snapshot()
	if got, want := mirroredPaths(mirrored), []string{first.Path, second.Path}; !equalStrings(got, want) {
		t.Fatalf("mirrored callback order mismatch: got %v want %v", got, want)
	}
}

type blockingOrderRecorder struct {
	firstPath    string
	enterOnce    sync.Once
	firstEntered chan string
	releaseFirst chan struct{}

	mu     sync.Mutex
	events []Event
}

func newBlockingOrderRecorder(firstPath string) *blockingOrderRecorder {
	return &blockingOrderRecorder{
		firstPath:    firstPath,
		firstEntered: make(chan string, 1),
		releaseFirst: make(chan struct{}),
	}
}

func (r *blockingOrderRecorder) Record(event Event) error {
	if event.Path == r.firstPath {
		r.enterOnce.Do(func() {
			r.firstEntered <- event.Path
		})
		<-r.releaseFirst
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = append(r.events, event)
	return nil
}

func (r *blockingOrderRecorder) ReleaseFirst() {
	close(r.releaseFirst)
}

func (r *blockingOrderRecorder) Snapshot() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]Event(nil), r.events...)
}

func (r *blockingOrderRecorder) waitEntered(t *testing.T) string {
	t.Helper()

	select {
	case path := <-r.firstEntered:
		return path
	case <-time.After(time.Second):
		t.Fatal("first recorder callback did not start")
		return ""
	}
}

func eventPaths(events []Event) []string {
	paths := make([]string, 0, len(events))
	for i := range events {
		paths = append(paths, events[i].Path)
	}
	return paths
}

func mirroredPaths(envelopes []observe.Envelope) []string {
	paths := make([]string, 0, len(envelopes))
	for i := range envelopes {
		paths = append(paths, envelopes[i].Node.Path)
	}
	return paths
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}

	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}

	return true
}
