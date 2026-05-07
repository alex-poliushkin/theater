package action

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/runtimevalue"
)

const (
	StateReadRef    = "action.state.read"
	StateUpdateRef  = "action.state.update"
	StateClaimRef   = "action.state.claim"
	StateRenewRef   = "action.state.renew"
	StateReleaseRef = "action.state.release"
	StateConsumeRef = "action.state.consume"
)

type stateReadAction struct{}

type stateUpdateAction struct{}

type stateClaimAction struct{}

type stateRenewAction struct{}

type stateReleaseAction struct{}

type stateConsumeAction struct{}

func (stateReadAction) Contract() theater.ActionContract {
	return theater.ActionContract{
		Inputs: map[string]theater.ValueContract{
			"record": {Kind: theater.ValueKindAny, Required: true, Sensitivity: theater.SensitivityInternal, Capture: theater.CaptureOmit},
		},
		Outputs: map[string]theater.ValueContract{
			"value":   {Kind: theater.ValueKindObject},
			"version": {Kind: theater.ValueKindString},
		},
	}
}

func (stateReadAction) Run(ctx context.Context, request theater.ActionRequest) (theater.Outputs, error) {
	record, err := recordHandleArg(request.Args["record"], "record")
	if err != nil {
		return nil, err
	}

	snapshot, err := stateManager(request.State).ReadRecord(ctx, record)
	if err != nil {
		return nil, err
	}

	return theater.Outputs{
		"value":   snapshot.Value,
		"version": snapshot.Version,
	}, nil
}

func (stateUpdateAction) Contract() theater.ActionContract {
	return theater.ActionContract{
		Inputs: map[string]theater.ValueContract{
			"record":           {Kind: theater.ValueKindAny, Required: true, Sensitivity: theater.SensitivityInternal, Capture: theater.CaptureOmit},
			"expected_version": {Kind: theater.ValueKindString, Required: true},
			"value":            {Kind: theater.ValueKindObject, Required: true},
		},
		Outputs: map[string]theater.ValueContract{
			"value":   {Kind: theater.ValueKindObject},
			"version": {Kind: theater.ValueKindString},
		},
	}
}

func (stateUpdateAction) Run(ctx context.Context, request theater.ActionRequest) (theater.Outputs, error) {
	record, err := recordHandleArg(request.Args["record"], "record")
	if err != nil {
		return nil, err
	}

	expectedVersion, err := runtimevalue.Wrap(request.Args["expected_version"]).String("expected_version")
	if err != nil {
		return nil, err
	}

	value, err := runtimevalue.Wrap(request.Args["value"]).Object("value")
	if err != nil {
		return nil, err
	}

	snapshot, err := stateManager(request.State).CompareAndSetRecord(ctx, record, expectedVersion, value)
	if err != nil {
		return nil, err
	}

	return theater.Outputs{
		"value":   snapshot.Value,
		"version": snapshot.Version,
	}, nil
}

func (stateClaimAction) Contract() theater.ActionContract {
	return theater.ActionContract{
		Inputs: map[string]theater.ValueContract{
			"pool":     {Kind: theater.ValueKindAny, Required: true, Sensitivity: theater.SensitivityInternal, Capture: theater.CaptureOmit},
			"selector": {Kind: theater.ValueKindObject},
			"lease":    {Kind: theater.ValueKindObject, Required: true},
		},
		Outputs: map[string]theater.ValueContract{
			"item":  {Kind: theater.ValueKindObject},
			"claim": {Kind: theater.ValueKindAny, Sensitivity: theater.SensitivityInternal, Capture: theater.CaptureOmit},
		},
	}
}

func (stateClaimAction) Run(ctx context.Context, request theater.ActionRequest) (theater.Outputs, error) {
	pool, err := poolHandleArg(request.Args["pool"], "pool")
	if err != nil {
		return nil, err
	}

	selector, err := selectorArg(request.Args["selector"])
	if err != nil {
		return nil, err
	}

	lease, err := leaseArg(request.Args["lease"])
	if err != nil {
		return nil, err
	}

	result, err := stateManager(request.State).Claim(ctx, pool, selector, lease)
	if err != nil {
		return nil, err
	}

	return theater.Outputs{
		"item":  result.Item,
		"claim": result.Claim,
	}, nil
}

func (stateRenewAction) Contract() theater.ActionContract {
	return theater.ActionContract{
		Inputs: map[string]theater.ValueContract{
			"claim": {Kind: theater.ValueKindAny, Required: true, Sensitivity: theater.SensitivityInternal, Capture: theater.CaptureOmit},
			"ttl":   {Kind: theater.ValueKindString, Required: true},
		},
		Outputs: map[string]theater.ValueContract{
			"claim": {Kind: theater.ValueKindAny, Sensitivity: theater.SensitivityInternal, Capture: theater.CaptureOmit},
		},
	}
}

func (stateRenewAction) Run(ctx context.Context, request theater.ActionRequest) (theater.Outputs, error) {
	claim, err := claimHandleArg(request.Args["claim"], "claim")
	if err != nil {
		return nil, err
	}

	ttl, err := durationArg(request.Args["ttl"], "ttl")
	if err != nil {
		return nil, err
	}

	renewed, err := stateManager(request.State).Renew(ctx, claim, ttl)
	if err != nil {
		return nil, err
	}

	return theater.Outputs{"claim": renewed}, nil
}

