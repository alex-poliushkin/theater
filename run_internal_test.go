package theater

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestScenarioBindingSourceUsesDirectDependenciesOnly(t *testing.T) {
	t.Parallel()

	source := scenarioBindingSource(
		[]scenarioDependencyPlan{
			{CallID: "register-user"},
		},
		map[string]scenarioState{
			"register-user": {
				Status: StatusPassed,
				Exports: Values{
					"issued_token": "from-register",
				},
			},
			"prepare-user": {
				Status: StatusPassed,
				Exports: Values{
					"seed_token": "from-prepare",
				},
			},
		},
	)

	if got, want := source["issued_token"], "from-register"; got != want {
		t.Fatalf("issued token mismatch: got %v want %v", got, want)
	}

	if _, ok := source["seed_token"]; ok {
		t.Fatal("non-dependency exports must not leak into scenario bindings")
	}
}

func TestScenarioBindingSourceClonesDependencyExports(t *testing.T) {
	t.Parallel()

	states := map[string]scenarioState{
		"register-user": {
			Status: StatusPassed,
			Exports: Values{
				"profile": map[string]any{"id": "user-7"},
				"raw":     []byte("payload"),
			},
		},
	}

	source := scenarioBindingSource(
		[]scenarioDependencyPlan{{CallID: "register-user"}},
		states,
	)

	source["profile"].(map[string]any)["id"] = "mutated"
	source["raw"].([]byte)[0] = 'P'

	if got, want := states["register-user"].Exports["profile"].(map[string]any)["id"], "user-7"; got != want {
		t.Fatalf("state profile mismatch: got %v want %v", got, want)
	}

	if got, want := string(states["register-user"].Exports["raw"].([]byte)), "payload"; got != want {
		t.Fatalf("state raw mismatch: got %q want %q", got, want)
	}
}

func TestScopeWriteAllAndResolveRefDoNotShareMutableValues(t *testing.T) {
	t.Parallel()

	inputs := Values{
		"profile": map[string]any{"id": "user-7"},
		"raw":     []byte("payload"),
	}

	scope := newValueScope(nil)
	scope.writeAll(inputs)

	inputs["profile"].(map[string]any)["id"] = "mutated-input"
	inputs["raw"].([]byte)[0] = 'P'

	resolver := newReferenceResolver(scope)

	profile, err := resolver.ResolveRef(refPlan{Name: "profile"})
	if err != nil {
		t.Fatalf("resolve profile failed: %v", err)
	}
	profile.(map[string]any)["id"] = "mutated-result"

	raw, err := resolver.ResolveRef(refPlan{Name: "raw"})
	if err != nil {
		t.Fatalf("resolve raw failed: %v", err)
	}
	raw.([]byte)[0] = 'X'

	nextProfile, err := resolver.ResolveRef(refPlan{Name: "profile"})
	if err != nil {
		t.Fatalf("resolve profile again failed: %v", err)
	}
	if got, want := nextProfile.(map[string]any)["id"], "user-7"; got != want {
		t.Fatalf("scope profile mismatch: got %v want %v", got, want)
	}

	nextRaw, err := resolver.ResolveRef(refPlan{Name: "raw"})
	if err != nil {
		t.Fatalf("resolve raw again failed: %v", err)
	}
	if got, want := string(nextRaw.([]byte)), "payload"; got != want {
		t.Fatalf("scope raw mismatch: got %q want %q", got, want)
	}
}

func TestNewScenarioResourcesApplyCatalogScenarioScopeInitializers(t *testing.T) {
	t.Parallel()

	resourceKey := NewResourceKey("test/runtime", "shared")
	shared := &testScenarioScopeResource{id: "run-shared"}
	catalog := NewCatalog()
	if err := catalog.RegisterScenarioScopeInitializer("test/runtime/shared", func() ScenarioScopeInitializer {
		return &testScenarioScopeInitializer{
			key:   resourceKey,
			value: shared,
		}
	}); err != nil {
		t.Fatalf("register scenario scope initializer failed: %v", err)
	}

	scopeRun := newScenarioScopeRun(catalog)
	first := newScenarioResources(scopeRun)
	second := newScenarioResources(scopeRun)

	gotFirst, ok := first.GetOrCreate(resourceKey, func() any {
		t.Fatal("first scope should already be initialized")
		return nil
	}).(*testScenarioScopeResource)
	if !ok {
		t.Fatalf("first resource type mismatch: got %T", gotFirst)
	}

	gotSecond, ok := second.GetOrCreate(resourceKey, func() any {
		t.Fatal("second scope should already be initialized")
		return nil
	}).(*testScenarioScopeResource)
	if !ok {
		t.Fatalf("second resource type mismatch: got %T", gotSecond)
	}

	if gotFirst != shared || gotSecond != shared {
		t.Fatal("expected scenario scopes to reuse the run-scoped shared resource")
	}
}

