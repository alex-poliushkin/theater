package thtr

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/builtin"
	builtinexpectation "github.com/alex-poliushkin/theater/builtin/expectation"
	"github.com/alex-poliushkin/theater/internal/testkit"
	"github.com/alex-poliushkin/theater/internal/testkit/errtest"
)

func TestDecodeLowersStage(t *testing.T) {
	t.Parallel()

	spec, err := Decode(strings.NewReader(`stage smoke
scenario ping
  act get-health
    do action.http(method: "GET", url: "/health")
    expect status: field(status_code) == 200
`), nil)
	if err != nil {
		failWithSpan(t, "decode", err)
	}

	if got, want := spec.ID, "smoke"; got != want {
		t.Fatalf("stage id mismatch: got %q want %q", got, want)
	}
	if got, want := spec.Scenarios[0].Acts[0].Expectations[0].Assert.Ref, builtinexpectation.EqualRef; got != want {
		t.Fatalf("assert ref mismatch: got %q want %q", got, want)
	}
	expectedBinding := spec.Scenarios[0].Acts[0].Expectations[0].Assert.Args["expected"]
	if expectedBinding.SourceSpan == nil {
		t.Fatal("expectation binding source span must be present")
	}
	if expectedBinding.SourceSpan.Line == 0 || expectedBinding.SourceSpan.Column == 0 {
		t.Fatalf("expectation binding source span must include line and column: %#v", expectedBinding.SourceSpan)
	}
}

func TestParseLowersActLogSugarToLogSpec(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET", url: "/login")
    log response = object {
      status: field(status_code),
      user_id: field(body) | decode(json) | path("/data/id")
    }
    log status = field(status_code)
    log request = $request_id
    log audit = list [ field(status_code), $request_id ]
call run = login(request_id: "req-123")
`), nil)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	logs := spec.Scenarios[0].Acts[0].Logs
	if got, want := len(logs), 4; got != want {
		t.Fatalf("log count mismatch: got %d want %d", got, want)
	}

	log := logs[0]
	if got, want := log.ID, "response"; got != want {
		t.Fatalf("log id mismatch: got %q want %q", got, want)
	}
	if got, want := log.Capture, theater.CaptureSummary; got != want {
		t.Fatalf("log capture mismatch: got %q want %q", got, want)
	}
	if got, want := log.Sensitivity, theater.SensitivityInternal; got != want {
		t.Fatalf("log sensitivity mismatch: got %q want %q", got, want)
	}
	if log.SourceSpan == nil {
		t.Fatal("log source span must be present")
	}
	if got, want := log.Value.Object["status"].Field, "status_code"; got != want {
		t.Fatalf("status field mismatch: got %q want %q", got, want)
	}
	userID := log.Value.Object["user_id"]
	if got, want := userID.Field, "body"; got != want {
		t.Fatalf("user id field mismatch: got %q want %q", got, want)
	}
	if got, want := userID.Decode, theater.DecodeJSON; got != want {
		t.Fatalf("user id decode mismatch: got %q want %q", got, want)
	}
	if got, want := userID.Path, theater.JSONPointer("/data/id"); got != want {
		t.Fatalf("user id path mismatch: got %q want %q", got, want)
	}
	if got, want := logs[1].Value.Field, "status_code"; got != want {
		t.Fatalf("direct field log mismatch: got %q want %q", got, want)
	}
	if got, want := logs[2].Value.Ref, "request_id"; got != want {
		t.Fatalf("direct ref log mismatch: got %q want %q", got, want)
	}
	if got, want := logs[3].Value.List[0].Field, "status_code"; got != want {
		t.Fatalf("list field log mismatch: got %q want %q", got, want)
	}
	if got, want := logs[3].Value.List[1].Ref, "request_id"; got != want {
		t.Fatalf("list ref log mismatch: got %q want %q", got, want)
	}
}

func TestParseRejectsUnsupportedActLogValue(t *testing.T) {
	t.Parallel()

	_, err := Parse([]byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET", url: "/login")
    log response = "static text"
call run = login()
`), nil)

	errtest.RequireContains(t, err, "log value must start with field(...), $ref, object, or list")
}

func TestParseDetailedRewritesActLogValidationDiagnostics(t *testing.T) {
	t.Parallel()

	const sourceFile = "/tmp/logs.thtr"
	result, err := ParseDetailed([]byte(`stage smoke
scenario login
  act submit
    do action.test()
    log response = object {
      status: field(status_code),
      missing: field(body)
    }
call run = login()
`), sourceFile, nil)
	if err != nil {
		t.Fatalf("parse detailed failed: %v", err)
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.test", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"status_code": {Kind: theater.ValueKindNumber},
			},
		},
	}); err != nil {
		t.Fatalf("register action: %v", err)
	}

	validator := theater.NewValidator(catalog, nil)
	diagnostics := result.RewriteDiagnostics(validator.Validate(result.Spec))
	diagnostic := findDiagnosticByCodeValue(diagnostics, "unknown_log_field")
	if diagnostic == nil {
		t.Fatalf("expected unknown_log_field diagnostic, got %#v", diagnostics)
	}
	if got, want := diagnostic.Span.File, sourceFile; got != want {
		t.Fatalf("diagnostic file mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 7; got != want {
		t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
	}
}

func TestParseLowersStage(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage smoke
scenario ping
  act get-health
    do action.http(method: "GET", url: "/health")
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	if got, want := spec.Scenarios[0].Acts[0].Action.Use, "action.http"; got != want {
		t.Fatalf("action use mismatch: got %q want %q", got, want)
	}
}

func TestParseLowersGenerateCallToCanonicalGeneratorRef(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage smoke
scenario login
  act submit
    do action.http(method: "POST", url: "/login")

call run = login(
  email: generate.email(domain: "example.test"),
  external_id: generate.acme.identity.email(domain: "example.test"),
)
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	bindings := spec.ScenarioCalls[0].Bindings
	if got, want := bindings["email"].Generator, "email"; got != want {
		t.Fatalf("built-in generator ref mismatch: got %q want %q", got, want)
	}
	if got, want := bindings["external_id"].Generator, "acme.identity.email"; got != want {
		t.Fatalf("dotted generator ref mismatch: got %q want %q", got, want)
	}
}

