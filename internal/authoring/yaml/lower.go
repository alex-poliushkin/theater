package yaml

import (
	"errors"
	"fmt"

	goyaml "gopkg.in/yaml.v3"

	"github.com/alex-poliushkin/theater"
)

const (
	yamlFieldArgs      = "args"
	yamlFieldField     = "field"
	yamlFieldGenerator = "generator"
	yamlFieldKind      = "kind"
	yamlFieldList      = "list"
	yamlFieldObject    = "object"
	yamlFieldParts     = "parts"
	yamlFieldPath      = "path"
	yamlFieldRef       = "ref"
	yamlFieldThrough   = "through"
	yamlFieldValue     = "value"
	yamlFieldDecode    = "decode"
)

func lowerStage(raw rawStageSpec, matchers theater.MatcherSugarResolver, sourceFile string) (theater.StageSpec, error) {
	spec := theater.StageSpec{
		ID:            raw.ID,
		Name:          raw.Name,
		HTTP:          raw.HTTP.Clone(),
		State:         raw.State.Clone(),
		Scenarios:     make([]theater.ScenarioSpec, 0, len(raw.Scenarios)),
		ScenarioCalls: make([]theater.ScenarioCallSpec, 0, len(raw.ScenarioCalls)),
		SourceSpan:    bindSourceRef(raw.Span, sourceFile),
	}

	for _, rawScenario := range raw.Scenarios {
		scenario := theater.ScenarioSpec{
			ID:         rawScenario.ID,
			Name:       rawScenario.Name,
			Inputs:     rawScenario.Inputs,
			Acts:       make([]theater.ActSpec, 0, len(rawScenario.Acts)),
			SourceSpan: bindSourceRef(rawScenario.Span, sourceFile),
		}

		for actIndex := range rawScenario.Acts {
			rawAct := rawScenario.Acts[actIndex]
			actionBindings, err := lowerBindingNodeMapWithSource(rawAct.Action.With, sourceFile)
			if err != nil {
				return theater.StageSpec{}, err
			}

			properties, err := lowerPropertyMap(rawAct.Properties, sourceFile)
			if err != nil {
				return theater.StageSpec{}, err
			}

			exports, err := lowerExports(rawAct.Exports, sourceFile)
			if err != nil {
				return theater.StageSpec{}, err
			}

			logs, err := lowerLogs(rawAct.Logs, sourceFile)
			if err != nil {
				return theater.StageSpec{}, err
			}

			act := theater.ActSpec{
				ID:         rawAct.ID,
				Name:       rawAct.Name,
				Eventually: cloneEventuallySpec(rawAct.Eventually),
				Properties: properties,
				Action: theater.ActionSpec{
					Use:        rawAct.Action.Use,
					With:       actionBindings,
					Repeatable: rawAct.Action.Repeatable,
					SourceSpan: bindSourceRef(rawAct.Action.Span, sourceFile),
				},
				CaptureAuth:  rawAct.CaptureAuth.Clone(),
				Logs:         logs,
				Expectations: make([]theater.ExpectationSpec, 0, len(rawAct.Expectations)),
				Exports:      exports,
				Transitions:  rawAct.Transitions,
				SourceSpan:   bindSourceRef(rawAct.Span, sourceFile),
			}

			for _, rawExpectation := range rawAct.Expectations {
				expectation, err := lowerExpectation(rawExpectation, matchers, sourceFile)
				if err != nil {
					return theater.StageSpec{}, err
				}

				act.Expectations = append(act.Expectations, expectation)
			}

			scenario.Acts = append(scenario.Acts, act)
		}

		spec.Scenarios = append(spec.Scenarios, scenario)
	}

	for i := range raw.ScenarioCalls {
		rawCall := raw.ScenarioCalls[i]
		bindings, err := lowerBindingNodeMapWithSource(rawCall.Bindings, sourceFile)
		if err != nil {
			return theater.StageSpec{}, err
		}

		exports, err := lowerExports(rawCall.Exports, sourceFile)
		if err != nil {
			return theater.StageSpec{}, err
		}

		spec.ScenarioCalls = append(spec.ScenarioCalls, theater.ScenarioCallSpec{
			ID:           rawCall.ID,
			Name:         rawCall.Name,
			ScenarioID:   rawCall.ScenarioID,
			Bindings:     bindings,
			Exports:      exports,
			Dependencies: rawCall.Dependencies,
			SourceSpan:   bindSourceRef(rawCall.Span, sourceFile),
		})
	}

	return spec, nil
}

