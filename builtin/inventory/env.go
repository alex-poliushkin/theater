package inventory

import (
	"context"
	"fmt"
	"os"

	"github.com/alex-poliushkin/theater"
)

type envInventory struct{}

func (envInventory) Contract() theater.InventoryContract {
	return theater.InventoryContract{
		Summary: "read a single environment variable",
		Args: []theater.ArgSpec{
			{
				Name:     "name",
				Accepts:  theater.ValueContract{Kinds: theater.NewValueKindSet(theater.ValueKindString)},
				Required: true,
			},
		},
		Produces: theater.ValueContract{Kinds: theater.NewValueKindSet(theater.ValueKindString)},
	}
}

func (envInventory) Acquire(_ context.Context, request theater.InventoryRequest) (any, error) {
	name, err := stringArg(request.Args, "name")
	if err != nil {
		return nil, err
	}

	value, ok := os.LookupEnv(name)
	if !ok {
		return nil, fmt.Errorf("environment variable %q is not set", name)
	}

	return value, nil
}
