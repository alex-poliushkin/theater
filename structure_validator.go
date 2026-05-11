package theater

import (
	"fmt"
	"strings"
	"time"
)

type diagnosticSet struct {
	items []Diagnostic
}

type structureValidator struct {
	stage       *stagePlan
	diagnostics diagnosticSet
	scenarios   scenarioAddressRegistry
	access      internalScenarioAccessPolicy
	callIDs     map[string]struct{}
	callExports map[string]map[string]struct{}
}

func newStructureValidator(stage *stagePlan) *structureValidator {
	return &structureValidator{
		stage:       stage,
		scenarios:   newScenarioAddressRegistry(stage.Scenarios),
		access:      internalScenarioAccessPolicy{},
		callIDs:     make(map[string]struct{}, len(stage.ScenarioCalls)),
		callExports: make(map[string]map[string]struct{}, len(stage.ScenarioCalls)),
	}
}

func (v *structureValidator) Validate() []Diagnostic {
	v.validateStage()
	sortDiagnostics(v.diagnostics.items)
	return v.diagnostics.items
}

func (v *structureValidator) validateStage() {
	if v.stage.ID == "" {
		v.diagnostics.add(v.stage.Path, "missing_stage_id", "stage id is required")
	}

	v.diagnostics.addAll(duplicateScenarioIDDiagnostics(v.stage.Scenarios))
	v.diagnostics.addAll(duplicateScenarioCallIDDiagnostics(v.stage.ScenarioCalls))

	for _, scenario := range v.stage.Scenarios {
		v.validateScenario(scenario)
	}

	for _, call := range v.stage.ScenarioCalls {
		v.validateScenarioCall(call)
	}

	for _, call := range v.stage.ScenarioCalls {
		v.validateScenarioDependencies(call)
	}

	v.diagnostics.addAll(validateHTTPAuthoring(v.stage))
	v.diagnostics.addAll(validateScenarioCallExportAliases(v.stage.ScenarioCalls))
	v.diagnostics.addAll(validateScenarioCallBindingRefs(v.stage.ScenarioCalls, v.callExports))

	if hasScenarioDependencyCycle(v.stage) {
		v.diagnostics.add(v.stage.Path, "scenario_dependency_cycle", "scenario call dependency graph must be acyclic")
	}
}

func (v *structureValidator) validateScenario(scenario scenarioPlan) {
	actIDs := make(map[string]struct{}, len(scenario.Acts))

	if scenario.ID == "" {
		v.diagnostics.add(scenario.Path, "missing_scenario_id", "scenario id is required")
	} else if err := validateScenarioAddress(scenario.ID); err != nil {
		v.diagnostics.add(
			scenario.Path,
			"invalid_scenario_id",
			fmt.Sprintf("scenario id %q is invalid: %v", scenario.ID, err),
		)
	}

	if len(scenario.Acts) == 0 {
		v.diagnostics.add(
			scenario.Path,
			"missing_scenario_acts",
			fmt.Sprintf("scenario %q must define at least one act", scenario.ID),
		)
	}

	v.diagnostics.addAll(validateValueContracts(scenario.Path, scenario.Inputs))
	v.diagnostics.addAll(duplicateActIDDiagnostics(scenario.Acts))

	for i := range scenario.Acts {
		act := &scenario.Acts[i]
		v.validateAct(act)
		actIDs[act.ID] = struct{}{}
	}

	for i := range scenario.Acts {
		act := &scenario.Acts[i]
		for _, transition := range act.Transitions {
			if transition.To == "" {
				continue
			}
			if _, ok := actIDs[transition.To]; ok {
				continue
			}

			v.diagnostics.add(
				act.Path,
				"missing_transition_target",
				fmt.Sprintf("transition %q references unknown act %q", transition.On, transition.To),
			)
		}
	}

	if hasActTransitionCycle(scenario) {
		v.diagnostics.add(scenario.Path, "act_transition_cycle", "act transition graph must be acyclic in v1")
	}

	v.diagnostics.addAll(validateScenarioLocalBindingRefs(scenario, analyzeScenarioScope(scenario)))
}

