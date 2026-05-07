package theatercli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunLowerTHTRStateErgonomicsFixtureMatchesGolden(t *testing.T) {
	t.Parallel()

	source := readStateErgonomicsFixture(t, "success-input.thtr")
	want := readStateErgonomicsFixture(t, "success-lowered.yaml")
	path := writeStageFile(t, "stage.thtr", string(source))

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"lower", "-file", path}, &stdout, &stderr)
	if got, wantCode := code, 0; got != wantCode {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, wantCode, stderr.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
	if got, wantOutput := stdout.String(), string(want); got != wantOutput {
		t.Fatalf("lower output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, wantOutput)
	}
}

func TestRunFmtTHTRStateErgonomicsFixtureMatchesGolden(t *testing.T) {
	t.Parallel()

	source := readStateErgonomicsFixture(t, "success-input.thtr")
	want := readStateErgonomicsFixture(t, "success-formatted.thtr")
	path := writeStageFile(t, "stage.thtr", string(source))

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"fmt", "-file", path}, &stdout, &stderr)
	if got, wantCode := code, 0; got != wantCode {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, wantCode, stderr.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
	if got, wantOutput := stdout.String(), string(want); got != wantOutput {
		t.Fatalf("fmt output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, wantOutput)
	}
}

func TestRunLowerTHTRStateErgonomicsAliasUseFixtureShowsActionBreadcrumb(t *testing.T) {
	t.Parallel()

	source := readStateErgonomicsFixture(t, "invalid-alias-use.thtr")
	path := writeStageFile(t, "invalid.thtr", string(source))

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"lower", "-file", path}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	output := normalizeRunOutput(stdout.String(), map[string]string{
		path: "<stage-file>",
	})
	if !strings.Contains(output, "[thtr_lower_error]") {
		t.Fatalf("output must include lower diagnostic code: %q", output)
	}
	if !strings.Contains(output, "source: <stage-file>:12:13") {
		t.Fatalf("output must include action source span: %q", output)
	}
	if !strings.Contains(output, "breadcrumb: scenario login -> act claim -> action") {
		t.Fatalf("output must include action breadcrumb: %q", output)
	}
}

func TestRunValidateTHTRStateErgonomicsEventuallyFixtureShowsActBreadcrumb(t *testing.T) {
	t.Parallel()

	source := readStateErgonomicsFixture(t, "invalid-eventually-claim.thtr")
	path := writeStageFile(t, "invalid.thtr", string(source))

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"validate", "-file", path}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	output := normalizeRunOutput(stdout.String(), map[string]string{
		path: "<stage-file>",
	})
	if !strings.Contains(output, "[state_mutation_inside_eventually]") {
		t.Fatalf("output must include state mutation diagnostic code: %q", output)
	}
	if !strings.Contains(output, "source: <stage-file>:10:3") {
		t.Fatalf("output must include eventually act source span: %q", output)
	}
	if !strings.Contains(output, "breadcrumb: scenario verify-state -> act lifecycle") {
		t.Fatalf("output must include act breadcrumb: %q", output)
	}
}

func TestRunLowerTHTRRejectsRemovedStateSurface(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		source      string
		wantMessage string
		wantSource  string
	}{
		{
			name: "state cas",
			source: `stage main
scenario login
  act update
    do state.cas(expected_version: "1")
`,
			wantMessage: "state.cas has been removed; use state.update(... if_version: ...)",
			wantSource:  "source: <stage-file>:4:8",
		},
		{
			name: "state update expected version",
			source: `stage main
scenario login
  act update
    do state.update(expected_version: "1")
`,
			wantMessage: "state.update uses if_version; expected_version is the canonical action field",
			wantSource:  "source: <stage-file>:4:21",
		},
		{
			name: "state claim where",
			source: `stage main
scenario login
  act claim
    do state.claim
      where:
        purpose: "registration"
`,
			wantMessage: "state.claim where has been removed; use fields:",
			wantSource:  "source: <stage-file>:5:7",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := writeStageFile(t, "invalid.thtr", tc.source)
			var stdout strings.Builder
			var stderr strings.Builder

			code := run([]string{"lower", "-file", path}, &stdout, &stderr)
			if got, want := code, 1; got != want {
				t.Fatalf("exit code mismatch: got %d want %d, stderr=%q", got, want, stderr.String())
			}
			if got := strings.TrimSpace(stderr.String()); got != "" {
				t.Fatalf("stderr mismatch: got %q want empty", got)
			}

			output := normalizeRunOutput(stdout.String(), map[string]string{
				path: "<stage-file>",
			})
			if !strings.Contains(output, "[thtr_lower_error]") {
				t.Fatalf("output must include lower diagnostic code: %q", output)
			}
			if !strings.Contains(output, tc.wantMessage) {
				t.Fatalf("output must include diagnostic message %q:\n%s", tc.wantMessage, output)
			}
			if !strings.Contains(output, tc.wantSource) {
				t.Fatalf("output must include source %q:\n%s", tc.wantSource, output)
			}
			if !strings.Contains(output, "breadcrumb: scenario login -> act") {
				t.Fatalf("output must include act breadcrumb: %q", output)
			}
		})
	}
}

func TestRunTHTRStateErgonomicsFlowPasses(t *testing.T) {
	t.Parallel()

	runtimeRoot := seedFileBackendRuntime(t)
	path := writeStageFile(t, "stage.thtr", fmt.Sprintf(`stage smoke

state
  backend local = state.backend.file(root: %q)
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

scenario exp/state-smoke
  act read-meta
    do state.read(record: shared_meta)
    export meta_version = field(version)

  act update-meta
    do state.update(
      record: shared_meta,
      if_version: $meta_version,
      value: object { owner: "tse014-run", status: "reserved" }
    )

  act claim-item
    do state.claim
      pool: otp_identities
      id: "mailbox-release"
      lease:
        ttl: 5m
        on_expiry: reclaim
    export otp_claim = field(claim)

  act release-item
    do state.release(claim: $otp_claim)

call run-state = exp/state-smoke()
`, runtimeRoot))

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"run", "-file", path, "--live", "off"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
	if !strings.Contains(stdout.String(), path+": passed") {
		t.Fatalf("run output must report passed stage: %q", stdout.String())
	}
}

func TestRunTHTRFileBackendLifecycleExamplePasses(t *testing.T) {
	t.Parallel()

	runtimeRoot := seedFileBackendRuntime(t)
	root := repoRoot(t)
	sourcePath := filepath.Join(root, "testdata", "workflows", "state", "file-backend-lifecycle.thtr")

	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read file backend example failed: %v", err)
	}

	stage := strings.ReplaceAll(
		string(source),
		`root: "__THEATER_TEST_FILE_STATE_ROOT__"`,
		fmt.Sprintf(`root: %q`, runtimeRoot),
	)
	path := writeStageFile(t, "file-backend-lifecycle.thtr", stage)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"run", "-file", path, "--live", "off"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d, stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
	if !strings.Contains(stdout.String(), path+": passed") {
		t.Fatalf("run output must report passed stage: %q", stdout.String())
	}
}

func readStateErgonomicsFixture(t *testing.T, name string) []byte {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve fixture path failed")
	}

	path := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "thtr-state-ergonomics", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s failed: %v", name, err)
	}

	return data
}

func seedFileBackendRuntime(t *testing.T) string {
	t.Helper()

	runtimeRoot := filepath.Join(t.TempDir(), "runtime")
	seedWorkflowFileState(t, runtimeRoot)
	return runtimeRoot
}
