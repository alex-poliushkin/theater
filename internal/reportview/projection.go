package reportview

import reportmodel "github.com/alex-poliushkin/theater/report"

const (
	failurePriorityExpectation = iota
	failurePriorityAction
	failurePriorityAct
	failurePriorityScenario
	failurePriorityOther
)

type Projection struct {
	Document      reportmodel.RunDocument
	Report        reportmodel.Report
	Scenarios     []ScenarioView
	ConvergedActs int
	ExtraAttempts int
}

type ScenarioView struct {
	Node            reportmodel.NodeReport
	PrimaryFailure  *reportmodel.NodeReport
	TerminalFailure *reportmodel.NodeReport
	Eventually      *reportmodel.EventuallyReport
	SourceSpan      *reportmodel.SourceRef
}

func New(document reportmodel.RunDocument) *Projection {
	projection := &Projection{
		Document: document,
		Report:   document.Report,
	}

	failuresByScenario := make(map[string][]reportmodel.NodeReport)
	eventuallyByScenario := make(map[string]reportmodel.EventuallyReport)
	scenarios := make([]reportmodel.NodeReport, 0)

	for i := range document.Report.Nodes {
		node := document.Report.Nodes[i]
		if node.Kind == reportmodel.NodeKindScenario && node.Status.IsTerminal() {
			scenarios = append(scenarios, node)
		}

		if node.ScenarioPath == "" {
			continue
		}

		if node.Kind == reportmodel.NodeKindAct && node.Eventually != nil {
			projection.trackEventually(node, eventuallyByScenario)
		}

		if node.Status == reportmodel.StatusFailed && node.Failure != nil {
			failuresByScenario[node.ScenarioPath] = append(failuresByScenario[node.ScenarioPath], node)
		}
	}

	projection.Scenarios = make([]ScenarioView, 0, len(scenarios))
	for i := range scenarios {
		scenario := scenarios[i]
		failures := failuresByScenario[scenario.ScenarioPath]
		eventually := cloneEventually(eventuallyByScenario[scenario.ScenarioPath])
		primary := primaryFailureNode(failures, eventually)
		terminal := terminalFailureNode(scenario, failures, primary, eventually)
		projection.Scenarios = append(projection.Scenarios, ScenarioView{
			Node:            scenario,
			PrimaryFailure:  cloneNodeReport(primary),
			TerminalFailure: cloneNodeReport(terminal),
			Eventually:      eventually,
			SourceSpan:      bestSourceSpan(scenario, primary),
		})
	}

	return projection
}

func (p *Projection) HasFailedScenario() bool {
	for i := range p.Scenarios {
		if p.Scenarios[i].Node.Status == reportmodel.StatusFailed {
			return true
		}
	}

	return false
}

func (p *Projection) trackEventually(
	node reportmodel.NodeReport,
	eventuallyByScenario map[string]reportmodel.EventuallyReport,
) {
	if node.Eventually.AttemptsTotal > 1 {
		p.ExtraAttempts += node.Eventually.AttemptsTotal - 1
		if node.Eventually.FinalOutcome == reportmodel.StatusPassed {
			p.ConvergedActs++
		}
	}

	if node.ScenarioPath == "" {
		return
	}

	current, ok := eventuallyByScenario[node.ScenarioPath]
	if !ok || shouldReplaceEventually(current, *node.Eventually) {
		eventuallyByScenario[node.ScenarioPath] = *node.Eventually
	}
}

func shouldReplaceEventually(current, candidate reportmodel.EventuallyReport) bool {
	if current.FinalOutcome == reportmodel.StatusPassed && candidate.FinalOutcome != reportmodel.StatusPassed {
		return true
	}
	if candidate.AttemptsTotal > current.AttemptsTotal {
		return true
	}

	return false
}

func primaryFailureNode(
	failures []reportmodel.NodeReport,
	eventually *reportmodel.EventuallyReport,
) *reportmodel.NodeReport {
	failures = narrowFailuresToLastObservedKind(failures, eventually)
	return selectPreferredFailureNode(failures, shouldPreferLatestEventuallyFailure(eventually))
}

func terminalFailureNode(
	scenario reportmodel.NodeReport,
	failures []reportmodel.NodeReport,
	fallback *reportmodel.NodeReport,
	eventually *reportmodel.EventuallyReport,
) *reportmodel.NodeReport {
	if scenario.Failure == nil {
		return fallback
	}

	matchingKind := filterFailuresByKind(failures, scenario.Failure.Kind)
	if len(matchingKind) == 0 {
		return fallback
	}

	terminal := selectPreferredFailureNode(matchingKind, shouldPreferLatestEventuallyFailure(eventually))
	if terminal != nil {
		return terminal
	}

	return fallback
}