func (v *structureValidator) validateScenarioCall(call scenarioCallPlan) {
	v.diagnostics.addAll(validateBindings(call.Path, call.Bindings))
	v.diagnostics.addAll(validateScenarioCallExports(call.Path, call.Exports))
	v.callExports[call.ID] = exportAliasSet(call.Exports)

	if call.ID == "" {
		v.diagnostics.add(call.Path, "missing_scenario_call_id", "scenario call id is required")
	}

	if call.ScenarioID == "" {
		v.diagnostics.add(
			call.Path,
			"missing_scenario_call_scenario_id",
			fmt.Sprintf("scenario call %q must reference a scenario", call.ID),
		)
	}

	v.callIDs[call.ID] = struct{}{}
	if call.ScenarioID == "" {
		return
	}

	if err := validateScenarioAddress(call.ScenarioID); err != nil {
		v.diagnostics.add(
			call.Path,
			"invalid_scenario_call_scenario_id",
			fmt.Sprintf("scenario call %q references invalid scenario address %q: %v", call.ID, call.ScenarioID, err),
		)
		return
	}

	address := scenarioAddress(call.ScenarioID)
	scenario, ok := v.scenarios.Resolve(address)
	if !ok {
		v.diagnostics.add(
			call.Path,
			"missing_scenario_ref",
			fmt.Sprintf("scenario call %q references unknown scenario %q", call.ID, call.ScenarioID),
		)
		return
	}

	if !v.access.AllowsDirectCall(address) {
		v.diagnostics.add(
			call.Path,
			"internal_scenario_not_accessible",
			fmt.Sprintf("scenario call %q cannot target internal-only scenario %q", call.ID, call.ScenarioID),
		)
		return
	}

	v.diagnostics.addAll(validateScenarioCallBindings(call, scenario.Inputs, nil, nil, nil))
}

func (v *structureValidator) validateScenarioDependencies(call scenarioCallPlan) {
	for _, dependency := range call.Dependencies {
		if !dependency.When.Valid() {
			v.diagnostics.add(
				dependency.Path,
				"invalid_dependency_predicate",
				fmt.Sprintf("dependency predicate %q is invalid", dependency.When),
			)
		}

		if _, ok := v.callIDs[dependency.CallID]; ok {
			continue
		}

		v.diagnostics.add(
			dependency.Path,
			"missing_dependency_ref",
			fmt.Sprintf("scenario call %q references unknown dependency %q", call.ID, dependency.CallID),
		)
	}
}

func (v *structureValidator) validateAct(act *actPlan) {
	actionPath := act.Path + "/action"

	if act.ID == "" {
		v.diagnostics.add(act.Path, "missing_act_id", "act id is required")
	}

	if act.Action.Use == "" {
		v.diagnostics.add(
			actionPath,
			"missing_action_use",
			fmt.Sprintf("act %q must define an action use ref", act.ID),
		)
	}

	v.diagnostics.addAll(validateBindings(actionPath, act.Action.With))

	for i := range act.Properties {
		v.validateProperty(&act.Properties[i])
	}

	if hasPropertyDependencyCycle(act.Properties) {
		v.diagnostics.add(act.Path, "property_dependency_cycle", "property dependency graph must be acyclic")
	}

	v.diagnostics.addAll(validateActExports(act.Path, act.Exports))
	v.validateLogs(act)
	v.validateExpectations(act)
	v.validateTransitionOutcomes(act)
	v.validateEventually(act)
	v.diagnostics.addAll(validateStateMutationEventually(act))
}

func (v *structureValidator) validateLogs(act *actPlan) {
	v.diagnostics.addAll(duplicateLogIDDiagnostics(act))
	v.validateLogCount(act)

	for i := range act.Logs {
		log := &act.Logs[i]
		path := log.Path

		if log.ID == "" {
			v.diagnostics.add(path, "missing_log_id", "log id is required")
		}

		if !log.Format.Valid() {
			v.diagnostics.add(
				path,
				"invalid_log_format",
				fmt.Sprintf("log %q format %q is invalid", log.ID, log.Format),
			)
		}

		if log.Capture != "" && log.Capture != CaptureOmit && log.Capture != CaptureSummary {
			v.diagnostics.add(
				path,
				"invalid_log_capture",
				fmt.Sprintf("log %q capture %q is invalid", log.ID, log.Capture),
			)
		}

		if log.Sensitivity != "" &&
			log.Sensitivity != SensitivityInternal &&
			log.Sensitivity != SensitivityPersonal &&
			log.Sensitivity != SensitivitySecret {
			v.diagnostics.add(
				path,
				"invalid_log_sensitivity",
				fmt.Sprintf("log %q sensitivity %q is invalid", log.ID, log.Sensitivity),
			)
		}

		hasValue := logValueConfigured(log.Value)
		hasMessage := log.Message != ""
		hasFields := len(log.Fields) != 0
		switch {
		case hasValue && (hasMessage || hasFields):
			v.diagnostics.add(path, "invalid_log_form", fmt.Sprintf("log %q must use either value or message with fields", log.ID))
		case hasFields && !hasMessage:
			v.diagnostics.add(path, "invalid_log_form", fmt.Sprintf("log %q fields require message", log.ID))
		case !hasValue && !hasMessage:
			v.diagnostics.add(path, "invalid_log_form", fmt.Sprintf("log %q must define value or message", log.ID))
		}

		if hasValue {
			v.diagnostics.addAll(validateLogValue(log.Value))
		}
		for key := range log.Fields {
			v.diagnostics.addAll(validateLogValue(log.Fields[key]))
		}
	}
}

