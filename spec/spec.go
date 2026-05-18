package spec

import reportmodel "github.com/alex-poliushkin/theater/report"

// Public enum values used by the authoring and diagnostic model.
const (
	SeverityError DiagnosticSeverity = reportmodel.SeverityError
	SeverityHint  DiagnosticSeverity = reportmodel.SeverityHint

	BindingKindLiteral  BindingKind = "literal"
	BindingKindRef      BindingKind = "ref"
	BindingKindObject   BindingKind = "object"
	BindingKindList     BindingKind = "list"
	BindingKindString   BindingKind = "string"
	BindingKindGenerate BindingKind = "generate"
	BindingKindCoalesce BindingKind = "coalesce"
	BindingKindEnv      BindingKind = "env"

	LogFormatText LogFormat = "text"
	LogFormatJSON LogFormat = "json"

	DecodeJSON          DecodeKind        = "json"
	SubjectFromProperty SubjectSourceKind = "property"

	TriggerPredicateSuccess TriggerPredicate = "success"
	TriggerPredicateFailure TriggerPredicate = "failure"
	TriggerPredicateDone    TriggerPredicate = "done"

	TransitionOnPass    TransitionOutcome = "on_pass"
	TransitionOnFail    TransitionOutcome = "on_fail"
	TransitionOnTimeout TransitionOutcome = "on_timeout"
	TransitionOnCancel  TransitionOutcome = "on_cancel"
)

// DiagnosticSeverity classifies compile and validation diagnostics.
type DiagnosticSeverity = reportmodel.DiagnosticSeverity

// SourceRef points to a source location in an authoring file when known.
type SourceRef = reportmodel.SourceRef

// BindingKind identifies the wire form used by a binding.
type BindingKind string

// LogFormat identifies the preferred rendering format for one scenario-authored
// log record.
type LogFormat string

// DecodeKind identifies an optional decode step before structural traversal.
type DecodeKind string

// SubjectSourceKind identifies which current-act value family an expectation
// subject reads from.
type SubjectSourceKind string

// TriggerPredicate identifies when a dependency is considered satisfied.
type TriggerPredicate string

// TransitionOutcome identifies which act outcome triggers a transition.
type TransitionOutcome string

// BindingSpec describes a literal, ref, object, list, string, generator,
// coalesce, or environment binding.
type BindingSpec struct {
	Kind       BindingKind            `yaml:"kind"`
	Value      any                    `yaml:"value,omitempty"`
	Ref        *RefSpec               `yaml:"ref,omitempty" json:"ref,omitempty"`
	Object     map[string]BindingSpec `yaml:"object,omitempty"`
	List       []BindingSpec          `yaml:"list,omitempty"`
	Parts      []BindingSpec          `yaml:"parts,omitempty"`
	Generator  string                 `yaml:"generator,omitempty" json:"generator,omitempty"`
	Env        string                 `yaml:"name,omitempty" json:"name,omitempty"`
	Args       map[string]BindingSpec `yaml:"args,omitempty" json:"args,omitempty"`
	Candidates []BindingSpec          `yaml:"candidates,omitempty" json:"candidates,omitempty"`
	SourceSpan *SourceRef             `yaml:"-" json:"-"`
}

// Clone returns a deep copy of the binding spec.
func (s BindingSpec) Clone() BindingSpec {
	cloned := s
	if s.Ref != nil {
		ref := *s.Ref
		cloned.Ref = &ref
	}
	if s.Object != nil {
		cloned.Object = make(map[string]BindingSpec, len(s.Object))
		for key := range s.Object {
			cloned.Object[key] = s.Object[key].Clone()
		}
	}
	if s.List != nil {
		cloned.List = make([]BindingSpec, 0, len(s.List))
		for i := range s.List {
			cloned.List = append(cloned.List, s.List[i].Clone())
		}
	}
	if s.Parts != nil {
		cloned.Parts = make([]BindingSpec, 0, len(s.Parts))
		for i := range s.Parts {
			cloned.Parts = append(cloned.Parts, s.Parts[i].Clone())
		}
	}
	if s.Args != nil {
		cloned.Args = make(map[string]BindingSpec, len(s.Args))
		for key := range s.Args {
			cloned.Args[key] = s.Args[key].Clone()
		}
	}
	if s.Candidates != nil {
		cloned.Candidates = make([]BindingSpec, 0, len(s.Candidates))
		for i := range s.Candidates {
			cloned.Candidates = append(cloned.Candidates, s.Candidates[i].Clone())
		}
	}
	if s.SourceSpan != nil {
		source := *s.SourceSpan
		cloned.SourceSpan = &source
	}

	return cloned
}

