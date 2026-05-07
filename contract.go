package theater

import (
	specmodel "github.com/alex-poliushkin/theater/spec"
	statemodel "github.com/alex-poliushkin/theater/state"
)

// Supported canonical runtime value kinds.
const (
	ValueKindAny    ValueKind = specmodel.ValueKindAny
	ValueKindBytes  ValueKind = specmodel.ValueKindBytes
	ValueKindString ValueKind = specmodel.ValueKindString
	ValueKindNumber ValueKind = specmodel.ValueKindNumber
	ValueKindBool   ValueKind = specmodel.ValueKindBool
	ValueKindObject ValueKind = specmodel.ValueKindObject
	ValueKindList   ValueKind = specmodel.ValueKindList
	ValueKindNull   ValueKind = specmodel.ValueKindNull
)

// ValueKind identifies the canonical runtime shape of a value.
type ValueKind = specmodel.ValueKind

// ValueKindSet is a set of allowed runtime value kinds.
type ValueKindSet = specmodel.ValueKindSet

// ValueContract describes accepted or produced runtime values, including
// optional structure and diagnostic visibility policy.
type ValueContract = specmodel.ValueContract

// ActionContract describes the declared inputs and outputs of an action.
type ActionContract = specmodel.ActionContract

// ArgSpec describes one inventory call-site argument.
type ArgSpec = specmodel.ArgSpec

// InventoryContract describes inventory args and the value contract it
// produces.
type InventoryContract = specmodel.InventoryContract

// ParamSpec describes one decorator configuration parameter.
type ParamSpec = specmodel.ParamSpec

// DecoratorContract describes the input, output, and configuration surface of a
// decorator.
type DecoratorContract = specmodel.DecoratorContract

// Values is a generic named value bag used by decorators and matchers.
type Values = specmodel.Values

// Args is the runtime argument bag passed to actions and inventories.
type Args = specmodel.Args

// Outputs is the named output bag produced by an action.
type Outputs = specmodel.Outputs

// PathContext carries runtime paths that help adapters attribute work to the
// current stage, scenario, act, and property.
type PathContext = specmodel.PathContext

// InventoryRequest is the runtime request passed to an inventory acquisition.
type InventoryRequest struct {
	Args      Args                `json:"args,omitempty"`
	HTTP      *HTTPSpec           `json:"http,omitempty"`
	State     *statemodel.Manager `json:"-"`
	Paths     PathContext         `json:"paths,omitempty"`
	Attempt   int                 `json:"attempt,omitempty"`
	Resources ResourceScope       `json:"-"`
}

// DecoratorFunc transforms one value after decorator compilation.
type DecoratorFunc = specmodel.DecoratorFunc

// DecoratorDef registers a decorator contract and compile function.
type DecoratorDef = specmodel.DecoratorDef

// NewValueKindSet builds a set containing the provided kinds.
func NewValueKindSet(kinds ...ValueKind) ValueKindSet {
	return specmodel.NewValueKindSet(kinds...)
}
