package thtr

import (
	"testing"

	"github.com/alex-poliushkin/theater/internal/testkit/errtest"
)

func TestParseTokensParsesCoreScenarioStructure(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke

scenario verify-email(flow_id: string!, email: string!)
  act submit-email
    do action.http(method: "PATCH", url: "/flows")
    expect status: field(status_code) == 200
    export request_id = field(body) | path("/id")
    on pass -> poll

  act poll
    eventually 30s every 1s
    do repeatable action.http
      method: "GET"
      url: "/notifications"
    expect otp: (
      field(body)
      | decode(json)
      | path("/data")
    ) matches r"^[A-Z0-9]{6}$"

call run = verify-email(flow_id: "flow", email: "user@example.test")
  export otp = $otp
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	document, err := parseTokens(tokens)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if got, want := document.Stage.ID, "smoke"; got != want {
		t.Fatalf("stage id mismatch: got %q want %q", got, want)
	}
	if got, want := len(document.Scenarios), 1; got != want {
		t.Fatalf("scenario count mismatch: got %d want %d", got, want)
	}
	scenario := document.Scenarios[0]
	if got, want := scenario.ID, "verify-email"; got != want {
		t.Fatalf("scenario id mismatch: got %q want %q", got, want)
	}
	if got, want := len(scenario.Inputs), 2; got != want {
		t.Fatalf("input count mismatch: got %d want %d", got, want)
	}
	if got, want := len(scenario.Acts), 2; got != want {
		t.Fatalf("act count mismatch: got %d want %d", got, want)
	}
	if scenario.Acts[1].Eventually == nil {
		t.Fatal("second act must include eventually")
	}
	if got, want := len(document.Calls), 1; got != want {
		t.Fatalf("call count mismatch: got %d want %d", got, want)
	}
}

func TestParseTokensParsesTopLevelSections(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke

http
  session browser = http.session.browser()

state
  backend local = state.backend.file(root: "/tmp/theater-state")

scenario login
  act submit
    do action.http(method: "GET")
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	document, err := parseTokens(tokens)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if document.HTTP == nil {
		t.Fatal("http section must be present")
	}
	if got, want := len(document.HTTP.Entries), 1; got != want {
		t.Fatalf("http entry count mismatch: got %d want %d", got, want)
	}
	if document.State == nil {
		t.Fatal("state section must be present")
	}
	if got, want := len(document.State.Entries), 1; got != want {
		t.Fatalf("state entry count mismatch: got %d want %d", got, want)
	}
}

func TestParseTokensParsesStateAliasEntries(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke

state
  backend local = state.backend.file(root: "/tmp/theater-state")
  record shared_meta = state.record(
    backend: local,
    record: "env/shared-meta",
    min_guarantee: local-atomic
  )
  pool otp_identities = state.pool
    backend: local
    pool: "otp-identities"
    min_guarantee: local-atomic

scenario login
  act submit
    do action.http(method: "GET")
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	document, err := parseTokens(tokens)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if document.State == nil {
		t.Fatal("state section must be present")
	}
	if got, want := len(document.State.Entries), 3; got != want {
		t.Fatalf("state entry count mismatch: got %d want %d", got, want)
	}

	if got, want := document.State.Entries[0].Kind, "backend"; got != want {
		t.Fatalf("first state entry kind mismatch: got %q want %q", got, want)
	}
	if got, want := document.State.Entries[1].Kind, "record"; got != want {
		t.Fatalf("second state entry kind mismatch: got %q want %q", got, want)
	}
	if got, want := document.State.Entries[1].ID, "shared_meta"; got != want {
		t.Fatalf("record alias id mismatch: got %q want %q", got, want)
	}
	if got, want := document.State.Entries[1].Call.Name, "state.record"; got != want {
		t.Fatalf("record alias call mismatch: got %q want %q", got, want)
	}
	if got, want := document.State.Entries[2].Kind, "pool"; got != want {
		t.Fatalf("third state entry kind mismatch: got %q want %q", got, want)
	}
	if got, want := document.State.Entries[2].ID, "otp_identities"; got != want {
		t.Fatalf("pool alias id mismatch: got %q want %q", got, want)
	}
	if got, want := document.State.Entries[2].Call.Name, "state.pool"; got != want {
		t.Fatalf("pool alias call mismatch: got %q want %q", got, want)
	}
	if !document.State.Entries[2].Call.BlockForm {
		t.Fatal("pool alias call must preserve block form")
	}
}

