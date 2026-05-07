package theatercli

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/alex-poliushkin/theater"
	theaterthtr "github.com/alex-poliushkin/theater/thtr"
	theateryaml "github.com/alex-poliushkin/theater/yaml"
)

const commandListScenarios = "scenarios"

const (
	maxScenarioLibraryFiles    = 1024
	maxScenarioLibraryFileSize = 4 * 1024 * 1024
)

const (
	scenarioCallPlaceholderFalse = "false"
	scenarioCallPlaceholderNull  = "null"
)

type scenarioLibrarySyntax string

const (
	scenarioLibrarySyntaxAll  scenarioLibrarySyntax = "all"
	scenarioLibrarySyntaxYAML scenarioLibrarySyntax = "yaml"
	scenarioLibrarySyntaxTHTR scenarioLibrarySyntax = "thtr"
)

var listIgnoredLibraryDirectoryNames = map[string]struct{}{
	"examples": {},
	"fixtures": {},
	"internal": {},
	"testdata": {},
}

type listScenariosOptions struct {
	root         string
	format       outputFormat
	syntax       scenarioLibrarySyntax
	callSkeleton bool
}

type scenarioListResult struct {
	RepoRoot    string           `json:"repo_root"`
	LibraryRoot string           `json:"library_root"`
	Scenarios   []listedScenario `json:"scenarios"`
}

type listedScenario struct {
	ID     string                `json:"id"`
	Syntax string                `json:"syntax"`
	Inputs []listedScenarioInput `json:"inputs,omitempty"`
	Call   listedScenarioCall    `json:"call"`
	Source listedScenarioSource  `json:"source"`
}

type listedScenarioInput struct {
	Name     string `json:"name"`
	Contract string `json:"contract"`
	Required bool   `json:"required,omitempty"`
}

