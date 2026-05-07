package theater

type executionAddressSpace struct {
	scenarioCallPath string
	paths            runtimePathCodec
}

type executionNodeAddress struct {
	scenarioCallPath string
	actID            string
	kind             NodeKind
	nodeRef          string
}

func newExecutionAddressSpace(scenarioCallPath string) executionAddressSpace {
	return executionAddressSpace{
		scenarioCallPath: scenarioCallPath,
		paths:            runtimePathCodec{},
	}
}

func (s executionAddressSpace) scenarioPath() string {
	return s.scenarioCallPath
}

func (s executionAddressSpace) actPath(actID string) string {
	return s.paths.JoinChild(s.scenarioCallPath, "act", actID)
}

func (s executionAddressSpace) actionPath(actID string) string {
	return s.actPath(actID) + "/action"
}

func (s executionAddressSpace) expectationPath(actID, expectationID string) string {
	return s.paths.JoinChild(s.actPath(actID), "expectation", expectationID)
}

func (s executionAddressSpace) logPath(actID, logID string) string {
	return s.paths.JoinChild(s.actPath(actID), "log", logID)
}

func (s executionAddressSpace) scenarioAddress() executionNodeAddress {
	return executionNodeAddress{
		scenarioCallPath: s.scenarioCallPath,
		kind:             NodeKindScenario,
	}
}

func (s executionAddressSpace) actAddress(actID string) executionNodeAddress {
	return executionNodeAddress{
		scenarioCallPath: s.scenarioCallPath,
		actID:            actID,
		kind:             NodeKindAct,
	}
}

func (s executionAddressSpace) actionAddress(actID string) executionNodeAddress {
	return executionNodeAddress{
		scenarioCallPath: s.scenarioCallPath,
		actID:            actID,
		kind:             NodeKindAction,
		nodeRef:          "action",
	}
}

func (s executionAddressSpace) expectationAddress(actID, expectationID string) executionNodeAddress {
	return executionNodeAddress{
		scenarioCallPath: s.scenarioCallPath,
		actID:            actID,
		kind:             NodeKindExpectation,
		nodeRef:          expectationID,
	}
}

func (s executionAddressSpace) logAddress(actID, logID string) executionNodeAddress {
	return executionNodeAddress{
		scenarioCallPath: s.scenarioCallPath,
		actID:            actID,
		kind:             NodeKindLog,
		nodeRef:          logID,
	}
}

func (a executionNodeAddress) running(attempt int) *NodeAddress {
	return a.reportAddress(attempt, nil)
}

func (a executionNodeAddress) finished(attempt int, failure *Failure) *NodeAddress {
	return a.reportAddress(attempt, failure)
}

func (a executionNodeAddress) reportAddress(attempt int, failure *Failure) *NodeAddress {
	address := &NodeAddress{
		ScenarioCallPath: a.scenarioCallPath,
		ActID:            a.actID,
		Kind:             a.kind,
		NodeRef:          a.nodeRef,
		Phase:            a.phase(failure),
		AttemptIndex:     attempt,
	}

	if a.kind == NodeKindScenario {
		address.AttemptIndex = 0
	}

	return address
}

func (a executionNodeAddress) phase(failure *Failure) string {
	switch a.kind {
	case NodeKindAction:
		return "action.execute"
	case NodeKindExpectation:
		if failure != nil && failure.Kind == FailureKindObservation {
			return "subject.resolve"
		}

		return "assert.evaluate"
	case NodeKindAct:
		return "act.execute"
	case NodeKindScenario:
		return "scenario.execute"
	case NodeKindLog:
		return "log.evaluate"
	default:
		return ""
	}
}
