package theater

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/alex-poliushkin/theater/internal/pluginredact"
	"github.com/alex-poliushkin/theater/internal/pluginregistry"
	"github.com/alex-poliushkin/theater/internal/secretvalue"

	"github.com/google/jsonschema-go/jsonschema"
)

const (
	jsonSchemaTypeArray   = "array"
	jsonSchemaTypeBoolean = "boolean"
	jsonSchemaTypeNull    = "null"
	jsonSchemaTypeObject  = "object"
	jsonSchemaTypeString  = "string"
)

func pluginActionContract(capability pluginregistry.LoadedCapability) (ActionContract, error) {
	if !schemaAllowsObject(capability.PropertySchema) {
		return ActionContract{}, errors.New("plugin action property_schema must describe an object")
	}

	contract := ActionContract{
		Inputs:  make(map[string]ValueContract, len(capability.PropertySchema.Properties)),
		Outputs: map[string]ValueContract{},
	}

	for name, shape := range capability.PropertySchema.Properties {
		contract.Inputs[name] = schemaShapeToValueContract(shape, capability.PropertySchema.Required[name])
	}
	applySensitiveValueContractPaths(contract.Inputs, capability.Manifest.Annotations.SensitiveInputPaths)

	if !capability.ResultSchema.Any {
		if !schemaAllowsObject(capability.ResultSchema) {
			return ActionContract{}, errors.New("plugin action result_schema must describe an object")
		}

		contract.Outputs = make(map[string]ValueContract, len(capability.ResultSchema.Properties))
		for name, shape := range capability.ResultSchema.Properties {
			contract.Outputs[name] = schemaShapeToValueContract(shape, capability.ResultSchema.Required[name])
		}
	}
	applySensitiveValueContractPaths(contract.Outputs, capability.Manifest.Annotations.SensitiveOutputPaths)

	return contract, nil
}

func pluginInventoryContract(capability pluginregistry.LoadedCapability) (InventoryContract, error) {
	if !schemaAllowsObject(capability.PropertySchema) {
		return InventoryContract{}, errors.New("plugin inventory property_schema must describe an object")
	}

	keys := make([]string, 0, len(capability.PropertySchema.Properties))
	for name := range capability.PropertySchema.Properties {
		keys = append(keys, name)
	}
	sort.Strings(keys)

	contract := InventoryContract{
		Summary:  capability.Manifest.Summary,
		Args:     make([]ArgSpec, 0, len(keys)),
		Produces: schemaShapeToValueContract(capability.ResultSchema, false),
	}
	for _, name := range keys {
		shape := capability.PropertySchema.Properties[name]
		contract.Args = append(contract.Args, ArgSpec{
			Name:        name,
			Accepts:     schemaShapeToValueContract(shape, capability.PropertySchema.Required[name]),
			Required:    capability.PropertySchema.Required[name],
			Description: shape.Description,
		})
	}
	applySensitiveValueContract(&contract.Produces, capability.Manifest.Annotations.SensitiveOutputPaths)

	return contract, nil
}

func pluginParamSpecs(capability pluginregistry.LoadedCapability) ([]ParamSpec, error) {
	if !schemaAllowsObject(capability.PropertySchema) {
		return nil, errors.New("plugin property_schema must describe an object")
	}

	keys := make([]string, 0, len(capability.PropertySchema.Properties))
	for name := range capability.PropertySchema.Properties {
		keys = append(keys, name)
	}
	sort.Strings(keys)

	params := make([]ParamSpec, 0, len(keys))
	for _, name := range keys {
		shape := capability.PropertySchema.Properties[name]
		params = append(params, ParamSpec{
			Name:        name,
			Accepts:     schemaShapeToValueContract(shape, capability.PropertySchema.Required[name]),
			Required:    capability.PropertySchema.Required[name],
			Description: shape.Description,
		})
	}

	return params, nil
}

func pluginTransformContract(capability pluginregistry.LoadedCapability) (DecoratorContract, error) {
	if capability.Manifest.Annotations.Transform == nil {
		return DecoratorContract{}, errors.New("transform metadata is required")
	}

	params, err := pluginParamSpecs(capability)
	if err != nil {
		return DecoratorContract{}, err
	}

	contract := DecoratorContract{
		Accepts:  capability.Manifest.Annotations.Transform.Accepts.Clone(),
		Produces: capability.Manifest.Annotations.Transform.Produces.Clone(),
		Params:   params,
		Summary:  capability.Manifest.Summary,
	}
	applySensitiveValueContract(&contract.Produces, capability.Manifest.Annotations.SensitiveOutputPaths)

	return contract, nil
}

