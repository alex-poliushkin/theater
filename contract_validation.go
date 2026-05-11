package theater

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/alex-poliushkin/theater/internal/runtimevalue"
)

const (
	actExportPathCode               = "incompatible_act_export_path"
	actExportDecodeCode             = "incompatible_act_export_decode"
	actExportTransformCode          = "incompatible_act_export_transform"
	expectationSubjectPathCode      = "incompatible_expectation_subject_path"
	expectationSubjectDecodeCode    = "incompatible_expectation_subject_decode"
	expectationSubjectTransformCode = "incompatible_expectation_subject_transform"
)

type contractValidator struct {
	catalog  CatalogResolver
	matchers MatcherResolver
}

func validateActionBindings(
	path string,
	bindings map[string]bindingPlan,
	inputs map[string]ValueContract,
	resolver GeneratorResolver,
	matchers MatcherResolver,
	decorators DecoratorResolver,
) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	for key := range bindings {
		binding := bindings[key]
		spec, ok := inputs[key]
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "unknown_action_arg",
				Path:     bindingPath(path, key),
				Severity: SeverityError,
				Summary:  fmt.Sprintf("action input %q is not declared", key),
			})
			continue
		}

		if err := validateBindingContractWithResolver(resolver, matchers, decorators, binding, spec); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "incompatible_action_arg",
				Path:     bindingPath(path, key),
				Severity: SeverityError,
				Summary:  fmt.Sprintf("action input %q %v", key, err),
			})
		}
	}

	for key, spec := range inputs {
		if !spec.Required {
			continue
		}

		if _, ok := bindings[key]; ok {
			continue
		}

		diagnostics = append(diagnostics, Diagnostic{
			Code:     "missing_action_arg",
			Path:     path,
			Severity: SeverityError,
			Summary:  fmt.Sprintf("action input %q is required", key),
		})
	}

	return diagnostics
}

func validateActionOutputs(
	act *actPlan,
	outputs map[string]ValueContract,
	resolver GeneratorResolver,
	matchers MatcherResolver,
	decorators DecoratorResolver,
) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)

	for i := range act.Expectations {
		expectation := &act.Expectations[i]
		if expectation.Subject.From == SubjectFromProperty || expectation.Subject.Field == "" {
			continue
		}

		path := joinChildPath(act.Path, "expectation", expectation.ID)
		output, ok := outputs[expectation.Subject.Field]
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "unknown_expectation_subject_field",
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("expectation subject field %q is not declared by action %q", expectation.Subject.Field, act.Action.Use),
			})
			continue
		}

		if err := validateSelectorContract(expectation.Subject.selectorPlan, output, decorators); err != nil {
			code := expectationSubjectPathCode
			var typed selectorContractError
			if errors.As(err, &typed) {
				switch typed.code {
				case selectorContractCodeDecode:
					code = expectationSubjectDecodeCode
				case selectorContractCodeRegexp, selectorContractCodeTransform:
					code = expectationSubjectTransformCode
				}
			}

			diagnostics = append(diagnostics, Diagnostic{
				Code:     code,
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("expectation subject field %q %v", expectation.Subject.Field, err),
			})
		}

		if _, _, err := validateSelectorBindingContracts(expectation.Subject.selectorPlan, resolver, matchers, decorators); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     expectationSubjectTransformCode,
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("expectation subject field %q %v", expectation.Subject.Field, err),
			})
		}
	}

	for i := range act.Exports {
		export := act.Exports[i]
		if export.Field == "" {
			continue
		}

		path := exportPath(act.Path, exportAlias(export))
		output, ok := outputs[export.Field]
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "unknown_action_output_ref",
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("export field %q is not declared by action %q", export.Field, act.Action.Use),
			})
			continue
		}

		if err := validateSelectorContract(export.selectorPlan, output, decorators); err != nil {
			code := actExportPathCode
			var typed selectorContractError
			if errors.As(err, &typed) {
				switch typed.code {
				case selectorContractCodeDecode:
					code = actExportDecodeCode
				case selectorContractCodeRegexp, selectorContractCodeTransform:
					code = actExportTransformCode
				}
			}

			diagnostics = append(diagnostics, Diagnostic{
				Code:     code,
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("export field %q %v", export.Field, err),
			})
		}

		if _, _, err := validateSelectorBindingContracts(export.selectorPlan, resolver, matchers, decorators); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     actExportTransformCode,
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("export field %q %v", export.Field, err),
			})
		}
	}

	return diagnostics
}

