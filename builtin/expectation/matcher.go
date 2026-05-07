package expectation

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/runtimevalue"
)

type equalMatcher struct {
	expected any
}

type containsMatcher struct {
	expected any
}

type matchesMatcher struct {
	pattern string
	regexp  *regexp.Regexp
}

type comparisonMatcher struct {
	label    string
	expected any
	number   numericValue
	match    func(cmp int) bool
}

type betweenMatcher struct {
	min    any
	max    any
	minNum numericValue
	maxNum numericValue
}

type hasKeyMatcher struct {
	key string
}

type lacksKeyMatcher struct {
	key string
}

type nullMatcher struct{}

type notNullMatcher struct{}

type presentMatcher struct{}

// Descriptors returns all built-in matcher descriptors.
func Descriptors() []theater.MatcherDescriptor {
	return []theater.MatcherDescriptor{
		equalDescriptor(),
		containsDescriptor(),
		matchesDescriptor(),
		notDescriptor(),
		presentDescriptor(),
		nullDescriptor(),
		notNullDescriptor(),
		comparisonDescriptor(GTRef, "greater than", func(cmp int) bool { return cmp > 0 }),
		comparisonDescriptor(GTERef, "greater than or equal to", func(cmp int) bool { return cmp >= 0 }),
		comparisonDescriptor(LTRef, "less than", func(cmp int) bool { return cmp < 0 }),
		comparisonDescriptor(LTERef, "less than or equal to", func(cmp int) bool { return cmp <= 0 }),
		betweenDescriptor(),
		hasItemDescriptor(),
		allItemsDescriptor(),
		hasKeyDescriptor(),
		lacksKeyDescriptor(),
		hasEntryDescriptor(),
	}
}

func (m equalMatcher) Check(_ context.Context, actual any) error {
	if valuesEqual(actual, m.expected) {
		return nil
	}

	return theater.MismatchError(fmt.Errorf("actual %v does not equal expected %v", actual, m.expected))
}

func (m containsMatcher) Check(_ context.Context, actual any) error {
	wrapped := runtimevalue.Wrap(actual)
	if typed, ok := wrapped.StringOK(); ok {
		expected, err := runtimevalue.String(m.expected, "expected")
		if err != nil {
			return err
		}

		if strings.Contains(typed, expected) {
			return nil
		}

		return theater.MismatchError(fmt.Errorf("actual %v does not contain expected %v", actual, m.expected))
	}

	typed, ok := wrapped.ListOK()
	if !ok {
		return fmt.Errorf("contains matcher requires string or list actual, got %T", actual)
	}

	if sliceContains(typed, m.expected) {
		return nil
	}

	return theater.MismatchError(fmt.Errorf("actual %v does not contain expected %v", actual, m.expected))
}

func (m matchesMatcher) Check(_ context.Context, actual any) error {
	value, err := runtimevalue.String(actual, "actual")
	if err != nil {
		return err
	}

	if m.regexp.MatchString(value) {
		return nil
	}

	return theater.MismatchError(fmt.Errorf("actual %q does not match pattern %q", value, m.pattern))
}

func (m comparisonMatcher) Check(_ context.Context, actual any) error {
	actualNumber, err := parseNumericValue(actual, "actual")
	if err != nil {
		return err
	}

	if m.match(actualNumber.rational.Cmp(m.number.rational)) {
		return nil
	}

	return theater.MismatchError(fmt.Errorf("actual %v is not %s %v", actual, m.label, m.expected))
}

func (m betweenMatcher) Check(_ context.Context, actual any) error {
	value, err := parseNumericValue(actual, "actual")
	if err != nil {
		return err
	}

	if value.rational.Cmp(m.minNum.rational) >= 0 && value.rational.Cmp(m.maxNum.rational) <= 0 {
		return nil
	}

	return theater.MismatchError(fmt.Errorf("actual %v is not between %v and %v", actual, m.min, m.max))
}

func (m hasKeyMatcher) Check(_ context.Context, actual any) error {
	object, ok := runtimevalue.Wrap(actual).ObjectOK()
	if !ok {
		return fmt.Errorf("has_key matcher requires object actual, got %T", actual)
	}

	if _, ok := object[m.key]; ok {
		return nil
	}

	return theater.MismatchError(fmt.Errorf("actual object does not contain key %q", m.key))
}

func (m lacksKeyMatcher) Check(_ context.Context, actual any) error {
	object, ok := runtimevalue.Wrap(actual).ObjectOK()
	if !ok {
		return fmt.Errorf("lacks_key matcher requires object actual, got %T", actual)
	}

	if _, ok := object[m.key]; ok {
		return theater.MismatchError(fmt.Errorf("actual object contains key %q", m.key))
	}

	return nil
}

func (nullMatcher) Check(_ context.Context, actual any) error {
	if runtimevalue.Wrap(actual).Kind() == runtimevalue.KindNull {
		return nil
	}

	return theater.MismatchError(fmt.Errorf("actual %v is not null", actual))
}

