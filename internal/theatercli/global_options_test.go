package theatercli

import (
	"io"
	"testing"
)

func TestParseCommandOptionsResolvePluginDefaultsFromEnvironment(t *testing.T) {
	t.Setenv(envPluginsConfig, "env.plugins.json")
	t.Setenv(envPluginsLock, "env.plugins.lock.json")

	app := newApplication(io.Discard, io.Discard)
	options, ok := app.parseCommandOptions(commandValidate, []string{"-file", "stage.yaml"})
	if !ok {
		t.Fatal("parse command options returned false")
	}
	if got, want := options.pluginsConfig, "env.plugins.json"; got != want {
		t.Fatalf("plugins config mismatch: got %q want %q", got, want)
	}
	if got, want := options.pluginsLock, "env.plugins.lock.json"; got != want {
		t.Fatalf("plugins lock mismatch: got %q want %q", got, want)
	}
}

func TestParseCommandOptionsFlagsOverridePluginEnvironment(t *testing.T) {
	t.Setenv(envPluginsConfig, "env.plugins.json")
	t.Setenv(envPluginsLock, "env.plugins.lock.json")

	app := newApplication(io.Discard, io.Discard)
	options, ok := app.parseCommandOptions(commandRun, []string{
		"-file", "stage.yaml",
		"-plugins-config", "flag.plugins.json",
		"-plugins-lock", "flag.plugins.lock.json",
	})
	if !ok {
		t.Fatal("parse command options returned false")
	}
	if got, want := options.pluginsConfig, "flag.plugins.json"; got != want {
		t.Fatalf("plugins config mismatch: got %q want %q", got, want)
	}
	if got, want := options.pluginsLock, "flag.plugins.lock.json"; got != want {
		t.Fatalf("plugins lock mismatch: got %q want %q", got, want)
	}
}

func TestParseCommandOptionsResolveMixedPluginSources(t *testing.T) {
	tests := []struct {
		name       string
		envConfig  string
		envLock    string
		args       []string
		wantConfig string
		wantLock   string
	}{
		{
			name:       "env config with flag lock",
			envConfig:  "env.plugins.json",
			args:       []string{"-file", "stage.yaml", "-plugins-lock", "flag.plugins.lock.json"},
			wantConfig: "env.plugins.json",
			wantLock:   "flag.plugins.lock.json",
		},
		{
			name:       "flag config with env lock",
			envLock:    "env.plugins.lock.json",
			args:       []string{"-file", "stage.yaml", "-plugins-config", "flag.plugins.json"},
			wantConfig: "flag.plugins.json",
			wantLock:   "env.plugins.lock.json",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if test.envConfig != "" {
				t.Setenv(envPluginsConfig, test.envConfig)
			}
			if test.envLock != "" {
				t.Setenv(envPluginsLock, test.envLock)
			}

			app := newApplication(io.Discard, io.Discard)
			options, ok := app.parseCommandOptions(commandRun, test.args)
			if !ok {
				t.Fatal("parse command options returned false")
			}
			if got, want := options.pluginsConfig, test.wantConfig; got != want {
				t.Fatalf("plugins config mismatch: got %q want %q", got, want)
			}
			if got, want := options.pluginsLock, test.wantLock; got != want {
				t.Fatalf("plugins lock mismatch: got %q want %q", got, want)
			}
		})
	}
}

func TestParsePluginCommandOptionsResolvePluginDefaultsFromEnvironment(t *testing.T) {
	t.Setenv(envPluginsConfig, "env.plugins.json")
	t.Setenv(envPluginsLock, "env.plugins.lock.json")

	app := newApplication(io.Discard, io.Discard)
	options, ok := app.parsePluginCommandOptions(commandPluginsInspect, nil)
	if !ok {
		t.Fatal("parse plugin command options returned false")
	}
	if got, want := options.pluginsConfig, "env.plugins.json"; got != want {
		t.Fatalf("plugins config mismatch: got %q want %q", got, want)
	}
	if got, want := options.pluginsLock, "env.plugins.lock.json"; got != want {
		t.Fatalf("plugins lock mismatch: got %q want %q", got, want)
	}
}

func TestParsePluginCommandOptionsFlagsOverridePluginEnvironment(t *testing.T) {
	t.Setenv(envPluginsConfig, "env.plugins.json")
	t.Setenv(envPluginsLock, "env.plugins.lock.json")

	app := newApplication(io.Discard, io.Discard)
	options, ok := app.parsePluginCommandOptions(commandPluginsLock, []string{
		"-plugins-config", "flag.plugins.json",
		"-plugins-lock", "flag.plugins.lock.json",
	})
	if !ok {
		t.Fatal("parse plugin command options returned false")
	}
	if got, want := options.pluginsConfig, "flag.plugins.json"; got != want {
		t.Fatalf("plugins config mismatch: got %q want %q", got, want)
	}
	if got, want := options.pluginsLock, "flag.plugins.lock.json"; got != want {
		t.Fatalf("plugins lock mismatch: got %q want %q", got, want)
	}
}

