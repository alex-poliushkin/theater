package thtr

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/alex-poliushkin/theater"
	builtinexpectation "github.com/alex-poliushkin/theater/builtin/expectation"
)

const (
	sectionHTTPAuth             = "auth"
	sectionHTTPIdentity         = "identity"
	sectionHTTPSession          = "session"
	sectionStateBackend         = "backend"
	sectionStateRecord          = "record"
	sectionStatePool            = "pool"
	selectorRootField           = "field"
	selectorStepDecode          = "decode"
	selectorStepPath            = "path"
	selectorStepPick            = "pick"
	selectorStepRegexp          = "regexp"
	selectorStepTransformPrefix = "transform."

	stateRecordAliasCall     = "state.record"
	statePoolAliasCall       = "state.pool"
	stateReadSugarCall       = "state.read"
	stateRemovedCASSugarCall = "state.cas"
	stateUpdateSugarCall     = "state.update"
	stateClaimSugarCall      = "state.claim"
	stateRenewSugarCall      = "state.renew"
	stateReleaseSugarCall    = "state.release"
	stateConsumeSugarCall    = "state.consume"
	stateReadActionCall      = "action.state.read"
	stateUpdateActionCall    = "action.state.update"
	stateClaimActionCall     = "action.state.claim"
	stateRenewActionCall     = "action.state.renew"
	stateReleaseActionCall   = "action.state.release"
	stateConsumeActionCall   = "action.state.consume"
	stateRecordInventoryCall = "inventory.state.record"
	statePoolInventoryCall   = "inventory.state.pool"
	hiddenStateAliasPrefix   = "thtr:hidden:state"
	stateClaimArgID          = "id"
	stateClaimArgFields      = "fields"
	stateClaimArgLease       = "lease"
	stateClaimLeaseOnExpiry  = "on_expiry"

	bindingCallCoalesce = "coalesce"
	bindingCallEnv      = "env"
)

type lowerError struct {
	span    sourceSpan
	message string
}

type stateAliasKind string

const (
	stateAliasKindRecord stateAliasKind = "record"
	stateAliasKindPool   stateAliasKind = "pool"
)

type stateAliasSpec struct {
	kind     stateAliasKind
	span     sourceSpan
	with     map[string]theater.BindingSpec
	argSpans map[string]sourceSpan
}

type stateAliasTable map[string]stateAliasSpec

func (e *lowerError) Error() string {
	return e.message
}

func (e *lowerError) Span() sourceSpan {
	return e.span
}

func lowerStageSpec(document *syntaxDocument, sourceFile string) (theater.StageSpec, error) {
	name, err := lowerOptionalName(document.Stage.Name)
	if err != nil {
		return theater.StageSpec{}, err
	}

	spec := theater.StageSpec{
		ID:            document.Stage.ID,
		Name:          name,
		Scenarios:     make([]theater.ScenarioSpec, 0, len(document.Scenarios)),
		ScenarioCalls: make([]theater.ScenarioCallSpec, 0, len(document.Calls)),
		SourceSpan:    bindSourceSpan(document.Stage.Span, sourceFile),
	}

	if document.HTTP != nil {
		httpSpec, err := lowerHTTPSection(*document.HTTP)
		if err != nil {
			return theater.StageSpec{}, err
		}
		spec.HTTP = httpSpec
	}

	var stateAliases stateAliasTable
	if document.State != nil {
		stateSpec, aliases, err := lowerStateSection(*document.State)
		if err != nil {
			return theater.StageSpec{}, err
		}
		spec.State = stateSpec
		stateAliases = aliases
	}

	for i := range document.Scenarios {
		scenario, err := lowerScenario(document.Scenarios[i], sourceFile, stateAliases)
		if err != nil {
			return theater.StageSpec{}, err
		}
		spec.Scenarios = append(spec.Scenarios, scenario)
	}

	for i := range document.Calls {
		call, err := lowerScenarioCall(document.Calls[i], sourceFile)
		if err != nil {
			return theater.StageSpec{}, err
		}
		spec.ScenarioCalls = append(spec.ScenarioCalls, call)
	}

	return spec, nil
}

func lowerScenario(scenario scenarioSyntax, sourceFile string, stateAliases stateAliasTable) (theater.ScenarioSpec, error) {
	name, err := lowerOptionalName(scenario.Name)
	if err != nil {
		return theater.ScenarioSpec{}, err
	}

	spec := theater.ScenarioSpec{
		ID:           scenario.ID,
		Name:         name,
		Inputs:       make(map[string]theater.ValueContract, len(scenario.Inputs)),
		AuthBindings: make(map[string]theater.HTTPAuthBindingSpec, len(scenario.AuthBindings)),
		Preflight:    make([]theater.PreflightSpec, 0, len(scenario.Preflight)),
		Acts:         make([]theater.ActSpec, 0, len(scenario.Acts)),
		SourceSpan:   bindSourceSpan(scenario.Span, sourceFile),
	}

	for i := range scenario.Inputs {
		input := scenario.Inputs[i]
		if _, exists := spec.Inputs[input.Name]; exists {
			return theater.ScenarioSpec{}, &lowerError{
				span:    input.Span,
				message: fmt.Sprintf("input %q is duplicated", input.Name),
			}
		}
		spec.Inputs[input.Name] = theater.ValueContract{
			Kind:     theater.ValueKind(input.Type),
			Required: input.Required,
		}
	}
	if len(spec.Inputs) == 0 {
		spec.Inputs = nil
	}

	for i := range scenario.AuthBindings {
		authBinding, err := lowerAuthBinding(scenario.AuthBindings[i])
		if err != nil {
			return theater.ScenarioSpec{}, err
		}
		if _, exists := spec.AuthBindings[scenario.AuthBindings[i].Auth]; exists {
			return theater.ScenarioSpec{}, &lowerError{
				span:    scenario.AuthBindings[i].Span,
				message: fmt.Sprintf("auth binding %q is duplicated", scenario.AuthBindings[i].Auth),
			}
		}

		spec.AuthBindings[scenario.AuthBindings[i].Auth] = authBinding
	}
	if len(spec.AuthBindings) == 0 {
		spec.AuthBindings = nil
	}

	for i := range scenario.Preflight {
		preflight, err := lowerPreflight(scenario.Preflight[i], sourceFile)
		if err != nil {
			return theater.ScenarioSpec{}, err
		}
		spec.Preflight = append(spec.Preflight, preflight)
	}
	if len(spec.Preflight) == 0 {
		spec.Preflight = nil
	}

	for i := range scenario.Acts {
		act, err := lowerAct(scenario.Acts[i], sourceFile, stateAliases)
		if err != nil {
			return theater.ScenarioSpec{}, err
		}
		spec.Acts = append(spec.Acts, act)
	}

	return spec, nil
}

func lowerAuthBinding(binding authBindingSyntax) (theater.HTTPAuthBindingSpec, error) {
	spec := theater.HTTPAuthBindingSpec{
		Slots: make(map[string]theater.BindingSpec, len(binding.Slots)),
	}
	for i := range binding.Slots {
		slot := binding.Slots[i]
		if len(slot.Mapping) != 0 {
			return theater.HTTPAuthBindingSpec{}, &lowerError{
				span:    slot.Span,
				message: "auth binding slot value must be expression",
			}
		}
		if _, exists := spec.Slots[slot.Name]; exists {
			return theater.HTTPAuthBindingSpec{}, &lowerError{
				span:    slot.Span,
				message: fmt.Sprintf("auth binding slot %q is duplicated", slot.Name),
			}
		}

		value, err := lowerBindingExpression(slot.Value)
		if err != nil {
			return theater.HTTPAuthBindingSpec{}, err
		}
		spec.Slots[slot.Name] = value
	}
	if len(spec.Slots) == 0 {
		spec.Slots = nil
	}

	return spec, nil
}

func lowerPreflight(preflight preflightSyntax, sourceFile string) (theater.PreflightSpec, error) {
	input, err := lowerPreflightRef(preflight.Input)
	if err != nil {
		return theater.PreflightSpec{}, err
	}

	assert, err := lowerAssertion(preflight.Assert)
	if err != nil {
		return theater.PreflightSpec{}, err
	}

	var override *theater.RefSpec
	if preflight.Override != nil {
		resolved, err := lowerPreflightRef(preflight.Override)
		if err != nil {
			return theater.PreflightSpec{}, err
		}
		override = &resolved
	}

	return theater.PreflightSpec{
		ID:         preflight.ID,
		Input:      input,
		Assert:     assert,
		Override:   override,
		SourceSpan: bindSourceSpan(preflight.Span, sourceFile),
	}, nil
}

func lowerPreflightRef(expr expressionSyntax) (theater.RefSpec, error) {
	root, decode, path, through, err := lowerSelection(expr)
	if err != nil {
		return theater.RefSpec{}, err
	}
	if root.ref == nil {
		return theater.RefSpec{}, &lowerError{
			span:    expr.ExpressionSpan(),
			message: "preflight input and override must start with $ref",
		}
	}

	return theater.RefSpec{
		Name:    root.ref.Name,
		Decode:  decode,
		Path:    path,
		Through: through,
	}, nil
}

func lowerAct(act actSyntax, sourceFile string, stateAliases stateAliasTable) (theater.ActSpec, error) {
	name, err := lowerOptionalName(act.Name)
	if err != nil {
		return theater.ActSpec{}, err
	}

	properties, err := lowerProperties(act.Properties)
	if err != nil {
		return theater.ActSpec{}, err
	}

	action, hiddenProperties, err := lowerAction(*act.Action, sourceFile, stateAliases)
	if err != nil {
		return theater.ActSpec{}, err
	}
	for name, property := range hiddenProperties {
		properties[name] = property
	}

	var captureAuth *theater.HTTPAuthCaptureSpec
	if act.CaptureAuth != nil {
		captureAuth, err = lowerCaptureAuth(*act.CaptureAuth)
		if err != nil {
			return theater.ActSpec{}, err
		}
	}

	expectations, err := lowerActExpectations(act.Expectations, act.Exports, sourceFile)
	if err != nil {
		return theater.ActSpec{}, err
	}

	logs, err := lowerActLogs(act.Logs, sourceFile)
	if err != nil {
		return theater.ActSpec{}, err
	}

	exports, err := lowerActExports(act.Exports)
	if err != nil {
		return theater.ActSpec{}, err
	}

	transitions, err := lowerTransitions(act.Transitions)
	if err != nil {
		return theater.ActSpec{}, err
	}

	return theater.ActSpec{
		ID:           act.ID,
		Name:         name,
		Eventually:   lowerEventually(act.Eventually),
		Properties:   properties,
		Action:       action,
		CaptureAuth:  captureAuth,
		Logs:         logs,
		Expectations: expectations,
		Exports:      exports,
		Transitions:  transitions,
		SourceSpan:   bindSourceSpan(act.Span, sourceFile),
	}, nil
}

func lowerActLogs(logs []logSyntax, sourceFile string) ([]theater.LogSpec, error) {
	if len(logs) == 0 {
		return nil, nil
	}

	lowered := make([]theater.LogSpec, 0, len(logs))
	for i := range logs {
		log, err := lowerActLog(logs[i], sourceFile)
		if err != nil {
			return nil, err
		}
		lowered = append(lowered, log)
	}

	return lowered, nil
}

func lowerActLog(log logSyntax, sourceFile string) (theater.LogSpec, error) {
	value, err := lowerLogValueExpression(log.Value, sourceFile)
	if err != nil {
		return theater.LogSpec{}, err
	}

	return theater.LogSpec{
		ID:          log.ID,
		Value:       value,
		Capture:     theater.CaptureSummary,
		Sensitivity: theater.SensitivityInternal,
		SourceSpan:  bindSourceSpan(log.Span, sourceFile),
	}, nil
}

