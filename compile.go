package theater

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/alex-poliushkin/theater/internal/runtimevalue"
)

const (
	visitNew = iota
	visitActive
	visitDone

	invalidSelectorDecodeCode = "invalid_selector_decode"
	invalidSelectorPathCode   = "invalid_selector_path"
)

type stagePlan struct {
	ID            string
	Path          string
	PlanOrdinal   int
	HTTP          *HTTPSpec
	State         *StateSpec
	Scenarios     []scenarioPlan
	ScenarioCalls []scenarioCallPlan
	SourceSpan    *SourceRef
}

type scenarioPlan struct {
	ID          string
	Path        string
	PlanOrdinal int
	Inputs      map[string]ValueContract
	Acts        []actPlan
	SourceSpan  *SourceRef
}

type scenarioCallPlan struct {
	ID           string
	Path         string
	PlanOrdinal  int
	ScenarioID   string
	Bindings     map[string]bindingPlan
	Exports      []exportPlan
	Dependencies []scenarioDependencyPlan
	SourceSpan   *SourceRef
}

type scenarioDependencyPlan struct {
	Path   string
	CallID string
	When   TriggerPredicate
}

type actPlan struct {
	ID           string
	Path         string
	PlanOrdinal  int
	Eventually   *eventuallyPlan
	Properties   []propertyPlan
	Action       actionPlan
	CaptureAuth  *httpAuthCapturePlan
	Logs         []logPlan
	Expectations []expectationPlan
	Exports      []exportPlan
	Transitions  []transitionPlan
	SourceSpan   *SourceRef
}

type actionPlan struct {
	Use        string
	With       map[string]bindingPlan
	Repeatable bool
	SourceSpan *SourceRef
}

type httpAuthCapturePlan struct {
	Auth  string
	Slots map[string]HTTPCaptureSourceSpec
}

type eventuallyPlan struct {
	TimeoutText  string
	IntervalText string
	Timeout      time.Duration
	Interval     time.Duration
}

type inventoryPlan struct {
	Present bool
	Use     string
	With    map[string]bindingPlan
}

type selectorPlan struct {
	Decode  DecodeKind
	Path    JSONPointer
	Through []throughStepPlan
}

type refPlan struct {
	Name string
	selectorPlan
}

type subjectPlan struct {
	From  SubjectSourceKind
	Ref   string
	Field string
	selectorPlan
}

type assertPlan struct {
	Ref  string
	Args map[string]bindingPlan
}

type logPlan struct {
	ID          string
	Path        string
	Value       logValuePlan
	Message     string
	Fields      map[string]logValuePlan
	Format      LogFormat
	Capture     Capture
	Sensitivity Sensitivity
	Required    bool
	SourceSpan  *SourceRef
}

type logValuePlan struct {
	Path       string
	Field      string
	Ref        string
	Object     map[string]logValuePlan
	List       []logValuePlan
	SourceSpan *SourceRef
	selectorPlan
}

type bindingPlan struct {
	Path       string
	Kind       BindingKind
	Value      any
	Ref        *refPlan
	Object     map[string]bindingPlan
	List       []bindingPlan
	Parts      []bindingPlan
	Generator  string
	Args       map[string]bindingPlan
	SourceSpan *SourceRef
}

type throughStepPlan struct {
	Path   JSONPointer
	Pick   *pickStepPlan
	Regexp *regexpStepPlan
}

type pickStepPlan struct {
	At     JSONPointer
	Equals bindingPlan
	Where  []pickWhereClausePlan
}

type pickWhereClausePlan struct {
	Subject relativeSubjectPlan
	Assert  assertPlan
}

type relativeSubjectPlan struct {
	Decode DecodeKind
	Path   JSONPointer
}

type regexpStepPlan struct {
	Pattern string
	Group   int
}

type exportPlan struct {
	As    string
	Ref   *refPlan
	Field string
	selectorPlan
}

type expectationPlan struct {
	ID         string
	Subject    subjectPlan
	Assert     assertPlan
	Matcher    MatcherDescriptor
	SourceSpan *SourceRef
}

