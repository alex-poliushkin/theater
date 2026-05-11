package theater

import "fmt"

const (
	nestedMatcherAllItemsRef = "expectation.all_items"
	nestedMatcherHasEntryRef = "expectation.has_entry"
	nestedMatcherHasItemRef  = "expectation.has_item"
	nestedMatcherNotRef      = "expectation.not"
)

type stageContractValidator struct {
	stage       *stagePlan
	catalog     CatalogResolver
	matchers    MatcherResolver
	diagnostics diagnosticSet
}

func newStageContractValidator(stage *stagePlan, catalog CatalogResolver, matchers MatcherResolver) *stageContractValidator {
	return &stageContractValidator{
		stage:    stage,
		catalog:  catalog,
		matchers: matchers,
	}
}

func (v *stageContractValidator) Validate() []Diagnostic {
	finalRootsByScenario := make(map[string]map[string]struct{}, len(v.stage.Scenarios))
	stateDiagnostics, stateDescriptors := validateStateRegistry(v.stage, v.catalog)
	v.diagnostics.addAll(stateDiagnostics)

	for i := range v.stage.Scenarios {
		scenario := &v.stage.Scenarios[i]
		scopeAnalysis := analyzeScenarioScope(*scenario)
		contractAnalysis := analyzeScenarioContracts(*scenario, scopeAnalysis, v.catalog, v.catalog)
		stateHandles := analyzeScenarioStateHandles(*scenario, scopeAnalysis)
		finalRootsByScenario[scenario.ID] = finalScenarioRoots(*scenario, scopeAnalysis)
		v.diagnostics.addAll(validateScenarioScopeCollisions(*scenario, scopeAnalysis))
		for j := range scenario.Acts {
			v.validateAct(&scenario.Acts[j], scopeAnalysis, contractAnalysis, stateHandles, stateDescriptors)
		}
	}

	for i := range v.stage.ScenarioCalls {
		v.validateScenarioCallBindings(&v.stage.ScenarioCalls[i])
		v.validateScenarioCallExports(&v.stage.ScenarioCalls[i], finalRootsByScenario)
	}

	sortDiagnostics(v.diagnostics.items)
	return v.diagnostics.items
}

func (v *stageContractValidator) validateScenarioCallBindings(call *scenarioCallPlan) {
	if call == nil {
		return
	}

	scenario := scenarioByID(v.stage.Scenarios, call.ScenarioID)
	if scenario == nil {
		return
	}

	v.diagnostics.addAll(validateScenarioCallBindings(*call, scenario.Inputs, v.catalog, v.matchers, v.catalog))
}

func (v *stageContractValidator) validateAct(
	act *actPlan,
	scopeAnalysis scenarioScopeAnalysis,
	contractAnalysis scenarioContractAnalysis,
	stateHandles scenarioStateHandleAnalysis,
	stateDescriptors map[string]StateDescriptor,
) {
	actionOutputs := map[string]ValueContract(nil)
	actEntryRoots := scopeAnalysis.actRoots(act.ID)
	actEntryContracts := contractAnalysis.actContracts(act.ID)

	if dependencyMissing(v.catalog) {
		if !dependencyMissing(v.matchers) {
			v.validateMatchers(act, actionOutputs)
		}
		return
	}

	actionOutputs = v.validateActActionContracts(act, scopeAnalysis, actEntryRoots, actEntryContracts)
	v.diagnostics.addAll(validateStateActContracts(act, stateHandles.actEntry[act.ID], stateDescriptors))
	if !dependencyMissing(v.matchers) {
		v.validateMatchers(act, actionOutputs)
	}
}

