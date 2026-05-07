package theatercli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/builtin"
	authoringthtr "github.com/alex-poliushkin/theater/internal/authoring/thtr"
	"github.com/alex-poliushkin/theater/internal/runobserve"
)

const (
	commandComplete   = "__complete"
	commandCompletion = "completion"
	commandDoctor     = "doctor"
	commandExplain    = "explain"
	commandFmt        = "fmt"
	commandHelp       = "help"
	commandInit       = "init"
	commandList       = "list"
	commandLower      = "lower"
	commandMigrate    = "migrate"
	commandPlugins    = "plugins"
	commandRun        = "run"
	commandValidate   = "validate"
	commandVersion    = "version"

	exitCodeCommandError = 2
)

func Run(args []string, stdout, stderr io.Writer) int {
	return run(args, stdout, stderr)
}

type application struct {
	stdin             io.Reader
	stdout            io.Writer
	stderr            io.Writer
	commands          commandCatalog
	outputControl     outputControl
	isTerminal        func(io.Writer) bool
	isInputTerminal   func(io.Reader) bool
	services          *builtinServices
	debugPathRenderer debugPathRenderer
	runRenderer       runDocumentRenderer
	validateRenderer  validationRenderer
}

type builtinServices struct {
	catalog       *theater.Catalog
	matcherSugar  *theater.MatcherCatalog
	matchers      theater.MatcherResolver
	pluginCatalog *theater.PluginCatalog
	runner        *theater.Runner
	validator     *theater.Validator
}

type commandOptions struct {
	globalOptions
	file            string
	mapPath         string
	write           bool
	check           bool
	diff            bool
	pluginExporters []string
	format          outputFormat
	live            liveMode
	debugMode       theater.DebugMode
	debugBreaks     []string
	debugBreakFiles []string
	debugPaths      bool
	debugStep       bool
	debugDumpPath   string
	stopOnFailure   bool
}

func run(args []string, stdout, stderr io.Writer) int {
	return newApplication(stdout, stderr).Run(args)
}

func newApplication(stdout, stderr io.Writer) *application {
	return &application{
		stdin:             os.Stdin,
		stdout:            stdout,
		stderr:            stderr,
		commands:          newCommandCatalog(),
		outputControl:     resolveOutputControl(os.LookupEnv),
		isTerminal:        isTerminalWriter,
		isInputTerminal:   isTerminalReader,
		debugPathRenderer: newDebugPathRenderer(stdout, stderr),
		runRenderer:       newRunDocumentRenderer(stdout, stderr),
		validateRenderer:  newValidationRenderer(stdout, stderr),
	}
}

func (a *application) Run(args []string) int {
	if len(args) == 0 {
		a.printUsage()
		return exitCodeCommandError
	}
	if isHelpFlag(args[0]) {
		if len(args) != 1 {
			fmt.Fprintln(a.stderr, "root help flags do not accept positional arguments; use \"theater help <command>\"")
			return exitCodeCommandError
		}
		a.printExplicitHelp(a.stdout, nil)
		return 0
	}
	if isVersionFlag(args[0]) {
		return a.versionCommand(args[1:])
	}

	command := a.commands.Must().subcommand(args[0])
	if command == nil {
		fmt.Fprintf(a.stderr, "unknown subcommand %q\n", args[0])
		a.printUsage()
		return exitCodeCommandError
	}
	if helpTarget, ok := a.commandHelpRequest(args); ok {
		a.printExplicitHelp(a.stdout, helpTarget)
		return 0
	}

	return a.runCommand(command, args[1:])
}

func (a *application) runCommand(command *commandSpec, args []string) int {
	switch command.Name {
	case commandComplete:
		return a.completeCommand(args)
	case commandCompletion:
		return a.completionCommand(args)
	case commandExplain:
		return a.explainCommand(args)
	case commandDoctor:
		return a.doctorCommand(args)
	case commandInit:
		return a.initCommand(args)
	case commandList:
		return a.runListCommand(args)
	case commandFmt:
		return a.formatStage(args)
	case commandHelp:
		return a.helpCommand(args)
	case commandLower:
		return a.lowerStage(args)
	case commandMigrate:
		return a.runMigrateCommand(args)
	case commandVersion:
		return a.versionCommand(args)
	case commandRun:
		return a.runStage(args)
	case commandValidate:
		return a.validateStage(args)
	case commandPlugins:
		return a.runPluginsCommand(args)
	default:
		fmt.Fprintf(a.stderr, "unknown subcommand %q\n", command.Name)
		a.printUsage()
		return exitCodeCommandError
	}
}

