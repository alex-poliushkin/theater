package yaml_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex-poliushkin/theater"
	builtinexpectation "github.com/alex-poliushkin/theater/builtin/expectation"
	"github.com/alex-poliushkin/theater/internal/testkit/errtest"
	theateryaml "github.com/alex-poliushkin/theater/yaml"
)

const (
	yamlUnknownFieldFragment         = "field unknown_field not found"
	yamlMultipleDocumentsMessage     = "multiple YAML documents are not supported"
	yamlFlowOutsideRootFragment      = "must be located under"
	yamlLibraryScenarioCallsFragment = "must not declare scenario_calls"
	yamlMissingRepoRootsFragment     = "repo-local theater roots not found"
	yamlLegacyMaxAttemptsFragment    = "field max_attempts not found"
	yamlMissingSugarResolverFragment = `assert matcher "eq" requires matcher sugar resolver`
	yamlPickWhereMixedFragment       = "pick where cannot be combined with at or equals"
	yamlPickWhereSubjectFragment     = "subject must declare decode or path"
)

type matcherSugarResolver struct {
	descriptor theater.MatcherDescriptor
}

func (r matcherSugarResolver) ResolveSugarKey(key string) (theater.MatcherDescriptor, error) {
	if key != "eq" {
		return theater.MatcherDescriptor{}, errors.New("unexpected matcher sugar")
	}

	return r.descriptor, nil
}

func TestParseDecodesStageSpecWithStrictKnownFields(t *testing.T) {
	t.Parallel()

	spec, err := theateryaml.Parse([]byte(`
id: main
scenarios:
  - id: register
    acts:
      - id: submit
        action:
          use: action.register
        exports:
          - field: token
scenario_calls:
  - id: register-user
    scenario_id: register
    exports:
      - ref: token
        as: issued_token
`), matcherCatalog(t))
	if err != nil {
		t.Fatalf("parse yaml failed: %v", err)
	}

	if got, want := spec.ID, "main"; got != want {
		t.Fatalf("stage id mismatch: got %q want %q", got, want)
	}

	if got, want := len(spec.Scenarios), 1; got != want {
		t.Fatalf("scenario count mismatch: got %d want %d", got, want)
	}

	if got, want := spec.Scenarios[0].Acts[0].Action.Use, "action.register"; got != want {
		t.Fatalf("action ref mismatch: got %q want %q", got, want)
	}

	if got, want := spec.ScenarioCalls[0].Exports[0].As, "issued_token"; got != want {
		t.Fatalf("export alias mismatch: got %q want %q", got, want)
	}
}

func TestParseDecodesActLogs(t *testing.T) {
	t.Parallel()

	spec, err := theateryaml.Parse([]byte(`
id: main
scenarios:
  - id: register
    acts:
      - id: submit
        action:
          use: action.register
        logs:
          - id: response
            format: json
            capture: summary
            sensitivity: internal
            required: true
            value:
              object:
                status:
                  field: status_code
                user_id:
                  field: body
                  decode: json
                  path: /data/id
          - id: audit
            message: response received
            fields:
              request_id:
                ref: request_id
              body:
                field: body
                through:
                  - regexp:
                      pattern: '"id":"([^"]+)"'
                      group: 1
scenario_calls:
  - id: register-user
    scenario_id: register
`), matcherCatalog(t))
	if err != nil {
		t.Fatalf("parse yaml failed: %v", err)
	}

	logs := spec.Scenarios[0].Acts[0].Logs
	if got, want := len(logs), 2; got != want {
		t.Fatalf("log count mismatch: got %d want %d", got, want)
	}

	response := logs[0]
	if got, want := response.ID, "response"; got != want {
		t.Fatalf("log id mismatch: got %q want %q", got, want)
	}
	if got, want := response.Format, theater.LogFormatJSON; got != want {
		t.Fatalf("log format mismatch: got %q want %q", got, want)
	}
	if got, want := response.Capture, theater.CaptureSummary; got != want {
		t.Fatalf("log capture mismatch: got %q want %q", got, want)
	}
	if got, want := response.Sensitivity, theater.SensitivityInternal; got != want {
		t.Fatalf("log sensitivity mismatch: got %q want %q", got, want)
	}
	if !response.Required {
		t.Fatal("log required flag must be true")
	}
	if got, want := response.Value.Object["status"].Field, "status_code"; got != want {
		t.Fatalf("object field selector mismatch: got %q want %q", got, want)
	}
	if got, want := response.Value.Object["user_id"].Path, theater.JSONPointer("/data/id"); got != want {
		t.Fatalf("object path selector mismatch: got %q want %q", got, want)
	}

	audit := logs[1]
	if got, want := audit.Message, "response received"; got != want {
		t.Fatalf("log message mismatch: got %q want %q", got, want)
	}
	if got, want := audit.Fields["request_id"].Ref, "request_id"; got != want {
		t.Fatalf("field ref mismatch: got %q want %q", got, want)
	}
	if audit.Fields["body"].Through[0].Regexp == nil {
		t.Fatal("field through regexp must be decoded")
	}
}

