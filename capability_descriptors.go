package theater

import (
	"sort"
	"strings"

	internalpluginregistry "github.com/alex-poliushkin/theater/internal/pluginregistry"
)

const (
	CapabilityFamilyAction         CapabilityFamily = "action"
	CapabilityFamilyGenerator      CapabilityFamily = "generator"
	CapabilityFamilyInventory      CapabilityFamily = "inventory"
	CapabilityFamilyMatcher        CapabilityFamily = "matcher"
	CapabilityFamilyReportExporter CapabilityFamily = "report-exporter"
	CapabilityFamilyStateBackend   CapabilityFamily = "state-backend"
	CapabilityFamilyTransform      CapabilityFamily = "transform"

	CapabilityProviderBuiltin CapabilityProviderKind = "builtin"
	CapabilityProviderPlugin  CapabilityProviderKind = "plugin"
)

// CapabilityFamily identifies one discoverable runtime capability family.
type CapabilityFamily string

// CapabilityProviderKind identifies where one capability came from.
type CapabilityProviderKind string

// CapabilityProvider describes whether a capability is built-in or comes from a
// loaded plugin overlay.
type CapabilityProvider struct {
	Kind          CapabilityProviderKind
	PluginID      string
	PluginVersion string
}

// CapabilityDescriptor describes one discoverable runtime capability together
// with the contract relevant to its family.
type CapabilityDescriptor struct {
	Family         CapabilityFamily
	Ref            string
	Summary        string
	Provider       CapabilityProvider
	Action         *ActionCapabilityDescriptor
	Generator      *GeneratorCapabilityDescriptor
	Inventory      *InventoryCapabilityDescriptor
	Matcher        *MatcherCapabilityDescriptor
	ReportExporter *ReportExporterCapabilityDescriptor
	StateBackend   *StateBackendCapabilityDescriptor
	Transform      *TransformCapabilityDescriptor
}

// ActionCapabilityDescriptor describes one action contract.
type ActionCapabilityDescriptor struct {
	Inputs  map[string]ValueContract
	Outputs map[string]ValueContract
}

// GeneratorCapabilityDescriptor describes one generator contract.
type GeneratorCapabilityDescriptor struct {
	Args     []ArgSpec
	Produces ValueContract
}

// InventoryCapabilityDescriptor describes one inventory contract.
type InventoryCapabilityDescriptor struct {
	Args     []ArgSpec
	Produces ValueContract
}

// MatcherCapabilityDescriptor describes one matcher contract.
type MatcherCapabilityDescriptor struct {
	Args   []MatcherArg
	Actual ValueContract
	Sugar  SugarSpec
}

// ReportExporterCapabilityDescriptor describes one report exporter contract.
type ReportExporterCapabilityDescriptor struct {
	Params []ParamSpec
}

// StateBackendCapabilityDescriptor describes one persistent-state backend
// contract.
type StateBackendCapabilityDescriptor struct {
	Descriptor StateDescriptor
	Params     []ParamSpec
}

// TransformCapabilityDescriptor describes one value transform contract.
type TransformCapabilityDescriptor struct {
	Accepts  ValueContract
	Params   []ParamSpec
	Produces ValueContract
}

// CapabilityFamilies returns the canonical family order used by discovery and
// CLI rendering.
func CapabilityFamilies() []CapabilityFamily {
	return []CapabilityFamily{
		CapabilityFamilyAction,
		CapabilityFamilyInventory,
		CapabilityFamilyGenerator,
		CapabilityFamilyTransform,
		CapabilityFamilyMatcher,
		CapabilityFamilyReportExporter,
		CapabilityFamilyStateBackend,
	}
}

// DescribeCapabilities returns the discoverable built-in runtime capabilities
// from the provided catalog and matcher catalog.
func DescribeCapabilities(catalog *Catalog, matchers *MatcherCatalog) []CapabilityDescriptor {
	provider := CapabilityProvider{Kind: CapabilityProviderBuiltin}
	descriptors := catalog.capabilityDescriptors(provider)
	descriptors = append(descriptors, matcherCapabilityDescriptors(matchers, provider)...)
	sortCapabilityDescriptors(descriptors)
	return descriptors
}