func (notNullMatcher) Check(_ context.Context, actual any) error {
	if runtimevalue.Wrap(actual).Kind() != runtimevalue.KindNull {
		return nil
	}

	return theater.MismatchError(errors.New("actual value is null"))
}

func (presentMatcher) Check(context.Context, any) error {
	return nil
}

func equalDescriptor() theater.MatcherDescriptor {
	return theater.MatcherDescriptor{
		Ref:     EqualRef,
		Summary: "actual value equals expected value",
		Args: []theater.MatcherArg{{
			Name:     "expected",
			Required: true,
			Accepts:  theater.ValueContract{Kind: theater.ValueKindAny},
		}},
		Actual: theater.ValueContract{Kind: theater.ValueKindAny},
		Sugar: theater.SugarSpec{
			Keys:           []string{"eq"},
			Form:           theater.SugarFormUnary,
			PositionalArgs: []string{"expected"},
		},
		Compile: func(_ theater.MatcherCompileContext, args theater.Values) (theater.Matcher, error) {
			expected, err := requiredArg(args, "expected")
			if err != nil {
				return nil, err
			}

			return equalMatcher{expected: expected}, nil
		},
	}
}

func containsDescriptor() theater.MatcherDescriptor {
	return theater.MatcherDescriptor{
		Ref:     ContainsRef,
		Summary: "actual string or list contains expected value",
		Args: []theater.MatcherArg{{
			Name:     "expected",
			Required: true,
			Accepts:  theater.ValueContract{Kind: theater.ValueKindAny},
		}},
		Actual: theater.ValueContract{Kinds: theater.NewValueKindSet(theater.ValueKindString, theater.ValueKindList)},
		Sugar: theater.SugarSpec{
			Keys:           []string{"contains"},
			Form:           theater.SugarFormUnary,
			PositionalArgs: []string{"expected"},
		},
		Compile: func(_ theater.MatcherCompileContext, args theater.Values) (theater.Matcher, error) {
			expected, err := requiredArg(args, "expected")
			if err != nil {
				return nil, err
			}

			return containsMatcher{expected: expected}, nil
		},
	}
}

func matchesDescriptor() theater.MatcherDescriptor {
	return theater.MatcherDescriptor{
		Ref:     MatchesRef,
		Summary: "actual string matches regular expression",
		Args: []theater.MatcherArg{{
			Name:     "pattern",
			Required: true,
			Accepts:  theater.ValueContract{Kind: theater.ValueKindString},
		}},
		Actual: theater.ValueContract{Kind: theater.ValueKindString},
		Sugar: theater.SugarSpec{
			Keys:           []string{"matches"},
			Form:           theater.SugarFormUnary,
			PositionalArgs: []string{"pattern"},
		},
		Compile: func(_ theater.MatcherCompileContext, args theater.Values) (theater.Matcher, error) {
			patternValue, err := requiredArg(args, "pattern")
			if err != nil {
				return nil, err
			}

			pattern, err := runtimevalue.String(patternValue, "pattern")
			if err != nil {
				return nil, err
			}

			compiled, err := regexp.Compile(pattern)
			if err != nil {
				return nil, err
			}

			return matchesMatcher{
				pattern: pattern,
				regexp:  compiled,
			}, nil
		},
	}
}

func presentDescriptor() theater.MatcherDescriptor {
	return theater.MatcherDescriptor{
		Ref:     PresentRef,
		Summary: "selected value is present",
		Actual:  theater.ValueContract{Kind: theater.ValueKindAny},
		Sugar:   theater.SugarSpec{Form: theater.SugarFormNone},
		Compile: func(theater.MatcherCompileContext, theater.Values) (theater.Matcher, error) {
			return presentMatcher{}, nil
		},
	}
}

func nullDescriptor() theater.MatcherDescriptor {
	return theater.MatcherDescriptor{
		Ref:     NullRef,
		Summary: "actual value is null",
		Actual:  theater.ValueContract{Kind: theater.ValueKindNull},
		Sugar:   theater.SugarSpec{Form: theater.SugarFormNone},
		Compile: func(theater.MatcherCompileContext, theater.Values) (theater.Matcher, error) {
			return nullMatcher{}, nil
		},
	}
}

func notNullDescriptor() theater.MatcherDescriptor {
	return theater.MatcherDescriptor{
		Ref:     NotNullRef,
		Summary: "actual value is not null",
		Actual: theater.ValueContract{Kinds: theater.NewValueKindSet(
			theater.ValueKindBytes,
			theater.ValueKindString,
			theater.ValueKindNumber,
			theater.ValueKindBool,
			theater.ValueKindObject,
			theater.ValueKindList,
		)},
		Sugar: theater.SugarSpec{Form: theater.SugarFormNone},
		Compile: func(theater.MatcherCompileContext, theater.Values) (theater.Matcher, error) {
			return notNullMatcher{}, nil
		},
	}
}