func TestParseTokensRetainsCommentTrivia(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
# heading
scenario login
  # act comment
  act submit
    do action.http(method: "GET")
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	document, err := parseTokens(tokens)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if got, want := len(document.Comments), 2; got != want {
		t.Fatalf("comment count mismatch: got %d want %d", got, want)
	}
}

func TestParseTokensDoesNotDuplicateLeadingComments(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
# heading
scenario login
  act submit
    do action.http(method: "GET")
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	document, err := parseTokens(tokens)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if got, want := len(document.Comments), 1; got != want {
		t.Fatalf("comment count mismatch: got %d want %d", got, want)
	}
}

func TestParseTokensPreservesTrailingCommentsOnInlineForms(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET") # action comment

call run = login() # call comment
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	document, err := parseTokens(tokens)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if got, want := len(document.Comments), 2; got != want {
		t.Fatalf("comment count mismatch: got %d want %d", got, want)
	}
}

func TestParseTokensParsesScalarAndUnaryAssertionForms(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET")
    expect body-has-token: field(body) contains "token"
    expect duration-high: field(duration_ms) > 100
    expect server-error: field(status_code) >= 500
    expect retries-ok: field(retry_count) <= 10
    expect retries-range: field(retry_count) between 1 and 5
    expect not-server-error: field(status_code) not >= 500
    expect not-retries-range: field(retry_count) not between 1 and 5
    expect not-custom: field(status_code) not assert plugin.custom(expected: 200)
    expect has-trace: field(headers) has key("X-Trace")
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	document, err := parseTokens(tokens)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	assertions := document.Scenarios[0].Acts[0].Expectations
	if got, want := assertions[0].Assert.Kind, assertionKindContains; got != want {
		t.Fatalf("contains assertion kind mismatch: got %q want %q", got, want)
	}
	if got, want := assertions[1].Assert.Kind, assertionKindGT; got != want {
		t.Fatalf("gt assertion kind mismatch: got %q want %q", got, want)
	}
	if got, want := assertions[2].Assert.Kind, assertionKindGTE; got != want {
		t.Fatalf("gte assertion kind mismatch: got %q want %q", got, want)
	}
	if got, want := assertions[3].Assert.Kind, assertionKindLTE; got != want {
		t.Fatalf("lte assertion kind mismatch: got %q want %q", got, want)
	}
	if got, want := assertions[4].Assert.Kind, assertionKindBetween; got != want {
		t.Fatalf("between assertion kind mismatch: got %q want %q", got, want)
	}
	if got, want := assertions[5].Assert.Kind, assertionKindGTE; got != want {
		t.Fatalf("negated gte assertion kind mismatch: got %q want %q", got, want)
	}
	if assertions[5].Assert.NegationSpan == nil {
		t.Fatal("negated assertion must record negation span")
	}
	if got, want := assertions[6].Assert.Kind, assertionKindBetween; got != want {
		t.Fatalf("negated between assertion kind mismatch: got %q want %q", got, want)
	}
	if assertions[6].Assert.NegationSpan == nil {
		t.Fatal("negated between assertion must record negation span")
	}
	if got, want := assertions[7].Assert.Kind, assertionKindCall; got != want {
		t.Fatalf("negated assert-call kind mismatch: got %q want %q", got, want)
	}
	if assertions[7].Assert.NegationSpan == nil {
		t.Fatal("negated assert-call must record negation span")
	}
	if got, want := assertions[8].Assert.Kind, assertionKindHasKey; got != want {
		t.Fatalf("has key assertion kind mismatch: got %q want %q", got, want)
	}
}

func TestParseTokensRejectsBetweenWithoutAnd(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET")
    expect bad: field(retry_count) between 1
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	_, err = parseTokens(tokens)
	if err == nil {
		t.Fatal("expected parser error, got nil")
	}

	errtest.RequireContains(t, err, `expected "and" after between lower bound`)
}