func lowerLogValueExpression(expr expressionSyntax, sourceFile string) (theater.LogValueSpec, error) {
	switch value := ungroupExpression(expr).(type) {
	case callExpressionSyntax, refExpressionSyntax, pipelineExpressionSyntax:
		return lowerLogSelectionExpression(expr, sourceFile)
	case objectExpressionSyntax:
		object, err := lowerLogValueObject(value.Fields, sourceFile)
		if err != nil {
			return theater.LogValueSpec{}, err
		}
		return theater.LogValueSpec{
			Object:     object,
			SourceSpan: bindSourceSpan(value.Span, sourceFile),
		}, nil
	case listExpressionSyntax:
		list, err := lowerLogValueList(value.Items, sourceFile)
		if err != nil {
			return theater.LogValueSpec{}, err
		}
		return theater.LogValueSpec{
			List:       list,
			SourceSpan: bindSourceSpan(value.Span, sourceFile),
		}, nil
	default:
		return theater.LogValueSpec{}, &lowerError{
			span:    expr.ExpressionSpan(),
			message: "log value must start with field(...), $ref, object, or list",
		}
	}
}

func lowerLogSelectionExpression(expr expressionSyntax, sourceFile string) (theater.LogValueSpec, error) {
	root, decode, path, through, err := lowerSelection(expr)
	if err != nil {
		return theater.LogValueSpec{}, err
	}

	value := theater.LogValueSpec{
		Decode:     decode,
		Path:       path,
		Through:    through,
		SourceSpan: bindSourceSpan(expr.ExpressionSpan(), sourceFile),
	}
	if root.ref != nil {
		value.Ref = root.ref.Name
		return value, nil
	}
	if root.field != "" {
		value.Field = root.field
		return value, nil
	}

	return theater.LogValueSpec{}, &lowerError{
		span:    expr.ExpressionSpan(),
		message: "log value must start with field(...) or $ref",
	}
}

func lowerLogValueObject(fields []mappingEntrySyntax, sourceFile string) (map[string]theater.LogValueSpec, error) {
	object := make(map[string]theater.LogValueSpec, len(fields))
	for i := range fields {
		value, err := lowerLogValueExpression(fields[i].Value, sourceFile)
		if err != nil {
			return nil, err
		}
		object[fields[i].Name] = value
	}

	return object, nil
}

func lowerLogValueList(items []expressionSyntax, sourceFile string) ([]theater.LogValueSpec, error) {
	list := make([]theater.LogValueSpec, 0, len(items))
	for i := range items {
		value, err := lowerLogValueExpression(items[i], sourceFile)
		if err != nil {
			return nil, err
		}
		list = append(list, value)
	}

	return list, nil
}

func lowerEventually(eventually *eventuallySyntax) *theater.EventuallySpec {
	if eventually == nil {
		return nil
	}

	return &theater.EventuallySpec{
		Timeout:  eventually.Timeout,
		Interval: eventually.Interval,
	}
}

func lowerAction(
	action actionSyntax,
	sourceFile string,
	stateAliases stateAliasTable,
) (spec theater.ActionSpec, hiddenProperties map[string]theater.PropertySpec, err error) {
	canonicalCall, err := canonicalizeStateActionCall(action.Call)
	if err != nil {
		return theater.ActionSpec{}, nil, err
	}
	with, hiddenProperties, err := lowerNamedBindingArgsWithStateAliases(
		canonicalCall.Args,
		"action call",
		canonicalCall.Name,
		stateAliases,
	)
	if err != nil {
		return theater.ActionSpec{}, nil, err
	}

	spec = theater.ActionSpec{
		Use:        canonicalCall.Name,
		With:       with,
		Repeatable: action.Repeatable,
		SourceSpan: bindSourceSpan(action.Span, sourceFile),
	}
	return spec, hiddenProperties, nil
}

func lowerProperties(properties []propertySyntax) (map[string]theater.PropertySpec, error) {
	if len(properties) == 0 {
		return map[string]theater.PropertySpec{}, nil
	}

	lowered := make(map[string]theater.PropertySpec, len(properties))
	for i := range properties {
		property := properties[i]
		if _, exists := lowered[property.Name]; exists {
			return nil, &lowerError{
				span:    property.Span,
				message: fmt.Sprintf("property %q is duplicated", property.Name),
			}
		}
		spec, err := lowerProperty(property)
		if err != nil {
			return nil, err
		}
		lowered[property.Name] = spec
	}

	return lowered, nil
}

func lowerProperty(property propertySyntax) (theater.PropertySpec, error) {
	base, decorators := splitPropertyPipeline(property.Value)

	loweredDecorators := make([]theater.DecoratorSpec, 0, len(decorators))
	for i := range decorators {
		args, err := lowerNamedStaticArgs(decorators[i].Args, "decorator call")
		if err != nil {
			return theater.PropertySpec{}, err
		}
		loweredDecorators = append(loweredDecorators, theater.DecoratorSpec{
			Use:  decorators[i].Name,
			With: args,
		})
	}

	spec := theater.PropertySpec{
		Decorators: loweredDecorators,
	}
	if baseCall, ok := ungroupExpression(base).(callExpressionSyntax); ok && strings.HasPrefix(baseCall.Name, "inventory.") {
		with, err := lowerNamedBindingArgs(baseCall.Args, "inventory call")
		if err != nil {
			return theater.PropertySpec{}, err
		}
		spec.Inventory = &theater.InventoryCall{
			Use:  baseCall.Name,
			With: with,
		}
		return spec, nil
	}

	value, err := lowerBindingExpression(base)
	if err != nil {
		return theater.PropertySpec{}, err
	}
	spec.Value = &value
	return spec, nil
}

func splitPropertyPipeline(expr expressionSyntax) (base expressionSyntax, decorators []callExpressionSyntax) {
	switch value := ungroupExpression(expr).(type) {
	case callExpressionSyntax:
		return value, nil
	case pipelineExpressionSyntax:
		return value.Base, value.Steps
	default:
		return value, nil
	}
}

func lowerExpectation(expectation expectationSyntax, sourceFile string) (theater.ExpectationSpec, error) {
	subject, err := lowerSubject(expectation.Subject)
	if err != nil {
		return theater.ExpectationSpec{}, err
	}

	assert, err := lowerAssertion(expectation.Assert)
	if err != nil {
		return theater.ExpectationSpec{}, err
	}

	return theater.ExpectationSpec{
		ID:         expectation.ID,
		Subject:    subject,
		Assert:     assert,
		SourceSpan: bindSourceSpan(expectation.Span, sourceFile),
	}, nil
}

func lowerActExpectations(
	expectations []expectationSyntax,
	exports []exportSyntax,
	sourceFile string,
) ([]theater.ExpectationSpec, error) {
	lowered := make([]theater.ExpectationSpec, 0, len(expectations)+exportAssertionCount(exports))
	for i := range expectations {
		expectation, err := lowerExpectation(expectations[i], sourceFile)
		if err != nil {
			return nil, err
		}
		lowered = append(lowered, expectation)
	}
	for i := range exports {
		if exports[i].Assert == nil {
			continue
		}
		expectation, err := lowerExportAssertion(exports[i], sourceFile)
		if err != nil {
			return nil, err
		}
		lowered = append(lowered, expectation)
	}

	return lowered, nil
}

func exportAssertionCount(exports []exportSyntax) int {
	count := 0
	for i := range exports {
		if exports[i].Assert != nil {
			count++
		}
	}
	return count
}

func lowerExportAssertion(export exportSyntax, sourceFile string) (theater.ExpectationSpec, error) {
	subject, err := lowerSubject(export.Value)
	if err != nil {
		var typed *lowerError
		if errors.As(err, &typed) && typed.message == "subject must start with field(...)" {
			return theater.ExpectationSpec{}, &lowerError{
				span:    typed.span,
				message: "export assertion subject must start with field(...)",
			}
		}
		return theater.ExpectationSpec{}, err
	}

	assert, err := lowerAssertion(*export.Assert)
	if err != nil {
		return theater.ExpectationSpec{}, err
	}

	return theater.ExpectationSpec{
		ID:         export.Name,
		Subject:    subject,
		Assert:     assert,
		SourceSpan: bindSourceSpan(export.Assert.Span, sourceFile),
	}, nil
}

func lowerSubject(expr expressionSyntax) (theater.SubjectSpec, error) {
	root, decode, path, through, err := lowerSelection(expr)
	if err != nil {
		return theater.SubjectSpec{}, err
	}
	if root.field == "" {
		return theater.SubjectSpec{}, &lowerError{
			span:    expr.ExpressionSpan(),
			message: "subject must start with field(...)",
		}
	}

	return theater.SubjectSpec{
		Field:   root.field,
		Decode:  decode,
		Path:    path,
		Through: through,
	}, nil
}

func lowerAssertion(assertion assertionSyntax) (theater.AssertSpec, error) {
	lowered, err := lowerAssertionCore(assertion)
	if err != nil {
		return theater.AssertSpec{}, err
	}
	if assertion.NegationSpan == nil {
		return lowered, nil
	}

	return lowerNegatedAssertion(lowered), nil
}

func lowerAssertionCore(assertion assertionSyntax) (theater.AssertSpec, error) {
	switch assertion.Kind {
	case assertionKindEqual:
		return lowerUnaryAssertion(builtinexpectation.EqualRef, "expected", assertion.Value)
	case assertionKindNotEqual:
		equal, err := lowerUnaryAssertion(builtinexpectation.EqualRef, "expected", assertion.Value)
		if err != nil {
			return theater.AssertSpec{}, err
		}
		return lowerNegatedAssertion(equal), nil
	case assertionKindContains:
		return lowerUnaryAssertion(builtinexpectation.ContainsRef, "expected", assertion.Value)
	case assertionKindMatches:
		return lowerUnaryAssertion(builtinexpectation.MatchesRef, "pattern", assertion.Value)
	case assertionKindPresent, assertionKindNull, assertionKindNotNull:
		return lowerZeroArgAssertion(assertion.Kind)
	case assertionKindGT:
		return lowerUnaryAssertion(builtinexpectation.GTRef, "expected", assertion.Value)
	case assertionKindGTE:
		return lowerUnaryAssertion(builtinexpectation.GTERef, "expected", assertion.Value)
	case assertionKindLT:
		return lowerUnaryAssertion(builtinexpectation.LTRef, "expected", assertion.Value)
	case assertionKindLTE:
		return lowerUnaryAssertion(builtinexpectation.LTERef, "expected", assertion.Value)
	case assertionKindBetween:
		return lowerBetweenAssertion(assertion.Value, assertion.SecondValue)
	case assertionKindHasItem:
		return lowerCollectionAssertion(builtinexpectation.HasItemRef, assertion.Clauses)
	case assertionKindAllItems:
		return lowerCollectionAssertion(builtinexpectation.AllItemsRef, assertion.Clauses)
	case assertionKindHasKey, assertionKindLacksKey:
		return lowerKeyAssertion(assertion.Kind, assertion.Value)
	case assertionKindHasEntry:
		return lowerHasEntryAssertion(assertion.Value, assertion.Nested)
	case assertionKindCall:
		call, ok := ungroupExpression(assertion.Value).(callExpressionSyntax)
		if !ok {
			return theater.AssertSpec{}, &lowerError{
				span:    assertion.Value.ExpressionSpan(),
				message: "assert must be matcher call",
			}
		}
		args, err := lowerNamedBindingArgs(call.Args, "assert call")
		if err != nil {
			return theater.AssertSpec{}, err
		}
		return theater.AssertSpec{
			Ref:  call.Name,
			Args: args,
		}, nil
	default:
		return theater.AssertSpec{}, &lowerError{
			span:    assertion.Span,
			message: "assertion kind is not supported",
		}
	}
}

func lowerHasEntryAssertion(key expressionSyntax, nested *assertionSyntax) (theater.AssertSpec, error) {
	keyBinding, err := lowerBindingExpression(key)
	if err != nil {
		return theater.AssertSpec{}, err
	}
	nestedAssert, err := lowerAssertion(*nested)
	if err != nil {
		return theater.AssertSpec{}, err
	}

	return theater.AssertSpec{
		Ref: builtinexpectation.HasEntryRef,
		Args: map[string]theater.BindingSpec{
			"key":    keyBinding,
			"assert": lowerNestedAssertBinding(nestedAssert),
		},
	}, nil
}

func lowerZeroArgAssertion(kind assertionKind) (theater.AssertSpec, error) {
	switch kind {
	case assertionKindPresent:
		return theater.AssertSpec{Ref: builtinexpectation.PresentRef}, nil
	case assertionKindNull:
		return theater.AssertSpec{Ref: builtinexpectation.NullRef}, nil
	case assertionKindNotNull:
		return theater.AssertSpec{Ref: builtinexpectation.NotNullRef}, nil
	default:
		return theater.AssertSpec{}, fmt.Errorf("zero-arg assertion kind %q is not supported", kind)
	}
}

