package action

import (
	"context"
	"testing"

	"github.com/alex-poliushkin/theater"
)

func TestGenerateActionReturnsClonedValuesObject(t *testing.T) {
	t.Parallel()

	request := theater.ActionRequest{
		Args: theater.Args{
			"outputs": map[string]any{
				"email": "demo@example.test",
			},
		},
	}

	outputs, err := generateAction{}.Run(context.Background(), request)
	if err != nil {
		t.Fatalf("run generate action failed: %v", err)
	}

	values, ok := outputs["values"].(map[string]any)
	if !ok {
		t.Fatalf("values output type mismatch: got %T", outputs["values"])
	}

	values["email"] = "changed@example.test"

	if got, want := request.Args["outputs"].(map[string]any)["email"], "demo@example.test"; got != want {
		t.Fatalf("input clone mismatch: got %v want %v", got, want)
	}
}
