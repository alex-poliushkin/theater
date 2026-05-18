package theatercli

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alex-poliushkin/theater"
)

func TestRootHelpFlagPrintsHelpAndExitsZero(t *testing.T) {
	t.Parallel()

	for _, helpFlag := range []string{"-h", "--help"} {
		helpFlag := helpFlag
		t.Run(helpFlag, func(t *testing.T) {
			t.Parallel()

			var stdout strings.Builder
			var stderr strings.Builder

			code := run([]string{helpFlag}, &stdout, &stderr)
			if got, want := code, 0; got != want {
				t.Fatalf("exit code mismatch: got %d want %d", got, want)
			}
			if strings.TrimSpace(stderr.String()) != "" {
				t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
			}
			if !strings.Contains(stdout.String(), "Validate-first CLI for reusable verification flows.") {
				t.Fatalf("root help missing description: %q", stdout.String())
			}
		})
	}
}

func TestRootHelpFlagRejectsExtraArgs(t *testing.T) {
	t.Parallel()

	for _, helpFlag := range []string{"-h", "--help"} {
		helpFlag := helpFlag
		t.Run(helpFlag, func(t *testing.T) {
			t.Parallel()

			var stdout strings.Builder
			var stderr strings.Builder

			code := run([]string{helpFlag, commandRun}, &stdout, &stderr)
			if got, want := code, exitCodeCommandError; got != want {
				t.Fatalf("exit code mismatch: got %d want %d", got, want)
			}
			if strings.TrimSpace(stdout.String()) != "" {
				t.Fatalf("stdout mismatch: got %q want empty", stdout.String())
			}
			if !strings.Contains(stderr.String(), `use "theater help <command>"`) {
				t.Fatalf("stderr missing guidance: %q", stderr.String())
			}
		})
	}
}

func TestHelpCommandPrintsCommandHelpToStdout(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandHelp, commandRun}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Validate, execute, and render a stage run.") {
		t.Fatalf("stdout missing command help: %q", stdout.String())
	}
	for _, want := range []string{
		"Options:",
		"Output behavior:",
		"Related:",
		"--debug-dump <string>",
		"theater help exit-codes",
		"theater help debug-selectors",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing full operator page content %q: %q", want, stdout.String())
		}
	}
}

func TestHelpCommandWithoutArgsPrintsRootHelpToStdout(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandHelp}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}
	for _, want := range []string{
		"Validate-first CLI for reusable verification flows.",
		"Commands:",
		"Start Here:",
		"Environment:",
		commandInit,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing root help snippet %q: %q", want, stdout.String())
		}
	}
}

