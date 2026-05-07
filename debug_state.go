package theater

import (
	"context"
	"sync"
	"time"

	statemodel "github.com/alex-poliushkin/theater/state"
)

const debugStateRecorderMaxAccesses = 128

type debugStateSnapshot struct {
	Accesses    []debugStateAccess
	Enrichments []debugStateEnrichment
	Omitted     int
}

type debugStateAccess struct {
	Seq   uint64
	Op    string
	Key   string
	Value debugSafeValue
	Err   string
}

type debugStateEnrichment struct {
	Backend string
	Fields  debugSnapshotSection
	Err     string
}

type debugStateRecorder struct {
	mu          sync.Mutex
	accesses    []debugStateAccess
	enrichments []debugStateRecorderEnrichment
	omitted     int
	nextSeq     uint64
	limit       int
	builder     debugSnapshotBuilder
}

type debugStateBackend struct {
	name     string
	backend  statemodel.Backend
	recorder *debugStateRecorder
}

type debugStateRecorderEnrichment struct {
	backend     string
	snapshotter DebugStateSnapshotter
}

func newDebugStateRecorder() *debugStateRecorder {
	return &debugStateRecorder{
		limit:   debugStateRecorderMaxAccesses,
		builder: newDebugSnapshotBuilder(),
	}
}

func (r *debugStateRecorder) Snapshot(ctx context.Context) (debugStateSnapshot, error) {
	if r == nil {
		return debugStateSnapshot{}, nil
	}

	r.mu.Lock()
	enrichments := append([]debugStateRecorderEnrichment(nil), r.enrichments...)
	snapshot := debugStateSnapshot{
		Accesses: make([]debugStateAccess, len(r.accesses)),
		Omitted:  r.omitted,
	}
	copy(snapshot.Accesses, r.accesses)
	r.mu.Unlock()

	var err error
	snapshot.Enrichments, err = r.snapshotEnrichments(ctx, enrichments)
	if err != nil {
		return debugStateSnapshot{}, err
	}

	return snapshot, nil
}

func (r *debugStateRecorder) wrapBackend(name string, backend statemodel.Backend) statemodel.Backend {
	if r == nil || backend == nil {
		return backend
	}

	r.registerEnrichment(name, backend)
	return debugStateBackend{
		name:     name,
		backend:  backend,
		recorder: r,
	}
}