func lowerKeyAssertion(kind assertionKind, value expressionSyntax) (theater.AssertSpec, error) {
	switch kind {
	case assertionKindHasKey:
		return lowerUnaryAssertion(builtinexpectation.HasKeyRef, "key", value)
	case assertionKindLacksKey:
		return lowerUnaryAssertion(builtinexpectation.LacksKeyRef, "key", value)
	default:
		return theater.AssertSpec{}, fmt.Errorf("key assertion kind %q is not supported", kind)
	}
}

func lowerUnaryAssertion(ref, argName string, value expressionSyntax) (theater.AssertSpec, error) {
	binding, err := lowerBindingExpression(value)
	if err != nil {
		return theater.AssertSpec{}, err
	}

	return theater.AssertSpec{
		Ref: ref,
		Args: map[string]theater.BindingSpec{
			argName: binding,
		},
	}, nil
}

func lowerBetweenAssertion(minValue, maxValue expressionSyntax) (theater.AssertSpec, error) {
	minBinding, err := lowerBindingExpression(minValue)
	if err != nil {
		return theater.AssertSpec{}, err
	}
	maxBinding, err := lowerBindingExpression(maxValue)
	if err != nil {
		return theater.AssertSpec{}, err
	}

	return theater.AssertSpec{
		Ref: builtinexpectation.BetweenRef,
		Args: map[string]theater.BindingSpec{
			"min": minBinding,
			"max": maxBinding,
		},
	}, nil
}

func lowerNegatedAssertion(assert theater.AssertSpec) theater.AssertSpec {
	return theater.AssertSpec{
		Ref: builtinexpectation.NotRef,
		Args: map[string]theater.BindingSpec{
			"assert": lowerNestedAssertBinding(assert),
		},
	}
}

func lowerCollectionAssertion(ref string, clauses []relativeClauseSyntax) (theater.AssertSpec, error) {
	where := make([]theater.BindingSpec, 0, len(clauses))
	for i := range clauses {
		clause, err := lowerRelativeClause(clauses[i])
		if err != nil {
			return theater.AssertSpec{}, err
		}
		where = append(where, clause)
	}

	return theater.AssertSpec{
		Ref: ref,
		Args: map[string]theater.BindingSpec{
			"where": {
				Kind: theater.BindingKindList,
				List: where,
			},
		},
	}, nil
}

func lowerRelativeClause(clause relativeClauseSyntax) (theater.BindingSpec, error) {
	subject, err := lowerRelativeClauseSubject(clause.Subject)
	if err != nil {
		return theater.BindingSpec{}, err
	}
	assert, err := lowerAssertion(clause.Assert)
	if err != nil {
		return theater.BindingSpec{}, err
	}

	return theater.BindingSpec{
		Kind: theater.BindingKindObject,
		Object: map[string]theater.BindingSpec{
			"subject": subject,
			"assert":  lowerNestedAssertBinding(assert),
		},
	}, nil
}

func lowerRelativeClauseSubject(expr expressionSyntax) (theater.BindingSpec, error) {
	decode, path, err := lowerRelativeClauseSelection(expr)
	if err != nil {
		return theater.BindingSpec{}, err
	}

	object := make(map[string]theater.BindingSpec, 2)
	if decode != "" {
		object["decode"] = theater.BindingSpec{
			Kind:  theater.BindingKindLiteral,
			Value: string(decode),
		}
	}
	if path != "" {
		object["path"] = theater.BindingSpec{
			Kind:  theater.BindingKindLiteral,
			Value: string(path),
		}
	}

	return theater.BindingSpec{
		Kind:   theater.BindingKindObject,
		Object: object,
	}, nil
}

func lowerRelativeClauseSelection(expr expressionSyntax) (decode theater.DecodeKind, path theater.JSONPointer, err error) {
	var steps []callExpressionSyntax

	switch value := ungroupExpression(expr).(type) {
	case callExpressionSyntax:
		steps = []callExpressionSyntax{value}
	case pipelineExpressionSyntax:
		base, ok := ungroupExpression(value.Base).(callExpressionSyntax)
		if !ok {
			return "", "", &lowerError{
				span:    value.Base.ExpressionSpan(),
				message: `relative clause subject may start only with decode(...) or path(...)`,
			}
		}
		steps = append(steps, base)
		steps = append(steps, value.Steps...)
	default:
		return "", "", &lowerError{
			span:    expr.ExpressionSpan(),
			message: `relative clause subject may start only with decode(...) or path(...)`,
		}
	}

	if len(steps) == 0 || (steps[0].Name != selectorStepDecode && steps[0].Name != selectorStepPath) {
		return "", "", &lowerError{
			span:    expr.ExpressionSpan(),
			message: `relative clause subject may start only with decode(...) or path(...)`,
		}
	}

	return lowerRelativeClauseSelectionSteps(steps)
}

func lowerRelativeClauseSelectionSteps(steps []callExpressionSyntax) (decode theater.DecodeKind, path theater.JSONPointer, err error) {
	for i := range steps {
		step := steps[i]
		switch step.Name {
		case selectorStepDecode:
			if decode != "" {
				return "", "", &lowerError{
					span:    step.Span,
					message: "decode step may appear only once",
				}
			}
			if path != "" {
				return "", "", &lowerError{
					span:    step.Span,
					message: "decode step must appear before path in relative clause subject",
				}
			}
			value, err := lowerDecodeKind(step)
			if err != nil {
				return "", "", err
			}
			decode = value
		case selectorStepPath:
			if path != "" {
				return "", "", &lowerError{
					span:    step.Span,
					message: "path step may appear only once in relative clause subject",
				}
			}
			value, err := lowerPathCall(step)
			if err != nil {
				return "", "", err
			}
			path = value
		default:
			return "", "", &lowerError{
				span:    step.Span,
				message: fmt.Sprintf("relative clause subject step %q is not supported", step.Name),
			}
		}
	}

	return decode, path, nil
}

func lowerNestedAssertBinding(assert theater.AssertSpec) theater.BindingSpec {
	args := assert.Args
	if args == nil {
		args = map[string]theater.BindingSpec{}
	}

	return theater.BindingSpec{
		Kind: theater.BindingKindObject,
		Object: map[string]theater.BindingSpec{
			"ref": {
				Kind:  theater.BindingKindLiteral,
				Value: assert.Ref,
			},
			"args": {
				Kind:   theater.BindingKindObject,
				Object: args,
			},
		},
	}
}

func lowerActExports(exports []exportSyntax) ([]theater.ExportSpec, error) {
	if len(exports) == 0 {
		return nil, nil
	}

	lowered := make([]theater.ExportSpec, 0, len(exports))
	for i := range exports {
		export, err := lowerActExport(exports[i])
		if err != nil {
			return nil, err
		}
		lowered = append(lowered, export)
	}

	return lowered, nil
}

func lowerActExport(export exportSyntax) (theater.ExportSpec, error) {
	root, decode, path, through, err := lowerSelection(export.Value)
	if err != nil {
		return theater.ExportSpec{}, err
	}

	result := theater.ExportSpec{
		As:      export.Name,
		Decode:  decode,
		Path:    path,
		Through: through,
	}
	if root.ref != nil {
		result.Ref = root.ref
		result.Decode = ""
		result.Path = ""
		result.Through = nil
		return result, nil
	}
	if root.field != "" {
		result.Field = root.field
		return result, nil
	}

	return theater.ExportSpec{}, &lowerError{
		span:    export.Span,
		message: "export must start with field(...) or $ref",
	}
}

func lowerTransitions(transitions []transitionSyntax) ([]theater.TransitionSpec, error) {
	if len(transitions) == 0 {
		return nil, nil
	}

	lowered := make([]theater.TransitionSpec, 0, len(transitions))
	for i := range transitions {
		outcome, err := lowerTransitionOutcome(transitions[i])
		if err != nil {
			return nil, err
		}
		lowered = append(lowered, theater.TransitionSpec{
			On: outcome,
			To: transitions[i].To,
		})
	}

	return lowered, nil
}

func lowerTransitionOutcome(transition transitionSyntax) (theater.TransitionOutcome, error) {
	switch transition.Event {
	case "pass":
		return theater.TransitionOnPass, nil
	case "fail":
		return theater.TransitionOnFail, nil
	case "timeout":
		return theater.TransitionOnTimeout, nil
	case "cancel":
		return theater.TransitionOnCancel, nil
	default:
		return "", &lowerError{
			span:    transition.Span,
			message: fmt.Sprintf("unsupported transition event %q", transition.Event),
		}
	}
}

func lowerScenarioCall(call scenarioCallSyntax, sourceFile string) (theater.ScenarioCallSpec, error) {
	name, err := lowerOptionalName(call.Name)
	if err != nil {
		return theater.ScenarioCallSpec{}, err
	}

	bindings, err := lowerNamedBindingArgs(call.Bindings, "scenario call")
	if err != nil {
		return theater.ScenarioCallSpec{}, err
	}

	exports, err := lowerScenarioCallExports(call.Exports)
	if err != nil {
		return theater.ScenarioCallSpec{}, err
	}

	dependencies := lowerScenarioCallDependencies(call.Dependencies)

	return theater.ScenarioCallSpec{
		ID:           call.ID,
		Name:         name,
		ScenarioID:   call.ScenarioID,
		Bindings:     bindings,
		Exports:      exports,
		Dependencies: dependencies,
		SourceSpan:   bindSourceSpan(call.Span, sourceFile),
	}, nil
}

func lowerOptionalName(name *nameSyntax) (string, error) {
	if name == nil {
		return "", nil
	}

	return lowerStringValue(name.Value)
}

func lowerScenarioCallDependencies(dependencies []dependencySyntax) []theater.ScenarioDependencySpec {
	if len(dependencies) == 0 {
		return nil
	}

	lowered := make([]theater.ScenarioDependencySpec, 0, len(dependencies))
	for i := range dependencies {
		when := theater.TriggerPredicate(dependencies[i].When)
		if when == "" {
			when = theater.TriggerPredicateSuccess
		}
		lowered = append(lowered, theater.ScenarioDependencySpec{
			CallID: dependencies[i].CallID,
			When:   when,
		})
	}
	return lowered
}

func lowerCaptureAuth(captureAuth captureAuthSyntax) (*theater.HTTPAuthCaptureSpec, error) {
	spec := &theater.HTTPAuthCaptureSpec{
		Auth:  captureAuth.Auth,
		Slots: make(map[string]theater.HTTPCaptureSourceSpec, len(captureAuth.Slots)),
	}
	for i := range captureAuth.Slots {
		slot := captureAuth.Slots[i]
		if _, exists := spec.Slots[slot.Name]; exists {
			return nil, &lowerError{
				span:    slot.Span,
				message: fmt.Sprintf("capture_auth slot %q is duplicated", slot.Name),
			}
		}
		source, err := lowerCaptureSource(slot)
		if err != nil {
			return nil, err
		}
		spec.Slots[slot.Name] = source
	}
	if len(spec.Slots) == 0 {
		spec.Slots = nil
	}

	return spec, nil
}

func lowerCaptureSource(slot mappingEntrySyntax) (theater.HTTPCaptureSourceSpec, error) {
	if len(slot.Mapping) != 0 {
		return theater.HTTPCaptureSourceSpec{}, &lowerError{
			span:    slot.Span,
			message: "capture_auth slot source must be call",
		}
	}

	call, ok := ungroupExpression(slot.Value).(callExpressionSyntax)
	if !ok {
		return theater.HTTPCaptureSourceSpec{}, &lowerError{
			span:    slot.Span,
			message: "capture_auth slot source must be call",
		}
	}

	value, err := lowerCaptureSourceValue(call)
	if err != nil {
		return theater.HTTPCaptureSourceSpec{}, err
	}

	switch call.Name {
	case "response_header":
		return theater.HTTPCaptureSourceSpec{ResponseHeader: value}, nil
	case "response_cookie":
		return theater.HTTPCaptureSourceSpec{ResponseCookie: value}, nil
	case "json_pointer":
		pointer, err := theater.ParseJSONPointer(value)
		if err != nil {
			return theater.HTTPCaptureSourceSpec{}, &lowerError{
				span:    call.Span,
				message: err.Error(),
			}
		}
		return theater.HTTPCaptureSourceSpec{JSONPointer: pointer}, nil
	case "form_field":
		return theater.HTTPCaptureSourceSpec{FormField: value}, nil
	default:
		return theater.HTTPCaptureSourceSpec{}, &lowerError{
			span:    call.Span,
			message: fmt.Sprintf("capture_auth source %q is not supported", call.Name),
		}
	}
}