func lowerExpectation(raw rawExpectationSpec, matchers theater.MatcherSugarResolver, sourceFile string) (theater.ExpectationSpec, error) {
	subject, err := lowerSubject(raw.Subject.Node, sourceFile)
	if err != nil {
		return theater.ExpectationSpec{}, err
	}

	assert, err := lowerAssert(raw.Assert.Node, matchers, sourceFile)
	if err != nil {
		return theater.ExpectationSpec{}, err
	}

	return theater.ExpectationSpec{
		ID:         raw.ID,
		Subject:    subject,
		Assert:     assert,
		SourceSpan: bindSourceRef(raw.Span, sourceFile),
	}, nil
}

func cloneEventuallySpec(spec *theater.EventuallySpec) *theater.EventuallySpec {
	if spec == nil {
		return nil
	}

	cloned := *spec
	return &cloned
}

func lowerSubject(node *goyaml.Node, sourceFile string) (theater.SubjectSpec, error) {
	if node == nil {
		return theater.SubjectSpec{}, errors.New("subject is required")
	}

	switch node.Kind {
	case goyaml.ScalarNode:
		var field string
		if err := node.Decode(&field); err != nil {
			return theater.SubjectSpec{}, err
		}

		return theater.SubjectSpec{Field: field}, nil
	case goyaml.MappingNode:
		return lowerSubjectObject(node, sourceFile)
	default:
		return theater.SubjectSpec{}, nodeError(node, "subject must be string or object")
	}
}

func lowerSubjectObject(node *goyaml.Node, sourceFile string) (theater.SubjectSpec, error) {
	subject := theater.SubjectSpec{}
	pairs, err := mappingPairs(node)
	if err != nil {
		return theater.SubjectSpec{}, err
	}

	for _, pair := range pairs {
		if err := lowerSubjectField(&subject, pair.key, pair.value, sourceFile); err != nil {
			return theater.SubjectSpec{}, err
		}
	}

	return subject, nil
}

func lowerSubjectField(subject *theater.SubjectSpec, key, value *goyaml.Node, sourceFile string) error {
	switch key.Value {
	case "from":
		return value.Decode(&subject.From)
	case yamlFieldRef:
		return value.Decode(&subject.Ref)
	case "field":
		return value.Decode(&subject.Field)
	case yamlFieldDecode:
		return value.Decode(&subject.Decode)
	case yamlFieldPath:
		path, err := lowerJSONPointerNode(value)
		if err != nil {
			return err
		}
		subject.Path = path
		return nil
	case yamlFieldThrough:
		through, err := lowerThroughNodeList(value, sourceFile)
		if err != nil {
			return err
		}
		subject.Through = through
		return nil
	default:
		return nodeError(key, fmt.Sprintf("field %s not found in type theater.SubjectSpec", key.Value))
	}
}

func lowerAssert(node *goyaml.Node, matchers theater.MatcherSugarResolver, sourceFile string) (theater.AssertSpec, error) {
	if node == nil {
		return theater.AssertSpec{}, errors.New("assert is required")
	}
	if node.Kind != goyaml.MappingNode {
		return theater.AssertSpec{}, nodeError(node, "assert must be object")
	}

	pairs, err := mappingPairs(node)
	if err != nil {
		return theater.AssertSpec{}, err
	}

	keys := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		keys = append(keys, pair.key.Value)
	}

	if containsKey(keys, yamlFieldRef) || containsKey(keys, yamlFieldArgs) {
		return lowerCanonicalAssert(node, sourceFile)
	}

	if len(keys) != 1 {
		return theater.AssertSpec{}, nodeError(node, "assert must define exactly one matcher")
	}

	key := pairs[0].key
	value := pairs[0].value

	if dependencyMissing(matchers) {
		return theater.AssertSpec{}, nodeError(key, fmt.Sprintf("assert matcher %q requires matcher sugar resolver", key.Value))
	}

	descriptor, err := matchers.ResolveSugarKey(key.Value)
	if err != nil {
		return theater.AssertSpec{}, nodeError(key, fmt.Sprintf("assert matcher %q is not supported", key.Value))
	}

	args, err := lowerSugarArgs(value, descriptor, sourceFile)
	if err != nil {
		return theater.AssertSpec{}, err
	}

	return theater.AssertSpec{
		Ref:  descriptor.Ref,
		Args: args,
	}, nil
}

