package decorator

import (
	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/runtimevalue"
)

func jsonDecoratorDef() theater.DecoratorDef {
	return theater.DecoratorDef{
		Contract: theater.DecoratorContract{
			Accepts: theater.ValueContract{
				Kinds: theater.NewValueKindSet(theater.ValueKindString, theater.ValueKindBytes),
			},
			Produces: theater.ValueContract{
				Kinds: theater.NewValueKindSet(
					theater.ValueKindBool,
					theater.ValueKindNumber,
					theater.ValueKindString,
					theater.ValueKindObject,
					theater.ValueKindList,
					theater.ValueKindNull,
				),
			},
			Summary: "decode a JSON document into structured values",
		},
		Compile: func(_ theater.Values) (theater.DecoratorFunc, error) {
			return func(value any) (any, error) {
				return runtimevalue.DecodeJSON(value, "value")
			}, nil
		},
	}
}