func TestHelpCommandPrintsOptionsForPrimaryCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "init",
			args: []string{commandHelp, commandInit},
			want: []string{
				"Options:",
				"--syntax <string>",
				"Starter contract:",
				"Write the default YAML starter:",
				"theater init --syntax thtr",
			},
		},
		{
			name: "validate",
			args: []string{commandHelp, commandValidate},
			want: []string{
				"Options:",
				"--debug-paths",
				"--plugins-config <string>",
				"Validation modes:",
				"theater help debug-selectors",
			},
		},
		{
			name: "explain",
			args: []string{commandHelp, commandExplain},
			want: []string{
				"Options:",
				"--plugins-config <string>",
				"--plugins-lock <string>",
				"Targets:",
				"theater explain action http",
				envPluginsConfig,
				"Resolution:",
			},
		},
		{
			name: "doctor",
			args: []string{commandHelp, commandDoctor},
			want: []string{
				"Options:",
				"--plugins-config <string>",
				"--plugins-lock <string>",
				"--write-path <string>",
				"Checks:",
				"theater plugins doctor",
				"Resolution:",
			},
		},
		{
			name: "fmt",
			args: []string{commandHelp, commandFmt},
			want: []string{
				"Options:",
				"--file <string>",
				"--write",
				"Formatting behavior:",
				"Rewrite the file in place:",
			},
		},
		{
			name: "lower",
			args: []string{commandHelp, commandLower},
			want: []string{
				"Options:",
				"--file <string>",
				"--map <string>",
				"Lowering behavior:",
				"Emit YAML plus a source-map sidecar:",
			},
		},
		{
			name: "plugins digest",
			args: []string{commandHelp, commandPlugins, commandPluginsDigest},
			want: []string{"Options:", "--manifest <string>", "--write"},
		},
		{
			name: "plugins inspect",
			args: []string{commandHelp, commandPlugins, commandPluginsInspect},
			want: []string{"Options:", "--plugins-config <string>", "--format <string>"},
		},
		{
			name: "plugins lock",
			args: []string{commandHelp, commandPlugins, commandPluginsLock},
			want: []string{"Options:", "--plugins-config <string>", "--plugins-lock <string>"},
		},
		{
			name: "plugins doctor",
			args: []string{commandHelp, commandPlugins, commandPluginsDoctor},
			want: []string{
				"Options:",
				"--plugins-config <string>",
				"--plugins-lock <string>",
				"Checks:",
				"doctor validates the plugin registry file",
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
				t.Fatalf("exit code mismatch: got %d want %d", got, want)
			}
			if strings.TrimSpace(stderr.String()) != "" {
				t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
			}
			for _, want := range test.want {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("stdout missing explicit help snippet %q: %q", want, stdout.String())
				}
			}
		})
	}
}

func TestHelpCommandHelpFlagsPrintHelpCommandHelp(t *testing.T) {
	t.Parallel()

	for _, helpFlag := range []string{"-h", "--help"} {
		helpFlag := helpFlag
		t.Run(helpFlag, func(t *testing.T) {
			t.Parallel()

			var stdout strings.Builder
			var stderr strings.Builder

			code := run([]string{commandHelp, helpFlag}, &stdout, &stderr)
			if got, want := code, 0; got != want {
				t.Fatalf("exit code mismatch: got %d want %d", got, want)
			}
			if strings.TrimSpace(stderr.String()) != "" {
				t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
			}
			if !strings.Contains(stdout.String(), "Show help for a command or topic.") {
				t.Fatalf("stdout missing help command help: %q", stdout.String())
			}
		})
	}
}

func TestVersionCommandPrintsVersion(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandVersion}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}
	if got, want := strings.TrimSpace(stdout.String()), "theater "+theater.Version(); got != want {
		t.Fatalf("stdout mismatch: got %q want %q", got, want)
	}
}

func TestVersionCommandUsesRootLinkerVersion(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	binaryPath := filepath.Join(tempDir, "theater")
	build := exec.Command(
		"go",
		"build",
		"-ldflags", "-X github.com/alex-poliushkin/theater.version=v-test",
		"-o", binaryPath,
		"./cmd/theater",
	)
	build.Dir = repoRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build theater binary failed: %v output=%s", err, string(output))
	}

	command := exec.Command(binaryPath, commandVersion)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run theater version failed: %v output=%s", err, string(output))
	}
	if got, want := strings.TrimSpace(string(output)), "theater v-test"; got != want {
		t.Fatalf("stdout mismatch: got %q want %q", got, want)
	}
}

func TestRootVersionFlagsPrintVersionAndExitZero(t *testing.T) {
	t.Parallel()

	for _, versionFlag := range []string{"-version", "--version"} {
		versionFlag := versionFlag
		t.Run(versionFlag, func(t *testing.T) {
			t.Parallel()

			var stdout strings.Builder
			var stderr strings.Builder

			code := run([]string{versionFlag}, &stdout, &stderr)
			if got, want := code, 0; got != want {
				t.Fatalf("exit code mismatch: got %d want %d", got, want)
			}
			if strings.TrimSpace(stderr.String()) != "" {
				t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
			}
			if got, want := strings.TrimSpace(stdout.String()), "theater "+theater.Version(); got != want {
				t.Fatalf("stdout mismatch: got %q want %q", got, want)
			}
		})
	}
}