func validateActRefExports(
	act *actPlan,
	actEntryContracts map[string]ValueContract,
	properties map[string]ValueContract,
	resolver GeneratorResolver,
	matchers MatcherResolver,
	decorators DecoratorResolver,
) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	refContracts := mergeContractMaps(properties, actEntryContracts)
	for i := range act.Exports {
		export := act.Exports[i]
		if export.Ref == nil {
			continue
		}

		path := exportPath(act.Path, exportAlias(export))
		contract, hasContract := refContracts[export.Ref.Name]
		if hasContract {
			if err := validateActRefExportSelectorContract(export, contract, decorators); err != nil {
				diagnostics = append(diagnostics, Diagnostic{
					Code:     actExportSelectorCode(err),
					Path:     path,
					Severity: SeverityError,
					Summary:  fmt.Sprintf("export ref %q %v", export.Ref.Name, err),
				})
			}
		}

		if _, _, err := validateSelectorBindingContracts(export.Ref.selectorPlan, resolver, matchers, decorators); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     actExportTransformCode,
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("export ref %q %v", export.Ref.Name, err),
			})
		}
		if _, _, err := validateSelectorBindingContracts(export.selectorPlan, resolver, matchers, decorators); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     actExportTransformCode,
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("export ref %q %v", export.Ref.Name, err),
			})
		}
	}

	return diagnostics
}

func validateActRefExportSelectorContract(
	export exportPlan,
	contract ValueContract,
	decorators DecoratorResolver,
) error {
	selected, known, err := selectedSelectorContract(export.Ref.selectorPlan, contract, decorators)
	if err != nil || !known {
		return err
	}

	return validateSelectorContract(export.selectorPlan, selected, decorators)
}

func actExportSelectorCode(err error) string {
	code := actExportPathCode
	var typed selectorContractError
	if errors.As(err, &typed) {
		switch typed.code {
		case selectorContractCodeDecode:
			code = actExportDecodeCode
		case selectorContractCodeRegexp, selectorContractCodeTransform:
			code = actExportTransformCode
		}
	}
	return code
}

func validateLogActionOutputs(
	act *actPlan,
	outputs map[string]ValueContract,
	resolver GeneratorResolver,
	matchers MatcherResolver,
	decorators DecoratorResolver,
) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	for i := range act.Logs {
		log := act.Logs[i]
		diagnostics = append(diagnostics, validateLogValueActionOutput(log.Value, outputs, resolver, matchers, decorators)...)
		for key := range log.Fields {
			diagnostics = append(diagnostics, validateLogValueActionOutput(log.Fields[key], outputs, resolver, matchers, decorators)...)
		}
	}

	return diagnostics
}

func validateLogValueActionOutput(
	value logValuePlan,
	outputs map[string]ValueContract,
	resolver GeneratorResolver,
	matchers MatcherResolver,
	decorators DecoratorResolver,
) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	if value.Field != "" {
		diagnostics = append(diagnostics, validateLogValueActionField(value, outputs, decorators)...)
	}

	if value.Field != "" || value.Ref != "" {
		if _, _, err := validateSelectorBindingContracts(value.selectorPlan, resolver, matchers, decorators); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "incompatible_log_value_transform",
				Path:     value.Path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("log value %v", err),
			})
		}
	}

	for key := range value.Object {
		diagnostics = append(diagnostics, validateLogValueActionOutput(value.Object[key], outputs, resolver, matchers, decorators)...)
	}
	for i := range value.List {
		diagnostics = append(diagnostics, validateLogValueActionOutput(value.List[i], outputs, resolver, matchers, decorators)...)
	}

	return diagnostics
}

func validateLogValueActionField(value logValuePlan, outputs map[string]ValueContract, decorators DecoratorResolver) []Diagnostic {
	output, ok := outputs[value.Field]
	if !ok {
		return []Diagnostic{{
			Code:     "unknown_log_field",
			Path:     value.Path,
			Severity: SeverityError,
			Summary:  fmt.Sprintf("log field %q is not declared by action", value.Field),
		}}
	}

	if err := validateSelectorContract(value.selectorPlan, output, decorators); err != nil {
		code := "incompatible_log_value_path"
		var typed selectorContractError
		if errors.As(err, &typed) {
			switch typed.code {
			case selectorContractCodeDecode:
				code = "incompatible_log_value_decode"
			case selectorContractCodeRegexp, selectorContractCodeTransform:
				code = "incompatible_log_value_transform"
			}
		}

		return []Diagnostic{{
			Code:     code,
			Path:     value.Path,
			Severity: SeverityError,
			Summary:  fmt.Sprintf("log field %q %v", value.Field, err),
		}}
	}

	return nil
}

