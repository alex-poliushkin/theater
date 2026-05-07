package theatercli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/builtin"
)

const (
	explainFlagPluginsConfig = "plugins-config"
	explainFlagPluginsLock   = "plugins-lock"
)

type explainTargetFamily struct {
	Aliases []string
	Family  theater.CapabilityFamily
}

type explainTopic struct {
	Aliases  []string
	Name     string
	Short    string
	Sections []commandHelpSection
}

type explainCapabilityMatch struct {
	Descriptor theater.CapabilityDescriptor
	LocalRef   string
}

func (a *application) explainCommand(args []string) int {
	options, targets, ok := a.parseExplainOptions(args)
	if !ok {
		return exitCodeCommandError
	}
	style := a.textStyler(a.stdout)

	if len(targets) == 1 {
		if topic, ok := lookupExplainTopic(targets[0]); ok {
			return writeExplainTopic(a.stdout, topic, style)
		}
	}
	if len(targets) == 2 {
		if _, ok := normalizeExplainFamily(targets[0]); !ok {
			fmt.Fprintf(a.stderr, "unknown explain family %q\n", targets[0])
			fmt.Fprintln(a.stderr, `Use "theater explain" to list capability families and topics.`)
			return exitCodeCommandError
		}
	}
	if len(targets) > 2 {
		fmt.Fprintln(a.stderr, "explain accepts at most two targets")
		return exitCodeCommandError
	}

	services, err := a.ensureServices(options.pluginsConfig, options.pluginsLock)
	if err != nil {
		fmt.Fprintf(a.stderr, "build built-in catalogs: %v\n", err)
		return exitCodeCommandError
	}

	descriptors := capabilityDescriptors(services)
	switch len(targets) {
	case 0:
		return writeExplainOverview(a.stdout, descriptors, style)
	case 1:
		target := targets[0]
		if family, ok := normalizeExplainFamily(target); ok {
			return writeExplainFamily(a.stdout, family, descriptors, style)
		}
		matches := findUnscopedCapabilityMatches(descriptors, target)
		if len(matches) != 0 {
			return writeExplainCapabilityMatches(a.stdout, target, matches, style)
		}
		fmt.Fprintf(a.stderr, "unknown explain target %q\n", target)
		fmt.Fprintln(a.stderr, `Use "theater explain" to list capability families and topics.`)
		return exitCodeCommandError
	case 2:
		family, _ := normalizeExplainFamily(targets[0])
		matches := findScopedCapabilityMatches(descriptors, family, targets[1])
		switch len(matches) {
		case 0:
			fmt.Fprintf(a.stderr, "unknown %s capability %q\n", family, targets[1])
			fmt.Fprintf(a.stderr, "Use \"theater explain %s\" to list loaded capabilities in this family.\n", family)
			return exitCodeCommandError
		case 1:
			return writeExplainCapability(a.stdout, matches[0].Descriptor, style)
		default:
			return writeExplainCapabilityMatches(a.stdout, targets[1], matches, style)
		}
	default:
		fmt.Fprintln(a.stderr, "explain accepts at most two targets")
		return exitCodeCommandError
	}
}

func (a *application) parseExplainOptions(args []string) (globalOptions, []string, bool) {
	normalizedArgs, err := normalizeExplainArgs(args)
	if err != nil {
		fmt.Fprintf(a.stderr, "%v\n", err)
		return globalOptions{}, nil, false
	}

	flags, options := a.newExplainCommandFlagSet()
	if err := flags.Parse(normalizedArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return globalOptions{}, nil, false
		}
		return globalOptions{}, nil, false
	}

	resolved := sharedGlobalOptionContract.Resolve(*options)
	if resolved.pluginsConfig == "" && resolved.pluginsLock != "" {
		fmt.Fprintln(a.stderr, "explain requires --plugins-config when --plugins-lock is set")
		return globalOptions{}, nil, false
	}
	if flags.NArg() > 2 {
		fmt.Fprintln(a.stderr, "explain accepts at most two targets")
		return globalOptions{}, nil, false
	}

	return resolved, flags.Args(), true
}

