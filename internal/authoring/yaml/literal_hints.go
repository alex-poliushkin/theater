package yaml

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	goyaml "gopkg.in/yaml.v3"

	"github.com/alex-poliushkin/theater"
)

const redundantLiteralWrapperCode = "redundant_literal_wrapper"

func LiteralWrapperHintsFile(path string) ([]theater.Diagnostic, error) {
	return literalWrapperHintsFile(path, literalWrapperHintOptions{})
}

func LiteralWrapperHintsForLocation(
	location FlowFileLocation,
	matchers theater.MatcherSugarResolver,
) ([]theater.Diagnostic, error) {
	hints, err := literalWrapperHintsFile(location.Path, literalWrapperHintOptions{matchers: matchers})
	if err != nil {
		return nil, err
	}
	if !location.InFlowRoot {
		return hints, nil
	}

	flowSpec, err := loadFlowStage(location.Path, matchers)
	if err != nil {
		return nil, err
	}
	neededScenarioIDs := unresolvedFlowScenarioIDs(flowSpec)
	if len(neededScenarioIDs) == 0 {
		return hints, nil
	}

	libraryFiles, err := collectLibraryFiles(location.Layout.LibraryRoot)
	if err != nil {
		return nil, err
	}
	index, err := buildFlowLibraryIndex(libraryFiles)
	if err != nil {
		return nil, err
	}
	selectedFiles, err := selectFlowLibraryFiles(index, neededScenarioIDs)
	if err != nil {
		return nil, err
	}

	scenarioIDsByFile := make(map[string]map[string]struct{})
	for scenarioID := range neededScenarioIDs {
		for _, libraryFile := range index.byScenarioID[scenarioID] {
			if scenarioIDsByFile[libraryFile] == nil {
				scenarioIDsByFile[libraryFile] = make(map[string]struct{})
			}
			scenarioIDsByFile[libraryFile][scenarioID] = struct{}{}
		}
	}

	for _, libraryFile := range selectedFiles {
		libraryHints, err := literalWrapperHintsFile(libraryFile, literalWrapperHintOptions{
			matchers:        matchers,
			scenarioIDs:     scenarioIDsByFile[libraryFile],
			stageIDOverride: flowSpec.ID,
		})
		if err != nil {
			return nil, err
		}
		hints = append(hints, libraryHints...)
	}
	sortDiagnostics(hints)

	return hints, nil
}

func LiteralWrapperHints(reader io.Reader, sourceFile string) ([]theater.Diagnostic, error) {
	return literalWrapperHints(reader, sourceFile, literalWrapperHintOptions{})
}

type literalWrapperHintOptions struct {
	matchers        theater.MatcherSugarResolver
	scenarioIDs     map[string]struct{}
	stageIDOverride string
}

type literalWrapperHintWalker struct {
	sourceFile      string
	matchers        theater.MatcherSugarResolver
	scenarioIDs     map[string]struct{}
	stageIDOverride string
	hints           []theater.Diagnostic
}

func literalWrapperHintsFile(path string, options literalWrapperHintOptions) ([]theater.Diagnostic, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return literalWrapperHints(file, path, options)
}

func literalWrapperHints(
	reader io.Reader,
	sourceFile string,
	options literalWrapperHintOptions,
) ([]theater.Diagnostic, error) {
	var document goyaml.Node
	if err := goyaml.NewDecoder(reader).Decode(&document); err != nil {
		return nil, err
	}
	if len(document.Content) == 0 {
		return nil, nil
	}

	walker := literalWrapperHintWalker{
		sourceFile:      sourceFile,
		matchers:        options.matchers,
		scenarioIDs:     options.scenarioIDs,
		stageIDOverride: options.stageIDOverride,
		hints:           make([]theater.Diagnostic, 0),
	}
	hints := walker.walkStage(document.Content[0])
	sortDiagnostics(hints)

	return hints, nil
}

func (w *literalWrapperHintWalker) walkStage(stage *goyaml.Node) []theater.Diagnostic {
	stageID := scalarValue(mappingValue(stage, "id"))
	if w.stageIDOverride != "" {
		stageID = w.stageIDOverride
	}
	stagePath := yamlRuntimePath("stage", stageID)

	w.walkScenarios(mappingValue(stage, "scenarios"), stagePath)
	if w.scenarioIDs == nil {
		w.walkScenarioCalls(mappingValue(stage, "scenario_calls"), stagePath)
	}

	return w.hints
}