func TestRootVersionFlagsRejectExtraArgs(t *testing.T) {
	t.Parallel()

	for _, versionFlag := range []string{"-version", "--version"} {
		versionFlag := versionFlag
		t.Run(versionFlag, func(t *testing.T) {
			t.Parallel()

			var stdout strings.Builder
			var stderr strings.Builder

			code := run([]string{versionFlag, "extra"}, &stdout, &stderr)
			if got, want := code, exitCodeCommandError; got != want {
				t.Fatalf("exit code mismatch: got %d want %d", got, want)
			}
			if strings.TrimSpace(stdout.String()) != "" {
				t.Fatalf("stdout mismatch: got %q want empty", stdout.String())
			}
			if !strings.Contains(stderr.String(), "version does not accept positional arguments") {
				t.Fatalf("stderr missing validation error: %q", stderr.String())
			}
		})
	}
}

func TestAuxCommandsHelpFlagsUseMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "version -h", args: []string{commandVersion, "-h"}, want: "Print the theater version."},
		{name: "version --help", args: []string{commandVersion, "--help"}, want: "Print the theater version."},
		{name: "completion -h", args: []string{commandCompletion, "-h"}, want: "Generate shell completion scripts."},
		{name: "completion --help", args: []string{commandCompletion, "--help"}, want: "Generate shell completion scripts."},
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
			if got := strings.TrimSpace(stderr.String()); got != "" {
				t.Fatalf("stderr mismatch: got %q want empty", got)
			}
			if !strings.Contains(stdout.String(), test.want) {
				t.Fatalf("stdout missing help snippet %q: %q", test.want, stdout.String())
			}
		})
	}
}

func TestHelpCommandTrailingHelpFlagPrintsTargetHelp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "help help -h", args: []string{commandHelp, commandHelp, "-h"}, want: "Show help for a command or topic."},
		{name: "help run --help", args: []string{commandHelp, commandRun, "--help"}, want: "Validate, execute, and render a stage run."},
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
			if !strings.Contains(stdout.String(), test.want) {
				t.Fatalf("stdout missing help snippet %q: %q", test.want, stdout.String())
			}
		})
	}
}

func TestHelpCommandSupportsRunRelatedTopics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "environment",
			args: []string{commandHelp, "environment"},
			want: []string{
				"Explain supported CLI environment variables.",
				"Scope:",
				"Environment:",
				envPluginsConfig,
				envPluginsLock,
				"Resolution:",
				"Command flags override environment variables.",
				"Related:",
				"theater plugins doctor",
			},
		},
		{
			name: "exit-codes",
			args: []string{commandHelp, "exit-codes"},
			want: []string{
				"Explain theater exit behavior.",
				"Public exit codes:",
				"0  success",
				"1  validation diagnostics, authoring diagnostics, failed run, or canceled run",
				"2  command usage error, unsupported format, or other command-level failure before a result contract exists",
			},
		},
		{
			name: "debug-selectors",
			args: []string{commandHelp, "debug-selectors"},
			want: []string{
				"Explain debug selector shape for run debug mode.",
				"Selector shape:",
				"kind=<scenario_call|act|action|expectation>,phase=<before|after>,path=<prepared runtime path>",
				"Selector files:",
				"one selector per non-empty line",
				"lines starting with # are ignored",
			},
		},
		{
			name: "formats",
			args: []string{commandHelp, "formats"},
			want: []string{
				"Explain CLI output surfaces.",
				"Output formats:",
				"text  default human-readable output for run, validate, debug-path discovery, and scenario discovery",
				"json  machine-readable stdout for run, validate, debug-path discovery, plugins inspect, and scenario discovery",
				"junit  compact scenario-call JUnit XML for run and report render",
				"markdown  detailed human-readable CI summary for report render",
				"Stdout and stderr:",
				"JSON, JUnit, and Markdown keep stdout artifact-safe while command-level failures still print on stderr.",
				"Related:",
				"theater report render --input build/run.json --format markdown",
				"theater help exit-codes",
			},
		},
		{
			name: "compatibility",
			args: []string{commandHelp, "compatibility"},
			want: []string{
				"Explain the current CLI compatibility window.",
				"Preferred syntax:",
				"Use positional stage paths for theater validate and theater run in new commands, docs, and shell history.",
				"Supported legacy forms:",
				"The current compatibility window covers every existing long option that the CLI already accepts with a single dash, not only the examples listed here.",
				"Examples include -file, -plugins-config, -plugins-lock, -live, and -debug-paths.",
				"Deprecation behavior:",
				"Legacy forms continue to run without runtime deprecation warnings during the current compatibility window",
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
				t.Fatalf("exit code mismatch: got %d want %d", got, want)
			}
			if strings.TrimSpace(stderr.String()) != "" {
				t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
			}
			for _, want := range test.want {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("stdout missing topic snippet %q: %q", want, stdout.String())
				}
			}
		})
	}
}

