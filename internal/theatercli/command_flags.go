package theatercli

import (
	"flag"
	"strings"

	"github.com/alex-poliushkin/theater"
)

const (
	stageFileArgument = "<stage.{yaml|yml|thtr}>"
	stageFileHelpText = "path to stage .yaml, .yml, or .thtr file"
)

type stageCommandFlagValues struct {
	format          string
	live            string
	debugMode       string
	pluginReadiness string
}

type pluginCommandFlagValues struct {
	format          string
	pluginReadiness string
}

type listScenariosFlagValues struct {
	format string
	syntax string
}

type reportRenderOptions struct {
	format outputFormat
	input  string
}

type reportRenderFlagValues struct {
	format string
}

type repeatableStringFlag struct {
	values *[]string
}

func (a *application) helpFlagSet(spec *commandSpec) *flag.FlagSet {
	if spec == nil || spec.HelpTopic {
		return nil
	}

	switch spec.FlagProfile {
	case commandFlagProfileDoctor:
		flags, _ := a.newDoctorCommandFlagSet()
		return flags
	case commandFlagProfileExplain:
		flags, _ := a.newExplainCommandFlagSet()
		return flags
	case commandFlagProfileInit:
		flags, _, _ := a.newInitCommandFlagSet()
		return flags
	case commandFlagProfileListScenarios:
		flags, _, _ := a.newListScenariosCommandFlagSet()
		return flags
	case commandFlagProfileRun:
		flags, _, _ := a.newStageCommandFlagSet(commandRun)
		return flags
	case commandFlagProfileValidate:
		flags, _, _ := a.newStageCommandFlagSet(commandValidate)
		return flags
	case commandFlagProfileFmt:
		flags, _ := a.newFormatCommandFlagSet()
		return flags
	case commandFlagProfileLower:
		flags, _ := a.newLowerCommandFlagSet()
		return flags
	case commandFlagProfileMigrateYAML:
		flags, _ := a.newMigrateFromYAMLFlagSet()
		return flags
	case commandFlagProfilePluginsDigest:
		flags, _, _ := a.newPluginCommandFlagSet(commandPluginsDigest)
		return flags
	case commandFlagProfilePluginsInspect:
		flags, _, _ := a.newPluginCommandFlagSet(commandPluginsInspect)
		return flags
	case commandFlagProfilePluginsDoctor:
		flags, _, _ := a.newPluginCommandFlagSet(commandPluginsDoctor)
		return flags
	case commandFlagProfilePluginsLock:
		flags, _, _ := a.newPluginCommandFlagSet(commandPluginsLock)
		return flags
	case commandFlagProfileReportRender:
		flags, _, _ := a.newReportRenderCommandFlagSet()
		return flags
	default:
		return nil
	}
}

func (a *application) newFormatCommandFlagSet() (*flag.FlagSet, *commandOptions) {
	flags := a.newFlagSet(a.commands.Must(commandFmt))
	options := commandOptions{}
	registerFormatCommandFlags(flags, &options)
	return flags, &options
}

func (a *application) newExplainCommandFlagSet() (*flag.FlagSet, *globalOptions) {
	flags := a.newFlagSet(a.commands.Must(commandExplain))
	options := globalOptions{}
	sharedGlobalOptionContract.RegisterPluginFlags(flags, &options)
	return flags, &options
}

func (a *application) newDoctorCommandFlagSet() (*flag.FlagSet, *doctorOptions) {
	flags := a.newFlagSet(a.commands.Must(commandDoctor))
	options := doctorOptions{}
	sharedGlobalOptionContract.RegisterPluginFlags(flags, &options.globalOptions)
	flags.Var(&repeatableStringFlag{values: &options.writePaths}, "write-path", "`string` output path that must be writable")
	return flags, &options
}

func (a *application) newInitCommandFlagSet() (*flag.FlagSet, *initCommandOptions, *initCommandFlagValues) {
	flags := a.newFlagSet(a.commands.Must(commandInit))
	options := &initCommandOptions{}
	values := &initCommandFlagValues{syntax: string(initSyntaxYAML)}
	registerInitCommandFlags(flags, values)
	return flags, options, values
}