func (v *structureValidator) validateLogCount(act *actPlan) {
	if len(act.Logs) <= DefaultScenarioLogRecordsPerAct {
		return
	}

	v.diagnostics.add(
		act.Path+"/logs",
		"too_many_logs",
		fmt.Sprintf("act %q declares %d logs; maximum is %d", act.ID, len(act.Logs), DefaultScenarioLogRecordsPerAct),
	)
}

func (v *structureValidator) validateProperty(property *propertyPlan) {
	propertyPath := property.Path

	if property.ID == "" {
		v.diagnostics.add(propertyPath, "missing_property_key", "property key is required")
	}

	if !property.Inventory.Present {
		v.diagnostics.add(
			propertyPath,
			"missing_property_inventory",
			fmt.Sprintf("property %q must define an inventory call", property.ID),
		)
		return
	}

	if property.Inventory.Use == "" {
		v.diagnostics.add(
			propertyPath+"/inventory",
			"missing_property_inventory_use",
			fmt.Sprintf("property %q inventory use is required", property.ID),
		)
	}

	v.diagnostics.addAll(validateBindings(propertyPath+"/inventory/with", property.Inventory.With))

	for i := range property.Decorators {
		if property.Decorators[i].Use != "" {
			continue
		}

		v.diagnostics.add(
			joinChildPath(propertyPath, "decorator", ""),
			"missing_property_decorator_use",
			fmt.Sprintf("property %q decorator use is required", property.ID),
		)
	}
}

func (v *structureValidator) validateExpectations(act *actPlan) {
	v.diagnostics.addAll(duplicateExpectationIDDiagnostics(act))
	propertyIDs := propertyIDSet(act.Properties)

	for i := range act.Expectations {
		expectation := &act.Expectations[i]
		expectationPath := joinChildPath(act.Path, "expectation", expectation.ID)

		if expectation.ID == "" {
			v.diagnostics.add(expectationPath, "missing_expectation_id", "expectation id is required")
		}

		switch {
		case !expectation.Subject.From.Valid():
			v.diagnostics.add(
				expectationPath,
				"invalid_expectation_subject_from",
				fmt.Sprintf("expectation %q subject from %q is invalid", expectation.ID, expectation.Subject.From),
			)
		case expectation.Subject.From == SubjectFromProperty:
			if expectation.Subject.Ref == "" {
				v.diagnostics.add(
					expectationPath,
					"missing_expectation_subject_ref",
					fmt.Sprintf("expectation %q must define a subject ref", expectation.ID),
				)
			}

			if expectation.Subject.Field != "" {
				v.diagnostics.add(
					expectationPath,
					"unexpected_expectation_subject_field",
					fmt.Sprintf("expectation %q subject field is only supported for current action outputs", expectation.ID),
				)
			}

			if err := validateRefName(expectation.Subject.Ref); err != nil {
				v.diagnostics.add(expectationPath, "invalid_expectation_subject_ref", err.Error())
			}

			if expectation.Subject.Ref != "" {
				if _, ok := propertyIDs[expectation.Subject.Ref]; !ok {
					v.diagnostics.add(
						expectationPath,
						"unknown_expectation_subject_ref",
						fmt.Sprintf("expectation %q subject ref %q is not declared by act %q", expectation.ID, expectation.Subject.Ref, act.ID),
					)
				}
			}
		default:
			if expectation.Subject.Field == "" {
				v.diagnostics.add(
					expectationPath,
					"missing_expectation_subject_field",
					fmt.Sprintf("expectation %q must define a subject field", expectation.ID),
				)
			}

			if err := validateFieldName(expectation.Subject.Field); err != nil {
				v.diagnostics.add(expectationPath, "invalid_expectation_subject_field", err.Error())
			}
		}

		v.diagnostics.addAll(validateExpectationSubjectSelector(expectationPath, expectation.ID, expectation.Subject.selectorPlan))

		if expectation.Assert.Ref == "" {
			v.diagnostics.add(
				expectationPath,
				"missing_expectation_assert_ref",
				fmt.Sprintf("expectation %q must define an assert ref", expectation.ID),
			)
		}

		v.diagnostics.addAll(validateBindings(expectationPath, expectation.Assert.Args))
	}
}

