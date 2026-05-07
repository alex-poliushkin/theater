package theater

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/alex-poliushkin/theater/internal/liveobserve"
	"github.com/alex-poliushkin/theater/observe"
)

const (
	debugLiveRecentLimit      = 32
	debugLiveTextPreviewLimit = 512
	debugSchedulerPathLimit   = 8
)

type debugRecentSnapshot struct {
	Items   []debugEventSummary
	Omitted int
}

type debugEventSummary struct {
	Seq     uint64
	Kind    string
	Path    string
	Attempt int
	Text    string
}

type debugSchedulerSummary struct {
	FocusedLane string
	Active      int
	Ready       int
	Blocked     int
	ReadyPaths  []string
}

type debugLiveBridge struct {
	mu           sync.Mutex
	lanes        map[string]*debugRecentBuffer
	limit        int
	subscription observe.Subscription
	subDone      chan struct{}
	closed       bool
}

type debugRecentBuffer struct {
	items   []debugEventSummary
	omitted int
}

type debugLiveSink struct {
	inner  observe.Sink
	bridge *debugLiveBridge
}

type debugSchedulerState struct {
	mu         sync.Mutex
	active     int
	ready      int
	blocked    int
	readyPaths []string
}

type debugLiveSubscribableSink interface {
	observe.Sink
	Subscribe() observe.Subscription
}

func newDebugLiveBridge(limit int) *debugLiveBridge {
	if limit <= 0 {
		limit = debugLiveRecentLimit
	}

	return &debugLiveBridge{
		lanes: make(map[string]*debugRecentBuffer),
		limit: limit,
	}
}

func newDebugSchedulerState() *debugSchedulerState {
	return &debugSchedulerState{}
}

func (d *debugRuntime) ensureLiveBridge() {
	if d == nil {
		return
	}
	if d.liveBridge == nil {
		d.liveBridge = newDebugLiveBridge(debugLiveRecentLimit)
	}
	if d.scheduler == nil {
		d.scheduler = newDebugSchedulerState()
	}
}

func (d *debugRuntime) prepareLiveSink(sink observe.Sink) observe.Sink {
	if d == nil {
		return sink
	}

	d.ensureLiveBridge()
	if bus, ok := sink.(debugLiveSubscribableSink); ok {
		d.liveBridge.attachDroppedSubscription(bus.Subscribe())
	}

	return debugLiveSink{
		inner:  sink,
		bridge: d.liveBridge,
	}
}

func (d *debugRuntime) prepareRun(live observe.Sink) (observe.Sink, func(context.Context) error, error) {
	if d == nil {
		return live, func(context.Context) error { return nil }, nil
	}

	d.resetPerRunState()
	if d.controller != nil {
		d.controller.Reset()
	}
	if d.artifactPath != "" {
		sink, err := openDebugArtifactSink(d.artifactPath)
		if err != nil {
			return nil, nil, err
		}
		d.artifactSink = sink
	}

	return d.prepareLiveSink(live), d.close, nil
}

func (d *debugRuntime) close(ctx context.Context) error {
	if d == nil {
		return nil
	}

	var closeErr error

	if d.liveBridge != nil {
		d.liveBridge.Close()
	}
	if d.promptSession != nil {
		d.promptSession.Close()
	}
	if d.artifactSink != nil {
		if d.controller != nil && d.controller.mode == debugModeInteractive {
			_, closeErr = d.artifactSink.WriteSummary(ctx, debugArtifactSessionSummary{
				Records: d.artifactSink.RecordCount(),
			})
		}
		closeErr = errors.Join(closeErr, d.artifactSink.Close())
	}
	d.resetPerRunState()

	return closeErr
}

func (d *debugRuntime) storeScheduler(active, ready, blocked int, readyPaths []string) {
	if d == nil {
		return
	}

	d.ensureLiveBridge()
	d.scheduler.Store(active, ready, blocked, readyPaths)
}

func (d *debugRuntime) resetPerRunState() {
	if d == nil {
		return
	}

	d.storeDurableEventSeq(0)
	d.artifactSink = nil
	d.liveBridge = nil
	d.scheduler = nil
	d.stateRecorder = nil
}

