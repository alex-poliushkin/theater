package runtimevalue

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/alex-poliushkin/theater/internal/secretvalue"
)

const (
	KindUnknown Kind = iota
	KindNull
	KindBytes
	KindString
	KindNumber
	KindBool
	KindObject
	KindList
)

type Kind uint8

type Value struct {
	raw any
}

func Wrap(value any) Value {
	return Value{raw: value}
}

func (v Value) Raw() any {
	return v.raw
}

func (v Value) IsSecret() bool {
	return secretvalue.Is(v.raw)
}

func (v Value) Kind() Kind {
	return detectKind(unwrapped(v.raw))
}

func (v Value) Clone() any {
	return Clone(v.raw)
}

func (v Value) String(field string) (string, error) {
	typed, ok := v.StringOK()
	if !ok {
		return "", fmt.Errorf("%s must be string, got %T", field, v.raw)
	}

	return typed, nil
}

func (v Value) StringOK() (string, bool) {
	typed, ok := unwrapped(v.raw).(string)
	return typed, ok
}

func (v Value) Bytes(field string) ([]byte, error) {
	switch typed := unwrapped(v.raw).(type) {
	case []byte:
		cloned := make([]byte, len(typed))
		copy(cloned, typed)
		return cloned, nil
	case string:
		return []byte(typed), nil
	default:
		return nil, fmt.Errorf("%s must be string or []byte, got %T", field, v.raw)
	}
}

func (v Value) BytesOK() ([]byte, bool) {
	typed, ok := unwrapped(v.raw).([]byte)
	return typed, ok
}

func (v Value) BytesView(field string) ([]byte, error) {
	typed, ok := v.BytesOK()
	if !ok {
		return nil, fmt.Errorf("%s must be []byte, got %T", field, v.raw)
	}

	return typed, nil
}

func (v Value) Bool(field string) (bool, error) {
	typed, ok := v.BoolOK()
	if !ok {
		return false, fmt.Errorf("%s must be bool, got %T", field, v.raw)
	}

	return typed, nil
}

func (v Value) BoolOK() (value, ok bool) {
	value, ok = unwrapped(v.raw).(bool)
	return value, ok
}

func (v Value) Float64(field string) (float64, error) {
	return numericFloat64(unwrapped(v.raw), field)
}

func (v Value) Int(field string) (int, error) {
	return numericInt(unwrapped(v.raw), field)
}

func (v Value) Object(field string) (map[string]any, error) {
	typed, ok := v.ObjectOK()
	if !ok {
		return nil, fmt.Errorf("%s must be object, got %T", field, v.raw)
	}

	return typed, nil
}

func (v Value) ObjectOK() (map[string]any, bool) {
	return normalizedObject(unwrapped(v.raw))
}

func (v Value) List(field string) ([]any, error) {
	typed, ok := v.ListOK()
	if !ok {
		return nil, fmt.Errorf("%s must be list, got %T", field, v.raw)
	}

	return typed, nil
}

func (v Value) ListOK() ([]any, bool) {
	return normalizedList(unwrapped(v.raw))
}

func (v Value) DecodeJSON(field string) (Value, error) {
	data, err := v.Bytes(field)
	if err != nil {
		return Value{}, err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return Value{}, err
	}

	var extra any
	err = decoder.Decode(&extra)
	switch {
	case err == nil:
		return Value{}, errors.New("JSON input must contain exactly one value")
	case errors.Is(err, io.EOF):
		return Wrap(PreserveSecret(v.raw, decoded)), nil
	default:
		return Value{}, err
	}
}

func detectKind(value any) Kind {
	switch value.(type) {
	case nil:
		return KindNull
	case []byte:
		return KindBytes
	case string:
		return KindString
	case bool:
		return KindBool
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64,
		json.Number:
		return KindNumber
	case map[string]any:
		return KindObject
	case []any:
		return KindList
	default:
		if _, ok := normalizedObject(value); ok {
			return KindObject
		}

		if _, ok := normalizedList(value); ok {
			return KindList
		}

		return KindUnknown
	}
}

func unwrapped(value any) any {
	revealed, ok := secretvalue.Reveal(value)
	if !ok {
		return value
	}

	return revealed
}

func numericFloat64(value any, field string) (float64, error) {
	switch typed := value.(type) {
	case int:
		return float64(typed), nil
	case int8:
		return float64(typed), nil
	case int16:
		return float64(typed), nil
	case int32:
		return float64(typed), nil
	case int64:
		return float64(typed), nil
	case uint:
		return float64(typed), nil
	case uint8:
		return float64(typed), nil
	case uint16:
		return float64(typed), nil
	case uint32:
		return float64(typed), nil
	case uint64:
		return float64(typed), nil
	case float32:
		return float64(typed), nil
	case float64:
		return typed, nil
	case json.Number:
		number, err := typed.Float64()
		if err != nil {
			return 0, err
		}

		return number, nil
	default:
		return 0, fmt.Errorf("%s must be numeric, got %T", field, value)
	}
}

func numericInt(value any, field string) (int, error) {
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int8:
		return int(typed), nil
	case int16:
		return int(typed), nil
	case int32:
		return int(typed), nil
	case int64:
		return int(typed), nil
	case uint:
		return int(typed), nil
	case uint8:
		return int(typed), nil
	case uint16:
		return int(typed), nil
	case uint32:
		return int(typed), nil
	case uint64:
		return int(typed), nil
	case float32:
		if float32(int(typed)) != typed {
			return 0, fmt.Errorf("%s must be integer, got %v", field, typed)
		}
		return int(typed), nil
	case float64:
		if float64(int(typed)) != typed {
			return 0, fmt.Errorf("%s must be integer, got %v", field, typed)
		}
		return int(typed), nil
	case json.Number:
		i, err := typed.Int64()
		if err != nil {
			return 0, fmt.Errorf("%s must be integer: %w", field, err)
		}
		return int(i), nil
	default:
		return 0, fmt.Errorf("%s must be integer, got %T", field, value)
	}
}

func normalizedObject(value any) (map[string]any, bool) {
	if typed, ok := value.(map[string]any); ok {
		if typed == nil {
			return nil, true
		}

		normalized := make(map[string]any, len(typed))
		for key, child := range typed {
			normalized[key] = Clone(child)
		}

		return normalized, true
	}

	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() || reflected.Kind() != reflect.Map || reflected.Type().Key().Kind() != reflect.String {
		return nil, false
	}
	if reflected.IsNil() {
		return nil, true
	}

	normalized := make(map[string]any, reflected.Len())
	iter := reflected.MapRange()
	for iter.Next() {
		normalized[iter.Key().String()] = Clone(iter.Value().Interface())
	}

	return normalized, true
}

func normalizedList(value any) ([]any, bool) {
	if typed, ok := value.([]any); ok {
		if typed == nil {
			return nil, true
		}

		normalized := make([]any, len(typed))
		for i := range typed {
			normalized[i] = Clone(typed[i])
		}

		return normalized, true
	}

	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return nil, false
	}

	switch reflected.Kind() {
	case reflect.Slice, reflect.Array:
	default:
		return nil, false
	}
	if reflected.Kind() == reflect.Slice && reflected.IsNil() {
		return nil, true
	}

	if reflected.Kind() == reflect.Slice && reflected.Type() == reflect.TypeOf([]byte(nil)) {
		return nil, false
	}

	normalized := make([]any, reflected.Len())
	for i := 0; i < reflected.Len(); i++ {
		normalized[i] = Clone(reflected.Index(i).Interface())
	}

	return normalized, true
}
