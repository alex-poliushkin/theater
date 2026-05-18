package theater

import specmodel "github.com/alex-poliushkin/theater/spec"

// Public enum values used by the authoring and diagnostic model.
const (
	SeverityError DiagnosticSeverity = specmodel.SeverityError
	SeverityHint  DiagnosticSeverity = specmodel.SeverityHint

	BindingKindLiteral  BindingKind = specmodel.BindingKindLiteral
	BindingKindRef      BindingKind = specmodel.BindingKindRef
	BindingKindObject   BindingKind = specmodel.BindingKindObject
	BindingKindList     BindingKind = specmodel.BindingKindList
	BindingKindString   BindingKind = specmodel.BindingKindString
	BindingKindGenerate BindingKind = specmodel.BindingKindGenerate
	BindingKindCoalesce BindingKind = specmodel.BindingKindCoalesce
	BindingKindEnv      BindingKind = specmodel.BindingKindEnv

	LogFormatText LogFormat = specmodel.LogFormatText
	LogFormatJSON LogFormat = specmodel.LogFormatJSON

	DecodeJSON          DecodeKind        = specmodel.DecodeJSON
	SubjectFromProperty SubjectSourceKind = specmodel.SubjectFromProperty

	TriggerPredicateSuccess TriggerPredicate = specmodel.TriggerPredicateSuccess
	TriggerPredicateFailure TriggerPredicate = specmodel.TriggerPredicateFailure
	TriggerPredicateDone    TriggerPredicate = specmodel.TriggerPredicateDone

	TransitionOnPass    TransitionOutcome = specmodel.TransitionOnPass
	TransitionOnFail    TransitionOutcome = specmodel.TransitionOnFail
	TransitionOnTimeout TransitionOutcome = specmodel.TransitionOnTimeout
	TransitionOnCancel  TransitionOutcome = specmodel.TransitionOnCancel

	HTTPSessionNone                 = specmodel.HTTPSessionNone
	HTTPAuthNone                    = specmodel.HTTPAuthNone
	HTTPAPIKeyInHeader HTTPAPIKeyIn = specmodel.HTTPAPIKeyInHeader
	HTTPAPIKeyInQuery  HTTPAPIKeyIn = specmodel.HTTPAPIKeyInQuery
)

// BindingKind identifies the wire form used by a binding.
type BindingKind = specmodel.BindingKind

// LogFormat identifies the preferred rendering format for one scenario-authored
// log record.
type LogFormat = specmodel.LogFormat

// DecodeKind identifies an optional decode step before structural traversal.
type DecodeKind = specmodel.DecodeKind

// SubjectSourceKind identifies which current-act value family an expectation
// subject reads from.
type SubjectSourceKind = specmodel.SubjectSourceKind

// TriggerPredicate identifies when a dependency is considered satisfied.
type TriggerPredicate = specmodel.TriggerPredicate

// TransitionOutcome identifies which act outcome triggers a transition.
type TransitionOutcome = specmodel.TransitionOutcome

// BindingSpec describes a literal, ref, object, list, string, generator,
// coalesce, or environment binding.
type BindingSpec = specmodel.BindingSpec

// LogSpec declares one act-local scenario-authored log record.
type LogSpec = specmodel.LogSpec

// LogValueSpec selects or builds a report-only value for a scenario-authored
// log record.
type LogValueSpec = specmodel.LogValueSpec

// HTTPAPIKeyIn identifies where an API key attachment is applied.
type HTTPAPIKeyIn = specmodel.HTTPAPIKeyIn

// HTTPSpec declares shared stage-level HTTP sessions and auth configs.
type HTTPSpec = specmodel.HTTPSpec

// StateSpec declares shared stage-level persistent state backends.
type StateSpec = specmodel.StateSpec

// StateBackendSpec declares one configured persistent state backend.
type StateBackendSpec = specmodel.StateBackendSpec

// HTTPSessionSpec reserves a named managed cookie session.
type HTTPSessionSpec = specmodel.HTTPSessionSpec

