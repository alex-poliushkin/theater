package decorator

import (
	"fmt"

	"github.com/alex-poliushkin/theater/internal/runtimevalue"
)

func intValue(value any, name string) (int, error) {
	return runtimevalue.Int(value, name)
}

func singleRune(value any, name string) (rune, error) {
	text, err := runtimevalue.String(value, name)
	if err != nil {
		return 0, err
	}

	runes := []rune(text)
	if len(runes) != 1 {
		return 0, fmt.Errorf("%s must be single character string", name)
	}

	return runes[0], nil
}