func TestHelpCommandSupportsCompatibilityAlias(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandHelp, "migration"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Explain the current CLI compatibility window.") {
		t.Fatalf("stdout missing compatibility topic help: %q", stdout.String())
	}
}

func TestHelpCommandRejectsUnknownTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "unknown topic",
			args: []string{commandHelp, "environmnt"},
			want: `unknown help target "environmnt"`,
		},
		{
			name: "extra tokens after topic",
			args: []string{commandHelp, "environment", "extra"},
			want: `unknown help target "environment extra"`,
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
				t.Fatalf("stderr missing error snippet %q: %q", test.want, stderr.String())
			}
			if !strings.Contains(stderr.String(), "Usage:\n  theater <command> [options]") {
				t.Fatalf("stderr missing usage output: %q", stderr.String())
			}
		})
	}
}

func TestCompletionCommandGeneratesScripts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		shell string
		want  string
	}{
		{shell: "bash", want: "_theater_complete"},
		{shell: "zsh", want: "#compdef theater"},
		{shell: "fish", want: "complete -c theater"},
		{shell: "powershell", want: "Register-ArgumentCompleter"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.shell, func(t *testing.T) {
			t.Parallel()

			var stdout strings.Builder
			var stderr strings.Builder

			code := run([]string{commandCompletion, test.shell}, &stdout, &stderr)
			if got, want := code, 0; got != want {
				t.Fatalf("exit code mismatch: got %d want %d", got, want)
			}
			if strings.TrimSpace(stderr.String()) != "" {
				t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
			}
			if !strings.Contains(stdout.String(), test.want) {
				t.Fatalf("completion script missing %q: %q", test.want, stdout.String())
			}
		})
	}
}

func TestCompletionScriptsEnableDescriptionsWhereSupported(t *testing.T) {
	t.Parallel()

	tests := []struct {
		shell                 string
		wantDescriptionBridge bool
		wantPowerShellSplit   bool
	}{
		{shell: "bash", wantDescriptionBridge: false},
		{shell: "zsh", wantDescriptionBridge: true},
		{shell: "fish", wantDescriptionBridge: true},
		{shell: "powershell", wantDescriptionBridge: true, wantPowerShellSplit: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.shell, func(t *testing.T) {
			t.Parallel()

			var stdout strings.Builder
			var stderr strings.Builder

			code := run([]string{commandCompletion, test.shell}, &stdout, &stderr)
			if got, want := code, 0; got != want {
				t.Fatalf("exit code mismatch: got %d want %d", got, want)
			}
			if strings.TrimSpace(stderr.String()) != "" {
				t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
			}

			output := stdout.String()
			if strings.Contains(output, completionDescriptionsEnv) != test.wantDescriptionBridge {
				t.Fatalf("completion script description bridge mismatch for %s: %q", test.shell, output)
			}
			if test.wantPowerShellSplit && !strings.Contains(output, "-split [char]9, 2") {
				t.Fatalf("powershell completion script missing tab split: %q", output)
			}
		})
	}
}

func TestCompleteCommandUsesMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "root", args: []string{commandComplete, "pl"}, want: []string{commandPlugins}},
		{name: "help targets", args: []string{commandComplete, commandHelp, ""}, want: []string{commandInit, commandRun, commandValidate, commandExplain, commandDoctor, commandPlugins, commandReport, "environment", "exit-codes", "formats", "debug-selectors", "compatibility", "migration"}},
		{name: "explain targets", args: []string{commandComplete, commandExplain, ""}, want: []string{"action", "actions", "inventory", "formats", "output-format", "state-backend"}},
		{name: "explain generator targets", args: []string{commandComplete, commandExplain, "generator", ""}, want: []string{"date", "email", "uuid", "timestamp"}},
		{name: "explain action targets", args: []string{commandComplete, commandExplain, "action", ""}, want: []string{"http", "generate", "action.http", "action.generate"}},
		{name: "plugins", args: []string{commandComplete, commandPlugins, ""}, want: []string{commandPluginsDigest, commandPluginsInspect, commandPluginsLock, commandPluginsDoctor}},
		{name: "plugins digest flags", args: []string{commandComplete, commandPlugins, commandPluginsDigest, "--"}, want: []string{"--manifest", "--write"}},
		{name: "run flags", args: []string{commandComplete, commandRun, "-d"}, want: []string{"-debug", "-debug-dump"}},
		{name: "run long flags", args: []string{commandComplete, commandRun, "--d"}, want: []string{"--debug", "--debug-dump"}},
		{name: "run sidecar flags", args: []string{commandComplete, commandRun, "--j"}, want: []string{"--json-output", "--junit-output"}},
		{name: "run markdown sidecar flag", args: []string{commandComplete, commandRun, "--m"}, want: []string{"--markdown-output"}},
		{name: "run overwrite flag", args: []string{commandComplete, commandRun, "--o"}, want: []string{"--overwrite"}},
		{name: "completion shells", args: []string{commandComplete, commandCompletion, ""}, want: []string{"bash", "fish", "powershell", "zsh"}},
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
			for _, want := range test.want {
				if !slices.Contains(lines, want) {
					t.Fatalf("completion output missing %q: %q", want, stdout.String())
				}
			}
			if slices.Contains(lines, commandComplete) {
				t.Fatalf("completion output leaked hidden command %q: %q", commandComplete, stdout.String())
			}
		})
	}
}

func TestCompleteCommandIncludesDescriptionsWhenEnabled(t *testing.T) {
	t.Setenv(completionDescriptionsEnv, "1")

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandComplete, ""}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	assertCompletionLine(t, lines, commandRun, "Validate, execute, and render a stage run.")
	assertCompletionLine(t, lines, commandPlugins, "Inspect, digest, lock, and diagnose plugin registries.")
}