func lowerCanonicalAssert(node *goyaml.Node, sourceFile string) (theater.AssertSpec, error) {
	assert := theater.AssertSpec{
		Args: map[string]theater.BindingSpec{},
	}

	pairs, err := mappingPairs(node)
	if err != nil {
		return theater.AssertSpec{}, err
	}

	for _, pair := range pairs {
		key := pair.key
		value := pair.value

		switch key.Value {
		case yamlFieldRef:
			if err := value.Decode(&assert.Ref); err != nil {
				return theater.AssertSpec{}, err
			}
		case yamlFieldArgs:
			args, err := decodeBindingMapWithSource(value, sourceFile)
			if err != nil {
				return theater.AssertSpec{}, err
			}

			assert.Args = args
		default:
			return theater.AssertSpec{}, nodeError(key, fmt.Sprintf("field %s not found in type theater.AssertSpec", key.Value))
		}
	}

	return assert, nil
}

func lowerSugarArgs(
	node *goyaml.Node,
	descriptor theater.MatcherDescriptor,
	sourceFile string,
) (map[string]theater.BindingSpec, error) {
	switch descriptor.Sugar.Form {
	case theater.SugarFormNone:
		if node.Kind != goyaml.ScalarNode || node.Tag != "!!null" {
			return nil, nodeError(node, "matcher does not accept sugar arguments")
		}

		return map[string]theater.BindingSpec{}, nil
	case theater.SugarFormUnary:
		if len(descriptor.Sugar.PositionalArgs) != 1 {
			return nil, errors.New("matcher unary sugar is invalid")
		}

		binding, err := decodeBindingSpecOrLiteralWithSource(node, sourceFile)
		if err != nil {
			return nil, err
		}

		return map[string]theater.BindingSpec{
			descriptor.Sugar.PositionalArgs[0]: binding,
		}, nil
	case theater.SugarFormFixedTuple:
		return lowerFixedTupleArgs(node, descriptor, sourceFile)
	default:
		return nil, nodeError(node, fmt.Sprintf("matcher sugar form %q is invalid", descriptor.Sugar.Form))
	}
}

func lowerFixedTupleArgs(
	node *goyaml.Node,
	descriptor theater.MatcherDescriptor,
	sourceFile string,
) (map[string]theater.BindingSpec, error) {
	switch node.Kind {
	case goyaml.SequenceNode:
		return lowerFixedTupleSequence(node, descriptor, sourceFile)
	case goyaml.MappingNode:
		return lowerFixedTupleMapping(node, descriptor, sourceFile)
	default:
		return nil, nodeError(node, descriptor.Sugar.Keys[0]+" matcher must be sequence or object")
	}
}

func lowerFixedTupleSequence(
	node *goyaml.Node,
	descriptor theater.MatcherDescriptor,
	sourceFile string,
) (map[string]theater.BindingSpec, error) {
	if len(node.Content) != len(descriptor.Sugar.PositionalArgs) {
		return nil, nodeError(
			node,
			fmt.Sprintf("%s matcher must provide %d values", descriptor.Sugar.Keys[0], len(descriptor.Sugar.PositionalArgs)),
		)
	}

	args := make(map[string]theater.BindingSpec, len(descriptor.Sugar.PositionalArgs))
	for i, name := range descriptor.Sugar.PositionalArgs {
		binding, err := decodeBindingSpecOrLiteralWithSource(node.Content[i], sourceFile)
		if err != nil {
			return nil, err
		}

		args[name] = binding
	}

	return args, nil
}

