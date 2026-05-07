package theatercli

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestRootHelpShowsDocumentedAliases(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"--help"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "validate, check") {
		t.Fatalf("root help missing validate alias: %q", stdout.String())
	}
}

func TestValidateAliasCheckExecutesValidate(t *testing.T) {
	t.Parallel()

	path := writeStageYAML(t, `
id: main
scenario_calls: []
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{"check", "-file", path}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q", got, want, stderr.String())
	}
	if got, want := strings.TrimSpace(stdout.String()), path+": valid"; got != want {
		t.Fatalf("stdout mismatch: got %q want %q", got, want)
	}
}

func TestPluginsAliasLsExecutesInspect(t *testing.T) {
	t.Parallel()

	configPath, lockPath := writeSmokePluginFiles(t)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)

	if code := app.Run([]string{"plugins", "lock", "-plugins-config", configPath, "-plugins-lock", lockPath}); code != 0 {
		t.Fatalf("plugins lock exit code: %d stderr=%s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"plugins", "ls", "-plugins-config", configPath, "-plugins-lock", lockPath}); code != 0 {
		t.Fatalf("plugins ls exit code: %d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "smoke-plugin 0.2.0") {
		t.Fatalf("plugins ls output missing plugin header: %s", stdout.String())
	}
}

func TestHelpCommandAcceptsAliases(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandHelp, "check"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Compile and validate a stage without live execution.") {
		t.Fatalf("stdout missing validate help: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Aliases:") || !strings.Contains(stdout.String(), "check") {
		t.Fatalf("stdout missing alias section: %q", stdout.String())
	}
}

func TestCompletionIncludesAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "root alias", args: []string{commandComplete, "ch"}, want: "check"},
		{name: "plugins alias", args: []string{commandComplete, commandPlugins, ""}, want: "ls"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var stdout strings.Builder
			var stderr strings.Builder

			code := run(test.args, &stdout, &stderr)
			if got, want := code, 0; got != want {
				t.Fatalf("exit code mismatch: got %d want %d", got, want)
			}
			if strings.TrimSpace(stderr.String()) != "" {
				t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
			}

			lines := strings.Fields(stdout.String())
			if !slices.Contains(lines, test.want) {
				t.Fatalf("completion output missing %q: %q", test.want, stdout.String())
			}
		})
	}
}