type propertyPlan struct {
	ID           string
	Path         string
	Dependencies []string
	Inventory    inventoryPlan
	Decorators   []decoratorPlan
}

type decoratorPlan struct {
	Use       string
	With      Values
	Contract  DecoratorContract
	Transform DecoratorFunc
}

type transitionPlan struct {
	On TransitionOutcome
	To string
}

func cloneValueContracts(specs map[string]ValueContract) map[string]ValueContract {
	if len(specs) == 0 {
		return nil
	}

	cloned := make(map[string]ValueContract, len(specs))
	for key, spec := range specs {
		cloned[key] = cloneValueContract(spec)
	}

	return cloned
}

func cloneValues(values Values) Values {
	if len(values) == 0 {
		return nil
	}

	cloned := make(Values, len(values))
	for key, value := range values {
		cloned[key] = runtimevalue.Clone(value)
	}

	return cloned
}

func cloneValueContract(spec ValueContract) ValueContract {
	return spec.Clone()
}

func hasPropertyDependencyCycle(properties []propertyPlan) bool {
	graph := make(map[string][]string, len(properties))
	for i := range properties {
		dependencies := append([]string(nil), properties[i].Dependencies...)
		sort.Strings(dependencies)
		graph[properties[i].ID] = dependencies
	}

	return hasDirectedCycle(graph)
}

func validateScenarioCallBindings(
	call scenarioCallPlan,
	inputs map[string]ValueContract,
	resolver GeneratorResolver,
	matchers MatcherResolver,
) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)

	for key := range call.Bindings {
		binding := call.Bindings[key]
		spec, ok := inputs[key]
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "unknown_scenario_input",
				Path:     bindingPath(call.Path, key),
				Severity: SeverityError,
				Summary:  fmt.Sprintf("scenario input %q is not declared", key),
			})
			continue
		}

		if err := validateBindingContractWithResolver(resolver, matchers, binding, spec); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "incompatible_scenario_input",
				Path:     bindingPath(call.Path, key),
				Severity: SeverityError,
				Summary:  fmt.Sprintf("scenario input %q %v", key, err),
			})
		}
	}

	for key, spec := range inputs {
		if !spec.Required {
			continue
		}

		if _, ok := call.Bindings[key]; ok {
			continue
		}

		diagnostics = append(diagnostics, Diagnostic{
			Code:     "missing_scenario_input",
			Path:     call.Path,
			Severity: SeverityError,
			Summary:  fmt.Sprintf("scenario input %q is required", key),
		})
	}

	return diagnostics
}

func validateValueContracts(path string, specs map[string]ValueContract) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	for key, spec := range specs {
		diagnostics = append(diagnostics, validateValueContract(bindingPath(path+"/input", key), key, spec)...)
	}

	return diagnostics
}

func validateValueContract(path, name string, spec ValueContract) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	if !spec.Valid() {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "invalid_value_contract_kind",
			Path:     path,
			Severity: SeverityError,
			Summary:  fmt.Sprintf("value contract %q must declare at least one valid kind", name),
		})
	}

	if spec.Sensitivity != "" && !spec.Sensitivity.Valid() {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "invalid_value_contract_sensitivity",
			Path:     path,
			Severity: SeverityError,
			Summary:  fmt.Sprintf("value contract %q sensitivity %q is invalid", name, spec.Sensitivity),
		})
	}

	if spec.Capture != "" && !spec.Capture.Valid() {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "invalid_value_contract_capture",
			Path:     path,
			Severity: SeverityError,
			Summary:  fmt.Sprintf("value contract %q capture %q is invalid", name, spec.Capture),
		})
	}

	for key, field := range spec.Fields {
		diagnostics = append(diagnostics, validateValueContract(bindingChildPath(path, key), key, field)...)
	}

	if spec.Elem != nil {
		diagnostics = append(diagnostics, validateValueContract(path+"[]", name, *spec.Elem)...)
	}

	return diagnostics
}

