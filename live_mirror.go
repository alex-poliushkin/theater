package theater

import (
	"fmt"

	"github.com/alex-poliushkin/theater/observe"
)

const (
	liveScenarioLogStream          = "log"
	liveScenarioLogNoPreviewText   = "<no preview>"
	liveScenarioLogTruncatedSuffix = " [truncated]"
)

func mirroredEnvelopeFromEvent(event Event) (observe.Envelope, bool) {
	if event.Kind == EventKindLogEmitted {
		return mirroredLogEnvelopeFromEvent(event)
	}

	nodeKind, ok := mirroredObserveNodeKindFromEventKind(event.Kind)
	if !ok {
		return observe.Envelope{}, false
	}

	envelope := observe.Envelope{
		Kind:          observe.KindTransition,
		DurableMirror: true,
		Node: observe.NodeRef{
			Kind:           nodeKind,
			StageID:        event.StageID,
			ScenarioID:     event.ScenarioID,
			ScenarioCallID: event.ScenarioCallID,
			Path:           event.Path,
			Attempt:        event.Attempt,
		},
		Transition: &observe.Transition{
			EventKind: event.Kind,
			Status:    string(event.Status),
		},
	}

	if event.Failure != nil {
		envelope.Transition.FailureKind = string(event.Failure.Kind)
		envelope.Transition.FailureAt = event.Failure.At
		envelope.Transition.FailureSummary = event.Failure.Message()
	}

	return envelope, true
}

func mirroredLogEnvelopeFromEvent(event Event) (observe.Envelope, bool) {
	if event.Log == nil || event.Log.Dropped {
		return observe.Envelope{}, false
	}

	text := liveScenarioLogText(*event.Log)
	if text == "" {
		return observe.Envelope{}, false
	}

	return observe.Envelope{
		Kind:          observe.KindLogChunk,
		DurableMirror: true,
		Node: observe.NodeRef{
			Kind:           observe.NodeKindLog,
			StageID:        event.StageID,
			ScenarioID:     event.ScenarioID,
			ScenarioCallID: event.ScenarioCallID,
			Path:           event.Path,
			Attempt:        event.Attempt,
		},
		LogChunk: &observe.LogChunk{
			Stream: liveScenarioLogStream,
			Data:   []byte(text + "\n"),
		},
	}, true
}

func liveScenarioLogText(record LogRecord) string {
	switch record.Status {
	case LogStatusEmitted:
		return fmt.Sprintf("log %s: %s", record.ID, liveScenarioLogPreviewText(record))
	case LogStatusOmitted:
		return fmt.Sprintf("log %s omitted: %s", record.ID, liveScenarioLogPreviewText(record))
	case LogStatusError:
		return fmt.Sprintf("log %s error: %s", record.ID, liveScenarioLogFailureSummary(record))
	default:
		return ""
	}
}

func liveScenarioLogPreviewText(record LogRecord) string {
	if record.Preview == nil {
		return liveScenarioLogNoPreviewText
	}

	switch {
	case record.Preview.Text != "":
		text := record.Preview.Text
		if record.Preview.Truncated || record.Truncated {
			text += liveScenarioLogTruncatedSuffix
		}
		return text
	case record.Preview.OmittedReason != "":
		return "<" + record.Preview.OmittedReason + ">"
	default:
		return liveScenarioLogNoPreviewText
	}
}

func liveScenarioLogFailureSummary(record LogRecord) string {
	if record.Failure == nil || record.Failure.Summary == "" {
		return "<evaluation failed>"
	}

	return record.Failure.Summary
}