func validatePropertyExpectationSubjects(
	act *actPlan,
	properties map[string]ValueContract,
	resolver GeneratorResolver,
	matchers MatcherResolver,
	decorators DecoratorResolver,
) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)

	for i := range act.Expectations {
		expectation := &act.Expectations[i]
		if expectation.Subject.From != SubjectFromProperty || expectation.Subject.Ref == "" {
			continue
		}

		contract, ok := properties[expectation.Subject.Ref]
		if !ok {
			continue
		}

		path := joinChildPath(act.Path, "expectation", expectation.ID)
		if err := validateSelectorContract(expectation.Subject.selectorPlan, contract, decorators); err != nil {
			code := expectationSubjectPathCode
			var typed selectorContractError
			if errors.As(err, &typed) {
				switch typed.code {
				case selectorContractCodeDecode:
					code = expectationSubjectDecodeCode
				case selectorContractCodeRegexp, selectorContractCodeTransform:
					code = expectationSubjectTransformCode
				}
			}

			diagnostics = append(diagnostics, Diagnostic{
				Code:     code,
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("expectation subject ref %q %v", expectation.Subject.Ref, err),
			})
		}

		if _, _, err := validateSelectorBindingContracts(expectation.Subject.selectorPlan, resolver, matchers, decorators); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     expectationSubjectTransformCode,
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("expectation subject ref %q %v", expectation.Subject.Ref, err),
			})
		}
	}

	return diagnostics
}

func bindingsStatic(bindings map[string]bindingPlan) bool {
	for key := range bindings {
		if !bindingStatic(bindings[key]) {
			return false
		}
	}

	return true
}

func bindingStatic(binding bindingPlan) bool {
	switch binding.Kind {
	case BindingKindLiteral:
		return true
	case BindingKindRef:
		return false
	case BindingKindObject:
		for key := range binding.Object {
			if !bindingStatic(binding.Object[key]) {
				return false
			}
		}

		return true
	case BindingKindList:
		for i := range binding.List {
			if !bindingStatic(binding.List[i]) {
				return false
			}
		}

		return true
	case BindingKindString:
		for i := range binding.Parts {
			if !bindingStatic(binding.Parts[i]) {
				return false
			}
		}

		return true
	case BindingKindGenerate:
		return false
	default:
		return false
	}
}

func validateResolvedArgs(contract ActionContract, args Args) error {
	for key, value := range args {
		spec, ok := contract.Inputs[key]
		if !ok {
			return fmt.Errorf("action input %q is not declared", key)
		}

		if err := validateResolvedContract(key, spec, value); err != nil {
			return err
		}
	}

	for key, spec := range contract.Inputs {
		if !spec.Required {
			continue
		}

		if _, ok := args[key]; ok {
			continue
		}

		return fmt.Errorf("action input %q is required", key)
	}

	return nil
}

func validateResolvedGeneratorArgs(contract GeneratorContract, args Args) error {
	for key, value := range args {
		found := false
		for i := range contract.Args {
			spec := contract.Args[i]
			if spec.Name != key {
				continue
			}

			if err := validateResolvedContract(key, spec.Accepts, value); err != nil {
				return err
			}

			found = true
			break
		}

		if !found {
			return fmt.Errorf("generator arg %q is not declared", key)
		}
	}

	for i := range contract.Args {
		spec := contract.Args[i]
		if !spec.Required {
			continue
		}

		if _, ok := args[spec.Name]; ok {
			continue
		}

		return fmt.Errorf("generator arg %q is required", spec.Name)
	}

	return nil
}

func validateResolvedOutputs(contract ActionContract, outputs Outputs) error {
	for key, value := range outputs {
		spec, ok := contract.Outputs[key]
		if !ok {
			return fmt.Errorf("action output %q is not declared", key)
		}

		if err := validateResolvedContract(key, spec, value); err != nil {
			return err
		}
	}

	for key, spec := range contract.Outputs {
		value, ok := outputs[key]
		if !ok {
			if spec.Required {
				return fmt.Errorf("action output %q is required", key)
			}

			continue
		}

		if err := validateResolvedContract(key, spec, value); err != nil {
			return err
		}
	}

	return nil
}