func TestCompleteCommandSuggestsStageAndPluginPaths(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	stageYAMLPath := filepath.Join(tempDir, "scenario.yaml")
	stageTHTRPath := filepath.Join(tempDir, "scenario.thtr")
	pluginConfigPath := filepath.Join(tempDir, "plugins.json")
	pluginLockPath := filepath.Join(tempDir, "plugins.lock.json")
	notesPath := filepath.Join(tempDir, "notes.txt")
	unsafeStagePath := filepath.Join(tempDir, "bad\tstage.yaml")

	for _, path := range []string{stageYAMLPath, stageTHTRPath, pluginConfigPath, pluginLockPath, notesPath, unsafeStagePath} {
		if err := os.WriteFile(path, []byte("test\n"), 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", path, err)
		}
	}

	tests := []struct {
		name   string
		args   []string
		want   []string
		absent []string
	}{
		{
			name:   "run positional stage path",
			args:   []string{commandComplete, commandRun, filepath.Join(tempDir, "sce")},
			want:   []string{stageYAMLPath, stageTHTRPath},
			absent: []string{pluginConfigPath, notesPath, unsafeStagePath},
		},
		{
			name:   "fmt positional file path",
			args:   []string{commandComplete, commandFmt, filepath.Join(tempDir, "sce")},
			want:   []string{stageTHTRPath},
			absent: []string{stageYAMLPath, pluginConfigPath},
		},
		{
			name:   "fmt check positional file path",
			args:   []string{commandComplete, commandFmt, "--check", filepath.Join(tempDir, "sce")},
			want:   []string{stageTHTRPath},
			absent: []string{stageYAMLPath, pluginConfigPath},
		},
		{
			name:   "fmt diff positional file path",
			args:   []string{commandComplete, commandFmt, "--diff", filepath.Join(tempDir, "sce")},
			want:   []string{stageTHTRPath},
			absent: []string{stageYAMLPath, pluginConfigPath},
		},
		{
			name:   "lower positional file path",
			args:   []string{commandComplete, commandLower, filepath.Join(tempDir, "sce")},
			want:   []string{stageTHTRPath},
			absent: []string{stageYAMLPath, pluginConfigPath},
		},
		{
			name:   "lower map then positional file path",
			args:   []string{commandComplete, commandLower, "--map", filepath.Join(tempDir, "stage.map.json"), filepath.Join(tempDir, "sce")},
			want:   []string{stageTHTRPath},
			absent: []string{stageYAMLPath, pluginConfigPath},
		},
		{
			name:   "lower path already supplied",
			args:   []string{commandComplete, commandLower, stageTHTRPath, filepath.Join(tempDir, "sce")},
			absent: []string{stageTHTRPath, stageYAMLPath, pluginConfigPath},
		},
		{
			name:   "plugins config path",
			args:   []string{commandComplete, commandPlugins, commandPluginsInspect, "--plugins-config", filepath.Join(tempDir, "pl")},
			want:   []string{pluginConfigPath},
			absent: []string{stageYAMLPath, pluginLockPath, notesPath},
		},
		{
			name:   "plugins lock path",
			args:   []string{commandComplete, commandRun, "--plugins-lock", filepath.Join(tempDir, "plugins")},
			want:   []string{pluginLockPath},
			absent: []string{stageYAMLPath, pluginConfigPath, notesPath},
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
				t.Fatalf("exit code mismatch: got %d want %d", got, want)
			}
			if strings.TrimSpace(stderr.String()) != "" {
				t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
			}

			lines := strings.Fields(stdout.String())
			for _, want := range test.want {
				if !slices.Contains(lines, want) {
					t.Fatalf("completion output missing %q: %q", want, stdout.String())
				}
			}
			for _, absent := range test.absent {
				if slices.Contains(lines, absent) {
					t.Fatalf("completion output unexpectedly included %q: %q", absent, stdout.String())
				}
			}
		})
	}
}