func (a *application) formatStage(args []string) int {
	options, ok := a.parseFormatOptions(args)
	if !ok {
		return exitCodeCommandError
	}

	if strings.ToLower(filepath.Ext(options.file)) != thtrFileExtension {
		fmt.Fprintf(a.stderr, "fmt requires a .thtr file\n")
		return exitCodeCommandError
	}

	formatted, err := authoringthtr.FormatFile(options.file)
	if err != nil {
		var diagnosticError *authoringthtr.DiagnosticError
		if errors.As(err, &diagnosticError) {
			return a.validateRenderer.Render(outputFormatText, options.file, []theater.Diagnostic{diagnosticError.Diagnostic()})
		}
		fmt.Fprintf(a.stderr, "format spec: %v\n", err)
		return exitCodeCommandError
	}

	if options.write {
		if options.check || options.diff {
			fmt.Fprintln(a.stderr, "fmt --write cannot be combined with --check or --diff")
			return exitCodeCommandError
		}
		if err := rewriteFileAtomically(options.file, formatted); err != nil {
			fmt.Fprintf(a.stderr, "write formatted file: %v\n", err)
			return exitCodeCommandError
		}
		return 0
	}

	if options.check || options.diff {
		return a.checkFormattedSource(options, formatted)
	}

	if _, err := a.stdout.Write(formatted); err != nil {
		fmt.Fprintf(a.stderr, "write formatted source: %v\n", err)
		return exitCodeCommandError
	}
	return 0
}

func (a *application) runStage(args []string) int {
	options, ok := a.parseCommandOptions("run", args)
	if !ok {
		return exitCodeCommandError
	}

	loaded, services, ok := a.loadStage(options)
	if !ok {
		return exitCodeCommandError
	}
	if len(loaded.AuthoringDiagnostics) != 0 {
		return a.runRenderer.Render(options.format, options.file, newAuthoringFailureRunDocument(options.file, loaded.AuthoringDiagnostics))
	}

	ctx := context.Background()
	var live *liveSession
	runOptions := theater.RunOptions{}
	debugOptions, debugEnabled, err := a.buildDebugOptions(options)
	if err != nil {
		fmt.Fprintf(a.stderr, "%v\n", err)
		return exitCodeCommandError
	}
	if debugEnabled {
		runOptions.Debug = debugOptions
	}

	debugInteractive := debugOptions != nil && debugOptions.Mode == theater.DebugModeInteractive
	if debugOptions != nil || shouldEnableLive(options.format, options.live) {
		bus := runobserve.NewBus(newRunID(loaded.Spec.ID), 0)
		runOptions.Live = bus
		if shouldEnableLive(options.format, options.live) && !debugInteractive {
			live = newLiveSessionWithTerminal(
				a.stderr,
				len(loaded.Spec.ScenarioCalls),
				bus.Subscribe(),
				a.terminalPresentationEnabled(a.stderr),
			)
		}
	}
	if len(options.pluginExporters) != 0 {
		runOptions.ReportExporters = make([]theater.ReportExportSpec, 0, len(options.pluginExporters))
		for i := range options.pluginExporters {
			runOptions.ReportExporters = append(runOptions.ReportExporters, theater.ReportExportSpec{
				Ref: options.pluginExporters[i],
			})
		}
	}

	stopLive := func() {
		if live != nil {
			live.Stop()
			live = nil
		}
	}

	result, err := services.runner.Run(ctx, loaded.Spec, runOptions)
	stopLive()
	if err != nil {
		fmt.Fprintf(a.stderr, "run stage: %v\n", err)
		return exitCodeCommandError
	}
	result.Diagnostics = loaded.RewriteDiagnostics(result.Diagnostics)

	return a.runRenderer.Render(options.format, options.file, result.Document())
}