func TestParseTokensRejectsInvalidHasAssertionCore(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET")
    expect bad: field(headers) has item("X-Trace")
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	_, err = parseTokens(tokens)
	if err == nil {
		t.Fatal("expected parser error, got nil")
	}

	errtest.RequireContains(t, err, `expected "where" after "has item"`)
}

func TestParseTokensRejectsBareNotWithoutAssertionCore(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET")
    expect bad: field(status_code) not
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	_, err = parseTokens(tokens)
	if err == nil {
		t.Fatal("expected parser error, got nil")
	}

	errtest.RequireContains(t, err, "expected assertion core after not")
}

func TestParseTokensParsesCollectionWhereAssertionForms(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET")
    expect has-demo-notification: field(body) | decode(json) | path("/notifications") has item where path("/receiverAddress") == "demo@example.test"
    expect all-recipients-present: field(body) | decode(json) | path("/notifications") all items where (
      path("/receiverAddress") contains "@example.test",
      path("/subject") not assert plugin.custom(expected: "Verification Code")
    )
    expect active-user: field(body) | decode(json) has entry("status") == "active"
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	document, err := parseTokens(tokens)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	assertions := document.Scenarios[0].Acts[0].Expectations
	if got, want := assertions[0].Assert.Kind, assertionKindHasItem; got != want {
		t.Fatalf("has item assertion kind mismatch: got %q want %q", got, want)
	}
	if got, want := len(assertions[0].Assert.Clauses), 1; got != want {
		t.Fatalf("has item clause count mismatch: got %d want %d", got, want)
	}
	if got, want := assertions[0].Assert.Clauses[0].Assert.Kind, assertionKindEqual; got != want {
		t.Fatalf("has item clause assert kind mismatch: got %q want %q", got, want)
	}

	if got, want := assertions[1].Assert.Kind, assertionKindAllItems; got != want {
		t.Fatalf("all items assertion kind mismatch: got %q want %q", got, want)
	}
	if got, want := len(assertions[1].Assert.Clauses), 2; got != want {
		t.Fatalf("all items clause count mismatch: got %d want %d", got, want)
	}
	if got, want := assertions[1].Assert.Clauses[0].Assert.Kind, assertionKindContains; got != want {
		t.Fatalf("first grouped clause assert kind mismatch: got %q want %q", got, want)
	}
	if got, want := assertions[1].Assert.Clauses[1].Assert.Kind, assertionKindCall; got != want {
		t.Fatalf("second grouped clause assert kind mismatch: got %q want %q", got, want)
	}
	if assertions[1].Assert.Clauses[1].Assert.NegationSpan == nil {
		t.Fatal("grouped negated assert-call clause must record negation span")
	}

	if got, want := assertions[2].Assert.Kind, assertionKindHasEntry; got != want {
		t.Fatalf("has entry assertion kind mismatch: got %q want %q", got, want)
	}
	if got, want := assertions[2].Assert.Nested.Kind, assertionKindEqual; got != want {
		t.Fatalf("has entry nested assertion kind mismatch: got %q want %q", got, want)
	}
}

func TestParseTokensRejectsMissingRelativeClauseAfterWhere(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET")
    expect bad: field(body) has item where
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	_, err = parseTokens(tokens)
	if err == nil {
		t.Fatal("expected parser error, got nil")
	}

	errtest.RequireContains(t, err, "expected relative clause after where")
}

func TestParseTokensRejectsMalformedGroupedWhereClauseList(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET")
    expect bad: field(body) all items where (
      path("/receiverAddress") == "demo@example.test"
      path("/subject") contains "Verification Code"
    )
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	_, err = parseTokens(tokens)
	if err == nil {
		t.Fatal("expected parser error, got nil")
	}

	errtest.RequireContains(t, err, `expected "," or ")" after relative clause`)
}

func TestParseTokensRejectsUnsupportedInputType(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login(user: email)
  act submit
    do action.http(method: "GET")
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	_, err = parseTokens(tokens)
	if err == nil {
		t.Fatal("expected parser error, got nil")
	}
	if got := err.Error(); got != `unsupported input type "email"` {
		t.Fatalf("parser error mismatch: got %q", got)
	}
}