func validateBindingContractWithResolver(
	resolver GeneratorResolver,
	matchers MatcherResolver,
	decorators DecoratorResolver,
	binding bindingPlan,
	spec ValueContract,
) error {
	switch binding.Kind {
	case BindingKindLiteral:
		return validateResolvedContract("literal", spec, binding.Value)
	case BindingKindRef:
		if binding.Ref == nil {
			return nil
		}

		selected, known, err := validateSelectorBindingContracts(binding.Ref.selectorPlan, resolver, matchers, decorators)
		if err != nil {
			return err
		}
		if !known || spec.Supports(ValueKindAny) {
			return nil
		}
		if err := contractCompatibilityError(selected, spec); err != nil {
			return fmt.Errorf("selector produces %s incompatible with %s: %w", contractKindString(selected), contractKindString(spec), err)
		}
		return nil
	case BindingKindObject:
		return validateObjectBindingContract(resolver, matchers, decorators, binding, spec)
	case BindingKindList:
		return validateListBindingContract(resolver, matchers, decorators, binding, spec)
	case BindingKindString:
		return validateStringBindingContract(resolver, matchers, decorators, binding, spec)
	case BindingKindGenerate:
		return validateGenerateBindingContract(resolver, matchers, decorators, binding, spec)
	default:
		return nil
	}
}

func validateObjectBindingContract(
	resolver GeneratorResolver,
	matchers MatcherResolver,
	decorators DecoratorResolver,
	binding bindingPlan,
	spec ValueContract,
) error {
	if !spec.Supports(ValueKindObject) {
		return fmt.Errorf("expects %s, got object", contractKindString(spec))
	}

	for key := range binding.Object {
		child := binding.Object[key]
		fieldSpec, ok := objectMemberContract(spec, key)
		if !ok {
			if objectAllowsUnconstrainedMembers(spec) {
				continue
			}

			return fmt.Errorf("field %q is not declared", key)
		}

		if err := validateBindingContractWithResolver(resolver, matchers, decorators, child, fieldSpec); err != nil {
			return fmt.Errorf("field %q: %w", key, err)
		}
	}

	for key, fieldSpec := range spec.Fields {
		if !fieldSpec.Required {
			continue
		}
		if _, ok := binding.Object[key]; ok {
			continue
		}

		return fmt.Errorf("field %q is required", key)
	}

	return nil
}

func validateListBindingContract(
	resolver GeneratorResolver,
	matchers MatcherResolver,
	decorators DecoratorResolver,
	binding bindingPlan,
	spec ValueContract,
) error {
	if !spec.Supports(ValueKindList) {
		return fmt.Errorf("expects %s, got list", contractKindString(spec))
	}
	if spec.Elem == nil {
		return nil
	}

	for i := range binding.List {
		if err := validateBindingContractWithResolver(resolver, matchers, decorators, binding.List[i], *spec.Elem); err != nil {
			return fmt.Errorf("item %d: %w", i, err)
		}
	}

	return nil
}

func validateStringBindingContract(
	resolver GeneratorResolver,
	matchers MatcherResolver,
	decorators DecoratorResolver,
	binding bindingPlan,
	spec ValueContract,
) error {
	if !spec.Supports(ValueKindString) {
		return fmt.Errorf("expects %s, got string", contractKindString(spec))
	}
	if len(binding.Parts) == 0 {
		return errors.New("string parts are required")
	}

	stringPartContract := ValueContract{
		Kinds: NewValueKindSet(ValueKindString, ValueKindNumber, ValueKindBool),
	}
	for i := range binding.Parts {
		if err := validateBindingContractWithResolver(resolver, matchers, decorators, binding.Parts[i], stringPartContract); err != nil {
			return fmt.Errorf("part %d: %w", i, err)
		}
	}

	return nil
}

