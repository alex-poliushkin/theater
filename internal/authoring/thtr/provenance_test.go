package thtr

import (
	"strings"
	"testing"
)

func TestLowerDocumentWithSourceMapTracksSpecPathAndYAMLRange(t *testing.T) {
	t.Parallel()

	lowered := mustLowerDocumentWithSourceMap(t, `stage registration
scenario register
  act fetch-profile
    prop response_json = inventory.http.get(url: "https://example.test/profile") | json.decode
    do action.generate
      outputs:
        profile_id: $response_json | path("/id")
    expect has-profile-id: field(profile_id) assert plugin.custom(expected: 200)
    export issued_profile_id = field(profile_id)

call register-user = register()
  export final_profile_id = $issued_profile_id
`)

	if !strings.Contains(string(lowered.YAML), "scenario_calls:") {
		t.Fatalf("canonical yaml must include scenario_calls: %q", string(lowered.YAML))
	}

	entry, ok := lowered.SourceMap.LookupSpecPath(
		"stage.registration/scenario.register/act.fetch-profile/property.response_json/inventory/with/binding.url",
	)
	if !ok {
		t.Fatal("inventory binding source map entry must be present")
	}
	if got, want := entry.Source.File, "/tmp/registration.thtr"; got != want {
		t.Fatalf("source file mismatch: got %q want %q", got, want)
	}
	if got, want := entry.Source.StartLine, 4; got != want {
		t.Fatalf("source start line mismatch: got %d want %d", got, want)
	}
	if entry.YAML == nil {
		t.Fatal("yaml range must be present")
	}

	roundtrip, ok := lowered.SourceMap.LookupYAMLPosition(entry.YAML.StartLine, entry.YAML.StartColumn)
	if !ok {
		t.Fatal("yaml position lookup must resolve back to a source entry")
	}
	if got, want := roundtrip.SpecPath, entry.SpecPath; got != want {
		t.Fatalf("yaml lookup path mismatch: got %q want %q", got, want)
	}
}

func TestLowerDocumentWithSourceMapTracksActLogSugar(t *testing.T) {
	t.Parallel()

	lowered := mustLowerDocumentWithSourceMap(t, `stage main
scenario login
  act submit
    do action.http(method: "GET", url: "/login")
    log response = object {
      status: field(status_code),
      user_id: field(body) | decode(json) | path("/data/id")
    }
`)

	logEntry, ok := lowered.SourceMap.LookupSpecPath("stage.main/scenario.login/act.submit/log.response")
	if !ok {
		t.Fatal("log source map entry must be present")
	}
	if got, want := logEntry.Source.StartLine, 5; got != want {
		t.Fatalf("log source line mismatch: got %d want %d", got, want)
	}
	if got, want := logEntry.Source.StartColumn, 5; got != want {
		t.Fatalf("log source column mismatch: got %d want %d", got, want)
	}

	fieldEntry, ok := lowered.SourceMap.LookupSpecPath("stage.main/scenario.login/act.submit/log.response/value.user_id")
	if !ok {
		t.Fatal("log value child source map entry must be present")
	}
	if got, want := fieldEntry.Source.StartLine, 7; got != want {
		t.Fatalf("log child source line mismatch: got %d want %d", got, want)
	}
	if got, want := fieldEntry.Source.StartColumn, 7; got != want {
		t.Fatalf("log child source column mismatch: got %d want %d", got, want)
	}
	if fieldEntry.YAML == nil {
		t.Fatal("log value child yaml range must be present")
	}
}

func TestSourceMapLookupFallsBackToNearestRecordedAncestor(t *testing.T) {
	t.Parallel()

	lowered := mustLowerDocumentWithSourceMap(t, `stage otp-smoke
scenario verify-email(flow_id: string!, email: string!)
  act submit-email
    do action.http
      method: "PATCH"
      url: "https://example.test/flows/${flow_id}/registrations"
`)

	entry, ok := lowered.SourceMap.LookupSpecPath(
		"stage.otp-smoke/scenario.verify-email/act.submit-email/action/binding.url.parts[1]",
	)
	if !ok {
		t.Fatal("fallback source map entry must be present")
	}
	if got, want := entry.SpecPath, "stage.otp-smoke/scenario.verify-email/act.submit-email/action/binding.url"; got != want {
		t.Fatalf("fallback path mismatch: got %q want %q", got, want)
	}
	if got, want := entry.Source.StartLine, 6; got != want {
		t.Fatalf("fallback line mismatch: got %d want %d", got, want)
	}
}

