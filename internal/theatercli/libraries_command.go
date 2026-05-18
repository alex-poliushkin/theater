package theatercli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	goyaml "gopkg.in/yaml.v3"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/authoring/flowauth"
	authoringyaml "github.com/alex-poliushkin/theater/internal/authoring/yaml"
	theaterthtr "github.com/alex-poliushkin/theater/thtr"
	theateryaml "github.com/alex-poliushkin/theater/yaml"
)

const (
	commandLibrariesInspect = "inspect"
	thtrIdentifierTokenKind = "identifier"
	thtrScenarioKeyword     = "scenario"
)

type librariesInspectOptions struct {
	file   string
	format outputFormat
}

type libraryInspectionResult struct {
	File                   string                      `json:"file"`
	Syntax                 string                      `json:"syntax"`
	RepoRoot               string                      `json:"repo_root"`
	FlowRoot               string                      `json:"flow_root"`
	LibraryRoot            string                      `json:"library_root"`
	StageID                string                      `json:"stage_id"`
	SelectedLibraryFiles   []inspectedLibraryFile      `json:"selected_library_files"`
	UnselectedLibraryFiles []inspectedLibraryFile      `json:"unselected_library_files,omitempty"`
	ScenarioCallEdges      []inspectedScenarioCallEdge `json:"scenario_call_edges"`
	AuthContributions      []inspectedAuthContribution `json:"auth_contributions,omitempty"`
	RejectedAuth           []inspectedRejectedAuth     `json:"rejected_auth,omitempty"`
	InputRequirements      []inspectedInputRequirement `json:"input_requirements,omitempty"`
	Exports                []inspectedExport           `json:"exports,omitempty"`
	Diagnostics            []theater.Diagnostic        `json:"diagnostics,omitempty"`
}

type inspectedLibraryFile struct {
	Path                 string                 `json:"path"`
	Syntax               string                 `json:"syntax"`
	SelectedScenarios    []inspectedScenarioRef `json:"selected_scenarios,omitempty"`
	IgnoredScenarios     []inspectedScenarioRef `json:"ignored_scenarios,omitempty"`
	Scenarios            []inspectedScenarioRef `json:"scenarios,omitempty"`
	AuthNames            []string               `json:"auth_names,omitempty"`
	ContributedAuthNames []string               `json:"contributed_auth_names,omitempty"`
}

type inspectedScenarioRef struct {
	ID     string               `json:"id"`
	Source listedScenarioSource `json:"source"`
}

type inspectedScenarioCallEdge struct {
	CallID      string               `json:"call_id"`
	ScenarioID  string               `json:"scenario_id"`
	Kind        string               `json:"kind"`
	LibraryFile string               `json:"library_file,omitempty"`
	Source      listedScenarioSource `json:"source"`
}

type inspectedAuthContribution struct {
	Name        string               `json:"name"`
	LibraryFile string               `json:"library_file"`
	Source      listedScenarioSource `json:"source"`
}

type inspectedRejectedAuth struct {
	Name        string               `json:"name"`
	Code        string               `json:"code"`
	Summary     string               `json:"summary"`
	LibraryFile string               `json:"library_file"`
	Source      listedScenarioSource `json:"source"`
}

type inspectedInputRequirement struct {
	ScenarioID  string               `json:"scenario_id"`
	Name        string               `json:"name"`
	Contract    string               `json:"contract"`
	Required    bool                 `json:"required,omitempty"`
	LibraryFile string               `json:"library_file"`
	Source      listedScenarioSource `json:"source"`
}

type inspectedExport struct {
	Owner      string               `json:"owner"`
	ScenarioID string               `json:"scenario_id,omitempty"`
	CallID     string               `json:"call_id,omitempty"`
	ActID      string               `json:"act_id,omitempty"`
	Name       string               `json:"name"`
	Source     listedScenarioSource `json:"source"`
}

type libraryScenarioIndexEntry struct {
	File   string
	Source listedScenarioSource
}

func (a *application) runLibrariesCommand(args []string) int {
	command, rest, ok := a.resolveRequiredSubcommand(commandLibraries, args)
	if !ok {
		return exitCodeCommandError
	}

	switch command.Name {
	case commandLibrariesInspect:
		return a.inspectLibraries(rest)
	default:
		fmt.Fprintf(a.stderr, "unknown libraries subcommand %q\n", args[0])
		a.commands.PrintCommand(a.stderr, a.commands.Must(commandLibraries), nil, a.textStyler(a.stderr))
		return exitCodeCommandError
	}
}