func TestScenarioScopeRunInitializesHTTPAuthSlotsOnEveryMatchingInitializer(t *testing.T) {
	t.Parallel()

	calls := make([]string, 0, 2)
	catalog := NewCatalog()
	for _, name := range []string{"first", "second"} {
		name := name
		if err := catalog.RegisterScenarioScopeInitializer("test/runtime/auth/"+name, func() ScenarioScopeInitializer {
			return &testHTTPAuthSlotInitializer{name: name, calls: &calls}
		}); err != nil {
			t.Fatalf("register scenario scope initializer %q failed: %v", name, err)
		}
	}

	scopeRun := newScenarioScopeRun(catalog)
	resources := newScenarioResources(scopeRun)
	err := scopeRun.InitializeHTTPAuthSlots(resources, map[string]Values{
		"mobile_api": {"access_token": "issued-token"},
	})
	if err != nil {
		t.Fatalf("initialize http auth slots failed: %v", err)
	}

	if !reflect.DeepEqual(calls, []string{"first", "second"}) {
		t.Fatalf("http auth slot initializer calls mismatch: got %#v", calls)
	}
}

func TestAuthBindingSourceRefsIncludeSelectorPickBindings(t *testing.T) {
	t.Parallel()

	refs := authBindingSourceRefs(map[string]httpAuthBindingPlan{
		"mobile_api": {
			Slots: map[string]bindingPlan{
				"access_token": {
					Kind: BindingKindRef,
					Ref: &refPlan{
						Name: "tokens",
						selectorPlan: selectorPlan{
							Through: []throughStepPlan{{
								Pick: &pickStepPlan{
									At: JSONPointer("/value"),
									Equals: bindingPlan{
										Kind: BindingKindRef,
										Ref:  &refPlan{Name: "access_token"},
									},
									Where: []pickWhereClausePlan{{
										Assert: assertPlan{
											Args: map[string]bindingPlan{
												"expected": {
													Kind: BindingKindRef,
													Ref:  &refPlan{Name: "tenant_id"},
												},
											},
										},
									}},
								},
							}},
						},
					},
				},
			},
		},
	})

	for _, name := range []string{"tokens", "access_token", "tenant_id"} {
		if _, ok := refs[name]; !ok {
			t.Fatalf("auth binding source refs must include %q, got %#v", name, refs)
		}
	}
}

func TestPreflightCatalogDetectsMissingRefs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		spec      StageSpec
		prepare   func(t *testing.T, catalog *Catalog)
		wantAt    string
		wantCause string
	}{
		{
			name: "missing action",
			spec: StageSpec{
				ID: "main",
				Scenarios: []ScenarioSpec{
					{
						ID: "login",
						Acts: []ActSpec{
							{ID: "submit", Action: ActionSpec{Use: "action.login"}},
						},
					},
				},
			},
			wantAt:    "stage.main/scenario.login/act.submit",
			wantCause: `action "action.login" is not registered`,
		},
		{
			name: "missing inventory",
			spec: StageSpec{
				ID: "main",
				Scenarios: []ScenarioSpec{
					{
						ID: "login",
						Acts: []ActSpec{
							{
								ID:     "submit",
								Action: ActionSpec{Use: "action.login"},
								Properties: map[string]PropertySpec{
									"seed": {Inventory: &InventoryCall{Use: "inventory.seed"}},
								},
							},
						},
					},
				},
			},
			prepare: func(t *testing.T, catalog *Catalog) {
				t.Helper()
				if err := catalog.RegisterAction("action.login", noopAction{}); err != nil {
					t.Fatalf("register action failed: %v", err)
				}
			},
			wantAt:    "stage.main/scenario.login/act.submit/property.seed",
			wantCause: `inventory "inventory.seed" is not registered`,
		},
		{
			name: "missing decorator",
			spec: StageSpec{
				ID: "main",
				Scenarios: []ScenarioSpec{
					{
						ID: "login",
						Acts: []ActSpec{
							{
								ID:     "submit",
								Action: ActionSpec{Use: "action.login"},
								Properties: map[string]PropertySpec{
									"seed": {
										Inventory: &InventoryCall{Use: "inventory.seed"},
										Decorators: []DecoratorSpec{
											{Use: "math.double"},
										},
									},
								},
							},
						},
					},
				},
			},
			prepare: func(t *testing.T, catalog *Catalog) {
				t.Helper()
				if err := catalog.RegisterAction("action.login", noopAction{}); err != nil {
					t.Fatalf("register action failed: %v", err)
				}
				if err := catalog.RegisterInventory("inventory.seed", noopInventory{}); err != nil {
					t.Fatalf("register inventory failed: %v", err)
				}
			},
			wantAt:    "stage.main/scenario.login/act.submit/property.seed",
			wantCause: `decorator "math.double" is not registered`,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			catalog := NewCatalog()
			if testCase.prepare != nil {
				testCase.prepare(t, catalog)
			}

			stage := compileStageSpec(testCase.spec)
			failure := preflightCatalog(stage, catalog)
			if failure == nil {
				t.Fatal("expected setup failure, got nil")
			}

			if got, want := failure.Kind, FailureKindSetup; got != want {
				t.Fatalf("failure kind mismatch: got %q want %q", got, want)
			}

			if got, want := failure.At, testCase.wantAt; got != want {
				t.Fatalf("failure path mismatch: got %q want %q", got, want)
			}

			if got, want := failure.Cause.Error(), testCase.wantCause; got != want {
				t.Fatalf("failure cause mismatch: got %q want %q", got, want)
			}
		})
	}
}

