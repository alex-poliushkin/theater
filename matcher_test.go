package theater_test

import (
	"context"
	"testing"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/testkit/errtest"
)

const duplicateMatcherSugarKeyFragment = `matcher sugar key "eq" is already registered`

func TestNewMatcherCatalogRejectsDuplicateSugarKeys(t *testing.T) {
	t.Parallel()

	first := theater.MatcherDescriptor{
		Ref: "expectation.first",
		Sugar: theater.SugarSpec{
			Keys:           []string{"eq"},
			Form:           theater.SugarFormUnary,
			PositionalArgs: []string{"expected"},
		},
		Compile: func(theater.MatcherCompileContext, theater.Values) (theater.Matcher, error) {
			return noopMatcher{}, nil
		},
		Args: []theater.MatcherArg{{
			Name:     "expected",
			Accepts:  theater.ValueContract{Kind: theater.ValueKindAny},
			Required: true,
		}},
	}
	second := theater.MatcherDescriptor{
		Ref: "expectation.second",
		Sugar: theater.SugarSpec{
			Keys:           []string{"eq"},
			Form:           theater.SugarFormUnary,
			PositionalArgs: []string{"expected"},
		},
		Compile: func(theater.MatcherCompileContext, theater.Values) (theater.Matcher, error) {
			return noopMatcher{}, nil
		},
		Args: []theater.MatcherArg{{
			Name:     "expected",
			Accepts:  theater.ValueContract{Kind: theater.ValueKindAny},
			Required: true,
		}},
	}

	_, err := theater.NewMatcherCatalog(first, second)
	if err == nil {
		t.Fatal("expected duplicate sugar key error, got nil")
	}

	errtest.RequireContains(t, err, duplicateMatcherSugarKeyFragment)
}

type noopMatcher struct{}

func (noopMatcher) Check(context.Context, any) error {
	return nil
}
