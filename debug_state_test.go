package theater

import (
	"context"
	"errors"
	"testing"
	"time"

	statemodel "github.com/alex-poliushkin/theater/state"
)

func TestDebugStateRecorderSnapshotKeepsRecentAccesses(t *testing.T) {
	t.Parallel()

	recorder := &debugStateRecorder{
		limit:   2,
		builder: newDebugSnapshotBuilder(),
	}

	recorder.record("get", "debug/record/session", map[string]any{
		"value": map[string]any{"token": "issued-token"},
	}, nil)
	recorder.record("put", "debug/record/session", map[string]any{
		"expected_version": "1",
		"value":            map[string]any{"token": "rotated-token"},
	}, errors.New("version mismatch"))
	recorder.record("delete", "debug/pool/leases/item/otp-1/claim/claim-1", map[string]any{
		"reason": "used",
	}, nil)

	snapshot, err := recorder.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	if got, want := len(snapshot.Accesses), 2; got != want {
		t.Fatalf("access count mismatch: got %d want %d", got, want)
	}
	if got, want := snapshot.Omitted, 1; got != want {
		t.Fatalf("omitted count mismatch: got %d want %d", got, want)
	}

	first := snapshot.Accesses[0]
	if got, want := first.Seq, uint64(2); got != want {
		t.Fatalf("first seq mismatch: got %d want %d", got, want)
	}
	if got, want := first.Op, "put"; got != want {
		t.Fatalf("first op mismatch: got %q want %q", got, want)
	}
	if got, want := first.Key, "debug/record/session"; got != want {
		t.Fatalf("first key mismatch: got %q want %q", got, want)
	}
	if got, want := first.Err, "version mismatch"; got != want {
		t.Fatalf("first err mismatch: got %q want %q", got, want)
	}
	if got, want := first.Value.Kind, "object"; got != want {
		t.Fatalf("first value kind mismatch: got %q want %q", got, want)
	}

	second := snapshot.Accesses[1]
	if got, want := second.Seq, uint64(3); got != want {
		t.Fatalf("second seq mismatch: got %d want %d", got, want)
	}
	if got, want := second.Op, "delete"; got != want {
		t.Fatalf("second op mismatch: got %q want %q", got, want)
	}
	if got, want := second.Key, "debug/pool/leases/item/otp-1/claim/claim-1"; got != want {
		t.Fatalf("second key mismatch: got %q want %q", got, want)
	}
}