// LogSpec declares one act-local scenario-authored log record.
type LogSpec struct {
	ID          string                  `yaml:"id" json:"id"`
	Value       LogValueSpec            `yaml:"value,omitempty" json:"value,omitempty"`
	Message     string                  `yaml:"message,omitempty" json:"message,omitempty"`
	Fields      map[string]LogValueSpec `yaml:"fields,omitempty" json:"fields,omitempty"`
	Format      LogFormat               `yaml:"format,omitempty" json:"format,omitempty"`
	Capture     Capture                 `yaml:"capture,omitempty" json:"capture,omitempty"`
	Sensitivity Sensitivity             `yaml:"sensitivity,omitempty" json:"sensitivity,omitempty"`
	Required    bool                    `yaml:"required,omitempty" json:"required,omitempty"`
	SourceSpan  *SourceRef              `yaml:"-" json:"-"`
}

// LogValueSpec selects or builds a report-only value for a scenario-authored
// log record.
type LogValueSpec struct {
	Field      string                  `yaml:"field,omitempty" json:"field,omitempty"`
	Ref        string                  `yaml:"ref,omitempty" json:"ref,omitempty"`
	Object     map[string]LogValueSpec `yaml:"object,omitempty" json:"object,omitempty"`
	List       []LogValueSpec          `yaml:"list,omitempty" json:"list,omitempty"`
	Decode     DecodeKind              `yaml:"decode,omitempty" json:"decode,omitempty"`
	Path       JSONPointer             `yaml:"path,omitempty" json:"path,omitempty"`
	Through    []ThroughStepSpec       `yaml:"through,omitempty" json:"through,omitempty"`
	SourceSpan *SourceRef              `yaml:"-" json:"-"`
}

// ExportSpec declares a named export. Act exports can select current action
// outputs with Field or available act-scope values with Ref. Scenario-call
// exports use Ref to select from the completed scenario scope.
type ExportSpec struct {
	As      string
	Ref     *RefSpec
	Field   string
	Decode  DecodeKind
	Path    JSONPointer
	Through []ThroughStepSpec
}

// InventoryCall configures one inventory use+with call-site.
type InventoryCall struct {
	Use  string                 `yaml:"use" json:"use"`
	With map[string]BindingSpec `yaml:"with,omitempty" json:"with,omitempty"`
}

// PropertySpec describes an act property resolved through either a value
// binding or an inventory call, then an optional decorator chain.
type PropertySpec struct {
	Value      *BindingSpec    `yaml:"value,omitempty" json:"value,omitempty"`
	Inventory  *InventoryCall  `yaml:"inventory,omitempty" json:"inventory,omitempty"`
	Decorators []DecoratorSpec `yaml:"decorators,omitempty" json:"decorators,omitempty"`
}

// DecoratorSpec selects a decorator and its static configuration.
type DecoratorSpec struct {
	Use  string         `yaml:"use" json:"use"`
	With map[string]any `yaml:"with,omitempty" json:"with,omitempty"`
}

// StageSpec is the top-level public stage definition.
type StageSpec struct {
	ID            string             `yaml:"id"`
	Name          string             `yaml:"name,omitempty"`
	HTTP          *HTTPSpec          `yaml:"http,omitempty"`
	State         *StateSpec         `yaml:"state,omitempty"`
	Scenarios     []ScenarioSpec     `yaml:"scenarios"`
	ScenarioCalls []ScenarioCallSpec `yaml:"scenario_calls"`
	SourceSpan    *SourceRef         `yaml:"-" json:"-"`
}

// ScenarioSpec defines a reusable scenario with inputs and ordered acts.
type ScenarioSpec struct {
	ID           string                         `yaml:"id"`
	Name         string                         `yaml:"name,omitempty"`
	Inputs       map[string]ValueContract       `yaml:"inputs,omitempty"`
	AuthBindings map[string]HTTPAuthBindingSpec `yaml:"auth_bindings,omitempty" json:"auth_bindings,omitempty"`
	Preflight    []PreflightSpec                `yaml:"preflight,omitempty" json:"preflight,omitempty"`
	Acts         []ActSpec                      `yaml:"acts"`
	SourceSpan   *SourceRef                     `yaml:"-" json:"-"`
}

// PreflightSpec defines one scenario-level guard evaluated after scenario-call
// input bindings resolve and before scenario side effects begin.
type PreflightSpec struct {
	ID         string     `yaml:"id" json:"id"`
	Input      RefSpec    `yaml:"input" json:"input"`
	Assert     AssertSpec `yaml:"assert" json:"assert"`
	Override   *RefSpec   `yaml:"override,omitempty" json:"override,omitempty"`
	SourceSpan *SourceRef `yaml:"-" json:"-"`
}