func TestRunActionEmitsFinishedNodeForSetupFailure(t *testing.T) {
	t.Parallel()

	recorder := &runtimeTestEventRecorder{}
	catalog := NewCatalog()
	if err := catalog.RegisterAction("action.login", testContractAction{
		contract: ActionContract{
			Inputs: map[string]ValueContract{
				"token": {Kind: ValueKindString, Required: true},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	execution := newRuntimeTestActExecution(testRuntimeActPlan(), catalog, recorder.Record)
	outcome, err := execution.runAction(context.Background(), newValueScope(nil), 1)
	if err != nil {
		t.Fatalf("run action failed: %v", err)
	}

	if got, want := outcome.status, StatusFailed; got != want {
		t.Fatalf("status mismatch: got %q want %q", got, want)
	}

	if outcome.failure == nil {
		t.Fatal("expected setup failure")
	}

	if got, want := outcome.failure.Kind, FailureKindSetup; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}

	event, node := expectSingleTerminalNodeEvent(t, recorder, EventKindActionFinished, "stage.main/call.login-user/act.submit/action")
	if got, want := event.Status, StatusFailed; got != want {
		t.Fatalf("event status mismatch: got %q want %q", got, want)
	}

	if got, want := node.Kind, NodeKindAction; got != want {
		t.Fatalf("node kind mismatch: got %q want %q", got, want)
	}
}

func TestRunExpectationEmitsFinishedNodeWhenMatcherIsMissing(t *testing.T) {
	t.Parallel()

	recorder := &runtimeTestEventRecorder{}
	execution := newRuntimeTestActExecution(testRuntimeActPlan(), nil, recorder.Record)

	outcome, err := execution.runExpectation(
		context.Background(),
		expectationPlan{
			ID:      "token",
			Subject: subjectPlan{Field: "token"},
		},
		newValueScope(nil),
		Values{"token": "issued-token"},
		nil,
		1,
	)
	if err != nil {
		t.Fatalf("run expectation failed: %v", err)
	}

	if got, want := outcome.status, StatusFailed; got != want {
		t.Fatalf("status mismatch: got %q want %q", got, want)
	}

	if outcome.failure == nil {
		t.Fatal("expected setup failure")
	}

	if got, want := outcome.failure.Kind, FailureKindSetup; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}

	event, node := expectSingleTerminalNodeEvent(t, recorder, EventKindExpectationFinished, "stage.main/call.login-user/act.submit/expectation.token")
	if got, want := event.Status, StatusFailed; got != want {
		t.Fatalf("event status mismatch: got %q want %q", got, want)
	}

	if got, want := node.Kind, NodeKindExpectation; got != want {
		t.Fatalf("node kind mismatch: got %q want %q", got, want)
	}
}

func TestRunExpectationEmitsFinishedNodeWhenBindingResolutionFails(t *testing.T) {
	t.Parallel()

	recorder := &runtimeTestEventRecorder{}
	execution := newRuntimeTestActExecution(testRuntimeActPlan(), nil, recorder.Record)
	compiled := false

	outcome, err := execution.runExpectation(
		context.Background(),
		expectationPlan{
			ID:      "token",
			Subject: subjectPlan{Field: "token"},
			Assert: assertPlan{
				Args: map[string]bindingPlan{
					"expected": {
						Kind: BindingKindRef,
						Ref:  &refPlan{Name: "missing"},
					},
				},
			},
			Matcher: MatcherDescriptor{
				Ref: "expectation.token",
				Compile: func(MatcherCompileContext, Values) (Matcher, error) {
					compiled = true
					return nil, nil
				},
			},
		},
		newValueScope(nil),
		Values{"token": "issued-token"},
		nil,
		1,
	)
	if err != nil {
		t.Fatalf("run expectation failed: %v", err)
	}

	if compiled {
		t.Fatal("matcher compile must not run after binding failure")
	}

	if got, want := outcome.status, StatusFailed; got != want {
		t.Fatalf("status mismatch: got %q want %q", got, want)
	}

	if outcome.failure == nil {
		t.Fatal("expected setup failure")
	}

	if got, want := outcome.failure.Kind, FailureKindSetup; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}

	event, node := expectSingleTerminalNodeEvent(t, recorder, EventKindExpectationFinished, "stage.main/call.login-user/act.submit/expectation.token")
	if got, want := event.Status, StatusFailed; got != want {
		t.Fatalf("event status mismatch: got %q want %q", got, want)
	}

	if got, want := node.Kind, NodeKindExpectation; got != want {
		t.Fatalf("node kind mismatch: got %q want %q", got, want)
	}
}

func TestRunExpectationEmitsFinishedNodeWhenMatcherCompileFails(t *testing.T) {
	t.Parallel()

	recorder := &runtimeTestEventRecorder{}
	execution := newRuntimeTestActExecution(testRuntimeActPlan(), nil, recorder.Record)

	outcome, err := execution.runExpectation(
		context.Background(),
		expectationPlan{
			ID:      "token",
			Subject: subjectPlan{Field: "token"},
			Matcher: MatcherDescriptor{
				Ref: "expectation.token",
				Compile: func(MatcherCompileContext, Values) (Matcher, error) {
					return nil, errors.New("matcher compile failed")
				},
			},
		},
		newValueScope(nil),
		Values{"token": "issued-token"},
		nil,
		1,
	)
	if err != nil {
		t.Fatalf("run expectation failed: %v", err)
	}

	if got, want := outcome.status, StatusFailed; got != want {
		t.Fatalf("status mismatch: got %q want %q", got, want)
	}

	if outcome.failure == nil {
		t.Fatal("expected setup failure")
	}

	if got, want := outcome.failure.Kind, FailureKindSetup; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}

	event, node := expectSingleTerminalNodeEvent(t, recorder, EventKindExpectationFinished, "stage.main/call.login-user/act.submit/expectation.token")
	if got, want := event.Status, StatusFailed; got != want {
		t.Fatalf("event status mismatch: got %q want %q", got, want)
	}

	if got, want := node.Kind, NodeKindExpectation; got != want {
		t.Fatalf("node kind mismatch: got %q want %q", got, want)
	}
}