type listedScenarioSource struct {
	File   string `json:"file"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
}

type listedScenarioCall struct {
	ID             string   `json:"id"`
	Syntax         string   `json:"syntax"`
	RequiredInputs []string `json:"required_inputs"`
	Snippet        string   `json:"snippet"`
}

func (a *application) runListCommand(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.stderr, "list requires a resource")
		a.commands.PrintCommand(a.stderr, a.commands.Must(commandList), nil, a.textStyler(a.stderr))
		return exitCodeCommandError
	}

	command := a.commands.Must(commandList).subcommand(args[0])
	if command == nil {
		fmt.Fprintf(a.stderr, "unknown list resource %q\n", args[0])
		a.commands.PrintCommand(a.stderr, a.commands.Must(commandList), nil, a.textStyler(a.stderr))
		return exitCodeCommandError
	}

	switch command.Name {
	case commandListScenarios:
		return a.listScenariosCommand(args[1:])
	default:
		fmt.Fprintf(a.stderr, "unknown list resource %q\n", args[0])
		a.commands.PrintCommand(a.stderr, a.commands.Must(commandList), nil, a.textStyler(a.stderr))
		return exitCodeCommandError
	}
}

func (a *application) listScenariosCommand(args []string) int {
	options, ok := a.parseListScenariosOptions(args)
	if !ok {
		return exitCodeCommandError
	}

	services, err := a.ensureServices("", "")
	if err != nil {
		fmt.Fprintf(a.stderr, "build built-in catalogs: %v\n", err)
		return exitCodeCommandError
	}

	result, err := discoverLibraryScenarios(options.root, services.matcherSugar, options.syntax)
	if err != nil {
		fmt.Fprintf(a.stderr, "list scenarios: %s\n", sanitizeCLIText(err.Error()))
		return exitCodeCommandError
	}

	if err := renderScenarioList(a.stdout, options.format, result, options.callSkeleton); err != nil {
		fmt.Fprintf(a.stderr, "render scenario list: %v\n", err)
		return exitCodeCommandError
	}

	return 0
}

func (a *application) parseListScenariosOptions(args []string) (listScenariosOptions, bool) {
	flags, options, values := a.newListScenariosCommandFlagSet()
	if err := flags.Parse(args); err != nil {
		return listScenariosOptions{}, false
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(a.stderr, "list scenarios does not accept positional arguments")
		return listScenariosOptions{}, false
	}
	if options.root == "" {
		workingDir, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(a.stderr, "resolve current directory: %v\n", err)
			return listScenariosOptions{}, false
		}
		options.root = workingDir
	}

	format, err := parseValidationOutputFormat(values.format)
	if err != nil {
		fmt.Fprintf(a.stderr, "%v\n", err)
		return listScenariosOptions{}, false
	}
	options.format = format
	if options.callSkeleton && options.format != outputFormatText {
		fmt.Fprintln(a.stderr, "--call-skeleton is only supported with --format text")
		return listScenariosOptions{}, false
	}

	syntax, err := parseListScenarioSyntax(values.syntax)
	if err != nil {
		fmt.Fprintf(a.stderr, "%v\n", err)
		return listScenariosOptions{}, false
	}
	options.syntax = syntax

	return *options, true
}

func discoverLibraryScenarios(
	root string,
	matchers theater.MatcherSugarResolver,
	syntax scenarioLibrarySyntax,
) (scenarioListResult, error) {
	start, err := filepath.Abs(root)
	if err != nil {
		return scenarioListResult{}, err
	}

	layout, ok := resolveRepoLayout(start)
	if !ok {
		return scenarioListResult{}, fmt.Errorf("repo-local theater roots not found from %s", start)
	}

	files, err := collectScenarioLibraryFiles(layout.LibraryRoot, syntax)
	if err != nil {
		return scenarioListResult{}, err
	}

	result := scenarioListResult{
		RepoRoot:    layout.RepoRoot,
		LibraryRoot: layout.LibraryRoot,
		Scenarios:   make([]listedScenario, 0),
	}
	seen := make(map[string]string)
	for i := range files {
		stage, err := loadScenarioLibraryStage(files[i], matchers)
		if err != nil {
			return scenarioListResult{}, err
		}
		if len(stage.ScenarioCalls) != 0 {
			return scenarioListResult{}, fmt.Errorf("library file %s must not declare scenario_calls", files[i])
		}
		fileSyntax, ok := scenarioLibraryFileSyntax(files[i])
		if !ok {
			return scenarioListResult{}, fmt.Errorf("unsupported library file extension: %s", files[i])
		}

		for j := range stage.Scenarios {
			scenario := stage.Scenarios[j]
			if !directScenarioIDPublic(scenario.ID) {
				continue
			}
			if existing, ok := seen[scenario.ID]; ok {
				return scenarioListResult{}, fmt.Errorf(
					"library scenario %q is declared in multiple files: %s, %s",
					scenario.ID,
					existing,
					files[i],
				)
			}

			seen[scenario.ID] = files[i]
			result.Scenarios = append(result.Scenarios, listedScenario{
				ID:     scenario.ID,
				Syntax: string(fileSyntax),
				Inputs: listedScenarioInputs(scenario.Inputs),
				Call:   listedScenarioCallFor(scenario.ID, scenario.Inputs, fileSyntax),
				Source: listedScenarioSourceFor(layout.RepoRoot, files[i], scenario.SourceSpan),
			})
		}
	}

	sort.Slice(result.Scenarios, func(i, j int) bool {
		return result.Scenarios[i].ID < result.Scenarios[j].ID
	})
	return result, nil
}

func collectScenarioLibraryFiles(libraryRoot string, syntax scenarioLibrarySyntax) ([]string, error) {
	files := make([]string, 0)
	err := filepath.WalkDir(libraryRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path != libraryRoot && listShouldIgnoreLibraryDirectory(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		fileSyntax, ok := scenarioLibraryFileSyntax(path)
		if !ok {
			return fmt.Errorf("unsupported library file syntax for %s (supported: .yaml, .yml, .thtr)", path)
		}
		if syntax != scenarioLibrarySyntaxAll && syntax != fileSyntax {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("library file %s must be a regular file; symlinks are not allowed", path)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("library file %s must be a regular file", path)
		}
		if info.Size() > maxScenarioLibraryFileSize {
			return fmt.Errorf("library file %s exceeds discovery size limit of %d bytes", path, maxScenarioLibraryFileSize)
		}
		files = append(files, path)
		if len(files) > maxScenarioLibraryFiles {
			return fmt.Errorf("library discovery supports at most %d scenario files", maxScenarioLibraryFiles)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(files)
	return files, nil
}

func loadScenarioLibraryStage(path string, matchers theater.MatcherSugarResolver) (theater.StageSpec, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case yamlFileExtension, ymlFileExtension:
		return theateryaml.LoadFile(path, matchers)
	case thtrFileExtension:
		return theaterthtr.LoadFile(path, matchers)
	default:
		return theater.StageSpec{}, fmt.Errorf("unsupported library file extension: %s", path)
	}
}

func renderScenarioList(writer io.Writer, format outputFormat, result scenarioListResult, callSkeleton bool) error {
	switch format {
	case outputFormatText:
		return renderScenarioListText(writer, result, callSkeleton)
	case outputFormatJSON:
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func renderScenarioListText(writer io.Writer, result scenarioListResult, callSkeleton bool) error {
	table := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "SCENARIO\tSYNTAX\tINPUTS\tSOURCE"); err != nil {
		return err
	}
	for i := range result.Scenarios {
		scenario := result.Scenarios[i]
		if _, err := fmt.Fprintf(
			table,
			"%s\t%s\t%s\t%s\n",
			sanitizeCLIText(scenario.ID),
			sanitizeCLIText(scenario.Syntax),
			sanitizeCLIText(formatListedScenarioInputs(scenario.Inputs)),
			sanitizeCLIText(formatListedScenarioSource(scenario.Source)),
		); err != nil {
			return err
		}
	}

	if err := table.Flush(); err != nil {
		return err
	}
	if callSkeleton {
		return renderScenarioCallSkeletonsText(writer, result)
	}
	return nil
}

func listedScenarioInputs(inputs map[string]theater.ValueContract) []listedScenarioInput {
	names := make([]string, 0, len(inputs))
	for name := range inputs {
		names = append(names, name)
	}
	sort.Strings(names)

	listed := make([]listedScenarioInput, 0, len(names))
	for i := range names {
		contract := inputs[names[i]]
		listed = append(listed, listedScenarioInput{
			Name:     names[i],
			Contract: formatValueContract(contract, false),
			Required: contract.Required,
		})
	}
	return listed
}

func listedScenarioCallFor(
	scenarioID string,
	inputs map[string]theater.ValueContract,
	syntax scenarioLibrarySyntax,
) listedScenarioCall {
	callID := scenarioCallSkeletonID(scenarioID)
	requiredInputs := requiredScenarioInputNames(inputs)
	return listedScenarioCall{
		ID:             callID,
		Syntax:         string(syntax),
		RequiredInputs: requiredInputs,
		Snippet:        scenarioCallSkeletonSnippet(callID, scenarioID, requiredInputs, inputs, syntax),
	}
}

func listedScenarioSourceFor(repoRoot, fallbackPath string, source *theater.SourceRef) listedScenarioSource {
	if source == nil {
		return listedScenarioSource{File: repoRelativePath(repoRoot, fallbackPath)}
	}

	file := source.File
	if file == "" {
		file = fallbackPath
	}
	return listedScenarioSource{
		File:   repoRelativePath(repoRoot, file),
		Line:   source.Line,
		Column: source.Column,
	}
}

func formatListedScenarioInputs(inputs []listedScenarioInput) string {
	if len(inputs) == 0 {
		return "-"
	}

	parts := make([]string, 0, len(inputs))
	for i := range inputs {
		parts = append(parts, sanitizeCLIText(inputs[i].Name)+":"+sanitizeCLIText(inputs[i].Contract))
	}
	return strings.Join(parts, ", ")
}

func formatListedScenarioSource(source listedScenarioSource) string {
	if source.Line == 0 {
		return source.File
	}
	if source.Column == 0 {
		return fmt.Sprintf("%s:%d", source.File, source.Line)
	}
	return fmt.Sprintf("%s:%d:%d", source.File, source.Line, source.Column)
}

func renderScenarioCallSkeletonsText(writer io.Writer, result scenarioListResult) error {
	if _, err := fmt.Fprintln(writer); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(writer, "Call skeletons:"); err != nil {
		return err
	}
	for i := range result.Scenarios {
		scenario := result.Scenarios[i]
		if _, err := fmt.Fprintf(
			writer,
			"%s (%s):\n%s\n",
			sanitizeCLIText(scenario.ID),
			sanitizeCLIText(scenario.Call.Syntax),
			sanitizeCLIMultilineText(scenario.Call.Snippet),
		); err != nil {
			return err
		}
		if i < len(result.Scenarios)-1 {
			if _, err := fmt.Fprintln(writer); err != nil {
				return err
			}
		}
	}
	return nil
}

func sanitizeCLIMultilineText(raw string) string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	for i := range lines {
		lines[i] = sanitizeCLIText(lines[i])
	}
	return strings.Join(lines, "\n")
}

func scenarioCallSkeletonSnippet(
	callID string,
	scenarioID string,
	requiredInputs []string,
	inputs map[string]theater.ValueContract,
	syntax scenarioLibrarySyntax,
) string {
	switch syntax {
	case scenarioLibrarySyntaxTHTR:
		return thtrScenarioCallSkeleton(callID, scenarioID, requiredInputs, inputs)
	default:
		return yamlScenarioCallSkeleton(callID, scenarioID, requiredInputs, inputs)
	}
}

func yamlScenarioCallSkeleton(
	callID string,
	scenarioID string,
	requiredInputs []string,
	inputs map[string]theater.ValueContract,
) string {
	var builder strings.Builder
	fmt.Fprintln(&builder, "scenario_calls:")
	fmt.Fprintf(&builder, "  - id: %s\n", callID)
	fmt.Fprintf(&builder, "    scenario_id: %s\n", yamlScenarioCallString(scenarioID))
	if len(requiredInputs) > 0 {
		fmt.Fprintln(&builder, "    bindings:")
		for i := range requiredInputs {
			name := requiredInputs[i]
			fmt.Fprintf(&builder, "      %s: %s\n", yamlScenarioCallString(name), yamlScenarioCallPlaceholder(inputs[name]))
		}
	}
	return strings.TrimRight(builder.String(), "\n")
}

func yamlScenarioCallString(value string) string {
	return strconv.Quote(sanitizeCLIText(value))
}

func thtrScenarioCallSkeleton(
	callID string,
	scenarioID string,
	requiredInputs []string,
	inputs map[string]theater.ValueContract,
) string {
	bindings := make([]string, 0, len(requiredInputs))
	for i := range requiredInputs {
		name := requiredInputs[i]
		bindings = append(bindings, fmt.Sprintf("%s: %s", name, thtrScenarioCallPlaceholder(inputs[name])))
	}
	return fmt.Sprintf("call %s = %s(%s)", callID, scenarioID, strings.Join(bindings, ", "))
}

func scenarioCallSkeletonID(scenarioID string) string {
	var builder strings.Builder
	builder.WriteString("run")
	lastWasSeparator := false
	for _, r := range strings.ToLower(scenarioID) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if !lastWasSeparator {
				builder.WriteRune('-')
			}
			builder.WriteRune(r)
			lastWasSeparator = true
		default:
			lastWasSeparator = false
		}
	}
	return strings.TrimRight(builder.String(), "-") + "-x" + hex.EncodeToString([]byte(scenarioID))
}

func requiredScenarioInputNames(inputs map[string]theater.ValueContract) []string {
	names := make([]string, 0, len(inputs))
	for name, contract := range inputs {
		if contract.Required {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func yamlScenarioCallPlaceholder(contract theater.ValueContract) string {
	switch preferredScenarioCallPlaceholderKind(contract) {
	case theater.ValueKindNumber:
		return "0"
	case theater.ValueKindBool:
		return scenarioCallPlaceholderFalse
	case theater.ValueKindObject:
		return "{}"
	case theater.ValueKindList:
		return "[]"
	case theater.ValueKindNull:
		return scenarioCallPlaceholderNull
	default:
		return "TODO-string"
	}
}

func thtrScenarioCallPlaceholder(contract theater.ValueContract) string {
	switch preferredScenarioCallPlaceholderKind(contract) {
	case theater.ValueKindNumber:
		return "0"
	case theater.ValueKindBool:
		return scenarioCallPlaceholderFalse
	case theater.ValueKindObject:
		return "object {}"
	case theater.ValueKindList:
		return "list []"
	case theater.ValueKindNull:
		return scenarioCallPlaceholderNull
	default:
		return `"TODO-string"`
	}
}

func preferredScenarioCallPlaceholderKind(contract theater.ValueContract) theater.ValueKind {
	for _, kind := range []theater.ValueKind{
		theater.ValueKindString,
		theater.ValueKindNumber,
		theater.ValueKindBool,
		theater.ValueKindObject,
		theater.ValueKindList,
		theater.ValueKindNull,
		theater.ValueKindBytes,
		theater.ValueKindAny,
	} {
		if contract.Supports(kind) {
			return kind
		}
	}
	return theater.ValueKindString
}

func repoRelativePath(repoRoot, path string) string {
	relative, err := filepath.Rel(repoRoot, path)
	if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}

func parseListScenarioSyntax(value string) (scenarioLibrarySyntax, error) {
	switch scenarioLibrarySyntax(strings.ToLower(value)) {
	case scenarioLibrarySyntaxAll:
		return scenarioLibrarySyntaxAll, nil
	case scenarioLibrarySyntaxYAML:
		return scenarioLibrarySyntaxYAML, nil
	case scenarioLibrarySyntaxTHTR:
		return scenarioLibrarySyntaxTHTR, nil
	default:
		return "", fmt.Errorf("unsupported scenario syntax %q (supported: all, yaml, thtr)", value)
	}
}

func scenarioLibraryFileSyntax(path string) (scenarioLibrarySyntax, bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case yamlFileExtension, ymlFileExtension:
		return scenarioLibrarySyntaxYAML, true
	case thtrFileExtension:
		return scenarioLibrarySyntaxTHTR, true
	default:
		return "", false
	}
}

func listShouldIgnoreLibraryDirectory(name string) bool {
	_, ok := listIgnoredLibraryDirectoryNames[name]
	return ok
}

func directScenarioIDPublic(id string) bool {
	segments := strings.Split(id, "/")
	for i := range segments {
		if segments[i] != "internal" {
			continue
		}
		return i == len(segments)-1
	}
	return true
}
