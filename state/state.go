package state

import (
	"context"
	"fmt"
	"sort"
	"time"
)

const (
	GuaranteeReadOnly         GuaranteeTier = "read-only"
	GuaranteeLocalAtomic      GuaranteeTier = "local-atomic"
	GuaranteeSharedOptimistic GuaranteeTier = "shared-optimistic"
	GuaranteeSharedAtomic     GuaranteeTier = "shared-atomic"

	ExpiryStale   ExpiryPolicy = "stale"
	ExpiryReclaim ExpiryPolicy = "reclaim"
)

type GuaranteeTier string

type ExpiryPolicy string

type RecordHandle struct {
	Backend      string        `json:"backend,omitempty"`
	Record       string        `json:"record,omitempty"`
	MinGuarantee GuaranteeTier `json:"min_guarantee,omitempty"`
}

type PoolHandle struct {
	Backend      string        `json:"backend,omitempty"`
	Pool         string        `json:"pool,omitempty"`
	MinGuarantee GuaranteeTier `json:"min_guarantee,omitempty"`
}

type RecordSnapshot struct {
	Key       string         `json:"key,omitempty"`
	Value     map[string]any `json:"value,omitempty"`
	Version   string         `json:"version,omitempty"`
	Backend   string         `json:"backend,omitempty"`
	Guarantee GuaranteeTier  `json:"guarantee,omitempty"`
}

type ClaimHandle struct {
	Backend      string        `json:"backend,omitempty"`
	Pool         string        `json:"pool,omitempty"`
	ItemID       string        `json:"item_id,omitempty"`
	ClaimID      string        `json:"claim_id,omitempty"`
	ExpiresAt    time.Time     `json:"expires_at,omitempty"`
	Version      string        `json:"version,omitempty"`
	ExpiryPolicy ExpiryPolicy  `json:"expiry_policy,omitempty"`
	Guarantee    GuaranteeTier `json:"guarantee,omitempty"`
}

type ClaimResult struct {
	Item  map[string]any `json:"item,omitempty"`
	Claim ClaimHandle    `json:"claim"`
}

type Selector struct {
	ID     string            `json:"id,omitempty"`
	Fields map[string]string `json:"fields,omitempty"`
}

type LeaseSpec struct {
	TTL          time.Duration `json:"ttl,omitempty"`
	ExpiryPolicy ExpiryPolicy  `json:"expiry_policy,omitempty"`
}

type Descriptor struct {
	Guarantee       GuaranteeTier `json:"guarantee,omitempty"`
	SupportsCAS     bool          `json:"supports_cas,omitempty"`
	SupportsClaim   bool          `json:"supports_claim,omitempty"`
	SupportsRenew   bool          `json:"supports_renew,omitempty"`
	SupportsRelease bool          `json:"supports_release,omitempty"`
	SupportsConsume bool          `json:"supports_consume,omitempty"`
}

type Backend interface {
	Describe(ctx context.Context) (Descriptor, error)
	ReadRecord(ctx context.Context, key string) (RecordSnapshot, error)
	CompareAndSetRecord(ctx context.Context, key, expectedVersion string, value map[string]any) (RecordSnapshot, error)
	Claim(ctx context.Context, pool string, selector Selector, lease LeaseSpec) (ClaimResult, error)
	Renew(ctx context.Context, claim ClaimHandle, ttl time.Duration) (ClaimHandle, error)
	Release(ctx context.Context, claim ClaimHandle, reason string) error
	Consume(ctx context.Context, claim ClaimHandle, reason string, tombstone map[string]any) error
}

type Manager struct {
	backends map[string]Backend
}

func NewManager(backends map[string]Backend) *Manager {
	if len(backends) == 0 {
		return &Manager{}
	}

	cloned := make(map[string]Backend, len(backends))
	for name, backend := range backends {
		cloned[name] = backend
	}

	return &Manager{backends: cloned}
}

func (t GuaranteeTier) Valid() bool {
	switch t {
	case GuaranteeReadOnly, GuaranteeLocalAtomic, GuaranteeSharedOptimistic, GuaranteeSharedAtomic:
		return true
	default:
		return false
	}
}

func (t GuaranteeTier) Supports(required GuaranteeTier) bool {
	if required == "" {
		return true
	}

	return t.rank() >= required.rank()
}

func (p ExpiryPolicy) Valid() bool {
	switch p {
	case ExpiryStale, ExpiryReclaim:
		return true
	default:
		return false
	}
}

func (m *Manager) ReadRecord(ctx context.Context, handle RecordHandle) (RecordSnapshot, error) {
	backend, err := m.backend(handle.Backend)
	if err != nil {
		return RecordSnapshot{}, err
	}

	snapshot, err := backend.ReadRecord(ctx, handle.Record)
	if err != nil {
		return RecordSnapshot{}, err
	}
	snapshot.Backend = handle.Backend
	return snapshot, nil
}

func (m *Manager) CompareAndSetRecord(
	ctx context.Context,
	handle RecordHandle,
	expectedVersion string,
	value map[string]any,
) (RecordSnapshot, error) {
	backend, err := m.backend(handle.Backend)
	if err != nil {
		return RecordSnapshot{}, err
	}

	snapshot, err := backend.CompareAndSetRecord(ctx, handle.Record, expectedVersion, value)
	if err != nil {
		return RecordSnapshot{}, err
	}
	snapshot.Backend = handle.Backend
	return snapshot, nil
}

func (m *Manager) Claim(ctx context.Context, handle PoolHandle, selector Selector, lease LeaseSpec) (ClaimResult, error) {
	backend, err := m.backend(handle.Backend)
	if err != nil {
		return ClaimResult{}, err
	}

	result, err := backend.Claim(ctx, handle.Pool, selector, lease)
	if err != nil {
		return ClaimResult{}, err
	}
	result.Claim.Backend = handle.Backend
	return result, nil
}

func (m *Manager) Renew(ctx context.Context, claim ClaimHandle, ttl time.Duration) (ClaimHandle, error) {
	backend, err := m.backend(claim.Backend)
	if err != nil {
		return ClaimHandle{}, err
	}

	renewed, err := backend.Renew(ctx, claim, ttl)
	if err != nil {
		return ClaimHandle{}, err
	}
	renewed.Backend = claim.Backend
	return renewed, nil
}

func (m *Manager) Release(ctx context.Context, claim ClaimHandle, reason string) error {
	backend, err := m.backend(claim.Backend)
	if err != nil {
		return err
	}

	return backend.Release(ctx, claim, reason)
}

func (m *Manager) Consume(ctx context.Context, claim ClaimHandle, reason string, tombstone map[string]any) error {
	backend, err := m.backend(claim.Backend)
	if err != nil {
		return err
	}

	return backend.Consume(ctx, claim, reason, tombstone)
}

func (m *Manager) backend(name string) (Backend, error) {
	if m == nil {
		return nil, fmt.Errorf("state backend %q is not configured", name)
	}

	backend, ok := m.backends[name]
	if !ok {
		return nil, fmt.Errorf("state backend %q is not configured", name)
	}

	return backend, nil
}

func (t GuaranteeTier) rank() int {
	switch t {
	case GuaranteeReadOnly:
		return 1
	case GuaranteeLocalAtomic:
		return 2
	case GuaranteeSharedOptimistic:
		return 3
	case GuaranteeSharedAtomic:
		return 4
	default:
		return 0
	}
}

func SortedBackendNames(backends map[string]Backend) []string {
	if len(backends) == 0 {
		return nil
	}

	names := make([]string, 0, len(backends))
	for name := range backends {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
