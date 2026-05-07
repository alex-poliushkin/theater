package thtr

import (
	"fmt"
	"strconv"
)

const expectedAssertionMessage = "expected ==, !=, matches, contains, >, >=, <, <=, " +
	"between ... and ..., has key(...), has no key(...), has entry(...) ..., lacks key(...), has item where ..., " +
	"all items where ..., is null, is not null, is present, or assert"

const expectedActEntryMessage = "expected name, eventually, prop, do, capture_auth, log, expect, export, or on"

const expectedActEntryOrderMessage = "act entries must follow name, eventually, prop, do, capture_auth, log, expect, export, on order"

const missingAssertionMessage = `is missing is not supported because missing selector targets fail before matcher evaluation; ` +
	`use lacks key(...) on the containing object to assert an absent object member`

const absentAssertionMessage = `is absent is not supported because missing selector targets fail before matcher evaluation; ` +
	`use lacks key(...) on the containing object to assert an absent object member`

const notPresentAssertionMessage = `is not present is not supported because missing selector targets fail before matcher evaluation; ` +
	`use lacks key(...) on the containing object to assert an absent object member`

var allowedInputTypes = map[string]struct{}{
	"any":    {},
	"bytes":  {},
	"string": {},
	"number": {},
	"bool":   {},
	"object": {},
	"list":   {},
	"null":   {},
}

var allowedTransitionEvents = map[string]struct{}{
	"pass":    {},
	"fail":    {},
	"timeout": {},
	"cancel":  {},
}

var allowedDependencyPredicates = map[string]struct{}{
	"success": {},
	"failure": {},
	"done":    {},
}

type parserError struct {
	span    sourceSpan
	message string
}

func (e *parserError) Error() string {
	return e.message
}

func (e *parserError) Span() sourceSpan {
	return e.span
}

func parseTokens(tokens []token) (*syntaxDocument, error) {
	p := parser{tokens: tokens}
	return p.parseDocument()
}

type parser struct {
	tokens   []token
	pos      int
	comments []commentSyntax
}

type actSection int

const (
	actSectionName actSection = iota
	actSectionEventually
	actSectionProperty
	actSectionAction
	actSectionCaptureAuth
	actSectionLog
	actSectionExpectation
	actSectionExport
	actSectionTransition
)

type callSection int

const (
	callSectionName callSection = iota
	callSectionDependency
	callSectionExport
)

func (p *parser) parseDocument() (*syntaxDocument, error) {
	p.skipIgnorable()
	stage, err := p.parseStage()
	if err != nil {
		return nil, err
	}

	document := &syntaxDocument{
		Stage: stage,
		Span:  stage.Span,
	}

	p.skipIgnorable()
	if p.atKeyword("name") {
		name, err := p.parseName()
		if err != nil {
			return nil, err
		}
		document.Stage.Name = &name
		document.Span.End = name.Span.End
	}

	p.skipIgnorable()
	if p.atKeyword("http") {
		section, err := p.parseStageSection("http")
		if err != nil {
			return nil, err
		}
		document.HTTP = &section
		document.Span.End = section.Span.End
	}

	p.skipIgnorable()
	if p.atKeyword("state") {
		section, err := p.parseStageSection("state")
		if err != nil {
			return nil, err
		}
		document.State = &section
		document.Span.End = section.Span.End
	}

	seenCall := false
	for {
		p.skipIgnorable()
		switch {
		case p.at(tokenEOF):
			document.Span.End = p.peek().Span.End
			document.Comments = append([]commentSyntax(nil), p.comments...)
			return document, nil
		case p.atKeyword("scenario"):
			if seenCall {
				return nil, &parserError{
					span:    p.peek().Span,
					message: "scenario blocks must appear before call blocks",
				}
			}
			scenario, err := p.parseScenario()
			if err != nil {
				return nil, err
			}
			document.Scenarios = append(document.Scenarios, scenario)
			document.Span.End = scenario.Span.End
		case p.atKeyword("call"):
			seenCall = true
			call, err := p.parseScenarioCall()
			if err != nil {
				return nil, err
			}
			document.Calls = append(document.Calls, call)
			document.Span.End = call.Span.End
		default:
			return nil, p.errorAtCurrent("expected scenario, call, or end of file")
		}
	}
}

func (p *parser) parseStageSection(name string) (stageSectionSyntax, error) {
	start, err := p.expectKeyword(name)
	if err != nil {
		return stageSectionSyntax{}, err
	}

	section := stageSectionSyntax{
		Name: name,
		Span: start.Span,
	}
	if err := p.expectBlockStart(); err != nil {
		return stageSectionSyntax{}, err
	}
	for !p.at(tokenDedent) && !p.at(tokenEOF) {
		p.skipIgnorable()
		if p.at(tokenDedent) || p.at(tokenEOF) {
			break
		}
		entry, err := p.parseStageSectionEntry()
		if err != nil {
			return stageSectionSyntax{}, err
		}
		section.Entries = append(section.Entries, entry)
		section.Span.End = entry.Span.End
	}
	if _, err := p.expect(tokenDedent); err != nil {
		return stageSectionSyntax{}, err
	}
	return section, nil
}

func (p *parser) parseStageSectionEntry() (stageSectionEntrySyntax, error) {
	kind, kindSpan, err := p.parseLocalIdentifier()
	if err != nil {
		return stageSectionEntrySyntax{}, err
	}
	id, _, err := p.parseLocalIdentifier()
	if err != nil {
		return stageSectionEntrySyntax{}, err
	}
	if _, err := p.expect(tokenEqual); err != nil {
		return stageSectionEntrySyntax{}, err
	}
	call, err := p.parseCallExpression(true)
	if err != nil {
		return stageSectionEntrySyntax{}, err
	}
	if !call.BlockForm {
		if err := p.expectLineEnd(); err != nil {
			return stageSectionEntrySyntax{}, err
		}
	}
	return stageSectionEntrySyntax{
		Kind: kind,
		ID:   id,
		Call: call,
		Span: sourceSpan{Start: kindSpan.Start, End: call.Span.End},
	}, nil
}

func (p *parser) parseStage() (stageSyntax, error) {
	start, err := p.expectKeyword("stage")
	if err != nil {
		return stageSyntax{}, err
	}
	id, span, err := p.parseLocalIdentifier()
	if err != nil {
		return stageSyntax{}, err
	}
	if err := p.expectLineEnd(); err != nil {
		return stageSyntax{}, err
	}
	return stageSyntax{
		ID:   id,
		Span: sourceSpan{Start: start.Span.Start, End: span.End},
	}, nil
}

func (p *parser) parseScenario() (scenarioSyntax, error) {
	start, err := p.expectKeyword("scenario")
	if err != nil {
		return scenarioSyntax{}, err
	}
	id, idSpan, err := p.parseSlashName()
	if err != nil {
		return scenarioSyntax{}, err
	}

	scenario := scenarioSyntax{
		ID:   id,
		Span: sourceSpan{Start: start.Span.Start, End: idSpan.End},
	}

	if p.match(tokenLParen) {
		inputs, end, err := p.parseInputList()
		if err != nil {
			return scenarioSyntax{}, err
		}
		scenario.Inputs = inputs
		scenario.Span.End = end.End
	}

	if err := p.expectBlockStart(); err != nil {
		return scenarioSyntax{}, err
	}
	seenActs := false
	for !p.at(tokenDedent) && !p.at(tokenEOF) {
		p.skipIgnorable()
		if p.at(tokenDedent) || p.at(tokenEOF) {
			break
		}
		if p.atKeyword("name") {
			if scenario.Name != nil {
				return scenarioSyntax{}, &parserError{
					span:    p.peek().Span,
					message: "scenario already declares name",
				}
			}
			if seenActs {
				return scenarioSyntax{}, &parserError{
					span:    p.peek().Span,
					message: "scenario entries must follow name, act order",
				}
			}
			name, err := p.parseName()
			if err != nil {
				return scenarioSyntax{}, err
			}
			scenario.Name = &name
			scenario.Span.End = name.Span.End
			continue
		}

		seenActs = true
		act, err := p.parseAct()
		if err != nil {
			return scenarioSyntax{}, err
		}
		scenario.Acts = append(scenario.Acts, act)
		scenario.Span.End = act.Span.End
	}
	if _, err := p.expect(tokenDedent); err != nil {
		return scenarioSyntax{}, err
	}
	if len(scenario.Acts) == 0 {
		return scenarioSyntax{}, &parserError{
			span:    scenario.Span,
			message: "scenario must declare at least one act",
		}
	}
	return scenario, nil
}

