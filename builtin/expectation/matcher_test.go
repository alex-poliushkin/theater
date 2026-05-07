package expectation_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/alex-poliushkin/theater"
	builtinexpectation "github.com/alex-poliushkin/theater/builtin/expectation"
	"github.com/alex-poliushkin/theater/internal/testkit"
)

func TestDescriptorsBuildMatcherCatalog(t *testing.T) {
	t.Parallel()

	catalog, err := theater.NewMatcherCatalog(builtinexpectation.Descriptors()...)
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	refs := []string{
		builtinexpectation.EqualRef,
		builtinexpectation.ContainsRef,
		builtinexpectation.MatchesRef,
		builtinexpectation.NotRef,
		builtinexpectation.PresentRef,
		builtinexpectation.NullRef,
		builtinexpectation.NotNullRef,
		builtinexpectation.GTRef,
		builtinexpectation.GTERef,
		builtinexpectation.LTRef,
		builtinexpectation.LTERef,
		builtinexpectation.BetweenRef,
		builtinexpectation.HasItemRef,
		builtinexpectation.AllItemsRef,
		builtinexpectation.HasKeyRef,
		builtinexpectation.LacksKeyRef,
		builtinexpectation.HasEntryRef,
	}

	for _, ref := range refs {
		if _, err := catalog.Resolve(ref); err != nil {
			t.Fatalf("resolve matcher %q failed: %v", ref, err)
		}
	}
}

func TestEqualMatcherChecksScalarsAndNumbers(t *testing.T) {
	t.Parallel()

	matcher := compileMatcher(t, builtinexpectation.EqualRef, theater.Values{"expected": 200})
	if err := matcher.Check(context.Background(), json.Number("200")); err != nil {
		t.Fatalf("equal matcher must pass: %v", err)
	}
}

func TestEqualMatcherPreservesLargeIntegerPrecision(t *testing.T) {
	t.Parallel()

	matcher := compileMatcher(t, builtinexpectation.EqualRef, theater.Values{"expected": uint64(9007199254740993)})
	if err := matcher.Check(context.Background(), json.Number("9007199254740993")); err != nil {
		t.Fatalf("equal matcher must preserve large integer precision: %v", err)
	}

	if err := matcher.Check(context.Background(), json.Number("9007199254740992")); err == nil {
		t.Fatal("equal matcher must reject off-by-one large integer values")
	}
}

func TestEqualMatcherSupportsMixedNumericRepresentations(t *testing.T) {
	t.Parallel()

	matcher := compileMatcher(t, builtinexpectation.EqualRef, theater.Values{"expected": json.Number("1.0")})
	if err := matcher.Check(context.Background(), int64(1)); err != nil {
		t.Fatalf("equal matcher must compare integer and decimal numeric representations by value: %v", err)
	}
}

func TestContainsMatcherChecksStringSubstring(t *testing.T) {
	t.Parallel()

	matcher := compileMatcher(t, builtinexpectation.ContainsRef, theater.Values{"expected": "issued-token"})
	if err := matcher.Check(context.Background(), `{"token":"issued-token"}`); err != nil {
		t.Fatalf("contains matcher must pass: %v", err)
	}
}

func TestContainsMatcherUsesExactNumericEquality(t *testing.T) {
	t.Parallel()

	matcher := compileMatcher(t, builtinexpectation.ContainsRef, theater.Values{"expected": uint64(9007199254740993)})
	if err := matcher.Check(context.Background(), []any{json.Number("9007199254740992"), json.Number("9007199254740993")}); err != nil {
		t.Fatalf("contains matcher must find exact large integer match: %v", err)
	}

	if err := matcher.Check(context.Background(), []any{json.Number("9007199254740992")}); err == nil {
		t.Fatal("contains matcher must not collapse distinct large integers")
	}
}

func TestBetweenMatcherChecksNumericRange(t *testing.T) {
	t.Parallel()

	matcher := compileMatcher(t, builtinexpectation.BetweenRef, theater.Values{
		"min": 200,
		"max": 299,
	})
	if err := matcher.Check(context.Background(), 204); err != nil {
		t.Fatalf("between matcher must pass: %v", err)
	}
}

func TestBetweenMatcherPreservesLargeIntegerPrecision(t *testing.T) {
	t.Parallel()

	matcher := compileMatcher(t, builtinexpectation.BetweenRef, theater.Values{
		"min": json.Number("9007199254740993"),
		"max": json.Number("9007199254740993"),
	})
	if err := matcher.Check(context.Background(), json.Number("9007199254740993")); err != nil {
		t.Fatalf("between matcher must keep exact single-value range: %v", err)
	}

	if err := matcher.Check(context.Background(), json.Number("9007199254740992")); err == nil {
		t.Fatal("between matcher must reject off-by-one large integer outside range")
	}
}

