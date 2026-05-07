package runobserve

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/alex-poliushkin/theater/internal/liveobserve"
	"github.com/alex-poliushkin/theater/observe"
)

const (
	DefaultQueueCapacity  = 64
	DefaultLogChunkBytes  = liveobserve.DefaultLogChunkBytes
	DefaultStreamTailSize = liveobserve.DefaultStreamTailSize
)

type StreamSummary = liveobserve.StreamSummary

type Bus struct {
	runID         string
	queueCap      int
	nextSeq       atomic.Uint64
	mu            sync.Mutex
	nextSubID     uint64
	subscriptions map[uint64]*subscriptionState
}

type Publisher struct {
	publisher *liveobserve.Publisher
}

func NewBus(runID string, queueCap int) *Bus {
	if queueCap <= 0 {
		queueCap = DefaultQueueCapacity
	}

	return &Bus{
		runID:         runID,
		queueCap:      queueCap,
		subscriptions: make(map[uint64]*subscriptionState),
	}
}

func NewPublisher(sink observe.Sink, node observe.NodeRef, tailLimit int) *Publisher {
	return &Publisher{
		publisher: liveobserve.NewPublisher(sink, node, DefaultLogChunkBytes, tailLimit),
	}
}

func (b *Bus) Publish(env observe.Envelope) uint64 {
	if b == nil {
		return 0
	}

	prepared := b.prepareEnvelope(env)
	var dropped uint64

	b.mu.Lock()
	defer b.mu.Unlock()

	for _, subscription := range b.subscriptions {
		sentQueued := b.flushQueued(subscription)
		if len(subscription.queue) != 0 {
			b.enqueueDropped(subscription, prepared.Node)
			dropped++
			continue
		}

		if b.deliver(subscription.events, prepared) {
			continue
		}

		if sentQueued && b.enqueuePending(subscription, prepared) {
			continue
		}

		b.enqueueDropped(subscription, prepared.Node)
		dropped++
	}

	return dropped
}

func (b *Bus) Subscribe() observe.Subscription {
	if b == nil {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.nextSubID++
	id := b.nextSubID
	subscription := &subscription{
		id:     id,
		bus:    b,
		events: make(chan observe.Envelope, b.queueCap),
	}
	b.subscriptions[id] = &subscriptionState{events: subscription.events}
	return subscription
}

func (p *Publisher) Progress(progress observe.Progress) {
	if p == nil {
		return
	}

	p.publisher.Progress(progress)
}

func (p *Publisher) Diagnostic(diagnostic observe.Diagnostic) {
	if p == nil {
		return
	}

	p.publisher.Diagnostic(diagnostic)
}

func (p *Publisher) LogChunk(chunk observe.LogChunk) {
	if p == nil {
		return
	}

	p.publisher.LogChunk(chunk)
}

func (p *Publisher) Snapshot() map[string]StreamSummary {
	if p == nil {
		return nil
	}

	return p.publisher.Snapshot()
}

type subscription struct {
	id     uint64
	bus    *Bus
	events chan observe.Envelope
	once   sync.Once
}

func (s *subscription) Close() {
	if s == nil {
		return
	}

	s.once.Do(func() {
		if s.bus == nil {
			close(s.events)
			return
		}

		s.bus.mu.Lock()
		state, ok := s.bus.subscriptions[s.id]
		if ok {
			delete(s.bus.subscriptions, s.id)
			close(state.events)
		}
		s.bus.mu.Unlock()
	})
}

func (s *subscription) Events() <-chan observe.Envelope {
	if s == nil {
		return nil
	}

	return s.events
}

type subscriptionState struct {
	events chan observe.Envelope
	queue  []observe.Envelope
}

func (b *Bus) flushQueued(subscription *subscriptionState) bool {
	sentAny := false
	for len(subscription.queue) != 0 {
		if !b.deliver(subscription.events, subscription.queue[0]) {
			return sentAny
		}

		subscription.queue = subscription.queue[1:]
		sentAny = true
	}

	return sentAny
}

func (b *Bus) enqueuePending(subscription *subscriptionState, env observe.Envelope) bool {
	for i := range subscription.queue {
		if subscription.queue[i].Kind != observe.KindDropped {
			return false
		}
	}

	subscription.queue = append(subscription.queue, env)
	return true
}

func (b *Bus) enqueueDropped(subscription *subscriptionState, node observe.NodeRef) {
	if n := len(subscription.queue); n > 0 {
		last := &subscription.queue[n-1]
		if last.Kind == observe.KindDropped && last.Dropped != nil && last.Node == node {
			last.Dropped.Count++
			return
		}
	}

	subscription.queue = append(subscription.queue, observe.Envelope{
		RunID: b.runID,
		Kind:  observe.KindDropped,
		Node:  node,
		Dropped: &observe.DroppedNotice{
			Count: 1,
		},
	})
}

func (b *Bus) prepareEnvelope(env observe.Envelope) observe.Envelope {
	env.RunID = b.runID
	if env.ObservedAt.IsZero() {
		env.ObservedAt = time.Now().UTC()
	}
	env.Seq = 0

	return liveobserve.CloneEnvelope(env)
}

func (b *Bus) deliver(events chan observe.Envelope, env observe.Envelope) bool {
	delivered := env
	delivered.RunID = b.runID
	if delivered.ObservedAt.IsZero() {
		delivered.ObservedAt = time.Now().UTC()
	}
	delivered.Seq = b.nextSeq.Add(1)
	return trySend(events, delivered)
}

func trySend(events chan observe.Envelope, env observe.Envelope) bool {
	select {
	case events <- env:
		return true
	default:
		return false
	}
}