func (v *stageContractValidator) validateActActionContracts(
	act *actPlan,
	scopeAnalysis scenarioScopeAnalysis,
	actEntryRoots map[string]struct{},
	actEntryContracts map[string]ValueContract,
) map[string]ValueContract {
	actionPath := act.Path + "/action"
	v.diagnostics.addAll(validatePropertyContracts(act, v.catalog, v.matchers))
	propertyOutputs := propertyValueContracts(act, v.catalog)

	runner, err := v.catalog.ResolveAction(act.Action.Use)
	if err != nil {
		v.diagnostics.add(actionPath, "unknown_action_use", err.Error())
		return nil
	}

	contract := runner.Contract()
	v.diagnostics.addAll(validateActionBindings(actionPath, act.Action.With, contract.Inputs, v.catalog, v.matchers, v.catalog))
	v.diagnostics.addAll(validateActionOutputs(act, contract.Outputs, v.catalog, v.matchers, v.catalog))
	v.diagnostics.addAll(validateActRefExports(act, actEntryContracts, propertyOutputs, v.catalog, v.matchers, v.catalog))
	v.diagnostics.addAll(validateLogActionOutputs(act, contract.Outputs, v.catalog, v.matchers, v.catalog))
	v.diagnostics.addAll(validatePropertyExpectationSubjects(act, propertyOutputs, v.catalog, v.matchers, v.catalog))
	if scopeAnalysis.isReachable(act.ID) {
		v.diagnostics.addAll(validateExpectationOutputRootCollisions(act, actEntryRoots, contract.Outputs))
		v.diagnostics.addAll(validateExpectationArgRefs(act, actEntryRoots, contract.Outputs))
		v.diagnostics.addAll(validateActExportRefs(act, actEntryRoots, contract.Outputs))
	}

	return contract.Outputs
}

func (v *stageContractValidator) validateMatchers(
	act *actPlan,
	actionOutputs map[string]ValueContract,
) {
	propertyOutputs := propertyValueContracts(act, v.catalog)

	for i := range act.Expectations {
		expectation := &act.Expectations[i]
		path := joinChildPath(act.Path, "expectation", expectation.ID)

		descriptor, err := v.matchers.Resolve(expectation.Assert.Ref)
		if err != nil {
			v.diagnostics.add(path+"/assert", "unknown_expectation_assert_ref", err.Error())
			continue
		}

		argDiagnostics := validateExpectationArgs(path, expectation, descriptor, v.catalog, v.matchers, v.catalog)
		nestedDiagnostics := validateNestedMatcherArgs(
			path+"/assert",
			expectation.Assert.Ref,
			expectation.Assert.Args,
			v.catalog,
			v.matchers,
			v.catalog,
		)
		v.diagnostics.addAll(argDiagnostics)
		v.diagnostics.addAll(nestedDiagnostics)
		v.diagnostics.addAll(validateExpectationSubjectKind(path, expectation, actionOutputs, propertyOutputs, descriptor, v.catalog))
		if len(argDiagnostics) == 0 && len(nestedDiagnostics) == 0 {
			v.diagnostics.addAll(validateExpectationCompile(path, expectation, descriptor, v.matchers))
		}
	}
}

func (v *stageContractValidator) validateScenarioCallExports(
	call *scenarioCallPlan,
	finalScenarioRoots map[string]map[string]struct{},
) {
	availableRoots, ok := finalScenarioRoots[call.ScenarioID]
	if !ok {
		return
	}

	for i := range call.Exports {
		export := call.Exports[i]
		path := exportPath(call.Path, exportAlias(export))
		if export.Ref == nil || export.Ref.Name == "" {
			if _, _, err := validateSelectorBindingContracts(export.selectorPlan, v.catalog, v.matchers, v.catalog); err != nil {
				v.diagnostics.add(path, "incompatible_scenario_call_export_transform", err.Error())
			}
			continue
		}

		if _, ok := availableRoots[export.Ref.Name]; !ok {
			v.diagnostics.add(
				path,
				"unresolved_scenario_call_export_ref",
				fmt.Sprintf("scenario call export ref %q is not available in final scenario scope", export.Ref.Name),
			)
		}

		v.diagnostics.addAll(
			validateSelectorBindingRefAvailability(
				path,
				export.Ref.selectorPlan,
				availableRoots,
				func(name string) string {
					return fmt.Sprintf("binding ref %q is not available in final scenario scope", name)
				},
			),
		)

		if _, _, err := validateSelectorBindingContracts(export.Ref.selectorPlan, v.catalog, v.matchers, v.catalog); err != nil {
			v.diagnostics.add(path, "incompatible_scenario_call_export_transform", err.Error())
		}
	}
}