// DescribePluginCapabilities returns the discoverable capability surface for a
// plugin overlay, including the base built-ins and plugin-provided additions.
func DescribePluginCapabilities(catalog *PluginCatalog) []CapabilityDescriptor {
	if catalog == nil {
		return nil
	}

	descriptors := DescribeCapabilities(catalog.base, catalog.baseMatchers)
	descriptors = append(descriptors, catalog.pluginCapabilityDescriptors()...)
	sortCapabilityDescriptors(descriptors)
	return descriptors
}

func (c *Catalog) capabilityDescriptors(provider CapabilityProvider) []CapabilityDescriptor {
	if c == nil {
		return nil
	}

	descriptors := make([]CapabilityDescriptor, 0)
	actionRefs := sortedMapKeys(c.actions)
	for _, ref := range actionRefs {
		contract := c.actions[ref].Contract()
		descriptors = append(descriptors, CapabilityDescriptor{
			Family:   CapabilityFamilyAction,
			Ref:      ref,
			Provider: provider,
			Action: &ActionCapabilityDescriptor{
				Inputs:  cloneValueContracts(contract.Inputs),
				Outputs: cloneValueContracts(contract.Outputs),
			},
		})
	}

	generatorRefs := sortedMapKeys(c.generators)
	for _, ref := range generatorRefs {
		def := c.generators[ref]
		descriptors = append(descriptors, CapabilityDescriptor{
			Family:   CapabilityFamilyGenerator,
			Ref:      ref,
			Summary:  strings.TrimSpace(def.Contract.Summary),
			Provider: provider,
			Generator: &GeneratorCapabilityDescriptor{
				Args:     cloneArgSpecs(def.Contract.Args),
				Produces: cloneValueContract(def.Contract.Produces),
			},
		})
	}

	inventoryRefs := sortedMapKeys(c.inventories)
	for _, ref := range inventoryRefs {
		contract := c.inventories[ref].Contract()
		descriptors = append(descriptors, CapabilityDescriptor{
			Family:   CapabilityFamilyInventory,
			Ref:      ref,
			Summary:  strings.TrimSpace(contract.Summary),
			Provider: provider,
			Inventory: &InventoryCapabilityDescriptor{
				Args:     cloneArgSpecs(contract.Args),
				Produces: cloneValueContract(contract.Produces),
			},
		})
	}

	reportExporterRefs := sortedMapKeys(c.reportExporters)
	for _, ref := range reportExporterRefs {
		def := c.reportExporters[ref]
		descriptors = append(descriptors, CapabilityDescriptor{
			Family:   CapabilityFamilyReportExporter,
			Ref:      ref,
			Provider: provider,
			ReportExporter: &ReportExporterCapabilityDescriptor{
				Params: cloneCapabilityParamSpecs(def.Params),
			},
		})
	}

	stateBackendRefs := sortedMapKeys(c.stateBackends)
	for _, ref := range stateBackendRefs {
		def := c.stateBackends[ref]
		descriptor := describeStateBackend(def)
		descriptors = append(descriptors, CapabilityDescriptor{
			Family:   CapabilityFamilyStateBackend,
			Ref:      ref,
			Provider: provider,
			StateBackend: &StateBackendCapabilityDescriptor{
				Descriptor: descriptor,
				Params:     cloneCapabilityParamSpecs(def.Params),
			},
		})
	}

	transformRefs := sortedMapKeys(c.decorators)
	for _, ref := range transformRefs {
		contract := c.decorators[ref].Contract
		descriptors = append(descriptors, CapabilityDescriptor{
			Family:   CapabilityFamilyTransform,
			Ref:      ref,
			Summary:  strings.TrimSpace(contract.Summary),
			Provider: provider,
			Transform: &TransformCapabilityDescriptor{
				Accepts:  cloneValueContract(contract.Accepts),
				Params:   cloneCapabilityParamSpecs(contract.Params),
				Produces: cloneValueContract(contract.Produces),
			},
		})
	}

	return descriptors
}