func selectPreferredFailureNode(
	failures []reportmodel.NodeReport,
	preferLatest bool,
) *reportmodel.NodeReport {
	var primary *reportmodel.NodeReport
	for i := range failures {
		node := &failures[i]
		if node.Status != reportmodel.StatusFailed || node.Failure == nil {
			continue
		}

		if primary == nil || shouldPreferFailureNode(*node, *primary, preferLatest) {
			primary = node
		}
	}

	return primary
}

func failurePriority(node reportmodel.NodeReport) int {
	switch node.Kind {
	case reportmodel.NodeKindExpectation:
		return failurePriorityExpectation
	case reportmodel.NodeKindAction:
		return failurePriorityAction
	case reportmodel.NodeKindAct:
		return failurePriorityAct
	case reportmodel.NodeKindScenario:
		return failurePriorityScenario
	default:
		return failurePriorityOther
	}
}

func narrowFailuresToLastObservedKind(
	failures []reportmodel.NodeReport,
	eventually *reportmodel.EventuallyReport,
) []reportmodel.NodeReport {
	if eventually == nil || eventually.AttemptsTotal <= 1 || eventually.LastObservedFailure == nil {
		return failures
	}

	matchingKind := filterFailuresByKind(failures, eventually.LastObservedFailure.Kind)
	if len(matchingKind) == 0 {
		return failures
	}

	return matchingKind
}

func filterFailuresByKind(
	failures []reportmodel.NodeReport,
	kind reportmodel.FailureKind,
) []reportmodel.NodeReport {
	matching := make([]reportmodel.NodeReport, 0, len(failures))
	for i := range failures {
		node := failures[i]
		if node.Status != reportmodel.StatusFailed || node.Failure == nil {
			continue
		}

		if node.Failure.Kind != kind {
			continue
		}

		matching = append(matching, node)
	}

	return matching
}

func shouldPreferLatestEventuallyFailure(eventually *reportmodel.EventuallyReport) bool {
	return eventually != nil && eventually.AttemptsTotal > 1 && eventually.FinalOutcome == reportmodel.StatusFailed
}

func shouldPreferFailureNode(
	candidate reportmodel.NodeReport,
	current reportmodel.NodeReport,
	preferLatest bool,
) bool {
	if failurePriority(candidate) != failurePriority(current) {
		return failurePriority(candidate) < failurePriority(current)
	}

	if !preferLatest {
		return false
	}

	if candidate.Attempt != current.Attempt {
		return candidate.Attempt > current.Attempt
	}

	if laterNodeEnd(candidate, current) {
		return true
	}
	if laterNodeEnd(current, candidate) {
		return false
	}

	if nodeAttemptIndex(candidate) != nodeAttemptIndex(current) {
		return nodeAttemptIndex(candidate) > nodeAttemptIndex(current)
	}

	return false
}

func laterNodeEnd(left, right reportmodel.NodeReport) bool {
	if left.EndedAt.IsZero() {
		return false
	}
	if right.EndedAt.IsZero() {
		return true
	}

	return left.EndedAt.After(right.EndedAt)
}

func nodeAttemptIndex(node reportmodel.NodeReport) int {
	if node.Address == nil {
		return 0
	}

	return node.Address.AttemptIndex
}

func bestSourceSpan(
	scenario reportmodel.NodeReport,
	primaryFailure *reportmodel.NodeReport,
) *reportmodel.SourceRef {
	if primaryFailure != nil && primaryFailure.SourceSpan != nil {
		return cloneSourceRef(primaryFailure.SourceSpan)
	}
	if scenario.SourceSpan != nil {
		return cloneSourceRef(scenario.SourceSpan)
	}

	return nil
}

func cloneNodeReport(node *reportmodel.NodeReport) *reportmodel.NodeReport {
	if node == nil {
		return nil
	}

	cloned := *node
	return &cloned
}

func cloneEventually(eventually reportmodel.EventuallyReport) *reportmodel.EventuallyReport {
	if eventually.AttemptsTotal == 0 && eventually.FinalOutcome == "" && eventually.TerminationReason == "" {
		return nil
	}

	cloned := eventually
	return &cloned
}

func cloneSourceRef(source *reportmodel.SourceRef) *reportmodel.SourceRef {
	if source == nil {
		return nil
	}

	cloned := *source
	return &cloned
}