// HTTPAuthSpec declares one reusable HTTP auth attachment set.
type HTTPAuthSpec = specmodel.HTTPAuthSpec

// HTTPAuthBindingSpec initializes declared auth slots for one scenario
// execution from scenario-start bindings.
type HTTPAuthBindingSpec = specmodel.HTTPAuthBindingSpec

// HTTPIdentitySpec bundles one optional session ref and one optional auth ref.
type HTTPIdentitySpec = specmodel.HTTPIdentitySpec

// HTTPAuthAttachmentSpec declares one typed request auth attachment.
type HTTPAuthAttachmentSpec = specmodel.HTTPAuthAttachmentSpec

// HTTPBearerAuthSpec attaches a bearer token to Authorization.
type HTTPBearerAuthSpec = specmodel.HTTPBearerAuthSpec

// HTTPBasicAuthSpec attaches HTTP Basic credentials.
type HTTPBasicAuthSpec = specmodel.HTTPBasicAuthSpec

// HTTPAPIKeyAuthSpec attaches an API key to a header or query parameter.
type HTTPAPIKeyAuthSpec = specmodel.HTTPAPIKeyAuthSpec

// HTTPHeaderSlotAuthSpec attaches one captured auth slot to a request header.
type HTTPHeaderSlotAuthSpec = specmodel.HTTPHeaderSlotAuthSpec

// HTTPQuerySlotAuthSpec attaches one captured auth slot to a query parameter.
type HTTPQuerySlotAuthSpec = specmodel.HTTPQuerySlotAuthSpec

// HTTPFormSlotAuthSpec attaches one captured auth slot to a form field.
type HTTPFormSlotAuthSpec = specmodel.HTTPFormSlotAuthSpec

// HTTPAuthCaptureSpec captures response material into named auth slots after
// a successful HTTP action.
type HTTPAuthCaptureSpec = specmodel.HTTPAuthCaptureSpec

// HTTPCaptureSourceSpec selects one response source for a captured auth slot.
type HTTPCaptureSourceSpec = specmodel.HTTPCaptureSourceSpec

// ExportSpec declares a named export. Act exports can select current action
// outputs with Field or available act-scope values with Ref. Scenario-call
// exports use Ref to select from the completed scenario scope.
type ExportSpec = specmodel.ExportSpec

// InventoryCall configures one inventory use+with call-site.
type InventoryCall = specmodel.InventoryCall

// PropertySpec describes an act property resolved through either a value
// binding or an inventory call, then an optional decorator chain.
type PropertySpec = specmodel.PropertySpec

// DecoratorSpec selects a decorator and its static configuration.
type DecoratorSpec = specmodel.DecoratorSpec

// StageSpec is the top-level public stage definition.
type StageSpec = specmodel.StageSpec

// ScenarioSpec defines a reusable scenario with inputs and ordered acts.
type ScenarioSpec = specmodel.ScenarioSpec

// PreflightSpec defines one scenario-level guard evaluated before scenario
// side effects begin.
type PreflightSpec = specmodel.PreflightSpec

// ScenarioCallSpec invokes a scenario within a stage.
type ScenarioCallSpec = specmodel.ScenarioCallSpec

// ScenarioDependencySpec gates one scenario call on another.
type ScenarioDependencySpec = specmodel.ScenarioDependencySpec

// EventuallySpec configures whole-act convergence timing.
type EventuallySpec = specmodel.EventuallySpec

// ActSpec defines one executable act inside a scenario.
type ActSpec = specmodel.ActSpec

// ActionSpec selects the action implementation and bound inputs for an act.
type ActionSpec = specmodel.ActionSpec

// ExpectationSpec defines one assertion over a current-act value.
type ExpectationSpec = specmodel.ExpectationSpec

// SubjectSpec selects either a current action output or a current-act property
// value before matcher evaluation.
type SubjectSpec = specmodel.SubjectSpec

// AssertSpec selects a matcher and its bound args.
type AssertSpec = specmodel.AssertSpec

// TransitionSpec links an act outcome to the next act id.
type TransitionSpec = specmodel.TransitionSpec
