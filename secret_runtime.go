package theater

import "github.com/alex-poliushkin/theater/internal/runtimevalue"

func protectActionArgs(args Args, specs map[string]ValueContract) Args {
	protected := protectValues(Values(args), specs)
	if protected == nil {
		return nil
	}

	return Args(protected)
}

func protectActionOutputs(outputs Outputs, specs map[string]ValueContract) Outputs {
	protected := protectValues(Values(outputs), specs)
	if protected == nil {
		return nil
	}

	return Outputs(protected)
}

func protectInventoryArgs(args Args, specs []ArgSpec) Args {
	if len(args) == 0 {
		return nil
	}

	index := make(map[string]ValueContract, len(specs))
	for i := range specs {
		index[specs[i].Name] = specs[i].Accepts
	}

	protected := protectValues(Values(args), index)
	if protected == nil {
		return nil
	}

	return Args(protected)
}

func protectMatcherArgs(args Values, specs []MatcherArg) Values {
	if len(args) == 0 {
		return nil
	}

	index := make(map[string]ValueContract, len(specs))
	for i := range specs {
		index[specs[i].Name] = specs[i].Accepts
	}

	return protectValues(args, index)
}

func protectDecoratorArgs(args Values, specs []ParamSpec) Values {
	if len(args) == 0 {
		return nil
	}

	index := make(map[string]ValueContract, len(specs))
	for i := range specs {
		index[specs[i].Name] = specs[i].Accepts
	}

	return protectValues(args, index)
}

func protectValue(value any, spec ValueContract) any {
	protected := value
	wasSecret := runtimevalue.Wrap(value).IsSecret()

	switch resolvedValueKind(value) {
	case ValueKindObject:
		object, ok := runtimevalue.Object(value)
		if ok {
			cloned := make(map[string]any, len(object))
			for key, child := range object {
				member := child
				if memberSpec, ok := objectMemberContract(spec, key); ok {
					member = protectValue(child, memberSpec)
				}

				cloned[key] = member
			}

			protected = cloned
		}
	case ValueKindList:
		items, ok := runtimevalue.List(value)
		if ok {
			cloned := make([]any, len(items))
			for i := range items {
				cloned[i] = items[i]
				if spec.Elem != nil {
					cloned[i] = protectValue(items[i], *spec.Elem)
				}
			}

			protected = cloned
		}
	}

	if (spec.Sensitivity == SensitivitySecret || wasSecret) && protected != nil && !runtimevalue.Wrap(protected).IsSecret() {
		return NewSecret(protected)
	}

	return protected
}

func protectValues(values Values, specs map[string]ValueContract) Values {
	if len(values) == 0 {
		return nil
	}

	protected := make(Values, len(values))
	for key, value := range values {
		if spec, ok := specs[key]; ok {
			protected[key] = protectValue(value, spec)
			continue
		}

		protected[key] = value
	}

	return protected
}
