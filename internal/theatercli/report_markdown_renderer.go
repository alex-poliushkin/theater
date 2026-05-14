package theatercli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/alex-poliushkin/theater/internal/reportview"
	reportmodel "github.com/alex-poliushkin/theater/report"
)

const (
	reportMarkdownScenarioLimit = 128
	reportMarkdownNodeLimit     = 512
	reportMarkdownLogLimit      = 256
	reportMarkdownPreviewLimit  = 500

	reportMarkdownEmpty       = "<empty>"
	reportMarkdownRedacted    = "<redacted>"
	reportMarkdownUnavailable = "<unavailable>"
)

type reportMarkdownRenderer struct{}

type reportMarkdownView struct {
	file       string
	document   reportmodel.RunDocument
	projection *reportview.Projection
	nodes      map[string][]reportmodel.NodeReport
	logs       map[string][]reportmodel.LogRecord
}

func newReportMarkdownRenderer() reportMarkdownRenderer {
	return reportMarkdownRenderer{}
}

func (r reportMarkdownRenderer) Write(writer io.Writer, file string, document reportmodel.RunDocument) error {
	_, err := io.WriteString(writer, newReportMarkdownView(file, document).String())
	return err
}

func newReportMarkdownView(file string, document reportmodel.RunDocument) reportMarkdownView {
	return reportMarkdownView{
		file:       file,
		document:   document,
		projection: reportview.New(document),
		nodes:      groupReportNodesByScenario(document.Report.Nodes),
		logs:       groupReportLogsByScenario(document.Report.Logs),
	}
}

func (v reportMarkdownView) String() string {
	var builder strings.Builder
	builder.WriteString("# Theater Run Report\n\n")
	renderMarkdownRunSummary(&builder, v.file, v.document)
	renderMarkdownDiagnostics(&builder, v.document.Diagnostics)
	if v.document.Report.Failure != nil && !v.projection.HasFailedScenario() {
		builder.WriteString("\n## Run Failure\n\n")
		renderMarkdownFailure(&builder, "", nil, v.document.Report.Failure, nil)
	}
	v.renderScenarios(&builder)
	return builder.String()
}

func renderMarkdownRunSummary(builder *strings.Builder, file string, document reportmodel.RunDocument) {
	report := document.Report
	if file != "" {
		fmt.Fprintf(builder, "- File: %s\n", markdownCode(file))
	}
	fmt.Fprintf(builder, "- Stage: %s\n", markdownCode(emptyFallback(report.StageID, report.StagePath)))
	fmt.Fprintf(builder, "- Status: %s\n", markdownCode(string(report.Status)))
	if report.DurationMs > 0 {
		fmt.Fprintf(builder, "- Duration: %s\n", markdownCode(humanDuration(report.DurationMs)))
	}
	fmt.Fprintf(
		builder,
		"- Scenarios: total=%d passed=%d failed=%d canceled=%d skipped=%d\n",
		report.Summary.TotalScenarios,
		report.Summary.PassedScenarios,
		report.Summary.FailedScenarios,
		report.Summary.CanceledScenarios,
		report.Summary.SkippedScenarios,
	)
	if document.SchemaVersion != "" {
		fmt.Fprintf(builder, "- Schema: %s\n", markdownCode(document.SchemaVersion))
	}
}

func renderMarkdownDiagnostics(builder *strings.Builder, diagnostics []reportmodel.Diagnostic) {
	if len(diagnostics) == 0 {
		return
	}

	builder.WriteString("\n## Diagnostics\n\n")
	for i := range diagnostics {
		diagnostic := diagnostics[i]
		fmt.Fprintf(
			builder,
			"- %s %s: %s\n",
			markdownCode(diagnostic.Code),
			markdownCode(emptyFallback(diagnostic.Path, "<unknown>")),
			markdownText(diagnostic.Summary),
		)
		if source := formatSourceSpan(&diagnostic.Span); source != "" {
			fmt.Fprintf(builder, "  - Source: %s\n", markdownCode(source))
		}
		if breadcrumb := formatDiagnosticBreadcrumb(diagnostic.Path); breadcrumb != "" {
			fmt.Fprintf(builder, "  - Breadcrumb: %s\n", markdownCode(breadcrumb))
		}
	}
}

func (v reportMarkdownView) renderScenarios(builder *strings.Builder) {
	if len(v.projection.Scenarios) == 0 {
		return
	}

	builder.WriteString("\n## Scenarios\n")
	limit := min(len(v.projection.Scenarios), reportMarkdownScenarioLimit)
	for i := 0; i < limit; i++ {
		v.renderScenario(builder, v.projection.Scenarios[i])
	}
	if omitted := len(v.projection.Scenarios) - limit; omitted > 0 {
		fmt.Fprintf(builder, "\n_Omitted %d scenario(s) after the first %d._\n", omitted, limit)
	}
}