func (c *PluginCatalog) pluginCapabilityDescriptors() []CapabilityDescriptor {
	if c == nil {
		return nil
	}

	descriptors := make([]CapabilityDescriptor, 0)
	pluginIDs := sortedMapKeys(c.loaded.Plugins)
	for _, pluginID := range pluginIDs {
		plugin := c.loaded.Plugins[pluginID]
		for _, name := range plugin.Config.AllowCapabilities {
			entry := c.capabilities[name]
			descriptors = append(descriptors, describePluginCapability(plugin, entry))
		}
	}

	return descriptors
}

func matcherCapabilityDescriptors(matchers *MatcherCatalog, provider CapabilityProvider) []CapabilityDescriptor {
	if matchers == nil {
		return nil
	}

	descriptors := make([]CapabilityDescriptor, 0)
	matcherDescriptors := matchers.Descriptors()
	for i := range matcherDescriptors {
		descriptor := matcherDescriptors[i]
		descriptors = append(descriptors, CapabilityDescriptor{
			Family:   CapabilityFamilyMatcher,
			Ref:      descriptor.Ref,
			Summary:  strings.TrimSpace(descriptor.Summary),
			Provider: provider,
			Matcher: &MatcherCapabilityDescriptor{
				Args:   cloneMatcherArgs(descriptor.Args),
				Actual: cloneValueContract(descriptor.Actual),
				Sugar:  cloneSugarSpec(descriptor.Sugar),
			},
		})
	}

	return descriptors
}

func describePluginCapability(
	plugin internalpluginregistry.LoadedPlugin,
	entry pluginCapabilityEntry,
) CapabilityDescriptor {
	provider := CapabilityProvider{
		Kind:          CapabilityProviderPlugin,
		PluginID:      plugin.ID,
		PluginVersion: plugin.Manifest.Plugin.Version,
	}

	switch entry.capability.Manifest.Kind {
	case "action":
		contract := entry.action.Contract()
		return CapabilityDescriptor{
			Family:   CapabilityFamilyAction,
			Ref:      entry.capability.Manifest.Name,
			Summary:  strings.TrimSpace(entry.capability.Manifest.Summary),
			Provider: provider,
			Action: &ActionCapabilityDescriptor{
				Inputs:  cloneValueContracts(contract.Inputs),
				Outputs: cloneValueContracts(contract.Outputs),
			},
		}
	case "inventory":
		contract := entry.inventory.Contract()
		return CapabilityDescriptor{
			Family:   CapabilityFamilyInventory,
			Ref:      entry.capability.Manifest.Name,
			Summary:  strings.TrimSpace(entry.capability.Manifest.Summary),
			Provider: provider,
			Inventory: &InventoryCapabilityDescriptor{
				Args:     cloneArgSpecs(contract.Args),
				Produces: cloneValueContract(contract.Produces),
			},
		}
	case "transform":
		contract := entry.decorator.Contract
		return CapabilityDescriptor{
			Family:   CapabilityFamilyTransform,
			Ref:      entry.capability.Manifest.Name,
			Summary:  strings.TrimSpace(entry.capability.Manifest.Summary),
			Provider: provider,
			Transform: &TransformCapabilityDescriptor{
				Accepts:  cloneValueContract(contract.Accepts),
				Params:   cloneCapabilityParamSpecs(contract.Params),
				Produces: cloneValueContract(contract.Produces),
			},
		}
	case "matcher":
		return CapabilityDescriptor{
			Family:   CapabilityFamilyMatcher,
			Ref:      entry.capability.Manifest.Name,
			Summary:  strings.TrimSpace(entry.capability.Manifest.Summary),
			Provider: provider,
			Matcher: &MatcherCapabilityDescriptor{
				Args:   cloneMatcherArgs(entry.matcher.Args),
				Actual: cloneValueContract(entry.matcher.Actual),
				Sugar:  cloneSugarSpec(entry.matcher.Sugar),
			},
		}
	case "report_exporter":
		return CapabilityDescriptor{
			Family:   CapabilityFamilyReportExporter,
			Ref:      entry.capability.Manifest.Name,
			Summary:  strings.TrimSpace(entry.capability.Manifest.Summary),
			Provider: provider,
			ReportExporter: &ReportExporterCapabilityDescriptor{
				Params: cloneCapabilityParamSpecs(entry.reportExporter.Params),
			},
		}
	case "state_backend":
		descriptor := describeStateBackend(entry.stateBackend)
		return CapabilityDescriptor{
			Family:   CapabilityFamilyStateBackend,
			Ref:      entry.capability.Manifest.Name,
			Summary:  strings.TrimSpace(entry.capability.Manifest.Summary),
			Provider: provider,
			StateBackend: &StateBackendCapabilityDescriptor{
				Descriptor: descriptor,
				Params:     cloneCapabilityParamSpecs(entry.stateBackend.Params),
			},
		}
	default:
		return CapabilityDescriptor{}
	}
}

