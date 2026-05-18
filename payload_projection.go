package theater

import (
	"slices"
	"strconv"
	"strings"

	"github.com/alex-poliushkin/theater/internal/secretvalue"
	"github.com/alex-poliushkin/theater/internal/streamtext"
)

const redactedPreview = secretvalue.RedactedText

// ProjectedPayload is a sanitized preview plus normalized payload metadata.
type ProjectedPayload struct {
	Preview  string
	Metadata PayloadMetadata
}

func (reportProjector) Project(events []Event) (Report, error) {
	accumulator := newReportAccumulator()

	for i := range events {
		if err := accumulator.Apply(events[i]); err != nil {
			return Report{}, err
		}
	}

	return accumulator.Report()
}

func deriveStageStatus(summary Summary) Status {
	switch {
	case summary.TotalScenarios == 0:
		return StatusPassed
	case summary.FailedScenarios > 0:
		return StatusFailed
	case summary.CanceledScenarios > 0:
		return StatusCanceled
	case summary.PassedScenarios == summary.TotalScenarios:
		return StatusPassed
	case summary.SkippedScenarios == summary.TotalScenarios:
		return StatusSkipped
	default:
		return StatusPassed
	}
}

// SafeProjectText projects raw text into a report-safe preview using the given
// payload metadata and preview limit.
func SafeProjectText(raw string, metadata PayloadMetadata, limit int) ProjectedPayload {
	projected := ProjectedPayload{
		Metadata: metadata,
	}

	switch metadata.Capture {
	case CaptureOmit:
		return projected
	case CaptureSummary, CaptureArtifactRef:
	default:
		return projected
	}

	if metadata.Sensitivity == SensitivitySecret {
		projected.Metadata.Redacted = true
		projected.Preview = redactedPreview
		return projected
	}

	sanitized := sanitizeProjection(raw)
	if limit >= 0 && len(sanitized) > limit {
		projected.Metadata.Truncated = true
		projected.Preview = truncateProjection(sanitized, limit)
		return projected
	}

	projected.Preview = sanitized
	return projected
}

func nodeReportFromEvent(event Event) (NodeReport, bool) {
	if !event.Status.IsTerminal() {
		return NodeReport{}, false
	}

	kind, ok := nodeReportKindFromEventKind(event.Kind)
	if !ok {
		return NodeReport{}, false
	}

	return NodeReport{
		ID:             nodeStableID(event),
		Kind:           kind,
		StageID:        event.StageID,
		Path:           event.Path,
		ScenarioID:     event.ScenarioID,
		ScenarioCallID: event.ScenarioCallID,
		ScenarioPath:   event.ScenarioPath,
		Attempt:        event.Attempt,
		ScenarioSeq:    event.ScenarioSeq,
		Status:         event.Status,
		SkipReason:     event.SkipReason,
		Failure:        event.Failure,
		StartedAt:      event.StartedAt,
		EndedAt:        event.EndedAt,
		DurationMs:     event.DurationMs,
		Address:        cloneNodeAddress(event.Address),
		SourceSpan:     cloneSourceRef(event.SourceSpan),
		Preview:        clonePreview(event.Preview),
		Artifacts:      artifactRefsFromEvent(event),
		Contrast:       cloneContrast(event.Contrast),
		Observations:   cloneActionObservations(event.Observations),
		Diagnostics:    cloneNodeDiagnostics(event.Diagnostics),
		Eventually:     cloneEventuallyReport(event.Eventually),
		Payload:        clonePayloadMetadata(event.Payload),
	}, true
}

func nodeStableID(event Event) string {
	if event.Attempt <= 1 {
		return event.Path
	}

	return event.Path + "#attempt." + strconv.Itoa(event.Attempt)
}

func sortNodeReports(nodes []NodeReport) {
	slices.SortFunc(nodes, func(left, right NodeReport) int {
		if left.ScenarioSeq != right.ScenarioSeq {
			if left.ScenarioSeq < right.ScenarioSeq {
				return -1
			}

			return 1
		}

		if left.Path != right.Path {
			if left.Path < right.Path {
				return -1
			}

			return 1
		}

		if left.Attempt != right.Attempt {
			if left.Attempt < right.Attempt {
				return -1
			}

			return 1
		}

		leftRank := nodeReportKindRank(left.Kind)
		rightRank := nodeReportKindRank(right.Kind)
		if leftRank < rightRank {
			return -1
		}
		if leftRank > rightRank {
			return 1
		}

		return 0
	})
}

func sortLogRecords(logs []LogRecord) {
	slices.SortFunc(logs, func(left, right LogRecord) int {
		if left.ScenarioSeq != right.ScenarioSeq {
			if left.ScenarioSeq < right.ScenarioSeq {
				return -1
			}

			return 1
		}

		if left.Path != right.Path {
			if left.Path < right.Path {
				return -1
			}

			return 1
		}

		if left.Attempt != right.Attempt {
			if left.Attempt < right.Attempt {
				return -1
			}

			return 1
		}

		return 0
	})
}

func sanitizeProjection(raw string) string {
	replacer := strings.NewReplacer(
		"\r\n", "\\n",
		"\r", "\\r",
		"\n", "\\n",
	)

	return replacer.Replace(raw)
}

