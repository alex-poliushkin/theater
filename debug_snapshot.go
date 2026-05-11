package theater

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/alex-poliushkin/theater/internal/runtimevalue"
)

const (
	debugSnapshotPreviewLimit        = actionObservationPreviewLimit
	debugSnapshotMaxCollectionFields = 64
	debugSnapshotMaxSectionFields    = 128
)

type debugSnapshotSection struct {
	Fields  []debugSnapshotField
	Omitted int
}

type debugSnapshotField struct {
	Key        string
	Origin     string
	Value      debugSafeValue
	SourceSpan *SourceRef `json:"source,omitempty"`
}

type debugSafeValue struct {
	Kind          string
	Text          string
	SizeHint      int64
	ContentType   string
	Redacted      bool
	Truncated     bool
	OmittedReason string
	Omitted       int
	Children      []debugSnapshotField
}

type debugSnapshotBuilder struct {
	previewLimit    int
	collectionLimit int
	sectionLimit    int
}

func newDebugSnapshotBuilder() debugSnapshotBuilder {
	return debugSnapshotBuilder{
		previewLimit:    debugSnapshotPreviewLimit,
		collectionLimit: debugSnapshotMaxCollectionFields,
		sectionLimit:    debugSnapshotMaxSectionFields,
	}
}

func (b debugSnapshotBuilder) actionInputsSection(
	args Args,
	contract ActionContract,
	bindings map[string]bindingPlan,
) debugSnapshotSection {
	return b.valuesSectionWithSources(
		Values(args),
		builderSnapshotSpecs(contract.Inputs),
		"action.input",
		bindingSourceSpans(bindings),
	)
}

func (b debugSnapshotBuilder) actionOutputsSection(outputs Outputs, contract ActionContract) debugSnapshotSection {
	return b.valuesSection(Values(outputs), builderSnapshotSpecs(contract.Outputs), "action.output")
}

func (b debugSnapshotBuilder) expectationInputsSection(
	actual any,
	actualSpec ValueContract,
	args Values,
	argSpecs []MatcherArg,
	bindings map[string]bindingPlan,
) debugSnapshotSection {
	values := make(Values, len(args)+1)
	values["actual"] = actual
	for key, value := range args {
		values["arg."+key] = value
	}

	specs := make(map[string]ValueContract, len(argSpecs)+1)
	specs["actual"] = actualSpec
	for i := range argSpecs {
		specs["arg."+argSpecs[i].Name] = argSpecs[i].Accepts
	}

	origins := make(map[string]string, len(values))
	origins["actual"] = "expectation.actual"
	for key := range args {
		origins["arg."+key] = "expectation.arg." + key
	}

	return b.sectionFromValuesWithSources(values, specs, origins, matcherArgSourceSpans(bindings))
}

func matcherArgSourceSpans(bindings map[string]bindingPlan) map[string]*SourceRef {
	if len(bindings) == 0 {
		return nil
	}

	sources := make(map[string]*SourceRef, len(bindings))
	for key := range bindings {
		if bindings[key].SourceSpan == nil {
			continue
		}

		sources["arg."+key] = bindings[key].SourceSpan
	}
	if len(sources) == 0 {
		return nil
	}

	return sources
}

func (b debugSnapshotBuilder) scopeSection(scope *valueScope) debugSnapshotSection {
	if scope == nil {
		return debugSnapshotSection{}
	}

	frames := make([]*valueScope, 0, 4)
	for current := scope; current != nil; current = current.parent {
		frames = append(frames, current)
	}

	visible := make(map[string]debugSnapshotField)
	for depth := range frames {
		origin := "scope.current"
		if depth > 0 {
			origin = "scope.parent." + strconv.Itoa(depth)
		}

		for key, value := range frames[depth].values {
			if isMissingValue(value) {
				continue
			}
			if _, ok := visible[key]; ok {
				continue
			}

			visible[key] = debugSnapshotField{
				Key:    key,
				Origin: origin,
				Value:  b.safeValue(value, ValueContract{}),
			}
		}
	}

	return b.sectionFromFields(visible)
}

func (b debugSnapshotBuilder) valuesSection(
	values Values,
	specs map[string]ValueContract,
	originPrefix string,
) debugSnapshotSection {
	return b.valuesSectionWithSources(values, specs, originPrefix, nil)
}

func (b debugSnapshotBuilder) valuesSectionWithSources(
	values Values,
	specs map[string]ValueContract,
	originPrefix string,
	sources map[string]*SourceRef,
) debugSnapshotSection {
	if len(values) == 0 {
		return debugSnapshotSection{}
	}

	origins := make(map[string]string, len(values))
	for key := range values {
		origins[key] = originPrefix + "." + key
	}

	return b.sectionFromValuesWithSources(values, specs, origins, sources)
}

func (b debugSnapshotBuilder) sectionFromValues(
	values Values,
	specs map[string]ValueContract,
	origins map[string]string,
) debugSnapshotSection {
	return b.sectionFromValuesWithSources(values, specs, origins, nil)
}

func (b debugSnapshotBuilder) sectionFromValuesWithSources(
	values Values,
	specs map[string]ValueContract,
	origins map[string]string,
	sources map[string]*SourceRef,
) debugSnapshotSection {
	if len(values) == 0 {
		return debugSnapshotSection{}
	}

	fields := make(map[string]debugSnapshotField, len(values))
	for key, value := range values {
		spec := ValueContract{}
		if specs != nil {
			next, ok := specs[key]
			if !ok {
				continue
			}
			spec = next
		}

		origin := key
		if origins != nil {
			if text, ok := origins[key]; ok && text != "" {
				origin = text
			}
		}

		fields[key] = debugSnapshotField{
			Key:        key,
			Origin:     origin,
			Value:      b.safeValue(value, spec),
			SourceSpan: cloneSourceRef(sources[key]),
		}
	}

	return b.sectionFromFields(fields)
}