func (p *parser) parseInputList() ([]inputSyntax, sourceSpan, error) {
	var inputs []inputSyntax
	end := sourceSpan{}
	for !p.at(tokenRParen) {
		name, nameSpan, err := p.parseLocalIdentifier()
		if err != nil {
			return nil, sourceSpan{}, err
		}
		if _, err := p.expect(tokenColon); err != nil {
			return nil, sourceSpan{}, err
		}
		typeToken, err := p.expect(tokenIdentifier)
		if err != nil {
			return nil, sourceSpan{}, err
		}
		if _, ok := allowedInputTypes[typeToken.Text]; !ok {
			return nil, sourceSpan{}, &parserError{
				span:    typeToken.Span,
				message: fmt.Sprintf("unsupported input type %q", typeToken.Text),
			}
		}
		required := p.match(tokenBang)
		input := inputSyntax{
			Name:     name,
			Type:     typeToken.Text,
			Required: required,
			Span: sourceSpan{
				Start: nameSpan.Start,
				End:   typeToken.Span.End,
			},
		}
		if required {
			input.Span.End = p.previous().Span.End
		}
		inputs = append(inputs, input)
		end = input.Span
		if !p.match(tokenComma) {
			break
		}
	}
	right, err := p.expect(tokenRParen)
	if err != nil {
		return nil, sourceSpan{}, err
	}
	if len(inputs) == 0 {
		end = right.Span
	} else {
		end.End = right.Span.End
	}
	return inputs, end, nil
}

func (p *parser) parseAct() (actSyntax, error) {
	start, err := p.expectKeyword("act")
	if err != nil {
		return actSyntax{}, err
	}
	id, idSpan, err := p.parseLocalIdentifier()
	if err != nil {
		return actSyntax{}, err
	}
	act := actSyntax{
		ID:   id,
		Span: sourceSpan{Start: start.Span.Start, End: idSpan.End},
	}
	if err := p.expectBlockStart(); err != nil {
		return actSyntax{}, err
	}
	lastSection := actSectionName
	for !p.at(tokenDedent) && !p.at(tokenEOF) {
		p.skipIgnorable()
		if p.at(tokenDedent) || p.at(tokenEOF) {
			break
		}
		section, handled := p.classifyActEntry()
		if !handled {
			return actSyntax{}, p.errorAtCurrent(expectedActEntryMessage)
		}
		if section < lastSection {
			return actSyntax{}, &parserError{
				span:    p.peek().Span,
				message: expectedActEntryOrderMessage,
			}
		}
		lastSection = section
		handled, err := p.parseActEntry(&act)
		if err != nil {
			return actSyntax{}, err
		}
		if !handled {
			return actSyntax{}, p.errorAtCurrent(expectedActEntryMessage)
		}
	}
	if _, err := p.expect(tokenDedent); err != nil {
		return actSyntax{}, err
	}
	if act.Action == nil {
		return actSyntax{}, &parserError{
			span:    act.Span,
			message: "act must declare exactly one do action",
		}
	}
	return act, nil
}

func (p *parser) classifyActEntry() (actSection, bool) {
	switch {
	case p.atKeyword("name"):
		return actSectionName, true
	case p.atKeyword("eventually"):
		return actSectionEventually, true
	case p.atKeyword("prop"):
		return actSectionProperty, true
	case p.atKeyword("do"):
		return actSectionAction, true
	case p.atKeyword("capture_auth"):
		return actSectionCaptureAuth, true
	case p.atKeyword("log"):
		return actSectionLog, true
	case p.atKeyword("expect"):
		return actSectionExpectation, true
	case p.atKeyword("export"):
		return actSectionExport, true
	case p.atKeyword("on"):
		return actSectionTransition, true
	default:
		return actSectionEventually, false
	}
}

func (p *parser) parseActEntry(act *actSyntax) (bool, error) {
	entry := p.actEntryParser()
	if entry == nil {
		return false, nil
	}
	return true, entry(act)
}

func (p *parser) actEntryParser() func(*actSyntax) error {
	switch {
	case p.atKeyword("name"):
		return p.parseActNameEntry
	case p.atKeyword("eventually"):
		return p.parseActEventuallyEntry
	case p.atKeyword("prop"):
		return p.parseActPropertyEntry
	case p.atKeyword("do"):
		return p.parseActActionEntry
	case p.atKeyword("capture_auth"):
		return p.parseActCaptureAuthEntry
	case p.atKeyword("log"):
		return p.parseActLogEntry
	case p.atKeyword("expect"):
		return p.parseActExpectationEntry
	case p.atKeyword("export"):
		return p.parseActExportEntry
	case p.atKeyword("on"):
		return p.parseActTransitionEntry
	default:
		return nil
	}
}

func (p *parser) parseActNameEntry(act *actSyntax) error {
	if act.Name != nil {
		return &parserError{
			span:    p.peek().Span,
			message: "act already declares name",
		}
	}
	name, err := p.parseName()
	if err != nil {
		return err
	}
	act.Name = &name
	act.Span.End = name.Span.End
	return nil
}

func (p *parser) parseActEventuallyEntry(act *actSyntax) error {
	if act.Eventually != nil {
		return &parserError{
			span:    p.peek().Span,
			message: "act already declares eventually",
		}
	}
	eventually, err := p.parseEventually()
	if err != nil {
		return err
	}
	act.Eventually = &eventually
	act.Span.End = eventually.Span.End
	return nil
}

func (p *parser) parseActPropertyEntry(act *actSyntax) error {
	property, err := p.parseProperty()
	if err != nil {
		return err
	}
	act.Properties = append(act.Properties, property)
	act.Span.End = property.Span.End
	return nil
}

func (p *parser) parseActActionEntry(act *actSyntax) error {
	if act.Action != nil {
		return &parserError{
			span:    p.peek().Span,
			message: "act already declares a do action",
		}
	}
	action, err := p.parseAction()
	if err != nil {
		return err
	}
	act.Action = &action
	act.Span.End = action.Span.End
	return nil
}

func (p *parser) parseActCaptureAuthEntry(act *actSyntax) error {
	if act.CaptureAuth != nil {
		return &parserError{
			span:    p.peek().Span,
			message: "act already declares capture_auth",
		}
	}
	captureAuth, err := p.parseCaptureAuth()
	if err != nil {
		return err
	}
	act.CaptureAuth = &captureAuth
	act.Span.End = captureAuth.Span.End
	return nil
}

func (p *parser) parseActLogEntry(act *actSyntax) error {
	log, err := p.parseLog()
	if err != nil {
		return err
	}
	act.Logs = append(act.Logs, log)
	act.Span.End = log.Span.End
	return nil
}

func (p *parser) parseActExpectationEntry(act *actSyntax) error {
	expectation, err := p.parseExpectation()
	if err != nil {
		return err
	}
	act.Expectations = append(act.Expectations, expectation)
	act.Span.End = expectation.Span.End
	return nil
}

func (p *parser) parseLog() (logSyntax, error) {
	start, err := p.expectKeyword("log")
	if err != nil {
		return logSyntax{}, err
	}
	id, _, err := p.parseLocalIdentifier()
	if err != nil {
		return logSyntax{}, err
	}
	if _, err := p.expect(tokenEqual); err != nil {
		return logSyntax{}, err
	}
	value, err := p.parseExpression()
	if err != nil {
		return logSyntax{}, err
	}
	if err := p.expectLineEnd(); err != nil {
		return logSyntax{}, err
	}
	return logSyntax{
		ID:    id,
		Value: value,
		Span:  sourceSpan{Start: start.Span.Start, End: value.ExpressionSpan().End},
	}, nil
}

func (p *parser) parseActExportEntry(act *actSyntax) error {
	export, err := p.parseExport()
	if err != nil {
		return err
	}
	act.Exports = append(act.Exports, export)
	act.Span.End = export.Span.End
	return nil
}

func (p *parser) parseActTransitionEntry(act *actSyntax) error {
	transition, err := p.parseTransition()
	if err != nil {
		return err
	}
	act.Transitions = append(act.Transitions, transition)
	act.Span.End = transition.Span.End
	return nil
}