func joinChildPath(parentPath, kind, id string) string {
	return runtimePathCodec{}.JoinChild(parentPath, kind, id)
}

func duplicateIDDiagnostics[T any](
	items []T,
	id func(item T) string,
	path func(item T) string,
	code string,
	kind string,
) []Diagnostic {
	seen := make(map[string]struct{}, len(items))
	duplicates := make(map[string]string)

	for _, item := range items {
		itemID := id(item)
		if _, ok := seen[itemID]; ok {
			if _, recorded := duplicates[itemID]; !recorded {
				duplicates[itemID] = path(item)
			}

			continue
		}

		seen[itemID] = struct{}{}
	}

	diagnostics := make([]Diagnostic, 0, len(duplicates))
	for duplicateID, duplicatePath := range duplicates {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     code,
			Path:     duplicatePath,
			Severity: SeverityError,
			Summary:  fmt.Sprintf("%s id %q must be unique", kind, duplicateID),
		})
	}

	return diagnostics
}

func duplicateActIDDiagnostics(acts []actPlan) []Diagnostic {
	return duplicateIDDiagnostics(
		acts,
		func(act actPlan) string { return act.ID },
		func(act actPlan) string { return act.Path },
		"duplicate_act_id",
		"act",
	)
}

func duplicateScenarioIDDiagnostics(scenarios []scenarioPlan) []Diagnostic {
	return duplicateIDDiagnostics(
		scenarios,
		func(scenario scenarioPlan) string { return scenario.ID },
		func(scenario scenarioPlan) string { return scenario.Path },
		"duplicate_scenario_id",
		"scenario",
	)
}

func duplicateScenarioCallIDDiagnostics(calls []scenarioCallPlan) []Diagnostic {
	return duplicateIDDiagnostics(
		calls,
		func(call scenarioCallPlan) string { return call.ID },
		func(call scenarioCallPlan) string { return call.Path },
		"duplicate_scenario_call_id",
		"scenario call",
	)
}

func duplicateExpectationIDDiagnostics(act *actPlan) []Diagnostic {
	return duplicateIDDiagnostics(
		act.Expectations,
		func(expectation expectationPlan) string { return expectation.ID },
		func(expectation expectationPlan) string {
			return joinChildPath(act.Path, "expectation", expectation.ID)
		},
		"duplicate_expectation_id",
		"expectation",
	)
}

func duplicateLogIDDiagnostics(act *actPlan) []Diagnostic {
	return duplicateIDDiagnostics(
		act.Logs,
		func(log logPlan) string { return log.ID },
		func(log logPlan) string { return log.Path },
		"duplicate_log_id",
		"log",
	)
}

func bindingChildPath(parentPath, key string) string {
	if key == "" {
		return parentPath
	}

	return fmt.Sprintf("%s.%s", parentPath, key)
}

func bindingPath(parentPath, key string) string {
	if key == "" {
		return parentPath + "/binding"
	}

	return joinChildPath(parentPath, "binding", key)
}

func exportPath(parentPath, alias string) string {
	if alias == "" {
		return parentPath + "/export"
	}

	return joinChildPath(parentPath, "export", alias)
}

func logPath(parentPath, id string) string {
	if id == "" {
		return parentPath + "/log"
	}

	return joinChildPath(parentPath, "log", id)
}

