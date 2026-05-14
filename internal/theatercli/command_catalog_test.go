package theatercli

import (
	"strings"
	"testing"
)

func TestRunWithoutArgsPrintsMetadataDrivenRootHelp(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run(nil, &stdout, &stderr)
	if got, want := code, exitCodeCommandError; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}

	output := stderr.String()
	if !strings.Contains(output, "Validate-first CLI for reusable verification flows.") {
		t.Fatalf("root help missing short description: %q", output)
	}
	if !strings.Contains(output, "Commands:") {
		t.Fatalf("root help missing commands section: %q", output)
	}
	if !strings.Contains(output, commandInit) || !strings.Contains(output, "Create a small repo-aware starter stage.") {
		t.Fatalf("root help missing init onboarding surface: %q", output)
	}
	if !strings.Contains(output, envPluginsConfig) || !strings.Contains(output, "Resolution:") {
		t.Fatalf("root help missing environment contract: %q", output)
	}
	if !strings.Contains(output, defaultResolutionEnvOverridesBuiltIns) {
		t.Fatalf("root help missing precedence contract: %q", output)
	}
	if strings.Contains(output, "usage:\n") {
		t.Fatalf("root help must no longer use legacy lowercase usage block: %q", output)
	}
}

func TestRunCommandHelpUsesCommandMetadata(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandRun, "-h"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	output := stdout.String()
	if !strings.Contains(output, "Validate, execute, and render a stage run.") {
		t.Fatalf("command help missing short description: %q", output)
	}
	if !strings.Contains(output, "Usage:\n  theater run "+stageFileArgument) {
		t.Fatalf("command help missing positional usage: %q", output)
	}
	if !strings.Contains(output, "Examples:") {
		t.Fatalf("command help missing examples section: %q", output)
	}
	for _, want := range []string{
		"Quiet text output:",
		"Machine-readable JSON output:",
		"CI-friendly JUnit output:",
		"Dump debug sidecar:",
		"Interactive debug session:",
		"Run with plugin registry and lock files:",
		"Run with a plugin report exporter:",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("command help missing run example %q: %q", want, output)
		}
	}
	for _, want := range []string{
		"theater run theater/flows/http/example-domain.yaml --format json > build/example-domain.run.json",
		"theater run theater/flows/plugins/hello-world-plugin.yaml",
		"--plugin-exporter <report-exporter-capability>",
		"--live off",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("command help missing run example body %q: %q", want, output)
		}
	}
	if !strings.Contains(output, "Output:") {
		t.Fatalf("command help missing grouped flags: %q", output)
	}
	for _, want := range []string{
		"Output behavior:",
		"Debug workflow:",
		"Related:",
		"theater validate " + stageFileArgument,
		"theater help exit-codes",
		"theater help debug-selectors",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("command help missing run operator guidance %q: %q", want, output)
		}
	}
	if !strings.Contains(output, envPluginsConfig) || !strings.Contains(output, defaultResolutionFlagsOverrideEnv) {
		t.Fatalf("command help missing default resolution contract: %q", output)
	}
	if !strings.Contains(output, defaultResolutionEnvOverridesBuiltIns) {
		t.Fatalf("command help missing precedence contract: %q", output)
	}
	if !strings.Contains(output, "--file remains available when explicit spelling helps readability") {
		t.Fatalf("command help missing compatibility note: %q", output)
	}
	if !strings.Contains(output, "--file <string>") || !strings.Contains(output, "--live <string>") {
		t.Fatalf("command help missing long-form options: %q", output)
	}
	for _, want := range []string{
		"debug selector (repeatable)",
		"path to a debug selector file (repeatable)",
		"report exporter capability to invoke after run (repeatable)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("command help missing repeatable option contract %q: %q", want, output)
		}
	}
	if strings.Contains(output, "\n    -file <string>") {
		t.Fatalf("command help must not present legacy single-dash long options as primary: %q", output)
	}
	if strings.Contains(output, "Usage of run:") {
		t.Fatalf("command help must not use raw flag usage: %q", output)
	}
}

