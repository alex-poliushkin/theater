package spec

import reportmodel "github.com/alex-poliushkin/theater/report"

// Supported canonical runtime value kinds.
const (
	SensitivityNone     Sensitivity = reportmodel.SensitivityNone
	SensitivityInternal Sensitivity = reportmodel.SensitivityInternal
	SensitivityPersonal Sensitivity = reportmodel.SensitivityPersonal
	SensitivitySecret   Sensitivity = reportmodel.SensitivitySecret

	CaptureOmit        Capture = reportmodel.CaptureOmit
	CaptureSummary     Capture = reportmodel.CaptureSummary
	CaptureArtifactRef Capture = reportmodel.CaptureArtifactRef

	ValueKindAny    ValueKind = "any"
	ValueKindBytes  ValueKind = "bytes"
	ValueKindString ValueKind = "string"
	ValueKindNumber ValueKind = "number"
	ValueKindBool   ValueKind = "bool"
	ValueKindObject ValueKind = "object"
	ValueKindList   ValueKind = "list"
	ValueKindNull   ValueKind = "null"
)

// Sensitivity classifies how a value should be treated in diagnostics and
// report payloads.
type Sensitivity = reportmodel.Sensitivity

// Capture defines how much of a value may appear in previews or artifact
// payloads.
type Capture = reportmodel.Capture

// ValueKind identifies the canonical runtime shape of a value.
type ValueKind string

// ValueKindSet is a set of allowed runtime value kinds.
type ValueKindSet map[ValueKind]struct{}

// ValueContract describes accepted or produced runtime values, including
// optional structure and diagnostic visibility policy.
type ValueContract struct {
	Kind        ValueKind                `yaml:"type,omitempty" json:"type,omitempty"`
	Kinds       ValueKindSet             `json:"kinds,omitempty"`
	Required    bool                     `yaml:"required,omitempty" json:"required,omitempty"`
	Description string                   `yaml:"description,omitempty" json:"description,omitempty"`
	Sensitivity Sensitivity              `yaml:"sensitivity,omitempty" json:"sensitivity,omitempty"`
	Capture     Capture                  `yaml:"capture,omitempty" json:"capture,omitempty"`
	Fields      map[string]ValueContract `yaml:"fields,omitempty" json:"fields,omitempty"`
	Elem        *ValueContract           `yaml:"elem,omitempty" json:"elem,omitempty"`
}

// ActionContract describes the declared inputs and outputs of an action.
type ActionContract struct {
	Inputs  map[string]ValueContract `json:"inputs"`
	Outputs map[string]ValueContract `json:"outputs"`
}

// ArgSpec describes one inventory call-site argument.
type ArgSpec struct {
	Name        string        `json:"name"`
	Accepts     ValueContract `json:"accepts"`
	Required    bool          `json:"required,omitempty"`
	Description string        `json:"description,omitempty"`
}

// InventoryContract describes inventory args and the value contract it
// produces.
type InventoryContract struct {
	Summary  string        `json:"summary,omitempty"`
	Args     []ArgSpec     `json:"args,omitempty"`
	Produces ValueContract `json:"produces"`
}

// ParamSpec describes one decorator configuration parameter.
type ParamSpec struct {
	Name        string        `json:"name"`
	Accepts     ValueContract `json:"accepts"`
	Required    bool          `json:"required,omitempty"`
	Default     any           `json:"default,omitempty"`
	Enum        []any         `json:"enum,omitempty"`
	Description string        `json:"description,omitempty"`
}

// DecoratorContract describes the input, output, and configuration surface of a
// decorator.
type DecoratorContract struct {
	Accepts  ValueContract `json:"accepts"`
	Produces ValueContract `json:"produces"`
	Params   []ParamSpec   `json:"params,omitempty"`
	Summary  string        `json:"summary,omitempty"`
}

// Values is a generic named value bag used by decorators and matchers.
type Values map[string]any

// Args is the runtime argument bag passed to actions and inventories.
type Args map[string]any

// Outputs is the named output bag produced by an action.
type Outputs map[string]any

// PathContext carries runtime paths that help adapters attribute work to the
// current stage, scenario, act, and property.
type PathContext struct {
	StagePath    string `json:"stage_path,omitempty"`
	ScenarioPath string `json:"scenario_path,omitempty"`
	ActPath      string `json:"act_path,omitempty"`
	PropertyPath string `json:"property_path,omitempty"`
}

// DecoratorFunc transforms one value after decorator compilation.
type DecoratorFunc func(value any) (any, error)

// DecoratorDef registers a decorator contract and compile function.
type DecoratorDef struct {
	Contract DecoratorContract
	Compile  func(args Values) (DecoratorFunc, error)
}

func (k ValueKind) Valid() bool {
	switch k {
	case ValueKindAny, ValueKindBytes, ValueKindString, ValueKindNumber, ValueKindBool, ValueKindObject, ValueKindList, ValueKindNull:
		return true
	default:
		return false
	}
}

// NewValueKindSet builds a set containing the provided kinds.
func NewValueKindSet(kinds ...ValueKind) ValueKindSet {
	if len(kinds) == 0 {
		return nil
	}

	set := make(ValueKindSet, len(kinds))
	for _, kind := range kinds {
		set[kind] = struct{}{}
	}

	return set
}

func (s ValueKindSet) Contains(kind ValueKind) bool {
	_, ok := s[kind]
	return ok
}

func (s ValueKindSet) Clone() ValueKindSet {
	if len(s) == 0 {
		return nil
	}

	cloned := make(ValueKindSet, len(s))
	for kind := range s {
		cloned[kind] = struct{}{}
	}

	return cloned
}

func (s ValueKindSet) Valid() bool {
	if len(s) == 0 {
		return false
	}

	for kind := range s {
		if !kind.Valid() {
			return false
		}
	}

	return true
}

func (c ValueContract) Clone() ValueContract {
	cloned := c
	cloned.Kinds = c.Kinds.Clone()
	if len(c.Fields) != 0 {
		cloned.Fields = make(map[string]ValueContract, len(c.Fields))
		for key, field := range c.Fields {
			cloned.Fields[key] = field.Clone()
		}
	}

	if c.Elem != nil {
		elem := c.Elem.Clone()
		cloned.Elem = &elem
	}

	return cloned
}

func (c ValueContract) KindsSet() ValueKindSet {
	size := len(c.Kinds)
	if c.Kind != "" {
		size++
	}
	if size == 0 {
		return nil
	}

	kinds := c.Kinds.Clone()
	if kinds == nil {
		kinds = make(ValueKindSet, size)
	}
	if c.Kind != "" {
		kinds[c.Kind] = struct{}{}
	}

	return kinds
}

func (c ValueContract) Supports(kind ValueKind) bool {
	kinds := c.KindsSet()
	return kinds.Contains(ValueKindAny) || kinds.Contains(kind)
}

func (c ValueContract) Valid() bool {
	if !c.KindsSet().Valid() {
		return false
	}

	for _, field := range c.Fields {
		if !field.Valid() {
			return false
		}
	}

	if c.Elem != nil && !c.Elem.Valid() {
		return false
	}

	return true
}