func (a *application) newListScenariosCommandFlagSet() (*flag.FlagSet, *listScenariosOptions, *listScenariosFlagValues) {
	flags := a.newFlagSet(a.commands.Must(commandList, commandListScenarios))
	options := listScenariosOptions{format: outputFormatText, syntax: scenarioLibrarySyntaxAll}
	values := &listScenariosFlagValues{
		format: string(outputFormatText),
		syntax: string(scenarioLibrarySyntaxAll),
	}
	registerListScenariosCommandFlags(flags, &options, values)
	return flags, &options, values
}

func (a *application) newLowerCommandFlagSet() (*flag.FlagSet, *commandOptions) {
	flags := a.newFlagSet(a.commands.Must(commandLower))
	options := commandOptions{}
	registerLowerCommandFlags(flags, &options)
	return flags, &options
}

func (a *application) newMigrateFromYAMLFlagSet() (*flag.FlagSet, *migrateCommandOptions) {
	flags := a.newFlagSet(a.commands.Must(commandMigrate, commandMigrateFromYAML))
	options := migrateCommandOptions{}
	registerMigrateFromYAMLFlags(flags, &options)
	return flags, &options
}

func (a *application) newReportRenderCommandFlagSet() (*flag.FlagSet, *reportRenderOptions, *reportRenderFlagValues) {
	flags := a.newFlagSet(a.commands.Must(commandReport, commandReportRender))
	options := &reportRenderOptions{format: outputFormatMarkdown}
	values := &reportRenderFlagValues{format: string(outputFormatMarkdown)}
	registerReportRenderCommandFlags(flags, options, values)
	return flags, options, values
}

func (a *application) newPluginCommandFlagSet(command string) (*flag.FlagSet, *pluginCommandOptions, *pluginCommandFlagValues) {
	profile := pluginCommandProfileFor(command)
	flags := a.newFlagSet(a.commands.Must(commandPlugins, profile.command))
	options := pluginCommandOptions{format: outputFormatText}
	values := &pluginCommandFlagValues{
		format:          string(outputFormatText),
		pluginReadiness: string(pluginReadinessRuntime),
	}
	registerPluginCommandFlags(profile, flags, &options, values)
	return flags, &options, values
}

func (a *application) newStageCommandFlagSet(command string) (*flag.FlagSet, *commandOptions, *stageCommandFlagValues) {
	flags := a.newFlagSet(a.commands.Must(command))
	options := commandOptions{
		format:          outputFormatText,
		live:            liveModeAuto,
		pluginReadiness: pluginReadinessRuntime,
	}
	values := &stageCommandFlagValues{
		format:          string(outputFormatText),
		live:            string(liveModeAuto),
		debugMode:       string(theater.DebugModeOff),
		pluginReadiness: string(pluginReadinessRuntime),
	}
	registerStageCommandFlags(command, flags, &options, values)
	return flags, &options, values
}

func registerFormatCommandFlags(flags *flag.FlagSet, options *commandOptions) {
	flags.StringVar(&options.file, "file", "", "path to .thtr stage file")
	flags.BoolVar(&options.write, "write", false, "rewrite the input file in place")
	flags.BoolVar(&options.check, "check", false, "exit non-zero when the input file is not formatted")
	flags.BoolVar(&options.diff, "diff", false, "print formatting changes without rewriting the input file")
}

func registerListScenariosCommandFlags(flags *flag.FlagSet, options *listScenariosOptions, values *listScenariosFlagValues) {
	flags.StringVar(&options.root, "root", "", "repository root or path inside a theater repository")
	flags.StringVar(&values.format, "format", values.format, "output format")
	flags.StringVar(&values.syntax, "syntax", values.syntax, "scenario syntax filter")
	flags.BoolVar(&options.callSkeleton, "call-skeleton", false, "print runnable scenario call skeletons in text output")
}

func registerLowerCommandFlags(flags *flag.FlagSet, options *commandOptions) {
	flags.StringVar(&options.file, "file", "", "path to .thtr stage file")
	flags.StringVar(&options.mapPath, "map", "", "path to write source-map JSON")
}