func TestSourceMapRecordsExactThroughStepPath(t *testing.T) {
	t.Parallel()

	lowered := mustLowerDocumentWithSourceMap(t, `stage otp-smoke
scenario verify-email(email: string!)
  act poll-notifications
    do action.http(method: "GET", url: "/notifications")
    expect otp-shape: field(body) | decode(json) | path("/data") | regexp(pattern: r"[A-Z0-9]+", group: 1) matches r"^[A-Z0-9]{6}$"
`)

	entry, ok := lowered.SourceMap.LookupSpecPath(
		"stage.otp-smoke/scenario.verify-email/act.poll-notifications/expectation.otp-shape/subject/through[0]",
	)
	if !ok {
		t.Fatal("through step entry must be present")
	}
	if got, want := entry.SpecPath, "stage.otp-smoke/scenario.verify-email/act.poll-notifications/expectation.otp-shape/subject/through[0]"; got != want {
		t.Fatalf("through step path mismatch: got %q want %q", got, want)
	}
	if got, want := entry.Source.StartLine, 5; got != want {
		t.Fatalf("through step line mismatch: got %d want %d", got, want)
	}
}

func TestSourceMapRecordsTransformThroughArgs(t *testing.T) {
	t.Parallel()

	lowered := mustLowerDocumentWithSourceMap(t, `stage jwt-smoke
scenario inspect
  act request
    do action.http(method: "GET", url: "/token")
    export uid = field(body) | decode(json) | path("/token") | transform.jwt.claims(audience: "mobile") | path("/uid")
`)

	entry, ok := lowered.SourceMap.LookupSpecPath(
		"stage.jwt-smoke/scenario.inspect/act.request/export.uid/through[0]/transform/with/binding.audience",
	)
	if !ok {
		t.Fatal("transform arg source map entry must be present")
	}
	if got, want := entry.Source.StartLine, 5; got != want {
		t.Fatalf("transform arg line mismatch: got %d want %d", got, want)
	}
}

func TestSourceMapRecordsPickWhereClausePaths(t *testing.T) {
	t.Parallel()

	lowered := mustLowerDocumentWithSourceMap(t, `stage otp-smoke
scenario verify-email(email: string!)
  act poll-notifications
    do action.http(method: "GET", url: "/notifications")
    export otp = field(body) | decode(json) | path("/items") | pick where (
      path("/receiverAddress") == $email,
      path("/subject") contains "Verification Code"
    )
`)

	whereEntry, ok := lowered.SourceMap.LookupSpecPath(
		"stage.otp-smoke/scenario.verify-email/act.poll-notifications/export.otp/through[0]/pick/where",
	)
	if !ok {
		t.Fatal("pick where source map entry must be present")
	}
	if got, want := whereEntry.Source.StartLine, 5; got != want {
		t.Fatalf("pick where line mismatch: got %d want %d", got, want)
	}

	subjectEntry, ok := lowered.SourceMap.LookupSpecPath(
		"stage.otp-smoke/scenario.verify-email/act.poll-notifications/export.otp/through[0]/pick/where[0]/subject/path",
	)
	if !ok {
		t.Fatal("pick where subject path source map entry must be present")
	}
	if got, want := subjectEntry.Source.StartLine, 6; got != want {
		t.Fatalf("pick where subject path line mismatch: got %d want %d", got, want)
	}

	assertEntry, ok := lowered.SourceMap.LookupSpecPath(
		"stage.otp-smoke/scenario.verify-email/act.poll-notifications/export.otp/through[0]/pick/where[0]/assert/binding.expected",
	)
	if !ok {
		t.Fatal("pick where assert arg source map entry must be present")
	}
	if got, want := assertEntry.Source.StartLine, 6; got != want {
		t.Fatalf("pick where assert arg line mismatch: got %d want %d", got, want)
	}
}

