package theater

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/alex-poliushkin/theater/internal/runtimevalue"
)

func validatePropertyContracts(act *actPlan, catalog propertyCatalog, matchers MatcherResolver) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)

	for i := range act.Properties {
		property := &act.Properties[i]
		path := property.Path
		if !property.Inventory.Present || property.Inventory.Use == "" {
			continue
		}

		inventory, err := catalog.ResolveInventory(property.Inventory.Use)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "unknown_property_inventory_use",
				Path:     path,
				Severity: SeverityError,
				Summary:  err.Error(),
			})
			continue
		}

		current := inventory.Contract().Produces.Clone()
		if err := validateInventoryContract(inventory.Contract()); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "invalid_property_inventory_contract",
				Path:     path,
				Severity: SeverityError,
				Summary:  err.Error(),
			})
		}

		diagnostics = append(
			diagnostics,
			validateInventoryBindings(path+"/inventory/with", property.Inventory.With, inventory.Contract().Args, catalog, matchers, catalog)...,
		)

		for j := range property.Decorators {
			decorator := &property.Decorators[j]
			decoratorPath := joinChildPath(path, "decorator", decoratorKey(decorator, j))
			if decorator.Use == "" {
				continue
			}

			def, err := catalog.ResolveDecorator(decorator.Use)
			if err != nil {
				diagnostics = append(diagnostics, Diagnostic{
					Code:     "unknown_property_decorator_use",
					Path:     decoratorPath,
					Severity: SeverityError,
					Summary:  err.Error(),
				})
				continue
			}

			if err := validateDecoratorContract(def.Contract); err != nil {
				diagnostics = append(diagnostics, Diagnostic{
					Code:     "invalid_property_decorator_contract",
					Path:     decoratorPath,
					Severity: SeverityError,
					Summary:  err.Error(),
				})
				continue
			}

			resolvedArgs, paramDiagnostics := prepareDecoratorParams(decoratorPath, decorator.With, def.Contract.Params)
			diagnostics = append(diagnostics, paramDiagnostics...)
			if len(paramDiagnostics) != 0 {
				continue
			}

			if err := contractCompatibilityError(current, def.Contract.Accepts); err != nil {
				diagnostics = append(diagnostics, Diagnostic{
					Code:     "incompatible_property_decorator_input_kind",
					Path:     decoratorPath,
					Severity: SeverityError,
					Summary: fmt.Sprintf(
						"decorator %q accepts %s, got %s from previous step: %v",
						decorator.Use,
						contractKindString(def.Contract.Accepts),
						contractKindString(current),
						err,
					),
				})
				continue
			}

			if _, err := def.Compile(cloneValues(resolvedArgs)); err != nil {
				diagnostics = append(diagnostics, Diagnostic{
					Code:     "invalid_property_decorator_config",
					Path:     decoratorPath,
					Severity: SeverityError,
					Summary:  fmt.Sprintf("decorator %q is invalid: %v", decorator.Use, err),
				})
				continue
			}

			current = def.Contract.Produces.Clone()
		}
	}

	return diagnostics
}

func propertyValueContracts(act *actPlan, catalog propertyCatalog) map[string]ValueContract {
	if len(act.Properties) == 0 {
		return nil
	}

	contracts := make(map[string]ValueContract, len(act.Properties))
	for i := range act.Properties {
		property := &act.Properties[i]
		if property.ID == "" || !property.Inventory.Present || property.Inventory.Use == "" {
			continue
		}

		contract, ok := propertyValueContract(property, catalog)
		if !ok {
			continue
		}

		contracts[property.ID] = contract
	}

	if len(contracts) == 0 {
		return nil
	}

	return contracts
}

func propertyValueContract(property *propertyPlan, catalog propertyCatalog) (ValueContract, bool) {
	inventory, err := catalog.ResolveInventory(property.Inventory.Use)
	if err != nil {
		return ValueContract{}, false
	}

	inventoryContract := inventory.Contract()
	if err := validateInventoryContract(inventoryContract); err != nil {
		return ValueContract{}, false
	}

	current := inventoryContract.Produces.Clone()
	for i := range property.Decorators {
		decorator := &property.Decorators[i]
		if decorator.Use == "" {
			return ValueContract{}, false
		}

		def, err := catalog.ResolveDecorator(decorator.Use)
		if err != nil {
			return ValueContract{}, false
		}

		if err := validateDecoratorContract(def.Contract); err != nil {
			return ValueContract{}, false
		}

		resolvedArgs, diagnostics := prepareDecoratorParams("", decorator.With, def.Contract.Params)
		if len(diagnostics) != 0 {
			return ValueContract{}, false
		}

		if err := contractCompatibilityError(current, def.Contract.Accepts); err != nil {
			return ValueContract{}, false
		}

		if _, err := def.Compile(cloneValues(resolvedArgs)); err != nil {
			return ValueContract{}, false
		}

		current = def.Contract.Produces.Clone()
	}

	return current, true
}

