package testkit

import (
	"context"
	"reflect"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/runtimevalue"
)

type ScriptedAction struct {
	CheckFunc     func(args theater.Args) error
	RunFunc       func(args theater.Args) (theater.Outputs, error)
	ContractValue theater.ActionContract
	Err           error
	Output        theater.Outputs
	Calls         []theater.Args
}

type ScriptedInventory struct {
	ContractValue theater.InventoryContract
	AcquireFunc   func(request theater.InventoryRequest) (any, error)
	Err           error
	Output        any
	Calls         []theater.InventoryRequest
}

type ScriptedDecorator struct {
	ContractValue theater.DecoratorContract
	CompileFunc   func(args theater.Values) (theater.DecoratorFunc, error)
	TransformFunc func(value any) (any, error)
	Err           error
	Output        any
	CompileCalls  []theater.Values
	Calls         []any
}

type ScriptedExpectation struct {
	CompileFunc  func(args theater.Values) (theater.Matcher, error)
	CheckFunc    func(actual any) error
	CompileCalls []theater.Values
	Calls        []any
}

func TerminalError(err error) error {
	if err == nil {
		return nil
	}

	return terminalError{err: err}
}

func (s *ScriptedAction) Contract() theater.ActionContract {
	if len(s.ContractValue.Outputs) != 0 || len(s.ContractValue.Inputs) != 0 {
		return cloneActionContract(s.ContractValue)
	}

	return theater.ActionContract{Outputs: inferOutputContracts(s.Output)}
}

func (s *ScriptedAction) Run(_ context.Context, request theater.ActionRequest) (theater.Outputs, error) {
	s.Calls = append(s.Calls, cloneArgs(request.Args))

	if s.CheckFunc != nil {
		if err := s.CheckFunc(request.Args); err != nil {
			return theater.Outputs{}, err
		}
	}

	if s.RunFunc != nil {
		return s.RunFunc(request.Args)
	}

	if s.Err != nil {
		return theater.Outputs{}, s.Err
	}

	return cloneOutputs(s.Output), nil
}

func (s *ScriptedExpectation) Descriptor(ref string) theater.MatcherDescriptor {
	return theater.MatcherDescriptor{
		Ref:    ref,
		Actual: theater.ValueContract{Kind: theater.ValueKindAny},
		Sugar:  theater.SugarSpec{Form: theater.SugarFormNone},
		Compile: func(_ theater.MatcherCompileContext, args theater.Values) (theater.Matcher, error) {
			s.CompileCalls = append(s.CompileCalls, cloneValues(args))
			if s.CompileFunc != nil {
				return s.CompileFunc(args)
			}

			return scriptedMatcher{owner: s}, nil
		},
	}
}

func (s *ScriptedInventory) Acquire(_ context.Context, request theater.InventoryRequest) (any, error) {
	s.Calls = append(s.Calls, cloneInventoryRequest(request))

	if s.AcquireFunc != nil {
		return s.AcquireFunc(request)
	}

	if s.Err != nil {
		return nil, s.Err
	}

	return s.Output, nil
}

func (s *ScriptedInventory) Contract() theater.InventoryContract {
	if s.ContractValue.Produces.Valid() {
		return cloneInventoryContract(s.ContractValue)
	}

	return theater.InventoryContract{
		Produces: inferValueContract(s.Output),
	}
}

func (s *ScriptedDecorator) Definition() theater.DecoratorDef {
	contract := cloneDecoratorContract(s.ContractValue)
	if !contract.Accepts.Valid() {
		contract.Accepts = theater.ValueContract{Kind: theater.ValueKindAny}
	}
	if !contract.Produces.Valid() {
		contract.Produces = inferValueContract(s.Output)
	}

	return theater.DecoratorDef{
		Contract: contract,
		Compile: func(args theater.Values) (theater.DecoratorFunc, error) {
			s.CompileCalls = append(s.CompileCalls, cloneValues(args))
			if s.CompileFunc != nil {
				return s.CompileFunc(args)
			}

			return func(value any) (any, error) {
				s.Calls = append(s.Calls, runtimevalue.Clone(value))
				if s.TransformFunc != nil {
					return s.TransformFunc(value)
				}
				if s.Err != nil {
					return nil, s.Err
				}

				return runtimevalue.Clone(s.Output), nil
			}, nil
		},
	}
}

type scriptedMatcher struct {
	owner *ScriptedExpectation
}

type terminalError struct {
	err error
}

func (m scriptedMatcher) Check(_ context.Context, actual any) error {
	m.owner.Calls = append(m.owner.Calls, actual)
	if m.owner.CheckFunc == nil {
		return nil
	}

	return m.owner.CheckFunc(actual)
}

