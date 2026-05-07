package theater

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/alex-poliushkin/theater/internal/runtimevalue"
	"github.com/alex-poliushkin/theater/internal/streamtext"
)

const actionObservationPreviewLimit = 4 * 1024

var errPreviewLimitExceeded = errors.New("preview limit exceeded")

const (
	observedPreviewKindScalar      = "scalar"
	observedPreviewKindList        = "list"
	observedPreviewKindBytes       = "bytes"
	observedPreviewKindUnknown     = "unknown"
	previewOmittedReasonSensitive  = "sensitive"
	previewOmittedReasonNotVisible = "not_visible"
	observedContentTypeJSON        = "application/json"
	observedContentTypeOctetStream = "application/octet-stream"
	observedContentTypeTextPlain   = "text/plain"
)

func buildActionObservations(args Args, outputs Outputs, contract ActionContract) *ActionObservations {
	observations := &ActionObservations{
		Inputs:  observeValues(args, contract.Inputs, "action.input"),
		Outputs: observeValues(outputs, contract.Outputs, "action.output"),
	}

	if len(observations.Inputs) == 0 && len(observations.Outputs) == 0 {
		return nil
	}

	return observations
}

func observeValues(values map[string]any, specs map[string]ValueContract, originPrefix string) map[string]ObservedValue {
	if len(values) == 0 || len(specs) == 0 {
		return nil
	}

	observed := make(map[string]ObservedValue)
	for key, value := range values {
		spec, ok := specs[key]
		if !ok || !hasDiagnosticsVisibility(spec) {
			continue
		}

		observed[key] = observeValue(originPrefix+"."+key, value, spec)
	}

	if len(observed) == 0 {
		return nil
	}

	return observed
}

func hasDiagnosticsVisibility(spec ValueContract) bool {
	return spec.Capture != "" || spec.Sensitivity != ""
}

func observeValue(origin string, value any, spec ValueContract) ObservedValue {
	return observeValueWithPreviewLimit(origin, value, spec, actionObservationPreviewLimit)
}

func observeLogValue(origin string, value any, spec ValueContract) ObservedValue {
	sensitivity, capture := normalizeVisibility(spec)
	if runtimevalue.Wrap(value).IsSecret() {
		sensitivity = SensitivitySecret
	}

	preview := buildLogObservedPreview(value, sensitivity, capture)
	if capture != CaptureSummary {
		return ObservedValue{Preview: preview}
	}

	return ObservedValue{
		Preview: preview,
		Payload: &PayloadMetadata{
			Origin:      origin,
			Sensitivity: sensitivity,
			Redacted:    preview.Redacted,
			Truncated:   preview.Truncated,
			ContentType: preview.ContentType,
			SizeBytes:   preview.SizeHint,
			Capture:     capture,
		},
	}
}

func observeValueWithPreviewLimit(origin string, value any, spec ValueContract, previewLimit int) ObservedValue {
	sensitivity, capture := normalizeVisibility(spec)
	if runtimevalue.Wrap(value).IsSecret() {
		sensitivity = SensitivitySecret
	}

	preview := buildObservedPreview(value, sensitivity, capture, previewLimit)
	if capture != CaptureSummary {
		return ObservedValue{Preview: preview}
	}

	return ObservedValue{
		Preview: preview,
		Payload: &PayloadMetadata{
			Origin:      origin,
			Sensitivity: sensitivity,
			Redacted:    preview.Redacted,
			Truncated:   preview.Truncated,
			ContentType: preview.ContentType,
			SizeBytes:   preview.SizeHint,
			Capture:     capture,
		},
	}
}

func buildStreamObservations(summaries map[string]actionStreamSummary) *ActionObservations {
	if len(summaries) == 0 {
		return nil
	}

	observations := &ActionObservations{
		Streams: make(map[string]ObservedStream, len(summaries)),
	}
	for stream, summary := range summaries {
		observations.Streams[stream] = observeStream(stream, summary)
	}

	return observations
}

