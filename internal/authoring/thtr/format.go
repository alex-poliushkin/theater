package thtr

import (
	"os"
	"strconv"
	"strings"
)

const (
	formatIndent         = "  "
	formatMaxInlineWidth = 100
	dynamicObjectPrefix  = "object {"
	dynamicListPrefix    = "list ["
)

func Format(data []byte) ([]byte, error) {
	return formatWithSource(data, "")
}

// FormatSource formats `.thtr` source bytes and attaches sourceFile to
// structured formatter diagnostics.
func FormatSource(data []byte, sourceFile string) ([]byte, error) {
	return formatWithSource(data, sourceFile)
}

func FormatFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return formatWithSource(data, path)
}

func formatWithSource(data []byte, sourceFile string) ([]byte, error) {
	tokens, err := lex(data)
	if err != nil {
		return nil, newDiagnosticError(sourceFile, "thtr_lex_error", "", err)
	}

	document, err := parseTokens(tokens)
	if err != nil {
		return nil, newDiagnosticError(sourceFile, "thtr_parse_error", "", err)
	}

	formatter := thtrFormatter{
		comments: append([]commentSyntax(nil), document.Comments...),
	}
	formatter.writeDocument(document)
	return []byte(formatter.builder.String()), nil
}

type thtrFormatter struct {
	builder       strings.Builder
	comments      []commentSyntax
	commentCursor int
	hasOutput     bool
	lastBlank     bool
}

func (f *thtrFormatter) writeDocument(document *syntaxDocument) {
	f.writeCommentsBefore(document.Stage.Span.Start.Line)
	f.writeLine(appendTrailingComment("stage "+document.Stage.ID, f.takeTrailingComment(document.Stage.Span.Start.Line)))
	if document.Stage.Name != nil {
		f.writeNameLine(document.Stage.Name, 0)
	}

	if document.HTTP != nil {
		f.blankLine()
		f.writeCommentsBefore(document.HTTP.Span.Start.Line)
		f.writeStageSection(*document.HTTP)
	}
	if document.State != nil {
		f.blankLine()
		f.writeCommentsBefore(document.State.Span.Start.Line)
		f.writeStageSection(*document.State)
	}

	for i := range document.Scenarios {
		f.blankLine()
		f.writeCommentsBefore(document.Scenarios[i].Span.Start.Line)
		f.writeScenario(document.Scenarios[i])
	}
	for i := range document.Calls {
		f.blankLine()
		f.writeCommentsBefore(document.Calls[i].Span.Start.Line)
		f.writeScenarioCall(document.Calls[i])
	}

	f.writeRemainingComments()
}

func (f *thtrFormatter) writeStageSection(section stageSectionSyntax) {
	f.writeLine(appendTrailingComment(section.Name, f.takeTrailingComment(section.Span.Start.Line)))
	for i := range section.Entries {
		f.writeCommentsBefore(section.Entries[i].Span.Start.Line)
		f.writeMultilineText(f.renderStageSectionEntry(section.Entries[i], 1))
	}
}

func (f *thtrFormatter) writeScenario(scenario scenarioSyntax) {
	header := "scenario " + scenario.ID
	if len(scenario.Inputs) != 0 {
		header += "(" + f.renderInputs(scenario.Inputs) + ")"
	}
	f.writeLine(appendTrailingComment(header, f.takeTrailingComment(scenario.Span.Start.Line)))
	if scenario.Name != nil {
		f.writeNameLine(scenario.Name, 1)
	}

	for i := range scenario.AuthBindings {
		f.writeCommentsBefore(scenario.AuthBindings[i].Span.Start.Line)
		f.writeAuthBinding(scenario.AuthBindings[i], 1)
	}

	for i := range scenario.Preflight {
		f.writeCommentsBefore(scenario.Preflight[i].Span.Start.Line)
		f.writePreflight(scenario.Preflight[i], 1)
	}

	for i := range scenario.Acts {
		if i > 0 {
			f.blankLine()
		}
		f.writeCommentsBefore(scenario.Acts[i].Span.Start.Line)
		f.writeAct(scenario.Acts[i], 1)
	}
}

func (f *thtrFormatter) writePreflight(preflight preflightSyntax, indent int) {
	prefix := "preflight " + preflight.ID + ": "
	input := f.renderExpressionForPrefix(preflight.Input, indent, len(prefix))
	text := indented(indent, prefix) + input
	text += f.renderAssertionForPrefix(preflight.Assert, indent, lastLineWidth(text))
	if preflight.Override != nil {
		overridePrefix := " override "
		text += overridePrefix +
			f.renderExpressionForPrefix(
				preflight.Override,
				indent,
				lastLineWidth(text)+len(overridePrefix),
			)
	}
	f.writeLine(appendTrailingComment(text, f.takeTrailingComment(preflight.Span.Start.Line)))
}

