package expectation

import (
	"context"
	"errors"
	"fmt"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/runtimevalue"
	"github.com/alex-poliushkin/theater/internal/selectvalue"
)

type relativeClause struct {
	decode  theater.DecodeKind
	path    theater.JSONPointer
	matcher theater.Matcher
}

type hasItemMatcher struct {
	clauses []relativeClause
}

type allItemsMatcher struct {
	clauses []relativeClause
}

type hasEntryMatcher struct {
	key     string
	matcher theater.Matcher
}

func hasItemDescriptor() theater.MatcherDescriptor {
	return theater.MatcherDescriptor{
		Ref:     HasItemRef,
		Summary: "actual list contains at least one item matching all where clauses",
		Args: []theater.MatcherArg{{
			Name:     "where",
			Required: true,
			Accepts: theater.ValueContract{
				Kind: theater.ValueKindList,
				Elem: &theater.ValueContract{Kind: theater.ValueKindObject},
			},
		}},
		Actual: theater.ValueContract{Kind: theater.ValueKindList},
		Sugar: theater.SugarSpec{
			Keys:           []string{"has_item"},
			Form:           theater.SugarFormUnary,
			PositionalArgs: []string{"where"},
		},
		Compile: func(ctx theater.MatcherCompileContext, args theater.Values) (theater.Matcher, error) {
			clauses, err := compileRelativeClauses(ctx, args)
			if err != nil {
				return nil, err
			}

			return hasItemMatcher{clauses: clauses}, nil
		},
	}
}

func allItemsDescriptor() theater.MatcherDescriptor {
	return theater.MatcherDescriptor{
		Ref:     AllItemsRef,
		Summary: "every actual list item matches all where clauses",
		Args: []theater.MatcherArg{{
			Name:     "where",
			Required: true,
			Accepts: theater.ValueContract{
				Kind: theater.ValueKindList,
				Elem: &theater.ValueContract{Kind: theater.ValueKindObject},
			},
		}},
		Actual: theater.ValueContract{Kind: theater.ValueKindList},
		Sugar: theater.SugarSpec{
			Keys:           []string{"all_items"},
			Form:           theater.SugarFormUnary,
			PositionalArgs: []string{"where"},
		},
		Compile: func(ctx theater.MatcherCompileContext, args theater.Values) (theater.Matcher, error) {
			clauses, err := compileRelativeClauses(ctx, args)
			if err != nil {
				return nil, err
			}

			return allItemsMatcher{clauses: clauses}, nil
		},
	}
}

func hasEntryDescriptor() theater.MatcherDescriptor {
	return theater.MatcherDescriptor{
		Ref:     HasEntryRef,
		Summary: "actual object contains key whose value matches nested assert",
		Args: []theater.MatcherArg{
			{
				Name:     "key",
				Required: true,
				Accepts:  theater.ValueContract{Kind: theater.ValueKindString},
			},
			{
				Name:     "assert",
				Required: true,
				Accepts:  theater.ValueContract{Kind: theater.ValueKindObject},
			},
		},
		Actual: theater.ValueContract{Kind: theater.ValueKindObject},
		Sugar: theater.SugarSpec{
			Keys:           []string{"has_entry"},
			Form:           theater.SugarFormFixedTuple,
			PositionalArgs: []string{"key", "assert"},
		},
		Compile: func(ctx theater.MatcherCompileContext, args theater.Values) (theater.Matcher, error) {
			keyValue, err := requiredArg(args, "key")
			if err != nil {
				return nil, err
			}

			key, err := runtimevalue.String(keyValue, "key")
			if err != nil {
				return nil, err
			}

			assertValue, err := requiredArg(args, "assert")
			if err != nil {
				return nil, err
			}

			matcher, err := compileNestedMatcher(ctx, assertValue)
			if err != nil {
				return nil, fmt.Errorf("assert %w", err)
			}

			return hasEntryMatcher{key: key, matcher: matcher}, nil
		},
	}
}

func (m hasItemMatcher) Check(ctx context.Context, actual any) error {
	items, ok := runtimevalue.Wrap(actual).ListOK()
	if !ok {
		return fmt.Errorf("has_item matcher requires list actual, got %T", actual)
	}

	for i := range items {
		match, err := matchesRelativeClauses(ctx, items[i], m.clauses)
		if err != nil {
			return err
		}

		if match {
			return nil
		}
	}

	return theater.MismatchError(errors.New("actual list does not contain item matching where clauses"))
}

