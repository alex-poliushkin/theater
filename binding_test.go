package theater

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestResolveBindingsSupportsLiteralRefObjectAndList(t *testing.T) {
	t.Parallel()

	values, err := newReferenceResolver(Values{
		"issued_token": "token-1",
		"user": map[string]any{
			"id": "user-7",
		},
	}).ResolveBindings(map[string]bindingPlan{
		"token": {
			Kind: BindingKindRef,
			Ref:  &refPlan{Name: "issued_token"},
		},
		"retry": {
			Kind:  BindingKindLiteral,
			Value: 3,
		},
		"payload": {
			Kind: BindingKindObject,
			Object: map[string]bindingPlan{
				"id": {
					Kind: BindingKindRef,
					Ref: &refPlan{
						Name: "user",
						selectorPlan: selectorPlan{
							Path: JSONPointer("/id"),
						},
					},
				},
				"flags": {
					Kind: BindingKindList,
					List: []bindingPlan{
						{Kind: BindingKindLiteral, Value: "a"},
						{Kind: BindingKindLiteral, Value: "b"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("resolve bindings failed: %v", err)
	}

	if got, want := values["token"], "token-1"; got != want {
		t.Fatalf("token mismatch: got %v want %v", got, want)
	}

	if got, want := values["retry"], 3; got != want {
		t.Fatalf("retry mismatch: got %v want %v", got, want)
	}

	payload, ok := values["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload type mismatch: got %T", values["payload"])
	}

	if got, want := payload["id"], "user-7"; got != want {
		t.Fatalf("payload id mismatch: got %v want %v", got, want)
	}

	flags, ok := payload["flags"].([]any)
	if !ok {
		t.Fatalf("flags type mismatch: got %T", payload["flags"])
	}

	if got, want := len(flags), 2; got != want {
		t.Fatalf("flags count mismatch: got %d want %d", got, want)
	}
}

func TestResolveBindingsSupportsCoalesce(t *testing.T) {
	t.Parallel()

	values, err := newReferenceResolver(Values{
		"primary": newMissingValue("optional scenario input"),
		"profile": map[string]any{
			"name": "Ada",
		},
	}).ResolveBindings(map[string]bindingPlan{
		"name": {
			Kind: BindingKindCoalesce,
			Candidates: []bindingPlan{
				{Kind: BindingKindRef, Ref: &refPlan{Name: "primary"}},
				{
					Kind: BindingKindRef,
					Ref: &refPlan{
						Name: "profile",
						selectorPlan: selectorPlan{
							Path: JSONPointer("/name"),
						},
					},
				},
				{Kind: BindingKindLiteral, Value: "fallback"},
			},
		},
		"empty": {
			Kind: BindingKindCoalesce,
			Candidates: []bindingPlan{
				{Kind: BindingKindLiteral, Value: ""},
				{Kind: BindingKindLiteral, Value: "fallback"},
			},
		},
	})
	if err != nil {
		t.Fatalf("resolve bindings failed: %v", err)
	}

	if got, want := values["name"], "Ada"; got != want {
		t.Fatalf("coalesce selected value mismatch: got %v want %v", got, want)
	}

	if got, want := values["empty"], ""; got != want {
		t.Fatalf("coalesce empty string mismatch: got %v want %v", got, want)
	}
}

func TestResolveBindingsSupportsEnvSource(t *testing.T) {
	envName := "THEATER_TEST_BINDING_ENV_SOURCE"
	t.Setenv(envName, "from-env")

	value, err := newReferenceResolver(nil).ResolveBinding(bindingPlan{
		Kind: BindingKindEnv,
		Env:  envName,
	})
	if err != nil {
		t.Fatalf("resolve env binding failed: %v", err)
	}
	if got, want := value, "from-env"; got != want {
		t.Fatalf("env value mismatch: got %v want %v", got, want)
	}
}

func TestResolveCoalesceSkipsUnsetEnvSourceButNotEmptyEnv(t *testing.T) {
	unsetName := "THEATER_TEST_BINDING_ENV_UNSET"
	emptyName := "THEATER_TEST_BINDING_ENV_EMPTY"
	_ = os.Unsetenv(unsetName)
	t.Setenv(emptyName, "")

	values, err := newReferenceResolver(nil).ResolveBindings(map[string]bindingPlan{
		"fallback": {
			Kind: BindingKindCoalesce,
			Candidates: []bindingPlan{
				{Kind: BindingKindEnv, Env: unsetName},
				{Kind: BindingKindLiteral, Value: "fallback"},
			},
		},
		"empty": {
			Kind: BindingKindCoalesce,
			Candidates: []bindingPlan{
				{Kind: BindingKindEnv, Env: emptyName},
				{Kind: BindingKindLiteral, Value: "fallback"},
			},
		},
	})
	if err != nil {
		t.Fatalf("resolve env coalesce bindings failed: %v", err)
	}
	if got, want := values["fallback"], "fallback"; got != want {
		t.Fatalf("env fallback mismatch: got %v want %v", got, want)
	}
	if got, want := values["empty"], ""; got != want {
		t.Fatalf("empty env mismatch: got %v want %v", got, want)
	}
}

func TestInferredBindingContractTreatsEnvAsSecret(t *testing.T) {
	t.Parallel()

	contract := inferredBindingContract(nil, &bindingPlan{
		Kind: BindingKindEnv,
		Env:  "THEATER_SECRET_TOKEN",
	})

	if got, want := contract.Kind, ValueKindString; got != want {
		t.Fatalf("env contract kind mismatch: got %q want %q", got, want)
	}
	if got, want := contract.Sensitivity, SensitivitySecret; got != want {
		t.Fatalf("env contract sensitivity mismatch: got %q want %q", got, want)
	}
}

func TestInferredCoalesceContractPreservesMostSensitiveCandidate(t *testing.T) {
	t.Parallel()

	contract := inferredBindingContract(nil, &bindingPlan{
		Kind: BindingKindCoalesce,
		Candidates: []bindingPlan{
			{Kind: BindingKindLiteral, Value: "fallback"},
			{Kind: BindingKindEnv, Env: "THEATER_SECRET_TOKEN"},
		},
	})

	if got, want := contract.Kind, ValueKindString; got != want {
		t.Fatalf("coalesce contract kind mismatch: got %q want %q", got, want)
	}
	if got, want := contract.Sensitivity, SensitivitySecret; got != want {
		t.Fatalf("coalesce contract sensitivity mismatch: got %q want %q", got, want)
	}
}

func TestResolveCoalesceDoesNotSkipSelectorFailures(t *testing.T) {
	t.Parallel()

	_, err := newReferenceResolver(Values{
		"profile": map[string]any{"name": "Ada"},
	}).ResolveBinding(bindingPlan{
		Kind: BindingKindCoalesce,
		Candidates: []bindingPlan{
			{
				Kind: BindingKindRef,
				Ref: &refPlan{
					Name: "profile",
					selectorPlan: selectorPlan{
						Path: JSONPointer("/missing"),
					},
				},
			},
			{Kind: BindingKindLiteral, Value: "fallback"},
		},
	})
	if err == nil {
		t.Fatal("expected selector failure")
	}
	if got := err.Error(); !strings.Contains(got, "/missing") {
		t.Fatalf("selector failure mismatch: got %q", got)
	}
}

func TestResolveCoalesceRejectsAllMissingCandidates(t *testing.T) {
	t.Parallel()

	_, err := newReferenceResolver(Values{
		"primary": newMissingValue("optional scenario input"),
	}).ResolveBinding(bindingPlan{
		Kind: BindingKindCoalesce,
		Candidates: []bindingPlan{
			{Kind: BindingKindRef, Ref: &refPlan{Name: "primary"}},
		},
	})
	if err == nil {
		t.Fatal("expected all-missing coalesce failure")
	}
	if got := err.Error(); !strings.Contains(got, "coalesce candidates are missing") {
		t.Fatalf("all-missing failure mismatch: got %q", got)
	}
}

func TestCompileBindingAndResolveBindingDoNotShareLiteralContainers(t *testing.T) {
	t.Parallel()

	literal := map[string]any{
		"profile": map[string]any{"id": "user-7"},
		"raw":     []byte("payload"),
	}

	plan := planFragmentCompiler{}.compileBinding("stage.main/test", BindingSpec{
		Kind:  BindingKindLiteral,
		Value: literal,
	})

	literal["profile"].(map[string]any)["id"] = "mutated-source"
	literal["raw"].([]byte)[0] = 'P'

	first, err := newReferenceResolver(nil).ResolveBinding(plan)
	if err != nil {
		t.Fatalf("resolve binding failed: %v", err)
	}

	firstObject := first.(map[string]any)
	firstObject["profile"].(map[string]any)["id"] = "mutated-result"
	firstObject["raw"].([]byte)[0] = 'X'

	second, err := newReferenceResolver(nil).ResolveBinding(plan)
	if err != nil {
		t.Fatalf("resolve binding failed: %v", err)
	}

	secondObject := second.(map[string]any)
	if got, want := secondObject["profile"].(map[string]any)["id"], "user-7"; got != want {
		t.Fatalf("literal profile mismatch: got %v want %v", got, want)
	}

	if got, want := string(secondObject["raw"].([]byte)), "payload"; got != want {
		t.Fatalf("literal raw mismatch: got %q want %q", got, want)
	}
}

func TestResolveRefAndExportCloneSelectedValues(t *testing.T) {
	t.Parallel()

	source := Values{
		"response": map[string]any{
			"body": map[string]any{"token": "issued"},
		},
		"raw": []byte("payload"),
	}

	resolver := newReferenceResolver(source)

	body, err := resolver.ResolveRef(refPlan{
		Name: "response",
		selectorPlan: selectorPlan{
			Path: JSONPointer("/body"),
		},
	})
	if err != nil {
		t.Fatalf("resolve ref failed: %v", err)
	}

	body.(map[string]any)["token"] = "mutated"

	raw, err := resolver.ResolveRef(refPlan{Name: "raw"})
	if err != nil {
		t.Fatalf("resolve raw ref failed: %v", err)
	}

	raw.([]byte)[0] = 'P'

	exported, err := resolver.ExportValues([]exportPlan{{Field: "response"}})
	if err != nil {
		t.Fatalf("export values failed: %v", err)
	}

	exported["response"].(map[string]any)["body"].(map[string]any)["token"] = "export-mutated"

	if got, want := source["response"].(map[string]any)["body"].(map[string]any)["token"], "issued"; got != want {
		t.Fatalf("source token mismatch: got %v want %v", got, want)
	}

	if got, want := string(source["raw"].([]byte)), "payload"; got != want {
		t.Fatalf("source raw mismatch: got %q want %q", got, want)
	}
}

func TestReferenceResolverUsesScopeChainWithoutSnapshot(t *testing.T) {
	t.Parallel()

	parent := newValueScope(nil)
	parent.writeAll(Values{
		"profile": map[string]any{"id": "user-7"},
	})

	child := newValueScope(parent)
	child.writeAll(Values{
		"token": "issued",
	})

	resolved, err := newReferenceResolver(child).ResolveBindings(map[string]bindingPlan{
		"token": {
			Kind: BindingKindRef,
			Ref:  &refPlan{Name: "token"},
		},
		"profile": {
			Kind: BindingKindRef,
			Ref:  &refPlan{Name: "profile"},
		},
	})
	if err != nil {
		t.Fatalf("resolve bindings failed: %v", err)
	}

	if got, want := resolved["token"], "issued"; got != want {
		t.Fatalf("token mismatch: got %v want %v", got, want)
	}

	profile, ok := resolved["profile"].(map[string]any)
	if !ok {
		t.Fatalf("profile type mismatch: got %T", resolved["profile"])
	}

	profile["id"] = "mutated"

	next, err := newReferenceResolver(child).ResolveRef(refPlan{Name: "profile"})
	if err != nil {
		t.Fatalf("resolve ref failed: %v", err)
	}

	if got, want := next.(map[string]any)["id"], "user-7"; got != want {
		t.Fatalf("scope profile mismatch: got %v want %v", got, want)
	}
}

func TestLayeredValueLookupPrefersOverlayOverScope(t *testing.T) {
	t.Parallel()

	scope := newValueScope(nil)
	scope.writeAll(Values{
		"profile": map[string]any{"id": "scope"},
		"token":   "from-scope",
	})

	resolved, err := newReferenceResolver(layeredValueLookup{
		primary: mapValueLookup(Values{
			"profile": map[string]any{"id": "overlay"},
		}),
		fallback: scope,
	}).ResolveBindings(map[string]bindingPlan{
		"profile": {
			Kind: BindingKindRef,
			Ref:  &refPlan{Name: "profile"},
		},
		"token": {
			Kind: BindingKindRef,
			Ref:  &refPlan{Name: "token"},
		},
	})
	if err != nil {
		t.Fatalf("resolve bindings failed: %v", err)
	}

	if got, want := resolved["profile"].(map[string]any)["id"], "overlay"; got != want {
		t.Fatalf("profile mismatch: got %v want %v", got, want)
	}

	if got, want := resolved["token"], "from-scope"; got != want {
		t.Fatalf("token mismatch: got %v want %v", got, want)
	}
}

func TestResolveBindingsSupportsStringAndThroughPipeline(t *testing.T) {
	t.Parallel()

	values, err := newReferenceResolver(Values{
		"flow_id": "flow-7",
		"email":   "demo@example.test",
		"notifications": []any{
			map[string]any{"body": "Verification Code 111111"},
			map[string]any{"receiverAddress": "other@example.test", "body": "Verification Code 222222"},
			map[string]any{"receiverAddress": "demo@example.test", "body": "Verification Code 654321"},
		},
	}).ResolveBindings(map[string]bindingPlan{
		"url": {
			Kind: BindingKindString,
			Parts: []bindingPlan{
				{Kind: BindingKindLiteral, Value: "/email-v1/flows/"},
				{Kind: BindingKindRef, Ref: &refPlan{Name: "flow_id"}},
				{Kind: BindingKindLiteral, Value: "/verifications/email"},
			},
		},
		"otp": {
			Kind: BindingKindRef,
			Ref: &refPlan{
				Name: "notifications",
				selectorPlan: selectorPlan{
					Through: []throughStepPlan{
						{
							Pick: &pickStepPlan{
								At: JSONPointer("/receiverAddress"),
								Equals: bindingPlan{
									Kind: BindingKindRef,
									Ref:  &refPlan{Name: "email"},
								},
							},
						},
						{Path: JSONPointer("/body")},
						{Regexp: &regexpStepPlan{Pattern: `\b(\d{6})\b`, Group: 1}},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("resolve bindings failed: %v", err)
	}

	if got, want := values["url"], "/email-v1/flows/flow-7/verifications/email"; got != want {
		t.Fatalf("url mismatch: got %v want %v", got, want)
	}
	if got, want := values["otp"], "654321"; got != want {
		t.Fatalf("otp mismatch: got %v want %v", got, want)
	}
}

func TestResolveBindingsSupportsDecoratorThroughPipeline(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog()
	if err := catalog.RegisterDecorator("transform.test.wrap", DecoratorDef{
		Contract: DecoratorContract{
			Accepts: ValueContract{Kind: ValueKindString},
			Produces: ValueContract{
				Kind: ValueKindObject,
				Fields: map[string]ValueContract{
					"wrapped": {Kind: ValueKindString},
				},
			},
			Params: []ParamSpec{
				{Name: "prefix", Accepts: ValueContract{Kind: ValueKindString}, Required: true},
				{Name: "suffix", Accepts: ValueContract{Kind: ValueKindString}, Required: true},
			},
		},
		Compile: func(args Values) (DecoratorFunc, error) {
			prefix, _ := args["prefix"].(string)
			suffix, _ := args["suffix"].(string)
			return func(value any) (any, error) {
				return map[string]any{"wrapped": prefix + value.(string) + suffix}, nil
			}, nil
		},
	}); err != nil {
		t.Fatalf("register decorator failed: %v", err)
	}

	values, err := newReferenceResolver(Values{
		"token": "issued",
	}).withDecorators(catalog).ResolveBindings(map[string]bindingPlan{
		"wrapped": {
			Kind: BindingKindRef,
			Ref: &refPlan{
				Name: "token",
				selectorPlan: selectorPlan{
					Through: []throughStepPlan{
						{
							Transform: &decoratorPlan{
								Use: "transform.test.wrap",
								With: Values{
									"prefix": "<<",
									"suffix": ">>",
								},
							},
						},
						{Path: JSONPointer("/wrapped")},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("resolve bindings failed: %v", err)
	}

	if got, want := values["wrapped"], "<<issued>>"; got != want {
		t.Fatalf("wrapped mismatch: got %v want %v", got, want)
	}
}

func TestResolveBindingsPreservesSecretAcrossDecoratorThroughPipeline(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog()
	if err := catalog.RegisterDecorator("transform.test.derive", DecoratorDef{
		Contract: DecoratorContract{
			Accepts:  ValueContract{Kind: ValueKindString},
			Produces: ValueContract{Kind: ValueKindString},
		},
		Compile: func(Values) (DecoratorFunc, error) {
			return func(any) (any, error) {
				return "derived-token", nil
			}, nil
		},
	}); err != nil {
		t.Fatalf("register decorator failed: %v", err)
	}

	values, err := newReferenceResolver(Values{
		"token": NewSecret("issued-token"),
	}).withDecorators(catalog).ResolveBindings(map[string]bindingPlan{
		"derived": {
			Kind: BindingKindRef,
			Ref: &refPlan{
				Name: "token",
				selectorPlan: selectorPlan{
					Through: []throughStepPlan{{
						Transform: &decoratorPlan{Use: "transform.test.derive"},
					}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("resolve bindings failed: %v", err)
	}

	secret, ok := values["derived"].(Secret)
	if !ok {
		t.Fatalf("derived value must remain secret, got %T", values["derived"])
	}
	if got, want := secret.Reveal(), any("derived-token"); got != want {
		t.Fatalf("secret payload mismatch: got %v want %v", got, want)
	}
}

func TestResolveBindingsSupportsPickWhereThroughPipeline(t *testing.T) {
	t.Parallel()

	matchers := pickWhereMatcherCatalog(t)
	values, err := newReferenceResolver(Values{
		"email": "demo@example.test",
		"notifications": []any{
			map[string]any{
				"subject": "Verification Code",
				"body":    "Verification Code 000000",
			},
			map[string]any{
				"receiverAddress": "demo@example.test",
				"subject":         "Password Reset",
				"body":            "Verification Code 111111",
			},
			map[string]any{
				"receiverAddress": "other@example.test",
				"subject":         "Verification Code",
				"body":            "Verification Code 222222",
			},
			map[string]any{
				"receiverAddress": "demo@example.test",
				"subject":         "Verification Code",
				"body":            "Verification Code 654321",
			},
		},
	}).withMatchers(matchers).ResolveBindings(map[string]bindingPlan{
		"otp": {
			Kind: BindingKindRef,
			Ref: &refPlan{
				Name: "notifications",
				selectorPlan: selectorPlan{
					Through: []throughStepPlan{
						{
							Pick: &pickStepPlan{
								Where: []pickWhereClausePlan{
									{
										Subject: relativeSubjectPlan{Path: JSONPointer("/receiverAddress")},
										Assert: assertPlan{
											Ref: "expectation.equal",
											Args: map[string]bindingPlan{
												"expected": {
													Kind: BindingKindRef,
													Ref:  &refPlan{Name: "email"},
												},
											},
										},
									},
									{
										Subject: relativeSubjectPlan{Path: JSONPointer("/subject")},
										Assert: assertPlan{
											Ref: "expectation.contains",
											Args: map[string]bindingPlan{
												"expected": {
													Kind:  BindingKindLiteral,
													Value: "Verification",
												},
											},
										},
									},
								},
							},
						},
						{Path: JSONPointer("/body")},
						{Regexp: &regexpStepPlan{Pattern: `\b(\d{6})\b`, Group: 1}},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("resolve bindings failed: %v", err)
	}

	if got, want := values["otp"], "654321"; got != want {
		t.Fatalf("otp mismatch: got %v want %v", got, want)
	}
}

func TestResolvePickWhereRequiresExactOneMatch(t *testing.T) {
	t.Parallel()

	matchers := pickWhereMatcherCatalog(t)
	testCases := []struct {
		name    string
		items   []any
		wantErr string
	}{
		{
			name: "zero",
			items: []any{
				map[string]any{"receiverAddress": "other@example.test", "subject": "Verification Code"},
			},
			wantErr: "pick matched no items",
		},
		{
			name: "multiple",
			items: []any{
				map[string]any{"receiverAddress": "demo@example.test", "subject": "Verification Code"},
				map[string]any{"receiverAddress": "demo@example.test", "subject": "Verification Code"},
			},
			wantErr: "pick matched multiple items",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := newReferenceResolver(Values{
				"email":         "demo@example.test",
				"notifications": testCase.items,
			}).withMatchers(matchers).ResolveRef(refPlan{
				Name: "notifications",
				selectorPlan: selectorPlan{
					Through: []throughStepPlan{{
						Pick: &pickStepPlan{
							Where: []pickWhereClausePlan{{
								Subject: relativeSubjectPlan{Path: JSONPointer("/receiverAddress")},
								Assert: assertPlan{
									Ref: "expectation.equal",
									Args: map[string]bindingPlan{
										"expected": {
											Kind: BindingKindRef,
											Ref:  &refPlan{Name: "email"},
										},
									},
								},
							}},
						},
					}},
				},
			})
			if err == nil {
				t.Fatal("expected pick where to fail")
			}
			if !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("error mismatch: got %q want contains %q", err, testCase.wantErr)
			}
		})
	}
}

func TestResolvePickWherePropagatesMatcherFailure(t *testing.T) {
	t.Parallel()

	matchers := pickWhereMatcherCatalog(t)
	_, err := newReferenceResolver(Values{
		"notifications": []any{
			map[string]any{"subject": 42},
			map[string]any{"subject": "Verification Code"},
		},
	}).withMatchers(matchers).ResolveRef(refPlan{
		Name: "notifications",
		selectorPlan: selectorPlan{
			Through: []throughStepPlan{{
				Pick: &pickStepPlan{
					Where: []pickWhereClausePlan{{
						Subject: relativeSubjectPlan{Path: JSONPointer("/subject")},
						Assert: assertPlan{
							Ref: "expectation.contains",
							Args: map[string]bindingPlan{
								"expected": {
									Kind:  BindingKindLiteral,
									Value: "Verification",
								},
							},
						},
					}},
				},
			}},
		},
	})
	if err == nil {
		t.Fatal("expected matcher failure, got nil")
	}
	if got, want := err.Error(), "actual must be string"; !strings.Contains(got, want) {
		t.Fatalf("error mismatch: got %q want contains %q", got, want)
	}
}

func TestExportValuesAppliesThroughPipeline(t *testing.T) {
	t.Parallel()

	source := Values{
		"body": `{"notifications":[{"receiverAddress":"demo@example.test","body":"Verification Code 654321"}]}`,
	}

	exported, err := newReferenceResolver(source).ExportValues([]exportPlan{{
		As:    "otp",
		Field: "body",
		selectorPlan: selectorPlan{
			Decode: DecodeJSON,
			Path:   JSONPointer("/notifications"),
			Through: []throughStepPlan{
				{
					Pick: &pickStepPlan{
						At: JSONPointer("/receiverAddress"),
						Equals: bindingPlan{
							Kind:  BindingKindLiteral,
							Value: "demo@example.test",
						},
					},
				},
				{Path: JSONPointer("/body")},
				{Regexp: &regexpStepPlan{Pattern: `\b(\d{6})\b`, Group: 1}},
			},
		},
	}})
	if err != nil {
		t.Fatalf("export values failed: %v", err)
	}

	if got, want := exported["otp"], "654321"; got != want {
		t.Fatalf("exported otp mismatch: got %v want %v", got, want)
	}
}

func pickWhereMatcherCatalog(t *testing.T) *MatcherCatalog {
	t.Helper()

	catalog, err := NewMatcherCatalog(
		MatcherDescriptor{
			Ref: "expectation.equal",
			Args: []MatcherArg{{
				Name:     "expected",
				Required: true,
				Accepts:  ValueContract{Kind: ValueKindAny},
			}},
			Sugar: SugarSpec{Form: SugarFormNone},
			Compile: func(_ MatcherCompileContext, args Values) (Matcher, error) {
				expected, ok := args["expected"]
				if !ok {
					return nil, fmt.Errorf("expected arg is required")
				}

				return pickWhereEqualMatcher{expected: expected}, nil
			},
		},
		MatcherDescriptor{
			Ref: "expectation.contains",
			Args: []MatcherArg{{
				Name:     "expected",
				Required: true,
				Accepts:  ValueContract{Kind: ValueKindString},
			}},
			Sugar: SugarSpec{Form: SugarFormNone},
			Compile: func(_ MatcherCompileContext, args Values) (Matcher, error) {
				expected, ok := args["expected"].(string)
				if !ok {
					return nil, fmt.Errorf("expected arg must be string")
				}

				return pickWhereContainsMatcher{expected: expected}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("create matcher catalog failed: %v", err)
	}

	return catalog
}

type pickWhereEqualMatcher struct {
	expected any
}

func (m pickWhereEqualMatcher) Check(_ context.Context, actual any) error {
	if reflect.DeepEqual(actual, m.expected) {
		return nil
	}

	return MismatchError(fmt.Errorf("actual %v does not equal expected %v", actual, m.expected))
}

type pickWhereContainsMatcher struct {
	expected string
}

func (m pickWhereContainsMatcher) Check(_ context.Context, actual any) error {
	text, ok := actual.(string)
	if !ok {
		return fmt.Errorf("actual must be string")
	}
	if strings.Contains(text, m.expected) {
		return nil
	}

	return MismatchError(fmt.Errorf("actual %q does not contain expected %q", text, m.expected))
}
