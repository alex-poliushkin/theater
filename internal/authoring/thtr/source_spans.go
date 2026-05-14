package thtr

import (
	"fmt"

	"github.com/alex-poliushkin/theater"
)

func applySourceMapToBindingSourceSpans(spec *theater.StageSpec, sourceMap *sourceMap) {
	if spec == nil || sourceMap == nil {
		return
	}

	codec := sourcePathCodec{}
	stagePath := codec.Join("stage", spec.ID)
	for scenarioIndex := range spec.Scenarios {
		scenario := &spec.Scenarios[scenarioIndex]
		scenarioPath := codec.JoinChild(stagePath, "scenario", scenario.ID)
		applyScenarioSourceMapBindingSpans(scenario, scenarioPath, sourceMap)
	}
	for callIndex := range spec.ScenarioCalls {
		call := &spec.ScenarioCalls[callIndex]
		callPath := codec.JoinChild(stagePath, "call", call.ID)
		applySourceMapBindingMapSpans(call.Bindings, callPath, sourceMap)
		for exportIndex := range call.Exports {
			export := &call.Exports[exportIndex]
			applySourceMapExportSpans(export, exportPath(callPath, export.As), sourceMap)
		}
	}
}

func applyScenarioSourceMapBindingSpans(scenario *theater.ScenarioSpec, scenarioPath string, sourceMap *sourceMap) {
	codec := sourcePathCodec{}
	for authName, authBinding := range scenario.AuthBindings {
		authBindingPath := codec.JoinChild(scenarioPath, "auth_bindings", authName)
		for slotName := range authBinding.Slots {
			slotBinding := authBinding.Slots[slotName]
			slotPath := codec.JoinChild(authBindingPath, "slot", slotName)
			applySourceMapBindingSpan(&slotBinding, slotPath, sourceMap)
			authBinding.Slots[slotName] = slotBinding
		}
		scenario.AuthBindings[authName] = authBinding
	}

	for actIndex := range scenario.Acts {
		act := &scenario.Acts[actIndex]
		actPath := codec.JoinChild(scenarioPath, "act", act.ID)
		applySourceMapBindingMapSpans(act.Action.With, actPath+"/action", sourceMap)
		for propertyID, property := range act.Properties {
			if property.Value != nil {
				propertyPath := codec.JoinChild(actPath, "property", propertyID)
				applySourceMapBindingSpan(property.Value, propertyPath+"/value", sourceMap)
			}
			if property.Inventory != nil {
				propertyPath := codec.JoinChild(actPath, "property", propertyID)
				applySourceMapBindingMapSpans(property.Inventory.With, propertyPath+"/inventory/with", sourceMap)
			}
		}
		for expectationIndex := range act.Expectations {
			expectation := &act.Expectations[expectationIndex]
			expectationPath := codec.JoinChild(actPath, "expectation", expectation.ID)
			applySourceMapThroughSpans(expectation.Subject.Through, expectationPath+"/subject", sourceMap)
			applySourceMapAssertSpans(&expectation.Assert, expectationPath+"/assert", sourceMap)
		}
		for logIndex := range act.Logs {
			log := &act.Logs[logIndex]
			logPath := codec.JoinChild(actPath, "log", log.ID)
			applySourceMapLogValueSpans(&log.Value, logPath+"/value", sourceMap)
		}
		for exportIndex := range act.Exports {
			export := &act.Exports[exportIndex]
			applySourceMapExportSpans(export, exportPath(actPath, export.As), sourceMap)
		}
	}
}

func applySourceMapLogValueSpans(value *theater.LogValueSpec, path string, sourceMap *sourceMap) {
	if value == nil {
		return
	}

	applySourceMapThroughSpans(value.Through, path, sourceMap)
	for key := range value.Object {
		childPath := bindingChildPath(path, key)
		child := value.Object[key]
		applySourceMapLogValueSpans(&child, childPath, sourceMap)
		value.Object[key] = child
	}
	for i := range value.List {
		childPath := fmt.Sprintf("%s[%d]", path, i)
		applySourceMapLogValueSpans(&value.List[i], childPath, sourceMap)
	}
}

