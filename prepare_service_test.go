package theater

import (
	"context"
	"testing"
	"time"
)

func TestValidatorSeparatesCompileValidateAndPrepare(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "login",
				Acts: []ActSpec{
					{
						ID:         "submit",
						Eventually: &EventuallySpec{Timeout: "10ms", Interval: "1ms"},
						Action: ActionSpec{
							Use:        "action.login",
							Repeatable: true,
						},
						Properties: map[string]PropertySpec{
							"payload": {
								Inventory: &InventoryCall{Use: "inventory.payload"},
								Decorators: []DecoratorSpec{
									{Use: "decorator.normalize"},
								},
							},
						},
						Expectations: []ExpectationSpec{
							{
								ID:      "token",
								Subject: SubjectSpec{Field: "token"},
								Assert:  AssertSpec{Ref: "expectation.token"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	catalog := NewCatalog()
	if err := catalog.RegisterAction("action.login", prepareTestAction{
		contract: ActionContract{
			Outputs: map[string]ValueContract{
				"token": {Kind: ValueKindString},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	if err := catalog.RegisterInventory("inventory.payload", prepareTestInventory{
		contract: InventoryContract{
			Produces: ValueContract{Kind: ValueKindString},
		},
	}); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}

	decorator := prepareTestDecorator{
		contract: DecoratorContract{
			Accepts:  ValueContract{Kind: ValueKindString},
			Produces: ValueContract{Kind: ValueKindString},
			Params: []ParamSpec{
				{Name: "prefix", Accepts: ValueContract{Kind: ValueKindString}, Default: "token:"},
				{Name: "enabled", Accepts: ValueContract{Kind: ValueKindBool}, Default: true},
			},
		},
	}
	if err := catalog.RegisterDecorator("decorator.normalize", decorator.Definition()); err != nil {
		t.Fatalf("register decorator failed: %v", err)
	}

	matchers, err := NewMatcherCatalog(MatcherDescriptor{
		Ref:    "expectation.token",
		Actual: ValueContract{Kind: ValueKindString},
		Sugar:  SugarSpec{Form: SugarFormNone},
		Compile: func(MatcherCompileContext, Values) (Matcher, error) {
			return prepareTestMatcher{}, nil
		},
	})
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	validator := NewValidator(catalog, matchers)
	compiled := validator.compile(spec)
	diagnostics := validator.validate(context.Background(), compiled)
	if got, want := len(diagnostics), 0; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}

	rawAct := compiled.Scenarios[0].Acts[0]
	if got := rawAct.Eventually.Timeout; got != 0 {
		t.Fatalf("compiled timeout must stay unset, got %s", got)
	}

	if got := rawAct.Eventually.Interval; got != 0 {
		t.Fatalf("compiled interval must stay unset, got %s", got)
	}

	if got := rawAct.Expectations[0].Matcher.Ref; got != "" {
		t.Fatalf("compiled matcher must stay unset, got %q", got)
	}

	if got := rawAct.Properties[0].Decorators[0].Transform; got != nil {
		t.Fatal("compiled decorator must stay unprepared")
	}

	if _, ok := rawAct.Properties[0].Decorators[0].With["comma"]; ok {
		t.Fatal("compiled decorator defaults must stay unapplied")
	}

	prepared, err := validator.prepare(compiled)
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	if prepared != compiled {
		t.Fatal("prepare must reuse the compiled plan pointer")
	}

	preparedAct := prepared.Scenarios[0].Acts[0]
	if got, want := preparedAct.Eventually.Timeout, 10*time.Millisecond; got != want {
		t.Fatalf("prepared timeout mismatch: got %s want %s", got, want)
	}

	if got, want := preparedAct.Eventually.Interval, time.Millisecond; got != want {
		t.Fatalf("prepared interval mismatch: got %s want %s", got, want)
	}

	if got, want := preparedAct.Expectations[0].Matcher.Ref, "expectation.token"; got != want {
		t.Fatalf("prepared matcher ref mismatch: got %q want %q", got, want)
	}

	preparedDecorator := preparedAct.Properties[0].Decorators[0]
	if preparedDecorator.Transform == nil {
		t.Fatal("prepared decorator transform is nil")
	}

	if got, want := preparedDecorator.With["prefix"], "token:"; got != want {
		t.Fatalf("prepared decorator default mismatch: got %#v want %#v", got, want)
	}

	if got, want := preparedDecorator.With["enabled"], true; got != want {
		t.Fatalf("prepared decorator bool default mismatch: got %#v want %#v", got, want)
	}

	if got, want := compiled.Scenarios[0].Acts[0].Eventually.Timeout, 10*time.Millisecond; got != want {
		t.Fatalf("compiled timeout mismatch after prepare: got %s want %s", got, want)
	}

	if got, want := compiled.Scenarios[0].Acts[0].Expectations[0].Matcher.Ref, "expectation.token"; got != want {
		t.Fatalf("compiled matcher mismatch after prepare: got %q want %q", got, want)
	}

	if got, want := compiled.Scenarios[0].Acts[0].Properties[0].Decorators[0].With["prefix"], "token:"; got != want {
		t.Fatalf("compiled decorator defaults mismatch after prepare: got %#v want %#v", got, want)
	}
}

type prepareTestAction struct {
	contract ActionContract
}

type prepareTestInventory struct {
	contract InventoryContract
}

type prepareTestDecorator struct {
	contract DecoratorContract
}

type prepareTestMatcher struct{}

func (a prepareTestAction) Contract() ActionContract {
	return a.contract
}

func (a prepareTestAction) Run(context.Context, ActionRequest) (Outputs, error) {
	return Outputs{"token": "issued-token"}, nil
}

func (i prepareTestInventory) Contract() InventoryContract {
	return i.contract
}

func (i prepareTestInventory) Acquire(context.Context, InventoryRequest) (any, error) {
	return "payload", nil
}

func (d prepareTestDecorator) Definition() DecoratorDef {
	return DecoratorDef{
		Contract: d.contract,
		Compile: func(args Values) (DecoratorFunc, error) {
			return func(value any) (any, error) {
				return value, nil
			}, nil
		},
	}
}

func (prepareTestMatcher) Check(context.Context, any) error {
	return nil
}
