package theater_test

import (
	"os"
	"strings"
	"testing"

	"github.com/alex-poliushkin/theater"
	builtinaction "github.com/alex-poliushkin/theater/builtin/action"
	builtindecorator "github.com/alex-poliushkin/theater/builtin/decorator"
	builtinexpectation "github.com/alex-poliushkin/theater/builtin/expectation"
	"github.com/alex-poliushkin/theater/internal/testkit"
)

func TestValidateReturnsSortedDiagnosticsWithoutCatalog(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.submit"},
						Transitions: []theater.TransitionSpec{
							{On: theater.TransitionOnPass, To: "missing"},
						},
					},
				},
			},
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit-again",
						Action: theater.ActionSpec{Use: "action.submit"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	diagnostics := validateStage(spec, nil, nil)

	if got, want := len(diagnostics), 2; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "duplicate_scenario_id"; got != want {
		t.Fatalf("first diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[1].Code, "missing_transition_target"; got != want {
		t.Fatalf("second diagnostic code mismatch: got %q want %q", got, want)
	}
}

func TestValidateReturnsNoDiagnosticsForValidSpec(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.submit"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	if got := len(validateStage(spec, nil, nil)); got != 0 {
		t.Fatalf("diagnostic count mismatch: got %d want 0", got)
	}
}

func TestValidateRejectsScenarioWithoutActs(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	diagnostics := validateStage(spec, nil, nil)
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "missing_scenario_acts"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/scenario.login"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Summary, `scenario "login" must define at least one act`; got != want {
		t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
	}
}