func lowerFixedTupleMapping(
	node *goyaml.Node,
	descriptor theater.MatcherDescriptor,
	sourceFile string,
) (map[string]theater.BindingSpec, error) {
	args := make(map[string]theater.BindingSpec, len(descriptor.Sugar.PositionalArgs))
	allowed := make(map[string]struct{}, len(descriptor.Sugar.PositionalArgs))
	for _, name := range descriptor.Sugar.PositionalArgs {
		allowed[name] = struct{}{}
	}

	pairs, err := mappingPairs(node)
	if err != nil {
		return nil, err
	}

	for _, pair := range pairs {
		key := pair.key
		value := pair.value
		if _, ok := allowed[key.Value]; !ok {
			return nil, nodeError(key, fmt.Sprintf("%s matcher field %q is not supported", descriptor.Sugar.Keys[0], key.Value))
		}

		binding, err := decodeBindingSpecOrLiteralWithSource(value, sourceFile)
		if err != nil {
			return nil, err
		}

		args[key.Value] = binding
	}

	for _, name := range descriptor.Sugar.PositionalArgs {
		if _, ok := args[name]; ok {
			continue
		}

		return nil, nodeError(node, fmt.Sprintf("%s matcher requires %q", descriptor.Sugar.Keys[0], name))
	}

	return args, nil
}

func decodeBindingMapWithSource(node *goyaml.Node, sourceFile string) (map[string]theater.BindingSpec, error) {
	if node.Kind != goyaml.MappingNode {
		return nil, nodeError(node, "args must be object")
	}

	pairs, err := mappingPairs(node)
	if err != nil {
		return nil, err
	}

	args := make(map[string]theater.BindingSpec, len(pairs))
	for _, pair := range pairs {
		key := pair.key
		value := pair.value

		binding, err := decodeBindingSpecOrLiteralWithSource(value, sourceFile)
		if err != nil {
			return nil, err
		}

		args[key.Value] = binding
	}

	return args, nil
}

func decodeBindingSpecOrLiteralWithSource(node *goyaml.Node, sourceFile string) (theater.BindingSpec, error) {
	looksLike, err := looksLikeBindingSpec(node)
	if err != nil {
		return theater.BindingSpec{}, err
	}

	if !looksLike {
		var value any
		if err := node.Decode(&value); err != nil {
			return theater.BindingSpec{}, err
		}

		return theater.BindingSpec{
			Kind:       theater.BindingKindLiteral,
			Value:      value,
			SourceSpan: bindSourceRef(rawSourceRef(node), sourceFile),
		}, nil
	}

	if generate, ok, err := lowerGenerateBindingNodeWithSource(node, sourceFile); err != nil {
		return theater.BindingSpec{}, err
	} else if ok {
		generate.SourceSpan = bindSourceRef(rawSourceRef(node), sourceFile)
		return generate, nil
	}

	binding := rawBindingSpec{}
	if err := node.Decode(&binding); err != nil {
		return theater.BindingSpec{}, err
	}

	lowered, err := lowerBindingWithSource(binding, sourceFile)
	if err != nil {
		return theater.BindingSpec{}, err
	}
	lowered.SourceSpan = bindSourceRef(rawSourceRef(node), sourceFile)
	return lowered, nil
}

func looksLikeBindingSpec(node *goyaml.Node) (bool, error) {
	if node.Kind != goyaml.MappingNode {
		return false, nil
	}

	pairs, err := mappingPairs(node)
	if err != nil {
		return false, err
	}

	for _, pair := range pairs {
		if pair.key.Value == yamlFieldArgs {
			return false, nil
		}
	}

	for _, pair := range pairs {
		key := pair.key.Value
		switch key {
		case yamlFieldKind, yamlFieldValue, yamlFieldRef, yamlFieldObject, yamlFieldList, yamlFieldParts, yamlFieldGenerator:
			return true, nil
		}
	}

	return false, nil
}

func containsKey(keys []string, target string) bool {
	for _, key := range keys {
		if key == target {
			return true
		}
	}

	return false
}

func bindSourceRef(source theater.SourceRef, file string) *theater.SourceRef {
	if source.Line == 0 && source.Column == 0 && file == "" {
		return nil
	}

	bound := source
	bound.File = file
	return &bound
}

