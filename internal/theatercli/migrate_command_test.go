package theatercli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	goyaml "gopkg.in/yaml.v3"

	"github.com/alex-poliushkin/theater"
	authoringthtr "github.com/alex-poliushkin/theater/internal/authoring/thtr"
	authoringyaml "github.com/alex-poliushkin/theater/internal/authoring/yaml"
)

func TestRunMigrateFromYAMLWritesTHTRWithSafeExpectationSugar(t *testing.T) {
	t.Parallel()

	path := writeStageYAML(t, `
id: smoke
scenarios:
  - id: http/check
    acts:
      - id: fetch
        action:
          use: action.http
          with:
            method:
              kind: literal
              value: GET
            url:
              kind: literal
              value: https://example.com
        expectations:
          - id: status-ok
            subject:
              field: status_code
            assert:
              ref: expectation.equal
              args:
                expected:
                  kind: literal
                  value: 200
          - id: page-text
            subject:
              field: body
            assert:
              ref: expectation.contains
              args:
                expected:
                  kind: literal
                  value: Example Domain
          - id: latency-high
            subject:
              field: duration_ms
            assert:
              ref: expectation.gt
              args:
                expected:
                  kind: literal
                  value: 100
          - id: retries-left
            subject:
              field: retry_count
            assert:
              ref: expectation.lte
              args:
                expected:
                  kind: literal
                  value: 5
          - id: has-token
            subject:
              field: body
              decode: json
              path: /data
            assert:
              ref: expectation.has_key
              args:
                key:
                  kind: literal
                  value: token
          - id: duration-window
            subject:
              field: duration_ms
            assert:
              ref: expectation.between
              args:
                min:
                  kind: literal
                  value: 200
                max:
                  kind: literal
                  value: 299
scenario_calls:
  - id: run
    scenario_id: http/check
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"migrate", "from-yaml", "--file", path}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	output := stdout.String()
	for _, want := range []string{
		`expect status-ok: field(status_code) == 200`,
		`expect page-text: field(body) contains "Example Domain"`,
		`expect latency-high: field(duration_ms) > 100`,
		`expect retries-left: field(retry_count) <= 5`,
		`expect has-token: field(body) | decode(json) | path("/data") has key("token")`,
		`expect duration-window: field(duration_ms) between 200 and 299`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("migrated output must include %q:\n%s", want, output)
		}
	}

	formatted, err := authoringthtr.Format([]byte(output))
	if err != nil {
		t.Fatalf("format migrated output failed: %v", err)
	}
	if string(formatted) != output {
		t.Fatalf("migrated output must already be formatter-clean:\n--- got ---\n%s\n--- fmt ---\n%s", output, string(formatted))
	}

	gotSpec, err := authoringthtr.Parse([]byte(output), nil)
	if err != nil {
		t.Fatalf("parse migrated output failed: %v", err)
	}
	wantSpec, err := authoringyaml.LoadFile(path, nil)
	if err != nil {
		t.Fatalf("load source yaml failed: %v", err)
	}
	requireSemanticStageYAMLEqual(t, gotSpec, wantSpec)
}

func TestRunMigrateFromYAMLFlowAssemblesReferencedLibraryScenarios(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	flowPath := filepath.Join(repo, "theater", "flows", "auth", "login-smoke.yaml")
	libraryPath := filepath.Join(repo, "theater", "lib", "auth", "login.yaml")

	if err := os.MkdirAll(filepath.Dir(flowPath), 0o755); err != nil {
		t.Fatalf("prepare flow dir failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(libraryPath), 0o755); err != nil {
		t.Fatalf("prepare library dir failed: %v", err)
	}

	if err := os.WriteFile(flowPath, []byte(`
id: login-smoke
scenario_calls:
  - id: run-login
    scenario_id: auth/login
`), 0o600); err != nil {
		t.Fatalf("write flow yaml failed: %v", err)
	}
	if err := os.WriteFile(libraryPath, []byte(`
id: auth-library
scenarios:
  - id: auth/login
    acts:
      - id: submit
        action:
          use: action.http
          with:
            method:
              kind: literal
              value: GET
            url:
              kind: literal
              value: https://example.com
        expectations:
          - id: status-ok
            subject:
              field: status_code
            assert:
              ref: expectation.equal
              args:
                expected:
                  kind: literal
                  value: 200
`), 0o600); err != nil {
		t.Fatalf("write library yaml failed: %v", err)
	}

	restore := chdirForTest(t, repo)
	defer restore()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"migrate", "from-yaml", "--file", flowPath}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	output := stdout.String()
	if !strings.Contains(output, "scenario auth/login") {
		t.Fatalf("migrated flow must inline referenced library scenario:\n%s", output)
	}
	if !strings.Contains(output, "call run-login = auth/login()") {
		t.Fatalf("migrated flow must preserve runnable call:\n%s", output)
	}

	migratedPath := filepath.Join(repo, "theater", "flows", "auth", "login-smoke.thtr")
	if err := os.WriteFile(migratedPath, []byte(output), 0o600); err != nil {
		t.Fatalf("write migrated flow failed: %v", err)
	}

	stdout.Reset()
	stderr.Reset()

	code = run([]string{"validate", "--file", migratedPath}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("validate exit code mismatch: got %d want %d, stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if got, want := strings.TrimSpace(stdout.String()), migratedPath+": valid"; got != want {
		t.Fatalf("validate stdout mismatch: got %q want %q", got, want)
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("validate stderr mismatch: got %q want empty", got)
	}
}

func TestRunMigrateFromYAMLFlowIgnoresSiblingStateErgonomicsTHTRFiles(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	flowPath := filepath.Join(repo, "theater", "flows", "auth", "login-smoke.yaml")
	libraryPath := filepath.Join(repo, "theater", "lib", "auth", "login.yaml")
	thtrPath := filepath.Join(repo, "theater", "lib", "state", "verify.thtr")

	for _, path := range []string{flowPath, libraryPath, thtrPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("prepare parent for %s failed: %v", path, err)
		}
	}

	if err := os.WriteFile(flowPath, []byte(`
id: login-smoke
scenario_calls:
  - id: run-login
    scenario_id: auth/login
`), 0o600); err != nil {
		t.Fatalf("write flow yaml failed: %v", err)
	}
	if err := os.WriteFile(libraryPath, []byte(`
id: auth-library
scenarios:
  - id: auth/login
    acts:
      - id: submit
        action:
          use: action.http
          with:
            method:
              kind: literal
              value: GET
            url:
              kind: literal
              value: https://example.com
`), 0o600); err != nil {
		t.Fatalf("write library yaml failed: %v", err)
	}
	if err := os.WriteFile(thtrPath, readStateErgonomicsFixture(t, "success-input.thtr"), 0o600); err != nil {
		t.Fatalf("write sibling thtr fixture failed: %v", err)
	}

	restore := chdirForTest(t, repo)
	defer restore()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"migrate", "from-yaml", "--file", flowPath}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	output := stdout.String()
	if !strings.Contains(output, "scenario auth/login") {
		t.Fatalf("migrated flow must still include referenced yaml library scenario:\n%s", output)
	}
	if strings.Contains(output, "scenario verify-state") {
		t.Fatalf("migrated yaml flow must ignore sibling thtr state ergonomics library files:\n%s", output)
	}
}

func TestRunMigrateFromYAMLLoadsPluginBackedYAML(t *testing.T) {
	t.Parallel()

	configPath, lockPath := writeSmokePluginFiles(t)

	var stdout strings.Builder
	var stderr strings.Builder

	if code := run([]string{"plugins", "lock", "--plugins-config", configPath, "--plugins-lock", lockPath}, &stdout, &stderr); code != 0 {
		t.Fatalf("plugins lock exit code mismatch: got %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}

	path := writeStageYAML(t, `
id: plugin-migrate
scenarios:
  - id: plugins/smoke
    acts:
      - id: echo
        properties:
          message:
            inventory:
              use: inventory.smoke.echo
              with:
                value:
                  kind: literal
                  value: hello
        action:
          use: action.smoke.echo
          with:
            value:
              kind: ref
              ref: message
        expectations:
          - id: echoed
            subject:
              field: echo
            assert:
              ref: expectation.equal
              args:
                expected:
                  kind: literal
                  value: hello
scenario_calls:
  - id: run
    scenario_id: plugins/smoke
`)

	stdout.Reset()
	stderr.Reset()

	code := run(
		[]string{
			"migrate", "from-yaml",
			"--file", path,
			"--plugins-config", configPath,
			"--plugins-lock", lockPath,
		},
		&stdout,
		&stderr,
	)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
	if !strings.Contains(stdout.String(), `do action.smoke.echo(value: $message)`) {
		t.Fatalf("migrated output must preserve plugin-backed action:\n%s", stdout.String())
	}
}

func TestRunMigrateFromYAMLRewritesRepeatedStateHandlesConservatively(t *testing.T) {
	t.Parallel()

	path := writeStageYAML(t, `
id: state-migrate
state:
  backends:
    local:
      use: state.backend.file
      with:
        root: /tmp/theater-state
scenarios:
  - id: state/demo
    acts:
      - id: read-shared
        properties:
          shared_record:
            inventory:
              use: inventory.state.record
              with:
                backend: local
                record: env/shared-meta
                min_guarantee: local-atomic
        action:
          use: action.state.read
          with:
            record:
              kind: ref
              ref: shared_record
        exports:
          - as: shared_record_version
            field: version
      - id: update-shared
        properties:
          shared_record:
            inventory:
              use: inventory.state.record
              with:
                backend: local
                record: env/shared-meta
                min_guarantee: local-atomic
        action:
          use: action.state.update
          with:
            record:
              kind: ref
              ref: shared_record
            expected_version:
              kind: ref
              ref: shared_record_version
            value:
              kind: object
              object:
                owner: tutorial-run
      - id: claim-shared
        properties:
          otp_pool:
            inventory:
              use: inventory.state.pool
              with:
                backend: local
                pool: otp-identities
                min_guarantee: local-atomic
        action:
          use: action.state.claim
          with:
            pool:
              kind: ref
              ref: otp_pool
            selector:
              kind: object
              object:
                fields:
                  kind: object
                  object:
                    purpose: registration
            lease:
              kind: object
              object:
                ttl: 5m
                on_expiry: reclaim
        exports:
          - as: otp_claim
            field: claim
      - id: renew-claim
        action:
          use: action.state.renew
          with:
            claim:
              kind: ref
              ref: otp_claim
            ttl: 10m
      - id: claim-shared-again
        properties:
          otp_pool:
            inventory:
              use: inventory.state.pool
              with:
                backend: local
                pool: otp-identities
                min_guarantee: local-atomic
        action:
          use: action.state.claim
          with:
            pool:
              kind: ref
              ref: otp_pool
            selector:
              kind: object
              object:
                fields:
                  kind: object
                  object:
                    purpose: registration
            lease:
              kind: object
              object:
                ttl: 5m
      - id: read-oneoff
        properties:
          temp_record:
            inventory:
              use: inventory.state.record
              with:
                backend: local
                record: env/temporary
        action:
          use: action.state.read
          with:
            record:
              kind: ref
              ref: temp_record
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"migrate", "from-yaml", "--file", path}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	output := stdout.String()
	for _, want := range []string{
		`record shared_record = state.record`,
		`pool otp_pool = state.pool`,
		`do state.read(record: shared_record)`,
		`do state.update`,
		`if_version: $shared_record_version`,
		`do state.claim`,
		`fields: object { purpose: "registration" }`,
		`lease: object { on_expiry: reclaim, ttl: 5m }`,
		`do action.state.renew(claim: $otp_claim, ttl: 10m)`,
		`prop temp_record = inventory.state.record(`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("migrated output must include %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{
		`prop shared_record = inventory.state.record`,
		`prop otp_pool = inventory.state.pool`,
	} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("migrated output must rewrite repeated state handle %q:\n%s", unwanted, output)
		}
	}
	if strings.Contains(output, `record temp_record = state.record`) {
		t.Fatalf("migrated output must not hoist one-off state handle:\n%s", output)
	}

	formatted, err := authoringthtr.Format([]byte(output))
	if err != nil {
		t.Fatalf("format migrated output failed: %v", err)
	}
	if string(formatted) != output {
		t.Fatalf("migrated output must already be formatter-clean:\n--- got ---\n%s\n--- fmt ---\n%s", output, string(formatted))
	}

	migratedPath := filepath.Join(t.TempDir(), "state-migrate.thtr")
	if err := os.WriteFile(migratedPath, []byte(output), 0o600); err != nil {
		t.Fatalf("write migrated output failed: %v", err)
	}

	stdout.Reset()
	stderr.Reset()

	code = run([]string{"validate", "--file", migratedPath}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("validate exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if got, want := strings.TrimSpace(stdout.String()), migratedPath+": valid"; got != want {
		t.Fatalf("validate stdout mismatch: got %q want %q", got, want)
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("validate stderr mismatch: got %q want empty", got)
	}
}