func TestValidateWithCatalogReportsActionContractDiagnostics(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID: "submit",
						Action: theater.ActionSpec{
							Use: "action.submit",
						},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "status",
								Subject: theater.SubjectSpec{Field: "status_code"},
								Assert:  theater.AssertSpec{Ref: "expectation.equal", Args: map[string]theater.BindingSpec{"expected": {Kind: theater.BindingKindLiteral, Value: 200}}},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Inputs: map[string]theater.ValueContract{
				"url": {Kind: theater.ValueKindString, Required: true},
			},
			Outputs: map[string]theater.ValueContract{
				"body": {Kind: theater.ValueKindString},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers, err := newMatcherCatalog(builtinexpectation.Descriptors()...)
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matchers)
	if got, want := len(diagnostics), 2; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "missing_action_arg"; got != want {
		t.Fatalf("first diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[1].Code, "unknown_expectation_subject_field"; got != want {
		t.Fatalf("second diagnostic code mismatch: got %q want %q", got, want)
	}
}

func TestValidateReportsMatcherCompileDiagnostics(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "probe",
				Acts: []theater.ActSpec{
					{
						ID:     "request",
						Action: theater.ActionSpec{Use: "action.http"},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "body-pattern",
								Subject: theater.SubjectSpec{Field: "body"},
								Assert: theater.AssertSpec{
									Ref: "expectation.matches",
									Args: map[string]theater.BindingSpec{
										"pattern": {Kind: theater.BindingKindLiteral, Value: "("},
									},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "probe-server", ScenarioID: "probe"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.http", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"body": {Kind: theater.ValueKindString},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers, err := newMatcherCatalog(builtinexpectation.Descriptors()...)
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matchers)
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "invalid_expectation_assert_args"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
}

func TestValidateTreatsPluginAssertAndWhereArgsAsPlainData(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "probe",
			Acts: []theater.ActSpec{{
				ID:     "request",
				Action: theater.ActionSpec{Use: "action.probe"},
				Expectations: []theater.ExpectationSpec{
					{
						ID:      "object-assert",
						Subject: theater.SubjectSpec{Field: "payload"},
						Assert: theater.AssertSpec{
							Ref: "matcher.plugin.object_arg",
							Args: map[string]theater.BindingSpec{
								"assert": {
									Kind: theater.BindingKindObject,
									Object: map[string]theater.BindingSpec{
										"ref":  {Kind: theater.BindingKindLiteral, Value: "matcher.plugin.data"},
										"args": {Kind: theater.BindingKindObject, Object: map[string]theater.BindingSpec{}},
									},
								},
							},
						},
					},
					{
						ID:      "list-where",
						Subject: theater.SubjectSpec{Field: "payload"},
						Assert: theater.AssertSpec{
							Ref: "matcher.plugin.list_arg",
							Args: map[string]theater.BindingSpec{
								"where": {
									Kind: theater.BindingKindList,
									List: []theater.BindingSpec{{
										Kind: theater.BindingKindObject,
										Object: map[string]theater.BindingSpec{
											"assert": {
												Kind: theater.BindingKindObject,
												Object: map[string]theater.BindingSpec{
													"ref":  {Kind: theater.BindingKindLiteral, Value: "matcher.plugin.data"},
													"args": {Kind: theater.BindingKindObject, Object: map[string]theater.BindingSpec{}},
												},
											},
										},
									}},
								},
							},
						},
					},
				},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "probe-server", ScenarioID: "probe"}},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.probe", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"payload": {Kind: theater.ValueKindAny},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers, err := newMatcherCatalog(
		plainDataMatcherDescriptor("matcher.plugin.object_arg", "assert", theater.ValueContract{Kind: theater.ValueKindObject}),
		plainDataMatcherDescriptor("matcher.plugin.list_arg", "where", theater.ValueContract{
			Kind: theater.ValueKindList,
			Elem: &theater.ValueContract{Kind: theater.ValueKindObject},
		}),
	)
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	if diagnostics := validateStage(spec, catalog, matchers); len(diagnostics) != 0 {
		t.Fatalf("expected plain plugin data args to validate cleanly, got %#v", diagnostics)
	}
}

func TestValidateHTTPAuthoringRules(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		spec     theater.StageSpec
		wantCode string
	}{
		{
			name: "named session must be declared",
			spec: theater.StageSpec{
				ID: "main",
				Scenarios: []theater.ScenarioSpec{{
					ID: "probe",
					Acts: []theater.ActSpec{{
						ID: "request",
						Action: theater.ActionSpec{
							Use: builtinaction.HTTPRef,
							With: map[string]theater.BindingSpec{
								"url":     {Kind: theater.BindingKindLiteral, Value: "https://example.test"},
								"session": {Kind: theater.BindingKindLiteral, Value: "auth"},
							},
						},
					}},
				}},
				ScenarioCalls: []theater.ScenarioCallSpec{{ID: "probe-call", ScenarioID: "probe"}},
			},
			wantCode: "unknown_http_session_ref",
		},
		{
			name: "session name none is reserved",
			spec: theater.StageSpec{
				ID: "main",
				HTTP: &theater.HTTPSpec{
					Sessions: map[string]theater.HTTPSessionSpec{
						theater.HTTPSessionNone: {},
					},
				},
				Scenarios:     []theater.ScenarioSpec{{ID: "probe", Acts: []theater.ActSpec{{ID: "request", Action: theater.ActionSpec{Use: "action.noop"}}}}},
				ScenarioCalls: []theater.ScenarioCallSpec{{ID: "probe-call", ScenarioID: "probe"}},
			},
			wantCode: "reserved_http_session_name",
		},
		{
			name: "unknown auth ref",
			spec: theater.StageSpec{
				ID: "main",
				Scenarios: []theater.ScenarioSpec{{
					ID: "probe",
					Acts: []theater.ActSpec{{
						ID: "request",
						Action: theater.ActionSpec{
							Use: builtinaction.HTTPRef,
							With: map[string]theater.BindingSpec{
								"url":  {Kind: theater.BindingKindLiteral, Value: "https://example.test"},
								"auth": {Kind: theater.BindingKindLiteral, Value: "ci_api"},
							},
						},
					}},
				}},
				ScenarioCalls: []theater.ScenarioCallSpec{{ID: "probe-call", ScenarioID: "probe"}},
			},
			wantCode: "unknown_http_auth_ref",
		},
		{
			name: "auth name none is reserved",
			spec: theater.StageSpec{
				ID: "main",
				HTTP: &theater.HTTPSpec{
					Auth: map[string]theater.HTTPAuthSpec{
						theater.HTTPAuthNone: {Attach: []theater.HTTPAuthAttachmentSpec{{Bearer: &theater.HTTPBearerAuthSpec{Token: "token"}}}},
					},
				},
				Scenarios:     []theater.ScenarioSpec{{ID: "probe", Acts: []theater.ActSpec{{ID: "request", Action: theater.ActionSpec{Use: "action.noop"}}}}},
				ScenarioCalls: []theater.ScenarioCallSpec{{ID: "probe-call", ScenarioID: "probe"}},
			},
			wantCode: "reserved_http_auth_name",
		},
		{
			name: "unknown identity ref",
			spec: theater.StageSpec{
				ID: "main",
				Scenarios: []theater.ScenarioSpec{{
					ID: "probe",
					Acts: []theater.ActSpec{{
						ID: "request",
						Action: theater.ActionSpec{
							Use: builtinaction.HTTPRef,
							With: map[string]theater.BindingSpec{
								"url":      {Kind: theater.BindingKindLiteral, Value: "https://example.test"},
								"identity": {Kind: theater.BindingKindLiteral, Value: "user"},
							},
						},
					}},
				}},
				ScenarioCalls: []theater.ScenarioCallSpec{{ID: "probe-call", ScenarioID: "probe"}},
			},
			wantCode: "unknown_http_identity_ref",
		},
		{
			name: "authorization header conflicts with typed bearer auth",
			spec: theater.StageSpec{
				ID: "main",
				HTTP: &theater.HTTPSpec{
					Auth: map[string]theater.HTTPAuthSpec{
						"ci_api": {Attach: []theater.HTTPAuthAttachmentSpec{{Bearer: &theater.HTTPBearerAuthSpec{Token: "token"}}}},
					},
				},
				Scenarios: []theater.ScenarioSpec{{
					ID: "probe",
					Acts: []theater.ActSpec{{
						ID: "request",
						Action: theater.ActionSpec{
							Use: builtinaction.HTTPRef,
							With: map[string]theater.BindingSpec{
								"url":  {Kind: theater.BindingKindLiteral, Value: "https://example.test"},
								"auth": {Kind: theater.BindingKindLiteral, Value: "ci_api"},
								"headers": {Kind: theater.BindingKindLiteral, Value: map[string]any{
									"Authorization": "Bearer manual",
								}},
							},
						},
					}},
				}},
				ScenarioCalls: []theater.ScenarioCallSpec{{ID: "probe-call", ScenarioID: "probe"}},
			},
			wantCode: "conflicting_http_auth_header",
		},
		{
			name: "cookie header conflicts with implicit managed session",
			spec: theater.StageSpec{
				ID: "main",
				Scenarios: []theater.ScenarioSpec{{
					ID: "probe",
					Acts: []theater.ActSpec{{
						ID: "request",
						Action: theater.ActionSpec{
							Use: builtinaction.HTTPRef,
							With: map[string]theater.BindingSpec{
								"url": {Kind: theater.BindingKindLiteral, Value: "https://example.test"},
								"headers": {Kind: theater.BindingKindLiteral, Value: map[string]any{
									"Cookie": "sid=manual",
								}},
							},
						},
					}},
				}},
				ScenarioCalls: []theater.ScenarioCallSpec{{ID: "probe-call", ScenarioID: "probe"}},
			},
			wantCode: "conflicting_http_cookie_header",
		},
		{
			name: "duplicate auth header target",
			spec: theater.StageSpec{
				ID: "main",
				HTTP: &theater.HTTPSpec{
					Auth: map[string]theater.HTTPAuthSpec{
						"dup": {Attach: []theater.HTTPAuthAttachmentSpec{
							{Bearer: &theater.HTTPBearerAuthSpec{Token: "first"}},
							{Basic: &theater.HTTPBasicAuthSpec{Username: "user", Password: "pass"}},
						}},
					},
				},
				Scenarios:     []theater.ScenarioSpec{{ID: "probe", Acts: []theater.ActSpec{{ID: "request", Action: theater.ActionSpec{Use: "action.noop"}}}}},
				ScenarioCalls: []theater.ScenarioCallSpec{{ID: "probe-call", ScenarioID: "probe"}},
			},
			wantCode: "duplicate_http_auth_header",
		},
		{
			name: "literal url query conflicts with api key query attachment",
			spec: theater.StageSpec{
				ID: "main",
				HTTP: &theater.HTTPSpec{
					Auth: map[string]theater.HTTPAuthSpec{
						"dup": {Attach: []theater.HTTPAuthAttachmentSpec{
							{APIKey: &theater.HTTPAPIKeyAuthSpec{In: theater.HTTPAPIKeyInQuery, Name: "api_key", Value: "secret"}},
						}},
					},
				},
				Scenarios: []theater.ScenarioSpec{{
					ID: "probe",
					Acts: []theater.ActSpec{{
						ID: "request",
						Action: theater.ActionSpec{
							Use: builtinaction.HTTPRef,
							With: map[string]theater.BindingSpec{
								"url":  {Kind: theater.BindingKindLiteral, Value: "https://example.test/path?api_key=manual"},
								"auth": {Kind: theater.BindingKindLiteral, Value: "dup"},
							},
						},
					}},
				}},
				ScenarioCalls: []theater.ScenarioCallSpec{{ID: "probe-call", ScenarioID: "probe"}},
			},
			wantCode: "conflicting_http_auth_query",
		},
		{
			name: "form conflicts with body",
			spec: theater.StageSpec{
				ID: "main",
				Scenarios: []theater.ScenarioSpec{{
					ID: "probe",
					Acts: []theater.ActSpec{{
						ID: "request",
						Action: theater.ActionSpec{
							Use: builtinaction.HTTPRef,
							With: map[string]theater.BindingSpec{
								"url":  {Kind: theater.BindingKindLiteral, Value: "https://example.test"},
								"body": {Kind: theater.BindingKindLiteral, Value: "raw"},
								"form": {Kind: theater.BindingKindLiteral, Value: map[string]any{"username": "demo"}},
							},
						},
					}},
				}},
				ScenarioCalls: []theater.ScenarioCallSpec{{ID: "probe-call", ScenarioID: "probe"}},
			},
			wantCode: "conflicting_http_form_body",
		},
		{
			name: "json conflicts with body",
			spec: theater.StageSpec{
				ID: "main",
				Scenarios: []theater.ScenarioSpec{{
					ID: "probe",
					Acts: []theater.ActSpec{{
						ID: "request",
						Action: theater.ActionSpec{
							Use: builtinaction.HTTPRef,
							With: map[string]theater.BindingSpec{
								"url":  {Kind: theater.BindingKindLiteral, Value: "https://example.test"},
								"body": {Kind: theater.BindingKindLiteral, Value: "raw"},
								"json": {Kind: theater.BindingKindLiteral, Value: map[string]any{"username": "demo"}},
							},
						},
					}},
				}},
				ScenarioCalls: []theater.ScenarioCallSpec{{ID: "probe-call", ScenarioID: "probe"}},
			},
			wantCode: "conflicting_http_json_body",
		},
		{
			name: "json content type must be application json",
			spec: theater.StageSpec{
				ID: "main",
				Scenarios: []theater.ScenarioSpec{{
					ID: "probe",
					Acts: []theater.ActSpec{{
						ID: "request",
						Action: theater.ActionSpec{
							Use: builtinaction.HTTPRef,
							With: map[string]theater.BindingSpec{
								"url":  {Kind: theater.BindingKindLiteral, Value: "https://example.test"},
								"json": {Kind: theater.BindingKindLiteral, Value: map[string]any{"username": "demo"}},
								"headers": {Kind: theater.BindingKindLiteral, Value: map[string]any{
									"Content-Type": "text/plain",
								}},
							},
						},
					}},
				}},
				ScenarioCalls: []theater.ScenarioCallSpec{{ID: "probe-call", ScenarioID: "probe"}},
			},
			wantCode: "conflicting_http_json_content_type",
		},
		{
			name: "capture auth only on action http",
			spec: theater.StageSpec{
				ID: "main",
				Scenarios: []theater.ScenarioSpec{{
					ID: "probe",
					Acts: []theater.ActSpec{{
						ID:     "request",
						Action: theater.ActionSpec{Use: "action.noop"},
						CaptureAuth: &theater.HTTPAuthCaptureSpec{
							Auth: "ci_api",
							Slots: map[string]theater.HTTPCaptureSourceSpec{
								"csrf": {ResponseHeader: "X-CSRF-Token"},
							},
						},
					}},
				}},
				ScenarioCalls: []theater.ScenarioCallSpec{{ID: "probe-call", ScenarioID: "probe"}},
			},
			wantCode: "invalid_http_capture_auth_usage",
		},
		{
			name: "capture auth slot must be declared by target auth",
			spec: theater.StageSpec{
				ID: "main",
				HTTP: &theater.HTTPSpec{
					Auth: map[string]theater.HTTPAuthSpec{
						"web": {Attach: []theater.HTTPAuthAttachmentSpec{{HeaderSlot: &theater.HTTPHeaderSlotAuthSpec{Name: "X-CSRF-Token", Slot: "csrf"}}}},
					},
				},
				Scenarios: []theater.ScenarioSpec{{
					ID: "probe",
					Acts: []theater.ActSpec{{
						ID: "request",
						Action: theater.ActionSpec{
							Use: builtinaction.HTTPRef,
							With: map[string]theater.BindingSpec{
								"url": {Kind: theater.BindingKindLiteral, Value: "https://example.test"},
							},
						},
						CaptureAuth: &theater.HTTPAuthCaptureSpec{
							Auth: "web",
							Slots: map[string]theater.HTTPCaptureSourceSpec{
								"missing": {ResponseHeader: "X-CSRF-Token"},
							},
						},
					}},
				}},
				ScenarioCalls: []theater.ScenarioCallSpec{{ID: "probe-call", ScenarioID: "probe"}},
			},
			wantCode: "unknown_http_capture_slot_ref",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			diagnostics := validateStage(tc.spec, nil, nil)
			found := false
			for _, diagnostic := range diagnostics {
				if diagnostic.Code == tc.wantCode {
					found = true
					break
				}
			}

			if !found {
				t.Fatalf("expected diagnostic %q, got %#v", tc.wantCode, diagnostics)
			}
		})
	}
}