func validateActExportRefs(
	act *actPlan,
	actEntryRoots map[string]struct{},
	actionOutputs map[string]ValueContract,
) []Diagnostic {
	availableRoots := cloneRootSet(actEntryRoots)
	for i := range act.Properties {
		if act.Properties[i].ID == "" {
			continue
		}

		availableRoots[act.Properties[i].ID] = struct{}{}
	}
	for name := range actionOutputs {
		availableRoots[name] = struct{}{}
	}

	diagnostics := make([]Diagnostic, 0)
	for i := range act.Exports {
		path := exportPath(act.Path, exportAlias(act.Exports[i]))
		if act.Exports[i].Ref != nil {
			diagnostics = append(
				diagnostics,
				validateSelectorBindingRefAvailability(
					path,
					act.Exports[i].Ref.selectorPlan,
					availableRoots,
					func(name string) string {
						return fmt.Sprintf("binding ref %q is not available in scenario scope at this point", name)
					},
				)...,
			)
		}
		diagnostics = append(
			diagnostics,
			validateSelectorBindingRefAvailability(
				path,
				act.Exports[i].selectorPlan,
				availableRoots,
				func(name string) string {
					return fmt.Sprintf("binding ref %q is not available in scenario scope at this point", name)
				},
			)...,
		)
	}

	return diagnostics
}

func validateStageContracts(stage *stagePlan, catalog CatalogResolver, matchers MatcherResolver) []Diagnostic {
	return newStageContractValidator(stage, catalog, matchers).Validate()
}

func (v contractValidator) Validate(stage *stagePlan) []Diagnostic {
	return validateStageContracts(stage, v.catalog, v.matchers)
}

func validateExpectationArgs(
	path string,
	expectation *expectationPlan,
	descriptor MatcherDescriptor,
	resolver GeneratorResolver,
	matchers MatcherResolver,
	decorators DecoratorResolver,
) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	argSpecs := make(map[string]MatcherArg, len(descriptor.Args))
	for i := range descriptor.Args {
		arg := descriptor.Args[i]
		argSpecs[arg.Name] = arg
	}

	for key := range expectation.Assert.Args {
		binding := expectation.Assert.Args[key]
		arg, ok := argSpecs[key]
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "unexpected_expectation_assert_arg",
				Path:     bindingPath(path+"/assert", key),
				Severity: SeverityError,
				Summary:  fmt.Sprintf("assert %q does not support arg %q", expectation.Assert.Ref, key),
			})
			continue
		}

		if err := validateBindingContractWithResolver(resolver, matchers, decorators, binding, arg.Accepts); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "incompatible_expectation_assert_arg",
				Path:     bindingPath(path+"/assert", key),
				Severity: SeverityError,
				Summary:  fmt.Sprintf("assert %q arg %q %v", expectation.Assert.Ref, key, err),
			})
		}
	}

	for i := range descriptor.Args {
		arg := descriptor.Args[i]
		if !arg.Required {
			continue
		}

		if _, ok := expectation.Assert.Args[arg.Name]; ok {
			continue
		}

		diagnostics = append(diagnostics, Diagnostic{
			Code:     "missing_expectation_assert_arg",
			Path:     path + "/assert",
			Severity: SeverityError,
			Summary:  fmt.Sprintf("assert %q requires arg %q", expectation.Assert.Ref, arg.Name),
		})
	}

	return diagnostics
}

func validateNestedMatcherArgs(
	assertPath string,
	ref string,
	args map[string]bindingPlan,
	resolver GeneratorResolver,
	matchers MatcherResolver,
	decorators DecoratorResolver,
) []Diagnostic {
	switch ref {
	case nestedMatcherNotRef, nestedMatcherHasEntryRef:
		binding, ok := args["assert"]
		if !ok {
			return nil
		}
		nested, ok := nestedAssertPlan(binding)
		if !ok {
			return nil
		}
		return validateNestedMatcherAssert(bindingPath(assertPath, "assert"), nested, resolver, matchers, decorators)
	case nestedMatcherHasItemRef, nestedMatcherAllItemsRef:
		binding, ok := args["where"]
		if !ok {
			return nil
		}
		return validateWhereMatcherArgs(bindingPath(assertPath, "where"), binding, resolver, matchers, decorators)
	default:
		return nil
	}
}