func validateLogValue(value logValuePlan) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	configured := 0
	if value.Field != "" {
		configured++
		if err := validateFieldName(value.Field); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "invalid_log_value_field",
				Path:     value.Path,
				Severity: SeverityError,
				Summary:  err.Error(),
			})
		}
	}
	if value.Ref != "" {
		configured++
		if err := validateRefName(value.Ref); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "invalid_log_value_ref",
				Path:     value.Path,
				Severity: SeverityError,
				Summary:  err.Error(),
			})
		}
	}
	if len(value.Object) != 0 {
		configured++
		for key := range value.Object {
			diagnostics = append(diagnostics, validateLogValue(value.Object[key])...)
		}
	}
	if len(value.List) != 0 {
		configured++
		for i := range value.List {
			diagnostics = append(diagnostics, validateLogValue(value.List[i])...)
		}
	}

	if configured != 1 {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "invalid_log_value_source",
			Path:     value.Path,
			Severity: SeverityError,
			Summary:  "log value must declare exactly one of field, ref, object, or list",
		})
	}

	if len(value.Object) != 0 || len(value.List) != 0 {
		if value.Decode != "" || !value.selectorPlan.Path.IsZero() || len(value.Through) != 0 {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "invalid_log_value_selector",
				Path:     value.Path,
				Severity: SeverityError,
				Summary:  "log value selectors require field or ref",
			})
		}
		return diagnostics
	}

	for _, diagnostic := range validateSelector(value.Path, value.selectorPlan) {
		switch diagnostic.Code {
		case invalidSelectorDecodeCode:
			diagnostic.Code = "invalid_log_value_decode"
			diagnostic.Summary = fmt.Sprintf("log value decode %q is invalid", value.Decode)
		case invalidSelectorPathCode:
			diagnostic.Code = "invalid_log_value_path"
			diagnostic.Summary = "log value path is invalid: " + diagnostic.Summary
		default:
			diagnostic.Code = strings.Replace(diagnostic.Code, "invalid_selector_", "invalid_log_value_", 1)
			diagnostic.Summary = "log value " + diagnostic.Summary
		}
		diagnostics = append(diagnostics, diagnostic)
	}

	return diagnostics
}

func logValueConfigured(value logValuePlan) bool {
	return value.Field != "" ||
		value.Ref != "" ||
		len(value.Object) != 0 ||
		len(value.List) != 0 ||
		value.Decode != "" ||
		!value.selectorPlan.Path.IsZero() ||
		len(value.Through) != 0
}

func validateActExports(parentPath string, exports []exportPlan) []Diagnostic {
	diagnostics := validateExports(parentPath, exports)

	for _, export := range exports {
		path := exportPath(parentPath, exportAlias(export))
		if export.Ref != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "invalid_act_export_ref",
				Path:     path,
				Severity: SeverityError,
				Summary:  "act export must select an action output via field, not ref",
			})
		}

		if export.Field == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "missing_act_export_field",
				Path:     path,
				Severity: SeverityError,
				Summary:  "act export field is required",
			})
			continue
		}

		if err := validateFieldName(export.Field); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "invalid_act_export_field",
				Path:     path,
				Severity: SeverityError,
				Summary:  err.Error(),
			})
		}

		diagnostics = append(diagnostics, validateSelector(path, export.selectorPlan)...)
	}

	return diagnostics
}

func validateScenarioCallExports(parentPath string, exports []exportPlan) []Diagnostic {
	diagnostics := validateExports(parentPath, exports)

	for _, export := range exports {
		path := exportPath(parentPath, exportAlias(export))
		if export.Field != "" {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "invalid_scenario_call_export_field",
				Path:     path,
				Severity: SeverityError,
				Summary:  "scenario call export must select a scenario value via ref, not field",
			})
		}

		if export.Ref == nil || export.Ref.Name == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "missing_scenario_call_export_ref",
				Path:     path,
				Severity: SeverityError,
				Summary:  "scenario call export ref is required",
			})
			continue
		}

		if err := validateRefSpec(*export.Ref); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "invalid_scenario_call_export_ref",
				Path:     path,
				Severity: SeverityError,
				Summary:  err.Error(),
			})
		}
	}

	return diagnostics
}

func validateSelector(path string, selector selectorPlan) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)

	if !selector.Decode.Valid() {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     invalidSelectorDecodeCode,
			Path:     path,
			Severity: SeverityError,
			Summary:  fmt.Sprintf("selector decode %q is invalid", selector.Decode),
		})
	}

	if err := validateJSONPointer(selector.Path); err != nil {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     invalidSelectorPathCode,
			Path:     path,
			Severity: SeverityError,
			Summary:  err.Error(),
		})
	}

	for i := range selector.Through {
		diagnostics = append(diagnostics, validateThroughStep(fmt.Sprintf("%s/through[%d]", path, i), selector.Through[i])...)
	}

	return diagnostics
}