func TestSourceMapRecordsExportAssertionAsExpectation(t *testing.T) {
	t.Parallel()

	lowered := mustLowerDocumentWithSourceMap(t, `stage otp-smoke
scenario verify-email(email: string!)
  act poll-notifications
    do action.http(method: "GET", url: "/notifications")
    export otp = (
      field(body)
      | decode(json)
      | path("/data")
      | pick where path("/receiverAddress") == $email
      | path("/body")
    ) matches r"^[0-9]{6}$"
`)

	subjectEntry, ok := lowered.SourceMap.LookupSpecPath(
		"stage.otp-smoke/scenario.verify-email/act.poll-notifications/expectation.otp/subject/through[0]/pick/where[0]/subject/path",
	)
	if !ok {
		t.Fatal("export assertion subject source map entry must be present")
	}
	if got, want := subjectEntry.Source.StartLine, 9; got != want {
		t.Fatalf("export assertion subject start line mismatch: got %d want %d", got, want)
	}
	if got, want := subjectEntry.Source.StartColumn, 20; got != want {
		t.Fatalf("export assertion subject start column mismatch: got %d want %d", got, want)
	}
	if got, want := subjectEntry.Source.EndLine, 9; got != want {
		t.Fatalf("export assertion subject end line mismatch: got %d want %d", got, want)
	}
	if got, want := subjectEntry.Source.EndColumn, 44; got != want {
		t.Fatalf("export assertion subject end column mismatch: got %d want %d", got, want)
	}
	patternEntry, ok := lowered.SourceMap.LookupSpecPath(
		"stage.otp-smoke/scenario.verify-email/act.poll-notifications/expectation.otp/assert/binding.pattern",
	)
	if !ok {
		t.Fatal("export assertion pattern source map entry must be present")
	}
	if got, want := patternEntry.Source.StartLine, 11; got != want {
		t.Fatalf("export assertion pattern start line mismatch: got %d want %d", got, want)
	}
	if got, want := patternEntry.Source.StartColumn, 15; got != want {
		t.Fatalf("export assertion pattern start column mismatch: got %d want %d", got, want)
	}
	if got, want := patternEntry.Source.EndLine, 11; got != want {
		t.Fatalf("export assertion pattern end line mismatch: got %d want %d", got, want)
	}
	if got, want := patternEntry.Source.EndColumn, 28; got != want {
		t.Fatalf("export assertion pattern end column mismatch: got %d want %d", got, want)
	}
	if patternEntry.YAML == nil {
		t.Fatal("export assertion pattern must have YAML range")
	}
	if !strings.Contains(string(lowered.YAML), "expectations:\n            - id: otp") {
		t.Fatalf("canonical YAML must include generated expectation: %s", string(lowered.YAML))
	}
	if _, ok := lowered.SourceMap.LookupSpecPath(
		"stage.otp-smoke/scenario.verify-email/act.poll-notifications/export.otp/through[0]/pick/where[0]/subject/path",
	); !ok {
		t.Fatal("export selector source map entry must be present")
	}
}

func TestSourceMapRecordsActionPath(t *testing.T) {
	t.Parallel()

	lowered := mustLowerDocumentWithSourceMap(t, `stage main-v1
scenario auth-login
  act wait-ready
    do action.http(method: "GET", url: "/health")
`)

	entry, ok := lowered.SourceMap.LookupSpecPath("stage.main-v1/scenario.auth-login/act.wait-ready/action")
	if !ok {
		t.Fatal("action path entry must be present")
	}
	if got, want := entry.Source.StartLine, 4; got != want {
		t.Fatalf("action path line mismatch: got %d want %d", got, want)
	}
}

