package theatercli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/junit"
	reportmodel "github.com/alex-poliushkin/theater/report"
)

const (
	outputFormatJSON     outputFormat = "json"
	outputFormatJUnit    outputFormat = "junit"
	outputFormatMarkdown outputFormat = "markdown"
	outputFormatText     outputFormat = "text"
)

type outputFormat string

type outputFormatSet map[outputFormat]struct{}

type runDocumentExporter interface {
	Write(io.Writer, reportmodel.RunDocument) error
}

type runDocumentRenderer struct {
	stdout        io.Writer
	stderr        io.Writer
	junitExporter runDocumentExporter
	textRenderer  runTextRenderer
}

type validationRenderer struct {
	stdout io.Writer
	stderr io.Writer
}

type validationTextView struct {
	file        string
	diagnostics []theater.Diagnostic
}

func newRunDocumentRenderer(stdout, stderr io.Writer) runDocumentRenderer {
	return runDocumentRenderer{
		stdout:        stdout,
		stderr:        stderr,
		junitExporter: junit.NewExporter(),
		textRenderer:  newRunTextRenderer(),
	}
}

func newValidationRenderer(stdout, stderr io.Writer) validationRenderer {
	return validationRenderer{stdout: stdout, stderr: stderr}
}

func parseOutputFormat(raw string) (outputFormat, error) {
	return outputFormatSet{
		outputFormatText:  {},
		outputFormatJSON:  {},
		outputFormatJUnit: {},
	}.Parse(raw)
}

func parseValidationOutputFormat(raw string) (outputFormat, error) {
	return outputFormatSet{
		outputFormatText: {},
		outputFormatJSON: {},
	}.Parse(raw)
}

func parseReportOutputFormat(raw string) (outputFormat, error) {
	return outputFormatSet{
		outputFormatJUnit:    {},
		outputFormatMarkdown: {},
	}.Parse(raw)
}

func (s outputFormatSet) Parse(raw string) (outputFormat, error) {
	format := outputFormat(raw)
	if _, ok := s[format]; ok {
		return format, nil
	}

	return "", fmt.Errorf("unsupported format %q (supported: %s)", raw, s.supportedValues())
}

func (s outputFormatSet) supportedValues() string {
	candidates := []outputFormat{outputFormatText, outputFormatJSON, outputFormatJUnit, outputFormatMarkdown}
	values := make([]string, 0, len(s))
	for _, candidate := range candidates {
		if _, ok := s[candidate]; ok {
			values = append(values, string(candidate))
		}
	}
	return strings.Join(values, ", ")
}

func (r runDocumentRenderer) Render(format outputFormat, file string, document reportmodel.RunDocument) int {
	switch format {
	case outputFormatJSON:
		return r.renderJSON(file, document)
	case outputFormatJUnit:
		return r.renderJUnit(document)
	case outputFormatText:
		return r.renderText(file, document)
	default:
		fmt.Fprintf(r.stderr, "unsupported format %q\n", format)
		return exitCodeCommandError
	}
}

func (r runDocumentRenderer) renderJSON(file string, document reportmodel.RunDocument) int {
	response := struct {
		File   string                  `json:"file"`
		Result reportmodel.RunDocument `json:"result"`
	}{
		File:   file,
		Result: document,
	}

	if err := writeJSON(r.stdout, response); err != nil {
		fmt.Fprintf(r.stderr, "encode json: %v\n", err)
		return exitCodeCommandError
	}

	return runExitCode(document)
}

func (r runDocumentRenderer) renderJUnit(document reportmodel.RunDocument) int {
	if err := r.junitExporter.Write(r.stdout, document); err != nil {
		fmt.Fprintf(r.stderr, "encode junit: %v\n", err)
		return exitCodeCommandError
	}

	return runExitCode(document)
}

func (r runDocumentRenderer) renderText(file string, document reportmodel.RunDocument) int {
	if err := r.textRenderer.Write(r.stdout, file, document); err != nil {
		fmt.Fprintf(r.stderr, "write text: %v\n", err)
		return exitCodeCommandError
	}

	return runExitCode(document)
}

func (r validationRenderer) Render(format outputFormat, file string, diagnostics []theater.Diagnostic) int {
	switch format {
	case outputFormatJSON:
		return r.renderJSON(file, diagnostics)
	case outputFormatText:
		return r.renderText(file, diagnostics)
	default:
		fmt.Fprintf(r.stderr, "unsupported format %q\n", format)
		return exitCodeCommandError
	}
}