func normalizeVisibility(spec ValueContract) (sensitivity Sensitivity, capture Capture) {
	sensitivity = spec.Sensitivity
	if sensitivity == "" {
		sensitivity = SensitivityInternal
	}

	capture = spec.Capture
	if capture == "" {
		capture = CaptureOmit
	}

	return sensitivity, capture
}

func buildObservedPreview(value any, sensitivity Sensitivity, capture Capture, limit int) *Preview {
	kind := observedPreviewKind(value)
	sizeHint := observedSizeHint(value)
	contentType := observedContentType(value)
	redacted := sensitivity == SensitivitySecret || runtimevalue.Wrap(value).IsSecret()

	if capture == CaptureOmit {
		preview := &Preview{
			Kind:        kind,
			SizeHint:    sizeHint,
			ContentType: contentType,
		}
		if redacted {
			preview.Redacted = true
			preview.Text = redactedPreview
			preview.OmittedReason = previewOmittedReasonSensitive
			return preview
		}

		preview.OmittedReason = previewOmittedReasonNotVisible
		return preview
	}

	if redacted {
		return &Preview{
			Kind:        kind,
			Text:        redactedPreview,
			SizeHint:    sizeHint,
			ContentType: contentType,
			Redacted:    true,
		}
	}

	text, truncated := observedPreviewText(value, limit)
	preview := &Preview{
		Kind:        kind,
		Text:        text,
		SizeHint:    sizeHint,
		Truncated:   truncated,
		ContentType: contentType,
	}

	return preview
}

func buildLogObservedPreview(value any, sensitivity Sensitivity, capture Capture) *Preview {
	kind := observedLogPreviewKind(value)
	sizeHint := observedLogSizeHint(value)
	contentType := observedLogContentType(value)
	redacted := sensitivity == SensitivitySecret || runtimevalue.Wrap(value).IsSecret()

	if capture == CaptureOmit {
		preview := &Preview{
			Kind:        kind,
			SizeHint:    sizeHint,
			ContentType: contentType,
		}
		if redacted {
			preview.Redacted = true
			preview.Text = redactedPreview
			preview.OmittedReason = previewOmittedReasonSensitive
			return preview
		}

		preview.OmittedReason = previewOmittedReasonNotVisible
		return preview
	}

	if redacted {
		return &Preview{
			Kind:        kind,
			Text:        redactedPreview,
			SizeHint:    sizeHint,
			ContentType: contentType,
			Redacted:    true,
		}
	}

	text, truncated, hint := observedLogPreviewText(value, DefaultScenarioLogPreviewLimitBytes)
	if sizeHint == 0 {
		sizeHint = hint
	}
	return &Preview{
		Kind:        kind,
		Text:        text,
		SizeHint:    sizeHint,
		Truncated:   truncated,
		ContentType: contentType,
	}
}

func observedPreviewKind(value any) string {
	switch resolvedValueKind(value) {
	case ValueKindString:
		return jsonSchemaTypeString
	case ValueKindNumber:
		return observedPreviewKindScalar
	case ValueKindBool:
		return observedPreviewKindScalar
	case ValueKindObject:
		return jsonSchemaTypeObject
	case ValueKindList:
		return observedPreviewKindList
	case ValueKindBytes:
		return observedPreviewKindBytes
	case ValueKindNull:
		return jsonSchemaTypeNull
	default:
		return observedPreviewKindUnknown
	}
}

func observedSizeHint(value any) int64 {
	wrapped := runtimevalue.Wrap(value)
	if text, ok := wrapped.StringOK(); ok {
		return int64(len(text))
	}

	if bytes, ok := wrapped.BytesOK(); ok {
		return int64(len(bytes))
	}

	text, _ := observedJSONText(value)
	return int64(len(text))
}

func observedLogSizeHint(value any) int64 {
	wrapped := runtimevalue.Wrap(value)
	if text, ok := wrapped.StringOK(); ok {
		return int64(len(text))
	}

	if bytes, ok := wrapped.BytesOK(); ok {
		return int64(len(bytes))
	}

	return 0
}