func validateInventoryContract(contract InventoryContract) error {
	if !contract.Produces.Valid() {
		return errors.New("inventory contract must declare a valid produces contract")
	}

	seen := make(map[string]struct{}, len(contract.Args))
	for i := range contract.Args {
		arg := contract.Args[i]
		if arg.Name == "" {
			return errors.New("inventory arg name is required")
		}

		if !arg.Accepts.Valid() {
			return fmt.Errorf("inventory arg %q must declare a valid accepts contract", arg.Name)
		}

		if _, ok := seen[arg.Name]; ok {
			return fmt.Errorf("inventory arg %q is declared more than once", arg.Name)
		}

		seen[arg.Name] = struct{}{}
	}

	return nil
}

func validateInventoryBindings(
	path string,
	bindings map[string]bindingPlan,
	specs []ArgSpec,
	resolver GeneratorResolver,
	matchers MatcherResolver,
	decorators DecoratorResolver,
) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	specIndex := make(map[string]ArgSpec, len(specs))
	for i := range specs {
		spec := specs[i]
		specIndex[spec.Name] = spec
	}

	for key := range bindings {
		binding := bindings[key]
		spec, ok := specIndex[key]
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "unknown_inventory_arg",
				Path:     bindingPath(path, key),
				Severity: SeverityError,
				Summary:  fmt.Sprintf("inventory arg %q is not declared", key),
			})
			continue
		}

		if err := validateBindingContractWithResolver(resolver, matchers, decorators, binding, spec.Accepts); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "incompatible_inventory_arg",
				Path:     bindingPath(path, key),
				Severity: SeverityError,
				Summary:  fmt.Sprintf("inventory arg %q %v", key, err),
			})
		}
	}

	for i := range specs {
		spec := specs[i]
		if !spec.Required {
			continue
		}

		if _, ok := bindings[spec.Name]; ok {
			continue
		}

		diagnostics = append(diagnostics, Diagnostic{
			Code:     "missing_inventory_arg",
			Path:     path,
			Severity: SeverityError,
			Summary:  fmt.Sprintf("inventory arg %q is required", spec.Name),
		})
	}

	return diagnostics
}

func validateDecoratorContract(contract DecoratorContract) error {
	if !contract.Accepts.Valid() {
		return errors.New("decorator contract must declare a valid accepts contract")
	}

	if !contract.Produces.Valid() {
		return errors.New("decorator contract must declare a valid produces contract")
	}

	seen := make(map[string]struct{}, len(contract.Params))
	for i := range contract.Params {
		param := contract.Params[i]
		if param.Name == "" {
			return errors.New("decorator param name is required")
		}

		if !param.Accepts.Valid() {
			return fmt.Errorf("decorator param %q must declare a valid accepts contract", param.Name)
		}

		for j := range param.Enum {
			if err := validateResolvedContract(param.Name, param.Accepts, param.Enum[j]); err != nil {
				return fmt.Errorf("decorator param %q enum value is invalid: %w", param.Name, err)
			}
		}

		if param.Default != nil {
			if param.Required {
				return fmt.Errorf("decorator param %q cannot be both required and defaulted", param.Name)
			}

			if err := validateResolvedContract(param.Name, param.Accepts, param.Default); err != nil {
				return fmt.Errorf("decorator param %q default is invalid: %w", param.Name, err)
			}

			if len(param.Enum) != 0 && !enumContains(param.Enum, param.Default) {
				return fmt.Errorf("decorator param %q default must be one of %v", param.Name, param.Enum)
			}
		}

		if _, ok := seen[param.Name]; ok {
			return fmt.Errorf("decorator param %q is declared more than once", param.Name)
		}
		seen[param.Name] = struct{}{}
	}

	return nil
}

func prepareDecoratorParams(path string, args Values, specs []ParamSpec) (resolved Values, diagnostics []Diagnostic) {
	resolved = applyDecoratorParamDefaults(args, specs)
	diagnostics = validateResolvedParams(
		path,
		args,
		resolved,
		specs,
		"decorator",
		"unexpected_property_decorator_param",
		"missing_property_decorator_param",
		"incompatible_property_decorator_param",
		"invalid_property_decorator_param",
	)
	return resolved, diagnostics
}