func capabilityDescriptors(services *builtinServices) []theater.CapabilityDescriptor {
	if services.pluginCatalog != nil {
		return theater.DescribePluginCapabilities(services.pluginCatalog)
	}

	return theater.DescribeCapabilities(services.catalog, services.matcherSugar)
}

func writeExplainOverview(writer io.Writer, descriptors []theater.CapabilityDescriptor, style cliTextStyler) int {
	var builder strings.Builder
	builder.WriteString("Explain discoverable runtime capabilities and CLI topics.\n")

	fmt.Fprintf(&builder, "\n%s\n", style.Heading("Capability families:"))
	table := tabwriter.NewWriter(&builder, 0, 0, 2, ' ', 0)
	for _, family := range theater.CapabilityFamilies() {
		count, example := capabilityFamilySummary(family, descriptors)
		fmt.Fprintf(table, "  %s\t%d", family, count)
		if example != "" {
			fmt.Fprintf(table, "\te.g. %s", sanitizeExplainText(example))
		}
		fmt.Fprintln(table)
	}
	_ = table.Flush()

	fmt.Fprintf(&builder, "\n%s\n", style.Heading("Topics:"))
	topicWriter := tabwriter.NewWriter(&builder, 0, 0, 2, ' ', 0)
	topics := explainTopics()
	for i := range topics {
		topic := topics[i]
		fmt.Fprintf(topicWriter, "  %s\t%s\n", topic.Name, topic.Short)
	}
	_ = topicWriter.Flush()

	builder.WriteString(
		"\nUse `theater explain <family>` to list a family, `theater explain <family> <ref>` " +
			"for details, `theater explain <query>` to search, or `theater explain <topic>` for CLI-specific guidance.\n",
	)
	_, _ = io.WriteString(writer, builder.String())
	return 0
}

func writeExplainFamily(
	writer io.Writer,
	family theater.CapabilityFamily,
	descriptors []theater.CapabilityDescriptor,
	style cliTextStyler,
) int {
	filtered := filterCapabilityFamily(descriptors, family)

	var builder strings.Builder
	fmt.Fprintf(&builder, "Family: %s\n", family)
	if len(filtered) == 0 {
		fmt.Fprintf(&builder, "\nNo capabilities are currently loaded for %s.\n", family)
		_, _ = io.WriteString(writer, builder.String())
		return 0
	}

	fmt.Fprintf(&builder, "\n%s\n", style.Heading(fmt.Sprintf("Capabilities (%d):", len(filtered))))
	table := tabwriter.NewWriter(&builder, 0, 0, 2, ' ', 0)
	for i := range filtered {
		descriptor := filtered[i]
		fmt.Fprintf(table, "  %s\t%s", sanitizeExplainText(descriptor.Ref), formatCapabilityProvider(descriptor.Provider))
		if descriptor.Summary != "" {
			fmt.Fprintf(table, "\t%s", sanitizeExplainText(descriptor.Summary))
		}
		fmt.Fprintln(table)
	}
	_ = table.Flush()

	fmt.Fprintf(&builder, "\nUse `theater explain %s <ref>` to inspect one capability contract.\n", family)
	_, _ = io.WriteString(writer, builder.String())
	return 0
}

func writeExplainCapabilityMatches(
	writer io.Writer,
	query string,
	matches []explainCapabilityMatch,
	style cliTextStyler,
) int {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Matches for %q:\n", sanitizeExplainText(query))
	fmt.Fprintf(&builder, "\n%s\n", style.Heading("Capabilities:"))
	table := tabwriter.NewWriter(&builder, 0, 0, 2, ' ', 0)
	for i := range matches {
		match := matches[i]
		fmt.Fprintf(
			table,
			"  %s\t%s\t%s\n",
			match.Descriptor.Family,
			sanitizeExplainText(match.Descriptor.Ref),
			sanitizeExplainText(explainInspectCommand(match)),
		)
	}
	_ = table.Flush()

	builder.WriteString("\nUse `theater explain <family> <ref>` to inspect one capability contract.\n")
	_, _ = io.WriteString(writer, builder.String())
	return 0
}