func TestInitCommandHelpUsesCommandMetadata(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandInit, "-h"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	output := stdout.String()
	for _, want := range []string{
		"Create a small repo-aware starter stage.",
		"Usage:\n  theater init [theater/flows/.../starter.{yaml|thtr}] [--syntax yaml|thtr]",
		"Write the default YAML starter:",
		"Start with compact .thtr authoring:",
		"Starter contract:",
		"Syntax:",
		"Related:",
		"--syntax <string>",
		"theater init theater/flows/http/login-smoke.yaml",
		"theater validate theater/flows/http/starter.yaml",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("init help missing %q: %q", want, output)
		}
	}
	if strings.Contains(output, "Usage of init:") {
		t.Fatalf("init help must not use raw flag usage: %q", output)
	}
}

func TestPluginsCommandWithoutSubcommandPrintsPluginHelp(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandPlugins}, &stdout, &stderr)
	if got, want := code, exitCodeCommandError; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}

	output := stderr.String()
	if !strings.Contains(output, "plugins requires a subcommand") {
		t.Fatalf("plugin command missing original error message: %q", output)
	}
	if !strings.Contains(output, "Inspect, digest, lock, and diagnose plugin registries.") {
		t.Fatalf("plugin command missing metadata help: %q", output)
	}
	for _, want := range []string{
		"digest",
		"inspect",
		"lock",
		"doctor",
		"1. theater plugins inspect",
		"2. theater plugins digest",
		"3. theater plugins lock",
		"4. theater plugins doctor",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("plugin command help missing %q: %q", want, output)
		}
	}
}

func TestPluginsHelpFlagsPrintPluginHelpWithoutUnknownSubcommandError(t *testing.T) {
	t.Parallel()

	for _, helpFlag := range []string{"-h", "--help"} {
		helpFlag := helpFlag
		t.Run(helpFlag, func(t *testing.T) {
			t.Parallel()

			var stdout strings.Builder
			var stderr strings.Builder

			code := run([]string{commandPlugins, helpFlag}, &stdout, &stderr)
			if got, want := code, 0; got != want {
				t.Fatalf("exit code mismatch: got %d want %d", got, want)
			}
			if got := strings.TrimSpace(stderr.String()); got != "" {
				t.Fatalf("stderr mismatch: got %q want empty", got)
			}

			output := stdout.String()
			if !strings.Contains(output, "Inspect, digest, lock, and diagnose plugin registries.") {
				t.Fatalf("plugin help missing metadata help: %q", output)
			}
			if strings.Contains(output, "unknown plugins subcommand") {
				t.Fatalf("plugin help must not report an unknown subcommand: %q", output)
			}
		})
	}
}

func TestPluginsInspectHelpUsesCommandMetadata(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandPlugins, commandPluginsInspect, "-h"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	output := stdout.String()
	if !strings.Contains(output, "Resolve and print the current plugin set.") {
		t.Fatalf("plugin inspect help missing short description: %q", output)
	}
	if !strings.Contains(output, "Examples:") {
		t.Fatalf("plugin inspect help missing examples section: %q", output)
	}
	for _, want := range []string{
		"Output:",
		"inspect prints the resolved plugin ids, versions, manifest paths, executable paths, and allowed capabilities.",
		"theater plugins doctor",
		envPluginsLock,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("plugin inspect help missing %q: %q", want, output)
		}
	}
	if !strings.Contains(output, defaultResolutionFlagsOverrideEnv) || !strings.Contains(output, defaultResolutionPluginRegistryRequired) || !strings.Contains(output, defaultResolutionPluginLockOptional) {
		t.Fatalf("plugin inspect help missing precedence contract: %q", output)
	}
	if !strings.Contains(output, "--plugins-config <string>") || !strings.Contains(output, "--format <string>") {
		t.Fatalf("plugin inspect help missing long-form options: %q", output)
	}
	if strings.Contains(output, "\n    -plugins-config <string>") {
		t.Fatalf("plugin inspect help must not present legacy single-dash long options as primary: %q", output)
	}
	if strings.Contains(output, "Usage of plugins inspect:") {
		t.Fatalf("plugin inspect help must not use raw flag usage: %q", output)
	}
}