func resolveDecoratorArgs(args Values, specs []ParamSpec) (Values, error) {
	resolved := applyDecoratorParamDefaults(args, specs)
	specIndex := make(map[string]ParamSpec, len(specs))
	for i := range specs {
		specIndex[specs[i].Name] = specs[i]
	}

	for key := range args {
		if _, ok := specIndex[key]; !ok {
			return nil, fmt.Errorf("decorator does not support param %q", key)
		}
	}

	for i := range specs {
		spec := specs[i]
		value, ok := resolved[spec.Name]
		if !ok {
			if spec.Required {
				return nil, fmt.Errorf("decorator requires param %q", spec.Name)
			}
			continue
		}

		if err := validateResolvedContract(spec.Name, spec.Accepts, value); err != nil {
			return nil, err
		}

		if len(spec.Enum) != 0 && !enumContains(spec.Enum, value) {
			return nil, fmt.Errorf("decorator param %q must be one of %v", spec.Name, spec.Enum)
		}
	}

	return protectDecoratorArgs(resolved, specs), nil
}

func applyDecoratorParamDefaults(args Values, specs []ParamSpec) Values {
	resolved := cloneValues(args)
	for i := range specs {
		spec := specs[i]
		if spec.Default == nil {
			continue
		}

		if resolved == nil {
			resolved = make(Values)
		}
		if _, ok := resolved[spec.Name]; ok {
			continue
		}

		resolved[spec.Name] = runtimevalue.Clone(spec.Default)
	}

	if len(resolved) == 0 {
		return nil
	}

	return resolved
}

func validateResolvedParams(
	path string,
	args Values,
	resolved Values,
	specs []ParamSpec,
	subject string,
	unexpectedCode string,
	missingCode string,
	incompatibleCode string,
	invalidCode string,
) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	specIndex := make(map[string]ParamSpec, len(specs))
	for i := range specs {
		specIndex[specs[i].Name] = specs[i]
	}

	for key := range args {
		if _, ok := specIndex[key]; ok {
			continue
		}

		diagnostics = append(diagnostics, Diagnostic{
			Code:     unexpectedCode,
			Path:     bindingPath(path+"/with", key),
			Severity: SeverityError,
			Summary:  fmt.Sprintf("%s does not support param %q", subject, key),
		})
	}

	for i := range specs {
		spec := specs[i]
		value, ok := resolved[spec.Name]
		if !ok {
			if !spec.Required {
				continue
			}

			diagnostics = append(diagnostics, Diagnostic{
				Code:     missingCode,
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("%s requires param %q", subject, spec.Name),
			})
			continue
		}

		if err := validateResolvedContract(spec.Name, spec.Accepts, value); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     incompatibleCode,
				Path:     bindingPath(path+"/with", spec.Name),
				Severity: SeverityError,
				Summary:  err.Error(),
			})
			continue
		}

		if len(spec.Enum) != 0 && !enumContains(spec.Enum, value) {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     invalidCode,
				Path:     bindingPath(path+"/with", spec.Name),
				Severity: SeverityError,
				Summary:  fmt.Sprintf("%s param %q must be one of %v", subject, spec.Name, spec.Enum),
			})
		}
	}

	return diagnostics
}

func cloneDecoratorContract(contract DecoratorContract) DecoratorContract {
	cloned := contract
	cloned.Accepts = contract.Accepts.Clone()
	cloned.Produces = contract.Produces.Clone()
	cloned.Params = cloneParamSpecs(contract.Params)
	return cloned
}

func cloneParamSpecs(specs []ParamSpec) []ParamSpec {
	if len(specs) == 0 {
		return nil
	}

	cloned := make([]ParamSpec, len(specs))
	for i := range specs {
		cloned[i] = ParamSpec{
			Name:        specs[i].Name,
			Accepts:     specs[i].Accepts.Clone(),
			Required:    specs[i].Required,
			Default:     runtimevalue.Clone(specs[i].Default),
			Enum:        runtimevalue.CloneSlice(specs[i].Enum),
			Description: specs[i].Description,
		}
	}

	return cloned
}

func kindSetSubset(actual, allowed ValueKindSet) bool {
	if len(actual) == 0 || len(allowed) == 0 {
		return false
	}

	for kind := range actual {
		if !allowed.Contains(kind) && !allowed.Contains(ValueKindAny) {
			return false
		}
	}

	return true
}

func contractCompatibilityError(actual, allowed ValueContract) error {
	if !actual.Valid() || !allowed.Valid() {
		return errors.New("invalid value contract")
	}

	actualKinds := actual.KindsSet()
	allowedKinds := allowed.KindsSet()
	if allowedKinds.Contains(ValueKindAny) {
		return nil
	}
	if !kindSetSubset(actualKinds, allowedKinds) {
		return kindSubsetError(actualKinds, allowedKinds)
	}

	for kind := range actualKinds {
		switch kind {
		case ValueKindObject:
			if err := objectContractCompatibilityError(actual, allowed); err != nil {
				return err
			}
		case ValueKindList:
			if err := listContractCompatibilityError(actual, allowed); err != nil {
				return err
			}
		}
	}

	return nil
}

