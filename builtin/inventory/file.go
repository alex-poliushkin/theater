package inventory

import (
	"context"
	"fmt"
	"os"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/runtimevalue"
)

type fileInventory struct{}

func (fileInventory) Contract() theater.InventoryContract {
	return theater.InventoryContract{
		Summary: "read a file as raw bytes",
		Args: []theater.ArgSpec{
			{
				Name:     "path",
				Accepts:  theater.ValueContract{Kinds: theater.NewValueKindSet(theater.ValueKindString)},
				Required: true,
			},
		},
		Produces: theater.ValueContract{Kinds: theater.NewValueKindSet(theater.ValueKindBytes)},
	}
}

func (fileInventory) Acquire(_ context.Context, request theater.InventoryRequest) (any, error) {
	path, err := stringArg(request.Args, "path")
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func stringArg(args theater.Args, key string) (string, error) {
	value, ok := args[key]
	if !ok {
		return "", fmt.Errorf("%s arg is required", key)
	}

	return runtimevalue.String(value, key)
}