func (f *thtrFormatter) writeAuthBinding(binding authBindingSyntax, indent int) {
	f.writeLine(appendTrailingComment(indented(indent, "bind auth "+binding.Auth), f.takeTrailingComment(binding.Span.Start.Line)))
	for i := range binding.Slots {
		f.writeCommentsBefore(binding.Slots[i].Span.Start.Line)
		f.writeMultilineText(f.renderBlockMappingEntry(binding.Slots[i], indent+1))
	}
}

func (f *thtrFormatter) writeAct(act actSyntax, indent int) {
	f.writeLine(appendTrailingComment(indented(indent, "act "+act.ID), f.takeTrailingComment(act.Span.Start.Line)))
	if act.Name != nil {
		f.writeNameLine(act.Name, indent+1)
	}

	if act.Eventually != nil {
		f.writeCommentsBefore(act.Eventually.Span.Start.Line)
		text := "eventually " + act.Eventually.Timeout + " every " + act.Eventually.Interval
		f.writeLine(appendTrailingComment(statementPrefix(indent+1, text), f.takeTrailingComment(act.Eventually.Span.Start.Line)))
	}
	for i := range act.Properties {
		f.writeCommentsBefore(act.Properties[i].Span.Start.Line)
		prefix := "prop " + act.Properties[i].Name + " = "
		text := prefix + f.renderExpressionForPrefix(
			act.Properties[i].Value,
			indent+1,
			len(statementPrefix(indent+1, prefix)),
		)
		f.writeMultilineText(appendTrailingComment(statementPrefix(indent+1, text), f.takeTrailingComment(act.Properties[i].Span.Start.Line)))
	}
	if act.Action != nil {
		f.writeCommentsBefore(act.Action.Span.Start.Line)
		f.writeMultilineText(f.renderAction(*act.Action, indent+1))
	}
	if act.CaptureAuth != nil {
		f.writeCommentsBefore(act.CaptureAuth.Span.Start.Line)
		f.writeMultilineText(f.renderCaptureAuth(*act.CaptureAuth, indent+1))
	}
	for i := range act.Logs {
		f.writeCommentsBefore(act.Logs[i].Span.Start.Line)
		prefix := "log " + act.Logs[i].ID + " = "
		value := f.renderExpressionForPrefix(
			act.Logs[i].Value,
			indent+1,
			len(statementPrefix(indent+1, prefix)),
		)
		text := prefix + value
		f.writeMultilineText(appendTrailingComment(statementPrefix(indent+1, text), f.takeTrailingComment(act.Logs[i].Span.Start.Line)))
	}
	for i := range act.Expectations {
		f.writeCommentsBefore(act.Expectations[i].Span.Start.Line)
		prefix := "expect " + act.Expectations[i].ID + ": "
		subject := f.renderExpressionForPrefix(
			act.Expectations[i].Subject,
			indent+1,
			len(statementPrefix(indent+1, prefix)),
		)
		text := prefix + subject
		text += f.renderAssertionForPrefix(
			act.Expectations[i].Assert,
			indent+1,
			lastLineWidth(statementPrefix(indent+1, text)),
		)
		f.writeMultilineText(appendTrailingComment(statementPrefix(indent+1, text), f.takeTrailingComment(act.Expectations[i].Span.Start.Line)))
	}
	for i := range act.Exports {
		f.writeCommentsBefore(act.Exports[i].Span.Start.Line)
		prefix := "export " + act.Exports[i].Name + " = "
		value := f.renderExpressionForPrefix(
			act.Exports[i].Value,
			indent+1,
			len(statementPrefix(indent+1, prefix)),
		)
		text := prefix + value
		if act.Exports[i].Assert != nil {
			text += f.renderAssertionForPrefix(
				*act.Exports[i].Assert,
				indent+1,
				lastLineWidth(statementPrefix(indent+1, text)),
			)
		}
		f.writeMultilineText(appendTrailingComment(statementPrefix(indent+1, text), f.takeTrailingComment(act.Exports[i].Span.Start.Line)))
	}
	for i := range act.Transitions {
		f.writeCommentsBefore(act.Transitions[i].Span.Start.Line)
		text := "on " + act.Transitions[i].Event + " -> " + act.Transitions[i].To
		f.writeLine(appendTrailingComment(statementPrefix(indent+1, text), f.takeTrailingComment(act.Transitions[i].Span.Start.Line)))
	}
}

func (f *thtrFormatter) writeScenarioCall(call scenarioCallSyntax) {
	f.writeMultilineText(f.renderScenarioCallHeader(call))
	if call.Name != nil {
		f.writeNameLine(call.Name, 1)
	}
	for i := range call.Dependencies {
		f.writeCommentsBefore(call.Dependencies[i].Span.Start.Line)
		text := "dependency " + call.Dependencies[i].CallID
		if call.Dependencies[i].When != "" {
			text += " when " + call.Dependencies[i].When
		}
		f.writeLine(appendTrailingComment(statementPrefix(1, text), f.takeTrailingComment(call.Dependencies[i].Span.Start.Line)))
	}

	for i := range call.Exports {
		f.writeCommentsBefore(call.Exports[i].Span.Start.Line)
		prefix := "export " + call.Exports[i].Name + " = "
		value := f.renderExpressionForPrefix(call.Exports[i].Value, 1, len(statementPrefix(1, prefix)))
		text := prefix + value
		if call.Exports[i].Assert != nil {
			text += f.renderAssertionForPrefix(*call.Exports[i].Assert, 1, lastLineWidth(statementPrefix(1, text)))
		}
		f.writeMultilineText(appendTrailingComment(statementPrefix(1, text), f.takeTrailingComment(call.Exports[i].Span.Start.Line)))
	}
}