func kindSubsetError(actualKinds, allowedKinds ValueKindSet) error {
	rejected := make([]string, 0, len(actualKinds))
	for kind := range actualKinds {
		if allowedKinds.Contains(kind) {
			continue
		}

		rejected = append(rejected, string(kind))
	}

	sort.Strings(rejected)
	if len(rejected) == 1 {
		return fmt.Errorf("kind %q is not accepted", rejected[0])
	}

	return fmt.Errorf("kinds %s are not accepted", strings.Join(rejected, ", "))
}

func objectContractCompatibilityError(actual, allowed ValueContract) error {
	if err := allowedObjectFieldsCompatibilityError(actual, allowed); err != nil {
		return err
	}
	if err := actualObjectFieldsCompatibilityError(actual, allowed); err != nil {
		return err
	}
	if err := actualObjectElemCompatibilityError(actual, allowed); err != nil {
		return err
	}

	return nil
}

func allowedObjectFieldsCompatibilityError(actual, allowed ValueContract) error {
	for key, allowedField := range allowed.Fields {
		actualField, ok := actual.Fields[key]
		if ok {
			if allowedField.Required && !actualField.Required {
				return fmt.Errorf("required field %q is not guaranteed", key)
			}
			if err := contractCompatibilityError(actualField, allowedField); err != nil {
				return fmt.Errorf("field %q: %w", key, err)
			}
			continue
		}

		if actual.Elem != nil {
			if err := contractCompatibilityError(*actual.Elem, allowedField); err != nil {
				return fmt.Errorf("field %q via elem: %w", key, err)
			}
		}
		if allowedField.Required {
			return fmt.Errorf("required field %q is not guaranteed", key)
		}
	}

	return nil
}

func actualObjectFieldsCompatibilityError(actual, allowed ValueContract) error {
	for key, actualField := range actual.Fields {
		allowedField, ok := allowed.Fields[key]
		if ok {
			if err := contractCompatibilityError(actualField, allowedField); err != nil {
				return fmt.Errorf("field %q: %w", key, err)
			}
			continue
		}

		if objectAllowsUnconstrainedMembers(allowed) {
			continue
		}
		if allowed.Elem == nil {
			return fmt.Errorf("field %q is not accepted by next step", key)
		}
		if err := contractCompatibilityError(actualField, *allowed.Elem); err != nil {
			return fmt.Errorf("field %q: %w", key, err)
		}
	}

	return nil
}

func actualObjectElemCompatibilityError(actual, allowed ValueContract) error {
	if actual.Elem == nil {
		return nil
	}
	if objectAllowsUnconstrainedMembers(allowed) {
		return nil
	}
	if allowed.Elem == nil {
		return errors.New("arbitrary object members are not accepted by next step")
	}
	if err := contractCompatibilityError(*actual.Elem, *allowed.Elem); err != nil {
		return fmt.Errorf("object elem: %w", err)
	}

	for key, allowedField := range allowed.Fields {
		if _, ok := actual.Fields[key]; ok {
			continue
		}
		if err := contractCompatibilityError(*actual.Elem, allowedField); err != nil {
			return fmt.Errorf("field %q via elem: %w", key, err)
		}
	}

	return nil
}

func listContractCompatibilityError(actual, allowed ValueContract) error {
	if allowed.Elem == nil {
		return nil
	}
	if actual.Elem == nil {
		return errors.New("list elements are not guaranteed")
	}
	if err := contractCompatibilityError(*actual.Elem, *allowed.Elem); err != nil {
		return fmt.Errorf("list elements: %w", err)
	}

	return nil
}

func kindSetString(kinds ValueKindSet) string {
	if len(kinds) == 0 {
		return "<none>"
	}

	names := make([]string, 0, len(kinds))
	for kind := range kinds {
		names = append(names, string(kind))
	}

	sort.Strings(names)
	return strings.Join(names, "|")
}

func enumContains(values []any, candidate any) bool {
	for i := range values {
		if reflect.DeepEqual(runtimevalue.Reveal(values[i]), runtimevalue.Reveal(candidate)) {
			return true
		}
	}

	return false
}

func decoratorKey(plan *decoratorPlan, index int) string {
	if plan.Use != "" {
		return plan.Use
	}

	return strconv.Itoa(index)
}