func (e terminalError) Error() string {
	return e.err.Error()
}

func (e terminalError) Unwrap() error {
	return e.err
}

func (e terminalError) TheaterTerminal() bool {
	return true
}

func cloneArgs(args theater.Args) theater.Args {
	if args == nil {
		return nil
	}

	cloned := make(theater.Args, len(args))
	for key, value := range args {
		cloned[key] = value
	}

	return cloned
}

func cloneOutputs(outputs theater.Outputs) theater.Outputs {
	if outputs == nil {
		return nil
	}

	cloned := make(theater.Outputs, len(outputs))
	for key, value := range outputs {
		cloned[key] = value
	}

	return cloned
}

func cloneValues(values theater.Values) theater.Values {
	if values == nil {
		return nil
	}

	cloned := make(theater.Values, len(values))
	for key, value := range values {
		cloned[key] = value
	}

	return cloned
}

func cloneActionContract(contract theater.ActionContract) theater.ActionContract {
	return theater.ActionContract{
		Inputs:  cloneValueContracts(contract.Inputs),
		Outputs: cloneValueContracts(contract.Outputs),
	}
}

func cloneInventoryContract(contract theater.InventoryContract) theater.InventoryContract {
	cloned := theater.InventoryContract{
		Summary:  contract.Summary,
		Produces: contract.Produces.Clone(),
	}

	if len(contract.Args) != 0 {
		cloned.Args = make([]theater.ArgSpec, len(contract.Args))
		for i := range contract.Args {
			cloned.Args[i] = theater.ArgSpec{
				Name:        contract.Args[i].Name,
				Accepts:     contract.Args[i].Accepts.Clone(),
				Required:    contract.Args[i].Required,
				Description: contract.Args[i].Description,
			}
		}
	}

	return cloned
}

func cloneInventoryRequest(request theater.InventoryRequest) theater.InventoryRequest {
	return theater.InventoryRequest{
		Args:      cloneArgs(request.Args),
		Paths:     request.Paths,
		Resources: request.Resources,
	}
}

func cloneDecoratorContract(contract theater.DecoratorContract) theater.DecoratorContract {
	cloned := theater.DecoratorContract{
		Accepts:  contract.Accepts.Clone(),
		Produces: contract.Produces.Clone(),
		Summary:  contract.Summary,
	}

	if len(contract.Params) != 0 {
		cloned.Params = make([]theater.ParamSpec, len(contract.Params))
		for i := range contract.Params {
			cloned.Params[i] = theater.ParamSpec{
				Name:        contract.Params[i].Name,
				Accepts:     contract.Params[i].Accepts.Clone(),
				Required:    contract.Params[i].Required,
				Default:     runtimevalue.Clone(contract.Params[i].Default),
				Enum:        runtimevalue.CloneSlice(contract.Params[i].Enum),
				Description: contract.Params[i].Description,
			}
		}
	}

	return cloned
}

func cloneValueContracts(specs map[string]theater.ValueContract) map[string]theater.ValueContract {
	if len(specs) == 0 {
		return nil
	}

	cloned := make(map[string]theater.ValueContract, len(specs))
	for key, spec := range specs {
		cloned[key] = cloneValueContract(spec)
	}

	return cloned
}

func cloneValueContract(spec theater.ValueContract) theater.ValueContract {
	cloned := spec
	cloned.Fields = cloneValueContracts(spec.Fields)
	if spec.Elem != nil {
		elem := cloneValueContract(*spec.Elem)
		cloned.Elem = &elem
	}

	return cloned
}

func inferOutputContracts(outputs theater.Outputs) map[string]theater.ValueContract {
	if len(outputs) == 0 {
		return map[string]theater.ValueContract{}
	}

	specs := make(map[string]theater.ValueContract, len(outputs))
	for key, value := range outputs {
		specs[key] = inferValueContract(value)
	}

	return specs
}

func inferValueContract(value any) theater.ValueContract {
	switch value.(type) {
	case []byte:
		return theater.ValueContract{Kind: theater.ValueKindBytes}
	case string:
		return theater.ValueContract{Kind: theater.ValueKindString}
	case bool:
		return theater.ValueContract{Kind: theater.ValueKindBool}
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return theater.ValueContract{Kind: theater.ValueKindNumber}
	default:
		if value == nil {
			return theater.ValueContract{Kind: theater.ValueKindNull}
		}

		switch reflect.TypeOf(value).Kind() {
		case reflect.Map:
			return theater.ValueContract{Kind: theater.ValueKindObject}
		case reflect.Slice, reflect.Array:
			return theater.ValueContract{Kind: theater.ValueKindList}
		default:
			return theater.ValueContract{Kind: theater.ValueKindString}
		}
	}
}
