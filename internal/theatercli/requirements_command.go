package theatercli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/authoring/flowauth"
	authoringyaml "github.com/alex-poliushkin/theater/internal/authoring/yaml"
	internalpluginregistry "github.com/alex-poliushkin/theater/internal/pluginregistry"
)

const (
	commandRequirementsInspect = "inspect"

	requirementReadinessAvailable = "available"
	requirementReadinessBound     = "bound"
	requirementReadinessMissing   = "missing"
	requirementReadinessUnchecked = "unchecked"
)

type requirementsInspectOptions struct {
	globalOptions
	file     string
	format   outputFormat
	checkEnv bool
}

type requirementsInventory struct {
	File         string                   `json:"file"`
	CheckEnv     bool                     `json:"check_env"`
	Requirements []runtimeRequirementItem `json:"requirements"`
}

type runtimeRequirementItem struct {
	Kind       string               `json:"kind"`
	Name       string               `json:"name"`
	Owner      string               `json:"owner"`
	Required   bool                 `json:"required"`
	Readiness  string               `json:"readiness"`
	PluginID   string               `json:"plugin_id,omitempty"`
	ScenarioID string               `json:"scenario_id,omitempty"`
	AuthName   string               `json:"auth_name,omitempty"`
	Slot       string               `json:"slot,omitempty"`
	Source     listedScenarioSource `json:"source"`
}

func (a *application) runRequirementsCommand(args []string) int {
	command, rest, ok := a.resolveRequiredSubcommand(commandRequirements, args)
	if !ok {
		return exitCodeCommandError
	}

	switch command.Name {
	case commandRequirementsInspect:
		return a.inspectRequirements(rest)
	default:
		fmt.Fprintf(a.stderr, "unknown requirements subcommand %q\n", args[0])
		a.commands.PrintCommand(a.stderr, a.commands.Must(commandRequirements), nil, a.textStyler(a.stderr))
		return exitCodeCommandError
	}
}

func (a *application) inspectRequirements(args []string) int {
	options, ok := a.parseRequirementsInspectOptions(args)
	if !ok {
		return exitCodeCommandError
	}

	inventory, err := a.buildRequirementsInventory(options)
	if err != nil {
		fmt.Fprintf(a.stderr, "inspect requirements: %s\n", sanitizeCLIText(err.Error()))
		return exitCodeCommandError
	}
	if err := renderRequirementsInventory(a.stdout, options.format, inventory); err != nil {
		fmt.Fprintf(a.stderr, "render requirements inventory: %v\n", err)
		return exitCodeCommandError
	}
	if requirementsInventoryHasMissing(inventory) {
		return 1
	}
	return 0
}

func (a *application) parseRequirementsInspectOptions(args []string) (requirementsInspectOptions, bool) {
	normalizedArgs, usedPositionalStagePath, err := normalizeStageFileArgs(commandRequirementsInspectPath, args)
	if err != nil {
		fmt.Fprintf(a.stderr, "%v\n", err)
		return requirementsInspectOptions{}, false
	}

	flags, options, values := a.newRequirementsInspectCommandFlagSet()
	if err := flags.Parse(normalizedArgs); err != nil {
		return requirementsInspectOptions{}, false
	}
	if usedPositionalStagePath && flags.NArg() != 0 {
		fmt.Fprintln(a.stderr, "requirements inspect accepts exactly one stage file path")
		return requirementsInspectOptions{}, false
	}
	if options.file == "" {
		fmt.Fprintln(a.stderr, "requirements inspect requires a stage file path via positional argument or --file")
		return requirementsInspectOptions{}, false
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(a.stderr, "requirements inspect accepts exactly one stage file path")
		return requirementsInspectOptions{}, false
	}

	parsedFormat, err := parseValidationOutputFormat(values.format)
	if err != nil {
		fmt.Fprintf(a.stderr, "%v\n", err)
		return requirementsInspectOptions{}, false
	}
	options.globalOptions = sharedGlobalOptionContract.Resolve(options.globalOptions)
	options.format = parsedFormat
	return *options, true
}