func (p *parser) parseName() (nameSyntax, error) {
	start, err := p.expectKeyword("name")
	if err != nil {
		return nameSyntax{}, err
	}
	value, err := p.parseExpression()
	if err != nil {
		return nameSyntax{}, err
	}
	if !isNameLiteral(value) {
		return nameSyntax{}, &parserError{
			span:    value.ExpressionSpan(),
			message: "name value must be single-line string literal",
		}
	}
	if err := p.expectLineEnd(); err != nil {
		return nameSyntax{}, err
	}
	return nameSyntax{
		Value: value,
		Span:  sourceSpan{Start: start.Span.Start, End: value.ExpressionSpan().End},
	}, nil
}

func (p *parser) parseEventually() (eventuallySyntax, error) {
	start, err := p.expectKeyword("eventually")
	if err != nil {
		return eventuallySyntax{}, err
	}
	timeout, _, err := p.parseDurationToken()
	if err != nil {
		return eventuallySyntax{}, err
	}
	if _, err := p.expectKeyword("every"); err != nil {
		return eventuallySyntax{}, err
	}
	interval, intervalSpan, err := p.parseDurationToken()
	if err != nil {
		return eventuallySyntax{}, err
	}
	if err := p.expectLineEnd(); err != nil {
		return eventuallySyntax{}, err
	}
	return eventuallySyntax{
		Timeout:  timeout,
		Interval: interval,
		Span: sourceSpan{
			Start: start.Span.Start,
			End:   intervalSpan.End,
		},
	}, nil
}

func (p *parser) parseProperty() (propertySyntax, error) {
	start, err := p.expectKeyword("prop")
	if err != nil {
		return propertySyntax{}, err
	}
	name, _, err := p.parseLocalIdentifier()
	if err != nil {
		return propertySyntax{}, err
	}
	if _, err := p.expect(tokenEqual); err != nil {
		return propertySyntax{}, err
	}
	value, err := p.parseExpression()
	if err != nil {
		return propertySyntax{}, err
	}
	if err := p.expectLineEnd(); err != nil {
		return propertySyntax{}, err
	}
	return propertySyntax{
		Name:  name,
		Value: value,
		Span:  sourceSpan{Start: start.Span.Start, End: value.ExpressionSpan().End},
	}, nil
}

func (p *parser) parseAction() (actionSyntax, error) {
	start, err := p.expectKeyword("do")
	if err != nil {
		return actionSyntax{}, err
	}
	repeatable := p.matchKeyword("repeatable")
	call, err := p.parseCallExpression(true)
	if err != nil {
		return actionSyntax{}, err
	}
	if !call.BlockForm {
		if err := p.expectLineEnd(); err != nil {
			return actionSyntax{}, err
		}
	}
	return actionSyntax{
		Repeatable: repeatable,
		Call:       call,
		Span:       sourceSpan{Start: start.Span.Start, End: call.Span.End},
	}, nil
}

func (p *parser) parseExpectation() (expectationSyntax, error) {
	start, err := p.expectKeyword("expect")
	if err != nil {
		return expectationSyntax{}, err
	}
	id, _, err := p.parseLocalIdentifier()
	if err != nil {
		return expectationSyntax{}, err
	}
	if _, err := p.expect(tokenColon); err != nil {
		return expectationSyntax{}, err
	}
	subject, err := p.parseExpression()
	if err != nil {
		return expectationSyntax{}, err
	}
	assertion, err := p.parseAssertion()
	if err != nil {
		return expectationSyntax{}, err
	}
	if err := p.expectLineEnd(); err != nil {
		return expectationSyntax{}, err
	}
	return expectationSyntax{
		ID:      id,
		Subject: subject,
		Assert:  assertion,
		Span: sourceSpan{
			Start: start.Span.Start,
			End:   assertion.Span.End,
		},
	}, nil
}

func (p *parser) parseAssertion() (assertionSyntax, error) {
	negationSpan := p.parseAssertionNegation()

	if assertion, ok, err := p.parseSimpleAssertion(negationSpan); ok || err != nil {
		return assertion, err
	}
	if assertion, ok, err := p.parseComparisonAssertion(negationSpan); ok || err != nil {
		return assertion, err
	}
	if assertion, ok, err := p.parseBetweenAssertion(negationSpan); ok || err != nil {
		return assertion, err
	}
	if assertion, ok, err := p.parseCollectionAssertion(negationSpan); ok || err != nil {
		return assertion, err
	}
	if assertion, ok, err := p.parseHasKeyAssertion(negationSpan); ok || err != nil {
		return assertion, err
	}
	if assertion, ok, err := p.parseLacksKeyAssertion(negationSpan); ok || err != nil {
		return assertion, err
	}
	if assertion, ok, err := p.parseIsAssertion(negationSpan); ok || err != nil {
		return assertion, err
	}
	if assertion, ok, err := p.parseAssertCallAssertion(negationSpan); ok || err != nil {
		return assertion, err
	}

	if negationSpan != nil {
		return assertionSyntax{}, p.errorAtCurrent("expected assertion core after not")
	}

	return assertionSyntax{}, p.errorAtCurrent(expectedAssertionMessage)
}

func (p *parser) parseAssertionNegation() *sourceSpan {
	if !p.matchKeyword("not") {
		return nil
	}

	span := p.previous().Span
	return &span
}

func (p *parser) parseSimpleAssertion(negationSpan *sourceSpan) (assertionSyntax, bool, error) {
	switch {
	case p.match(tokenEqual):
		if _, err := p.expect(tokenEqual); err != nil {
			return assertionSyntax{}, true, err
		}
		value, err := p.parseExpression()
		if err != nil {
			return assertionSyntax{}, true, err
		}
		return newAssertionSyntax(assertionKindEqual, negationSpan, value), true, nil
	case p.match(tokenBang):
		bangToken := p.previous()
		if _, err := p.expect(tokenEqual); err != nil {
			return assertionSyntax{}, true, err
		}
		if negationSpan != nil {
			return assertionSyntax{}, true, &parserError{
				span:    bangToken.Span,
				message: `not != is not supported; use ==`,
			}
		}
		value, err := p.parseExpression()
		if err != nil {
			return assertionSyntax{}, true, err
		}
		return newAssertionSyntax(assertionKindNotEqual, negationSpan, value), true, nil
	case p.matchKeyword("matches"):
		value, err := p.parseExpression()
		if err != nil {
			return assertionSyntax{}, true, err
		}
		return newAssertionSyntax(assertionKindMatches, negationSpan, value), true, nil
	case p.matchKeyword("contains"):
		value, err := p.parseExpression()
		if err != nil {
			return assertionSyntax{}, true, err
		}
		return newAssertionSyntax(assertionKindContains, negationSpan, value), true, nil
	default:
		return assertionSyntax{}, false, nil
	}
}

func (p *parser) parseComparisonAssertion(negationSpan *sourceSpan) (assertionSyntax, bool, error) {
	if p.match(tokenGreater) {
		kind := assertionKindGT
		if p.match(tokenEqual) {
			kind = assertionKindGTE
		}
		value, err := p.parseExpression()
		if err != nil {
			return assertionSyntax{}, true, err
		}
		return newAssertionSyntax(kind, negationSpan, value), true, nil
	}
	if p.match(tokenLess) {
		kind := assertionKindLT
		if p.match(tokenEqual) {
			kind = assertionKindLTE
		}
		value, err := p.parseExpression()
		if err != nil {
			return assertionSyntax{}, true, err
		}
		return newAssertionSyntax(kind, negationSpan, value), true, nil
	}

	return assertionSyntax{}, false, nil
}

func (p *parser) parseBetweenAssertion(negationSpan *sourceSpan) (assertionSyntax, bool, error) {
	if !p.atKeyword("between") {
		return assertionSyntax{}, false, nil
	}

	start, err := p.expectKeyword("between")
	if err != nil {
		return assertionSyntax{}, true, err
	}
	minValue, err := p.parseExpression()
	if err != nil {
		return assertionSyntax{}, true, err
	}
	if _, err := p.expectKeyword("and"); err != nil {
		return assertionSyntax{}, true, &parserError{
			span:    minValue.ExpressionSpan(),
			message: `expected "and" after between lower bound`,
		}
	}
	maxValue, err := p.parseExpression()
	if err != nil {
		return assertionSyntax{}, true, err
	}

	return assertionSyntax{
		Kind:         assertionKindBetween,
		NegationSpan: negationSpan,
		Value:        minValue,
		SecondValue:  maxValue,
		Span: assertionSpan(negationSpan, sourceSpan{
			Start: start.Span.Start,
			End:   maxValue.ExpressionSpan().End,
		}),
	}, true, nil
}

