package theatercli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplicationDoctorReportsReadyForRepoLayoutAndTTY(t *testing.T) {
	t.Parallel()

	repoRoot := createCLIFlowRepo(t)
	restore := chdirForTest(t, repoRoot)
	defer restore()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	app.isInputTerminal = func(io.Reader) bool { return true }
	app.isTerminal = func(io.Writer) bool { return true }

	if code := app.Run([]string{commandDoctor}); code != 0 {
		t.Fatalf("doctor exit code: %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	output := stdout.String()
	for _, want := range []string{
		"theater doctor: ready",
		"repo-aware flow layout",
		"interactive debug TTY: stdin and stderr are TTYs",
		"plugin registry file and lock file pairing: skipped because no plugin registry file or lock file was provided",
		"writable output destinations: skipped because no --write-path values were provided",
		"Run theater plugins doctor when you need checksum-drift diagnostics for the plugin registry.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor output missing %q: %s", want, output)
		}
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
}

func TestApplicationDoctorReportsRepoLayoutFailure(t *testing.T) {
	t.Parallel()

	restore := chdirForTest(t, t.TempDir())
	defer restore()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	app.isInputTerminal = func(io.Reader) bool { return true }
	app.isTerminal = func(io.Writer) bool { return true }

	if code := app.Run([]string{commandDoctor}); code != 1 {
		t.Fatalf("doctor exit code mismatch: got %d want 1 stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	output := stdout.String()
	for _, want := range []string{
		"theater doctor: not ready",
		"FAIL  repo-aware flow layout:",
		"Fix the failing checks and rerun theater doctor before validate or run.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor output missing %q: %s", want, output)
		}
	}
}

func TestApplicationDoctorReportsFatalAndAdvisoryChecksTogether(t *testing.T) {
	t.Parallel()

	restore := chdirForTest(t, t.TempDir())
	defer restore()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	app.isInputTerminal = func(io.Reader) bool { return false }
	app.isTerminal = func(io.Writer) bool { return false }

	if code := app.Run([]string{commandDoctor}); code != 1 {
		t.Fatalf("doctor exit code mismatch: got %d want 1 stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	output := stdout.String()
	for _, want := range []string{
		"theater doctor: not ready",
		"FAIL  repo-aware flow layout:",
		"WARN  interactive debug TTY: stdin_tty=false stderr_tty=false",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor output missing %q: %s", want, output)
		}
	}
}

func TestApplicationDoctorReportsInteractiveDebugTTYWarning(t *testing.T) {
	t.Parallel()

	repoRoot := createCLIFlowRepo(t)
	restore := chdirForTest(t, repoRoot)
	defer restore()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	app.isInputTerminal = func(io.Reader) bool { return false }
	app.isTerminal = func(io.Writer) bool { return false }

	if code := app.Run([]string{commandDoctor}); code != 0 {
		t.Fatalf("doctor exit code mismatch: got %d want 0 stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	output := stdout.String()
	for _, want := range []string{
		"theater doctor: ready",
		"WARN  interactive debug TTY: stdin_tty=false stderr_tty=false",
		"interactive debug requires a TTY",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor output missing %q: %s", want, output)
		}
	}
}

func TestApplicationDoctorReportsPluginPairAndReachability(t *testing.T) {
	t.Parallel()

	repoRoot := createCLIFlowRepo(t)
	restore := chdirForTest(t, repoRoot)
	defer restore()

	configPath, lockPath := writeSmokePluginFiles(t)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	app.isInputTerminal = func(io.Reader) bool { return true }
	app.isTerminal = func(io.Writer) bool { return true }

	if code := app.Run([]string{commandPlugins, commandPluginsLock, "--plugins-config", configPath, "--plugins-lock", lockPath}); code != 0 {
		t.Fatalf("plugins lock exit code: %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{commandDoctor, "--plugins-config", configPath, "--plugins-lock", lockPath}); code != 0 {
		t.Fatalf("doctor exit code mismatch: got %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	output := stdout.String()
	for _, want := range []string{
		"plugin registry file and lock file pairing:",
		"plugin executable reachability:",
		"1 plugin(s) reachable from",
		"Run theater plugins doctor when you need checksum-drift diagnostics for the plugin registry.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor output missing %q: %s", want, output)
		}
	}
}

func TestApplicationDoctorUsesPluginEnvironmentDefaults(t *testing.T) {
	repoRoot := createCLIFlowRepo(t)
	restore := chdirForTest(t, repoRoot)
	defer restore()

	configPath, lockPath := writeSmokePluginFiles(t)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	app.isInputTerminal = func(io.Reader) bool { return true }
	app.isTerminal = func(io.Writer) bool { return true }

	if code := app.Run([]string{commandPlugins, commandPluginsLock, "--plugins-config", configPath, "--plugins-lock", lockPath}); code != 0 {
		t.Fatalf("plugins lock exit code: %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	t.Setenv(envPluginsConfig, configPath)
	t.Setenv(envPluginsLock, lockPath)
	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{commandDoctor}); code != 0 {
		t.Fatalf("doctor exit code mismatch: got %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	output := stdout.String()
	for _, want := range []string{
		"plugin registry file and lock file pairing:",
		"plugin executable reachability:",
		lockPath,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor output missing %q: %s", want, output)
		}
	}
}

func TestApplicationDoctorFlagsOverridePluginEnvironment(t *testing.T) {
	repoRoot := createCLIFlowRepo(t)
	restore := chdirForTest(t, repoRoot)
	defer restore()

	configPath, lockPath := writeSmokePluginFiles(t)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	app.isInputTerminal = func(io.Reader) bool { return true }
	app.isTerminal = func(io.Writer) bool { return true }

	if code := app.Run([]string{commandPlugins, commandPluginsLock, "--plugins-config", configPath, "--plugins-lock", lockPath}); code != 0 {
		t.Fatalf("plugins lock exit code: %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	t.Setenv(envPluginsConfig, filepath.Join(t.TempDir(), "missing.plugins.json"))
	t.Setenv(envPluginsLock, filepath.Join(t.TempDir(), "missing.plugins.lock.json"))
	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{commandDoctor, "--plugins-config", configPath, "--plugins-lock", lockPath}); code != 0 {
		t.Fatalf("doctor exit code mismatch: got %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	output := stdout.String()
	if !strings.Contains(output, configPath) || !strings.Contains(output, lockPath) {
		t.Fatalf("doctor output must reflect explicit flag values: %s", output)
	}
}

func TestApplicationDoctorReportsPluginPairingFailure(t *testing.T) {
	t.Parallel()

	repoRoot := createCLIFlowRepo(t)
	restore := chdirForTest(t, repoRoot)
	defer restore()

	configPath, _ := writeSmokePluginFiles(t)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	app.isInputTerminal = func(io.Reader) bool { return true }
	app.isTerminal = func(io.Writer) bool { return true }

	if code := app.Run([]string{commandDoctor, "--plugins-config", configPath}); code != 1 {
		t.Fatalf("doctor exit code mismatch: got %d want 1 stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	output := stdout.String()
	for _, want := range []string{
		"FAIL  plugin registry file and lock file pairing: --plugins-config was provided without --plugins-lock",
		"OK  plugin executable reachability:",
		"Run theater plugins doctor for deeper plugin registry diagnostics once the basic pair is in place.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor output missing %q: %s", want, output)
		}
	}
}

func TestApplicationDoctorReportsPluginLockWithoutConfig(t *testing.T) {
	t.Parallel()

	repoRoot := createCLIFlowRepo(t)
	restore := chdirForTest(t, repoRoot)
	defer restore()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	app.isInputTerminal = func(io.Reader) bool { return true }
	app.isTerminal = func(io.Writer) bool { return true }

	lockPath := filepath.Join(t.TempDir(), "plugins.lock.json")
	if code := app.Run([]string{commandDoctor, "--plugins-lock", lockPath}); code != 1 {
		t.Fatalf("doctor exit code mismatch: got %d want 1 stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	output := stdout.String()
	for _, want := range []string{
		"FAIL  plugin registry file and lock file pairing: --plugins-lock was provided without --plugins-config",
		"OK  plugin executable reachability: skipped because --plugins-config was not provided",
		"Run theater plugins doctor for deeper plugin registry diagnostics once the basic pair is in place.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor output missing %q: %s", want, output)
		}
	}
}

func TestApplicationDoctorAcceptsWritePathWithMissingParentDirectory(t *testing.T) {
	t.Parallel()

	repoRoot := createCLIFlowRepo(t)
	restore := chdirForTest(t, repoRoot)
	defer restore()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	app.isInputTerminal = func(io.Reader) bool { return true }
	app.isTerminal = func(io.Writer) bool { return true }

	path := filepath.Join(t.TempDir(), "missing", "example-domain.debug.ndjson")
	if code := app.Run([]string{commandDoctor, "--write-path", path}); code != 0 {
		t.Fatalf("doctor exit code mismatch: got %d want 0 stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	output := stdout.String()
	for _, want := range []string{
		"theater doctor: ready",
		"OK  writable output destinations:",
		"example-domain.debug.ndjson",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor output missing %q: %s", want, output)
		}
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("doctor must create missing parent directory like debug dump output: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("doctor must not create final output file: %v", err)
	}
	assertDebugDumpCreatesMissingParentDirectory(t)
}

func TestApplicationDoctorReportsWritePathDirectoryFailure(t *testing.T) {
	t.Parallel()

	repoRoot := createCLIFlowRepo(t)
	restore := chdirForTest(t, repoRoot)
	defer restore()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	app.isInputTerminal = func(io.Reader) bool { return true }
	app.isTerminal = func(io.Writer) bool { return true }

	path := t.TempDir()
	if code := app.Run([]string{commandDoctor, "--write-path", path}); code != 1 {
		t.Fatalf("doctor exit code mismatch: got %d want 1 stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	output := stdout.String()
	for _, want := range []string{
		"theater doctor: not ready",
		"FAIL  writable output destinations:",
		"path is a directory",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor output missing %q: %s", want, output)
		}
	}
}

func TestApplicationDoctorReportsWritePathTrailingSeparatorFailure(t *testing.T) {
	t.Parallel()

	repoRoot := createCLIFlowRepo(t)
	restore := chdirForTest(t, repoRoot)
	defer restore()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	app.isInputTerminal = func(io.Reader) bool { return true }
	app.isTerminal = func(io.Writer) bool { return true }

	path := filepath.Join(t.TempDir(), "debug-output") + string(os.PathSeparator)
	if code := app.Run([]string{commandDoctor, "--write-path", path}); code != 1 {
		t.Fatalf("doctor exit code mismatch: got %d want 1 stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	output := stdout.String()
	for _, want := range []string{
		"FAIL  writable output destinations:",
		"path must name a file",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor output missing %q: %s", want, output)
		}
	}
}

func TestApplicationDoctorAcceptsRepeatedWritePaths(t *testing.T) {
	t.Parallel()

	repoRoot := createCLIFlowRepo(t)
	restore := chdirForTest(t, repoRoot)
	defer restore()

	tempDir := t.TempDir()
	pathA := filepath.Join(tempDir, "first.debug.ndjson")
	pathB := filepath.Join(tempDir, "second.debug.ndjson")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	app.isInputTerminal = func(io.Reader) bool { return true }
	app.isTerminal = func(io.Writer) bool { return true }

	if code := app.Run([]string{commandDoctor, "--write-path", pathA, "--write-path", pathB}); code != 0 {
		t.Fatalf("doctor exit code mismatch: got %d want 0 stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "OK  writable output destinations: 2 path(s) are writable") {
		t.Fatalf("doctor output missing repeated write-path success: %s", output)
	}
}

func assertDebugDumpCreatesMissingParentDirectory(t *testing.T) {
	t.Helper()

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        action:
          use: action.command
          with:
            executable:
              kind: literal
              value: echo
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)
	dumpPath := filepath.Join(t.TempDir(), "missing", "run.debug.ndjson")

	var stdout strings.Builder
	var stderr strings.Builder
	code := run([]string{
		commandRun,
		"--file", path,
		"--debug", "dump",
		"--break", "name=before-command,kind=action,phase=before,path=**",
		"--debug-dump", dumpPath,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run debug dump exit code mismatch: got %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if _, err := os.Stat(dumpPath); err != nil {
		t.Fatalf("debug dump must create missing parent and output file: %v", err)
	}
}
