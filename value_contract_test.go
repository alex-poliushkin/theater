package theater

import (
	"context"
	"encoding/json"
	"testing"
)

func TestValueContractCloneDeepCopiesNestedContracts(t *testing.T) {
	t.Parallel()

	original := ValueContract{
		Kinds: NewValueKindSet(ValueKindObject, ValueKindList),
		Fields: map[string]ValueContract{
			"name": {Kind: ValueKindString},
		},
		Elem: &ValueContract{Kind: ValueKindNumber},
	}

	if !original.Valid() {
		t.Fatal("expected original contract to be valid")
	}
	if !original.Supports(ValueKindObject) || !original.Supports(ValueKindList) {
		t.Fatal("expected union kinds to be supported")
	}

	cloned := original.Clone()
	cloned.Kinds[ValueKindBool] = struct{}{}
	field := cloned.Fields["name"]
	field.Kind = ValueKindBool
	cloned.Fields["name"] = field
	cloned.Elem.Kind = ValueKindString

	if original.Supports(ValueKindBool) {
		t.Fatal("expected clone kind mutation not to leak into original")
	}
	if got, want := original.Fields["name"].Kind, ValueKindString; got != want {
		t.Fatalf("field clone mismatch: got %q want %q", got, want)
	}
	if original.Elem == nil {
		t.Fatal("expected original elem contract to stay present")
	}
	if got, want := original.Elem.Kind, ValueKindNumber; got != want {
		t.Fatalf("elem clone mismatch: got %q want %q", got, want)
	}
}

func TestValueKindSetJSONUsesArrayShape(t *testing.T) {
	t.Parallel()

	contract := ValueContract{
		Kinds: NewValueKindSet(ValueKindString, ValueKindObject),
		Fields: map[string]ValueContract{
			"otp": {Kind: ValueKindString, Required: true},
		},
	}

	raw, err := json.Marshal(contract)
	if err != nil {
		t.Fatalf("marshal value contract: %v", err)
	}
	if got, want := string(raw), `{"kinds":["string","object"],"fields":{"otp":{"type":"string","required":true}}}`; got != want {
		t.Fatalf("value contract JSON mismatch: got %s want %s", got, want)
	}

	var decoded ValueContract
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal value contract: %v", err)
	}
	if !decoded.Supports(ValueKindString) || !decoded.Supports(ValueKindObject) {
		t.Fatalf("decoded contract must support string and object: %#v", decoded)
	}
	if field := decoded.Fields["otp"]; !field.Required || field.Kind != ValueKindString {
		t.Fatalf("decoded object field contract mismatch: %#v", field)
	}
}

func TestValueKindSetJSONReadsLegacyObjectShape(t *testing.T) {
	t.Parallel()

	var contract ValueContract
	if err := json.Unmarshal([]byte(`{"kinds":{"object":{},"string":{}}}`), &contract); err != nil {
		t.Fatalf("unmarshal legacy value contract: %v", err)
	}

	if !contract.Supports(ValueKindString) || !contract.Supports(ValueKindObject) {
		t.Fatalf("legacy contract must support string and object: %#v", contract)
	}
}

func TestValidateDecoratorContractAcceptsValueContractParams(t *testing.T) {
	t.Parallel()

	contract := DecoratorContract{
		Accepts:  ValueContract{Kinds: NewValueKindSet(ValueKindString, ValueKindBytes)},
		Produces: ValueContract{Kind: ValueKindList},
		Params: []ParamSpec{
			{Name: "comma", Accepts: ValueContract{Kind: ValueKindString}},
			{Name: "trim", Accepts: ValueContract{Kind: ValueKindBool}},
		},
	}

	if err := validateDecoratorContract(contract); err != nil {
		t.Fatalf("expected decorator contract to validate, got %v", err)
	}
}