func TestParseTokensRejectsActWithoutAction(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    expect status: field(status_code) == 200
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	_, err = parseTokens(tokens)

	errtest.RequireContains(t, err, "act must declare exactly one do action")
}

func TestParseTokensRejectsDuplicateAction(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET")
    do action.http(method: "POST")
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	_, err = parseTokens(tokens)

	errtest.RequireContains(t, err, "act already declares a do action")
}

func TestParseTokensRejectsOutOfOrderActEntries(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    expect status: field(status_code) == 200
    do action.http(method: "GET")
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	_, err = parseTokens(tokens)

	errtest.RequireContains(t, err, expectedActEntryOrderMessage)
}

func TestParseTokensRejectsOutOfOrderActLogEntries(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		source string
	}{
		{
			name: "log before action",
			source: `stage smoke
scenario login
  act submit
    log response = field(status_code)
    do action.http(method: "GET")
`,
		},
		{
			name: "capture auth after log",
			source: `stage smoke
scenario login
  act submit
    do action.http(method: "GET")
    log response = field(status_code)
    capture_auth csrf_auth
`,
		},
		{
			name: "log after expectation",
			source: `stage smoke
scenario login
  act submit
    do action.http(method: "GET")
    expect status: field(status_code) == 200
    log response = field(status_code)
`,
		},
		{
			name: "log after export",
			source: `stage smoke
scenario login
  act submit
    do action.http(method: "GET")
    export status = field(status_code)
    log response = field(status_code)
`,
		},
		{
			name: "log after transition",
			source: `stage smoke
scenario login
  act submit
    do action.http(method: "GET")
    on pass -> done
    log response = field(status_code)
`,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			tokens, err := lex([]byte(testCase.source))
			if err != nil {
				t.Fatalf("lex failed: %v", err)
			}

			_, err = parseTokens(tokens)

			errtest.RequireContains(t, err, expectedActEntryOrderMessage)
		})
	}
}

func TestParseTokensParsesActLogEntries(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET")
    log response = object { status: field(status_code), user_id: field(body) | decode(json) | path("/data/id") }
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	document, err := parseTokens(tokens)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	logs := document.Scenarios[0].Acts[0].Logs
	if got, want := len(logs), 1; got != want {
		t.Fatalf("log count mismatch: got %d want %d", got, want)
	}
	if got, want := logs[0].ID, "response"; got != want {
		t.Fatalf("log id mismatch: got %q want %q", got, want)
	}
}

func TestParseTokensParsesPropertyDecoratorPipeline(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    prop response = inventory.http.get(url: "/csrf") | json.decode
    do action.http(method: "GET")
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	document, err := parseTokens(tokens)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	property := document.Scenarios[0].Acts[0].Properties[0]
	pipeline, ok := property.Value.(pipelineExpressionSyntax)
	if !ok {
		t.Fatalf("property value must be pipeline, got %T", property.Value)
	}
	if got, want := len(pipeline.Steps), 1; got != want {
		t.Fatalf("pipeline step count mismatch: got %d want %d", got, want)
	}
}

func TestParseTokensParsesScenarioCallExports(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET")

call run = login()
  export session_id = $session_id
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	document, err := parseTokens(tokens)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if got, want := len(document.Calls), 1; got != want {
		t.Fatalf("call count mismatch: got %d want %d", got, want)
	}
	if got, want := len(document.Calls[0].Exports), 1; got != want {
		t.Fatalf("call export count mismatch: got %d want %d", got, want)
	}
	ref, ok := document.Calls[0].Exports[0].Value.(refExpressionSyntax)
	if !ok {
		t.Fatalf("call export value must be ref, got %T", document.Calls[0].Exports[0].Value)
	}
	if got, want := ref.Name, "session_id"; got != want {
		t.Fatalf("call export ref mismatch: got %q want %q", got, want)
	}
}

func TestParseTokensParsesActExportAssertion(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET")
    export otp = field(body) | decode(json) | path("/otp") matches r"^[0-9]{6}$"
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	document, err := parseTokens(tokens)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	export := document.Scenarios[0].Acts[0].Exports[0]
	if export.Assert == nil {
		t.Fatal("export assertion must be present")
	}
	if got, want := export.Assert.Kind, assertionKindMatches; got != want {
		t.Fatalf("export assertion kind mismatch: got %q want %q", got, want)
	}
}