func lowerCaptureSourceValue(call callExpressionSyntax) (string, error) {
	arg, err := expectSinglePositionalArg(call, call.Name)
	if err != nil {
		return "", err
	}
	return lowerStringValue(arg)
}

func lowerScenarioCallExports(exports []exportSyntax) ([]theater.ExportSpec, error) {
	if len(exports) == 0 {
		return nil, nil
	}

	lowered := make([]theater.ExportSpec, 0, len(exports))
	for i := range exports {
		if exports[i].Assert != nil {
			return nil, &lowerError{
				span:    exports[i].Assert.Span,
				message: "scenario call export assertions are not supported",
			}
		}
		expr := ungroupExpression(exports[i].Value)
		ref, ok := expr.(refExpressionSyntax)
		if !ok {
			return nil, &lowerError{
				span:    exports[i].Value.ExpressionSpan(),
				message: "scenario call export must be direct ref",
			}
		}
		lowered = append(lowered, theater.ExportSpec{
			As: exports[i].Name,
			Ref: &theater.RefSpec{
				Name: ref.Name,
			},
		})
	}

	return lowered, nil
}

func lowerHTTPSection(section stageSectionSyntax) (*theater.HTTPSpec, error) {
	spec := &theater.HTTPSpec{
		Sessions:   map[string]theater.HTTPSessionSpec{},
		Auth:       map[string]theater.HTTPAuthSpec{},
		Identities: map[string]theater.HTTPIdentitySpec{},
	}

	for i := range section.Entries {
		entry := section.Entries[i]
		switch entry.Kind {
		case sectionHTTPSession:
			if len(entry.Call.Args) != 0 {
				return nil, &lowerError{
					span:    entry.Call.Span,
					message: "http session does not accept arguments",
				}
			}
			if _, exists := spec.Sessions[entry.ID]; exists {
				return nil, &lowerError{
					span:    entry.Span,
					message: fmt.Sprintf("http session %q is duplicated", entry.ID),
				}
			}
			spec.Sessions[entry.ID] = theater.HTTPSessionSpec{}
		case sectionHTTPAuth:
			if _, exists := spec.Auth[entry.ID]; exists {
				return nil, &lowerError{
					span:    entry.Span,
					message: fmt.Sprintf("http auth %q is duplicated", entry.ID),
				}
			}
			auth, err := lowerHTTPAuth(entry)
			if err != nil {
				return nil, err
			}
			spec.Auth[entry.ID] = auth
		case sectionHTTPIdentity:
			if _, exists := spec.Identities[entry.ID]; exists {
				return nil, &lowerError{
					span:    entry.Span,
					message: fmt.Sprintf("http identity %q is duplicated", entry.ID),
				}
			}
			args, err := lowerNamedStaticArgs(entry.Call.Args, "http identity")
			if err != nil {
				return nil, err
			}
			identity := theater.HTTPIdentitySpec{}
			if session, ok := args["session"]; ok {
				sessionName, ok := session.(string)
				if !ok {
					return nil, &lowerError{
						span:    entry.Call.Span,
						message: "http identity session must be string",
					}
				}
				identity.Session = sessionName
			}
			if auth, ok := args["auth"]; ok {
				authName, ok := auth.(string)
				if !ok {
					return nil, &lowerError{
						span:    entry.Call.Span,
						message: "http identity auth must be string",
					}
				}
				identity.Auth = authName
			}
			spec.Identities[entry.ID] = identity
		default:
			return nil, &lowerError{
				span:    entry.Span,
				message: fmt.Sprintf("http section entry %q is not supported", entry.Kind),
			}
		}
	}

	if len(spec.Sessions) == 0 {
		spec.Sessions = nil
	}
	if len(spec.Auth) == 0 {
		spec.Auth = nil
	}
	if len(spec.Identities) == 0 {
		spec.Identities = nil
	}

	return spec, nil
}

func lowerHTTPAuth(entry stageSectionEntrySyntax) (theater.HTTPAuthSpec, error) {
	args, err := lowerNamedStaticArgs(entry.Call.Args, "http auth")
	if err != nil {
		return theater.HTTPAuthSpec{}, err
	}

	auth := theater.HTTPAuthSpec{}
	if rawAttach, ok := args["attach"]; ok {
		attach, err := lowerHTTPAttachList(rawAttach, entry.Span)
		if err != nil {
			return theater.HTTPAuthSpec{}, err
		}
		auth.Attach = attach
	}

	return auth, nil
}

func lowerHTTPAttachList(value any, span sourceSpan) ([]theater.HTTPAuthAttachmentSpec, error) {
	rawItems, ok := value.([]any)
	if !ok {
		return nil, &lowerError{
			span:    span,
			message: "http auth attach must be list",
		}
	}

	attachments := make([]theater.HTTPAuthAttachmentSpec, 0, len(rawItems))
	for i := range rawItems {
		attachment, err := lowerHTTPAttachment(rawItems[i], span)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, attachment)
	}

	return attachments, nil
}

func lowerHTTPAttachment(value any, span sourceSpan) (theater.HTTPAuthAttachmentSpec, error) {
	object, ok := value.(map[string]any)
	if !ok || len(object) != 1 {
		return theater.HTTPAuthAttachmentSpec{}, &lowerError{
			span:    span,
			message: "http auth attachment must be single-key object",
		}
	}

	for key, rawConfig := range object {
		config, ok := rawConfig.(map[string]any)
		if !ok {
			return theater.HTTPAuthAttachmentSpec{}, &lowerError{
				span:    span,
				message: fmt.Sprintf("http auth attachment %q must be object", key),
			}
		}
		switch key {
		case "bearer":
			return lowerBearerAttachment(config, span)
		case "basic":
			return lowerBasicAttachment(config, span)
		case "api_key":
			return lowerAPIKeyAttachment(config, span)
		case "header_slot":
			return lowerHeaderSlotAttachment(config, span)
		case "query_slot":
			return lowerQuerySlotAttachment(config, span)
		case "form_slot":
			return lowerFormSlotAttachment(config, span)
		default:
			return theater.HTTPAuthAttachmentSpec{}, &lowerError{
				span:    span,
				message: fmt.Sprintf("http auth attachment %q is not supported", key),
			}
		}
	}

	return theater.HTTPAuthAttachmentSpec{}, &lowerError{
		span:    span,
		message: "http auth attachment is empty",
	}
}

func lowerBearerAttachment(config map[string]any, span sourceSpan) (theater.HTTPAuthAttachmentSpec, error) {
	token, tokenPresent, err := stringField(config, "token", span)
	if err != nil {
		return theater.HTTPAuthAttachmentSpec{}, err
	}
	tokenSlot, tokenSlotPresent, err := stringField(config, "token_slot", span)
	if err != nil {
		return theater.HTTPAuthAttachmentSpec{}, err
	}
	if tokenPresent == tokenSlotPresent {
		return theater.HTTPAuthAttachmentSpec{}, &lowerError{
			span:    span,
			message: `bearer attachment must declare exactly one of "token" or "token_slot"`,
		}
	}

	return theater.HTTPAuthAttachmentSpec{
		Bearer: &theater.HTTPBearerAuthSpec{Token: token, TokenSlot: tokenSlot},
	}, nil
}

func lowerBasicAttachment(config map[string]any, span sourceSpan) (theater.HTTPAuthAttachmentSpec, error) {
	username, err := requireStringField(config, "username", span)
	if err != nil {
		return theater.HTTPAuthAttachmentSpec{}, err
	}
	password, err := requireStringField(config, "password", span)
	if err != nil {
		return theater.HTTPAuthAttachmentSpec{}, err
	}
	return theater.HTTPAuthAttachmentSpec{
		Basic: &theater.HTTPBasicAuthSpec{
			Username: username,
			Password: password,
		},
	}, nil
}

func lowerAPIKeyAttachment(config map[string]any, span sourceSpan) (theater.HTTPAuthAttachmentSpec, error) {
	inValue, err := requireStringField(config, "in", span)
	if err != nil {
		return theater.HTTPAuthAttachmentSpec{}, err
	}
	name, err := requireStringField(config, "name", span)
	if err != nil {
		return theater.HTTPAuthAttachmentSpec{}, err
	}
	keyValue, err := requireStringField(config, "value", span)
	if err != nil {
		return theater.HTTPAuthAttachmentSpec{}, err
	}
	apiKeyIn := theater.HTTPAPIKeyIn(inValue)
	if !apiKeyIn.Valid() {
		return theater.HTTPAuthAttachmentSpec{}, &lowerError{
			span:    span,
			message: fmt.Sprintf("http api_key in %q is not supported", inValue),
		}
	}
	return theater.HTTPAuthAttachmentSpec{
		APIKey: &theater.HTTPAPIKeyAuthSpec{
			In:    apiKeyIn,
			Name:  name,
			Value: keyValue,
		},
	}, nil
}

func lowerHeaderSlotAttachment(config map[string]any, span sourceSpan) (theater.HTTPAuthAttachmentSpec, error) {
	name, err := requireStringField(config, "name", span)
	if err != nil {
		return theater.HTTPAuthAttachmentSpec{}, err
	}
	slot, err := requireStringField(config, "slot", span)
	if err != nil {
		return theater.HTTPAuthAttachmentSpec{}, err
	}
	return theater.HTTPAuthAttachmentSpec{
		HeaderSlot: &theater.HTTPHeaderSlotAuthSpec{Name: name, Slot: slot},
	}, nil
}

func lowerQuerySlotAttachment(config map[string]any, span sourceSpan) (theater.HTTPAuthAttachmentSpec, error) {
	name, err := requireStringField(config, "name", span)
	if err != nil {
		return theater.HTTPAuthAttachmentSpec{}, err
	}
	slot, err := requireStringField(config, "slot", span)
	if err != nil {
		return theater.HTTPAuthAttachmentSpec{}, err
	}
	return theater.HTTPAuthAttachmentSpec{
		QuerySlot: &theater.HTTPQuerySlotAuthSpec{Name: name, Slot: slot},
	}, nil
}

func lowerFormSlotAttachment(config map[string]any, span sourceSpan) (theater.HTTPAuthAttachmentSpec, error) {
	name, err := requireStringField(config, "name", span)
	if err != nil {
		return theater.HTTPAuthAttachmentSpec{}, err
	}
	slot, err := requireStringField(config, "slot", span)
	if err != nil {
		return theater.HTTPAuthAttachmentSpec{}, err
	}
	return theater.HTTPAuthAttachmentSpec{
		FormSlot: &theater.HTTPFormSlotAuthSpec{Name: name, Slot: slot},
	}, nil
}

func requireStringField(object map[string]any, name string, span sourceSpan) (string, error) {
	text, ok, err := stringField(object, name, span)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", &lowerError{
			span:    span,
			message: fmt.Sprintf("field %q is required", name),
		}
	}

	return text, nil
}

func stringField(object map[string]any, name string, span sourceSpan) (text string, ok bool, err error) {
	value, ok := object[name]
	if !ok {
		return "", false, nil
	}
	text, ok = value.(string)
	if !ok {
		return "", false, &lowerError{
			span:    span,
			message: fmt.Sprintf("field %q must be string", name),
		}
	}
	return text, true, nil
}