func TestValidateReportsSelectorThroughDiagnostics(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "probe",
			Acts: []theater.ActSpec{{
				ID:     "request",
				Action: theater.ActionSpec{Use: "action.http"},
				Expectations: []theater.ExpectationSpec{{
					ID: "body-otp",
					Subject: theater.SubjectSpec{
						Field: "body",
						Through: []theater.ThroughStepSpec{{
							Regexp: &theater.RegexpStepSpec{
								Pattern: "(",
							},
						}},
					},
					Assert: theater.AssertSpec{Ref: "expectation.equal", Args: map[string]theater.BindingSpec{
						"expected": {Kind: theater.BindingKindLiteral, Value: "654321"},
					}},
				}},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "probe-call", ScenarioID: "probe"}},
	}

	diagnostics := validateStage(spec, nil, nil)
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostics[0].Code, "invalid_expectation_subject_through_regexp"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
}

func TestValidateReportsSelectorTransformThroughShapeDiagnostics(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		through  theater.ThroughStepSpec
		wantCode string
	}{
		{
			name: "empty transform use",
			through: theater.ThroughStepSpec{
				Transform: &theater.DecoratorSpec{},
			},
			wantCode: "invalid_expectation_subject_through_transform",
		},
		{
			name: "mixed path and transform",
			through: theater.ThroughStepSpec{
				Path:      "/uid",
				Transform: &theater.DecoratorSpec{Use: "transform.jwt.claims"},
			},
			wantCode: "invalid_expectation_subject_through_step",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			spec := theater.StageSpec{
				ID: "main",
				Scenarios: []theater.ScenarioSpec{{
					ID: "login",
					Acts: []theater.ActSpec{{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.submit"},
						Expectations: []theater.ExpectationSpec{{
							ID: "token",
							Subject: theater.SubjectSpec{
								Field:   "token",
								Through: []theater.ThroughStepSpec{testCase.through},
							},
							Assert: theater.AssertSpec{Ref: builtinexpectation.PresentRef},
						}},
					}},
				}},
				ScenarioCalls: []theater.ScenarioCallSpec{{ID: "login", ScenarioID: "login"}},
			}

			catalog := theater.NewCatalog()
			if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{
				ContractValue: theater.ActionContract{
					Outputs: map[string]theater.ValueContract{
						"token": {Kind: theater.ValueKindString},
					},
				},
			}); err != nil {
				t.Fatalf("register action failed: %v", err)
			}
			if err := catalog.RegisterDecorator("transform.jwt.claims", theater.DecoratorDef{
				Contract: theater.DecoratorContract{
					Accepts:  theater.ValueContract{Kind: theater.ValueKindString},
					Produces: theater.ValueContract{Kind: theater.ValueKindObject},
				},
				Compile: func(theater.Values) (theater.DecoratorFunc, error) {
					return func(value any) (any, error) { return value, nil }, nil
				},
			}); err != nil {
				t.Fatalf("register decorator failed: %v", err)
			}

			diagnostics := validateStage(spec, catalog, matcherCatalog(t, builtinexpectation.Descriptors()...))
			for _, diagnostic := range diagnostics {
				if diagnostic.Code == testCase.wantCode {
					return
				}
			}
			t.Fatalf("expected diagnostic %q, got %#v", testCase.wantCode, diagnostics)
		})
	}
}

func TestValidateReportsPickWhereAssertDiagnostics(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		assert      theater.AssertSpec
		wantSummary string
	}{
		{
			name: "unknown matcher",
			assert: theater.AssertSpec{
				Ref: "expectation.missing",
			},
			wantSummary: "expectation.missing",
		},
		{
			name: "missing required arg",
			assert: theater.AssertSpec{
				Ref: builtinexpectation.ContainsRef,
			},
			wantSummary: `requires arg "expected"`,
		},
		{
			name: "unexpected arg",
			assert: theater.AssertSpec{
				Ref: builtinexpectation.ContainsRef,
				Args: map[string]theater.BindingSpec{
					"expected": {Kind: theater.BindingKindLiteral, Value: "Verification"},
					"extra":    {Kind: theater.BindingKindLiteral, Value: true},
				},
			},
			wantSummary: `does not support arg "extra"`,
		},
		{
			name: "static compile error",
			assert: theater.AssertSpec{
				Ref: builtinexpectation.MatchesRef,
				Args: map[string]theater.BindingSpec{
					"pattern": {Kind: theater.BindingKindLiteral, Value: "("},
				},
			},
			wantSummary: "is invalid",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			spec := theater.StageSpec{
				ID: "main",
				Scenarios: []theater.ScenarioSpec{{
					ID: "probe",
					Acts: []theater.ActSpec{{
						ID:     "request",
						Action: theater.ActionSpec{Use: "action.http"},
						Expectations: []theater.ExpectationSpec{{
							ID: "body-otp",
							Subject: theater.SubjectSpec{
								Field: "body",
								Through: []theater.ThroughStepSpec{{
									Pick: &theater.PickStepSpec{
										Where: []theater.PickWhereClauseSpec{{
											Subject: theater.RelativeSubjectSpec{Path: theater.JSONPointer("/subject")},
											Assert:  testCase.assert,
										}},
									},
								}},
							},
							Assert: theater.AssertSpec{
								Ref: builtinexpectation.EqualRef,
								Args: map[string]theater.BindingSpec{
									"expected": {Kind: theater.BindingKindLiteral, Value: "anything"},
								},
							},
						}},
					}},
				}},
				ScenarioCalls: []theater.ScenarioCallSpec{{ID: "probe-call", ScenarioID: "probe"}},
			}

			catalog := theater.NewCatalog()
			if err := catalog.RegisterAction("action.http", &testkit.ScriptedAction{
				ContractValue: theater.ActionContract{
					Outputs: map[string]theater.ValueContract{
						"body": {Kind: theater.ValueKindList},
					},
				},
			}); err != nil {
				t.Fatalf("register action failed: %v", err)
			}

			matchers, err := newMatcherCatalog(builtinexpectation.Descriptors()...)
			if err != nil {
				t.Fatalf("new matcher catalog failed: %v", err)
			}

			diagnostics := validateStage(spec, catalog, matchers)
			found := false
			for _, diagnostic := range diagnostics {
				if diagnostic.Code == "incompatible_expectation_subject_transform" &&
					strings.Contains(diagnostic.Summary, testCase.wantSummary) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected pick where diagnostic containing %q, got %#v", testCase.wantSummary, diagnostics)
			}
		})
	}
}

func TestValidateReportsPickWhereEmptyRelativeSubject(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "probe",
			Acts: []theater.ActSpec{{
				ID:     "request",
				Action: theater.ActionSpec{Use: "action.http"},
				Expectations: []theater.ExpectationSpec{{
					ID: "body-otp",
					Subject: theater.SubjectSpec{
						Field: "body",
						Through: []theater.ThroughStepSpec{{
							Pick: &theater.PickStepSpec{
								Where: []theater.PickWhereClauseSpec{{
									Assert: theater.AssertSpec{
										Ref: builtinexpectation.ContainsRef,
										Args: map[string]theater.BindingSpec{
											"expected": {Kind: theater.BindingKindLiteral, Value: "Verification"},
										},
									},
								}},
							},
						}},
					},
					Assert: theater.AssertSpec{
						Ref: builtinexpectation.EqualRef,
						Args: map[string]theater.BindingSpec{
							"expected": {Kind: theater.BindingKindLiteral, Value: "anything"},
						},
					},
				}},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "probe-call", ScenarioID: "probe"}},
	}

	diagnostics := validateStage(spec, nil, nil)
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostics[0].Code, "invalid_expectation_subject_through_pick"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostics[0].Summary, "pick where subject must declare decode or path"; !strings.Contains(got, want) {
		t.Fatalf("diagnostic summary mismatch: got %q want contains %q", got, want)
	}
}