func bindingSourceSpans(bindings map[string]bindingPlan) map[string]*SourceRef {
	if len(bindings) == 0 {
		return nil
	}

	sources := make(map[string]*SourceRef, len(bindings))
	for key := range bindings {
		if bindings[key].SourceSpan == nil {
			continue
		}

		sources[key] = bindings[key].SourceSpan
	}
	if len(sources) == 0 {
		return nil
	}

	return sources
}

func (b debugSnapshotBuilder) sectionFromFields(fields map[string]debugSnapshotField) debugSnapshotSection {
	if len(fields) == 0 {
		return debugSnapshotSection{}
	}

	builder := b.normalized()
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	limit := builder.sectionLimit
	if limit > len(keys) {
		limit = len(keys)
	}

	section := debugSnapshotSection{
		Fields: make([]debugSnapshotField, 0, limit),
	}
	for i := 0; i < limit; i++ {
		section.Fields = append(section.Fields, fields[keys[i]])
	}
	if len(keys) > limit {
		section.Omitted = len(keys) - limit
	}

	return section
}

func (b debugSnapshotBuilder) safeValue(value any, spec ValueContract) debugSafeValue {
	builder := b.normalized()
	protected := protectValue(value, spec)
	preview := builder.preview(protected, spec)
	safe := debugSafeValue{
		Kind:          preview.Kind,
		Text:          preview.Text,
		SizeHint:      preview.SizeHint,
		ContentType:   preview.ContentType,
		Redacted:      preview.Redacted,
		Truncated:     preview.Truncated,
		OmittedReason: preview.OmittedReason,
	}
	if safe.Redacted {
		return safe
	}

	switch resolvedValueKind(protected) {
	case ValueKindObject:
		object, ok := runtimevalue.Object(protected)
		if ok {
			safe.Children, safe.Omitted = builder.objectChildren(object, spec)
		}
	case ValueKindList:
		list, ok := runtimevalue.List(protected)
		if ok {
			safe.Children, safe.Omitted = builder.listChildren(list, spec)
		}
	}

	return safe
}

func (b debugSnapshotBuilder) preview(value any, spec ValueContract) *Preview {
	builder := b.normalized()
	sensitivity, _ := normalizeVisibility(spec)
	if runtimevalue.Wrap(value).IsSecret() {
		sensitivity = SensitivitySecret
	}

	kind := observedPreviewKind(value)
	sizeHint := observedSizeHint(value)
	contentType := observedContentType(value)
	if sensitivity == SensitivitySecret {
		return &Preview{
			Kind:        kind,
			Text:        redactedPreview,
			SizeHint:    sizeHint,
			ContentType: contentType,
			Redacted:    true,
		}
	}

	text, truncated := debugObservedPreviewText(value, builder.previewLimit)
	return &Preview{
		Kind:        kind,
		Text:        text,
		SizeHint:    sizeHint,
		ContentType: contentType,
		Truncated:   truncated,
	}
}

func builderSnapshotSpecs(specs map[string]ValueContract) map[string]ValueContract {
	if specs != nil {
		return specs
	}

	return map[string]ValueContract{}
}

func (b debugSnapshotBuilder) objectChildren(
	object map[string]any,
	spec ValueContract,
) (children []debugSnapshotField, omitted int) {
	if len(object) == 0 {
		return nil, 0
	}

	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	limit := min(b.normalized().collectionLimit, len(keys))
	children = make([]debugSnapshotField, 0, limit)
	for i := 0; i < limit; i++ {
		childSpec := ValueContract{}
		if next, ok := objectMemberContract(spec, keys[i]); ok {
			childSpec = next
		}

		children = append(children, debugSnapshotField{
			Key:   keys[i],
			Value: b.safeValue(object[keys[i]], childSpec),
		})
	}

	return children, len(keys) - limit
}

func (b debugSnapshotBuilder) listChildren(
	list []any,
	spec ValueContract,
) (children []debugSnapshotField, omitted int) {
	if len(list) == 0 {
		return nil, 0
	}

	limit := min(b.normalized().collectionLimit, len(list))
	children = make([]debugSnapshotField, 0, limit)
	for i := 0; i < limit; i++ {
		childSpec := ValueContract{}
		if spec.Elem != nil {
			childSpec = *spec.Elem
		}

		children = append(children, debugSnapshotField{
			Key:   strconv.Itoa(i),
			Value: b.safeValue(list[i], childSpec),
		})
	}

	return children, len(list) - limit
}

func (b debugSnapshotBuilder) normalized() debugSnapshotBuilder {
	if b.previewLimit <= 0 {
		b.previewLimit = debugSnapshotPreviewLimit
	}
	if b.collectionLimit <= 0 {
		b.collectionLimit = debugSnapshotMaxCollectionFields
	}
	if b.sectionLimit <= 0 {
		b.sectionLimit = debugSnapshotMaxSectionFields
	}

	return b
}

func debugObservedPreviewText(value any, limit int) (string, bool) {
	wrapped := runtimevalue.Wrap(value)
	if text, ok := wrapped.StringOK(); ok {
		sanitized := sanitizeProjection(text)
		return truncatePreviewMiddle(sanitized, limit)
	}

	if bytes, ok := wrapped.BytesOK(); ok {
		return truncatePreviewMiddle(strconv.Itoa(len(bytes))+" bytes", limit)
	}

	text, ok := observedJSONText(value)
	if !ok {
		return truncatePreviewMiddle(sanitizeProjection(fmt.Sprintf("%v", value)), limit)
	}

	return truncatePreviewMiddle(sanitizeProjection(text), limit)
}
