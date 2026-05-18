package junit

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/alex-poliushkin/theater/internal/reportview"
	reportmodel "github.com/alex-poliushkin/theater/report"
)

const (
	messageLimit = 200
	bodyLimit    = 8 * 1024

	validationClassname = "theater.validation"
	validationName      = "compile-validate"
	runtimeClassname    = "theater.runtime"
	runtimeName         = "run-abort"
	failureElement      = "failure"
	errorElement        = "error"

	diagnosticPreviewEmpty           = "<empty>"
	diagnosticPreviewRedacted        = "<redacted>"
	diagnosticPreviewUnavailable     = "<unavailable>"
	diagnosticPreviewRedactedSuffix  = " (redacted)"
	diagnosticPreviewTruncatedSuffix = " (truncated)"
)

// Exporter writes theater run documents as JUnit XML.
type Exporter struct{}

type testSuites struct {
	XMLName xml.Name    `xml:"testsuites"`
	Suites  []testSuite `xml:"testsuite"`
}

type testSuite struct {
	XMLName   xml.Name   `xml:"testsuite"`
	Name      string     `xml:"name,attr"`
	Tests     int        `xml:"tests,attr"`
	Failures  int        `xml:"failures,attr,omitempty"`
	Errors    int        `xml:"errors,attr,omitempty"`
	Skipped   int        `xml:"skipped,attr,omitempty"`
	Time      string     `xml:"time,attr,omitempty"`
	Timestamp string     `xml:"timestamp,attr,omitempty"`
	Cases     []testCase `xml:"testcase"`
}

type testCase struct {
	XMLName   xml.Name     `xml:"testcase"`
	Classname string       `xml:"classname,attr,omitempty"`
	Name      string       `xml:"name,attr"`
	File      string       `xml:"file,attr,omitempty"`
	Time      string       `xml:"time,attr,omitempty"`
	Failure   *testOutcome `xml:"failure,omitempty"`
	Error     *testOutcome `xml:"error,omitempty"`
	Skipped   *testOutcome `xml:"skipped,omitempty"`
}

type testOutcome struct {
	Message string `xml:"message,attr,omitempty"`
	Body    string `xml:",chardata"`
}

// NewExporter constructs a JUnit exporter.
func NewExporter() *Exporter {
	return &Exporter{}
}

// Write encodes doc as JUnit XML into w.
func (e *Exporter) Write(w io.Writer, doc reportmodel.RunDocument) error {
	if w == nil {
		return errors.New("writer is required")
	}

	if err := doc.Validate(); err != nil {
		return fmt.Errorf("run document is invalid: %w", err)
	}

	suites := buildTestSuites(doc)

	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}

	encoder := xml.NewEncoder(w)
	encoder.Indent("", "  ")
	if err := encoder.Encode(suites); err != nil {
		return err
	}

	return encoder.Close()
}

