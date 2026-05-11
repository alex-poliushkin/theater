package theatercli

import (
	"strings"
	"testing"

	theater "github.com/alex-poliushkin/theater"
)

func TestExplainCommandOverviewListsFamiliesAndTopics(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandExplain}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}

	for _, want := range []string{
		"Explain discoverable runtime capabilities and CLI topics.",
		"Capability families:",
		"action",
		"inventory",
		"generator",
		"transform",
		"matcher",
		"report-exporter",
		"state-backend",
		"Topics:",
		"formats",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing overview snippet %q: %q", want, stdout.String())
		}
	}
}

func TestExplainCommandListsCapabilityFamily(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandExplain, "action"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}

	for _, want := range []string{
		"Family: action",
		"Capabilities (",
		"action.command",
		"action.generate",
		"action.http",
		"builtin",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing family snippet %q: %q", want, stdout.String())
		}
	}
}

func TestExplainCommandShowsCapabilityDetail(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandExplain, "action", "http"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}

	for _, want := range []string{
		"Capability: action.http",
		"Family:",
		"Provider:",
		"builtin",
		"Inputs:",
		"url",
		"required",
		"Outputs:",
		"status_code",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing capability snippet %q: %q", want, stdout.String())
		}
	}
}

func TestExplainCommandShowsDetailAcrossFamilies(t *testing.T) {
	t.Parallel()

	configPath, _ := writeSmokePluginFiles(t)

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "inventory",
			args: []string{commandExplain, "inventory", "http.get"},
			want: []string{"Capability: inventory.http.get", "Args:", "url", "Produces:", "bytes"},
		},
		{
			name: "action full ref",
			args: []string{commandExplain, "action", "action.http"},
			want: []string{"Capability: action.http", "Inputs:", "url", "Outputs:", "status_code"},
		},
		{
			name: "transform",
			args: []string{commandExplain, "transform", "json.decode"},
			want: []string{"Capability: json.decode", "Accepts:", "string|bytes", "Produces:", "object|list|null"},
		},
		{
			name: "matcher",
			args: []string{commandExplain, "matcher", "equal"},
			want: []string{"Capability: expectation.equal", "Args:", "expected", "Actual:", "any", "Sugar:", "eq"},
		},
		{
			name: "generator",
			args: []string{commandExplain, "generator", "email"},
			want: []string{"Capability: email", "Args:", "domain", "required", "Produces:", "string"},
		},
		{
			name: "state backend",
			args: []string{commandExplain, "state-backend", "file"},
			want: []string{"Capability: state.backend.file", "Params:", "root", "State behavior:", "guarantee: local-atomic"},
		},
		{
			name: "plugin report exporter",
			args: []string{commandExplain, "report-exporter", "smoke.write", "--plugins-config", configPath},
			want: []string{"Capability: report_exporter.smoke.write", "Provider:", "plugin smoke-plugin@0.2.0", "Params:", "path"},
		},
		{
			name: "plugin report exporter full ref",
			args: []string{commandExplain, "report-exporter", "report_exporter.smoke.write", "--plugins-config", configPath},
			want: []string{"Capability: report_exporter.smoke.write", "Provider:", "plugin smoke-plugin@0.2.0", "Params:", "path"},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var stdout strings.Builder
			var stderr strings.Builder

			code := run(test.args, &stdout, &stderr)
			if got, want := code, 0; got != want {
				t.Fatalf("exit code mismatch: got %d want %d stderr=%s", got, want, stderr.String())
			}
			if strings.TrimSpace(stderr.String()) != "" {
				t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
			}

			for _, want := range test.want {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("stdout missing detail snippet %q: %q", want, stdout.String())
				}
			}
		})
	}
}

func TestExplainFormatsUnionValueContracts(t *testing.T) {
	t.Parallel()

	got := formatValueContract(theater.ValueContract{
		Kinds: theater.NewValueKindSet(theater.ValueKindObject, theater.ValueKindString),
	}, false)
	if want := "string|object"; got != want {
		t.Fatalf("union contract format mismatch: got %q want %q", got, want)
	}
}