func (f *thtrFormatter) renderScenarioCallHeader(call scenarioCallSyntax) string {
	prefix := "call " + call.ID + " = " + call.ScenarioID
	if !f.shouldUseMultilineCallBindings(call, len(prefix)+1) {
		header := prefix + "(" + f.renderCallBindings(call.Bindings, 0) + ")"
		return appendTrailingComment(header, f.takeTrailingComment(call.Span.Start.Line))
	}

	lines := []string{appendTrailingComment(prefix+"(", f.takeTrailingComment(call.Span.Start.Line))}
	for i := range call.Bindings {
		lines = append(lines, f.collectIndentedCommentsBefore(call.Bindings[i].Span.Start.Line, 1)...)
		prefix := call.Bindings[i].Name + ": "
		value := f.renderExpressionForPrefix(call.Bindings[i].Value, 1, len(statementPrefix(1, prefix)))
		line := statementPrefix(1, prefix+value)
		if i < len(call.Bindings)-1 {
			line += ","
		}
		line = appendTrailingComment(line, f.takeTrailingComment(call.Bindings[i].Span.Start.Line))
		lines = append(lines, line)
	}
	lines = append(lines, ")")
	return strings.Join(lines, "\n")
}

func (f *thtrFormatter) shouldUseMultilineCallBindings(call scenarioCallSyntax, prefixWidth int) bool {
	if len(call.Bindings) == 0 {
		return false
	}
	if f.hasCommentsInScenarioCallBindings(call) {
		return true
	}

	inline := f.renderCallBindings(call.Bindings, 0)
	if strings.Contains(inline, "\n") {
		return true
	}
	return prefixWidth+len(inline) >= formatMaxInlineWidth
}

func (f *thtrFormatter) hasCommentsInScenarioCallBindings(call scenarioCallSyntax) bool {
	lastBindingLine := call.Bindings[len(call.Bindings)-1].Span.Start.Line
	for i := range f.comments {
		line := f.comments[i].Span.Start.Line
		if line < call.Span.Start.Line || line > lastBindingLine {
			continue
		}
		if line > call.Span.Start.Line && commentIndentLevel(f.comments[i]) >= 1 {
			return true
		}
	}
	return false
}

func (f *thtrFormatter) renderStageSectionEntry(entry stageSectionEntrySyntax, indent int) string {
	prefix := indented(indent, entry.Kind+" "+entry.ID+" = ")
	call := f.renderCallForStatement(entry.Call, indent+1, len(prefix))
	return appendTrailingComment(prefix+call, f.takeTrailingComment(entry.Span.Start.Line))
}

func (f *thtrFormatter) renderAction(action actionSyntax, indent int) string {
	prefix := indented(indent, "do ")
	if action.Repeatable {
		prefix += "repeatable "
	}
	call := f.renderCallForStatement(action.Call, indent+1, len(prefix))
	return appendTrailingComment(prefix+call, f.takeTrailingComment(action.Span.Start.Line))
}

func (f *thtrFormatter) writeNameLine(name *nameSyntax, indent int) {
	f.writeCommentsBefore(name.Span.Start.Line)
	line := statementPrefix(indent, "name "+f.renderExpression(name.Value, indent))
	line = appendTrailingComment(line, f.takeTrailingComment(name.Span.Start.Line))
	f.writeLine(line)
}

func (f *thtrFormatter) renderCaptureAuth(captureAuth captureAuthSyntax, indent int) string {
	lines := []string{
		appendTrailingComment(indented(indent, "capture_auth "+captureAuth.Auth), f.takeTrailingComment(captureAuth.Span.Start.Line)),
	}
	for i := range captureAuth.Slots {
		lines = append(lines, f.collectIndentedCommentsBefore(captureAuth.Slots[i].Span.Start.Line, indent+1)...)
		lines = append(lines, f.renderBlockMappingEntry(captureAuth.Slots[i], indent+1))
	}
	lines = append(lines, f.collectIndentedCommentsBefore(captureAuth.Span.End.Line, indent+1)...)
	return strings.Join(lines, "\n")
}

func (f *thtrFormatter) renderCallForStatement(call callExpressionSyntax, argIndent, prefixWidth int) string {
	if !f.shouldUseBlockCall(call, argIndent, prefixWidth) {
		return f.renderCallInlineForPrefix(call, argIndent, prefixWidth)
	}

	return f.renderBlockCall(call, argIndent)
}

