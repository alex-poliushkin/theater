package theatercli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const completionDescriptionsEnv = "THEATER_COMPLETE_DESCRIPTIONS"
const capabilityFamilyCompletionDescription = "capability family"

type completionSuggestion struct {
	Description string
	Value       string
}

type completionPathKind string

const (
	completionPathInitTarget   completionPathKind = "init-target"
	completionPathPluginConfig completionPathKind = "plugins-config"
	completionPathPluginLock   completionPathKind = "plugins-lock"
	completionPathStage        completionPathKind = "stage"
	completionPathTHTR         completionPathKind = "thtr"
)

func (a *application) completionSuggestions(args []string) []completionSuggestion {
	if suggestions, ok := a.completionPathSuggestions(args); ok {
		return suggestions
	}

	values := a.commands.Complete(args)
	suggestions := make([]completionSuggestion, 0, len(values))
	for _, value := range values {
		suggestions = append(suggestions, completionSuggestion{
			Value:       value,
			Description: a.completionDescription(args, value),
		})
	}
	return uniqueSortedCompletionSuggestions(suggestions)
}

func (a *application) completionDescription(args []string, value string) string {
	if len(args) == 0 {
		return a.commandCompletionDescription(a.commands.root, value)
	}

	context := args[:len(args)-1]
	if len(context) != 0 && context[0] == commandHelp {
		return a.helpCompletionDescription(value)
	}
	if len(context) != 0 && context[0] == commandExplain {
		return explainCompletionDescription(value)
	}

	resolved := a.commands.resolveCompletionContext(context, a.commands.root)
	return a.commandCompletionDescription(resolved, value)
}

func (a *application) commandCompletionDescription(spec *commandSpec, value string) string {
	if spec == nil {
		return ""
	}

	for _, child := range spec.Subcommands {
		if child == nil || child.Hidden {
			continue
		}
		if child.Name == value {
			return child.Short
		}
		for _, alias := range child.Aliases {
			if alias == value {
				return "alias for " + child.Path
			}
		}
	}

	if spec.Name == commandCompletion {
		for _, suggestion := range completionShellSuggestions() {
			if suggestion.Value == value {
				return suggestion.Description
			}
		}
	}

	flags := a.helpFlagSet(spec)
	if flags == nil {
		return ""
	}

	name, isLegacyCompatibility := strings.CutPrefix(value, "-")
	if strings.HasPrefix(value, "--") {
		name = strings.TrimPrefix(value, "--")
		isLegacyCompatibility = false
	}
	item := flags.Lookup(name)
	if item == nil {
		return ""
	}
	if isLegacyCompatibility {
		return "legacy compatibility spelling for --" + name
	}
	return formatFlagDescription(item)
}

func (a *application) helpCompletionDescription(value string) string {
	if description := a.commandCompletionDescription(a.commands.root, value); description != "" {
		return description
	}

	for _, topic := range a.commands.topics {
		if topic == nil || topic.Hidden {
			continue
		}
		if topic.Name == value {
			return topic.Short
		}
		for _, alias := range topic.Aliases {
			if alias == value {
				return "alias for " + topic.Path
			}
		}
	}

	return ""
}

func explainCompletionDescription(value string) string {
	for _, family := range explainFamilies() {
		canonical := string(family.Family)
		if canonical == value {
			return capabilityFamilyCompletionDescription
		}
		for _, alias := range family.Aliases {
			if alias == value {
				if alias == canonical {
					return capabilityFamilyCompletionDescription
				}
				return "alias for " + canonical
			}
		}
	}

	for _, topic := range explainTopics() {
		if topic.Name == value {
			return topic.Short
		}
		for _, alias := range topic.Aliases {
			if alias == value {
				return "alias for " + topic.Name
			}
		}
	}

	return ""
}

func completionShellSuggestions() []completionSuggestion {
	suggestions := make([]completionSuggestion, 0, len(commandCompletionValues()))
	for _, value := range commandCompletionValues() {
		description := "generate a " + strings.ToUpper(value[:1]) + value[1:] + " completion script"
		if value == completionShellPowerShell {
			description = "generate a PowerShell completion script"
		}
		suggestions = append(suggestions, completionSuggestion{
			Value:       value,
			Description: description,
		})
	}
	return suggestions
}