func (p *parser) parseCollectionAssertion(negationSpan *sourceSpan) (assertionSyntax, bool, error) {
	switch {
	case p.atKeyword("has") && p.atKeywordOffset(1, "entry"):
		assertion, err := p.parseHasEntryAssertion(negationSpan)
		return assertion, true, err
	case p.atKeyword("has") && p.atKeywordOffset(1, "item"):
		start, err := p.expectKeyword("has")
		if err != nil {
			return assertionSyntax{}, true, err
		}
		itemToken, err := p.expectKeyword("item")
		if err != nil {
			return assertionSyntax{}, true, err
		}
		clauses, clausesSpan, err := p.parseRelativeClauseList(`"has item"`, itemToken.Span)
		if err != nil {
			return assertionSyntax{}, true, err
		}
		return assertionSyntax{
			Kind:         assertionKindHasItem,
			NegationSpan: negationSpan,
			WhereSpan:    clausesSpan,
			Clauses:      clauses,
			Span: assertionSpan(negationSpan, sourceSpan{
				Start: start.Span.Start,
				End:   clausesSpan.End,
			}),
		}, true, nil
	case p.atKeyword("all"):
		start, err := p.expectKeyword("all")
		if err != nil {
			return assertionSyntax{}, true, err
		}
		itemsToken, err := p.expectKeyword("items")
		if err != nil {
			return assertionSyntax{}, true, &parserError{
				span:    start.Span,
				message: `expected "items" after "all"`,
			}
		}
		clauses, clausesSpan, err := p.parseRelativeClauseList(`"all items"`, itemsToken.Span)
		if err != nil {
			return assertionSyntax{}, true, err
		}
		return assertionSyntax{
			Kind:         assertionKindAllItems,
			NegationSpan: negationSpan,
			WhereSpan:    clausesSpan,
			Clauses:      clauses,
			Span: assertionSpan(negationSpan, sourceSpan{
				Start: start.Span.Start,
				End:   clausesSpan.End,
			}),
		}, true, nil
	default:
		return assertionSyntax{}, false, nil
	}
}

func (p *parser) parseHasEntryAssertion(negationSpan *sourceSpan) (assertionSyntax, error) {
	start, err := p.expectKeyword("has")
	if err != nil {
		return assertionSyntax{}, err
	}
	entryToken, err := p.expectKeyword("entry")
	if err != nil {
		return assertionSyntax{}, err
	}
	if _, err := p.expect(tokenLParen); err != nil {
		return assertionSyntax{}, err
	}
	key, err := p.parseExpression()
	if err != nil {
		return assertionSyntax{}, err
	}
	if _, err := p.expect(tokenRParen); err != nil {
		return assertionSyntax{}, err
	}
	nested, err := p.parseAssertion()
	if err != nil {
		return assertionSyntax{}, err
	}

	return assertionSyntax{
		Kind:         assertionKindHasEntry,
		NegationSpan: negationSpan,
		Value:        key,
		Nested:       &nested,
		Span: assertionSpan(negationSpan, sourceSpan{
			Start: start.Span.Start,
			End:   nested.Span.End,
		}),
		WhereSpan: sourceSpan{
			Start: start.Span.Start,
			End:   entryToken.Span.End,
		},
	}, nil
}

func (p *parser) parseHasKeyAssertion(negationSpan *sourceSpan) (assertionSyntax, bool, error) {
	if !p.atKeyword("has") {
		return assertionSyntax{}, false, nil
	}

	if p.peekOffset(1).Kind == tokenIdentifier && p.peekOffset(1).Text == "no" {
		assertion, err := p.parseHasNoKeyAssertion(negationSpan)
		return assertion, true, err
	}

	assertion, err := p.parseKeyCallAssertion("has", assertionKindHasKey, negationSpan, `"key" or "item"`)
	return assertion, true, err
}

func (p *parser) parseHasNoKeyAssertion(negationSpan *sourceSpan) (assertionSyntax, error) {
	startToken, err := p.expectKeyword("has")
	if err != nil {
		return assertionSyntax{}, err
	}
	if _, err := p.expectKeyword("no"); err != nil {
		return assertionSyntax{}, err
	}
	keyToken, err := p.expectKeyword("key")
	if err != nil {
		return assertionSyntax{}, &parserError{
			span:    startToken.Span,
			message: `expected "key" after "has no"`,
		}
	}
	if _, err := p.expect(tokenLParen); err != nil {
		return assertionSyntax{}, err
	}
	value, err := p.parseExpression()
	if err != nil {
		return assertionSyntax{}, err
	}
	end, err := p.expect(tokenRParen)
	if err != nil {
		return assertionSyntax{}, err
	}

	return assertionSyntax{
		Kind:         assertionKindLacksKey,
		NegationSpan: negationSpan,
		Value:        value,
		Span: assertionSpan(negationSpan, sourceSpan{
			Start: keyToken.Span.Start,
			End:   end.Span.End,
		}),
	}, nil
}

func (p *parser) parseLacksKeyAssertion(negationSpan *sourceSpan) (assertionSyntax, bool, error) {
	if !p.atKeyword("lacks") {
		return assertionSyntax{}, false, nil
	}

	assertion, err := p.parseKeyCallAssertion("lacks", assertionKindLacksKey, negationSpan, `"key"`)
	return assertion, true, err
}

func (p *parser) parseKeyCallAssertion(
	keyword string,
	kind assertionKind,
	negationSpan *sourceSpan,
	expectedKeyMessage string,
) (assertionSyntax, error) {
	startToken, err := p.expectKeyword(keyword)
	if err != nil {
		return assertionSyntax{}, err
	}

	keyToken, err := p.expectKeyword("key")
	if err != nil {
		return assertionSyntax{}, &parserError{
			span:    startToken.Span,
			message: `expected ` + expectedKeyMessage + ` after "` + keyword + `"`,
		}
	}
	if _, err := p.expect(tokenLParen); err != nil {
		return assertionSyntax{}, err
	}
	value, err := p.parseExpression()
	if err != nil {
		return assertionSyntax{}, err
	}
	end, err := p.expect(tokenRParen)
	if err != nil {
		return assertionSyntax{}, err
	}

	return assertionSyntax{
		Kind:         kind,
		NegationSpan: negationSpan,
		Value:        value,
		Span: assertionSpan(negationSpan, sourceSpan{
			Start: keyToken.Span.Start,
			End:   end.Span.End,
		}),
	}, nil
}

func (p *parser) parseIsAssertion(negationSpan *sourceSpan) (assertionSyntax, bool, error) {
	if !p.atKeyword("is") {
		return assertionSyntax{}, false, nil
	}
	isToken, err := p.expectKeyword("is")
	if err != nil {
		return assertionSyntax{}, true, err
	}

	if p.matchKeyword("not") {
		if p.matchKeyword("present") {
			return assertionSyntax{}, true, &parserError{
				span:    p.previous().Span,
				message: notPresentAssertionMessage,
			}
		}
		nullToken, err := p.expectKeyword("null")
		if err != nil {
			return assertionSyntax{}, true, &parserError{
				span:    isToken.Span,
				message: `expected "null" after "is not"`,
			}
		}
		return assertionSyntax{
			Kind:         assertionKindNotNull,
			NegationSpan: negationSpan,
			Span: assertionSpan(negationSpan, sourceSpan{
				Start: isToken.Span.Start,
				End:   nullToken.Span.End,
			}),
		}, true, nil
	}

	if p.matchKeyword("null") {
		return assertionSyntax{
			Kind:         assertionKindNull,
			NegationSpan: negationSpan,
			Span: assertionSpan(negationSpan, sourceSpan{
				Start: isToken.Span.Start,
				End:   p.previous().Span.End,
			}),
		}, true, nil
	}
	if p.matchKeyword("present") {
		if negationSpan != nil {
			return assertionSyntax{}, true, &parserError{
				span:    p.previous().Span,
				message: notPresentAssertionMessage,
			}
		}
		return assertionSyntax{
			Kind:         assertionKindPresent,
			NegationSpan: negationSpan,
			Span: assertionSpan(negationSpan, sourceSpan{
				Start: isToken.Span.Start,
				End:   p.previous().Span.End,
			}),
		}, true, nil
	}
	if p.matchKeyword("missing") {
		return assertionSyntax{}, true, &parserError{
			span:    p.previous().Span,
			message: missingAssertionMessage,
		}
	}
	if p.matchKeyword("absent") {
		return assertionSyntax{}, true, &parserError{
			span:    p.previous().Span,
			message: absentAssertionMessage,
		}
	}

	return assertionSyntax{}, true, &parserError{
		span:    isToken.Span,
		message: `expected "null", "not null", or "present" after "is"`,
	}
}