func TestParseTokensParsesNamesDependenciesAndCaptureAuth(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
name "Smoke stage"

scenario auth/login(email: string!)
  name "Login scenario"
  act submit
    name "Submit request"
    do action.http(method: "POST", url: "/login")
    capture_auth web
      csrf: response_header("X-CSRF-Token")
      session: response_cookie("session")
  act confirm
    do action.http(method: "GET", url: "/confirm")

call run-login = auth/login(email: "user@example.test")
  name "Run login"
  dependency bootstrap
  dependency provision-user when done
  export session_id = $session_id
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	document, err := parseTokens(tokens)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if document.Stage.Name == nil {
		t.Fatal("stage name must be present")
	}
	stageName, ok := document.Stage.Name.Value.(literalExpressionSyntax)
	if !ok {
		t.Fatalf("stage name value must be string literal, got %T", document.Stage.Name.Value)
	}
	if got, want := stageName.Text, `"Smoke stage"`; got != want {
		t.Fatalf("stage name mismatch: got %q want %q", got, want)
	}

	scenario := document.Scenarios[0]
	if scenario.Name == nil {
		t.Fatal("scenario name must be present")
	}
	if scenario.Acts[0].Name == nil {
		t.Fatal("act name must be present")
	}
	if scenario.Acts[0].CaptureAuth == nil {
		t.Fatal("capture_auth must be present")
	}
	if got, want := scenario.Acts[0].CaptureAuth.Auth, "web"; got != want {
		t.Fatalf("capture_auth auth mismatch: got %q want %q", got, want)
	}
	if got, want := len(scenario.Acts[0].CaptureAuth.Slots), 2; got != want {
		t.Fatalf("capture_auth slot count mismatch: got %d want %d", got, want)
	}
	slotSource, ok := scenario.Acts[0].CaptureAuth.Slots[0].Value.(callExpressionSyntax)
	if !ok {
		t.Fatalf("capture_auth slot source must be call, got %T", scenario.Acts[0].CaptureAuth.Slots[0].Value)
	}
	if got, want := slotSource.Name, "response_header"; got != want {
		t.Fatalf("capture_auth slot source mismatch: got %q want %q", got, want)
	}

	call := document.Calls[0]
	if call.Name == nil {
		t.Fatal("call name must be present")
	}
	if got, want := len(call.Dependencies), 2; got != want {
		t.Fatalf("dependency count mismatch: got %d want %d", got, want)
	}
	if got, want := call.Dependencies[0].CallID, "bootstrap"; got != want {
		t.Fatalf("first dependency id mismatch: got %q want %q", got, want)
	}
	if got, want := call.Dependencies[0].When, ""; got != want {
		t.Fatalf("first dependency predicate mismatch: got %q want %q", got, want)
	}
	if got, want := call.Dependencies[1].When, "done"; got != want {
		t.Fatalf("second dependency predicate mismatch: got %q want %q", got, want)
	}
}

func TestParseTokensRejectsUnsupportedTransitionEvent(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET")
    on maybe -> next
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	_, err = parseTokens(tokens)

	errtest.RequireContains(t, err, `unsupported transition event "maybe"`)
}

func TestParseTokensRejectsScenarioAfterCall(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET")

call run = login()

scenario second
  act submit
    do action.http(method: "GET")
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	_, err = parseTokens(tokens)

	errtest.RequireContains(t, err, "scenario blocks must appear before call blocks")
}

func TestParseTokensRejectsOutOfOrderCallEntries(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET")

call run = login()
  export session_id = $session_id
  dependency bootstrap
`)

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	_, err = parseTokens(tokens)

	errtest.RequireContains(t, err, "call entries must follow name, dependency, export order")
}

func TestParseTokensRejectsMultilineNameLiteral(t *testing.T) {
	t.Parallel()

	source := []byte("stage smoke\nname \"\"\"\n  Smoke stage\n\"\"\"\n")

	tokens, err := lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	_, err = parseTokens(tokens)

	errtest.RequireContains(t, err, "name value must be single-line string literal")
}