func validateRefSpec(ref refPlan) error {
	if err := validateRefName(ref.Name); err != nil {
		return err
	}

	if !ref.Decode.Valid() {
		return fmt.Errorf("ref %q decode %q is invalid", ref.Name, ref.Decode)
	}

	if err := validateJSONPointer(ref.Path); err != nil {
		return fmt.Errorf("ref %q path is invalid: %w", ref.Name, err)
	}

	for i := range ref.Through {
		if diagnostics := validateThroughStep(fmt.Sprintf("ref %q through[%d]", ref.Name, i), ref.Through[i]); len(diagnostics) != 0 {
			return fmt.Errorf("%s", diagnostics[0].Summary)
		}
	}

	return nil
}

func validateThroughStep(path string, step throughStepPlan) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	configured := 0
	if !step.Path.IsRoot() {
		configured++
		if err := validateJSONPointer(step.Path); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "invalid_selector_through_path",
				Path:     path,
				Severity: SeverityError,
				Summary:  err.Error(),
			})
		}
	}
	if step.Pick != nil {
		configured++
		diagnostics = append(diagnostics, validatePickStep(path, *step.Pick)...)
	}
	if step.Regexp != nil {
		configured++
		if step.Regexp.Pattern == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "invalid_selector_through_regexp",
				Path:     path,
				Severity: SeverityError,
				Summary:  "regexp.pattern is required",
			})
		} else if _, err := regexp.Compile(step.Regexp.Pattern); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "invalid_selector_through_regexp",
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("regexp pattern is invalid: %v", err),
			})
		}
		if step.Regexp.Group < 0 {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "invalid_selector_through_regexp",
				Path:     path,
				Severity: SeverityError,
				Summary:  "regexp.group must be zero or positive",
			})
		}
	}
	if configured != 1 {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "invalid_selector_through_step",
			Path:     path,
			Severity: SeverityError,
			Summary:  "through step must declare exactly one of path, pick, or regexp",
		})
	}

	return diagnostics
}

func validatePickStep(path string, step pickStepPlan) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	hasLegacyPredicate := !step.At.IsRoot() || step.Equals.Kind != ""
	hasWherePredicate := len(step.Where) != 0

	switch {
	case hasLegacyPredicate && hasWherePredicate:
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "invalid_selector_through_pick",
			Path:     path,
			Severity: SeverityError,
			Summary:  "pick cannot combine at/equals with where",
		})
	case hasWherePredicate:
		for i := range step.Where {
			diagnostics = append(diagnostics, validatePickWhereClause(fmt.Sprintf("%s/pick/where[%d]", path, i), step.Where[i])...)
		}
	default:
		if step.At.IsRoot() {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "invalid_selector_through_pick",
				Path:     path,
				Severity: SeverityError,
				Summary:  `pick.at is required`,
			})
		} else if err := validateJSONPointer(step.At); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "invalid_selector_through_pick",
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("pick.at %v", err),
			})
		}

		diagnostics = append(diagnostics, validateBinding(path+"/pick/equals", step.Equals)...)
	}

	return diagnostics
}

func validatePickWhereClause(path string, clause pickWhereClausePlan) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	if !clause.Subject.Decode.Valid() {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "invalid_selector_through_pick",
			Path:     path,
			Severity: SeverityError,
			Summary:  fmt.Sprintf("pick where subject decode %q is invalid", clause.Subject.Decode),
		})
	}
	if clause.Subject.Decode == "" && clause.Subject.Path.IsRoot() {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "invalid_selector_through_pick",
			Path:     path,
			Severity: SeverityError,
			Summary:  "pick where subject must declare decode or path",
		})
	}
	if err := validateJSONPointer(clause.Subject.Path); err != nil {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "invalid_selector_through_pick",
			Path:     path,
			Severity: SeverityError,
			Summary:  fmt.Sprintf("pick where subject path is invalid: %v", err),
		})
	}
	if clause.Assert.Ref == "" {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "invalid_selector_through_pick",
			Path:     path,
			Severity: SeverityError,
			Summary:  "pick where clause assert ref is required",
		})
	}
	diagnostics = append(diagnostics, validateBindings(path+"/assert", clause.Assert.Args)...)
	return diagnostics
}