func TestCompleteCommandRestrictsInitTargetsToTheaterFlows(t *testing.T) {
	repoRoot := t.TempDir()
	restore := chdirForTest(t, repoRoot)
	defer restore()

	goodYAMLPath := filepath.Join(repoRoot, "theater", "flows", "http", "good.yaml")
	goodTHTRPath := filepath.Join(repoRoot, "theater", "flows", "http", "good.thtr")
	badPath := filepath.Join(repoRoot, "other", "bad.yaml")
	for _, path := range []string{goodYAMLPath, goodTHTRPath, badPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s failed: %v", path, err)
		}
		if err := os.WriteFile(path, []byte("test\n"), 0o644); err != nil {
			t.Fatalf("write %s failed: %v", path, err)
		}
	}

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "blank prefix suggests flows root",
			args: []string{commandComplete, commandInit, ""},
			want: []string{filepath.Join("theater", "flows") + string(filepath.Separator)},
		},
		{
			name: "partial prefix suggests flows root",
			args: []string{commandComplete, commandInit, filepath.Join("theater", "f")},
			want: []string{filepath.Join("theater", "flows") + string(filepath.Separator)},
		},
		{
			name: "prefix under flows suggests only flow files",
			args: []string{commandComplete, commandInit, filepath.Join("theater", "flows", "http", "go")},
			want: []string{
				filepath.Join("theater", "flows", "http", "good.yaml"),
				filepath.Join("theater", "flows", "http", "good.thtr"),
			},
		},
	}

	for _, test := range tests {
		var stdout strings.Builder
		var stderr strings.Builder

		code := run(test.args, &stdout, &stderr)
		if got, want := code, 0; got != want {
			t.Fatalf("%s exit code mismatch: got %d want %d", test.name, got, want)
		}
		if strings.TrimSpace(stderr.String()) != "" {
			t.Fatalf("%s stderr mismatch: got %q want empty", test.name, stderr.String())
		}

		lines := strings.Fields(stdout.String())
		for _, want := range test.want {
			if !slices.Contains(lines, want) {
				t.Fatalf("%s completion output missing %q: %q", test.name, want, stdout.String())
			}
		}
		if slices.Contains(lines, filepath.Join("other", "bad.yaml")) {
			t.Fatalf("%s completion output leaked non-flows target: %q", test.name, stdout.String())
		}
	}

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandComplete, commandInit, filepath.Join("other", "b")}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("outside-root exit code mismatch: got %d want %d", got, want)
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("outside-root stderr mismatch: got %q want empty", stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != "" {
		t.Fatalf("outside-root completion must stay empty, got %q", stdout.String())
	}
}

func TestBashCompletionScriptUsesCompleteBackendEndToEnd(t *testing.T) {
	t.Parallel()

	bashPath := requireShell(t, "bash")

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandCompletion, "bash"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}

	tempDir := t.TempDir()
	binaryPath := filepath.Join(tempDir, "theater")
	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/theater")
	build.Dir = repoRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build theater binary failed: %v output=%s", err, string(output))
	}

	scriptPath := filepath.Join(tempDir, "complete.sh")
	script := stdout.String() + `
COMP_WORDS=(theater plugins "")
COMP_CWORD=2
_theater_complete
printf '%s\n' "${COMPREPLY[@]}"
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write completion script failed: %v", err)
	}

	command := exec.Command(bashPath, scriptPath)
	command.Env = append(os.Environ(), "PATH="+tempDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run completion script failed: %v output=%s", err, string(output))
	}

	lines := strings.Fields(string(output))
	for _, want := range []string{commandPluginsDigest, commandPluginsInspect, commandPluginsLock, commandPluginsDoctor} {
		if !slices.Contains(lines, want) {
			t.Fatalf("completion script output missing %q: %q", want, string(output))
		}
	}
}

func TestBashCompletionScriptCompletesStagePathsEndToEnd(t *testing.T) {
	t.Parallel()

	bashPath := requireShell(t, "bash")

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandCompletion, "bash"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}

	tempDir := t.TempDir()
	stagePath := filepath.Join(tempDir, "scenario.yaml")
	notesPath := filepath.Join(tempDir, "notes.txt")
	for _, path := range []string{stagePath, notesPath} {
		if err := os.WriteFile(path, []byte("test\n"), 0o644); err != nil {
			t.Fatalf("write fixture %s failed: %v", path, err)
		}
	}

	binaryPath := filepath.Join(tempDir, "theater")
	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/theater")
	build.Dir = repoRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build theater binary failed: %v output=%s", err, string(output))
	}

	scriptPath := filepath.Join(tempDir, "complete-stage.sh")
	script := stdout.String() + `
