package theatercli

import (
	"fmt"
	"io"
	"strings"

	"github.com/alex-poliushkin/theater/internal/reportview"
	reportmodel "github.com/alex-poliushkin/theater/report"
)

const (
	reportSummaryDiagnosticLimit = 20
	reportSummaryScenarioLimit   = 20
	reportSummaryNodeLimit       = 50
	reportSummaryTextLimit       = 240
)

type reportSummaryMarkdownRenderer struct{}

type reportSummaryMarkdownView struct {
	file       string
	document   reportmodel.RunDocument
	projection *reportview.Projection
}

func newReportSummaryMarkdownRenderer() reportSummaryMarkdownRenderer {
	return reportSummaryMarkdownRenderer{}
}

func (r reportSummaryMarkdownRenderer) Write(writer io.Writer, file string, document reportmodel.RunDocument) error {
	_, err := io.WriteString(writer, newReportSummaryMarkdownView(file, document).String())
	return err
}

func newReportSummaryMarkdownView(file string, document reportmodel.RunDocument) reportSummaryMarkdownView {
	return reportSummaryMarkdownView{
		file:       file,
		document:   document,
		projection: reportview.New(document),
	}
}

func (v reportSummaryMarkdownView) String() string {
	var builder strings.Builder
	builder.WriteString("# Theater Run Summary\n\n")
	renderSummaryOverview(&builder, v.file, v.document)
	renderSummaryDiagnostics(&builder, v.document.Diagnostics)
	v.renderFailures(&builder)
	return builder.String()
}

func renderSummaryOverview(builder *strings.Builder, file string, document reportmodel.RunDocument) {
	report := document.Report
	if file != "" {
		fmt.Fprintf(builder, "- File: %s\n", markdownCode(file))
	}
	fmt.Fprintf(builder, "- Stage: %s\n", markdownCode(emptyFallback(report.StageID, report.StagePath)))
	fmt.Fprintf(builder, "- Status: %s\n", markdownCode(string(report.Status)))
	fmt.Fprintf(
		builder,
		"- Scenarios: total=%d passed=%d failed=%d canceled=%d skipped=%d\n",
		report.Summary.TotalScenarios,
		report.Summary.PassedScenarios,
		report.Summary.FailedScenarios,
		report.Summary.CanceledScenarios,
		report.Summary.SkippedScenarios,
	)
	fmt.Fprintf(builder, "- Run: %s\n", markdownCode(document.RunID))
	fmt.Fprintf(builder, "- Theater: %s\n", markdownCode(document.TheaterVersion))
	fmt.Fprintf(builder, "- Schema: %s\n", markdownCode(document.ReportSchemaVersion))
}

func renderSummaryDiagnostics(builder *strings.Builder, diagnostics []reportmodel.Diagnostic) {
	if len(diagnostics) == 0 {
		return
	}

	builder.WriteString("\n## Diagnostics\n\n")
	limit := min(len(diagnostics), reportSummaryDiagnosticLimit)
	for i := 0; i < limit; i++ {
		diagnostic := diagnostics[i]
		fmt.Fprintf(
			builder,
			"- %s %s: %s\n",
			markdownCode(diagnostic.Code),
			markdownCode(emptyFallback(diagnostic.Path, "<unknown>")),
			summaryMarkdownCode(diagnostic.Summary),
		)
		if source := formatSourceSpan(&diagnostic.Span); source != "" {
			fmt.Fprintf(builder, "  - Source: %s\n", markdownCode(source))
		}
	}
	if omitted := len(diagnostics) - limit; omitted > 0 {
		fmt.Fprintf(builder, "\n_Omitted %d diagnostic(s) after the first %d._\n", omitted, reportSummaryDiagnosticLimit)
	}
}

func (v reportSummaryMarkdownView) renderFailures(builder *strings.Builder) {
	if v.document.Report.Failure != nil && !v.projection.HasFailedScenario() {
		builder.WriteString("\n## Run Failure\n\n")
		renderSummaryFailure(builder, "- ", nil, v.document.Report.Failure)
	}
	v.renderFailedScenarios(builder)
}