func TestLoadFileLowersStage(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "smoke.thtr")
	if err := os.WriteFile(path, []byte(`stage smoke
scenario ping
  act get-health
    do action.http(method: "GET", url: "/health")
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	spec, err := LoadFile(path, nil)
	if err != nil {
		failWithSpan(t, "load file", err)
	}

	if spec.Scenarios[0].Acts[0].SourceSpan == nil {
		t.Fatal("act source span must be present")
	}
	if got, want := spec.Scenarios[0].Acts[0].SourceSpan.File, path; got != want {
		t.Fatalf("source file mismatch: got %q want %q", got, want)
	}

	methodBinding := spec.Scenarios[0].Acts[0].Action.With["method"]
	if methodBinding.SourceSpan == nil {
		t.Fatal("action binding source span must be present")
	}
	if got, want := methodBinding.SourceSpan.File, path; got != want {
		t.Fatalf("action binding source file mismatch: got %q want %q", got, want)
	}
	if methodBinding.SourceSpan.Line == 0 || methodBinding.SourceSpan.Column == 0 {
		t.Fatalf("action binding source span must include line and column: %#v", methodBinding.SourceSpan)
	}
}

func TestLoadFileBindsNestedAuthoringSourceSpans(t *testing.T) {
	t.Parallel()

	source := `stage smoke
scenario login(email: string!, expected_email: string!)
  act submit
    prop profile = inventory.http.get(url: "/profile-source")
    do action.http
      json: object {
        profile: object {
          email: $email,
          tags: list ["mailhog-source", generate.email(domain: "tag.example")],
        },
        subject: string("hello source ", $email),
      }
    expect token: field(body) == object { value: "ok-source" }
    export matched_by_equals = field(body) | decode(json) | path("/items") | pick(at: "/receiverAddress", equals: $email)
    export matched_by_where = field(body) | decode(json) | path("/items") | pick where (
      path("/receiverAddress") == $expected_email
    )

call run-login = login(
  email: generate.email(domain: "call.example"),
  expected_email: "demo@example.test",
)
`
	path := filepath.Join(t.TempDir(), "source-spans.thtr")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	spec, err := LoadFile(path, nil)
	if err != nil {
		failWithSpan(t, "load file", err)
	}

	act := spec.Scenarios[0].Acts[0]
	requireTHSourceSpanAt(t, act.Properties["profile"].Inventory.With["url"].SourceSpan, path, source, `url: "/profile-source"`)

	jsonBinding := act.Action.With["json"]
	requireTHSourceSpanAt(t, jsonBinding.SourceSpan, path, source, `json: object`)
	requireTHSourceSpanAt(t, jsonBinding.Object["profile"].SourceSpan, path, source, `profile: object`)
	requireTHSourceSpanAt(t, jsonBinding.Object["profile"].Object["email"].SourceSpan, path, source, `email: $email`)
	tags := jsonBinding.Object["profile"].Object["tags"]
	requireTHSourceSpanAt(t, tags.SourceSpan, path, source, `tags: list`)
	requireTHSourceSpanAt(t, tags.List[0].SourceSpan, path, source, `"mailhog-source"`)
	requireTHSourceSpanAt(t, tags.List[1].SourceSpan, path, source, `generate.email(domain: "tag.example")`)
	requireTHSourceSpanAt(t, tags.List[1].Args["domain"].SourceSpan, path, source, `domain: "tag.example"`)
	subject := jsonBinding.Object["subject"]
	requireTHSourceSpanAt(t, subject.SourceSpan, path, source, `subject: string`)
	requireTHSourceSpanAt(t, subject.Parts[0].SourceSpan, path, source, `"hello source "`)

	expected := act.Expectations[0].Assert.Args["expected"]
	requireTHSourceSpanAt(t, expected.SourceSpan, path, source, `object { value: "ok-source" }`)
	requireTHSourceSpanAt(t, expected.Object["value"].SourceSpan, path, source, `value: "ok-source"`)

	equals := act.Exports[0].Through[0].Pick.Equals
	requireTHSourceSpanAt(t, equals.SourceSpan, path, source, `equals: $email`)
	whereExpected := act.Exports[1].Through[0].Pick.Where[0].Assert.Args["expected"]
	requireTHSourceSpanAt(t, whereExpected.SourceSpan, path, source, `$expected_email`)

	callEmail := spec.ScenarioCalls[0].Bindings["email"]
	requireTHSourceSpanAt(t, callEmail.SourceSpan, path, source, `email: generate.email(domain: "call.example")`)
	requireTHSourceSpanAt(t, callEmail.Args["domain"].SourceSpan, path, source, `domain: "call.example"`)
	requireTHSourceSpanAt(t, spec.ScenarioCalls[0].Bindings["expected_email"].SourceSpan, path, source, `expected_email: "demo@example.test"`)
}

func TestLoadFlowFileLoadsRepoAwareFlow(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "smoke.thtr"), `stage smoke

call login-user = auth/login()
`)
	writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.thtr"), `stage auth-lib

scenario auth/login
  act submit
    do action.http()
`)

	spec, err := LoadFlowFile(flowPath, nil)
	if err != nil {
		t.Fatalf("load flow file failed: %v", err)
	}

	if got, want := len(spec.Scenarios), 1; got != want {
		t.Fatalf("scenario count mismatch: got %d want %d", got, want)
	}
}

func TestLoadFlowFileDetailedRewritesValidationDiagnosticsFromLibraryFile(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	flowPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "smoke.thtr"), `stage smoke

call login-user = auth/login()
`)
	libraryPath := writeFlowLoaderFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.thtr"), `stage auth-lib

scenario auth/login
  act submit
    eventually 1s every 1s
    do repeatable action.http(method: "GET", url: "/health")
`)

	result, err := LoadFlowFileDetailed(flowPath, nil)
	if err != nil {
		t.Fatalf("load flow file detailed failed: %v", err)
	}

	validator := theater.NewValidator(nil, nil)
	diagnostics := result.RewriteDiagnostics(validator.Validate(result.Spec))
	diagnostic := findDiagnosticByCodeValue(diagnostics, "invalid_eventually_interval")
	if diagnostic == nil {
		t.Fatalf("expected invalid_eventually_interval diagnostic, got %#v", diagnostics)
	}
	if got, want := diagnostic.Span.File, libraryPath; got != want {
		t.Fatalf("diagnostic source file mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 5; got != want {
		t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
	}
}

func TestLoadFileDetailedRewritesValidationDiagnostics(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.thtr")
	if err := os.WriteFile(path, []byte(`stage main
scenario login
  act submit
    eventually 1s every 1s
    do repeatable action.http(method: "GET", url: "/health")
    expect status: field(status_code) == 200
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	result, err := LoadFileDetailed(path, nil)
	if err != nil {
		t.Fatalf("load file detailed failed: %v", err)
	}

	validator := theater.NewValidator(nil, nil)
	diagnostics := result.RewriteDiagnostics(validator.Validate(result.Spec))
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostics[0].Code, "invalid_eventually_interval"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostics[0].Span.File, path; got != want {
		t.Fatalf("diagnostic source file mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostics[0].Span.Line, 4; got != want {
		t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
	}
}

func TestLoadFileDetailedReturnsParseDiagnosticError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.thtr")
	if err := os.WriteFile(path, []byte("stage main\nscenario\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := LoadFileDetailed(path, nil)
	if err == nil {
		t.Fatal("expected parse diagnostic error, got nil")
	}

	var diagnosticError *DiagnosticError
	if !errors.As(err, &diagnosticError) {
		t.Fatalf("expected diagnostic error, got %T", err)
	}

	diagnostic := diagnosticError.Diagnostic()
	if got, want := diagnostic.Code, "thtr_parse_error"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Path, path; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 2; got != want {
		t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
	}
}

func TestLoadFileDetailedReturnsLoweringDiagnosticBreadcrumb(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.thtr")
	if err := os.WriteFile(path, []byte(`stage main
scenario login
  act submit
    do action.http(method: "GET", url: "/health")

call login-once = login()
  export token = field(body)
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := LoadFileDetailed(path, nil)
	if err == nil {
		t.Fatal("expected lowering diagnostic error, got nil")
	}

	var diagnosticError *DiagnosticError
	if !errors.As(err, &diagnosticError) {
		t.Fatalf("expected diagnostic error, got %T", err)
	}

	diagnostic := diagnosticError.Diagnostic()
	if got, want := diagnostic.Code, "thtr_lower_error"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Path, "stage.main/call.login-once/export.token"; got != want {
		t.Fatalf("diagnostic breadcrumb path mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 7; got != want {
		t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
	}
}

func TestParseLowersHTTPStatePropertiesAndExports(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage registration

http
  session browser = http.session.browser()
  auth ci_api = http.auth(
    attach: list [
      object { bearer: object { token: "static-token" } },
      object { api_key: object { in: query, name: "api_key", value: "query-token" } },
    ],
  )
  identity admin = http.identity(session: browser, auth: ci_api)

state
  backend local = state.backend.file(root: "/tmp/theater-state")

scenario register
  act fetch-profile
    prop response_json = inventory.http.get(url: "https://example.test/profile") | json.decode
    do action.generate
      outputs:
        profile_id: $response_json | path("/id")
    expect has-profile-id: field(profile_id) matches r"^[A-Za-z0-9-]+$"
    export issued_profile_id = field(profile_id)

call register-user = register()
  export final_profile_id = $issued_profile_id
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	if spec.HTTP == nil {
		t.Fatal("http spec must be present")
	}
	if _, ok := spec.HTTP.Sessions["browser"]; !ok {
		t.Fatal("http session browser must be present")
	}
	if got, want := len(spec.HTTP.Auth["ci_api"].Attach), 2; got != want {
		t.Fatalf("http auth attach count mismatch: got %d want %d", got, want)
	}
	if spec.HTTP.Auth["ci_api"].Attach[0].Bearer == nil {
		t.Fatal("first http auth attachment must be bearer")
	}
	if got, want := spec.HTTP.Identities["admin"].Session, "browser"; got != want {
		t.Fatalf("http identity session mismatch: got %q want %q", got, want)
	}
	if spec.State == nil {
		t.Fatal("state spec must be present")
	}
	if got, want := spec.State.Backends["local"].Use, "state.backend.file"; got != want {
		t.Fatalf("state backend use mismatch: got %q want %q", got, want)
	}
	if got, want := spec.State.Backends["local"].With["root"], "/tmp/theater-state"; got != want {
		t.Fatalf("state backend root mismatch: got %#v want %#v", got, want)
	}

	property := spec.Scenarios[0].Acts[0].Properties["response_json"]
	if property.Inventory == nil {
		t.Fatal("property inventory must be present")
	}
	if got, want := property.Inventory.Use, "inventory.http.get"; got != want {
		t.Fatalf("property inventory use mismatch: got %q want %q", got, want)
	}
	if got, want := property.Decorators[0].Use, "json.decode"; got != want {
		t.Fatalf("decorator use mismatch: got %q want %q", got, want)
	}

	output := spec.Scenarios[0].Acts[0].Action.With["outputs"].Object["profile_id"]
	if output.Ref == nil {
		t.Fatal("generated output binding ref must be present")
	}
	if got, want := output.Ref.Name, "response_json"; got != want {
		t.Fatalf("generated output ref name mismatch: got %q want %q", got, want)
	}
	if got, want := output.Ref.Path, theater.JSONPointer("/id"); got != want {
		t.Fatalf("generated output ref path mismatch: got %q want %q", got, want)
	}

	expectation := spec.Scenarios[0].Acts[0].Expectations[0]
	if got, want := expectation.Assert.Ref, builtinexpectation.MatchesRef; got != want {
		t.Fatalf("matches ref mismatch: got %q want %q", got, want)
	}
	if got, want := expectation.Assert.Args["pattern"].Value, "^[A-Za-z0-9-]+$"; got != want {
		t.Fatalf("matches pattern mismatch: got %#v want %#v", got, want)
	}

	export := spec.ScenarioCalls[0].Exports[0]
	if export.Ref == nil {
		t.Fatal("scenario call export ref must be present")
	}
	if got, want := export.Ref.Name, "issued_profile_id"; got != want {
		t.Fatalf("scenario call export ref mismatch: got %q want %q", got, want)
	}
}

func TestParseLowersStateAliasesToHiddenHandleProperties(t *testing.T) {
	t.Parallel()

	result, err := ParseDetailed([]byte(`stage smoke

state
  backend local = state.backend.file(root: "/tmp/theater-state")
  record shared_meta = state.record(
    backend: local,
    record: "env/shared-meta",
    min_guarantee: local-atomic
  )
  pool otp_identities = state.pool(
    backend: local,
    pool: "otp-identities",
    min_guarantee: local-atomic
  )

scenario verify-state
  act read-record
    do action.state.read(record: shared_meta)

  act update-record
    do action.state.update(
      record: shared_meta,
      expected_version: "7",
      value: object { owner: "tutorial-run" }
    )

  act claim-item
    do action.state.claim
      pool: otp_identities
      lease:
        ttl: 5m
        on_expiry: reclaim
`), "", nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	spec := result.Spec
	if spec.State == nil {
		t.Fatal("state spec must be present")
	}
	if got, want := len(spec.State.Backends), 1; got != want {
		t.Fatalf("state backend count mismatch: got %d want %d", got, want)
	}

	const (
		recordHiddenRef = "thtr:hidden:state:record:shared_meta"
		poolHiddenRef   = "thtr:hidden:state:pool:otp_identities"
	)

	readAct := spec.Scenarios[0].Acts[0]
	readRecord := readAct.Properties[recordHiddenRef]
	if readRecord.Inventory == nil {
		t.Fatal("read act hidden record inventory must be present")
	}
	if got, want := readRecord.Inventory.Use, "inventory.state.record"; got != want {
		t.Fatalf("read act hidden record inventory mismatch: got %q want %q", got, want)
	}
	if got, want := readRecord.Inventory.With["backend"].Value, "local"; got != want {
		t.Fatalf("read act hidden record backend mismatch: got %#v want %#v", got, want)
	}
	if got, want := readRecord.Inventory.With["record"].Value, "env/shared-meta"; got != want {
		t.Fatalf("read act hidden record name mismatch: got %#v want %#v", got, want)
	}
	if got, want := readAct.Action.With["record"].Ref.Name, recordHiddenRef; got != want {
		t.Fatalf("read act action record ref mismatch: got %q want %q", got, want)
	}

	updateAct := spec.Scenarios[0].Acts[1]
	updateRecord := updateAct.Properties[recordHiddenRef]
	if updateRecord.Inventory == nil {
		t.Fatal("update act hidden record inventory must be present")
	}
	if got, want := updateRecord.Inventory.Use, "inventory.state.record"; got != want {
		t.Fatalf("update act hidden record inventory mismatch: got %q want %q", got, want)
	}
	if got, want := updateAct.Action.With["record"].Ref.Name, recordHiddenRef; got != want {
		t.Fatalf("update act action record ref mismatch: got %q want %q", got, want)
	}

	claimAct := spec.Scenarios[0].Acts[2]
	claimPool := claimAct.Properties[poolHiddenRef]
	if claimPool.Inventory == nil {
		t.Fatal("claim act hidden pool inventory must be present")
	}
	if got, want := claimPool.Inventory.Use, "inventory.state.pool"; got != want {
		t.Fatalf("claim act hidden pool inventory mismatch: got %q want %q", got, want)
	}
	if got, want := claimPool.Inventory.With["backend"].Value, "local"; got != want {
		t.Fatalf("claim act hidden pool backend mismatch: got %#v want %#v", got, want)
	}
	if got, want := claimPool.Inventory.With["pool"].Value, "otp-identities"; got != want {
		t.Fatalf("claim act hidden pool name mismatch: got %#v want %#v", got, want)
	}
	if got, want := claimAct.Action.With["pool"].Ref.Name, poolHiddenRef; got != want {
		t.Fatalf("claim act action pool ref mismatch: got %q want %q", got, want)
	}
	if got, want := claimAct.Action.With["lease"].Object["on_expiry"].Value, "reclaim"; got != want {
		t.Fatalf("claim act lease policy mismatch: got %#v want %#v", got, want)
	}
}

func TestParseLowersStateReadSugarToCanonicalAction(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage smoke

state
  backend local = state.backend.file(root: "/tmp/theater-state")
  record shared_meta = state.record(
    backend: local,
    record: "env/shared-meta",
    min_guarantee: local-atomic
  )

scenario verify-state
  act read-record
    do state.read(record: shared_meta)
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	act := spec.Scenarios[0].Acts[0]
	if got, want := act.Action.Use, "action.state.read"; got != want {
		t.Fatalf("canonical state.read use mismatch: got %q want %q", got, want)
	}
	if got, want := act.Action.With["record"].Ref.Name, "thtr:hidden:state:record:shared_meta"; got != want {
		t.Fatalf("state.read hidden record ref mismatch: got %q want %q", got, want)
	}
	if got, want := act.Properties["thtr:hidden:state:record:shared_meta"].Inventory.Use, "inventory.state.record"; got != want {
		t.Fatalf("state.read hidden inventory use mismatch: got %q want %q", got, want)
	}
}

func TestParseLowersStateUpdateSugarToCanonicalAction(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage smoke

state
  backend local = state.backend.file(root: "/tmp/theater-state")
  record shared_meta = state.record(
    backend: local,
    record: "env/shared-meta",
    min_guarantee: local-atomic
  )

scenario verify-state
  act update-record
    do state.update(
      record: shared_meta,
      if_version: "7",
      value: object { owner: "tutorial-run" }
    )
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	act := spec.Scenarios[0].Acts[0]
	if got, want := act.Action.Use, stateUpdateActionCall; got != want {
		t.Fatalf("canonical state.update use mismatch: got %q want %q", got, want)
	}
	if got, want := act.Action.With["record"].Ref.Name, "thtr:hidden:state:record:shared_meta"; got != want {
		t.Fatalf("state.update hidden record ref mismatch: got %q want %q", got, want)
	}
	if got, want := act.Action.With["expected_version"].Value, "7"; got != want {
		t.Fatalf("state.update if_version mismatch: got %#v want %#v", got, want)
	}
	if got, want := act.Action.With["value"].Object["owner"].Value, "tutorial-run"; got != want {
		t.Fatalf("state.update value mismatch: got %#v want %#v", got, want)
	}
	if got, want := act.Properties["thtr:hidden:state:record:shared_meta"].Inventory.Use, stateRecordInventoryCall; got != want {
		t.Fatalf("state.update hidden inventory use mismatch: got %q want %q", got, want)
	}
}

func TestParseValidatesStateUpdateBesideExplicitHandleUpdate(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage smoke

state
  backend local = state.backend.file(root: "/tmp/theater-state")
  record shared_meta = state.record(
    backend: local,
    record: "env/shared-meta",
    min_guarantee: local-atomic
  )

scenario verify-state
  act explicit-update
    prop manual_record = inventory.state.record(
      backend: "local",
      record: "env/shared-meta",
      min_guarantee: "local-atomic"
    )
    do action.state.update(
      record: $manual_record,
      expected_version: "1",
      value: object { owner: "manual" }
    )

  act alias-update
    do state.update(
      record: shared_meta,
      if_version: "2",
      value: object { owner: "alias" }
    )
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	bundle, err := builtin.NewBundle()
	if err != nil {
		t.Fatalf("new builtin bundle failed: %v", err)
	}

	validator := theater.NewValidator(bundle.Catalog, bundle.Matchers)
	if diagnostics := validator.Validate(spec); len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
}

func TestLoadFileDetailedReturnsLoweringDiagnosticForMissingStateUpdateVersion(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.thtr")
	if err := os.WriteFile(path, []byte(`stage main
state
  backend local = state.backend.file(root: "`+t.TempDir()+`")
  record shared_meta = state.record(
    backend: local,
    record: "env/shared-meta",
    min_guarantee: local-atomic
  )
scenario login
  act update
    do state.update(
      record: shared_meta,
      value: object { owner: "tutorial-run" }
    )
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := LoadFileDetailed(path, nil)
	if err == nil {
		t.Fatal("expected lowering diagnostic error, got nil")
	}

	var diagnosticError *DiagnosticError
	if !errors.As(err, &diagnosticError) {
		t.Fatalf("expected diagnostic error, got %T", err)
	}

	diagnostic := diagnosticError.Diagnostic()
	if got, want := diagnostic.Span.File, path; got != want {
		t.Fatalf("diagnostic source file mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 11; got != want {
		t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostic.Summary, `state.update requires if_version`; got != want {
		t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
	}
}

func TestLoadFileDetailedRejectsCanonicalVersionArgInStateUpdateSugar(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.thtr")
	if err := os.WriteFile(path, []byte(`stage main
state
  backend local = state.backend.file(root: "`+t.TempDir()+`")
  record shared_meta = state.record(
    backend: local,
    record: "env/shared-meta",
    min_guarantee: local-atomic
  )
scenario login
  act update
    do state.update(
      record: shared_meta,
      expected_version: "1",
      value: object { owner: "tutorial-run" }
    )
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := LoadFileDetailed(path, nil)
	if err == nil {
		t.Fatal("expected lowering diagnostic error, got nil")
	}

	var diagnosticError *DiagnosticError
	if !errors.As(err, &diagnosticError) {
		t.Fatalf("expected diagnostic error, got %T", err)
	}

	diagnostic := diagnosticError.Diagnostic()
	if got, want := diagnostic.Code, "thtr_lower_error"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 13; got != want {
		t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostic.Summary, `state.update uses if_version; expected_version is the canonical action field`; got != want {
		t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
	}
}

func TestLoadFileDetailedRejectsRemovedStateCAS(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.thtr")
	if err := os.WriteFile(path, []byte(`stage main
state
  backend local = state.backend.file(root: "`+t.TempDir()+`")
  record shared_meta = state.record(
    backend: local,
    record: "env/shared-meta",
    min_guarantee: local-atomic
  )
scenario login
  act update
    do state.cas(
      record: shared_meta,
      expected_version: "1",
      value: object { owner: "tutorial-run" }
    )
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := LoadFileDetailed(path, nil)
	if err == nil {
		t.Fatal("expected lowering diagnostic error, got nil")
	}

	var diagnosticError *DiagnosticError
	if !errors.As(err, &diagnosticError) {
		t.Fatalf("expected diagnostic error, got %T", err)
	}

	diagnostic := diagnosticError.Diagnostic()
	if got, want := diagnostic.Code, "thtr_lower_error"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 11; got != want {
		t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostic.Summary, `state.cas has been removed; use state.update(... if_version: ...)`; got != want {
		t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
	}
}

func TestLoadFileDetailedReturnsLoweringDiagnosticForUnknownStateAliasBackend(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.thtr")
	if err := os.WriteFile(path, []byte(`stage main
state
  backend local = state.backend.file(root: "/tmp/theater-state")
  record shared_meta = state.record(
    backend: missing,
    record: "env/shared-meta",
    min_guarantee: local-atomic
  )
scenario login
  act read
    do action.state.read(record: shared_meta)
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := LoadFileDetailed(path, nil)
	if err == nil {
		t.Fatal("expected lowering diagnostic error, got nil")
	}

	var diagnosticError *DiagnosticError
	if !errors.As(err, &diagnosticError) {
		t.Fatalf("expected diagnostic error, got %T", err)
	}

	diagnostic := diagnosticError.Diagnostic()
	if got, want := diagnostic.Code, "thtr_lower_error"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Path, "stage.main/state/record.shared_meta"; got != want {
		t.Fatalf("diagnostic breadcrumb path mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 5; got != want {
		t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostic.Summary, `state alias "shared_meta" references unknown backend "missing"`; got != want {
		t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
	}
}

func TestLoadFileDetailedReturnsLoweringDiagnosticForInvalidStateAliasGuarantee(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.thtr")
	if err := os.WriteFile(path, []byte(`stage main
state
  backend local = state.backend.file(root: "/tmp/theater-state")
  record shared_meta = state.record(
    backend: local,
    record: "env/shared-meta",
    min_guarantee: strongest
  )
scenario login
  act read
    do action.state.read(record: shared_meta)
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := LoadFileDetailed(path, nil)
	if err == nil {
		t.Fatal("expected lowering diagnostic error, got nil")
	}

	var diagnosticError *DiagnosticError
	if !errors.As(err, &diagnosticError) {
		t.Fatalf("expected diagnostic error, got %T", err)
	}

	diagnostic := diagnosticError.Diagnostic()
	if got, want := diagnostic.Code, "thtr_lower_error"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Path, "stage.main/state/record.shared_meta"; got != want {
		t.Fatalf("diagnostic breadcrumb path mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 7; got != want {
		t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostic.Summary, `state min_guarantee "strongest" is invalid`; got != want {
		t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
	}
}

func TestLoadFileDetailedReturnsLoweringDiagnosticForCrossKindStateAliasCollision(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.thtr")
	if err := os.WriteFile(path, []byte(`stage main
state
  backend local = state.backend.file(root: "/tmp/theater-state")
  record shared_meta = state.record(
    backend: local,
    record: "env/shared-meta",
    min_guarantee: local-atomic
  )
  pool shared_meta = state.pool(
    backend: local,
    pool: "otp-identities",
    min_guarantee: local-atomic
  )
scenario login
  act read
    do action.state.read(record: shared_meta)
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := LoadFileDetailed(path, nil)
	if err == nil {
		t.Fatal("expected lowering diagnostic error, got nil")
	}

	var diagnosticError *DiagnosticError
	if !errors.As(err, &diagnosticError) {
		t.Fatalf("expected diagnostic error, got %T", err)
	}

	diagnostic := diagnosticError.Diagnostic()
	if got, want := diagnostic.Code, "thtr_lower_error"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Path, "stage.main/state/pool.shared_meta"; got != want {
		t.Fatalf("diagnostic breadcrumb path mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 9; got != want {
		t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostic.Summary, `state alias "shared_meta" is duplicated`; got != want {
		t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
	}
}

func TestLoadFileDetailedReturnsLoweringDiagnosticForStateAliasKindMismatch(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.thtr")
	if err := os.WriteFile(path, []byte(`stage main
state
  backend local = state.backend.file(root: "/tmp/theater-state")
  record shared_meta = state.record(
    backend: local,
    record: "env/shared-meta",
    min_guarantee: local-atomic
  )
scenario login
  act claim
    do action.state.claim
      pool: shared_meta
      lease:
        ttl: 5m
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := LoadFileDetailed(path, nil)
	if err == nil {
		t.Fatal("expected lowering diagnostic error, got nil")
	}

	var diagnosticError *DiagnosticError
	if !errors.As(err, &diagnosticError) {
		t.Fatalf("expected diagnostic error, got %T", err)
	}

	diagnostic := diagnosticError.Diagnostic()
	if got, want := diagnostic.Code, "thtr_lower_error"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Path, "stage.main/scenario.login/act.claim/action"; got != want {
		t.Fatalf("diagnostic breadcrumb path mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 12; got != want {
		t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostic.Summary, `state action arg "pool" requires pool alias, got record alias "shared_meta"`; got != want {
		t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
	}
}

func TestParseLowersStateClaimSugarToCanonicalAction(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage smoke

state
  backend local = state.backend.file(root: "/tmp/theater-state")
  pool otp_identities = state.pool(
    backend: local,
    pool: "otp-identities",
    min_guarantee: local-atomic
  )

scenario verify-state
  act claim-item
    do state.claim
      pool: otp_identities
      id: "fixture-1"
      fields:
        purpose: "registration"
        provider: "mailhog"
      lease:
        ttl: 5m
        on_expiry: reclaim
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	act := spec.Scenarios[0].Acts[0]
	if got, want := act.Action.Use, stateClaimActionCall; got != want {
		t.Fatalf("canonical state.claim use mismatch: got %q want %q", got, want)
	}
	if got, want := act.Action.With["pool"].Ref.Name, "thtr:hidden:state:pool:otp_identities"; got != want {
		t.Fatalf("state.claim hidden pool ref mismatch: got %q want %q", got, want)
	}
	selector := act.Action.With["selector"]
	if got, want := selector.Object["id"].Value, "fixture-1"; got != want {
		t.Fatalf("state.claim selector id mismatch: got %#v want %#v", got, want)
	}
	if got, want := selector.Object["fields"].Object["purpose"].Value, "registration"; got != want {
		t.Fatalf("state.claim selector purpose mismatch: got %#v want %#v", got, want)
	}
	if got, want := selector.Object["fields"].Object["provider"].Value, "mailhog"; got != want {
		t.Fatalf("state.claim selector provider mismatch: got %#v want %#v", got, want)
	}
	if got, want := act.Action.With["lease"].Object["ttl"].Value, "5m"; got != want {
		t.Fatalf("state.claim lease ttl mismatch: got %#v want %#v", got, want)
	}
	if got, want := act.Action.With["lease"].Object["on_expiry"].Value, "reclaim"; got != want {
		t.Fatalf("state.claim lease policy mismatch: got %#v want %#v", got, want)
	}
	if got, want := act.Properties["thtr:hidden:state:pool:otp_identities"].Inventory.Use, statePoolInventoryCall; got != want {
		t.Fatalf("state.claim hidden inventory use mismatch: got %q want %q", got, want)
	}
}

func TestParseLowersStateClaimSelectorEscapeHatch(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage smoke

state
  backend local = state.backend.file(root: "/tmp/theater-state")
  pool otp_identities = state.pool(
    backend: local,
    pool: "otp-identities",
    min_guarantee: local-atomic
  )

scenario verify-state
  act claim-item
    do state.claim(
      pool: otp_identities,
      selector: object {
        id: "fixture-1",
        fields: object { purpose: "registration" }
      },
      lease: object { ttl: 5m }
    )
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	act := spec.Scenarios[0].Acts[0]
	if got, want := act.Action.Use, stateClaimActionCall; got != want {
		t.Fatalf("canonical state.claim escape-hatch use mismatch: got %q want %q", got, want)
	}
	selector := act.Action.With["selector"]
	if got, want := selector.Object["id"].Value, "fixture-1"; got != want {
		t.Fatalf("state.claim escape-hatch selector id mismatch: got %#v want %#v", got, want)
	}
	if got, want := selector.Object["fields"].Object["purpose"].Value, "registration"; got != want {
		t.Fatalf("state.claim escape-hatch selector purpose mismatch: got %#v want %#v", got, want)
	}
}

func TestParseLowersStateClaimObjectFieldsAndPreservesCanonicalOutputs(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage smoke

state
  backend local = state.backend.file(root: "/tmp/theater-state")
  pool otp_identities = state.pool(
    backend: local,
    pool: "otp-identities",
    min_guarantee: local-atomic
  )

scenario verify-state
  act claim-item
    do state.claim(
      pool: otp_identities,
      fields: object { purpose: "registration" },
      lease: object { ttl: 5m }
    )
    export claimed_item = field(item)
    export claimed_handle = field(claim)
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	act := spec.Scenarios[0].Acts[0]
	if got, want := act.Action.Use, stateClaimActionCall; got != want {
		t.Fatalf("canonical state.claim object-fields use mismatch: got %q want %q", got, want)
	}
	selector := act.Action.With["selector"]
	if got, want := selector.Object["fields"].Object["purpose"].Value, "registration"; got != want {
		t.Fatalf("state.claim object-fields selector purpose mismatch: got %#v want %#v", got, want)
	}
	if got, want := len(act.Exports), 2; got != want {
		t.Fatalf("state.claim export count mismatch: got %d want %d", got, want)
	}
	if got, want := act.Exports[0].Field, "item"; got != want {
		t.Fatalf("state.claim item export field mismatch: got %q want %q", got, want)
	}
	if got, want := act.Exports[1].Field, "claim"; got != want {
		t.Fatalf("state.claim claim export field mismatch: got %q want %q", got, want)
	}

	bundle, err := builtin.NewBundle()
	if err != nil {
		t.Fatalf("new builtin bundle failed: %v", err)
	}

	validator := theater.NewValidator(bundle.Catalog, bundle.Matchers)
	if diagnostics := validator.Validate(spec); len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
}

func TestParseDetailedTracksStateClaimSelectorSugarSourceMap(t *testing.T) {
	t.Parallel()

	result, err := ParseDetailed([]byte(`stage smoke

state
  backend local = state.backend.file(root: "/tmp/theater-state")
  pool otp_identities = state.pool(
    backend: local,
    pool: "otp-identities",
    min_guarantee: local-atomic
  )

scenario verify-state
  act claim-item
    do state.claim
      pool: otp_identities
      id: "fixture-1"
      fields:
        purpose: "registration"
        provider: "mailhog"
      lease:
        ttl: 5m
`), "/tmp/claim.thtr", nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	if result.sourceMap == nil {
		t.Fatal("source map must be present")
	}

	idEntry, ok := result.sourceMap.LookupSpecPath(
		"stage.smoke/scenario.verify-state/act.claim-item/action/binding.selector.id",
	)
	if !ok {
		t.Fatal("selector id source map entry must be present")
	}
	if got, want := idEntry.Source.StartLine, 15; got != want {
		t.Fatalf("selector id line mismatch: got %d want %d", got, want)
	}

	fieldsEntry, ok := result.sourceMap.LookupSpecPath(
		"stage.smoke/scenario.verify-state/act.claim-item/action/binding.selector.fields",
	)
	if !ok {
		t.Fatal("selector fields source map entry must be present")
	}
	if got, want := fieldsEntry.Source.StartLine, 16; got != want {
		t.Fatalf("selector fields line mismatch: got %d want %d", got, want)
	}
}

func TestLoadFileDetailedReturnsLoweringDiagnosticForStateClaimSelectorConflict(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.thtr")
	if err := os.WriteFile(path, []byte(`stage main
state
  backend local = state.backend.file(root: "/tmp/theater-state")
  pool otp_identities = state.pool(
    backend: local,
    pool: "otp-identities",
    min_guarantee: local-atomic
  )
scenario login
  act claim
    do state.claim
      pool: otp_identities
      selector:
        id: "fixture-1"
      fields:
        purpose: "registration"
      lease:
        ttl: 5m
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := LoadFileDetailed(path, nil)
	if err == nil {
		t.Fatal("expected lowering diagnostic error, got nil")
	}

	var diagnosticError *DiagnosticError
	if !errors.As(err, &diagnosticError) {
		t.Fatalf("expected diagnostic error, got %T", err)
	}

	diagnostic := diagnosticError.Diagnostic()
	if got, want := diagnostic.Code, "thtr_lower_error"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Path, "stage.main/scenario.login/act.claim/action"; got != want {
		t.Fatalf("diagnostic breadcrumb path mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 13; got != want {
		t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostic.Summary, "state.claim selector cannot be combined with id or fields"; got != want {
		t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
	}
}

func TestLoadFileDetailedReturnsLoweringDiagnosticForRemovedStateClaimWhere(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.thtr")
	if err := os.WriteFile(path, []byte(`stage main
state
  backend local = state.backend.file(root: "/tmp/theater-state")
  pool otp_identities = state.pool(
    backend: local,
    pool: "otp-identities",
    min_guarantee: local-atomic
  )
scenario login
  act claim
    do state.claim
      pool: otp_identities
      where:
        purpose: "registration"
      lease:
        ttl: 5m
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := LoadFileDetailed(path, nil)
	if err == nil {
		t.Fatal("expected lowering diagnostic error, got nil")
	}

	var diagnosticError *DiagnosticError
	if !errors.As(err, &diagnosticError) {
		t.Fatalf("expected diagnostic error, got %T", err)
	}

	diagnostic := diagnosticError.Diagnostic()
	if got, want := diagnostic.Code, "thtr_lower_error"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Path, "stage.main/scenario.login/act.claim/action"; got != want {
		t.Fatalf("diagnostic breadcrumb path mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 13; got != want {
		t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostic.Summary, "state.claim where has been removed; use fields:"; got != want {
		t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
	}
}

func TestLoadFileDetailedReturnsLoweringDiagnosticForStateClaimFieldsNonObject(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.thtr")
	if err := os.WriteFile(path, []byte(`stage main
state
  backend local = state.backend.file(root: "/tmp/theater-state")
  pool otp_identities = state.pool(
    backend: local,
    pool: "otp-identities",
    min_guarantee: local-atomic
  )
scenario login
  act claim
    do state.claim
      pool: otp_identities
      fields: "registration"
      lease:
        ttl: 5m
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := LoadFileDetailed(path, nil)
	if err == nil {
		t.Fatal("expected lowering diagnostic error, got nil")
	}

	var diagnosticError *DiagnosticError
	if !errors.As(err, &diagnosticError) {
		t.Fatalf("expected diagnostic error, got %T", err)
	}

	diagnostic := diagnosticError.Diagnostic()
	if got, want := diagnostic.Code, "thtr_lower_error"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Path, "stage.main/scenario.login/act.claim/action"; got != want {
		t.Fatalf("diagnostic breadcrumb path mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 13; got != want {
		t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostic.Summary, "state.claim fields must be object with exact top-level fields"; got != want {
		t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
	}
}

func TestLoadFileDetailedReturnsLoweringDiagnosticForStateClaimFieldsNestedObject(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.thtr")
	if err := os.WriteFile(path, []byte(`stage main
state
  backend local = state.backend.file(root: "/tmp/theater-state")
  pool otp_identities = state.pool(
    backend: local,
    pool: "otp-identities",
    min_guarantee: local-atomic
  )
scenario login
  act claim
    do state.claim
      pool: otp_identities
      fields:
        metadata:
          provider: "mailhog"
      lease:
        ttl: 5m
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := LoadFileDetailed(path, nil)
	if err == nil {
		t.Fatal("expected lowering diagnostic error, got nil")
	}

	var diagnosticError *DiagnosticError
	if !errors.As(err, &diagnosticError) {
		t.Fatalf("expected diagnostic error, got %T", err)
	}

	diagnostic := diagnosticError.Diagnostic()
	if got, want := diagnostic.Code, "thtr_lower_error"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Path, "stage.main/scenario.login/act.claim/action"; got != want {
		t.Fatalf("diagnostic breadcrumb path mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 14; got != want {
		t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostic.Summary, "state.claim fields only supports exact top-level field matching"; got != want {
		t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
	}
}

func TestLoadFileDetailedReturnsLoweringDiagnosticForStateClaimFieldsListValue(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.thtr")
	if err := os.WriteFile(path, []byte(`stage main
state
  backend local = state.backend.file(root: "/tmp/theater-state")
  pool otp_identities = state.pool(
    backend: local,
    pool: "otp-identities",
    min_guarantee: local-atomic
  )
scenario login
  act claim
    do state.claim
      pool: otp_identities
      fields:
        tags: list ["mailhog"]
      lease:
        ttl: 5m
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := LoadFileDetailed(path, nil)
	if err == nil {
		t.Fatal("expected lowering diagnostic error, got nil")
	}

	var diagnosticError *DiagnosticError
	if !errors.As(err, &diagnosticError) {
		t.Fatalf("expected diagnostic error, got %T", err)
	}

	diagnostic := diagnosticError.Diagnostic()
	if got, want := diagnostic.Code, "thtr_lower_error"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Path, "stage.main/scenario.login/act.claim/action"; got != want {
		t.Fatalf("diagnostic breadcrumb path mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 14; got != want {
		t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostic.Summary, "state.claim fields only supports exact top-level field matching"; got != want {
		t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
	}
}

func TestParseLowersClaimLifecycleStateSugarToCanonicalActions(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage smoke

scenario verify-state
  act renew-claim
    do state.renew(claim: $otp_claim, ttl: 10m)

  act release-claim
    do state.release(claim: $otp_claim)

  act consume-claim
    do state.consume(
      claim: $otp_claim,
      tombstone: object { reason: "registration-complete" }
    )
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	renewAct := spec.Scenarios[0].Acts[0]
	if got, want := renewAct.Action.Use, "action.state.renew"; got != want {
		t.Fatalf("canonical state.renew use mismatch: got %q want %q", got, want)
	}
	if got, want := renewAct.Action.With["claim"].Ref.Name, "otp_claim"; got != want {
		t.Fatalf("state.renew claim ref mismatch: got %q want %q", got, want)
	}
	if got, want := renewAct.Action.With["ttl"].Value, "10m"; got != want {
		t.Fatalf("state.renew ttl mismatch: got %#v want %#v", got, want)
	}

	releaseAct := spec.Scenarios[0].Acts[1]
	if got, want := releaseAct.Action.Use, "action.state.release"; got != want {
		t.Fatalf("canonical state.release use mismatch: got %q want %q", got, want)
	}
	if got, want := releaseAct.Action.With["claim"].Ref.Name, "otp_claim"; got != want {
		t.Fatalf("state.release claim ref mismatch: got %q want %q", got, want)
	}

	consumeAct := spec.Scenarios[0].Acts[2]
	if got, want := consumeAct.Action.Use, "action.state.consume"; got != want {
		t.Fatalf("canonical state.consume use mismatch: got %q want %q", got, want)
	}
	if got, want := consumeAct.Action.With["claim"].Ref.Name, "otp_claim"; got != want {
		t.Fatalf("state.consume claim ref mismatch: got %q want %q", got, want)
	}
	if got, want := consumeAct.Action.With["tombstone"].Object["reason"].Value, "registration-complete"; got != want {
		t.Fatalf("state.consume tombstone mismatch: got %#v want %#v", got, want)
	}
}

func TestParseAllowsStateConsumeSugarWithoutTombstone(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage smoke

scenario verify-state
  act consume-claim
    do state.consume(claim: $otp_claim)
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	act := spec.Scenarios[0].Acts[0]
	if got, want := act.Action.Use, "action.state.consume"; got != want {
		t.Fatalf("canonical state.consume without tombstone use mismatch: got %q want %q", got, want)
	}
	if _, ok := act.Action.With["tombstone"]; ok {
		t.Fatalf("state.consume without tombstone must not synthesize tombstone binding: %#v", act.Action.With)
	}
}

func TestParseAndValidateClaimLifecycleStateSugarKeepsClaimHandlesExplicit(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage smoke

state
  backend local = state.backend.file(root: "/tmp/theater-state")
  pool otp_identities = state.pool(
    backend: local,
    pool: "otp-identities",
    min_guarantee: local-atomic
  )

scenario verify-state
  act claim-item
    do state.claim
      pool: otp_identities
      id: "fixture-1"
      lease:
        ttl: 5m
    export otp_claim = field(claim)

  act renew-claim
    do state.renew(claim: $otp_claim, ttl: 10m)

  act release-claim
    do state.release(claim: $otp_claim)

  act consume-claim
    do state.consume(claim: $otp_claim)
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	bundle, err := builtin.NewBundle()
	if err != nil {
		t.Fatalf("new builtin bundle failed: %v", err)
	}

	validator := theater.NewValidator(bundle.Catalog, bundle.Matchers)
	if diagnostics := validator.Validate(spec); len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
}

func TestLoadFileDetailedRewritesMissingClaimDiagnosticForClaimLifecycleSugar(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{
			name: "renew",
			body: `    do state.renew(ttl: 10m)
`,
		},
		{
			name: "release",
			body: `    do state.release()
`,
		},
		{
			name: "consume",
			body: `    do state.consume()
`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "invalid.thtr")
			if err := os.WriteFile(path, []byte(`stage main
scenario verify-state
  act lifecycle
`+tc.body), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			result, err := LoadFileDetailed(path, nil)
			if err != nil {
				t.Fatalf("load file detailed failed: %v", err)
			}

			bundle, err := builtin.NewBundle()
			if err != nil {
				t.Fatalf("new builtin bundle failed: %v", err)
			}

			validator := theater.NewValidator(bundle.Catalog, bundle.Matchers)
			diagnostics := result.RewriteDiagnostics(validator.Validate(result.Spec))
			diagnostic := findDiagnosticByCodeValue(diagnostics, "missing_action_arg")
			if diagnostic == nil {
				t.Fatalf("expected missing_action_arg diagnostic, got %#v", diagnostics)
			}
			if got, want := diagnostic.Span.File, path; got != want {
				t.Fatalf("diagnostic source file mismatch: got %q want %q", got, want)
			}
			if got, want := diagnostic.Summary, `action input "claim" is required`; got != want {
				t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
			}
		})
	}
}

func TestLoadFileDetailedRewritesUnknownClaimRefDiagnosticsForClaimLifecycleSugar(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{
			name: "renew",
			body: `    do state.renew(claim: $missing_claim, ttl: 10m)
`,
		},
		{
			name: "release",
			body: `    do state.release(claim: $missing_claim)
`,
		},
		{
			name: "consume",
			body: `    do state.consume(claim: $missing_claim)
`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "invalid.thtr")
			if err := os.WriteFile(path, []byte(`stage main
scenario verify-state
  act lifecycle
`+tc.body), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			result, err := LoadFileDetailed(path, nil)
			if err != nil {
				t.Fatalf("load file detailed failed: %v", err)
			}

			bundle, err := builtin.NewBundle()
			if err != nil {
				t.Fatalf("new builtin bundle failed: %v", err)
			}

			validator := theater.NewValidator(bundle.Catalog, bundle.Matchers)
			diagnostics := result.RewriteDiagnostics(validator.Validate(result.Spec))
			diagnostic := findDiagnosticByCodeValue(diagnostics, "unresolved_binding_ref")
			if diagnostic == nil {
				t.Fatalf("expected unresolved_binding_ref diagnostic, got %#v", diagnostics)
			}
			if got, want := diagnostic.Span.File, path; got != want {
				t.Fatalf("diagnostic source file mismatch: got %q want %q", got, want)
			}
			if got, want := diagnostic.Span.Line, 4; got != want {
				t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
			}
			if got, want := diagnostic.Summary, `binding ref "missing_claim" is not available in scenario scope at this point`; got != want {
				t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
			}
		})
	}
}

func TestLoadFileDetailedRewritesStateMutationEventuallyDiagnosticsForStateSugar(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		input         string
		state         string
		body          string
		wantLine      int
		wantActionUse string
	}{
		{
			name: "update",
			state: `state
  backend local = state.backend.file(root: "/tmp/theater-state")
  record shared_meta = state.record(
    backend: local,
    record: "env/shared-meta",
    min_guarantee: local-atomic
  )
`,
			body: `    do state.update(
      record: shared_meta,
      if_version: "1",
      value: object { owner: "tutorial-run" }
    )
`,
			wantLine:      10,
			wantActionUse: "action.state.update",
		},
		{
			name: "claim",
			state: `state
  backend local = state.backend.file(root: "/tmp/theater-state")
  pool otp_identities = state.pool(
    backend: local,
    pool: "otp-identities",
    min_guarantee: local-atomic
  )
`,
			body: `    do state.claim
      pool: otp_identities
      id: "fixture-1"
      lease:
        ttl: 5m
`,
			wantLine:      10,
			wantActionUse: "action.state.claim",
		},
		{
			name:  "renew",
			input: "(otp_claim: any!)",
			body: `    do state.renew(claim: $otp_claim, ttl: 10m)
`,
			wantLine:      3,
			wantActionUse: "action.state.renew",
		},
		{
			name:  "release",
			input: "(otp_claim: any!)",
			body: `    do state.release(claim: $otp_claim)
`,
			wantLine:      3,
			wantActionUse: "action.state.release",
		},
		{
			name:  "consume",
			input: "(otp_claim: any!)",
			body: `    do state.consume(claim: $otp_claim)
`,
			wantLine:      3,
			wantActionUse: "action.state.consume",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "invalid.thtr")
			content := `stage main
` + tc.state + `scenario verify-state` + tc.input + `
  act lifecycle
    eventually 5s every 1s
` + tc.body
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			result, err := LoadFileDetailed(path, nil)
			if err != nil {
				t.Fatalf("load file detailed failed: %v", err)
			}

			bundle, err := builtin.NewBundle()
			if err != nil {
				t.Fatalf("new builtin bundle failed: %v", err)
			}

			validator := theater.NewValidator(bundle.Catalog, bundle.Matchers)
			diagnostics := result.RewriteDiagnostics(validator.Validate(result.Spec))
			diagnostic := findDiagnosticByCodeValue(diagnostics, "state_mutation_inside_eventually")
			if diagnostic == nil {
				t.Fatalf("expected state_mutation_inside_eventually diagnostic, got %#v", diagnostics)
			}
			if got, want := diagnostic.Span.File, path; got != want {
				t.Fatalf("diagnostic source file mismatch: got %q want %q", got, want)
			}
			if got, want := diagnostic.Span.Line, tc.wantLine; got != want {
				t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
			}
			if got, want := diagnostic.Summary, `act "lifecycle" eventually must not use mutating state action "`+tc.wantActionUse+`"`; got != want {
				t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
			}
		})
	}
}

func TestParseValidatesMixedExplicitHandleAndAliasBackedStateAuthoring(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage smoke

state
  backend local = state.backend.file(root: "/tmp/theater-state")
  pool otp_identities = state.pool(
    backend: local,
    pool: "otp-identities",
    min_guarantee: local-atomic
  )

scenario verify-state
  act explicit-claim
    prop manual_pool = inventory.state.pool(
      backend: "local",
      pool: "otp-identities",
      min_guarantee: "local-atomic"
    )
    do action.state.claim(
      pool: $manual_pool,
      lease: object { ttl: 5m }
    )
    export explicit_claim = field(claim)

  act alias-renew
    do state.renew(claim: $explicit_claim, ttl: 10m)

  act alias-claim
    do state.claim
      pool: otp_identities
      id: "fixture-1"
      lease:
        ttl: 5m
    export alias_claim = field(claim)

  act explicit-consume
    do action.state.consume(
      claim: $alias_claim,
      tombstone: object { reason: "registration-complete" }
    )
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	bundle, err := builtin.NewBundle()
	if err != nil {
		t.Fatalf("new builtin bundle failed: %v", err)
	}

	validator := theater.NewValidator(bundle.Catalog, bundle.Matchers)
	if diagnostics := validator.Validate(spec); len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
}

func TestLoadFileDetailedRewritesStateAliasGuaranteeDiagnostics(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.thtr")
	if err := os.WriteFile(path, []byte(`stage main
state
  backend local = state.backend.file(root: "`+t.TempDir()+`")
  record shared_meta = state.record(
    backend: local,
    record: "env/shared-meta",
    min_guarantee: shared-atomic
  )
scenario login
  act read
    do action.state.read(record: shared_meta)
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	result, err := LoadFileDetailed(path, nil)
	if err != nil {
		t.Fatalf("load file detailed failed: %v", err)
	}

	bundle, err := builtin.NewBundle()
	if err != nil {
		t.Fatalf("new builtin bundle failed: %v", err)
	}

	validator := theater.NewValidator(bundle.Catalog, bundle.Matchers)
	diagnostics := result.RewriteDiagnostics(validator.Validate(result.Spec))
	diagnostic := findDiagnosticByCodeValue(diagnostics, "insufficient_state_backend_guarantee")
	if diagnostic == nil {
		t.Fatalf("expected insufficient_state_backend_guarantee diagnostic, got %#v", diagnostics)
	}
	if got, want := diagnostic.Span.File, path; got != want {
		t.Fatalf("diagnostic source file mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 7; got != want {
		t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
	}
}

func TestParseKeepsMultilineInterpolationMarkersLiteral(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage main
scenario login
  act submit
    do action.http
      url: """
        /flows/${flow_id}
      """
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	url := spec.Scenarios[0].Acts[0].Action.With["url"]
	if got, want := url.Kind, theater.BindingKindLiteral; got != want {
		t.Fatalf("multiline url kind mismatch: got %q want %q", got, want)
	}
	if got, want := url.Value, "/flows/${flow_id}"; got != want {
		t.Fatalf("multiline url value mismatch: got %#v want %#v", got, want)
	}
}

func TestParseTrimsDelimiterOnlyMultilineStringLines(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage main
scenario profile
  act read
    do action.command
      executable: "printf"
      args: list [
        """
        {"data":{"id":"user-123"}}
        """
      ]
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	args := spec.Scenarios[0].Acts[0].Action.With["args"]
	if got, want := args.Kind, theater.BindingKindList; got != want {
		t.Fatalf("args kind mismatch: got %q want %q", got, want)
	}
	if got, want := len(args.List), 1; got != want {
		t.Fatalf("args length mismatch: got %d want %d", got, want)
	}
	if got, want := args.List[0].Value, `{"data":{"id":"user-123"}}`; got != want {
		t.Fatalf("multiline json value mismatch: got %#v want %#v", got, want)
	}
}

func TestParseLowersNamesDependenciesAndCaptureAuth(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage smoke
name "Smoke stage"

http
  auth web = http.auth(
    attach: list [
      object { header_slot: object { name: "X-CSRF-Token", slot: "csrf" } },
      object { header_slot: object { name: "X-Session", slot: "session" } },
    ],
  )

scenario auth/login(email: string!)
  name "Login scenario"
  act submit
    name "Submit request"
    do action.http(method: "POST", url: "/login")
    capture_auth web
      csrf: response_header("X-CSRF-Token")
      session: response_cookie("session")
      token: json_pointer("/data/token")
      form_token: form_field("csrf_token")

call run-login = auth/login(email: "user@example.test")
  name "Run login"
  dependency bootstrap
  dependency provision-user when done
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	if got, want := spec.Name, "Smoke stage"; got != want {
		t.Fatalf("stage name mismatch: got %q want %q", got, want)
	}
	if got, want := spec.Scenarios[0].Name, "Login scenario"; got != want {
		t.Fatalf("scenario name mismatch: got %q want %q", got, want)
	}
	if got, want := spec.Scenarios[0].Acts[0].Name, "Submit request"; got != want {
		t.Fatalf("act name mismatch: got %q want %q", got, want)
	}
	if got, want := spec.ScenarioCalls[0].Name, "Run login"; got != want {
		t.Fatalf("call name mismatch: got %q want %q", got, want)
	}
	if got, want := len(spec.ScenarioCalls[0].Dependencies), 2; got != want {
		t.Fatalf("dependency count mismatch: got %d want %d", got, want)
	}
	if got, want := spec.ScenarioCalls[0].Dependencies[0].CallID, "bootstrap"; got != want {
		t.Fatalf("first dependency id mismatch: got %q want %q", got, want)
	}
	if got, want := spec.ScenarioCalls[0].Dependencies[0].When, theater.TriggerPredicateSuccess; got != want {
		t.Fatalf("first dependency predicate mismatch: got %q want %q", got, want)
	}
	if got, want := spec.ScenarioCalls[0].Dependencies[1].When, theater.TriggerPredicateDone; got != want {
		t.Fatalf("second dependency predicate mismatch: got %q want %q", got, want)
	}

	capture := spec.Scenarios[0].Acts[0].CaptureAuth
	if capture == nil {
		t.Fatal("capture_auth must be present")
	}
	if got, want := capture.Auth, "web"; got != want {
		t.Fatalf("capture_auth auth mismatch: got %q want %q", got, want)
	}
	if got, want := capture.Slots["csrf"].ResponseHeader, "X-CSRF-Token"; got != want {
		t.Fatalf("response_header slot mismatch: got %q want %q", got, want)
	}
	if got, want := capture.Slots["session"].ResponseCookie, "session"; got != want {
		t.Fatalf("response_cookie slot mismatch: got %q want %q", got, want)
	}
	if got, want := capture.Slots["token"].JSONPointer, theater.JSONPointer("/data/token"); got != want {
		t.Fatalf("json_pointer slot mismatch: got %q want %q", got, want)
	}
	if got, want := capture.Slots["form_token"].FormField, "csrf_token"; got != want {
		t.Fatalf("form_field slot mismatch: got %q want %q", got, want)
	}
}

func findDiagnosticByCodeValue(diagnostics []theater.Diagnostic, code string) *theater.Diagnostic {
	for i := range diagnostics {
		if diagnostics[i].Code == code {
			return &diagnostics[i]
		}
	}

	return nil
}

func TestParseLowersSelectorsInterpolationAndTransitions(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage otp-smoke
scenario verify-email(flow_id: string!, email: string!)
  act submit-email
    do action.http
      method: "PATCH"
      url: "https://example.test/flows/${flow_id}/registrations"
      json: object { email: $email }
    on pass -> poll-notifications

  act poll-notifications
    eventually 30s every 1s
    do repeatable action.http
      method: "GET"
      url: "https://example.test/debug/notifications?format=json"
    expect otp-shape: (
      field(body)
      | decode(json)
      | path("/data")
      | pick(at: "/receiverAddress", equals: $email)
      | path("/body")
      | regexp(pattern: r"(?i)verification code:\s*([A-Z0-9]+)", group: 1)
    ) matches r"^[A-Z0-9]{6}$"
    export otp = (
      field(body)
      | decode(json)
      | path("/data")
      | pick(at: "/receiverAddress", equals: $email)
      | path("/body")
      | regexp(pattern: r"(?i)verification code:\s*([A-Z0-9]+)", group: 1)
    )
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	if got, want := spec.Scenarios[0].Inputs["flow_id"].Kind, theater.ValueKindString; got != want {
		t.Fatalf("flow_id kind mismatch: got %q want %q", got, want)
	}
	if !spec.Scenarios[0].Inputs["flow_id"].Required {
		t.Fatal("flow_id must be required")
	}

	url := spec.Scenarios[0].Acts[0].Action.With["url"]
	if got, want := url.Kind, theater.BindingKindString; got != want {
		t.Fatalf("interpolated url kind mismatch: got %q want %q", got, want)
	}
	if got, want := len(url.Parts), 3; got != want {
		t.Fatalf("interpolated url parts mismatch: got %d want %d", got, want)
	}
	if url.Parts[1].Ref == nil || url.Parts[1].Ref.Name != "flow_id" {
		t.Fatal("interpolated url must include flow_id ref part")
	}

	jsonBody := spec.Scenarios[0].Acts[0].Action.With["json"]
	if got, want := jsonBody.Kind, theater.BindingKindObject; got != want {
		t.Fatalf("json body kind mismatch: got %q want %q", got, want)
	}
	if jsonBody.Object["email"].Ref == nil || jsonBody.Object["email"].Ref.Name != "email" {
		t.Fatal("json body email must be ref binding")
	}

	if got, want := spec.Scenarios[0].Acts[0].Transitions[0].On, theater.TransitionOnPass; got != want {
		t.Fatalf("transition outcome mismatch: got %q want %q", got, want)
	}

	pollAct := spec.Scenarios[0].Acts[1]
	if pollAct.Eventually == nil {
		t.Fatal("poll act must include eventually")
	}
	if !pollAct.Action.Repeatable {
		t.Fatal("poll act action must be repeatable")
	}

	subject := pollAct.Expectations[0].Subject
	if got, want := subject.Field, "body"; got != want {
		t.Fatalf("subject field mismatch: got %q want %q", got, want)
	}
	if got, want := subject.Decode, theater.DecodeJSON; got != want {
		t.Fatalf("subject decode mismatch: got %q want %q", got, want)
	}
	if got, want := subject.Path, theater.JSONPointer("/data"); got != want {
		t.Fatalf("subject path mismatch: got %q want %q", got, want)
	}
	if got, want := len(subject.Through), 3; got != want {
		t.Fatalf("through length mismatch: got %d want %d", got, want)
	}
	if subject.Through[0].Pick == nil || subject.Through[0].Pick.Equals.Ref == nil || subject.Through[0].Pick.Equals.Ref.Name != "email" {
		t.Fatal("pick step must compare against email ref")
	}
	if subject.Through[2].Regexp == nil {
		t.Fatal("regexp step must be present")
	}

	export := pollAct.Exports[0]
	if got, want := export.Field, "body"; got != want {
		t.Fatalf("export field mismatch: got %q want %q", got, want)
	}
	if got, want := export.Path, theater.JSONPointer("/data"); got != want {
		t.Fatalf("export path mismatch: got %q want %q", got, want)
	}
	if got, want := len(export.Through), 3; got != want {
		t.Fatalf("export through length mismatch: got %d want %d", got, want)
	}
}

func TestParseLowersQuotedDataKeys(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage smoke
scenario login(email: string!)
  act submit
    do action.http
      method: "POST"
      url: "/login"
      json: object {
        "email": $email,
        "profile.name": "Demo",
        "@type": "User"
      }
      headers: object { "Content-Type": "application/json", "x-csrf-token": $csrf }
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	jsonBody := spec.Scenarios[0].Acts[0].Action.With["json"]
	if got, want := jsonBody.Kind, theater.BindingKindObject; got != want {
		t.Fatalf("json body kind mismatch: got %q want %q", got, want)
	}
	if jsonBody.Object["email"].Ref == nil || jsonBody.Object["email"].Ref.Name != "email" {
		t.Fatal("quoted email key must lower to email ref binding")
	}
	if got, want := jsonBody.Object["profile.name"].Value, "Demo"; got != want {
		t.Fatalf("dotted data key value mismatch: got %#v want %#v", got, want)
	}
	if got, want := jsonBody.Object["@type"].Value, "User"; got != want {
		t.Fatalf("at-sign data key value mismatch: got %#v want %#v", got, want)
	}

	headers := spec.Scenarios[0].Acts[0].Action.With["headers"]
	if got, want := headers.Object["Content-Type"].Value, "application/json"; got != want {
		t.Fatalf("quoted header key value mismatch: got %#v want %#v", got, want)
	}
	if headers.Object["x-csrf-token"].Ref == nil || headers.Object["x-csrf-token"].Ref.Name != "csrf" {
		t.Fatal("quoted csrf header key must lower to csrf ref binding")
	}
}

func TestLoadFileDetailedRewritesQuotedDataKeyBindingDiagnostics(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.thtr")
	if err := os.WriteFile(path, []byte(`stage smoke
scenario login
  act submit
    do action.http
      method: "POST"
      url: "/login"
      json: object {
        "profile.name": $missing_profile
      }
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	result, err := LoadFileDetailed(path, nil)
	if err != nil {
		t.Fatalf("load file detailed failed: %v", err)
	}

	bundle, err := builtin.NewBundle()
	if err != nil {
		t.Fatalf("new builtin bundle failed: %v", err)
	}

	validator := theater.NewValidator(bundle.Catalog, bundle.Matchers)
	diagnostics := result.RewriteDiagnostics(validator.Validate(result.Spec))
	diagnostic := findDiagnosticByCodeValue(diagnostics, "unresolved_binding_ref")
	if diagnostic == nil {
		t.Fatalf("expected unresolved_binding_ref diagnostic, got %#v", diagnostics)
	}
	if got, want := diagnostic.Span.File, path; got != want {
		t.Fatalf("diagnostic source file mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 8; got != want {
		t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
	}
	if diagnostic.Span.Column == 0 {
		t.Fatal("diagnostic source column must be populated")
	}
	if got, want := diagnostic.Summary, `binding ref "missing_profile" is not available in scenario scope at this point`; got != want {
		t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
	}
}

func TestParseLowersPickWhereSelector(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage otp-smoke
scenario wait-for-otp(email: string!)
  act poll-notifications
    do action.http(method: "GET", url: "/notifications")
    export otp = (
      field(body)
      | decode(json)
      | path("/items")
      | pick where (
        path("/receiverAddress") == $email,
        path("/subject") contains "Verification Code"
      )
      | path("/body")
      | regexp(pattern: r"([0-9]{6})", group: 1)
    )
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	export := spec.Scenarios[0].Acts[0].Exports[0]
	if got, want := len(export.Through), 3; got != want {
		t.Fatalf("export through length mismatch: got %d want %d", got, want)
	}

	pick := export.Through[0].Pick
	if pick == nil {
		t.Fatal("first export through step must be pick")
	}
	if got, want := len(pick.Where), 2; got != want {
		t.Fatalf("pick where length mismatch: got %d want %d", got, want)
	}
	if got, want := pick.Where[0].Subject.Path, theater.JSONPointer("/receiverAddress"); got != want {
		t.Fatalf("first pick where subject path mismatch: got %q want %q", got, want)
	}
	if got, want := pick.Where[0].Assert.Ref, builtinexpectation.EqualRef; got != want {
		t.Fatalf("first pick where assert ref mismatch: got %q want %q", got, want)
	}
	if pick.Where[0].Assert.Args["expected"].Ref == nil || pick.Where[0].Assert.Args["expected"].Ref.Name != "email" {
		t.Fatal("first pick where expected arg must be email ref")
	}
	if got, want := pick.Where[1].Subject.Path, theater.JSONPointer("/subject"); got != want {
		t.Fatalf("second pick where subject path mismatch: got %q want %q", got, want)
	}
	if got, want := pick.Where[1].Assert.Ref, builtinexpectation.ContainsRef; got != want {
		t.Fatalf("second pick where assert ref mismatch: got %q want %q", got, want)
	}
	if export.Through[2].Regexp == nil {
		t.Fatal("regexp step must be present after pick where")
	}
}

func TestParseLowersActExportAssertion(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage otp-smoke
scenario wait-for-otp(email: string!)
  act poll-notifications
    do action.http(method: "GET", url: "/notifications")
    export otp = (
      field(body)
      | decode(json)
      | path("/items")
      | pick where (
        path("/receiverAddress") == $email,
        path("/subject") contains "Verification Code"
      )
      | path("/body")
      | regexp(pattern: r"([0-9]{6})", group: 1)
    ) matches r"^[0-9]{6}$"
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	act := spec.Scenarios[0].Acts[0]
	if got, want := len(act.Expectations), 1; got != want {
		t.Fatalf("expectation count mismatch: got %d want %d", got, want)
	}
	expectation := act.Expectations[0]
	if got, want := expectation.ID, "otp"; got != want {
		t.Fatalf("expectation id mismatch: got %q want %q", got, want)
	}
	if got, want := expectation.Subject.Field, "body"; got != want {
		t.Fatalf("expectation subject field mismatch: got %q want %q", got, want)
	}
	if got, want := len(expectation.Subject.Through), 3; got != want {
		t.Fatalf("expectation through length mismatch: got %d want %d", got, want)
	}
	if expectation.Subject.Through[0].Pick == nil || len(expectation.Subject.Through[0].Pick.Where) != 2 {
		t.Fatal("expectation must reuse export pick where selector")
	}
	if got, want := expectation.Assert.Ref, builtinexpectation.MatchesRef; got != want {
		t.Fatalf("expectation assert ref mismatch: got %q want %q", got, want)
	}
	if got, want := len(act.Exports), 1; got != want {
		t.Fatalf("export count mismatch: got %d want %d", got, want)
	}
	export := act.Exports[0]
	if got, want := export.Field, "body"; got != want {
		t.Fatalf("export field mismatch: got %q want %q", got, want)
	}
	if got, want := len(export.Through), 3; got != want {
		t.Fatalf("export through length mismatch: got %d want %d", got, want)
	}
}

func TestParseRejectsPickWhereInvalidRelativeSubject(t *testing.T) {
	t.Parallel()

	_, err := Parse([]byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET", url: "/notifications")
    export bad = field(body) | decode(json) | path("/items") | pick where field(status_code) == 200
`), nil)
	if err == nil {
		t.Fatal("expected parse to fail")
	}

	errtest.RequireContains(t, err, `relative clause subject may start only with decode(...) or path(...)`)
}

func TestParseRejectsNonInventoryPropertyPipeline(t *testing.T) {
	t.Parallel()

	_, err := Parse([]byte(`stage smoke
scenario login
  act submit
    prop bad = json.decode
    do action.http(method: "GET")
`), nil)

	errtest.RequireContains(t, err, "property must start with inventory call")
}

func TestParseRejectsScenarioCallSelectorExport(t *testing.T) {
	t.Parallel()

	_, err := Parse([]byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET")

call run = login()
  export final_session = $issued | path("/token")
`), nil)

	errtest.RequireContains(t, err, "scenario call export must be direct ref")
}

func TestParseRejectsScenarioCallExportAssertion(t *testing.T) {
	t.Parallel()

	_, err := Parse([]byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET", url: "/login")

call run = login()
  export final_session = $issued matches r"^sess_"
`), nil)
	if err == nil {
		t.Fatal("expected parse to fail")
	}

	errtest.RequireContains(t, err, "scenario call export assertions are not supported")
}

func TestParseRejectsActExportAssertionFromRef(t *testing.T) {
	t.Parallel()

	_, err := Parse([]byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET", url: "/login")
    export otp = $raw matches r"^sess_"
`), nil)
	if err == nil {
		t.Fatal("expected parse to fail")
	}

	errtest.RequireContains(t, err, "export assertion subject must start with field(...)")
	var diagnosticError *DiagnosticError
	if !errors.As(err, &diagnosticError) {
		t.Fatalf("expected diagnostic error, got %T", err)
	}
	diagnostic := diagnosticError.Diagnostic()
	if got, want := diagnostic.Span.Line, 5; got != want {
		t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostic.Span.Column, 18; got != want {
		t.Fatalf("diagnostic source column mismatch: got %d want %d", got, want)
	}
}

func TestParseLowersExplicitAssertRefWithoutMatcherResolver(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage smoke
scenario ping
  act get-health
    do action.http(method: "GET", url: "/health")
    expect status: field(status_code) assert plugin.custom(expected: 200)
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	assertion := spec.Scenarios[0].Acts[0].Expectations[0].Assert
	if got, want := assertion.Ref, "plugin.custom"; got != want {
		t.Fatalf("assert ref mismatch: got %q want %q", got, want)
	}
	if got, want := assertion.Args["expected"].Value, 200; got != want {
		t.Fatalf("assert expected arg mismatch: got %#v want %#v", got, want)
	}
}

func TestParseLowersExplicitNotAssertWithoutMatcherResolver(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage smoke
scenario ping
  act get-health
    do action.http(method: "GET", url: "/health")
    expect status: field(status_code) assert expectation.not(
      assert: object {
        ref: "expectation.gte",
        args: object {
          expected: 500
        }
      }
    )
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	assertion := spec.Scenarios[0].Acts[0].Expectations[0].Assert
	if got, want := assertion.Ref, builtinexpectation.NotRef; got != want {
		t.Fatalf("assert ref mismatch: got %q want %q", got, want)
	}

	assertArg, ok := assertion.Args["assert"]
	if !ok {
		t.Fatal(`assert arg "assert" must be present`)
	}

	if got, want := assertArg.Kind, theater.BindingKindObject; got != want {
		t.Fatalf("assert arg kind mismatch: got %q want %q", got, want)
	}

	if got, want := assertArg.Object["ref"].Value, builtinexpectation.GTERef; got != want {
		t.Fatalf("nested assert ref mismatch: got %#v want %#v", got, want)
	}

	args, ok := assertArg.Object["args"]
	if !ok {
		t.Fatal(`nested assert arg "args" must be present`)
	}

	if got, want := args.Kind, theater.BindingKindObject; got != want {
		t.Fatalf("nested assert args kind mismatch: got %q want %q", got, want)
	}

	if got, want := args.Object["expected"].Value, 500; got != want {
		t.Fatalf("nested assert expected mismatch: got %#v want %#v", got, want)
	}
}

func TestParseLowersScalarAndUnarySugarWithoutMatcherResolver(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage smoke
scenario ping
  act get-health
    do action.http(method: "GET", url: "/health")
    expect page-text: field(body) contains "Example Domain"
    expect latency-high: field(duration_ms) > 100
    expect retries-low: field(retry_count) < 10
    expect retries-ok: field(retry_count) <= 10
    expect retries-at-least-once: field(retry_count) >= 1
    expect retries-in-range: field(retry_count) between 1 and 5
    expect has-session-token: field(body) | decode(json) has key("session_token")
    expect not-server-error: field(status_code) not >= 500
    expect not-retries-in-range: field(retry_count) not between 1 and 5
    expect not-custom-error: field(status_code) not assert plugin.custom(expected: 500)
    expect not-not-found: field(status_code) != 404
    expect no-error-key: field(body) | decode(json) lacks key("error")
    expect deleted-null: field(body) | decode(json) | path("/deleted_at") is null
    expect trace-present: field(body) | decode(json) | path("/trace_id") is present
    expect name-not-null: field(body) | decode(json) | path("/name") is not null
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	expectations := spec.Scenarios[0].Acts[0].Expectations
	testCases := []struct {
		index     int
		wantRef   string
		wantArg   string
		wantValue any
	}{
		{index: 0, wantRef: builtinexpectation.ContainsRef, wantArg: "expected", wantValue: "Example Domain"},
		{index: 1, wantRef: builtinexpectation.GTRef, wantArg: "expected", wantValue: 100},
		{index: 2, wantRef: builtinexpectation.LTRef, wantArg: "expected", wantValue: 10},
		{index: 3, wantRef: builtinexpectation.LTERef, wantArg: "expected", wantValue: 10},
		{index: 4, wantRef: builtinexpectation.GTERef, wantArg: "expected", wantValue: 1},
		{index: 6, wantRef: builtinexpectation.HasKeyRef, wantArg: "key", wantValue: "session_token"},
	}

	for _, testCase := range testCases {
		assertion := expectations[testCase.index].Assert
		if got, want := assertion.Ref, testCase.wantRef; got != want {
			t.Fatalf("assert ref mismatch for expectation %d: got %q want %q", testCase.index, got, want)
		}
		if got, want := assertion.Args[testCase.wantArg].Value, testCase.wantValue; got != want {
			t.Fatalf(
				"assert arg mismatch for expectation %d: got %#v want %#v",
				testCase.index,
				got,
				want,
			)
		}
	}

	between := expectations[5].Assert
	if got, want := between.Ref, builtinexpectation.BetweenRef; got != want {
		t.Fatalf("between assert ref mismatch: got %q want %q", got, want)
	}
	if got, want := between.Args["min"].Value, 1; got != want {
		t.Fatalf("between min mismatch: got %#v want %#v", got, want)
	}
	if got, want := between.Args["max"].Value, 5; got != want {
		t.Fatalf("between max mismatch: got %#v want %#v", got, want)
	}

	negated := expectations[7].Assert
	if got, want := negated.Ref, builtinexpectation.NotRef; got != want {
		t.Fatalf("negated assert ref mismatch: got %q want %q", got, want)
	}

	negatedAssert := negated.Args["assert"]
	if got, want := negatedAssert.Object["ref"].Value, builtinexpectation.GTERef; got != want {
		t.Fatalf("negated nested ref mismatch: got %#v want %#v", got, want)
	}
	if got, want := negatedAssert.Object["args"].Object["expected"].Value, 500; got != want {
		t.Fatalf("negated nested expected mismatch: got %#v want %#v", got, want)
	}

	negatedBetween := expectations[8].Assert
	if got, want := negatedBetween.Ref, builtinexpectation.NotRef; got != want {
		t.Fatalf("negated between ref mismatch: got %q want %q", got, want)
	}
	if got, want := negatedBetween.Args["assert"].Object["ref"].Value, builtinexpectation.BetweenRef; got != want {
		t.Fatalf("negated between nested ref mismatch: got %#v want %#v", got, want)
	}
	if got, want := negatedBetween.Args["assert"].Object["args"].Object["min"].Value, 1; got != want {
		t.Fatalf("negated between min mismatch: got %#v want %#v", got, want)
	}
	if got, want := negatedBetween.Args["assert"].Object["args"].Object["max"].Value, 5; got != want {
		t.Fatalf("negated between max mismatch: got %#v want %#v", got, want)
	}

	negatedCall := expectations[9].Assert
	if got, want := negatedCall.Ref, builtinexpectation.NotRef; got != want {
		t.Fatalf("negated call ref mismatch: got %q want %q", got, want)
	}
	if got, want := negatedCall.Args["assert"].Object["ref"].Value, "plugin.custom"; got != want {
		t.Fatalf("negated call nested ref mismatch: got %#v want %#v", got, want)
	}
	if got, want := negatedCall.Args["assert"].Object["args"].Object["expected"].Value, 500; got != want {
		t.Fatalf("negated call expected mismatch: got %#v want %#v", got, want)
	}

	notEqual := expectations[10].Assert
	if got, want := notEqual.Ref, builtinexpectation.NotRef; got != want {
		t.Fatalf("not equal ref mismatch: got %q want %q", got, want)
	}
	if got, want := notEqual.Args["assert"].Object["ref"].Value, builtinexpectation.EqualRef; got != want {
		t.Fatalf("not equal nested ref mismatch: got %#v want %#v", got, want)
	}
	if got, want := notEqual.Args["assert"].Object["args"].Object["expected"].Value, 404; got != want {
		t.Fatalf("not equal expected mismatch: got %#v want %#v", got, want)
	}

	lacksKey := expectations[11].Assert
	if got, want := lacksKey.Ref, builtinexpectation.LacksKeyRef; got != want {
		t.Fatalf("lacks key ref mismatch: got %q want %q", got, want)
	}
	if got, want := lacksKey.Args["key"].Value, "error"; got != want {
		t.Fatalf("lacks key arg mismatch: got %#v want %#v", got, want)
	}

	for _, testCase := range []struct {
		index   int
		wantRef string
	}{
		{index: 12, wantRef: builtinexpectation.NullRef},
		{index: 13, wantRef: builtinexpectation.PresentRef},
		{index: 14, wantRef: builtinexpectation.NotNullRef},
	} {
		if got, want := expectations[testCase.index].Assert.Ref, testCase.wantRef; got != want {
			t.Fatalf("assert ref mismatch for expectation %d: got %q want %q", testCase.index, got, want)
		}
	}
}

func TestParseLowersHasNoKeySugarWithoutMatcherResolver(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage smoke
scenario ping
  act get-health
    do action.http(method: "GET", url: "/health")
    expect no-error-key: field(body) | decode(json) has no key("error")
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	assertion := spec.Scenarios[0].Acts[0].Expectations[0].Assert
	if got, want := assertion.Ref, builtinexpectation.LacksKeyRef; got != want {
		t.Fatalf("has no key ref mismatch: got %q want %q", got, want)
	}
	if got, want := assertion.Args["key"].Value, "error"; got != want {
		t.Fatalf("has no key arg mismatch: got %#v want %#v", got, want)
	}
}

func TestParseRejectsMissingAndUnsupportedPresenceAssertions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		source     string
		message    string
		wantLine   int
		wantColumn int
	}{
		{
			name:       "missing",
			source:     `field(body) | decode(json) | path("/deleted_at") is missing`,
			message:    missingAssertionMessage,
			wantLine:   5,
			wantColumn: 69,
		},
		{
			name:       "absent",
			source:     `field(body) | decode(json) | path("/deleted_at") is absent`,
			message:    absentAssertionMessage,
			wantLine:   5,
			wantColumn: 69,
		},
		{
			name:       "is not present",
			source:     `field(body) | decode(json) | path("/trace_id") is not present`,
			message:    notPresentAssertionMessage,
			wantLine:   5,
			wantColumn: 71,
		},
		{
			name:       "not present",
			source:     `field(body) | decode(json) | path("/trace_id") not is present`,
			message:    notPresentAssertionMessage,
			wantLine:   5,
			wantColumn: 71,
		},
		{
			name:       "not not-equal",
			source:     `field(status_code) not != 404`,
			message:    `not != is not supported; use ==`,
			wantLine:   5,
			wantColumn: 40,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := Parse([]byte(`stage smoke
scenario ping
  act get-health
    do action.http(method: "GET", url: "/health")
    expect bad: `+testCase.source+`
`), nil)
			if err == nil {
				t.Fatal("expected parse diagnostic error, got nil")
			}

			var diagnosticError *DiagnosticError
			if !errors.As(err, &diagnosticError) {
				t.Fatalf("expected diagnostic error, got %T", err)
			}
			if got, want := diagnosticError.Diagnostic().Code, "thtr_parse_error"; got != want {
				t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
			}
			if got, want := diagnosticError.Diagnostic().Span.Line, testCase.wantLine; got != want {
				t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
			}
			if got, want := diagnosticError.Diagnostic().Span.Column, testCase.wantColumn; got != want {
				t.Fatalf("diagnostic column mismatch: got %d want %d", got, want)
			}

			errtest.RequireContains(t, err, testCase.message)
		})
	}
}

func TestParseRejectsQuotedCoreIdentifiers(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		source string
	}{
		{
			name: "stage id",
			source: `stage "smoke"
scenario ping
  act get-health
    do action.http(method: "GET", url: "/health")
`,
		},
		{
			name: "act id",
			source: `stage smoke
scenario ping
  act "get-health"
    do action.http(method: "GET", url: "/health")
`,
		},
		{
			name: "call id",
			source: `stage smoke
scenario ping
  act get-health
    do action.http(method: "GET", url: "/health")

call "run-ping" = ping()
`,
		},
		{
			name: "scenario target",
			source: `stage smoke
call run-ping = "ping"()
`,
		},
		{
			name: "argument name",
			source: `stage smoke
scenario ping
  act get-health
    do action.http("method": "GET", url: "/health")
`,
		},
		{
			name: "ref name",
			source: `stage smoke
scenario ping
  act get-health
    do action.http(method: "GET", url: "/health")
    export token = $"token"
`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := Parse([]byte(testCase.source), nil)
			if err == nil {
				t.Fatal("expected parse diagnostic error, got nil")
			}

			var diagnosticError *DiagnosticError
			if !errors.As(err, &diagnosticError) {
				t.Fatalf("expected diagnostic error, got %T", err)
			}
			if got, want := diagnosticError.Diagnostic().Code, "thtr_parse_error"; got != want {
				t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
			}

			errtest.RequireContains(t, err, "quoted core identifiers are not supported")
		})
	}
}

func TestParseLowersCollectionWhereSugarWithoutMatcherResolver(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage smoke
scenario ping
  act get-health
    do action.http(method: "GET", url: "/health")
    expect has-demo-notification: field(body) | decode(json) | path("/notifications") has item where path("/receiverAddress") == "demo@example.test"
    expect all-recipients-present: field(body) | decode(json) | path("/notifications") all items where (
      path("/receiverAddress") contains "@example.test",
      path("/subject") not assert plugin.custom(expected: "Verification Code")
    )
    expect active-user: field(body) | decode(json) has entry("status") == "active"
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	expectations := spec.Scenarios[0].Acts[0].Expectations

	hasItem := expectations[0].Assert
	if got, want := hasItem.Ref, builtinexpectation.HasItemRef; got != want {
		t.Fatalf("has item ref mismatch: got %q want %q", got, want)
	}
	if got, want := hasItem.Args["where"].Kind, theater.BindingKindList; got != want {
		t.Fatalf("has item where kind mismatch: got %q want %q", got, want)
	}
	if got, want := len(hasItem.Args["where"].List), 1; got != want {
		t.Fatalf("has item where length mismatch: got %d want %d", got, want)
	}

	firstClause := hasItem.Args["where"].List[0]
	if got, want := firstClause.Kind, theater.BindingKindObject; got != want {
		t.Fatalf("first clause kind mismatch: got %q want %q", got, want)
	}
	if got, want := firstClause.Object["subject"].Object["path"].Value, "/receiverAddress"; got != want {
		t.Fatalf("first clause subject path mismatch: got %#v want %#v", got, want)
	}
	if got, want := firstClause.Object["assert"].Object["ref"].Value, builtinexpectation.EqualRef; got != want {
		t.Fatalf("first clause assert ref mismatch: got %#v want %#v", got, want)
	}
	if got, want := firstClause.Object["assert"].Object["args"].Object["expected"].Value, "demo@example.test"; got != want {
		t.Fatalf("first clause assert expected mismatch: got %#v want %#v", got, want)
	}

	allItems := expectations[1].Assert
	if got, want := allItems.Ref, builtinexpectation.AllItemsRef; got != want {
		t.Fatalf("all items ref mismatch: got %q want %q", got, want)
	}
	if got, want := len(allItems.Args["where"].List), 2; got != want {
		t.Fatalf("all items where length mismatch: got %d want %d", got, want)
	}
	if got, want := allItems.Args["where"].List[0].Object["assert"].Object["ref"].Value, builtinexpectation.ContainsRef; got != want {
		t.Fatalf("first grouped clause assert ref mismatch: got %#v want %#v", got, want)
	}

	negatedClauseAssert := allItems.Args["where"].List[1].Object["assert"]
	if got, want := negatedClauseAssert.Object["ref"].Value, builtinexpectation.NotRef; got != want {
		t.Fatalf("negated clause assert ref mismatch: got %#v want %#v", got, want)
	}
	if got, want := negatedClauseAssert.Object["args"].Object["assert"].Object["ref"].Value, "plugin.custom"; got != want {
		t.Fatalf("negated clause nested ref mismatch: got %#v want %#v", got, want)
	}
	if got, want := negatedClauseAssert.Object["args"].Object["assert"].Object["args"].Object["expected"].Value, "Verification Code"; got != want {
		t.Fatalf("negated clause nested expected mismatch: got %#v want %#v", got, want)
	}

	hasEntry := expectations[2].Assert
	if got, want := hasEntry.Ref, builtinexpectation.HasEntryRef; got != want {
		t.Fatalf("has entry ref mismatch: got %q want %q", got, want)
	}
	if got, want := hasEntry.Args["key"].Value, "status"; got != want {
		t.Fatalf("has entry key mismatch: got %#v want %#v", got, want)
	}
	nestedAssert := hasEntry.Args["assert"].Object
	if got, want := nestedAssert["ref"].Value, builtinexpectation.EqualRef; got != want {
		t.Fatalf("has entry nested ref mismatch: got %#v want %#v", got, want)
	}
	if got, want := nestedAssert["args"].Object["expected"].Value, "active"; got != want {
		t.Fatalf("has entry nested expected mismatch: got %#v want %#v", got, want)
	}
}

func TestParseLowersCanonicalBetweenAssertWithoutMatcherResolver(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage smoke
scenario ping
  act get-health
    do action.http(method: "GET", url: "/health")
    expect retries-range: field(retry_count) assert expectation.between(min: 1, max: 5)
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	assertion := spec.Scenarios[0].Acts[0].Expectations[0].Assert
	if got, want := assertion.Ref, builtinexpectation.BetweenRef; got != want {
		t.Fatalf("canonical between ref mismatch: got %q want %q", got, want)
	}
	if got, want := assertion.Args["min"].Value, 1; got != want {
		t.Fatalf("canonical between min mismatch: got %#v want %#v", got, want)
	}
	if got, want := assertion.Args["max"].Value, 5; got != want {
		t.Fatalf("canonical between max mismatch: got %#v want %#v", got, want)
	}
}

func TestParseLowersNegatedCanonicalBetweenAssertWithoutMatcherResolver(t *testing.T) {
	t.Parallel()

	spec, err := Parse([]byte(`stage smoke
scenario ping
  act get-health
    do action.http(method: "GET", url: "/health")
    expect retries-range: field(retry_count) not assert expectation.between(min: 1, max: 5)
`), nil)
	if err != nil {
		failWithSpan(t, "parse", err)
	}

	assertion := spec.Scenarios[0].Acts[0].Expectations[0].Assert
	if got, want := assertion.Ref, builtinexpectation.NotRef; got != want {
		t.Fatalf("negated canonical between ref mismatch: got %q want %q", got, want)
	}
	if got, want := assertion.Args["assert"].Object["ref"].Value, builtinexpectation.BetweenRef; got != want {
		t.Fatalf("negated canonical between nested ref mismatch: got %#v want %#v", got, want)
	}
	if got, want := assertion.Args["assert"].Object["args"].Object["min"].Value, 1; got != want {
		t.Fatalf("negated canonical between min mismatch: got %#v want %#v", got, want)
	}
	if got, want := assertion.Args["assert"].Object["args"].Object["max"].Value, 5; got != want {
		t.Fatalf("negated canonical between max mismatch: got %#v want %#v", got, want)
	}
}

func TestLoadFileDetailedReturnsLoweringDiagnosticBreadcrumbForRelativeClauseSubject(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.thtr")
	if err := os.WriteFile(path, []byte(`stage main
scenario login
  act submit
    do action.http(method: "GET", url: "/health")
    expect bad: field(body) | decode(json) | path("/notifications") has item where field(status_code) == 200
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := LoadFileDetailed(path, nil)
	if err == nil {
		t.Fatal("expected lowering diagnostic error, got nil")
	}

	var diagnosticError *DiagnosticError
	if !errors.As(err, &diagnosticError) {
		t.Fatalf("expected diagnostic error, got %T", err)
	}

	diagnostic := diagnosticError.Diagnostic()
	if got, want := diagnostic.Code, "thtr_lower_error"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Path, "stage.main/scenario.login/act.submit/expectation.bad/assert/clause[0]/subject"; got != want {
		t.Fatalf("diagnostic breadcrumb path mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 5; got != want {
		t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostic.Summary, `relative clause subject may start only with decode(...) or path(...)`; got != want {
		t.Fatalf("diagnostic summary mismatch: got %q want %q", got, want)
	}
}

func TestLoadFileDetailedRewritesCollectionClauseValidationDiagnostics(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.thtr")
	if err := os.WriteFile(path, []byte(`stage main
scenario login
  act submit
    do action.http(method: "GET", url: "/health")
    expect receiver-present: field(body) | decode(json) | path("/notifications") has item where path("/receiverAddress") == $expected_email
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	result, err := LoadFileDetailed(path, nil)
	if err != nil {
		t.Fatalf("load file detailed failed: %v", err)
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.http", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"body": {Kind: theater.ValueKindString},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers, err := theater.NewMatcherCatalog(builtinexpectation.Descriptors()...)
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	validator := theater.NewValidator(catalog, matchers)
	diagnostics := result.RewriteDiagnostics(validator.Validate(result.Spec))
	diagnostic := findDiagnosticByCodeValue(diagnostics, "unresolved_binding_ref")
	if diagnostic == nil {
		t.Fatalf("expected unresolved_binding_ref diagnostic, got %#v", diagnostics)
	}
	if got, want := diagnostic.Span.File, path; got != want {
		t.Fatalf("diagnostic source file mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 5; got != want {
		t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
	}
}

func TestParseRejectsDuplicateProperties(t *testing.T) {
	t.Parallel()

	_, err := Parse([]byte(`stage smoke
scenario login
  act submit
    prop token = inventory.http.get(url: "/one")
    prop token = inventory.http.get(url: "/two")
    do action.http(method: "GET")
`), nil)

	errtest.RequireContains(t, err, `property "token" is duplicated`)
}

func TestParseRejectsLateDecodeSelectorStep(t *testing.T) {
	t.Parallel()

	_, err := Parse([]byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET")
    expect token: field(body) | path("/data") | decode(json) == "x"
`), nil)

	errtest.RequireContains(t, err, "decode step must appear before path and through steps")
}

func TestLoadFileDetailedReturnsLoweringDiagnosticBreadcrumbForCaptureAuthSlot(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.thtr")
	if err := os.WriteFile(path, []byte(`stage main
scenario login
  act submit
    do action.http(method: "POST", url: "/login")
    capture_auth web
      csrf: decode(json)
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := LoadFileDetailed(path, nil)
	if err == nil {
		t.Fatal("expected lowering diagnostic error, got nil")
	}

	var diagnosticError *DiagnosticError
	if !errors.As(err, &diagnosticError) {
		t.Fatalf("expected diagnostic error, got %T", err)
	}

	diagnostic := diagnosticError.Diagnostic()
	if got, want := diagnostic.Code, "thtr_lower_error"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Path, "stage.main/scenario.login/act.submit/capture_auth/slot.csrf"; got != want {
		t.Fatalf("diagnostic breadcrumb path mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 6; got != want {
		t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
	}
}

func TestLoadFileDetailedRewritesCaptureAuthValidationDiagnostics(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.thtr")
	if err := os.WriteFile(path, []byte(`stage main

http
  auth web = http.auth(
    attach: list [
      object { header_slot: object { name: "X-Session", slot: "session" } },
    ],
  )

scenario login
  act submit
    do action.http(method: "POST", url: "/login")
    capture_auth web
      csrf: response_header("X-CSRF-Token")
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	result, err := LoadFileDetailed(path, nil)
	if err != nil {
		t.Fatalf("load file detailed failed: %v", err)
	}

	validator := theater.NewValidator(nil, nil)
	diagnostics := result.RewriteDiagnostics(validator.Validate(result.Spec))
	diagnostic := findDiagnosticByCodeValue(diagnostics, "unknown_http_capture_slot_ref")
	if diagnostic == nil {
		t.Fatalf("expected unknown_http_capture_slot_ref diagnostic, got %#v", diagnostics)
	}
	if got, want := diagnostic.Span.File, path; got != want {
		t.Fatalf("diagnostic source file mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 14; got != want {
		t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
	}
}

func TestLoadFileDetailedRewritesInvalidCaptureAuthUsageDiagnostic(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.thtr")
	if err := os.WriteFile(path, []byte(`stage main
scenario login
  act submit
    do action.command(executable: "true")
    capture_auth web
      csrf: response_header("X-CSRF-Token")
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	result, err := LoadFileDetailed(path, nil)
	if err != nil {
		t.Fatalf("load file detailed failed: %v", err)
	}

	validator := theater.NewValidator(nil, nil)
	diagnostics := result.RewriteDiagnostics(validator.Validate(result.Spec))
	diagnostic := findDiagnosticByCodeValue(diagnostics, "invalid_http_capture_auth_usage")
	if diagnostic == nil {
		t.Fatalf("expected invalid_http_capture_auth_usage diagnostic, got %#v", diagnostics)
	}
	if got, want := diagnostic.Path, "stage.main/scenario.login/act.submit/capture_auth"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.File, path; got != want {
		t.Fatalf("diagnostic source file mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 5; got != want {
		t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
	}
}

func TestParseRejectsDuplicateCaptureAuthSlots(t *testing.T) {
	t.Parallel()

	_, err := Parse([]byte(`stage main
scenario login
  act submit
    do action.http(method: "POST", url: "/login")
    capture_auth web
      csrf: response_header("X-CSRF-Token")
      csrf: response_cookie("session")
`), nil)

	errtest.RequireContains(t, err, `capture_auth slot "csrf" is duplicated`)
}

func TestLoadFileDetailedRewritesDependencyValidationDiagnostics(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.thtr")
	if err := os.WriteFile(path, []byte(`stage main
scenario login
  act submit
    do action.http(method: "POST", url: "/login")

call run-login = login()
  dependency bootstrap
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	result, err := LoadFileDetailed(path, nil)
	if err != nil {
		t.Fatalf("load file detailed failed: %v", err)
	}

	validator := theater.NewValidator(nil, nil)
	diagnostics := result.RewriteDiagnostics(validator.Validate(result.Spec))
	diagnostic := findDiagnosticByCodeValue(diagnostics, "missing_dependency_ref")
	if diagnostic == nil {
		t.Fatalf("expected missing_dependency_ref diagnostic, got %#v", diagnostics)
	}
	if got, want := diagnostic.Span.File, path; got != want {
		t.Fatalf("diagnostic source file mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.Line, 7; got != want {
		t.Fatalf("diagnostic source line mismatch: got %d want %d", got, want)
	}
}

func failWithSpan(t *testing.T, action string, err error) {
	t.Helper()

	type spanProvider interface {
		Span() sourceSpan
	}

	if provider, ok := err.(spanProvider); ok {
		span := provider.Span()
		t.Fatalf("%s failed at %d:%d: %v", action, span.Start.Line, span.Start.Column, err)
	}

	t.Fatalf("%s failed: %v", action, err)
}

func requireTHSourceSpanAt(
	t *testing.T,
	sourceRef *theater.SourceRef,
	file string,
	source string,
	needle string,
) {
	t.Helper()

	wantLine, wantColumn := thtrFixtureSourcePosition(t, source, needle)
	if sourceRef == nil {
		t.Fatalf("source span is nil, want %s:%d:%d", file, wantLine, wantColumn)
	}
	if got := sourceRef.File; got != file {
		t.Fatalf("source file mismatch for %q: got %q want %q", needle, got, file)
	}
	if got := sourceRef.Line; got != wantLine {
		t.Fatalf("source line mismatch for %q: got %d want %d", needle, got, wantLine)
	}
	if got := sourceRef.Column; got != wantColumn {
		t.Fatalf("source column mismatch for %q: got %d want %d", needle, got, wantColumn)
	}
}

func thtrFixtureSourcePosition(t *testing.T, source string, needle string) (line int, column int) {
	t.Helper()

	offset := strings.Index(source, needle)
	if offset < 0 {
		t.Fatalf("source fixture does not contain %q", needle)
	}

	line, column = 1, 1
	for _, char := range source[:offset] {
		if char == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}

	return line, column
}