func writeExplainCapability(writer io.Writer, descriptor theater.CapabilityDescriptor, style cliTextStyler) int {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Capability: %s\n", sanitizeExplainText(descriptor.Ref))
	fmt.Fprintf(&builder, "\n%s\n  %s\n", style.Heading("Family:"), descriptor.Family)
	fmt.Fprintf(&builder, "\n%s\n  %s\n", style.Heading("Provider:"), formatCapabilityProvider(descriptor.Provider))
	if descriptor.Summary != "" {
		fmt.Fprintf(&builder, "\n%s\n  %s\n", style.Heading("Summary:"), sanitizeExplainText(descriptor.Summary))
	}

	switch {
	case descriptor.Action != nil:
		renderValueContractTable(&builder, "Inputs", descriptor.Action.Inputs)
		renderValueContractTable(&builder, "Outputs", descriptor.Action.Outputs)
	case descriptor.Generator != nil:
		renderArgSpecTable(&builder, "Args", descriptor.Generator.Args)
		renderValueContract(&builder, "Produces", descriptor.Generator.Produces)
	case descriptor.Inventory != nil:
		renderArgSpecTable(&builder, "Args", descriptor.Inventory.Args)
		renderValueContract(&builder, "Produces", descriptor.Inventory.Produces)
	case descriptor.Matcher != nil:
		renderMatcherArgTable(&builder, "Args", descriptor.Matcher.Args)
		renderValueContract(&builder, "Actual", descriptor.Matcher.Actual)
		renderSugarSpec(&builder, descriptor.Matcher.Sugar)
	case descriptor.ReportExporter != nil:
		renderParamSpecTable(&builder, "Params", descriptor.ReportExporter.Params)
	case descriptor.StateBackend != nil:
		renderParamSpecTable(&builder, "Params", descriptor.StateBackend.Params)
		renderStateDescriptor(&builder, descriptor.StateBackend.Descriptor)
	case descriptor.Transform != nil:
		renderParamSpecTable(&builder, "Params", descriptor.Transform.Params)
		renderValueContract(&builder, "Accepts", descriptor.Transform.Accepts)
		renderValueContract(&builder, "Produces", descriptor.Transform.Produces)
	}

	_, _ = io.WriteString(writer, builder.String())
	return 0
}

func writeExplainTopic(writer io.Writer, topic explainTopic, style cliTextStyler) int {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Topic: %s\n", topic.Name)
	fmt.Fprintf(&builder, "\n%s\n", topic.Short)
	renderHelpSections(&builder, topic.Sections, style)
	_, _ = io.WriteString(writer, builder.String())
	return 0
}

func renderArgSpecTable(builder *strings.Builder, title string, specs []theater.ArgSpec) {
	if len(specs) == 0 {
		return
	}

	fmt.Fprintf(builder, "\n%s:\n", title)
	writer := tabwriter.NewWriter(builder, 0, 0, 2, ' ', 0)
	for i := range specs {
		spec := specs[i]
		fmt.Fprintf(writer, "  %s\t%s\n", sanitizeExplainText(spec.Name), formatValueContract(spec.Accepts, spec.Required))
	}
	_ = writer.Flush()
}

func renderMatcherArgTable(builder *strings.Builder, title string, specs []theater.MatcherArg) {
	if len(specs) == 0 {
		return
	}

	fmt.Fprintf(builder, "\n%s:\n", title)
	writer := tabwriter.NewWriter(builder, 0, 0, 2, ' ', 0)
	for i := range specs {
		spec := specs[i]
		fmt.Fprintf(writer, "  %s\t%s\n", sanitizeExplainText(spec.Name), formatValueContract(spec.Accepts, spec.Required))
	}
	_ = writer.Flush()
}

func renderParamSpecTable(builder *strings.Builder, title string, specs []theater.ParamSpec) {
	if len(specs) == 0 {
		return
	}

	fmt.Fprintf(builder, "\n%s:\n", title)
	writer := tabwriter.NewWriter(builder, 0, 0, 2, ' ', 0)
	for i := range specs {
		spec := specs[i]
		fmt.Fprintf(writer, "  %s\t%s\n", sanitizeExplainText(spec.Name), formatValueContract(spec.Accepts, spec.Required))
	}
	_ = writer.Flush()
}

