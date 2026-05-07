package theater

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/alex-poliushkin/theater/internal/runtimevalue"
	"github.com/alex-poliushkin/theater/internal/selectvalue"
)

type valueLookup interface {
	lookupValue(name string) (any, bool)
}

type referenceResolver struct {
	source         valueLookup
	propertySource valueLookup
	bindingSource  valueLookup
	generators     GeneratorResolver
	matchers       MatcherResolver
	generation     *generationRuntime
	identity       executionIdentity
}

type layeredValueLookup struct {
	primary  valueLookup
	fallback valueLookup
}

type mapValueLookup Values

func newReferenceResolver(source any) referenceResolver {
	lookup := newValueLookup(source)
	return referenceResolver{
		source:        lookup,
		bindingSource: lookup,
	}
}

func newSubjectResolver(actionSource, propertySource, bindingSource any) referenceResolver {
	return referenceResolver{
		source:         newValueLookup(actionSource),
		propertySource: newValueLookup(propertySource),
		bindingSource:  newValueLookup(bindingSource),
	}
}

func (r referenceResolver) withGeneration(
	resolver GeneratorResolver,
	generation *generationRuntime,
	identity executionIdentity,
) referenceResolver {
	r.generators = resolver
	r.generation = generation
	r.identity = identity
	return r
}

func (r referenceResolver) withMatchers(matchers MatcherResolver) referenceResolver {
	r.matchers = matchers
	return r
}

func (r referenceResolver) withBindingSource(source any) referenceResolver {
	r.bindingSource = newValueLookup(source)
	return r
}

func newValueLookup(source any) valueLookup {
	switch typed := source.(type) {
	case nil:
		return nil
	case valueLookup:
		return typed
	case Values:
		return mapValueLookup(typed)
	default:
		panic(fmt.Sprintf("unsupported reference source %T", source))
	}
}

func (v mapValueLookup) lookupValue(name string) (any, bool) {
	if len(v) == 0 {
		return nil, false
	}

	value, ok := v[name]
	return value, ok
}

func (l layeredValueLookup) lookupValue(name string) (any, bool) {
	if l.primary != nil {
		if value, ok := l.primary.lookupValue(name); ok {
			return value, true
		}
	}

	if l.fallback != nil {
		return l.fallback.lookupValue(name)
	}

	return nil, false
}

func (r referenceResolver) ExportValues(exports []exportPlan) (Values, error) {
	return r.ExportValuesContext(context.Background(), exports)
}

func (r referenceResolver) ExportValuesContext(ctx context.Context, exports []exportPlan) (Values, error) {
	if len(exports) == 0 {
		return Values{}, nil
	}

	exported := make(Values, len(exports))
	for _, export := range exports {
		value, err := r.ResolveExportContext(ctx, export)
		if err != nil {
			return nil, err
		}

		exported[exportAlias(export)] = value
	}

	return exported, nil
}

func exportAlias(export exportPlan) string {
	if export.As != "" {
		return export.As
	}

	if export.Ref != nil {
		return export.Ref.Name
	}

	return export.Field
}

func (r referenceResolver) ResolveBindings(bindings map[string]bindingPlan) (Values, error) {
	return r.ResolveBindingsContext(context.Background(), bindings)
}

func (r referenceResolver) ResolveBindingsContext(ctx context.Context, bindings map[string]bindingPlan) (Values, error) {
	if len(bindings) == 0 {
		return Values{}, nil
	}

	resolved := make(Values, len(bindings))
	for key := range bindings {
		value, err := r.ResolveBindingContext(ctx, bindings[key])
		if err != nil {
			return nil, err
		}

		resolved[key] = value
	}

	return resolved, nil
}

func (r referenceResolver) ResolveBinding(binding bindingPlan) (any, error) {
	return r.ResolveBindingContext(context.Background(), binding)
}