func (a *application) lowerStage(args []string) int {
	options, ok := a.parseLowerOptions(args)
	if !ok {
		return exitCodeCommandError
	}

	if strings.ToLower(filepath.Ext(options.file)) != thtrFileExtension {
		fmt.Fprintf(a.stderr, "lower requires a .thtr file\n")
		return exitCodeCommandError
	}

	result, err := authoringthtr.LoadFileDetailed(options.file, nil)
	if err != nil {
		var diagnosticError *authoringthtr.DiagnosticError
		if errors.As(err, &diagnosticError) {
			return a.validateRenderer.Render(outputFormatText, options.file, []theater.Diagnostic{diagnosticError.Diagnostic()})
		}
		fmt.Fprintf(a.stderr, "lower spec: %v\n", err)
		return exitCodeCommandError
	}

	if _, err := a.stdout.Write(result.CanonicalYAML()); err != nil {
		fmt.Fprintf(a.stderr, "write yaml: %v\n", err)
		return exitCodeCommandError
	}
	if options.mapPath == "" {
		return 0
	}

	data, err := result.MarshalSourceMap()
	if err != nil {
		fmt.Fprintf(a.stderr, "encode source map: %v\n", err)
		return exitCodeCommandError
	}
	if err := os.WriteFile(options.mapPath, data, 0o600); err != nil {
		fmt.Fprintf(a.stderr, "write source map: %v\n", err)
		return exitCodeCommandError
	}

	return 0
}

func (a *application) validateStage(args []string) int {
	options, ok := a.parseCommandOptions("validate", args)
	if !ok {
		return exitCodeCommandError
	}

	loaded, services, ok := a.loadStage(options)
	if !ok {
		return exitCodeCommandError
	}
	if len(loaded.AuthoringDiagnostics) != 0 {
		return a.validateRenderer.Render(options.format, options.file, loaded.AuthoringDiagnostics)
	}
	if options.debugPaths {
		listing, err := services.validator.ListDebugPaths(loaded.Spec)
		if err != nil {
			fmt.Fprintf(a.stderr, "discover debug paths: %v\n", err)
			return exitCodeCommandError
		}

		diagnostics := loaded.RewriteDiagnostics(listing.Diagnostics)
		if len(diagnostics) != 0 {
			return a.validateRenderer.Render(options.format, options.file, diagnostics)
		}

		return a.debugPathRenderer.Render(options.format, options.file, listing.Paths)
	}

	diagnostics := loaded.RewriteDiagnostics(services.validator.Validate(loaded.Spec))
	diagnostics = append(diagnostics, loaded.ValidationHints...)
	return a.validateRenderer.Render(options.format, options.file, diagnostics)
}

func (a *application) parseCommandOptions(command string, args []string) (commandOptions, bool) {
	normalizedArgs, usedPositionalStagePath, err := normalizeStageFileArgs(command, args)
	if err != nil {
		fmt.Fprintf(a.stderr, "%v\n", err)
		return commandOptions{}, false
	}

	flags, options, values := a.newStageCommandFlagSet(command)

	if err := flags.Parse(normalizedArgs); err != nil {
		return commandOptions{}, false
	}
	options.globalOptions = sharedGlobalOptionContract.Resolve(options.globalOptions)
	if usedPositionalStagePath && flags.NArg() != 0 {
		fmt.Fprintf(a.stderr, "%s accepts exactly one stage file path\n", command)
		return commandOptions{}, false
	}
	if options.file == "" {
		fmt.Fprintf(a.stderr, "%s requires a stage file path via positional argument or --file\n", command)
		return commandOptions{}, false
	}
	if options.pluginsConfig != "" && options.pluginsLock == "" {
		fmt.Fprintf(a.stderr, "%s requires --plugins-lock when --plugins-config is set\n", command)
		return commandOptions{}, false
	}
	if options.pluginsConfig == "" && options.pluginsLock != "" {
		fmt.Fprintf(a.stderr, "%s requires --plugins-config when --plugins-lock is set\n", command)
		return commandOptions{}, false
	}

	parsedFormat, err := a.parseOutputFormat(command, values.format)
	if err != nil {
		fmt.Fprintf(a.stderr, "%v\n", err)
		return commandOptions{}, false
	}

	options.format = parsedFormat
	if command == commandRun {
		parsedLive, err := parseLiveMode(values.live)
		if err != nil {
			fmt.Fprintf(a.stderr, "%v\n", err)
			return commandOptions{}, false
		}

		options.live = parsedLive
		parsedDebugMode, err := parseDebugMode(values.debugMode)
		if err != nil {
			fmt.Fprintf(a.stderr, "%v\n", err)
			return commandOptions{}, false
		}

		options.debugMode = parsedDebugMode
	}
	return *options, true
}