func lowerStateSection(section stageSectionSyntax) (*theater.StateSpec, stateAliasTable, error) {
	spec := &theater.StateSpec{
		Backends: map[string]theater.StateBackendSpec{},
	}
	aliases := stateAliasTable{}

	for i := range section.Entries {
		entry := section.Entries[i]
		switch entry.Kind {
		case sectionStateBackend:
			if _, exists := spec.Backends[entry.ID]; exists {
				return nil, nil, &lowerError{
					span:    entry.Span,
					message: fmt.Sprintf("state backend %q is duplicated", entry.ID),
				}
			}

			with, err := lowerNamedStaticArgs(entry.Call.Args, "state backend")
			if err != nil {
				return nil, nil, err
			}
			spec.Backends[entry.ID] = theater.StateBackendSpec{
				Use:  entry.Call.Name,
				With: with,
			}
		case sectionStateRecord, sectionStatePool:
			if _, exists := aliases[entry.ID]; exists {
				return nil, nil, &lowerError{
					span:    entry.Span,
					message: fmt.Sprintf("state alias %q is duplicated", entry.ID),
				}
			}

			alias, err := lowerStateAlias(entry)
			if err != nil {
				return nil, nil, err
			}
			aliases[entry.ID] = alias
		default:
			return nil, nil, &lowerError{
				span:    entry.Span,
				message: fmt.Sprintf("state section entry %q is not supported", entry.Kind),
			}
		}
	}

	if len(spec.Backends) == 0 {
		spec.Backends = nil
	}
	if err := validateStateAliases(aliases, spec.Backends); err != nil {
		return nil, nil, err
	}
	if len(aliases) == 0 {
		aliases = nil
	}

	return spec, aliases, nil
}

type selectionRoot struct {
	field string
	ref   *theater.RefSpec
}

func lowerSelection(
	expr expressionSyntax,
) (
	root selectionRoot,
	decode theater.DecodeKind,
	path theater.JSONPointer,
	through []theater.ThroughStepSpec,
	err error,
) {
	var (
		steps []callExpressionSyntax
	)

	switch value := ungroupExpression(expr).(type) {
	case callExpressionSyntax:
		rootCall := value
		if rootCall.Name != selectorRootField {
			return selectionRoot{}, "", "", nil, &lowerError{
				span:    rootCall.Span,
				message: "selector root is not supported",
			}
		}
		field, err := lowerFieldName(rootCall)
		if err != nil {
			return selectionRoot{}, "", "", nil, err
		}
		root.field = field
	case refExpressionSyntax:
		root.ref = &theater.RefSpec{Name: value.Name}
	case pipelineExpressionSyntax:
		ungrouped := ungroupExpression(value.Base)
		switch base := ungrouped.(type) {
		case callExpressionSyntax:
			if base.Name != selectorRootField {
				return selectionRoot{}, "", "", nil, &lowerError{
					span:    base.Span,
					message: "selector root is not supported",
				}
			}
			field, err := lowerFieldName(base)
			if err != nil {
				return selectionRoot{}, "", "", nil, err
			}
			root.field = field
		case refExpressionSyntax:
			root.ref = &theater.RefSpec{Name: base.Name}
		default:
			return selectionRoot{}, "", "", nil, &lowerError{
				span:    value.Base.ExpressionSpan(),
				message: "selector root is not supported",
			}
		}
		steps = value.Steps
	default:
		return selectionRoot{}, "", "", nil, &lowerError{
			span:    expr.ExpressionSpan(),
			message: "selector expression is not supported",
		}
	}

	decode, path, through, err = lowerSelectionSteps(steps)
	if err != nil {
		return selectionRoot{}, "", "", nil, err
	}

	if root.ref != nil {
		root.ref.Decode = decode
		root.ref.Path = path
		root.ref.Through = through
	}

	return root, decode, path, through, nil
}

func lowerSelectionSteps(
	steps []callExpressionSyntax,
) (
	decode theater.DecodeKind,
	path theater.JSONPointer,
	through []theater.ThroughStepSpec,
	err error,
) {
	for i := range steps {
		step := steps[i]
		switch step.Name {
		case selectorStepDecode:
			if decode != "" {
				return "", "", nil, &lowerError{
					span:    step.Span,
					message: "decode step may appear only once",
				}
			}
			if path != "" || len(through) != 0 {
				return "", "", nil, &lowerError{
					span:    step.Span,
					message: "decode step must appear before path and through steps",
				}
			}
			value, err := lowerDecodeKind(step)
			if err != nil {
				return "", "", nil, err
			}
			decode = value
		case selectorStepPath:
			value, err := lowerPathCall(step)
			if err != nil {
				return "", "", nil, err
			}
			if path == "" && len(through) == 0 {
				path = value
				continue
			}
			through = append(through, theater.ThroughStepSpec{Path: value})
		case selectorStepPick:
			pick, err := lowerPickStep(step)
			if err != nil {
				return "", "", nil, err
			}
			through = append(through, theater.ThroughStepSpec{Pick: pick})
		case selectorStepRegexp:
			regexp, err := lowerRegexpStep(step)
			if err != nil {
				return "", "", nil, err
			}
			through = append(through, theater.ThroughStepSpec{Regexp: regexp})
		default:
			if strings.HasPrefix(step.Name, selectorStepTransformPrefix) {
				args, err := lowerNamedStaticArgs(step.Args, "transform call")
				if err != nil {
					return "", "", nil, err
				}
				through = append(through, theater.ThroughStepSpec{
					Transform: &theater.DecoratorSpec{
						Use:  step.Name,
						With: args,
					},
				})
				continue
			}

			return "", "", nil, &lowerError{
				span:    step.Span,
				message: fmt.Sprintf("selector step %q is not supported", step.Name),
			}
		}
	}

	if len(through) == 0 {
		through = nil
	}

	return decode, path, through, nil
}

func lowerFieldName(call callExpressionSyntax) (string, error) {
	arg, err := expectSinglePositionalArg(call, "field")
	if err != nil {
		return "", err
	}
	return lowerBareIdentifierLike(arg)
}

func lowerDecodeKind(call callExpressionSyntax) (theater.DecodeKind, error) {
	arg, err := expectSinglePositionalArg(call, "decode")
	if err != nil {
		return "", err
	}
	value, err := lowerBareIdentifierLike(arg)
	if err != nil {
		return "", err
	}
	switch value {
	case "json":
		return theater.DecodeJSON, nil
	default:
		return "", &lowerError{
			span:    call.Span,
			message: fmt.Sprintf("decode kind %q is not supported", value),
		}
	}
}

func lowerPathCall(call callExpressionSyntax) (theater.JSONPointer, error) {
	arg, err := expectSinglePositionalArg(call, "path")
	if err != nil {
		return "", err
	}
	value, err := lowerStringValue(arg)
	if err != nil {
		return "", err
	}
	path, err := theater.ParseJSONPointer(value)
	if err != nil {
		return "", &lowerError{
			span:    call.Span,
			message: err.Error(),
		}
	}
	return path, nil
}

func lowerPickStep(call callExpressionSyntax) (*theater.PickStepSpec, error) {
	if len(call.Clauses) != 0 {
		if len(call.Args) != 0 {
			return nil, &lowerError{
				span:    call.Span,
				message: "pick where cannot be combined with call arguments",
			}
		}

		where, err := lowerPickWhereClauses(call.Clauses)
		if err != nil {
			return nil, err
		}

		return &theater.PickStepSpec{
			Where: where,
		}, nil
	}

	args, err := lowerNamedBindingArgs(call.Args, "pick")
	if err != nil {
		return nil, err
	}

	rawAt, ok := args["at"]
	if !ok {
		return nil, &lowerError{
			span:    call.Span,
			message: `pick requires "at"`,
		}
	}
	if rawAt.Kind != theater.BindingKindLiteral {
		return nil, &lowerError{
			span:    call.Span,
			message: `pick "at" must be literal string`,
		}
	}
	atValue, ok := rawAt.Value.(string)
	if !ok {
		return nil, &lowerError{
			span:    call.Span,
			message: `pick "at" must be literal string`,
		}
	}
	at, err := theater.ParseJSONPointer(atValue)
	if err != nil {
		return nil, &lowerError{
			span:    call.Span,
			message: err.Error(),
		}
	}

	equals, ok := args["equals"]
	if !ok {
		return nil, &lowerError{
			span:    call.Span,
			message: `pick requires "equals"`,
		}
	}

	return &theater.PickStepSpec{
		At:     at,
		Equals: equals,
	}, nil
}

func lowerPickWhereClauses(clauses []relativeClauseSyntax) ([]theater.PickWhereClauseSpec, error) {
	where := make([]theater.PickWhereClauseSpec, 0, len(clauses))
	for i := range clauses {
		clause, err := lowerPickWhereClause(clauses[i])
		if err != nil {
			return nil, err
		}
		where = append(where, clause)
	}

	return where, nil
}

func lowerPickWhereClause(clause relativeClauseSyntax) (theater.PickWhereClauseSpec, error) {
	decode, path, err := lowerRelativeClauseSelection(clause.Subject)
	if err != nil {
		return theater.PickWhereClauseSpec{}, err
	}

	assert, err := lowerAssertion(clause.Assert)
	if err != nil {
		return theater.PickWhereClauseSpec{}, err
	}

	return theater.PickWhereClauseSpec{
		Subject: theater.RelativeSubjectSpec{
			Decode: decode,
			Path:   path,
		},
		Assert: assert,
	}, nil
}

func lowerRegexpStep(call callExpressionSyntax) (*theater.RegexpStepSpec, error) {
	argsByName, err := indexArgumentsByName(call.Args, "regexp")
	if err != nil {
		return nil, err
	}
	patternArg, ok := argsByName["pattern"]
	if !ok {
		return nil, &lowerError{
			span:    call.Span,
			message: `regexp requires "pattern"`,
		}
	}
	pattern, err := lowerStringValue(patternArg.Value)
	if err != nil {
		return nil, err
	}

	group := 0
	if groupArg, ok := argsByName["group"]; ok {
		group, err = lowerIntValue(groupArg.Value)
		if err != nil {
			return nil, err
		}
	}

	return &theater.RegexpStepSpec{
		Pattern: pattern,
		Group:   group,
	}, nil
}

func lowerNamedBindingArgs(args []callArgumentSyntax, context string) (map[string]theater.BindingSpec, error) {
	if len(args) == 0 {
		return map[string]theater.BindingSpec{}, nil
	}

	indexed, err := indexArgumentsByName(args, context)
	if err != nil {
		return nil, err
	}

	lowered := make(map[string]theater.BindingSpec, len(indexed))
	for name, arg := range indexed {
		binding, err := lowerArgumentBinding(arg)
		if err != nil {
			return nil, err
		}
		lowered[name] = binding
	}
	return lowered, nil
}

func lowerNamedStaticArgs(args []callArgumentSyntax, context string) (map[string]any, error) {
	if len(args) == 0 {
		return map[string]any{}, nil
	}

	indexed, err := indexArgumentsByName(args, context)
	if err != nil {
		return nil, err
	}

	lowered := make(map[string]any, len(indexed))
	for name, arg := range indexed {
		value, err := lowerArgumentStaticValue(arg)
		if err != nil {
			return nil, err
		}
		lowered[name] = value
	}
	return lowered, nil
}

func lowerNamedBindingArgsWithStateAliases(
	args []callArgumentSyntax,
	context string,
	actionUse string,
	aliases stateAliasTable,
) (bindings map[string]theater.BindingSpec, hiddenProperties map[string]theater.PropertySpec, err error) {
	if len(args) == 0 {
		return map[string]theater.BindingSpec{}, nil, nil
	}

	indexed, err := indexArgumentsByName(args, context)
	if err != nil {
		return nil, nil, err
	}

	lowered := make(map[string]theater.BindingSpec, len(indexed))
	hiddenProperties = make(map[string]theater.PropertySpec)
	createdHiddenRefs := make(map[string]string)
	requirements := stateAliasActionArgKinds(actionUse)
	for name, arg := range indexed {
		if kind, ok := requirements[name]; ok {
			binding, hiddenID, hiddenProperty, matched, err := lowerStateAliasBindingArg(arg, kind, aliases, createdHiddenRefs)
			if err != nil {
				return nil, nil, err
			}
			if matched {
				lowered[name] = binding
				hiddenProperties[hiddenID] = hiddenProperty
				continue
			}
		}
		if binding, matched, err := lowerStateActionSpecialBinding(actionUse, name, arg); matched || err != nil {
			if err != nil {
				return nil, nil, err
			}
			lowered[name] = binding
			continue
		}

		binding, err := lowerArgumentBinding(arg)
		if err != nil {
			return nil, nil, err
		}
		lowered[name] = binding
	}

	if len(hiddenProperties) == 0 {
		hiddenProperties = nil
	}

	return lowered, hiddenProperties, nil
}