func (w *literalWrapperHintWalker) walkScenarios(scenarios *goyaml.Node, stagePath string) {
	if scenarios == nil || scenarios.Kind != goyaml.SequenceNode {
		return
	}

	for i := range scenarios.Content {
		scenario := scenarios.Content[i]
		scenarioID := scalarValue(mappingValue(scenario, "id"))
		if w.scenarioIDs != nil {
			if _, ok := w.scenarioIDs[scenarioID]; !ok {
				continue
			}
		}
		scenarioPath := yamlRuntimeChildPath(stagePath, "scenario", scenarioID)
		w.walkActs(mappingValue(scenario, "acts"), scenarioPath)
	}
}

func (w *literalWrapperHintWalker) walkActs(acts *goyaml.Node, scenarioPath string) {
	if acts == nil || acts.Kind != goyaml.SequenceNode {
		return
	}

	for i := range acts.Content {
		act := acts.Content[i]
		actID := scalarValue(mappingValue(act, "id"))
		actPath := yamlRuntimeChildPath(scenarioPath, "act", actID)

		action := mappingValue(act, "action")
		w.walkBindingMap(mappingValue(action, "with"), actPath+"/action")

		w.walkProperties(mappingValue(act, "properties"), actPath)
		w.walkExpectations(mappingValue(act, "expectations"), actPath)
		w.walkExports(mappingValue(act, "exports"), actPath)
	}
}

func (w *literalWrapperHintWalker) walkProperties(properties *goyaml.Node, actPath string) {
	if properties == nil || properties.Kind != goyaml.MappingNode {
		return
	}

	for key, value := range mappingNodePairs(properties) {
		propertyPath := yamlRuntimeChildPath(actPath, "property", key)
		inventory := mappingValue(value, "inventory")
		w.walkBindingMap(mappingValue(inventory, "with"), propertyPath+"/inventory/with")
	}
}

func (w *literalWrapperHintWalker) walkExpectations(expectations *goyaml.Node, actPath string) {
	if expectations == nil || expectations.Kind != goyaml.SequenceNode {
		return
	}

	for i := range expectations.Content {
		expectation := expectations.Content[i]
		expectationID := scalarValue(mappingValue(expectation, "id"))
		expectationPath := yamlRuntimeChildPath(actPath, "expectation", expectationID)

		w.walkThrough(mappingValue(mappingValue(expectation, "subject"), "through"), expectationPath+"/subject")
		w.walkAssert(mappingValue(expectation, "assert"), expectationPath+"/assert")
	}
}

func (w *literalWrapperHintWalker) walkScenarioCalls(calls *goyaml.Node, stagePath string) {
	if calls == nil || calls.Kind != goyaml.SequenceNode {
		return
	}

	for i := range calls.Content {
		call := calls.Content[i]
		callID := scalarValue(mappingValue(call, "id"))
		callPath := yamlRuntimeChildPath(stagePath, "call", callID)

		w.walkBindingMap(mappingValue(call, "bindings"), callPath)
		w.walkExports(mappingValue(call, "exports"), callPath)
	}
}

func (w *literalWrapperHintWalker) walkExports(exports *goyaml.Node, parentPath string) {
	if exports == nil || exports.Kind != goyaml.SequenceNode {
		return
	}

	for i := range exports.Content {
		export := exports.Content[i]
		alias := scalarValue(mappingValue(export, "as"))
		exportPath := yamlExportPath(parentPath, alias)
		w.walkRef(mappingValue(export, "ref"), exportPath)
		w.walkThrough(mappingValue(export, "through"), exportPath)
	}
}

func (w *literalWrapperHintWalker) walkAssert(assert *goyaml.Node, assertPath string) {
	if assert == nil || assert.Kind != goyaml.MappingNode {
		return
	}

	if args := mappingValue(assert, "args"); args != nil {
		w.walkBindingMap(args, assertPath)
		return
	}
	if mappingValue(assert, "ref") != nil {
		return
	}

	pairs := mappingNodePairList(assert)
	if len(pairs) != 1 {
		return
	}
	w.walkMatcherSugar(pairs[0].key.Value, pairs[0].value, assertPath)
}

func (w *literalWrapperHintWalker) walkBindingMap(bindings *goyaml.Node, parentPath string) {
	if bindings == nil || bindings.Kind != goyaml.MappingNode {
		return
	}

	for key, value := range mappingNodePairs(bindings) {
		w.walkBinding(value, yamlBindingPath(parentPath, key))
	}
}