func TestRunLogsEvaluatesValueAndMessageRecords(t *testing.T) {
	t.Parallel()

	act := testRuntimeActPlan()
	act.Logs = []logPlan{
		{
			ID: "response",
			Value: logValuePlan{
				Object: map[string]logValuePlan{
					"status": {Field: "status_code"},
					"user_id": {
						Field: "body",
						selectorPlan: selectorPlan{
							Decode: DecodeJSON,
							Path:   "/data/id",
						},
					},
				},
			},
		},
		{
			ID:      "audit",
			Message: "response received",
			Fields: map[string]logValuePlan{
				"request_id": {Ref: "request_id"},
			},
		},
		{
			ID: "list",
			Value: logValuePlan{
				List: []logValuePlan{
					{Field: "status_code"},
					{Ref: "request_id"},
				},
			},
		},
	}

	actScope := newValueScope(nil)
	actScope.writeAll(Values{"request_id": "req-123"})
	recorder := &runtimeTestEventRecorder{}
	execution := newRuntimeTestActExecution(act, nil, recorder.Record)
	records, outcome, err := execution.runLogs(
		context.Background(),
		actScope,
		Values{
			"status_code": 201,
			"body":        `{"data":{"id":"user-123"}}`,
		},
		2,
	)
	if err != nil {
		t.Fatalf("run logs failed: %v", err)
	}
	if outcome != nil {
		t.Fatalf("unexpected log outcome: %#v", outcome)
	}
	if got, want := len(records), 3; got != want {
		t.Fatalf("record count mismatch: got %d want %d", got, want)
	}

	response := records[0]
	if got, want := response.Status, logEvaluationStatusEmitted; got != want {
		t.Fatalf("response status mismatch: got %q want %q", got, want)
	}
	if got, want := response.Attempt, 2; got != want {
		t.Fatalf("response attempt mismatch: got %d want %d", got, want)
	}
	if got, want := response.Path, "stage.main/call.login-user/act.submit/log.response"; got != want {
		t.Fatalf("response path mismatch: got %q want %q", got, want)
	}
	responseValue, ok := response.Value.(Values)
	if !ok {
		t.Fatalf("response value type mismatch: %#v", response.Value)
	}
	if got, want := responseValue["status"], 201; got != want {
		t.Fatalf("response status value mismatch: got %#v want %#v", got, want)
	}
	if got, want := responseValue["user_id"], "user-123"; got != want {
		t.Fatalf("response user id mismatch: got %#v want %#v", got, want)
	}

	audit := records[1]
	if got, want := audit.Message, "response received"; got != want {
		t.Fatalf("audit message mismatch: got %q want %q", got, want)
	}
	if got, want := audit.Fields["request_id"], "req-123"; got != want {
		t.Fatalf("audit field mismatch: got %#v want %#v", got, want)
	}

	list, ok := records[2].Value.([]any)
	if !ok {
		t.Fatalf("list value type mismatch: %#v", records[2].Value)
	}
	if got, want := list, []any{201, "req-123"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("list value mismatch: got %#v want %#v", got, want)
	}

	events := recorder.Events()
	if got, want := len(events), 3; got != want {
		t.Fatalf("log event count mismatch: got %d want %d", got, want)
	}
	if got, want := events[0].Kind, EventKindLogEmitted; got != want {
		t.Fatalf("log event kind mismatch: got %q want %q", got, want)
	}
	if events[0].Log == nil {
		t.Fatal("log event must carry report log record")
	}
	if got, want := events[0].Log.ID, "response"; got != want {
		t.Fatalf("event log id mismatch: got %q want %q", got, want)
	}
	if got, want := events[0].Log.Address.Kind, NodeKindLog; got != want {
		t.Fatalf("event log address kind mismatch: got %q want %q", got, want)
	}
	if got, want := events[0].Log.Address.AttemptIndex, 2; got != want {
		t.Fatalf("event log attempt mismatch: got %d want %d", got, want)
	}
}

