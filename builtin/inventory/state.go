package inventory

import (
	"context"
	"fmt"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/runtimevalue"
)

const (
	StateRecordRef = "inventory.state.record"
	StatePoolRef   = "inventory.state.pool"
)

type stateRecordInventory struct{}

type statePoolInventory struct{}

func (stateRecordInventory) Contract() theater.InventoryContract {
	return theater.InventoryContract{
		Summary: "build a persistent-state record handle",
		Args: []theater.ArgSpec{
			{Name: "backend", Accepts: theater.ValueContract{Kind: theater.ValueKindString}, Required: true},
			{Name: "record", Accepts: theater.ValueContract{Kind: theater.ValueKindString}, Required: true},
			{Name: "min_guarantee", Accepts: theater.ValueContract{Kind: theater.ValueKindString}},
		},
		Produces: theater.ValueContract{Kind: theater.ValueKindAny, Sensitivity: theater.SensitivityInternal, Capture: theater.CaptureOmit},
	}
}

func (stateRecordInventory) Acquire(_ context.Context, request theater.InventoryRequest) (any, error) {
	return acquireStateRecordHandle(request.Args)
}

func (statePoolInventory) Contract() theater.InventoryContract {
	return theater.InventoryContract{
		Summary: "build a persistent-state fixture-pool handle",
		Args: []theater.ArgSpec{
			{Name: "backend", Accepts: theater.ValueContract{Kind: theater.ValueKindString}, Required: true},
			{Name: "pool", Accepts: theater.ValueContract{Kind: theater.ValueKindString}, Required: true},
			{Name: "min_guarantee", Accepts: theater.ValueContract{Kind: theater.ValueKindString}},
		},
		Produces: theater.ValueContract{Kind: theater.ValueKindAny, Sensitivity: theater.SensitivityInternal, Capture: theater.CaptureOmit},
	}
}

func (statePoolInventory) Acquire(_ context.Context, request theater.InventoryRequest) (any, error) {
	return acquireStatePoolHandle(request.Args)
}

func acquireStateRecordHandle(args theater.Args) (theater.StateRecordHandle, error) {
	backend, err := runtimevalue.Wrap(args["backend"]).String("backend")
	if err != nil {
		return theater.StateRecordHandle{}, err
	}

	record, err := runtimevalue.Wrap(args["record"]).String("record")
	if err != nil {
		return theater.StateRecordHandle{}, err
	}

	minGuarantee, err := optionalGuaranteeArg(args["min_guarantee"], "min_guarantee")
	if err != nil {
		return theater.StateRecordHandle{}, err
	}

	return theater.StateRecordHandle{
		Backend:      backend,
		Record:       record,
		MinGuarantee: minGuarantee,
	}, nil
}

func acquireStatePoolHandle(args theater.Args) (theater.StatePoolHandle, error) {
	backend, err := runtimevalue.Wrap(args["backend"]).String("backend")
	if err != nil {
		return theater.StatePoolHandle{}, err
	}

	pool, err := runtimevalue.Wrap(args["pool"]).String("pool")
	if err != nil {
		return theater.StatePoolHandle{}, err
	}

	minGuarantee, err := optionalGuaranteeArg(args["min_guarantee"], "min_guarantee")
	if err != nil {
		return theater.StatePoolHandle{}, err
	}

	return theater.StatePoolHandle{
		Backend:      backend,
		Pool:         pool,
		MinGuarantee: minGuarantee,
	}, nil
}

func optionalGuaranteeArg(value any, field string) (theater.StateGuaranteeTier, error) {
	if value == nil {
		return "", nil
	}

	text, err := runtimevalue.Wrap(value).String(field)
	if err != nil {
		return "", err
	}

	tier := theater.StateGuaranteeTier(text)
	if !tier.Valid() {
		return "", fmt.Errorf("%s %q is invalid", field, text)
	}

	return tier, nil
}