func TestParseRejectsUnknownActLogFields(t *testing.T) {
	t.Parallel()

	_, err := theateryaml.Parse([]byte(`
id: main
scenarios:
  - id: register
    acts:
      - id: submit
        action:
          use: action.register
        logs:
          - id: response
            destination: stdout
            value:
              field: body
scenario_calls:
  - id: register-user
    scenario_id: register
`), matcherCatalog(t))
	if err == nil {
		t.Fatal("expected unknown log field error")
	}

	if !strings.Contains(err.Error(), "field destination not found") {
		t.Fatalf("error mismatch: got %q", err)
	}
}

func TestParseRejectsInvalidActLogValues(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		logValue string
		fragment string
	}{
		{
			name: "scalar value",
			logValue: `
            value: body
`,
			fragment: "log value must be object",
		},
		{
			name: "object not mapping",
			logValue: `
            value:
              object: []
`,
			fragment: "log value object must be object",
		},
		{
			name: "list not sequence",
			logValue: `
            value:
              list: {}
`,
			fragment: "log value list must be sequence",
		},
		{
			name: "unknown nested field",
			logValue: `
            value:
              object:
                response:
                  destination: stdout
`,
			fragment: "field destination not found in type theater.LogValueSpec",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := theateryaml.Parse([]byte(`
id: main
scenarios:
  - id: register
    acts:
      - id: submit
        action:
          use: action.register
        logs:
          - id: response
`+tc.logValue+`
scenario_calls:
  - id: register-user
    scenario_id: register
`), matcherCatalog(t))
			if err == nil {
				t.Fatal("expected invalid log value error")
			}

			if !strings.Contains(err.Error(), tc.fragment) {
				t.Fatalf("error mismatch: got %q want fragment %q", err, tc.fragment)
			}
		})
	}
}

func TestParseDecodesTopLevelHTTPSessionsAndAuth(t *testing.T) {
	t.Parallel()

	spec, err := theateryaml.Parse([]byte(`
id: main
http:
  sessions:
    web: {}
  auth:
    ci_api:
      attach:
        - bearer:
            token: static-token
        - api_key:
            in: query
            name: api_key
            value: query-token
scenarios: []
scenario_calls: []
`), matcherCatalog(t))
	if err != nil {
		t.Fatalf("parse yaml failed: %v", err)
	}

	if spec.HTTP == nil {
		t.Fatal("http spec must be present")
	}
	if _, ok := spec.HTTP.Sessions["web"]; !ok {
		t.Fatal("http session web must be present")
	}
	auth, ok := spec.HTTP.Auth["ci_api"]
	if !ok {
		t.Fatal("http auth ci_api must be present")
	}
	if got, want := len(auth.Attach), 2; got != want {
		t.Fatalf("auth attachment count mismatch: got %d want %d", got, want)
	}
	if auth.Attach[0].Bearer == nil || auth.Attach[0].Bearer.Token != "static-token" {
		t.Fatalf("bearer attachment mismatch: %#v", auth.Attach[0].Bearer)
	}
	if auth.Attach[1].APIKey == nil || auth.Attach[1].APIKey.In != theater.HTTPAPIKeyInQuery {
		t.Fatalf("api_key attachment mismatch: %#v", auth.Attach[1].APIKey)
	}
}

func TestParseDecodesTopLevelStateBackends(t *testing.T) {
	t.Parallel()

	spec, err := theateryaml.Parse([]byte(`
id: main
state:
  backends:
    local:
      use: state.backend.file
      with:
        root: /tmp/theater-state
scenarios: []
scenario_calls: []
`), matcherCatalog(t))
	if err != nil {
		t.Fatalf("parse yaml failed: %v", err)
	}

	if spec.State == nil {
		t.Fatal("state spec must be present")
	}

	backend, ok := spec.State.Backends["local"]
	if !ok {
		t.Fatal("state backend local must be present")
	}

	if got, want := backend.Use, "state.backend.file"; got != want {
		t.Fatalf("backend use mismatch: got %q want %q", got, want)
	}

	if got, want := backend.With["root"], "/tmp/theater-state"; got != want {
		t.Fatalf("backend root mismatch: got %#v want %#v", got, want)
	}
}