func pluginMatcherArgs(capability pluginregistry.LoadedCapability) ([]MatcherArg, error) {
	if !schemaAllowsObject(capability.PropertySchema) {
		return nil, errors.New("plugin matcher property_schema must describe an object")
	}

	keys := make([]string, 0, len(capability.PropertySchema.Properties))
	for name := range capability.PropertySchema.Properties {
		keys = append(keys, name)
	}
	sort.Strings(keys)

	args := make([]MatcherArg, 0, len(keys))
	for _, name := range keys {
		shape := capability.PropertySchema.Properties[name]
		args = append(args, MatcherArg{
			Name:     name,
			Accepts:  schemaShapeToValueContract(shape, capability.PropertySchema.Required[name]),
			Required: capability.PropertySchema.Required[name],
			Summary:  shape.Description,
		})
	}

	return args, nil
}

func pluginStateDescriptor(capability pluginregistry.LoadedCapability) (StateDescriptor, error) {
	return capability.Manifest.Annotations.StateDescriptor()
}

func schemaShapeToValueContract(shape pluginregistry.SchemaShape, required bool) ValueContract {
	contract := ValueContract{
		Required:    required,
		Description: shape.Description,
	}

	if shape.Any || len(shape.Types) == 0 {
		contract.Kind = ValueKindAny
		return contract
	}

	kinds := schemaKinds(shape.Types)
	switch len(kinds) {
	case 0:
		contract.Kind = ValueKindAny
	case 1:
		for kind := range kinds {
			contract.Kind = kind
		}
	default:
		contract.Kinds = kinds
	}

	if kinds.Contains(ValueKindObject) {
		if len(shape.Properties) != 0 {
			contract.Fields = make(map[string]ValueContract, len(shape.Properties))
			for name, child := range shape.Properties {
				contract.Fields[name] = schemaShapeToValueContract(child, shape.Required[name])
			}
		}
		if shape.AdditionalProperties != nil {
			elem := schemaShapeToValueContract(*shape.AdditionalProperties, false)
			contract.Elem = &elem
		}
	}
	if kinds.Contains(ValueKindList) && shape.Items != nil {
		elem := schemaShapeToValueContract(*shape.Items, false)
		contract.Elem = &elem
	}

	return contract
}

func schemaAllowsObject(shape pluginregistry.SchemaShape) bool {
	return shape.Any || containsSchemaType(shape.Types, jsonSchemaTypeObject)
}

func schemaKinds(types []string) ValueKindSet {
	if len(types) == 0 {
		return NewValueKindSet(ValueKindAny)
	}

	kinds := make([]ValueKind, 0, len(types))
	for _, typ := range types {
		switch typ {
		case jsonSchemaTypeString:
			kinds = append(kinds, ValueKindString)
		case "integer", "number":
			kinds = append(kinds, ValueKindNumber)
		case jsonSchemaTypeBoolean:
			kinds = append(kinds, ValueKindBool)
		case jsonSchemaTypeObject:
			kinds = append(kinds, ValueKindObject)
		case jsonSchemaTypeArray:
			kinds = append(kinds, ValueKindList)
		case jsonSchemaTypeNull:
			kinds = append(kinds, ValueKindNull)
		}
	}

	return NewValueKindSet(kinds...)
}

func containsSchemaType(types []string, target string) bool {
	for _, typ := range types {
		if typ == target {
			return true
		}
	}

	return false
}

func validateJSONCompatibleSchema(resolved *jsonschema.Resolved, value any) error {
	if resolved == nil {
		return nil
	}

	if err := resolved.Validate(value); err != nil {
		return fmt.Errorf("validate JSON schema: %w", err)
	}

	return nil
}