func TestValidateReportsIncompatibleExpectationSubjectTransform(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "probe",
			Acts: []theater.ActSpec{{
				ID:     "request",
				Action: theater.ActionSpec{Use: "action.http"},
				Expectations: []theater.ExpectationSpec{{
					ID: "body-otp",
					Subject: theater.SubjectSpec{
						Field: "body",
						Through: []theater.ThroughStepSpec{{
							Regexp: &theater.RegexpStepSpec{
								Pattern: `\d+`,
							},
						}},
					},
					Assert: theater.AssertSpec{Ref: "expectation.equal", Args: map[string]theater.BindingSpec{
						"expected": {Kind: theater.BindingKindLiteral, Value: "654321"},
					}},
				}},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "probe-call", ScenarioID: "probe"}},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.http", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"body": {Kind: theater.ValueKindObject},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers, err := newMatcherCatalog(builtinexpectation.Descriptors()...)
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matchers)
	found := false
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "incompatible_expectation_subject_transform" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected incompatible transform diagnostic, got %#v", diagnostics)
	}
}

func TestValidateWithNilCatalogStillReportsMatcherDiagnostics(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		spec     theater.StageSpec
		wantCode string
	}{
		{
			name: "unknown matcher ref",
			spec: theater.StageSpec{
				ID: "main",
				Scenarios: []theater.ScenarioSpec{
					{
						ID: "probe",
						Acts: []theater.ActSpec{
							{
								ID:     "request",
								Action: theater.ActionSpec{Use: "action.http"},
								Expectations: []theater.ExpectationSpec{
									{
										ID:      "body-pattern",
										Subject: theater.SubjectSpec{Field: "body"},
										Assert:  theater.AssertSpec{Ref: "expectation.missing"},
									},
								},
							},
						},
					},
				},
				ScenarioCalls: []theater.ScenarioCallSpec{
					{ID: "probe-server", ScenarioID: "probe"},
				},
			},
			wantCode: "unknown_expectation_assert_ref",
		},
		{
			name: "invalid matcher args",
			spec: theater.StageSpec{
				ID: "main",
				Scenarios: []theater.ScenarioSpec{
					{
						ID: "probe",
						Acts: []theater.ActSpec{
							{
								ID:     "request",
								Action: theater.ActionSpec{Use: "action.http"},
								Expectations: []theater.ExpectationSpec{
									{
										ID:      "body-pattern",
										Subject: theater.SubjectSpec{Field: "body"},
										Assert: theater.AssertSpec{
											Ref: "expectation.matches",
											Args: map[string]theater.BindingSpec{
												"pattern": {Kind: theater.BindingKindLiteral, Value: "("},
											},
										},
									},
								},
							},
						},
					},
				},
				ScenarioCalls: []theater.ScenarioCallSpec{
					{ID: "probe-server", ScenarioID: "probe"},
				},
			},
			wantCode: "invalid_expectation_assert_args",
		},
	}

	matchers, err := newMatcherCatalog(builtinexpectation.Descriptors()...)
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			diagnostics := validateStage(testCase.spec, nil, matchers)
			if got, want := len(diagnostics), 1; got != want {
				t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
			}

			if got, want := diagnostics[0].Code, testCase.wantCode; got != want {
				t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
			}
		})
	}
}

