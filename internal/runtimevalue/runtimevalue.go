package runtimevalue

import (
	"fmt"

	"github.com/alex-poliushkin/theater/internal/secretvalue"
)

func Bytes(value any, field string) ([]byte, error) {
	return Wrap(value).Bytes(field)
}

func DecodeJSON(value any, field string) (any, error) {
	decoded, err := Wrap(value).DecodeJSON(field)
	if err != nil {
		return nil, err
	}

	return decoded.Raw(), nil
}

func PreserveSecret(source, derived any) any {
	if !Wrap(source).IsSecret() || Wrap(derived).IsSecret() {
		return derived
	}

	return secretvalue.New(derived)
}

func Clone(value any) any {
	switch typed := value.(type) {
	case secretvalue.Value:
		return secretvalue.New(Clone(typed.Reveal()))
	case []byte:
		cloned := make([]byte, len(typed))
		copy(cloned, typed)
		return cloned
	default:
		if object, ok := normalizedObject(typed); ok {
			return object
		}

		if items, ok := normalizedList(typed); ok {
			return items
		}

		return value
	}
}

func Reveal(value any) any {
	switch typed := value.(type) {
	case secretvalue.Value:
		return Reveal(typed.Reveal())
	case []byte:
		cloned := make([]byte, len(typed))
		copy(cloned, typed)
		return cloned
	case map[string]any:
		revealed := make(map[string]any, len(typed))
		for key, child := range typed {
			revealed[key] = Reveal(child)
		}

		return revealed
	case []any:
		revealed := make([]any, len(typed))
		for i := range typed {
			revealed[i] = Reveal(typed[i])
		}

		return revealed
	default:
		return value
	}
}

func CloneSlice(items []any) []any {
	if len(items) == 0 {
		return nil
	}

	cloned := make([]any, len(items))
	for i := range items {
		cloned[i] = Clone(items[i])
	}

	return cloned
}

func List(value any) ([]any, bool) {
	return Wrap(value).ListOK()
}

func Object(value any) (map[string]any, bool) {
	return Wrap(value).ObjectOK()
}

func String(value any, field string) (string, error) {
	return Wrap(value).String(field)
}

func Bool(value any, field string) (bool, error) {
	return Wrap(value).Bool(field)
}

func Float64(value any, field string) (float64, error) {
	return Wrap(value).Float64(field)
}

func Int(value any, field string) (int, error) {
	return Wrap(value).Int(field)
}

func StringList(value any, field string) ([]string, error) {
	items, ok := List(value)
	if !ok {
		return nil, fmt.Errorf("%s must be list, got %T", field, value)
	}

	list := make([]string, len(items))
	for i := range items {
		typed, err := String(items[i], fmt.Sprintf("%s[%d]", field, i))
		if err != nil {
			return nil, err
		}

		list[i] = typed
	}

	return list, nil
}

func StringMap(value any, field string) (map[string]string, error) {
	object, ok := Object(value)
	if !ok {
		return nil, fmt.Errorf("%s must be object, got %T", field, value)
	}

	mapped := make(map[string]string, len(object))
	for key, item := range object {
		typed, err := String(item, field+"."+key)
		if err != nil {
			return nil, err
		}

		mapped[key] = typed
	}

	return mapped, nil
}

func StringSliceMap(value any, field string) (map[string][]string, error) {
	if value == nil {
		return map[string][]string{}, nil
	}

	object, ok := Object(value)
	if !ok {
		return nil, fmt.Errorf("%s must be object, got %T", field, value)
	}

	result := make(map[string][]string, len(object))
	for key, raw := range object {
		values, err := stringSlice(raw, field+"."+key)
		if err != nil {
			return nil, err
		}

		result[key] = values
	}

	return result, nil
}

func ValidateCanonical(field string, value any) error {
	switch typed := Wrap(value); typed.Kind() {
	case KindNull, KindBytes, KindString, KindBool, KindNumber:
		return nil
	case KindObject:
		object, _ := typed.ObjectOK()
		for key, member := range object {
			if err := ValidateCanonical(field+"."+key, member); err != nil {
				return err
			}
		}

		return nil
	case KindList:
		items, _ := typed.ListOK()
		for i := range items {
			if err := ValidateCanonical(fmt.Sprintf("%s[%d]", field, i), items[i]); err != nil {
				return err
			}
		}

		return nil
	default:
		return fmt.Errorf("%s must use canonical runtime containers, got %T", field, value)
	}
}

func stringSlice(value any, field string) ([]string, error) {
	switch typed := Wrap(value); typed.Kind() {
	case KindString:
		text, _ := typed.StringOK()
		return []string{text}, nil
	case KindList:
		items, _ := typed.ListOK()
		return StringList(items, field)
	default:
		return nil, fmt.Errorf("%s must be string or list of strings, got %T", field, value)
	}
}
