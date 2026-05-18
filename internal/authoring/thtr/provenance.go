package thtr

import (
	"fmt"
	"strconv"
	"strings"

	goyaml "gopkg.in/yaml.v3"

	"github.com/alex-poliushkin/theater"
)

const sourceMapVersion = "v1alpha1"

type loweredDocument struct {
	Spec      theater.StageSpec
	YAML      []byte
	SourceMap *sourceMap
}

type sourceMap struct {
	Version    string           `json:"version" yaml:"version"`
	Entries    []sourceMapEntry `json:"entries" yaml:"entries"`
	bySpecPath map[string]int
}

type sourceMapEntry struct {
	NodeID   string         `json:"node_id" yaml:"node_id"`
	SpecPath string         `json:"spec_path" yaml:"spec_path"`
	Source   sourceMapRange `json:"source" yaml:"source"`
	YAML     *yamlMapRange  `json:"yaml,omitempty" yaml:"yaml,omitempty"`
	locator  []yamlPathStep `json:"-" yaml:"-"`
}

type sourceMapRange struct {
	File        string `json:"file" yaml:"file"`
	StartLine   int    `json:"start_line" yaml:"start_line"`
	StartColumn int    `json:"start_column" yaml:"start_column"`
	EndLine     int    `json:"end_line" yaml:"end_line"`
	EndColumn   int    `json:"end_column" yaml:"end_column"`
}

type yamlMapRange struct {
	StartLine   int `json:"start_line" yaml:"start_line"`
	StartColumn int `json:"start_column" yaml:"start_column"`
	EndLine     int `json:"end_line" yaml:"end_line"`
	EndColumn   int `json:"end_column" yaml:"end_column"`
}

type yamlPathStep struct {
	Key   string
	Index int
}

type sourceMapBuilder struct {
	sourceFile string
	entries    []sourceMapEntry
	bySpecPath map[string]int
}

type sourcePathCodec struct{}

func lowerDocumentWithSourceMap(document *syntaxDocument, sourceFile string) (loweredDocument, error) {
	spec, err := lowerStageSpec(document, sourceFile)
	if err != nil {
		return loweredDocument{}, err
	}

	sourceMap, err := buildSourceMap(document, spec, sourceFile)
	if err != nil {
		return loweredDocument{}, err
	}
	applySourceMapToBindingSourceSpans(&spec, sourceMap)

	data, err := marshalCanonicalYAML(spec, sourceMap)
	if err != nil {
		return loweredDocument{}, err
	}

	return loweredDocument{
		Spec:      spec,
		YAML:      data,
		SourceMap: sourceMap,
	}, nil
}

func buildSourceMap(document *syntaxDocument, spec theater.StageSpec, sourceFile string) (*sourceMap, error) {
	builder := newSourceMapBuilder(sourceFile)
	codec := sourcePathCodec{}
	stagePath := codec.Join("stage", spec.ID)
	builder.record(stagePath, document.Stage.Span, nil)
	if document.Stage.Name != nil {
		builder.record(stagePath+"/name", document.Stage.Name.Span, yamlKeyPath("name"))
	}

	if document.HTTP != nil && spec.HTTP != nil {
		buildHTTPSourceMap(builder, stagePath, *document.HTTP)
	}
	if document.State != nil && spec.State != nil {
		buildStateSourceMap(builder, stagePath, *document.State)
	}

	stateAliases, err := sourceMapStateAliases(document.State)
	if err != nil {
		return nil, err
	}

	for i := range document.Scenarios {
		if i >= len(spec.Scenarios) {
			break
		}
		if err := buildScenarioSourceMap(builder, stagePath, document.Scenarios[i], i, stateAliases); err != nil {
			return nil, err
		}
	}

	for i := range document.Calls {
		if i >= len(spec.ScenarioCalls) {
			break
		}
		buildScenarioCallSourceMap(builder, stagePath, document.Calls[i], i)
	}

	return builder.build(), nil
}

func buildHTTPSourceMap(builder *sourceMapBuilder, stagePath string, section stageSectionSyntax) {
	builder.record(stagePath+"/http", section.Span, yamlKeyPath("http"))

	for i := range section.Entries {
		entry := section.Entries[i]
		var locator []yamlPathStep
		switch entry.Kind {
		case sectionHTTPSession:
			locator = yamlKeyPath("http", "sessions", entry.ID)
		case sectionHTTPAuth:
			locator = yamlKeyPath("http", "auth", entry.ID)
		case sectionHTTPIdentity:
			locator = yamlKeyPath("http", "identities", entry.ID)
		default:
			continue
		}
		entryPath := httpEntryPath(stagePath, entry.Kind, entry.ID)
		builder.record(entryPath, entry.Span, locator)
		if entry.Kind == sectionHTTPAuth {
			recordHTTPAuthAttachmentSourceMap(builder, entryPath, locator, entry)
		}
	}
}

func recordHTTPAuthAttachmentSourceMap(
	builder *sourceMapBuilder,
	authPath string,
	authLocator []yamlPathStep,
	entry stageSectionEntrySyntax,
) {
	attachArg, ok := findArgument(entry.Call.Args, "attach")
	if !ok {
		return
	}

	list, ok := ungroupExpression(attachArg.Value).(listExpressionSyntax)
	if !ok {
		return
	}

	attachLocator := appendYAMLPath(authLocator, yamlKey("attach"))
	for i := range list.Items {
		builder.record(
			fmt.Sprintf("%s/attach[%d]", authPath, i),
			list.Items[i].ExpressionSpan(),
			appendYAMLPath(attachLocator, yamlIndex(i)),
		)
	}
}

func buildStateSourceMap(builder *sourceMapBuilder, stagePath string, section stageSectionSyntax) {
	builder.record(stagePath+"/state", section.Span, yamlKeyPath("state"))

	for i := range section.Entries {
		entry := section.Entries[i]
		path := stateEntryPath(stagePath, entry.Kind, entry.ID)
		if entry.Kind != "backend" {
			builder.record(path, entry.Span, nil)
			continue
		}
		builder.record(
			path,
			entry.Span,
			yamlKeyPath("state", "backends", entry.ID),
		)
	}
}

