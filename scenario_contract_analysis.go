package theater

import "reflect"

type scenarioContractAnalysis struct {
	actEntryContracts map[string]map[string]ValueContract
}

func analyzeScenarioContracts(
	scenario scenarioPlan,
	scopeAnalysis scenarioScopeAnalysis,
	catalog CatalogResolver,
	decorators DecoratorResolver,
) scenarioContractAnalysis {
	analysis := scenarioContractAnalysis{
		actEntryContracts: make(map[string]map[string]ValueContract),
	}
	if dependencyMissing(catalog) {
		return analysis
	}
	if len(scenario.Acts) == 0 || scenario.Acts[0].ID == "" {
		return analysis
	}

	actOrder, actsByID := uniqueScenarioActs(scenario.Acts)
	entryID := scenario.Acts[0].ID
	if _, ok := actsByID[entryID]; !ok {
		return analysis
	}

	successors, predecessors := buildScenarioScopeGraph(actOrder, actsByID)
	order := topologicalReachableActs(actOrder, scopeAnalysis.reachableActs, successors)
	if len(order) == 0 {
		return analysis
	}

	inputContracts := cloneContractMap(scenario.Inputs)
	for _, actID := range order {
		if actID == entryID {
			analysis.actEntryContracts[actID] = cloneContractMap(inputContracts)
			continue
		}

		incoming := reachablePredecessors(predecessors[actID], scopeAnalysis.reachableActs)
		if len(incoming) == 0 {
			continue
		}

		var contracts map[string]ValueContract
		for _, edge := range incoming {
			carried := carriedActContracts(
				analysis.actContracts(edge.fromID),
				actsByID[edge.fromID],
				edge.carriesExports,
				catalog,
				decorators,
			)
			if contracts == nil {
				contracts = carried
				continue
			}

			contracts = intersectContractMaps(contracts, carried)
		}

		analysis.actEntryContracts[actID] = contracts
	}

	return analysis
}

func (a scenarioContractAnalysis) actContracts(actID string) map[string]ValueContract {
	return cloneContractMap(a.actEntryContracts[actID])
}

func carriedActContracts(
	contracts map[string]ValueContract,
	act actPlan,
	carriesExports bool,
	catalog CatalogResolver,
	decorators DecoratorResolver,
) map[string]ValueContract {
	carried := cloneContractMap(contracts)
	if !carriesExports {
		return carried
	}

	for _, export := range act.Exports {
		alias := exportAlias(export)
		if alias == "" {
			continue
		}

		contract, known := selectedActExportContract(export, act, carried, catalog, decorators)
		if !known {
			continue
		}

		if carried == nil {
			carried = make(map[string]ValueContract)
		}
		carried[alias] = contract
	}

	return carried
}

func selectedActExportContract(
	export exportPlan,
	act actPlan,
	entryContracts map[string]ValueContract,
	catalog CatalogResolver,
	decorators DecoratorResolver,
) (ValueContract, bool) {
	propertyContracts := propertyValueContracts(&act, catalog)
	switch {
	case export.Ref != nil:
		refContracts := mergeContractMaps(propertyContracts, entryContracts)
		contract, ok := refContracts[export.Ref.Name]
		if !ok {
			return ValueContract{}, false
		}

		return selectedSequentialSelectorContract(
			contract,
			decorators,
			export.Ref.selectorPlan,
			export.selectorPlan,
		)
	case export.Field != "":
		runner, err := catalog.ResolveAction(act.Action.Use)
		if err != nil {
			return ValueContract{}, false
		}
		contract, ok := runner.Contract().Outputs[export.Field]
		if !ok {
			return ValueContract{}, false
		}

		return selectedSequentialSelectorContract(contract, decorators, export.selectorPlan)
	default:
		return ValueContract{}, false
	}
}

func selectedSequentialSelectorContract(
	contract ValueContract,
	decorators DecoratorResolver,
	selectors ...selectorPlan,
) (ValueContract, bool) {
	current := contract
	for _, selector := range selectors {
		selected, known, err := selectedSelectorContract(selector, current, decorators)
		if err != nil || !known {
			return ValueContract{}, false
		}
		current = selected
	}

	return current.Clone(), true
}

func cloneContractMap(contracts map[string]ValueContract) map[string]ValueContract {
	if len(contracts) == 0 {
		return nil
	}

	cloned := make(map[string]ValueContract, len(contracts))
	for name, contract := range contracts {
		cloned[name] = contract.Clone()
	}
	return cloned
}

func mergeContractMaps(primary, fallback map[string]ValueContract) map[string]ValueContract {
	merged := cloneContractMap(fallback)
	if len(primary) == 0 {
		return merged
	}
	if merged == nil {
		merged = make(map[string]ValueContract, len(primary))
	}
	for name, contract := range primary {
		merged[name] = contract.Clone()
	}
	return merged
}

func intersectContractMaps(left, right map[string]ValueContract) map[string]ValueContract {
	if len(left) == 0 || len(right) == 0 {
		return nil
	}

	intersection := make(map[string]ValueContract)
	for name, leftContract := range left {
		rightContract, ok := right[name]
		if !ok || !reflect.DeepEqual(leftContract, rightContract) {
			continue
		}

		intersection[name] = leftContract.Clone()
	}
	if len(intersection) == 0 {
		return nil
	}

	return intersection
}