func TestRunLogsKeepsRuntimeErrorsNonFatalByDefault(t *testing.T) {
	t.Parallel()

	act := testRuntimeActPlan()
	act.Logs = []logPlan{{
		ID:    "response",
		Value: logValuePlan{Field: "missing_body"},
	}}

	recorder := &runtimeTestEventRecorder{}
	execution := newRuntimeTestActExecution(act, nil, recorder.Record)
	records, outcome, err := execution.runLogs(context.Background(), newValueScope(nil), Values{"body": "ok"}, 1)
	if err != nil {
		t.Fatalf("run logs failed: %v", err)
	}
	if outcome != nil {
		t.Fatalf("unexpected log outcome: %#v", outcome)
	}
	if got, want := len(records), 1; got != want {
		t.Fatalf("record count mismatch: got %d want %d", got, want)
	}
	if got, want := records[0].Status, logEvaluationStatusError; got != want {
		t.Fatalf("record status mismatch: got %q want %q", got, want)
	}
	if records[0].Failure == nil {
		t.Fatal("error record must carry failure details")
	}
	events := recorder.Events()
	if got, want := len(events), 1; got != want {
		t.Fatalf("log event count mismatch: got %d want %d", got, want)
	}
	if events[0].Log == nil {
		t.Fatal("log event must carry report log record")
	}
	if got, want := events[0].Log.Status, LogStatusError; got != want {
		t.Fatalf("event log status mismatch: got %q want %q", got, want)
	}
}

func TestRunLogsRequiredErrorFailsAct(t *testing.T) {
	t.Parallel()

	act := testRuntimeActPlan()
	act.Logs = []logPlan{{
		ID:       "response",
		Value:    logValuePlan{Field: "missing_body"},
		Required: true,
	}}

	recorder := &runtimeTestEventRecorder{}
	execution := newRuntimeTestActExecution(act, nil, recorder.Record)
	records, outcome, err := execution.runLogs(context.Background(), newValueScope(nil), Values{"body": "ok"}, 1)
	if err != nil {
		t.Fatalf("run logs failed: %v", err)
	}
	if outcome == nil {
		t.Fatal("expected required log failure")
	}
	if got, want := outcome.status, StatusFailed; got != want {
		t.Fatalf("outcome status mismatch: got %q want %q", got, want)
	}
	if outcome.failure == nil {
		t.Fatal("expected required log failure details")
	}
	if got, want := outcome.failure.Kind, FailureKindObservation; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}
	if got, want := outcome.failure.At, "stage.main/call.login-user/act.submit/log.response"; got != want {
		t.Fatalf("failure path mismatch: got %q want %q", got, want)
	}
	if got, want := len(records), 1; got != want {
		t.Fatalf("record count mismatch: got %d want %d", got, want)
	}
	if got, want := records[0].Status, logEvaluationStatusError; got != want {
		t.Fatalf("record status mismatch: got %q want %q", got, want)
	}
}

