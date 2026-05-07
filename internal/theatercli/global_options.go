package theatercli

import (
	"flag"
	"os"
)

const (
	defaultResolutionEnvOverridesBuiltIns  = "Environment variables override built-in defaults."
	defaultResolutionEnvSatisfyPluginFiles = "Environment variables can provide required " +
		"plugin registry file and lock file paths."
	defaultResolutionFlagsOverrideEnv = "Command flags override environment variables."
	defaultResolutionColorExplicit    = "THEATER_COLOR accepts auto, always, or never and overrides " +
		"common color environment conventions."
	defaultResolutionColorFallback = "When THEATER_COLOR is unset, NO_COLOR disables ANSI styling, " +
		"CLICOLOR_FORCE forces it, and CLICOLOR=0 disables it."
	defaultResolutionColorFallbackOrder = "When THEATER_COLOR is unset and multiple common color " +
		"environment variables are set together, precedence is NO_COLOR, then CLICOLOR_FORCE, then CLICOLOR=0."
	defaultResolutionColorMachineOutput = "JSON and JUnit never emit ANSI styling or other " +
		"human-oriented decoration."
	defaultResolutionColorAuto = "In auto mode, ANSI styling is enabled only for human-oriented " +
		"text output on capable terminals."
	defaultResolutionDumbTerminal   = "In auto mode, TERM=dumb disables terminal styling and live frame rendering."
	defaultResolutionNoConfigFile   = "The CLI does not read a mutable config file."
	defaultResolutionPluginBuiltIns = "When neither source provides a plugin registry " +
		"file or plugin lock file, theater uses built-in capabilities only."
	defaultResolutionPluginCommandsNeedFiles = "theater plugins inspect and theater " +
		"plugins doctor require a plugin registry file; theater plugins lock " +
		"requires both a plugin registry file and a plugin lock file."
	defaultResolutionPluginFilesByCommand = "Commands without a plugin registry file " +
		"or plugin lock file either use built-in capabilities only or skip " +
		"plugin-specific checks, depending on the command."
	defaultResolutionPluginFilesSkipped = "When neither source provides a plugin " +
		"registry file or plugin lock file, doctor skips plugin-specific checks."
	defaultResolutionPluginLockOptional     = "When no plugin lock file is provided, the command uses the plugin registry file only."
	defaultResolutionPluginLockRequired     = "A plugin lock file is required."
	defaultResolutionPluginRegistryRequired = "A plugin registry file is required."
	envPluginsConfig                        = "THEATER_PLUGINS_CONFIG"
	envPluginsLock                          = "THEATER_PLUGINS_LOCK"
)

type globalOptions struct {
	pluginsConfig string
	pluginsLock   string
}

type globalOptionContract struct {
	environment []environmentHelpEntry
	defaults    []string
}

var sharedGlobalOptionContract = globalOptionContract{
	environment: []environmentHelpEntry{
		{
			Name:        envPluginsConfig,
			Description: "Default plugin registry file when --plugins-config is omitted.",
		},
		{
			Name:        envPluginsLock,
			Description: "Default plugin lock file when --plugins-lock is omitted.",
		},
	},
	defaults: []string{
		defaultResolutionFlagsOverrideEnv,
		defaultResolutionEnvOverridesBuiltIns,
		defaultResolutionPluginBuiltIns,
		defaultResolutionNoConfigFile,
	},
}

func (c globalOptionContract) RegisterPluginFlags(flags *flag.FlagSet, options *globalOptions) {
	flags.StringVar(
		&options.pluginsConfig,
		"plugins-config",
		"",
		"path to plugin registry file; legacy -plugins-config remains accepted for compatibility",
	)
	flags.StringVar(
		&options.pluginsLock,
		"plugins-lock",
		"",
		"path to plugin lock file; legacy -plugins-lock remains accepted for compatibility",
	)
}

func (c globalOptionContract) Resolve(options globalOptions) globalOptions {
	return globalOptions{
		pluginsConfig: firstNonEmpty(options.pluginsConfig, os.Getenv(envPluginsConfig)),
		pluginsLock:   firstNonEmpty(options.pluginsLock, os.Getenv(envPluginsLock)),
	}
}

func (c globalOptionContract) EnvironmentHelp() []environmentHelpEntry {
	return append([]environmentHelpEntry(nil), c.environment...)
}

func (c globalOptionContract) DefaultResolutionHelp() []string {
	return append([]string(nil), c.defaults...)
}

func sharedPluginEnvironmentHelp() []environmentHelpEntry {
	return sharedGlobalOptionContract.EnvironmentHelp()
}

func sharedOutputEnvironmentHelp() []environmentHelpEntry {
	return []environmentHelpEntry{
		{
			Name:        envTheaterColor,
			Description: "Color policy for human-oriented text output: auto, always, or never.",
		},
		{
			Name:        envNoColor,
			Description: "Disable ANSI styling when THEATER_COLOR is unset.",
		},
		{
			Name:        envCLIColor,
			Description: "Set to 0 to disable ANSI styling when THEATER_COLOR is unset.",
		},
		{
			Name:        envCLIColorForce,
			Description: "Set to a non-zero value to force ANSI styling when THEATER_COLOR is unset.",
		},
	}
}

func sharedPluginDefaultResolutionHelp() []string {
	return sharedGlobalOptionContract.DefaultResolutionHelp()
}

func outputDefaultResolutionHelp() []string {
	return []string{
		defaultResolutionColorExplicit,
		defaultResolutionColorFallback,
		defaultResolutionColorFallbackOrder,
		defaultResolutionColorMachineOutput,
		defaultResolutionColorAuto,
		defaultResolutionDumbTerminal,
	}
}

func doctorDefaultResolutionHelp() []string {
	return []string{
		defaultResolutionFlagsOverrideEnv,
		defaultResolutionEnvSatisfyPluginFiles,
		defaultResolutionPluginFilesSkipped,
		defaultResolutionNoConfigFile,
	}
}

func environmentTopicDefaultResolutionHelp() []string {
	return []string{
		defaultResolutionFlagsOverrideEnv,
		defaultResolutionEnvSatisfyPluginFiles,
		defaultResolutionPluginFilesByCommand,
		defaultResolutionPluginCommandsNeedFiles,
		defaultResolutionNoConfigFile,
	}
}

func pluginFamilyDefaultResolutionHelp() []string {
	return []string{
		defaultResolutionFlagsOverrideEnv,
		defaultResolutionEnvSatisfyPluginFiles,
		defaultResolutionPluginCommandsNeedFiles,
		defaultResolutionNoConfigFile,
	}
}

func pluginInspectDefaultResolutionHelp() []string {
	return []string{
		defaultResolutionFlagsOverrideEnv,
		defaultResolutionEnvSatisfyPluginFiles,
		defaultResolutionPluginRegistryRequired,
		defaultResolutionPluginLockOptional,
		defaultResolutionNoConfigFile,
	}
}

func pluginLockDefaultResolutionHelp() []string {
	return []string{
		defaultResolutionFlagsOverrideEnv,
		defaultResolutionEnvSatisfyPluginFiles,
		defaultResolutionPluginRegistryRequired,
		defaultResolutionPluginLockRequired,
		defaultResolutionNoConfigFile,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func combineEnvironmentHelp(groups ...[]environmentHelpEntry) []environmentHelpEntry {
	combined := make([]environmentHelpEntry, 0)
	for _, group := range groups {
		combined = append(combined, group...)
	}
	return combined
}

func combineDefaultResolutionHelp(groups ...[]string) []string {
	combined := make([]string, 0)
	for _, group := range groups {
		combined = append(combined, group...)
	}
	return combined
}