func observedContentType(value any) string {
	switch resolvedValueKind(value) {
	case ValueKindObject, ValueKindList, ValueKindNumber, ValueKindBool, ValueKindNull:
		return observedContentTypeJSON
	case ValueKindBytes:
		return observedContentTypeOctetStream
	default:
		return observedContentTypeTextPlain
	}
}

func observedLogPreviewKind(value any) string {
	switch resolvedLogValueKind(value) {
	case ValueKindString:
		return jsonSchemaTypeString
	case ValueKindNumber:
		return observedPreviewKindScalar
	case ValueKindBool:
		return observedPreviewKindScalar
	case ValueKindObject:
		return jsonSchemaTypeObject
	case ValueKindList:
		return observedPreviewKindList
	case ValueKindBytes:
		return observedPreviewKindBytes
	case ValueKindNull:
		return jsonSchemaTypeNull
	default:
		return observedPreviewKindUnknown
	}
}

func observedLogContentType(value any) string {
	switch resolvedLogValueKind(value) {
	case ValueKindObject, ValueKindList, ValueKindNumber, ValueKindBool, ValueKindNull:
		return observedContentTypeJSON
	case ValueKindBytes:
		return observedContentTypeOctetStream
	default:
		return observedContentTypeTextPlain
	}
}

func resolvedLogValueKind(value any) ValueKind {
	switch typed := value.(type) {
	case nil:
		return ValueKindNull
	case []byte:
		return ValueKindBytes
	case string:
		return ValueKindString
	case bool:
		return ValueKindBool
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64,
		json.Number:
		return ValueKindNumber
	case map[string]any:
		return ValueKindObject
	case []any:
		return ValueKindList
	default:
		return reflectedLogValueKind(typed)
	}
}

func reflectedLogValueKind(value any) ValueKind {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return ValueKindNull
	}

	switch reflected.Kind() {
	case reflect.Map:
		if reflected.Type().Key().Kind() == reflect.String {
			return ValueKindObject
		}
	case reflect.Slice, reflect.Array:
		if reflected.Kind() == reflect.Slice && reflected.Type().Elem().Kind() == reflect.Uint8 {
			return ValueKindBytes
		}
		return ValueKindList
	case reflect.String:
		return ValueKindString
	case reflect.Bool:
		return ValueKindBool
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return ValueKindNumber
	}

	return ValueKindAny
}

func observedPreviewText(value any, limit int) (string, bool) {
	wrapped := runtimevalue.Wrap(value)
	if text, ok := wrapped.StringOK(); ok {
		sanitized := sanitizeProjection(text)
		return truncatePreviewMiddle(sanitized, limit)
	}

	if bytes, ok := wrapped.BytesOK(); ok {
		return truncatePreviewMiddle(fmt.Sprintf("%d bytes", len(bytes)), limit)
	}

	text, ok := observedJSONText(value)
	if !ok {
		return truncatePreviewMiddle(fmt.Sprintf("%v", value), limit)
	}

	return truncatePreviewMiddle(sanitizeProjection(text), limit)
}

func observedLogPreviewText(value any, limit int) (text string, truncated bool, sizeHint int64) {
	wrapped := runtimevalue.Wrap(value)
	if text, ok := wrapped.StringOK(); ok {
		sanitized := sanitizeProjection(text)
		preview, truncated := truncatePreviewMiddle(sanitized, limit)
		return preview, truncated, int64(len(text))
	}

	if bytes, ok := wrapped.BytesOK(); ok {
		text := fmt.Sprintf("%d bytes", len(bytes))
		preview, truncated := truncatePreviewMiddle(text, limit)
		return preview, truncated, int64(len(bytes))
	}

	text, truncated, ok := observedBoundedJSONText(value, limit)
	if !ok {
		text = fmt.Sprintf("%v", value)
	}
	sanitized := sanitizeProjection(text)
	preview, previewTruncated := truncatePreviewMiddle(sanitized, limit)
	return preview, truncated || previewTruncated, int64(len(text))
}