func (a *application) inspectLibraries(args []string) int {
	options, ok := a.parseLibrariesInspectOptions(args)
	if !ok {
		return exitCodeCommandError
	}

	services, err := a.ensureServices("", "", pluginReadinessRuntime)
	if err != nil {
		fmt.Fprintf(a.stderr, "build built-in catalogs: %v\n", err)
		return exitCodeCommandError
	}

	result, err := inspectSelectedLibraries(options.file, services.matcherSugar)
	if err != nil {
		fmt.Fprintf(a.stderr, "inspect libraries: %s\n", sanitizeCLIText(err.Error()))
		return exitCodeCommandError
	}

	if err := renderLibraryInspection(a.stdout, options.format, result); err != nil {
		fmt.Fprintf(a.stderr, "render library inspection: %v\n", err)
		return exitCodeCommandError
	}
	if len(result.Diagnostics) != 0 {
		return 1
	}
	return 0
}

func (a *application) parseLibrariesInspectOptions(args []string) (librariesInspectOptions, bool) {
	normalizedArgs, usedPositionalStagePath, err := normalizeStageFileArgs(commandLibrariesInspect, args)
	if err != nil {
		fmt.Fprintf(a.stderr, "%v\n", err)
		return librariesInspectOptions{}, false
	}

	flags, options, values := a.newLibrariesInspectCommandFlagSet()
	if err := flags.Parse(normalizedArgs); err != nil {
		return librariesInspectOptions{}, false
	}
	if usedPositionalStagePath && flags.NArg() != 0 {
		fmt.Fprintln(a.stderr, "libraries inspect accepts exactly one stage file path")
		return librariesInspectOptions{}, false
	}
	if options.file == "" {
		fmt.Fprintln(a.stderr, "libraries inspect requires a stage file path via positional argument or --file")
		return librariesInspectOptions{}, false
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(a.stderr, "libraries inspect accepts exactly one stage file path")
		return librariesInspectOptions{}, false
	}

	parsedFormat, err := parseValidationOutputFormat(values.format)
	if err != nil {
		fmt.Fprintf(a.stderr, "%v\n", err)
		return librariesInspectOptions{}, false
	}
	options.format = parsedFormat
	return *options, true
}

func inspectSelectedLibraries(
	path string,
	matchers theater.MatcherSugarResolver,
) (libraryInspectionResult, error) {
	location, err := authoringyaml.ResolveFlowFileLocation(path)
	if err != nil {
		return libraryInspectionResult{}, inputInspectionError(path, err)
	}
	if !location.RepoFound {
		return libraryInspectionResult{}, fmt.Errorf("repo-local theater roots not found for flow file %s", filepath.ToSlash(path))
	}
	repoRoot := repoRootFromFlowLayout(location.Layout)
	if !location.InFlowRoot {
		return libraryInspectionResult{}, fmt.Errorf("flow file must be located under %s", repoRelativePath(repoRoot, location.Layout.FlowRoot))
	}

	syntax, ok := scenarioLibraryFileSyntax(location.Path)
	if !ok {
		return libraryInspectionResult{}, fmt.Errorf("unsupported stage file extension: %s", repoRelativePath(repoRoot, location.Path))
	}

	flowSpec, err := loadLibraryInspectionStage(location.Path, matchers)
	if err != nil {
		return libraryInspectionResult{}, repoRelativeInspectionError(repoRoot, err)
	}
	neededScenarioIDs := unresolvedInspectionScenarioIDs(flowSpec)
	libraryFiles, err := collectInspectionLibraryFiles(location.Layout.LibraryRoot, syntax)
	if err != nil {
		return libraryInspectionResult{}, repoRelativeInspectionError(repoRoot, err)
	}
	scenarioOwners, indexErrors, err := buildLibraryScenarioIndex(repoRoot, libraryFiles, neededScenarioIDs)
	if err != nil {
		return libraryInspectionResult{}, err
	}
	selectedFiles, err := selectedInspectionLibraryFiles(scenarioOwners, neededScenarioIDs)
	if err != nil {
		if len(indexErrors) != 0 {
			return libraryInspectionResult{}, fmt.Errorf("%w; %w", err, indexErrors[0])
		}
		return libraryInspectionResult{}, err
	}
	libraries, err := loadLibraryInspectionStages(repoRoot, selectedFiles, matchers)
	if err != nil {
		return libraryInspectionResult{}, err
	}

	result := libraryInspectionResult{
		File:        repoRelativePath(repoRoot, location.Path),
		Syntax:      string(syntax),
		RepoRoot:    ".",
		FlowRoot:    repoRelativePath(repoRoot, location.Layout.FlowRoot),
		LibraryRoot: repoRelativePath(repoRoot, location.Layout.LibraryRoot),
		StageID:     flowSpec.ID,
	}
	selectedSet := stringSet(selectedFiles)
	result.ScenarioCallEdges = inspectScenarioCallEdges(repoRoot, flowSpec, scenarioOwners)
	result.SelectedLibraryFiles = inspectSelectedLibraryFiles(
		repoRoot,
		libraryFiles,
		libraries,
		selectedSet,
		neededScenarioIDs,
		flowSpec,
		&result,
	)
	result.UnselectedLibraryFiles = inspectUnselectedLibraryFiles(repoRoot, libraryFiles, selectedSet)
	return result, nil
}