func TestParseDecodesHTTPIdentitiesAndCaptureAuth(t *testing.T) {
	t.Parallel()

	spec, err := theateryaml.Parse([]byte(`
id: main
http:
  sessions:
    web: {}
  auth:
    web_csrf:
      attach:
        - header_slot:
            name: X-CSRF-Token
            slot: csrf
        - form_slot:
            name: csrf
            slot: csrf
  identities:
    user:
      session: web
      auth: web_csrf
scenarios:
  - id: login
    acts:
      - id: submit
        action:
          use: action.http
          with:
            method: POST
            url: https://example.test/login
            identity: user
            auth: none
            form:
              username: demo
        capture_auth:
          auth: web_csrf
          slots:
            csrf:
              response_header: X-CSRF-Token
scenario_calls:
  - id: login-user
    scenario_id: login
`), matcherCatalog(t))
	if err != nil {
		t.Fatalf("parse yaml failed: %v", err)
	}

	if spec.HTTP == nil {
		t.Fatal("http spec must be present")
	}

	identity, ok := spec.HTTP.Identities["user"]
	if !ok {
		t.Fatal("http identity user must be present")
	}
	if got, want := identity.Session, "web"; got != want {
		t.Fatalf("identity session mismatch: got %q want %q", got, want)
	}
	if got, want := identity.Auth, "web_csrf"; got != want {
		t.Fatalf("identity auth mismatch: got %q want %q", got, want)
	}

	auth, ok := spec.HTTP.Auth["web_csrf"]
	if !ok {
		t.Fatal("http auth web_csrf must be present")
	}
	if auth.Attach[0].HeaderSlot == nil || auth.Attach[0].HeaderSlot.Slot != "csrf" {
		t.Fatalf("header slot attachment mismatch: %#v", auth.Attach[0].HeaderSlot)
	}
	if auth.Attach[1].FormSlot == nil || auth.Attach[1].FormSlot.Name != "csrf" {
		t.Fatalf("form slot attachment mismatch: %#v", auth.Attach[1].FormSlot)
	}

	actionWith := spec.Scenarios[0].Acts[0].Action.With
	if got, want := actionWith["identity"].Value, "user"; got != want {
		t.Fatalf("identity binding mismatch: got %#v want %#v", got, want)
	}
	if got, want := actionWith["auth"].Value, theater.HTTPAuthNone; got != want {
		t.Fatalf("auth binding mismatch: got %#v want %#v", got, want)
	}
	if got, want := actionWith["form"].Kind, theater.BindingKindLiteral; got != want {
		t.Fatalf("form binding kind mismatch: got %q want %q", got, want)
	}

	capture := spec.Scenarios[0].Acts[0].CaptureAuth
	if capture == nil {
		t.Fatal("capture_auth must be present")
	}
	if got, want := capture.Auth, "web_csrf"; got != want {
		t.Fatalf("capture auth ref mismatch: got %q want %q", got, want)
	}
	if got, want := capture.Slots["csrf"].ResponseHeader, "X-CSRF-Token"; got != want {
		t.Fatalf("capture header mismatch: got %q want %q", got, want)
	}
}

func TestParseDecodesStringBindingsJSONBodiesAndThroughPipeline(t *testing.T) {
	t.Parallel()

	spec, err := theateryaml.Parse([]byte(`
id: main
scenarios:
  - id: verify
    acts:
      - id: submit
        action:
          use: action.http
          with:
            method: POST
            url:
              kind: string
              parts:
                - /flows/
                - kind: ref
                  ref: create_flow
                  field: body
                  decode: json
                  path: /flowId
                - /verifications/email
            json:
              kind: object
              object:
                code:
                  kind: ref
                  ref:
                    name: notifications
                    decode: json
                    path: /data
                    through:
                      - pick:
                          at: /receiverAddress
                          equals: demo@example.test
                      - path: /body
                      - regexp:
                          pattern: '\b([0-9]{6})\b'
                          group: 1
        exports:
          - as: otp
            field: body
            decode: json
            path: /data
            through:
              - pick:
                  at: /receiverAddress
                  equals: demo@example.test
              - path: /body
              - regexp:
                  pattern: '\b([0-9]{6})\b'
                  group: 1
scenario_calls:
  - id: verify-user
    scenario_id: verify
`), matcherCatalog(t))
	if err != nil {
		t.Fatalf("parse yaml failed: %v", err)
	}

	with := spec.Scenarios[0].Acts[0].Action.With
	if got, want := with["url"].Kind, theater.BindingKindString; got != want {
		t.Fatalf("url binding kind mismatch: got %q want %q", got, want)
	}
	if got, want := len(with["url"].Parts), 3; got != want {
		t.Fatalf("url part count mismatch: got %d want %d", got, want)
	}

	jsonBody := with["json"]
	if got, want := jsonBody.Kind, theater.BindingKindObject; got != want {
		t.Fatalf("json binding kind mismatch: got %q want %q", got, want)
	}

	codeBinding, ok := jsonBody.Object["code"]
	if !ok {
		t.Fatal("json code binding must be present")
	}
	if codeBinding.Ref == nil {
		t.Fatal("json code ref must be present")
	}
	if got, want := len(codeBinding.Ref.Through), 3; got != want {
		t.Fatalf("json code through count mismatch: got %d want %d", got, want)
	}
	if codeBinding.Ref.Through[0].Pick == nil {
		t.Fatal("first through step must be pick")
	}
	if codeBinding.Ref.Through[2].Regexp == nil {
		t.Fatal("third through step must be regexp")
	}

	exportSpec := spec.Scenarios[0].Acts[0].Exports[0]
	if got, want := len(exportSpec.Through), 3; got != want {
		t.Fatalf("export through count mismatch: got %d want %d", got, want)
	}
}