func buildScenarioSourceMap(
	builder *sourceMapBuilder,
	stagePath string,
	scenario scenarioSyntax,
	index int,
	stateAliases stateAliasTable,
) error {
	codec := sourcePathCodec{}
	scenarioPath := codec.JoinChild(stagePath, "scenario", scenario.ID)
	scenarioLocator := yamlPathWithIndex("scenarios", index)
	builder.record(scenarioPath, scenario.Span, scenarioLocator)
	if scenario.Name != nil {
		builder.record(scenarioPath+"/name", scenario.Name.Span, appendYAMLPath(scenarioLocator, yamlKey("name")))
	}

	for i := range scenario.Inputs {
		input := scenario.Inputs[i]
		builder.record(
			bindingPath(scenarioPath+"/input", input.Name),
			input.Span,
			appendYAMLPath(scenarioLocator, yamlKey("inputs"), yamlKey(input.Name)),
		)
	}

	for i := range scenario.AuthBindings {
		authBinding := scenario.AuthBindings[i]
		authBindingPath := codec.JoinChild(scenarioPath, "auth_bindings", authBinding.Auth)
		authBindingLocator := appendYAMLPath(scenarioLocator, yamlKey("auth_bindings"), yamlKey(authBinding.Auth))
		builder.record(authBindingPath, authBinding.Span, authBindingLocator)
		recordAuthBindingSlots(
			builder,
			authBindingPath,
			appendYAMLPath(authBindingLocator, yamlKey("slots")),
			authBinding.Slots,
		)
	}

	for i := range scenario.Preflight {
		preflightLocator := appendYAMLPath(scenarioLocator, yamlKey("preflight"), yamlIndex(i))
		buildPreflightSourceMap(builder, scenarioPath, scenario.Preflight[i], preflightLocator)
	}

	for i := range scenario.Acts {
		actLocator := appendYAMLPath(
			scenarioLocator,
			yamlKey("acts"),
			yamlIndex(i),
		)
		if err := buildActSourceMap(builder, scenarioPath, scenario.Acts[i], actLocator, stateAliases); err != nil {
			return err
		}
	}

	return nil
}

func buildPreflightSourceMap(
	builder *sourceMapBuilder,
	scenarioPath string,
	preflight preflightSyntax,
	preflightLocator []yamlPathStep,
) {
	codec := sourcePathCodec{}
	preflightPath := codec.JoinChild(scenarioPath, "preflight", preflight.ID)
	builder.record(preflightPath, preflight.Span, preflightLocator)
	builder.record(preflightPath+"/input", preflight.Input.ExpressionSpan(), appendYAMLPath(preflightLocator, yamlKey("input")))
	builder.record(preflightPath+"/assert", preflight.Assert.Span, appendYAMLPath(preflightLocator, yamlKey("assert")))
	recordAssertionPaths(builder, preflightPath+"/assert", appendYAMLPath(preflightLocator, yamlKey("assert")), preflight.Assert)
	if preflight.Override != nil {
		builder.record(
			preflightPath+"/override",
			preflight.Override.ExpressionSpan(),
			appendYAMLPath(preflightLocator, yamlKey("override")),
		)
	}
}

func recordAuthBindingSlots(
	builder *sourceMapBuilder,
	authBindingPath string,
	slotsLocator []yamlPathStep,
	slots []mappingEntrySyntax,
) {
	codec := sourcePathCodec{}
	for i := range slots {
		slot := slots[i]
		slotPath := codec.JoinChild(authBindingPath, "slot", slot.Name)
		slotLocator := appendYAMLPath(slotsLocator, yamlKey(slot.Name))
		builder.record(slotPath, slot.Span, slotLocator)
		if len(slot.Mapping) != 0 {
			recordMappingEntries(builder, slotPath, appendYAMLPath(slotLocator, yamlKey("object")), slot.Mapping)
			continue
		}
		recordBindingExpressionChildren(builder, slotPath, slotLocator, slot.Value)
	}
}

func buildActSourceMap(
	builder *sourceMapBuilder,
	scenarioPath string,
	act actSyntax,
	actLocator []yamlPathStep,
	stateAliases stateAliasTable,
) error {
	codec := sourcePathCodec{}
	actPath := codec.JoinChild(scenarioPath, "act", act.ID)
	builder.record(actPath, act.Span, actLocator)
	if act.Name != nil {
		builder.record(actPath+"/name", act.Name.Span, appendYAMLPath(actLocator, yamlKey("name")))
	}

	if act.Eventually != nil {
		eventuallyLocator := appendYAMLPath(actLocator, yamlKey("eventually"))
		builder.record(
			actPath+"/eventually/timeout",
			act.Eventually.Span,
			appendYAMLPath(eventuallyLocator, yamlKey("timeout")),
		)
		builder.record(
			actPath+"/eventually/interval",
			act.Eventually.Span,
			appendYAMLPath(eventuallyLocator, yamlKey("interval")),
		)
	}

	for i := range act.Properties {
		propertiesLocator := appendYAMLPath(actLocator, yamlKey("properties"))
		if err := buildPropertySourceMap(builder, actPath, propertiesLocator, act.Properties[i]); err != nil {
			return err
		}
	}

	if act.Action != nil {
		actionPath := actPath + "/action"
		actionLocator := appendYAMLPath(actLocator, yamlKey("action"))
		builder.record(actionPath, act.Action.Span, actionLocator)
		canonicalCall, err := canonicalizeStateActionCall(act.Action.Call)
		if err != nil {
			return err
		}
		recordCallBindingArgs(builder, actionPath, appendYAMLPath(actionLocator, yamlKey("with")), canonicalCall.Args)
		recordHiddenStateAliasPaths(builder, actPath, *act.Action, stateAliases)
	}
	if act.CaptureAuth != nil {
		capturePath := actPath + "/capture_auth"
		captureLocator := appendYAMLPath(actLocator, yamlKey("capture_auth"))
		builder.record(capturePath, act.CaptureAuth.Span, captureLocator)
		builder.record(capturePath+"/auth", act.CaptureAuth.Span, appendYAMLPath(captureLocator, yamlKey("auth")))
		for i := range act.CaptureAuth.Slots {
			slotPath := joinChildPath(capturePath, "slot", act.CaptureAuth.Slots[i].Name)
			slotLocator := appendYAMLPath(captureLocator, yamlKey("slots"), yamlKey(act.CaptureAuth.Slots[i].Name))
			builder.record(slotPath, act.CaptureAuth.Slots[i].Span, slotLocator)
		}
	}

	for i := range act.Logs {
		logLocator := appendYAMLPath(actLocator, yamlKey("logs"), yamlIndex(i))
		buildActLogSourceMap(builder, actPath, act.Logs[i], logLocator)
	}

	expectationIndex := 0
	for i := range act.Expectations {
		expectationLocator := appendYAMLPath(
			actLocator,
			yamlKey("expectations"),
			yamlIndex(expectationIndex),
		)
		if err := buildExpectationSourceMap(builder, actPath, act.Expectations[i], expectationLocator); err != nil {
			return err
		}
		expectationIndex++
	}
	for i := range act.Exports {
		if act.Exports[i].Assert == nil {
			continue
		}
		expectationLocator := appendYAMLPath(
			actLocator,
			yamlKey("expectations"),
			yamlIndex(expectationIndex),
		)
		buildExportAssertionSourceMap(builder, actPath, act.Exports[i], expectationLocator)
		expectationIndex++
	}

	for i := range act.Exports {
		buildActExportSourceMap(builder, actPath, act.Exports[i], appendYAMLPath(actLocator, yamlKey("exports"), yamlIndex(i)))
	}

	for i := range act.Transitions {
		transitionPath := fmt.Sprintf("%s/transition[%d]", actPath, i)
		transitionLocator := appendYAMLPath(actLocator, yamlKey("transitions"), yamlIndex(i))
		builder.record(transitionPath, act.Transitions[i].Span, transitionLocator)
		builder.record(transitionPath+"/on", act.Transitions[i].Span, appendYAMLPath(transitionLocator, yamlKey("on")))
		builder.record(transitionPath+"/to", act.Transitions[i].Span, appendYAMLPath(transitionLocator, yamlKey("to")))
	}

	return nil
}