func lowerStateActionSpecialBinding(
	actionUse string,
	name string,
	arg callArgumentSyntax,
) (binding theater.BindingSpec, matched bool, err error) {
	if actionUse == stateClaimActionCall && name == stateClaimArgLease {
		binding, err := lowerStateClaimLeaseBinding(arg)
		return binding, true, err
	}

	return theater.BindingSpec{}, false, nil
}

func lowerStateClaimLeaseBinding(arg callArgumentSyntax) (theater.BindingSpec, error) {
	if len(arg.Mapping) != 0 {
		object, err := lowerStateClaimLeaseEntries(arg.Mapping)
		if err != nil {
			return theater.BindingSpec{}, err
		}
		return theater.BindingSpec{
			Kind:   theater.BindingKindObject,
			Object: object,
		}, nil
	}

	object, ok := ungroupExpression(arg.Value).(objectExpressionSyntax)
	if !ok {
		return lowerArgumentBinding(arg)
	}

	lowered, err := lowerStateClaimLeaseEntries(object.Fields)
	if err != nil {
		return theater.BindingSpec{}, err
	}
	return theater.BindingSpec{
		Kind:   theater.BindingKindObject,
		Object: lowered,
	}, nil
}

func lowerStateClaimLeaseEntries(entries []mappingEntrySyntax) (map[string]theater.BindingSpec, error) {
	if len(entries) == 0 {
		return map[string]theater.BindingSpec{}, nil
	}

	object := make(map[string]theater.BindingSpec, len(entries))
	for i := range entries {
		entry := entries[i]
		if entry.Name == stateClaimLeaseOnExpiry && len(entry.Mapping) == 0 {
			if symbol, ok := ungroupExpression(entry.Value).(symbolExpressionSyntax); ok {
				object[entry.Name] = theater.BindingSpec{
					Kind:  theater.BindingKindLiteral,
					Value: symbol.Name,
				}
				continue
			}
		}
		if len(entry.Mapping) != 0 {
			child, err := lowerMappingObject(entry.Mapping)
			if err != nil {
				return nil, err
			}
			object[entry.Name] = theater.BindingSpec{
				Kind:   theater.BindingKindObject,
				Object: child,
			}
			continue
		}
		binding, err := lowerBindingExpression(entry.Value)
		if err != nil {
			return nil, err
		}
		object[entry.Name] = binding
	}

	return object, nil
}

func indexArgumentsByName(args []callArgumentSyntax, context string) (map[string]callArgumentSyntax, error) {
	indexed := make(map[string]callArgumentSyntax, len(args))
	for i := range args {
		arg := args[i]
		if arg.Name == "" {
			return nil, &lowerError{
				span:    arg.Span,
				message: context + " requires named arguments",
			}
		}
		if _, exists := indexed[arg.Name]; exists {
			return nil, &lowerError{
				span:    arg.Span,
				message: fmt.Sprintf("%s argument %q is duplicated", context, arg.Name),
			}
		}
		indexed[arg.Name] = arg
	}
	return indexed, nil
}

func lowerArgumentBinding(arg callArgumentSyntax) (theater.BindingSpec, error) {
	if len(arg.Mapping) != 0 {
		object, err := lowerMappingObject(arg.Mapping)
		if err != nil {
			return theater.BindingSpec{}, err
		}
		return theater.BindingSpec{
			Kind:   theater.BindingKindObject,
			Object: object,
		}, nil
	}

	return lowerBindingExpression(arg.Value)
}

func lowerArgumentStaticValue(arg callArgumentSyntax) (any, error) {
	if len(arg.Mapping) != 0 {
		return lowerStaticMappingObject(arg.Mapping)
	}

	return lowerStaticExpression(arg.Value)
}

func lowerMappingObject(entries []mappingEntrySyntax) (map[string]theater.BindingSpec, error) {
	if len(entries) == 0 {
		return map[string]theater.BindingSpec{}, nil
	}

	object := make(map[string]theater.BindingSpec, len(entries))
	for i := range entries {
		entry := entries[i]
		if len(entry.Mapping) != 0 {
			child, err := lowerMappingObject(entry.Mapping)
			if err != nil {
				return nil, err
			}
			object[entry.Name] = theater.BindingSpec{
				Kind:   theater.BindingKindObject,
				Object: child,
			}
			continue
		}
		binding, err := lowerBindingExpression(entry.Value)
		if err != nil {
			return nil, err
		}
		object[entry.Name] = binding
	}
	return object, nil
}

func lowerStaticMappingObject(entries []mappingEntrySyntax) (map[string]any, error) {
	if len(entries) == 0 {
		return map[string]any{}, nil
	}

	object := make(map[string]any, len(entries))
	for i := range entries {
		entry := entries[i]
		if len(entry.Mapping) != 0 {
			child, err := lowerStaticMappingObject(entry.Mapping)
			if err != nil {
				return nil, err
			}
			object[entry.Name] = child
			continue
		}
		value, err := lowerStaticExpression(entry.Value)
		if err != nil {
			return nil, err
		}
		object[entry.Name] = value
	}
	return object, nil
}

func lowerBindingExpression(expr expressionSyntax) (theater.BindingSpec, error) {
	switch value := ungroupExpression(expr).(type) {
	case literalExpressionSyntax:
		return lowerLiteralBinding(value)
	case refExpressionSyntax:
		return theater.BindingSpec{
			Kind: theater.BindingKindRef,
			Ref: &theater.RefSpec{
				Name: value.Name,
			},
		}, nil
	case objectExpressionSyntax:
		object, err := lowerInlineObject(value.Fields)
		if err != nil {
			return theater.BindingSpec{}, err
		}
		return theater.BindingSpec{
			Kind:   theater.BindingKindObject,
			Object: object,
		}, nil
	case listExpressionSyntax:
		list, err := lowerInlineList(value.Items)
		if err != nil {
			return theater.BindingSpec{}, err
		}
		return theater.BindingSpec{
			Kind: theater.BindingKindList,
			List: list,
		}, nil
	case callExpressionSyntax:
		switch {
		case value.Name == string(tokenString):
			return lowerStringCall(value)
		case value.Name == bindingCallCoalesce:
			return lowerCoalesceCall(value)
		case value.Name == bindingCallEnv:
			return lowerEnvCall(value)
		case strings.HasPrefix(value.Name, "generate."):
			return lowerGenerateCall(value)
		default:
			return theater.BindingSpec{}, &lowerError{
				span:    value.Span,
				message: fmt.Sprintf("binding call %q is not supported", value.Name),
			}
		}
	case pipelineExpressionSyntax:
		root, _, _, _, err := lowerSelection(value)
		if err != nil {
			return theater.BindingSpec{}, err
		}
		if root.ref == nil {
			return theater.BindingSpec{}, &lowerError{
				span:    value.Span,
				message: "binding pipeline must start with $ref",
			}
		}
		return theater.BindingSpec{
			Kind: theater.BindingKindRef,
			Ref:  root.ref,
		}, nil
	case symbolExpressionSyntax:
		return theater.BindingSpec{}, &lowerError{
			span:    value.Span,
			message: "bare symbol is not valid binding value",
		}
	default:
		return theater.BindingSpec{}, &lowerError{
			span:    expr.ExpressionSpan(),
			message: "binding expression is not supported",
		}
	}
}

func lowerCoalesceCall(call callExpressionSyntax) (theater.BindingSpec, error) {
	if len(call.Args) == 0 {
		return theater.BindingSpec{}, &lowerError{
			span:    call.Span,
			message: "coalesce(...) requires at least one candidate",
		}
	}

	candidates := make([]theater.BindingSpec, 0, len(call.Args))
	for i := range call.Args {
		if call.Args[i].Name != "" {
			return theater.BindingSpec{}, &lowerError{
				span:    call.Args[i].Span,
				message: "coalesce(...) accepts positional candidates only",
			}
		}

		candidate, err := lowerBindingExpression(call.Args[i].Value)
		if err != nil {
			return theater.BindingSpec{}, err
		}
		candidates = append(candidates, candidate)
	}

	return theater.BindingSpec{
		Kind:       theater.BindingKindCoalesce,
		Candidates: candidates,
	}, nil
}

func lowerEnvCall(call callExpressionSyntax) (theater.BindingSpec, error) {
	arg, err := expectSinglePositionalArg(call, "env")
	if err != nil {
		return theater.BindingSpec{}, err
	}

	name, err := lowerStringValue(arg)
	if err != nil {
		return theater.BindingSpec{}, err
	}
	if name == "" {
		return theater.BindingSpec{}, &lowerError{
			span:    call.Span,
			message: "env(...) name must be non-empty",
		}
	}

	return theater.BindingSpec{
		Kind: theater.BindingKindEnv,
		Env:  name,
	}, nil
}

func lowerInlineObject(entries []mappingEntrySyntax) (map[string]theater.BindingSpec, error) {
	object := make(map[string]theater.BindingSpec, len(entries))
	for i := range entries {
		entry := entries[i]
		binding, err := lowerBindingExpression(entry.Value)
		if err != nil {
			return nil, err
		}
		object[entry.Name] = binding
	}
	if len(object) == 0 {
		return map[string]theater.BindingSpec{}, nil
	}
	return object, nil
}

func lowerInlineList(items []expressionSyntax) ([]theater.BindingSpec, error) {
	list := make([]theater.BindingSpec, 0, len(items))
	for i := range items {
		binding, err := lowerBindingExpression(items[i])
		if err != nil {
			return nil, err
		}
		list = append(list, binding)
	}
	if len(list) == 0 {
		return nil, nil
	}
	return list, nil
}

func lowerLiteralBinding(literal literalExpressionSyntax) (theater.BindingSpec, error) {
	switch literal.Kind {
	case literalKindString:
		value, err := decodeStringLiteral(literal)
		if err != nil {
			return theater.BindingSpec{}, err
		}
		parts, ok, err := lowerInterpolatedString(value, literal.Span)
		if err != nil {
			return theater.BindingSpec{}, err
		}
		if ok {
			return theater.BindingSpec{
				Kind:  theater.BindingKindString,
				Parts: parts,
			}, nil
		}
		return theater.BindingSpec{
			Kind:  theater.BindingKindLiteral,
			Value: value,
		}, nil
	case literalKindMultilineString:
		value, err := decodeStringLiteral(literal)
		if err != nil {
			return theater.BindingSpec{}, err
		}
		return theater.BindingSpec{
			Kind:  theater.BindingKindLiteral,
			Value: value,
		}, nil
	case literalKindRawString:
		value, err := decodeStringLiteral(literal)
		if err != nil {
			return theater.BindingSpec{}, err
		}
		return theater.BindingSpec{
			Kind:  theater.BindingKindLiteral,
			Value: value,
		}, nil
	case literalKindNumber:
		number, err := lowerNumberLiteral(literal)
		if err != nil {
			return theater.BindingSpec{}, err
		}
		return theater.BindingSpec{
			Kind:  theater.BindingKindLiteral,
			Value: number,
		}, nil
	case literalKindBool:
		return theater.BindingSpec{
			Kind:  theater.BindingKindLiteral,
			Value: literal.Text == "true",
		}, nil
	case literalKindNull:
		return theater.BindingSpec{
			Kind:  theater.BindingKindLiteral,
			Value: nil,
		}, nil
	case literalKindDuration:
		return theater.BindingSpec{
			Kind:  theater.BindingKindLiteral,
			Value: literal.Text,
		}, nil
	default:
		return theater.BindingSpec{}, &lowerError{
			span:    literal.Span,
			message: "literal kind is not supported",
		}
	}
}

func lowerStaticExpression(expr expressionSyntax) (any, error) {
	switch value := ungroupExpression(expr).(type) {
	case literalExpressionSyntax:
		binding, err := lowerLiteralBinding(value)
		if err != nil {
			return nil, err
		}
		return binding.Value, nil
	case symbolExpressionSyntax:
		return value.Name, nil
	case objectExpressionSyntax:
		return lowerStaticInlineObject(value.Fields)
	case listExpressionSyntax:
		return lowerStaticInlineList(value.Items)
	default:
		return nil, &lowerError{
			span:    expr.ExpressionSpan(),
			message: "static expression is not supported",
		}
	}
}