func (v reportSummaryMarkdownView) renderFailedScenarios(builder *strings.Builder) {
	failed := 0
	for i := range v.projection.Scenarios {
		scenario := v.projection.Scenarios[i]
		if scenario.Node.Status != reportmodel.StatusFailed {
			continue
		}
		if failed == 0 {
			builder.WriteString("\n## Failed Scenarios\n\n")
		}
		if failed >= reportSummaryScenarioLimit {
			break
		}
		renderSummaryScenarioFailure(builder, scenario, v.document.Report.Nodes)
		failed++
	}
	if omitted := v.document.Report.Summary.FailedScenarios - failed; omitted > 0 {
		fmt.Fprintf(builder, "\n_Omitted %d failed scenario(s) after the first %d._\n", omitted, reportSummaryScenarioLimit)
	}
}

func renderSummaryScenarioFailure(
	builder *strings.Builder,
	scenario reportview.ScenarioView,
	nodes []reportmodel.NodeReport,
) {
	node := scenario.Node
	fmt.Fprintf(builder, "- Scenario %s failed\n", markdownCode(emptyFallback(node.ScenarioCallID, node.Path)))
	if node.ScenarioID != "" {
		fmt.Fprintf(builder, "  - Scenario: %s\n", markdownCode(node.ScenarioID))
	}
	if source := formatSourceSpan(scenario.SourceSpan); source != "" {
		fmt.Fprintf(builder, "  - Source: %s\n", markdownCode(source))
	}
	if scenario.TerminalFailure != nil && scenario.TerminalFailure.Failure != nil {
		renderSummaryFailure(builder, "  - ", scenario.TerminalFailure, scenario.TerminalFailure.Failure)
	} else if node.Failure != nil {
		renderSummaryFailure(builder, "  - ", &node, node.Failure)
	}

	rendered := 0
	for i := range nodes {
		candidate := nodes[i]
		if candidate.ScenarioPath != node.ScenarioPath || candidate.Status != reportmodel.StatusFailed || candidate.Failure == nil {
			continue
		}
		if rendered >= reportSummaryNodeLimit {
			break
		}
		renderSummaryFailedNode(builder, candidate)
		rendered++
	}
	if omitted := countFailedScenarioNodes(nodes, node.ScenarioPath) - rendered; omitted > 0 {
		fmt.Fprintf(builder, "  - Omitted %d failed node(s) after the first %d.\n", omitted, reportSummaryNodeLimit)
	}
}

func renderSummaryFailure(
	builder *strings.Builder,
	prefix string,
	node *reportmodel.NodeReport,
	failure *reportmodel.Failure,
) {
	if failure == nil {
		return
	}
	fmt.Fprintf(builder, "%sSummary: %s\n", prefix, renderSummaryFailureMessage(failure))
	fmt.Fprintf(builder, "%sKind: %s\n", prefix, markdownCode(string(failure.Kind)))
	if failure.At != "" {
		fmt.Fprintf(builder, "%sAt: %s\n", prefix, markdownCode(failure.At))
	}
	if node != nil {
		if source := formatSourceSpan(node.SourceSpan); source != "" {
			fmt.Fprintf(builder, "%sSource: %s\n", prefix, markdownCode(source))
		}
	}
}

func renderSummaryFailedNode(builder *strings.Builder, node reportmodel.NodeReport) {
	fmt.Fprintf(
		builder,
		"  - Failed node: %s (%s)",
		markdownCode(emptyFallback(node.ID, node.Path)),
		markdownCode(string(node.Kind)),
	)
	if node.Failure != nil {
		fmt.Fprintf(builder, ": %s", renderSummaryFailureMessage(node.Failure))
	}
	builder.WriteString("\n")
	if source := formatSourceSpan(node.SourceSpan); source != "" {
		fmt.Fprintf(builder, "    - Source: %s\n", markdownCode(source))
	}
}

func countFailedScenarioNodes(nodes []reportmodel.NodeReport, scenarioPath string) int {
	count := 0
	for i := range nodes {
		if nodes[i].ScenarioPath == scenarioPath && nodes[i].Status == reportmodel.StatusFailed && nodes[i].Failure != nil {
			count++
		}
	}
	return count
}

func renderSummaryFailureMessage(failure *reportmodel.Failure) string {
	return summaryMarkdownCode(emptyFallback(failure.Summary, string(failure.Kind)))
}

func summaryMarkdownCode(value string) string {
	if len(value) > reportSummaryTextLimit {
		value = value[:reportSummaryTextLimit] + "..." + renderPreviewTruncatedSuffix
	}
	return markdownCode(value)
}