func lowerPropertyMap(rawProperties map[string]rawPropertySpec, sourceFile string) (map[string]theater.PropertySpec, error) {
	properties := make(map[string]theater.PropertySpec, len(rawProperties))
	for key, property := range rawProperties {
		var (
			inventory *theater.InventoryCall
			err       error
		)
		if property.Inventory != nil {
			inventory, err = lowerInventoryCall(property.Inventory, sourceFile)
			if err != nil {
				return nil, err
			}
		}

		properties[key] = theater.PropertySpec{
			Inventory:  inventory,
			Decorators: property.Decorators,
		}
	}

	return properties, nil
}

func lowerInventoryCall(raw *rawInventoryCall, sourceFile string) (*theater.InventoryCall, error) {
	with, err := lowerBindingNodeMapWithSource(raw.With, sourceFile)
	if err != nil {
		return nil, err
	}

	return &theater.InventoryCall{
		Use:  raw.Use,
		With: with,
	}, nil
}

func lowerExports(rawExports []rawExportSpec, sourceFile string) ([]theater.ExportSpec, error) {
	if len(rawExports) == 0 {
		return []theater.ExportSpec{}, nil
	}

	exports := make([]theater.ExportSpec, 0, len(rawExports))
	for _, rawExport := range rawExports {
		export, err := lowerExport(rawExport, sourceFile)
		if err != nil {
			return nil, err
		}

		exports = append(exports, export)
	}

	return exports, nil
}

func lowerExport(raw rawExportSpec, sourceFile string) (theater.ExportSpec, error) {
	var (
		ref *theater.RefSpec
		err error
	)
	if raw.Ref != nil {
		ref, err = lowerRef(raw.Ref, sourceFile)
		if err != nil {
			return theater.ExportSpec{}, err
		}
	}

	path, err := lowerJSONPointer(raw.Path)
	if err != nil {
		return theater.ExportSpec{}, err
	}
	through, err := lowerThrough(raw.Through, sourceFile)
	if err != nil {
		return theater.ExportSpec{}, err
	}

	return theater.ExportSpec{
		As:      raw.As,
		Ref:     ref,
		Field:   raw.Field,
		Decode:  raw.Decode,
		Path:    path,
		Through: through,
	}, nil
}

func lowerLogs(rawLogs []rawLogSpec, sourceFile string) ([]theater.LogSpec, error) {
	if len(rawLogs) == 0 {
		return []theater.LogSpec{}, nil
	}

	logs := make([]theater.LogSpec, 0, len(rawLogs))
	for i := range rawLogs {
		log, err := lowerLog(rawLogs[i], sourceFile)
		if err != nil {
			return nil, err
		}

		logs = append(logs, log)
	}

	return logs, nil
}

func lowerLog(raw rawLogSpec, sourceFile string) (theater.LogSpec, error) {
	value, err := lowerLogValueNode(raw.Value, sourceFile)
	if err != nil {
		return theater.LogSpec{}, err
	}

	fields, err := lowerLogValueNodeMap(raw.Fields, sourceFile)
	if err != nil {
		return theater.LogSpec{}, err
	}

	return theater.LogSpec{
		ID:          raw.ID,
		Value:       value,
		Message:     raw.Message,
		Fields:      fields,
		Format:      raw.Format,
		Capture:     raw.Capture,
		Sensitivity: raw.Sensitivity,
		Required:    raw.Required,
		SourceSpan:  bindSourceRef(raw.Span, sourceFile),
	}, nil
}

func lowerLogValueNodeMap(rawValues map[string]rawLogValueNode, sourceFile string) (map[string]theater.LogValueSpec, error) {
	if len(rawValues) == 0 {
		return map[string]theater.LogValueSpec{}, nil
	}

	values := make(map[string]theater.LogValueSpec, len(rawValues))
	for key, rawValue := range rawValues {
		value, err := lowerLogValueNode(rawValue, sourceFile)
		if err != nil {
			return nil, err
		}

		values[key] = value
	}

	return values, nil
}