func (r referenceResolver) ResolveBindingContext(ctx context.Context, binding bindingPlan) (any, error) {
	switch binding.Kind {
	case BindingKindLiteral:
		return runtimevalue.Clone(binding.Value), nil
	case BindingKindRef:
		if binding.Ref == nil {
			return nil, errors.New("binding ref is missing")
		}

		return r.resolveNamedValueFromContext(ctx, r.bindingSource, binding.Ref.Name, binding.Ref.selectorPlan, "ref %q is missing")
	case BindingKindObject:
		object := make(map[string]any, len(binding.Object))
		for key := range binding.Object {
			value, err := r.ResolveBindingContext(ctx, binding.Object[key])
			if err != nil {
				return nil, err
			}

			object[key] = value
		}

		return object, nil
	case BindingKindList:
		list := make([]any, 0, len(binding.List))
		for i := range binding.List {
			value, err := r.ResolveBindingContext(ctx, binding.List[i])
			if err != nil {
				return nil, err
			}

			list = append(list, value)
		}

		return list, nil
	case BindingKindString:
		return r.resolveStringBindingContext(ctx, binding)
	case BindingKindGenerate:
		return r.resolveGenerateBindingContext(ctx, binding)
	default:
		return nil, fmt.Errorf("binding kind %q is invalid", binding.Kind)
	}
}

func (r referenceResolver) ResolveSubject(subject subjectPlan) (any, error) {
	return r.ResolveSubjectContext(context.Background(), subject)
}

func (r referenceResolver) ResolveSubjectContext(ctx context.Context, subject subjectPlan) (any, error) {
	switch subject.From {
	case SubjectFromProperty:
		return r.resolveNamedValueFromContext(ctx, r.propertySource, subject.Ref, subject.selectorPlan, "subject ref %q is missing")
	default:
		return r.resolveNamedValueFromContext(ctx, r.source, subject.Field, subject.selectorPlan, "subject field %q is missing")
	}
}

func (r referenceResolver) ResolveRef(ref refPlan) (any, error) {
	return r.ResolveRefContext(context.Background(), ref)
}

func (r referenceResolver) ResolveRefContext(ctx context.Context, ref refPlan) (any, error) {
	return r.resolveNamedValueContext(ctx, ref.Name, ref.selectorPlan, "ref %q is missing")
}

func (r referenceResolver) ResolveExport(export exportPlan) (any, error) {
	return r.ResolveExportContext(context.Background(), export)
}

func (r referenceResolver) ResolveExportContext(ctx context.Context, export exportPlan) (any, error) {
	switch {
	case export.Ref != nil:
		return r.ResolveRefContext(ctx, *export.Ref)
	case export.Field != "":
		return r.resolveNamedValueContext(ctx, export.Field, export.selectorPlan, "export field %q is missing")
	default:
		return nil, errors.New("export selector is missing")
	}
}

func (r referenceResolver) resolveNamedValueContext(
	ctx context.Context,
	name string,
	selector selectorPlan,
	missingFormat string,
) (any, error) {
	return r.resolveNamedValueFromContext(ctx, r.source, name, selector, missingFormat)
}

func (r referenceResolver) resolveNamedValueFromContext(
	ctx context.Context,
	source valueLookup,
	name string,
	selector selectorPlan,
	missingFormat string,
) (any, error) {
	if source == nil {
		return nil, fmt.Errorf(missingFormat, name)
	}

	value, ok := source.lookupValue(name)
	if !ok {
		return nil, fmt.Errorf(missingFormat, name)
	}

	return r.resolveSelectedValueContext(ctx, value, selector)
}

func (r referenceResolver) resolveSelectedValueContext(ctx context.Context, value any, selector selectorPlan) (any, error) {
	current, err := selectvalue.Resolve(value, selector.Decode, selector.Path)
	if err != nil {
		return nil, err
	}

	for i := range selector.Through {
		current, err = r.applyThroughStepContext(ctx, current, selector.Through[i])
		if err != nil {
			return nil, err
		}
	}

	return current, nil
}

func (r referenceResolver) resolveStringBindingContext(ctx context.Context, binding bindingPlan) (any, error) {
	if len(binding.Parts) == 0 {
		return nil, errors.New("string parts are required")
	}

	var builder strings.Builder
	for i := range binding.Parts {
		value, err := r.ResolveBindingContext(ctx, binding.Parts[i])
		if err != nil {
			return nil, err
		}

		text, err := stringifyBindingPart(value, fmt.Sprintf("string part %d", i))
		if err != nil {
			return nil, err
		}
		builder.WriteString(text)
	}

	return builder.String(), nil
}