func validateExpectationSubjectSelector(path, expectationID string, selector selectorPlan) []Diagnostic {
	diagnostics := validateSelector(path, selector)
	for i := range diagnostics {
		switch diagnostics[i].Code {
		case "invalid_selector_decode":
			diagnostics[i].Code = "invalid_expectation_subject_decode"
			diagnostics[i].Summary = fmt.Sprintf(
				"expectation %q subject decode %q is invalid",
				expectationID,
				selector.Decode,
			)
		case "invalid_selector_path":
			diagnostics[i].Code = "invalid_expectation_subject_path"
			diagnostics[i].Summary = fmt.Sprintf(
				"expectation %q subject path is invalid: %v",
				expectationID,
				diagnostics[i].Summary,
			)
		default:
			diagnostics[i].Code = strings.Replace(diagnostics[i].Code, "invalid_selector_", "invalid_expectation_subject_", 1)
			diagnostics[i].Summary = fmt.Sprintf("expectation %q subject %s", expectationID, diagnostics[i].Summary)
		}
	}

	return diagnostics
}

func propertyIDSet(properties []propertyPlan) map[string]struct{} {
	if len(properties) == 0 {
		return nil
	}

	ids := make(map[string]struct{}, len(properties))
	for i := range properties {
		if properties[i].ID == "" {
			continue
		}

		ids[properties[i].ID] = struct{}{}
	}

	return ids
}

func (v *structureValidator) validateTransitionOutcomes(act *actPlan) {
	for _, transition := range act.Transitions {
		if transition.On.Valid() {
			continue
		}

		v.diagnostics.add(
			act.Path,
			"invalid_transition_outcome",
			fmt.Sprintf("transition outcome %q is invalid", transition.On),
		)
	}
}

func (v *structureValidator) validateEventually(act *actPlan) {
	if act.Eventually == nil {
		if act.Action.Repeatable {
			v.diagnostics.add(
				act.Path+"/action",
				"repeatable_without_eventually",
				fmt.Sprintf("act %q action.repeatable requires eventually", act.ID),
			)
		}
		return
	}

	if len(act.Expectations) == 0 {
		v.diagnostics.add(
			act.Path,
			"eventually_requires_expectations",
			fmt.Sprintf("act %q eventually requires at least one expectation", act.ID),
		)
	}

	if !act.Action.Repeatable {
		v.diagnostics.add(
			act.Path+"/action",
			"eventually_requires_repeatable_action",
			fmt.Sprintf("act %q eventually requires action.repeatable=true", act.ID),
		)
	}

	timeout, err := time.ParseDuration(act.Eventually.TimeoutText)
	if err != nil || timeout <= 0 {
		v.diagnostics.add(
			act.Path+"/eventually/timeout",
			"invalid_eventually_timeout",
			fmt.Sprintf("act %q eventually timeout must be a positive duration", act.ID),
		)
	}

	interval, err := time.ParseDuration(act.Eventually.IntervalText)
	if err != nil || interval <= 0 {
		v.diagnostics.add(
			act.Path+"/eventually/interval",
			"invalid_eventually_interval",
			fmt.Sprintf("act %q eventually interval must be a positive duration", act.ID),
		)
	}

	if timeout > 0 && interval > 0 && interval >= timeout {
		v.diagnostics.add(
			act.Path+"/eventually/interval",
			"invalid_eventually_interval",
			fmt.Sprintf("act %q eventually interval must be shorter than timeout", act.ID),
		)
	}
}

func (d *diagnosticSet) add(path, code, summary string) {
	d.items = append(d.items, Diagnostic{
		Code:     code,
		Path:     path,
		Severity: SeverityError,
		Summary:  summary,
	})
}

func (d *diagnosticSet) addAll(diagnostics []Diagnostic) {
	d.items = append(d.items, diagnostics...)
}

func validateStage(stage *stagePlan) []Diagnostic {
	return newStructureValidator(stage).Validate()
}