func sourceMapStateAliases(section *stageSectionSyntax) (stateAliasTable, error) {
	if section == nil {
		return stateAliasTable{}, nil
	}

	aliases := stateAliasTable{}
	for i := range section.Entries {
		entry := section.Entries[i]
		if entry.Kind != sectionStateRecord && entry.Kind != sectionStatePool {
			continue
		}
		alias, err := lowerStateAlias(entry)
		if err != nil {
			return nil, err
		}
		aliases[entry.ID] = alias
	}
	if len(aliases) == 0 {
		return stateAliasTable{}, nil
	}
	return aliases, nil
}

func recordHiddenStateAliasPaths(
	builder *sourceMapBuilder,
	actPath string,
	action actionSyntax,
	stateAliases stateAliasTable,
) {
	if len(stateAliases) == 0 {
		return
	}

	requirements := stateAliasActionArgKinds(action.Call.Name)
	for i := range action.Call.Args {
		arg := action.Call.Args[i]
		kind, ok := requirements[arg.Name]
		if !ok || len(arg.Mapping) != 0 {
			continue
		}
		symbol, ok := ungroupExpression(arg.Value).(symbolExpressionSyntax)
		if !ok {
			continue
		}
		alias, ok := stateAliases[symbol.Name]
		if !ok || alias.kind != kind {
			continue
		}
		recordHiddenStateAliasProperty(builder, actPath, symbol.Name, alias)
	}
}

func recordHiddenStateAliasProperty(
	builder *sourceMapBuilder,
	actPath string,
	aliasName string,
	alias stateAliasSpec,
) {
	propertyPath := joinChildPath(actPath, "property", hiddenStateAliasRef(alias.kind, aliasName))
	builder.record(propertyPath, alias.span, nil)

	inventoryPath := propertyPath + "/inventory"
	builder.record(inventoryPath, alias.span, nil)

	for key, span := range alias.argSpans {
		builder.record(bindingPath(inventoryPath+"/with", key), span, nil)
	}
}

func buildPropertySourceMap(
	builder *sourceMapBuilder,
	actPath string,
	propertiesLocator []yamlPathStep,
	property propertySyntax,
) error {
	codec := sourcePathCodec{}
	propertyPath := codec.JoinChild(actPath, "property", property.Name)
	propertyLocator := appendYAMLPath(propertiesLocator, yamlKey(property.Name))
	builder.record(propertyPath, property.Span, propertyLocator)

	base, decorators := splitPropertyPipeline(property.Value)

	if baseCall, ok := ungroupExpression(base).(callExpressionSyntax); ok && strings.HasPrefix(baseCall.Name, "inventory.") {
		inventoryPath := propertyPath + "/inventory"
		inventoryLocator := appendYAMLPath(propertyLocator, yamlKey("inventory"))
		builder.record(inventoryPath, baseCall.Span, inventoryLocator)
		recordCallBindingArgs(builder, inventoryPath+"/with", appendYAMLPath(inventoryLocator, yamlKey("with")), baseCall.Args)
	} else {
		valuePath := propertyPath + "/value"
		valueLocator := appendYAMLPath(propertyLocator, yamlKey("value"))
		builder.record(valuePath, base.ExpressionSpan(), valueLocator)
		recordBindingExpressionChildren(builder, valuePath, valueLocator, base)
	}

	for i := range decorators {
		decoratorPath := joinChildPath(propertyPath, "decorator", decoratorKey(decorators[i].Name, i))
		decoratorLocator := appendYAMLPath(propertyLocator, yamlKey("decorators"), yamlIndex(i))
		builder.record(decoratorPath, decorators[i].Span, decoratorLocator)
		recordDecoratorArgs(builder, decoratorPath, decoratorLocator, decorators[i].Args)
	}

	return nil
}

func buildActLogSourceMap(
	builder *sourceMapBuilder,
	actPath string,
	log logSyntax,
	logLocator []yamlPathStep,
) {
	logPath := joinChildPath(actPath, "log", log.ID)
	builder.record(logPath, log.Span, logLocator)

	valuePath := logPath + "/value"
	valueLocator := appendYAMLPath(logLocator, yamlKey("value"))
	builder.record(valuePath, log.Value.ExpressionSpan(), valueLocator)
	recordLogValuePaths(builder, valuePath, valueLocator, log.Value)
}

func buildExpectationSourceMap(
	builder *sourceMapBuilder,
	actPath string,
	expectation expectationSyntax,
	expectationLocator []yamlPathStep,
) error {
	expectationPath := joinChildPath(actPath, "expectation", expectation.ID)
	builder.record(expectationPath, expectation.Span, expectationLocator)

	subjectPath := expectationPath + "/subject"
	subjectLocator := appendYAMLPath(expectationLocator, yamlKey("subject"))
	builder.record(subjectPath, expectation.Subject.ExpressionSpan(), subjectLocator)
	recordSelectorPaths(builder, subjectPath, subjectLocator, expectation.Subject)

	assertPath := expectationPath + "/assert"
	assertLocator := appendYAMLPath(expectationLocator, yamlKey("assert"))
	builder.record(assertPath, expectation.Assert.Span, assertLocator)
	recordAssertionPaths(builder, assertPath, assertLocator, expectation.Assert)

	return nil
}

func buildActExportSourceMap(
	builder *sourceMapBuilder,
	actPath string,
	export exportSyntax,
	exportLocator []yamlPathStep,
) {
	exportPath := exportPath(actPath, export.Name)
	builder.record(exportPath, export.Span, exportLocator)
	recordSelectorPaths(builder, exportPath, exportLocator, export.Value)
}