func TestActExecutionReturnsAttemptAddressedLogRecords(t *testing.T) {
	t.Parallel()

	act := testRuntimeActPlan()
	act.Logs = []logPlan{{
		ID:    "response",
		Value: logValuePlan{Field: "token"},
	}}

	catalog := NewCatalog()
	if err := catalog.RegisterAction("action.login", runtimeStaticAction{
		contract: ActionContract{
			Outputs: map[string]ValueContract{
				"token": {Kind: ValueKindString},
			},
		},
		outputs: Outputs{"token": "issued-token"},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	recorder := &runtimeTestEventRecorder{}
	execution := newRuntimeTestActExecution(act, catalog, recorder.Record)
	outcome, err := execution.Run(context.Background(), 3)
	if err != nil {
		t.Fatalf("act execution failed: %v", err)
	}
	if got, want := outcome.status, StatusPassed; got != want {
		t.Fatalf("outcome status mismatch: got %q want %q", got, want)
	}
	if got, want := len(outcome.logs), 1; got != want {
		t.Fatalf("log count mismatch: got %d want %d", got, want)
	}
	if got, want := outcome.logs[0].Attempt, 3; got != want {
		t.Fatalf("log attempt mismatch: got %d want %d", got, want)
	}
	if got, want := outcome.logs[0].Value, "issued-token"; got != want {
		t.Fatalf("log value mismatch: got %#v want %#v", got, want)
	}
}

func TestActExecutionRunsRequiredLogsBeforeExpectations(t *testing.T) {
	t.Parallel()

	act := testRuntimeActPlan()
	act.Logs = []logPlan{{
		ID:       "response",
		Value:    logValuePlan{Field: "missing_body"},
		Required: true,
	}}

	compiled := false
	checked := false
	act.Expectations = []expectationPlan{{
		ID:      "token",
		Subject: subjectPlan{Field: "token"},
		Matcher: MatcherDescriptor{
			Ref: "expectation.record",
			Compile: func(MatcherCompileContext, Values) (Matcher, error) {
				compiled = true
				return recordingMatcher{checked: &checked}, nil
			},
		},
	}}

	catalog := NewCatalog()
	if err := catalog.RegisterAction("action.login", runtimeStaticAction{
		contract: ActionContract{
			Outputs: map[string]ValueContract{
				"token": {Kind: ValueKindString},
			},
		},
		outputs: Outputs{"token": "issued-token"},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	recorder := &runtimeTestEventRecorder{}
	execution := newRuntimeTestActExecution(act, catalog, recorder.Record)
	outcome, err := execution.Run(context.Background(), 1)
	if err != nil {
		t.Fatalf("act execution failed: %v", err)
	}
	if got, want := outcome.status, StatusFailed; got != want {
		t.Fatalf("outcome status mismatch: got %q want %q", got, want)
	}
	if outcome.failure == nil || outcome.failure.At != "stage.main/call.login-user/act.submit/log.response" {
		t.Fatalf("failure mismatch: %#v", outcome.failure)
	}
	if compiled {
		t.Fatal("expectation matcher must not compile after required log failure")
	}
	if checked {
		t.Fatal("expectation matcher must not run after required log failure")
	}
}

func TestActExecutionSkipsLogsWhenActionFails(t *testing.T) {
	t.Parallel()

	act := testRuntimeActPlan()
	act.Logs = []logPlan{{
		ID:       "response",
		Value:    logValuePlan{Field: "missing_body"},
		Required: true,
	}}

	catalog := NewCatalog()
	if err := catalog.RegisterAction("action.login", runtimeStaticAction{
		err: errors.New("action boom"),
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	recorder := &runtimeTestEventRecorder{}
	execution := newRuntimeTestActExecution(act, catalog, recorder.Record)
	outcome, err := execution.Run(context.Background(), 1)
	if err != nil {
		t.Fatalf("act execution failed: %v", err)
	}
	if got, want := outcome.status, StatusFailed; got != want {
		t.Fatalf("outcome status mismatch: got %q want %q", got, want)
	}
	if outcome.failure == nil {
		t.Fatal("expected action failure")
	}
	if got, want := outcome.failure.At, "stage.main/call.login-user/act.submit/action"; got != want {
		t.Fatalf("failure path mismatch: got %q want %q", got, want)
	}
	if got, want := outcome.failure.Kind, FailureKindAction; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}
	for _, event := range recorder.Events() {
		if event.Kind == EventKindLogEmitted {
			t.Fatalf("logs must be skipped after action failure: %#v", event)
		}
	}
}

func TestResolvePropertiesReturnsInternalFailureWhenInventoryPanics(t *testing.T) {
	t.Parallel()

	recorder := &runtimeTestEventRecorder{}
	catalog := NewCatalog()
	if err := catalog.RegisterInventory("inventory.seed", panicInventory{
		contract: InventoryContract{
			Produces: ValueContract{Kind: ValueKindString},
		},
		message: "inventory boom",
	}); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}

	act := testRuntimeActPlan()
	act.Properties = []propertyPlan{{
		ID:   "seed",
		Path: act.Path + "/property.seed",
		Inventory: inventoryPlan{
			Present: true,
			Use:     "inventory.seed",
		},
	}}

	execution := newRuntimeTestActExecution(act, catalog, recorder.Record)
	failure := execution.resolveProperties(context.Background(), newValueScope(nil), 1)
	if failure == nil {
		t.Fatal("expected inventory panic failure")
	}

	if got, want := failure.Kind, FailureKindInternal; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}
	if got, want := failure.At, "stage.main/call.login-user/act.submit/property.seed"; got != want {
		t.Fatalf("failure path mismatch: got %q want %q", got, want)
	}
	if got, want := failure.Summary, "inventory panicked"; got != want {
		t.Fatalf("failure summary mismatch: got %q want %q", got, want)
	}
	if got, want := failure.Cause.Error(), `inventory "inventory.seed" panicked: inventory boom`; got != want {
		t.Fatalf("failure cause mismatch: got %q want %q", got, want)
	}
}

func TestResolvePropertiesReturnsInternalFailureWhenDecoratorPanics(t *testing.T) {
	t.Parallel()

	recorder := &runtimeTestEventRecorder{}
	catalog := NewCatalog()
	if err := catalog.RegisterInventory("inventory.seed", staticInventory{
		contract: InventoryContract{
			Produces: ValueContract{Kind: ValueKindString},
		},
		value: "seed",
	}); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}

	act := testRuntimeActPlan()
	act.Properties = []propertyPlan{{
		ID:   "seed",
		Path: act.Path + "/property.seed",
		Inventory: inventoryPlan{
			Present: true,
			Use:     "inventory.seed",
		},
		Decorators: []decoratorPlan{{
			Use: "decorator.normalize",
			Contract: DecoratorContract{
				Produces: ValueContract{Kind: ValueKindString},
			},
			Transform: func(any) (any, error) {
				panic("decorator boom")
			},
		}},
	}}

	execution := newRuntimeTestActExecution(act, catalog, recorder.Record)
	failure := execution.resolveProperties(context.Background(), newValueScope(nil), 1)
	if failure == nil {
		t.Fatal("expected decorator panic failure")
	}

	if got, want := failure.Kind, FailureKindInternal; got != want {
		t.Fatalf("failure kind mismatch: got %q want %q", got, want)
	}
	if got, want := failure.At, "stage.main/call.login-user/act.submit/property.seed"; got != want {
		t.Fatalf("failure path mismatch: got %q want %q", got, want)
	}
	if got, want := failure.Summary, "decorator panicked"; got != want {
		t.Fatalf("failure summary mismatch: got %q want %q", got, want)
	}
	if got, want := failure.Cause.Error(), `decorator "decorator.normalize" panicked: decorator boom`; got != want {
		t.Fatalf("failure cause mismatch: got %q want %q", got, want)
	}
}

func TestRunExpectationReturnsInternalFailureWhenMatcherPanics(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		expectation expectationPlan
		wantCause   string
	}{
		{
			name: "compile panic",
			expectation: expectationPlan{
				ID:      "token",
				Subject: subjectPlan{Field: "token"},
				Matcher: MatcherDescriptor{
					Ref: "expectation.token",
					Compile: func(MatcherCompileContext, Values) (Matcher, error) {
						panic("compile boom")
					},
				},
			},
			wantCause: `matcher "expectation.token" panicked: compile boom`,
		},
		{
			name: "check panic",
			expectation: expectationPlan{
				ID:      "token",
				Subject: subjectPlan{Field: "token"},
				Matcher: MatcherDescriptor{
					Ref: "expectation.token",
					Compile: func(MatcherCompileContext, Values) (Matcher, error) {
						return panicMatcher{message: "check boom"}, nil
					},
				},
			},
			wantCause: `matcher "expectation.token" panicked: check boom`,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			recorder := &runtimeTestEventRecorder{}
			execution := newRuntimeTestActExecution(testRuntimeActPlan(), nil, recorder.Record)

			outcome, err := execution.runExpectation(
				context.Background(),
				testCase.expectation,
				newValueScope(nil),
				Values{"token": "issued-token"},
				nil,
				1,
			)
			if err != nil {
				t.Fatalf("run expectation failed: %v", err)
			}

			if got, want := outcome.status, StatusFailed; got != want {
				t.Fatalf("status mismatch: got %q want %q", got, want)
			}
			if outcome.failure == nil {
				t.Fatal("expected matcher panic failure")
			}
			if got, want := outcome.failure.Kind, FailureKindInternal; got != want {
				t.Fatalf("failure kind mismatch: got %q want %q", got, want)
			}
			if got, want := outcome.failure.At, "stage.main/call.login-user/act.submit/expectation.token"; got != want {
				t.Fatalf("failure path mismatch: got %q want %q", got, want)
			}
			if got, want := outcome.failure.Summary, "matcher panicked"; got != want {
				t.Fatalf("failure summary mismatch: got %q want %q", got, want)
			}
			if got, want := outcome.failure.Cause.Error(), testCase.wantCause; got != want {
				t.Fatalf("failure cause mismatch: got %q want %q", got, want)
			}
		})
	}
}

