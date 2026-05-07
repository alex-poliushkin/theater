package thtr

type syntaxDocument struct {
	Stage     stageSyntax
	HTTP      *stageSectionSyntax
	State     *stageSectionSyntax
	Scenarios []scenarioSyntax
	Calls     []scenarioCallSyntax
	Comments  []commentSyntax
	Span      sourceSpan
}

type stageSectionSyntax struct {
	Name    string
	Entries []stageSectionEntrySyntax
	Span    sourceSpan
}

type stageSectionEntrySyntax struct {
	Kind string
	ID   string
	Call callExpressionSyntax
	Span sourceSpan
}

type commentSyntax struct {
	Text string
	Span sourceSpan
}

type stageSyntax struct {
	ID   string
	Name *nameSyntax
	Span sourceSpan
}

type scenarioSyntax struct {
	ID     string
	Name   *nameSyntax
	Inputs []inputSyntax
	Acts   []actSyntax
	Span   sourceSpan
}

type inputSyntax struct {
	Name     string
	Type     string
	Required bool
	Span     sourceSpan
}

type actSyntax struct {
	ID           string
	Name         *nameSyntax
	Eventually   *eventuallySyntax
	Properties   []propertySyntax
	Action       *actionSyntax
	CaptureAuth  *captureAuthSyntax
	Logs         []logSyntax
	Expectations []expectationSyntax
	Exports      []exportSyntax
	Transitions  []transitionSyntax
	Span         sourceSpan
}

type eventuallySyntax struct {
	Timeout  string
	Interval string
	Span     sourceSpan
}

type propertySyntax struct {
	Name  string
	Value expressionSyntax
	Span  sourceSpan
}

type actionSyntax struct {
	Repeatable bool
	Call       callExpressionSyntax
	Span       sourceSpan
}

type logSyntax struct {
	ID    string
	Value expressionSyntax
	Span  sourceSpan
}

type expectationSyntax struct {
	ID      string
	Subject expressionSyntax
	Assert  assertionSyntax
	Span    sourceSpan
}

type relativeClauseSyntax struct {
	Subject expressionSyntax
	Assert  assertionSyntax
	Span    sourceSpan
}

type assertionSyntax struct {
	Kind         assertionKind
	NegationSpan *sourceSpan
	Value        expressionSyntax
	SecondValue  expressionSyntax
	Nested       *assertionSyntax
	WhereSpan    sourceSpan
	Clauses      []relativeClauseSyntax
	Span         sourceSpan
}

type assertionKind string

const (
	assertionKindEqual    assertionKind = "equal"
	assertionKindNotEqual assertionKind = "not_equal"
	assertionKindMatches  assertionKind = "matches"
	assertionKindContains assertionKind = "contains"
	assertionKindPresent  assertionKind = "present"
	assertionKindNull     assertionKind = "null"
	assertionKindNotNull  assertionKind = "not_null"
	assertionKindGT       assertionKind = "gt"
	assertionKindGTE      assertionKind = "gte"
	assertionKindLT       assertionKind = "lt"
	assertionKindLTE      assertionKind = "lte"
	assertionKindBetween  assertionKind = "between"
	assertionKindHasItem  assertionKind = "has_item"
	assertionKindAllItems assertionKind = "all_items"
	assertionKindHasKey   assertionKind = "has_key"
	assertionKindHasEntry assertionKind = "has_entry"
	assertionKindLacksKey assertionKind = "lacks_key"
	assertionKindCall     assertionKind = "call"
)

type exportSyntax struct {
	Name   string
	Value  expressionSyntax
	Assert *assertionSyntax
	Span   sourceSpan
}

type transitionSyntax struct {
	Event string
	To    string
	Span  sourceSpan
}

type scenarioCallSyntax struct {
	ID           string
	Name         *nameSyntax
	ScenarioID   string
	Bindings     []callArgumentSyntax
	Dependencies []dependencySyntax
	Exports      []exportSyntax
	Span         sourceSpan
}

type nameSyntax struct {
	Value expressionSyntax
	Span  sourceSpan
}

type dependencySyntax struct {
	CallID string
	When   string
	Span   sourceSpan
}

type captureAuthSyntax struct {
	Auth  string
	Slots []mappingEntrySyntax
	Span  sourceSpan
}

type callArgumentSyntax struct {
	Name    string
	Value   expressionSyntax
	Mapping []mappingEntrySyntax
	Span    sourceSpan
}

type mappingEntrySyntax struct {
	Name    string
	Value   expressionSyntax
	Mapping []mappingEntrySyntax
	Span    sourceSpan
}

type expressionSyntax interface {
	expressionNode()
	ExpressionSpan() sourceSpan
}

type literalExpressionSyntax struct {
	Kind literalExpressionKind
	Text string
	Span sourceSpan
}

type literalExpressionKind string

const (
	literalKindNumber          literalExpressionKind = "number"
	literalKindDuration        literalExpressionKind = "duration"
	literalKindString          literalExpressionKind = "string"
	literalKindRawString       literalExpressionKind = "raw_string"
	literalKindMultilineString literalExpressionKind = "multiline_string"
	literalKindBool            literalExpressionKind = "bool"
	literalKindNull            literalExpressionKind = "null"
)

type symbolExpressionSyntax struct {
	Name string
	Span sourceSpan
}

type refExpressionSyntax struct {
	Name string
	Span sourceSpan
}

type callExpressionSyntax struct {
	Name      string
	Args      []callArgumentSyntax
	WhereSpan sourceSpan
	Clauses   []relativeClauseSyntax
	BlockForm bool
	Span      sourceSpan
}

type pipelineExpressionSyntax struct {
	Base  expressionSyntax
	Steps []callExpressionSyntax
	Span  sourceSpan
}

type objectExpressionSyntax struct {
	Dynamic bool
	Fields  []mappingEntrySyntax
	Span    sourceSpan
}

type listExpressionSyntax struct {
	Dynamic bool
	Items   []expressionSyntax
	Span    sourceSpan
}

type groupedExpressionSyntax struct {
	Inner expressionSyntax
	Span  sourceSpan
}

func (literalExpressionSyntax) expressionNode() {}
func (v literalExpressionSyntax) ExpressionSpan() sourceSpan {
	return v.Span
}

func (symbolExpressionSyntax) expressionNode() {}
func (v symbolExpressionSyntax) ExpressionSpan() sourceSpan {
	return v.Span
}

func (refExpressionSyntax) expressionNode() {}
func (v refExpressionSyntax) ExpressionSpan() sourceSpan {
	return v.Span
}

func (callExpressionSyntax) expressionNode() {}
func (v callExpressionSyntax) ExpressionSpan() sourceSpan {
	return v.Span
}

func (pipelineExpressionSyntax) expressionNode() {}
func (v pipelineExpressionSyntax) ExpressionSpan() sourceSpan {
	return v.Span
}

func (objectExpressionSyntax) expressionNode() {}
func (v objectExpressionSyntax) ExpressionSpan() sourceSpan {
	return v.Span
}

func (listExpressionSyntax) expressionNode() {}
func (v listExpressionSyntax) ExpressionSpan() sourceSpan {
	return v.Span
}

func (groupedExpressionSyntax) expressionNode() {}
func (v groupedExpressionSyntax) ExpressionSpan() sourceSpan {
	return v.Span
}