func (v reportMarkdownView) renderScenario(builder *strings.Builder, scenario reportview.ScenarioView) {
	node := scenario.Node
	key := node.Path
	builder.WriteString("\n")
	fmt.Fprintf(builder, "### Scenario %s\n\n", markdownCode(emptyFallback(node.ScenarioCallID, key)))
	fmt.Fprintf(builder, "- Scenario: %s\n", markdownCode(emptyFallback(node.ScenarioID, "<unknown>")))
	fmt.Fprintf(builder, "- Status: %s\n", markdownCode(string(node.Status)))
	if source := formatSourceSpan(scenario.SourceSpan); source != "" {
		fmt.Fprintf(builder, "- Source: %s\n", markdownCode(source))
	}
	if node.DurationMs > 0 {
		fmt.Fprintf(builder, "- Duration: %s\n", markdownCode(humanDuration(node.DurationMs)))
	}
	if scenario.PrimaryFailure != nil && scenario.PrimaryFailure.Failure != nil {
		renderMarkdownFailure(builder, "", scenario.PrimaryFailure, scenario.PrimaryFailure.Failure, scenario.PrimaryFailure.Observations)
	} else if node.Failure != nil {
		renderMarkdownFailure(builder, "", &node, node.Failure, node.Observations)
	}

	v.renderScenarioNodes(builder, key)
}

func (v reportMarkdownView) renderScenarioNodes(builder *strings.Builder, scenarioPath string) {
	nodes := v.nodes[scenarioPath]
	logs := v.logs[scenarioPath]
	acts := filterMarkdownNodes(nodes, reportmodel.NodeKindAct, "")
	renderedNodes := 0
	renderedLogs := 0
	for i := range acts {
		if renderedNodes >= reportMarkdownNodeLimit {
			break
		}
		act := acts[i]
		actID := markdownNodeActID(act)
		renderMarkdownAct(builder, act)
		renderedNodes++

		expectations := filterMarkdownNodes(nodes, reportmodel.NodeKindExpectation, actID)
		for j := range expectations {
			if renderedNodes >= reportMarkdownNodeLimit {
				break
			}
			renderMarkdownExpectation(builder, expectations[j])
			renderedNodes++
		}

		actLogs := filterMarkdownLogs(logs, actID)
		for j := range actLogs {
			if renderedLogs >= reportMarkdownLogLimit {
				break
			}
			renderMarkdownLog(builder, actLogs[j])
			renderedLogs++
		}
	}

	renderedNodes += renderUnattachedMarkdownNodes(builder, nodes, renderedNodes)
	renderedLogs += renderUnattachedMarkdownLogs(builder, logs, renderedLogs)
	if omitted := countRenderableMarkdownNodes(nodes) - renderedNodes; omitted > 0 {
		fmt.Fprintf(builder, "- Omitted %d node(s) after the first %d.\n", omitted, reportMarkdownNodeLimit)
	}
	if omitted := len(logs) - renderedLogs; omitted > 0 {
		fmt.Fprintf(builder, "- Omitted %d log record(s) after the first %d.\n", omitted, reportMarkdownLogLimit)
	}
}

func renderMarkdownAct(builder *strings.Builder, node reportmodel.NodeReport) {
	fmt.Fprintf(builder, "- Act %s %s\n", markdownCode(emptyFallback(markdownNodeActID(node), node.Path)), node.Status)
	if node.DurationMs > 0 {
		fmt.Fprintf(builder, "  - Duration: %s\n", markdownCode(humanDuration(node.DurationMs)))
	}
	if source := formatSourceSpan(node.SourceSpan); source != "" {
		fmt.Fprintf(builder, "  - Source: %s\n", markdownCode(source))
	}
	if node.Eventually != nil {
		fmt.Fprintf(builder, "  - Eventually: %s\n", renderMarkdownEventually(*node.Eventually))
	}
	if node.Failure != nil {
		renderMarkdownFailure(builder, "  ", &node, node.Failure, node.Observations)
	}
}

func renderMarkdownExpectation(builder *strings.Builder, node reportmodel.NodeReport) {
	fmt.Fprintf(builder, "  - Expectation %s %s\n", markdownCode(emptyFallback(markdownNodeRef(node), node.Path)), node.Status)
	if source := formatSourceSpan(node.SourceSpan); source != "" {
		fmt.Fprintf(builder, "    - Source: %s\n", markdownCode(source))
	}
	if node.Failure != nil {
		renderMarkdownFailure(builder, "    ", &node, node.Failure, node.Observations)
	}
}