func registerMigrateFromYAMLFlags(flags *flag.FlagSet, options *migrateCommandOptions) {
	flags.StringVar(&options.file, "file", "", "path to .yaml or .yml stage file")
	sharedGlobalOptionContract.RegisterPluginFlags(flags, &options.globalOptions)
}

func registerInitCommandFlags(flags *flag.FlagSet, values *initCommandFlagValues) {
	flags.StringVar(&values.syntax, "syntax", values.syntax, "starter syntax")
}

func registerReportRenderCommandFlags(flags *flag.FlagSet, options *reportRenderOptions, values *reportRenderFlagValues) {
	flags.StringVar(&options.input, "input", "", "path to run JSON or - for stdin")
	flags.StringVar(&values.format, "format", values.format, "output format")
}

func registerPluginCommandFlags(
	profile pluginCommandProfile,
	flags *flag.FlagSet,
	options *pluginCommandOptions,
	values *pluginCommandFlagValues,
) {
	if profile.requireConfig || profile.requireLock {
		sharedGlobalOptionContract.RegisterPluginFlags(flags, &options.globalOptions)
	}
	if profile.requireManifest {
		flags.StringVar(&options.manifestPath, "manifest", "", "path to plugin manifest file")
	}
	if profile.allowFormat {
		flags.StringVar(&values.format, "format", values.format, "output format")
	}
	if profile.allowReadiness {
		flags.StringVar(&values.pluginReadiness, "plugins-readiness", values.pluginReadiness, "plugin readiness mode: runtime or descriptor")
	}
	if profile.allowWrite {
		flags.BoolVar(&options.write, "write", false, "rewrite descriptor_digest in the manifest file")
	}
}

func registerStageCommandFlags(command string, flags *flag.FlagSet, options *commandOptions, values *stageCommandFlagValues) {
	flags.StringVar(&options.file, "file", "", stageFileHelpText)
	sharedGlobalOptionContract.RegisterPluginFlags(flags, &options.globalOptions)
	flags.StringVar(&values.format, "format", values.format, "output format")
	if command == commandRun {
		flags.StringVar(&values.live, "live", values.live, "live output mode")
		flags.StringVar(&values.debugMode, "debug", values.debugMode, "debug mode")
		flags.StringVar(&options.runSidecars.JSON, "json-output", "", "write run JSON sidecar to path")
		flags.StringVar(&options.runSidecars.JUnit, "junit-output", "", "write JUnit sidecar to path")
		flags.StringVar(&options.runSidecars.Markdown, "markdown-output", "", "write Markdown sidecar to path")
		flags.StringVar(&options.runSidecars.Summary, "summary-output", "", "write compact Markdown summary sidecar to path")
		flags.BoolVar(&options.runSidecars.Overwrite, "overwrite", false, "replace existing sidecar output files")
		flags.Var(&repeatableStringFlag{values: &options.debugBreaks}, "break", "debug selector")
		flags.Var(&repeatableStringFlag{values: &options.debugBreakFiles}, "break-file", "path to a debug selector file")
		flags.BoolVar(&options.debugStep, "step", false, "pause at the first debuggable boundary in interactive debug mode")
		flags.StringVar(&options.debugDumpPath, "debug-dump", "", "write debug sidecar NDJSON to path")
		flags.BoolVar(&options.stopOnFailure, "stop-on-failure", false, "add a terminal failure debug selector")
		flags.Var(&repeatableStringFlag{values: &options.pluginExporters}, "plugin-exporter", "report exporter capability to invoke after run")
	}
	if command == commandValidate {
		flags.BoolVar(&options.debugPaths, "debug-paths", false, "list reusable debug selectors")
		flags.StringVar(&values.pluginReadiness, "plugins-readiness", values.pluginReadiness, "plugin readiness mode: runtime or descriptor")
	}
}

func (f *repeatableStringFlag) String() string {
	if f == nil || f.values == nil || len(*f.values) == 0 {
		return ""
	}

	return strings.Join(*f.values, ",")
}

func (f *repeatableStringFlag) Set(value string) error {
	*f.values = append(*f.values, value)
	return nil
}