func TestParseDecodesPickWhereThroughPipeline(t *testing.T) {
	t.Parallel()

	spec, err := theateryaml.Parse([]byte(`
id: main
scenarios:
  - id: verify
    acts:
      - id: poll
        action:
          use: action.http
          with:
            method: GET
            url: /notifications
        exports:
          - as: otp
            field: body
            decode: json
            path: /items
            through:
              - pick:
                  where:
                    - subject:
                        path: /receiverAddress
                      assert:
                        ref: expectation.equal
                        args:
                          expected:
                            kind: ref
                            ref: email
                    - subject:
                        path: /subject
                      assert:
                        ref: expectation.contains
                        args:
                          expected: Verification
              - path: /body
scenario_calls:
  - id: verify-user
    scenario_id: verify
`), matcherCatalog(t))
	if err != nil {
		t.Fatalf("parse yaml failed: %v", err)
	}

	exportSpec := spec.Scenarios[0].Acts[0].Exports[0]
	if got, want := len(exportSpec.Through), 2; got != want {
		t.Fatalf("export through count mismatch: got %d want %d", got, want)
	}

	pick := exportSpec.Through[0].Pick
	if pick == nil {
		t.Fatal("first through step must be pick")
	}
	if got, want := len(pick.Where), 2; got != want {
		t.Fatalf("pick where count mismatch: got %d want %d", got, want)
	}
	if got, want := pick.Where[0].Subject.Path, theater.JSONPointer("/receiverAddress"); got != want {
		t.Fatalf("first where subject path mismatch: got %q want %q", got, want)
	}
	if got, want := pick.Where[0].Assert.Ref, builtinexpectation.EqualRef; got != want {
		t.Fatalf("first where assert ref mismatch: got %q want %q", got, want)
	}
	if pick.Where[0].Assert.Args["expected"].Ref == nil || pick.Where[0].Assert.Args["expected"].Ref.Name != "email" {
		t.Fatal("first where expected arg must be email ref")
	}
}

func TestParseRejectsPickWhereMixedWithAtEquals(t *testing.T) {
	t.Parallel()

	_, err := theateryaml.Parse([]byte(`
id: main
scenarios:
  - id: verify
    acts:
      - id: poll
        action:
          use: action.http
        exports:
          - as: otp
            field: body
            through:
              - pick:
                  at: /receiverAddress
                  equals:
                    kind: ref
                    ref: email
                  where:
                    - subject:
                        path: /subject
                      assert:
                        ref: expectation.contains
                        args:
                          expected: Verification
scenario_calls:
  - id: verify-user
    scenario_id: verify
`), matcherCatalog(t))
	if err == nil {
		t.Fatal("expected mixed pick where error, got nil")
	}

	errtest.RequireContains(t, err, yamlPickWhereMixedFragment)
}

func TestParseRejectsPickWhereEmptySubject(t *testing.T) {
	t.Parallel()

	_, err := theateryaml.Parse([]byte(`
id: main
scenarios:
  - id: verify
    acts:
      - id: poll
        action:
          use: action.http
        exports:
          - as: otp
            field: body
            through:
              - pick:
                  where:
                    - subject: {}
                      assert:
                        ref: expectation.contains
                        args:
                          expected: Verification
scenario_calls:
  - id: verify-user
    scenario_id: verify
`), matcherCatalog(t))
	if err == nil {
		t.Fatal("expected empty pick where subject error, got nil")
	}

	errtest.RequireContains(t, err, yamlPickWhereSubjectFragment)
}

func TestParseRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	_, err := theateryaml.Parse([]byte(`
id: main
unknown_field: true
scenarios: []
scenario_calls: []
`), matcherCatalog(t))
	if err == nil {
		t.Fatal("expected strict yaml error, got nil")
	}

	errtest.RequireContains(t, err, yamlUnknownFieldFragment)
}

func TestParseRejectsMultipleDocuments(t *testing.T) {
	t.Parallel()

	_, err := theateryaml.Parse([]byte(`
id: main
scenarios: []
scenario_calls: []
---
id: other
scenarios: []
scenario_calls: []
`), matcherCatalog(t))
	if err == nil {
		t.Fatal("expected multiple document error, got nil")
	}

	errtest.RequireEqual(t, err, yamlMultipleDocumentsMessage)
}

func TestLoadFileReadsStageSpecFromDisk(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "stage.yaml")
	if err := os.WriteFile(path, []byte(`
id: main
scenarios:
  - id: login
    acts:
      - id: submit
        eventually:
          timeout: 30s
          interval: 2s
        action:
          use: action.login
          repeatable: true
        expectations:
          - id: token
            subject: token
            assert:
              eq: issued-token
scenario_calls:
  - id: login-user
    scenario_id: login
`), 0o600); err != nil {
		t.Fatalf("write yaml failed: %v", err)
	}

	spec, err := theateryaml.LoadFile(path, matcherCatalog(t))
	if err != nil {
		t.Fatalf("load file failed: %v", err)
	}

	if spec.Scenarios[0].Acts[0].Eventually == nil {
		t.Fatal("eventually spec is nil")
	}

	if got, want := spec.Scenarios[0].Acts[0].Eventually.Timeout, "30s"; got != want {
		t.Fatalf("eventually timeout mismatch: got %q want %q", got, want)
	}

	if got, want := spec.Scenarios[0].Acts[0].Action.Repeatable, true; got != want {
		t.Fatalf("action repeatable mismatch: got %v want %v", got, want)
	}

	if got, want := spec.ScenarioCalls[0].ID, "login-user"; got != want {
		t.Fatalf("scenario call id mismatch: got %q want %q", got, want)
	}

	if got, want := spec.ScenarioCalls[0].ScenarioID, "login"; got != want {
		t.Fatalf("scenario call scenario id mismatch: got %q want %q", got, want)
	}
}

func TestParseAcceptsDedicatedMatcherSugarResolver(t *testing.T) {
	t.Parallel()

	spec, err := theateryaml.Parse([]byte(`
id: main
scenarios:
  - id: login
    acts:
      - id: submit
        action:
          use: action.login
        expectations:
          - id: token
            subject: token
            assert:
              eq: issued-token
scenario_calls:
  - id: login-user
    scenario_id: login
`), matcherSugarResolver{
		descriptor: theater.MatcherDescriptor{
			Ref: "expectation.equal",
			Sugar: theater.SugarSpec{
				Keys:           []string{"eq"},
				Form:           theater.SugarFormUnary,
				PositionalArgs: []string{"expected"},
			},
		},
	})
	if err != nil {
		t.Fatalf("parse yaml failed: %v", err)
	}

	expectation := spec.Scenarios[0].Acts[0].Expectations[0]
	if got, want := expectation.Assert.Ref, "expectation.equal"; got != want {
		t.Fatalf("assert ref mismatch: got %q want %q", got, want)
	}
	if got, want := expectation.Assert.Args["expected"].Value, "issued-token"; got != want {
		t.Fatalf("assert arg mismatch: got %#v want %#v", got, want)
	}
}