func (stateReleaseAction) Contract() theater.ActionContract {
	return theater.ActionContract{
		Inputs: map[string]theater.ValueContract{
			"claim": {Kind: theater.ValueKindAny, Required: true, Sensitivity: theater.SensitivityInternal, Capture: theater.CaptureOmit},
		},
	}
}

func (stateReleaseAction) Run(ctx context.Context, request theater.ActionRequest) (theater.Outputs, error) {
	claim, err := claimHandleArg(request.Args["claim"], "claim")
	if err != nil {
		return nil, err
	}

	if err := stateManager(request.State).Release(ctx, claim, ""); err != nil {
		return nil, err
	}

	return theater.Outputs{}, nil
}

func (stateConsumeAction) Contract() theater.ActionContract {
	return theater.ActionContract{
		Inputs: map[string]theater.ValueContract{
			"claim":     {Kind: theater.ValueKindAny, Required: true, Sensitivity: theater.SensitivityInternal, Capture: theater.CaptureOmit},
			"tombstone": {Kind: theater.ValueKindObject},
		},
	}
}

func (stateConsumeAction) Run(ctx context.Context, request theater.ActionRequest) (theater.Outputs, error) {
	claim, err := claimHandleArg(request.Args["claim"], "claim")
	if err != nil {
		return nil, err
	}

	tombstone, err := optionalObjectArg(request.Args["tombstone"], "tombstone")
	if err != nil {
		return nil, err
	}

	if err := stateManager(request.State).Consume(ctx, claim, "", tombstone); err != nil {
		return nil, err
	}

	return theater.Outputs{}, nil
}

func stateManager(manager *theater.StateManager) *theater.StateManager {
	return manager
}

func recordHandleArg(value any, field string) (theater.StateRecordHandle, error) {
	handle, ok := value.(theater.StateRecordHandle)
	if !ok {
		return theater.StateRecordHandle{}, fmt.Errorf("%s must be state record handle, got %T", field, value)
	}

	return handle, nil
}

func poolHandleArg(value any, field string) (theater.StatePoolHandle, error) {
	handle, ok := value.(theater.StatePoolHandle)
	if !ok {
		return theater.StatePoolHandle{}, fmt.Errorf("%s must be state pool handle, got %T", field, value)
	}

	return handle, nil
}

func claimHandleArg(value any, field string) (theater.StateClaimHandle, error) {
	handle, ok := value.(theater.StateClaimHandle)
	if !ok {
		return theater.StateClaimHandle{}, fmt.Errorf("%s must be state claim handle, got %T", field, value)
	}

	return handle, nil
}

func selectorArg(value any) (theater.StateSelector, error) {
	if value == nil {
		return theater.StateSelector{}, nil
	}

	object, err := runtimevalue.Wrap(value).Object("selector")
	if err != nil {
		return theater.StateSelector{}, err
	}

	selector := theater.StateSelector{}
	if rawID, ok := object["id"]; ok {
		id, err := runtimevalue.Wrap(rawID).String("selector.id")
		if err != nil {
			return theater.StateSelector{}, err
		}
		selector.ID = id
	}

	if rawFields, ok := object["fields"]; ok {
		fieldsObject, err := runtimevalue.Wrap(rawFields).Object("selector.fields")
		if err != nil {
			return theater.StateSelector{}, err
		}

		selector.Fields = make(map[string]string, len(fieldsObject))
		for key, raw := range fieldsObject {
			text, err := runtimevalue.Wrap(raw).String("selector.fields." + key)
			if err != nil {
				return theater.StateSelector{}, err
			}
			selector.Fields[key] = text
		}
		if len(selector.Fields) == 0 {
			selector.Fields = nil
		}
	}

	return selector, nil
}

func leaseArg(value any) (theater.StateLeaseSpec, error) {
	object, err := runtimevalue.Wrap(value).Object("lease")
	if err != nil {
		return theater.StateLeaseSpec{}, err
	}

	rawTTL, ok := object["ttl"]
	if !ok {
		return theater.StateLeaseSpec{}, errors.New("lease.ttl is required")
	}

	ttl, err := durationArg(rawTTL, "lease.ttl")
	if err != nil {
		return theater.StateLeaseSpec{}, err
	}

	policy := theater.StateExpiryStale
	if rawPolicy, ok := object["on_expiry"]; ok {
		text, err := runtimevalue.Wrap(rawPolicy).String("lease.on_expiry")
		if err != nil {
			return theater.StateLeaseSpec{}, err
		}

		policy = theater.StateExpiryPolicy(text)
		if !policy.Valid() {
			return theater.StateLeaseSpec{}, fmt.Errorf("lease.on_expiry %q is invalid", text)
		}
	}

	return theater.StateLeaseSpec{
		TTL:          ttl,
		ExpiryPolicy: policy,
	}, nil
}

func durationArg(value any, field string) (time.Duration, error) {
	text, err := runtimevalue.Wrap(value).String(field)
	if err != nil {
		return 0, err
	}

	duration, err := time.ParseDuration(text)
	if err != nil {
		return 0, err
	}
	if duration <= 0 {
		return 0, fmt.Errorf("%s must be positive duration", field)
	}

	return duration, nil
}

func optionalObjectArg(value any, field string) (map[string]any, error) {
	if value == nil {
		return map[string]any{}, nil
	}

	return runtimevalue.Wrap(value).Object(field)
}
