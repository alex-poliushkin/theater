package theater

import (
	"fmt"
	"sort"
)

const (
	stateRecordInventoryRef = "inventory.state.record"
	statePoolInventoryRef   = "inventory.state.pool"

	stateReadActionRef    = "action.state.read"
	stateUpdateActionRef  = "action.state.update"
	stateClaimActionRef   = "action.state.claim"
	stateRenewActionRef   = "action.state.renew"
	stateReleaseActionRef = "action.state.release"
	stateConsumeActionRef = "action.state.consume"
)

type stateHandleKind uint8

const (
	stateHandleUnknown stateHandleKind = iota
	stateHandleRecord
	stateHandlePool
	stateHandleClaim
)

type scenarioStateHandleAnalysis struct {
	actEntry map[string]map[string]stateHandleKind
	actCarry map[string]map[string]stateHandleKind
}

func validateStateRegistry(
	stage *stagePlan,
	resolver StateBackendResolver,
) (diagnostics []Diagnostic, descriptors map[string]StateDescriptor) {
	if stage == nil || stage.State == nil || len(stage.State.Backends) == 0 || dependencyMissing(resolver) {
		return nil, nil
	}

	diagnostics = make([]Diagnostic, 0)
	descriptors = make(map[string]StateDescriptor, len(stage.State.Backends))
	names := make([]string, 0, len(stage.State.Backends))
	for name := range stage.State.Backends {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		backend := stage.State.Backends[name]
		path := stateBackendPath(stage.Path, name)
		if err := validateRefName(name); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "invalid_state_backend_name",
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("state backend %q is invalid: %v", name, err),
			})
		}
		if backend.Use == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "missing_state_backend_use",
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("state backend %q must define use", name),
			})
			continue
		}

		def, err := resolver.ResolveStateBackend(backend.Use)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "unknown_state_backend_use",
				Path:     path + "/use",
				Severity: SeverityError,
				Summary:  err.Error(),
			})
			continue
		}

		resolved, paramDiagnostics := prepareStateBackendParams(path, Values(backend.With), def.Params)
		diagnostics = append(diagnostics, paramDiagnostics...)
		if len(paramDiagnostics) != 0 {
			continue
		}

		descriptor, err := def.Describe(resolved)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "invalid_state_backend_config",
				Path:     path,
				Severity: SeverityError,
				Summary:  err.Error(),
			})
			continue
		}

		descriptors[name] = descriptor
	}

	return diagnostics, descriptors
}

func validateStateActContracts(
	act *actPlan,
	entryKinds map[string]stateHandleKind,
	descriptors map[string]StateDescriptor,
) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	availableKinds := cloneStateHandleKinds(entryKinds)

	for i := range act.Properties {
		property := act.Properties[i]
		switch property.Inventory.Use {
		case stateRecordInventoryRef:
			diagnostics = append(
				diagnostics,
				validateStateInventoryBinding(property.Path+"/inventory/with", property.Inventory.With, "record", descriptors)...,
			)
			availableKinds[property.ID] = stateHandleRecord
		case statePoolInventoryRef:
			diagnostics = append(
				diagnostics,
				validateStateInventoryBinding(property.Path+"/inventory/with", property.Inventory.With, "pool", descriptors)...,
			)
			availableKinds[property.ID] = stateHandlePool
		}
	}

	requirements := stateActionArgRequirements(act.Action.Use)
	if len(requirements) == 0 {
		return diagnostics
	}

	for key, required := range requirements {
		binding, ok := act.Action.With[key]
		if !ok {
			continue
		}

		actual, known := bindingStateHandleKind(binding, availableKinds)
		if !known {
			continue
		}
		if actual == required {
			continue
		}

		diagnostics = append(diagnostics, Diagnostic{
			Code:     "incompatible_state_handle_ref",
			Path:     bindingPath(act.Path+"/action", key),
			Severity: SeverityError,
			Summary:  fmt.Sprintf("state action arg %q requires %s, got %s", key, required, actual),
		})
	}

	return diagnostics
}