func TestValidateHTTPActionAcceptsTimeoutInput(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "probe",
				Acts: []theater.ActSpec{
					{
						ID: "request",
						Action: theater.ActionSpec{
							Use: "action.http",
							With: map[string]theater.BindingSpec{
								"url":     {Kind: theater.BindingKindLiteral, Value: "https://example.test/resource"},
								"timeout": {Kind: theater.BindingKindLiteral, Value: "2s"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "probe-server", ScenarioID: "probe"},
		},
	}

	catalog := theater.NewCatalog()
	if err := builtinaction.Register(catalog); err != nil {
		t.Fatalf("register actions failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, nil)
	if got, want := len(diagnostics), 0; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}
}

func TestValidateReportsDecoratorContractDiagnostics(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "probe",
				Acts: []theater.ActSpec{
					{
						ID:     "request",
						Action: theater.ActionSpec{Use: "action.submit"},
						Properties: map[string]theater.PropertySpec{
							"payload": {
								Inventory: &theater.InventoryCall{Use: "inventory.payload"},
								Decorators: []theater.DecoratorSpec{
									{Use: "csv.decode"},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "probe-server", ScenarioID: "probe"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	if err := catalog.RegisterInventory("inventory.payload", &testkit.ScriptedInventory{
		ContractValue: theater.InventoryContract{
			Produces: theater.ValueContract{Kinds: theater.NewValueKindSet(theater.ValueKindObject)},
		},
	}); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}

	if err := builtindecorator.Register(catalog); err != nil {
		t.Fatalf("register decorators failed: %v", err)
	}

	matchers, err := newMatcherCatalog(builtinexpectation.Descriptors()...)
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matchers)
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "incompatible_property_decorator_input_kind"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/scenario.probe/act.request/property.payload/decorator.csv~2decode"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateAllowsPropertyValueDecorator(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "probe",
				Acts: []theater.ActSpec{
					{
						ID:     "request",
						Action: theater.ActionSpec{Use: "action.submit"},
						Properties: map[string]theater.PropertySpec{
							"payload": {
								Value: &theater.BindingSpec{
									Kind:  theater.BindingKindLiteral,
									Value: `{"items":[]}`,
								},
								Decorators: []theater.DecoratorSpec{
									{Use: builtindecorator.JSONRef},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "probe-server", ScenarioID: "probe"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	if err := builtindecorator.Register(catalog); err != nil {
		t.Fatalf("register decorators failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matcherCatalog(t))
	if got, want := len(diagnostics), 0; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}
}

func TestValidateReportsPropertyValueDecoratorContractDiagnostics(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "probe",
				Acts: []theater.ActSpec{
					{
						ID:     "request",
						Action: theater.ActionSpec{Use: "action.submit"},
						Properties: map[string]theater.PropertySpec{
							"payload": {
								Value: &theater.BindingSpec{
									Kind: theater.BindingKindObject,
									Object: map[string]theater.BindingSpec{
										"id": {Kind: theater.BindingKindLiteral, Value: "user-123"},
									},
								},
								Decorators: []theater.DecoratorSpec{
									{Use: builtindecorator.CSVRef},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "probe-server", ScenarioID: "probe"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	if err := builtindecorator.Register(catalog); err != nil {
		t.Fatalf("register decorators failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matcherCatalog(t))
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}
	if got, want := diagnostics[0].Code, "incompatible_property_decorator_input_kind"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
}

func TestValidateReportsDecoratorConfigDiagnostics(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "probe",
				Acts: []theater.ActSpec{
					{
						ID:     "request",
						Action: theater.ActionSpec{Use: "action.submit"},
						Properties: map[string]theater.PropertySpec{
							"payload": {
								Inventory: &theater.InventoryCall{Use: "inventory.payload"},
								Decorators: []theater.DecoratorSpec{
									{
										Use: builtindecorator.CSVRef,
										With: map[string]any{
											"comma":   ";",
											"comment": ";",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "probe-server", ScenarioID: "probe"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	if err := catalog.RegisterInventory("inventory.payload", &testkit.ScriptedInventory{
		ContractValue: theater.InventoryContract{
			Produces: theater.ValueContract{Kinds: theater.NewValueKindSet(theater.ValueKindString)},
		},
	}); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}

	if err := builtindecorator.Register(catalog); err != nil {
		t.Fatalf("register decorators failed: %v", err)
	}

	matchers, err := newMatcherCatalog(builtinexpectation.Descriptors()...)
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matchers)
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "invalid_property_decorator_config"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/scenario.probe/act.request/property.payload/decorator.csv~2decode"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateRejectsStructurallyIncompatibleDecoratorInputContracts(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name              string
		produces          theater.ValueContract
		accepts           theater.ValueContract
		wantSummarySubstr string
	}{
		{
			name: "object required field mismatch",
			produces: theater.ValueContract{
				Kind: theater.ValueKindObject,
				Fields: map[string]theater.ValueContract{
					"token": {Kind: theater.ValueKindString},
				},
			},
			accepts: theater.ValueContract{
				Kind: theater.ValueKindObject,
				Fields: map[string]theater.ValueContract{
					"token": {Kind: theater.ValueKindString, Required: true},
				},
			},
			wantSummarySubstr: `required field "token" is not guaranteed`,
		},
		{
			name: "list elem mismatch",
			produces: theater.ValueContract{
				Kind: theater.ValueKindList,
				Elem: &theater.ValueContract{Kind: theater.ValueKindString},
			},
			accepts: theater.ValueContract{
				Kind: theater.ValueKindList,
				Elem: &theater.ValueContract{Kind: theater.ValueKindNumber},
			},
			wantSummarySubstr: `list elements: kind "string" is not accepted`,
		},
		{
			name: "nullability mismatch",
			produces: theater.ValueContract{
				Kinds: theater.NewValueKindSet(theater.ValueKindString, theater.ValueKindNull),
			},
			accepts: theater.ValueContract{
				Kind: theater.ValueKindString,
			},
			wantSummarySubstr: `kind "null" is not accepted`,
		},
	}

	for _, testcase := range testCases {
		testcase := testcase
		t.Run(testcase.name, func(t *testing.T) {
			t.Parallel()

			spec := theater.StageSpec{
				ID: "main",
				Scenarios: []theater.ScenarioSpec{
					{
						ID: "probe",
						Acts: []theater.ActSpec{
							{
								ID:     "request",
								Action: theater.ActionSpec{Use: "action.submit"},
								Properties: map[string]theater.PropertySpec{
									"payload": {
										Inventory: &theater.InventoryCall{Use: "inventory.payload"},
										Decorators: []theater.DecoratorSpec{
											{Use: "decorator.validate"},
										},
									},
								},
							},
						},
					},
				},
				ScenarioCalls: []theater.ScenarioCallSpec{
					{ID: "probe-server", ScenarioID: "probe"},
				},
			}

			catalog := theater.NewCatalog()
			if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{}); err != nil {
				t.Fatalf("register action failed: %v", err)
			}
			if err := catalog.RegisterInventory("inventory.payload", &testkit.ScriptedInventory{
				ContractValue: theater.InventoryContract{
					Produces: testcase.produces,
				},
			}); err != nil {
				t.Fatalf("register inventory failed: %v", err)
			}
			if err := catalog.RegisterDecorator("decorator.validate", theater.DecoratorDef{
				Contract: theater.DecoratorContract{
					Accepts:  testcase.accepts,
					Produces: testcase.accepts,
				},
				Compile: func(args theater.Values) (theater.DecoratorFunc, error) {
					return func(value any) (any, error) { return value, nil }, nil
				},
			}); err != nil {
				t.Fatalf("register decorator failed: %v", err)
			}

			matchers, err := newMatcherCatalog(builtinexpectation.Descriptors()...)
			if err != nil {
				t.Fatalf("new matcher catalog failed: %v", err)
			}

			diagnostics := validateStage(spec, catalog, matchers)
			if got, want := len(diagnostics), 1; got != want {
				t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
			}
			if got, want := diagnostics[0].Code, "incompatible_property_decorator_input_kind"; got != want {
				t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
			}
			if got, want := diagnostics[0].Path, "stage.main/scenario.probe/act.request/property.payload/decorator.decorator~2validate"; got != want {
				t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
			}
			if !strings.Contains(diagnostics[0].Summary, testcase.wantSummarySubstr) {
				t.Fatalf("diagnostic summary mismatch: got %q want substring %q", diagnostics[0].Summary, testcase.wantSummarySubstr)
			}
		})
	}
}

func TestValidateRejectsObjectValuesThatViolateObjectElemSpec(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "check-command-env",
				Acts: []theater.ActSpec{
					{
						ID: "run",
						Action: theater.ActionSpec{
							Use: "action.command",
							With: map[string]theater.BindingSpec{
								"executable": {Kind: theater.BindingKindLiteral, Value: "/bin/echo"},
								"env": {
									Kind: theater.BindingKindObject,
									Object: map[string]theater.BindingSpec{
										"BAD": {Kind: theater.BindingKindLiteral, Value: 42},
									},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "run-command", ScenarioID: "check-command-env"},
		},
	}

	catalog, matchers, err := newBuiltins()
	if err != nil {
		t.Fatalf("new builtins failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matchers)
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "incompatible_action_arg"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
}

func TestValidateRejectsDirectCallsIntoInternalScenarioNamespace(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "identity/internal/bootstrap",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.submit"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "bootstrap-user", ScenarioID: "identity/internal/bootstrap"},
		},
	}

	diagnostics := validateStage(spec, nil, nil)
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "internal_scenario_not_accessible"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
}

func TestValidateRejectsRootInternalScenarioNamespace(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "internal/bootstrap",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.submit"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "bootstrap-user", ScenarioID: "internal/bootstrap"},
		},
	}

	diagnostics := validateStage(spec, nil, nil)
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}

	if got, want := diagnostics[0].Code, "internal_scenario_not_accessible"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
}

func TestValidateRejectsMissingLocalActionRef(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID: "submit",
						Action: theater.ActionSpec{
							Use: "action.submit",
							With: map[string]theater.BindingSpec{
								"token": {
									Kind: theater.BindingKindRef,
									Ref:  &theater.RefSpec{Name: "missing_token"},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	diagnostics := validateStage(spec, nil, nil)
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}

	if got, want := diagnostics[0].Code, "unresolved_binding_ref"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/scenario.login/act.submit/action/binding.token"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateRejectsMissingLogRef(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.submit"},
						Logs: []theater.LogSpec{{
							ID:    "response",
							Value: theater.LogValueSpec{Ref: "missing_token"},
						}},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	diagnostics := validateStage(spec, nil, nil)
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}

	if got, want := diagnostics[0].Code, "unresolved_log_ref"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/scenario.login/act.submit/log.response/value"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateRejectsMissingLogSelectorBindingRef(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.submit"},
						Logs: []theater.LogSpec{{
							ID: "response",
							Value: theater.LogValueSpec{
								Field: "body",
								Through: []theater.ThroughStepSpec{{
									Pick: &theater.PickStepSpec{
										At: "/id",
										Equals: theater.BindingSpec{
											Kind: theater.BindingKindRef,
											Ref:  &theater.RefSpec{Name: "missing_token"},
										},
									},
								}},
							},
						}},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	diagnostics := validateStage(spec, nil, nil)
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}

	if got, want := diagnostics[0].Code, "unresolved_binding_ref"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/scenario.login/act.submit/log.response/value/through[0]/pick/equals"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateRejectsInvalidLogActionFields(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		log     theater.LogSpec
		outputs map[string]theater.ValueContract
		code    string
		path    string
	}{
		{
			name: "unknown value field",
			log: theater.LogSpec{
				ID:    "response",
				Value: theater.LogValueSpec{Field: "missing_body"},
			},
			outputs: map[string]theater.ValueContract{
				"body": {Kind: theater.ValueKindString},
			},
			code: "unknown_log_field",
			path: "stage.main/scenario.login/act.submit/log.response/value",
		},
		{
			name: "unknown message field",
			log: theater.LogSpec{
				ID:      "response",
				Message: "response received",
				Fields: map[string]theater.LogValueSpec{
					"status": {Field: "missing_status"},
				},
			},
			outputs: map[string]theater.ValueContract{
				"body": {Kind: theater.ValueKindString},
			},
			code: "unknown_log_field",
			path: "stage.main/scenario.login/act.submit/log.response/fields.status",
		},
		{
			name: "unknown nested object field",
			log: theater.LogSpec{
				ID: "response",
				Value: theater.LogValueSpec{
					Object: map[string]theater.LogValueSpec{
						"user": {Field: "missing_body"},
					},
				},
			},
			outputs: map[string]theater.ValueContract{
				"body": {Kind: theater.ValueKindString},
			},
			code: "unknown_log_field",
			path: "stage.main/scenario.login/act.submit/log.response/value.user",
		},
		{
			name: "unknown nested list field",
			log: theater.LogSpec{
				ID: "response",
				Value: theater.LogValueSpec{
					List: []theater.LogValueSpec{
						{Field: "missing_body"},
					},
				},
			},
			outputs: map[string]theater.ValueContract{
				"body": {Kind: theater.ValueKindString},
			},
			code: "unknown_log_field",
			path: "stage.main/scenario.login/act.submit/log.response/value[0]",
		},
		{
			name: "path incompatible with scalar field",
			log: theater.LogSpec{
				ID: "response",
				Value: theater.LogValueSpec{
					Field: "body",
					Path:  "/id",
				},
			},
			outputs: map[string]theater.ValueContract{
				"body": {Kind: theater.ValueKindString},
			},
			code: "incompatible_log_value_path",
			path: "stage.main/scenario.login/act.submit/log.response/value",
		},
		{
			name: "decode incompatible with object field",
			log: theater.LogSpec{
				ID: "response",
				Value: theater.LogValueSpec{
					Field:  "body",
					Decode: theater.DecodeJSON,
				},
			},
			outputs: map[string]theater.ValueContract{
				"body": {Kind: theater.ValueKindObject},
			},
			code: "incompatible_log_value_decode",
			path: "stage.main/scenario.login/act.submit/log.response/value",
		},
		{
			name: "regexp incompatible with object field",
			log: theater.LogSpec{
				ID: "response",
				Value: theater.LogValueSpec{
					Field: "body",
					Through: []theater.ThroughStepSpec{{
						Regexp: &theater.RegexpStepSpec{Pattern: "(.+)"},
					}},
				},
			},
			outputs: map[string]theater.ValueContract{
				"body": {Kind: theater.ValueKindObject},
			},
			code: "incompatible_log_value_transform",
			path: "stage.main/scenario.login/act.submit/log.response/value",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actionRef := "action.submit"
			catalog := theater.NewCatalog()
			if err := catalog.RegisterAction(actionRef, &testkit.ScriptedAction{
				ContractValue: theater.ActionContract{
					Outputs: tc.outputs,
				},
			}); err != nil {
				t.Fatalf("register action failed: %v", err)
			}

			spec := theater.StageSpec{
				ID: "main",
				Scenarios: []theater.ScenarioSpec{
					{
						ID: "login",
						Acts: []theater.ActSpec{
							{
								ID:     "submit",
								Action: theater.ActionSpec{Use: actionRef},
								Logs:   []theater.LogSpec{tc.log},
							},
						},
					},
				},
				ScenarioCalls: []theater.ScenarioCallSpec{
					{ID: "login-user", ScenarioID: "login"},
				},
			}

			diagnostics := validateStage(spec, catalog, nil)
			if got, want := len(diagnostics), 1; got != want {
				t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
			}

			if got, want := diagnostics[0].Code, tc.code; got != want {
				t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
			}

			if got, want := diagnostics[0].Path, tc.path; got != want {
				t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
			}
		})
	}
}

func TestValidateRejectsMissingLocalPropertyInventoryRef(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID: "submit",
						Properties: map[string]theater.PropertySpec{
							"session": {
								Inventory: &theater.InventoryCall{
									Use: "inventory.session",
									With: map[string]theater.BindingSpec{
										"token": {
											Kind: theater.BindingKindRef,
											Ref:  &theater.RefSpec{Name: "missing_token"},
										},
									},
								},
							},
						},
						Action: theater.ActionSpec{Use: "action.submit"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	diagnostics := validateStage(spec, nil, nil)
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}

	if got, want := diagnostics[0].Code, "unresolved_binding_ref"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/scenario.login/act.submit/property.session/inventory/with/binding.token"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateRejectsPropertyKeyThatCollidesWithScenarioInput(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Inputs: map[string]theater.ValueContract{
					"token": {Kind: theater.ValueKindString},
				},
				Acts: []theater.ActSpec{
					{
						ID: "submit",
						Properties: map[string]theater.PropertySpec{
							"token": {
								Inventory: &theater.InventoryCall{
									Use: "inventory.token",
								},
							},
						},
						Action: theater.ActionSpec{Use: "action.submit"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	diagnostics := validateStage(spec, nil, nil)
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}

	if got, want := diagnostics[0].Code, "colliding_property_name"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/scenario.login/act.submit/property.token"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateRejectsMissingExpectationArgRef(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.submit"},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "status",
								Subject: theater.SubjectSpec{Field: "status"},
								Assert: theater.AssertSpec{
									Ref: "expectation.equal",
									Args: map[string]theater.BindingSpec{
										"expected": {
											Kind: theater.BindingKindRef,
											Ref:  &theater.RefSpec{Name: "missing_status"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterInventory("inventory.token", &testkit.ScriptedInventory{
		ContractValue: theater.InventoryContract{
			Produces: theater.ValueContract{Kind: theater.ValueKindString},
		},
	}); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}

	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"status": {Kind: theater.ValueKindString},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers, err := newMatcherCatalog(builtinexpectation.Descriptors()...)
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matchers)
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}

	if got, want := diagnostics[0].Code, "unresolved_binding_ref"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/scenario.login/act.submit/expectation.status/assert/binding.expected"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateAllowsExpectationArgsToReadCurrentActionOutputs(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.submit"},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "token",
								Subject: theater.SubjectSpec{Field: "token"},
								Assert: theater.AssertSpec{
									Ref: "expectation.equal",
									Args: map[string]theater.BindingSpec{
										"expected": {
											Kind: theater.BindingKindRef,
											Ref:  &theater.RefSpec{Name: "token"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterInventory("inventory.token", &testkit.ScriptedInventory{
		ContractValue: theater.InventoryContract{
			Produces: theater.ValueContract{Kind: theater.ValueKindString},
		},
	}); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}

	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers, err := newMatcherCatalog(builtinexpectation.Descriptors()...)
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	if got := len(validateStage(spec, catalog, matchers)); got != 0 {
		t.Fatalf("diagnostic count mismatch: got %d want 0", got)
	}
}

func TestValidateRejectsActExportAliasThatCollidesWithScenarioInput(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Inputs: map[string]theater.ValueContract{
					"token": {Kind: theater.ValueKindString},
				},
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.submit"},
						Exports: []theater.ExportSpec{
							{Field: "issued", As: "token"},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	diagnostics := validateStage(spec, nil, nil)
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}

	if got, want := diagnostics[0].Code, "colliding_act_export_name"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/scenario.login/act.submit/export.token"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateAllowsActExportCurrentPropertyRef(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID: "submit",
						Properties: map[string]theater.PropertySpec{
							"token": {
								Inventory: &theater.InventoryCall{Use: "inventory.token"},
							},
						},
						Action:  theater.ActionSpec{Use: "action.submit"},
						Exports: []theater.ExportSpec{{Ref: &theater.RefSpec{Name: "token"}}},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterInventory("inventory.token", &testkit.ScriptedInventory{
		ContractValue: theater.InventoryContract{
			Produces: theater.ValueContract{Kind: theater.ValueKindString},
		},
	}); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}
	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matcherCatalog(t))
	if got, want := len(diagnostics), 0; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}
}

func TestValidateRejectsUnavailableActExportRef(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:      "submit",
						Action:  theater.ActionSpec{Use: "action.submit"},
						Exports: []theater.ExportSpec{{Ref: &theater.RefSpec{Name: "token"}}},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	diagnostics := validateStage(spec, nil, matcherCatalog(t))
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}

	if got, want := diagnostics[0].Code, "unresolved_act_export_ref"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/scenario.login/act.submit/export.token"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateRejectsRefsThatAreNotGuaranteedAcrossActPaths(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "start",
						Action: theater.ActionSpec{Use: "action.start"},
						Transitions: []theater.TransitionSpec{
							{On: theater.TransitionOnPass, To: "issue"},
							{On: theater.TransitionOnFail, To: "fallback"},
						},
					},
					{
						ID:     "issue",
						Action: theater.ActionSpec{Use: "action.issue"},
						Exports: []theater.ExportSpec{
							{Field: "token", As: "issued_token"},
						},
						Transitions: []theater.TransitionSpec{
							{On: theater.TransitionOnPass, To: "verify"},
						},
					},
					{
						ID:     "fallback",
						Action: theater.ActionSpec{Use: "action.fallback"},
						Transitions: []theater.TransitionSpec{
							{On: theater.TransitionOnPass, To: "verify"},
						},
					},
					{
						ID: "verify",
						Action: theater.ActionSpec{
							Use: "action.verify",
							With: map[string]theater.BindingSpec{
								"token": {
									Kind: theater.BindingKindRef,
									Ref:  &theater.RefSpec{Name: "issued_token"},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	diagnostics := validateStage(spec, nil, nil)
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}

	if got, want := diagnostics[0].Code, "unresolved_binding_ref"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/scenario.login/act.verify/action/binding.token"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateRejectsScenarioCallExportRefMissingFromFinalScope(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "start",
						Action: theater.ActionSpec{Use: "action.start"},
						Transitions: []theater.TransitionSpec{
							{On: theater.TransitionOnPass, To: "issue"},
							{On: theater.TransitionOnFail, To: "fallback"},
						},
					},
					{
						ID:     "issue",
						Action: theater.ActionSpec{Use: "action.issue"},
						Exports: []theater.ExportSpec{
							{Field: "token", As: "issued_token"},
						},
					},
					{
						ID:     "fallback",
						Action: theater.ActionSpec{Use: "action.fallback"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{
				ID:         "login-user",
				ScenarioID: "login",
				Exports: []theater.ExportSpec{
					{Ref: &theater.RefSpec{Name: "issued_token"}, As: "final_token"},
				},
			},
		},
	}

	diagnostics := validateStage(spec, nil, matcherCatalog(t))
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}

	if got, want := diagnostics[0].Code, "unresolved_scenario_call_export_ref"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/call.login-user/export.final_token"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateRejectsExpectationSubjectDecodeThatConflictsWithOutputContract(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.submit"},
						Expectations: []theater.ExpectationSpec{
							{
								ID: "body",
								Subject: theater.SubjectSpec{
									Field:  "body",
									Decode: theater.DecodeJSON,
								},
								Assert: theater.AssertSpec{
									Ref: "expectation.equal",
									Args: map[string]theater.BindingSpec{
										"expected": {Kind: theater.BindingKindLiteral, Value: map[string]any{}},
									},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"body": {Kind: theater.ValueKindObject},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matcherCatalog(t, builtinexpectation.Descriptors()...))
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}

	if got, want := diagnostics[0].Code, "incompatible_expectation_subject_decode"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
}

func TestValidateRejectsExpectationSubjectPathThatConflictsWithOutputContract(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.submit"},
						Expectations: []theater.ExpectationSpec{
							{
								ID: "body",
								Subject: theater.SubjectSpec{
									Field: "body",
									Path:  "/token",
								},
								Assert: theater.AssertSpec{
									Ref: "expectation.equal",
									Args: map[string]theater.BindingSpec{
										"expected": {Kind: theater.BindingKindLiteral, Value: "issued-token"},
									},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"body": {Kind: theater.ValueKindString},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matcherCatalog(t, builtinexpectation.Descriptors()...))
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}

	if got, want := diagnostics[0].Code, "incompatible_expectation_subject_path"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
}

func TestValidateRejectsPropertySubjectFieldCombination(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "login",
			Acts: []theater.ActSpec{{
				ID: "submit",
				Properties: map[string]theater.PropertySpec{
					"payload": {
						Inventory: &theater.InventoryCall{Use: "inventory.payload"},
					},
				},
				Action: theater.ActionSpec{Use: "action.submit"},
				Expectations: []theater.ExpectationSpec{{
					ID: "payload",
					Subject: theater.SubjectSpec{
						From:  theater.SubjectFromProperty,
						Ref:   "payload",
						Field: "body",
					},
					Assert: theater.AssertSpec{Ref: builtinexpectation.EqualRef},
				}},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "login-user", ScenarioID: "login"}},
	}

	diagnostics := validateStage(spec, nil, nil)
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}

	if got, want := diagnostics[0].Code, "unexpected_expectation_subject_field"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
}

func TestValidateRejectsUnknownPropertySubjectRef(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "login",
			Acts: []theater.ActSpec{{
				ID:     "submit",
				Action: theater.ActionSpec{Use: "action.submit"},
				Expectations: []theater.ExpectationSpec{{
					ID: "payload",
					Subject: theater.SubjectSpec{
						From: theater.SubjectFromProperty,
						Ref:  "payload",
					},
					Assert: theater.AssertSpec{Ref: builtinexpectation.EqualRef},
				}},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "login-user", ScenarioID: "login"}},
	}

	diagnostics := validateStage(spec, nil, nil)
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}

	if got, want := diagnostics[0].Code, "unknown_expectation_subject_ref"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
}

func TestValidateAllowsUnsetNamedEnvPropertyValue(t *testing.T) {
	envName := "THEATER_TEST_VALIDATE_ENV_UNSET"
	if err := os.Unsetenv(envName); err != nil {
		t.Fatalf("unset env failed: %v", err)
	}

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "login",
			Acts: []theater.ActSpec{{
				ID: "submit",
				Properties: map[string]theater.PropertySpec{
					"email": {
						Value: &theater.BindingSpec{
							Kind: theater.BindingKindEnv,
							Env:  envName,
						},
					},
				},
				Action: theater.ActionSpec{Use: "action.submit"},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "login-user", ScenarioID: "login"}},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matcherCatalog(t))
	if got, want := len(diagnostics), 0; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}
}

func TestValidateAllowsPropertySubjectTraversalOverJSONDecodedProperty(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "login",
			Acts: []theater.ActSpec{{
				ID: "submit",
				Properties: map[string]theater.PropertySpec{
					"notifications": {
						Inventory: &theater.InventoryCall{Use: "inventory.notifications"},
						Decorators: []theater.DecoratorSpec{
							{Use: builtindecorator.JSONRef},
						},
					},
				},
				Action: theater.ActionSpec{Use: "action.submit"},
				Expectations: []theater.ExpectationSpec{{
					ID: "receiver-present",
					Subject: theater.SubjectSpec{
						From: theater.SubjectFromProperty,
						Ref:  "notifications",
						Path: "/data",
					},
					Assert: theater.AssertSpec{
						Ref: builtinexpectation.HasItemRef,
						Args: map[string]theater.BindingSpec{
							"where": {
								Kind: theater.BindingKindLiteral,
								Value: []any{
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
							},
						},
					},
				}},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "login-user", ScenarioID: "login"}},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	if err := catalog.RegisterInventory("inventory.notifications", &testkit.ScriptedInventory{
		ContractValue: theater.InventoryContract{
			Produces: theater.ValueContract{Kind: theater.ValueKindString},
		},
	}); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}

	if err := builtindecorator.Register(catalog); err != nil {
		t.Fatalf("register decorators failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matcherCatalog(t, builtinexpectation.Descriptors()...))
	if got, want := len(diagnostics), 0; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}
}

func TestValidateRejectsActionOutputNameThatCollidesWithPropertyRootInExpectationScope(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID: "submit",
						Properties: map[string]theater.PropertySpec{
							"token": {
								Inventory: &theater.InventoryCall{
									Use: "inventory.token",
								},
							},
						},
						Action: theater.ActionSpec{Use: "action.submit"},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "status",
								Subject: theater.SubjectSpec{Field: "status"},
								Assert: theater.AssertSpec{
									Ref: "expectation.equal",
									Args: map[string]theater.BindingSpec{
										"expected": {
											Kind: theater.BindingKindRef,
											Ref:  &theater.RefSpec{Name: "token"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterInventory("inventory.token", &testkit.ScriptedInventory{
		ContractValue: theater.InventoryContract{
			Produces: theater.ValueContract{Kind: theater.ValueKindString},
		},
	}); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}

	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"status": {Kind: theater.ValueKindString},
				"token":  {Kind: theater.ValueKindString},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matcherCatalog(t, builtinexpectation.Descriptors()...))
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}

	if got, want := diagnostics[0].Code, "colliding_action_output_name"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/scenario.login/act.submit/expectation.status/assert/binding.expected"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateRejectsActExportDecodeThatConflictsWithOutputContract(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.submit"},
						Exports: []theater.ExportSpec{
							{
								As:     "decoded_body",
								Field:  "body",
								Decode: theater.DecodeJSON,
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"body": {Kind: theater.ValueKindObject},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matcherCatalog(t))
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}

	if got, want := diagnostics[0].Code, "incompatible_act_export_decode"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
}

func TestValidateRejectsActExportPathThatConflictsWithScalarOutputContract(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.submit"},
						Exports: []theater.ExportSpec{
							{
								As:    "issued_token",
								Field: "body",
								Path:  "/token",
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"body": {Kind: theater.ValueKindString},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matcherCatalog(t))
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}

	if got, want := diagnostics[0].Code, "incompatible_act_export_path"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
}

func TestValidateRejectsActExportPathThatConflictsWithScalarPropertyContract(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID: "submit",
						Properties: map[string]theater.PropertySpec{
							"payload": {
								Inventory: &theater.InventoryCall{Use: "inventory.payload"},
							},
						},
						Action: theater.ActionSpec{Use: "action.submit"},
						Exports: []theater.ExportSpec{
							{
								As:  "issued_token",
								Ref: &theater.RefSpec{Name: "payload", Path: theater.JSONPointer("/token")},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterInventory("inventory.payload", &testkit.ScriptedInventory{
		ContractValue: theater.InventoryContract{
			Produces: theater.ValueContract{Kind: theater.ValueKindString},
		},
	}); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}
	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matcherCatalog(t))
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}

	if got, want := diagnostics[0].Code, "incompatible_act_export_path"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/scenario.login/act.submit/export.issued_token"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateRejectsActExportPathThatConflictsWithScalarPropertyValueContract(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID: "submit",
						Properties: map[string]theater.PropertySpec{
							"payload": {
								Value: &theater.BindingSpec{
									Kind:  theater.BindingKindLiteral,
									Value: "token",
								},
							},
						},
						Action: theater.ActionSpec{Use: "action.submit"},
						Exports: []theater.ExportSpec{
							{
								As:  "issued_token",
								Ref: &theater.RefSpec{Name: "payload", Path: theater.JSONPointer("/token")},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matcherCatalog(t))
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}

	if got, want := diagnostics[0].Code, "incompatible_act_export_path"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
}

func TestValidateRejectsActExportPathThatConflictsWithScalarScenarioInputContract(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Inputs: map[string]theater.ValueContract{
					"payload": {Kind: theater.ValueKindString},
				},
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.submit"},
						Exports: []theater.ExportSpec{
							{
								As:  "issued_token",
								Ref: &theater.RefSpec{Name: "payload", Path: theater.JSONPointer("/token")},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{
				ID:         "login-user",
				ScenarioID: "login",
				Bindings: map[string]theater.BindingSpec{
					"payload": {Kind: theater.BindingKindLiteral, Value: "raw-token"},
				},
			},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matcherCatalog(t))
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}

	if got, want := diagnostics[0].Code, "incompatible_act_export_path"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/scenario.login/act.submit/export.issued_token"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateRejectsActExportPathThatConflictsWithScalarPriorExportContract(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:      "issue",
						Action:  theater.ActionSpec{Use: "action.issue"},
						Exports: []theater.ExportSpec{{As: "payload", Field: "payload"}},
						Transitions: []theater.TransitionSpec{
							{On: theater.TransitionOnPass, To: "submit"},
						},
					},
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.submit"},
						Exports: []theater.ExportSpec{
							{
								As:  "issued_token",
								Ref: &theater.RefSpec{Name: "payload", Path: theater.JSONPointer("/token")},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.issue", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"payload": {Kind: theater.ValueKindString},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matcherCatalog(t))
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}

	if got, want := diagnostics[0].Code, "incompatible_act_export_path"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}

	if got, want := diagnostics[0].Path, "stage.main/scenario.login/act.submit/export.issued_token"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidateAcceptsSelectorTransformThroughPipeline(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "login",
			Acts: []theater.ActSpec{{
				ID:     "submit",
				Action: theater.ActionSpec{Use: "action.submit"},
				Expectations: []theater.ExpectationSpec{{
					ID: "uid",
					Subject: theater.SubjectSpec{
						Field: "token",
						Through: []theater.ThroughStepSpec{
							{
								Transform: &theater.DecoratorSpec{
									Use: "transform.jwt.claims",
								},
							},
							{Path: "/uid"},
						},
					},
					Assert: theater.AssertSpec{
						Ref: builtinexpectation.EqualRef,
						Args: map[string]theater.BindingSpec{
							"expected": literalBinding("user-123"),
						},
					},
				}},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	if err := catalog.RegisterDecorator("transform.jwt.claims", theater.DecoratorDef{
		Contract: theater.DecoratorContract{
			Accepts: theater.ValueContract{Kind: theater.ValueKindString},
			Produces: theater.ValueContract{
				Kind: theater.ValueKindObject,
				Fields: map[string]theater.ValueContract{
					"uid": {Kind: theater.ValueKindString},
				},
			},
		},
		Compile: func(theater.Values) (theater.DecoratorFunc, error) {
			return func(value any) (any, error) { return value, nil }, nil
		},
	}); err != nil {
		t.Fatalf("register decorator failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matcherCatalog(t, builtinexpectation.Descriptors()...))
	if got := len(diagnostics); got != 0 {
		t.Fatalf("diagnostic count mismatch: got %d want 0 (%v)", got, diagnostics)
	}
}

func TestValidateRejectsSelectorTransformInputContractMismatch(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "login",
			Acts: []theater.ActSpec{{
				ID:     "submit",
				Action: theater.ActionSpec{Use: "action.submit"},
				Expectations: []theater.ExpectationSpec{{
					ID: "uid",
					Subject: theater.SubjectSpec{
						Field: "token",
						Through: []theater.ThroughStepSpec{{
							Transform: &theater.DecoratorSpec{
								Use: "transform.object-only",
							},
						}},
					},
					Assert: theater.AssertSpec{Ref: builtinexpectation.PresentRef},
				}},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	if err := catalog.RegisterDecorator("transform.object-only", theater.DecoratorDef{
		Contract: theater.DecoratorContract{
			Accepts:  theater.ValueContract{Kind: theater.ValueKindObject},
			Produces: theater.ValueContract{Kind: theater.ValueKindObject},
		},
		Compile: func(theater.Values) (theater.DecoratorFunc, error) {
			return func(value any) (any, error) { return value, nil }, nil
		},
	}); err != nil {
		t.Fatalf("register decorator failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matcherCatalog(t, builtinexpectation.Descriptors()...))
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}
	if got, want := diagnostics[0].Code, "incompatible_expectation_subject_transform"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if !strings.Contains(diagnostics[0].Summary, `kind "string" is not accepted`) {
		t.Fatalf("diagnostic summary mismatch: got %q", diagnostics[0].Summary)
	}
}

func TestValidateRejectsBindingSelectorTransformConfig(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "login",
			Inputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString},
			},
			Acts: []theater.ActSpec{{
				ID: "submit",
				Action: theater.ActionSpec{
					Use: "action.submit",
					With: map[string]theater.BindingSpec{
						"value": {
							Kind: theater.BindingKindRef,
							Ref: &theater.RefSpec{
								Name: "token",
								Through: []theater.ThroughStepSpec{{
									Transform: &theater.DecoratorSpec{Use: "transform.wrap"},
								}},
							},
						},
					},
				},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{
			ID:         "login",
			ScenarioID: "login",
			Bindings: map[string]theater.BindingSpec{
				"token": literalBinding("issued"),
			},
		}},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Inputs: map[string]theater.ValueContract{
				"value": {Kind: theater.ValueKindString},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	if err := catalog.RegisterDecorator("transform.wrap", theater.DecoratorDef{
		Contract: theater.DecoratorContract{
			Accepts:  theater.ValueContract{Kind: theater.ValueKindString},
			Produces: theater.ValueContract{Kind: theater.ValueKindString},
			Params: []theater.ParamSpec{
				{Name: "prefix", Accepts: theater.ValueContract{Kind: theater.ValueKindString}, Required: true},
			},
		},
		Compile: func(theater.Values) (theater.DecoratorFunc, error) {
			return func(value any) (any, error) { return value, nil }, nil
		},
	}); err != nil {
		t.Fatalf("register decorator failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matcherCatalog(t, builtinexpectation.Descriptors()...))
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}
	if got, want := diagnostics[0].Code, "incompatible_action_arg"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if !strings.Contains(diagnostics[0].Summary, `transform "transform.wrap" config is invalid`) {
		t.Fatalf("diagnostic summary mismatch: got %q", diagnostics[0].Summary)
	}
}

func TestValidateRejectsBindingSelectorTransformOutputContractMismatch(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "login",
			Inputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString},
			},
			Acts: []theater.ActSpec{{
				ID: "submit",
				Action: theater.ActionSpec{
					Use: "action.submit",
					With: map[string]theater.BindingSpec{
						"value": {
							Kind: theater.BindingKindRef,
							Ref: &theater.RefSpec{
								Name: "token",
								Through: []theater.ThroughStepSpec{{
									Transform: &theater.DecoratorSpec{Use: "transform.claims"},
								}},
							},
						},
					},
				},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{
			ID:         "login",
			ScenarioID: "login",
			Bindings: map[string]theater.BindingSpec{
				"token": literalBinding("issued"),
			},
		}},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Inputs: map[string]theater.ValueContract{
				"value": {Kind: theater.ValueKindString},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	if err := catalog.RegisterDecorator("transform.claims", theater.DecoratorDef{
		Contract: theater.DecoratorContract{
			Accepts:  theater.ValueContract{Kind: theater.ValueKindString},
			Produces: theater.ValueContract{Kind: theater.ValueKindObject},
		},
		Compile: func(theater.Values) (theater.DecoratorFunc, error) {
			return func(value any) (any, error) { return value, nil }, nil
		},
	}); err != nil {
		t.Fatalf("register decorator failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matcherCatalog(t, builtinexpectation.Descriptors()...))
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}
	if got, want := diagnostics[0].Code, "incompatible_action_arg"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if !strings.Contains(diagnostics[0].Summary, `selector produces object incompatible with string`) {
		t.Fatalf("diagnostic summary mismatch: got %q", diagnostics[0].Summary)
	}
}

func TestValidateRejectsActExportPathThatConflictsWithListOutputContract(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.submit"},
						Exports: []theater.ExportSpec{
							{
								As:    "first_item",
								Field: "items",
								Path:  "/bad",
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.submit", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"items": {Kind: theater.ValueKindList},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	diagnostics := validateStage(spec, catalog, matcherCatalog(t))
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d (%v)", got, want, diagnostics)
	}

	if got, want := diagnostics[0].Code, "incompatible_act_export_path"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
}

func plainDataMatcherDescriptor(ref string, argName string, accepts theater.ValueContract) theater.MatcherDescriptor {
	return theater.MatcherDescriptor{
		Ref:    ref,
		Actual: theater.ValueContract{Kind: theater.ValueKindAny},
		Sugar:  theater.SugarSpec{Form: theater.SugarFormNone},
		Args: []theater.MatcherArg{{
			Name:     argName,
			Required: true,
			Accepts:  accepts,
		}},
		Compile: func(theater.MatcherCompileContext, theater.Values) (theater.Matcher, error) {
			return noopMatcher{}, nil
		},
	}
}