func jsonCompatibleValue(value any) (any, error) {
	switch typed := value.(type) {
	case secretvalue.Value:
		return jsonCompatibleValue(typed.Reveal())
	case nil, string, bool, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		raw, err := json.Marshal(typed)
		if err != nil {
			return nil, err
		}
		var cloned any
		if err := json.Unmarshal(raw, &cloned); err != nil {
			return nil, err
		}
		return cloned, nil
	case []byte:
		return nil, errors.New("bytes values must not cross the plugin boundary")
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, child := range typed {
			resolved, err := jsonCompatibleValue(child)
			if err != nil {
				return nil, fmt.Errorf("field %q: %w", key, err)
			}
			cloned[key] = resolved
		}
		return cloned, nil
	case []any:
		cloned := make([]any, len(typed))
		for i := range typed {
			resolved, err := jsonCompatibleValue(typed[i])
			if err != nil {
				return nil, fmt.Errorf("item %d: %w", i, err)
			}
			cloned[i] = resolved
		}
		return cloned, nil
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return nil, fmt.Errorf("value is not JSON-compatible: %w", err)
		}
		var cloned any
		if err := json.Unmarshal(raw, &cloned); err != nil {
			return nil, fmt.Errorf("decode JSON-compatible clone: %w", err)
		}
		return cloned, nil
	}
}

func protectJSONCompatibleValue(value any, pointers []string) (any, error) {
	protected, err := pluginredact.ProtectPointers(value, pointers)
	if err != nil {
		return nil, err
	}

	return protected, nil
}

func protectJSONCompatibleObject(value any, pointers []string) (map[string]any, error) {
	protected, err := protectJSONCompatibleValue(value, pointers)
	if err != nil {
		return nil, err
	}
	if object, ok := protected.(map[string]any); ok {
		return object, nil
	}
	if !containsRootJSONPointer(pointers) {
		return nil, fmt.Errorf("protected plugin value must be object, got %T", protected)
	}

	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("plugin value must be object, got %T", value)
	}
	protectedObject := make(map[string]any, len(object))
	for key, child := range object {
		protectedObject[key] = secretvalue.New(child)
	}
	return protectedObject, nil
}

func redactorForJSONPointers(value any, pointers []string) pluginredact.Redactor {
	values, err := pluginredact.StringsAtPointers(value, pointers)
	if err != nil {
		return pluginredact.Redactor{}
	}

	return pluginredact.FromStrings(values)
}

func redactorForPluginValueInput(properties map[string]any, value any, pointers []string) pluginredact.Redactor {
	if len(pointers) == 0 {
		return pluginredact.Redactor{}
	}

	propertyPointers := make([]string, 0, len(pointers))
	valuePointers := make([]string, 0, 1)
	for i := range pointers {
		if pointers[i] == "" {
			valuePointers = append(valuePointers, pointers[i])
			continue
		}
		propertyPointers = append(propertyPointers, pointers[i])
	}

	return redactorForJSONPointers(properties, propertyPointers).
		Merge(redactorForJSONPointers(value, valuePointers))
}

func containsRootJSONPointer(pointers []string) bool {
	for i := range pointers {
		if pointers[i] == "" {
			return true
		}
	}
	return false
}

