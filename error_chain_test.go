package theater

import (
	"encoding/json"
	"errors"
	"strconv"
	"testing"
)

func TestPlanPreparerPreservesDecoratorCompileCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("compile decorator failed")

	catalog := NewCatalog()
	if err := catalog.RegisterInventory("inventory.seed", noopInventory{}); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}

	if err := catalog.RegisterDecorator("decorator.fail", DecoratorDef{
		Contract: DecoratorContract{
			Accepts:  ValueContract{Kind: ValueKindAny},
			Produces: ValueContract{Kind: ValueKindAny},
		},
		Compile: func(Values) (DecoratorFunc, error) {
			return nil, cause
		},
	}); err != nil {
		t.Fatalf("register decorator failed: %v", err)
	}

	stage := compileStageSpec(StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "login",
				Acts: []ActSpec{
					{
						ID: "submit",
						Properties: map[string]PropertySpec{
							"seed": {
								Inventory: &InventoryCall{Use: "inventory.seed"},
								Decorators: []DecoratorSpec{
									{Use: "decorator.fail"},
								},
							},
						},
					},
				},
			},
		},
	})

	_, err := planPreparer{catalog: catalog}.Prepare(stage)
	if err == nil {
		t.Fatal("expected prepare error, got nil")
	}

	var prepErr planPreparationError
	if !errors.As(err, &prepErr) {
		t.Fatalf("expected planPreparationError, got %T", err)
	}

	if !errors.Is(err, cause) {
		t.Fatalf("expected errors.Is to match compile cause, got %v", err)
	}
}

func TestValidateSelectorContractPreservesIndexParseCause(t *testing.T) {
	t.Parallel()

	err := validateSelectorContract(
		selectorPlan{Path: JSONPointer("/bad")},
		ValueContract{Kind: ValueKindList},
	)
	if err == nil {
		t.Fatal("expected selector contract error, got nil")
	}

	if got, want := err.Error(), `path "/bad" token "bad" must be array index`; got != want {
		t.Fatalf("error message mismatch: got %q want %q", got, want)
	}

	var contractErr selectorContractError
	if !errors.As(err, &contractErr) {
		t.Fatalf("expected selectorContractError, got %T", err)
	}

	var numErr *strconv.NumError
	if !errors.As(err, &numErr) {
		t.Fatalf("expected strconv.NumError in error chain, got %v", err)
	}
}

func TestResolveSubjectPreservesIndexParseCause(t *testing.T) {
	t.Parallel()

	_, err := newReferenceResolver(Values{
		"items": []any{"first"},
	}).ResolveSubject(subjectPlan{
		Field: "items",
		selectorPlan: selectorPlan{
			Path: JSONPointer("/bad"),
		},
	})
	if err == nil {
		t.Fatal("expected resolve subject error, got nil")
	}

	if got, want := err.Error(), `path token "bad" must be array index`; got != want {
		t.Fatalf("error message mismatch: got %q want %q", got, want)
	}

	var numErr *strconv.NumError
	if !errors.As(err, &numErr) {
		t.Fatalf("expected strconv.NumError in error chain, got %v", err)
	}
}

func TestValidateResolvedContractPreservesNumberParseCause(t *testing.T) {
	t.Parallel()

	err := validateResolvedContract("count", ValueContract{Kind: ValueKindNumber}, json.Number("not-a-number"))
	if err == nil {
		t.Fatal("expected contract validation error, got nil")
	}

	if got, want := err.Error(), "count expects number, got json.Number"; got != want {
		t.Fatalf("error message mismatch: got %q want %q", got, want)
	}

	var numErr *strconv.NumError
	if !errors.As(err, &numErr) {
		t.Fatalf("expected strconv.NumError in error chain, got %v", err)
	}
}