// ScenarioCallSpec invokes a scenario within a stage.
type ScenarioCallSpec struct {
	ID           string                   `yaml:"id"`
	Name         string                   `yaml:"name,omitempty"`
	ScenarioID   string                   `yaml:"scenario_id"`
	Bindings     map[string]BindingSpec   `yaml:"bindings,omitempty"`
	Exports      []ExportSpec             `yaml:"exports,omitempty"`
	Dependencies []ScenarioDependencySpec `yaml:"dependencies,omitempty"`
	SourceSpan   *SourceRef               `yaml:"-" json:"-"`
}

// ScenarioDependencySpec gates one scenario call on another.
type ScenarioDependencySpec struct {
	CallID string           `yaml:"call_id"`
	When   TriggerPredicate `yaml:"when,omitempty"`
}

// EventuallySpec configures whole-act convergence timing.
type EventuallySpec struct {
	Timeout  string `yaml:"timeout" json:"timeout"`
	Interval string `yaml:"interval" json:"interval"`
}

// ActSpec defines one executable act inside a scenario.
type ActSpec struct {
	ID           string                  `yaml:"id"`
	Name         string                  `yaml:"name,omitempty"`
	Eventually   *EventuallySpec         `yaml:"eventually,omitempty" json:"eventually,omitempty"`
	Properties   map[string]PropertySpec `yaml:"properties,omitempty"`
	Action       ActionSpec              `yaml:"action"`
	CaptureAuth  *HTTPAuthCaptureSpec    `yaml:"capture_auth,omitempty" json:"capture_auth,omitempty"`
	Logs         []LogSpec               `yaml:"logs,omitempty" json:"logs,omitempty"`
	Expectations []ExpectationSpec       `yaml:"expectations,omitempty"`
	Exports      []ExportSpec            `yaml:"exports,omitempty"`
	Transitions  []TransitionSpec        `yaml:"transitions,omitempty"`
	SourceSpan   *SourceRef              `yaml:"-" json:"-"`
}

// ActionSpec selects the action implementation and bound inputs for an act.
type ActionSpec struct {
	Use        string                 `yaml:"use"`
	With       map[string]BindingSpec `yaml:"with,omitempty"`
	Repeatable bool                   `yaml:"repeatable,omitempty" json:"repeatable,omitempty"`
	SourceSpan *SourceRef             `yaml:"-" json:"-"`
}

// ExpectationSpec defines one assertion over a current-act value.
type ExpectationSpec struct {
	ID         string      `yaml:"id"`
	Subject    SubjectSpec `yaml:"subject"`
	Assert     AssertSpec  `yaml:"assert"`
	SourceSpan *SourceRef  `yaml:"-" json:"-"`
}

// SubjectSpec selects either a current action output or a current-act property
// value before matcher evaluation.
type SubjectSpec struct {
	From    SubjectSourceKind `yaml:"from,omitempty" json:"from,omitempty"`
	Ref     string            `yaml:"ref,omitempty" json:"ref,omitempty"`
	Field   string            `yaml:"field,omitempty" json:"field,omitempty"`
	Decode  DecodeKind        `yaml:"decode,omitempty" json:"decode,omitempty"`
	Path    JSONPointer       `yaml:"path,omitempty" json:"path,omitempty"`
	Through []ThroughStepSpec `yaml:"through,omitempty" json:"through,omitempty"`
}

// AssertSpec selects a matcher and its bound args.
type AssertSpec struct {
	Ref  string
	Args map[string]BindingSpec
}

// TransitionSpec links an act outcome to the next act id.
type TransitionSpec struct {
	On TransitionOutcome `yaml:"on"`
	To string            `yaml:"to"`
}

func (k BindingKind) Valid() bool {
	switch k {
	case BindingKindLiteral,
		BindingKindRef,
		BindingKindObject,
		BindingKindList,
		BindingKindString,
		BindingKindGenerate,
		BindingKindCoalesce,
		BindingKindEnv:
		return true
	default:
		return false
	}
}

func (f LogFormat) Valid() bool {
	switch f {
	case "", LogFormatText, LogFormatJSON:
		return true
	default:
		return false
	}
}

func (k DecodeKind) Valid() bool {
	switch k {
	case "", DecodeJSON:
		return true
	default:
		return false
	}
}

func (k SubjectSourceKind) Valid() bool {
	switch k {
	case "", SubjectFromProperty:
		return true
	default:
		return false
	}
}

func (p TriggerPredicate) Valid() bool {
	switch p {
	case TriggerPredicateSuccess, TriggerPredicateFailure, TriggerPredicateDone:
		return true
	default:
		return false
	}
}

func (o TransitionOutcome) Valid() bool {
	switch o {
	case TransitionOnPass, TransitionOnFail, TransitionOnTimeout, TransitionOnCancel:
		return true
	default:
		return false
	}
}