func lowerLogValueNode(raw rawLogValueNode, sourceFile string) (theater.LogValueSpec, error) {
	if raw.Node == nil {
		return theater.LogValueSpec{}, nil
	}

	if raw.Node.Kind != goyaml.MappingNode {
		return theater.LogValueSpec{}, nodeError(raw.Node, "log value must be object")
	}

	pairs, err := mappingPairs(raw.Node)
	if err != nil {
		return theater.LogValueSpec{}, err
	}

	value := theater.LogValueSpec{
		Object:     map[string]theater.LogValueSpec{},
		List:       []theater.LogValueSpec{},
		SourceSpan: bindSourceRef(rawSourceRef(raw.Node), sourceFile),
	}

	for _, pair := range pairs {
		if err := lowerLogValueField(&value, pair.key, pair.value, sourceFile); err != nil {
			return theater.LogValueSpec{}, err
		}
	}

	if len(value.Object) == 0 {
		value.Object = nil
	}
	if len(value.List) == 0 {
		value.List = nil
	}

	return value, nil
}

func lowerLogValueField(value *theater.LogValueSpec, key, rawValue *goyaml.Node, sourceFile string) error {
	switch key.Value {
	case yamlFieldField:
		return rawValue.Decode(&value.Field)
	case yamlFieldRef:
		return rawValue.Decode(&value.Ref)
	case yamlFieldObject:
		object, err := lowerLogValueNodeObject(rawValue, sourceFile)
		if err != nil {
			return err
		}
		value.Object = object
		return nil
	case yamlFieldList:
		list, err := lowerLogValueNodeList(rawValue, sourceFile)
		if err != nil {
			return err
		}
		value.List = list
		return nil
	case yamlFieldDecode:
		return rawValue.Decode(&value.Decode)
	case yamlFieldPath:
		path, err := lowerJSONPointerNode(rawValue)
		if err != nil {
			return err
		}
		value.Path = path
		return nil
	case yamlFieldThrough:
		through, err := lowerThroughNodeList(rawValue, sourceFile)
		if err != nil {
			return err
		}
		value.Through = through
		return nil
	default:
		return nodeError(key, fmt.Sprintf("field %s not found in type theater.LogValueSpec", key.Value))
	}
}

func lowerLogValueNodeObject(node *goyaml.Node, sourceFile string) (map[string]theater.LogValueSpec, error) {
	if node.Kind != goyaml.MappingNode {
		return nil, nodeError(node, "log value object must be object")
	}

	pairs, err := mappingPairs(node)
	if err != nil {
		return nil, err
	}

	object := make(map[string]theater.LogValueSpec, len(pairs))
	for _, pair := range pairs {
		value, err := lowerLogValueNode(rawLogValueNode{Node: pair.value}, sourceFile)
		if err != nil {
			return nil, err
		}
		object[pair.key.Value] = value
	}

	return object, nil
}

func lowerLogValueNodeList(node *goyaml.Node, sourceFile string) ([]theater.LogValueSpec, error) {
	if node.Kind != goyaml.SequenceNode {
		return nil, nodeError(node, "log value list must be sequence")
	}

	values := make([]theater.LogValueSpec, 0, len(node.Content))
	for i := range node.Content {
		value, err := lowerLogValueNode(rawLogValueNode{Node: node.Content[i]}, sourceFile)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}

	return values, nil
}

func lowerBindingNodeMapWithSource(
	rawBindings map[string]rawBindingNode,
	sourceFile string,
) (map[string]theater.BindingSpec, error) {
	if len(rawBindings) == 0 {
		return map[string]theater.BindingSpec{}, nil
	}

	bindings := make(map[string]theater.BindingSpec, len(rawBindings))
	for key, rawBinding := range rawBindings {
		binding, err := lowerBindingNodeWithSource(rawBinding, sourceFile)
		if err != nil {
			return nil, err
		}

		bindings[key] = binding
	}

	return bindings, nil
}

func lowerBindingNodeWithSource(raw rawBindingNode, sourceFile string) (theater.BindingSpec, error) {
	if raw.Node == nil {
		return theater.BindingSpec{}, errors.New("binding is required")
	}

	return decodeBindingSpecOrLiteralWithSource(raw.Node, sourceFile)
}