func TestSourceMapRecordsScalarUnaryExpectationArgPaths(t *testing.T) {
	t.Parallel()

	lowered := mustLowerDocumentWithSourceMap(t, `stage smoke
scenario login
  act submit
    do action.http(method: "GET", url: "/health")
    expect page-text: field(body) contains "Example Domain"
    expect not-server-error: field(status_code) not >= 500
    expect not-not-found: field(status_code) != 404
    expect no-error-key: field(body) | decode(json) lacks key("error")
`)

	entry, ok := lowered.SourceMap.LookupSpecPath(
		"stage.smoke/scenario.login/act.submit/expectation.page-text/assert/binding.expected",
	)
	if !ok {
		t.Fatal("scalar expectation arg source map entry must be present")
	}
	if got, want := entry.Source.StartLine, 5; got != want {
		t.Fatalf("scalar expectation arg line mismatch: got %d want %d", got, want)
	}

	negatedEntry, ok := lowered.SourceMap.LookupSpecPath(
		"stage.smoke/scenario.login/act.submit/expectation.not-server-error/assert/binding.assert/binding.args/binding.expected",
	)
	if !ok {
		t.Fatal("negated expectation arg source map entry must be present")
	}
	if got, want := negatedEntry.Source.StartLine, 6; got != want {
		t.Fatalf("negated expectation arg line mismatch: got %d want %d", got, want)
	}

	notEqualEntry, ok := lowered.SourceMap.LookupSpecPath(
		"stage.smoke/scenario.login/act.submit/expectation.not-not-found/assert/binding.assert/binding.args/binding.expected",
	)
	if !ok {
		t.Fatal("not-equal expectation arg source map entry must be present")
	}
	if got, want := notEqualEntry.Source.StartLine, 7; got != want {
		t.Fatalf("not-equal expectation arg line mismatch: got %d want %d", got, want)
	}

	lacksKeyEntry, ok := lowered.SourceMap.LookupSpecPath(
		"stage.smoke/scenario.login/act.submit/expectation.no-error-key/assert/binding.key",
	)
	if !ok {
		t.Fatal("lacks-key expectation arg source map entry must be present")
	}
	if got, want := lacksKeyEntry.Source.StartLine, 8; got != want {
		t.Fatalf("lacks-key expectation arg line mismatch: got %d want %d", got, want)
	}
}

func TestSourceMapRecordsNegatedAssertCallArgPaths(t *testing.T) {
	t.Parallel()

	lowered := mustLowerDocumentWithSourceMap(t, `stage smoke
scenario login
  act submit
    do action.http(method: "GET", url: "/health")
    expect not-custom-error: field(status_code) not assert plugin.custom(expected: 500)
`)

	entry, ok := lowered.SourceMap.LookupSpecPath(
		"stage.smoke/scenario.login/act.submit/expectation.not-custom-error/assert/binding.assert/binding.args/binding.expected",
	)
	if !ok {
		t.Fatal("negated assert-call arg source map entry must be present")
	}
	if got, want := entry.Source.StartLine, 5; got != want {
		t.Fatalf("negated assert-call arg line mismatch: got %d want %d", got, want)
	}
}