func repoRootFromFlowLayout(layout authoringyaml.FlowRepoLayout) string {
	return filepath.Dir(filepath.Dir(layout.FlowRoot))
}

func loadLibraryInspectionStage(path string, matchers theater.MatcherSugarResolver) (theater.StageSpec, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case yamlFileExtension, ymlFileExtension:
		return theateryaml.LoadFile(path, matchers)
	case thtrFileExtension:
		return theaterthtr.LoadFile(path, matchers)
	default:
		return theater.StageSpec{}, fmt.Errorf("unsupported stage file extension: %s", path)
	}
}

func collectInspectionLibraryFiles(libraryRoot string, syntax scenarioLibrarySyntax) ([]string, error) {
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
		if !ok || (syntax != scenarioLibrarySyntaxAll && syntax != fileSyntax) {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > maxScenarioLibraryFileSize {
			return nil
		}
		files = append(files, path)
		if len(files) > maxScenarioLibraryFiles {
			return fmt.Errorf("library inspection supports at most %d scenario files", maxScenarioLibraryFiles)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(files)
	return files, nil
}

func loadLibraryInspectionStages(
	repoRoot string,
	files []string,
	matchers theater.MatcherSugarResolver,
) (map[string]theater.StageSpec, error) {
	libraries := make(map[string]theater.StageSpec, len(files))
	for i := range files {
		stage, err := loadScenarioLibraryStage(files[i], matchers)
		if err != nil {
			return nil, repoRelativeInspectionError(repoRoot, err)
		}
		if len(stage.ScenarioCalls) != 0 {
			return nil, fmt.Errorf("library file %s must not declare scenario_calls", repoRelativePath(repoRoot, files[i]))
		}
		libraries[files[i]] = stage
	}
	return libraries, nil
}

func buildLibraryScenarioIndex(
	repoRoot string,
	libraryFiles []string,
	neededScenarioIDs map[string]struct{},
) (map[string]libraryScenarioIndexEntry, []error, error) {
	owners := make(map[string]libraryScenarioIndexEntry)
	var indexErrors []error
	for i := range libraryFiles {
		entries, err := indexInspectableScenarioRefs(repoRoot, libraryFiles[i])
		if err != nil {
			indexErrors = append(indexErrors, err)
			continue
		}
		for j := range entries {
			scenarioID := entries[j].ID
			if _, needed := neededScenarioIDs[scenarioID]; !needed {
				continue
			}
			if existing, ok := owners[scenarioID]; ok {
				if existing.File == libraryFiles[i] {
					return nil, nil, fmt.Errorf(
						"library scenario %q is declared multiple times in %s",
						scenarioID,
						repoRelativePath(repoRoot, libraryFiles[i]),
					)
				}
				return nil, nil, fmt.Errorf(
					"library scenario %q is declared in multiple files: %s, %s",
					scenarioID,
					repoRelativePath(repoRoot, existing.File),
					repoRelativePath(repoRoot, libraryFiles[i]),
				)
			}
			owners[scenarioID] = libraryScenarioIndexEntry{File: libraryFiles[i], Source: entries[j].Source}
		}
	}
	return owners, indexErrors, nil
}

func inputInspectionError(path string, err error) error {
	message := filepath.ToSlash(err.Error())
	absolute, absErr := filepath.Abs(path)
	if absErr == nil {
		message = strings.ReplaceAll(message, filepath.ToSlash(absolute), filepath.ToSlash(path))
	}
	return fmt.Errorf("%s", message)
}

func repoRelativeInspectionError(repoRoot string, err error) error {
	message := filepath.ToSlash(err.Error())
	root := filepath.ToSlash(repoRoot)
	if root != "" {
		message = strings.ReplaceAll(message, root+"/", "")
		message = strings.ReplaceAll(message, root, ".")
	}
	return fmt.Errorf("%s", message)
}

func scenarioIndexError(repoRoot, path string, err error) error {
	return fmt.Errorf(
		"inspect library scenario index %s: %w",
		repoRelativePath(repoRoot, path),
		repoRelativeInspectionError(repoRoot, err),
	)
}

func inspectableScenarioRefs(repoRoot, path string) []inspectedScenarioRef {
	refs, _ := indexInspectableScenarioRefs(repoRoot, path)
	return refs
}

func indexInspectableScenarioRefs(repoRoot, path string) ([]inspectedScenarioRef, error) {
	syntax, ok := scenarioLibraryFileSyntax(path)
	if !ok {
		return nil, nil
	}
	switch syntax {
	case scenarioLibrarySyntaxYAML:
		return inspectableYAMLScenarioRefs(repoRoot, path)
	case scenarioLibrarySyntaxTHTR:
		return inspectableTHTRScenarioRefs(repoRoot, path)
	default:
		return nil, nil
	}
}

func inspectableYAMLScenarioRefs(repoRoot, path string) ([]inspectedScenarioRef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, scenarioIndexError(repoRoot, path, err)
	}
	var root goyaml.Node
	if err := goyaml.NewDecoder(bytes.NewReader(data)).Decode(&root); err != nil {
		return nil, scenarioIndexError(repoRoot, path, err)
	}
	if len(root.Content) == 0 {
		return nil, nil
	}
	scenarios := yamlMappingValue(root.Content[0], "scenarios")
	if scenarios == nil || scenarios.Kind != goyaml.SequenceNode {
		return nil, nil
	}

	refs := make([]inspectedScenarioRef, 0, len(scenarios.Content))
	for i := range scenarios.Content {
		item := scenarios.Content[i]
		idNode := yamlMappingValue(item, "id")
		if idNode == nil || idNode.Value == "" {
			continue
		}
		refs = append(refs, inspectedScenarioRef{
			ID: idNode.Value,
			Source: listedScenarioSource{
				File:   repoRelativePath(repoRoot, path),
				Line:   item.Line,
				Column: item.Column,
			},
		})
	}
	return refs, nil
}

func inspectableTHTRScenarioRefs(repoRoot, path string) ([]inspectedScenarioRef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, scenarioIndexError(repoRoot, path, err)
	}
	tokens, err := theaterthtr.Tokenize(data)
	if err != nil {
		return nil, scenarioIndexError(repoRoot, path, err)
	}
	refs := make([]inspectedScenarioRef, 0)
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Kind != thtrIdentifierTokenKind || tokens[i].Text != thtrScenarioKeyword {
			continue
		}
		id, ok := thtrScenarioIDFromTokens(tokens[i+1:])
		if !ok {
			continue
		}
		refs = append(refs, inspectedScenarioRef{
			ID: id,
			Source: listedScenarioSource{
				File:   repoRelativePath(repoRoot, path),
				Line:   tokens[i].StartLine,
				Column: tokens[i].StartColumn,
			},
		})
	}
	return refs, nil
}