func (w *literalWrapperHintWalker) walkBinding(binding *goyaml.Node, path string) {
	if binding == nil || binding.Kind != goyaml.MappingNode {
		return
	}

	if isRedundantLiteralWrapper(binding) {
		w.hints = append(w.hints, theater.Diagnostic{
			Code:     redundantLiteralWrapperCode,
			Path:     path,
			Severity: theater.SeverityHint,
			Summary:  "literal wrapper is unnecessary here; use the value directly",
			Span: theater.SourceRef{
				File:   w.sourceFile,
				Line:   binding.Line,
				Column: binding.Column,
			},
		})
		return
	}

	if !looksLikeYAMLBindingSpec(binding) {
		return
	}

	w.walkRef(mappingValue(binding, "ref"), path)
	w.walkBindingMap(mappingValue(binding, "object"), path)
	w.walkBindingList(mappingValue(binding, "list"), path, yamlListBindingKey)
	w.walkBindingList(mappingValue(binding, "parts"), path, yamlPartBindingKey)

	if scalarValue(mappingValue(binding, "kind")) == string(theater.BindingKindGenerate) {
		for key, value := range mappingNodePairs(binding) {
			if key == "kind" || key == "generator" {
				continue
			}
			w.walkBinding(value, yamlBindingPath(path, key))
		}
	}
}

func (w *literalWrapperHintWalker) walkBindingList(list *goyaml.Node, parentPath string, key func(int) string) {
	if list == nil || list.Kind != goyaml.SequenceNode {
		return
	}

	for i := range list.Content {
		w.walkBinding(list.Content[i], yamlBindingPath(parentPath, key(i)))
	}
}

func (w *literalWrapperHintWalker) walkRef(ref *goyaml.Node, parentPath string) {
	if ref == nil || ref.Kind != goyaml.MappingNode {
		return
	}

	w.walkThrough(mappingValue(ref, "through"), parentPath)
}

func (w *literalWrapperHintWalker) walkThrough(through *goyaml.Node, parentPath string) {
	if through == nil || through.Kind != goyaml.SequenceNode {
		return
	}

	for i := range through.Content {
		stepPath := yamlRuntimeChildPath(parentPath, "through", strconv.Itoa(i))
		pick := mappingValue(through.Content[i], "pick")
		if pick == nil {
			continue
		}

		w.walkBinding(mappingValue(pick, "equals"), yamlBindingPath(stepPath, "equals"))
		w.walkPickWhere(mappingValue(pick, "where"), stepPath+"/pick")
	}
}

func (w *literalWrapperHintWalker) walkPickWhere(where *goyaml.Node, pickPath string) {
	if where == nil || where.Kind != goyaml.SequenceNode {
		return
	}

	for i := range where.Content {
		clausePath := yamlRuntimeChildPath(pickPath, "where", strconv.Itoa(i))
		w.walkAssert(mappingValue(where.Content[i], "assert"), clausePath+"/assert")
	}
}

func (w *literalWrapperHintWalker) walkMatcherSugar(key string, value *goyaml.Node, assertPath string) {
	if dependencyMissing(w.matchers) {
		w.walkUnknownMatcherSugar(value, assertPath)
		return
	}

	descriptor, err := w.matchers.ResolveSugarKey(key)
	if err != nil {
		w.walkUnknownMatcherSugar(value, assertPath)
		return
	}

	switch descriptor.Sugar.Form {
	case theater.SugarFormUnary:
		argName := key
		if len(descriptor.Sugar.PositionalArgs) != 0 {
			argName = descriptor.Sugar.PositionalArgs[0]
		}
		w.walkBinding(value, yamlBindingPath(assertPath, argName))
	case theater.SugarFormFixedTuple:
		w.walkFixedTupleMatcherSugar(value, descriptor.Sugar.PositionalArgs, assertPath)
	case theater.SugarFormNone:
		return
	default:
		w.walkUnknownMatcherSugar(value, assertPath)
	}
}

func (w *literalWrapperHintWalker) walkFixedTupleMatcherSugar(
	value *goyaml.Node,
	argNames []string,
	assertPath string,
) {
	switch {
	case value == nil:
		return
	case value.Kind == goyaml.SequenceNode:
		for i := range value.Content {
			argName := yamlListBindingKey(i)
			if i < len(argNames) {
				argName = argNames[i]
			}
			w.walkBinding(value.Content[i], yamlBindingPath(assertPath, argName))
		}
	case value.Kind == goyaml.MappingNode:
		w.walkBindingMap(value, assertPath)
	}
}

