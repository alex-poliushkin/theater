package theatercli

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alex-poliushkin/theater/internal/reportview"
	reportmodel "github.com/alex-poliushkin/theater/report"
)

const preflightDiagnosticTextLimit = 240

type runTextRenderer struct{}

type runTextView struct {
	file       string
	document   reportmodel.RunDocument
	projection *reportview.Projection
}

func newRunTextRenderer() runTextRenderer {
	return runTextRenderer{}
}

func (r runTextRenderer) Write(writer io.Writer, file string, document reportmodel.RunDocument) error {
	_, err := io.WriteString(writer, newRunTextView(file, document).String())
	return err
}

func newRunTextView(file string, document reportmodel.RunDocument) runTextView {
	return runTextView{
		file:       file,
		document:   document,
		projection: reportview.New(document),
	}
}

func (v runTextView) String() string {
	var builder strings.Builder

	fmt.Fprintf(
		&builder,
		"%s: %s (passed=%d failed=%d canceled=%d skipped=%d",
		v.file,
		v.document.Report.Status,
		v.document.Report.Summary.PassedScenarios,
		v.document.Report.Summary.FailedScenarios,
		v.document.Report.Summary.CanceledScenarios,
		v.document.Report.Summary.SkippedScenarios,
	)
	if v.document.Report.DurationMs > 0 {
		fmt.Fprintf(&builder, " duration=%s", humanDuration(v.document.Report.DurationMs))
	}
	builder.WriteString(")\n")

	if v.projection.ConvergedActs > 0 {
		fmt.Fprintf(
			&builder,
			"eventually: converged_acts=%d extra_attempts=%d\n",
			v.projection.ConvergedActs,
			v.projection.ExtraAttempts,
		)
	}

	renderDiagnosticsBlock(&builder, v.document.Diagnostics)

	if v.document.Report.Failure != nil && !v.projection.HasFailedScenario() {
		renderStageFailureBlock(&builder, v.document.Report.Failure)
	}

	for i := range v.projection.Scenarios {
		if !shouldRenderScenarioCard(v.projection.Scenarios[i].Node) {
			continue
		}

		renderScenarioCard(&builder, v.projection.Scenarios[i])
	}

	return builder.String()
}

func shouldRenderScenarioCard(node reportmodel.NodeReport) bool {
	switch node.Status {
	case reportmodel.StatusFailed, reportmodel.StatusCanceled:
		return true
	case reportmodel.StatusSkipped:
		return node.SkipReason == reportmodel.SkipReasonExplicit
	default:
		return false
	}
}

func renderDiagnosticsBlock(builder *strings.Builder, diagnostics []reportmodel.Diagnostic) {
	if len(diagnostics) == 0 {
		return
	}

	builder.WriteString("\ndiagnostics:\n")
	for _, diagnostic := range diagnostics {
		fmt.Fprintf(
			builder,
			"- [%s] %s: %s",
			diagnostic.Code,
			emptyFallback(diagnostic.Path, "<unknown>"),
			diagnostic.Summary,
		)
		if source := formatSourceSpan(&diagnostic.Span); source != "" {
			fmt.Fprintf(builder, "\n  source: %s", source)
		}
		if breadcrumb := formatDiagnosticBreadcrumb(diagnostic.Path); breadcrumb != "" {
			fmt.Fprintf(builder, "\n  breadcrumb: %s", breadcrumb)
		}
		builder.WriteByte('\n')
	}
}

func renderStageFailureBlock(builder *strings.Builder, failure *reportmodel.Failure) {
	if failure == nil {
		return
	}

	builder.WriteString("\nrun failure:\n")
	fmt.Fprintf(builder, "  summary: %s\n", failure.Message())
	fmt.Fprintf(builder, "  kind: %s\n", failure.Kind)
	fmt.Fprintf(builder, "  at: %s\n", failure.At)
}