func lowerBindingWithSource(raw rawBindingSpec, sourceFile string) (theater.BindingSpec, error) {
	var (
		ref *theater.RefSpec
		err error
	)
	if raw.Ref != nil {
		ref, err = lowerRef(raw.Ref, sourceFile)
		if err != nil {
			return theater.BindingSpec{}, err
		}
	}

	object, err := lowerBindingNodeMapWithSource(raw.Object, sourceFile)
	if err != nil {
		return theater.BindingSpec{}, err
	}

	list, err := lowerBindingNodeListWithSource(raw.List, sourceFile)
	if err != nil {
		return theater.BindingSpec{}, err
	}
	parts, err := lowerBindingNodeListWithSource(raw.Parts, sourceFile)
	if err != nil {
		return theater.BindingSpec{}, err
	}

	return theater.BindingSpec{
		Kind:      raw.Kind,
		Value:     raw.Value,
		Ref:       ref,
		Object:    object,
		List:      list,
		Parts:     parts,
		Generator: raw.Generator,
	}, nil
}

func lowerGenerateBindingNodeWithSource(
	node *goyaml.Node,
	sourceFile string,
) (binding theater.BindingSpec, matched bool, err error) {
	if node == nil || node.Kind != goyaml.MappingNode {
		return theater.BindingSpec{}, false, nil
	}

	pairs, err := mappingPairs(node)
	if err != nil {
		return theater.BindingSpec{}, false, err
	}

	kind := ""
	for _, pair := range pairs {
		if pair.key.Value != yamlFieldKind {
			continue
		}

		if err := pair.value.Decode(&kind); err != nil {
			return theater.BindingSpec{}, false, err
		}
		break
	}

	if kind != string(theater.BindingKindGenerate) {
		return theater.BindingSpec{}, false, nil
	}

	binding = theater.BindingSpec{
		Kind:       theater.BindingKindGenerate,
		Args:       make(map[string]theater.BindingSpec),
		SourceSpan: bindSourceRef(rawSourceRef(node), sourceFile),
	}

	for _, pair := range pairs {
		switch pair.key.Value {
		case yamlFieldKind:
			continue
		case yamlFieldGenerator:
			if err := pair.value.Decode(&binding.Generator); err != nil {
				return theater.BindingSpec{}, false, err
			}
		default:
			arg, err := decodeBindingSpecOrLiteralWithSource(pair.value, sourceFile)
			if err != nil {
				return theater.BindingSpec{}, false, err
			}
			binding.Args[pair.key.Value] = arg
		}
	}

	if len(binding.Args) == 0 {
		binding.Args = nil
	}

	return binding, true, nil
}

func lowerBindingNodeListWithSource(rawBindings []rawBindingNode, sourceFile string) ([]theater.BindingSpec, error) {
	if len(rawBindings) == 0 {
		return []theater.BindingSpec{}, nil
	}

	bindings := make([]theater.BindingSpec, 0, len(rawBindings))
	for _, rawBinding := range rawBindings {
		binding, err := lowerBindingNodeWithSource(rawBinding, sourceFile)
		if err != nil {
			return nil, err
		}

		bindings = append(bindings, binding)
	}

	return bindings, nil
}

func lowerRef(raw *rawRefSpec, sourceFile string) (*theater.RefSpec, error) {
	path, err := lowerJSONPointer(raw.Path)
	if err != nil {
		return nil, err
	}
	through, err := lowerThrough(raw.Through, sourceFile)
	if err != nil {
		return nil, err
	}

	return &theater.RefSpec{
		Name:    raw.Name,
		Decode:  raw.Decode,
		Path:    path,
		Through: through,
	}, nil
}

func lowerThrough(raw []rawThroughStepSpec, sourceFile string) ([]theater.ThroughStepSpec, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	through := make([]theater.ThroughStepSpec, 0, len(raw))
	for i := range raw {
		step, err := lowerThroughStep(raw[i], sourceFile)
		if err != nil {
			return nil, err
		}

		through = append(through, step)
	}

	return through, nil
}

func lowerThroughNodeList(node *goyaml.Node, sourceFile string) ([]theater.ThroughStepSpec, error) {
	if node.Kind != goyaml.SequenceNode {
		return nil, nodeError(node, "through must be list")
	}

	raw := make([]rawThroughStepSpec, len(node.Content))
	for i := range node.Content {
		if err := node.Content[i].Decode(&raw[i]); err != nil {
			return nil, err
		}
	}

	return lowerThrough(raw, sourceFile)
}