func validateGenerateBindingContract(
	resolver GeneratorResolver,
	matchers MatcherResolver,
	decorators DecoratorResolver,
	binding bindingPlan,
	spec ValueContract,
) error {
	if dependencyMissing(resolver) {
		return nil
	}

	def, err := resolver.ResolveGenerator(binding.Generator)
	if err != nil {
		return err
	}

	if err := validateGeneratorBindingArgs(resolver, matchers, decorators, binding, def.Contract); err != nil {
		return err
	}

	if err := validateStaticGenerateBindingArgs(def, binding.Args); err != nil {
		return err
	}

	if spec.Supports(ValueKindAny) {
		return nil
	}

	producedKinds := def.Contract.Produces.KindsSet()
	if len(producedKinds) == 0 {
		return errors.New("generator contract produces is invalid")
	}
	for kind := range producedKinds {
		if spec.Supports(kind) {
			continue
		}

		return fmt.Errorf("expects %s, got %s", contractKindString(spec), contractKindString(def.Contract.Produces))
	}

	return nil
}

func validateStaticGenerateBindingArgs(def GeneratorDef, args map[string]bindingPlan) error {
	if !bindingsStatic(args) {
		return nil
	}

	resolved, err := resolveStaticBindings(args)
	if err != nil {
		return err
	}

	if err := validateResolvedGeneratorArgs(def.Contract, Args(resolved)); err != nil {
		return err
	}

	if def.Validate != nil {
		return def.Validate(cloneValues(resolved))
	}

	return nil
}

func resolveStaticBindings(bindings map[string]bindingPlan) (Values, error) {
	if len(bindings) == 0 {
		return Values{}, nil
	}

	resolved := make(Values, len(bindings))
	for key := range bindings {
		value, err := resolveStaticBinding(bindings[key])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}

		resolved[key] = value
	}

	return resolved, nil
}

func resolveStaticBinding(binding bindingPlan) (any, error) {
	switch binding.Kind {
	case BindingKindLiteral:
		return runtimevalue.Clone(binding.Value), nil
	case BindingKindObject:
		object := make(map[string]any, len(binding.Object))
		for key := range binding.Object {
			value, err := resolveStaticBinding(binding.Object[key])
			if err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}

			object[key] = value
		}

		return object, nil
	case BindingKindList:
		list := make([]any, 0, len(binding.List))
		for i := range binding.List {
			value, err := resolveStaticBinding(binding.List[i])
			if err != nil {
				return nil, fmt.Errorf("%d: %w", i, err)
			}

			list = append(list, value)
		}

		return list, nil
	case BindingKindString:
		var builder strings.Builder
		for i := range binding.Parts {
			value, err := resolveStaticBinding(binding.Parts[i])
			if err != nil {
				return nil, fmt.Errorf("part %d: %w", i, err)
			}

			text, err := stringifyBindingPart(value, fmt.Sprintf("string part %d", i))
			if err != nil {
				return nil, err
			}
			builder.WriteString(text)
		}

		return builder.String(), nil
	default:
		return nil, fmt.Errorf("binding kind %q is not static", binding.Kind)
	}
}

func validateSelectorBindingContracts(
	selector selectorPlan,
	resolver GeneratorResolver,
	matchers MatcherResolver,
	decorators DecoratorResolver,
) (selected ValueContract, known bool, err error) {
	var current ValueContract
	known = false

	for i := range selector.Through {
		step := selector.Through[i]
		switch {
		case step.Transform != nil:
			def, transformKnown, err := resolveSelectorDecorator(*step.Transform, decorators)
			if err != nil {
				return ValueContract{}, false, fmt.Errorf("through step %d transform %w", i, err)
			}
			if !transformKnown {
				current = ValueContract{}
				known = false
				continue
			}
			if known {
				if err := contractCompatibilityError(current, def.Contract.Accepts); err != nil {
					return ValueContract{}, false, fmt.Errorf(
						"through step %d transform input is incompatible with contract %s: %w",
						i,
						contractKindString(def.Contract.Accepts),
						err,
					)
				}
			}
			current = def.Contract.Produces.Clone()
			known = true
		case known:
			selected, selectedKnown, err := selectedThroughContract(step, current, decorators)
			if err != nil {
				return ValueContract{}, false, fmt.Errorf("through step %d %w", i, err)
			}
			current = selected
			known = selectedKnown
		}

		if step.Pick == nil {
			continue
		}

		if len(step.Pick.Where) != 0 {
			for j := range step.Pick.Where {
				if err := validatePickWhereAssert(
					resolver,
					matchers,
					decorators,
					step.Pick.Where[j].Assert,
				); err != nil {
					return ValueContract{}, false, fmt.Errorf("through step %d pick where[%d] assert %w", i, j, err)
				}
			}
			continue
		}

		if err := validateBindingContractWithResolver(
			resolver,
			matchers,
			decorators,
			step.Pick.Equals,
			ValueContract{Kind: ValueKindAny},
		); err != nil {
			return ValueContract{}, false, fmt.Errorf("through step %d pick equals %w", i, err)
		}
	}

	return current, known, nil
}