func renderValueContractTable(builder *strings.Builder, title string, contracts map[string]theater.ValueContract) {
	if len(contracts) == 0 {
		return
	}

	fmt.Fprintf(builder, "\n%s:\n", title)
	names := make([]string, 0, len(contracts))
	for name := range contracts {
		names = append(names, name)
	}
	sort.Strings(names)

	writer := tabwriter.NewWriter(builder, 0, 0, 2, ' ', 0)
	for _, name := range names {
		fmt.Fprintf(writer, "  %s\t%s\n", sanitizeExplainText(name), formatValueContract(contracts[name], false))
	}
	_ = writer.Flush()
}

func renderValueContract(builder *strings.Builder, title string, contract theater.ValueContract) {
	if contract.Kind == "" && len(contract.Kinds) == 0 {
		return
	}

	fmt.Fprintf(builder, "\n%s:\n  %s\n", title, formatValueContract(contract, false))
}

func renderStateDescriptor(builder *strings.Builder, descriptor theater.StateDescriptor) {
	fmt.Fprintf(builder, "\nState behavior:\n  guarantee: %s\n", descriptor.Guarantee)
	features := make([]string, 0, 5)
	if descriptor.SupportsCAS {
		features = append(features, "cas")
	}
	if descriptor.SupportsClaim {
		features = append(features, "claim")
	}
	if descriptor.SupportsRenew {
		features = append(features, "renew")
	}
	if descriptor.SupportsRelease {
		features = append(features, "release")
	}
	if descriptor.SupportsConsume {
		features = append(features, "consume")
	}
	if len(features) != 0 {
		fmt.Fprintf(builder, "  operations: %s\n", strings.Join(features, ", "))
	}
}

func renderSugarSpec(builder *strings.Builder, sugar theater.SugarSpec) {
	if sugar.Form == theater.SugarFormNone && len(sugar.Keys) == 0 && len(sugar.PositionalArgs) == 0 {
		return
	}

	fmt.Fprintf(builder, "\nSugar:\n  form: %s\n", sugar.Form)
	if len(sugar.Keys) != 0 {
		fmt.Fprintf(builder, "  keys: %s\n", sanitizeExplainList(sugar.Keys))
	}
	if len(sugar.PositionalArgs) != 0 {
		fmt.Fprintf(builder, "  positional args: %s\n", sanitizeExplainList(sugar.PositionalArgs))
	}
}

func capabilityFamilySummary(family theater.CapabilityFamily, descriptors []theater.CapabilityDescriptor) (count int, example string) {
	filtered := filterCapabilityFamily(descriptors, family)
	if len(filtered) == 0 {
		return 0, ""
	}

	return len(filtered), filtered[0].Ref
}

func filterCapabilityFamily(descriptors []theater.CapabilityDescriptor, family theater.CapabilityFamily) []theater.CapabilityDescriptor {
	filtered := make([]theater.CapabilityDescriptor, 0)
	for i := range descriptors {
		descriptor := descriptors[i]
		if descriptor.Family == family {
			filtered = append(filtered, descriptor)
		}
	}

	return filtered
}

func findScopedCapabilityMatches(
	descriptors []theater.CapabilityDescriptor,
	family theater.CapabilityFamily,
	query string,
) []explainCapabilityMatch {
	matches := make([]explainCapabilityMatch, 0)
	for i := range descriptors {
		descriptor := descriptors[i]
		if descriptor.Family != family {
			continue
		}
		for _, candidate := range scopedCapabilityRefCandidates(descriptor.Family, descriptor.Ref) {
			if candidate == query {
				matches = append(matches, explainCapabilityMatch{
					Descriptor: descriptor,
					LocalRef:   primaryScopedCapabilityRef(descriptor.Family, descriptor.Ref),
				})
				break
			}
		}
	}
	return sortExplainCapabilityMatches(matches)
}