func lowerStateAlias(entry stageSectionEntrySyntax) (stateAliasSpec, error) {
	kind, ok := stateAliasKindForEntry(entry.Kind)
	if !ok {
		return stateAliasSpec{}, &lowerError{
			span:    entry.Span,
			message: fmt.Sprintf("state alias kind %q is not supported", entry.Kind),
		}
	}
	if got, want := entry.Call.Name, stateAliasCallForKind(kind); got != want {
		return stateAliasSpec{}, &lowerError{
			span:    entry.Call.Span,
			message: fmt.Sprintf("state %s alias must use %q", entry.Kind, want),
		}
	}

	staticArgs, err := lowerNamedStaticArgs(entry.Call.Args, "state alias")
	if err != nil {
		return stateAliasSpec{}, err
	}
	with, err := lowerStaticBindings(staticArgs)
	if err != nil {
		return stateAliasSpec{}, err
	}

	return stateAliasSpec{
		kind:     kind,
		span:     entry.Span,
		with:     with,
		argSpans: argumentSpans(entry.Call.Args),
	}, nil
}

func stateAliasActionArgKinds(actionUse string) map[string]stateAliasKind {
	switch canonicalStateActionUse(actionUse) {
	case stateReadActionCall, stateUpdateActionCall:
		return map[string]stateAliasKind{"record": stateAliasKindRecord}
	case "action.state.claim":
		return map[string]stateAliasKind{"pool": stateAliasKindPool}
	default:
		return nil
	}
}

func canonicalStateActionUse(actionUse string) string {
	switch actionUse {
	case stateReadSugarCall:
		return stateReadActionCall
	case stateUpdateSugarCall:
		return stateUpdateActionCall
	case stateClaimSugarCall:
		return stateClaimActionCall
	case stateRenewSugarCall:
		return stateRenewActionCall
	case stateReleaseSugarCall:
		return stateReleaseActionCall
	case stateConsumeSugarCall:
		return stateConsumeActionCall
	default:
		return actionUse
	}
}

func canonicalizeStateActionCall(call callExpressionSyntax) (callExpressionSyntax, error) {
	if call.Name == stateRemovedCASSugarCall {
		return callExpressionSyntax{}, &lowerError{
			span:    call.Span,
			message: "state.cas has been removed; use state.update(... if_version: ...)",
		}
	}

	canonical := call
	canonical.Name = canonicalStateActionUse(call.Name)

	switch call.Name {
	case stateUpdateSugarCall:
		args, err := canonicalizeStateUpdateArgs(call.Args, call.Span)
		if err != nil {
			return callExpressionSyntax{}, err
		}
		canonical.Args = args
		return canonical, nil
	case stateClaimSugarCall:
		args, err := canonicalizeStateClaimArgs(call.Args)
		if err != nil {
			return callExpressionSyntax{}, err
		}
		canonical.Args = args
		return canonical, nil
	default:
		return canonical, nil
	}
}

func canonicalizeStateUpdateArgs(args []callArgumentSyntax, callSpan sourceSpan) ([]callArgumentSyntax, error) {
	indexed, err := indexArgumentsByName(args, "state.update")
	if err != nil {
		return nil, err
	}

	if _, hasExpectedVersion := indexed["expected_version"]; hasExpectedVersion {
		return nil, &lowerError{
			span:    indexed["expected_version"].Span,
			message: "state.update uses if_version; expected_version is the canonical action field",
		}
	}

	if _, hasIfVersion := indexed["if_version"]; !hasIfVersion {
		return nil, &lowerError{
			span:    callSpan,
			message: "state.update requires if_version",
		}
	}

	canonical := make([]callArgumentSyntax, 0, len(args))
	for i := range args {
		arg := args[i]
		if arg.Name == "if_version" {
			arg.Name = "expected_version"
		}
		canonical = append(canonical, arg)
	}

	return canonical, nil
}

func canonicalizeStateClaimArgs(args []callArgumentSyntax) ([]callArgumentSyntax, error) {
	indexed, err := indexArgumentsByName(args, "state.claim")
	if err != nil {
		return nil, err
	}

	selectorArg, hasSelector := indexed["selector"]
	idArg, hasID := indexed[stateClaimArgID]
	fieldsArg, hasFields := indexed[stateClaimArgFields]
	whereArg, hasWhere := indexed["where"]
	if hasWhere {
		return nil, &lowerError{
			span:    whereArg.Span,
			message: "state.claim where has been removed; use fields:",
		}
	}
	if hasSelector && (hasID || hasFields) {
		return nil, &lowerError{
			span:    selectorArg.Span,
			message: "state.claim selector cannot be combined with id or fields",
		}
	}

	canonical := make([]callArgumentSyntax, 0, len(args))
	for i := range args {
		switch args[i].Name {
		case stateClaimArgID, stateClaimArgFields:
			continue
		default:
			canonical = append(canonical, args[i])
		}
	}

	if hasSelector {
		return canonical, nil
	}

	selectorSugar, ok, err := lowerStateClaimSelectorSugar(idArg, hasID, fieldsArg, hasFields)
	if err != nil {
		return nil, err
	}
	if ok {
		canonical = append(canonical, selectorSugar)
	}

	return canonical, nil
}

func lowerStateClaimSelectorSugar(
	idArg callArgumentSyntax,
	hasID bool,
	fieldsArg callArgumentSyntax,
	hasFields bool,
) (callArgumentSyntax, bool, error) {
	if !hasID && !hasFields {
		return callArgumentSyntax{}, false, nil
	}

	entries := make([]mappingEntrySyntax, 0, 2)
	var spans []sourceSpan
	if hasID {
		entries = append(entries, mappingEntrySyntax{
			Name:  stateClaimArgID,
			Value: idArg.Value,
			Span:  idArg.Span,
		})
		spans = append(spans, idArg.Span)
	}
	if hasFields {
		fields, err := stateClaimFieldEntries(fieldsArg)
		if err != nil {
			return callArgumentSyntax{}, false, err
		}
		fieldsSpan := combineSourceSpans(fieldsArg.Span, mappingEntriesSpan(fields))
		entries = append(entries, mappingEntrySyntax{
			Name: stateClaimArgFields,
			Value: objectExpressionSyntax{
				Dynamic: true,
				Fields:  fields,
				Span:    fieldsSpan,
			},
			Span: fieldsArg.Span,
		})
		spans = append(spans, fieldsArg.Span)
	}

	selectorSpan := combineSourceSpans(spans...)
	return callArgumentSyntax{
		Name: "selector",
		Value: objectExpressionSyntax{
			Dynamic: true,
			Fields:  entries,
			Span:    selectorSpan,
		},
		Span: selectorSpan,
	}, true, nil
}

func stateClaimFieldEntries(arg callArgumentSyntax) ([]mappingEntrySyntax, error) {
	var entries []mappingEntrySyntax
	if len(arg.Mapping) != 0 {
		entries = arg.Mapping
	} else {
		switch value := ungroupExpression(arg.Value).(type) {
		case objectExpressionSyntax:
			entries = value.Fields
		default:
			return nil, &lowerError{
				span:    arg.Span,
				message: "state.claim fields must be object with exact top-level fields",
			}
		}
	}

	for i := range entries {
		if err := validateStateClaimFieldEntry(entries[i]); err != nil {
			return nil, err
		}
	}

	return entries, nil
}

func validateStateClaimFieldEntry(entry mappingEntrySyntax) error {
	if len(entry.Mapping) != 0 {
		return &lowerError{
			span:    entry.Span,
			message: "state.claim fields only supports exact top-level field matching",
		}
	}

	switch ungroupExpression(entry.Value).(type) {
	case objectExpressionSyntax, listExpressionSyntax:
		return &lowerError{
			span:    entry.Span,
			message: "state.claim fields only supports exact top-level field matching",
		}
	default:
		return nil
	}
}

func combineSourceSpans(spans ...sourceSpan) sourceSpan {
	var combined sourceSpan
	for _, span := range spans {
		if span == (sourceSpan{}) {
			continue
		}
		if combined == (sourceSpan{}) || span.Start.Offset < combined.Start.Offset {
			combined.Start = span.Start
		}
		if combined == (sourceSpan{}) || span.End.Offset > combined.End.Offset {
			combined.End = span.End
		}
	}

	return combined
}

func mappingEntriesSpan(entries []mappingEntrySyntax) sourceSpan {
	if len(entries) == 0 {
		return sourceSpan{}
	}

	return combineSourceSpans(entries[0].Span, entries[len(entries)-1].Span)
}

func lowerStateAliasBindingArg(
	arg callArgumentSyntax,
	kind stateAliasKind,
	aliases stateAliasTable,
	createdHiddenRefs map[string]string,
) (binding theater.BindingSpec, hiddenID string, hiddenProperty theater.PropertySpec, matched bool, err error) {
	if len(arg.Mapping) != 0 {
		return theater.BindingSpec{}, "", theater.PropertySpec{}, false, nil
	}

	symbol, ok := ungroupExpression(arg.Value).(symbolExpressionSyntax)
	if !ok {
		return theater.BindingSpec{}, "", theater.PropertySpec{}, false, nil
	}

	alias, ok := aliases[symbol.Name]
	if !ok {
		return theater.BindingSpec{}, "", theater.PropertySpec{}, false, nil
	}
	if alias.kind != kind {
		return theater.BindingSpec{}, "", theater.PropertySpec{}, false, &lowerError{
			span: symbol.Span,
			message: fmt.Sprintf(
				`state action arg %q requires %s alias, got %s alias %q`,
				arg.Name,
				stateAliasKindLabel(kind),
				stateAliasKindLabel(alias.kind),
				symbol.Name,
			),
		}
	}

	hiddenID, matched = createdHiddenRefs[symbol.Name]
	if !matched {
		hiddenID = hiddenStateAliasRef(kind, symbol.Name)
		createdHiddenRefs[symbol.Name] = hiddenID
	}

	return theater.BindingSpec{
		Kind: theater.BindingKindRef,
		Ref: &theater.RefSpec{
			Name: hiddenID,
		},
	}, hiddenID, alias.propertySpec(), true, nil
}

func hiddenStateAliasRef(kind stateAliasKind, alias string) string {
	return hiddenStateAliasPrefix + ":" + string(kind) + ":" + alias
}

func stateAliasKindForEntry(kind string) (stateAliasKind, bool) {
	switch kind {
	case sectionStateRecord:
		return stateAliasKindRecord, true
	case sectionStatePool:
		return stateAliasKindPool, true
	default:
		return "", false
	}
}

func stateAliasCallForKind(kind stateAliasKind) string {
	switch kind {
	case stateAliasKindRecord:
		return stateRecordAliasCall
	case stateAliasKindPool:
		return statePoolAliasCall
	default:
		return ""
	}
}

func stateAliasKindLabel(kind stateAliasKind) string {
	return string(kind)
}

func (s stateAliasSpec) propertySpec() theater.PropertySpec {
	return theater.PropertySpec{
		Inventory: &theater.InventoryCall{
			Use:  s.inventoryUse(),
			With: cloneBindingMap(s.with),
		},
	}
}

func (s stateAliasSpec) inventoryUse() string {
	switch s.kind {
	case stateAliasKindRecord:
		return stateRecordInventoryCall
	case stateAliasKindPool:
		return statePoolInventoryCall
	default:
		return ""
	}
}

func lowerStaticBindings(values map[string]any) (map[string]theater.BindingSpec, error) {
	if len(values) == 0 {
		return map[string]theater.BindingSpec{}, nil
	}

	bindings := make(map[string]theater.BindingSpec, len(values))
	for key, value := range values {
		binding, err := lowerStaticBindingValue(value)
		if err != nil {
			return nil, err
		}
		bindings[key] = binding
	}
	return bindings, nil
}

func validateStateAliases(aliases stateAliasTable, backends map[string]theater.StateBackendSpec) error {
	if len(aliases) == 0 {
		return nil
	}

	names := make([]string, 0, len(aliases))
	for name := range aliases {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if err := validateStateAlias(name, aliases[name], backends); err != nil {
			return err
		}
	}

	return nil
}