func TestComparisonMatchersPreserveLargeIntegerPrecision(t *testing.T) {
	t.Parallel()

	matcher := compileMatcher(t, builtinexpectation.GTRef, theater.Values{
		"expected": json.Number("9007199254740992"),
	})
	if err := matcher.Check(context.Background(), json.Number("9007199254740993")); err != nil {
		t.Fatalf("gt matcher must preserve large integer ordering: %v", err)
	}

	if err := matcher.Check(context.Background(), json.Number("9007199254740992")); err == nil {
		t.Fatal("gt matcher must reject equal large integer values")
	}
}

func TestComparisonMatchersSupportScientificJSONNumbers(t *testing.T) {
	t.Parallel()

	matcher := compileMatcher(t, builtinexpectation.GTERef, theater.Values{
		"expected": json.Number("1e3"),
	})
	if err := matcher.Check(context.Background(), 1000); err != nil {
		t.Fatalf("gte matcher must parse scientific json.Number inputs: %v", err)
	}
}

func TestHasKeyMatcherChecksObjects(t *testing.T) {
	t.Parallel()

	matcher := compileMatcher(t, builtinexpectation.HasKeyRef, theater.Values{"key": "token"})
	if err := matcher.Check(context.Background(), map[string]any{"token": "issued-token"}); err != nil {
		t.Fatalf("has_key matcher must pass: %v", err)
	}
}

func TestLacksKeyMatcherChecksObjects(t *testing.T) {
	t.Parallel()

	matcher := compileMatcher(t, builtinexpectation.LacksKeyRef, theater.Values{"key": "error"})
	if err := matcher.Check(context.Background(), map[string]any{"status": "ok"}); err != nil {
		t.Fatalf("lacks_key matcher must pass: %v", err)
	}

	if err := matcher.Check(context.Background(), map[string]any{"error": "bad"}); err == nil {
		t.Fatal("lacks_key matcher must reject present key")
	} else if !theater.IsMatcherMismatch(err) {
		t.Fatalf("lacks_key present-key failure must be mismatch: %v", err)
	}

	if err := matcher.Check(context.Background(), "not an object"); err == nil {
		t.Fatal("lacks_key matcher must reject non-object actual")
	} else if theater.IsMatcherMismatch(err) {
		t.Fatalf("lacks_key non-object failure must stay a type error: %v", err)
	}
}

func TestNullAndPresenceMatchers(t *testing.T) {
	t.Parallel()

	nullMatcher := compileMatcher(t, builtinexpectation.NullRef, theater.Values{})
	if err := nullMatcher.Check(context.Background(), nil); err != nil {
		t.Fatalf("null matcher must pass for nil: %v", err)
	}
	if err := nullMatcher.Check(context.Background(), "value"); err == nil {
		t.Fatal("null matcher must reject non-null values")
	} else if !theater.IsMatcherMismatch(err) {
		t.Fatalf("null matcher failure must be mismatch: %v", err)
	}

	notNullMatcher := compileMatcher(t, builtinexpectation.NotNullRef, theater.Values{})
	if err := notNullMatcher.Check(context.Background(), "value"); err != nil {
		t.Fatalf("not_null matcher must pass for non-null values: %v", err)
	}
	if err := notNullMatcher.Check(context.Background(), nil); err == nil {
		t.Fatal("not_null matcher must reject nil")
	} else if !theater.IsMatcherMismatch(err) {
		t.Fatalf("not_null matcher failure must be mismatch: %v", err)
	}

	presentMatcher := compileMatcher(t, builtinexpectation.PresentRef, theater.Values{})
	if err := presentMatcher.Check(context.Background(), nil); err != nil {
		t.Fatalf("present matcher must pass once selector produced a value, including null: %v", err)
	}
}

func TestNotMatcherInvertsNestedMatcherResult(t *testing.T) {
	t.Parallel()

	matcher := compileMatcher(t, builtinexpectation.NotRef, theater.Values{
		"assert": map[string]any{
			"ref": builtinexpectation.EqualRef,
			"args": map[string]any{
				"expected": 200,
			},
		},
	})

	if err := matcher.Check(context.Background(), 404); err != nil {
		t.Fatalf("not matcher must pass when nested matcher fails: %v", err)
	}

	if err := matcher.Check(context.Background(), 200); err == nil {
		t.Fatal("not matcher must fail when nested matcher passes")
	} else if !theater.IsMatcherMismatch(err) {
		t.Fatalf("not matcher failure must be classified as matcher mismatch: %v", err)
	}
}

