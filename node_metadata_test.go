package theater

import (
	"testing"

	"github.com/alex-poliushkin/theater/observe"
)

func TestNodeReportKindFromEventKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		eventKind string
		want      NodeKind
		wantOK    bool
	}{
		{name: "stage running", eventKind: EventKindStageRunning},
		{name: "stage finished", eventKind: EventKindStageFinished},
		{name: "scenario running", eventKind: EventKindScenarioRunning},
		{name: "scenario finished", eventKind: EventKindScenarioFinished, want: NodeKindScenario, wantOK: true},
		{name: "act finished", eventKind: EventKindActFinished, want: NodeKindAct, wantOK: true},
		{name: "action finished", eventKind: EventKindActionFinished, want: NodeKindAction, wantOK: true},
		{name: "expectation finished", eventKind: EventKindExpectationFinished, want: NodeKindExpectation, wantOK: true},
		{name: "unknown", eventKind: "unknown.event"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := nodeReportKindFromEventKind(tt.eventKind)
			if ok != tt.wantOK {
				t.Fatalf("result presence mismatch: got %t want %t", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("report kind mismatch: got %q want %q", got, tt.want)
			}
		})
	}
}

func TestMirroredObserveNodeKindFromEventKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		eventKind string
		want      observe.NodeKind
		wantOK    bool
	}{
		{name: "stage running", eventKind: EventKindStageRunning, want: observe.NodeKindStage, wantOK: true},
		{name: "stage finished", eventKind: EventKindStageFinished, want: observe.NodeKindStage, wantOK: true},
		{name: "scenario finished", eventKind: EventKindScenarioFinished, want: observe.NodeKindScenario, wantOK: true},
		{name: "act running", eventKind: EventKindActRunning, want: observe.NodeKindAct, wantOK: true},
		{name: "action finished", eventKind: EventKindActionFinished, want: observe.NodeKindAction, wantOK: true},
		{name: "expectation finished", eventKind: EventKindExpectationFinished},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := mirroredObserveNodeKindFromEventKind(tt.eventKind)
			if ok != tt.wantOK {
				t.Fatalf("result presence mismatch: got %t want %t", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("observe kind mismatch: got %q want %q", got, tt.want)
			}
		})
	}
}

func TestObserveNodeKindFromReportKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		kind NodeKind
		want observe.NodeKind
	}{
		{name: "scenario", kind: NodeKindScenario, want: observe.NodeKindScenario},
		{name: "act", kind: NodeKindAct, want: observe.NodeKindAct},
		{name: "action", kind: NodeKindAction, want: observe.NodeKindAction},
		{name: "expectation", kind: NodeKindExpectation, want: observe.NodeKindScenario},
		{name: "log", kind: NodeKindLog, want: observe.NodeKindLog},
		{name: "unknown", kind: NodeKind("unknown"), want: observe.NodeKindScenario},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := observeNodeKindFromReportKind(tt.kind); got != tt.want {
				t.Fatalf("observe kind mismatch: got %q want %q", got, tt.want)
			}
		})
	}
}

func TestNodeReportKindRank(t *testing.T) {
	t.Parallel()

	if got, want := nodeReportKindRank(NodeKind("unknown")), nodeReportRankOther; got != want {
		t.Fatalf("unknown rank mismatch: got %d want %d", got, want)
	}

	if !(nodeReportKindRank(NodeKindScenario) < nodeReportKindRank(NodeKindAction)) {
		t.Fatal("scenario rank must sort before action")
	}
	if !(nodeReportKindRank(NodeKindAction) < nodeReportKindRank(NodeKindExpectation)) {
		t.Fatal("action rank must sort before expectation")
	}
	if !(nodeReportKindRank(NodeKindExpectation) < nodeReportKindRank(NodeKindAct)) {
		t.Fatal("expectation rank must sort before act")
	}
}

func TestExecutionNodeObserveNodeRefUsesMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		kind NodeKind
		want observe.NodeKind
	}{
		{name: "scenario", kind: NodeKindScenario, want: observe.NodeKindScenario},
		{name: "act", kind: NodeKindAct, want: observe.NodeKindAct},
		{name: "action", kind: NodeKindAction, want: observe.NodeKindAction},
		{name: "expectation", kind: NodeKindExpectation, want: observe.NodeKindScenario},
		{name: "log", kind: NodeKindLog, want: observe.NodeKindLog},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			node := executionNode{
				recorder: executionRecorder{
					identity: executionIdentity{
						stageID:        "main",
						scenarioID:     "login",
						scenarioCallID: "login-user",
					},
				},
				path: "stage.main/call.login-user",
				address: executionNodeAddress{
					kind: tt.kind,
				},
			}

			ref := node.observeNodeRef(3)
			if got := ref.Kind; got != tt.want {
				t.Fatalf("observe node kind mismatch: got %q want %q", got, tt.want)
			}
			if got, want := ref.Attempt, 3; got != want {
				t.Fatalf("attempt mismatch: got %d want %d", got, want)
			}
		})
	}
}