func findUnscopedCapabilityMatches(descriptors []theater.CapabilityDescriptor, query string) []explainCapabilityMatch {
	matches := make([]explainCapabilityMatch, 0)
	seen := make(map[string]struct{})
	for i := range descriptors {
		descriptor := descriptors[i]
		candidates := scopedCapabilityRefCandidates(descriptor.Family, descriptor.Ref)
		for _, candidate := range candidates {
			if candidate != query && !strings.Contains(candidate, query) {
				continue
			}
			key := string(descriptor.Family) + "\x00" + descriptor.Ref
			if _, ok := seen[key]; ok {
				break
			}
			seen[key] = struct{}{}
			matches = append(matches, explainCapabilityMatch{
				Descriptor: descriptor,
				LocalRef:   primaryScopedCapabilityRef(descriptor.Family, descriptor.Ref),
			})
			break
		}
	}
	return sortExplainCapabilityMatches(matches)
}

func sortExplainCapabilityMatches(matches []explainCapabilityMatch) []explainCapabilityMatch {
	sort.SliceStable(matches, func(i, j int) bool {
		left := matches[i].Descriptor
		right := matches[j].Descriptor
		if left.Family != right.Family {
			return left.Family < right.Family
		}
		return left.Ref < right.Ref
	})
	return matches
}

func explainInspectCommand(match explainCapabilityMatch) string {
	return fmt.Sprintf("theater explain %s %s", match.Descriptor.Family, match.LocalRef)
}

func primaryScopedCapabilityRef(family theater.CapabilityFamily, ref string) string {
	for _, candidate := range scopedCapabilityRefCandidates(family, ref) {
		if candidate != ref {
			return candidate
		}
	}
	return ref
}

func scopedCapabilityRefCandidates(family theater.CapabilityFamily, ref string) []string {
	candidates := []string{ref}
	for _, prefix := range scopedCapabilityRefPrefixes(family) {
		if local := strings.TrimPrefix(ref, prefix); local != ref && local != "" {
			candidates = append(candidates, local)
		}
	}
	return uniqueSorted(candidates)
}

func scopedCapabilityRefPrefixes(family theater.CapabilityFamily) []string {
	switch family {
	case theater.CapabilityFamilyAction:
		return []string{"action."}
	case theater.CapabilityFamilyInventory:
		return []string{"inventory."}
	case theater.CapabilityFamilyMatcher:
		return []string{"expectation.", "matcher."}
	case theater.CapabilityFamilyReportExporter:
		return []string{"report_exporter.", "report-exporter.", "report.exporter."}
	case theater.CapabilityFamilyStateBackend:
		return []string{"state.backend.", "state-backend.", "state_backend."}
	case theater.CapabilityFamilyTransform:
		return []string{"transform."}
	default:
		return nil
	}
}

func formatCapabilityProvider(provider theater.CapabilityProvider) string {
	if provider.Kind == theater.CapabilityProviderPlugin {
		return fmt.Sprintf(
			"plugin %s@%s",
			sanitizeExplainText(provider.PluginID),
			sanitizeExplainText(provider.PluginVersion),
		)
	}

	return string(provider.Kind)
}

func formatValueContract(contract theater.ValueContract, required bool) string {
	parts := []string{formatValueKinds(contract)}
	if required || contract.Required {
		parts = append(parts, "required")
	}
	if contract.Sensitivity != "" {
		parts = append(parts, "sensitivity="+string(contract.Sensitivity))
	}
	if contract.Capture != "" {
		parts = append(parts, "capture="+string(contract.Capture))
	}
	if contract.Description != "" {
		parts = append(parts, sanitizeExplainText(contract.Description))
	}

	return strings.Join(parts, "; ")
}

func formatValueKinds(contract theater.ValueContract) string {
	kinds := contract.KindsSet()
	if len(kinds) == 0 {
		return "any"
	}

	ordered := []theater.ValueKind{
		theater.ValueKindAny,
		theater.ValueKindString,
		theater.ValueKindNumber,
		theater.ValueKindBool,
		theater.ValueKindBytes,
		theater.ValueKindObject,
		theater.ValueKindList,
		theater.ValueKindNull,
	}

	labels := make([]string, 0, len(kinds))
	for _, kind := range ordered {
		if kinds.Contains(kind) {
			labels = append(labels, string(kind))
		}
	}

	return strings.Join(labels, "|")
}