func (f *thtrFormatter) shouldUseBlockCall(call callExpressionSyntax, argIndent, prefixWidth int) bool {
	if len(call.Args) == 0 {
		return call.BlockForm
	}
	if hasNestedMappingArgs(call.Args) {
		return true
	}
	if f.hasCommentsWithin(call.Span) {
		return true
	}

	inline := f.renderCallInlineForPrefix(call, argIndent, prefixWidth)
	if strings.Contains(inline, "\n") {
		return true
	}

	return prefixWidth+len(inline) > formatMaxInlineWidth
}

func (f *thtrFormatter) renderCallInline(call callExpressionSyntax) string {
	return f.renderCallInlineForPrefix(call, 0, 0)
}

func (f *thtrFormatter) renderCallInlineForPrefix(call callExpressionSyntax, indent, prefixWidth int) string {
	if len(call.Clauses) != 0 {
		return f.renderSelectorWhereCall(call, 0)
	}
	if len(call.Args) == 0 {
		return call.Name + "()"
	}

	args := make([]string, 0, len(call.Args))
	argPrefixWidth := prefixWidth + len(call.Name) + 1
	for i := range call.Args {
		args = append(args, f.renderCallArgumentInlineForPrefix(call.Args[i], indent, argPrefixWidth))
		argPrefixWidth += len(args[i])
		if i < len(call.Args)-1 {
			argPrefixWidth += 2
		}
	}

	return call.Name + "(" + strings.Join(args, ", ") + ")"
}

func (f *thtrFormatter) renderSelectorWhereCall(call callExpressionSyntax, indent int) string {
	if len(call.Clauses) == 1 {
		return call.Name + " where " + f.renderRelativeClause(call.Clauses[0], indent)
	}

	lines := []string{call.Name + " where ("}
	for i := range call.Clauses {
		line := indented(indent+1, f.renderRelativeClause(call.Clauses[i], indent+1))
		if i < len(call.Clauses)-1 {
			line += ","
		}
		lines = append(lines, line)
	}
	lines = append(lines, indented(indent, ")"))
	return strings.Join(lines, "\n")
}

func (f *thtrFormatter) renderCallArgumentInlineForPrefix(argument callArgumentSyntax, indent, prefixWidth int) string {
	if len(argument.Mapping) != 0 {
		return argument.Name + ": " + f.renderInlineMapping(argument.Mapping, indent)
	}
	if argument.Name == "" {
		return f.renderExpressionForPrefix(argument.Value, indent, prefixWidth)
	}
	valuePrefix := argument.Name + ": "
	return valuePrefix + f.renderExpressionForPrefix(argument.Value, indent, prefixWidth+len(valuePrefix))
}