func buildExportAssertionSourceMap(
	builder *sourceMapBuilder,
	actPath string,
	export exportSyntax,
	expectationLocator []yamlPathStep,
) {
	expectationPath := joinChildPath(actPath, "expectation", export.Name)
	builder.record(expectationPath, export.Assert.Span, expectationLocator)

	subjectPath := expectationPath + "/subject"
	subjectLocator := appendYAMLPath(expectationLocator, yamlKey("subject"))
	builder.record(subjectPath, export.Value.ExpressionSpan(), subjectLocator)
	recordSelectorPaths(builder, subjectPath, subjectLocator, export.Value)

	assertPath := expectationPath + "/assert"
	assertLocator := appendYAMLPath(expectationLocator, yamlKey("assert"))
	builder.record(assertPath, export.Assert.Span, assertLocator)
	recordAssertionPaths(builder, assertPath, assertLocator, *export.Assert)
}

func buildScenarioCallSourceMap(builder *sourceMapBuilder, stagePath string, call scenarioCallSyntax, index int) {
	codec := sourcePathCodec{}
	callPath := codec.JoinChild(stagePath, "call", call.ID)
	callLocator := yamlPathWithIndex("scenario_calls", index)
	builder.record(callPath, call.Span, callLocator)
	if call.Name != nil {
		builder.record(callPath+"/name", call.Name.Span, appendYAMLPath(callLocator, yamlKey("name")))
	}
	recordCallBindingArgs(builder, callPath, appendYAMLPath(callLocator, yamlKey("bindings")), call.Bindings)
	for i := range call.Dependencies {
		dependencyPath := fmt.Sprintf("%s/dependency[%d]", callPath, i)
		dependencyLocator := appendYAMLPath(callLocator, yamlKey("dependencies"), yamlIndex(i))
		builder.record(dependencyPath, call.Dependencies[i].Span, dependencyLocator)
	}

	for i := range call.Exports {
		exportPath := exportPath(callPath, call.Exports[i].Name)
		exportLocator := appendYAMLPath(callLocator, yamlKey("exports"), yamlIndex(i))
		builder.record(exportPath, call.Exports[i].Span, exportLocator)
		recordSelectorPaths(builder, exportPath, exportLocator, call.Exports[i].Value)
	}
}

func recordAssertionPaths(
	builder *sourceMapBuilder,
	assertPath string,
	assertLocator []yamlPathStep,
	assertion assertionSyntax,
) {
	if assertion.NegationSpan != nil || assertion.Kind == assertionKindNotEqual {
		recordNegatedAssertionPaths(builder, assertPath, assertLocator, assertion)
		return
	}

	recordAssertionCorePaths(builder, assertPath, appendYAMLPath(assertLocator, yamlKey("args")), assertion)
}

func recordAssertionCorePaths(
	builder *sourceMapBuilder,
	argsPath string,
	argsLocator []yamlPathStep,
	assertion assertionSyntax,
) {
	switch assertion.Kind {
	case assertionKindEqual, assertionKindNotEqual, assertionKindContains,
		assertionKindGT, assertionKindGTE, assertionKindLT, assertionKindLTE:
		recordAssertionArgBinding(builder, argsPath, argsLocator, "expected", assertion.Value)
	case assertionKindBetween:
		recordAssertionArgBinding(builder, argsPath, argsLocator, "min", assertion.Value)
		recordAssertionArgBinding(builder, argsPath, argsLocator, "max", assertion.SecondValue)
	case assertionKindMatches:
		recordAssertionArgBinding(builder, argsPath, argsLocator, "pattern", assertion.Value)
	case assertionKindHasItem, assertionKindAllItems:
		recordCollectionClausePaths(builder, argsPath, argsLocator, assertion.WhereSpan, assertion.Clauses)
	case assertionKindHasKey:
		recordAssertionArgBinding(builder, argsPath, argsLocator, "key", assertion.Value)
	case assertionKindHasEntry:
		recordAssertionArgBinding(builder, argsPath, argsLocator, "key", assertion.Value)
		recordNestedAssertionArgBinding(builder, argsPath, argsLocator, "assert", *assertion.Nested)
	case assertionKindLacksKey:
		recordAssertionArgBinding(builder, argsPath, argsLocator, "key", assertion.Value)
	case assertionKindCall:
		call, ok := ungroupExpression(assertion.Value).(callExpressionSyntax)
		if !ok {
			return
		}
		recordCallBindingArgs(builder, argsPath, argsLocator, call.Args)
	}
}

func recordNegatedAssertionPaths(
	builder *sourceMapBuilder,
	assertPath string,
	assertLocator []yamlPathStep,
	assertion assertionSyntax,
) {
	assertArgPath := bindingPath(assertPath, "assert")
	assertArgLocator := appendYAMLPath(assertLocator, yamlKey("args"), yamlKey("assert"))
	coreSpan := assertionCoreSpan(assertion)
	builder.record(assertArgPath, coreSpan, assertArgLocator)

	argsPath := bindingPath(assertArgPath, "args")
	argsLocator := appendYAMLPath(assertArgLocator, yamlKey("object"), yamlKey("args"))
	builder.record(argsPath, coreSpan, argsLocator)

	recordAssertionCorePaths(builder, argsPath, appendYAMLPath(argsLocator, yamlKey("object")), assertion)
}

func assertionCoreSpan(assertion assertionSyntax) sourceSpan {
	switch assertion.Kind {
	case assertionKindEqual, assertionKindNotEqual, assertionKindContains,
		assertionKindMatches, assertionKindGT, assertionKindGTE,
		assertionKindLT, assertionKindLTE, assertionKindHasKey,
		assertionKindLacksKey:
		return assertion.Value.ExpressionSpan()
	case assertionKindBetween:
		return combineSourceSpans(assertion.Value.ExpressionSpan(), assertion.SecondValue.ExpressionSpan())
	case assertionKindHasItem, assertionKindAllItems:
		return combineSourceSpans(assertion.WhereSpan, assertion.Span)
	case assertionKindHasEntry:
		return sourceSpan{
			Start: assertion.WhereSpan.Start,
			End:   assertion.Span.End,
		}
	case assertionKindCall:
		return assertion.Value.ExpressionSpan()
	default:
		return assertion.Span
	}
}

func recordNestedAssertionArgBinding(
	builder *sourceMapBuilder,
	argsPath string,
	argsLocator []yamlPathStep,
	name string,
	assertion assertionSyntax,
) {
	path := bindingPath(argsPath, name)
	locator := appendYAMLPath(argsLocator, yamlKey(name))
	builder.record(path, assertion.Span, locator)

	nestedArgsPath := bindingPath(path, "args")
	nestedArgsLocator := appendYAMLPath(locator, yamlKey("object"), yamlKey("args"))
	builder.record(nestedArgsPath, assertion.Span, nestedArgsLocator)
	recordAssertionPaths(builder, path, appendYAMLPath(locator, yamlKey("object")), assertion)
}