func normalizeExplainFamily(raw string) (theater.CapabilityFamily, bool) {
	for _, candidate := range explainFamilies() {
		if slices.Contains(candidate.Aliases, raw) {
			return candidate.Family, true
		}
	}

	return "", false
}

func lookupExplainTopic(raw string) (explainTopic, bool) {
	for _, topic := range explainTopics() {
		if topic.Name == raw || slices.Contains(topic.Aliases, raw) {
			return topic, true
		}
	}

	return explainTopic{}, false
}

func explainCompletionTargets() []string {
	candidates := make([]string, 0, 16)
	families := explainFamilies()
	for i := range families {
		family := families[i]
		candidates = append(candidates, family.Aliases...)
	}
	topics := explainTopics()
	for i := range topics {
		topic := topics[i]
		candidates = append(candidates, topic.Name)
		candidates = append(candidates, topic.Aliases...)
	}

	return uniqueSorted(candidates)
}

func explainFamilyCompletionTargets(family theater.CapabilityFamily) []string {
	bundle, err := builtin.NewBundle()
	if err != nil {
		return nil
	}
	descriptors := theater.DescribeCapabilities(bundle.Catalog, bundle.Matchers)
	candidates := make([]string, 0)
	for i := range descriptors {
		descriptor := descriptors[i]
		if descriptor.Family != family {
			continue
		}
		candidates = append(candidates, scopedCapabilityRefCandidates(descriptor.Family, descriptor.Ref)...)
	}
	return uniqueSorted(candidates)
}

func explainFamilies() []explainTargetFamily {
	return []explainTargetFamily{
		{Family: theater.CapabilityFamilyAction, Aliases: []string{"action", "actions"}},
		{Family: theater.CapabilityFamilyInventory, Aliases: []string{"inventory", "inventories"}},
		{Family: theater.CapabilityFamilyGenerator, Aliases: []string{"generator", "generators"}},
		{Family: theater.CapabilityFamilyTransform, Aliases: []string{"transform", "transforms", "decorator", "decorators"}},
		{Family: theater.CapabilityFamilyMatcher, Aliases: []string{"matcher", "matchers"}},
		{
			Family:  theater.CapabilityFamilyReportExporter,
			Aliases: []string{"report-exporter", "report-exporters", "report_exporter", "report_exporters"},
		},
		{
			Family:  theater.CapabilityFamilyStateBackend,
			Aliases: []string{"state-backend", "state-backends", "state_backend", "state_backends"},
		},
	}
}

func explainTopics() []explainTopic {
	return []explainTopic{
		formatsExplainTopic(),
	}
}

func normalizeExplainArgs(args []string) ([]string, error) {
	normalized := make([]string, 0, len(args))
	targets := make([]string, 0, 2)

	for i := 0; i < len(args); i++ {
		raw := args[i]
		if raw == doubleDashToken {
			rest := args[i+1:]
			if len(rest) == 0 {
				break
			}
			targets = append(targets, rest...)
			if len(targets) > 2 {
				return nil, errors.New("explain accepts at most two targets")
			}
			break
		}

		name, hasInlineValue, isFlag := parseCLIFlagToken(raw)
		if !isFlag {
			targets = append(targets, raw)
			if len(targets) > 2 {
				return nil, errors.New("explain accepts at most two targets")
			}
			continue
		}

		normalized = append(normalized, raw)
		if hasInlineValue || !isExplainValueFlag(name) {
			continue
		}
		if i+1 < len(args) {
			i++
			normalized = append(normalized, args[i])
		}
	}

	normalized = append(normalized, targets...)

	return normalized, nil
}

func isExplainValueFlag(name string) bool {
	switch name {
	case explainFlagPluginsConfig, explainFlagPluginsLock:
		return true
	default:
		return false
	}
}

func sanitizeExplainList(values []string) string {
	sanitized := make([]string, 0, len(values))
	for i := range values {
		sanitized = append(sanitized, sanitizeExplainText(values[i]))
	}

	return strings.Join(sanitized, ", ")
}

func sanitizeExplainText(raw string) string {
	return sanitizeCLIText(raw)
}