func (b *debugLiveBridge) Snapshot(lane string) debugRecentSnapshot {
	if b == nil {
		return debugRecentSnapshot{}
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	buffer, ok := b.lanes[lane]
	if !ok || len(buffer.items) == 0 {
		return debugRecentSnapshot{}
	}

	snapshot := debugRecentSnapshot{
		Items:   make([]debugEventSummary, len(buffer.items)),
		Omitted: buffer.omitted,
	}
	copy(snapshot.Items, buffer.items)
	return snapshot
}

func (b *debugLiveBridge) Close() {
	if b == nil {
		return
	}

	b.mu.Lock()
	if b.closed {
		done := b.subDone
		b.mu.Unlock()
		if done != nil {
			<-done
		}
		return
	}

	subscription := b.subscription
	done := b.subDone
	b.subscription = nil
	b.subDone = nil
	b.closed = true
	b.mu.Unlock()

	if subscription != nil {
		subscription.Close()
	}
	if done != nil {
		<-done
	}
}

func (b *debugLiveBridge) attachDroppedSubscription(subscription observe.Subscription) {
	if b == nil || subscription == nil {
		return
	}

	b.mu.Lock()
	if b.closed || b.subscription != nil {
		b.mu.Unlock()
		subscription.Close()
		return
	}

	done := make(chan struct{})
	b.subscription = subscription
	b.subDone = done
	b.mu.Unlock()

	go func() {
		defer close(done)
		for env := range subscription.Events() {
			if env.Kind != observe.KindDropped {
				continue
			}

			b.record(env)
		}
	}()
}

func (b *debugLiveBridge) record(env observe.Envelope) {
	if b == nil {
		return
	}

	summary := debugSummarizeEnvelope(env)
	lane := debugRecentLane(env.Node)
	if lane == "" {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	buffer, ok := b.lanes[lane]
	if !ok {
		buffer = &debugRecentBuffer{}
		b.lanes[lane] = buffer
	}

	buffer.items = append(buffer.items, summary)
	if len(buffer.items) > b.limit {
		buffer.items = append([]debugEventSummary(nil), buffer.items[len(buffer.items)-b.limit:]...)
		buffer.omitted++
	}
}

func (s debugLiveSink) Publish(env observe.Envelope) uint64 {
	if s.bridge != nil {
		s.bridge.record(liveobserve.CloneEnvelope(env))
	}
	if s.inner == nil {
		return 0
	}

	return s.inner.Publish(env)
}

func (s *debugSchedulerState) Store(active, ready, blocked int, readyPaths []string) {
	if s == nil {
		return
	}

	cloned := cloneDebugReadyPaths(readyPaths)
	if len(cloned) > debugSchedulerPathLimit {
		cloned = cloned[:debugSchedulerPathLimit]
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.active = active
	s.ready = ready
	s.blocked = blocked
	s.readyPaths = cloned
}

func (s *debugSchedulerState) Snapshot(focusedLane string) debugSchedulerSummary {
	if s == nil {
		return debugSchedulerSummary{FocusedLane: focusedLane}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return debugSchedulerSummary{
		FocusedLane: focusedLane,
		Active:      s.active,
		Ready:       s.ready,
		Blocked:     s.blocked,
		ReadyPaths:  cloneDebugReadyPaths(s.readyPaths),
	}
}

func debugRecentLane(node observe.NodeRef) string {
	if node.ScenarioCallID != "" {
		return node.ScenarioCallID
	}
	if node.Path != "" {
		return node.Path
	}

	return node.StageID
}

func debugSummarizeEnvelope(env observe.Envelope) debugEventSummary {
	return debugEventSummary{
		Seq:     env.Seq,
		Kind:    string(env.Kind),
		Path:    env.Node.Path,
		Attempt: env.Node.Attempt,
		Text:    debugEnvelopeText(env),
	}
}

func debugEnvelopeText(env observe.Envelope) string {
	var text string

	switch env.Kind {
	case observe.KindTransition:
		text = debugTransitionText(env)
	case observe.KindProgress:
		text = debugProgressText(env.Progress)
	case observe.KindDiagnostic:
		if env.Diagnostic != nil {
			text = env.Diagnostic.Message
		}
	case observe.KindLogChunk:
		if env.LogChunk != nil {
			text = env.LogChunk.Stream + ": " + sanitizeStreamPreviewText(env.LogChunk.Data)
		}
	case observe.KindDropped:
		if env.Dropped != nil {
			text = fmt.Sprintf("dropped %d", env.Dropped.Count)
		}
	}

	text, _ = truncatePreviewMiddle(text, debugLiveTextPreviewLimit)
	return text
}

func debugTransitionText(env observe.Envelope) string {
	if env.Transition == nil {
		return ""
	}

	text := env.Transition.Status
	if env.Transition.FailureSummary != "" {
		text += ": " + env.Transition.FailureSummary
	}

	return text
}

func debugProgressText(progress *observe.Progress) string {
	if progress == nil {
		return ""
	}
	if progress.Message != "" {
		return progress.Message
	}

	return progress.Phase
}

func cloneDebugReadyPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}

	cloned := make([]string, len(paths))
	copy(cloned, paths)
	return cloned
}

func debugSchedulerPaths(scheduled []scheduledScenarioRun) []string {
	if len(scheduled) == 0 {
		return nil
	}

	paths := make([]string, 0, len(scheduled))
	for i := range scheduled {
		paths = append(paths, scheduled[i].call.Path)
	}

	return paths
}

func (r *stageRunner) updateDebugScheduler(ready, scheduled []scheduledScenarioRun) {
	if r == nil || r.executor == nil || r.executor.debug == nil {
		return
	}

	pending := pendingScenarioCalls(r.stage, r.planner.states)
	blocked := len(pending) - len(ready)
	if blocked < 0 {
		blocked = 0
	}

	r.executor.debug.storeScheduler(
		len(scheduled),
		len(ready),
		blocked,
		debugSchedulerPaths(ready),
	)
}

func (d *debugRuntime) enrichBoundaryState(ctx context.Context, state debugBoundaryState) (debugBoundaryState, error) {
	if d == nil {
		return state, nil
	}
	if d.stateRecorder != nil {
		snapshot, err := d.stateRecorder.Snapshot(ctx)
		if err != nil {
			var panicErr boundaryPanicError
			if errors.As(err, &panicErr) {
				return state, newContainedDebugBoundaryError(state, "debug state snapshotter panicked", err)
			}

			return state, newContainedDebugBoundaryError(state, "debug state snapshotter failed", err)
		}

		state.State = snapshot
	}
	if d.liveBridge != nil {
		state.Recent = d.liveBridge.Snapshot(state.Ref.ScenarioCallID)
	}
	if d.scheduler != nil {
		state.Scheduler = d.scheduler.Snapshot(state.Ref.ScenarioPath)
	}

	return state, nil
}