func (a *application) buildRequirementsInventory(options requirementsInspectOptions) (requirementsInventory, error) {
	services, err := a.ensureServices(options.pluginsConfig, options.pluginsLock, pluginReadinessDescriptor)
	if err != nil {
		return requirementsInventory{}, err
	}

	loader := newStageFileLoader(services.matcherSugar)
	loaded, err := loader.Load(options.file)
	if err != nil {
		return requirementsInventory{}, err
	}

	repoRoot := requirementsRepoRoot(options.file)
	inventory := requirementsInventory{
		File:     sourceFor(repoRoot, options.file, loaded.Spec.SourceSpan).File,
		CheckEnv: options.checkEnv,
	}

	pluginRequirements, err := collectPluginEnvRequirements(options, repoRoot, loaded.Spec)
	if err != nil {
		return requirementsInventory{}, err
	}
	inventory.Requirements = append(inventory.Requirements, pluginRequirements...)
	inventory.Requirements = append(inventory.Requirements, collectHTTPAuthSlotRequirements(repoRoot, loaded.Spec)...)
	sortRuntimeRequirements(inventory.Requirements)
	return inventory, nil
}

func requirementsRepoRoot(path string) string {
	location, err := authoringyaml.ResolveFlowFileLocation(path)
	if err != nil || !location.RepoFound {
		return ""
	}
	return repoRootFromFlowLayout(location.Layout)
}

func collectPluginEnvRequirements(
	options requirementsInspectOptions,
	repoRoot string,
	stage theater.StageSpec,
) ([]runtimeRequirementItem, error) {
	if options.pluginsConfig == "" {
		return nil, nil
	}

	loaded, err := internalpluginregistry.LoadDescriptors(options.pluginsConfig, options.pluginsLock)
	if err != nil {
		return nil, err
	}

	pluginIDs := selectedPluginIDsForRequirements(loaded, stage)
	requirements := make([]runtimeRequirementItem, 0)
	for _, pluginID := range pluginIDs {
		plugin := loaded.Plugins[pluginID]
		for _, name := range sortedStrings(plugin.Config.Grants.EnvFromHost) {
			readiness := requirementReadinessUnchecked
			if options.checkEnv {
				readiness = requirementReadinessAvailable
				if _, ok := os.LookupEnv(name); !ok {
					readiness = requirementReadinessMissing
				}
			}
			requirements = append(requirements, runtimeRequirementItem{
				Kind:      "plugin_env_from_host",
				Name:      name,
				Owner:     plugin.ID,
				Required:  true,
				Readiness: readiness,
				PluginID:  plugin.ID,
				Source:    listedScenarioSource{File: requirementsSourcePath(repoRoot, loaded.ConfigPath)},
			})
		}
	}
	return requirements, nil
}

func requirementsSourcePath(repoRoot, path string) string {
	if path == "" {
		return ""
	}

	clean := filepath.Clean(path)
	if repoRoot == "" {
		return localSourcePath(clean)
	}

	relative, ok := repoRelativeSourcePath(repoRoot, clean)
	if ok {
		return relative
	}
	return localSourcePath(clean)
}