func recordAssertionArgBinding(
	builder *sourceMapBuilder,
	argsPath string,
	argsLocator []yamlPathStep,
	name string,
	expr expressionSyntax,
) {
	path := bindingPath(argsPath, name)
	locator := appendYAMLPath(argsLocator, yamlKey(name))
	builder.record(path, expr.ExpressionSpan(), locator)
	recordBindingExpressionChildren(builder, path, locator, expr)
}

func recordCollectionClausePaths(
	builder *sourceMapBuilder,
	argsPath string,
	argsLocator []yamlPathStep,
	whereSpan sourceSpan,
	clauses []relativeClauseSyntax,
) {
	wherePath := bindingPath(argsPath, "where")
	whereLocator := appendYAMLPath(argsLocator, yamlKey("where"))
	if whereSpan == (sourceSpan{}) {
		builder.record(wherePath, sourceSpan{}, whereLocator)
	} else {
		builder.record(wherePath, whereSpan, whereLocator)
	}

	for i := range clauses {
		itemPath := bindingPath(wherePath, clauseBindingKey(i))
		itemLocator := appendYAMLPath(whereLocator, yamlKey("list"), yamlIndex(i))
		builder.record(itemPath, clauses[i].Span, itemLocator)
		recordRelativeClausePaths(builder, itemPath, itemLocator, clauses[i])
	}
}

func recordRelativeClausePaths(
	builder *sourceMapBuilder,
	clausePath string,
	clauseLocator []yamlPathStep,
	clause relativeClauseSyntax,
) {
	subjectPath := bindingPath(clausePath, "subject")
	subjectLocator := appendYAMLPath(clauseLocator, yamlKey("object"), yamlKey("subject"))
	builder.record(subjectPath, clause.Subject.ExpressionSpan(), subjectLocator)
	recordRelativeClauseSubjectPaths(builder, subjectPath, subjectLocator, clause.Subject)

	assertPath := bindingPath(clausePath, "assert")
	assertLocator := appendYAMLPath(clauseLocator, yamlKey("object"), yamlKey("assert"))
	builder.record(assertPath, clause.Assert.Span, assertLocator)
	recordAssertionPaths(builder, assertPath, assertLocator, clause.Assert)
}

func recordRelativeClauseSubjectPaths(
	builder *sourceMapBuilder,
	subjectPath string,
	subjectLocator []yamlPathStep,
	expr expressionSyntax,
) {
	switch value := ungroupExpression(expr).(type) {
	case callExpressionSyntax:
		recordRelativeClauseSubjectSteps(builder, subjectPath, subjectLocator, []callExpressionSyntax{value})
	case pipelineExpressionSyntax:
		base, ok := ungroupExpression(value.Base).(callExpressionSyntax)
		if !ok {
			return
		}
		steps := make([]callExpressionSyntax, 0, len(value.Steps)+1)
		steps = append(steps, base)
		steps = append(steps, value.Steps...)
		recordRelativeClauseSubjectSteps(builder, subjectPath, subjectLocator, steps)
	case groupedExpressionSyntax:
		recordRelativeClauseSubjectPaths(builder, subjectPath, subjectLocator, value.Inner)
	}
}

func recordRelativeClauseSubjectSteps(
	builder *sourceMapBuilder,
	subjectPath string,
	subjectLocator []yamlPathStep,
	steps []callExpressionSyntax,
) {
	for i := range steps {
		switch steps[i].Name {
		case selectorStepDecode:
			path := bindingPath(subjectPath, "decode")
			locator := appendYAMLPath(subjectLocator, yamlKey("object"), yamlKey("decode"))
			builder.record(path, steps[i].Span, locator)
		case selectorStepPath:
			path := bindingPath(subjectPath, "path")
			locator := appendYAMLPath(subjectLocator, yamlKey("object"), yamlKey("path"))
			builder.record(path, steps[i].Span, locator)
		}
	}
}

func recordSelectorPaths(
	builder *sourceMapBuilder,
	path string,
	locator []yamlPathStep,
	expr expressionSyntax,
) {
	switch value := ungroupExpression(expr).(type) {
	case callExpressionSyntax:
		builder.record(path, value.Span, locator)
	case refExpressionSyntax:
		builder.record(path, value.Span, locator)
	case pipelineExpressionSyntax:
		builder.record(path, value.Span, locator)
		recordSelectorStepPaths(builder, path, locator, value.Steps)
	case groupedExpressionSyntax:
		recordSelectorPaths(builder, path, locator, value.Inner)
	}
}

func recordLogValuePaths(
	builder *sourceMapBuilder,
	path string,
	locator []yamlPathStep,
	expr expressionSyntax,
) {
	switch value := ungroupExpression(expr).(type) {
	case callExpressionSyntax, refExpressionSyntax, pipelineExpressionSyntax:
		recordSelectorPaths(builder, path, locator, value)
	case objectExpressionSyntax:
		builder.record(path, value.Span, locator)
		objectLocator := appendYAMLPath(locator, yamlKey("object"))
		for i := range value.Fields {
			childPath := bindingChildPath(path, value.Fields[i].Name)
			childLocator := appendYAMLPath(objectLocator, yamlKey(value.Fields[i].Name))
			builder.record(childPath, value.Fields[i].Span, childLocator)
			recordLogValuePaths(builder, childPath, childLocator, value.Fields[i].Value)
		}
	case listExpressionSyntax:
		builder.record(path, value.Span, locator)
		listLocator := appendYAMLPath(locator, yamlKey("list"))
		for i := range value.Items {
			childPath := fmt.Sprintf("%s[%d]", path, i)
			childLocator := appendYAMLPath(listLocator, yamlIndex(i))
			builder.record(childPath, value.Items[i].ExpressionSpan(), childLocator)
			recordLogValuePaths(builder, childPath, childLocator, value.Items[i])
		}
	case groupedExpressionSyntax:
		recordLogValuePaths(builder, path, locator, value.Inner)
	}
}

