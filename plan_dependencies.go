package theater

import "sort"

func sortedPropertyIDs(specs map[string]PropertySpec) []string {
	propertyIDs := make([]string, 0, len(specs))
	for propertyID := range specs {
		propertyIDs = append(propertyIDs, propertyID)
	}

	sort.Strings(propertyIDs)
	return propertyIDs
}

func orderPropertyPlans(properties []propertyPlan) []propertyPlan {
	if len(properties) < 2 {
		return properties
	}

	index := make(map[string]int, len(properties))
	dependents := make(map[string][]string, len(properties))
	inDegree := make(map[string]int, len(properties))
	for i := range properties {
		index[properties[i].ID] = i
		inDegree[properties[i].ID] = len(properties[i].Dependencies)
		for _, dependency := range properties[i].Dependencies {
			dependents[dependency] = append(dependents[dependency], properties[i].ID)
		}
	}

	ready := make([]string, 0, len(properties))
	for i := range properties {
		if inDegree[properties[i].ID] == 0 {
			ready = append(ready, properties[i].ID)
		}
	}
	sort.Strings(ready)

	ordered := make([]propertyPlan, 0, len(properties))
	for len(ready) != 0 {
		current := ready[0]
		ready = ready[1:]
		ordered = append(ordered, properties[index[current]])

		for _, dependent := range dependents[current] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				ready = append(ready, dependent)
				sort.Strings(ready)
			}
		}
	}

	if len(ordered) != len(properties) {
		return properties
	}

	return ordered
}

func propertyDependencies(bindings map[string]BindingSpec, names map[string]struct{}) []string {
	if len(bindings) == 0 {
		return nil
	}

	dependencies := make(map[string]struct{})
	for key := range bindings {
		collectPropertyDependencies(dependencies, bindings[key], names)
	}

	ordered := make([]string, 0, len(dependencies))
	for name := range dependencies {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	return ordered
}

func bindingPropertyDependencies(binding BindingSpec, names map[string]struct{}) []string {
	dependencies := make(map[string]struct{})
	collectPropertyDependencies(dependencies, binding, names)

	ordered := make([]string, 0, len(dependencies))
	for name := range dependencies {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	return ordered
}

func collectPropertyDependencies(
	dependencies map[string]struct{},
	binding BindingSpec,
	names map[string]struct{},
) {
	switch binding.Kind {
	case BindingKindRef:
		if binding.Ref == nil {
			return
		}

		if _, ok := names[binding.Ref.Name]; ok {
			dependencies[binding.Ref.Name] = struct{}{}
		}
		collectSelectorPropertyDependencies(dependencies, binding.Ref.Through, names)
	case BindingKindObject:
		for key := range binding.Object {
			collectPropertyDependencies(dependencies, binding.Object[key], names)
		}
	case BindingKindList:
		for i := range binding.List {
			collectPropertyDependencies(dependencies, binding.List[i], names)
		}
	case BindingKindString:
		for i := range binding.Parts {
			collectPropertyDependencies(dependencies, binding.Parts[i], names)
		}
	case BindingKindGenerate:
		for key := range binding.Args {
			collectPropertyDependencies(dependencies, binding.Args[key], names)
		}
	case BindingKindCoalesce:
		for i := range binding.Candidates {
			collectPropertyDependencies(dependencies, binding.Candidates[i], names)
		}
	}
}

func collectSelectorPropertyDependencies(
	dependencies map[string]struct{},
	through []ThroughStepSpec,
	names map[string]struct{},
) {
	for i := range through {
		if through[i].Pick == nil {
			continue
		}

		collectPropertyDependencies(dependencies, through[i].Pick.Equals, names)
	}
}