func (p *parser) parseRelativeClauseList(
	owner string,
	ownerSpan sourceSpan,
) ([]relativeClauseSyntax, sourceSpan, error) {
	whereToken, err := p.expectKeyword("where")
	if err != nil {
		return nil, sourceSpan{}, &parserError{
			span:    ownerSpan,
			message: `expected "where" after ` + owner,
		}
	}

	if p.match(tokenLParen) {
		return p.parseGroupedRelativeClauseList(whereToken.Span)
	}
	if p.at(tokenNewline) || p.at(tokenDedent) || p.at(tokenEOF) {
		return nil, sourceSpan{}, &parserError{
			span:    whereToken.Span,
			message: "expected relative clause after where",
		}
	}

	clause, err := p.parseRelativeClause()
	if err != nil {
		return nil, sourceSpan{}, err
	}

	return []relativeClauseSyntax{clause}, sourceSpan{
		Start: whereToken.Span.Start,
		End:   clause.Span.End,
	}, nil
}

func (p *parser) parseGroupedRelativeClauseList(start sourceSpan) ([]relativeClauseSyntax, sourceSpan, error) {
	p.skipIgnorable()
	if p.at(tokenRParen) {
		return nil, sourceSpan{}, &parserError{
			span:    start,
			message: "expected relative clause after where",
		}
	}

	clauses := make([]relativeClauseSyntax, 0, 2)
	for {
		p.skipIgnorable()
		clause, err := p.parseRelativeClause()
		if err != nil {
			return nil, sourceSpan{}, err
		}
		clauses = append(clauses, clause)

		p.skipIgnorable()
		if p.match(tokenComma) {
			p.skipIgnorable()
			if p.at(tokenRParen) {
				return nil, sourceSpan{}, &parserError{
					span:    p.previous().Span,
					message: "expected relative clause after comma",
				}
			}
			continue
		}
		if p.at(tokenRParen) {
			right, err := p.expect(tokenRParen)
			if err != nil {
				return nil, sourceSpan{}, err
			}
			return clauses, sourceSpan{
				Start: start.Start,
				End:   right.Span.End,
			}, nil
		}

		return nil, sourceSpan{}, p.errorAtCurrent(`expected "," or ")" after relative clause`)
	}
}

func (p *parser) parseRelativeClause() (relativeClauseSyntax, error) {
	subject, err := p.parseExpression()
	if err != nil {
		return relativeClauseSyntax{}, err
	}
	assertion, err := p.parseAssertion()
	if err != nil {
		return relativeClauseSyntax{}, err
	}
	return relativeClauseSyntax{
		Subject: subject,
		Assert:  assertion,
		Span: sourceSpan{
			Start: subject.ExpressionSpan().Start,
			End:   assertion.Span.End,
		},
	}, nil
}

func (p *parser) parseAssertCallAssertion(negationSpan *sourceSpan) (assertionSyntax, bool, error) {
	if !p.matchKeyword("assert") {
		return assertionSyntax{}, false, nil
	}

	call, err := p.parseCallExpression(false)
	if err != nil {
		return assertionSyntax{}, true, err
	}

	return assertionSyntax{
		Kind:         assertionKindCall,
		NegationSpan: negationSpan,
		Value:        call,
		Span:         assertionSpan(negationSpan, call.Span),
	}, true, nil
}

func newAssertionSyntax(
	kind assertionKind,
	negationSpan *sourceSpan,
	value expressionSyntax,
) assertionSyntax {
	return assertionSyntax{
		Kind:         kind,
		NegationSpan: negationSpan,
		Value:        value,
		Span:         assertionSpan(negationSpan, value.ExpressionSpan()),
	}
}

func assertionSpan(negationSpan *sourceSpan, body sourceSpan) sourceSpan {
	if negationSpan == nil {
		return body
	}

	return sourceSpan{
		Start: negationSpan.Start,
		End:   body.End,
	}
}

func (p *parser) parseExport() (exportSyntax, error) {
	start, err := p.expectKeyword("export")
	if err != nil {
		return exportSyntax{}, err
	}
	name, _, err := p.parseLocalIdentifier()
	if err != nil {
		return exportSyntax{}, err
	}
	if _, err := p.expect(tokenEqual); err != nil {
		return exportSyntax{}, err
	}
	value, err := p.parseExpression()
	if err != nil {
		return exportSyntax{}, err
	}
	end := value.ExpressionSpan()
	var assertion *assertionSyntax
	if !p.consumeLineEndIfPresent() {
		parsed, err := p.parseAssertion()
		if err != nil {
			return exportSyntax{}, err
		}
		assertion = &parsed
		end.End = parsed.Span.End
		if err := p.expectLineEnd(); err != nil {
			return exportSyntax{}, err
		}
	}
	return exportSyntax{
		Name:   name,
		Value:  value,
		Assert: assertion,
		Span:   sourceSpan{Start: start.Span.Start, End: end.End},
	}, nil
}

func (p *parser) parseTransition() (transitionSyntax, error) {
	start, err := p.expectKeyword("on")
	if err != nil {
		return transitionSyntax{}, err
	}
	event, _, err := p.parseLocalIdentifier()
	if err != nil {
		return transitionSyntax{}, err
	}
	if _, ok := allowedTransitionEvents[event]; !ok {
		return transitionSyntax{}, &parserError{
			span:    p.previous().Span,
			message: fmt.Sprintf("unsupported transition event %q", event),
		}
	}
	if _, err := p.expect(tokenArrow); err != nil {
		return transitionSyntax{}, err
	}
	target, targetSpan, err := p.parseLocalIdentifier()
	if err != nil {
		return transitionSyntax{}, err
	}
	if err := p.expectLineEnd(); err != nil {
		return transitionSyntax{}, err
	}
	return transitionSyntax{
		Event: event,
		To:    target,
		Span:  sourceSpan{Start: start.Span.Start, End: targetSpan.End},
	}, nil
}

func (p *parser) parseCaptureAuth() (captureAuthSyntax, error) {
	start, err := p.expectKeyword("capture_auth")
	if err != nil {
		return captureAuthSyntax{}, err
	}
	auth, authSpan, err := p.parseLocalIdentifier()
	if err != nil {
		return captureAuthSyntax{}, err
	}

	captureAuth := captureAuthSyntax{
		Auth: auth,
		Span: sourceSpan{Start: start.Span.Start, End: authSpan.End},
	}

	if err := p.expectBlockStart(); err != nil {
		return captureAuthSyntax{}, err
	}
	for !p.at(tokenDedent) && !p.at(tokenEOF) {
		p.skipIgnorable()
		if p.at(tokenDedent) || p.at(tokenEOF) {
			break
		}
		slot, err := p.parseMappingEntry()
		if err != nil {
			return captureAuthSyntax{}, err
		}
		captureAuth.Slots = append(captureAuth.Slots, slot)
		captureAuth.Span.End = slot.Span.End
	}
	if _, err := p.expect(tokenDedent); err != nil {
		return captureAuthSyntax{}, err
	}
	return captureAuth, nil
}

func (p *parser) parseScenarioCall() (scenarioCallSyntax, error) {
	start, err := p.expectKeyword("call")
	if err != nil {
		return scenarioCallSyntax{}, err
	}
	id, _, err := p.parseLocalIdentifier()
	if err != nil {
		return scenarioCallSyntax{}, err
	}
	if _, err := p.expect(tokenEqual); err != nil {
		return scenarioCallSyntax{}, err
	}
	scenarioID, _, err := p.parseSlashName()
	if err != nil {
		return scenarioCallSyntax{}, err
	}
	if _, err := p.expect(tokenLParen); err != nil {
		return scenarioCallSyntax{}, err
	}
	bindings, end, err := p.parseNamedArgumentList()
	if err != nil {
		return scenarioCallSyntax{}, err
	}
	call := scenarioCallSyntax{
		ID:         id,
		ScenarioID: scenarioID,
		Bindings:   bindings,
		Span: sourceSpan{
			Start: start.Span.Start,
			End:   end.End,
		},
	}
	bodyEnd, ok, err := p.parseOptionalCallBody(&call)
	if err != nil {
		return scenarioCallSyntax{}, err
	}
	if ok {
		call.Span.End = bodyEnd.End
		return call, nil
	}
	if err := p.expectLineEnd(); err != nil {
		return scenarioCallSyntax{}, err
	}
	call.Span.End = end.End
	return call, nil
}