func renderMarkdownLog(builder *strings.Builder, log reportmodel.LogRecord) {
	fmt.Fprintf(
		builder,
		"  - Log %s %s: %s\n",
		markdownCode(log.ID),
		log.Status,
		renderMarkdownPreview(log.Preview, log.Truncated),
	)
}

func renderMarkdownFailure(
	builder *strings.Builder,
	indent string,
	node *reportmodel.NodeReport,
	failure *reportmodel.Failure,
	observations *reportmodel.ActionObservations,
) {
	if failure == nil {
		return
	}

	fmt.Fprintf(builder, "%sFailure: %s\n", indent, markdownText(failure.Summary))
	fmt.Fprintf(builder, "%sKind: %s\n", indent, markdownCode(string(failure.Kind)))
	fmt.Fprintf(builder, "%sAt: %s\n", indent, markdownCode(failure.At))
	if node != nil {
		if actID := primaryActID(*node); actID != "" {
			fmt.Fprintf(builder, "%s- Act: %s\n", indent, markdownCode(actID))
		}
		if source := formatSourceSpan(node.SourceSpan); source != "" {
			fmt.Fprintf(builder, "%s- Source: %s\n", indent, markdownCode(source))
		}
		renderMarkdownContrast(builder, indent, node.Contrast)
	}
	if observations != nil {
		renderMarkdownObservedMap(builder, indent, "Input", observations.Inputs)
		renderMarkdownObservedMap(builder, indent, "Output", filteredObservedValues(observations.Outputs, observations.Streams))
		renderMarkdownObservedStreams(builder, indent, observations.Streams)
	}
}

func renderMarkdownContrast(builder *strings.Builder, indent string, contrast *reportmodel.Contrast) {
	if contrast == nil {
		return
	}
	if contrast.Summary != "" {
		fmt.Fprintf(builder, "%s- Contrast: %s\n", indent, markdownText(contrast.Summary))
	}
	if contrast.Expected != nil {
		fmt.Fprintf(builder, "%s- Expected: %s\n", indent, renderMarkdownPreview(contrast.Expected, contrast.Expected.Truncated))
	}
	if contrast.Actual != nil {
		fmt.Fprintf(builder, "%s- Actual: %s\n", indent, renderMarkdownPreview(contrast.Actual, contrast.Actual.Truncated))
	}
	if contrast.Excerpt != "" {
		fmt.Fprintf(builder, "%s- Excerpt: %s\n", indent, markdownText(contrast.Excerpt))
	}
}

func renderMarkdownObservedMap(
	builder *strings.Builder,
	indent string,
	title string,
	observed map[string]reportmodel.ObservedValue,
) {
	for _, key := range orderedObservedKeys(observed) {
		value := observed[key]
		truncated := value.Preview != nil && value.Preview.Truncated
		fmt.Fprintf(builder, "%s- %s %s: %s\n", indent, title, markdownCode(key), renderMarkdownPreview(value.Preview, truncated))
	}
}

func renderMarkdownObservedStreams(
	builder *strings.Builder,
	indent string,
	observed map[string]reportmodel.ObservedStream,
) {
	for _, key := range orderedObservedStreamKeys(observed) {
		value := observed[key]
		truncated := value.Preview != nil && value.Preview.Truncated
		rendered := renderMarkdownPreview(value.Preview, truncated)
		if value.DroppedChunks != 0 {
			rendered += fmt.Sprintf(" (live drops=%d)", value.DroppedChunks)
		}
		fmt.Fprintf(builder, "%s- Stream %s: %s\n", indent, markdownCode(key), rendered)
	}
}

func renderMarkdownEventually(eventually reportmodel.EventuallyReport) string {
	parts := []string{
		"attempts=" + strconv.Itoa(eventually.AttemptsTotal),
		"termination=" + string(eventually.TerminationReason),
	}
	if eventually.SuccessAttempt > 0 {
		parts = append(parts, "success_attempt="+strconv.Itoa(eventually.SuccessAttempt))
	}
	if eventually.ElapsedMs > 0 {
		parts = append(parts, "elapsed="+humanDuration(eventually.ElapsedMs))
	}
	if eventually.LastObservedFailure != nil {
		parts = append(parts, "last="+strconv.Quote(boundedMarkdownText(eventually.LastObservedFailure.Message())))
	}
	return strings.Join(parts, " ")
}

func renderMarkdownPreview(preview *reportmodel.Preview, truncated bool) string {
	if preview == nil {
		return reportMarkdownUnavailable
	}

	var rendered string
	switch {
	case preview.Redacted:
		rendered = reportMarkdownRedacted
	case preview.OmittedReason != "":
		rendered = "<" + preview.OmittedReason + ">"
	case preview.Text != "":
		rendered = markdownCode(preview.Text)
	default:
		rendered = reportMarkdownEmpty
	}
	if truncated || preview.Truncated {
		rendered += " (truncated)"
	}
	return rendered
}