func truncateProjection(value string, limit int) string {
	truncated, _ := streamtext.TruncateSuffix(value, limit, "...")
	return truncated
}

func artifactRefsFromEvent(event Event) []ArtifactRef {
	if event.Payload == nil || event.Payload.ArtifactRef == "" {
		return nil
	}

	artifacts := []ArtifactRef{
		{
			Name:             "payload",
			Kind:             "payload",
			ContentType:      event.Payload.ContentType,
			Locator:          event.Payload.ArtifactRef,
			SizeBytes:        event.Payload.SizeBytes,
			CreatedByAddress: cloneNodeAddress(event.Address),
			Sensitive:        event.Payload.Sensitivity != SensitivityNone,
			PreviewAvailable: event.Payload.Capture == CaptureSummary,
		},
	}

	return artifacts
}

func buildFailureIndex(report Report) []FailureIndexEntry {
	entries := make([]FailureIndexEntry, 0)

	for i := range report.Nodes {
		node := report.Nodes[i]
		if node.Status != StatusFailed || node.Failure == nil {
			continue
		}

		switch node.Kind {
		case NodeKindAction, NodeKindExpectation:
			entries = append(entries, failureEntryFromNode(node))
		}
	}

	if len(entries) > 0 {
		return entries
	}

	for i := range report.Nodes {
		node := report.Nodes[i]
		if node.Status != StatusFailed || node.Failure == nil {
			continue
		}

		switch node.Kind {
		case NodeKindAct, NodeKindScenario:
			entries = append(entries, failureEntryFromNode(node))
		}
	}

	if len(entries) > 0 {
		return entries
	}

	if report.Failure == nil {
		return nil
	}

	return []FailureIndexEntry{
		{
			Path:    report.StagePath,
			Failure: report.Failure,
		},
	}
}

func failureEntryFromNode(node NodeReport) FailureIndexEntry {
	return FailureIndexEntry{
		Path:       node.Path,
		Address:    cloneNodeAddress(node.Address),
		SourceSpan: cloneSourceRef(node.SourceSpan),
		Failure:    node.Failure,
	}
}

func cloneSourceRef(source *SourceRef) *SourceRef {
	if source == nil {
		return nil
	}

	cloned := *source
	return &cloned
}

func clonePreview(preview *Preview) *Preview {
	if preview == nil {
		return nil
	}

	cloned := *preview
	return &cloned
}

func cloneContrast(contrast *Contrast) *Contrast {
	if contrast == nil {
		return nil
	}

	cloned := *contrast
	cloned.Expected = clonePreview(contrast.Expected)
	cloned.Actual = clonePreview(contrast.Actual)
	return &cloned
}

func cloneFailure(failure *Failure) *Failure {
	if failure == nil {
		return nil
	}

	cloned := *failure
	return &cloned
}

func cloneNodeAddress(address *NodeAddress) *NodeAddress {
	if address == nil {
		return nil
	}

	cloned := *address
	return &cloned
}

func cloneNodeDiagnostics(diagnostics []NodeDiagnostic) []NodeDiagnostic {
	if len(diagnostics) == 0 {
		return nil
	}

	cloned := make([]NodeDiagnostic, len(diagnostics))
	for i := range diagnostics {
		cloned[i] = cloneNodeDiagnostic(diagnostics[i])
	}

	return cloned
}

func cloneNodeDiagnostic(diagnostic NodeDiagnostic) NodeDiagnostic {
	cloned := diagnostic
	if diagnostic.HTTP != nil {
		httpDiagnostic := *diagnostic.HTTP
		httpDiagnostic.ActionAddress = cloneNodeAddress(diagnostic.HTTP.ActionAddress)
		httpDiagnostic.ResponsePreview = clonePreview(diagnostic.HTTP.ResponsePreview)
		if len(diagnostic.HTTP.ResponseHeaders) != 0 {
			httpDiagnostic.ResponseHeaders = make(map[string][]string, len(diagnostic.HTTP.ResponseHeaders))
			for key, values := range diagnostic.HTTP.ResponseHeaders {
				httpDiagnostic.ResponseHeaders[key] = append([]string(nil), values...)
			}
		}
		cloned.HTTP = &httpDiagnostic
	}

	return cloned
}

func cloneLogRecord(record LogRecord) LogRecord {
	cloned := record
	cloned.SourceSpan = cloneSourceRef(record.SourceSpan)
	cloned.Address = cloneNodeAddress(record.Address)
	cloned.Preview = clonePreview(record.Preview)
	cloned.Payload = clonePayloadMetadata(record.Payload)
	cloned.Failure = cloneFailure(record.Failure)
	return cloned
}

func cloneEventuallyReport(report *EventuallyReport) *EventuallyReport {
	if report == nil {
		return nil
	}

	cloned := *report
	if len(report.AttemptTimeline) != 0 {
		cloned.AttemptTimeline = make([]AttemptReport, len(report.AttemptTimeline))
		copy(cloned.AttemptTimeline, report.AttemptTimeline)
	}
	return &cloned
}