func (p *parser) parseOptionalCallBody(call *scenarioCallSyntax) (sourceSpan, bool, error) {
	if !p.hasIndentedBlockAhead() {
		return sourceSpan{}, false, nil
	}
	if _, err := p.expect(tokenNewline); err != nil {
		return sourceSpan{}, false, err
	}
	p.skipComments()
	for p.match(tokenNewline) {
		p.skipComments()
	}
	indent, err := p.expect(tokenIndent)
	if err != nil {
		return sourceSpan{}, false, err
	}

	lastSection := callSectionName
	end := sourceSpan{Start: indent.Span.Start, End: indent.Span.End}
	for !p.at(tokenDedent) && !p.at(tokenEOF) {
		p.skipIgnorable()
		if p.at(tokenDedent) || p.at(tokenEOF) {
			break
		}
		section, handled := p.classifyCallEntry()
		if !handled {
			return sourceSpan{}, false, p.errorAtCurrent("expected name, dependency, or export")
		}
		if section < lastSection {
			return sourceSpan{}, false, &parserError{
				span:    p.peek().Span,
				message: "call entries must follow name, dependency, export order",
			}
		}
		lastSection = section

		switch section {
		case callSectionName:
			if call.Name != nil {
				return sourceSpan{}, false, &parserError{
					span:    p.peek().Span,
					message: "call already declares name",
				}
			}
			name, err := p.parseName()
			if err != nil {
				return sourceSpan{}, false, err
			}
			call.Name = &name
			end = name.Span
		case callSectionDependency:
			dependency, err := p.parseDependency()
			if err != nil {
				return sourceSpan{}, false, err
			}
			call.Dependencies = append(call.Dependencies, dependency)
			end = dependency.Span
		case callSectionExport:
			export, err := p.parseExport()
			if err != nil {
				return sourceSpan{}, false, err
			}
			if export.Assert != nil {
				return sourceSpan{}, false, &parserError{
					span:    export.Assert.Span,
					message: "scenario call export assertions are not supported",
				}
			}
			call.Exports = append(call.Exports, export)
			end = export.Span
		}
	}
	if _, err := p.expect(tokenDedent); err != nil {
		return sourceSpan{}, false, err
	}
	return sourceSpan{Start: end.Start, End: p.previous().Span.End}, true, nil
}

func (p *parser) classifyCallEntry() (callSection, bool) {
	switch {
	case p.atKeyword("name"):
		return callSectionName, true
	case p.atKeyword("dependency"):
		return callSectionDependency, true
	case p.atKeyword("export"):
		return callSectionExport, true
	default:
		return callSectionName, false
	}
}

func (p *parser) parseDependency() (dependencySyntax, error) {
	start, err := p.expectKeyword("dependency")
	if err != nil {
		return dependencySyntax{}, err
	}
	callID, callIDSpan, err := p.parseLocalIdentifier()
	if err != nil {
		return dependencySyntax{}, err
	}

	dependency := dependencySyntax{
		CallID: callID,
		Span:   sourceSpan{Start: start.Span.Start, End: callIDSpan.End},
	}
	if p.matchKeyword("when") {
		when, whenSpan, err := p.parseLocalIdentifier()
		if err != nil {
			return dependencySyntax{}, err
		}
		if _, ok := allowedDependencyPredicates[when]; !ok {
			return dependencySyntax{}, &parserError{
				span:    whenSpan,
				message: fmt.Sprintf("unsupported dependency predicate %q", when),
			}
		}
		dependency.When = when
		dependency.Span.End = whenSpan.End
	}
	if err := p.expectLineEnd(); err != nil {
		return dependencySyntax{}, err
	}
	return dependency, nil
}

func (p *parser) parseNamedArgumentList() ([]callArgumentSyntax, sourceSpan, error) {
	var args []callArgumentSyntax
	end := sourceSpan{}
	for !p.at(tokenRParen) {
		name, nameSpan, err := p.parseLocalIdentifier()
		if err != nil {
			return nil, sourceSpan{}, err
		}
		if _, err := p.expect(tokenColon); err != nil {
			return nil, sourceSpan{}, err
		}
		value, err := p.parseExpression()
		if err != nil {
			return nil, sourceSpan{}, err
		}
		arg := callArgumentSyntax{
			Name:  name,
			Value: value,
			Span:  sourceSpan{Start: nameSpan.Start, End: value.ExpressionSpan().End},
		}
		args = append(args, arg)
		end = arg.Span
		if !p.match(tokenComma) {
			break
		}
	}
	right, err := p.expect(tokenRParen)
	if err != nil {
		return nil, sourceSpan{}, err
	}
	if len(args) == 0 {
		end = right.Span
	} else {
		end.End = right.Span.End
	}
	return args, end, nil
}

func (p *parser) parseCallExpression(allowBlock bool) (callExpressionSyntax, error) {
	name, start, err := p.parseDotName()
	if err != nil {
		return callExpressionSyntax{}, err
	}
	call := callExpressionSyntax{
		Name: name,
		Span: start,
	}
	if name == selectorStepPick && p.atKeyword("where") {
		clauses, clausesSpan, err := p.parseRelativeClauseList(`"pick"`, start)
		if err != nil {
			return callExpressionSyntax{}, err
		}
		call.WhereSpan = clausesSpan
		call.Clauses = clauses
		call.Span.End = clausesSpan.End
		return call, nil
	}
	if p.match(tokenLParen) {
		args, end, err := p.parseCallArgumentList()
		if err != nil {
			return callExpressionSyntax{}, err
		}
		call.Args = args
		call.Span.End = end.End
		return call, nil
	}
	args, end, ok, err := p.parseOptionalBlockArguments(allowBlock)
	if err != nil {
		return callExpressionSyntax{}, err
	}
	if ok {
		call.Args = args
		call.BlockForm = true
		call.Span.End = end.End
	}
	return call, nil
}

func (p *parser) parseCallArgumentList() ([]callArgumentSyntax, sourceSpan, error) {
	var args []callArgumentSyntax
	end := sourceSpan{}
	for !p.at(tokenRParen) {
		arg, err := p.parseCallArgument()
		if err != nil {
			return nil, sourceSpan{}, err
		}
		args = append(args, arg)
		end = arg.Span
		if !p.match(tokenComma) {
			break
		}
	}
	right, err := p.expect(tokenRParen)
	if err != nil {
		return nil, sourceSpan{}, err
	}
	if len(args) == 0 {
		end = right.Span
	} else {
		end.End = right.Span.End
	}
	return args, end, nil
}

func (p *parser) parseCallArgument() (callArgumentSyntax, error) {
	if p.at(tokenString) && p.peekOffset(1).Kind == tokenColon {
		return callArgumentSyntax{}, quotedCoreIdentifierError(p.peek().Span)
	}
	if p.at(tokenIdentifier) && p.peekOffset(1).Kind == tokenColon {
		nameToken := p.peek()
		p.pos++
		if _, err := p.expect(tokenColon); err != nil {
			return callArgumentSyntax{}, err
		}
		value, err := p.parseExpression()
		if err != nil {
			return callArgumentSyntax{}, err
		}
		return callArgumentSyntax{
			Name:  nameToken.Text,
			Value: value,
			Span:  sourceSpan{Start: nameToken.Span.Start, End: value.ExpressionSpan().End},
		}, nil
	}
	value, err := p.parseExpression()
	if err != nil {
		return callArgumentSyntax{}, err
	}
	return callArgumentSyntax{
		Value: value,
		Span:  value.ExpressionSpan(),
	}, nil
}

func (p *parser) parseBlockArguments() ([]callArgumentSyntax, sourceSpan, error) {
	var args []callArgumentSyntax
	end := sourceSpan{}
	for !p.at(tokenDedent) && !p.at(tokenEOF) {
		p.skipIgnorable()
		if p.at(tokenDedent) || p.at(tokenEOF) {
			break
		}
		entry, err := p.parseMappingEntry()
		if err != nil {
			return nil, sourceSpan{}, err
		}
		args = append(args, callArgumentSyntax(entry))
		end = entry.Span
	}
	return args, end, nil
}

