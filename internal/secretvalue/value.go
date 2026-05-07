package secretvalue

import (
	"encoding/json"
	"fmt"
)

const RedactedText = "[redacted]"

type Value struct {
	raw any
}

func New(raw any) Value {
	if existing, ok := raw.(Value); ok {
		return existing
	}

	return Value{raw: raw}
}

func Is(value any) bool {
	_, ok := value.(Value)
	return ok
}

func Reveal(value any) (any, bool) {
	typed, ok := value.(Value)
	if !ok {
		return nil, false
	}

	return typed.raw, true
}

func (v Value) Reveal() any {
	return v.raw
}

func (Value) String() string {
	return RedactedText
}

func (Value) GoString() string {
	return RedactedText
}

func (Value) Format(state fmt.State, verb rune) {
	switch verb {
	case 'q':
		_, _ = fmt.Fprintf(state, "%q", RedactedText)
	default:
		_, _ = state.Write([]byte(RedactedText))
	}
}

func (Value) MarshalJSON() ([]byte, error) {
	return json.Marshal(RedactedText)
}

func (Value) MarshalText() ([]byte, error) {
	return []byte(RedactedText), nil
}
