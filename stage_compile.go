package theater

import "fmt"

type planCompileState struct {
	ordinal int
	paths   runtimePathCodec
}

type stageCompiler struct {
	state     *planCompileState
	fragments planFragmentCompiler
}

func compileStageSpec(spec StageSpec) *stagePlan {
	compiler := stageCompiler{
		state: &planCompileState{
			paths: runtimePathCodec{},
		},
		fragments: planFragmentCompiler{},
	}

	return compiler.compileStage(spec)
}

func (c stageCompiler) compileStage(spec StageSpec) *stagePlan {
	stagePath := c.state.paths.Join("stage", spec.ID)
	stage := &stagePlan{
		ID:            spec.ID,
		Path:          stagePath,
		PlanOrdinal:   c.state.nextOrdinal(),
		HTTP:          spec.HTTP.Clone(),
		State:         spec.State.Clone(),
		Scenarios:     make([]scenarioPlan, 0, len(spec.Scenarios)),
		ScenarioCalls: make([]scenarioCallPlan, 0, len(spec.ScenarioCalls)),
		SourceSpan:    cloneSourceRef(spec.SourceSpan),
	}

	for i := range spec.Scenarios {
		stage.Scenarios = append(stage.Scenarios, c.compileScenario(stagePath, spec.Scenarios[i]))
	}

	for i := range spec.ScenarioCalls {
		stage.ScenarioCalls = append(stage.ScenarioCalls, c.compileScenarioCall(stagePath, spec.ScenarioCalls[i]))
	}

	return stage
}

func (c stageCompiler) compileScenario(stagePath string, spec ScenarioSpec) scenarioPlan {
	scenarioPath := c.state.paths.JoinChild(stagePath, "scenario", spec.ID)
	scenario := scenarioPlan{
		ID:           spec.ID,
		Path:         scenarioPath,
		PlanOrdinal:  c.state.nextOrdinal(),
		Inputs:       cloneValueContracts(spec.Inputs),
		AuthBindings: c.compileHTTPAuthBindings(scenarioPath, spec.AuthBindings),
		Acts:         make([]actPlan, 0, len(spec.Acts)),
		SourceSpan:   cloneSourceRef(spec.SourceSpan),
	}

	for i := range spec.Acts {
		scenario.Acts = append(scenario.Acts, c.compileAct(scenarioPath, spec.Acts[i]))
	}

	return scenario
}

func (c stageCompiler) compileHTTPAuthBindings(
	scenarioPath string,
	specs map[string]HTTPAuthBindingSpec,
) map[string]httpAuthBindingPlan {
	if len(specs) == 0 {
		return nil
	}

	bindings := make(map[string]httpAuthBindingPlan, len(specs))
	for authName, spec := range specs {
		path := httpAuthBindingPath(scenarioPath, authName)
		plan := httpAuthBindingPlan{
			Path:  path,
			Slots: make(map[string]bindingPlan, len(spec.Slots)),
		}
		for slotName := range spec.Slots {
			plan.Slots[slotName] = c.fragments.compileBinding(httpAuthBindingSlotPath(path, slotName), spec.Slots[slotName])
		}
		if len(plan.Slots) == 0 {
			plan.Slots = nil
		}
		bindings[authName] = plan
	}

	return bindings
}

func (c stageCompiler) compileAct(scenarioPath string, spec ActSpec) actPlan {
	actPath := c.state.paths.JoinChild(scenarioPath, "act", spec.ID)
	act := actPlan{
		ID:          spec.ID,
		Path:        actPath,
		PlanOrdinal: c.state.nextOrdinal(),
		Eventually:  compileEventuallyPlan(spec.Eventually),
		Properties:  c.compileProperties(actPath, spec.Properties),
		Action: actionPlan{
			Use:        spec.Action.Use,
			With:       make(map[string]bindingPlan, len(spec.Action.With)),
			Repeatable: spec.Action.Repeatable,
			SourceSpan: cloneSourceRef(spec.Action.SourceSpan),
		},
		CaptureAuth:  compileHTTPAuthCapture(spec.CaptureAuth),
		Logs:         make([]logPlan, 0, len(spec.Logs)),
		Expectations: make([]expectationPlan, 0, len(spec.Expectations)),
		Exports:      make([]exportPlan, 0, len(spec.Exports)),
		Transitions:  make([]transitionPlan, 0, len(spec.Transitions)),
		SourceSpan:   cloneSourceRef(spec.SourceSpan),
	}

	for key := range spec.Action.With {
		act.Action.With[key] = c.fragments.compileBinding(bindingPath(actPath+"/action", key), spec.Action.With[key])
	}

	for i := range spec.Expectations {
		expectationPath := joinChildPath(actPath, "expectation", spec.Expectations[i].ID)
		act.Expectations = append(act.Expectations, c.fragments.compileExpectation(expectationPath, spec.Expectations[i]))
	}

	for i := range spec.Logs {
		logPath := logPath(actPath, spec.Logs[i].ID)
		act.Logs = append(act.Logs, c.fragments.compileLog(logPath, spec.Logs[i]))
	}

	for i := range spec.Exports {
		path := exportPath(actPath, exportAliasSpec(spec.Exports[i]))
		act.Exports = append(act.Exports, c.fragments.compileExport(path, spec.Exports[i]))
	}

	for i := range spec.Transitions {
		act.Transitions = append(act.Transitions, transitionPlan(spec.Transitions[i]))
	}

	return act
}

