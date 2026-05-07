package selectvalue

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/alex-poliushkin/theater/internal/runtimevalue"
	specmodel "github.com/alex-poliushkin/theater/spec"
)

// Resolve applies optional decode and RFC 6901 traversal to one runtime value.
func Resolve(value any, decode specmodel.DecodeKind, path specmodel.JSONPointer) (any, error) {
	current := value
	if decode == specmodel.DecodeJSON {
		decoded, err := runtimevalue.DecodeJSON(current, "decoded value")
		if err != nil {
			return nil, err
		}

		current = decoded
	}

	if path.IsRoot() {
		return runtimevalue.Clone(current), nil
	}

	var secretSource any
	tokens, err := pointerTokens(path)
	if err != nil {
		return nil, err
	}

	for _, token := range tokens {
		if runtimevalue.Wrap(current).IsSecret() {
			secretSource = current
		}

		if typed, ok := runtimevalue.Object(current); ok {
			next, ok := typed[token]
			if !ok {
				return nil, fmt.Errorf("path %q target is missing", path)
			}

			current = next
			continue
		}

		if typed, ok := runtimevalue.List(current); ok {
			index, err := pointerIndex(token, len(typed))
			if err != nil {
				return nil, err
			}

			current = typed[index]
			continue
		}

		return nil, fmt.Errorf("path %q cannot traverse %T", path, current)
	}

	if secretSource != nil {
		current = runtimevalue.PreserveSecret(secretSource, current)
	}

	return runtimevalue.Clone(current), nil
}

func decodePointerToken(token string) (string, error) {
	var builder strings.Builder
	for i := 0; i < len(token); i++ {
		if token[i] < 0x20 {
			return "", errors.New("path contains control characters")
		}

		if token[i] != '~' {
			builder.WriteByte(token[i])
			continue
		}

		if i+1 >= len(token) {
			return "", errors.New("path escape is truncated")
		}

		switch token[i+1] {
		case '0':
			builder.WriteByte('~')
		case '1':
			builder.WriteByte('/')
		default:
			return "", fmt.Errorf("path escape ~%c is invalid", token[i+1])
		}

		i++
	}

	return builder.String(), nil
}

func pointerIndex(token string, length int) (int, error) {
	if token == "-" {
		return 0, errors.New(`path token "-" is not supported`)
	}

	if len(token) > 1 && token[0] == '0' {
		return 0, fmt.Errorf("path token %q must not contain leading zeroes", token)
	}

	index, err := strconv.Atoi(token)
	if err != nil {
		return 0, causalError{
			summary: fmt.Sprintf("path token %q must be array index", token),
			cause:   err,
		}
	}

	if index < 0 || index >= length {
		return 0, fmt.Errorf("path index %d is out of range", index)
	}

	return index, nil
}

func pointerTokens(pointer specmodel.JSONPointer) ([]string, error) {
	if pointer.IsRoot() {
		return nil, nil
	}

	segments := strings.Split(pointer.String()[1:], "/")
	tokens := make([]string, 0, len(segments))
	for _, segment := range segments {
		token, err := decodePointerToken(segment)
		if err != nil {
			return nil, err
		}

		if token == "-" {
			return nil, errors.New(`path token "-" is not supported`)
		}

		tokens = append(tokens, token)
	}

	return tokens, nil
}

type causalError struct {
	summary string
	cause   error
}

func (e causalError) Error() string {
	return e.summary
}

func (e causalError) Unwrap() error {
	return e.cause
}