func recordSelectorStepPaths(
	builder *sourceMapBuilder,
	path string,
	locator []yamlPathStep,
	steps []callExpressionSyntax,
) {
	rootPathSeen := false
	throughIndex := 0

	for i := range steps {
		step := steps[i]
		switch step.Name {
		case "decode":
			continue
		case "path":
			if !rootPathSeen && throughIndex == 0 {
				rootPathSeen = true
				continue
			}
			throughPath := fmt.Sprintf("%s/through[%d]", path, throughIndex)
			throughLocator := appendYAMLPath(locator, yamlKey("through"), yamlIndex(throughIndex))
			builder.record(throughPath, step.Span, throughLocator)
			throughIndex++
		case "pick":
			throughPath := fmt.Sprintf("%s/through[%d]", path, throughIndex)
			throughLocator := appendYAMLPath(locator, yamlKey("through"), yamlIndex(throughIndex))
			builder.record(throughPath, step.Span, throughLocator)
			if len(step.Clauses) != 0 {
				recordPickWhereClausePaths(builder, throughPath+"/pick", appendYAMLPath(throughLocator, yamlKey("pick")), step.WhereSpan, step.Clauses)
				throughIndex++
				continue
			}
			equalsArg, ok := findArgument(step.Args, "equals")
			if !ok {
				throughIndex++
				continue
			}
			equalsPath := fmt.Sprintf("%s/through[%d]/pick/equals", path, throughIndex)
			equalsLocator := appendYAMLPath(throughLocator, yamlKey("pick"), yamlKey("equals"))
			builder.record(equalsPath, equalsArg.Span, equalsLocator)
			recordArgumentBindingChildren(builder, equalsPath, equalsLocator, equalsArg)
			throughIndex++
		case "regexp":
			throughPath := fmt.Sprintf("%s/through[%d]", path, throughIndex)
			throughLocator := appendYAMLPath(locator, yamlKey("through"), yamlIndex(throughIndex))
			builder.record(throughPath, step.Span, throughLocator)
			throughIndex++
		default:
			if strings.HasPrefix(step.Name, selectorStepTransformPrefix) {
				throughPath := fmt.Sprintf("%s/through[%d]", path, throughIndex)
				throughLocator := appendYAMLPath(locator, yamlKey("through"), yamlIndex(throughIndex))
				builder.record(throughPath, step.Span, throughLocator)
				recordDecoratorArgs(builder, throughPath+"/transform", appendYAMLPath(throughLocator, yamlKey("transform")), step.Args)
			}
			throughIndex++
		}
	}
}

func recordCallBindingArgs(
	builder *sourceMapBuilder,
	parentPath string,
	parentLocator []yamlPathStep,
	args []callArgumentSyntax,
) {
	for i := range args {
		arg := args[i]
		if arg.Name == "" {
			continue
		}
		path := bindingPath(parentPath, arg.Name)
		locator := appendYAMLPath(parentLocator, yamlKey(arg.Name))
		builder.record(path, arg.Span, locator)
		recordArgumentBindingChildren(builder, path, locator, arg)
	}
}

func recordDecoratorArgs(
	builder *sourceMapBuilder,
	decoratorPath string,
	decoratorLocator []yamlPathStep,
	args []callArgumentSyntax,
) {
	for i := range args {
		arg := args[i]
		if arg.Name == "" {
			continue
		}
		path := bindingPath(decoratorPath+"/with", arg.Name)
		locator := appendYAMLPath(decoratorLocator, yamlKey("with"), yamlKey(arg.Name))
		builder.record(path, arg.Span, locator)
		recordArgumentBindingChildren(builder, path, locator, arg)
	}
}

func recordArgumentBindingChildren(
	builder *sourceMapBuilder,
	path string,
	locator []yamlPathStep,
	arg callArgumentSyntax,
) {
	if len(arg.Mapping) != 0 {
		recordMappingEntries(builder, path, appendYAMLPath(locator, yamlKey("object")), arg.Mapping)
		return
	}

	recordBindingExpressionChildren(builder, path, locator, arg.Value)
}

func recordMappingEntries(
	builder *sourceMapBuilder,
	path string,
	locator []yamlPathStep,
	entries []mappingEntrySyntax,
) {
	for i := range entries {
		entry := entries[i]
		childPath := bindingChildPath(path, entry.Name)
		childLocator := appendYAMLPath(locator, yamlKey(entry.Name))
		builder.record(childPath, entry.Span, childLocator)
		if len(entry.Mapping) != 0 {
			recordMappingEntries(builder, childPath, appendYAMLPath(childLocator, yamlKey("object")), entry.Mapping)
			continue
		}
		recordBindingExpressionChildren(builder, childPath, childLocator, entry.Value)
	}
}

func recordPickWhereClausePaths(
	builder *sourceMapBuilder,
	pickPath string,
	pickLocator []yamlPathStep,
	whereSpan sourceSpan,
	clauses []relativeClauseSyntax,
) {
	wherePath := pickPath + "/where"
	whereLocator := appendYAMLPath(pickLocator, yamlKey("where"))
	builder.record(wherePath, whereSpan, whereLocator)

	for i := range clauses {
		clausePath := fmt.Sprintf("%s[%d]", wherePath, i)
		clauseLocator := appendYAMLPath(whereLocator, yamlIndex(i))
		builder.record(clausePath, clauses[i].Span, clauseLocator)

		subjectPath := clausePath + "/subject"
		subjectLocator := appendYAMLPath(clauseLocator, yamlKey("subject"))
		builder.record(subjectPath, clauses[i].Subject.ExpressionSpan(), subjectLocator)
		recordPickWhereSubjectPaths(builder, subjectPath, subjectLocator, clauses[i].Subject)

		assertPath := clausePath + "/assert"
		assertLocator := appendYAMLPath(clauseLocator, yamlKey("assert"))
		builder.record(assertPath, clauses[i].Assert.Span, assertLocator)
		recordAssertionPaths(builder, assertPath, assertLocator, clauses[i].Assert)
	}
}

func recordPickWhereSubjectPaths(
	builder *sourceMapBuilder,
	subjectPath string,
	subjectLocator []yamlPathStep,
	expr expressionSyntax,
) {
	switch value := ungroupExpression(expr).(type) {
	case callExpressionSyntax:
		recordPickWhereSubjectSteps(builder, subjectPath, subjectLocator, []callExpressionSyntax{value})
	case pipelineExpressionSyntax:
		base, ok := ungroupExpression(value.Base).(callExpressionSyntax)
		if !ok {
			return
		}
		steps := make([]callExpressionSyntax, 0, len(value.Steps)+1)
		steps = append(steps, base)
		steps = append(steps, value.Steps...)
		recordPickWhereSubjectSteps(builder, subjectPath, subjectLocator, steps)
	case groupedExpressionSyntax:
		recordPickWhereSubjectPaths(builder, subjectPath, subjectLocator, value.Inner)
	}
}