func observedJSONText(value any) (string, bool) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", false
	}

	return string(encoded), true
}

func observedBoundedJSONText(value any, limit int) (text string, truncated, ok bool) {
	writer := newPreviewLimitWriter(limit)
	err := writeBoundedJSONValue(writer, value)
	if err != nil && !errors.Is(err, errPreviewLimitExceeded) {
		return "", false, false
	}

	text = writer.String()
	return text, writer.Exceeded(), true
}

func truncatePreviewMiddle(value string, limit int) (string, bool) {
	return streamtext.TruncateMiddle(value, limit, "...")
}

type previewLimitWriter struct {
	builder strings.Builder
	limit   int
	seen    int
}

func newPreviewLimitWriter(limit int) *previewLimitWriter {
	return &previewLimitWriter{limit: limit}
}

func (w *previewLimitWriter) Write(data []byte) (int, error) {
	if w.limit < 0 {
		w.seen += len(data)
		_, _ = w.builder.Write(data)
		return len(data), nil
	}

	remaining := w.limit + 1 - w.builder.Len()
	if remaining <= 0 {
		w.seen += len(data)
		return 0, errPreviewLimitExceeded
	}

	if len(data) <= remaining {
		w.seen += len(data)
		_, _ = w.builder.Write(data)
		if w.builder.Len() > w.limit {
			return len(data), errPreviewLimitExceeded
		}

		return len(data), nil
	}

	w.seen += len(data)
	_, _ = w.builder.Write(data[:remaining])
	return remaining, errPreviewLimitExceeded
}

func (w *previewLimitWriter) Exceeded() bool {
	return w.limit >= 0 && w.seen > w.limit
}

func (w *previewLimitWriter) String() string {
	text := w.builder.String()
	if w.limit < 0 || len(text) <= w.limit {
		return text
	}

	prefix := streamtext.SafePrefixLen([]byte(text), w.limit)
	return text[:prefix]
}

func writeBoundedJSONValue(writer *previewLimitWriter, value any) error {
	if runtimevalue.Wrap(value).IsSecret() {
		return writeBoundedJSONString(writer, redactedPreview)
	}

	switch typed := value.(type) {
	case nil:
		return writePreviewLiteral(writer, "null")
	case string:
		return writeBoundedJSONString(writer, typed)
	case bool:
		if typed {
			return writePreviewLiteral(writer, "true")
		}

		return writePreviewLiteral(writer, "false")
	case json.Number:
		return writePreviewLiteral(writer, typed.String())
	case []byte:
		return writeBoundedJSONString(writer, fmt.Sprintf("%d bytes", len(typed)))
	case map[string]any:
		return writeBoundedJSONObject(writer, typed)
	case []any:
		return writeBoundedJSONList(writer, typed)
	default:
		if ok, err := writeBoundedJSONNumber(writer, value); ok {
			return err
		}

		return writeBoundedReflectJSONValue(writer, value)
	}
}

func writeBoundedJSONNumber(writer *previewLimitWriter, value any) (bool, error) {
	switch typed := value.(type) {
	case int:
		return true, writePreviewLiteral(writer, strconv.Itoa(typed))
	case int8:
		return true, writePreviewLiteral(writer, strconv.FormatInt(int64(typed), 10))
	case int16:
		return true, writePreviewLiteral(writer, strconv.FormatInt(int64(typed), 10))
	case int32:
		return true, writePreviewLiteral(writer, strconv.FormatInt(int64(typed), 10))
	case int64:
		return true, writePreviewLiteral(writer, strconv.FormatInt(typed, 10))
	case uint:
		return true, writePreviewLiteral(writer, strconv.FormatUint(uint64(typed), 10))
	case uint8:
		return true, writePreviewLiteral(writer, strconv.FormatUint(uint64(typed), 10))
	case uint16:
		return true, writePreviewLiteral(writer, strconv.FormatUint(uint64(typed), 10))
	case uint32:
		return true, writePreviewLiteral(writer, strconv.FormatUint(uint64(typed), 10))
	case uint64:
		return true, writePreviewLiteral(writer, strconv.FormatUint(typed, 10))
	case float32:
		return true, writePreviewLiteral(writer, strconv.FormatFloat(float64(typed), 'f', -1, 32))
	case float64:
		return true, writePreviewLiteral(writer, strconv.FormatFloat(typed, 'f', -1, 64))
	default:
		return false, nil
	}
}