func sortCapabilityDescriptors(descriptors []CapabilityDescriptor) {
	sort.Slice(descriptors, func(i, j int) bool {
		left := capabilityFamilyRank(descriptors[i].Family)
		right := capabilityFamilyRank(descriptors[j].Family)
		if left != right {
			return left < right
		}
		return descriptors[i].Ref < descriptors[j].Ref
	})
}

func capabilityFamilyRank(family CapabilityFamily) int {
	for index, item := range CapabilityFamilies() {
		if item == family {
			return index
		}
	}

	return len(CapabilityFamilies())
}

func cloneArgSpecs(specs []ArgSpec) []ArgSpec {
	if len(specs) == 0 {
		return nil
	}

	cloned := make([]ArgSpec, len(specs))
	copy(cloned, specs)
	for i := range cloned {
		cloned[i].Accepts = cloneValueContract(specs[i].Accepts)
	}

	return cloned
}

func cloneCapabilityParamSpecs(specs []ParamSpec) []ParamSpec {
	if len(specs) == 0 {
		return nil
	}

	cloned := make([]ParamSpec, len(specs))
	copy(cloned, specs)
	for i := range cloned {
		cloned[i].Accepts = cloneValueContract(specs[i].Accepts)
	}

	return cloned
}

func cloneMatcherArgs(args []MatcherArg) []MatcherArg {
	if len(args) == 0 {
		return nil
	}

	cloned := make([]MatcherArg, len(args))
	copy(cloned, args)
	for i := range cloned {
		cloned[i].Accepts = cloneValueContract(args[i].Accepts)
	}

	return cloned
}

func cloneSugarSpec(spec SugarSpec) SugarSpec {
	cloned := spec
	cloned.Keys = append([]string(nil), spec.Keys...)
	cloned.PositionalArgs = append([]string(nil), spec.PositionalArgs...)
	return cloned
}

func describeStateBackend(def StateBackendDef) StateDescriptor {
	if def.Describe == nil {
		return StateDescriptor{}
	}

	if descriptor, err := def.Describe(syntheticCapabilityValues(def.Params)); err == nil {
		return descriptor
	}
	if descriptor, err := def.Describe(nil); err == nil {
		return descriptor
	}

	return StateDescriptor{}
}

func syntheticCapabilityValues(params []ParamSpec) Values {
	if len(params) == 0 {
		return nil
	}

	values := make(Values, len(params))
	for i := range params {
		param := params[i]
		if !param.Required {
			continue
		}

		value, ok := syntheticCapabilityValue(param.Accepts)
		if !ok {
			continue
		}
		values[param.Name] = value
	}
	if len(values) == 0 {
		return nil
	}

	return values
}

func syntheticCapabilityValue(contract ValueContract) (any, bool) {
	kinds := contract.KindsSet()
	switch {
	case kinds.Contains(ValueKindObject):
		return map[string]any{}, true
	case kinds.Contains(ValueKindString):
		return "example", true
	case kinds.Contains(ValueKindNumber):
		return float64(1), true
	case kinds.Contains(ValueKindBool):
		return true, true
	case kinds.Contains(ValueKindList):
		return []any{}, true
	case kinds.Contains(ValueKindBytes):
		return []byte("example"), true
	case kinds.Contains(ValueKindAny):
		return map[string]any{}, true
	default:
		return nil, false
	}
}

func sortedMapKeys[V any](items map[string]V) []string {
	if len(items) == 0 {
		return nil
	}

	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