func TestParseAcceptsCollectionMatcherSugarAndImplicitLiteralBindings(t *testing.T) {
	t.Parallel()

	spec, err := theateryaml.Parse([]byte(`
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        properties:
          url:
            inventory:
              use: inventory.env
              with:
                name: THEATER_API_URL
        action:
          use: action.http
          with:
            method: GET
            url:
              kind: ref
              ref: url
        expectations:
          - id: receiver
            subject:
              field: body
              decode: json
              path: /data
            assert:
              has_item:
                - subject:
                    path: /receiverAddress
                  assert:
                    eq: "+13146235623"
scenario_calls:
  - id: probe-call
    scenario_id: probe
    bindings:
      tenant: dev
`), matcherCatalog(t))
	if err != nil {
		t.Fatalf("parse yaml failed: %v", err)
	}

	propertyWith := spec.Scenarios[0].Acts[0].Properties["url"].Inventory.With
	if got, want := propertyWith["name"].Kind, theater.BindingKindLiteral; got != want {
		t.Fatalf("inventory binding kind mismatch: got %q want %q", got, want)
	}

	actionWith := spec.Scenarios[0].Acts[0].Action.With
	if got, want := actionWith["method"].Kind, theater.BindingKindLiteral; got != want {
		t.Fatalf("action binding kind mismatch: got %q want %q", got, want)
	}

	expectation := spec.Scenarios[0].Acts[0].Expectations[0]
	if got, want := expectation.Assert.Ref, builtinexpectation.HasItemRef; got != want {
		t.Fatalf("assert ref mismatch: got %q want %q", got, want)
	}

	where := expectation.Assert.Args["where"]
	if got, want := where.Kind, theater.BindingKindLiteral; got != want {
		t.Fatalf("where binding kind mismatch: got %q want %q", got, want)
	}

	bindings := spec.ScenarioCalls[0].Bindings
	if got, want := bindings["tenant"].Kind, theater.BindingKindLiteral; got != want {
		t.Fatalf("scenario binding kind mismatch: got %q want %q", got, want)
	}
}

func TestParseAcceptsCanonicalAssertWithoutMatcherResolver(t *testing.T) {
	t.Parallel()

	spec, err := theateryaml.Parse([]byte(`
id: main
scenarios:
  - id: login
    acts:
      - id: submit
        action:
          use: action.login
        expectations:
          - id: token
            subject: token
            assert:
              ref: expectation.equal
              args:
                expected:
                  kind: literal
                  value: issued-token
scenario_calls:
  - id: login-user
    scenario_id: login
`), nil)
	if err != nil {
		t.Fatalf("parse yaml failed: %v", err)
	}

	if got, want := spec.Scenarios[0].Acts[0].Expectations[0].Assert.Ref, "expectation.equal"; got != want {
		t.Fatalf("assert ref mismatch: got %q want %q", got, want)
	}
}

func TestParseDecodesCanonicalNotMatcherWithoutMatcherResolver(t *testing.T) {
	t.Parallel()

	spec, err := theateryaml.Parse([]byte(`
id: main
scenarios:
  - id: login
    acts:
      - id: submit
        action:
          use: action.login
        expectations:
          - id: not-server-error
            subject: status_code
            assert:
              ref: expectation.not
              args:
                assert:
                  ref: expectation.gte
                  args:
                    expected: 500
scenario_calls:
  - id: login-user
    scenario_id: login
`), nil)
	if err != nil {
		t.Fatalf("parse yaml failed: %v", err)
	}

	expectation := spec.Scenarios[0].Acts[0].Expectations[0]
	if got, want := expectation.Assert.Ref, builtinexpectation.NotRef; got != want {
		t.Fatalf("assert ref mismatch: got %q want %q", got, want)
	}

	assertArg, ok := expectation.Assert.Args["assert"]
	if !ok {
		t.Fatal(`assert arg "assert" must be present`)
	}

	value, ok := assertArg.Value.(map[string]any)
	if !ok {
		t.Fatalf("assert arg value type mismatch: got %T want map[string]any", assertArg.Value)
	}

	if got, want := value["ref"], builtinexpectation.GTERef; got != want {
		t.Fatalf("nested assert ref mismatch: got %#v want %#v", got, want)
	}

	args, ok := value["args"].(map[string]any)
	if !ok {
		t.Fatalf("nested assert args type mismatch: got %T want map[string]any", value["args"])
	}

	if got, want := args["expected"], 500; got != want {
		t.Fatalf("nested assert expected mismatch: got %#v want %#v", got, want)
	}
}

func TestParseRejectsMatcherSugarWithoutResolver(t *testing.T) {
	t.Parallel()

	_, err := theateryaml.Parse([]byte(`
id: main
scenarios:
  - id: login
    acts:
      - id: submit
        action:
          use: action.login
        expectations:
          - id: token
            subject: token
            assert:
              eq: issued-token
scenario_calls:
  - id: login-user
    scenario_id: login
`), nil)
	if err == nil {
		t.Fatal("expected matcher sugar resolver error, got nil")
	}

	errtest.RequireContains(t, err, yamlMissingSugarResolverFragment)
}

func TestLoadFlowFileAssemblesRepoLocalStage(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowRepo(t)
	flowPath := writeRepoFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "login-smoke.yaml"), `
id: login-smoke
scenarios: []
scenario_calls:
  - id: smoke-login
    scenario_id: auth/login
`)
	writeRepoFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.yaml"), `
