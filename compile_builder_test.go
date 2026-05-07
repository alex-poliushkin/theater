package theater

import "testing"

func TestCompileStageSpecAssignsPathsOrdinalsDefaultsAndPropertyDependencies(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		HTTP: &HTTPSpec{
			Sessions: map[string]HTTPSessionSpec{
				"auth": {},
			},
			Auth: map[string]HTTPAuthSpec{
				"ci_api": {
					Attach: []HTTPAuthAttachmentSpec{{
						Bearer: &HTTPBearerAuthSpec{Token: "token"},
					}},
				},
			},
			Identities: map[string]HTTPIdentitySpec{
				"user": {
					Session: "auth",
					Auth:    "ci_api",
				},
			},
		},
		Scenarios: []ScenarioSpec{
			{
				ID: "auth/login",
				Acts: []ActSpec{
					{
						ID: "submit",
						Action: ActionSpec{
							Use: "action.submit",
						},
						CaptureAuth: &HTTPAuthCaptureSpec{
							Auth: "ci_api",
							Slots: map[string]HTTPCaptureSourceSpec{
								"csrf": {ResponseHeader: "X-CSRF-Token"},
							},
						},
						Properties: map[string]PropertySpec{
							"headers": {
								Inventory: &InventoryCall{
									Use: "inventory.headers",
									With: map[string]BindingSpec{
										"token": {
											Kind: BindingKindRef,
											Ref:  &RefSpec{Name: "token"},
										},
									},
								},
							},
							"missing": {},
							"token": {
								Inventory: &InventoryCall{
									Use: "inventory.token",
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{
				ID:         "login-user",
				ScenarioID: "auth/login",
				Dependencies: []ScenarioDependencySpec{
					{CallID: "prepare-user"},
				},
			},
		},
	}

	stage := compileStageSpec(spec)

	if got, want := stage.Path, "stage.main"; got != want {
		t.Fatalf("stage path mismatch: got %q want %q", got, want)
	}

	if stage.HTTP == nil {
		t.Fatal("compiled stage http spec must be present")
	}
	if stage.HTTP == spec.HTTP {
		t.Fatal("compiled stage http spec must be cloned")
	}
	if _, ok := stage.HTTP.Sessions["auth"]; !ok {
		t.Fatal("compiled stage http sessions must include auth")
	}
	if got, want := stage.HTTP.Auth["ci_api"].Attach[0].Bearer.Token, "token"; got != want {
		t.Fatalf("compiled auth token mismatch: got %q want %q", got, want)
	}
	if got, want := stage.HTTP.Identities["user"].Auth, "ci_api"; got != want {
		t.Fatalf("compiled identity auth mismatch: got %q want %q", got, want)
	}

	if got, want := stage.PlanOrdinal, 1; got != want {
		t.Fatalf("stage ordinal mismatch: got %d want %d", got, want)
	}

	scenario := stage.Scenarios[0]
	if got, want := scenario.Path, "stage.main/scenario.auth~1login"; got != want {
		t.Fatalf("scenario path mismatch: got %q want %q", got, want)
	}

	if got, want := scenario.PlanOrdinal, 2; got != want {
		t.Fatalf("scenario ordinal mismatch: got %d want %d", got, want)
	}

	act := scenario.Acts[0]
	if got, want := act.Path, "stage.main/scenario.auth~1login/act.submit"; got != want {
		t.Fatalf("act path mismatch: got %q want %q", got, want)
	}

	if got, want := act.PlanOrdinal, 3; got != want {
		t.Fatalf("act ordinal mismatch: got %d want %d", got, want)
	}
	if act.CaptureAuth == nil {
		t.Fatal("compiled capture auth must be present")
	}
	if got, want := act.CaptureAuth.Slots["csrf"].ResponseHeader, "X-CSRF-Token"; got != want {
		t.Fatalf("compiled capture header mismatch: got %q want %q", got, want)
	}

	if got, want := stage.ScenarioCalls[0].PlanOrdinal, 4; got != want {
		t.Fatalf("call ordinal mismatch: got %d want %d", got, want)
	}

	if got, want := stage.ScenarioCalls[0].Dependencies[0].When, TriggerPredicateSuccess; got != want {
		t.Fatalf("dependency predicate mismatch: got %q want %q", got, want)
	}

	if got, want := len(act.Properties), 3; got != want {
		t.Fatalf("property count mismatch: got %d want %d", got, want)
	}

	if got, want := act.Properties[0].ID, "missing"; got != want {
		t.Fatalf("first property id mismatch: got %q want %q", got, want)
	}

	if got, want := act.Properties[1].ID, "token"; got != want {
		t.Fatalf("second property id mismatch: got %q want %q", got, want)
	}

	if got, want := act.Properties[2].ID, "headers"; got != want {
		t.Fatalf("third property id mismatch: got %q want %q", got, want)
	}

	if act.Properties[0].Inventory.Present {
		t.Fatal("missing property inventory must stay absent after compile")
	}

	if got, want := len(act.Properties[2].Dependencies), 1; got != want {
		t.Fatalf("headers dependency count mismatch: got %d want %d", got, want)
	}

	if got, want := act.Properties[2].Dependencies[0], "token"; got != want {
		t.Fatalf("headers dependency mismatch: got %q want %q", got, want)
	}
}

func TestCompileStageSpecCopiesMetadataWithoutPrepareSideEffects(t *testing.T) {
	t.Parallel()

	spec := StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{
				ID: "login",
				Inputs: map[string]ValueContract{
					"token": {Kind: ValueKindString},
				},
				Acts: []ActSpec{
					{
						ID:         "submit",
						Eventually: &EventuallySpec{Timeout: "30s", Interval: "1s"},
						Properties: map[string]PropertySpec{
							"headers": {
								Inventory: &InventoryCall{
									Use: "inventory.headers",
									With: map[string]BindingSpec{
										"value": {
											Kind: BindingKindRef,
											Ref:  &RefSpec{Name: "token"},
										},
									},
								},
								Decorators: []DecoratorSpec{
									{
										Use: "decorator.json",
										With: map[string]any{
											"indent": "  ",
										},
									},
								},
							},
							"token": {
								Inventory: &InventoryCall{
									Use: "inventory.token",
								},
							},
						},
						Action: ActionSpec{
							Use: "action.submit",
							With: map[string]BindingSpec{
								"payload": {
									Kind: BindingKindRef,
									Ref:  &RefSpec{Name: "headers"},
								},
							},
							Repeatable: true,
							SourceSpan: &SourceRef{Line: 5, Column: 7},
						},
						Expectations: []ExpectationSpec{
							{
								ID:      "token",
								Subject: SubjectSpec{Field: "token"},
								Assert: AssertSpec{
									Ref: "expectation.equal",
									Args: map[string]BindingSpec{
										"expected": {
											Kind:  BindingKindLiteral,
											Value: "issued",
										},
									},
								},
								SourceSpan: &SourceRef{Line: 6, Column: 9},
							},
						},
						Exports: []ExportSpec{
							{Field: "token", As: "issued_token"},
						},
						Transitions: []TransitionSpec{
							{On: TransitionOnPass, To: "done"},
						},
						SourceSpan: &SourceRef{Line: 4, Column: 5},
					},
				},
				SourceSpan: &SourceRef{Line: 3, Column: 3},
			},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{
				ID:         "login-user",
				ScenarioID: "login",
				Bindings: map[string]BindingSpec{
					"token": {
						Kind:  BindingKindLiteral,
						Value: "issued",
					},
				},
				Exports: []ExportSpec{
					{As: "issued_token", Ref: &RefSpec{Name: "token"}},
				},
				Dependencies: []ScenarioDependencySpec{
					{CallID: "prepare-user", When: TriggerPredicateDone},
				},
				SourceSpan: &SourceRef{Line: 8, Column: 3},
			},
		},
		SourceSpan: &SourceRef{Line: 1, Column: 1},
	}

	stage := compileStageSpec(spec)

	if got, want := stage.Path, "stage.main"; got != want {
		t.Fatalf("stage path mismatch: got %q want %q", got, want)
	}

	if got, want := stage.PlanOrdinal, 1; got != want {
		t.Fatalf("stage ordinal mismatch: got %d want %d", got, want)
	}

	act := stage.Scenarios[0].Acts[0]
	if got, want := act.Path, "stage.main/scenario.login/act.submit"; got != want {
		t.Fatalf("act path mismatch: got %q want %q", got, want)
	}

	if got, want := act.PlanOrdinal, 3; got != want {
		t.Fatalf("act ordinal mismatch: got %d want %d", got, want)
	}

	if got, want := len(act.Properties), 2; got != want {
		t.Fatalf("property count mismatch: got %d want %d", got, want)
	}

	if got, want := act.Properties[0].ID, "token"; got != want {
		t.Fatalf("first property id mismatch: got %q want %q", got, want)
	}

	if got, want := act.Properties[1].ID, "headers"; got != want {
		t.Fatalf("second property id mismatch: got %q want %q", got, want)
	}

	if got, want := act.Properties[1].Dependencies[0], "token"; got != want {
		t.Fatalf("property dependency mismatch: got %q want %q", got, want)
	}

	if got, want := act.Eventually.TimeoutText, "30s"; got != want {
		t.Fatalf("eventually timeout text mismatch: got %q want %q", got, want)
	}

	if got := act.Eventually.Timeout; got != 0 {
		t.Fatalf("compile must not parse timeout, got %s", got)
	}

	if got := act.Properties[1].Decorators[0].Transform; got != nil {
		t.Fatal("compile must not prepare decorator transforms")
	}

	if got, want := act.Action.With["payload"].Kind, BindingKindRef; got != want {
		t.Fatalf("action binding kind mismatch: got %q want %q", got, want)
	}

	if act.Action.With["payload"].Ref == nil {
		t.Fatal("action binding ref must be compiled")
	}

	if got, want := act.Action.With["payload"].Ref.Name, "headers"; got != want {
		t.Fatalf("action binding ref mismatch: got %q want %q", got, want)
	}

	if got, want := act.Expectations[0].Matcher.Ref, ""; got != want {
		t.Fatalf("compile must leave matcher unresolved, got %q", got)
	}

	if got, want := stage.ScenarioCalls[0].Dependencies[0].When, TriggerPredicateDone; got != want {
		t.Fatalf("call dependency predicate mismatch: got %q want %q", got, want)
	}
}