func renderScenarioCard(builder *strings.Builder, card reportview.ScenarioView) {
	builder.WriteByte('\n')
	fmt.Fprintf(builder, "scenario %s [%s]\n", emptyFallback(card.Node.ScenarioCallID, card.Node.Path), card.Node.Status)
	fmt.Fprintf(builder, "  scenario: %s\n", emptyFallback(card.Node.ScenarioID, "<unknown>"))
	if source := formatSourceSpan(card.SourceSpan); source != "" {
		fmt.Fprintf(builder, "  source: %s\n", source)
	}
	if card.Node.DurationMs > 0 {
		fmt.Fprintf(builder, "  duration: %s\n", humanDuration(card.Node.DurationMs))
	}

	switch card.Node.Status {
	case reportmodel.StatusFailed:
		renderFailedScenarioCard(builder, card)
	case reportmodel.StatusCanceled:
		builder.WriteString("  summary: scenario canceled\n")
	case reportmodel.StatusSkipped:
		fmt.Fprintf(builder, "  summary: scenario skipped (%s)\n", emptyFallback(string(card.Node.SkipReason), "unspecified"))
	}
}

func renderFailedScenarioCard(builder *strings.Builder, card reportview.ScenarioView) {
	if card.PrimaryFailure == nil || card.PrimaryFailure.Failure == nil {
		if card.Node.Failure != nil {
			fmt.Fprintf(builder, "  summary: %s\n", card.Node.Failure.Message())
			fmt.Fprintf(builder, "  kind: %s\n", card.Node.Failure.Kind)
			fmt.Fprintf(builder, "  at: %s\n", card.Node.Failure.At)
			renderNodeDiagnostics(builder, card.Node.Diagnostics)
		}
		return
	}

	failure := card.PrimaryFailure.Failure
	fmt.Fprintf(builder, "  summary: %s\n", failure.Message())
	fmt.Fprintf(builder, "  kind: %s\n", failure.Kind)
	fmt.Fprintf(builder, "  at: %s\n", failure.At)

	if actID := primaryActID(*card.PrimaryFailure); actID != "" {
		fmt.Fprintf(builder, "  act: %s\n", actID)
	}

	if card.Eventually != nil {
		builder.WriteString("  eventually: ")
		fmt.Fprintf(
			builder,
			"attempts=%d termination=%s",
			card.Eventually.AttemptsTotal,
			card.Eventually.TerminationReason,
		)
		if card.Eventually.LastObservedFailure != nil {
			fmt.Fprintf(builder, " last=%q", card.Eventually.LastObservedFailure.Message())
		}
		builder.WriteByte('\n')
	}

	if card.PrimaryFailure.Observations != nil {
		renderObservedBlock(builder, "inputs", card.PrimaryFailure.Observations.Inputs)
		renderObservedBlock(
			builder,
			"outputs",
			filteredObservedValues(card.PrimaryFailure.Observations.Outputs, card.PrimaryFailure.Observations.Streams),
		)
		renderObservedStreamBlock(builder, "streams", card.PrimaryFailure.Observations.Streams)
	}
	renderNodeDiagnostics(builder, card.PrimaryFailure.Diagnostics)
}

func renderObservedBlock(builder *strings.Builder, title string, observed map[string]reportmodel.ObservedValue) {
	if len(observed) == 0 {
		return
	}

	builder.WriteString("  ")
	builder.WriteString(title)
	builder.WriteString(":\n")
	for _, key := range orderedObservedKeys(observed) {
		builder.WriteString("    ")
		builder.WriteString(key)
		builder.WriteString(": ")
		builder.WriteString(renderObservedValue(observed[key]))
		builder.WriteByte('\n')
	}
}

