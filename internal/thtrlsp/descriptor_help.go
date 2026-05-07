package thtrlsp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alex-poliushkin/theater"
)

type descriptorParameter struct {
	name        string
	valueType   string
	required    bool
	description string
}

func hoverForDocument(
	text string,
	position lspPosition,
	descriptors []theater.CapabilityDescriptor,
) *lspHover {
	ref, start, end, ok := wordAtPosition(text, position)
	if !ok {
		return nil
	}

	descriptor, ok := descriptorByLabel(descriptors)[ref]
	if !ok {
		return nil
	}

	value := descriptorHelpMarkdown(descriptor)
	if value == "" {
		return nil
	}

	rng := rangeForOffsets(text, start, end)
	return &lspHover{
		Contents: lspMarkupContent{
			Kind:  "markdown",
			Value: value,
		},
		Range: &rng,
	}
}

func signatureHelpForDocument(
	text string,
	position lspPosition,
	descriptors []theater.CapabilityDescriptor,
) lspSignatureHelp {
	ref, callStart, ok := callRefBeforePosition(text, position)
	if !ok {
		return lspSignatureHelp{}
	}

	descriptor, ok := descriptorByLabel(descriptors)[ref]
	if !ok {
		return lspSignatureHelp{}
	}

	signature := descriptorSignature(descriptor)
	if signature == "" {
		return lspSignatureHelp{}
	}

	params := descriptorParameters(descriptor)
	parameters := make([]lspParameterInformation, 0, len(params))
	for i := range params {
		parameters = append(parameters, lspParameterInformation{Label: parameterLabel(params[i])})
	}
	activeParameter := activeCallParameter(text, callStart+1, offsetForPosition(text, position), len(params))

	return lspSignatureHelp{
		Signatures: []lspSignatureInformation{
			{
				Label:         signature,
				Documentation: descriptorHelpMarkdown(descriptor),
				Parameters:    parameters,
			},
		},
		ActiveParameter: activeParameter,
	}
}

func descriptorByLabel(descriptors []theater.CapabilityDescriptor) map[string]theater.CapabilityDescriptor {
	index := make(map[string]theater.CapabilityDescriptor, len(descriptors))
	for i := range descriptors {
		descriptor := descriptors[i]
		index[descriptorCompletionLabel(descriptor)] = descriptor
	}

	return index
}

func descriptorHelpMarkdown(descriptor theater.CapabilityDescriptor) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "`%s`\n\n", descriptorCompletionLabel(descriptor))
	builder.WriteString(descriptorCompletionDetail(descriptor))

	if signature := descriptorSignature(descriptor); signature != "" {
		fmt.Fprintf(&builder, "\n\nSignature: `%s`", signature)
	}
	if actual := descriptorActualType(descriptor); actual != "" {
		fmt.Fprintf(&builder, "\n\nActual: `%s`", actual)
	}
	if produced := descriptorProducedType(descriptor); produced != "" {
		fmt.Fprintf(&builder, "\n\nProduces: `%s`", produced)
	}
	for _, parameter := range descriptorParameters(descriptor) {
		fmt.Fprintf(&builder, "\n\n- `%s`: `%s`", parameter.name, parameter.valueType)
		if parameter.required {
			builder.WriteString(" required")
		}
		if parameter.description != "" {
			builder.WriteString(" - ")
			builder.WriteString(parameter.description)
		}
	}

	return builder.String()
}

func descriptorSignature(descriptor theater.CapabilityDescriptor) string {
	params := descriptorParameters(descriptor)
	labels := make([]string, 0, len(params))
	for i := range params {
		labels = append(labels, parameterLabel(params[i]))
	}

	return fmt.Sprintf("%s(%s)", descriptorCompletionLabel(descriptor), strings.Join(labels, ", "))
}

func parameterLabel(parameter descriptorParameter) string {
	name := parameter.name
	if !parameter.required {
		name += "?"
	}

	return name + ": " + parameter.valueType
}

func descriptorActualType(descriptor theater.CapabilityDescriptor) string {
	if descriptor.Matcher == nil {
		return ""
	}

	return valueContractType(descriptor.Matcher.Actual)
}

func descriptorProducedType(descriptor theater.CapabilityDescriptor) string {
	switch {
	case descriptor.Generator != nil:
		return valueContractType(descriptor.Generator.Produces)
	case descriptor.Inventory != nil:
		return valueContractType(descriptor.Inventory.Produces)
	case descriptor.Transform != nil:
		return valueContractType(descriptor.Transform.Produces)
	default:
		return ""
	}
}

func descriptorParameters(descriptor theater.CapabilityDescriptor) []descriptorParameter {
	switch {
	case descriptor.Action != nil:
		return actionParameters(descriptor.Action.Inputs)
	case descriptor.Generator != nil:
		return argParameters(descriptor.Generator.Args)
	case descriptor.Inventory != nil:
		return argParameters(descriptor.Inventory.Args)
	case descriptor.Transform != nil:
		return paramParameters(descriptor.Transform.Params)
	case descriptor.Matcher != nil:
		return matcherParameters(descriptor.Matcher.Args)
	case descriptor.ReportExporter != nil:
		return paramParameters(descriptor.ReportExporter.Params)
	case descriptor.StateBackend != nil:
		return paramParameters(descriptor.StateBackend.Params)
	default:
		return nil
	}
}