// Marshal returns the JUnit XML encoding of doc.
func (e *Exporter) Marshal(doc reportmodel.RunDocument) ([]byte, error) {
	var buffer bytes.Buffer
	if err := e.Write(&buffer, doc); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func buildTestSuites(doc reportmodel.RunDocument) testSuites {
	projection := reportview.New(doc)
	suite := buildTestSuite(projection)
	return testSuites{Suites: []testSuite{suite}}
}

func buildTestSuite(projection *reportview.Projection) testSuite {
	cases := buildScenarioCases(projection)

	if synthetic, ok := buildSyntheticCase(projection, len(cases) > 0); ok {
		cases = append(cases, synthetic)
	}

	suite := testSuite{
		Name:  suiteName(projection.Report),
		Tests: len(cases),
		Cases: cases,
	}
	if hasTiming(projection.Report.StartedAt, projection.Report.EndedAt, projection.Report.DurationMs) {
		suite.Time = formatDuration(projection.Report.DurationMs)
		suite.Timestamp = projection.Report.StartedAt.UTC().Format(time.RFC3339)
	}

	for _, testcase := range cases {
		switch {
		case testcase.Failure != nil:
			suite.Failures++
		case testcase.Error != nil:
			suite.Errors++
		case testcase.Skipped != nil:
			suite.Skipped++
		}
	}

	return suite
}

func buildScenarioCases(projection *reportview.Projection) []testCase {
	cases := make([]testCase, 0)
	for i := range projection.Scenarios {
		scenario := projection.Scenarios[i]
		if !shouldExportScenario(scenario.Node) {
			continue
		}

		cases = append(cases, buildScenarioCase(projection, scenario))
	}

	return cases
}

func shouldExportScenario(node reportmodel.NodeReport) bool {
	return node.Status != reportmodel.StatusSkipped || node.SkipReason != reportmodel.SkipReasonStageAborted
}

func buildScenarioCase(projection *reportview.Projection, scenario reportview.ScenarioView) testCase {
	testcase := testCase{
		Classname: scenarioClassname(scenario.Node, projection.Report),
		Name:      scenarioName(scenario.Node),
		File:      scenarioFile(scenario),
	}
	if hasTiming(scenario.Node.StartedAt, scenario.Node.EndedAt, scenario.Node.DurationMs) {
		testcase.Time = formatDuration(scenario.Node.DurationMs)
	}

	switch scenario.Node.Status {
	case reportmodel.StatusPassed:
		return testcase
	case reportmodel.StatusSkipped:
		message, body := renderSkipped(scenario.Node)
		testcase.Skipped = &testOutcome{Message: message, Body: body}
		return testcase
	case reportmodel.StatusCanceled:
		message, body := renderCanceled(projection, scenario)
		testcase.Error = &testOutcome{Message: message, Body: body}
		return testcase
	case reportmodel.StatusFailed:
		message, body, element := renderFailed(projection, scenario)
		switch element {
		case failureElement:
			testcase.Failure = &testOutcome{Message: message, Body: body}
		default:
			testcase.Error = &testOutcome{Message: message, Body: body}
		}
		return testcase
	default:
		return testcase
	}
}

func buildSyntheticCase(projection *reportview.Projection, haveScenarioCases bool) (testCase, bool) {
	if len(projection.Document.Diagnostics) > 0 {
		message := boundedSingleLine("[validation] "+projection.Report.Failure.Summary, messageLimit)
		body := buildValidationBody(projection.Document)
		return testCase{
			Classname: validationClassname,
			Name:      validationName,
			Error: &testOutcome{
				Message: message,
				Body:    body,
			},
		}, true
	}

	if projection.Report.Failure == nil || haveScenarioCases {
		return testCase{}, false
	}

	message := boundedSingleLine("[runtime] "+projection.Report.Failure.Summary, messageLimit)
	body := buildRuntimeBody(projection.Document)
	return testCase{
		Classname: runtimeClassname,
		Name:      runtimeName,
		Error: &testOutcome{
			Message: message,
			Body:    body,
		},
	}, true
}

func suiteName(report reportmodel.Report) string {
	if report.StageID != "" {
		return report.StageID
	}

	return report.StagePath
}

func scenarioClassname(node reportmodel.NodeReport, report reportmodel.Report) string {
	if node.ScenarioID != "" {
		return node.ScenarioID
	}

	if report.StageID != "" {
		return report.StageID
	}

	return report.StagePath
}

func scenarioName(node reportmodel.NodeReport) string {
	if node.ScenarioCallID != "" {
		return node.ScenarioCallID
	}

	return node.Path
}

func scenarioFile(scenario reportview.ScenarioView) string {
	if scenario.SourceSpan == nil {
		return ""
	}
	if scenario.SourceSpan.File == "" {
		return ""
	}

	return scenario.SourceSpan.File
}

func renderSkipped(node reportmodel.NodeReport) (message, body string) {
	message = boundedSingleLine("[skipped] scenario skipped", messageLimit)
	body = buildBody(message, []string{
		"scenario_call: " + scenarioName(node),
		"summary: scenario skipped",
	})
	return message, body
}

func renderCanceled(projection *reportview.Projection, scenario reportview.ScenarioView) (message, body string) {
	message = boundedSingleLine("[canceled] scenario canceled", messageLimit)
	lines := []string{
		"stage: " + suiteName(projection.Report),
		"scenario: " + scenarioClassname(scenario.Node, projection.Report),
		"scenario_call: " + scenarioName(scenario.Node),
		"summary: scenario canceled",
	}
	if file := scenarioFile(scenario); file != "" {
		lines = append(lines, "source: "+file)
	}

	return message, buildBody(message, lines)
}

func renderFailed(projection *reportview.Projection, scenario reportview.ScenarioView) (message, body, element string) {
	failureNode := scenario.PrimaryFailure
	if failureNode == nil {
		failureNode = &scenario.Node
	}

	headlineNode := scenario.TerminalFailure
	if headlineNode == nil {
		headlineNode = failureNode
	}

	failure := scenario.Node.Failure
	if failure == nil && failureNode != nil {
		failure = failureNode.Failure
	}

	category := failureCategory(failure)
	message = boundedSingleLine(renderFailureHeadline(category, headlineNode, failure), messageLimit)
	lines := []string{
		"stage: " + suiteName(projection.Report),
		"scenario: " + scenarioClassname(scenario.Node, projection.Report),
		"scenario_call: " + scenarioName(scenario.Node),
	}
	if headlineNode != nil && headlineNode.Address != nil && headlineNode.Address.ActID != "" {
		lines = append(lines, "act: "+headlineNode.Address.ActID)
	}
	if file := scenarioFile(scenario); file != "" {
		lines = append(lines, "source: "+file)
	}
	if failure != nil {
		lines = append(lines,
			"failure_kind: "+string(failure.Kind),
			"failure_phase: "+string(failure.Phase),
			"failure_at: "+failure.At,
			"summary: "+failure.Summary,
		)
		if cause := boundedSingleLine(failure.Message(), bodyLimit); cause != failure.Summary {
			lines = append(lines, "cause: "+cause)
		}
	}
	if scenario.Eventually != nil {
		lines = append(lines, formatEventuallyLine(*scenario.Eventually))
	}
	if contrastLines := formatContrastLines(failureNode.Contrast); len(contrastLines) != 0 {
		lines = append(lines, contrastLines...)
	}
	if observationLines := formatObservationLines(failureNode.Observations); len(observationLines) != 0 {
		lines = append(lines, observationLines...)
	}
	if diagnosticLines := formatNodeDiagnosticLines(failureNode.Diagnostics); len(diagnosticLines) != 0 {
		lines = append(lines, diagnosticLines...)
	}

	element = errorElement
	if failure != nil && failure.Kind == reportmodel.FailureKindExpectation {
		element = failureElement
	}

	return message, buildBody(message, lines), element
}

func failureCategory(failure *reportmodel.Failure) string {
	if failure == nil {
		return "error"
	}

	switch failure.Kind {
	case reportmodel.FailureKindExpectation:
		return "expectation"
	case reportmodel.FailureKindAction:
		return "action"
	case reportmodel.FailureKindDefinition:
		return "validation"
	case reportmodel.FailureKindSetup:
		return "setup"
	case reportmodel.FailureKindTimeout:
		return "timeout"
	case reportmodel.FailureKindObservation:
		return "observation"
	case reportmodel.FailureKindInternal:
		return "internal"
	default:
		return errorElement
	}
}

func renderFailureHeadline(category string, node *reportmodel.NodeReport, failure *reportmodel.Failure) string {
	if failure == nil {
		return "[" + category + "] scenario failed"
	}

	if node != nil && node.Address != nil && node.Address.ActID != "" {
		return fmt.Sprintf("[%s] %s: %s", category, node.Address.ActID, failure.Summary)
	}

	return fmt.Sprintf("[%s] %s", category, failure.Summary)
}

func buildValidationBody(doc reportmodel.RunDocument) string {
	lines := []string{
		"stage: " + suiteName(doc.Report),
		"summary: " + doc.Report.Failure.Summary,
	}
	for _, diagnostic := range doc.Diagnostics {
		lines = append(lines, fmt.Sprintf("diagnostic: [%s] %s: %s", diagnostic.Code, diagnostic.Path, diagnostic.Summary))
	}

	message := boundedSingleLine("[validation] "+doc.Report.Failure.Summary, messageLimit)
	return buildBody(message, lines)
}

func buildRuntimeBody(doc reportmodel.RunDocument) string {
	lines := []string{
		"stage: " + suiteName(doc.Report),
	}
	if doc.Report.Failure != nil {
		lines = append(lines,
			"failure_kind: "+string(doc.Report.Failure.Kind),
			"failure_phase: "+string(doc.Report.Failure.Phase),
			"failure_at: "+doc.Report.Failure.At,
			"summary: "+doc.Report.Failure.Summary,
		)
	}

	message := boundedSingleLine("[runtime] "+doc.Report.Failure.Summary, messageLimit)
	return buildBody(message, lines)
}

func formatEventuallyLine(eventually reportmodel.EventuallyReport) string {
	line := fmt.Sprintf(
		"eventually: attempts=%d termination=%s final=%s",
		eventually.AttemptsTotal,
		eventually.TerminationReason,
		eventually.FinalOutcome,
	)
	if eventually.LastObservedFailure != nil {
		line += " last=" + eventually.LastObservedFailure.Summary
	}

	return boundedSingleLine(line, bodyLimit)
}

func formatContrastLines(contrast *reportmodel.Contrast) []string {
	if contrast == nil {
		return nil
	}

	lines := make([]string, 0, 3)
	if contrast.Summary != "" {
		lines = append(lines, "contrast: "+boundedSingleLine(contrast.Summary, bodyLimit))
	}
	if contrast.Expected != nil && contrast.Expected.Text != "" {
		lines = append(lines, "expected: "+boundedSingleLine(contrast.Expected.Text, bodyLimit))
	}
	if contrast.Actual != nil && contrast.Actual.Text != "" {
		lines = append(lines, "actual: "+boundedSingleLine(contrast.Actual.Text, bodyLimit))
	}

	return lines
}

func formatObservationLines(observations *reportmodel.ActionObservations) []string {
	if observations == nil {
		return nil
	}

	lines := make([]string, 0, 2)
	if line, ok := firstObservedLine("input", observations.Inputs); ok {
		lines = append(lines, line)
	}
	if line, ok := firstObservedLine("output", observations.Outputs); ok {
		lines = append(lines, line)
	}

	return lines
}

func formatNodeDiagnosticLines(diagnostics []reportmodel.NodeDiagnostic) []string {
	lines := make([]string, 0)
	for i := range diagnostics {
		switch diagnostics[i].Kind {
		case reportmodel.NodeDiagnosticKindHTTP:
			if diagnostics[i].HTTP != nil {
				lines = append(lines, formatHTTPDiagnosticLines(*diagnostics[i].HTTP)...)
			}
		case reportmodel.NodeDiagnosticKindPreflight:
			if diagnostics[i].Preflight != nil {
				lines = append(lines, formatPreflightDiagnosticLines(*diagnostics[i].Preflight)...)
			}
		}
	}

	return lines
}

func formatPreflightDiagnosticLines(diagnostic reportmodel.PreflightDiagnostic) []string {
	lines := []string{
		"preflight.guard_id: " + boundedSingleLine(diagnostic.GuardID, bodyLimit),
		"preflight.input_ref: " + boundedSingleLine(diagnostic.InputRef, bodyLimit),
		"preflight.reason_code: " + boundedSingleLine(diagnostic.ReasonCode, bodyLimit),
	}
	if diagnostic.InputPath != "" {
		lines = append(lines, "preflight.input_path: "+boundedSingleLine(diagnostic.InputPath, bodyLimit))
	}
	if diagnostic.AssertRef != "" {
		lines = append(lines, "preflight.assert_ref: "+boundedSingleLine(diagnostic.AssertRef, bodyLimit))
	}
	if diagnostic.OverridePresent {
		lines = append(
			lines,
			"preflight.override_ref: "+boundedSingleLine(diagnostic.OverrideRef, bodyLimit),
			fmt.Sprintf("preflight.override_used: %t", diagnostic.OverrideUsed),
		)
	}
	if source := formatSourceRef(diagnostic.SourceSpan); source != "" {
		lines = append(lines, "preflight.source: "+source)
	}
	if source := formatSourceRef(diagnostic.BindingSourceSpan); source != "" {
		lines = append(lines, "preflight.binding_source: "+source)
	}

	return lines
}

func formatHTTPDiagnosticLines(diagnostic reportmodel.HTTPDiagnostic) []string {
	lines := make([]string, 0, 5)
	request := strings.TrimSpace(diagnostic.Method + " " + diagnostic.URL)
	if request != "" {
		lines = append(lines, "http.request: "+boundedSingleLine(request, bodyLimit))
	}
	if diagnostic.StatusCode != 0 || diagnostic.Status != "" {
		response := strings.TrimSpace(fmt.Sprintf("%d %s", diagnostic.StatusCode, diagnostic.Status))
		lines = append(lines, "http.response: "+boundedSingleLine(response, bodyLimit))
	}
	if diagnostic.DurationMs >= 0 {
		lines = append(lines, fmt.Sprintf("http.duration_ms: %d", diagnostic.DurationMs))
	}
	for _, key := range orderedHTTPHeaderKeys(diagnostic.ResponseHeaders) {
		value := strings.Join(diagnostic.ResponseHeaders[key], ", ")
		lines = append(lines, fmt.Sprintf("http.header.%s: %s", key, boundedSingleLine(value, bodyLimit)))
	}
	if diagnostic.ResponsePreview != nil {
		lines = append(lines, "http.body: "+formatHTTPDiagnosticPreview(diagnostic.ResponsePreview))
	}

	return lines
}

func formatHTTPDiagnosticPreview(preview *reportmodel.Preview) string {
	if preview == nil {
		return diagnosticPreviewUnavailable
	}

	var rendered string
	switch {
	case preview.Text != "":
		rendered = boundedSingleLine(preview.Text, bodyLimit)
	case preview.OmittedReason != "":
		rendered = "<" + preview.OmittedReason + ">"
	case preview.Redacted:
		rendered = diagnosticPreviewRedacted
	default:
		rendered = diagnosticPreviewEmpty
	}
	if preview.Redacted && preview.Text != "" {
		rendered += diagnosticPreviewRedactedSuffix
	}
	if preview.Truncated {
		rendered += diagnosticPreviewTruncatedSuffix
	}

	return rendered
}

func formatSourceRef(source *reportmodel.SourceRef) string {
	if source == nil || source.File == "" {
		return ""
	}

	if source.Line > 0 && source.Column > 0 {
		return fmt.Sprintf("%s:%d:%d", source.File, source.Line, source.Column)
	}
	if source.Line > 0 {
		return fmt.Sprintf("%s:%d", source.File, source.Line)
	}

	return source.File
}

func orderedHTTPHeaderKeys(headers map[string][]string) []string {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func firstObservedLine(prefix string, values map[string]reportmodel.ObservedValue) (string, bool) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		observed := values[key]
		if observed.Preview == nil || observed.Preview.Text == "" {
			continue
		}

		return fmt.Sprintf("%s.%s: %s", prefix, key, boundedSingleLine(observed.Preview.Text, bodyLimit)), true
	}

	return "", false
}

func buildBody(message string, lines []string) string {
	filtered := make([]string, 0, len(lines)+1)
	filtered = append(filtered, message)
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		filtered = append(filtered, sanitizeText(line))
	}

	return boundedText(strings.Join(filtered, "\n"), bodyLimit)
}

func boundedSingleLine(value string, limit int) string {
	return boundedText(strings.ReplaceAll(sanitizeText(value), "\n", " "), limit)
}

func boundedText(value string, limit int) string {
	sanitized := sanitizeText(value)
	if len(sanitized) <= limit {
		return sanitized
	}
	if limit <= 3 {
		return sanitized[:limit]
	}

	return sanitized[:limit-3] + "..."
}

func sanitizeText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return value
}

func formatDuration(durationMs int64) string {
	return fmt.Sprintf("%.3f", float64(durationMs)/1000)
}

func hasTiming(startedAt, endedAt time.Time, durationMs int64) bool {
	return !startedAt.IsZero() || !endedAt.IsZero() || durationMs != 0
}