func applySourceMapExportSpans(export *theater.ExportSpec, path string, sourceMap *sourceMap) {
	if export == nil {
		return
	}

	if export.Ref != nil {
		applySourceMapThroughSpans(export.Ref.Through, path, sourceMap)
	}
	applySourceMapThroughSpans(export.Through, path, sourceMap)
}

func applySourceMapAssertSpans(assert *theater.AssertSpec, assertPath string, sourceMap *sourceMap) {
	if assert == nil {
		return
	}

	applySourceMapBindingMapSpans(assert.Args, assertPath, sourceMap)
}

func applySourceMapBindingMapSpans(bindings map[string]theater.BindingSpec, parentPath string, sourceMap *sourceMap) {
	for key := range bindings {
		path := bindingPath(parentPath, key)
		binding := bindings[key]
		applySourceMapBindingSpan(&binding, path, sourceMap)
		bindings[key] = binding
	}
}

func applySourceMapBindingSpan(binding *theater.BindingSpec, path string, sourceMap *sourceMap) {
	if entry, ok := sourceMap.LookupExactSpecPath(path); ok {
		binding.SourceSpan = sourceRefFromSourceMapEntry(entry)
	}
	if binding.Ref != nil {
		applySourceMapThroughSpans(binding.Ref.Through, path, sourceMap)
	}

	switch binding.Kind {
	case theater.BindingKindObject:
		for key := range binding.Object {
			childPath := bindingChildPath(path, key)
			child := binding.Object[key]
			applySourceMapBindingSpan(&child, childPath, sourceMap)
			binding.Object[key] = child
		}
	case theater.BindingKindList:
		for i := range binding.List {
			childPath := fmt.Sprintf("%s[%d]", path, i)
			applySourceMapBindingSpan(&binding.List[i], childPath, sourceMap)
		}
	case theater.BindingKindString:
		for i := range binding.Parts {
			childPath := fmt.Sprintf("%s.parts[%d]", path, i)
			applySourceMapBindingSpan(&binding.Parts[i], childPath, sourceMap)
		}
	case theater.BindingKindGenerate:
		for key := range binding.Args {
			childPath := bindingChildPath(path, key)
			child := binding.Args[key]
			applySourceMapBindingSpan(&child, childPath, sourceMap)
			binding.Args[key] = child
		}
	case theater.BindingKindCoalesce:
		for i := range binding.Candidates {
			childPath := fmt.Sprintf("%s.candidates[%d]", path, i)
			applySourceMapBindingSpan(&binding.Candidates[i], childPath, sourceMap)
		}
	}
}

func applySourceMapThroughSpans(through []theater.ThroughStepSpec, parentPath string, sourceMap *sourceMap) {
	for i := range through {
		stepPath := fmt.Sprintf("%s/through[%d]", parentPath, i)
		if through[i].Pick == nil {
			continue
		}

		if through[i].Pick.Equals.Kind != "" {
			applySourceMapBindingSpan(&through[i].Pick.Equals, stepPath+"/pick/equals", sourceMap)
		}
		for j := range through[i].Pick.Where {
			clausePath := fmt.Sprintf("%s/pick/where[%d]", stepPath, j)
			applySourceMapAssertSpans(&through[i].Pick.Where[j].Assert, clausePath+"/assert", sourceMap)
		}
	}
}

func sourceRefFromSourceMapEntry(entry sourceMapEntry) *theater.SourceRef {
	if entry.Source.StartLine == 0 && entry.Source.StartColumn == 0 {
		return nil
	}

	return &theater.SourceRef{
		File:   entry.Source.File,
		Line:   entry.Source.StartLine,
		Column: entry.Source.StartColumn,
	}
}