func TestSourceMapRecordsCollectionWhereClausePaths(t *testing.T) {
	t.Parallel()

	lowered := mustLowerDocumentWithSourceMap(t, `stage smoke
scenario login
  act submit
    do action.http(method: "GET", url: "/health")
    expect has-demo-notification: field(body) | decode(json) | path("/notifications") has item where path("/receiverAddress") == "demo@example.test"
    expect all-recipients-present: field(body) | decode(json) | path("/notifications") all items where (
      path("/receiverAddress") contains "@example.test",
      path("/subject") not assert plugin.custom(expected: "Verification Code")
    )
    expect active-user: field(body) | decode(json) has entry("status") == "active"
`)

	subjectEntry, ok := lowered.SourceMap.LookupSpecPath(
		"stage.smoke/scenario.login/act.submit/expectation.has-demo-notification/assert/binding.where/binding.item-0/binding.subject/binding.path",
	)
	if !ok {
		t.Fatal("collection clause subject path source map entry must be present")
	}
	if got, want := subjectEntry.Source.StartLine, 5; got != want {
		t.Fatalf("collection clause subject path line mismatch: got %d want %d", got, want)
	}

	whereEntry, ok := lowered.SourceMap.LookupSpecPath(
		"stage.smoke/scenario.login/act.submit/expectation.all-recipients-present/assert/binding.where",
	)
	if !ok {
		t.Fatal("collection where list source map entry must be present")
	}
	if got, want := whereEntry.Source.StartLine, 6; got != want {
		t.Fatalf("collection where list line mismatch: got %d want %d", got, want)
	}

	assertEntry, ok := lowered.SourceMap.LookupSpecPath(
		"stage.smoke/scenario.login/act.submit/expectation.all-recipients-present/assert/binding.where/binding.item-1/binding.assert/binding.args/binding.assert/binding.args/binding.expected",
	)
	if !ok {
		t.Fatal("negated collection clause assert path source map entry must be present")
	}
	if got, want := assertEntry.Source.StartLine, 8; got != want {
		t.Fatalf("negated collection clause assert path line mismatch: got %d want %d", got, want)
	}

	keyEntry, ok := lowered.SourceMap.LookupSpecPath(
		"stage.smoke/scenario.login/act.submit/expectation.active-user/assert/binding.key",
	)
	if !ok {
		t.Fatal("has entry key source map entry must be present")
	}
	if got, want := keyEntry.Source.StartLine, 10; got != want {
		t.Fatalf("has entry key line mismatch: got %d want %d", got, want)
	}
	if got, want := keyEntry.Source.StartColumn, 62; got != want {
		t.Fatalf("has entry key start column mismatch: got %d want %d", got, want)
	}
	if got, want := keyEntry.Source.EndLine, 10; got != want {
		t.Fatalf("has entry key end line mismatch: got %d want %d", got, want)
	}
	if got, want := keyEntry.Source.EndColumn, 70; got != want {
		t.Fatalf("has entry key end column mismatch: got %d want %d", got, want)
	}

	nestedEntry, ok := lowered.SourceMap.LookupSpecPath(
		"stage.smoke/scenario.login/act.submit/expectation.active-user/assert/binding.assert/binding.args/binding.expected",
	)
	if !ok {
		t.Fatal("has entry nested assert arg source map entry must be present")
	}
	if got, want := nestedEntry.Source.StartLine, 10; got != want {
		t.Fatalf("has entry nested assert arg line mismatch: got %d want %d", got, want)
	}
	if got, want := nestedEntry.Source.StartColumn, 75; got != want {
		t.Fatalf("has entry nested assert arg start column mismatch: got %d want %d", got, want)
	}
	if got, want := nestedEntry.Source.EndLine, 10; got != want {
		t.Fatalf("has entry nested assert arg end line mismatch: got %d want %d", got, want)
	}
	if got, want := nestedEntry.Source.EndColumn, 83; got != want {
		t.Fatalf("has entry nested assert arg end column mismatch: got %d want %d", got, want)
	}
}

func TestSourceMapRecordsTransitionPath(t *testing.T) {
	t.Parallel()

	lowered := mustLowerDocumentWithSourceMap(t, `stage main
scenario login
  act submit
    do action.http(method: "POST", url: "/login")
    on pass -> confirm
  act confirm
    do action.http(method: "GET", url: "/confirm")
`)

	entry, ok := lowered.SourceMap.LookupSpecPath("stage.main/scenario.login/act.submit/transition[0]/to")
	if !ok {
		t.Fatal("transition target source map entry must be present")
	}
	if got, want := entry.SpecPath, "stage.main/scenario.login/act.submit/transition[0]/to"; got != want {
		t.Fatalf("transition target path mismatch: got %q want %q", got, want)
	}
	if got, want := entry.Source.StartLine, 5; got != want {
		t.Fatalf("transition target line mismatch: got %d want %d", got, want)
	}
}