func (p *parser) parseMappingEntry() (mappingEntrySyntax, error) {
	name, nameSpan, err := p.parseLocalIdentifier()
	if err != nil {
		return mappingEntrySyntax{}, err
	}
	if _, err := p.expect(tokenColon); err != nil {
		return mappingEntrySyntax{}, err
	}
	entry := mappingEntrySyntax{
		Name: name,
		Span: sourceSpan{Start: nameSpan.Start, End: nameSpan.End},
	}
	if p.match(tokenNewline) {
		children, end, err := p.parseNestedMappingEntries()
		if err != nil {
			return mappingEntrySyntax{}, err
		}
		entry.Mapping = children
		entry.Span.End = end.End
		return entry, nil
	}
	value, err := p.parseExpression()
	if err != nil {
		return mappingEntrySyntax{}, err
	}
	entry.Value = value
	entry.Span.End = value.ExpressionSpan().End
	if err := p.expectLineEnd(); err != nil {
		return mappingEntrySyntax{}, err
	}
	return entry, nil
}

func (p *parser) parseExpression() (expressionSyntax, error) {
	base, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	if !p.match(tokenPipe) {
		return base, nil
	}
	pipeline := pipelineExpressionSyntax{
		Base: base,
		Span: base.ExpressionSpan(),
	}
	for {
		step, err := p.parseCallExpression(false)
		if err != nil {
			return nil, err
		}
		pipeline.Steps = append(pipeline.Steps, step)
		pipeline.Span.End = step.Span.End
		if !p.match(tokenPipe) {
			break
		}
	}
	return pipeline, nil
}

func (p *parser) parsePrimary() (expressionSyntax, error) {
	if expr, ok, err := p.parseRefExpression(); ok || err != nil {
		return expr, err
	}
	if expr, ok := p.parseLiteralExpression(); ok {
		return expr, nil
	}
	if expr, ok, err := p.parseKeywordExpression(); ok || err != nil {
		return expr, err
	}
	if expr, ok, err := p.parseGroupedOrContainerExpression(); ok || err != nil {
		return expr, err
	}
	if expr, ok, err := p.parseIdentifierExpression(); ok || err != nil {
		return expr, err
	}
	return nil, p.errorAtCurrent("expected expression")
}

func (p *parser) parseOptionalBlockArguments(allowBlock bool) ([]callArgumentSyntax, sourceSpan, bool, error) {
	if !allowBlock || !p.hasIndentedBlockAhead() {
		return nil, sourceSpan{}, false, nil
	}
	if _, err := p.expect(tokenNewline); err != nil {
		return nil, sourceSpan{}, false, err
	}
	p.skipComments()
	for p.match(tokenNewline) {
		p.skipComments()
	}
	indent, err := p.expect(tokenIndent)
	if err != nil {
		return nil, sourceSpan{}, false, err
	}

	args, end, err := p.parseBlockArguments()
	if err != nil {
		return nil, sourceSpan{}, false, err
	}
	if _, err := p.expect(tokenDedent); err != nil {
		return nil, sourceSpan{}, false, err
	}
	if len(args) == 0 {
		end = sourceSpan{Start: indent.Span.Start, End: p.previous().Span.End}
	} else {
		end.End = p.previous().Span.End
	}
	return args, end, true, nil
}

func (p *parser) parseNestedMappingEntries() ([]mappingEntrySyntax, sourceSpan, error) {
	p.skipComments()
	for p.match(tokenNewline) {
		p.skipComments()
	}
	if !p.match(tokenIndent) {
		return nil, sourceSpan{}, p.errorAtCurrent("expected indented mapping block")
	}

	var entries []mappingEntrySyntax
	end := sourceSpan{}
	for !p.at(tokenDedent) && !p.at(tokenEOF) {
		p.skipIgnorable()
		if p.at(tokenDedent) || p.at(tokenEOF) {
			break
		}
		child, err := p.parseMappingEntry()
		if err != nil {
			return nil, sourceSpan{}, err
		}
		entries = append(entries, child)
		end = child.Span
	}
	if _, err := p.expect(tokenDedent); err != nil {
		return nil, sourceSpan{}, err
	}
	end.End = p.previous().Span.End
	return entries, end, nil
}

func (p *parser) hasIndentedBlockAhead() bool {
	if p.pos >= len(p.tokens) || p.tokens[p.pos].Kind != tokenNewline {
		return false
	}

	for index := p.pos + 1; index < len(p.tokens); index++ {
		switch p.tokens[index].Kind {
		case tokenComment, tokenNewline:
			continue
		case tokenIndent:
			return true
		default:
			return false
		}
	}

	return false
}

func (p *parser) parseRefExpression() (expressionSyntax, bool, error) {
	if !p.match(tokenDollar) {
		return nil, false, nil
	}
	name, span, err := p.parseLocalIdentifier()
	if err != nil {
		return nil, false, err
	}
	return refExpressionSyntax{
		Name: name,
		Span: sourceSpan{Start: p.previousOffset(2).Span.Start, End: span.End},
	}, true, nil
}

func (p *parser) parseLiteralExpression() (expressionSyntax, bool) {
	switch {
	case p.at(tokenString), p.at(tokenRawString), p.at(tokenMultilineString), p.at(tokenNumber), p.at(tokenDuration):
		token := p.peek()
		p.pos++
		return literalExpressionSyntax{
			Kind: literalKindForToken(token.Kind),
			Text: token.Text,
			Span: token.Span,
		}, true
	case p.atKeyword("true"), p.atKeyword("false"):
		token := p.peek()
		p.pos++
		return literalExpressionSyntax{Kind: literalKindBool, Text: token.Text, Span: token.Span}, true
	case p.atKeyword("null"):
		token := p.peek()
		p.pos++
		return literalExpressionSyntax{Kind: literalKindNull, Text: token.Text, Span: token.Span}, true
	default:
		return nil, false
	}
}

func isNameLiteral(expr expressionSyntax) bool {
	value, ok := ungroupExpression(expr).(literalExpressionSyntax)
	if !ok {
		return false
	}

	switch value.Kind {
	case literalKindString, literalKindRawString:
		return true
	default:
		return false
	}
}

func (p *parser) parseKeywordExpression() (expressionSyntax, bool, error) {
	switch {
	case p.atKeyword("object"):
		start, err := p.expectKeyword("object")
		if err != nil {
			return nil, true, err
		}
		if _, err := p.expect(tokenLBrace); err != nil {
			return nil, true, err
		}
		object, err := p.parseObjectExpression(true, start.Span.Start)
		return object, true, err
	case p.atKeyword("list"):
		start, err := p.expectKeyword("list")
		if err != nil {
			return nil, true, err
		}
		if _, err := p.expect(tokenLBracket); err != nil {
			return nil, true, err
		}
		list, err := p.parseListExpression(true, start.Span.Start)
		return list, true, err
	default:
		return nil, false, nil
	}
}

func (p *parser) parseGroupedOrContainerExpression() (expressionSyntax, bool, error) {
	switch {
	case p.at(tokenLBrace):
		start := p.peek()
		p.pos++
		object, err := p.parseObjectExpression(false, start.Span.Start)
		return object, true, err
	case p.at(tokenLBracket):
		start := p.peek()
		p.pos++
		list, err := p.parseListExpression(false, start.Span.Start)
		return list, true, err
	case p.at(tokenLParen):
		start := p.peek()
		p.pos++
		inner, err := p.parseExpression()
		if err != nil {
			return nil, true, err
		}
		end, err := p.expect(tokenRParen)
		if err != nil {
			return nil, true, err
		}
		return groupedExpressionSyntax{
			Inner: inner,
			Span:  sourceSpan{Start: start.Span.Start, End: end.Span.End},
		}, true, nil
	default:
		return nil, false, nil
	}
}

func (p *parser) parseIdentifierExpression() (expressionSyntax, bool, error) {
	if !p.at(tokenIdentifier) {
		return nil, false, nil
	}
	if p.canStartCall() {
		call, err := p.parseCallExpression(false)
		return call, true, err
	}
	token := p.peek()
	p.pos++
	return symbolExpressionSyntax{Name: token.Text, Span: token.Span}, true, nil
}

func (p *parser) parseObjectExpression(dynamic bool, start sourcePosition) (objectExpressionSyntax, error) {
	object := objectExpressionSyntax{Dynamic: dynamic, Span: sourceSpan{Start: start, End: start}}
	for !p.at(tokenRBrace) {
		entry, err := p.parseInlineMappingEntry()
		if err != nil {
			return objectExpressionSyntax{}, err
		}
		object.Fields = append(object.Fields, entry)
		object.Span.End = entry.Span.End
		if !p.match(tokenComma) {
			break
		}
	}
	end, err := p.expect(tokenRBrace)
	if err != nil {
		return objectExpressionSyntax{}, err
	}
	object.Span.End = end.Span.End
	return object, nil
}