func comparisonDescriptor(
	ref string,
	label string,
	match func(cmp int) bool,
) theater.MatcherDescriptor {
	return theater.MatcherDescriptor{
		Ref:     ref,
		Summary: fmt.Sprintf("actual value is %s expected value", label),
		Args: []theater.MatcherArg{{
			Name:     "expected",
			Required: true,
			Accepts:  theater.ValueContract{Kind: theater.ValueKindNumber},
		}},
		Actual: theater.ValueContract{Kind: theater.ValueKindNumber},
		Sugar: theater.SugarSpec{
			Keys:           []string{sugarKey(ref)},
			Form:           theater.SugarFormUnary,
			PositionalArgs: []string{"expected"},
		},
		Compile: func(_ theater.MatcherCompileContext, args theater.Values) (theater.Matcher, error) {
			expectedValue, err := requiredArg(args, "expected")
			if err != nil {
				return nil, err
			}

			expected, err := parseNumericValue(expectedValue, "expected")
			if err != nil {
				return nil, err
			}

			return comparisonMatcher{
				label:    label,
				expected: expectedValue,
				number:   expected,
				match:    match,
			}, nil
		},
	}
}

func betweenDescriptor() theater.MatcherDescriptor {
	return theater.MatcherDescriptor{
		Ref:     BetweenRef,
		Summary: "actual value is within an inclusive range",
		Args: []theater.MatcherArg{
			{Name: "min", Required: true, Accepts: theater.ValueContract{Kind: theater.ValueKindNumber}},
			{Name: "max", Required: true, Accepts: theater.ValueContract{Kind: theater.ValueKindNumber}},
		},
		Actual: theater.ValueContract{Kind: theater.ValueKindNumber},
		Sugar: theater.SugarSpec{
			Keys:           []string{"between"},
			Form:           theater.SugarFormFixedTuple,
			PositionalArgs: []string{"min", "max"},
		},
		Compile: func(_ theater.MatcherCompileContext, args theater.Values) (theater.Matcher, error) {
			minValue, err := requiredArg(args, "min")
			if err != nil {
				return nil, err
			}

			maxValue, err := requiredArg(args, "max")
			if err != nil {
				return nil, err
			}

			minNumber, err := parseNumericValue(minValue, "min")
			if err != nil {
				return nil, err
			}

			maxNumber, err := parseNumericValue(maxValue, "max")
			if err != nil {
				return nil, err
			}

			if minNumber.rational.Cmp(maxNumber.rational) > 0 {
				return nil, errors.New("min must be less than or equal to max")
			}

			return betweenMatcher{
				min:    minValue,
				max:    maxValue,
				minNum: minNumber,
				maxNum: maxNumber,
			}, nil
		},
	}
}

func hasKeyDescriptor() theater.MatcherDescriptor {
	return theater.MatcherDescriptor{
		Ref:     HasKeyRef,
		Summary: "actual object contains key",
		Args: []theater.MatcherArg{{
			Name:     "key",
			Required: true,
			Accepts:  theater.ValueContract{Kind: theater.ValueKindString},
		}},
		Actual: theater.ValueContract{Kind: theater.ValueKindObject},
		Sugar: theater.SugarSpec{
			Keys:           []string{"has_key"},
			Form:           theater.SugarFormUnary,
			PositionalArgs: []string{"key"},
		},
		Compile: func(_ theater.MatcherCompileContext, args theater.Values) (theater.Matcher, error) {
			keyValue, err := requiredArg(args, "key")
			if err != nil {
				return nil, err
			}

			key, err := runtimevalue.String(keyValue, "key")
			if err != nil {
				return nil, err
			}

			return hasKeyMatcher{key: key}, nil
		},
	}
}

func lacksKeyDescriptor() theater.MatcherDescriptor {
	return theater.MatcherDescriptor{
		Ref:     LacksKeyRef,
		Summary: "actual object does not contain key",
		Args: []theater.MatcherArg{{
			Name:     "key",
			Required: true,
			Accepts:  theater.ValueContract{Kind: theater.ValueKindString},
		}},
		Actual: theater.ValueContract{Kind: theater.ValueKindObject},
		Sugar:  theater.SugarSpec{Form: theater.SugarFormNone},
		Compile: func(_ theater.MatcherCompileContext, args theater.Values) (theater.Matcher, error) {
			keyValue, err := requiredArg(args, "key")
			if err != nil {
				return nil, err
			}

			key, err := runtimevalue.String(keyValue, "key")
			if err != nil {
				return nil, err
			}

			return lacksKeyMatcher{key: key}, nil
		},
	}
}

func requiredArg(args theater.Values, name string) (any, error) {
	value, ok := args[name]
	if !ok {
		return nil, fmt.Errorf("%s arg is required", name)
	}

	return value, nil
}

func sugarKey(ref string) string {
	return strings.TrimPrefix(ref, "expectation.")
}

func sliceContains(values []any, expected any) bool {
	for _, value := range values {
		if valuesEqual(value, expected) {
			return true
		}
	}

	return false
}

func valuesEqual(left, right any) bool {
	if cmp, ok := compareNumericValues(left, right); ok {
		return cmp == 0
	}

	return reflect.DeepEqual(runtimevalue.Reveal(left), runtimevalue.Reveal(right))
}
