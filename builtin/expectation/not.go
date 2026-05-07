package expectation

import (
	"context"
	"errors"
	"fmt"

	"github.com/alex-poliushkin/theater"
)

type notMatcher struct {
	matcher theater.Matcher
}

func (m notMatcher) Check(ctx context.Context, actual any) error {
	err := m.matcher.Check(ctx, actual)
	if err == nil {
		return theater.MismatchError(errors.New("actual value matches nested assert"))
	}

	if isTerminalMatcherError(err) {
		return err
	}

	if theater.IsMatcherMismatch(err) {
		return nil
	}

	return err
}

func notDescriptor() theater.MatcherDescriptor {
	return theater.MatcherDescriptor{
		Ref:     NotRef,
		Summary: "actual value does not match nested assert",
		Args: []theater.MatcherArg{{
			Name:     "assert",
			Required: true,
			Accepts:  theater.ValueContract{Kind: theater.ValueKindObject},
		}},
		Actual: theater.ValueContract{Kind: theater.ValueKindAny},
		Sugar: theater.SugarSpec{
			Form: theater.SugarFormNone,
		},
		Compile: func(ctx theater.MatcherCompileContext, args theater.Values) (theater.Matcher, error) {
			assertValue, err := requiredArg(args, "assert")
			if err != nil {
				return nil, err
			}

			matcher, err := compileNestedMatcher(ctx, assertValue)
			if err != nil {
				return nil, fmt.Errorf("assert %w", err)
			}

			return notMatcher{matcher: matcher}, nil
		},
	}
}
