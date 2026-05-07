package liveobserve

import (
	"bytes"
	"sync"

	"github.com/alex-poliushkin/theater/observe"
)

const (
	DefaultLogChunkBytes  = 4 * 1024
	DefaultStreamTailSize = 4 * 1024
)

type StreamSummary struct {
	SizeBytes     int64
	Tail          []byte
	DroppedChunks uint64
}

type Publisher struct {
	sink       observe.Sink
	node       observe.NodeRef
	chunkBytes int
	tailLimit  int
	mu         sync.Mutex
	streams    map[string]*streamSummaryState
}

type streamSummaryState struct {
	sizeBytes     int64
	tail          []byte
	droppedChunks uint64
}

func NewPublisher(
	sink observe.Sink,
	node observe.NodeRef,
	chunkBytes int,
	tailLimit int,
) *Publisher {
	if chunkBytes <= 0 {
		chunkBytes = DefaultLogChunkBytes
	}
	if tailLimit <= 0 {
		tailLimit = DefaultStreamTailSize
	}

	return &Publisher{
		sink:       sink,
		node:       node,
		chunkBytes: chunkBytes,
		tailLimit:  tailLimit,
		streams:    make(map[string]*streamSummaryState),
	}
}

func (p *Publisher) Progress(progress observe.Progress) {
	if p == nil || p.sink == nil {
		return
	}

	p.sink.Publish(observe.Envelope{
		Kind:     observe.KindProgress,
		Node:     p.node,
		Progress: cloneProgress(progress),
	})
}

func (p *Publisher) Diagnostic(diagnostic observe.Diagnostic) {
	if p == nil || p.sink == nil {
		return
	}

	p.sink.Publish(observe.Envelope{
		Kind:       observe.KindDiagnostic,
		Node:       p.node,
		Diagnostic: cloneDiagnostic(diagnostic),
	})
}

func (p *Publisher) LogChunk(chunk observe.LogChunk) {
	if p == nil || chunk.Stream == "" || len(chunk.Data) == 0 {
		return
	}

	p.recordSummary(chunk.Stream, chunk.Data, 0)
	if p.sink == nil {
		return
	}

	for offset := 0; offset < len(chunk.Data); offset += p.chunkBytes {
		end := offset + p.chunkBytes
		if end > len(chunk.Data) {
			end = len(chunk.Data)
		}

		drops := p.sink.Publish(observe.Envelope{
			Kind: observe.KindLogChunk,
			Node: p.node,
			LogChunk: &observe.LogChunk{
				Stream: chunk.Stream,
				Data:   bytes.Clone(chunk.Data[offset:end]),
			},
		})
		if drops == 0 {
			continue
		}

		p.recordSummary(chunk.Stream, nil, drops)
	}
}

func (p *Publisher) Snapshot() map[string]StreamSummary {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.streams) == 0 {
		return nil
	}

	snapshots := make(map[string]StreamSummary, len(p.streams))
	for stream, summary := range p.streams {
		snapshots[stream] = StreamSummary{
			SizeBytes:     summary.sizeBytes,
			Tail:          bytes.Clone(summary.tail),
			DroppedChunks: summary.droppedChunks,
		}
	}

	return snapshots
}

func CloneEnvelope(env observe.Envelope) observe.Envelope {
	if env.SourceAt != nil {
		sourceAt := *env.SourceAt
		env.SourceAt = &sourceAt
	}
	if env.Transition != nil {
		transition := *env.Transition
		env.Transition = &transition
	}
	if env.Progress != nil {
		env.Progress = cloneProgress(*env.Progress)
	}
	if env.Diagnostic != nil {
		env.Diagnostic = cloneDiagnostic(*env.Diagnostic)
	}
	if env.LogChunk != nil {
		logChunk := *env.LogChunk
		logChunk.Data = bytes.Clone(env.LogChunk.Data)
		env.LogChunk = &logChunk
	}
	if env.Dropped != nil {
		dropped := *env.Dropped
		env.Dropped = &dropped
	}

	return env
}

func (p *Publisher) recordSummary(stream string, chunk []byte, drops uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	summary, ok := p.streams[stream]
	if !ok {
		summary = &streamSummaryState{}
		p.streams[stream] = summary
	}

	summary.sizeBytes += int64(len(chunk))
	summary.droppedChunks += drops
	summary.tail = appendTail(summary.tail, chunk, p.tailLimit)
}

func appendTail(existing, chunk []byte, limit int) []byte {
	if limit <= 0 || len(chunk) == 0 {
		return existing
	}

	if len(chunk) >= limit {
		return bytes.Clone(chunk[len(chunk)-limit:])
	}

	if len(existing)+len(chunk) <= limit {
		tail := make([]byte, 0, len(existing)+len(chunk))
		tail = append(tail, existing...)
		tail = append(tail, chunk...)
		return tail
	}

	drop := len(existing) + len(chunk) - limit
	tail := make([]byte, 0, limit)
	tail = append(tail, existing[drop:]...)
	tail = append(tail, chunk...)
	return tail
}

func cloneProgress(progress observe.Progress) *observe.Progress {
	cloned := progress
	if progress.Current != nil {
		current := *progress.Current
		cloned.Current = &current
	}
	if progress.Total != nil {
		total := *progress.Total
		cloned.Total = &total
	}
	if progress.Percent != nil {
		percent := *progress.Percent
		cloned.Percent = &percent
	}

	return &cloned
}

func cloneDiagnostic(diagnostic observe.Diagnostic) *observe.Diagnostic {
	cloned := diagnostic
	if len(diagnostic.Fields) != 0 {
		cloned.Fields = make(map[string]string, len(diagnostic.Fields))
		for key, value := range diagnostic.Fields {
			cloned.Fields[key] = value
		}
	}

	return &cloned
}