func repoRelativeSourcePath(repoRoot, path string) (string, bool) {
	absolutePath := path
	if !filepath.IsAbs(absolutePath) {
		workingDir, err := os.Getwd()
		if err != nil {
			return "", false
		}
		absolutePath = filepath.Join(workingDir, path)
	}

	relative, err := filepath.Rel(repoRoot, absolutePath)
	if err != nil ||
		relative == "." ||
		relative == initParentDir ||
		strings.HasPrefix(relative, initParentDir+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(relative), true
}

func localSourcePath(path string) string {
	if filepath.IsAbs(path) {
		return filepath.ToSlash(filepath.Base(path))
	}
	return filepath.ToSlash(path)
}

func selectedPluginIDsForRequirements(
	loaded *internalpluginregistry.LoadedRegistry,
	stage theater.StageSpec,
) []string {
	if loaded == nil || len(loaded.Plugins) == 0 {
		return nil
	}

	capabilityPluginIDs := make(map[string]string)
	for _, pluginID := range sortedMapKeys(loaded.Plugins) {
		plugin := loaded.Plugins[pluginID]
		for _, capabilityName := range sortedStrings(plugin.Config.AllowCapabilities) {
			if _, ok := plugin.Capabilities[capabilityName]; ok {
				capabilityPluginIDs[capabilityName] = plugin.ID
			}
		}
	}

	stageCapabilityRefs := collectStageCapabilityRefsForRequirements(stage)
	selectedPluginIDs := make(map[string]struct{})
	for capabilityName := range stageCapabilityRefs {
		pluginID, ok := capabilityPluginIDs[capabilityName]
		if ok {
			selectedPluginIDs[pluginID] = struct{}{}
		}
	}
	return sortedMapKeys(selectedPluginIDs)
}

func collectStageCapabilityRefsForRequirements(stage theater.StageSpec) map[string]struct{} {
	refs := make(map[string]struct{})
	collectStateCapabilityRefsForRequirements(refs, stage.State)
	for i := range stage.Scenarios {
		collectScenarioCapabilityRefsForRequirements(refs, stage.Scenarios[i])
	}
	for i := range stage.ScenarioCalls {
		call := stage.ScenarioCalls[i]
		collectBindingMapCapabilityRefsForRequirements(refs, call.Bindings)
		for j := range call.Exports {
			collectExportCapabilityRefsForRequirements(refs, call.Exports[j])
		}
	}
	return refs
}

func collectStateCapabilityRefsForRequirements(refs map[string]struct{}, state *theater.StateSpec) {
	if state == nil {
		return
	}
	for name := range state.Backends {
		use := state.Backends[name].Use
		if use != "" {
			refs[use] = struct{}{}
		}
	}
}

func collectScenarioCapabilityRefsForRequirements(refs map[string]struct{}, scenario theater.ScenarioSpec) {
	for i := range scenario.Acts {
		act := scenario.Acts[i]
		if act.Action.Use != "" {
			refs[act.Action.Use] = struct{}{}
		}
		collectBindingMapCapabilityRefsForRequirements(refs, act.Action.With)
		for propertyName := range act.Properties {
			collectPropertyCapabilityRefsForRequirements(refs, act.Properties[propertyName])
		}
		for j := range act.Expectations {
			collectSubjectCapabilityRefsForRequirements(refs, act.Expectations[j].Subject)
			collectAssertCapabilityRefsForRequirements(refs, act.Expectations[j].Assert)
		}
		for j := range act.Logs {
			collectLogValueCapabilityRefsForRequirements(refs, act.Logs[j].Value)
			for fieldName := range act.Logs[j].Fields {
				collectLogValueCapabilityRefsForRequirements(refs, act.Logs[j].Fields[fieldName])
			}
		}
		for j := range act.Exports {
			collectExportCapabilityRefsForRequirements(refs, act.Exports[j])
		}
	}
}

func collectPropertyCapabilityRefsForRequirements(refs map[string]struct{}, property theater.PropertySpec) {
	if property.Inventory != nil {
		if property.Inventory.Use != "" {
			refs[property.Inventory.Use] = struct{}{}
		}
		collectBindingMapCapabilityRefsForRequirements(refs, property.Inventory.With)
	}
	if property.Value != nil {
		collectBindingCapabilityRefsForRequirements(refs, *property.Value)
	}
	for i := range property.Decorators {
		if property.Decorators[i].Use != "" {
			refs[property.Decorators[i].Use] = struct{}{}
		}
	}
}

func collectSubjectCapabilityRefsForRequirements(refs map[string]struct{}, subject theater.SubjectSpec) {
	collectThroughCapabilityRefsForRequirements(refs, subject.Through)
}

func collectAssertCapabilityRefsForRequirements(refs map[string]struct{}, assert theater.AssertSpec) {
	if assert.Ref != "" {
		refs[assert.Ref] = struct{}{}
	}
	collectBindingMapCapabilityRefsForRequirements(refs, assert.Args)
}

func collectExportCapabilityRefsForRequirements(refs map[string]struct{}, export theater.ExportSpec) {
	if export.Ref != nil {
		collectThroughCapabilityRefsForRequirements(refs, export.Ref.Through)
	}
	collectThroughCapabilityRefsForRequirements(refs, export.Through)
}

func collectLogValueCapabilityRefsForRequirements(refs map[string]struct{}, value theater.LogValueSpec) {
	collectThroughCapabilityRefsForRequirements(refs, value.Through)
	for name := range value.Object {
		collectLogValueCapabilityRefsForRequirements(refs, value.Object[name])
	}
	for i := range value.List {
		collectLogValueCapabilityRefsForRequirements(refs, value.List[i])
	}
}

func collectBindingMapCapabilityRefsForRequirements(refs map[string]struct{}, bindings map[string]theater.BindingSpec) {
	for name := range bindings {
		collectBindingCapabilityRefsForRequirements(refs, bindings[name])
	}
}

func collectBindingCapabilityRefsForRequirements(refs map[string]struct{}, binding theater.BindingSpec) {
	if binding.Ref != nil {
		collectThroughCapabilityRefsForRequirements(refs, binding.Ref.Through)
	}
	for name := range binding.Object {
		collectBindingCapabilityRefsForRequirements(refs, binding.Object[name])
	}
	for i := range binding.List {
		collectBindingCapabilityRefsForRequirements(refs, binding.List[i])
	}
	for i := range binding.Parts {
		collectBindingCapabilityRefsForRequirements(refs, binding.Parts[i])
	}
	for name := range binding.Args {
		collectBindingCapabilityRefsForRequirements(refs, binding.Args[name])
	}
	for i := range binding.Candidates {
		collectBindingCapabilityRefsForRequirements(refs, binding.Candidates[i])
	}
}

func collectThroughCapabilityRefsForRequirements(refs map[string]struct{}, through []theater.ThroughStepSpec) {
	for i := range through {
		step := through[i]
		if step.Transform != nil && step.Transform.Use != "" {
			refs[step.Transform.Use] = struct{}{}
		}
		if step.Pick == nil {
			continue
		}
		collectBindingCapabilityRefsForRequirements(refs, step.Pick.Equals)
		for j := range step.Pick.Where {
			collectAssertCapabilityRefsForRequirements(refs, step.Pick.Where[j].Assert)
		}
	}
}

func collectHTTPAuthSlotRequirements(repoRoot string, stage theater.StageSpec) []runtimeRequirementItem {
	if stage.HTTP == nil || len(stage.HTTP.Auth) == 0 || len(stage.ScenarioCalls) == 0 {
		return nil
	}

	selectedScenarioIDs := make(map[string]struct{}, len(stage.ScenarioCalls))
	for i := range stage.ScenarioCalls {
		selectedScenarioIDs[stage.ScenarioCalls[i].ScenarioID] = struct{}{}
	}

	requirements := make([]runtimeRequirementItem, 0)
	for i := range stage.Scenarios {
		scenario := stage.Scenarios[i]
		if _, selected := selectedScenarioIDs[scenario.ID]; !selected {
			continue
		}
		authNames := flowauth.SelectedScenarioHTTPAuthNames(stage.Scenarios[i:i+1], map[string]struct{}{scenario.ID: {}})
		for _, authName := range sortedMapKeys(authNames) {
			auth, ok := stage.HTTP.Auth[authName]
			if !ok {
				continue
			}
			binding := scenario.AuthBindings[authName]
			for _, slot := range declaredHTTPAuthSlots(auth) {
				readiness := requirementReadinessMissing
				source := sourceFor(repoRoot, "", scenario.SourceSpan)
				if bound, ok := binding.Slots[slot]; ok {
					readiness = requirementReadinessBound
					source = sourceFor(repoRoot, "", bound.SourceSpan)
				}
				requirements = append(requirements, runtimeRequirementItem{
					Kind:       "http_auth_slot",
					Name:       slot,
					Owner:      scenario.ID + ":" + authName,
					Required:   true,
					Readiness:  readiness,
					ScenarioID: scenario.ID,
					AuthName:   authName,
					Slot:       slot,
					Source:     source,
				})
			}
		}
	}
	return requirements
}

func declaredHTTPAuthSlots(auth theater.HTTPAuthSpec) []string {
	slots := make(map[string]struct{})
	for i := range auth.Attach {
		attachment := auth.Attach[i]
		switch {
		case attachment.Bearer != nil && attachment.Bearer.TokenSlot != "":
			slots[attachment.Bearer.TokenSlot] = struct{}{}
		case attachment.HeaderSlot != nil && attachment.HeaderSlot.Slot != "":
			slots[attachment.HeaderSlot.Slot] = struct{}{}
		case attachment.QuerySlot != nil && attachment.QuerySlot.Slot != "":
			slots[attachment.QuerySlot.Slot] = struct{}{}
		case attachment.FormSlot != nil && attachment.FormSlot.Slot != "":
			slots[attachment.FormSlot.Slot] = struct{}{}
		}
	}
	return sortedMapKeys(slots)
}

func renderRequirementsInventory(writer io.Writer, format outputFormat, inventory requirementsInventory) error {
	switch format {
	case outputFormatText:
		return renderRequirementsInventoryText(writer, inventory)
	case outputFormatJSON:
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(inventory)
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func renderRequirementsInventoryText(writer io.Writer, inventory requirementsInventory) error {
	status := doctorReadyStatus
	if requirementsInventoryHasMissing(inventory) {
		status = requirementReadinessMissing
	}
	if _, err := fmt.Fprintf(
		writer,
		"Runtime requirements: %s\nStatus: %s\nCheck env: %t\n\n",
		sanitizeCLIText(inventory.File),
		status,
		inventory.CheckEnv,
	); err != nil {
		return err
	}

	table := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "KIND\tNAME\tOWNER\tREADINESS\tSOURCE"); err != nil {
		return err
	}
	if len(inventory.Requirements) == 0 {
		if _, err := fmt.Fprintln(table, "-\t-\t-\tready\t-"); err != nil {
			return err
		}
	}
	for i := range inventory.Requirements {
		item := inventory.Requirements[i]
		if _, err := fmt.Fprintf(
			table,
			"%s\t%s\t%s\t%s\t%s\n",
			sanitizeCLIText(item.Kind),
			sanitizeCLIText(item.Name),
			sanitizeCLIText(item.Owner),
			sanitizeCLIText(item.Readiness),
			sanitizeCLIText(formatListedScenarioSource(item.Source)),
		); err != nil {
			return err
		}
	}
	return table.Flush()
}

func requirementsInventoryHasMissing(inventory requirementsInventory) bool {
	for i := range inventory.Requirements {
		if inventory.Requirements[i].Readiness == requirementReadinessMissing {
			return true
		}
	}
	return false
}

func sortRuntimeRequirements(requirements []runtimeRequirementItem) {
	sort.Slice(requirements, func(i, j int) bool {
		left := strings.Join([]string{
			requirements[i].Kind,
			requirements[i].Owner,
			requirements[i].Name,
		}, "\x00")
		right := strings.Join([]string{
			requirements[j].Kind,
			requirements[j].Owner,
			requirements[j].Name,
		}, "\x00")
		return left < right
	})
}