func (r *debugStateRecorder) record(op, key string, value any, err error) {
	if r == nil {
		return
	}

	access := debugStateAccess{
		Op:  op,
		Key: key,
	}
	if value != nil {
		access.Value = r.builder.safeValue(value, ValueContract{})
	}
	if err != nil {
		access.Err = err.Error()
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextSeq++
	access.Seq = r.nextSeq
	r.accesses = append(r.accesses, access)
	if len(r.accesses) > r.limit {
		r.accesses = append([]debugStateAccess(nil), r.accesses[len(r.accesses)-r.limit:]...)
		r.omitted++
	}
}

func (r *debugStateRecorder) registerEnrichment(name string, backend statemodel.Backend) {
	snapshotter, ok := backend.(DebugStateSnapshotter)
	if !ok {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range r.enrichments {
		if r.enrichments[i].backend == name {
			return
		}
	}

	r.enrichments = append(r.enrichments, debugStateRecorderEnrichment{
		backend:     name,
		snapshotter: snapshotter,
	})
}

func (r *debugStateRecorder) snapshotEnrichments(
	ctx context.Context,
	enrichments []debugStateRecorderEnrichment,
) ([]debugStateEnrichment, error) {
	if len(enrichments) == 0 {
		return nil, nil
	}

	snapshot := make([]debugStateEnrichment, 0, len(enrichments))
	for i := range enrichments {
		values, err, panicErr := debugSnapshotBackendState(ctx, enrichments[i].backend, enrichments[i].snapshotter)
		if panicErr != nil {
			return nil, panicErr
		}
		entry := debugStateEnrichment{
			Backend: enrichments[i].backend,
		}
		if err != nil {
			entry.Err = err.Error()
			snapshot = append(snapshot, entry)
			continue
		}
		if len(values) == 0 {
			continue
		}

		entry.Fields = r.builder.valuesSection(values, nil, "state.backend."+enrichments[i].backend)
		snapshot = append(snapshot, entry)
	}

	return snapshot, nil
}

func debugSnapshotBackendState(
	ctx context.Context,
	backend string,
	snapshotter DebugStateSnapshotter,
) (Values, error, error) {
	var (
		values Values
		err    error
	)

	panicErr := invokeBoundaryError("debug state snapshotter", backend, func() error {
		values, err = snapshotter.DebugStateSnapshot(ctx)
		return nil
	})
	if panicErr != nil {
		return Values{}, nil, panicErr
	}

	return values, err, nil
}

func (d *debugRuntime) ensureStateRecorder() {
	if d == nil || d.stateRecorder != nil {
		return
	}

	d.stateRecorder = newDebugStateRecorder()
}

func (d *debugRuntime) wrapStateBackend(name string, backend statemodel.Backend) statemodel.Backend {
	if d == nil || d.stateRecorder == nil {
		return backend
	}

	return d.stateRecorder.wrapBackend(name, backend)
}

func (b debugStateBackend) Describe(ctx context.Context) (statemodel.Descriptor, error) {
	return b.backend.Describe(ctx)
}

func (b debugStateBackend) ReadRecord(ctx context.Context, key string) (statemodel.RecordSnapshot, error) {
	snapshot, err := b.backend.ReadRecord(ctx, key)
	if err != nil {
		b.recorder.record("get", debugStateRecordKey(b.name, key), nil, err)
		return statemodel.RecordSnapshot{}, err
	}

	b.recorder.record("get", debugStateRecordKey(b.name, key), debugStateRecordPayload(snapshot), nil)
	return snapshot, nil
}

func (b debugStateBackend) CompareAndSetRecord(
	ctx context.Context,
	key, expectedVersion string,
	value map[string]any,
) (statemodel.RecordSnapshot, error) {
	snapshot, err := b.backend.CompareAndSetRecord(ctx, key, expectedVersion, value)
	if err != nil {
		b.recorder.record("put", debugStateRecordKey(b.name, key), debugStatePutAttemptPayload(expectedVersion, value), err)
		return statemodel.RecordSnapshot{}, err
	}

	b.recorder.record("put", debugStateRecordKey(b.name, key), debugStateRecordPayload(snapshot), nil)
	return snapshot, nil
}

func (b debugStateBackend) Claim(
	ctx context.Context,
	pool string,
	selector statemodel.Selector,
	lease statemodel.LeaseSpec,
) (statemodel.ClaimResult, error) {
	result, err := b.backend.Claim(ctx, pool, selector, lease)
	if err != nil {
		b.recorder.record("claim", debugStatePoolKey(b.name, pool), debugStateClaimAttemptPayload(selector, lease), err)
		return statemodel.ClaimResult{}, err
	}

	b.recorder.record("claim", debugStateClaimKey(b.name, result.Claim), debugStateClaimResultPayload(result), nil)
	return result, nil
}

func (b debugStateBackend) Renew(
	ctx context.Context,
	claim statemodel.ClaimHandle,
	ttl time.Duration,
) (statemodel.ClaimHandle, error) {
	renewed, err := b.backend.Renew(ctx, claim, ttl)
	if err != nil {
		b.recorder.record("renew", debugStateClaimKey(b.name, claim), map[string]any{"ttl": ttl.String()}, err)
		return statemodel.ClaimHandle{}, err
	}

	b.recorder.record("renew", debugStateClaimKey(b.name, renewed), debugStateClaimHandlePayload(renewed), nil)
	return renewed, nil
}

func (b debugStateBackend) Release(ctx context.Context, claim statemodel.ClaimHandle, reason string) error {
	err := b.backend.Release(ctx, claim, reason)
	if err != nil {
		b.recorder.record("release", debugStateClaimKey(b.name, claim), debugStateReasonPayload(reason), err)
		return err
	}

	b.recorder.record("release", debugStateClaimKey(b.name, claim), debugStateReasonPayload(reason), nil)
	return nil
}

func (b debugStateBackend) Consume(
	ctx context.Context,
	claim statemodel.ClaimHandle,
	reason string,
	tombstone map[string]any,
) error {
	err := b.backend.Consume(ctx, claim, reason, tombstone)
	payload := debugStateConsumePayload(reason, tombstone)
	if err != nil {
		b.recorder.record("delete", debugStateClaimKey(b.name, claim), payload, err)
		return err
	}

	b.recorder.record("delete", debugStateClaimKey(b.name, claim), payload, nil)
	return nil
}

func debugStateRecordKey(backend, key string) string {
	return backend + "/record/" + key
}

func debugStatePoolKey(backend, pool string) string {
	return backend + "/pool/" + pool
}

func debugStateClaimKey(backend string, claim statemodel.ClaimHandle) string {
	if claim.Backend != "" {
		backend = claim.Backend
	}

	key := backend
	if claim.Pool != "" {
		key += "/pool/" + claim.Pool
	}
	if claim.ItemID != "" {
		key += "/item/" + claim.ItemID
	}
	if claim.ClaimID != "" {
		key += "/claim/" + claim.ClaimID
	}

	return key
}

func debugStateRecordPayload(snapshot statemodel.RecordSnapshot) map[string]any {
	payload := map[string]any{
		"value": snapshot.Value,
	}
	if snapshot.Version != "" {
		payload["version"] = snapshot.Version
	}
	if snapshot.Guarantee != "" {
		payload["guarantee"] = string(snapshot.Guarantee)
	}

	return payload
}

func debugStatePutAttemptPayload(expectedVersion string, value map[string]any) map[string]any {
	payload := map[string]any{
		"value": value,
	}
	if expectedVersion != "" {
		payload["expected_version"] = expectedVersion
	}

	return payload
}

func debugStateClaimAttemptPayload(selector statemodel.Selector, lease statemodel.LeaseSpec) map[string]any {
	selectorPayload := debugStateSelectorPayload(selector)
	leasePayload := debugStateLeasePayload(lease)
	payload := map[string]any{
		"selector": selectorPayload,
		"lease":    leasePayload,
	}
	if len(selectorPayload) == 0 {
		delete(payload, "selector")
	}
	if len(leasePayload) == 0 {
		delete(payload, "lease")
	}

	return payload
}

func debugStateClaimResultPayload(result statemodel.ClaimResult) map[string]any {
	return map[string]any{
		"item":  result.Item,
		"claim": debugStateClaimHandlePayload(result.Claim),
	}
}

func debugStateClaimHandlePayload(claim statemodel.ClaimHandle) map[string]any {
	payload := map[string]any{}
	if claim.ItemID != "" {
		payload["item_id"] = claim.ItemID
	}
	if claim.ClaimID != "" {
		payload["claim_id"] = claim.ClaimID
	}
	if !claim.ExpiresAt.IsZero() {
		payload["expires_at"] = claim.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	if claim.Version != "" {
		payload["version"] = claim.Version
	}
	if claim.ExpiryPolicy != "" {
		payload["expiry_policy"] = string(claim.ExpiryPolicy)
	}
	if claim.Guarantee != "" {
		payload["guarantee"] = string(claim.Guarantee)
	}

	return payload
}

func debugStateSelectorPayload(selector statemodel.Selector) map[string]any {
	payload := map[string]any{}
	if selector.ID != "" {
		payload["id"] = selector.ID
	}
	if len(selector.Fields) != 0 {
		payload["fields"] = selector.Fields
	}

	return payload
}

func debugStateLeasePayload(lease statemodel.LeaseSpec) map[string]any {
	payload := map[string]any{}
	if lease.TTL != 0 {
		payload["ttl"] = lease.TTL.String()
	}
	if lease.ExpiryPolicy != "" {
		payload["expiry_policy"] = string(lease.ExpiryPolicy)
	}

	return payload
}

func debugStateReasonPayload(reason string) map[string]any {
	if reason == "" {
		return nil
	}

	return map[string]any{"reason": reason}
}

func debugStateConsumePayload(reason string, tombstone map[string]any) map[string]any {
	payload := map[string]any{}
	if reason != "" {
		payload["reason"] = reason
	}
	if len(tombstone) != 0 {
		payload["tombstone"] = tombstone
	}
	if len(payload) == 0 {
		return nil
	}

	return payload
}