id: auth-lib
scenarios:
  - id: auth/login
    acts:
      - id: submit
        action:
          use: action.http
scenario_calls: []
`)
	writeRepoFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "internal", "ignored.yaml"), `
id: ignored-lib
scenarios:
  - id: auth/internal/ignored
    acts:
      - id: submit
        action:
          use: action.http
scenario_calls: []
`)

	spec, err := theateryaml.LoadFlowFile(flowPath, matcherCatalog(t))
	if err != nil {
		t.Fatalf("load flow file failed: %v", err)
	}

	if got, want := spec.ID, "login-smoke"; got != want {
		t.Fatalf("stage id mismatch: got %q want %q", got, want)
	}
	if got, want := len(spec.Scenarios), 1; got != want {
		t.Fatalf("scenario count mismatch: got %d want %d", got, want)
	}
	if got, want := spec.Scenarios[0].ID, "auth/login"; got != want {
		t.Fatalf("scenario id mismatch: got %q want %q", got, want)
	}
	if got, want := spec.Scenarios[0].SourceSpan.File, filepath.Join(repoRoot, "theater", "lib", "auth", "login.yaml"); got != want {
		t.Fatalf("scenario source mismatch: got %q want %q", got, want)
	}
	if got, want := spec.SourceSpan.File, flowPath; got != want {
		t.Fatalf("flow source mismatch: got %q want %q", got, want)
	}
}

func TestLoadFileReadsNoSugarStageWithoutMatcherResolver(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "stage.yaml")
	if err := os.WriteFile(path, []byte(`
id: main
scenarios:
  - id: login
    acts:
      - id: submit
        action:
          use: action.login
scenario_calls:
  - id: login-user
    scenario_id: login
`), 0o600); err != nil {
		t.Fatalf("write yaml failed: %v", err)
	}

	spec, err := theateryaml.LoadFile(path, nil)
	if err != nil {
		t.Fatalf("load file failed: %v", err)
	}

	if got, want := spec.ScenarioCalls[0].ScenarioID, "login"; got != want {
		t.Fatalf("scenario call scenario id mismatch: got %q want %q", got, want)
	}
}

func TestLoadFlowFileAssemblesNoSugarStageWithoutMatcherResolver(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowRepo(t)
	flowPath := writeRepoFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "login-smoke.yaml"), `
id: login-smoke
scenarios: []
scenario_calls:
  - id: smoke-login
    scenario_id: auth/login
`)
	writeRepoFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.yaml"), `
id: auth-lib
scenarios:
  - id: auth/login
    acts:
      - id: submit
        action:
          use: action.http
scenario_calls: []
`)

	spec, err := theateryaml.LoadFlowFile(flowPath, nil)
	if err != nil {
		t.Fatalf("load flow file failed: %v", err)
	}

	if got, want := spec.Scenarios[0].ID, "auth/login"; got != want {
		t.Fatalf("scenario id mismatch: got %q want %q", got, want)
	}
}

func TestLoadFlowFileRejectsFileOutsideTheaterFlows(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowRepo(t)
	path := writeRepoFile(t, repoRoot, "stage.yaml", `
id: main
scenarios: []
scenario_calls: []
`)

	_, err := theateryaml.LoadFlowFile(path, matcherCatalog(t))
	if err == nil {
		t.Fatal("expected flow location error, got nil")
	}

	errtest.RequireContains(t, err, yamlFlowOutsideRootFragment)
}

func TestLoadFlowFileRejectsLibraryScenarioCalls(t *testing.T) {
	t.Parallel()

	repoRoot := createFlowRepo(t)
	flowPath := writeRepoFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "login-smoke.yaml"), `
id: login-smoke
scenarios: []
scenario_calls:
  - id: smoke-login
    scenario_id: auth/login
`)
	writeRepoFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.yaml"), `
id: auth-lib
scenarios:
  - id: auth/login
    acts:
      - id: submit
        action:
          use: action.http
scenario_calls:
  - id: invalid
    scenario_id: auth/login
`)

	_, err := theateryaml.LoadFlowFile(flowPath, matcherCatalog(t))
	if err == nil {
		t.Fatal("expected library scenario_calls error, got nil")
	}

	errtest.RequireContains(t, err, yamlLibraryScenarioCallsFragment)
}

func TestLoadFlowFileRejectsMissingRepoRoots(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "login-smoke.yaml")
	if err := os.WriteFile(path, []byte(`