func TestRunIncludesStateAccessesInDebugBoundarySnapshots(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		State: &StateSpec{
			Backends: map[string]StateBackendSpec{
				"debug": {Use: "state.debug"},
			},
		},
		Scenarios: []ScenarioSpec{{
			ID: "stateful",
			Acts: []ActSpec{{
				ID:     "touch",
				Action: ActionSpec{Use: "action.stateful"},
			}},
		}},
		ScenarioCalls: []ScenarioCallSpec{{
			ID:         "stateful-call",
			ScenarioID: "stateful",
		}},
	}

	backend := debugStateTestBackend{
		readRecord: statemodel.RecordSnapshot{
			Key:     "session",
			Value:   map[string]any{"token": "issued-token"},
			Version: "1",
		},
		compareAndSetRecord: statemodel.RecordSnapshot{
			Key:     "session",
			Value:   map[string]any{"token": "rotated-token"},
			Version: "2",
		},
		debugSnapshot: Values{
			"backend": "state.debug",
			"mode":    "enriched",
		},
	}

	catalog := NewCatalog()
	if err := catalog.RegisterAction("action.stateful", debugStatefulAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	if err := catalog.RegisterStateBackend("state.debug", StateBackendDef{
		Describe: func(config Values) (StateDescriptor, error) {
			return StateDescriptor{
				Guarantee:   StateGuaranteeLocalAtomic,
				SupportsCAS: true,
			}, nil
		},
		Open: func(config Values) (StateBackend, error) {
			return backend, nil
		},
	}); err != nil {
		t.Fatalf("register state backend failed: %v", err)
	}

	matchers, err := NewMatcherCatalog()
	if err != nil {
		t.Fatalf("register matcher catalog failed: %v", err)
	}

	hits := make([]debugBoundaryState, 0, 6)
	result, err := NewRunner(catalog, matchers).runWithDebugRuntime(
		context.Background(),
		spec,
		RunOptions{},
		&debugRuntime{
			breakpointSpecs: []string{"path=**/action,kind=action"},
			boundaryHook: func(_ context.Context, state debugBoundaryState) error {
				hits = append(hits, state)
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if got, want := result.Report.Status, StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	var before *debugBoundaryState
	var after *debugBoundaryState
	for i := range hits {
		if hits[i].Ref.Kind != debugBoundaryKindAction {
			continue
		}
		switch hits[i].Ref.Phase {
		case debugBoundaryPhaseBefore:
			before = &hits[i]
		case debugBoundaryPhaseAfter:
			after = &hits[i]
		}
	}
	if before == nil {
		t.Fatal("action before boundary was not recorded")
	}
	if after == nil {
		t.Fatal("action after boundary was not recorded")
	}
	if got := len(before.State.Accesses); got != 0 {
		t.Fatalf("before-boundary state access count mismatch: got %d want 0", got)
	}
	if got, want := after.Ref.Phase, debugBoundaryPhaseAfter; got != want {
		t.Fatalf("after-boundary phase mismatch: got %q want %q", got, want)
	}
	if got, want := len(after.State.Accesses), 2; got != want {
		t.Fatalf("after-boundary state access count mismatch: got %d want %d", got, want)
	}
	if got, want := len(after.State.Enrichments), 1; got != want {
		t.Fatalf("after-boundary enrichment count mismatch: got %d want %d", got, want)
	}

	getAccess := after.State.Accesses[0]
	if got, want := getAccess.Op, "get"; got != want {
		t.Fatalf("get access op mismatch: got %q want %q", got, want)
	}
	if got, want := getAccess.Key, "debug/record/session"; got != want {
		t.Fatalf("get access key mismatch: got %q want %q", got, want)
	}
	if got, want := getAccess.Value.Kind, "object"; got != want {
		t.Fatalf("get access kind mismatch: got %q want %q", got, want)
	}

	putAccess := after.State.Accesses[1]
	if got, want := putAccess.Op, "put"; got != want {
		t.Fatalf("put access op mismatch: got %q want %q", got, want)
	}
	if got, want := putAccess.Key, "debug/record/session"; got != want {
		t.Fatalf("put access key mismatch: got %q want %q", got, want)
	}
	if putAccess.Err != "" {
		t.Fatalf("put access err = %q, want empty", putAccess.Err)
	}

	enrichment := after.State.Enrichments[0]
	if got, want := enrichment.Backend, "debug"; got != want {
		t.Fatalf("enrichment backend mismatch: got %q want %q", got, want)
	}
	if enrichment.Err != "" {
		t.Fatalf("enrichment err = %q, want empty", enrichment.Err)
	}
	if got, want := len(enrichment.Fields.Fields), 2; got != want {
		t.Fatalf("enrichment field count mismatch: got %d want %d", got, want)
	}
}

func TestRunKeepsBackendSnapshotErrorsInsideStateEnrichment(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		State: &StateSpec{
			Backends: map[string]StateBackendSpec{
				"debug": {Use: "state.debug"},
			},
		},
		Scenarios: []ScenarioSpec{{
			ID: "stateful",
			Acts: []ActSpec{{
				ID:     "touch",
				Action: ActionSpec{Use: "action.stateful"},
			}},
		}},
		ScenarioCalls: []ScenarioCallSpec{{
			ID:         "stateful-call",
			ScenarioID: "stateful",
		}},
	}

	backend := debugStateTestBackend{
		readRecord: statemodel.RecordSnapshot{
			Key:     "session",
			Value:   map[string]any{"token": "issued-token"},
			Version: "1",
		},
		compareAndSetRecord: statemodel.RecordSnapshot{
			Key:     "session",
			Value:   map[string]any{"token": "rotated-token"},
			Version: "2",
		},
		debugSnapshotErr: errors.New("snapshot unavailable"),
	}

	catalog := NewCatalog()
	if err := catalog.RegisterAction("action.stateful", debugStatefulAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	if err := catalog.RegisterStateBackend("state.debug", StateBackendDef{
		Describe: func(config Values) (StateDescriptor, error) {
			return StateDescriptor{
				Guarantee:   StateGuaranteeLocalAtomic,
				SupportsCAS: true,
			}, nil
		},
		Open: func(config Values) (StateBackend, error) {
			return backend, nil
		},
	}); err != nil {
		t.Fatalf("register state backend failed: %v", err)
	}

	matchers, err := NewMatcherCatalog()
	if err != nil {
		t.Fatalf("register matcher catalog failed: %v", err)
	}

	hits := make([]debugBoundaryState, 0, 6)
	result, err := NewRunner(catalog, matchers).runWithDebugRuntime(
		context.Background(),
		spec,
		RunOptions{},
		&debugRuntime{
			breakpointSpecs: []string{"path=**/action,kind=action"},
			boundaryHook: func(_ context.Context, state debugBoundaryState) error {
				hits = append(hits, state)
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if got, want := result.Report.Status, StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	var after *debugBoundaryState
	for i := range hits {
		if hits[i].Ref.Kind == debugBoundaryKindAction && hits[i].Ref.Phase == debugBoundaryPhaseAfter {
			after = &hits[i]
			break
		}
	}
	if after == nil {
		t.Fatal("action after-boundary was not recorded")
	}
	if got, want := len(after.State.Enrichments), 1; got != want {
		t.Fatalf("after-boundary enrichment count mismatch: got %d want %d", got, want)
	}

	enrichment := after.State.Enrichments[0]
	if got, want := enrichment.Backend, "debug"; got != want {
		t.Fatalf("enrichment backend mismatch: got %q want %q", got, want)
	}
	if got, want := enrichment.Err, "snapshot unavailable"; got != want {
		t.Fatalf("enrichment err mismatch: got %q want %q", got, want)
	}
	if got, want := len(enrichment.Fields.Fields), 0; got != want {
		t.Fatalf("enrichment field count mismatch: got %d want %d", got, want)
	}
}

func TestRunConvertsPanickingDebugStateSnapshotterIntoFailedReport(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		State: &StateSpec{
			Backends: map[string]StateBackendSpec{
				"debug": {Use: "state.debug"},
			},
		},
		Scenarios: []ScenarioSpec{{
			ID: "stateful",
			Acts: []ActSpec{{
				ID:     "touch",
				Action: ActionSpec{Use: "action.stateful"},
			}},
		}},
		ScenarioCalls: []ScenarioCallSpec{{
			ID:         "stateful-call",
			ScenarioID: "stateful",
		}},
	}

	catalog := NewCatalog()
	if err := catalog.RegisterAction("action.stateful", debugStatefulAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	if err := catalog.RegisterStateBackend("state.debug", StateBackendDef{
		Describe: func(config Values) (StateDescriptor, error) {
			return StateDescriptor{
				Guarantee:   StateGuaranteeLocalAtomic,
				SupportsCAS: true,
			}, nil
		},
		Open: func(config Values) (StateBackend, error) {
			return debugStateTestBackend{
				readRecord: statemodel.RecordSnapshot{
					Key:     "session",
					Value:   map[string]any{"token": "issued-token"},
					Version: "1",
				},
				compareAndSetRecord: statemodel.RecordSnapshot{
					Key:     "session",
					Value:   map[string]any{"token": "rotated-token"},
					Version: "2",
				},
				debugSnapshotPanic: "backend snapshot boom",
			}, nil
		},
	}); err != nil {
		t.Fatalf("register state backend failed: %v", err)
	}

	matchers, err := NewMatcherCatalog()
	if err != nil {
		t.Fatalf("register matcher catalog failed: %v", err)
	}

	result, err := NewRunner(catalog, matchers).runWithDebugRuntime(
		context.Background(),
		spec,
		RunOptions{},
		&debugRuntime{
			breakpointSpecs: []string{"path=**/action,kind=action"},
			boundaryHook: func(_ context.Context, state debugBoundaryState) error {
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if got, want := result.Report.Status, StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if result.Report.Failure == nil {
		t.Fatal("report failure = nil, want contained debug failure")
	}
	if got, want := result.Report.Failure.Kind, FailureKindInternal; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}
	if got, want := result.Report.Failure.Summary, "debug state snapshotter panicked"; got != want {
		t.Fatalf("failure summary mismatch: got %q want %q", got, want)
	}
	if got, want := result.Report.Failure.Cause.Error(), `debug state snapshotter "debug" panicked: backend snapshot boom`; got != want {
		t.Fatalf("failure cause mismatch: got %q want %q", got, want)
	}
}

type debugStatefulAction struct{}

type debugStateTestBackend struct {
	readRecord          statemodel.RecordSnapshot
	readErr             error
	compareAndSetRecord statemodel.RecordSnapshot
	compareAndSetErr    error
	claimResult         statemodel.ClaimResult
	claimErr            error
	renewClaim          statemodel.ClaimHandle
	renewErr            error
	releaseErr          error
	consumeErr          error
	debugSnapshot       Values
	debugSnapshotErr    error
	debugSnapshotPanic  string
}

func (debugStatefulAction) Contract() ActionContract {
	return ActionContract{}
}

func (debugStatefulAction) Run(ctx context.Context, request ActionRequest) (Outputs, error) {
	record := StateRecordHandle{
		Backend: "debug",
		Record:  "session",
	}
	if _, err := request.State.ReadRecord(ctx, record); err != nil {
		return nil, err
	}
	if _, err := request.State.CompareAndSetRecord(ctx, record, "1", map[string]any{"token": "rotated-token"}); err != nil {
		return nil, err
	}

	return Outputs{}, nil
}

func (b debugStateTestBackend) Describe(context.Context) (statemodel.Descriptor, error) {
	return statemodel.Descriptor{
		Guarantee:   statemodel.GuaranteeLocalAtomic,
		SupportsCAS: true,
	}, nil
}

func (b debugStateTestBackend) ReadRecord(context.Context, string) (statemodel.RecordSnapshot, error) {
	if b.readErr != nil {
		return statemodel.RecordSnapshot{}, b.readErr
	}

	return b.readRecord, nil
}

func (b debugStateTestBackend) CompareAndSetRecord(
	context.Context,
	string,
	string,
	map[string]any,
) (statemodel.RecordSnapshot, error) {
	if b.compareAndSetErr != nil {
		return statemodel.RecordSnapshot{}, b.compareAndSetErr
	}

	return b.compareAndSetRecord, nil
}

func (b debugStateTestBackend) Claim(
	context.Context,
	string,
	statemodel.Selector,
	statemodel.LeaseSpec,
) (statemodel.ClaimResult, error) {
	if b.claimErr != nil {
		return statemodel.ClaimResult{}, b.claimErr
	}

	return b.claimResult, nil
}

func (b debugStateTestBackend) Renew(
	context.Context,
	statemodel.ClaimHandle,
	time.Duration,
) (statemodel.ClaimHandle, error) {
	if b.renewErr != nil {
		return statemodel.ClaimHandle{}, b.renewErr
	}

	return b.renewClaim, nil
}

func (b debugStateTestBackend) Release(context.Context, statemodel.ClaimHandle, string) error {
	return b.releaseErr
}

func (b debugStateTestBackend) Consume(context.Context, statemodel.ClaimHandle, string, map[string]any) error {
	return b.consumeErr
}

func (b debugStateTestBackend) DebugStateSnapshot(context.Context) (Values, error) {
	if b.debugSnapshotPanic != "" {
		panic(b.debugSnapshotPanic)
	}
	if b.debugSnapshotErr != nil {
		return nil, b.debugSnapshotErr
	}

	return cloneValues(b.debugSnapshot), nil
}