func thtrScenarioIDFromTokens(tokens []theaterthtr.Token) (string, bool) {
	var builder strings.Builder
	for i := range tokens {
		token := tokens[i]
		switch token.Kind {
		case thtrIdentifierTokenKind, "/", ".", "-":
			builder.WriteString(token.Text)
		default:
			id := builder.String()
			return id, id != ""
		}
	}
	id := builder.String()
	return id, id != ""
}

func yamlMappingValue(node *goyaml.Node, key string) *goyaml.Node {
	if node == nil || node.Kind != goyaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func selectedInspectionLibraryFiles(
	scenarioOwners map[string]libraryScenarioIndexEntry,
	neededScenarioIDs map[string]struct{},
) ([]string, error) {
	selected := make(map[string]struct{})
	for scenarioID := range neededScenarioIDs {
		entry, ok := scenarioOwners[scenarioID]
		if !ok {
			return nil, fmt.Errorf("referenced library scenario %q is not found", scenarioID)
		}
		selected[entry.File] = struct{}{}
	}
	return sortedStringSet(selected), nil
}

func inspectScenarioCallEdges(
	repoRoot string,
	flowSpec theater.StageSpec,
	scenarioOwners map[string]libraryScenarioIndexEntry,
) []inspectedScenarioCallEdge {
	local := localInspectionScenarioIDs(flowSpec)
	edges := make([]inspectedScenarioCallEdge, 0, len(flowSpec.ScenarioCalls))
	for i := range flowSpec.ScenarioCalls {
		call := flowSpec.ScenarioCalls[i]
		edge := inspectedScenarioCallEdge{
			CallID:     call.ID,
			ScenarioID: call.ScenarioID,
			Kind:       "local",
			Source:     sourceFor(repoRoot, "", call.SourceSpan),
		}
		if _, ok := local[call.ScenarioID]; !ok {
			edge.Kind = "library"
			edge.LibraryFile = repoRelativePath(repoRoot, scenarioOwners[call.ScenarioID].File)
		}
		edges = append(edges, edge)
	}
	return edges
}

func inspectSelectedLibraryFiles(
	repoRoot string,
	libraryFiles []string,
	libraries map[string]theater.StageSpec,
	selectedSet map[string]struct{},
	neededScenarioIDs map[string]struct{},
	flowSpec theater.StageSpec,
	result *libraryInspectionResult,
) []inspectedLibraryFile {
	flowAuthNames := flowauth.DeclaredHTTPAuthNames(flowSpec.HTTP)
	composedAuthOwners := make(map[string]string)
	files := make([]inspectedLibraryFile, 0, len(selectedSet))
	for i := range libraryFiles {
		if _, selected := selectedSet[libraryFiles[i]]; !selected {
			continue
		}
		stage := libraries[libraryFiles[i]]
		selectedScenarioIDs := selectedScenarioIDsFor(stage.Scenarios, neededScenarioIDs)
		selectedAuthNames := flowauth.SelectedScenarioHTTPAuthNames(stage.Scenarios, selectedScenarioIDs)
		displayLibraryFile := repoRelativePath(repoRoot, libraryFiles[i])
		issues := flowauth.SelectedLibraryHTTPAuthIssues(
			stage,
			selectedScenarioIDs,
			flowAuthNames,
			composedAuthOwners,
			displayLibraryFile,
		)
		rejected := stringSetFromAuthIssues(issues)
		for j := range issues {
			result.RejectedAuth = append(result.RejectedAuth, rejectedAuthFor(displayLibraryFile, issues[j]))
			result.Diagnostics = append(result.Diagnostics, diagnosticForRejectedAuth(displayLibraryFile, issues[j]))
		}

		file := inspectedLibraryFile{
			Path:                 displayLibraryFile,
			Syntax:               string(mustScenarioLibraryFileSyntax(libraryFiles[i])),
			SelectedScenarios:    selectedScenarioRefs(repoRoot, stage.Scenarios, selectedScenarioIDs, true),
			IgnoredScenarios:     selectedScenarioRefs(repoRoot, stage.Scenarios, selectedScenarioIDs, false),
			AuthNames:            sortedHTTPAuthNames(stage.HTTP),
			ContributedAuthNames: contributedAuthNames(stage.HTTP, selectedAuthNames, rejected),
		}
		files = append(files, file)
		for _, authName := range file.ContributedAuthNames {
			result.AuthContributions = append(result.AuthContributions, inspectedAuthContribution{
				Name:        authName,
				LibraryFile: file.Path,
				Source:      listedScenarioSource{File: file.Path},
			})
		}
		inputs := inputRequirementsFor(repoRoot, libraryFiles[i], stage.Scenarios, selectedScenarioIDs)
		result.InputRequirements = append(result.InputRequirements, inputs...)
		result.Exports = append(result.Exports, scenarioExportsFor(repoRoot, libraryFiles[i], stage.Scenarios, selectedScenarioIDs)...)
	}
	result.Exports = append(result.Exports, scenarioCallExportsFor(repoRoot, flowSpec)...)
	sortLibraryInspectionSlices(result)
	return files
}

func inspectUnselectedLibraryFiles(
	repoRoot string,
	libraryFiles []string,
	selectedSet map[string]struct{},
) []inspectedLibraryFile {
	files := make([]inspectedLibraryFile, 0)
	for i := range libraryFiles {
		if _, selected := selectedSet[libraryFiles[i]]; selected {
			continue
		}
		files = append(files, inspectedLibraryFile{
			Path:      repoRelativePath(repoRoot, libraryFiles[i]),
			Syntax:    string(mustScenarioLibraryFileSyntax(libraryFiles[i])),
			Scenarios: inspectableScenarioRefs(repoRoot, libraryFiles[i]),
		})
	}
	return files
}

func renderLibraryInspection(writer io.Writer, format outputFormat, result libraryInspectionResult) error {
	switch format {
	case outputFormatText:
		return renderLibraryInspectionText(writer, result)
	case outputFormatJSON:
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func renderLibraryInspectionText(writer io.Writer, result libraryInspectionResult) error {
	status := doctorReadyStatus
	if len(result.Diagnostics) != 0 {
		status = "diagnostics"
	}
	if _, err := fmt.Fprintf(
		writer,
		"Library inspection: %s\nStatus: %s\nSyntax: %s\n\n",
		sanitizeCLIText(result.File),
		status,
		sanitizeCLIText(result.Syntax),
	); err != nil {
		return err
	}
	if err := renderInspectedLibraryFiles(writer, "Selected library files", result.SelectedLibraryFiles, true); err != nil {
		return err
	}
	if err := renderInspectedLibraryFiles(writer, "Unselected library files", result.UnselectedLibraryFiles, false); err != nil {
		return err
	}
	if err := renderScenarioCallEdges(writer, result.ScenarioCallEdges); err != nil {
		return err
	}
	if err := renderAuthContributions(writer, result.AuthContributions, result.RejectedAuth); err != nil {
		return err
	}
	if err := renderInputRequirements(writer, result.InputRequirements); err != nil {
		return err
	}
	if err := renderInspectedExports(writer, result.Exports); err != nil {
		return err
	}
	return renderLibraryDiagnostics(writer, result.Diagnostics)
}

func renderInspectedLibraryFiles(
	writer io.Writer,
	title string,
	files []inspectedLibraryFile,
	selected bool,
) error {
	if _, err := fmt.Fprintf(writer, "%s\n", title); err != nil {
		return err
	}
	if len(files) == 0 {
		_, err := fmt.Fprintln(writer, "- none")
		return err
	}
	for i := range files {
		file := files[i]
		if _, err := fmt.Fprintf(writer, "- %s\n", sanitizeCLIText(file.Path)); err != nil {
			return err
		}
		if selected {
			if _, err := fmt.Fprintf(writer, "  Selected scenarios: %s\n", formatScenarioRefs(file.SelectedScenarios)); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(writer, "  Ignored scenarios: %s\n", formatScenarioRefs(file.IgnoredScenarios)); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(writer, "  Auth contributions: %s\n", formatStringList(file.ContributedAuthNames)); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintf(writer, "  Scenarios: %s\n", formatScenarioRefs(file.Scenarios)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(writer, "  Auth names: %s\n", formatStringList(file.AuthNames)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(writer)
	return err
}

func renderScenarioCallEdges(writer io.Writer, edges []inspectedScenarioCallEdge) error {
	if _, err := fmt.Fprintln(writer, "Scenario call graph"); err != nil {
		return err
	}
	if len(edges) == 0 {
		if _, err := fmt.Fprintln(writer, "- none"); err != nil {
			return err
		}
		_, err := fmt.Fprintln(writer)
		return err
	}
	for i := range edges {
		edge := edges[i]
		target := edge.Kind
		if edge.LibraryFile != "" {
			target = edge.LibraryFile
		}
		if _, err := fmt.Fprintf(
			writer,
			"- %s -> %s (%s) at %s\n",
			sanitizeCLIText(edge.CallID),
			sanitizeCLIText(edge.ScenarioID),
			sanitizeCLIText(target),
			sanitizeCLIText(formatListedScenarioSource(edge.Source)),
		); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(writer)
	return err
}

func renderAuthContributions(
	writer io.Writer,
	contributions []inspectedAuthContribution,
	rejected []inspectedRejectedAuth,
) error {
	if _, err := fmt.Fprintln(writer, "Auth contributions"); err != nil {
		return err
	}
	if len(contributions) == 0 {
		if _, err := fmt.Fprintln(writer, "- none"); err != nil {
			return err
		}
	}
	for i := range contributions {
		if _, err := fmt.Fprintf(
			writer,
			"- %s from %s\n",
			sanitizeCLIText(contributions[i].Name),
			sanitizeCLIText(contributions[i].LibraryFile),
		); err != nil {
			return err
		}
	}
	if len(rejected) != 0 {
		if _, err := fmt.Fprintln(writer, "Rejected auth"); err != nil {
			return err
		}
		for i := range rejected {
			if _, err := fmt.Fprintf(
				writer,
				"- %s %s at %s\n",
				sanitizeCLIText(rejected[i].Name),
				sanitizeCLIText(rejected[i].Code),
				sanitizeCLIText(formatListedScenarioSource(rejected[i].Source)),
			); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintln(writer)
	return err
}

func renderInputRequirements(writer io.Writer, inputs []inspectedInputRequirement) error {
	if _, err := fmt.Fprintln(writer, "Input requirements"); err != nil {
		return err
	}
	if len(inputs) == 0 {
		if _, err := fmt.Fprintln(writer, "- none"); err != nil {
			return err
		}
		_, err := fmt.Fprintln(writer)
		return err
	}
	table := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)
	for i := range inputs {
		required := "optional"
		if inputs[i].Required {
			required = "required"
		}
		if _, err := fmt.Fprintf(
			table,
			"- %s.%s\t%s\t%s\t%s\n",
			sanitizeCLIText(inputs[i].ScenarioID),
			sanitizeCLIText(inputs[i].Name),
			required,
			sanitizeCLIText(inputs[i].Contract),
			sanitizeCLIText(formatListedScenarioSource(inputs[i].Source)),
		); err != nil {
			return err
		}
	}
	if err := table.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintln(writer)
	return err
}

func renderInspectedExports(writer io.Writer, exports []inspectedExport) error {
	if _, err := fmt.Fprintln(writer, "Exports"); err != nil {
		return err
	}
	if len(exports) == 0 {
		if _, err := fmt.Fprintln(writer, "- none"); err != nil {
			return err
		}
		_, err := fmt.Fprintln(writer)
		return err
	}
	for i := range exports {
		if _, err := fmt.Fprintf(
			writer,
			"- %s.%s (%s) at %s\n",
			sanitizeCLIText(exportOwner(exports[i])),
			sanitizeCLIText(exports[i].Name),
			sanitizeCLIText(exports[i].Owner),
			sanitizeCLIText(formatListedScenarioSource(exports[i].Source)),
		); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(writer)
	return err
}

func renderLibraryDiagnostics(writer io.Writer, diagnostics []theater.Diagnostic) error {
	if len(diagnostics) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(writer, "Diagnostics"); err != nil {
		return err
	}
	for i := range diagnostics {
		if _, err := fmt.Fprintf(
			writer,
			"- %s %s at %s\n",
			sanitizeCLIText(diagnostics[i].Code),
			sanitizeCLIText(diagnostics[i].Summary),
			sanitizeCLIText(formatListedScenarioSource(sourceFor("", "", &diagnostics[i].Span))),
		); err != nil {
			return err
		}
	}
	return nil
}

func unresolvedInspectionScenarioIDs(flowSpec theater.StageSpec) map[string]struct{} {
	local := localInspectionScenarioIDs(flowSpec)
	needed := make(map[string]struct{})
	for i := range flowSpec.ScenarioCalls {
		scenarioID := flowSpec.ScenarioCalls[i].ScenarioID
		if _, ok := local[scenarioID]; ok {
			continue
		}
		needed[scenarioID] = struct{}{}
	}
	return needed
}

func localInspectionScenarioIDs(flowSpec theater.StageSpec) map[string]struct{} {
	local := make(map[string]struct{}, len(flowSpec.Scenarios))
	for i := range flowSpec.Scenarios {
		local[flowSpec.Scenarios[i].ID] = struct{}{}
	}
	return local
}

func selectedScenarioIDsFor(scenarios []theater.ScenarioSpec, needed map[string]struct{}) map[string]struct{} {
	selected := make(map[string]struct{})
	for i := range scenarios {
		if _, ok := needed[scenarios[i].ID]; ok {
			selected[scenarios[i].ID] = struct{}{}
		}
	}
	return selected
}

func selectedScenarioRefs(
	repoRoot string,
	scenarios []theater.ScenarioSpec,
	selected map[string]struct{},
	wantSelected bool,
) []inspectedScenarioRef {
	refs := make([]inspectedScenarioRef, 0)
	for i := range scenarios {
		_, ok := selected[scenarios[i].ID]
		if ok != wantSelected {
			continue
		}
		refs = append(refs, inspectedScenarioRef{
			ID:     scenarios[i].ID,
			Source: sourceFor(repoRoot, "", scenarios[i].SourceSpan),
		})
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })
	return refs
}

func inputRequirementsFor(
	repoRoot string,
	libraryFile string,
	scenarios []theater.ScenarioSpec,
	selected map[string]struct{},
) []inspectedInputRequirement {
	inputs := make([]inspectedInputRequirement, 0)
	for i := range scenarios {
		scenario := scenarios[i]
		if _, ok := selected[scenario.ID]; !ok {
			continue
		}
		names := sortedValueContractNames(scenario.Inputs)
		for _, name := range names {
			contract := scenario.Inputs[name]
			inputs = append(inputs, inspectedInputRequirement{
				ScenarioID:  scenario.ID,
				Name:        name,
				Contract:    formatValueContract(contract, false),
				Required:    contract.Required,
				LibraryFile: repoRelativePath(repoRoot, libraryFile),
				Source:      sourceFor(repoRoot, libraryFile, scenario.SourceSpan),
			})
		}
	}
	return inputs
}

func scenarioExportsFor(
	repoRoot string,
	libraryFile string,
	scenarios []theater.ScenarioSpec,
	selected map[string]struct{},
) []inspectedExport {
	exports := make([]inspectedExport, 0)
	for i := range scenarios {
		scenario := scenarios[i]
		if _, ok := selected[scenario.ID]; !ok {
			continue
		}
		for j := range scenario.Acts {
			act := scenario.Acts[j]
			for k := range act.Exports {
				exports = append(exports, inspectedExport{
					Owner:      "scenario",
					ScenarioID: scenario.ID,
					ActID:      act.ID,
					Name:       act.Exports[k].As,
					Source:     sourceFor(repoRoot, libraryFile, act.SourceSpan),
				})
			}
		}
	}
	return exports
}

func scenarioCallExportsFor(repoRoot string, flowSpec theater.StageSpec) []inspectedExport {
	exports := make([]inspectedExport, 0)
	for i := range flowSpec.ScenarioCalls {
		call := flowSpec.ScenarioCalls[i]
		for j := range call.Exports {
			exports = append(exports, inspectedExport{
				Owner:      "scenario_call",
				CallID:     call.ID,
				ScenarioID: call.ScenarioID,
				Name:       call.Exports[j].As,
				Source:     sourceFor(repoRoot, "", call.SourceSpan),
			})
		}
	}
	return exports
}

func diagnosticForRejectedAuth(libraryFile string, issue flowauth.SelectedLibraryAuthError) theater.Diagnostic {
	return theater.Diagnostic{
		Code:     issue.Code,
		Path:     "http.auth." + issue.AuthName,
		Severity: theater.SeverityError,
		Summary:  issue.Summary,
		Span: theater.SourceRef{
			File: libraryFile,
		},
	}
}

func rejectedAuthFor(libraryFile string, issue flowauth.SelectedLibraryAuthError) inspectedRejectedAuth {
	return inspectedRejectedAuth{
		Name:        issue.AuthName,
		Code:        issue.Code,
		Summary:     issue.Summary,
		LibraryFile: libraryFile,
		Source:      listedScenarioSource{File: libraryFile},
	}
}

func sourceFor(repoRoot, fallbackPath string, source *theater.SourceRef) listedScenarioSource {
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

func exportOwner(export inspectedExport) string {
	switch export.Owner {
	case "scenario_call":
		return export.CallID
	case "scenario":
		if export.ActID != "" {
			return export.ScenarioID + "." + export.ActID
		}
		return export.ScenarioID
	default:
		return export.Owner
	}
}

func sortLibraryInspectionSlices(result *libraryInspectionResult) {
	sort.Slice(result.AuthContributions, func(i, j int) bool {
		return result.AuthContributions[i].Name < result.AuthContributions[j].Name
	})
	sort.Slice(result.RejectedAuth, func(i, j int) bool {
		return result.RejectedAuth[i].Name < result.RejectedAuth[j].Name
	})
	sort.Slice(result.InputRequirements, func(i, j int) bool {
		return result.InputRequirements[i].ScenarioID+"."+result.InputRequirements[i].Name <
			result.InputRequirements[j].ScenarioID+"."+result.InputRequirements[j].Name
	})
	sort.Slice(result.Exports, func(i, j int) bool {
		return exportOwner(result.Exports[i])+"."+result.Exports[i].Name <
			exportOwner(result.Exports[j])+"."+result.Exports[j].Name
	})
}

func contributedAuthNames(
	httpSpec *theater.HTTPSpec,
	selectedAuthNames map[string]struct{},
	rejected map[string]struct{},
) []string {
	if httpSpec == nil || len(httpSpec.Auth) == 0 {
		return nil
	}
	contributed := make([]string, 0)
	for name := range selectedAuthNames {
		if _, ok := httpSpec.Auth[name]; !ok {
			continue
		}
		if _, isRejected := rejected[name]; isRejected {
			continue
		}
		contributed = append(contributed, name)
	}
	sort.Strings(contributed)
	return contributed
}

func sortedHTTPAuthNames(httpSpec *theater.HTTPSpec) []string {
	if httpSpec == nil || len(httpSpec.Auth) == 0 {
		return nil
	}
	names := make([]string, 0, len(httpSpec.Auth))
	for name := range httpSpec.Auth {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedValueContractNames(values map[string]theater.ValueContract) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedStringSet(values map[string]struct{}) []string {
	items := make([]string, 0, len(values))
	for value := range values {
		items = append(items, value)
	}
	sort.Strings(items)
	return items
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for i := range values {
		set[values[i]] = struct{}{}
	}
	return set
}

func stringSetFromAuthIssues(issues []flowauth.SelectedLibraryAuthError) map[string]struct{} {
	set := make(map[string]struct{}, len(issues))
	for i := range issues {
		set[issues[i].AuthName] = struct{}{}
	}
	return set
}

func mustScenarioLibraryFileSyntax(path string) scenarioLibrarySyntax {
	syntax, ok := scenarioLibraryFileSyntax(path)
	if !ok {
		return ""
	}
	return syntax
}

func formatScenarioRefs(refs []inspectedScenarioRef) string {
	if len(refs) == 0 {
		return "-"
	}
	ids := make([]string, 0, len(refs))
	for i := range refs {
		ids = append(ids, sanitizeCLIText(refs[i].ID))
	}
	return strings.Join(ids, ", ")
}

func formatStringList(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	sanitized := make([]string, 0, len(values))
	for i := range values {
		sanitized = append(sanitized, sanitizeCLIText(values[i]))
	}
	return strings.Join(sanitized, ", ")
}