type testScenarioScopeInitializer struct {
	key   ResourceKey
	value any
}

type testScenarioScopeResource struct {
	id string
}

type testHTTPAuthSlotInitializer struct {
	name  string
	calls *[]string
}

func (i *testScenarioScopeInitializer) InitializeScenarioScope(resources ResourceScope) {
	resources.GetOrCreate(i.key, func() any {
		return i.value
	})
}

func (*testScenarioScopeInitializer) Close() {}

func (*testHTTPAuthSlotInitializer) InitializeScenarioScope(ResourceScope) {}

func (i *testHTTPAuthSlotInitializer) InitializeHTTPAuthSlots(ResourceScope, map[string]Values) error {
	*i.calls = append(*i.calls, i.name)
	return nil
}

func (*testHTTPAuthSlotInitializer) Close() {}

type noopAction struct{}

type noopInventory struct{}

type noopDecorator struct{}

type testContractAction struct {
	contract ActionContract
}

type runtimeStaticAction struct {
	contract ActionContract
	outputs  Outputs
	err      error
}

type runtimeTestEventRecorder struct {
	events []Event
}

type recordingMatcher struct {
	checked *bool
}

type panicMatcher struct {
	message string
}

type panicInventory struct {
	contract InventoryContract
	message  string
}

type staticInventory struct {
	contract InventoryContract
	value    any
}