func validateWhereMatcherArgs(
	wherePath string,
	where bindingPlan,
	resolver GeneratorResolver,
	matchers MatcherResolver,
	decorators DecoratorResolver,
) []Diagnostic {
	if where.Kind != BindingKindList {
		return nil
	}

	diagnostics := make([]Diagnostic, 0)
	for i := range where.List {
		item := where.List[i]
		if item.Kind != BindingKindObject {
			continue
		}
		assertBinding, ok := item.Object["assert"]
		if !ok {
			continue
		}
		nested, ok := nestedAssertPlan(assertBinding)
		if !ok {
			continue
		}
		itemPath := bindingPath(wherePath, listBindingKey(i))
		diagnostics = append(
			diagnostics,
			validateNestedMatcherAssert(bindingPath(itemPath, "assert"), nested, resolver, matchers, decorators)...,
		)
	}
	return diagnostics
}

func validateNestedMatcherAssert(
	assertPath string,
	assert assertPlan,
	resolver GeneratorResolver,
	matchers MatcherResolver,
	decorators DecoratorResolver,
) []Diagnostic {
	descriptor, err := matchers.Resolve(assert.Ref)
	if err != nil {
		return []Diagnostic{{
			Code:     "unknown_expectation_assert_ref",
			Path:     assertPath,
			Severity: SeverityError,
			Summary:  err.Error(),
		}}
	}

	diagnostics := validateAssertArgs(assertPath, assert.Ref, assert.Args, descriptor, resolver, matchers, decorators)
	nestedDiagnostics := validateNestedMatcherArgs(assertPath, assert.Ref, assert.Args, resolver, matchers, decorators)
	diagnostics = append(diagnostics, nestedDiagnostics...)
	if len(diagnostics) != 0 {
		return diagnostics
	}

	return validateAssertCompile(assertPath, assert.Ref, assert.Args, descriptor, matchers)
}

func validateAssertArgs(
	assertPath string,
	ref string,
	args map[string]bindingPlan,
	descriptor MatcherDescriptor,
	resolver GeneratorResolver,
	matchers MatcherResolver,
	decorators DecoratorResolver,
) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	argSpecs := make(map[string]MatcherArg, len(descriptor.Args))
	for i := range descriptor.Args {
		arg := descriptor.Args[i]
		argSpecs[arg.Name] = arg
	}

	for key := range args {
		binding := args[key]
		arg, ok := argSpecs[key]
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "unexpected_expectation_assert_arg",
				Path:     bindingPath(assertPath, key),
				Severity: SeverityError,
				Summary:  fmt.Sprintf("assert %q does not support arg %q", ref, key),
			})
			continue
		}

		if err := validateBindingContractWithResolver(resolver, matchers, decorators, binding, arg.Accepts); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "incompatible_expectation_assert_arg",
				Path:     bindingPath(assertPath, key),
				Severity: SeverityError,
				Summary:  fmt.Sprintf("assert %q arg %q %v", ref, key, err),
			})
		}
	}

	for i := range descriptor.Args {
		arg := descriptor.Args[i]
		if !arg.Required {
			continue
		}
		if _, ok := args[arg.Name]; ok {
			continue
		}

		diagnostics = append(diagnostics, Diagnostic{
			Code:     "missing_expectation_assert_arg",
			Path:     assertPath,
			Severity: SeverityError,
			Summary:  fmt.Sprintf("assert %q requires arg %q", ref, arg.Name),
		})
	}

	return diagnostics
}

func validateAssertCompile(
	assertPath string,
	ref string,
	args map[string]bindingPlan,
	descriptor MatcherDescriptor,
	matchers MatcherResolver,
) []Diagnostic {
	if !bindingsStatic(args) {
		return nil
	}

	resolved, err := resolveStaticBindings(args)
	if err != nil {
		return []Diagnostic{{
			Code:     "invalid_expectation_assert_args",
			Path:     assertPath,
			Severity: SeverityError,
			Summary:  err.Error(),
		}}
	}

	if _, err := descriptor.Compile(newMatcherCompileResolver(matchers), resolved); err != nil {
		return []Diagnostic{{
			Code:     "invalid_expectation_assert_args",
			Path:     assertPath,
			Severity: SeverityError,
			Summary:  fmt.Sprintf("assert %q is invalid: %v", ref, err),
		}}
	}

	return nil
}