func TestPluginsDoctorHelpUsesCommandMetadata(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandPlugins, commandPluginsDoctor, "-h"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d", got, want)
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	output := stdout.String()
	for _, want := range []string{
		"Diagnose plugin registry readiness.",
		"Checks:",
		"doctor validates the plugin registry file, loads each plugin manifest, resolves each executable path, and optionally verifies manifest/executable checksums against the lock file.",
		"--plugins-readiness descriptor checks descriptor and manifest-lock readiness without resolving env_from_host grants or launching plugin code.",
		"theater plugins doctor --plugins-config build/sqlite.plugins.json",
		"--plugins-config <string>",
		"--plugins-lock <string>",
		"--plugins-readiness <string>",
		envPluginsConfig,
		defaultResolutionFlagsOverrideEnv,
		defaultResolutionPluginRegistryRequired,
		defaultResolutionPluginLockOptional,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("plugin doctor help missing %q: %q", want, output)
		}
	}
	if strings.Contains(output, "Usage of plugins doctor:") {
		t.Fatalf("plugin doctor help must not use raw flag usage: %q", output)
	}
	if strings.Contains(output, "--format <string>") {
		t.Fatalf("plugin doctor help must not expose inspect-only --format: %q", output)
	}
}

func TestCommandHelpUsesMetadataAcrossCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		args          []string
		wantSnippet   string
		legacySnippet string
	}{
		{
			name:          "explain",
			args:          []string{commandExplain, "-h"},
			wantSnippet:   "Explain discoverable runtime capabilities.",
			legacySnippet: "Usage of explain:",
		},
		{
			name:          "doctor",
			args:          []string{commandDoctor, "-h"},
			wantSnippet:   "Check theater workflow preconditions.",
			legacySnippet: "Usage of doctor:",
		},
		{
			name:          "validate",
			args:          []string{commandValidate, "-h"},
			wantSnippet:   "Compile and validate a stage without live execution.",
			legacySnippet: "Usage of validate:",
		},
		{
			name:          "fmt",
			args:          []string{commandFmt, "-h"},
			wantSnippet:   "Format one .thtr file into canonical layout.",
			legacySnippet: "Usage of fmt:",
		},
		{
			name:          "lower",
			args:          []string{commandLower, "-h"},
			wantSnippet:   "Lower one .thtr file into canonical YAML.",
			legacySnippet: "Usage of lower:",
		},
		{
			name:          "plugins doctor",
			args:          []string{commandPlugins, commandPluginsDoctor, "-h"},
			wantSnippet:   "Diagnose plugin registry readiness.",
			legacySnippet: "Usage of plugins doctor:",
		},
		{
			name:          "plugins lock",
			args:          []string{commandPlugins, commandPluginsLock, "-h"},
			wantSnippet:   "Write the plugin checksum lock file.",
			legacySnippet: "Usage of plugins lock:",
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
			if got := strings.TrimSpace(stderr.String()); got != "" {
				t.Fatalf("stderr mismatch: got %q want empty", got)
			}

			output := stdout.String()
			if !strings.Contains(output, test.wantSnippet) {
				t.Fatalf("help output missing metadata snippet %q: %q", test.wantSnippet, output)
			}
			if strings.Contains(output, test.legacySnippet) {
				t.Fatalf("help output must not use legacy flag usage %q: %q", test.legacySnippet, output)
			}
			if !strings.Contains(output, "Examples:") {
				t.Fatalf("help output missing examples section: %q", output)
			}
			if test.name == "validate" {
				if !strings.Contains(output, "Usage:\n  theater validate "+stageFileArgument) {
					t.Fatalf("validate help missing positional usage: %q", output)
				}
				if !strings.Contains(output, defaultResolutionFlagsOverrideEnv) || !strings.Contains(output, defaultResolutionEnvOverridesBuiltIns) {
					t.Fatalf("validate help missing precedence contract: %q", output)
				}
				if !strings.Contains(output, "--file remains available when explicit spelling helps readability") {
					t.Fatalf("validate help missing compatibility note: %q", output)
				}
				if !strings.Contains(output, "--debug-paths") {
					t.Fatalf("validate help missing long-form flag presentation: %q", output)
				}
				if !strings.Contains(output, "--plugins-readiness <string>") || !strings.Contains(output, "descriptor validates descriptor-backed stage structure") {
					t.Fatalf("validate help missing plugin readiness mode: %q", output)
				}
			}
			if test.name == "doctor" {
				for _, want := range []string{
					"--write-path <string>",
					"string output path that must be writable (repeatable)",
					"doctor checks the current working directory for the expected theater/flows and theater/lib layout.",
					"Interactive debug TTY availability is advisory",
					"Missing parent directories for write paths are created during the check",
					"theater plugins doctor",
				} {
					if !strings.Contains(output, want) {
						t.Fatalf("doctor help missing %q: %q", want, output)
					}
				}
			}
			if test.name == "fmt" {
				if !strings.Contains(output, "--file <string>") || !strings.Contains(output, "--write") ||
					!strings.Contains(output, "--check") || !strings.Contains(output, "--diff") {
					t.Fatalf("fmt help missing long-form options: %q", output)
				}
			}
			if test.name == "lower" {
				if !strings.Contains(output, "--file <string>") || !strings.Contains(output, "--map <string>") {
					t.Fatalf("lower help missing long-form options: %q", output)
				}
			}
		})
	}
}

