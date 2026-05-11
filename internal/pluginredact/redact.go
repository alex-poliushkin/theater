package pluginredact

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/alex-poliushkin/theater/internal/runtimevalue"
	"github.com/alex-poliushkin/theater/internal/secretvalue"
	"github.com/alex-poliushkin/theater/internal/selectvalue"
	specmodel "github.com/alex-poliushkin/theater/spec"
)

type Redactor struct {
	values []string
}

func New(sources ...any) Redactor {
	values := make([]string, 0)
	for i := range sources {
		values = append(values, collectStrings(sources[i])...)
	}

	return FromStrings(values)
}

func FromStrings(values []string) Redactor {
	if len(values) == 0 {
		return Redactor{}
	}

	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for i := range values {
		value := strings.TrimSpace(values[i])
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}

	sort.Slice(unique, func(i, j int) bool {
		if len(unique[i]) == len(unique[j]) {
			return unique[i] < unique[j]
		}
		return len(unique[i]) > len(unique[j])
	})

	return Redactor{values: unique}
}

func (r Redactor) Merge(other Redactor) Redactor {
	values := make([]string, 0, len(r.values)+len(other.values))
	values = append(values, r.values...)
	values = append(values, other.values...)
	return FromStrings(values)
}

func (r Redactor) RedactText(text string) string {
	if text == "" || len(r.values) == 0 {
		return text
	}

	redacted := text
	for i := range r.values {
		redacted = strings.ReplaceAll(redacted, r.values[i], secretvalue.RedactedText)
	}

	return redacted
}

func (r Redactor) RedactFields(fields map[string]string) map[string]string {
	if len(fields) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(fields))
	for key, value := range fields {
		cloned[key] = r.RedactText(value)
	}

	return cloned
}

func StringsAtPointers(value any, pointers []string) ([]string, error) {
	if len(pointers) == 0 || value == nil {
		return nil, nil
	}

	values := make([]string, 0)
	for i := range pointers {
		pointer, err := specmodel.ParseJSONPointer(pointers[i])
		if err != nil {
			return nil, err
		}

		resolved, err := selectvalue.Resolve(value, "", pointer)
		if err != nil {
			continue
		}

		values = append(values, collectStrings(resolved)...)
	}

	return values, nil
}

func ProtectPointers(value any, pointers []string) (any, error) {
	if len(pointers) == 0 || value == nil {
		return runtimevalue.Clone(value), nil
	}

	protected := runtimevalue.Clone(value)
	for i := range pointers {
		pointer, err := specmodel.ParseJSONPointer(pointers[i])
		if err != nil {
			return nil, err
		}
		protected = protectPointer(protected, pointer)
	}

	return protected, nil
}

func collectStrings(source any) []string {
	switch typed := source.(type) {
	case nil:
		return nil
	case string:
		if typed == "" {
			return nil
		}
		return []string{typed}
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, json.Number:
		return []string{fmt.Sprint(typed)}
	case secretvalue.Value:
		return collectStrings(typed.Reveal())
	case map[string]any:
		values := make([]string, 0)
		for key := range typed {
			values = append(values, collectStrings(typed[key])...)
		}
		return values
	case map[string]string:
		values := make([]string, 0, len(typed))
		for key := range typed {
			values = append(values, collectStrings(typed[key])...)
		}
		return values
	case []any:
		values := make([]string, 0)
		for i := range typed {
			values = append(values, collectStrings(typed[i])...)
		}
		return values
	default:
		return nil
	}
}

func protectPointer(value any, pointer specmodel.JSONPointer) any {
	if pointer.IsRoot() {
		return secretvalue.New(value)
	}

	tokens := pointerTokens(pointer)
	protected, _ := protectTokens(value, tokens)
	return protected
}

func protectTokens(value any, tokens []string) (any, bool) {
	if len(tokens) == 0 {
		return secretvalue.New(value), true
	}

	switch typed := value.(type) {
	case map[string]any:
		child, ok := typed[tokens[0]]
		if !ok {
			return value, false
		}

		protected, changed := protectTokens(child, tokens[1:])
		if !changed {
			return value, false
		}
		typed[tokens[0]] = protected
		return typed, true
	case []any:
		index, err := strconv.Atoi(tokens[0])
		if err != nil || index < 0 || index >= len(typed) {
			return value, false
		}

		protected, changed := protectTokens(typed[index], tokens[1:])
		if !changed {
			return value, false
		}
		typed[index] = protected
		return typed, true
	default:
		return value, false
	}
}

func pointerTokens(pointer specmodel.JSONPointer) []string {
	if pointer.IsRoot() {
		return nil
	}

	raw := strings.TrimPrefix(pointer.String(), "/")
	segments := strings.Split(raw, "/")
	tokens := make([]string, len(segments))
	for i := range segments {
		tokens[i] = decodePointerToken(segments[i])
	}

	return tokens
}

func decodePointerToken(token string) string {
	token = strings.ReplaceAll(token, "~1", "/")
	token = strings.ReplaceAll(token, "~0", "~")
	return token
}
