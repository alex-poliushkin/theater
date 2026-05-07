package inventory

import (
	"context"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/builtin/internal/builtinhttp"
)

type httpInventory struct{}

func (httpInventory) Contract() theater.InventoryContract {
	return theater.InventoryContract{
		Summary: "fetch a remote HTTP resource body with GET",
		Args: []theater.ArgSpec{
			{
				Name:     "url",
				Accepts:  theater.ValueContract{Kinds: theater.NewValueKindSet(theater.ValueKindString)},
				Required: true,
			},
			{
				Name:    "headers",
				Accepts: theater.ValueContract{Kinds: theater.NewValueKindSet(theater.ValueKindObject)},
			},
			{
				Name:        "form",
				Accepts:     theater.ValueContract{Kinds: theater.NewValueKindSet(theater.ValueKindObject)},
				Description: builtinhttp.FormArgDescription,
			},
			{
				Name:    "timeout",
				Accepts: theater.ValueContract{Kinds: theater.NewValueKindSet(theater.ValueKindString)},
			},
			{
				Name:        "session",
				Accepts:     theater.ValueContract{Kinds: theater.NewValueKindSet(theater.ValueKindString)},
				Description: builtinhttp.SessionArgDescription,
			},
			{
				Name:        "identity",
				Accepts:     theater.ValueContract{Kinds: theater.NewValueKindSet(theater.ValueKindString)},
				Description: builtinhttp.IdentityArgDescription,
			},
			{
				Name:        "auth",
				Accepts:     theater.ValueContract{Kinds: theater.NewValueKindSet(theater.ValueKindString)},
				Description: builtinhttp.AuthArgDescription,
			},
		},
		Produces: theater.ValueContract{Kinds: theater.NewValueKindSet(theater.ValueKindBytes)},
	}
}

func (httpInventory) Acquire(ctx context.Context, request theater.InventoryRequest) (any, error) {
	httpRequest, err := builtinhttp.RequestFromArgs(request.Args)
	if err != nil {
		return nil, err
	}

	response, err := builtinhttp.Do(ctx, request.Resources, request.HTTP, httpRequest)
	if err != nil {
		return nil, err
	}

	return response.Body, nil
}