func TestCommandLongHelpFlagsExitSuccessfully(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "run", args: []string{commandRun, "--help"}, want: "Validate, execute, and render a stage run."},
		{name: "run with path", args: []string{commandRun, "stage.yaml", "--help"}, want: "Validate, execute, and render a stage run."},
		{name: "validate", args: []string{commandValidate, "--help"}, want: "Compile and validate a stage without live execution."},
		{name: "fmt", args: []string{commandFmt, "--help"}, want: "Format one .thtr file into canonical layout."},
		{name: "fmt with path", args: []string{commandFmt, "stage.thtr", "--help"}, want: "Format one .thtr file into canonical layout."},
		{name: "lower", args: []string{commandLower, "--help"}, want: "Lower one .thtr file into canonical YAML."},
		{name: "lower with path", args: []string{commandLower, "stage.thtr", "--help"}, want: "Lower one .thtr file into canonical YAML."},
		{name: "plugins digest", args: []string{commandPlugins, commandPluginsDigest, "--help"}, want: "Print or update a plugin manifest descriptor digest."},
		{name: "plugins doctor", args: []string{commandPlugins, commandPluginsDoctor, "--help"}, want: "Diagnose plugin registry readiness."},
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

func TestAuthoringCommandHelpPagesUseWorkflowGuidance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "validate",
			args: []string{commandValidate, "-h"},
			want: []string{
				"Stage validation in text:",
				"Machine-readable validation output:",
				"Discover reusable debug selectors:",
				"Validation modes:",
				"--debug-paths prepares the stage and prints reusable debug selectors instead of the normal valid output.",
				"theater help debug-selectors",
				"theater validate theater/flows/http/example-domain.thtr",
			},
		},
		{
			name: "fmt",
			args: []string{commandFmt, "-h"},
			want: []string{
				"Print canonical .thtr to stdout:",
				"Rewrite the file in place:",
				"Formatting behavior:",
				"--write rewrites the input file in place; without it, fmt prints the canonical .thtr form to stdout.",
				"--check exits non-zero when the file is not already formatted",
				"--diff prints the formatting changes without rewriting the input file.",
				"--diff may be combined with --check",
				"--write cannot be combined with --check or --diff.",
				"theater lower <path.thtr>",
				"theater fmt theater/flows/http/example-domain.thtr --write",
				"theater fmt --check theater/flows/http/example-domain.thtr",
			},
		},
		{
			name: "lower",
			args: []string{commandLower, "-h"},
			want: []string{
				"Print canonical YAML to stdout:",
				"Emit YAML plus a source-map sidecar:",
				"Lowering behavior:",
				"lower always writes canonical YAML to stdout; --map adds a JSON source-map sidecar without changing stdout.",
				"theater fmt <path.thtr>",
				"theater lower theater/flows/http/example-domain.thtr --map build/example-domain.map.json",
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
			if got := strings.TrimSpace(stderr.String()); got != "" {
				t.Fatalf("stderr mismatch: got %q want empty", got)
			}

			output := stdout.String()
			for _, want := range test.want {
				if !strings.Contains(output, want) {
					t.Fatalf("help output missing authoring snippet %q: %q", want, output)
				}
			}
		})
	}
}