func writeBoundedJSONObject(writer *previewLimitWriter, object map[string]any) error {
	if err := writePreviewLiteral(writer, "{"); err != nil {
		return err
	}

	i := 0
	for key, value := range object {
		if i != 0 {
			if err := writePreviewLiteral(writer, ","); err != nil {
				return err
			}
		}
		if err := writeBoundedJSONString(writer, key); err != nil {
			return err
		}
		if err := writePreviewLiteral(writer, ":"); err != nil {
			return err
		}
		if err := writeBoundedJSONValue(writer, value); err != nil {
			return err
		}
		i++
	}

	return writePreviewLiteral(writer, "}")
}

func writeBoundedReflectJSONValue(writer *previewLimitWriter, value any) error {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return writePreviewLiteral(writer, "null")
	}

	switch reflected.Kind() {
	case reflect.Map:
		if reflected.Type().Key().Kind() != reflect.String {
			return writeBoundedJSONString(writer, fmt.Sprintf("%v", value))
		}
		return writeBoundedReflectJSONObject(writer, reflected)
	case reflect.Slice, reflect.Array:
		if reflected.Kind() == reflect.Slice && reflected.IsNil() {
			return writePreviewLiteral(writer, "null")
		}
		if reflected.Type().Elem().Kind() == reflect.Uint8 {
			return writeBoundedJSONString(writer, fmt.Sprintf("%d bytes", reflected.Len()))
		}
		return writeBoundedReflectJSONList(writer, reflected)
	case reflect.String:
		return writeBoundedJSONString(writer, reflected.String())
	case reflect.Bool:
		if reflected.Bool() {
			return writePreviewLiteral(writer, "true")
		}

		return writePreviewLiteral(writer, "false")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return writePreviewLiteral(writer, strconv.FormatInt(reflected.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return writePreviewLiteral(writer, strconv.FormatUint(reflected.Uint(), 10))
	case reflect.Float32, reflect.Float64:
		return writePreviewLiteral(writer, strconv.FormatFloat(reflected.Float(), 'f', -1, reflected.Type().Bits()))
	default:
		return writeBoundedJSONString(writer, fmt.Sprintf("%v", value))
	}
}

func writeBoundedReflectJSONObject(writer *previewLimitWriter, object reflect.Value) error {
	if object.IsNil() {
		return writePreviewLiteral(writer, "null")
	}
	if err := writePreviewLiteral(writer, "{"); err != nil {
		return err
	}

	i := 0
	iter := object.MapRange()
	for iter.Next() {
		if i != 0 {
			if err := writePreviewLiteral(writer, ","); err != nil {
				return err
			}
		}
		if err := writeBoundedJSONString(writer, iter.Key().String()); err != nil {
			return err
		}
		if err := writePreviewLiteral(writer, ":"); err != nil {
			return err
		}
		if err := writeBoundedJSONValue(writer, iter.Value().Interface()); err != nil {
			return err
		}
		i++
	}

	return writePreviewLiteral(writer, "}")
}

func writeBoundedReflectJSONList(writer *previewLimitWriter, list reflect.Value) error {
	if err := writePreviewLiteral(writer, "["); err != nil {
		return err
	}

	for i := 0; i < list.Len(); i++ {
		if i != 0 {
			if err := writePreviewLiteral(writer, ","); err != nil {
				return err
			}
		}
		if err := writeBoundedJSONValue(writer, list.Index(i).Interface()); err != nil {
			return err
		}
	}

	return writePreviewLiteral(writer, "]")
}

func writeBoundedJSONList(writer *previewLimitWriter, list []any) error {
	if err := writePreviewLiteral(writer, "["); err != nil {
		return err
	}

	for i := range list {
		if i != 0 {
			if err := writePreviewLiteral(writer, ","); err != nil {
				return err
			}
		}
		if err := writeBoundedJSONValue(writer, list[i]); err != nil {
			return err
		}
	}

	return writePreviewLiteral(writer, "]")
}

func writeBoundedJSONString(writer *previewLimitWriter, text string) error {
	if err := writePreviewLiteral(writer, `"`); err != nil {
		return err
	}

	for _, r := range text {
		var escaped string
		switch r {
		case '\\':
			escaped = `\\`
		case '"':
			escaped = `\"`
		case '\b':
			escaped = `\b`
		case '\f':
			escaped = `\f`
		case '\n':
			escaped = `\n`
		case '\r':
			escaped = `\r`
		case '\t':
			escaped = `\t`
		default:
			if r < 0x20 {
				escaped = fmt.Sprintf(`\u%04x`, r)
			} else {
				escaped = string(r)
			}
		}
		if err := writePreviewLiteral(writer, escaped); err != nil {
			return err
		}
	}

	return writePreviewLiteral(writer, `"`)
}

func writePreviewLiteral(writer *previewLimitWriter, text string) error {
	_, err := writer.Write([]byte(text))
	return err
}

func partialOutputObservations(err error, contract ActionContract) *ActionObservations {
	details, ok := actionErrorDetails(err)
	if !ok {
		return nil
	}

	if len(details.PartialOutputs()) == 0 {
		return nil
	}

	return buildActionObservations(nil, protectActionOutputs(details.PartialOutputs(), contract.Outputs), contract)
}

func mergeActionObservations(left, right *ActionObservations) *ActionObservations {
	if left == nil {
		return cloneActionObservations(right)
	}

	if right == nil {
		return cloneActionObservations(left)
	}

	merged := cloneActionObservations(left)
	if len(right.Inputs) != 0 {
		if merged.Inputs == nil {
			merged.Inputs = make(map[string]ObservedValue, len(right.Inputs))
		}

		for key, value := range right.Inputs {
			merged.Inputs[key] = cloneObservedValue(value)
		}
	}

	if len(right.Outputs) != 0 {
		if merged.Outputs == nil {
			merged.Outputs = make(map[string]ObservedValue, len(right.Outputs))
		}

		for key, value := range right.Outputs {
			merged.Outputs[key] = cloneObservedValue(value)
		}
	}

	if len(right.Streams) != 0 {
		if merged.Streams == nil {
			merged.Streams = make(map[string]ObservedStream, len(right.Streams))
		}

		for key, value := range right.Streams {
			merged.Streams[key] = cloneObservedStream(value)
		}
	}

	return merged
}

func actionFailureSummary(err error, fallback string) string {
	details, ok := actionErrorDetails(err)
	if !ok {
		return fallback
	}

	summary := details.FailureSummary()
	if summary == "" {
		return fallback
	}

	return summary
}

func actionErrorDetails(err error) (ActionErrorDetails, bool) {
	var details ActionErrorDetails
	if !errors.As(err, &details) {
		return nil, false
	}

	return details, true
}

func observeStream(stream string, summary actionStreamSummary) ObservedStream {
	previewText := sanitizeStreamPreviewText(summary.Tail)
	return ObservedStream{
		Preview: &Preview{
			Kind:        jsonSchemaTypeString,
			Text:        previewText,
			SizeHint:    summary.SizeBytes,
			Truncated:   summary.SizeBytes > int64(len(summary.Tail)),
			ContentType: observedContentTypeTextPlain,
		},
		Payload: &PayloadMetadata{
			Origin:      "action.stream." + stream,
			Sensitivity: SensitivityInternal,
			ContentType: observedContentTypeTextPlain,
			SizeBytes:   summary.SizeBytes,
			Capture:     CaptureSummary,
		},
		DroppedChunks: summary.DroppedChunks,
	}
}

func sanitizeStreamPreviewText(chunk []byte) string {
	return streamtext.Render(chunk)
}