func actionParameters(inputs map[string]theater.ValueContract) []descriptorParameter {
	keys := make([]string, 0, len(inputs))
	for key := range inputs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	params := make([]descriptorParameter, 0, len(keys))
	for _, key := range keys {
		input := inputs[key]
		params = append(params, descriptorParameter{
			name:        key,
			valueType:   valueContractType(input),
			required:    input.Required,
			description: input.Description,
		})
	}

	return params
}

func argParameters(args []theater.ArgSpec) []descriptorParameter {
	params := make([]descriptorParameter, 0, len(args))
	for i := range args {
		arg := args[i]
		params = append(params, descriptorParameter{
			name:        arg.Name,
			valueType:   valueContractType(arg.Accepts),
			required:    arg.Required,
			description: arg.Description,
		})
	}

	return params
}

func paramParameters(args []theater.ParamSpec) []descriptorParameter {
	params := make([]descriptorParameter, 0, len(args))
	for i := range args {
		arg := args[i]
		params = append(params, descriptorParameter{
			name:        arg.Name,
			valueType:   valueContractType(arg.Accepts),
			required:    arg.Required,
			description: arg.Description,
		})
	}

	return params
}

func matcherParameters(args []theater.MatcherArg) []descriptorParameter {
	params := make([]descriptorParameter, 0, len(args))
	for i := range args {
		arg := args[i]
		params = append(params, descriptorParameter{
			name:        arg.Name,
			valueType:   valueContractType(arg.Accepts),
			required:    arg.Required,
			description: arg.Summary,
		})
	}

	return params
}

func valueContractType(contract theater.ValueContract) string {
	kinds := contract.KindsSet()
	if len(kinds) == 0 || kinds.Contains(theater.ValueKindAny) {
		return string(theater.ValueKindAny)
	}

	values := make([]string, 0, len(kinds))
	for kind := range kinds {
		switch {
		case kind == theater.ValueKindList && contract.Elem != nil:
			values = append(values, "list<"+valueContractType(*contract.Elem)+">")
		default:
			values = append(values, string(kind))
		}
	}
	sort.Strings(values)
	return strings.Join(values, "|")
}

func wordAtPosition(text string, position lspPosition) (word string, start, end int, ok bool) {
	offset := offsetForPosition(text, position)
	if offset > 0 && (offset == len(text) || !isReferenceChar(text[offset])) && isReferenceChar(text[offset-1]) {
		offset--
	}
	if offset < 0 || offset >= len(text) || !isReferenceChar(text[offset]) {
		return "", 0, 0, false
	}

	start = offset
	for start > 0 && isReferenceChar(text[start-1]) {
		start--
	}
	end = offset + 1
	for end < len(text) && isReferenceChar(text[end]) {
		end++
	}

	return text[start:end], start, end, true
}

func callRefBeforePosition(text string, position lspPosition) (ref string, openOffset int, ok bool) {
	offset := offsetForPosition(text, position)
	if offset > len(text) {
		offset = len(text)
	}

	open := unmatchedCallOpenBeforeOffset(text, offset)
	if open < 0 {
		return "", 0, false
	}

	end := open
	start := end
	for start > 0 && isReferenceChar(text[start-1]) {
		start--
	}
	if start == end {
		return "", 0, false
	}

	return text[start:end], open, true
}

func unmatchedCallOpenBeforeOffset(text string, offset int) int {
	openStack := make([]int, 0)
	inString := false
	escaped := false
	for index := 0; index < offset; index++ {
		ch := text[index]
		if inString {
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case ch == '"':
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '(':
			openStack = append(openStack, index)
		case ')':
			if len(openStack) > 0 {
				openStack = openStack[:len(openStack)-1]
			}
		}
	}

	if len(openStack) == 0 {
		return -1
	}
	return openStack[len(openStack)-1]
}

func activeCallParameter(text string, start, end, parameterCount int) int {
	if parameterCount <= 0 {
		return 0
	}
	if start < 0 {
		start = 0
	}
	if end > len(text) {
		end = len(text)
	}
	if start >= end {
		return 0
	}

	parameter := 0
	depth := 0
	inString := false
	escaped := false
	for index := start; index < end; index++ {
		ch := text[index]
		if inString {
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case ch == '"':
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '(', '{', '[':
			depth++
		case ')', '}', ']':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parameter++
			}
		}
	}
	if parameter >= parameterCount {
		return parameterCount - 1
	}

	return parameter
}

func isReferenceChar(ch byte) bool {
	return ch >= 'a' && ch <= 'z' ||
		ch >= 'A' && ch <= 'Z' ||
		ch >= '0' && ch <= '9' ||
		ch == '_' ||
		ch == '.' ||
		ch == '/' ||
		ch == '-'
}