func validateFieldName(field string) error {
	if field == "" {
		return nil
	}

	if strings.Contains(field, ".") {
		root := strings.Split(field, ".")[0]
		remainder := strings.TrimPrefix(field, root)
		path := strings.ReplaceAll(remainder, ".", "/")
		return fmt.Errorf(
			"field %q is invalid: field selects a top-level value; use field: %s and path: %s",
			field,
			root,
			path,
		)
	}

	if strings.ContainsAny(field, "[]/#") || strings.Contains(field, "/") {
		return fmt.Errorf("field %q is invalid: traversal belongs in path, not field", field)
	}

	return nil
}

func validateBinding(path string, binding bindingPlan) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)

	if !binding.Kind.Valid() {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "invalid_binding_kind",
			Path:     path,
			Severity: SeverityError,
			Summary:  fmt.Sprintf("binding kind %q is invalid", binding.Kind),
		})

		return diagnostics
	}

	switch binding.Kind {
	case BindingKindRef:
		if binding.Ref == nil || binding.Ref.Name == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "missing_binding_ref",
				Path:     path,
				Severity: SeverityError,
				Summary:  "binding ref is required",
			})
			return diagnostics
		}

		if err := validateRefSpec(*binding.Ref); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "invalid_binding_ref",
				Path:     path,
				Severity: SeverityError,
				Summary:  err.Error(),
			})
		}
	case BindingKindObject:
		for key := range binding.Object {
			diagnostics = append(diagnostics, validateBinding(bindingChildPath(path, key), binding.Object[key])...)
		}
	case BindingKindList:
		for i := range binding.List {
			diagnostics = append(diagnostics, validateBinding(fmt.Sprintf("%s[%d]", path, i), binding.List[i])...)
		}
	case BindingKindString:
		if len(binding.Parts) == 0 {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "invalid_binding_string_parts",
				Path:     path,
				Severity: SeverityError,
				Summary:  "string binding parts are required",
			})
			return diagnostics
		}

		for i := range binding.Parts {
			diagnostics = append(diagnostics, validateBinding(fmt.Sprintf("%s.parts[%d]", path, i), binding.Parts[i])...)
		}
	case BindingKindGenerate:
		if binding.Generator == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "missing_binding_generator",
				Path:     path,
				Severity: SeverityError,
				Summary:  "generator binding generator is required",
			})
			return diagnostics
		}

		for key := range binding.Args {
			diagnostics = append(diagnostics, validateBinding(bindingChildPath(path, key), binding.Args[key])...)
		}
	}

	return diagnostics
}

func validateBindings(parentPath string, bindings map[string]bindingPlan) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	for key := range bindings {
		diagnostics = append(diagnostics, validateBinding(bindingPath(parentPath, key), bindings[key])...)
	}

	return diagnostics
}

func validateScenarioCallBindingRefs(
	calls []scenarioCallPlan,
	callExports map[string]map[string]struct{},
) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	for i := range calls {
		call := calls[i]
		allowedRoots := dependencyExportAliases(call.Dependencies, callExports)
		for key := range call.Bindings {
			diagnostics = append(diagnostics, validateBindingRefResolution(bindingPath(call.Path, key), call.Bindings[key], allowedRoots)...)
		}
	}

	return diagnostics
}

