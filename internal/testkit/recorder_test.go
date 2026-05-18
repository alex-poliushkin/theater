package testkit

import (
	"testing"

	"github.com/alex-poliushkin/theater"
)

func TestEventRecorderRejectsInvalidEvent(t *testing.T) {
	t.Parallel()

	recorder := EventRecorder{}

	err := recorder.Record(theater.Event{
		Kind:   "scenario.finished",
		Status: theater.StatusCanceled,
		Failure: &theater.Failure{
			Kind:    theater.FailureKindInternal,
			Phase:   theater.PhaseRun,
			At:      "stage.main/call.login-user",
			Summary: "should not happen",
		},
	})
	if err == nil {
		t.Fatal("expected invalid event to be rejected")
	}
}

func TestEventRecorderReturnsCopyAndReplayableReport(t *testing.T) {
	t.Parallel()

	recorder := EventRecorder{}
	stageFailure := &theater.Failure{
		Kind:    theater.FailureKindAction,
		Phase:   theater.PhaseRun,
		At:      "stage.main/call.login-user",
		Summary: "login failed",
	}

	events := []theater.Event{
		{
			Kind:         "scenario.finished",
			StagePath:    "stage.main",
			ScenarioPath: "stage.main/call.prepare-user",
			Path:         "stage.main/call.prepare-user",
			Attempt:      1,
			Status:       theater.StatusPassed,
			ScenarioSeq:  1,
		},
		{
			Kind:         "scenario.finished",
			StagePath:    "stage.main",
			ScenarioPath: "stage.main/call.login-user",
			Path:         "stage.main/call.login-user",
			Attempt:      1,
			Status:       theater.StatusFailed,
			Failure:      stageFailure,
			ScenarioSeq:  2,
		},
		{
			Kind:      "stage.finished",
			StagePath: "stage.main",
			Status:    theater.StatusFailed,
			Failure:   stageFailure,
		},
	}

	for i := range events {
		events[i].RunID = "stage.main/test-run"
		events[i].TheaterVersion = "test-version"
		if err := recorder.Record(events[i]); err != nil {
			t.Fatalf("record event failed: %v", err)
		}
	}

	copied := recorder.Events()
	copied[0].Kind = "mutated"

	report, err := ReplayReport(recorder.Events())
	if err != nil {
		t.Fatalf("replay report failed: %v", err)
	}

	if got, want := recorder.Events()[0].Kind, "scenario.finished"; got != want {
		t.Fatalf("recorder must return copy: got %q want %q", got, want)
	}

	if got, want := report.Summary.TotalScenarios, 2; got != want {
		t.Fatalf("total scenarios mismatch: got %d want %d", got, want)
	}

	if got, want := report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	doc, err := ReplayRunDocument(recorder.Events())
	if err != nil {
		t.Fatalf("replay run document failed: %v", err)
	}

	if got, want := doc.ReportSchemaVersion, theater.RunDocumentSchemaVersion; got != want {
		t.Fatalf("run document schema version mismatch: got %q want %q", got, want)
	}
	if got, want := doc.TheaterVersion, "test-version"; got != want {
		t.Fatalf("run document theater version mismatch: got %q want %q", got, want)
	}
	if got, want := doc.RunID, "stage.main/test-run"; got != want {
		t.Fatalf("run document id mismatch: got %q want %q", got, want)
	}
}