func validatePickWhereAssert(
	resolver GeneratorResolver,
	matchers MatcherResolver,
	decorators DecoratorResolver,
	assert assertPlan,
) error {
	if assert.Ref == "" || dependencyMissing(matchers) {
		return nil
	}

	descriptor, err := matchers.Resolve(assert.Ref)
	if err != nil {
		return err
	}

	if err := validatePickWhereAssertArgs(resolver, matchers, decorators, assert, descriptor); err != nil {
		return err
	}

	if !bindingsStatic(assert.Args) {
		return nil
	}

	args, err := resolveStaticBindings(assert.Args)
	if err != nil {
		return err
	}

	if _, err := descriptor.Compile(newMatcherCompileResolver(matchers), args); err != nil {
		return fmt.Errorf("%q is invalid: %w", assert.Ref, err)
	}

	return nil
}

func validatePickWhereAssertArgs(
	resolver GeneratorResolver,
	matchers MatcherResolver,
	decorators DecoratorResolver,
	assert assertPlan,
	descriptor MatcherDescriptor,
) error {
	argSpecs := make(map[string]MatcherArg, len(descriptor.Args))
	for i := range descriptor.Args {
		argSpecs[descriptor.Args[i].Name] = descriptor.Args[i]
	}

	for key := range assert.Args {
		arg, ok := argSpecs[key]
		if !ok {
			return fmt.Errorf("%q does not support arg %q", assert.Ref, key)
		}

		if err := validateBindingContractWithResolver(resolver, matchers, decorators, assert.Args[key], arg.Accepts); err != nil {
			return fmt.Errorf("%q arg %q %w", assert.Ref, key, err)
		}
	}

	for i := range descriptor.Args {
		arg := descriptor.Args[i]
		if !arg.Required {
			continue
		}
		if _, ok := assert.Args[arg.Name]; ok {
			continue
		}

		return fmt.Errorf("%q requires arg %q", assert.Ref, arg.Name)
	}

	return nil
}

func validateGeneratorBindingArgs(
	resolver GeneratorResolver,
	matchers MatcherResolver,
	decorators DecoratorResolver,
	binding bindingPlan,
	contract GeneratorContract,
) error {
	specs := make(map[string]ArgSpec, len(contract.Args))
	for i := range contract.Args {
		specs[contract.Args[i].Name] = contract.Args[i]
	}

	for key := range binding.Args {
		child := binding.Args[key]
		spec, ok := specs[key]
		if !ok {
			return fmt.Errorf("generator %q does not support arg %q", binding.Generator, key)
		}

		if err := validateBindingContractWithResolver(resolver, matchers, decorators, child, spec.Accepts); err != nil {
			return fmt.Errorf("generator %q arg %q %w", binding.Generator, key, err)
		}
	}

	for i := range contract.Args {
		spec := contract.Args[i]
		if !spec.Required {
			continue
		}

		if _, ok := binding.Args[spec.Name]; ok {
			continue
		}

		return fmt.Errorf("generator %q requires arg %q", binding.Generator, spec.Name)
	}

	return nil
}

func validateResolvedContract(field string, spec ValueContract, value any) error {
	if !spec.Valid() {
		return fmt.Errorf("%s expects valid contract", field)
	}

	if spec.Supports(ValueKindAny) {
		return nil
	}

	actualKind := resolvedValueKind(value)
	if actualKind == ValueKindNull {
		if spec.Supports(ValueKindNull) {
			return nil
		}

		return fmt.Errorf("%s expects %s, got nil", field, contractKindString(spec))
	}

	if !spec.Supports(actualKind) {
		return fmt.Errorf("%s expects %s, got %T", field, contractKindString(spec), value)
	}

	if err := validateResolvedScalarKind(field, actualKind, value); err != nil {
		return err
	}

	if actualKind == ValueKindObject || actualKind == ValueKindList {
		return validateResolvedCompositeValue(field, spec, value)
	}

	return nil
}