func orderedObservedKeys(observed map[string]reportmodel.ObservedValue) []string {
	keys := make([]string, 0, len(observed))
	for key := range observed {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func orderedObservedStreamKeys(observed map[string]reportmodel.ObservedStream) []string {
	keys := make([]string, 0, len(observed))
	for key := range observed {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func renderObservedValue(observed reportmodel.ObservedValue) string {
	if observed.Preview == nil {
		return "<unavailable>"
	}

	preview := observed.Preview
	switch {
	case preview.Redacted:
		return "<redacted>"
	case preview.OmittedReason != "":
		return "<" + preview.OmittedReason + ">"
	case preview.Text != "":
		return preview.Text
	default:
		return "<empty>"
	}
}

func renderObservedStreamBlock(builder *strings.Builder, title string, observed map[string]reportmodel.ObservedStream) {
	if len(observed) == 0 {
		return
	}

	builder.WriteString("  ")
	builder.WriteString(title)
	builder.WriteString(":\n")
	for _, key := range orderedObservedStreamKeys(observed) {
		builder.WriteString("    ")
		builder.WriteString(key)
		builder.WriteString(": ")
		builder.WriteString(renderObservedStreamValue(observed[key]))
		builder.WriteByte('\n')
	}
}

func renderObservedStreamValue(observed reportmodel.ObservedStream) string {
	value := renderObservedValue(reportmodel.ObservedValue{
		Preview: observed.Preview,
		Payload: observed.Payload,
	})
	if observed.DroppedChunks == 0 {
		return value
	}

	return fmt.Sprintf("%s (live drops=%d)", value, observed.DroppedChunks)
}

func renderNodeDiagnostics(builder *strings.Builder, diagnostics []reportmodel.NodeDiagnostic) {
	for i := range diagnostics {
		switch diagnostics[i].Kind {
		case reportmodel.NodeDiagnosticKindHTTP:
			if diagnostics[i].HTTP != nil {
				renderHTTPDiagnostic(builder, *diagnostics[i].HTTP)
			}
		case reportmodel.NodeDiagnosticKindPreflight:
			if diagnostics[i].Preflight != nil {
				renderPreflightDiagnostic(builder, *diagnostics[i].Preflight)
			}
		}
	}
}

func renderPreflightDiagnostic(builder *strings.Builder, diagnostic reportmodel.PreflightDiagnostic) {
	builder.WriteString("  preflight:\n")
	fmt.Fprintf(builder, "    guard: %s\n", renderPreflightDiagnosticText(diagnostic.GuardID))
	if diagnostic.InputPath != "" {
		fmt.Fprintf(builder, "    input: %s\n", renderPreflightDiagnosticText(diagnostic.InputPath))
	} else {
		fmt.Fprintf(builder, "    input: %s\n", renderPreflightDiagnosticText(diagnostic.InputRef))
	}
	if diagnostic.AssertRef != "" {
		fmt.Fprintf(builder, "    assert: %s\n", renderPreflightDiagnosticText(diagnostic.AssertRef))
	}
	fmt.Fprintf(builder, "    reason: %s\n", renderPreflightDiagnosticText(diagnostic.ReasonCode))
	if diagnostic.OverridePresent {
		fmt.Fprintf(builder, "    override: %s used=%t\n", renderPreflightDiagnosticText(diagnostic.OverrideRef), diagnostic.OverrideUsed)
	}
	if source := formatSourceSpan(diagnostic.SourceSpan); source != "" {
		fmt.Fprintf(builder, "    source: %s\n", renderPreflightDiagnosticText(source))
	}
	if source := formatSourceSpan(diagnostic.BindingSourceSpan); source != "" {
		fmt.Fprintf(builder, "    binding_source: %s\n", renderPreflightDiagnosticText(source))
	}
}

func renderPreflightDiagnosticText(value string) string {
	value = strings.TrimSpace(sanitizeCLIText(value))
	runes := []rune(value)
	if len(runes) <= preflightDiagnosticTextLimit {
		return value
	}

	return string(runes[:preflightDiagnosticTextLimit]) + "..." + renderPreviewTruncatedSuffix
}

func renderHTTPDiagnostic(builder *strings.Builder, diagnostic reportmodel.HTTPDiagnostic) {
	builder.WriteString("  http:\n")
	if diagnostic.FailureKind != "" {
		fmt.Fprintf(builder, "    failure: %s\n", diagnostic.FailureKind)
	}
	if diagnostic.Method != "" || diagnostic.URL != "" {
		fmt.Fprintf(builder, "    request: %s %s\n", diagnostic.Method, diagnostic.URL)
	}
	if diagnostic.RequestFingerprint != nil {
		renderHTTPDiagnosticFingerprint(builder, *diagnostic.RequestFingerprint)
	}
	if diagnostic.StatusCode != 0 || diagnostic.Status != "" {
		fmt.Fprintf(builder, "    response: %d %s\n", diagnostic.StatusCode, diagnostic.Status)
	}
	if diagnostic.ResponseMetadata != nil {
		renderHTTPDiagnosticResponseMetadata(builder, *diagnostic.ResponseMetadata)
	}
	if diagnostic.DurationMs >= 0 {
		fmt.Fprintf(builder, "    duration: %s\n", humanDuration(diagnostic.DurationMs))
	}
	for _, key := range orderedHeaderKeys(diagnostic.ResponseHeaders) {
		fmt.Fprintf(builder, "    header.%s: %s\n", key, strings.Join(diagnostic.ResponseHeaders[key], ", "))
	}
	if diagnostic.ResponsePreview != nil {
		builder.WriteString("    body: ")
		builder.WriteString(renderHTTPDiagnosticPreview(diagnostic.ResponsePreview))
		builder.WriteByte('\n')
	}
}

func renderHTTPDiagnosticFingerprint(builder *strings.Builder, fingerprint reportmodel.HTTPRequestFingerprint) {
	if fingerprint.Host != "" {
		fmt.Fprintf(builder, "    request.host: %s\n", fingerprint.Host)
	}
	if fingerprint.PathShape != "" {
		fmt.Fprintf(builder, "    request.path_shape: %s\n", fingerprint.PathShape)
	}
	if len(fingerprint.QueryKeys) != 0 {
		fmt.Fprintf(builder, "    request.query_keys: %s\n", strings.Join(fingerprint.QueryKeys, ", "))
	}
}

func renderHTTPDiagnosticResponseMetadata(builder *strings.Builder, metadata reportmodel.HTTPResponseMetadata) {
	if metadata.ContentType != "" {
		fmt.Fprintf(builder, "    response.content_type: %s\n", metadata.ContentType)
	}
	if metadata.ContentLengthBytes != 0 {
		fmt.Fprintf(builder, "    response.content_length_bytes: %d\n", metadata.ContentLengthBytes)
	}
	if metadata.PreviewKind != "" {
		fmt.Fprintf(builder, "    response.preview_kind: %s\n", metadata.PreviewKind)
	}
	if metadata.PreviewOmittedReason != "" {
		fmt.Fprintf(builder, "    response.preview_omitted_reason: %s\n", metadata.PreviewOmittedReason)
	}
}

func renderHTTPDiagnosticPreview(preview *reportmodel.Preview) string {
	if preview == nil {
		return renderPreviewUnavailable
	}

	var rendered string
	switch {
	case preview.Text != "":
		rendered = preview.Text
	case preview.OmittedReason != "":
		rendered = "<" + preview.OmittedReason + ">"
	case preview.Redacted:
		rendered = renderPreviewRedacted
	default:
		rendered = renderPreviewEmpty
	}
	if preview.Redacted && preview.Text != "" {
		rendered += renderPreviewRedactedSuffix
	}
	if preview.Truncated {
		rendered += renderPreviewTruncatedSuffix
	}

	return rendered
}

func orderedHeaderKeys(headers map[string][]string) []string {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func filteredObservedValues(
	observed map[string]reportmodel.ObservedValue,
	streams map[string]reportmodel.ObservedStream,
) map[string]reportmodel.ObservedValue {
	if len(observed) == 0 || len(streams) == 0 {
		return observed
	}

	filtered := make(map[string]reportmodel.ObservedValue, len(observed))
	for key, value := range observed {
		if _, ok := streams[key]; ok {
			continue
		}
		filtered[key] = value
	}

	return filtered
}

func primaryActID(node reportmodel.NodeReport) string {
	if node.Address != nil && node.Address.ActID != "" {
		return node.Address.ActID
	}

	return ""
}

func formatSourceSpan(source *reportmodel.SourceRef) string {
	if source == nil {
		return ""
	}

	parts := make([]string, 0, 3)
	if source.File != "" {
		parts = append(parts, source.File)
	}
	if source.Line > 0 {
		location := strconv.Itoa(source.Line)
		if source.Column > 0 {
			location = fmt.Sprintf("%s:%d", location, source.Column)
		}
		parts = append(parts, location)
	}

	return strings.Join(parts, ":")
}

func humanDuration(durationMs int64) string {
	return (time.Duration(durationMs) * time.Millisecond).String()
}

func emptyFallback(value, fallback string) string {
	if value != "" {
		return value
	}

	return fallback
}