func TestRunMigrateFromYAMLReportsMissingReferencedLibraryScenario(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	flowPath := filepath.Join(repo, "theater", "flows", "auth", "login-smoke.yaml")
	libraryPath := filepath.Join(repo, "theater", "lib", "auth", "register.yaml")

	if err := os.MkdirAll(filepath.Dir(flowPath), 0o755); err != nil {
		t.Fatalf("prepare flow dir failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(libraryPath), 0o755); err != nil {
		t.Fatalf("prepare library dir failed: %v", err)
	}

	if err := os.WriteFile(flowPath, []byte(`
id: login-smoke
scenario_calls:
  - id: run-login
    scenario_id: auth/login
`), 0o600); err != nil {
		t.Fatalf("write flow yaml failed: %v", err)
	}
	if err := os.WriteFile(libraryPath, []byte(`
id: auth-library
scenarios:
  - id: auth/register
    acts:
      - id: submit
        action:
          use: action.http
          with:
            method:
              kind: literal
              value: GET
            url:
              kind: literal
              value: https://example.com
`), 0o600); err != nil {
		t.Fatalf("write library yaml failed: %v", err)
	}

	restore := chdirForTest(t, repo)
	defer restore()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"migrate", "from-yaml", "--file", flowPath}, &stdout, &stderr)
	if got, want := code, exitCodeCommandError; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if !strings.Contains(stderr.String(), `referenced library scenario "auth/login" is not found`) {
		t.Fatalf("stderr mismatch: got %q want missing-library error", stderr.String())
	}
}