func recordPickWhereSubjectSteps(
	builder *sourceMapBuilder,
	subjectPath string,
	subjectLocator []yamlPathStep,
	steps []callExpressionSyntax,
) {
	for i := range steps {
		switch steps[i].Name {
		case selectorStepDecode:
			builder.record(subjectPath+"/decode", steps[i].Span, appendYAMLPath(subjectLocator, yamlKey("decode")))
		case selectorStepPath:
			builder.record(subjectPath+"/path", steps[i].Span, appendYAMLPath(subjectLocator, yamlKey("path")))
		}
	}
}

func recordBindingExpressionChildren(
	builder *sourceMapBuilder,
	path string,
	locator []yamlPathStep,
	expr expressionSyntax,
) {
	switch value := ungroupExpression(expr).(type) {
	case objectExpressionSyntax:
		recordMappingEntries(builder, path, appendYAMLPath(locator, yamlKey("object")), value.Fields)
	case listExpressionSyntax:
		for i := range value.Items {
			itemPath := fmt.Sprintf("%s[%d]", path, i)
			itemLocator := appendYAMLPath(locator, yamlKey("list"), yamlIndex(i))
			builder.record(itemPath, value.Items[i].ExpressionSpan(), itemLocator)
			recordBindingExpressionChildren(builder, itemPath, itemLocator, value.Items[i])
		}
	case callExpressionSyntax:
		switch {
		case value.Name == "string":
			for i := range value.Args {
				partPath := fmt.Sprintf("%s.parts[%d]", path, i)
				partLocator := appendYAMLPath(locator, yamlKey("parts"), yamlIndex(i))
				builder.record(partPath, value.Args[i].Span, partLocator)
				recordBindingExpressionChildren(builder, partPath, partLocator, value.Args[i].Value)
			}
		case strings.HasPrefix(value.Name, "generate."):
			for i := range value.Args {
				arg := value.Args[i]
				if arg.Name == "" {
					continue
				}
				childPath := bindingChildPath(path, arg.Name)
				childLocator := appendYAMLPath(locator, yamlKey("args"), yamlKey(arg.Name))
				builder.record(childPath, arg.Span, childLocator)
				recordArgumentBindingChildren(builder, childPath, childLocator, arg)
			}
		case value.Name == "coalesce":
			for i := range value.Args {
				candidatePath := fmt.Sprintf("%s.candidates[%d]", path, i)
				candidateLocator := appendYAMLPath(locator, yamlKey("candidates"), yamlIndex(i))
				builder.record(candidatePath, value.Args[i].Span, candidateLocator)
				recordBindingExpressionChildren(builder, candidatePath, candidateLocator, value.Args[i].Value)
			}
		}
	case pipelineExpressionSyntax:
		recordSelectorPaths(builder, path, locator, value)
	case groupedExpressionSyntax:
		recordBindingExpressionChildren(builder, path, locator, value.Inner)
	}
}

func marshalCanonicalYAML(spec theater.StageSpec, sourceMap *sourceMap) ([]byte, error) {
	data, err := goyaml.Marshal(spec)
	if err != nil {
		return nil, err
	}

	if sourceMap == nil {
		return data, nil
	}

	var document goyaml.Node
	if err := goyaml.Unmarshal(data, &document); err != nil {
		return nil, err
	}

	sourceMap.attachYAMLRanges(&document, data)
	return data, nil
}

func newSourceMapBuilder(sourceFile string) *sourceMapBuilder {
	return &sourceMapBuilder{
		sourceFile: sourceFile,
		entries:    make([]sourceMapEntry, 0, 32),
		bySpecPath: make(map[string]int, 32),
	}
}

func (b *sourceMapBuilder) record(specPath string, span sourceSpan, locator []yamlPathStep) {
	if _, exists := b.bySpecPath[specPath]; exists {
		return
	}

	b.bySpecPath[specPath] = len(b.entries)
	b.entries = append(b.entries, sourceMapEntry{
		NodeID:   specPath,
		SpecPath: specPath,
		Source: sourceMapRange{
			File:        b.sourceFile,
			StartLine:   span.Start.Line,
			StartColumn: span.Start.Column,
			EndLine:     span.End.Line,
			EndColumn:   span.End.Column,
		},
		locator: cloneYAMLPath(locator),
	})
}

func (b *sourceMapBuilder) build() *sourceMap {
	index := make(map[string]int, len(b.bySpecPath))
	for key, value := range b.bySpecPath {
		index[key] = value
	}

	return &sourceMap{
		Version:    sourceMapVersion,
		Entries:    append([]sourceMapEntry(nil), b.entries...),
		bySpecPath: index,
	}
}

func (m *sourceMap) LookupSpecPath(path string) (sourceMapEntry, bool) {
	for candidate := path; candidate != ""; candidate = fallbackSpecPath(candidate) {
		index, ok := m.bySpecPath[candidate]
		if !ok {
			continue
		}
		return m.Entries[index], true
	}

	return sourceMapEntry{}, false
}

func (m *sourceMap) LookupExactSpecPath(path string) (sourceMapEntry, bool) {
	if m == nil {
		return sourceMapEntry{}, false
	}

	index, ok := m.bySpecPath[path]
	if !ok {
		return sourceMapEntry{}, false
	}

	return m.Entries[index], true
}

func (m *sourceMap) LookupYAMLPosition(line, column int) (sourceMapEntry, bool) {
	best := -1
	for i := range m.Entries {
		entry := m.Entries[i]
		if entry.YAML == nil || !yamlRangeContains(*entry.YAML, line, column) {
			continue
		}
		if best == -1 || yamlRangeSpan(*entry.YAML) < yamlRangeSpan(*m.Entries[best].YAML) {
			best = i
		}
	}
	if best == -1 {
		return sourceMapEntry{}, false
	}
	return m.Entries[best], true
}

func (m *sourceMap) attachYAMLRanges(document *goyaml.Node, data []byte) {
	lines := strings.Split(string(data), "\n")

	for i := range m.Entries {
		node := resolveYAMLNode(document, m.Entries[i].locator)
		if node == nil {
			continue
		}
		m.Entries[i].YAML = yamlNodeRange(node, lines)
	}
}

func resolveYAMLNode(document *goyaml.Node, locator []yamlPathStep) *goyaml.Node {
	if document == nil {
		return nil
	}

	current := document
	if current.Kind == goyaml.DocumentNode {
		if len(current.Content) == 0 {
			return nil
		}
		current = current.Content[0]
	}

	for i := range locator {
		step := locator[i]
		switch {
		case step.Key != "":
			current = mappingValueNode(current, step.Key)
		case step.Index >= 0:
			current = sequenceItemNode(current, step.Index)
		default:
			return nil
		}
		if current == nil {
			return nil
		}
	}

	return current
}

func mappingValueNode(node *goyaml.Node, key string) *goyaml.Node {
	if node == nil || node.Kind != goyaml.MappingNode {
		return nil
	}

	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}

	return nil
}