func (p *parser) parseListExpression(dynamic bool, start sourcePosition) (listExpressionSyntax, error) {
	list := listExpressionSyntax{Dynamic: dynamic, Span: sourceSpan{Start: start, End: start}}
	for !p.at(tokenRBracket) {
		item, err := p.parseExpression()
		if err != nil {
			return listExpressionSyntax{}, err
		}
		list.Items = append(list.Items, item)
		list.Span.End = item.ExpressionSpan().End
		if !p.match(tokenComma) {
			break
		}
	}
	end, err := p.expect(tokenRBracket)
	if err != nil {
		return listExpressionSyntax{}, err
	}
	list.Span.End = end.Span.End
	return list, nil
}

func (p *parser) parseInlineMappingEntry() (mappingEntrySyntax, error) {
	name, nameSpan, err := p.parseDataKey()
	if err != nil {
		return mappingEntrySyntax{}, err
	}
	if _, err := p.expect(tokenColon); err != nil {
		return mappingEntrySyntax{}, err
	}
	value, err := p.parseExpression()
	if err != nil {
		return mappingEntrySyntax{}, err
	}
	return mappingEntrySyntax{
		Name:  name,
		Value: value,
		Span:  sourceSpan{Start: nameSpan.Start, End: value.ExpressionSpan().End},
	}, nil
}

func (p *parser) parseDataKey() (string, sourceSpan, error) {
	if p.at(tokenString) {
		token := p.peek()
		p.pos++
		value, err := strconv.Unquote(token.Text)
		if err != nil {
			return "", sourceSpan{}, &parserError{
				span:    token.Span,
				message: fmt.Sprintf("invalid quoted data key: %v", err),
			}
		}
		return value, token.Span, nil
	}
	if p.at(tokenRawString) || p.at(tokenMultilineString) {
		return "", sourceSpan{}, &parserError{
			span:    p.peek().Span,
			message: "data key must be a bare identifier or quoted string",
		}
	}
	return p.parseLocalIdentifier()
}

func (p *parser) parseDotName() (string, sourceSpan, error) {
	if p.at(tokenString) {
		return "", sourceSpan{}, quotedCoreIdentifierError(p.peek().Span)
	}
	first, err := p.expect(tokenIdentifier)
	if err != nil {
		return "", sourceSpan{}, err
	}
	name := first.Text
	span := first.Span
	for p.match(tokenDot) {
		if p.at(tokenString) {
			return "", sourceSpan{}, quotedCoreIdentifierError(p.peek().Span)
		}
		next, err := p.expect(tokenIdentifier)
		if err != nil {
			return "", sourceSpan{}, err
		}
		name += "." + next.Text
		span.End = next.Span.End
	}
	return name, span, nil
}

func (p *parser) parseSlashName() (string, sourceSpan, error) {
	if p.at(tokenString) {
		return "", sourceSpan{}, quotedCoreIdentifierError(p.peek().Span)
	}
	first, err := p.expect(tokenIdentifier)
	if err != nil {
		return "", sourceSpan{}, err
	}
	name := first.Text
	span := first.Span
	for p.match(tokenSlash) {
		if p.at(tokenString) {
			return "", sourceSpan{}, quotedCoreIdentifierError(p.peek().Span)
		}
		next, err := p.expect(tokenIdentifier)
		if err != nil {
			return "", sourceSpan{}, err
		}
		name += "/" + next.Text
		span.End = next.Span.End
	}
	return name, span, nil
}

func (p *parser) parseLocalIdentifier() (string, sourceSpan, error) {
	if p.at(tokenString) {
		return "", sourceSpan{}, quotedCoreIdentifierError(p.peek().Span)
	}
	token, err := p.expect(tokenIdentifier)
	if err != nil {
		return "", sourceSpan{}, err
	}
	return token.Text, token.Span, nil
}

func quotedCoreIdentifierError(span sourceSpan) *parserError {
	return &parserError{
		span:    span,
		message: "quoted core identifiers are not supported; use an unquoted identifier",
	}
}

func (p *parser) parseDurationToken() (string, sourceSpan, error) {
	token, err := p.expect(tokenDuration)
	if err != nil {
		return "", sourceSpan{}, err
	}
	return token.Text, token.Span, nil
}

func (p *parser) expectBlockStart() error {
	if err := p.expectLineEnd(); err != nil {
		return err
	}
	for {
		p.skipComments()
		if !p.match(tokenNewline) {
			break
		}
	}
	_, err := p.expect(tokenIndent)
	return err
}

func (p *parser) expectLineEnd() error {
	p.skipComments()
	if p.match(tokenNewline) {
		return nil
	}
	if p.at(tokenDedent) || p.at(tokenEOF) {
		return nil
	}
	return p.errorAtCurrent("expected line end")
}

func (p *parser) consumeLineEndIfPresent() bool {
	p.skipComments()
	if p.match(tokenNewline) {
		return true
	}
	return p.at(tokenDedent) || p.at(tokenEOF)
}

func (p *parser) skipIgnorable() {
	for {
		p.skipComments()
		if !p.match(tokenNewline) {
			return
		}
	}
}

func (p *parser) skipComments() {
	for p.pos < len(p.tokens) && p.tokens[p.pos].Kind == tokenComment {
		comment := p.tokens[p.pos]
		p.comments = append(p.comments, commentSyntax{Text: comment.Text, Span: comment.Span})
		p.pos++
	}
}

func (p *parser) canStartCall() bool {
	if !p.at(tokenIdentifier) {
		return false
	}
	next := p.peekOffset(1)
	return next.Kind == tokenDot || next.Kind == tokenLParen
}

func (p *parser) expectKeyword(keyword string) (token, error) {
	token, err := p.expect(tokenIdentifier)
	if err != nil {
		return token, err
	}
	if token.Text != keyword {
		return token, &parserError{
			span:    token.Span,
			message: fmt.Sprintf("expected keyword %q", keyword),
		}
	}
	return token, nil
}

func (p *parser) matchKeyword(keyword string) bool {
	if !p.atKeyword(keyword) {
		return false
	}
	p.pos++
	return true
}

func (p *parser) atKeyword(keyword string) bool {
	return p.at(tokenIdentifier) && p.peek().Text == keyword
}

func (p *parser) atKeywordOffset(offset int, keyword string) bool {
	token := p.peekOffset(offset)
	return token.Kind == tokenIdentifier && token.Text == keyword
}

func (p *parser) expect(kind tokenKind) (token, error) {
	p.skipComments()
	if p.at(kind) {
		token := p.peek()
		p.pos++
		return token, nil
	}
	return token{}, p.errorAtCurrent(fmt.Sprintf("expected %s", kind))
}

func (p *parser) at(kind tokenKind) bool {
	p.skipComments()
	if p.pos >= len(p.tokens) {
		return false
	}
	return p.tokens[p.pos].Kind == kind
}

func (p *parser) match(kind tokenKind) bool {
	if !p.at(kind) {
		return false
	}
	p.pos++
	return true
}

func (p *parser) peek() token {
	p.skipComments()
	if p.pos >= len(p.tokens) {
		return token{Kind: tokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *parser) peekOffset(offset int) token {
	index := p.pos
	skipped := 0
	for index < len(p.tokens) && skipped < offset {
		index++
		for index < len(p.tokens) && p.tokens[index].Kind == tokenComment {
			index++
		}
		skipped++
	}
	if index >= len(p.tokens) {
		return token{Kind: tokenEOF}
	}
	return p.tokens[index]
}

func (p *parser) previous() token {
	return p.tokens[p.pos-1]
}

func (p *parser) previousOffset(offset int) token {
	return p.tokens[p.pos-offset]
}

func (p *parser) errorAtCurrent(message string) error {
	token := p.peek()
	return &parserError{
		span:    token.Span,
		message: message,
	}
}

func literalKindForToken(kind tokenKind) literalExpressionKind {
	switch kind {
	case tokenNumber:
		return literalKindNumber
	case tokenDuration:
		return literalKindDuration
	case tokenRawString:
		return literalKindRawString
	case tokenMultilineString:
		return literalKindMultilineString
	default:
		return literalKindString
	}
}