func (m allItemsMatcher) Check(ctx context.Context, actual any) error {
	items, ok := runtimevalue.Wrap(actual).ListOK()
	if !ok {
		return fmt.Errorf("all_items matcher requires list actual, got %T", actual)
	}

	for i := range items {
		match, err := matchesRelativeClauses(ctx, items[i], m.clauses)
		if err != nil {
			return err
		}

		if !match {
			return theater.MismatchError(fmt.Errorf("actual list item %d does not match where clauses", i))
		}
	}

	return nil
}

func (m hasEntryMatcher) Check(ctx context.Context, actual any) error {
	object, ok := runtimevalue.Wrap(actual).ObjectOK()
	if !ok {
		return fmt.Errorf("has_entry matcher requires object actual, got %T", actual)
	}

	value, ok := object[m.key]
	if !ok {
		return theater.MismatchError(fmt.Errorf("actual object does not contain key %q", m.key))
	}

	return m.matcher.Check(ctx, value)
}

func compileRelativeClauses(ctx theater.MatcherCompileContext, args theater.Values) ([]relativeClause, error) {
	value, err := requiredArg(args, "where")
	if err != nil {
		return nil, err
	}

	items, ok := runtimevalue.Wrap(value).ListOK()
	if !ok {
		return nil, errors.New("where arg must be list")
	}

	if len(items) == 0 {
		return nil, errors.New("where arg must contain at least one clause")
	}

	clauses := make([]relativeClause, 0, len(items))
	for i := range items {
		clause, err := compileRelativeClause(ctx, items[i])
		if err != nil {
			return nil, fmt.Errorf("where[%d] %w", i, err)
		}

		clauses = append(clauses, clause)
	}

	return clauses, nil
}

func compileRelativeClause(ctx theater.MatcherCompileContext, value any) (relativeClause, error) {
	object, ok := runtimevalue.Wrap(value).ObjectOK()
	if !ok {
		return relativeClause{}, errors.New("clause must be object")
	}

	var clause relativeClause
	for key := range object {
		switch key {
		case "subject", "assert":
		default:
			return relativeClause{}, fmt.Errorf("field %q is not supported", key)
		}
	}

	subjectValue, ok := object["subject"]
	if ok {
		decode, path, err := parseRelativeSubject(subjectValue)
		if err != nil {
			return relativeClause{}, err
		}

		clause.decode = decode
		clause.path = path
	}

	assertValue, ok := object["assert"]
	if !ok {
		return relativeClause{}, errors.New(`field "assert" is required`)
	}

	matcher, err := compileNestedMatcher(ctx, assertValue)
	if err != nil {
		return relativeClause{}, err
	}

	clause.matcher = matcher
	return clause, nil
}

func parseRelativeSubject(
	value any,
) (decode theater.DecodeKind, path theater.JSONPointer, err error) {
	object, ok := runtimevalue.Wrap(value).ObjectOK()
	if !ok {
		return "", "", errors.New(`subject must be object with optional "decode" and "path"`)
	}

	for key, raw := range object {
		switch key {
		case "decode":
			text, err := runtimevalue.String(raw, "decode")
			if err != nil {
				return "", "", err
			}

			decode = theater.DecodeKind(text)
			if !decode.Valid() {
				return "", "", fmt.Errorf("subject decode %q is invalid", decode)
			}
		case "path":
			text, err := runtimevalue.String(raw, "path")
			if err != nil {
				return "", "", err
			}

			pointer, err := theater.ParseJSONPointer(text)
			if err != nil {
				return "", "", fmt.Errorf("subject path is invalid: %w", err)
			}

			path = pointer
		case "from":
			return "", "", errors.New(`subject field "from" is not supported in relative clauses`)
		case "ref":
			return "", "", errors.New(`subject field "ref" is not supported in relative clauses`)
		case "field":
			return "", "", errors.New(`subject field "field" is not supported in relative clauses`)
		default:
			return "", "", fmt.Errorf("subject field %q is not supported", key)
		}
	}

	return decode, path, nil
}