func (noopAction) Contract() ActionContract {
	return ActionContract{}
}

func (noopAction) Run(context.Context, ActionRequest) (Outputs, error) {
	return Outputs{}, nil
}

func (m panicMatcher) Check(context.Context, any) error {
	panic(m.message)
}

func (i panicInventory) Acquire(context.Context, InventoryRequest) (any, error) {
	panic(i.message)
}

func (i panicInventory) Contract() InventoryContract {
	return i.contract
}

func (i staticInventory) Acquire(context.Context, InventoryRequest) (any, error) {
	return i.value, nil
}

func (i staticInventory) Contract() InventoryContract {
	return i.contract
}

func (noopDecorator) Definition() DecoratorDef {
	return DecoratorDef{
		Contract: DecoratorContract{
			Accepts:  ValueContract{Kind: ValueKindAny},
			Produces: ValueContract{Kind: ValueKindAny},
		},
		Compile: func(Values) (DecoratorFunc, error) {
			return func(value any) (any, error) {
				return value, nil
			}, nil
		},
	}
}

func (noopInventory) Acquire(context.Context, InventoryRequest) (any, error) {
	return nil, nil
}

func (noopInventory) Contract() InventoryContract {
	return InventoryContract{
		Produces: ValueContract{Kind: ValueKindAny},
	}
}

func (a testContractAction) Contract() ActionContract {
	return a.contract
}

func (testContractAction) Run(context.Context, ActionRequest) (Outputs, error) {
	return nil, nil
}

func (a runtimeStaticAction) Contract() ActionContract {
	return a.contract
}

func (a runtimeStaticAction) Run(context.Context, ActionRequest) (Outputs, error) {
	if a.err != nil {
		return nil, a.err
	}

	return a.outputs, nil
}

func (r *runtimeTestEventRecorder) Events() []Event {
	copied := make([]Event, len(r.events))
	copy(copied, r.events)
	return copied
}

func (m recordingMatcher) Check(context.Context, any) error {
	if m.checked != nil {
		*m.checked = true
	}

	return nil
}

func (r *runtimeTestEventRecorder) Record(event Event) error {
	if err := event.Validate(); err != nil {
		return err
	}

	r.events = append(r.events, event)
	return nil
}

func testRuntimeActPlan() *actPlan {
	return &actPlan{
		ID:   "submit",
		Path: "stage.main/call.login-user/act.submit",
		Action: actionPlan{
			Use: "action.login",
		},
	}
}

func newRuntimeTestActExecution(act *actPlan, catalog runtimeCatalog, record func(Event) error) actExecution {
	identity := executionIdentity{
		stageID:        "main",
		stagePath:      "stage.main",
		scenarioID:     "auth/login",
		scenarioCallID: "login-user",
		scenarioPath:   "stage.main/call.login-user",
		scenarioSeq:    1,
	}
	recorder := newExecutionRecorder(identity, record)

	return actExecution{
		act:           act,
		actPath:       act.Path,
		actionPath:    recorder.action(act.ID).path,
		catalog:       catalog,
		identity:      identity,
		recorder:      recorder,
		resources:     NewResourceScope(),
		scenarioScope: newValueScope(nil),
	}
}

func expectSingleTerminalNodeEvent(
	t *testing.T,
	recorder *runtimeTestEventRecorder,
	wantKind string,
	wantPath string,
) (Event, NodeReport) {
	t.Helper()

	events := recorder.Events()
	if got, want := len(events), 1; got != want {
		t.Fatalf("event count mismatch: got %d want %d (%v)", got, want, events)
	}

	event := events[0]
	if got, want := event.Kind, wantKind; got != want {
		t.Fatalf("event kind mismatch: got %q want %q", got, want)
	}

	if got, want := event.Path, wantPath; got != want {
		t.Fatalf("event path mismatch: got %q want %q", got, want)
	}

	node, ok := nodeReportFromEvent(event)
	if !ok {
		t.Fatal("terminal event must materialize a node report")
	}

	return event, node
}