func validateResolvedScalarKind(field string, kind ValueKind, value any) error {
	wrapped := runtimevalue.Wrap(value)
	switch kind {
	case ValueKindBytes:
		if _, ok := wrapped.BytesOK(); !ok {
			return fmt.Errorf("%s expects bytes, got %T", field, value)
		}
	case ValueKindString:
		if _, ok := wrapped.StringOK(); !ok {
			return fmt.Errorf("%s expects string, got %T", field, value)
		}
	case ValueKindNumber:
		_, err := wrapped.Float64(field)
		if err != nil {
			return newCausalError(fmt.Sprintf("%s expects number, got %T", field, value), err)
		}

		return nil
	case ValueKindBool:
		if _, ok := wrapped.BoolOK(); !ok {
			return fmt.Errorf("%s expects bool, got %T", field, value)
		}
	}

	return nil
}

func validateResolvedCompositeValue(field string, spec ValueContract, value any) error {
	switch resolvedValueKind(value) {
	case ValueKindObject:
		object, ok := runtimevalue.Object(value)
		if !ok {
			return fmt.Errorf("%s expects object, got %T", field, value)
		}

		for key, member := range object {
			memberField := field + "." + key
			memberSpec, ok := objectMemberContract(spec, key)
			if !ok {
				if objectAllowsUnconstrainedMembers(spec) {
					if err := runtimevalue.ValidateCanonical(memberField, member); err != nil {
						return err
					}
					continue
				}

				return fmt.Errorf("%s field %q is not declared", field, key)
			}

			if err := validateResolvedContract(memberField, memberSpec, member); err != nil {
				return err
			}
		}

		for key, fieldSpec := range spec.Fields {
			if !fieldSpec.Required {
				continue
			}

			if _, ok := object[key]; ok {
				continue
			}

			return fmt.Errorf("%s field %q is required", field, key)
		}
	case ValueKindList:
		items, ok := runtimevalue.List(value)
		if !ok {
			return fmt.Errorf("%s expects list, got %T", field, value)
		}

		for i := range items {
			itemField := fmt.Sprintf("%s[%d]", field, i)
			if spec.Elem == nil {
				if err := runtimevalue.ValidateCanonical(itemField, items[i]); err != nil {
					return err
				}
				continue
			}

			if err := validateResolvedContract(itemField, *spec.Elem, items[i]); err != nil {
				return err
			}
		}
	}

	return nil
}

func contractKindString(contract ValueContract) string {
	return kindSetString(contract.KindsSet())
}

func contractsOverlap(left, right ValueContract) bool {
	if !left.Valid() || !right.Valid() {
		return false
	}

	leftKinds := left.KindsSet()
	rightKinds := right.KindsSet()
	if leftKinds.Contains(ValueKindAny) || rightKinds.Contains(ValueKindAny) {
		return true
	}

	for kind := range leftKinds {
		if rightKinds.Contains(kind) {
			return true
		}
	}

	return false
}

func resolvedValueKind(value any) ValueKind {
	switch runtimevalue.Wrap(value).Kind() {
	case runtimevalue.KindNull:
		return ValueKindNull
	case runtimevalue.KindBytes:
		return ValueKindBytes
	case runtimevalue.KindString:
		return ValueKindString
	case runtimevalue.KindBool:
		return ValueKindBool
	case runtimevalue.KindNumber:
		return ValueKindNumber
	case runtimevalue.KindObject:
		return ValueKindObject
	case runtimevalue.KindList:
		return ValueKindList
	default:
		return ValueKindAny
	}
}

func objectAllowsUnconstrainedMembers(spec ValueContract) bool {
	return len(spec.Fields) == 0 && spec.Elem == nil
}

func objectMemberContract(spec ValueContract, key string) (ValueContract, bool) {
	if fieldSpec, ok := spec.Fields[key]; ok {
		return fieldSpec, true
	}

	if spec.Elem != nil {
		return *spec.Elem, true
	}

	return ValueContract{}, false
}

func sortDiagnostics(diagnostics []Diagnostic) {
	sort.Slice(diagnostics, func(i, j int) bool {
		if diagnostics[i].Path != diagnostics[j].Path {
			return diagnostics[i].Path < diagnostics[j].Path
		}

		if diagnostics[i].Code != diagnostics[j].Code {
			return diagnostics[i].Code < diagnostics[j].Code
		}

		return diagnostics[i].Summary < diagnostics[j].Summary
	})
}