func TestNotMatcherSupportsNestedSugarAssert(t *testing.T) {
	t.Parallel()

	matcher := compileMatcher(t, builtinexpectation.NotRef, theater.Values{
		"assert": map[string]any{
			"has_key": "token",
		},
	})

	if err := matcher.Check(context.Background(), map[string]any{"status": "ok"}); err != nil {
		t.Fatalf("not matcher with nested sugar must pass: %v", err)
	}

	if err := matcher.Check(context.Background(), map[string]any{"token": "issued-token"}); err == nil {
		t.Fatal("not matcher with nested sugar must fail when nested matcher passes")
	}
}

func TestNotMatcherPropagatesNestedTypeErrors(t *testing.T) {
	t.Parallel()

	matcher := compileMatcher(t, builtinexpectation.NotRef, theater.Values{
		"assert": map[string]any{
			"ref": builtinexpectation.ContainsRef,
			"args": map[string]any{
				"expected": "token",
			},
		},
	})

	err := matcher.Check(context.Background(), 404)
	if err == nil {
		t.Fatal("not matcher must propagate nested matcher type errors")
	}

	if theater.IsMatcherMismatch(err) {
		t.Fatalf("not matcher must not convert nested matcher type errors into mismatch: %v", err)
	}
}

func TestHasItemMatcherChecksRelativeClauses(t *testing.T) {
	t.Parallel()

	matcher := compileMatcher(t, builtinexpectation.HasItemRef, theater.Values{
		"where": []any{
			map[string]any{
				"subject": map[string]any{"path": "/receiverAddress"},
				"assert": map[string]any{
					"ref": builtinexpectation.EqualRef,
					"args": map[string]any{
						"expected": "+13146235623",
					},
				},
			},
		},
	})

	err := matcher.Check(context.Background(), []any{
		map[string]any{"receiverAddress": "+13146235623"},
	})
	if err != nil {
		t.Fatalf("has_item matcher must pass: %v", err)
	}
}

func TestHasItemMatcherTreatsMissingRelativePathAsNonMatch(t *testing.T) {
	t.Parallel()

	matcher := compileMatcher(t, builtinexpectation.HasItemRef, theater.Values{
		"where": []any{
			map[string]any{
				"subject": map[string]any{"path": "/receiverAddress"},
				"assert": map[string]any{
					"ref": builtinexpectation.EqualRef,
					"args": map[string]any{
						"expected": "+13146235623",
					},
				},
			},
		},
	})

	if err := matcher.Check(context.Background(), []any{
		map[string]any{"id": "notification-1"},
	}); err == nil {
		t.Fatal("has_item matcher must fail when no candidate matches")
	}
}

func TestAllItemsMatcherPassesEmptyList(t *testing.T) {
	t.Parallel()

	matcher := compileMatcher(t, builtinexpectation.AllItemsRef, theater.Values{
		"where": []any{
			map[string]any{
				"subject": map[string]any{"path": "/receiverAddress"},
				"assert": map[string]any{
					"ref": builtinexpectation.MatchesRef,
					"args": map[string]any{
						"pattern": `^\+1`,
					},
				},
			},
		},
	})

	if err := matcher.Check(context.Background(), []any{}); err != nil {
		t.Fatalf("all_items matcher must pass empty list: %v", err)
	}
}

func TestHasEntryMatcherChecksNestedEntryValue(t *testing.T) {
	t.Parallel()

	matcher := compileMatcher(t, builtinexpectation.HasEntryRef, theater.Values{
		"key": "data",
		"assert": map[string]any{
			"ref": builtinexpectation.HasKeyRef,
			"args": map[string]any{
				"key": "receiverAddress",
			},
		},
	})

	err := matcher.Check(context.Background(), map[string]any{
		"data": map[string]any{
			"receiverAddress": "+13146235623",
		},
	})
	if err != nil {
		t.Fatalf("has_entry matcher must pass: %v", err)
	}
}