func completionDescriptionEnabled() bool {
	return os.Getenv(completionDescriptionsEnv) == "1"
}

func formatCompletionSuggestion(suggestion completionSuggestion) string {
	if !completionDescriptionEnabled() || suggestion.Description == "" {
		return suggestion.Value
	}
	return suggestion.Value + "\t" + sanitizeCompletionDescription(suggestion.Description)
}

func sanitizeCompletionDescription(description string) string {
	description = strings.ReplaceAll(description, "\t", " ")
	description = strings.ReplaceAll(description, "\n", " ")
	description = strings.TrimSpace(description)
	return description
}

func completionValueIsSafe(value string) bool {
	return !strings.ContainsAny(value, "\t\n\r")
}

func uniqueSortedCompletionSuggestions(values []completionSuggestion) []completionSuggestion {
	if len(values) == 0 {
		return nil
	}

	slices.SortFunc(values, func(left, right completionSuggestion) int {
		return strings.Compare(left.Value, right.Value)
	})

	compacted := values[:0]
	for _, value := range values {
		if value.Value == "" || !completionValueIsSafe(value.Value) {
			continue
		}
		if len(compacted) != 0 && compacted[len(compacted)-1].Value == value.Value {
			if compacted[len(compacted)-1].Description == "" {
				compacted[len(compacted)-1].Description = value.Description
			}
			continue
		}
		compacted = append(compacted, value)
	}
	return compacted
}

func (a *application) completionPathSuggestions(args []string) ([]completionSuggestion, bool) {
	kind, prefix, ok := a.completionPathExpectation(args)
	if !ok {
		return nil, false
	}

	return uniqueSortedCompletionSuggestions(fileCompletionSuggestions(prefix, kind)), true
}

func (a *application) completionPathExpectation(args []string) (completionPathKind, string, bool) {
	if len(args) == 0 {
		return "", "", false
	}

	current := args[len(args)-1]
	context := args[:len(args)-1]
	if len(context) == 0 {
		return "", "", false
	}

	command, commandArgs, ok := a.completionDirectCommandContext(context)
	if !ok {
		return "", "", false
	}

	if kind, ok := pendingCompletionPathKind(command, commandArgs); ok {
		return kind, current, true
	}

	if _, _, isFlag := parseCLIFlagToken(current); isFlag {
		return "", "", false
	}

	switch command {
	case commandInit:
		if !completionInitArgsHaveTarget(commandArgs) {
			return completionPathInitTarget, current, true
		}
	case commandFmt, commandLower:
		if !completionCommandArgsHaveStagePath(command, commandArgs) {
			return completionPathTHTR, current, true
		}
	case commandRun, commandValidate, commandLibrariesInspectPath, commandRequirementsInspectPath:
		if !completionCommandArgsHaveStagePath(command, commandArgs) {
			return completionPathStage, current, true
		}
	}

	return "", "", false
}

func (a *application) completionDirectCommandContext(context []string) (command string, commandArgs []string, ok bool) {
	if len(context) == 0 {
		return "", nil, false
	}

	spec := a.commands.root.subcommand(context[0])
	if spec == nil || spec.Hidden {
		return "", nil, false
	}

	if len(context) > 1 {
		if child := spec.subcommand(context[1]); child != nil && !child.Hidden {
			return spec.Name + " " + child.Name, context[2:], true
		}
	}

	return spec.Name, context[1:], true
}

func pendingCompletionPathKind(command string, args []string) (completionPathKind, bool) {
	if len(args) == 0 {
		return "", false
	}

	name, hasInlineValue, isFlag := parseCLIFlagToken(args[len(args)-1])
	if !isFlag || hasInlineValue {
		return "", false
	}

	switch name {
	case stageFileFlag:
		switch command {
		case commandRun, commandValidate, commandLibrariesInspectPath, commandRequirementsInspectPath:
			return completionPathStage, true
		case commandFmt, commandLower:
			return completionPathTHTR, true
		}
	case explainFlagPluginsConfig:
		return completionPathPluginConfig, true
	case explainFlagPluginsLock:
		return completionPathPluginLock, true
	}

	return "", false
}