func TestRunMigrateCommandRejectsInvalidArguments(t *testing.T) {
	t.Parallel()

	validPath := writeStageYAML(t, "id: smoke\n")
	tests := []struct {
		name       string
		args       []string
		wantErrSub string
	}{
		{
			name:       "missing subcommand",
			args:       []string{"migrate"},
			wantErrSub: "migrate requires a subcommand",
		},
		{
			name:       "unknown subcommand",
			args:       []string{"migrate", "unknown"},
			wantErrSub: `unknown migrate subcommand "unknown"`,
		},
		{
			name:       "missing file",
			args:       []string{"migrate", "from-yaml"},
			wantErrSub: "migrate from-yaml requires --file",
		},
		{
			name:       "invalid extension",
			args:       []string{"migrate", "from-yaml", "--file", "stage.txt"},
			wantErrSub: "migrate from-yaml requires a .yaml or .yml file",
		},
		{
			name:       "extra positional argument",
			args:       []string{"migrate", "from-yaml", "--file", validPath, "extra"},
			wantErrSub: "migrate from-yaml does not accept positional arguments",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var stdout strings.Builder
			var stderr strings.Builder

			code := run(test.args, &stdout, &stderr)
			if got, want := code, exitCodeCommandError; got != want {
				t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
			}
			if !strings.Contains(stderr.String(), test.wantErrSub) {
				t.Fatalf("stderr mismatch: got %q want substring %q", stderr.String(), test.wantErrSub)
			}
		})
	}
}