func (a *application) parseLowerOptions(args []string) (commandOptions, bool) {
	normalizedArgs, usedPositionalStagePath, err := normalizeStageFileArgs(commandLower, args)
	if err != nil {
		fmt.Fprintf(a.stderr, "%v\n", err)
		return commandOptions{}, false
	}

	flags, options := a.newLowerCommandFlagSet()
	if err := flags.Parse(normalizedArgs); err != nil {
		return commandOptions{}, false
	}
	if usedPositionalStagePath && flags.NArg() != 0 {
		fmt.Fprintln(a.stderr, "lower accepts exactly one .thtr file path")
		return commandOptions{}, false
	}
	if options.file == "" {
		fmt.Fprintln(a.stderr, "lower requires a .thtr file path via positional argument or --file")
		return commandOptions{}, false
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(a.stderr, "lower accepts exactly one .thtr file path")
		return commandOptions{}, false
	}

	return *options, true
}

func (a *application) parseFormatOptions(args []string) (commandOptions, bool) {
	normalizedArgs, usedPositionalStagePath, err := normalizeStageFileArgs(commandFmt, args)
	if err != nil {
		fmt.Fprintf(a.stderr, "%v\n", err)
		return commandOptions{}, false
	}

	flags, options := a.newFormatCommandFlagSet()
	if err := flags.Parse(normalizedArgs); err != nil {
		return commandOptions{}, false
	}
	if usedPositionalStagePath && flags.NArg() != 0 {
		fmt.Fprintln(a.stderr, "fmt accepts exactly one .thtr file path")
		return commandOptions{}, false
	}
	if options.file == "" {
		fmt.Fprintln(a.stderr, "fmt requires a .thtr file path via positional argument or --file")
		return commandOptions{}, false
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(a.stderr, "fmt accepts exactly one .thtr file path")
		return commandOptions{}, false
	}

	return *options, true
}

func (a *application) loadStage(options commandOptions) (stageLoadResult, *builtinServices, bool) {
	services, err := a.ensureServices(options.pluginsConfig, options.pluginsLock)
	if err != nil {
		fmt.Fprintf(a.stderr, "build built-in catalogs: %v\n", err)
		return stageLoadResult{}, nil, false
	}

	loader := newStageFileLoader(services.matcherSugar)
	loaded, err := loader.Load(options.file)
	if err != nil {
		var diagnosticError *authoringthtr.DiagnosticError
		if errors.As(err, &diagnosticError) {
			return stageLoadResult{
				AuthoringDiagnostics: []theater.Diagnostic{diagnosticError.Diagnostic()},
			}, services, true
		}
		fmt.Fprintf(a.stderr, "load spec: %v\n", err)
		return stageLoadResult{}, nil, false
	}

	return loaded, services, true
}

func (a *application) ensureServices(pluginsConfig, pluginsLock string) (*builtinServices, error) {
	if pluginsConfig == "" && pluginsLock == "" && a.services != nil {
		return a.services, nil
	}

	bundle, err := builtin.NewBundle()
	if err != nil {
		return nil, err
	}
	catalog := bundle.Catalog
	matchers := bundle.Matchers

	services := &builtinServices{
		catalog:      catalog,
		matcherSugar: matchers,
		matchers:     matchers,
		runner:       theater.NewRunner(catalog, matchers),
		validator:    theater.NewValidator(catalog, matchers),
	}
	if pluginsConfig != "" {
		pluginCatalog, err := theater.LoadPluginCatalog(catalog, matchers, pluginsConfig, pluginsLock)
		if err != nil {
			return nil, err
		}
		services.pluginCatalog = pluginCatalog
		services.matchers = pluginCatalog
		services.runner = theater.NewRunner(pluginCatalog, pluginCatalog)
		services.validator = theater.NewValidator(pluginCatalog, pluginCatalog)
	}

	if pluginsConfig == "" && pluginsLock == "" {
		a.services = services
	}

	return services, nil
}

func (a *application) printUsage() {
	a.commands.PrintCommand(a.stderr, a.commands.Must(), nil, a.textStyler(a.stderr))
}

