package theater

import (
	"time"

	"github.com/alex-poliushkin/theater/observe"
)

type executionIdentity struct {
	stageID        string
	stagePath      string
	scenarioID     string
	scenarioCallID string
	scenarioPath   string
	scenarioSeq    int
}

type executionRecorder struct {
	identity  executionIdentity
	addresses executionAddressSpace
	record    func(Event) error
}

type executionNode struct {
	recorder     executionRecorder
	runningKind  string
	finishedKind string
	path         string
	address      executionNodeAddress
}

type executionNodeResult struct {
	status       Status
	skipReason   SkipReason
	failure      *Failure
	startedAt    time.Time
	endedAt      time.Time
	sourceSpan   *SourceRef
	preview      *Preview
	contrast     *Contrast
	observations *ActionObservations
	eventually   *EventuallyReport
	payload      *PayloadMetadata
}

func newExecutionRecorder(identity executionIdentity, record func(Event) error) executionRecorder {
	return executionRecorder{
		identity:  identity,
		addresses: newExecutionAddressSpace(identity.scenarioPath),
		record:    record,
	}
}

func (r executionRecorder) scenario() executionNode {
	return executionNode{
		recorder:     r,
		runningKind:  EventKindScenarioRunning,
		finishedKind: EventKindScenarioFinished,
		path:         r.addresses.scenarioPath(),
		address:      r.addresses.scenarioAddress(),
	}
}

func (r executionRecorder) act(actID string) executionNode {
	return executionNode{
		recorder:     r,
		runningKind:  EventKindActRunning,
		finishedKind: EventKindActFinished,
		path:         r.addresses.actPath(actID),
		address:      r.addresses.actAddress(actID),
	}
}

func (r executionRecorder) action(actID string) executionNode {
	return executionNode{
		recorder:     r,
		runningKind:  EventKindActionRunning,
		finishedKind: EventKindActionFinished,
		path:         r.addresses.actionPath(actID),
		address:      r.addresses.actionAddress(actID),
	}
}

func (r executionRecorder) expectation(actID, expectationID string) executionNode {
	return executionNode{
		recorder:     r,
		finishedKind: EventKindExpectationFinished,
		path:         r.addresses.expectationPath(actID, expectationID),
		address:      r.addresses.expectationAddress(actID, expectationID),
	}
}

func (n executionNode) running(attempt int) error {
	if n.runningKind == "" {
		return nil
	}

	return n.recorder.record(Event{
		Kind:           n.runningKind,
		StageID:        n.recorder.identity.stageID,
		StagePath:      n.recorder.identity.stagePath,
		ScenarioID:     n.recorder.identity.scenarioID,
		ScenarioCallID: n.recorder.identity.scenarioCallID,
		ScenarioPath:   n.recorder.identity.scenarioPath,
		Path:           n.path,
		Address:        n.address.running(attempt),
		Attempt:        attempt,
		ScenarioSeq:    n.recorder.identity.scenarioSeq,
		Status:         StatusRunning,
	})
}

func (n executionNode) finished(attempt int, result executionNodeResult) error {
	return n.recorder.record(timedEvent(Event{
		Kind:           n.finishedKind,
		StageID:        n.recorder.identity.stageID,
		StagePath:      n.recorder.identity.stagePath,
		ScenarioID:     n.recorder.identity.scenarioID,
		ScenarioCallID: n.recorder.identity.scenarioCallID,
		ScenarioPath:   n.recorder.identity.scenarioPath,
		Path:           n.path,
		Address:        n.address.finished(attempt, result.failure),
		Attempt:        attempt,
		ScenarioSeq:    n.recorder.identity.scenarioSeq,
		Status:         result.status,
		SkipReason:     result.skipReason,
		Failure:        result.failure,
		SourceSpan:     result.sourceSpan,
		Preview:        result.preview,
		Contrast:       result.contrast,
		Observations:   result.observations,
		Eventually:     result.eventually,
		Payload:        result.payload,
	}, result.startedAt, result.endedAt))
}

func (n executionNode) observeNodeRef(attempt int) observe.NodeRef {
	return observe.NodeRef{
		Kind:           observeNodeKindFromReportKind(n.address.kind),
		StageID:        n.recorder.identity.stageID,
		ScenarioID:     n.recorder.identity.scenarioID,
		ScenarioCallID: n.recorder.identity.scenarioCallID,
		Path:           n.path,
		Attempt:        attempt,
	}
}