func nestedAssertPlan(binding bindingPlan) (assertPlan, bool) {
	if binding.Kind != BindingKindObject {
		return assertPlan{}, false
	}
	refBinding, ok := binding.Object["ref"]
	if !ok || refBinding.Kind != BindingKindLiteral {
		return assertPlan{}, false
	}
	ref, ok := refBinding.Value.(string)
	if !ok || ref == "" {
		return assertPlan{}, false
	}
	argsBinding, ok := binding.Object["args"]
	if !ok {
		return assertPlan{Ref: ref, Args: map[string]bindingPlan{}}, true
	}
	if argsBinding.Kind != BindingKindObject {
		return assertPlan{}, false
	}

	return assertPlan{Ref: ref, Args: argsBinding.Object}, true
}

func scenarioByID(scenarios []scenarioPlan, id string) *scenarioPlan {
	for i := range scenarios {
		if scenarios[i].ID == id {
			return &scenarios[i]
		}
	}

	return nil
}

func validateExpectationSubjectKind(
	path string,
	expectation *expectationPlan,
	actionOutputs map[string]ValueContract,
	propertyOutputs map[string]ValueContract,
	descriptor MatcherDescriptor,
	decorators DecoratorResolver,
) []Diagnostic {
	contract, ok, known := selectedExpectationSubjectContract(expectation, actionOutputs, propertyOutputs, decorators)
	if !ok || !known {
		return nil
	}

	if descriptor.Actual.Valid() && !contractsOverlap(contract, descriptor.Actual) {
		return []Diagnostic{{
			Code:     "incompatible_expectation_subject_kind",
			Path:     path,
			Severity: SeverityError,
			Summary: fmt.Sprintf(
				"expectation subject %s produces %s, incompatible with matcher %q",
				expectationSubjectSelector(expectation.Subject),
				contractKindString(contract),
				expectation.Assert.Ref,
			),
		}}
	}

	return nil
}

func validateExpectationCompile(
	path string,
	expectation *expectationPlan,
	descriptor MatcherDescriptor,
	matchers MatcherResolver,
) []Diagnostic {
	if !bindingsStatic(expectation.Assert.Args) {
		return nil
	}

	args, err := resolveStaticBindings(expectation.Assert.Args)
	if err != nil {
		return []Diagnostic{{
			Code:     "invalid_expectation_assert_args",
			Path:     path + "/assert",
			Severity: SeverityError,
			Summary:  err.Error(),
		}}
	}

	if _, err := descriptor.Compile(newMatcherCompileResolver(matchers), args); err != nil {
		return []Diagnostic{{
			Code:     "invalid_expectation_assert_args",
			Path:     path + "/assert",
			Severity: SeverityError,
			Summary:  fmt.Sprintf("assert %q is invalid: %v", expectation.Assert.Ref, err),
		}}
	}

	return nil
}

func selectedExpectationSubjectContract(
	expectation *expectationPlan,
	actionOutputs map[string]ValueContract,
	propertyOutputs map[string]ValueContract,
	decorators DecoratorResolver,
) (selected ValueContract, ok, known bool) {
	switch expectation.Subject.From {
	case SubjectFromProperty:
		contract, ok := propertyOutputs[expectation.Subject.Ref]
		if !ok {
			return ValueContract{}, false, false
		}

		selected, known, err := selectedSelectorContract(expectation.Subject.selectorPlan, contract, decorators)
		if err != nil {
			return ValueContract{}, false, false
		}

		return selected, true, known
	default:
		contract, ok := actionOutputs[expectation.Subject.Field]
		if !ok {
			return ValueContract{}, false, false
		}

		selected, known, err := selectedSelectorContract(expectation.Subject.selectorPlan, contract, decorators)
		if err != nil {
			return ValueContract{}, false, false
		}

		return selected, true, known
	}
}

func expectationSubjectSelector(subject subjectPlan) string {
	if subject.From == SubjectFromProperty {
		return fmt.Sprintf("ref %q", subject.Ref)
	}

	return fmt.Sprintf("field %q", subject.Field)
}