func sequenceItemNode(node *goyaml.Node, index int) *goyaml.Node {
	if node == nil || node.Kind != goyaml.SequenceNode {
		return nil
	}
	if index < 0 || index >= len(node.Content) {
		return nil
	}

	return node.Content[index]
}

func yamlNodeRange(node *goyaml.Node, lines []string) *yamlMapRange {
	if node == nil || node.Line == 0 {
		return nil
	}

	endLine, endColumn := yamlNodeEnd(node, lines)
	return &yamlMapRange{
		StartLine:   node.Line,
		StartColumn: node.Column,
		EndLine:     endLine,
		EndColumn:   endColumn,
	}
}

func yamlNodeEnd(node *goyaml.Node, lines []string) (endLine, endColumn int) {
	if node == nil {
		return 0, 0
	}
	if len(node.Content) == 0 {
		return yamlScalarNodeEnd(node, lines)
	}

	last := node.Content[len(node.Content)-1]
	return yamlNodeEnd(last, lines)
}

func yamlScalarNodeEnd(node *goyaml.Node, lines []string) (endLine, endColumn int) {
	if node.Style == goyaml.LiteralStyle || node.Style == goyaml.FoldedStyle {
		return yamlBlockScalarNodeEnd(node, lines)
	}

	valueLines := strings.Split(node.Value, "\n")
	if len(valueLines) == 0 {
		return node.Line, node.Column
	}

	lastWidth := len(valueLines[len(valueLines)-1])
	if lastWidth == 0 {
		lastWidth = 1
	}

	if len(valueLines) == 1 {
		return node.Line, node.Column + lastWidth - 1
	}

	return node.Line + len(valueLines) - 1, node.Column + lastWidth - 1
}

func yamlBlockScalarNodeEnd(node *goyaml.Node, lines []string) (endLine, endColumn int) {
	valueLines := strings.Split(node.Value, "\n")
	if len(valueLines) > 0 && valueLines[len(valueLines)-1] == "" {
		valueLines = valueLines[:len(valueLines)-1]
	}
	if len(valueLines) == 0 {
		return node.Line, node.Column
	}

	endLine = node.Line + len(valueLines)
	if endLine <= 0 {
		return node.Line, node.Column
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}

	endColumn = len(lines[endLine-1])
	if endColumn == 0 {
		endColumn = 1
	}

	return endLine, endColumn
}

func fallbackSpecPath(path string) string {
	if path == "" {
		return ""
	}

	if index := strings.LastIndex(path, "["); index != -1 && strings.HasSuffix(path, "]") {
		return path[:index]
	}

	lastSlash := strings.LastIndex(path, "/")
	if lastSlash == -1 {
		if index := strings.LastIndex(path, "."); index != -1 {
			return path[:index]
		}
		return ""
	}

	segment := path[lastSlash+1:]
	if index := strings.LastIndex(segment, "."); index != -1 {
		return path[:lastSlash+1] + segment[:index]
	}

	return path[:lastSlash]
}

func yamlRangeContains(r yamlMapRange, line, column int) bool {
	if line < r.StartLine || line > r.EndLine {
		return false
	}
	if line == r.StartLine && column < r.StartColumn {
		return false
	}
	if line == r.EndLine && column > r.EndColumn {
		return false
	}
	return true
}

func yamlRangeSpan(r yamlMapRange) int {
	return (r.EndLine-r.StartLine)*10_000 + (r.EndColumn - r.StartColumn)
}

func appendYAMLPath(base []yamlPathStep, steps ...yamlPathStep) []yamlPathStep {
	result := make([]yamlPathStep, 0, len(base)+len(steps))
	result = append(result, base...)
	result = append(result, steps...)
	return result
}

func cloneYAMLPath(path []yamlPathStep) []yamlPathStep {
	if len(path) == 0 {
		return nil
	}
	cloned := make([]yamlPathStep, len(path))
	copy(cloned, path)
	return cloned
}

func yamlKeyPath(keys ...string) []yamlPathStep {
	path := make([]yamlPathStep, 0, len(keys))
	for i := range keys {
		path = append(path, yamlKey(keys[i]))
	}
	return path
}

func yamlPathWithIndex(key string, index int) []yamlPathStep {
	return []yamlPathStep{yamlKey(key), yamlIndex(index)}
}

func yamlKey(key string) yamlPathStep {
	return yamlPathStep{Key: key, Index: -1}
}

func yamlIndex(index int) yamlPathStep {
	return yamlPathStep{Index: index}
}

func findArgument(args []callArgumentSyntax, name string) (callArgumentSyntax, bool) {
	for i := range args {
		if args[i].Name == name {
			return args[i], true
		}
	}
	return callArgumentSyntax{}, false
}

func bindingPath(parentPath, key string) string {
	if key == "" {
		return parentPath + "/binding"
	}
	return joinChildPath(parentPath, "binding", key)
}

func bindingChildPath(parentPath, key string) string {
	if key == "" {
		return parentPath
	}
	return parentPath + "." + key
}

func clauseBindingKey(index int) string {
	return "item-" + strconv.Itoa(index)
}

func exportPath(parentPath, alias string) string {
	if alias == "" {
		return parentPath + "/export"
	}
	return joinChildPath(parentPath, "export", alias)
}

func decoratorKey(use string, index int) string {
	if use != "" {
		return use
	}
	return strconv.Itoa(index)
}

func httpEntryPath(stagePath, kind, id string) string {
	return stagePath + "/http/" + sourcePathCodec{}.Join(kind, id)
}

func stateEntryPath(stagePath, kind, id string) string {
	return stagePath + "/state/" + sourcePathCodec{}.Join(kind, id)
}

func joinChildPath(parentPath, kind, id string) string {
	return sourcePathCodec{}.JoinChild(parentPath, kind, id)
}

func (sourcePathCodec) Join(kind, id string) string {
	return kind + "." + escapeSourcePathID(id)
}

func (sourcePathCodec) JoinChild(parentPath, kind, id string) string {
	return parentPath + "/" + kind + "." + escapeSourcePathID(id)
}

func escapeSourcePathID(id string) string {
	var builder strings.Builder
	builder.Grow(len(id))

	for i := 0; i < len(id); i++ {
		b := id[i]
		switch {
		case b == '~':
			builder.WriteString("~0")
		case b == '/':
			builder.WriteString("~1")
		case b == '.':
			builder.WriteString("~2")
		case b < 0x20 || b == 0x7f:
			builder.WriteString(fmt.Sprintf("~x%02X", b))
		default:
			builder.WriteByte(b)
		}
	}

	return builder.String()
}