COMP_WORDS=(theater run "` + filepath.Join(tempDir, "sc") + `")
COMP_CWORD=2
_theater_complete
printf '%s\n' "${COMPREPLY[@]}"
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write completion script failed: %v", err)
	}

	command := exec.Command(bashPath, scriptPath)
	command.Env = append(os.Environ(), "PATH="+tempDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run completion script failed: %v output=%s", err, string(output))
	}

	lines := strings.Fields(string(output))
	if !slices.Contains(lines, stagePath) {
		t.Fatalf("stage-path completion missing %q: %q", stagePath, string(output))
	}
	if slices.Contains(lines, notesPath) {
		t.Fatalf("stage-path completion leaked non-stage file %q: %q", notesPath, string(output))
	}
}

func TestZshCompletionScriptUsesCompleteBackendEndToEnd(t *testing.T) {
	t.Parallel()

	zshPath := requireShell(t, "zsh")

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandCompletion, "zsh"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}

	tempDir := t.TempDir()
	binaryPath := filepath.Join(tempDir, "theater")
	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/theater")
	build.Dir = repoRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build theater binary failed: %v output=%s", err, string(output))
	}

	scriptPath := filepath.Join(tempDir, "complete.zsh")
	script := `compdef() { :; }` + "\n" + stdout.String() + `
compadd() {
  while (( $# != 0 )); do
    case "$1" in
      -d)
        shift 2
        ;;
      --)
        shift
        break
        ;;
      *)
        shift
        ;;
    esac
  done
  print -rl -- "$@"
}
words=(theater plugins "")
_theater_complete
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write completion script failed: %v", err)
	}

	command := exec.Command(zshPath, scriptPath)
	command.Env = append(os.Environ(), "PATH="+tempDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run completion script failed: %v output=%s", err, string(output))
	}

	lines := strings.Fields(string(output))
	for _, want := range []string{commandPluginsDigest, commandPluginsInspect, commandPluginsLock, commandPluginsDoctor} {
		if !slices.Contains(lines, want) {
			t.Fatalf("completion script output missing %q: %q", want, string(output))
		}
	}
}

func TestZshCompletionScriptBridgesDescriptionsEndToEnd(t *testing.T) {
	t.Parallel()

	zshPath := requireShell(t, "zsh")

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandCompletion, "zsh"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}

	tempDir := t.TempDir()
	binaryPath := filepath.Join(tempDir, "theater")
	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/theater")
	build.Dir = repoRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build theater binary failed: %v output=%s", err, string(output))
	}

	scriptPath := filepath.Join(tempDir, "complete-desc.zsh")
	script := `compdef() { :; }` + "\n" + stdout.String() + `
compadd() {
  local description_array=""
  if [[ "$1" == "-d" ]]; then
    description_array="$2"
    shift 2
  fi
  if [[ "$1" == "--" ]]; then
    shift
  fi
  print -rl -- "$@"
  if [[ -n "${description_array}" ]]; then
    local -a descriptions
    eval "descriptions=(\"\${${description_array}[@]}\")"
    print -rl -- "DESC:${descriptions[@]}"
  fi
}
words=(theater "")
_theater_complete
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write completion script failed: %v", err)
	}

	command := exec.Command(zshPath, scriptPath)
	command.Env = append(os.Environ(), "PATH="+tempDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run completion script failed: %v output=%s", err, string(output))
	}

	rendered := string(output)
	if !strings.Contains(rendered, commandRun) {
		t.Fatalf("zsh completion output missing %q: %q", commandRun, rendered)
	}
	if !strings.Contains(rendered, "Validate, execute, and render a stage run.") {
		t.Fatalf("zsh completion output missing description bridge: %q", rendered)
	}
}

func assertCompletionLine(t *testing.T, lines []string, value, description string) {
	t.Helper()

	want := value + "\t" + description
	for _, line := range lines {
		if line == want {
			return
		}
	}

	t.Fatalf("completion output missing %q in %q", want, strings.Join(lines, "\n"))
}

func requireShell(t *testing.T, shell string) string {
	t.Helper()

	path, err := exec.LookPath(shell)
	if err != nil {
		t.Skipf("%s is not available: %v", shell, err)
	}
	return path
}
