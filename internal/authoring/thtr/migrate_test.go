package thtr

import (
	"reflect"
	"strings"
	"testing"

	goyaml "gopkg.in/yaml.v3"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/builtin"
	builtinexpectation "github.com/alex-poliushkin/theater/builtin/expectation"
)

func TestMarshalStageUsesSafeExpectationSugarAndRoundTrips(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID:   "smoke",
		Name: "Smoke stage",
		HTTP: &theater.HTTPSpec{
			Sessions: map[string]theater.HTTPSessionSpec{
				"browser": {},
			},
			Auth: map[string]theater.HTTPAuthSpec{
				"web": {
					Attach: []theater.HTTPAuthAttachmentSpec{
						{
							HeaderSlot: &theater.HTTPHeaderSlotAuthSpec{
								Name: "X-CSRF-Token",
								Slot: "csrf",
							},
						},
					},
				},
			},
			Identities: map[string]theater.HTTPIdentitySpec{
				"user": {
					Session: "browser",
					Auth:    "web",
				},
			},
		},
		State: &theater.StateSpec{
			Backends: map[string]theater.StateBackendSpec{
				"local": {
					Use: "state.backend.file",
					With: map[string]any{
						"root": "/tmp/theater-state",
						"metadata": map[string]any{
							"@type":        "FileBackend",
							"profile.name": "local",
						},
					},
				},
			},
		},
		Scenarios: []theater.ScenarioSpec{
			{
				ID:   "auth/login",
				Name: "Login scenario",
				Inputs: map[string]theater.ValueContract{
					"email": {
						Kind:     theater.ValueKindString,
						Required: true,
					},
				},
				Acts: []theater.ActSpec{
					{
						ID:   "submit",
						Name: "Submit request",
						Action: theater.ActionSpec{
							Use: "action.http",
							With: map[string]theater.BindingSpec{
								"method": literalBinding("POST"),
								"url": stringBinding(
									literalBinding("https://example.test/login?email="),
									refBinding("email"),
								),
								"headers": objectBinding(map[string]theater.BindingSpec{
									"Accept":       literalBinding("application/json"),
									"Content-Type": literalBinding("application/json"),
									"@type":        literalBinding("LoginRequest"),
								}),
							},
						},
						CaptureAuth: &theater.HTTPAuthCaptureSpec{
							Auth: "web",
							Slots: map[string]theater.HTTPCaptureSourceSpec{
								"csrf": {
									ResponseHeader: "X-CSRF-Token",
								},
							},
						},
						Expectations: []theater.ExpectationSpec{
							expectationSpec("status-ok", fieldSubject("status_code"), unaryAssert(builtinexpectation.EqualRef, "expected", numericBinding(200))),
							expectationSpec(
								"not-not-found",
								fieldSubject("status_code"),
								theater.AssertSpec{
									Ref: builtinexpectation.NotRef,
									Args: map[string]theater.BindingSpec{
										"assert": nestedAssertBinding(unaryAssert(
											builtinexpectation.EqualRef,
											"expected",
											numericBinding(404),
										)),
									},
								},
							),
							expectationSpec("page-text", fieldSubject("body"), unaryAssert(builtinexpectation.ContainsRef, "expected", literalBinding("Example Domain"))),
							expectationSpec("latency-high", fieldSubject("duration_ms"), unaryAssert(builtinexpectation.GTRef, "expected", numericBinding(100))),
							expectationSpec("retry-budget", fieldSubject("retry_count"), unaryAssert(builtinexpectation.LTERef, "expected", numericBinding(5))),
							expectationSpec(
								"has-token",
								subjectWithPath("body", theater.DecodeJSON, "/data"),
								unaryAssert(builtinexpectation.HasKeyRef, "key", literalBinding("token")),
							),
							expectationSpec(
								"no-error-key",
								subjectWithPath("body", theater.DecodeJSON, ""),
								unaryAssert(builtinexpectation.LacksKeyRef, "key", literalBinding("error")),
							),
							expectationSpec(
								"deleted-null",
								subjectWithPath("body", theater.DecodeJSON, "/deleted_at"),
								theater.AssertSpec{Ref: builtinexpectation.NullRef},
							),
							expectationSpec(
								"trace-present",
								subjectWithPath("body", theater.DecodeJSON, "/trace_id"),
								theater.AssertSpec{Ref: builtinexpectation.PresentRef},
							),
							expectationSpec(
								"name-not-null",
								subjectWithPath("body", theater.DecodeJSON, "/name"),
								theater.AssertSpec{Ref: builtinexpectation.NotNullRef},
							),
							expectationSpec(
								"duration-window",
								fieldSubject("duration_ms"),
								theater.AssertSpec{
									Ref: builtinexpectation.BetweenRef,
									Args: map[string]theater.BindingSpec{
										"min": numericBinding(200),
										"max": numericBinding(299),
									},
								},
							),
						},
						Exports: []theater.ExportSpec{
							{
								As:     "session_id",
								Field:  "body",
								Decode: theater.DecodeJSON,
								Path:   theater.JSONPointer("/session/id"),
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{
				ID:         "run-login",
				Name:       "Run login",
				ScenarioID: "auth/login",
				Bindings: map[string]theater.BindingSpec{
					"email": {
						Kind:      theater.BindingKindGenerate,
						Generator: "email",
						Args: map[string]theater.BindingSpec{
							"domain": literalBinding("example.test"),
						},
					},
				},
				Dependencies: []theater.ScenarioDependencySpec{
					{CallID: "bootstrap", When: theater.TriggerPredicateDone},
				},
				Exports: []theater.ExportSpec{
					{
						As: "final_session_id",
						Ref: &theater.RefSpec{
							Name: "session_id",
						},
					},
				},
			},
		},
	}

	got, err := MarshalStage(spec)
	if err != nil {
		t.Fatalf("marshal stage failed: %v", err)
	}

	text := string(got)
	for _, want := range []string{
		`expect status-ok: field(status_code) == 200`,
		`headers: object {`,
		`"@type": "LoginRequest"`,
		`Accept: "application/json"`,
		`Content-Type: "application/json"`,
		`metadata: object { "@type": "FileBackend", "profile.name": "local" }`,
		`expect not-not-found: field(status_code) assert expectation.not(`,
		`ref: "expectation.equal"`,
		`expected: 404`,
		`expect page-text: field(body) contains "Example Domain"`,
		`expect latency-high: field(duration_ms) > 100`,
		`expect retry-budget: field(retry_count) <= 5`,
		`expect has-token: field(body) | decode(json) | path("/data") has key("token")`,
		`expect no-error-key: field(body) | decode(json) lacks key("error")`,
		`expect deleted-null: field(body) | decode(json) | path("/deleted_at") is null`,
		`expect trace-present: field(body) | decode(json) | path("/trace_id") is present`,
		`expect name-not-null: field(body) | decode(json) | path("/name") is not null`,
		`expect duration-window: field(duration_ms) between 200 and 299`,
		`capture_auth web`,
		`call run-login = auth/login(email: generate.email(domain: "example.test"))`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("migrated source must include %q:\n%s", want, text)
		}
	}

	formatted, err := Format(got)
	if err != nil {
		t.Fatalf("format migrated source failed: %v", err)
	}
	if string(formatted) != text {
		t.Fatalf("migrated source must already be formatter-clean:\n--- got ---\n%s\n--- fmt ---\n%s", text, string(formatted))
	}

	roundTripped, err := Parse(got, nil)
	if err != nil {
		t.Fatalf("parse migrated source failed: %v", err)
	}
	requireSemanticStageYAMLEqual(t, roundTripped, spec)
}

func TestMarshalStageRendersDynamicBearerAuthBinding(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "mobile-dashboard",
		HTTP: &theater.HTTPSpec{
			Auth: map[string]theater.HTTPAuthSpec{
				"mobile_api": {Attach: []theater.HTTPAuthAttachmentSpec{{
					Bearer: &theater.HTTPBearerAuthSpec{TokenSlot: "access_token"},
				}}},
			},
		},
		Scenarios: []theater.ScenarioSpec{{
			ID: "mobile/dashboard-ready",
			Inputs: map[string]theater.ValueContract{
				"access_token": {Kind: theater.ValueKindString, Required: true},
			},
			AuthBindings: map[string]theater.HTTPAuthBindingSpec{
				"mobile_api": {
					Slots: map[string]theater.BindingSpec{
						"access_token": refBinding("access_token"),
					},
				},
			},
			Acts: []theater.ActSpec{{
				ID: "wait-customer",
				Action: theater.ActionSpec{
					Use: "action.http",
					With: map[string]theater.BindingSpec{
						"url":  literalBinding("https://gateway.example.test/customer"),
						"auth": literalBinding("mobile_api"),
					},
				},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{
			ID:         "run-dashboard",
			ScenarioID: "mobile/dashboard-ready",
			Bindings: map[string]theater.BindingSpec{
				"access_token": literalBinding("issued-token"),
			},
		}},
	}

	got, err := MarshalStage(spec)
	if err != nil {
		t.Fatalf("marshal stage failed: %v", err)
	}

	text := string(got)
	for _, want := range []string{
		`object { bearer: object { token_slot: "access_token" } }`,
		`bind auth mobile_api`,
		`access_token: $access_token`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("migrated source must include %q:\n%s", want, text)
		}
	}

	roundTripped, err := Parse(got, nil)
	if err != nil {
		t.Fatalf("parse migrated source failed: %v", err)
	}
	requireSemanticStageYAMLEqual(t, roundTripped, spec)
}

func TestMarshalStageKeepsCollectionMatchersCanonical(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "collections",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "mail/check",
				Acts: []theater.ActSpec{
					{
						ID: "assert-mail",
						Action: theater.ActionSpec{
							Use: "action.http",
							With: map[string]theater.BindingSpec{
								"method": literalBinding("GET"),
								"url":    literalBinding("https://example.test/messages"),
							},
						},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "has-recipient",
								Subject: subjectWithPath("body", theater.DecodeJSON, "/notifications"),
								Assert: theater.AssertSpec{
									Ref: builtinexpectation.HasItemRef,
									Args: map[string]theater.BindingSpec{
										"where": {
											Kind: theater.BindingKindList,
											List: []theater.BindingSpec{
												objectBinding(map[string]theater.BindingSpec{
													"subject": objectBinding(map[string]theater.BindingSpec{
														"path": literalBinding("/receiverAddress"),
													}),
													"assert": nestedAssertBinding(theater.AssertSpec{
														Ref: builtinexpectation.EqualRef,
														Args: map[string]theater.BindingSpec{
															"expected": literalBinding("demo@example.test"),
														},
													}),
												}),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	got, err := MarshalStage(spec)
	if err != nil {
		t.Fatalf("marshal stage failed: %v", err)
	}

	text := string(got)
	if !strings.Contains(text, `assert expectation.has_item(`) {
		t.Fatalf("collection matcher must stay canonical until explicitly rewritten:\n%s", text)
	}
	if strings.Contains(text, " has item where ") {
		t.Fatalf("collection matcher must not be auto-rewritten in the migrator yet:\n%s", text)
	}

	roundTripped, err := Parse(got, nil)
	if err != nil {
		t.Fatalf("parse migrated source failed: %v", err)
	}
	requireSemanticStageYAMLEqual(t, roundTripped, spec)
}

func TestMarshalStagePreservesActRefExportSelector(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "ref-export",
		Scenarios: []theater.ScenarioSpec{{
			ID: "profile",
			Acts: []theater.ActSpec{{
				ID:     "load",
				Action: theater.ActionSpec{Use: "action.noop"},
				Exports: []theater.ExportSpec{{
					As: "profile_id",
					Ref: &theater.RefSpec{
						Name: "profile",
						Path: theater.JSONPointer("/id"),
					},
				}},
			}},
		}},
	}

	got, err := MarshalStage(spec)
	if err != nil {
		t.Fatalf("marshal stage failed: %v", err)
	}

	text := string(got)
	if !strings.Contains(text, `export profile_id = $profile | path("/id")`) {
		t.Fatalf("ref export selector must be preserved:\n%s", text)
	}

	roundTripped, err := Parse(got, nil)
	if err != nil {
		t.Fatalf("parse migrated source failed: %v", err)
	}
	requireSemanticStageYAMLEqual(t, roundTripped, spec)
}

func TestMarshalStageRejectsUnrepresentableSplitActRefExportSelector(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "split-ref-export",
		Scenarios: []theater.ScenarioSpec{{
			ID: "profile",
			Acts: []theater.ActSpec{{
				ID:     "load",
				Action: theater.ActionSpec{Use: "action.noop"},
				Exports: []theater.ExportSpec{{
					As: "profile",
					Ref: &theater.RefSpec{
						Name: "raw",
						Path: theater.JSONPointer("/payload"),
					},
					Decode: theater.DecodeJSON,
				}},
			}},
		}},
	}

	_, err := MarshalStage(spec)
	if err == nil {
		t.Fatal("marshal stage must reject unrepresentable split ref export selector")
	}
	if !strings.Contains(err.Error(), "export-level decode") {
		t.Fatalf("marshal error mismatch: %v", err)
	}
}

func TestMarshalStageRendersHasEntrySugar(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "object-entry",
		Scenarios: []theater.ScenarioSpec{{
			ID: "profile",
			Acts: []theater.ActSpec{{
				ID:     "assert-profile",
				Action: theater.ActionSpec{Use: "action.http"},
				Expectations: []theater.ExpectationSpec{{
					ID:      "active-status",
					Subject: subjectWithPath("body", theater.DecodeJSON, ""),
					Assert: theater.AssertSpec{
						Ref: builtinexpectation.HasEntryRef,
						Args: map[string]theater.BindingSpec{
							"key": literalBinding("status"),
							"assert": nestedAssertBinding(theater.AssertSpec{
								Ref: builtinexpectation.EqualRef,
								Args: map[string]theater.BindingSpec{
									"expected": literalBinding("active"),
								},
							}),
						},
					},
				}},
			}},
		}},
	}

	got, err := MarshalStage(spec)
	if err != nil {
		t.Fatalf("marshal stage failed: %v", err)
	}

	text := string(got)
	if !strings.Contains(text, `expect active-status: field(body) | decode(json) has entry("status") == "active"`) {
		t.Fatalf("has_entry matcher must render as concise .thtr sugar:\n%s", text)
	}

	roundTripped, err := Parse(got, nil)
	if err != nil {
		t.Fatalf("parse migrated source failed: %v", err)
	}
	requireSemanticStageYAMLEqual(t, roundTripped, spec)
}

func TestMarshalStageRendersPickWhereSelector(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "pick-where",
		Scenarios: []theater.ScenarioSpec{{
			ID: "mail/wait",
			Inputs: map[string]theater.ValueContract{
				"email": {Kind: theater.ValueKindString, Required: true},
			},
			Acts: []theater.ActSpec{{
				ID: "poll",
				Action: theater.ActionSpec{
					Use: "action.http",
					With: map[string]theater.BindingSpec{
						"method": literalBinding("GET"),
						"url":    literalBinding("https://example.test/messages"),
					},
				},
				Exports: []theater.ExportSpec{{
					As:     "otp",
					Field:  "body",
					Decode: theater.DecodeJSON,
					Path:   theater.JSONPointer("/items"),
					Through: []theater.ThroughStepSpec{
						{
							Pick: &theater.PickStepSpec{
								Where: []theater.PickWhereClauseSpec{
									{
										Subject: theater.RelativeSubjectSpec{Path: theater.JSONPointer("/receiverAddress")},
										Assert: theater.AssertSpec{
											Ref: builtinexpectation.EqualRef,
											Args: map[string]theater.BindingSpec{
												"expected": refBinding("email"),
											},
										},
									},
									{
										Subject: theater.RelativeSubjectSpec{Path: theater.JSONPointer("/subject")},
										Assert: theater.AssertSpec{
											Ref: builtinexpectation.ContainsRef,
											Args: map[string]theater.BindingSpec{
												"expected": literalBinding("Verification"),
											},
										},
									},
								},
							},
						},
						{Path: theater.JSONPointer("/body")},
					},
				}},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{
			ID:         "wait-for-mail",
			ScenarioID: "mail/wait",
			Bindings: map[string]theater.BindingSpec{
				"email": literalBinding("demo@example.test"),
			},
		}},
	}

	got, err := MarshalStage(spec)
	if err != nil {
		t.Fatalf("marshal stage failed: %v", err)
	}

	text := string(got)
	for _, want := range []string{
		`pick where (`,
		`path("/receiverAddress") == $email`,
		`path("/subject") contains "Verification"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("migrated source must include %q:\n%s", want, text)
		}
	}

	formatted, err := Format(got)
	if err != nil {
		t.Fatalf("format migrated source failed: %v", err)
	}
	if string(formatted) != text {
		t.Fatalf("migrated source must already be formatter-clean:\n--- got ---\n%s\n--- fmt ---\n%s", text, string(formatted))
	}

	roundTripped, err := Parse(got, nil)
	if err != nil {
		t.Fatalf("parse migrated source failed: %v", err)
	}
	requireSemanticStageYAMLEqual(t, roundTripped, spec)
}

func TestMarshalStageRendersTransformSelector(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "transform-selector",
		Scenarios: []theater.ScenarioSpec{{
			ID: "jwt",
			Acts: []theater.ActSpec{{
				ID: "inspect",
				Action: theater.ActionSpec{
					Use: "action.http",
					With: map[string]theater.BindingSpec{
						"method": literalBinding("GET"),
						"url":    literalBinding("https://example.test/token"),
					},
				},
				Expectations: []theater.ExpectationSpec{{
					ID: "uid",
					Subject: theater.SubjectSpec{
						Field:  "body",
						Decode: theater.DecodeJSON,
						Path:   theater.JSONPointer("/token"),
						Through: []theater.ThroughStepSpec{
							{
								Transform: &theater.DecoratorSpec{
									Use: "transform.jwt.claims",
									With: map[string]any{
										"audience": "mobile",
									},
								},
							},
							{Path: theater.JSONPointer("/uid")},
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
	}

	got, err := MarshalStage(spec)
	if err != nil {
		t.Fatalf("marshal stage failed: %v", err)
	}

	text := string(got)
	want := `field(body) | decode(json) | path("/token") | transform.jwt.claims(audience: "mobile") | path("/uid") == "user-123"`
	if !strings.Contains(text, want) {
		t.Fatalf("migrated source must include transform selector:\n%s", text)
	}

	roundTripped, err := Parse(got, nil)
	if err != nil {
		t.Fatalf("parse migrated source failed: %v", err)
	}
	requireSemanticStageYAMLEqual(t, roundTripped, spec)
}

func TestMarshalStageHoistsRepeatedStateHandlesConservatively(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "state-migrate",
		State: &theater.StateSpec{
			Backends: map[string]theater.StateBackendSpec{
				"local": {
					Use: "state.backend.file",
					With: map[string]any{
						"root": "/tmp/theater-state",
					},
				},
			},
		},
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "state/demo",
				Acts: []theater.ActSpec{
					{
						ID: "read-shared",
						Properties: map[string]theater.PropertySpec{
							"shared_record": {
								Inventory: &theater.InventoryCall{
									Use: "inventory.state.record",
									With: map[string]theater.BindingSpec{
										"backend":       literalBinding("local"),
										"record":        literalBinding("env/shared-meta"),
										"min_guarantee": literalBinding("local-atomic"),
									},
								},
							},
						},
						Action: theater.ActionSpec{
							Use: "action.state.read",
							With: map[string]theater.BindingSpec{
								"record": refBinding("shared_record"),
							},
						},
						Exports: []theater.ExportSpec{{
							As:    "shared_record_version",
							Field: "version",
						}},
					},
					{
						ID: "update-shared",
						Properties: map[string]theater.PropertySpec{
							"shared_record": {
								Inventory: &theater.InventoryCall{
									Use: "inventory.state.record",
									With: map[string]theater.BindingSpec{
										"backend":       literalBinding("local"),
										"record":        literalBinding("env/shared-meta"),
										"min_guarantee": literalBinding("local-atomic"),
									},
								},
							},
						},
						Action: theater.ActionSpec{
							Use: "action.state.update",
							With: map[string]theater.BindingSpec{
								"record":           refBinding("shared_record"),
								"expected_version": refBinding("shared_record_version"),
								"value": objectBinding(map[string]theater.BindingSpec{
									"owner": literalBinding("tutorial-run"),
								}),
							},
						},
					},
					{
						ID: "claim-shared",
						Properties: map[string]theater.PropertySpec{
							"otp_pool": {
								Inventory: &theater.InventoryCall{
									Use: "inventory.state.pool",
									With: map[string]theater.BindingSpec{
										"backend":       literalBinding("local"),
										"pool":          literalBinding("otp-identities"),
										"min_guarantee": literalBinding("local-atomic"),
									},
								},
							},
						},
						Action: theater.ActionSpec{
							Use: "action.state.claim",
							With: map[string]theater.BindingSpec{
								"pool": refBinding("otp_pool"),
								"selector": objectBinding(map[string]theater.BindingSpec{
									"fields": objectBinding(map[string]theater.BindingSpec{
										"purpose": literalBinding("registration"),
									}),
								}),
								"lease": objectBinding(map[string]theater.BindingSpec{
									"ttl":       literalBinding("5m"),
									"on_expiry": literalBinding("reclaim"),
								}),
							},
						},
						Exports: []theater.ExportSpec{{
							As:    "otp_claim",
							Field: "claim",
						}},
					},
					{
						ID: "claim-shared-again",
						Properties: map[string]theater.PropertySpec{
							"otp_pool": {
								Inventory: &theater.InventoryCall{
									Use: "inventory.state.pool",
									With: map[string]theater.BindingSpec{
										"backend":       literalBinding("local"),
										"pool":          literalBinding("otp-identities"),
										"min_guarantee": literalBinding("local-atomic"),
									},
								},
							},
						},
						Action: theater.ActionSpec{
							Use: "action.state.claim",
							With: map[string]theater.BindingSpec{
								"pool": refBinding("otp_pool"),
								"selector": objectBinding(map[string]theater.BindingSpec{
									"fields": objectBinding(map[string]theater.BindingSpec{
										"purpose": literalBinding("registration"),
									}),
								}),
								"lease": objectBinding(map[string]theater.BindingSpec{
									"ttl": literalBinding("5m"),
								}),
							},
						},
					},
					{
						ID: "read-oneoff",
						Properties: map[string]theater.PropertySpec{
							"temp_record": {
								Inventory: &theater.InventoryCall{
									Use: "inventory.state.record",
									With: map[string]theater.BindingSpec{
										"backend": literalBinding("local"),
										"record":  literalBinding("env/temporary"),
									},
								},
							},
						},
						Action: theater.ActionSpec{
							Use: "action.state.read",
							With: map[string]theater.BindingSpec{
								"record": refBinding("temp_record"),
							},
						},
					},
				},
			},
		},
	}

	got, err := MarshalStage(spec)
	if err != nil {
		t.Fatalf("marshal stage failed: %v", err)
	}

	text := string(got)
	for _, want := range []string{
		`record shared_record = state.record`,
		`pool otp_pool = state.pool`,
		`do state.read(record: shared_record)`,
		`do state.update`,
		`if_version: $shared_record_version`,
		`do state.claim`,
		`fields: object { purpose: "registration" }`,
		`lease: object { on_expiry: reclaim, ttl: 5m }`,
		`prop temp_record = inventory.state.record(`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("migrated source must include %q:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{
		`prop shared_record = inventory.state.record`,
		`prop otp_pool = inventory.state.pool`,
	} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("migrated source must rewrite repeated state handle %q:\n%s", unwanted, text)
		}
	}
	if strings.Contains(text, `record temp_record = state.record`) {
		t.Fatalf("migrated source must not hoist one-off state handle:\n%s", text)
	}

	formatted, err := Format(got)
	if err != nil {
		t.Fatalf("format migrated source failed: %v", err)
	}
	if string(formatted) != text {
		t.Fatalf("migrated source must already be formatter-clean:\n--- got ---\n%s\n--- fmt ---\n%s", text, string(formatted))
	}

	roundTripped, err := Parse(got, nil)
	if err != nil {
		t.Fatalf("parse migrated source failed: %v", err)
	}

	bundle, err := builtin.NewBundle()
	if err != nil {
		t.Fatalf("new builtin bundle failed: %v", err)
	}
	validator := theater.NewValidator(bundle.Catalog, bundle.Matchers)
	if diagnostics := validator.Validate(roundTripped); len(diagnostics) != 0 {
		t.Fatalf("migrated source must validate cleanly: %#v", diagnostics)
	}
}

func TestInventoryCallsEquivalentIgnoresSourceSpans(t *testing.T) {
	t.Parallel()

	left := callExpressionSyntax{
		Name: migrateStateRecordInventory,
		Args: []callArgumentSyntax{
			{
				Name: migrateStateBackendArg,
				Value: literalExpressionSyntax{
					Kind: literalKindString,
					Text: `"local"`,
					Span: sourceSpan{
						Start: sourcePosition{Line: 1, Column: 1, Offset: 0},
						End:   sourcePosition{Line: 1, Column: 8, Offset: 7},
					},
				},
				Span: sourceSpan{
					Start: sourcePosition{Line: 1, Column: 1, Offset: 0},
					End:   sourcePosition{Line: 1, Column: 8, Offset: 7},
				},
			},
			{
				Name: migrateStateRecordArg,
				Value: literalExpressionSyntax{
					Kind: literalKindString,
					Text: `"env/shared-meta"`,
					Span: sourceSpan{
						Start: sourcePosition{Line: 1, Column: 10, Offset: 9},
						End:   sourcePosition{Line: 1, Column: 28, Offset: 27},
					},
				},
			},
		},
		Span: sourceSpan{
			Start: sourcePosition{Line: 1, Column: 1, Offset: 0},
			End:   sourcePosition{Line: 1, Column: 29, Offset: 28},
		},
	}
	right := callExpressionSyntax{
		Name: migrateStateRecordInventory,
		Args: []callArgumentSyntax{
			{
				Name: migrateStateBackendArg,
				Value: literalExpressionSyntax{
					Kind: literalKindString,
					Text: `"local"`,
					Span: sourceSpan{
						Start: sourcePosition{Line: 20, Column: 3, Offset: 200},
						End:   sourcePosition{Line: 20, Column: 10, Offset: 207},
					},
				},
				Span: sourceSpan{
					Start: sourcePosition{Line: 20, Column: 3, Offset: 200},
					End:   sourcePosition{Line: 20, Column: 10, Offset: 207},
				},
			},
			{
				Name: migrateStateRecordArg,
				Value: literalExpressionSyntax{
					Kind: literalKindString,
					Text: `"env/shared-meta"`,
					Span: sourceSpan{
						Start: sourcePosition{Line: 20, Column: 12, Offset: 209},
						End:   sourcePosition{Line: 20, Column: 30, Offset: 227},
					},
				},
			},
		},
		Span: sourceSpan{
			Start: sourcePosition{Line: 20, Column: 1, Offset: 198},
			End:   sourcePosition{Line: 20, Column: 31, Offset: 228},
		},
	}

	if !inventoryCallsEquivalent(left, right) {
		t.Fatalf("inventory call equivalence must ignore source spans")
	}
}

func TestMarshalStageKeepsRepeatedStateHandleExplicitWhenActUsesRefOutsideAction(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "state-migrate-guard",
		State: &theater.StateSpec{
			Backends: map[string]theater.StateBackendSpec{
				"local": {
					Use: "state.backend.file",
					With: map[string]any{
						"root": "/tmp/theater-state",
					},
				},
			},
		},
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "state/demo",
				Acts: []theater.ActSpec{
					{
						ID: "read-shared",
						Properties: map[string]theater.PropertySpec{
							"shared_record": {
								Inventory: &theater.InventoryCall{
									Use: "inventory.state.record",
									With: map[string]theater.BindingSpec{
										"backend": literalBinding("local"),
										"record":  literalBinding("env/shared-meta"),
									},
								},
							},
						},
						Action: theater.ActionSpec{
							Use: "action.state.read",
							With: map[string]theater.BindingSpec{
								"record": refBinding("shared_record"),
							},
						},
						Exports: []theater.ExportSpec{{
							As: "shared_record_handle",
							Ref: &theater.RefSpec{
								Name: "shared_record",
							},
						}},
					},
					{
						ID: "read-shared-again",
						Properties: map[string]theater.PropertySpec{
							"shared_record": {
								Inventory: &theater.InventoryCall{
									Use: "inventory.state.record",
									With: map[string]theater.BindingSpec{
										"backend": literalBinding("local"),
										"record":  literalBinding("env/shared-meta"),
									},
								},
							},
						},
						Action: theater.ActionSpec{
							Use: "action.state.read",
							With: map[string]theater.BindingSpec{
								"record": refBinding("shared_record"),
							},
						},
					},
				},
			},
		},
	}

	got, err := MarshalStage(spec)
	if err != nil {
		t.Fatalf("marshal stage failed: %v", err)
	}

	text := string(got)
	if strings.Contains(text, `record shared_record = state.record`) {
		t.Fatalf("repeated handle must stay explicit when one act uses the ref outside the action:\n%s", text)
	}
	if got, want := strings.Count(text, `prop shared_record = inventory.state.record(`), 2; got != want {
		t.Fatalf("shared record property count mismatch: got %d want %d\n%s", got, want, text)
	}
}

func TestMarshalStageKeepsRepeatedStateHandleExplicitWhenNestedAssertionUsesRef(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "state-migrate-nested-assert-guard",
		State: &theater.StateSpec{
			Backends: map[string]theater.StateBackendSpec{
				"local": {
					Use: "state.backend.file",
					With: map[string]any{
						"root": "/tmp/theater-state",
					},
				},
			},
		},
		Scenarios: []theater.ScenarioSpec{{
			ID: "state/demo",
			Acts: []theater.ActSpec{
				{
					ID: "read-shared",
					Properties: map[string]theater.PropertySpec{
						"shared_record": {
							Inventory: &theater.InventoryCall{
								Use: "inventory.state.record",
								With: map[string]theater.BindingSpec{
									"backend": literalBinding("local"),
									"record":  literalBinding("env/shared-meta"),
								},
							},
						},
					},
					Action: theater.ActionSpec{
						Use: "action.state.read",
						With: map[string]theater.BindingSpec{
							"record": refBinding("shared_record"),
						},
					},
					Expectations: []theater.ExpectationSpec{{
						ID:      "shared-record",
						Subject: theater.SubjectSpec{Field: "snapshot"},
						Assert: theater.AssertSpec{
							Ref: builtinexpectation.HasEntryRef,
							Args: map[string]theater.BindingSpec{
								"key": literalBinding("record"),
								"assert": nestedAssertBinding(theater.AssertSpec{
									Ref: builtinexpectation.EqualRef,
									Args: map[string]theater.BindingSpec{
										"expected": refBinding("shared_record"),
									},
								}),
							},
						},
					}},
				},
				{
					ID: "read-shared-again",
					Properties: map[string]theater.PropertySpec{
						"shared_record": {
							Inventory: &theater.InventoryCall{
								Use: "inventory.state.record",
								With: map[string]theater.BindingSpec{
									"backend": literalBinding("local"),
									"record":  literalBinding("env/shared-meta"),
								},
							},
						},
					},
					Action: theater.ActionSpec{
						Use: "action.state.read",
						With: map[string]theater.BindingSpec{
							"record": refBinding("shared_record"),
						},
					},
				},
			},
		}},
	}

	got, err := MarshalStage(spec)
	if err != nil {
		t.Fatalf("marshal stage failed: %v", err)
	}

	text := string(got)
	if strings.Contains(text, `record shared_record = state.record`) {
		t.Fatalf("repeated handle must stay explicit when nested assertion uses the ref:\n%s", text)
	}
	if got, want := strings.Count(text, `prop shared_record = inventory.state.record(`), 2; got != want {
		t.Fatalf("shared record property count mismatch: got %d want %d\n%s", got, want, text)
	}
}

func TestMarshalStageRejectsNonEncodableIdentifiers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mutate     func(*theater.StageSpec)
		wantErrSub string
	}{
		{
			name: "stage id",
			mutate: func(spec *theater.StageSpec) {
				spec.ID = "smoke\nexpect injected"
			},
			wantErrSub: `stage id "smoke`,
		},
		{
			name: "call name",
			mutate: func(spec *theater.StageSpec) {
				spec.Scenarios[0].Acts[0].Action.Use = "action.http\nexpect injected"
			},
			wantErrSub: `call name "action.http`,
		},
		{
			name: "ref name",
			mutate: func(spec *theater.StageSpec) {
				spec.Scenarios[0].Acts[0].Exports = []theater.ExportSpec{{
					As: "session_id",
					Ref: &theater.RefSpec{
						Name: "session\nid",
					},
				}}
			},
			wantErrSub: `ref name "session`,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			spec := minimalMigratableStageSpec()
			test.mutate(&spec)

			_, err := MarshalStage(spec)
			if err == nil {
				t.Fatal("expected migrate validation error, got nil")
			}
			if !strings.Contains(err.Error(), test.wantErrSub) {
				t.Fatalf("error mismatch: got %q want substring %q", err.Error(), test.wantErrSub)
			}
		})
	}
}

func expectationSpec(id string, subject theater.SubjectSpec, assert theater.AssertSpec) theater.ExpectationSpec {
	return theater.ExpectationSpec{
		ID:      id,
		Subject: subject,
		Assert:  assert,
	}
}

func fieldSubject(field string) theater.SubjectSpec {
	return theater.SubjectSpec{Field: field}
}

func subjectWithPath(field string, decode theater.DecodeKind, path theater.JSONPointer) theater.SubjectSpec {
	return theater.SubjectSpec{
		Field:  field,
		Decode: decode,
		Path:   path,
	}
}

func unaryAssert(ref, arg string, value theater.BindingSpec) theater.AssertSpec {
	return theater.AssertSpec{
		Ref: ref,
		Args: map[string]theater.BindingSpec{
			arg: value,
		},
	}
}

func nestedAssertBinding(assert theater.AssertSpec) theater.BindingSpec {
	return objectBinding(map[string]theater.BindingSpec{
		"ref":  literalBinding(assert.Ref),
		"args": objectBinding(assert.Args),
	})
}

func literalBinding(value string) theater.BindingSpec {
	return theater.BindingSpec{
		Kind:  theater.BindingKindLiteral,
		Value: value,
	}
}

func numericBinding(value int) theater.BindingSpec {
	return theater.BindingSpec{
		Kind:  theater.BindingKindLiteral,
		Value: value,
	}
}

func refBinding(name string) theater.BindingSpec {
	return theater.BindingSpec{
		Kind: theater.BindingKindRef,
		Ref: &theater.RefSpec{
			Name: name,
		},
	}
}

func stringBinding(parts ...theater.BindingSpec) theater.BindingSpec {
	return theater.BindingSpec{
		Kind:  theater.BindingKindString,
		Parts: parts,
	}
}

func objectBinding(fields map[string]theater.BindingSpec) theater.BindingSpec {
	return theater.BindingSpec{
		Kind:   theater.BindingKindObject,
		Object: fields,
	}
}

func minimalMigratableStageSpec() theater.StageSpec {
	return theater.StageSpec{
		ID: "smoke",
		Scenarios: []theater.ScenarioSpec{{
			ID: "auth/login",
			Acts: []theater.ActSpec{{
				ID: "submit",
				Action: theater.ActionSpec{
					Use: "action.http",
					With: map[string]theater.BindingSpec{
						"method": literalBinding("GET"),
						"url":    literalBinding("https://example.test"),
					},
				},
			}},
		}},
	}
}

func marshalStageYAML(t *testing.T, spec theater.StageSpec) string {
	t.Helper()

	data, err := goyaml.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal canonical yaml failed: %v", err)
	}

	return string(data)
}

func normalizeStageYAML(t *testing.T, spec theater.StageSpec) any {
	t.Helper()

	data, err := goyaml.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal canonical yaml failed: %v", err)
	}

	var normalized any
	if err := goyaml.Unmarshal(data, &normalized); err != nil {
		t.Fatalf("unmarshal canonical yaml failed: %v", err)
	}

	return normalized
}

func requireSemanticStageYAMLEqual(t *testing.T, got, want theater.StageSpec) {
	t.Helper()

	gotValue := normalizeStageYAML(t, got)
	wantValue := normalizeStageYAML(t, want)
	if reflect.DeepEqual(gotValue, wantValue) {
		return
	}

	t.Fatalf(
		"round-trip canonical yaml mismatch:\n--- got ---\n%s\n--- want ---\n%s",
		marshalStageYAML(t, got),
		marshalStageYAML(t, want),
	)
}