func analyzeScenarioStateHandles(scenario scenarioPlan, _ scenarioScopeAnalysis) scenarioStateHandleAnalysis {
	analysis := scenarioStateHandleAnalysis{
		actEntry: make(map[string]map[string]stateHandleKind),
		actCarry: make(map[string]map[string]stateHandleKind),
	}

	if len(scenario.Acts) == 0 || scenario.Acts[0].ID == "" {
		return analysis
	}

	actOrder, actsByID := uniqueScenarioActs(scenario.Acts)
	entryID := scenario.Acts[0].ID
	successors, predecessors := buildScenarioScopeGraph(actOrder, actsByID)
	reachable := reachableActIDs(entryID, successors)
	order := topologicalReachableActs(actOrder, reachable, successors)

	for _, actID := range order {
		if actID == entryID {
			analysis.actEntry[actID] = nil
		} else {
			analysis.actEntry[actID] = intersectIncomingStateHandles(
				predecessors[actID],
				reachable,
				analysis.actEntry,
				analysis.actCarry,
			)
		}

		available := cloneStateHandleKinds(analysis.actEntry[actID])
		for i := range actsByID[actID].Properties {
			switch actsByID[actID].Properties[i].Inventory.Use {
			case stateRecordInventoryRef:
				available[actsByID[actID].Properties[i].ID] = stateHandleRecord
			case statePoolInventoryRef:
				available[actsByID[actID].Properties[i].ID] = stateHandlePool
			}
		}

		for field, kind := range stateActionOutputKinds(actsByID[actID].Action.Use) {
			available[field] = kind
		}

		carried := cloneStateHandleKinds(analysis.actEntry[actID])
		for i := range actsByID[actID].Exports {
			alias := exportAlias(actsByID[actID].Exports[i])
			kind, ok := exportStateHandleKind(actsByID[actID].Exports[i], available)
			if !ok || alias == "" {
				continue
			}
			carried[alias] = kind
		}
		analysis.actCarry[actID] = carried
	}

	return analysis
}

func intersectIncomingStateHandles(
	incoming []scenarioScopeEdge,
	reachable map[string]struct{},
	entry map[string]map[string]stateHandleKind,
	carry map[string]map[string]stateHandleKind,
) map[string]stateHandleKind {
	reachableIncoming := reachablePredecessors(incoming, reachable)
	var result map[string]stateHandleKind
	for _, edge := range reachableIncoming {
		carried := entry[edge.fromID]
		if edge.carriesExports {
			carried = carry[edge.fromID]
		}
		if result == nil {
			result = cloneStateHandleKinds(carried)
			continue
		}

		result = intersectStateHandleKinds(result, carried)
	}

	return result
}

func prepareStateBackendParams(path string, args Values, specs []ParamSpec) (resolved Values, diagnostics []Diagnostic) {
	resolved = cloneValues(args)
	if resolved == nil {
		resolved = Values{}
	}

	diagnostics = validateResolvedParams(
		path,
		args,
		resolved,
		specs,
		"state backend",
		"unexpected_state_backend_param",
		"missing_state_backend_param",
		"incompatible_state_backend_param",
		"invalid_state_backend_param",
	)
	return resolved, diagnostics
}

func validateStateInventoryBinding(
	path string,
	with map[string]bindingPlan,
	target string,
	descriptors map[string]StateDescriptor,
) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	if !bindingsStatic(with) {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "dynamic_state_binding",
			Path:     path,
			Severity: SeverityError,
			Summary:  "state binding args must be literal-only",
		})
		return diagnostics
	}

	backend, ok := staticBindingString(with["backend"])
	if !ok || backend == "" {
		return diagnostics
	}

	descriptor, ok := descriptors[backend]
	if !ok {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "unknown_state_binding_backend",
			Path:     bindingPath(path, "backend"),
			Severity: SeverityError,
			Summary:  fmt.Sprintf("state backend %q is not declared in stage state.backends", backend),
		})
		return diagnostics
	}

	nameField := target
	if _, ok := staticBindingString(with[nameField]); !ok {
		return diagnostics
	}

	if requiredTierText, ok := staticBindingString(with["min_guarantee"]); ok && requiredTierText != "" {
		required := StateGuaranteeTier(requiredTierText)
		if !required.Valid() {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "invalid_state_min_guarantee",
				Path:     bindingPath(path, "min_guarantee"),
				Severity: SeverityError,
				Summary:  fmt.Sprintf("state min_guarantee %q is invalid", requiredTierText),
			})
		} else if !descriptor.Guarantee.Supports(required) {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "insufficient_state_backend_guarantee",
				Path:     bindingPath(path, "min_guarantee"),
				Severity: SeverityError,
				Summary:  fmt.Sprintf("state backend %q guarantee %q does not satisfy required %q", backend, descriptor.Guarantee, required),
			})
		}
	}

	return diagnostics
}