func TestHasItemMatcherSupportsNestedSugarAssert(t *testing.T) {
	t.Parallel()

	catalog, err := theater.NewMatcherCatalog(builtinexpectation.Descriptors()...)
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	matcher, err := catalog.Compile(builtinexpectation.HasItemRef, theater.Values{
		"where": []any{
			map[string]any{
				"subject": map[string]any{
					"path": "/receiverAddress",
				},
				"assert": map[string]any{
					"eq": "+13146235623",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("compile has_item matcher failed: %v", err)
	}

	err = matcher.Check(context.Background(), []any{
		map[string]any{"receiverAddress": "+13146235623"},
	})
	if err != nil {
		t.Fatalf("has_item matcher with nested sugar must pass: %v", err)
	}
}

func TestHasEntryMatcherSupportsSugarCompile(t *testing.T) {
	t.Parallel()

	catalog, err := theater.NewMatcherCatalog(builtinexpectation.Descriptors()...)
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	matcher, err := catalog.Compile(builtinexpectation.HasEntryRef, theater.Values{
		"key": "data",
		"assert": map[string]any{
			"has_key": "receiverAddress",
		},
	})
	if err != nil {
		t.Fatalf("compile has_entry matcher failed: %v", err)
	}

	err = matcher.Check(context.Background(), map[string]any{
		"data": map[string]any{
			"receiverAddress": "+13146235623",
		},
	})
	if err != nil {
		t.Fatalf("has_entry matcher with nested sugar must pass: %v", err)
	}
}

func TestNotMatcherWrapsPluginMatcher(t *testing.T) {
	t.Parallel()

	catalog, err := theater.NewMatcherCatalog(
		append(
			builtinexpectation.Descriptors(),
			theater.MatcherDescriptor{
				Ref:     "plugin.custom",
				Summary: "plugin matcher used by negation test",
				Args: []theater.MatcherArg{{
					Name:     "expected",
					Required: true,
					Accepts:  theater.ValueContract{Kind: theater.ValueKindString},
				}},
				Actual: theater.ValueContract{Kind: theater.ValueKindString},
				Sugar: theater.SugarSpec{
					Form: theater.SugarFormNone,
				},
				Compile: func(_ theater.MatcherCompileContext, args theater.Values) (theater.Matcher, error) {
					expected, ok := args["expected"].(string)
					if !ok {
						return nil, fmt.Errorf("expected arg must be string, got %T", args["expected"])
					}

					return pluginStringMatcher{expected: expected}, nil
				},
			},
		)...,
	)
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	matcher, err := catalog.Compile(builtinexpectation.NotRef, theater.Values{
		"assert": map[string]any{
			"ref": "plugin.custom",
			"args": map[string]any{
				"expected": "ok",
			},
		},
	})
	if err != nil {
		t.Fatalf("compile not matcher failed: %v", err)
	}

	if err := matcher.Check(context.Background(), "nope"); err != nil {
		t.Fatalf("not matcher around plugin matcher must pass: %v", err)
	}

	if err := matcher.Check(context.Background(), "ok"); err == nil {
		t.Fatal("not matcher around plugin matcher must fail when nested plugin matcher passes")
	}
}

func TestNotMatcherPropagatesTerminalErrors(t *testing.T) {
	t.Parallel()

	terminalCause := errors.New("terminal stop")
	catalog, err := theater.NewMatcherCatalog(
		append(
			builtinexpectation.Descriptors(),
			theater.MatcherDescriptor{
				Ref:     "test.terminal",
				Summary: "terminal matcher used by negation test",
				Sugar: theater.SugarSpec{
					Form: theater.SugarFormNone,
				},
				Compile: func(_ theater.MatcherCompileContext, _ theater.Values) (theater.Matcher, error) {
					return terminalTestMatcher{err: testkit.TerminalError(terminalCause)}, nil
				},
			},
		)...,
	)
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	matcher, err := catalog.Compile(builtinexpectation.NotRef, theater.Values{
		"assert": map[string]any{
			"ref": "test.terminal",
		},
	})
	if err != nil {
		t.Fatalf("compile not matcher failed: %v", err)
	}

	err = matcher.Check(context.Background(), "ignored")
	if !errors.Is(err, terminalCause) {
		t.Fatalf("not matcher must propagate terminal matcher errors: %v", err)
	}
}

func compileMatcher(t *testing.T, ref string, args theater.Values) theater.Matcher {
	t.Helper()

	catalog, err := theater.NewMatcherCatalog(builtinexpectation.Descriptors()...)
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	matcher, err := catalog.Compile(ref, args)
	if err != nil {
		t.Fatalf("compile matcher failed: %v", err)
	}

	return matcher
}

type pluginStringMatcher struct {
	expected string
}

func (m pluginStringMatcher) Check(_ context.Context, actual any) error {
	value, ok := actual.(string)
	if !ok {
		return fmt.Errorf("actual must be string, got %T", actual)
	}

	if value == m.expected {
		return nil
	}

	return theater.MismatchError(fmt.Errorf("actual %q does not equal expected %q", value, m.expected))
}

type terminalTestMatcher struct {
	err error
}

func (m terminalTestMatcher) Check(_ context.Context, _ any) error {
	return m.err
}