func TestParsePluginCommandOptionsResolveMixedPluginSources(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		envConfig  string
		envLock    string
		args       []string
		wantConfig string
		wantLock   string
	}{
		{
			name:       "env config with flag lock",
			command:    commandPluginsLock,
			envConfig:  "env.plugins.json",
			args:       []string{"-plugins-lock", "flag.plugins.lock.json"},
			wantConfig: "env.plugins.json",
			wantLock:   "flag.plugins.lock.json",
		},
		{
			name:       "flag config with env lock",
			command:    commandPluginsInspect,
			envLock:    "env.plugins.lock.json",
			args:       []string{"-plugins-config", "flag.plugins.json"},
			wantConfig: "flag.plugins.json",
			wantLock:   "env.plugins.lock.json",
		},
		{
			name:       "doctor inherits env lock",
			command:    commandPluginsDoctor,
			envConfig:  "env.plugins.json",
			envLock:    "env.plugins.lock.json",
			args:       nil,
			wantConfig: "env.plugins.json",
			wantLock:   "env.plugins.lock.json",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if test.envConfig != "" {
				t.Setenv(envPluginsConfig, test.envConfig)
			}
			if test.envLock != "" {
				t.Setenv(envPluginsLock, test.envLock)
			}

			app := newApplication(io.Discard, io.Discard)
			options, ok := app.parsePluginCommandOptions(test.command, test.args)
			if !ok {
				t.Fatal("parse plugin command options returned false")
			}
			if got, want := options.pluginsConfig, test.wantConfig; got != want {
				t.Fatalf("plugins config mismatch: got %q want %q", got, want)
			}
			if got, want := options.pluginsLock, test.wantLock; got != want {
				t.Fatalf("plugins lock mismatch: got %q want %q", got, want)
			}
		})
	}
}

func TestSharedPluginDefaultResolutionHelpFreezesPrecedenceOrder(t *testing.T) {
	got := sharedPluginDefaultResolutionHelp()
	want := []string{
		defaultResolutionFlagsOverrideEnv,
		defaultResolutionEnvOverridesBuiltIns,
		defaultResolutionPluginBuiltIns,
		defaultResolutionNoConfigFile,
	}

	if len(got) != len(want) {
		t.Fatalf("defaults length mismatch: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("default resolution mismatch at %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestCommandSpecificPluginResolutionHelpFreezesPrecedenceOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  []string
		want []string
	}{
		{
			name: "doctor",
			got:  doctorDefaultResolutionHelp(),
			want: []string{
				defaultResolutionFlagsOverrideEnv,
				defaultResolutionEnvSatisfyPluginFiles,
				defaultResolutionPluginFilesSkipped,
				defaultResolutionNoConfigFile,
			},
		},
		{
			name: "environment topic",
			got:  environmentTopicDefaultResolutionHelp(),
			want: []string{
				defaultResolutionFlagsOverrideEnv,
				defaultResolutionEnvSatisfyPluginFiles,
				defaultResolutionPluginFilesByCommand,
				defaultResolutionPluginCommandsNeedFiles,
				defaultResolutionNoConfigFile,
			},
		},
		{
			name: "plugins family",
			got:  pluginFamilyDefaultResolutionHelp(),
			want: []string{
				defaultResolutionFlagsOverrideEnv,
				defaultResolutionEnvSatisfyPluginFiles,
				defaultResolutionPluginCommandsNeedFiles,
				defaultResolutionNoConfigFile,
			},
		},
		{
			name: "plugins inspect",
			got:  pluginInspectDefaultResolutionHelp(),
			want: []string{
				defaultResolutionFlagsOverrideEnv,
				defaultResolutionEnvSatisfyPluginFiles,
				defaultResolutionPluginRegistryRequired,
				defaultResolutionPluginLockOptional,
				defaultResolutionNoConfigFile,
			},
		},
		{
			name: "plugins lock",
			got:  pluginLockDefaultResolutionHelp(),
			want: []string{
				defaultResolutionFlagsOverrideEnv,
				defaultResolutionEnvSatisfyPluginFiles,
				defaultResolutionPluginRegistryRequired,
				defaultResolutionPluginLockRequired,
				defaultResolutionNoConfigFile,
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if len(test.got) != len(test.want) {
				t.Fatalf("resolution length mismatch: got %d want %d", len(test.got), len(test.want))
			}
			for i := range test.want {
				if test.got[i] != test.want[i] {
					t.Fatalf("resolution mismatch at %d: got %q want %q", i, test.got[i], test.want[i])
				}
			}
		})
	}
}

func TestOutputControlHelpFreezesEnvironmentAndResolutionContract(t *testing.T) {
	t.Parallel()

	entries := sharedOutputEnvironmentHelp()
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	for _, want := range []string{
		envTheaterColor,
		envNoColor,
		envCLIColor,
		envCLIColorForce,
	} {
		found := false
		for _, name := range names {
			if name == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("output environment help missing %q: %#v", want, names)
		}
	}

	got := outputDefaultResolutionHelp()
	want := []string{
		defaultResolutionColorExplicit,
		defaultResolutionColorFallback,
		defaultResolutionColorFallbackOrder,
		defaultResolutionColorMachineOutput,
		defaultResolutionColorAuto,
		defaultResolutionDumbTerminal,
	}
	if len(got) != len(want) {
		t.Fatalf("defaults length mismatch: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("default resolution mismatch at %d: got %q want %q", i, got[i], want[i])
		}
	}
}