func TestPluginHelpPagesIncludeSharedDefaultResolution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "doctor", args: []string{commandDoctor, "-h"}},
		{name: "explain", args: []string{commandExplain, "-h"}},
		{name: "plugins", args: []string{commandPlugins, "-h"}},
		{name: "plugins doctor", args: []string{commandPlugins, commandPluginsDoctor, "-h"}},
		{name: "plugins lock", args: []string{commandPlugins, commandPluginsLock, "-h"}},
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

			output := stdout.String()
			if !strings.Contains(output, defaultResolutionFlagsOverrideEnv) || !strings.Contains(output, defaultResolutionNoConfigFile) {
				t.Fatalf("plugin help missing baseline default-resolution contract: %q", output)
			}
			if strings.Contains(test.name, "explain") {
				if !strings.Contains(output, defaultResolutionEnvOverridesBuiltIns) || !strings.Contains(output, defaultResolutionPluginBuiltIns) {
					t.Fatalf("explain help missing built-in fallback contract: %q", output)
				}
				if !strings.Contains(output, envPluginsConfig) || !strings.Contains(output, "--plugins-lock <string>") {
					t.Fatalf("explain help missing shared plugin contract: %q", output)
				}
			}
			if strings.Contains(test.name, "doctor") && test.name == "doctor" {
				if !strings.Contains(output, defaultResolutionEnvSatisfyPluginFiles) || !strings.Contains(output, defaultResolutionPluginFilesSkipped) {
					t.Fatalf("doctor help missing doctor-specific defaults: %q", output)
				}
				if !strings.Contains(output, "--plugins-config <string>") || !strings.Contains(output, "--write-path <string>") {
					t.Fatalf("doctor help missing long-form options: %q", output)
				}
			}
			if strings.Contains(test.name, "plugins") && test.name == "plugins" {
				if !strings.Contains(output, defaultResolutionEnvSatisfyPluginFiles) || !strings.Contains(output, defaultResolutionPluginCommandsNeedFiles) {
					t.Fatalf("plugins help missing plugin-family defaults: %q", output)
				}
			}
			if strings.Contains(test.name, "plugins lock") {
				if !strings.Contains(output, defaultResolutionPluginRegistryRequired) || !strings.Contains(output, defaultResolutionPluginLockRequired) {
					t.Fatalf("plugin lock help missing lock-specific defaults: %q", output)
				}
				if !strings.Contains(output, "--plugins-config <string>") || !strings.Contains(output, "--plugins-lock <string>") {
					t.Fatalf("plugin lock help missing long-form options: %q", output)
				}
			}
			if strings.Contains(test.name, "plugins doctor") {
				if !strings.Contains(output, defaultResolutionPluginRegistryRequired) || !strings.Contains(output, defaultResolutionPluginLockOptional) {
					t.Fatalf("plugin doctor help missing doctor-specific defaults: %q", output)
				}
				if !strings.Contains(output, "--plugins-config <string>") || !strings.Contains(output, "--plugins-lock <string>") {
					t.Fatalf("plugin doctor help missing long-form options: %q", output)
				}
			}
		})
	}
}

func TestCommandCatalogRootGroupsStayStable(t *testing.T) {
	t.Parallel()

	catalog := newCommandCatalog()
	root := catalog.Must()

	if got, want := len(root.Groups), 5; got != want {
		t.Fatalf("root group count mismatch: got %d want %d", got, want)
	}

	tests := []struct {
		group int
		names []string
	}{
		{group: 0, names: []string{commandInit, commandValidate, commandRun}},
		{group: 1, names: []string{commandFmt, commandLower, commandMigrate}},
		{group: 2, names: []string{commandExplain, commandDoctor, commandList, commandReport}},
		{group: 3, names: []string{commandPlugins}},
		{group: 4, names: []string{commandHelp, commandVersion, commandCompletion}},
	}

	for _, test := range tests {
		group := root.Groups[test.group]
		if got, want := len(group.Commands), len(test.names); got != want {
			t.Fatalf("group %q count mismatch: got %d want %d", group.Title, got, want)
		}
		for i, want := range test.names {
			if got := group.Commands[i].Name; got != want {
				t.Fatalf("group %q command %d mismatch: got %q want %q", group.Title, i, got, want)
			}
		}
	}
}