func groupReportNodesByScenario(nodes []reportmodel.NodeReport) map[string][]reportmodel.NodeReport {
	grouped := make(map[string][]reportmodel.NodeReport)
	for i := range nodes {
		node := nodes[i]
		if node.Kind == reportmodel.NodeKindScenario {
			continue
		}
		key := markdownNodeScenarioPath(node)
		if key == "" {
			continue
		}
		grouped[key] = append(grouped[key], node)
	}
	return grouped
}

func groupReportLogsByScenario(logs []reportmodel.LogRecord) map[string][]reportmodel.LogRecord {
	grouped := make(map[string][]reportmodel.LogRecord)
	for i := range logs {
		log := logs[i]
		key := log.ScenarioPath
		if key == "" && log.Address != nil {
			key = log.Address.ScenarioCallPath
		}
		if key == "" {
			continue
		}
		grouped[key] = append(grouped[key], log)
	}
	return grouped
}

func filterMarkdownNodes(nodes []reportmodel.NodeReport, kind reportmodel.NodeKind, actID string) []reportmodel.NodeReport {
	filtered := make([]reportmodel.NodeReport, 0)
	for i := range nodes {
		node := nodes[i]
		if node.Kind != kind {
			continue
		}
		if actID != "" && markdownNodeActID(node) != actID {
			continue
		}
		filtered = append(filtered, node)
	}
	return filtered
}

func filterMarkdownLogs(logs []reportmodel.LogRecord, actID string) []reportmodel.LogRecord {
	filtered := make([]reportmodel.LogRecord, 0)
	for i := range logs {
		if logs[i].ActID == actID {
			filtered = append(filtered, logs[i])
		}
	}
	return filtered
}

func renderUnattachedMarkdownNodes(builder *strings.Builder, nodes []reportmodel.NodeReport, alreadyRendered int) int {
	rendered := 0
	attachedActs := make(map[string]struct{})
	for i := range nodes {
		if nodes[i].Kind == reportmodel.NodeKindAct {
			attachedActs[markdownNodeActID(nodes[i])] = struct{}{}
		}
	}
	for i := range nodes {
		if alreadyRendered+rendered >= reportMarkdownNodeLimit {
			break
		}
		node := nodes[i]
		if node.Kind != reportmodel.NodeKindExpectation {
			continue
		}
		if _, ok := attachedActs[markdownNodeActID(node)]; ok {
			continue
		}
		renderMarkdownExpectation(builder, node)
		rendered++
	}
	return rendered
}

func renderUnattachedMarkdownLogs(builder *strings.Builder, logs []reportmodel.LogRecord, alreadyRendered int) int {
	rendered := 0
	attachedActs := make(map[string]struct{})
	for i := range logs {
		if logs[i].ActID != "" {
			attachedActs[logs[i].ActID] = struct{}{}
		}
	}
	if len(attachedActs) == 0 {
		for i := range logs {
			if alreadyRendered+rendered >= reportMarkdownLogLimit {
				break
			}
			renderMarkdownLog(builder, logs[i])
			rendered++
		}
	}
	return rendered
}

func countRenderableMarkdownNodes(nodes []reportmodel.NodeReport) int {
	count := 0
	for i := range nodes {
		switch nodes[i].Kind {
		case reportmodel.NodeKindAct, reportmodel.NodeKindExpectation:
			count++
		}
	}
	return count
}

func markdownNodeScenarioPath(node reportmodel.NodeReport) string {
	if node.Address != nil && node.Address.ScenarioCallPath != "" {
		return node.Address.ScenarioCallPath
	}
	return node.ScenarioPath
}

func markdownNodeActID(node reportmodel.NodeReport) string {
	if node.Address != nil && node.Address.ActID != "" {
		return node.Address.ActID
	}
	return ""
}

func markdownNodeRef(node reportmodel.NodeReport) string {
	if node.Address != nil && node.Address.NodeRef != "" {
		return node.Address.NodeRef
	}
	return ""
}

func markdownCode(value string) string {
	value = boundedMarkdownText(value)
	if value == "" {
		value = reportMarkdownEmpty
	}
	if strings.Contains(value, "`") {
		return "`` " + strings.ReplaceAll(value, "`", "'") + " ``"
	}
	return "`" + value + "`"
}

func markdownText(value string) string {
	return boundedMarkdownText(value)
}

func boundedMarkdownText(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
	if len(value) <= reportMarkdownPreviewLimit {
		return value
	}
	return value[:reportMarkdownPreviewLimit] + "... (truncated)"
}