func validateScenarioCallExportAliases(calls []scenarioCallPlan) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	owners := make(map[string]string)

	for _, call := range calls {
		for _, export := range call.Exports {
			alias := exportAlias(export)
			if alias == "" {
				continue
			}

			path := exportPath(call.Path, alias)
			ownerCallID, ok := owners[alias]
			if ok {
				if ownerCallID != call.ID {
					diagnostics = append(diagnostics, Diagnostic{
						Code:     "duplicate_stage_export_name",
						Path:     path,
						Severity: SeverityError,
						Summary:  fmt.Sprintf("stage export name %q must be unique across scenario calls", alias),
					})
				}

				continue
			}

			owners[alias] = call.ID
		}
	}

	return diagnostics
}

func dependencyExportAliases(
	dependencies []scenarioDependencyPlan,
	callExports map[string]map[string]struct{},
) map[string]struct{} {
	aliases := make(map[string]struct{})
	for _, dependency := range dependencies {
		exported, ok := callExports[dependency.CallID]
		if !ok {
			continue
		}

		for alias := range exported {
			aliases[alias] = struct{}{}
		}
	}

	return aliases
}

func exportAliasSet(exports []exportPlan) map[string]struct{} {
	aliases := make(map[string]struct{}, len(exports))
	for _, export := range exports {
		alias := exportAlias(export)
		if alias == "" {
			continue
		}

		aliases[alias] = struct{}{}
	}

	return aliases
}

func validateExports(parentPath string, exports []exportPlan) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	seen := make(map[string]struct{}, len(exports))

	for i := range exports {
		export := exports[i]
		alias := exportAlias(export)
		path := exportPath(parentPath, alias)

		if alias == "" {
			continue
		}

		if _, ok := seen[alias]; ok {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "duplicate_export_name",
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("export name %q must be unique", alias),
			})
			continue
		}

		seen[alias] = struct{}{}
	}

	return diagnostics
}

func hasActTransitionCycle(scenario scenarioPlan) bool {
	graph := make(map[string][]string, len(scenario.Acts))
	actIDs := make(map[string]struct{}, len(scenario.Acts))

	for i := range scenario.Acts {
		act := &scenario.Acts[i]
		actIDs[act.ID] = struct{}{}
	}

	for i := range scenario.Acts {
		act := &scenario.Acts[i]
		targets := make([]string, 0, len(act.Transitions))
		for _, transition := range act.Transitions {
			if transition.To == "" {
				continue
			}

			if _, ok := actIDs[transition.To]; !ok {
				continue
			}

			targets = append(targets, transition.To)
		}

		sort.Strings(targets)
		graph[act.ID] = targets
	}

	return hasDirectedCycle(graph)
}

func hasDirectedCycle(graph map[string][]string) bool {
	state := make(map[string]int, len(graph))
	nodes := make([]string, 0, len(graph))
	for node := range graph {
		nodes = append(nodes, node)
	}

	sort.Strings(nodes)
	for _, node := range nodes {
		if visitHasCycle(node, graph, state) {
			return true
		}
	}

	return false
}

func hasScenarioDependencyCycle(stage *stagePlan) bool {
	graph := make(map[string][]string, len(stage.ScenarioCalls))
	callIDs := make(map[string]struct{}, len(stage.ScenarioCalls))

	for _, call := range stage.ScenarioCalls {
		callIDs[call.ID] = struct{}{}
	}

	for _, call := range stage.ScenarioCalls {
		dependencies := make([]string, 0, len(call.Dependencies))
		for _, dependency := range call.Dependencies {
			if _, ok := callIDs[dependency.CallID]; !ok {
				continue
			}

			dependencies = append(dependencies, dependency.CallID)
		}

		sort.Strings(dependencies)
		graph[call.ID] = dependencies
	}

	return hasDirectedCycle(graph)
}

func visitHasCycle(node string, graph map[string][]string, state map[string]int) bool {
	switch state[node] {
	case visitActive:
		return true
	case visitDone:
		return false
	}

	state[node] = visitActive
	for _, next := range graph[node] {
		if visitHasCycle(next, graph, state) {
			return true
		}
	}

	state[node] = visitDone
	return false
}