func lowerThroughStep(raw rawThroughStepSpec, sourceFile string) (theater.ThroughStepSpec, error) {
	step := theater.ThroughStepSpec{}
	if raw.Path != "" {
		path, err := lowerJSONPointer(raw.Path)
		if err != nil {
			return theater.ThroughStepSpec{}, err
		}
		step.Path = path
	}
	if raw.Pick != nil {
		pick, err := lowerPickStep(raw.Pick, sourceFile)
		if err != nil {
			return theater.ThroughStepSpec{}, err
		}
		step.Pick = pick
	}
	if raw.Regexp != nil {
		step.Regexp = &theater.RegexpStepSpec{
			Pattern: raw.Regexp.Pattern,
			Group:   raw.Regexp.Group,
		}
	}
	if raw.Transform != nil {
		transform := *raw.Transform
		step.Transform = &transform
	}

	return step, nil
}

func lowerPickStep(raw *rawPickStepSpec, sourceFile string) (*theater.PickStepSpec, error) {
	if len(raw.Where) != 0 {
		if raw.At != "" || raw.Equals.Node != nil {
			return nil, errors.New("pick where cannot be combined with at or equals")
		}

		where, err := lowerPickWhereClauses(raw.Where, sourceFile)
		if err != nil {
			return nil, err
		}

		return &theater.PickStepSpec{Where: where}, nil
	}

	at, err := lowerJSONPointer(raw.At)
	if err != nil {
		return nil, err
	}
	equals, err := lowerBindingNodeWithSource(raw.Equals, sourceFile)
	if err != nil {
		return nil, err
	}

	return &theater.PickStepSpec{
		At:     at,
		Equals: equals,
	}, nil
}

func lowerPickWhereClauses(raw []rawPickWhereClauseSpec, sourceFile string) ([]theater.PickWhereClauseSpec, error) {
	clauses := make([]theater.PickWhereClauseSpec, 0, len(raw))
	for i := range raw {
		subject, err := lowerRelativeSubject(raw[i].Subject.Node)
		if err != nil {
			return nil, err
		}
		assert, err := lowerCanonicalAssert(raw[i].Assert.Node, sourceFile)
		if err != nil {
			return nil, err
		}
		clauses = append(clauses, theater.PickWhereClauseSpec{
			Subject: subject,
			Assert:  assert,
		})
	}

	return clauses, nil
}

func lowerRelativeSubject(node *goyaml.Node) (theater.RelativeSubjectSpec, error) {
	if node == nil {
		return theater.RelativeSubjectSpec{}, errors.New("subject is required")
	}
	if node.Kind != goyaml.MappingNode {
		return theater.RelativeSubjectSpec{}, nodeError(node, "subject must be object")
	}

	subject := theater.RelativeSubjectSpec{}
	pairs, err := mappingPairs(node)
	if err != nil {
		return theater.RelativeSubjectSpec{}, err
	}
	for _, pair := range pairs {
		switch pair.key.Value {
		case yamlFieldDecode:
			if err := pair.value.Decode(&subject.Decode); err != nil {
				return theater.RelativeSubjectSpec{}, err
			}
		case yamlFieldPath:
			path, err := lowerJSONPointerNode(pair.value)
			if err != nil {
				return theater.RelativeSubjectSpec{}, err
			}
			subject.Path = path
		default:
			return theater.RelativeSubjectSpec{}, nodeError(
				pair.key,
				fmt.Sprintf("field %s not found in type theater.RelativeSubjectSpec", pair.key.Value),
			)
		}
	}

	if subject.Decode == "" && subject.Path.IsRoot() {
		return theater.RelativeSubjectSpec{}, nodeError(node, "subject must declare decode or path")
	}

	return subject, nil
}

func lowerJSONPointerNode(node *goyaml.Node) (theater.JSONPointer, error) {
	var raw string
	if err := node.Decode(&raw); err != nil {
		return "", err
	}

	path, err := lowerJSONPointer(raw)
	if err != nil {
		return "", nodeError(node, err.Error())
	}

	return path, nil
}

func lowerJSONPointer(raw string) (theater.JSONPointer, error) {
	if raw == "" {
		return "", nil
	}

	return theater.ParseJSONPointer(raw)
}
