package theater

import "github.com/alex-poliushkin/theater/observe"

const (
	nodeReportRankScenario = iota
	nodeReportRankAction
	nodeReportRankExpectation
	nodeReportRankAct
	nodeReportRankOther
)

type runtimeNodeMetadata struct {
	reportKind    NodeKind
	reportRank    int
	observeKind   observe.NodeKind
	runningEvent  string
	finishedEvent string
	mirrorable    bool
}

var runtimeNodeMetadataTable = [...]runtimeNodeMetadata{
	{
		observeKind:   observe.NodeKindStage,
		runningEvent:  EventKindStageRunning,
		finishedEvent: EventKindStageFinished,
		mirrorable:    true,
	},
	{
		reportKind:    NodeKindScenario,
		reportRank:    nodeReportRankScenario,
		observeKind:   observe.NodeKindScenario,
		runningEvent:  EventKindScenarioRunning,
		finishedEvent: EventKindScenarioFinished,
		mirrorable:    true,
	},
	{
		reportKind:    NodeKindAct,
		reportRank:    nodeReportRankAct,
		observeKind:   observe.NodeKindAct,
		runningEvent:  EventKindActRunning,
		finishedEvent: EventKindActFinished,
		mirrorable:    true,
	},
	{
		reportKind:    NodeKindAction,
		reportRank:    nodeReportRankAction,
		observeKind:   observe.NodeKindAction,
		runningEvent:  EventKindActionRunning,
		finishedEvent: EventKindActionFinished,
		mirrorable:    true,
	},
	{
		reportKind:    NodeKindExpectation,
		reportRank:    nodeReportRankExpectation,
		observeKind:   observe.NodeKindScenario,
		finishedEvent: EventKindExpectationFinished,
	},
}

func nodeReportKindFromEventKind(kind string) (NodeKind, bool) {
	metadata, ok := runtimeNodeMetadataForEventKind(kind)
	if !ok || kind != metadata.finishedEvent || metadata.reportKind == "" {
		return "", false
	}

	return metadata.reportKind, true
}

func mirroredObserveNodeKindFromEventKind(kind string) (observe.NodeKind, bool) {
	metadata, ok := runtimeNodeMetadataForEventKind(kind)
	if !ok || !metadata.mirrorable {
		return "", false
	}

	return metadata.observeKind, true
}

func observeNodeKindFromReportKind(kind NodeKind) observe.NodeKind {
	if kind == NodeKindLog {
		return observe.NodeKindLog
	}

	metadata, ok := runtimeNodeMetadataForReportKind(kind)
	if !ok {
		return observe.NodeKindScenario
	}

	return metadata.observeKind
}

func nodeReportKindRank(kind NodeKind) int {
	metadata, ok := runtimeNodeMetadataForReportKind(kind)
	if !ok {
		return nodeReportRankOther
	}

	return metadata.reportRank
}

func runtimeNodeMetadataForEventKind(kind string) (runtimeNodeMetadata, bool) {
	for i := range runtimeNodeMetadataTable {
		metadata := runtimeNodeMetadataTable[i]
		if kind == metadata.runningEvent || kind == metadata.finishedEvent {
			return metadata, true
		}
	}

	return runtimeNodeMetadata{}, false
}

func runtimeNodeMetadataForReportKind(kind NodeKind) (runtimeNodeMetadata, bool) {
	for i := range runtimeNodeMetadataTable {
		metadata := runtimeNodeMetadataTable[i]
		if metadata.reportKind == kind {
			return metadata, true
		}
	}

	return runtimeNodeMetadata{}, false
}