func TestRunMigrateFromYAMLReportsLoadFailure(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	missingPath := filepath.Join(t.TempDir(), "missing.yaml")
	code := run([]string{"migrate", "from-yaml", "--file", missingPath}, &stdout, &stderr)
	if got, want := code, exitCodeCommandError; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if !strings.Contains(stderr.String(), "migrate from-yaml:") {
		t.Fatalf("stderr mismatch: got %q want load failure prefix", stderr.String())
	}
}

func TestRunMigrateFromYAMLReportsMarshalFailure(t *testing.T) {
	t.Parallel()

	path := writeStageYAML(t, `
id: smoke
scenarios:
  - id: auth/login
    acts:
      - id: submit
        action:
          use: "action.http\nexpect injected"
          with:
            method:
              kind: literal
              value: GET
            url:
              kind: literal
              value: https://example.com
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"migrate", "from-yaml", "--file", path}, &stdout, &stderr)
	if got, want := code, exitCodeCommandError; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if !strings.Contains(stderr.String(), `call name "action.http`) {
		t.Fatalf("stderr mismatch: got %q want migrate validation error", stderr.String())
	}
}

func TestRunMigrateFromYAMLReportsWriteFailure(t *testing.T) {
	t.Parallel()

	path := writeStageYAML(t, `
id: smoke
scenarios:
  - id: http/check
    acts:
      - id: fetch
        action:
          use: action.http
          with:
            method:
              kind: literal
              value: GET
            url:
              kind: literal
              value: https://example.com
`)

	var stderr strings.Builder
	code := run([]string{"migrate", "from-yaml", "--file", path}, failingWriter{err: errors.New("boom")}, &stderr)
	if got, want := code, exitCodeCommandError; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q", got, want, stderr.String())
	}
	if !strings.Contains(stderr.String(), "write migrated source: boom") {
		t.Fatalf("stderr mismatch: got %q want write failure", stderr.String())
	}
}

func marshalStageYAML(t *testing.T, spec theater.StageSpec) string {
	t.Helper()

	data, err := goyaml.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal canonical yaml failed: %v", err)
	}

	return string(data)
}

func normalizeStageYAML(t *testing.T, spec theater.StageSpec) any {
	t.Helper()

	data, err := goyaml.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal canonical yaml failed: %v", err)
	}

	var normalized any
	if err := goyaml.Unmarshal(data, &normalized); err != nil {
		t.Fatalf("unmarshal canonical yaml failed: %v", err)
	}

	return normalized
}

func requireSemanticStageYAMLEqual(t *testing.T, got, want theater.StageSpec) {
	t.Helper()

	gotValue := normalizeStageYAML(t, got)
	wantValue := normalizeStageYAML(t, want)
	if reflect.DeepEqual(gotValue, wantValue) {
		return
	}

	t.Fatalf(
		"round-trip canonical yaml mismatch:\n--- got ---\n%s\n--- want ---\n%s",
		marshalStageYAML(t, got),
		marshalStageYAML(t, want),
	)
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}
