package theatercli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateAcceptsDoubleDashLongOptions(t *testing.T) {
	t.Parallel()

	path := writeStageYAML(t, `
id: main
scenario_calls: []
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandValidate, "--file", path, "--format", "text"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q", got, want, stderr.String())
	}
	if got, want := strings.TrimSpace(stdout.String()), path+": valid"; got != want {
		t.Fatalf("stdout mismatch: got %q want %q", got, want)
	}
}

func TestValidateAcceptsLegacySingleDashLongOptionsWithoutWarnings(t *testing.T) {
	t.Parallel()

	path := writeStageYAML(t, `
id: main
scenario_calls: []
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandValidate, "-file", path, "-format", "text"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q", got, want, stderr.String())
	}
	if got, want := strings.TrimSpace(stdout.String()), path+": valid"; got != want {
		t.Fatalf("stdout mismatch: got %q want %q", got, want)
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}
}

func TestRunAcceptsDoubleDashLongOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("ok"))
	}))
	defer server.Close()

	t.Setenv("THEATER_STAGE_URL", server.URL)

	path := writeStageYAML(t, `
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
                name:
                  kind: literal
                  value: THEATER_STAGE_URL
        action:
          use: action.http
          with:
            url:
              kind: ref
              ref: url
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandRun, "--file", path, "--live", "off", "--format", "text"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q", got, want, stderr.String())
	}
	if !strings.Contains(stdout.String(), path+": passed") {
		t.Fatalf("stdout mismatch: %q", stdout.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}
}

func TestRunAcceptsLegacySingleDashLongOptionsWithoutWarnings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("ok"))
	}))
	defer server.Close()

	t.Setenv("THEATER_STAGE_URL", server.URL)

	path := writeStageYAML(t, `
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
                name:
                  kind: literal
                  value: THEATER_STAGE_URL
        action:
          use: action.http
          with:
            url:
              kind: ref
              ref: url
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandRun, "-file", path, "-live", "off", "-format", "text"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q", got, want, stderr.String())
	}
	if !strings.Contains(stdout.String(), path+": passed") {
		t.Fatalf("stdout mismatch: %q", stdout.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}
}

func TestPluginsInspectAcceptsDoubleDashLongOptions(t *testing.T) {
	t.Parallel()

	configPath, lockPath := writeSmokePluginFiles(t)
	var stdout strings.Builder
	var stderr strings.Builder
	app := newApplication(&stdout, &stderr)

	code := app.Run([]string{
		commandPlugins,
		commandPluginsLock,
		"--plugins-config", configPath,
		"--plugins-lock", lockPath,
	})
	if got, want := code, 0; got != want {
		t.Fatalf("plugins lock exit code mismatch: got %d want %d stderr=%q", got, want, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()

	code = app.Run([]string{
		commandPlugins,
		commandPluginsInspect,
		"--plugins-config", configPath,
		"--plugins-lock", lockPath,
		"--format", "json",
	})
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q", got, want, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"config_path":`) {
		t.Fatalf("stdout mismatch: %q", stdout.String())
	}
}

func TestPluginCommandsAcceptLegacySingleDashLongOptionsWithoutWarnings(t *testing.T) {
	t.Parallel()

	configPath, lockPath := writeSmokePluginFiles(t)
	var stdout strings.Builder
	var stderr strings.Builder
	app := newApplication(&stdout, &stderr)

	code := app.Run([]string{
		commandPlugins,
		commandPluginsLock,
		"-plugins-config", configPath,
		"-plugins-lock", lockPath,
	})
	if got, want := code, 0; got != want {
		t.Fatalf("plugins lock exit code mismatch: got %d want %d stderr=%q", got, want, stderr.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("plugins lock stderr mismatch: got %q want empty", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()

	code = app.Run([]string{
		commandPlugins,
		commandPluginsInspect,
		"-plugins-config", configPath,
		"-plugins-lock", lockPath,
		"-format", "json",
	})
	if got, want := code, 0; got != want {
		t.Fatalf("plugins inspect exit code mismatch: got %d want %d stderr=%q", got, want, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"config_path":`) {
		t.Fatalf("plugins inspect stdout mismatch: %q", stdout.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("plugins inspect stderr mismatch: got %q want empty", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()

	code = app.Run([]string{
		commandPlugins,
		commandPluginsDoctor,
		"-plugins-config", configPath,
		"-plugins-lock", lockPath,
	})
	if got, want := code, 0; got != want {
		t.Fatalf("plugins doctor exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "plugin registry: ready") {
		t.Fatalf("plugins doctor stdout mismatch: %q", stdout.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("plugins doctor stderr mismatch: got %q want empty", stderr.String())
	}
}

func TestPluginsDoctorAcceptsDoubleDashLongOptions(t *testing.T) {
	t.Parallel()

	configPath, lockPath := writeSmokePluginFiles(t)
	var stdout strings.Builder
	var stderr strings.Builder
	app := newApplication(&stdout, &stderr)

	code := app.Run([]string{
		commandPlugins,
		commandPluginsLock,
		"--plugins-config", configPath,
		"--plugins-lock", lockPath,
	})
	if got, want := code, 0; got != want {
		t.Fatalf("plugins lock exit code mismatch: got %d want %d stderr=%q", got, want, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()

	code = app.Run([]string{
		commandPlugins,
		commandPluginsDoctor,
		"--plugins-config", configPath,
		"--plugins-lock", lockPath,
	})
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "plugin registry: ready") {
		t.Fatalf("stdout mismatch: %q", stdout.String())
	}
}

func TestLongOptionErrorWordingUsesDoubleDash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "run missing file", args: []string{commandRun}, want: "run requires a stage file path via positional argument or --file"},
		{name: "doctor extra arg", args: []string{commandDoctor, "extra"}, want: "doctor does not accept positional arguments"},
		{name: "fmt missing file", args: []string{commandFmt}, want: "fmt requires a .thtr file path via positional argument or --file"},
		{name: "lower missing file", args: []string{commandLower}, want: "lower requires a .thtr file path via positional argument or --file"},
		{name: "plugins doctor extra arg", args: []string{commandPlugins, commandPluginsDoctor, "--plugins-config", "plugins.json", "extra"}, want: "plugins doctor does not accept positional arguments"},
		{name: "plugins digest missing manifest", args: []string{commandPlugins, commandPluginsDigest}, want: "plugins digest requires --manifest"},
		{name: "plugins doctor missing config", args: []string{commandPlugins, commandPluginsDoctor}, want: "plugins doctor requires --plugins-config"},
		{name: "plugins lock missing config", args: []string{commandPlugins, commandPluginsLock}, want: "plugins lock requires --plugins-config"},
		{name: "run missing plugins lock", args: []string{commandRun, "--file", "stage.yaml", "--plugins-config", "plugins.json"}, want: "run requires --plugins-lock when --plugins-config is set"},
		{name: "plugins lock missing lock", args: []string{commandPlugins, commandPluginsLock, "--plugins-config", "plugins.json"}, want: "plugins lock requires --plugins-lock"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var stdout strings.Builder
			var stderr strings.Builder

			code := run(test.args, &stdout, &stderr)
			if got, want := code, exitCodeCommandError; got != want {
				t.Fatalf("exit code mismatch: got %d want %d stderr=%q", got, want, stderr.String())
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("stderr missing %q: %q", test.want, stderr.String())
			}
		})
	}
}