func TestContractCompatibilityRejectsMissingRequiredFieldGuarantee(t *testing.T) {
	t.Parallel()

	err := contractCompatibilityError(
		ValueContract{
			Kind: ValueKindObject,
			Fields: map[string]ValueContract{
				"token": {Kind: ValueKindString},
			},
		},
		ValueContract{
			Kind: ValueKindObject,
			Fields: map[string]ValueContract{
				"token": {Kind: ValueKindString, Required: true},
			},
		},
	)
	if err == nil {
		t.Fatal("expected structural compatibility error")
	}

	if got, want := err.Error(), `required field "token" is not guaranteed`; got != want {
		t.Fatalf("compatibility error mismatch: got %q want %q", got, want)
	}
}

func TestContractCompatibilityRejectsListElemMismatch(t *testing.T) {
	t.Parallel()

	err := contractCompatibilityError(
		ValueContract{
			Kind: ValueKindList,
			Elem: &ValueContract{Kind: ValueKindString},
		},
		ValueContract{
			Kind: ValueKindList,
			Elem: &ValueContract{Kind: ValueKindNumber},
		},
	)
	if err == nil {
		t.Fatal("expected structural compatibility error")
	}

	if got, want := err.Error(), `list elements: kind "string" is not accepted`; got != want {
		t.Fatalf("compatibility error mismatch: got %q want %q", got, want)
	}
}

func TestContractCompatibilityRejectsObjectElemAgainstNamedField(t *testing.T) {
	t.Parallel()

	err := contractCompatibilityError(
		ValueContract{
			Kind: ValueKindObject,
			Elem: &ValueContract{Kind: ValueKindNumber},
		},
		ValueContract{
			Kind: ValueKindObject,
			Fields: map[string]ValueContract{
				"token": {Kind: ValueKindString},
			},
			Elem: &ValueContract{Kind: ValueKindNumber},
		},
	)
	if err == nil {
		t.Fatal("expected structural compatibility error")
	}

	if got, want := err.Error(), `field "token" via elem: kind "number" is not accepted`; got != want {
		t.Fatalf("compatibility error mismatch: got %q want %q", got, want)
	}
}

func TestValidateDecoratorContractRejectsInvalidDefault(t *testing.T) {
	t.Parallel()

	contract := DecoratorContract{
		Accepts:  ValueContract{Kind: ValueKindAny},
		Produces: ValueContract{Kind: ValueKindAny},
		Params: []ParamSpec{
			{Name: "comma", Accepts: ValueContract{Kind: ValueKindString}, Default: 1},
		},
	}

	err := validateDecoratorContract(contract)
	if err == nil {
		t.Fatal("expected invalid default error")
	}

	if got, want := err.Error(), `decorator param "comma" default is invalid: comma expects string, got int`; got != want {
		t.Fatalf("default validation mismatch: got %q want %q", got, want)
	}
}

func TestPrepareDecoratorParamsAppliesDefaults(t *testing.T) {
	t.Parallel()

	resolved, diagnostics := prepareDecoratorParams(
		"stage.main/scenario.probe/act.fetch/property.payload/decorator.csv~2decode",
		Values{
			"comment": "#",
		},
		[]ParamSpec{
			{Name: "comma", Accepts: ValueContract{Kind: ValueKindString}, Default: ","},
			{Name: "comment", Accepts: ValueContract{Kind: ValueKindString}},
			{Name: "trim_leading_space", Accepts: ValueContract{Kind: ValueKindBool}, Default: false},
		},
	)
	if len(diagnostics) != 0 {
		t.Fatalf("expected no diagnostics, got %v", diagnostics)
	}

	if got, want := resolved["comma"], ","; got != want {
		t.Fatalf("comma default mismatch: got %v want %v", got, want)
	}
	if got, want := resolved["comment"], "#"; got != want {
		t.Fatalf("comment mismatch: got %v want %v", got, want)
	}
	if got, want := resolved["trim_leading_space"], false; got != want {
		t.Fatalf("trim default mismatch: got %v want %v", got, want)
	}
}