func compileNestedMatcher(ctx theater.MatcherCompileContext, value any) (theater.Matcher, error) {
	object, ok := runtimevalue.Wrap(value).ObjectOK()
	if !ok {
		return nil, errors.New("assert must be object")
	}

	refValue, ok := object["ref"]
	if ok {
		ref, err := runtimevalue.String(refValue, "ref")
		if err != nil {
			return nil, err
		}

		argsValue, hasArgs := object["args"]
		if !hasArgs {
			return ctx.Compile(ref, nil)
		}

		argsObject, ok := runtimevalue.Wrap(argsValue).ObjectOK()
		if !ok {
			return nil, errors.New("assert args must be object")
		}

		args := make(theater.Values, len(argsObject))
		for key, argValue := range argsObject {
			args[key] = argValue
		}

		return ctx.Compile(ref, args)
	}

	if len(object) != 1 {
		return nil, errors.New(`assert must define exactly one matcher`)
	}

	for key, argValue := range object {
		descriptor, err := ctx.ResolveSugarKey(key)
		if err != nil {
			return nil, err
		}

		args, err := compileSugarArgs(argValue, descriptor)
		if err != nil {
			return nil, err
		}

		return ctx.Compile(descriptor.Ref, args)
	}

	return nil, errors.New(`assert must define exactly one matcher`)
}

func compileSugarArgs(value any, descriptor theater.MatcherDescriptor) (theater.Values, error) {
	switch descriptor.Sugar.Form {
	case theater.SugarFormNone:
		if value != nil {
			return nil, errors.New("matcher does not accept sugar arguments")
		}

		return theater.Values{}, nil
	case theater.SugarFormUnary:
		if len(descriptor.Sugar.PositionalArgs) != 1 {
			return nil, errors.New("matcher unary sugar is invalid")
		}

		return theater.Values{
			descriptor.Sugar.PositionalArgs[0]: value,
		}, nil
	case theater.SugarFormFixedTuple:
		return compileFixedTupleSugarArgs(value, descriptor)
	default:
		return nil, fmt.Errorf("matcher sugar form %q is invalid", descriptor.Sugar.Form)
	}
}

func compileFixedTupleSugarArgs(value any, descriptor theater.MatcherDescriptor) (theater.Values, error) {
	if items, ok := runtimevalue.Wrap(value).ListOK(); ok {
		if len(items) != len(descriptor.Sugar.PositionalArgs) {
			return nil, fmt.Errorf("%s matcher must provide %d values", descriptor.Sugar.Keys[0], len(descriptor.Sugar.PositionalArgs))
		}

		args := make(theater.Values, len(descriptor.Sugar.PositionalArgs))
		for i, name := range descriptor.Sugar.PositionalArgs {
			args[name] = items[i]
		}

		return args, nil
	}

	object, ok := runtimevalue.Wrap(value).ObjectOK()
	if !ok {
		return nil, fmt.Errorf("%s matcher must be sequence or object", descriptor.Sugar.Keys[0])
	}

	args := make(theater.Values, len(descriptor.Sugar.PositionalArgs))
	allowed := make(map[string]struct{}, len(descriptor.Sugar.PositionalArgs))
	for _, name := range descriptor.Sugar.PositionalArgs {
		allowed[name] = struct{}{}
	}

	for key, raw := range object {
		if _, ok := allowed[key]; !ok {
			return nil, fmt.Errorf("%s matcher field %q is not supported", descriptor.Sugar.Keys[0], key)
		}

		args[key] = raw
	}

	for _, name := range descriptor.Sugar.PositionalArgs {
		if _, ok := args[name]; ok {
			continue
		}

		return nil, fmt.Errorf("%s matcher requires %q", descriptor.Sugar.Keys[0], name)
	}

	return args, nil
}

func matchesRelativeClauses(ctx context.Context, value any, clauses []relativeClause) (bool, error) {
	for i := range clauses {
		actual, resolveErr := selectvalue.Resolve(value, clauses[i].decode, clauses[i].path)
		if resolveErr != nil {
			return selectionMiss(resolveErr)
		}

		if err := clauses[i].matcher.Check(ctx, actual); err != nil {
			if isTerminalMatcherError(err) {
				return false, err
			}

			return false, nil
		}
	}

	return true, nil
}

func selectionMiss(error) (bool, error) {
	return false, nil
}

type terminalMatcherError interface {
	TheaterTerminal() bool
}

func isTerminalMatcherError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var marker terminalMatcherError
	return errors.As(err, &marker) && marker.TheaterTerminal()
}
