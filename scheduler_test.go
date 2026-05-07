package theater

import "testing"

func TestReadyScenarioCallsReturnsDeterministicOrdinalOrder(t *testing.T) {
	t.Parallel()

	stage := compileStageSpec(StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{ID: "prepare"},
			{ID: "login"},
			{ID: "cleanup"},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
			{ID: "prepare-user", ScenarioID: "prepare"},
			{ID: "cleanup-user", ScenarioID: "cleanup"},
		},
	})

	ready := readyScenarioCalls(stage, nil)

	if got, want := len(ready), 3; got != want {
		t.Fatalf("ready call count mismatch: got %d want %d", got, want)
	}

	if got, want := ready[0].ID, "login-user"; got != want {
		t.Fatalf("first ready call mismatch: got %q want %q", got, want)
	}

	if got, want := ready[1].ID, "prepare-user"; got != want {
		t.Fatalf("second ready call mismatch: got %q want %q", got, want)
	}

	if got, want := ready[2].ID, "cleanup-user"; got != want {
		t.Fatalf("third ready call mismatch: got %q want %q", got, want)
	}
}

func TestReadyScenarioCallsRespectsDependencyPredicates(t *testing.T) {
	t.Parallel()

	stage := compileStageSpec(StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{ID: "prepare"},
			{ID: "login"},
			{ID: "cleanup"},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "prepare-user", ScenarioID: "prepare"},
			{
				ID:         "login-user",
				ScenarioID: "login",
				Dependencies: []ScenarioDependencySpec{
					{CallID: "prepare-user"},
				},
			},
			{
				ID:         "cleanup-user",
				ScenarioID: "cleanup",
				Dependencies: []ScenarioDependencySpec{
					{CallID: "login-user", When: TriggerPredicateDone},
				},
			},
		},
	})

	states := map[string]scenarioState{
		"prepare-user": {
			Status: StatusPassed,
		},
		"login-user": {
			Status: StatusFailed,
		},
	}

	ready := readyScenarioCalls(stage, states)

	if got, want := len(ready), 1; got != want {
		t.Fatalf("ready call count mismatch: got %d want %d", got, want)
	}

	if got, want := ready[0].ID, "cleanup-user"; got != want {
		t.Fatalf("ready call mismatch: got %q want %q", got, want)
	}
}

func TestReadyScenarioCallsSkipsNonPendingCalls(t *testing.T) {
	t.Parallel()

	stage := compileStageSpec(StageSpec{
		ID: "main",
		Scenarios: []ScenarioSpec{
			{ID: "prepare"},
			{ID: "login"},
		},
		ScenarioCalls: []ScenarioCallSpec{
			{ID: "prepare-user", ScenarioID: "prepare"},
			{ID: "login-user", ScenarioID: "login"},
		},
	})

	states := map[string]scenarioState{
		"prepare-user": {
			Status: StatusRunning,
		},
		"login-user": {
			Status: StatusPassed,
		},
	}

	ready := readyScenarioCalls(stage, states)

	if got, want := len(ready), 0; got != want {
		t.Fatalf("ready call count mismatch: got %d want %d", got, want)
	}
}

func TestReadyScenarioCallsAfterFailedBatchAllowsOnlyRecoveryAndDoneBranches(t *testing.T) {
	t.Parallel()

	ready := []scenarioCallPlan{
		{ID: "normal"},
		{
			ID: "recovery",
			Dependencies: []scenarioDependencyPlan{
				{CallID: "main", When: TriggerPredicateFailure},
			},
		},
		{
			ID: "finalizer",
			Dependencies: []scenarioDependencyPlan{
				{CallID: "main", When: TriggerPredicateDone},
			},
		},
		{
			ID: "success-child",
			Dependencies: []scenarioDependencyPlan{
				{CallID: "other", When: TriggerPredicateSuccess},
			},
		},
	}

	policy := defaultStageExecutionPolicy()
	if got, want := policy.FailureBehavior, stageFailurePolicyFailAfterReadyBatch; got != want {
		t.Fatalf("default failure policy mismatch: got %q want %q", got, want)
	}

	filtered := policy.filterReadyAfterFailure(ready)

	if got, want := len(filtered), 2; got != want {
		t.Fatalf("filtered ready count mismatch: got %d want %d", got, want)
	}

	if got, want := filtered[0].ID, "recovery"; got != want {
		t.Fatalf("first filtered call mismatch: got %q want %q", got, want)
	}

	if got, want := filtered[1].ID, "finalizer"; got != want {
		t.Fatalf("second filtered call mismatch: got %q want %q", got, want)
	}
}