func completionCommandArgsHaveStagePath(command string, args []string) bool {
	for i := 0; i < len(args); i++ {
		raw := args[i]
		if raw == doubleDashToken {
			return i+1 < len(args)
		}

		name, hasInlineValue, isFlag := parseCLIFlagToken(raw)
		if !isFlag {
			return true
		}
		if name == stageFileFlag {
			if hasInlineValue {
				return true
			}
			return i+1 < len(args)
		}
		if hasInlineValue || isStageCommandBoolFlag(command, name) {
			continue
		}
		if isStageCommandValueFlag(command, name) && i+1 < len(args) {
			i++
		}
	}

	return false
}

func completionInitArgsHaveTarget(args []string) bool {
	for i := 0; i < len(args); i++ {
		raw := args[i]
		if raw == doubleDashToken {
			return i+1 < len(args)
		}

		name, hasInlineValue, isFlag := parseCLIFlagToken(raw)
		if !isFlag {
			return true
		}
		if hasInlineValue || name != initFlagSyntax {
			continue
		}
		if i+1 < len(args) {
			i++
		}
	}

	return false
}

func fileCompletionSuggestions(prefix string, kind completionPathKind) []completionSuggestion {
	if kind == completionPathInitTarget {
		return initTargetCompletionSuggestions(prefix)
	}

	return existingFileCompletionSuggestions(prefix, kind)
}

func initTargetCompletionSuggestions(prefix string) []completionSuggestion {
	root := filepath.FromSlash(repoLayoutRootName + "/" + repoLayoutFlowRootName)
	if prefix == "" || root == prefix || strings.HasPrefix(root, prefix) {
		return []completionSuggestion{{
			Value:       root + string(filepath.Separator),
			Description: "directory",
		}}
	}
	if !strings.HasPrefix(prefix, root+string(filepath.Separator)) {
		return nil
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return nil
	}
	workspaceRoot := workingDir
	if layout, ok := resolveRepoLayout(workingDir); ok {
		workspaceRoot = layout.RepoRoot
	}

	pattern := filepath.Join(workspaceRoot, prefix) + "*"
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}

	suggestions := make([]completionSuggestion, 0, len(matches))
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			continue
		}

		relativeMatch, err := filepath.Rel(workspaceRoot, match)
		if err != nil {
			continue
		}
		if info.IsDir() {
			suggestions = append(suggestions, completionSuggestion{
				Value:       relativeMatch + string(filepath.Separator),
				Description: "directory",
			})
			continue
		}
		if !completionPathMatchesKind(relativeMatch, completionPathStage) {
			continue
		}
		suggestions = append(suggestions, completionSuggestion{
			Value:       relativeMatch,
			Description: completionPathDescription(completionPathInitTarget),
		})
	}

	return suggestions
}

func existingFileCompletionSuggestions(prefix string, kind completionPathKind) []completionSuggestion {
	pattern := prefix + "*"
	if prefix == "" {
		pattern = "*"
	}

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}

	suggestions := make([]completionSuggestion, 0, len(matches))
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			continue
		}

		if info.IsDir() {
			suggestions = append(suggestions, completionSuggestion{
				Value:       match + string(filepath.Separator),
				Description: "directory",
			})
			continue
		}
		if !completionPathMatchesKind(match, kind) {
			continue
		}
		suggestions = append(suggestions, completionSuggestion{
			Value:       match,
			Description: completionPathDescription(kind),
		})
	}

	return suggestions
}

func completionPathMatchesKind(path string, kind completionPathKind) bool {
	lowerPath := strings.ToLower(path)

	switch strings.ToLower(filepath.Ext(path)) {
	case yamlFileExtension, ymlFileExtension:
		return kind == completionPathInitTarget || kind == completionPathStage
	case thtrFileExtension:
		return kind == completionPathInitTarget || kind == completionPathStage || kind == completionPathTHTR
	case ".json":
		if kind == completionPathPluginLock {
			return strings.HasSuffix(lowerPath, ".lock.json")
		}
		return kind == completionPathPluginConfig && !strings.HasSuffix(lowerPath, ".lock.json")
	default:
		return false
	}
}

func completionPathDescription(kind completionPathKind) string {
	switch kind {
	case completionPathInitTarget:
		return "stage file under theater/flows/"
	case completionPathPluginConfig:
		return "plugin config path"
	case completionPathPluginLock:
		return "plugin lock path"
	case completionPathTHTR:
		return ".thtr stage file"
	default:
		return "stage file"
	}
}