func (r referenceResolver) resolveGenerateBindingContext(ctx context.Context, binding bindingPlan) (any, error) {
	if binding.Generator == "" {
		return nil, errors.New("binding generator is missing")
	}

	if dependencyMissing(r.generators) {
		return nil, fmt.Errorf("generator %q is not available", binding.Generator)
	}

	if r.generation == nil {
		return nil, errors.New("generation runtime is not available")
	}

	return r.generation.Resolve(ctx, r.generators, binding, r.identity, r)
}

func (r referenceResolver) applyThroughStepContext(ctx context.Context, value any, step throughStepPlan) (any, error) {
	switch {
	case !step.Path.IsRoot():
		return selectvalue.Resolve(value, "", step.Path)
	case step.Pick != nil:
		return r.applyPickStepContext(ctx, value, *step.Pick)
	case step.Regexp != nil:
		return applyRegexpStep(value, *step.Regexp)
	default:
		return nil, errors.New("through step is invalid")
	}
}

func (r referenceResolver) applyPickStepContext(ctx context.Context, value any, step pickStepPlan) (any, error) {
	items, ok := runtimevalue.Wrap(value).ListOK()
	if !ok {
		return nil, fmt.Errorf("pick requires list input, got %T", value)
	}

	if len(step.Where) != 0 {
		return r.applyPickWhereStepContext(ctx, value, items, step.Where)
	}

	expected, err := r.resolveBindingFromContext(ctx, r.bindingSource, step.Equals)
	if err != nil {
		return nil, err
	}

	matchIndex := -1
	for i := range items {
		actual, err := selectvalue.Resolve(items[i], "", step.At)
		if err != nil {
			continue
		}

		if !reflect.DeepEqual(runtimevalue.Reveal(actual), runtimevalue.Reveal(expected)) {
			continue
		}

		if matchIndex != -1 {
			return nil, errors.New("pick matched multiple items")
		}

		matchIndex = i
	}

	if matchIndex == -1 {
		return nil, errors.New("pick matched no items")
	}

	return runtimevalue.PreserveSecret(value, items[matchIndex]), nil
}

func (r referenceResolver) applyPickWhereStepContext(
	ctx context.Context,
	source any,
	items []any,
	clauses []pickWhereClausePlan,
) (any, error) {
	if dependencyMissing(r.matchers) {
		return nil, errors.New("pick where requires matcher resolver")
	}

	compiled, err := r.compilePickWhereClausesContext(ctx, clauses)
	if err != nil {
		return nil, err
	}

	matchIndex := -1
	for i := range items {
		matches, err := pickWhereItemMatches(ctx, items[i], compiled)
		if err != nil {
			return nil, err
		}
		if !matches {
			continue
		}
		if matchIndex != -1 {
			return nil, errors.New("pick matched multiple items")
		}
		matchIndex = i
	}

	if matchIndex == -1 {
		return nil, errors.New("pick matched no items")
	}

	return runtimevalue.PreserveSecret(source, items[matchIndex]), nil
}

func (r referenceResolver) compilePickWhereClausesContext(
	ctx context.Context,
	clauses []pickWhereClausePlan,
) ([]compiledPickWhereClause, error) {
	compiled := make([]compiledPickWhereClause, 0, len(clauses))
	for i := range clauses {
		args, err := r.ResolveBindingsContext(ctx, clauses[i].Assert.Args)
		if err != nil {
			return nil, fmt.Errorf("pick where[%d] assert args %w", i, err)
		}

		descriptor, err := r.matchers.Resolve(clauses[i].Assert.Ref)
		if err != nil {
			return nil, fmt.Errorf("pick where[%d] assert %w", i, err)
		}

		matcher, err := descriptor.Compile(newMatcherCompileResolver(r.matchers), args)
		if err != nil {
			return nil, fmt.Errorf("pick where[%d] assert %q is invalid: %w", i, clauses[i].Assert.Ref, err)
		}

		compiled = append(compiled, compiledPickWhereClause{
			Subject: clauses[i].Subject,
			Matcher: matcher,
		})
	}

	return compiled, nil
}