func (a *application) printExplicitHelp(writer io.Writer, spec *commandSpec) {
	if spec == nil {
		spec = a.commands.Must()
	}
	a.commands.PrintCommand(writer, spec, a.helpFlagSet(spec), a.textStyler(writer))
}

func (a *application) commandHelpRequest(args []string) (*commandSpec, bool) {
	if len(args) < 2 || args[0] == commandHelp {
		return nil, false
	}

	helpIndex := -1
	for i := 1; i < len(args); i++ {
		if isHelpFlag(args[i]) {
			helpIndex = i
			break
		}
	}
	if helpIndex == -1 {
		return nil, false
	}

	target := args[:helpIndex]
	spec, ok := a.commands.LookupHelpTarget(target...)
	if !ok || (spec.Hidden && !spec.HelpTopic) {
		return a.commandHelpFallback(target)
	}
	return spec, true
}

func (a *application) commandHelpFallback(target []string) (*commandSpec, bool) {
	if len(target) == 0 {
		return nil, false
	}

	spec := a.commands.root.subcommand(target[0])
	if spec == nil || spec.Hidden || len(spec.Subcommands) != 0 {
		return nil, false
	}
	return spec, true
}

func (a *application) newFlagSet(spec *commandSpec) *flag.FlagSet {
	flags := flag.NewFlagSet(spec.Name, flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	flags.Usage = func() {
		a.commands.PrintCommand(a.stderr, spec, flags, a.textStyler(a.stderr))
	}
	return flags
}

func (a *application) checkFormattedSource(options commandOptions, formatted []byte) int {
	original, err := os.ReadFile(options.file)
	if err != nil {
		fmt.Fprintf(a.stderr, "read source file: %v\n", err)
		return exitCodeCommandError
	}
	if bytes.Equal(original, formatted) {
		return 0
	}
	if options.diff {
		return a.writeFormatDiff(options, original, formatted)
	}

	fmt.Fprintf(a.stderr, "%s is not formatted; run theater fmt --write %s\n", options.file, options.file)
	return 1
}

func (a *application) writeFormatDiff(options commandOptions, original, formatted []byte) int {
	if err := writeFormatDiff(a.stdout, options.file, original, formatted); err != nil {
		fmt.Fprintf(a.stderr, "write format diff: %v\n", err)
		return exitCodeCommandError
	}
	return 1
}

func (a *application) terminalPresentationEnabled(writer io.Writer) bool {
	return a.outputControl.terminalPresentationEnabled(writer, a.isTerminal)
}

func (a *application) textStyler(writer io.Writer) cliTextStyler {
	return a.outputControl.styler(writer, a.isTerminal)
}

func isVersionFlag(raw string) bool {
	switch raw {
	case "-version", "--version":
		return true
	default:
		return false
	}
}

func (a *application) parseOutputFormat(command, raw string) (outputFormat, error) {
	switch command {
	case commandRun:
		return parseOutputFormat(raw)
	case commandValidate:
		return parseValidationOutputFormat(raw)
	default:
		return "", fmt.Errorf("unsupported command %q", command)
	}
}

func (a *application) buildDebugOptions(options commandOptions) (*theater.DebugOptions, bool, error) {
	if options.debugMode == theater.DebugModeOff {
		if options.debugStep ||
			options.debugDumpPath != "" ||
			options.stopOnFailure ||
			len(options.debugBreaks) != 0 ||
			len(options.debugBreakFiles) != 0 {
			return nil, false, errors.New("debug flags require --debug dump or --debug interactive")
		}

		return nil, false, nil
	}
	if options.debugStep && options.debugMode != theater.DebugModeInteractive {
		return nil, false, errors.New("--step requires --debug interactive")
	}
	if options.debugMode == theater.DebugModeDump && options.debugDumpPath == "" {
		return nil, false, errors.New("--debug dump requires --debug-dump <path>")
	}
	if options.debugMode == theater.DebugModeInteractive && (!a.isInputTerminal(a.stdin) || !a.isTerminal(a.stderr)) {
		return nil, false, errors.New("interactive debug requires a TTY on stdin and stderr; use --debug dump instead")
	}

	breakpoints := make([]string, 0, len(options.debugBreaks)+len(options.debugBreakFiles)+1)
	breakpoints = append(breakpoints, options.debugBreaks...)
	fileBreakpoints, err := loadDebugBreakpointFiles(options.debugBreakFiles)
	if err != nil {
		return nil, false, err
	}
	breakpoints = append(breakpoints, fileBreakpoints...)
	if options.stopOnFailure {
		breakpoints = append(breakpoints, "name=stop-on-failure,when=terminal-failure")
	}

	debugOptions := &theater.DebugOptions{
		Mode:        options.debugMode,
		StartPaused: options.debugStep,
		Breakpoints: breakpoints,
		DumpPath:    options.debugDumpPath,
	}
	if options.debugMode == theater.DebugModeInteractive {
		debugOptions.Input = a.stdin
		debugOptions.Output = a.stderr
	}

	return debugOptions, true, nil
}

func newRunID(stageID string) string {
	return fmt.Sprintf("%s/%d", stageID, time.Now().UTC().UnixNano())
}

func parseDebugMode(raw string) (theater.DebugMode, error) {
	switch theater.DebugMode(raw) {
	case theater.DebugModeOff, theater.DebugModeDump, theater.DebugModeInteractive:
		return theater.DebugMode(raw), nil
	default:
		return "", fmt.Errorf("unsupported debug mode %q (supported: off, dump, interactive)", raw)
	}
}

func writeFormatDiff(writer io.Writer, path string, original, formatted []byte) error {
	originalLines := splitFormatDiffLines(original)
	formattedLines := splitFormatDiffLines(formatted)
	prefix := commonFormatDiffPrefix(originalLines, formattedLines)
	suffix := commonFormatDiffSuffix(originalLines, formattedLines, prefix)
	removedCount := len(originalLines) - prefix - suffix
	addedCount := len(formattedLines) - prefix - suffix
	removedRange := formatUnifiedDiffRange(prefix, removedCount)
	addedRange := formatUnifiedDiffRange(prefix, addedCount)

	if _, err := fmt.Fprintf(writer, "--- %s\n+++ %s (formatted)\n@@ -%s +%s @@\n", path, path, removedRange, addedRange); err != nil {
		return err
	}
	for _, line := range originalLines[prefix : len(originalLines)-suffix] {
		if _, err := fmt.Fprintf(writer, "-%s\n", line); err != nil {
			return err
		}
	}
	for _, line := range formattedLines[prefix : len(formattedLines)-suffix] {
		if _, err := fmt.Fprintf(writer, "+%s\n", line); err != nil {
			return err
		}
	}
	return nil
}

func formatUnifiedDiffRange(prefix, count int) string {
	start := prefix + 1
	if count == 0 {
		start = prefix
	}
	return fmt.Sprintf("%d,%d", start, count)
}

func splitFormatDiffLines(data []byte) []string {
	text := strings.TrimRight(string(data), "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func commonFormatDiffPrefix(left, right []string) int {
	limit := min(len(left), len(right))
	for i := 0; i < limit; i++ {
		if left[i] != right[i] {
			return i
		}
	}
	return limit
}

func commonFormatDiffSuffix(left, right []string, prefix int) int {
	limit := min(len(left), len(right)) - prefix
	for i := 0; i < limit; i++ {
		if left[len(left)-1-i] != right[len(right)-1-i] {
			return i
		}
	}
	return limit
}

func newAuthoringFailureRunDocument(file string, diagnostics []theater.Diagnostic) theater.RunDocument {
	stageID := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	if stageID == "" {
		stageID = "unknown"
	}

	return theater.RunDocument{
		SchemaVersion: theater.RunDocumentSchemaVersion,
		Diagnostics:   diagnostics,
		Report: theater.Report{
			StageID:   stageID,
			StagePath: file,
			Status:    theater.StatusFailed,
			Failure: &theater.Failure{
				Kind:    theater.FailureKindDefinition,
				Phase:   theater.PhaseValidate,
				At:      file,
				Summary: fmt.Sprintf("authoring failed with %d diagnostic(s)", len(diagnostics)),
			},
		},
	}
}

func rewriteFileAtomically(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	tempFile, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()

	if err := tempFile.Chmod(info.Mode().Perm()); err != nil {
		_ = tempFile.Close()
		return err
	}
	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}

	cleanup = false
	return nil
}