func (c stageCompiler) compileProperties(actPath string, specs map[string]PropertySpec) []propertyPlan {
	properties := make([]propertyPlan, 0, len(specs))
	names := make(map[string]struct{}, len(specs))
	propertyIDs := sortedPropertyIDs(specs)
	for _, propertyID := range propertyIDs {
		names[propertyID] = struct{}{}
	}

	for _, propertyID := range propertyIDs {
		spec := specs[propertyID]
		property := propertyPlan{
			ID:           propertyID,
			Path:         c.state.paths.JoinChild(actPath, "property", propertyID),
			Dependencies: propertyDependencies(nil, names),
			Decorators:   make([]decoratorPlan, 0, len(spec.Decorators)),
		}

		if spec.Value != nil {
			value := c.fragments.compileBinding(property.Path+"/value", *spec.Value)
			property.Value = &value
			property.Dependencies = bindingPropertyDependencies(*spec.Value, names)
		}

		if spec.Inventory != nil {
			property.Inventory = inventoryPlan{
				Present: true,
				Use:     spec.Inventory.Use,
				With:    make(map[string]bindingPlan, len(spec.Inventory.With)),
			}

			for key := range spec.Inventory.With {
				property.Inventory.With[key] = c.fragments.compileBinding(
					bindingPath(property.Path+"/inventory/with", key),
					spec.Inventory.With[key],
				)
			}

			property.Dependencies = propertyDependencies(spec.Inventory.With, names)
		}

		for i := range spec.Decorators {
			property.Decorators = append(property.Decorators, c.fragments.compileDecorator(spec.Decorators[i]))
		}

		properties = append(properties, property)
	}

	return orderPropertyPlans(properties)
}

func (c stageCompiler) compileScenarioCall(stagePath string, spec ScenarioCallSpec) scenarioCallPlan {
	call := scenarioCallPlan{
		ID:           spec.ID,
		Path:         c.state.paths.JoinChild(stagePath, "call", spec.ID),
		PlanOrdinal:  c.state.nextOrdinal(),
		ScenarioID:   spec.ScenarioID,
		Bindings:     make(map[string]bindingPlan, len(spec.Bindings)),
		Exports:      make([]exportPlan, 0, len(spec.Exports)),
		Dependencies: make([]scenarioDependencyPlan, 0, len(spec.Dependencies)),
		SourceSpan:   cloneSourceRef(spec.SourceSpan),
	}

	for i := range spec.Dependencies {
		when := spec.Dependencies[i].When
		if when == "" {
			when = TriggerPredicateSuccess
		}

		call.Dependencies = append(call.Dependencies, scenarioDependencyPlan{
			Path:   fmt.Sprintf("%s/dependency[%d]", call.Path, i),
			CallID: spec.Dependencies[i].CallID,
			When:   when,
		})
	}

	for key := range spec.Bindings {
		call.Bindings[key] = c.fragments.compileBinding(bindingPath(call.Path, key), spec.Bindings[key])
	}

	for i := range spec.Exports {
		path := exportPath(call.Path, exportAliasSpec(spec.Exports[i]))
		call.Exports = append(call.Exports, c.fragments.compileExport(path, spec.Exports[i]))
	}

	return call
}

func (s *planCompileState) nextOrdinal() int {
	s.ordinal++
	return s.ordinal
}

func compileEventuallyPlan(spec *EventuallySpec) *eventuallyPlan {
	if spec == nil {
		return nil
	}

	return &eventuallyPlan{
		TimeoutText:  spec.Timeout,
		IntervalText: spec.Interval,
	}
}

func compileHTTPAuthCapture(spec *HTTPAuthCaptureSpec) *httpAuthCapturePlan {
	if spec == nil {
		return nil
	}

	plan := &httpAuthCapturePlan{
		Auth:  spec.Auth,
		Slots: make(map[string]HTTPCaptureSourceSpec, len(spec.Slots)),
	}
	for name, source := range spec.Slots {
		plan.Slots[name] = source
	}
	if len(plan.Slots) == 0 {
		plan.Slots = nil
	}

	return plan
}

func exportAliasSpec(export ExportSpec) string {
	if export.As != "" {
		return export.As
	}

	if export.Ref != nil {
		return export.Ref.Name
	}

	return export.Field
}