func TestSourceMapRecordsCaptureAuthSlotPath(t *testing.T) {
	t.Parallel()

	lowered := mustLowerDocumentWithSourceMap(t, `stage main

http
  auth web = http.auth(
    attach: list [
      object { header_slot: object { name: "X-CSRF-Token", slot: "csrf" } },
    ],
  )

scenario login
  act submit
    do action.http(method: "POST", url: "/login")
    capture_auth web
      csrf: json_pointer("/data/csrf")
`)

	entry, ok := lowered.SourceMap.LookupSpecPath("stage.main/scenario.login/act.submit/capture_auth/slot.csrf")
	if !ok {
		t.Fatal("capture_auth slot source map entry must be present")
	}
	if got, want := entry.Source.StartLine, 14; got != want {
		t.Fatalf("capture_auth slot line mismatch: got %d want %d", got, want)
	}
}

func TestSourceMapLookupSupportsMultilineYAMLScalarRanges(t *testing.T) {
	t.Parallel()

	lowered := mustLowerDocumentWithSourceMap(t, `stage main
scenario login
  act submit
    do action.http
      method: "POST"
      url: """
        /line-one
        /line-two
      """
`)

	entry, ok := lowered.SourceMap.LookupSpecPath("stage.main/scenario.login/act.submit/action/binding.url")
	if !ok {
		t.Fatal("multiline binding source map entry must be present")
	}
	if entry.YAML == nil {
		t.Fatal("multiline binding yaml range must be present")
	}
	if entry.YAML.EndLine <= entry.YAML.StartLine {
		t.Fatalf("multiline binding yaml range must span lines: got %+v", *entry.YAML)
	}

	roundtrip, ok := lowered.SourceMap.LookupYAMLPosition(entry.YAML.EndLine, entry.YAML.EndColumn)
	if !ok {
		t.Fatal("yaml position lookup must resolve multiline binding range")
	}
	if got, want := roundtrip.SpecPath, entry.SpecPath; got != want {
		t.Fatalf("multiline yaml lookup path mismatch: got %q want %q", got, want)
	}

	lines := strings.Split(strings.TrimSuffix(string(lowered.YAML), "\n"), "\n")
	if got, want := entry.YAML.EndColumn, len(lines[entry.YAML.EndLine-1]); got != want {
		t.Fatalf("multiline yaml end column mismatch: got %d want %d", got, want)
	}
}

func TestSourceMapLookupSupportsMultilineYAMLScalarRangesWithLeadingSpace(t *testing.T) {
	t.Parallel()

	lowered := mustLowerDocumentWithSourceMap(t, `stage main
scenario login
  act submit
    do action.http
      url: """
         leading
        plain
      """
`)

	entry, ok := lowered.SourceMap.LookupSpecPath("stage.main/scenario.login/act.submit/action/binding.url")
	if !ok {
		t.Fatal("multiline binding source map entry must be present")
	}
	if entry.YAML == nil {
		t.Fatal("multiline binding yaml range must be present")
	}

	lines := strings.Split(strings.TrimSuffix(string(lowered.YAML), "\n"), "\n")
	if got, want := entry.YAML.EndColumn, len(lines[entry.YAML.EndLine-1]); got != want {
		t.Fatalf("multiline yaml end column mismatch: got %d want %d", got, want)
	}

	roundtrip, ok := lowered.SourceMap.LookupYAMLPosition(entry.YAML.EndLine, entry.YAML.EndColumn)
	if !ok {
		t.Fatal("yaml position lookup must resolve multiline binding range")
	}
	if got, want := roundtrip.SpecPath, entry.SpecPath; got != want {
		t.Fatalf("multiline yaml lookup path mismatch: got %q want %q", got, want)
	}
}

func mustLowerDocumentWithSourceMap(t *testing.T, source string) loweredDocument {
	t.Helper()

	tokens, err := lex([]byte(source))
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	document, err := parseTokens(tokens)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	lowered, err := lowerDocumentWithSourceMap(document, "/tmp/registration.thtr")
	if err != nil {
		failWithSpan(t, "lower", err)
	}

	return lowered
}