func (w *literalWrapperHintWalker) walkUnknownMatcherSugar(value *goyaml.Node, assertPath string) {
	switch {
	case value == nil:
		return
	case value.Kind == goyaml.SequenceNode:
		w.walkBindingList(value, assertPath, yamlListBindingKey)
	case value.Kind == goyaml.MappingNode && (isRedundantLiteralWrapper(value) || looksLikeYAMLBindingSpec(value)):
		w.walkBinding(value, yamlBindingPath(assertPath, "value"))
	case value.Kind == goyaml.MappingNode:
		w.walkBindingMap(value, assertPath)
	}
}

func isRedundantLiteralWrapper(node *goyaml.Node) bool {
	if scalarValue(mappingValue(node, "kind")) != string(theater.BindingKindLiteral) {
		return false
	}

	value := mappingValue(node, "value")
	return value != nil && !literalWrapperRequired(value)
}

func literalWrapperRequired(value *goyaml.Node) bool {
	if value == nil || value.Kind != goyaml.MappingNode {
		return false
	}

	for key := range mappingNodePairs(value) {
		if yamlBindingReservedKeys[key] {
			return true
		}
	}

	return false
}

func looksLikeYAMLBindingSpec(node *goyaml.Node) bool {
	if node == nil || node.Kind != goyaml.MappingNode {
		return false
	}

	for key := range mappingNodePairs(node) {
		if key == yamlFieldArgs {
			return false
		}
	}

	for key := range mappingNodePairs(node) {
		if yamlBindingShapeKeys[key] {
			return true
		}
	}

	return false
}

func mappingValue(node *goyaml.Node, key string) *goyaml.Node {
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

func mappingNodePairs(node *goyaml.Node) map[string]*goyaml.Node {
	if node == nil || node.Kind != goyaml.MappingNode {
		return nil
	}

	pairs := make(map[string]*goyaml.Node, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		pairs[node.Content[i].Value] = node.Content[i+1]
	}

	return pairs
}

func mappingNodePairList(node *goyaml.Node) []mappingPair {
	pairs, err := mappingPairs(node)
	if err != nil {
		return nil
	}

	return pairs
}

func scalarValue(node *goyaml.Node) string {
	if node == nil || node.Kind != goyaml.ScalarNode {
		return ""
	}

	return node.Value
}

func yamlRuntimePath(kind, id string) string {
	return kind + "." + escapeYAMLRuntimePathID(id)
}

func yamlRuntimeChildPath(parentPath, kind, id string) string {
	return parentPath + "/" + yamlRuntimePath(kind, id)
}

func yamlBindingPath(parentPath, key string) string {
	if key == "" {
		return parentPath + "/binding"
	}

	return yamlRuntimeChildPath(parentPath, "binding", key)
}

func yamlExportPath(parentPath, alias string) string {
	if alias == "" {
		return parentPath + "/export"
	}

	return yamlRuntimeChildPath(parentPath, "export", alias)
}

func yamlListBindingKey(index int) string {
	return "item-" + strconv.Itoa(index)
}

func yamlPartBindingKey(index int) string {
	return "part-" + strconv.Itoa(index)
}

func escapeYAMLRuntimePathID(id string) string {
	var builder strings.Builder
	builder.Grow(len(id))

	for i := 0; i < len(id); i++ {
		switch b := id[i]; {
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

func sortDiagnostics(diagnostics []theater.Diagnostic) {
	sort.SliceStable(diagnostics, func(i, j int) bool {
		if diagnostics[i].Path != diagnostics[j].Path {
			return diagnostics[i].Path < diagnostics[j].Path
		}
		if diagnostics[i].Span.File != diagnostics[j].Span.File {
			return diagnostics[i].Span.File < diagnostics[j].Span.File
		}
		if diagnostics[i].Span.Line != diagnostics[j].Span.Line {
			return diagnostics[i].Span.Line < diagnostics[j].Span.Line
		}
		if diagnostics[i].Span.Column != diagnostics[j].Span.Column {
			return diagnostics[i].Span.Column < diagnostics[j].Span.Column
		}
		return diagnostics[i].Code < diagnostics[j].Code
	})
}

var yamlBindingReservedKeys = map[string]bool{
	yamlFieldKind:       true,
	yamlFieldValue:      true,
	yamlFieldRef:        true,
	yamlFieldObject:     true,
	yamlFieldList:       true,
	yamlFieldGenerator:  true,
	yamlFieldParts:      true,
	yamlFieldCandidates: true,
}

var yamlBindingShapeKeys = map[string]bool{
	yamlFieldKind:       true,
	yamlFieldValue:      true,
	yamlFieldRef:        true,
	yamlFieldObject:     true,
	yamlFieldList:       true,
	yamlFieldGenerator:  true,
	yamlFieldParts:      true,
	yamlFieldCandidates: true,
}