func TestExplainCommandSearchesUnscopedGuess(t *testing.T) {
	t.Parallel()

	configPath, _ := writeSmokePluginFiles(t)

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "shared http fragment",
			args: []string{commandExplain, "http"},
			want: []string{
				`Matches for "http":`,
				"action",
				"action.http",
				"theater explain action http",
				"inventory",
				"inventory.http.get",
				"theater explain inventory http.get",
			},
		},
		{
			name: "generator ref",
			args: []string{commandExplain, "email"},
			want: []string{
				`Matches for "email":`,
				"generator",
				"email",
				"theater explain generator email",
			},
		},
		{
			name: "plugin ref",
			args: []string{commandExplain, "smoke.write", "--plugins-config", configPath},
			want: []string{
				`Matches for "smoke.write":`,
				"report-exporter",
				"report_exporter.smoke.write",
				"theater explain report-exporter smoke.write",
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var stdout strings.Builder
			var stderr strings.Builder

			code := run(test.args, &stdout, &stderr)
			if got, want := code, 0; got != want {
				t.Fatalf("exit code mismatch: got %d want %d stderr=%s", got, want, stderr.String())
			}
			if strings.TrimSpace(stderr.String()) != "" {
				t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
			}
			if strings.Contains(stdout.String(), "Capability:") {
				t.Fatalf("unscoped guess must list matches instead of opening a detail page: %q", stdout.String())
			}
			for _, want := range test.want {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("stdout missing match snippet %q: %q", want, stdout.String())
				}
			}
		})
	}
}

func TestExplainCommandSearchesUnscopedHTTPWithExactRows(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandExplain, "http"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%s", got, want, stderr.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}

	want := strings.TrimSpace(`Matches for "http":

Capabilities:
  action     action.http         theater explain action http
  inventory  inventory.http.get  theater explain inventory http.get

Use ` + "`" + `theater explain <family> <ref>` + "`" + ` to inspect one capability contract.`)
	if got := strings.TrimSpace(stdout.String()); got != want {
		t.Fatalf("stdout mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestExplainCommandShowsTopicDetail(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandExplain, "formats"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}

	for _, want := range []string{
		"Topic: formats",
		"Explain text, json, and junit output surfaces.",
		"Output formats:",
		"text  default human-readable output",
		"Stdout and stderr:",
		"theater help exit-codes",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing topic snippet %q: %q", want, stdout.String())
		}
	}
}

func TestExplainCommandIncludesPluginProvenance(t *testing.T) {
	t.Parallel()

	configPath, _ := writeSmokePluginFiles(t)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandExplain, "action", "smoke.echo", "--plugins-config", configPath}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%s", got, want, stderr.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}

	for _, want := range []string{
		"Capability: action.smoke.echo",
		"Provider:",
		"plugin smoke-plugin@0.2.0",
		"Emit a simple echo output",
		"echo",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing plugin snippet %q: %q", want, stdout.String())
		}
	}
}

func TestExplainCommandRejectsInvalidTargetsAndFlagCombinations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "too many targets",
			args: []string{commandExplain, "action", "inventory", "extra"},
			want: "explain accepts at most two targets",
		},
		{
			name: "plugins lock without config",
			args: []string{commandExplain, "--plugins-lock", "plugins.lock.json"},
			want: "explain requires --plugins-config when --plugins-lock is set",
		},
		{
			name: "unknown target",
			args: []string{commandExplain, "unknown.capability"},
			want: `unknown explain target "unknown.capability"`,
		},
		{
			name: "unknown family target",
			args: []string{commandExplain, "unknown", "email"},
			want: `unknown explain family "unknown"`,
		},
		{
			name: "unknown family capability",
			args: []string{commandExplain, "generator", "missing"},
			want: `unknown generator capability "missing"`,
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
				t.Fatalf("exit code mismatch: got %d want %d", got, want)
			}
			if strings.TrimSpace(stdout.String()) != "" {
				t.Fatalf("stdout mismatch: got %q want empty", stdout.String())
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("stderr missing failure snippet %q: %q", test.want, stderr.String())
			}
		})
	}
}

func TestExplainCommandSupportsDoubleDashTarget(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandExplain, "--", "formats"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Topic: formats") {
		t.Fatalf("stdout missing double-dash target handling: %q", stdout.String())
	}
}

func TestExplainTopicDoesNotLoadPluginOverlay(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run(
		[]string{commandExplain, "formats", "--plugins-config", "missing.plugins.json"},
		&stdout,
		&stderr,
	)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%s", got, want, stderr.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Topic: formats") {
		t.Fatalf("stdout missing built-in topic output: %q", stdout.String())
	}
}

func TestExplainCommandSupportsFamilyAndTopicAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "family alias", args: []string{commandExplain, "actions"}, want: "Family: action"},
		{name: "topic alias", args: []string{commandExplain, "output-format"}, want: "Topic: formats"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var stdout strings.Builder
			var stderr strings.Builder

			code := run(test.args, &stdout, &stderr)
			if got, want := code, 0; got != want {
				t.Fatalf("exit code mismatch: got %d want %d stderr=%s", got, want, stderr.String())
			}
			if strings.TrimSpace(stderr.String()) != "" {
				t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
			}
			if !strings.Contains(stdout.String(), test.want) {
				t.Fatalf("stdout missing alias resolution %q: %q", test.want, stdout.String())
			}
		})
	}
}