func stateActionArgRequirements(use string) map[string]stateHandleKind {
	switch use {
	case stateReadActionRef, stateUpdateActionRef:
		return map[string]stateHandleKind{"record": stateHandleRecord}
	case stateClaimActionRef:
		return map[string]stateHandleKind{"pool": stateHandlePool}
	case stateRenewActionRef, stateReleaseActionRef, stateConsumeActionRef:
		return map[string]stateHandleKind{"claim": stateHandleClaim}
	default:
		return nil
	}
}

func stateActionOutputKinds(use string) map[string]stateHandleKind {
	switch use {
	case stateClaimActionRef, stateRenewActionRef:
		return map[string]stateHandleKind{"claim": stateHandleClaim}
	default:
		return nil
	}
}

func exportStateHandleKind(export exportPlan, available map[string]stateHandleKind) (stateHandleKind, bool) {
	if export.Ref != nil && selectorIdentity(export.Ref.selectorPlan) {
		kind, ok := available[export.Ref.Name]
		return kind, ok
	}

	if export.Field != "" && selectorIdentity(export.selectorPlan) {
		kind, ok := available[export.Field]
		return kind, ok
	}

	return stateHandleUnknown, false
}

func selectorIdentity(selector selectorPlan) bool {
	return selector.Decode == "" && selector.Path.IsRoot() && len(selector.Through) == 0
}

func bindingStateHandleKind(binding bindingPlan, available map[string]stateHandleKind) (stateHandleKind, bool) {
	switch binding.Kind {
	case BindingKindLiteral:
		switch binding.Value.(type) {
		case StateRecordHandle:
			return stateHandleRecord, true
		case StatePoolHandle:
			return stateHandlePool, true
		case StateClaimHandle:
			return stateHandleClaim, true
		default:
			return stateHandleUnknown, false
		}
	case BindingKindRef:
		if binding.Ref == nil || !selectorIdentity(binding.Ref.selectorPlan) {
			return stateHandleUnknown, false
		}
		kind, ok := available[binding.Ref.Name]
		return kind, ok
	default:
		return stateHandleUnknown, false
	}
}

func staticBindingString(binding bindingPlan) (string, bool) {
	if binding.Kind != BindingKindLiteral {
		return "", false
	}

	value, ok := binding.Value.(string)
	return value, ok
}

func cloneStateHandleKinds(source map[string]stateHandleKind) map[string]stateHandleKind {
	if len(source) == 0 {
		return map[string]stateHandleKind{}
	}

	cloned := make(map[string]stateHandleKind, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func validateStateMutationEventually(act *actPlan) []Diagnostic {
	if act == nil || act.Eventually == nil {
		return nil
	}

	switch act.Action.Use {
	case stateUpdateActionRef, stateClaimActionRef, stateRenewActionRef, stateReleaseActionRef, stateConsumeActionRef:
	default:
		return nil
	}

	return []Diagnostic{{
		Code:     "state_mutation_inside_eventually",
		Path:     act.Path,
		Severity: SeverityError,
		Summary:  fmt.Sprintf("act %q eventually must not use mutating state action %q", act.ID, act.Action.Use),
	}}
}

func intersectStateHandleKinds(
	left map[string]stateHandleKind,
	right map[string]stateHandleKind,
) map[string]stateHandleKind {
	intersection := make(map[string]stateHandleKind)
	for key, leftKind := range left {
		rightKind, ok := right[key]
		if !ok || rightKind != leftKind {
			continue
		}
		intersection[key] = leftKind
	}
	return intersection
}

func stateBackendPath(stagePath, name string) string {
	return stagePath + "/state/" + runtimePathCodec{}.Join("backend", name)
}

func (k stateHandleKind) String() string {
	switch k {
	case stateHandleRecord:
		return "record handle"
	case stateHandlePool:
		return "pool handle"
	case stateHandleClaim:
		return "claim handle"
	default:
		return "unknown handle"
	}
}