type compiledPickWhereClause struct {
	Subject relativeSubjectPlan
	Matcher Matcher
}

func pickWhereItemMatches(ctx context.Context, item any, clauses []compiledPickWhereClause) (bool, error) {
	for i := range clauses {
		actual, ok := pickWhereSubjectValue(item, clauses[i].Subject)
		if !ok {
			return false, nil
		}

		if err := clauses[i].Matcher.Check(ctx, actual); err != nil {
			if IsMatcherMismatch(err) {
				return false, nil
			}

			return false, err
		}
	}

	return true, nil
}

func pickWhereSubjectValue(item any, subject relativeSubjectPlan) (any, bool) {
	actual, err := selectvalue.Resolve(item, subject.Decode, subject.Path)
	if err != nil {
		return nil, false
	}

	return actual, true
}

func applyRegexpStep(value any, step regexpStepPlan) (any, error) {
	text, err := runtimevalue.String(value, "regexp input")
	if err != nil {
		bytesValue, bytesErr := runtimevalue.Bytes(value, "regexp input")
		if bytesErr != nil {
			return nil, err
		}

		text = string(bytesValue)
	}

	pattern, err := regexp.Compile(step.Pattern)
	if err != nil {
		return nil, err
	}

	matches := pattern.FindStringSubmatch(text)
	if matches == nil {
		return nil, errors.New("regexp matched no text")
	}
	if step.Group >= len(matches) {
		return nil, fmt.Errorf("regexp group %d is out of range", step.Group)
	}

	return runtimevalue.PreserveSecret(value, matches[step.Group]), nil
}

func (r referenceResolver) resolveBindingFromContext(ctx context.Context, source valueLookup, binding bindingPlan) (any, error) {
	clone := r
	clone.bindingSource = source
	return clone.ResolveBindingContext(ctx, binding)
}

func stringifyBindingPart(value any, field string) (string, error) {
	wrapped := runtimevalue.Wrap(value)
	switch wrapped.Kind() {
	case runtimevalue.KindString:
		return runtimevalue.String(value, field)
	case runtimevalue.KindNumber:
		return fmt.Sprintf("%v", runtimevalue.Reveal(value)), nil
	case runtimevalue.KindBool:
		boolean, err := runtimevalue.Bool(value, field)
		if err != nil {
			return "", err
		}

		if boolean {
			return "true", nil
		}

		return "false", nil
	default:
		return "", fmt.Errorf("%s must be string, number, or bool, got %T", field, value)
	}
}

func validateJSONPointer(pointer JSONPointer) error {
	if pointer.IsRoot() {
		return nil
	}

	raw := pointer.String()
	switch {
	case strings.HasPrefix(raw, "#/"):
		return errors.New(`use RFC 6901 string form "/token/id", not URI fragment form "#/token/id"`)
	case raw == "/":
		return errors.New(`"/" selects the member with key ""; omit path for the whole value`)
	case !strings.HasPrefix(raw, "/"):
		return errors.New("path must start with /")
	}

	_, err := pointerTokens(pointer)
	return err
}

func decodePointerToken(token string) (string, error) {
	var builder strings.Builder
	for i := 0; i < len(token); i++ {
		if token[i] < 0x20 {
			return "", errors.New("path contains control characters")
		}

		if token[i] != '~' {
			builder.WriteByte(token[i])
			continue
		}

		if i+1 >= len(token) {
			return "", errors.New("path escape is truncated")
		}

		switch token[i+1] {
		case '0':
			builder.WriteByte('~')
		case '1':
			builder.WriteByte('/')
		default:
			return "", fmt.Errorf("path escape ~%c is invalid", token[i+1])
		}

		i++
	}

	return builder.String(), nil
}

func pointerTokens(pointer JSONPointer) ([]string, error) {
	if pointer.IsRoot() {
		return nil, nil
	}

	segments := strings.Split(pointer.String()[1:], "/")
	tokens := make([]string, 0, len(segments))
	for _, segment := range segments {
		token, err := decodePointerToken(segment)
		if err != nil {
			return nil, err
		}

		if token == "-" {
			return nil, errors.New(`path token "-" is not supported`)
		}

		tokens = append(tokens, token)
	}

	return tokens, nil
}
