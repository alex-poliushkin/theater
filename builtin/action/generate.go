package action

import (
	"context"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/runtimevalue"
)

type generateAction struct{}

func (generateAction) Contract() theater.ActionContract {
	return theater.ActionContract{
		Inputs: map[string]theater.ValueContract{
			"outputs": {
				Kind:     theater.ValueKindObject,
				Required: true,
				Elem:     &theater.ValueContract{Kind: theater.ValueKindAny},
			},
		},
		Outputs: map[string]theater.ValueContract{
			"values": {
				Kind: theater.ValueKindObject,
				Elem: &theater.ValueContract{Kind: theater.ValueKindAny},
			},
		},
	}
}

func (generateAction) Run(_ context.Context, request theater.ActionRequest) (theater.Outputs, error) {
	return theater.Outputs{
		"values": runtimevalue.Clone(request.Args["outputs"]),
	}, nil
}