func partialBindingsJSON(bindings map[string]bindingPlan) (properties map[string]any, dynamic []string, err error) {
	if len(bindings) == 0 {
		return nil, nil, nil
	}

	keys := make([]string, 0, len(bindings))
	for key := range bindings {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	properties = make(map[string]any, len(bindings))
	dynamic = make([]string, 0)
	for _, key := range keys {
		value, paths, static, err := bindingPartialJSON(bindings[key], "/"+escapeJSONPointerToken(key))
		if err != nil {
			return nil, nil, err
		}
		if static {
			properties[key] = value
		}
		dynamic = append(dynamic, paths...)
	}

	return properties, dynamic, nil
}

func bindingPartialJSON(binding bindingPlan, path string) (value any, dynamic []string, static bool, err error) {
	switch binding.Kind {
	case BindingKindLiteral:
		value, err = jsonCompatibleValue(binding.Value)
		return value, nil, err == nil, err
	case BindingKindRef, BindingKindGenerate:
		return nil, []string{path}, false, nil
	case BindingKindString:
		if !bindingStatic(binding) {
			return nil, []string{path}, false, nil
		}

		var builder strings.Builder
		for i := range binding.Parts {
			part, _, _, err := bindingPartialJSON(binding.Parts[i], path)
			if err != nil {
				return nil, nil, false, err
			}
			builder.WriteString(fmt.Sprint(part))
		}
		return builder.String(), nil, true, nil
	case BindingKindObject:
		value := make(map[string]any, len(binding.Object))
		dynamic := make([]string, 0)
		keys := make([]string, 0, len(binding.Object))
		for key := range binding.Object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			childPath := path + "/" + escapeJSONPointerToken(key)
			childValue, childDynamic, static, err := bindingPartialJSON(binding.Object[key], childPath)
			if err != nil {
				return nil, nil, false, err
			}
			if static {
				value[key] = childValue
			}
			dynamic = append(dynamic, childDynamic...)
		}
		return value, dynamic, true, nil
	case BindingKindList:
		value := make([]any, 0, len(binding.List))
		dynamic := make([]string, 0)
		static := true
		for i := range binding.List {
			childPath := path + "/" + strconv.Itoa(i)
			childValue, childDynamic, childStatic, err := bindingPartialJSON(binding.List[i], childPath)
			if err != nil {
				return nil, nil, false, err
			}
			if !childStatic {
				static = false
			}
			if childStatic {
				value = append(value, childValue)
			}
			dynamic = append(dynamic, childDynamic...)
		}
		if !static {
			return nil, append(dynamic, path), false, nil
		}
		return value, dynamic, true, nil
	default:
		return nil, []string{path}, false, nil
	}
}

func escapeJSONPointerToken(value string) string {
	value = strings.ReplaceAll(value, "~", "~0")
	value = strings.ReplaceAll(value, "/", "~1")
	return value
}

func applySensitiveValueContractPaths(fields map[string]ValueContract, pointers []string) {
	rootSensitive := containsRootJSONPointer(pointers)
	for name := range fields {
		contract := fields[name]
		childPointers := onlyChildPointers(pointers, name)
		if rootSensitive {
			childPointers = append(childPointers, "")
		}
		applySensitiveValueContract(&contract, childPointers)
		fields[name] = contract
	}
}

func applySensitiveValueContract(contract *ValueContract, pointers []string) {
	if contract == nil || len(pointers) == 0 {
		return
	}

	for i := range pointers {
		if pointers[i] == "" {
			contract.Sensitivity = SensitivitySecret
			if contract.Capture == "" {
				contract.Capture = CaptureSummary
			}
			return
		}
	}

	if len(contract.Fields) != 0 {
		for name := range contract.Fields {
			child := contract.Fields[name]
			applySensitiveValueContract(&child, onlyChildPointers(pointers, name))
			contract.Fields[name] = child
		}
	}
	if contract.Elem != nil {
		child := *contract.Elem
		applySensitiveValueContract(&child, childPointersForElem(pointers))
		contract.Elem = &child
	}
}

func onlyChildPointers(pointers []string, name string) []string {
	if len(pointers) == 0 {
		return nil
	}

	escaped := "/" + escapeJSONPointerToken(name)
	children := make([]string, 0)
	for i := range pointers {
		pointer := pointers[i]
		if pointer == escaped {
			children = append(children, "")
			continue
		}
		if strings.HasPrefix(pointer, escaped+"/") {
			children = append(children, strings.TrimPrefix(pointer, escaped))
		}
	}

	return children
}

func childPointersForElem(pointers []string) []string {
	if len(pointers) == 0 {
		return nil
	}

	children := make([]string, 0)
	for i := range pointers {
		pointer := pointers[i]
		if !strings.HasPrefix(pointer, "/") {
			continue
		}
		token, rest := splitFirstPointerToken(pointer)
		if token == "" {
			continue
		}
		if _, err := strconv.Atoi(token); err == nil {
			children = append(children, rest)
		}
	}

	return children
}

func splitFirstPointerToken(pointer string) (token, rest string) {
	trimmed := strings.TrimPrefix(pointer, "/")
	if trimmed == "" {
		return "", ""
	}
	parts := strings.SplitN(trimmed, "/", 2)
	token = strings.ReplaceAll(strings.ReplaceAll(parts[0], "~1", "/"), "~0", "~")
	if len(parts) == 1 {
		return token, ""
	}

	return token, "/" + parts[1]
}
