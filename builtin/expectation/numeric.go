package expectation

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/alex-poliushkin/theater/internal/runtimevalue"
)

type numericValue struct {
	rational *big.Rat
}

func parseNumericValue(value any, field string) (numericValue, error) {
	raw := runtimevalue.Reveal(value)

	switch typed := raw.(type) {
	case int:
		return parseNumericLiteral(strconv.FormatInt(int64(typed), 10), field)
	case int8:
		return parseNumericLiteral(strconv.FormatInt(int64(typed), 10), field)
	case int16:
		return parseNumericLiteral(strconv.FormatInt(int64(typed), 10), field)
	case int32:
		return parseNumericLiteral(strconv.FormatInt(int64(typed), 10), field)
	case int64:
		return parseNumericLiteral(strconv.FormatInt(typed, 10), field)
	case uint:
		return parseNumericLiteral(strconv.FormatUint(uint64(typed), 10), field)
	case uint8:
		return parseNumericLiteral(strconv.FormatUint(uint64(typed), 10), field)
	case uint16:
		return parseNumericLiteral(strconv.FormatUint(uint64(typed), 10), field)
	case uint32:
		return parseNumericLiteral(strconv.FormatUint(uint64(typed), 10), field)
	case uint64:
		return parseNumericLiteral(strconv.FormatUint(typed, 10), field)
	case float32:
		if err := validateFiniteFloat(float64(typed), field); err != nil {
			return numericValue{}, err
		}
		return parseNumericLiteral(strconv.FormatFloat(float64(typed), 'g', -1, 32), field)
	case float64:
		if err := validateFiniteFloat(typed, field); err != nil {
			return numericValue{}, err
		}
		return parseNumericLiteral(strconv.FormatFloat(typed, 'g', -1, 64), field)
	case json.Number:
		return parseNumericLiteral(string(typed), field)
	default:
		return numericValue{}, fmt.Errorf("%s must be numeric, got %T", field, raw)
	}
}

func parseNumericLiteral(text, field string) (numericValue, error) {
	rational, err := parseNumericRat(text)
	if err != nil {
		return numericValue{}, fmt.Errorf("%s must be numeric: %w", field, err)
	}

	return numericValue{rational: rational}, nil
}

func parseNumericRat(text string) (*big.Rat, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, errorsNewNumericLiteral(text)
	}

	if rational, ok := new(big.Rat).SetString(trimmed); ok {
		return rational, nil
	}

	rational, ok := parseExponentNumericRat(trimmed)
	if ok {
		return rational, nil
	}

	return nil, errorsNewNumericLiteral(text)
}

func parseExponentNumericRat(text string) (*big.Rat, bool) {
	index := strings.IndexAny(text, "eE")
	if index == -1 {
		return nil, false
	}

	mantissa, ok := new(big.Rat).SetString(text[:index])
	if !ok {
		return nil, false
	}

	exponent, err := strconv.Atoi(text[index+1:])
	if err != nil {
		return nil, false
	}

	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(absInt(exponent))), nil)
	factor := new(big.Rat).SetInt(scale)
	scaled := new(big.Rat).Set(mantissa)
	if exponent >= 0 {
		return scaled.Mul(scaled, factor), true
	}

	return scaled.Quo(scaled, factor), true
}

func validateFiniteFloat(value float64, field string) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("%s must be finite numeric, got %v", field, value)
	}

	return nil
}

func compareNumericValues(left, right any) (int, bool) {
	leftNumber, err := parseNumericValue(left, "value")
	if err != nil {
		return 0, false
	}

	rightNumber, err := parseNumericValue(right, "value")
	if err != nil {
		return 0, false
	}

	return leftNumber.rational.Cmp(rightNumber.rational), true
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}

	return value
}

func errorsNewNumericLiteral(text string) error {
	return fmt.Errorf("invalid numeric literal %q", text)
}