func (r validationRenderer) renderJSON(file string, diagnostics []theater.Diagnostic) int {
	response := struct {
		File        string               `json:"file"`
		Valid       bool                 `json:"valid"`
		Diagnostics []theater.Diagnostic `json:"diagnostics"`
	}{
		File:        file,
		Valid:       !hasErrorDiagnostics(diagnostics),
		Diagnostics: diagnostics,
	}

	if err := writeJSON(r.stdout, response); err != nil {
		fmt.Fprintf(r.stderr, "encode json: %v\n", err)
		return exitCodeCommandError
	}

	return validationExitCode(diagnostics)
}

func (r validationRenderer) renderText(file string, diagnostics []theater.Diagnostic) int {
	if _, err := io.WriteString(r.stdout, newValidationTextView(file, diagnostics).String()); err != nil {
		fmt.Fprintf(r.stderr, "write text: %v\n", err)
		return exitCodeCommandError
	}

	return validationExitCode(diagnostics)
}

func newValidationTextView(file string, diagnostics []theater.Diagnostic) validationTextView {
	return validationTextView{file: file, diagnostics: diagnostics}
}

func (v validationTextView) String() string {
	var builder strings.Builder
	if len(v.diagnostics) == 0 {
		fmt.Fprintf(&builder, "%s: valid\n", v.file)
		return builder.String()
	}

	if hasErrorDiagnostics(v.diagnostics) {
		fmt.Fprintf(&builder, "%s: %d diagnostic(s)\n", v.file, len(v.diagnostics))
	} else {
		fmt.Fprintf(&builder, "%s: valid with %d hint(s)\n", v.file, len(v.diagnostics))
	}
	for _, diagnostic := range v.diagnostics {
		fmt.Fprintf(
			&builder,
			"- [%s] %s: %s\n",
			diagnostic.Code,
			emptyFallback(diagnostic.Path, "<unknown>"),
			diagnostic.Summary,
		)
		if source := formatSourceSpan(&diagnostic.Span); source != "" {
			fmt.Fprintf(&builder, "  source: %s\n", source)
		}
		if breadcrumb := formatDiagnosticBreadcrumb(diagnostic.Path); breadcrumb != "" {
			fmt.Fprintf(&builder, "  breadcrumb: %s\n", breadcrumb)
		}
	}

	return builder.String()
}

func runExitCode(document reportmodel.RunDocument) int {
	if hasErrorDiagnostics(document.Diagnostics) ||
		document.Report.Status == reportmodel.StatusFailed ||
		document.Report.Status == reportmodel.StatusCanceled {
		return 1
	}

	return 0
}

func validationExitCode(diagnostics []theater.Diagnostic) int {
	if !hasErrorDiagnostics(diagnostics) {
		return 0
	}

	return 1
}

func hasErrorDiagnostics(diagnostics []theater.Diagnostic) bool {
	for i := range diagnostics {
		if diagnostics[i].Severity != theater.SeverityHint {
			return true
		}
	}

	return false
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func formatDiagnosticBreadcrumb(path string) string {
	if path == "" || !strings.HasPrefix(path, "stage.") {
		return ""
	}

	segments := strings.Split(path, "/")
	parts := make([]string, 0, len(segments))
	for i := range segments {
		segment := segments[i]
		if segment == "" || strings.HasPrefix(segment, "stage.") {
			continue
		}

		switch {
		case strings.HasPrefix(segment, "scenario."):
			parts = append(parts, "scenario "+decodeDiagnosticPathID(strings.TrimPrefix(segment, "scenario.")))
		case strings.HasPrefix(segment, "call."):
			parts = append(parts, "call "+decodeDiagnosticPathID(strings.TrimPrefix(segment, "call.")))
		case strings.HasPrefix(segment, "act."):
			parts = append(parts, "act "+decodeDiagnosticPathID(strings.TrimPrefix(segment, "act.")))
		case strings.HasPrefix(segment, "property."):
			parts = append(parts, "prop "+decodeDiagnosticPathID(strings.TrimPrefix(segment, "property.")))
		case strings.HasPrefix(segment, "expectation."):
			parts = append(parts, "expect "+decodeDiagnosticPathID(strings.TrimPrefix(segment, "expectation.")))
		case strings.HasPrefix(segment, "export."):
			parts = append(parts, "export "+decodeDiagnosticPathID(strings.TrimPrefix(segment, "export.")))
		case strings.HasPrefix(segment, "binding."):
			parts = append(parts, "binding "+decodeDiagnosticPathID(strings.TrimPrefix(segment, "binding.")))
		case strings.HasPrefix(segment, "decorator."):
			parts = append(parts, "decorator "+decodeDiagnosticPathID(strings.TrimPrefix(segment, "decorator.")))
		default:
			parts = append(parts, decodeDiagnosticPathID(segment))
		}
	}

	return strings.Join(parts, " -> ")
}

func decodeDiagnosticPathID(value string) string {
	replacer := strings.NewReplacer(
		"~2", ".",
		"~1", "/",
		"~0", "~",
	)
	return replacer.Replace(value)
}