func TestCatalogDecoratorCompileAppliesParamDefaults(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog()
	if err := catalog.RegisterDecorator("decorate.append", DecoratorDef{
		Contract: DecoratorContract{
			Accepts:  ValueContract{Kind: ValueKindString},
			Produces: ValueContract{Kind: ValueKindString},
			Params: []ParamSpec{
				{Name: "suffix", Accepts: ValueContract{Kind: ValueKindString}, Default: "!"},
			},
		},
		Compile: func(args Values) (DecoratorFunc, error) {
			if got, want := args["suffix"], "!"; got != want {
				t.Fatalf("compile args mismatch: got %v want %v", got, want)
			}

			return func(value any) (any, error) {
				return value.(string) + args["suffix"].(string), nil
			}, nil
		},
	}); err != nil {
		t.Fatalf("register decorator failed: %v", err)
	}

	def, err := catalog.ResolveDecorator("decorate.append")
	if err != nil {
		t.Fatalf("resolve decorator failed: %v", err)
	}

	transform, err := def.Compile(nil)
	if err != nil {
		t.Fatalf("compile decorator failed: %v", err)
	}

	value, err := transform("ok")
	if err != nil {
		t.Fatalf("transform failed: %v", err)
	}

	if got, want := value, "ok!"; got != want {
		t.Fatalf("transformed value mismatch: got %v want %v", got, want)
	}
}

func TestValidateExpectationArgsUsesMatcherArgAccepts(t *testing.T) {
	t.Parallel()

	expectation := &expectationPlan{
		Assert: assertPlan{
			Ref: "expectation.example",
			Args: map[string]bindingPlan{
				"expected": {Kind: BindingKindLiteral, Value: "ok"},
			},
		},
	}
	descriptor := MatcherDescriptor{
		Ref:    "expectation.example",
		Args:   []MatcherArg{{Name: "expected", Required: true, Accepts: ValueContract{Kind: ValueKindString}}},
		Actual: ValueContract{Kind: ValueKindAny},
		Compile: func(MatcherCompileContext, Values) (Matcher, error) {
			return noopMatcher{}, nil
		},
	}

	if diagnostics := validateExpectationArgs("stage.main/expectation.example", expectation, descriptor, nil, nil, nil); len(diagnostics) != 0 {
		t.Fatalf("expected no diagnostics, got %v", diagnostics)
	}
}

func TestValidateBindingContractSupportsObjectFieldsAndElemTogether(t *testing.T) {
	t.Parallel()

	binding := bindingPlan{
		Kind: BindingKindObject,
		Object: map[string]bindingPlan{
			"token":   {Kind: BindingKindLiteral, Value: "issued-token"},
			"attempt": {Kind: BindingKindLiteral, Value: 2},
		},
	}
	spec := ValueContract{
		Kind: ValueKindObject,
		Fields: map[string]ValueContract{
			"token": {Kind: ValueKindString, Required: true},
		},
		Elem: &ValueContract{Kind: ValueKindNumber},
	}

	if err := validateBindingContractWithResolver(nil, nil, nil, binding, spec); err != nil {
		t.Fatalf("expected object binding to satisfy fields and elem, got %v", err)
	}
}

func TestValidateBindingContractRejectsUndeclaredFieldWithoutObjectElem(t *testing.T) {
	t.Parallel()

	binding := bindingPlan{
		Kind: BindingKindObject,
		Object: map[string]bindingPlan{
			"token": {Kind: BindingKindLiteral, Value: "issued-token"},
			"role":  {Kind: BindingKindLiteral, Value: "admin"},
		},
	}
	spec := ValueContract{
		Kind: ValueKindObject,
		Fields: map[string]ValueContract{
			"token": {Kind: ValueKindString},
		},
	}

	err := validateBindingContractWithResolver(nil, nil, nil, binding, spec)
	if err == nil {
		t.Fatal("expected undeclared field error")
	}

	if got, want := err.Error(), `field "role" is not declared`; got != want {
		t.Fatalf("binding error mismatch: got %q want %q", got, want)
	}
}

type noopMatcher struct{}

func (noopMatcher) Check(context.Context, any) error {
	return nil
}