id: login-smoke
scenarios: []
scenario_calls: []
`), 0o600); err != nil {
		t.Fatalf("write yaml failed: %v", err)
	}

	_, err := theateryaml.LoadFlowFile(path, matcherCatalog(t))
	if err == nil {
		t.Fatal("expected repo root error, got nil")
	}

	errtest.RequireContains(t, err, yamlMissingRepoRootsFragment)
}

func TestParseRejectsLegacyMaxAttemptsField(t *testing.T) {
	t.Parallel()

	_, err := theateryaml.Parse([]byte(`
id: main
scenarios:
  - id: login
    acts:
      - id: submit
        max_attempts: 2
        action:
          use: action.login
scenario_calls:
  - id: login-user
    scenario_id: login
`), matcherCatalog(t))
	if err == nil {
		t.Fatal("expected strict yaml error, got nil")
	}

	errtest.RequireContains(t, err, yamlLegacyMaxAttemptsFragment)
}

func TestParseLowersExpectationSugarToCanonicalShape(t *testing.T) {
	t.Parallel()

	spec, err := theateryaml.Parse([]byte(`
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        action:
          use: action.http
        expectations:
          - id: exact
            subject: status_code
            assert:
              eq:
                kind: ref
                ref: expected_status_code
scenario_calls:
  - id: probe-server
    scenario_id: probe
`), matcherCatalog(t))
	if err != nil {
		t.Fatalf("parse yaml failed: %v", err)
	}

	expectation := spec.Scenarios[0].Acts[0].Expectations[0]
	if got, want := expectation.Subject.Field, "status_code"; got != want {
		t.Fatalf("subject field mismatch: got %q want %q", got, want)
	}

	if got, want := expectation.Assert.Ref, "expectation.equal"; got != want {
		t.Fatalf("assert ref mismatch: got %q want %q", got, want)
	}

	if expectation.Assert.Args["expected"].Ref == nil {
		t.Fatal("assert arg ref is nil")
	}

	if got, want := expectation.Assert.Args["expected"].Ref.Name, "expected_status_code"; got != want {
		t.Fatalf("assert arg ref mismatch: got %q want %q", got, want)
	}
}

func TestParseLowersReferenceAndSelectorObjectsToCanonicalShape(t *testing.T) {
	t.Parallel()

	spec, err := theateryaml.Parse([]byte(`
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        action:
          use: action.http
        expectations:
          - id: token
            subject:
              field: body
              decode: json
              path: /token/id
            assert:
              ref: expectation.equal
              args:
                expected:
                  kind: ref
                  ref:
                    name: issued_payload
                    decode: json
                    path: /token/id
scenario_calls:
  - id: probe-server
    scenario_id: probe
    exports:
      - as: final_token
        ref:
          name: issued_payload
          decode: json
          path: /token/id
`), matcherCatalog(t))
	if err != nil {
		t.Fatalf("parse yaml failed: %v", err)
	}

	expectation := spec.Scenarios[0].Acts[0].Expectations[0]
	if got, want := expectation.Subject.Path, theater.JSONPointer("/token/id"); got != want {
		t.Fatalf("subject path mismatch: got %q want %q", got, want)
	}

	if got, want := expectation.Subject.Decode, theater.DecodeJSON; got != want {
		t.Fatalf("subject decode mismatch: got %q want %q", got, want)
	}

	expectedRef := expectation.Assert.Args["expected"].Ref
	if expectedRef == nil {
		t.Fatal("expected matcher ref must be present")
	}

	if got, want := expectedRef.Path, theater.JSONPointer("/token/id"); got != want {
		t.Fatalf("matcher ref path mismatch: got %q want %q", got, want)
	}

	exportRef := spec.ScenarioCalls[0].Exports[0].Ref
	if exportRef == nil {
		t.Fatal("export ref must be present")
	}

	if got, want := exportRef.Decode, theater.DecodeJSON; got != want {
		t.Fatalf("export ref decode mismatch: got %q want %q", got, want)
	}
}

func matcherCatalog(t *testing.T) *theater.MatcherCatalog {
	t.Helper()

	catalog, err := theater.NewMatcherCatalog(builtinexpectation.Descriptors()...)
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	return catalog
}

func createFlowRepo(t *testing.T) string {
	t.Helper()

	repoRoot := t.TempDir()
	for _, dir := range []string{
		filepath.Join(repoRoot, "theater", "lib"),
		filepath.Join(repoRoot, "theater", "flows"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s failed: %v", dir, err)
		}
	}

	return repoRoot
}

func writeRepoFile(t *testing.T, repoRoot string, relativePath string, body string) string {
	t.Helper()

	path := filepath.Join(repoRoot, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s failed: %v", filepath.Dir(path), err)
	}

	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s failed: %v", path, err)
	}

	return path
}