func validateStateAlias(name string, alias stateAliasSpec, backends map[string]theater.StateBackendSpec) error {
	backend, ok := bindingLiteralString(alias.with["backend"])
	if !ok || backend == "" {
		return &lowerError{
			span:    alias.argSpan("backend"),
			message: fmt.Sprintf(`state %s alias %q requires backend`, alias.kind, name),
		}
	}
	if _, ok := backends[backend]; !ok {
		return &lowerError{
			span:    alias.argSpan("backend"),
			message: fmt.Sprintf(`state alias %q references unknown backend %q`, name, backend),
		}
	}

	targetField := alias.kind.targetField()
	targetValue, ok := bindingLiteralString(alias.with[targetField])
	if !ok || targetValue == "" {
		return &lowerError{
			span:    alias.argSpan(targetField),
			message: fmt.Sprintf(`state %s alias %q requires %s`, alias.kind, name, targetField),
		}
	}

	guarantee, ok := bindingLiteralString(alias.with["min_guarantee"])
	if !ok || guarantee == "" {
		return &lowerError{
			span:    alias.argSpan("min_guarantee"),
			message: fmt.Sprintf(`state %s alias %q requires min_guarantee`, alias.kind, name),
		}
	}
	if tier := theater.StateGuaranteeTier(guarantee); !tier.Valid() {
		return &lowerError{
			span:    alias.argSpan("min_guarantee"),
			message: fmt.Sprintf(`state min_guarantee %q is invalid`, guarantee),
		}
	}

	return nil
}

func bindingLiteralString(binding theater.BindingSpec) (string, bool) {
	if binding.Kind != theater.BindingKindLiteral {
		return "", false
	}

	value, ok := binding.Value.(string)
	return value, ok
}

func argumentSpans(args []callArgumentSyntax) map[string]sourceSpan {
	if len(args) == 0 {
		return nil
	}

	spans := make(map[string]sourceSpan, len(args))
	for i := range args {
		if args[i].Name == "" {
			continue
		}
		spans[args[i].Name] = args[i].Span
	}

	return spans
}

func (k stateAliasKind) targetField() string {
	switch k {
	case stateAliasKindRecord:
		return "record"
	case stateAliasKindPool:
		return "pool"
	default:
		return ""
	}
}

func (s stateAliasSpec) argSpan(name string) sourceSpan {
	if span, ok := s.argSpans[name]; ok {
		return span
	}
	return s.span
}

func lowerStaticBindingValue(value any) (theater.BindingSpec, error) {
	switch typed := value.(type) {
	case nil, string, bool, int, float64:
		return theater.BindingSpec{
			Kind:  theater.BindingKindLiteral,
			Value: typed,
		}, nil
	case map[string]any:
		object, err := lowerStaticBindings(typed)
		if err != nil {
			return theater.BindingSpec{}, err
		}
		return theater.BindingSpec{
			Kind:   theater.BindingKindObject,
			Object: object,
		}, nil
	case []any:
		list := make([]theater.BindingSpec, 0, len(typed))
		for i := range typed {
			binding, err := lowerStaticBindingValue(typed[i])
			if err != nil {
				return theater.BindingSpec{}, err
			}
			list = append(list, binding)
		}
		return theater.BindingSpec{
			Kind: theater.BindingKindList,
			List: list,
		}, nil
	default:
		return theater.BindingSpec{}, fmt.Errorf("unsupported static binding value type %T", value)
	}
}

func cloneBindingMap(source map[string]theater.BindingSpec) map[string]theater.BindingSpec {
	if len(source) == 0 {
		return map[string]theater.BindingSpec{}
	}

	cloned := make(map[string]theater.BindingSpec, len(source))
	for key := range source {
		cloned[key] = cloneBindingSpec(source[key])
	}
	return cloned
}

func cloneBindingSpec(spec theater.BindingSpec) theater.BindingSpec {
	cloned := spec
	if spec.Ref != nil {
		ref := *spec.Ref
		cloned.Ref = &ref
	}
	if len(spec.Object) != 0 {
		cloned.Object = cloneBindingMap(spec.Object)
	}
	if len(spec.List) != 0 {
		cloned.List = make([]theater.BindingSpec, len(spec.List))
		for i := range spec.List {
			cloned.List[i] = cloneBindingSpec(spec.List[i])
		}
	}
	if len(spec.Parts) != 0 {
		cloned.Parts = make([]theater.BindingSpec, len(spec.Parts))
		for i := range spec.Parts {
			cloned.Parts[i] = cloneBindingSpec(spec.Parts[i])
		}
	}
	if len(spec.Args) != 0 {
		cloned.Args = cloneBindingMap(spec.Args)
	}
	if len(spec.Candidates) != 0 {
		cloned.Candidates = make([]theater.BindingSpec, len(spec.Candidates))
		for i := range spec.Candidates {
			cloned.Candidates[i] = cloneBindingSpec(spec.Candidates[i])
		}
	}
	return cloned
}

func lowerStaticInlineObject(entries []mappingEntrySyntax) (map[string]any, error) {
	object := make(map[string]any, len(entries))
	for i := range entries {
		value, err := lowerStaticExpression(entries[i].Value)
		if err != nil {
			return nil, err
		}
		object[entries[i].Name] = value
	}
	if len(object) == 0 {
		return map[string]any{}, nil
	}
	return object, nil
}

func lowerStaticInlineList(items []expressionSyntax) ([]any, error) {
	list := make([]any, 0, len(items))
	for i := range items {
		value, err := lowerStaticExpression(items[i])
		if err != nil {
			return nil, err
		}
		list = append(list, value)
	}
	if len(list) == 0 {
		return nil, nil
	}
	return list, nil
}

func lowerStringCall(call callExpressionSyntax) (theater.BindingSpec, error) {
	parts := make([]theater.BindingSpec, 0, len(call.Args))
	for i := range call.Args {
		if call.Args[i].Name != "" {
			return theater.BindingSpec{}, &lowerError{
				span:    call.Args[i].Span,
				message: "string(...) accepts positional arguments only",
			}
		}
		part, err := lowerBindingExpression(call.Args[i].Value)
		if err != nil {
			return theater.BindingSpec{}, err
		}
		parts = append(parts, part)
	}
	return theater.BindingSpec{
		Kind:  theater.BindingKindString,
		Parts: parts,
	}, nil
}

func lowerGenerateCall(call callExpressionSyntax) (theater.BindingSpec, error) {
	args, err := lowerNamedBindingArgs(call.Args, "generate call")
	if err != nil {
		return theater.BindingSpec{}, err
	}
	generatorRef := strings.TrimPrefix(call.Name, "generate.")
	if generatorRef == "" {
		return theater.BindingSpec{}, &lowerError{
			span:    call.Span,
			message: "generate call must select a generator ref",
		}
	}
	return theater.BindingSpec{
		Kind:      theater.BindingKindGenerate,
		Generator: generatorRef,
		Args:      args,
	}, nil
}

func lowerInterpolatedString(value string, span sourceSpan) (parts []theater.BindingSpec, ok bool, err error) {
	if !strings.Contains(value, "${") {
		return nil, false, nil
	}

	parts = make([]theater.BindingSpec, 0, 4)
	for value != "" {
		start := strings.Index(value, "${")
		if start == -1 {
			parts = append(parts, theater.BindingSpec{
				Kind:  theater.BindingKindLiteral,
				Value: value,
			})
			break
		}
		if start > 0 {
			parts = append(parts, theater.BindingSpec{
				Kind:  theater.BindingKindLiteral,
				Value: value[:start],
			})
		}
		value = value[start+2:]
		end := strings.IndexByte(value, '}')
		if end == -1 {
			return nil, false, &lowerError{
				span:    span,
				message: "string interpolation is unterminated",
			}
		}
		name := value[:end]
		if !isInterpolationIdentifier(name) {
			return nil, false, &lowerError{
				span:    span,
				message: fmt.Sprintf("string interpolation ref %q is invalid", name),
			}
		}
		parts = append(parts, theater.BindingSpec{
			Kind: theater.BindingKindRef,
			Ref: &theater.RefSpec{
				Name: name,
			},
		})
		value = value[end+1:]
	}

	return parts, true, nil
}

func isInterpolationIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
				return false
			}
			continue
		}
		if (r < 'A' || r > 'Z') &&
			(r < 'a' || r > 'z') &&
			(r < '0' || r > '9') &&
			r != '_' &&
			r != '-' {
			return false
		}
	}
	return true
}

func decodeStringLiteral(literal literalExpressionSyntax) (string, error) {
	switch literal.Kind {
	case literalKindString:
		value, err := strconv.Unquote(literal.Text)
		if err != nil {
			return "", err
		}
		return value, nil
	case literalKindRawString:
		return strings.TrimSuffix(strings.TrimPrefix(literal.Text, `r"`), `"`), nil
	case literalKindMultilineString:
		value := strings.TrimPrefix(literal.Text, `"""`)
		value = strings.TrimSuffix(value, `"""`)
		return dedentMultiline(value), nil
	default:
		return "", &lowerError{
			span:    literal.Span,
			message: "literal is not string-like",
		}
	}
}

func dedentMultiline(value string) string {
	lines := strings.Split(value, "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	minIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := 0
		for indent < len(line) && line[indent] == ' ' {
			indent++
		}
		if minIndent == -1 || indent < minIndent {
			minIndent = indent
		}
	}
	if minIndent <= 0 {
		for i := range lines {
			if strings.TrimSpace(lines[i]) == "" {
				lines[i] = ""
			}
		}
		return strings.Join(lines, "\n")
	}
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ""
			continue
		}
		lines[i] = line[minIndent:]
	}
	return strings.Join(lines, "\n")
}

func lowerNumberLiteral(literal literalExpressionSyntax) (any, error) {
	if strings.Contains(literal.Text, ".") {
		return strconv.ParseFloat(literal.Text, 64)
	}

	if integer, err := strconv.ParseInt(literal.Text, 10, 64); err == nil {
		return int(integer), nil
	}

	return strconv.ParseFloat(literal.Text, 64)
}

func lowerStringValue(expr expressionSyntax) (string, error) {
	switch value := ungroupExpression(expr).(type) {
	case literalExpressionSyntax:
		return decodeStringLiteral(value)
	case symbolExpressionSyntax:
		return value.Name, nil
	default:
		return "", &lowerError{
			span:    expr.ExpressionSpan(),
			message: "value must be string-like",
		}
	}
}

func lowerIntValue(expr expressionSyntax) (int, error) {
	literal, ok := ungroupExpression(expr).(literalExpressionSyntax)
	if !ok || literal.Kind != literalKindNumber {
		return 0, &lowerError{
			span:    expr.ExpressionSpan(),
			message: "value must be integer literal",
		}
	}
	integer, err := strconv.Atoi(literal.Text)
	if err != nil {
		return 0, &lowerError{
			span:    literal.Span,
			message: "value must be integer literal",
		}
	}
	return integer, nil
}

func lowerBareIdentifierLike(expr expressionSyntax) (string, error) {
	switch value := ungroupExpression(expr).(type) {
	case symbolExpressionSyntax:
		return value.Name, nil
	case literalExpressionSyntax:
		return decodeStringLiteral(value)
	default:
		return "", &lowerError{
			span:    expr.ExpressionSpan(),
			message: "value must be bare identifier or string",
		}
	}
}

func expectSinglePositionalArg(call callExpressionSyntax, context string) (expressionSyntax, error) {
	if len(call.Args) != 1 || call.Args[0].Name != "" {
		return nil, &lowerError{
			span:    call.Span,
			message: context + " expects one positional argument",
		}
	}
	return call.Args[0].Value, nil
}

func ungroupExpression(expr expressionSyntax) expressionSyntax {
	for {
		group, ok := expr.(groupedExpressionSyntax)
		if !ok {
			return expr
		}
		expr = group.Inner
	}
}

func bindSourceSpan(span sourceSpan, file string) *theater.SourceRef {
	if span.Start.Line == 0 && file == "" {
		return nil
	}
	return &theater.SourceRef{
		File:   file,
		Line:   span.Start.Line,
		Column: span.Start.Column,
	}
}