func (f *thtrFormatter) renderInlineMapping(entries []mappingEntrySyntax, indent int) string {
	parts := make([]string, 0, len(entries))
	for i := range entries {
		parts = append(parts, entries[i].Name+": "+f.renderExpression(entries[i].Value, indent))
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

func (f *thtrFormatter) renderBlockCall(call callExpressionSyntax, indent int) string {
	if len(call.Args) == 0 {
		return call.Name
	}

	lines := []string{call.Name}
	for i := range call.Args {
		lines = append(lines, f.collectIndentedCommentsBefore(call.Args[i].Span.Start.Line, indent)...)
		lines = append(lines, f.renderBlockArgument(call.Args[i], indent))
	}
	lines = append(lines, f.collectIndentedCommentsBefore(call.Span.End.Line, indent)...)
	return strings.Join(lines, "\n")
}

func (f *thtrFormatter) renderBlockArgument(argument callArgumentSyntax, indent int) string {
	return f.renderNamedBlockEntry(argument.Name, argument.Value, argument.Mapping, argument.Span, indent)
}

func (f *thtrFormatter) renderBlockMappingEntry(entry mappingEntrySyntax, indent int) string {
	return f.renderNamedBlockEntry(entry.Name, entry.Value, entry.Mapping, entry.Span, indent)
}

func (f *thtrFormatter) renderNamedBlockEntry(
	name string,
	value expressionSyntax,
	mapping []mappingEntrySyntax,
	span sourceSpan,
	indent int,
) string {
	prefix := strings.Repeat(formatIndent, indent) + name + ":"
	if len(mapping) == 0 {
		return appendTrailingComment(prefix+" "+f.renderExpressionForPrefix(value, indent, len(prefix)+1), f.takeTrailingComment(span.Start.Line))
	}

	lines := []string{appendTrailingComment(prefix, f.takeTrailingComment(span.Start.Line))}
	for i := range mapping {
		lines = append(lines, f.collectIndentedCommentsBefore(mapping[i].Span.Start.Line, indent+1)...)
		lines = append(lines, f.renderBlockMappingEntry(mapping[i], indent+1))
	}
	lines = append(lines, f.collectIndentedCommentsBefore(span.End.Line, indent+1)...)
	return strings.Join(lines, "\n")
}

func (f *thtrFormatter) renderInputs(inputs []inputSyntax) string {
	rendered := make([]string, 0, len(inputs))
	for i := range inputs {
		text := inputs[i].Name + ": " + inputs[i].Type
		if inputs[i].Required {
			text += "!"
		}
		rendered = append(rendered, text)
	}
	return strings.Join(rendered, ", ")
}

func (f *thtrFormatter) renderCallBindings(bindings []callArgumentSyntax, indent int) string {
	if len(bindings) == 0 {
		return ""
	}

	rendered := make([]string, 0, len(bindings))
	for i := range bindings {
		prefix := bindings[i].Name + ": "
		rendered = append(rendered, prefix+f.renderExpressionForPrefix(bindings[i].Value, indent, len(prefix)))
	}
	return strings.Join(rendered, ", ")
}

func (f *thtrFormatter) renderAssertion(assertion assertionSyntax, indent int) string {
	return f.renderAssertionForPrefix(assertion, indent, defaultPrefixWidth(indent))
}

func (f *thtrFormatter) renderAssertionForPrefix(assertion assertionSyntax, indent, prefixWidth int) string {
	negation := ""
	if assertion.NegationSpan != nil {
		negation = " not"
	}

	return negation + f.renderAssertionCoreForPrefix(assertion, indent, prefixWidth+len(negation))
}

func (f *thtrFormatter) renderAssertionCoreForPrefix(assertion assertionSyntax, indent, prefixWidth int) string {
	switch assertion.Kind {
	case assertionKindEqual:
		operator := " == "
		return operator + f.renderExpressionForPrefix(assertion.Value, indent, prefixWidth+len(operator))
	case assertionKindNotEqual:
		operator := " != "
		return operator + f.renderExpressionForPrefix(assertion.Value, indent, prefixWidth+len(operator))
	case assertionKindContains:
		operator := " contains "
		return operator + f.renderExpressionForPrefix(assertion.Value, indent, prefixWidth+len(operator))
	case assertionKindMatches:
		operator := " matches "
		return operator + f.renderExpressionForPrefix(assertion.Value, indent, prefixWidth+len(operator))
	case assertionKindPresent:
		return " is present"
	case assertionKindNull:
		return " is null"
	case assertionKindNotNull:
		return " is not null"
	case assertionKindGT:
		operator := " > "
		return operator + f.renderExpressionForPrefix(assertion.Value, indent, prefixWidth+len(operator))
	case assertionKindGTE:
		operator := " >= "
		return operator + f.renderExpressionForPrefix(assertion.Value, indent, prefixWidth+len(operator))
	case assertionKindLT:
		operator := " < "
		return operator + f.renderExpressionForPrefix(assertion.Value, indent, prefixWidth+len(operator))
	case assertionKindLTE:
		operator := " <= "
		return operator + f.renderExpressionForPrefix(assertion.Value, indent, prefixWidth+len(operator))
	case assertionKindBetween:
		operator := " between "
		minValue := f.renderExpressionForPrefix(assertion.Value, indent, prefixWidth+len(operator))
		secondOperator := " and "
		return operator + minValue +
			secondOperator + f.renderExpressionForPrefix(
			assertion.SecondValue,
			indent,
			lastLineWidth(strings.Repeat(" ", prefixWidth)+operator+minValue+secondOperator),
		)
	case assertionKindHasItem:
		return f.renderCollectionAssertion("has item", assertion.Clauses, indent)
	case assertionKindAllItems:
		return f.renderCollectionAssertion("all items", assertion.Clauses, indent)
	case assertionKindHasKey, assertionKindHasEntry, assertionKindLacksKey:
		return f.renderObjectAssertionCore(assertion, indent, prefixWidth)
	case assertionKindCall:
		call, ok := ungroupExpression(assertion.Value).(callExpressionSyntax)
		if !ok {
			return " assert " + f.renderExpression(assertion.Value, indent)
		}
		operator := " assert "
		return operator + f.renderCallInlineForPrefix(call, indent, prefixWidth+len(operator))
	default:
		return ""
	}
}

func (f *thtrFormatter) renderObjectAssertionCore(assertion assertionSyntax, indent, prefixWidth int) string {
	switch assertion.Kind {
	case assertionKindHasKey:
		return " has key(" + f.renderExpression(assertion.Value, indent) + ")"
	case assertionKindHasEntry:
		prefix := " has entry(" + f.renderExpression(assertion.Value, indent) + ")"
		return prefix + f.renderAssertionForPrefix(*assertion.Nested, indent, prefixWidth+len(prefix))
	case assertionKindLacksKey:
		return " lacks key(" + f.renderExpression(assertion.Value, indent) + ")"
	default:
		return ""
	}
}

func (f *thtrFormatter) renderCollectionAssertion(prefix string, clauses []relativeClauseSyntax, indent int) string {
	if len(clauses) == 1 {
		return " " + prefix + " where " + f.renderRelativeClause(clauses[0], indent)
	}

	lines := []string{" " + prefix + " where ("}
	for i := range clauses {
		line := indented(indent+1, f.renderRelativeClause(clauses[i], indent+1))
		if i < len(clauses)-1 {
			line += ","
		}
		lines = append(lines, line)
	}
	lines = append(lines, indented(indent, ")"))
	return strings.Join(lines, "\n")
}

func (f *thtrFormatter) renderRelativeClause(clause relativeClauseSyntax, indent int) string {
	return f.renderExpression(clause.Subject, indent) + f.renderAssertion(clause.Assert, indent)
}

func (f *thtrFormatter) renderExpression(expression expressionSyntax, indent int) string {
	return f.renderExpressionForPrefix(expression, indent, defaultPrefixWidth(indent))
}

func (f *thtrFormatter) renderExpressionForPrefix(expression expressionSyntax, indent, prefixWidth int) string {
	switch value := expression.(type) {
	case literalExpressionSyntax:
		return value.Text
	case symbolExpressionSyntax:
		return value.Name
	case refExpressionSyntax:
		return "$" + value.Name
	case callExpressionSyntax:
		return f.renderCallInlineForPrefix(value, indent, prefixWidth)
	case pipelineExpressionSyntax:
		return f.renderPipelineInline(value, indent)
	case objectExpressionSyntax:
		return f.renderObjectExpressionForPrefix(value, indent, prefixWidth)
	case listExpressionSyntax:
		return f.renderListExpressionForPrefix(value, indent, prefixWidth)
	case groupedExpressionSyntax:
		return f.renderGroupedExpression(value, indent)
	default:
		return ""
	}
}

func (f *thtrFormatter) renderPipelineInline(pipeline pipelineExpressionSyntax, indent int) string {
	if f.pipelineNeedsMultiline(pipeline) {
		return f.renderPipelineMultiline(pipeline, indent)
	}

	parts := []string{f.renderExpression(pipeline.Base, indent)}
	for i := range pipeline.Steps {
		parts = append(parts, f.renderCallInline(pipeline.Steps[i]))
	}
	return strings.Join(parts, " | ")
}

func (f *thtrFormatter) pipelineNeedsMultiline(pipeline pipelineExpressionSyntax) bool {
	for i := range pipeline.Steps {
		if strings.Contains(f.renderCallInline(pipeline.Steps[i]), "\n") {
			return true
		}
	}

	return false
}

func (f *thtrFormatter) renderPipelineMultiline(pipeline pipelineExpressionSyntax, indent int) string {
	lines := []string{"("}
	lines = append(lines, indentFirstLine(indent+1, f.renderExpression(pipeline.Base, indent+1)))
	for i := range pipeline.Steps {
		lines = append(lines, indented(indent+1, "| "+f.renderCallInline(pipeline.Steps[i])))
	}
	lines = append(lines, strings.Repeat(formatIndent, indent)+")")
	return strings.Join(lines, "\n")
}

func (f *thtrFormatter) renderObjectExpressionForPrefix(expression objectExpressionSyntax, indent, prefixWidth int) string {
	inline := f.renderObjectExpressionInline(expression, indent)
	if f.shouldUseMultilineObjectExpression(expression, inline, prefixWidth) {
		return f.renderMultilineObjectExpression(expression, indent)
	}

	return inline
}

func (f *thtrFormatter) renderObjectExpressionInline(expression objectExpressionSyntax, indent int) string {
	prefix := "{"
	if expression.Dynamic {
		prefix = dynamicObjectPrefix
	}
	if len(expression.Fields) == 0 {
		return prefix + "}"
	}

	fields := make([]string, 0, len(expression.Fields))
	for i := range expression.Fields {
		fields = append(fields, renderDataKey(expression.Fields[i].Name)+": "+f.renderExpression(expression.Fields[i].Value, indent))
	}
	return prefix + " " + strings.Join(fields, ", ") + " }"
}

func (f *thtrFormatter) renderListExpressionForPrefix(expression listExpressionSyntax, indent, prefixWidth int) string {
	inline := f.renderListExpressionInline(expression, indent)
	if f.shouldUseMultilineListExpression(expression, inline, prefixWidth) {
		return f.renderMultilineListExpression(expression, indent)
	}

	return inline
}

func (f *thtrFormatter) renderListExpressionInline(expression listExpressionSyntax, indent int) string {
	prefix := "["
	if expression.Dynamic {
		prefix = dynamicListPrefix
	}
	if len(expression.Items) == 0 {
		return prefix + "]"
	}

	items := make([]string, 0, len(expression.Items))
	for i := range expression.Items {
		items = append(items, f.renderExpression(expression.Items[i], indent))
	}
	return prefix + " " + strings.Join(items, ", ") + " ]"
}

func (f *thtrFormatter) shouldUseMultilineObjectExpression(expression objectExpressionSyntax, inline string, prefixWidth int) bool {
	if f.hasCommentsWithin(expression.Span) {
		return true
	}
	if strings.Contains(inline, "\n") {
		return true
	}
	if expression.Span.Start.Line != expression.Span.End.Line && readableMultilineObjectExpression(expression) {
		return true
	}
	return prefixWidth+len(inline) > formatMaxInlineWidth
}

func (f *thtrFormatter) shouldUseMultilineListExpression(expression listExpressionSyntax, inline string, prefixWidth int) bool {
	if f.hasCommentsWithin(expression.Span) {
		return true
	}
	if strings.Contains(inline, "\n") {
		return true
	}
	if expression.Span.Start.Line != expression.Span.End.Line && readableMultilineListExpression(expression) {
		return true
	}
	return prefixWidth+len(inline) > formatMaxInlineWidth
}

func readableMultilineObjectExpression(expression objectExpressionSyntax) bool {
	if len(expression.Fields) > 1 {
		return true
	}
	for i := range expression.Fields {
		if containerExpression(expression.Fields[i].Value) {
			return true
		}
	}
	return false
}

func readableMultilineListExpression(expression listExpressionSyntax) bool {
	if len(expression.Items) > 1 {
		return true
	}
	for i := range expression.Items {
		if containerExpression(expression.Items[i]) {
			return true
		}
	}
	return false
}

func containerExpression(expression expressionSyntax) bool {
	switch ungroupExpression(expression).(type) {
	case objectExpressionSyntax, listExpressionSyntax:
		return true
	default:
		return false
	}
}

func (f *thtrFormatter) renderMultilineObjectExpression(expression objectExpressionSyntax, indent int) string {
	prefix := "{"
	if expression.Dynamic {
		prefix = dynamicObjectPrefix
	}
	if len(expression.Fields) == 0 {
		return prefix + "}"
	}

	lines := []string{prefix}
	for i := range expression.Fields {
		lines = append(lines, f.collectIndentedCommentsBefore(expression.Fields[i].Span.Start.Line, indent+1)...)
		fieldPrefix := strings.Repeat(formatIndent, indent+1) +
			renderDataKey(expression.Fields[i].Name) + ": "
		line := fieldPrefix + f.renderExpressionForPrefix(expression.Fields[i].Value, indent+1, len(fieldPrefix))
		if i < len(expression.Fields)-1 {
			line += ","
		}
		line = appendTrailingComment(line, f.takeTrailingComment(expression.Fields[i].Span.Start.Line))
		lines = append(lines, line)
	}
	lines = append(lines, f.collectIndentedCommentsBefore(expression.Span.End.Line, indent+1)...)
	lines = append(lines, strings.Repeat(formatIndent, indent)+"}")
	return strings.Join(lines, "\n")
}

func (f *thtrFormatter) renderMultilineListExpression(expression listExpressionSyntax, indent int) string {
	prefix := "["
	if expression.Dynamic {
		prefix = dynamicListPrefix
	}
	if len(expression.Items) == 0 {
		return prefix + "]"
	}

	lines := []string{prefix}
	for i := range expression.Items {
		lines = append(lines, f.collectIndentedCommentsBefore(expression.Items[i].ExpressionSpan().Start.Line, indent+1)...)
		itemPrefix := strings.Repeat(formatIndent, indent+1)
		line := itemPrefix + f.renderExpressionForPrefix(expression.Items[i], indent+1, len(itemPrefix))
		if i < len(expression.Items)-1 {
			line += ","
		}
		line = appendTrailingComment(line, f.takeTrailingComment(expression.Items[i].ExpressionSpan().Start.Line))
		lines = append(lines, line)
	}
	lines = append(lines, f.collectIndentedCommentsBefore(expression.Span.End.Line, indent+1)...)
	lines = append(lines, strings.Repeat(formatIndent, indent)+"]")
	return strings.Join(lines, "\n")
}

func (f *thtrFormatter) renderGroupedExpression(expression groupedExpressionSyntax, indent int) string {
	pipeline, ok := ungroupExpression(expression.Inner).(pipelineExpressionSyntax)
	if !ok || len(pipeline.Steps) == 0 {
		if !f.hasCommentsWithin(expression.Span) {
			return "(" + f.renderExpression(expression.Inner, indent) + ")"
		}
		return f.renderMultilineGroupedExpression(expression, indent)
	}

	lines := []string{"("}
	lines = append(lines, f.collectIndentedCommentsBefore(pipeline.Base.ExpressionSpan().Start.Line, indent+1)...)
	baseLine := indentFirstLine(indent+1, f.renderExpression(pipeline.Base, indent+1))
	baseLine = appendTrailingCommentToFirstLine(baseLine, f.takeTrailingComment(pipeline.Base.ExpressionSpan().Start.Line))
	lines = append(lines, baseLine)
	for i := range pipeline.Steps {
		lines = append(lines, f.collectIndentedCommentsBefore(pipeline.Steps[i].Span.Start.Line, indent+1)...)
		stepLine := indented(indent+1, "| "+f.renderCallInline(pipeline.Steps[i]))
		stepLine = appendTrailingComment(stepLine, f.takeTrailingComment(pipeline.Steps[i].Span.Start.Line))
		lines = append(lines, stepLine)
	}
	lines = append(lines, f.collectIndentedCommentsBefore(expression.Span.End.Line, indent+1)...)
	lines = append(lines, strings.Repeat(formatIndent, indent)+")")
	return strings.Join(lines, "\n")
}

func (f *thtrFormatter) renderMultilineGroupedExpression(expression groupedExpressionSyntax, indent int) string {
	innerStartLine := expression.Inner.ExpressionSpan().Start.Line

	lines := []string{"("}
	lines = append(lines, f.collectIndentedCommentsBefore(innerStartLine, indent+1)...)
	innerLine := indentFirstLine(indent+1, f.renderExpression(expression.Inner, indent+1))
	innerLine = appendTrailingCommentToFirstLine(innerLine, f.takeTrailingComment(innerStartLine))
	lines = append(lines, innerLine)
	lines = append(lines, f.collectIndentedCommentsBefore(expression.Span.End.Line, indent+1)...)
	lines = append(lines, strings.Repeat(formatIndent, indent)+")")
	return strings.Join(lines, "\n")
}

func (f *thtrFormatter) writeCommentsBefore(line int) {
	for f.commentCursor < len(f.comments) && f.comments[f.commentCursor].Span.Start.Line < line {
		f.writeStandaloneComment(f.comments[f.commentCursor])
		f.commentCursor++
	}
}

func (f *thtrFormatter) writeRemainingComments() {
	for f.commentCursor < len(f.comments) {
		f.writeStandaloneComment(f.comments[f.commentCursor])
		f.commentCursor++
	}
}

func (f *thtrFormatter) writeStandaloneComment(comment commentSyntax) {
	f.writeLine(indented(commentIndentLevel(comment), comment.Text))
}

func (f *thtrFormatter) collectIndentedCommentsBefore(line, indent int) []string {
	lines := make([]string, 0, 1)
	for f.commentCursor < len(f.comments) && f.comments[f.commentCursor].Span.Start.Line < line {
		lines = append(lines, indented(indent, f.comments[f.commentCursor].Text))
		f.commentCursor++
	}
	return lines
}

func (f *thtrFormatter) takeTrailingComment(line int) string {
	if f.commentCursor >= len(f.comments) || f.comments[f.commentCursor].Span.Start.Line != line {
		return ""
	}

	parts := make([]string, 0, 1)
	for f.commentCursor < len(f.comments) && f.comments[f.commentCursor].Span.Start.Line == line {
		parts = append(parts, f.comments[f.commentCursor].Text)
		f.commentCursor++
	}
	return strings.Join(parts, " ")
}

func (f *thtrFormatter) hasCommentsWithin(span sourceSpan) bool {
	for i := range f.comments {
		if f.comments[i].Span.Start.Offset <= span.Start.Offset {
			continue
		}
		if f.comments[i].Span.Start.Offset >= span.End.Offset {
			continue
		}
		return true
	}
	return false
}

func (f *thtrFormatter) writeMultilineText(text string) {
	for _, line := range strings.Split(text, "\n") {
		f.writeLine(line)
	}
}

func renderDataKey(name string) string {
	if validBareDataKey(name) {
		return name
	}
	return strconv.Quote(name)
}

func validBareDataKey(name string) bool {
	if name == "" {
		return false
	}

	for i, r := range name {
		switch {
		case i == 0:
			if !isIdentifierStart(r) {
				return false
			}
		case isIdentifierPart(r):
			continue
		default:
			return false
		}
	}

	return true
}

func (f *thtrFormatter) writeLine(line string) {
	f.builder.WriteString(line)
	f.builder.WriteByte('\n')
	f.hasOutput = true
	f.lastBlank = line == ""
}

func (f *thtrFormatter) blankLine() {
	if !f.hasOutput || f.lastBlank {
		return
	}
	f.writeLine("")
}

func hasNestedMappingArgs(arguments []callArgumentSyntax) bool {
	for i := range arguments {
		if len(arguments[i].Mapping) != 0 {
			return true
		}
	}
	return false
}

func appendTrailingComment(text, comment string) string {
	if comment == "" {
		return text
	}

	lines := strings.Split(text, "\n")
	lines[len(lines)-1] += " " + comment
	return strings.Join(lines, "\n")
}

func appendTrailingCommentToFirstLine(text, comment string) string {
	if comment == "" {
		return text
	}

	lines := strings.Split(text, "\n")
	lines[0] += " " + comment
	return strings.Join(lines, "\n")
}

func indented(level int, text string) string {
	if level <= 0 {
		return text
	}

	prefix := strings.Repeat(formatIndent, level)
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

func statementPrefix(level int, text string) string {
	return strings.Repeat(formatIndent, level) + text
}

func defaultPrefixWidth(indent int) int {
	if indent <= 0 {
		return 0
	}
	return len(formatIndent) * indent
}

func lastLineWidth(text string) int {
	index := strings.LastIndexByte(text, '\n')
	if index == -1 {
		return len(text)
	}
	return len(text) - index - 1
}

func indentFirstLine(level int, text string) string {
	if level <= 0 {
		return text
	}

	lines := strings.Split(text, "\n")
	lines[0] = strings.Repeat(formatIndent, level) + lines[0]
	return strings.Join(lines, "\n")
}

func commentIndentLevel(comment commentSyntax) int {
	if comment.Span.Start.Column <= 1 {
		return 0
	}
	return (comment.Span.Start.Column - 1) / len(formatIndent)
}
