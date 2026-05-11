package theater

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alex-poliushkin/theater/internal/pluginhost"
	"github.com/alex-poliushkin/theater/internal/pluginredact"
	internalpluginregistry "github.com/alex-poliushkin/theater/internal/pluginregistry"
	"github.com/alex-poliushkin/theater/observe"
	pluginmanifest "github.com/alex-poliushkin/theater/plugin/manifest"
	pluginprotocol "github.com/alex-poliushkin/theater/plugin/protocol"
	pluginregistry "github.com/alex-poliushkin/theater/plugin/registry"
	statemodel "github.com/alex-poliushkin/theater/state"
)

// PluginCatalogOptions configures programmatic plugin overlay construction.
type PluginCatalogOptions struct {
	RootDir     string
	Config      pluginregistry.ConfigFile
	Lock        pluginregistry.LockFile
	RequireLock bool
}

// PluginCatalog overlays runtime-loaded plugin capabilities on top of a base catalog.
type PluginCatalog struct {
	base            *Catalog
	baseMatchers    *MatcherCatalog
	loaded          *internalpluginregistry.LoadedRegistry
	actions         map[string]Action
	decorators      map[string]DecoratorDef
	inventories     map[string]Inventory
	matchers        map[string]MatcherDescriptor
	reportExporters map[string]ReportExporterDef
	stateBackends   map[string]StateBackendDef
	capabilities    map[string]pluginCapabilityEntry
	runtimeMu       sync.RWMutex
	runtime         *pluginRuntime
}

// LoadPluginCatalog loads a plugin registry from files and overlays it on top of base.
func LoadPluginCatalog(base *Catalog, matchers *MatcherCatalog, configPath, lockPath string) (*PluginCatalog, error) {
	loaded, err := internalpluginregistry.Load(configPath, lockPath)
	if err != nil {
		return nil, err
	}

	return newPluginCatalog(base, matchers, loaded)
}

// LoadPluginDescriptorCatalog loads plugin descriptors for static analysis
// without resolving or hashing plugin executables.
//
// The returned catalog is intended for validation, completion, hover and other
// descriptor-only tooling. Runtime execution must use LoadPluginCatalog so the
// executable path and lock entry are checked before a plugin process can start.
func LoadPluginDescriptorCatalog(base *Catalog, matchers *MatcherCatalog, configPath, lockPath string) (*PluginCatalog, error) {
	loaded, err := internalpluginregistry.LoadDescriptors(configPath, lockPath)
	if err != nil {
		return nil, err
	}

	return newPluginCatalog(base, matchers, loaded)
}

// NewPluginCatalog overlays a programmatically provided plugin registry on top of base.
func NewPluginCatalog(base *Catalog, matchers *MatcherCatalog, options PluginCatalogOptions) (*PluginCatalog, error) {
	loaded, err := internalpluginregistry.Build(
		options.RootDir,
		filepath.Join(options.RootDir, "plugins.json"),
		filepath.Join(options.RootDir, "plugins.lock.json"),
		options.Config,
		options.Lock,
		options.RequireLock,
	)
	if err != nil {
		return nil, err
	}

	return newPluginCatalog(base, matchers, loaded)
}

type pluginCapabilityEntry struct {
	pluginID       string
	plugin         internalpluginregistry.LoadedPlugin
	capability     internalpluginregistry.LoadedCapability
	action         Action
	decorator      DecoratorDef
	inventory      Inventory
	matcher        MatcherDescriptor
	reportExporter ReportExporterDef
	stateBackend   StateBackendDef
}

type pluginStageCapabilityRef struct {
	pluginID          string
	capabilityName    string
	kind              pluginmanifest.CapabilityKind
	descriptorPath    string
	bindingParentPath string
	bindingProperties map[string]bindingPlan
	staticProperties  map[string]any
}

type pluginValidationCatalog interface {
	validatePlugins(context.Context, *stagePlan) []Diagnostic
}

type pluginRunCatalog interface {
	preparePluginRun(context.Context, *stagePlan) (context.Context, func(context.Context), error)
}

func (c *PluginCatalog) ResolveAction(ref string) (Action, error) {
	if action, ok := c.actions[ref]; ok {
		return action, nil
	}

	return c.base.ResolveAction(ref)
}

func (c *PluginCatalog) ResolveInventory(ref string) (Inventory, error) {
	if inventory, ok := c.inventories[ref]; ok {
		return inventory, nil
	}

	return c.base.ResolveInventory(ref)
}

func (c *PluginCatalog) ResolveGenerator(ref string) (GeneratorDef, error) {
	return c.base.ResolveGenerator(ref)
}

func (c *PluginCatalog) Resolve(ref string) (MatcherDescriptor, error) {
	if descriptor, ok := c.matchers[ref]; ok {
		return descriptor, nil
	}
	if c.baseMatchers == nil {
		return MatcherDescriptor{}, fmt.Errorf("matcher %q is not registered", ref)
	}

	return c.baseMatchers.Resolve(ref)
}

func (c *PluginCatalog) ResolveSugarKey(key string) (MatcherDescriptor, error) {
	if c.baseMatchers == nil {
		return MatcherDescriptor{}, fmt.Errorf("matcher sugar %q is not registered", key)
	}

	return c.baseMatchers.ResolveSugarKey(key)
}

func (c *PluginCatalog) ResolveStateBackend(ref string) (StateBackendDef, error) {
	if backend, ok := c.stateBackends[ref]; ok {
		return backend, nil
	}

	return c.base.ResolveStateBackend(ref)
}

func (c *PluginCatalog) ResolveReportExporter(ref string) (ReportExporterDef, error) {
	if exporter, ok := c.reportExporters[ref]; ok {
		return exporter, nil
	}

	return c.base.ResolveReportExporter(ref)
}

func (c *PluginCatalog) ResolveDecorator(ref string) (DecoratorDef, error) {
	if decorator, ok := c.decorators[ref]; ok {
		return decorator, nil
	}

	return c.base.ResolveDecorator(ref)
}

func (c *PluginCatalog) newScenarioScopeRun() *scenarioScopeRun {
	return c.base.newScenarioScopeRun()
}

func (c *PluginCatalog) validatePlugins(ctx context.Context, stage *stagePlan) []Diagnostic {
	references := c.collectStageCapabilityRefs(stage)
	if len(references) == 0 {
		return nil
	}

	diagnostics := make([]Diagnostic, 0)
	sessions, closeSessions, err := c.openPluginSessions(ctx, references, pluginprotocol.SessionModeValidate)
	if err != nil {
		return []Diagnostic{{
			Code:     "plugin_validate_session_failed",
			Path:     stage.Path,
			Severity: SeverityError,
			Summary:  err.Error(),
		}}
	}
	defer closeSessions(ctx)

	for _, reference := range references {
		capability := c.capabilities[reference.capabilityName]
		if !capability.capability.Manifest.Annotations.SupportsValidate {
			continue
		}

		properties, dynamicPaths, err := reference.propertiesJSON()
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "plugin_validate_binding_error",
				Path:     reference.descriptorPath,
				Severity: SeverityError,
				Summary:  err.Error(),
			})
			continue
		}

		session := sessions[reference.pluginID]
		callRedactor := redactorForJSONPointers(properties, capability.capability.Manifest.Annotations.SensitiveInputPaths)
		redactHookText := func(text string) string {
			return callRedactor.RedactText(session.RedactText(text))
		}
		callCtx, cancel := pluginPhaseTimeoutContext(ctx, capability.plugin.Config.Timeouts.Validate)
		var result pluginprotocol.ValidateResult
		err = session.Call(callCtx, pluginprotocol.MethodValidate, pluginprotocol.ValidateParams{
			Capability:   reference.capabilityName,
			Properties:   properties,
			DynamicPaths: dynamicPaths,
		}, pluginActionSink{redactor: callRedactor}, &result)
		cancel()
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "plugin_validate_call_failed",
				Path:     reference.descriptorPath,
				Severity: SeverityError,
				Summary:  redactPluginCallErrorText(err, redactHookText).Error(),
			})
			continue
		}

		for _, item := range result.Diagnostics {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "plugin_validate_diagnostic",
				Path:     redactHookText(pluginDiagnosticPath(reference.bindingParentPath, item.Path)),
				Severity: SeverityError,
				Summary:  redactHookText(item.Message),
			})
		}
	}

	return diagnostics
}

func (c *PluginCatalog) preparePluginRun(ctx context.Context, stage *stagePlan) (context.Context, func(context.Context), error) {
	references := c.collectStageCapabilityRefs(stage)
	if len(references) == 0 {
		return ctx, func(context.Context) {}, nil
	}

	sessions, closeSessions, err := c.openPluginSessions(ctx, references, pluginprotocol.SessionModeRun)
	if err != nil {
		return ctx, nil, err
	}

	for _, reference := range references {
		capability := c.capabilities[reference.capabilityName]
		if !capability.capability.Manifest.Annotations.SupportsPrepare {
			continue
		}

		properties, dynamicPaths, err := reference.propertiesJSON()
		if err != nil {
			closeSessions(ctx)
			return ctx, nil, err
		}

		callCtx, cancel := pluginPhaseTimeoutContext(ctx, capability.plugin.Config.Timeouts.Prepare)
		session := sessions[reference.pluginID]
		callRedactor := redactorForJSONPointers(properties, capability.capability.Manifest.Annotations.SensitiveInputPaths)
		redactHookText := func(text string) string {
			return callRedactor.RedactText(session.RedactText(text))
		}
		if err := session.Call(callCtx, pluginprotocol.MethodPrepare, pluginprotocol.PrepareParams{
			Capability:   reference.capabilityName,
			Properties:   properties,
			DynamicPaths: dynamicPaths,
		}, pluginActionSink{redactor: callRedactor}, nil); err != nil {
			cancel()
			closeSessions(ctx)
			return ctx, nil, redactPluginCallErrorText(err, redactHookText)
		}
		cancel()
	}

	runtime := &pluginRuntime{
		callContext: func() context.Context { return ctx },
		sessions:    sessions,
	}
	c.setRuntime(runtime)

	return withPluginRuntime(ctx, runtime), func(closeCtx context.Context) {
		c.setRuntime(nil)
		closeSessions(closeCtx)
	}, nil
}

func newPluginCatalog(base *Catalog, matchers *MatcherCatalog, loaded *internalpluginregistry.LoadedRegistry) (*PluginCatalog, error) {
	if base == nil {
		return nil, errors.New("base catalog is required")
	}

	catalog := &PluginCatalog{
		base:            base,
		baseMatchers:    matchers,
		loaded:          loaded,
		actions:         make(map[string]Action),
		decorators:      make(map[string]DecoratorDef),
		inventories:     make(map[string]Inventory),
		matchers:        make(map[string]MatcherDescriptor),
		reportExporters: make(map[string]ReportExporterDef),
		stateBackends:   make(map[string]StateBackendDef),
		capabilities:    make(map[string]pluginCapabilityEntry),
	}

	pluginIDs := make([]string, 0, len(loaded.Plugins))
	for id := range loaded.Plugins {
		pluginIDs = append(pluginIDs, id)
	}
	sort.Strings(pluginIDs)

	for _, pluginID := range pluginIDs {
		plugin := loaded.Plugins[pluginID]
		if len(plugin.Config.AllowCapabilities) == 0 {
			return nil, fmt.Errorf("plugin %q must allow at least one capability", pluginID)
		}

		for _, name := range plugin.Config.AllowCapabilities {
			capability, ok := plugin.Capabilities[name]
			if !ok {
				return nil, fmt.Errorf("plugin %q does not declare allowed capability %q", pluginID, name)
			}
			if _, ok := catalog.capabilities[name]; ok {
				return nil, fmt.Errorf("plugin capability %q is declared more than once", name)
			}
			if err := catalog.registerPluginCapability(pluginID, plugin, capability); err != nil {
				return nil, err
			}
		}
	}

	return catalog, nil
}

func (c *PluginCatalog) registerPluginCapability(
	pluginID string,
	plugin internalpluginregistry.LoadedPlugin,
	capability internalpluginregistry.LoadedCapability,
) error {
	switch capability.Manifest.Kind {
	case pluginmanifest.CapabilityKindAction:
		return c.registerPluginAction(pluginID, plugin, capability)
	case pluginmanifest.CapabilityKindInventory:
		return c.registerPluginInventory(pluginID, plugin, capability)
	case pluginmanifest.CapabilityKindTransform:
		return c.registerPluginTransform(pluginID, plugin, capability)
	case pluginmanifest.CapabilityKindMatcher:
		return c.registerPluginMatcher(pluginID, plugin, capability)
	case pluginmanifest.CapabilityKindReportExporter:
		return c.registerPluginReportExporter(pluginID, plugin, capability)
	case pluginmanifest.CapabilityKindStateBackend:
		return c.registerPluginStateBackend(pluginID, plugin, capability)
	default:
		return fmt.Errorf("plugin capability kind %q is not supported", capability.Manifest.Kind)
	}
}

func (c *PluginCatalog) registerPluginAction(
	pluginID string,
	plugin internalpluginregistry.LoadedPlugin,
	capability internalpluginregistry.LoadedCapability,
) error {
	name := capability.Manifest.Name
	if _, ok := c.base.actions[name]; ok {
		return fmt.Errorf("plugin capability %q collides with built-in action", name)
	}

	contract, err := pluginActionContract(capability)
	if err != nil {
		return fmt.Errorf("plugin action %q: %w", name, err)
	}

	action := pluginAction{
		pluginID:       pluginID,
		capability:     capability,
		contract:       contract,
		capabilityName: name,
	}
	c.actions[name] = action
	c.capabilities[name] = pluginCapabilityEntry{
		pluginID:   pluginID,
		plugin:     plugin,
		capability: capability,
		action:     action,
	}

	return nil
}

func (c *PluginCatalog) registerPluginInventory(
	pluginID string,
	plugin internalpluginregistry.LoadedPlugin,
	capability internalpluginregistry.LoadedCapability,
) error {
	name := capability.Manifest.Name
	if _, ok := c.base.inventories[name]; ok {
		return fmt.Errorf("plugin capability %q collides with built-in inventory", name)
	}

	contract, err := pluginInventoryContract(capability)
	if err != nil {
		return fmt.Errorf("plugin inventory %q: %w", name, err)
	}

	inventory := pluginInventory{
		pluginID:       pluginID,
		capability:     capability,
		contract:       contract,
		capabilityName: name,
	}
	c.inventories[name] = inventory
	c.capabilities[name] = pluginCapabilityEntry{
		pluginID:   pluginID,
		plugin:     plugin,
		capability: capability,
		inventory:  inventory,
	}

	return nil
}

func (c *PluginCatalog) registerPluginTransform(
	pluginID string,
	plugin internalpluginregistry.LoadedPlugin,
	capability internalpluginregistry.LoadedCapability,
) error {
	name := capability.Manifest.Name
	if _, err := c.base.ResolveDecorator(name); err == nil {
		return fmt.Errorf("plugin capability %q collides with built-in decorator", name)
	}

	def, err := c.pluginTransformDef(pluginID, plugin, capability)
	if err != nil {
		return fmt.Errorf("plugin transform %q: %w", name, err)
	}

	c.decorators[name] = def
	c.capabilities[name] = pluginCapabilityEntry{
		pluginID:   pluginID,
		plugin:     plugin,
		capability: capability,
		decorator:  def,
	}

	return nil
}

func (c *PluginCatalog) registerPluginMatcher(
	pluginID string,
	plugin internalpluginregistry.LoadedPlugin,
	capability internalpluginregistry.LoadedCapability,
) error {
	name := capability.Manifest.Name
	if c.baseMatchers != nil {
		if _, err := c.baseMatchers.Resolve(name); err == nil {
			return fmt.Errorf("plugin capability %q collides with built-in matcher", name)
		}
	}

	descriptor, err := pluginMatcherDescriptor(pluginID, capability)
	if err != nil {
		return fmt.Errorf("plugin matcher %q: %w", name, err)
	}

	c.matchers[name] = descriptor
	c.capabilities[name] = pluginCapabilityEntry{
		pluginID:   pluginID,
		plugin:     plugin,
		capability: capability,
		matcher:    descriptor,
	}

	return nil
}

func (c *PluginCatalog) registerPluginReportExporter(
	pluginID string,
	plugin internalpluginregistry.LoadedPlugin,
	capability internalpluginregistry.LoadedCapability,
) error {
	name := capability.Manifest.Name
	if _, err := c.base.ResolveReportExporter(name); err == nil {
		return fmt.Errorf("plugin capability %q collides with built-in report exporter", name)
	}

	params, err := pluginParamSpecs(capability)
	if err != nil {
		return fmt.Errorf("plugin report exporter %q: %w", name, err)
	}

	exporter := pluginReportExporter{
		pluginID:       pluginID,
		plugin:         plugin,
		capability:     capability,
		capabilityName: name,
	}
	def := ReportExporterDef{
		Params: params,
		Export: exporter.Export,
	}
	c.reportExporters[name] = def
	c.capabilities[name] = pluginCapabilityEntry{
		pluginID:       pluginID,
		plugin:         plugin,
		capability:     capability,
		reportExporter: def,
	}

	return nil
}

func (c *PluginCatalog) registerPluginStateBackend(
	pluginID string,
	plugin internalpluginregistry.LoadedPlugin,
	capability internalpluginregistry.LoadedCapability,
) error {
	name := capability.Manifest.Name
	if _, err := c.base.ResolveStateBackend(name); err == nil {
		return fmt.Errorf("plugin capability %q collides with built-in state backend", name)
	}

	params, err := pluginParamSpecs(capability)
	if err != nil {
		return fmt.Errorf("plugin state backend %q: %w", name, err)
	}
	descriptor, err := pluginStateDescriptor(capability)
	if err != nil {
		return fmt.Errorf("plugin state backend %q: %w", name, err)
	}

	backend := pluginStateBackendDef{
		pluginID:       pluginID,
		capability:     capability,
		capabilityName: name,
		descriptor:     descriptor,
	}
	def := StateBackendDef{
		Params:   params,
		Describe: backend.Describe,
		Open:     backend.Open,
	}
	c.stateBackends[name] = def
	c.capabilities[name] = pluginCapabilityEntry{
		pluginID:     pluginID,
		plugin:       plugin,
		capability:   capability,
		stateBackend: def,
	}

	return nil
}

func (c *PluginCatalog) collectStageCapabilityRefs(stage *stagePlan) []pluginStageCapabilityRef {
	if stage == nil {
		return nil
	}

	refs := make([]pluginStageCapabilityRef, 0)
	if stage.State != nil {
		names := make([]string, 0, len(stage.State.Backends))
		for name := range stage.State.Backends {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			backend := stage.State.Backends[name]
			if capability, ok := c.capabilities[backend.Use]; ok {
				refs = append(refs, pluginStageCapabilityRef{
					pluginID:          capability.pluginID,
					capabilityName:    backend.Use,
					kind:              capability.capability.Manifest.Kind,
					descriptorPath:    stateBackendPath(stage.Path, name),
					bindingParentPath: stateBackendPath(stage.Path, name) + "/with",
					staticProperties:  backend.With,
				})
			}
		}
	}
	for i := range stage.Scenarios {
		scenario := stage.Scenarios[i]
		for j := range scenario.Acts {
			act := scenario.Acts[j]
			if capability, ok := c.capabilities[act.Action.Use]; ok {
				refs = append(refs, pluginStageCapabilityRef{
					pluginID:          capability.pluginID,
					capabilityName:    act.Action.Use,
					kind:              capability.capability.Manifest.Kind,
					descriptorPath:    act.Path + "/action",
					bindingParentPath: act.Path + "/action",
					bindingProperties: act.Action.With,
				})
			}

			for k := range act.Properties {
				property := act.Properties[k]
				if capability, ok := c.capabilities[property.Inventory.Use]; ok {
					refs = append(refs, pluginStageCapabilityRef{
						pluginID:          capability.pluginID,
						capabilityName:    property.Inventory.Use,
						kind:              capability.capability.Manifest.Kind,
						descriptorPath:    property.Path + "/inventory",
						bindingParentPath: property.Path + "/inventory/with",
						bindingProperties: property.Inventory.With,
					})
				}
				for m := range property.Decorators {
					decorator := property.Decorators[m]
					if capability, ok := c.capabilities[decorator.Use]; ok {
						refs = append(refs, pluginStageCapabilityRef{
							pluginID:          capability.pluginID,
							capabilityName:    decorator.Use,
							kind:              capability.capability.Manifest.Kind,
							descriptorPath:    joinChildPath(property.Path, "decorator", decoratorKey(&decorator, m)),
							bindingParentPath: joinChildPath(property.Path, "decorator", decoratorKey(&decorator, m)) + "/with",
							staticProperties:  decorator.With,
						})
					}
				}
			}

			for k := range act.Expectations {
				expectation := act.Expectations[k]
				if capability, ok := c.capabilities[expectation.Assert.Ref]; ok {
					refs = append(refs, pluginStageCapabilityRef{
						pluginID:          capability.pluginID,
						capabilityName:    expectation.Assert.Ref,
						kind:              capability.capability.Manifest.Kind,
						descriptorPath:    joinChildPath(act.Path, "expectation", expectation.ID) + "/assert",
						bindingParentPath: joinChildPath(act.Path, "expectation", expectation.ID) + "/assert/args",
						bindingProperties: expectation.Assert.Args,
					})
				}
			}
		}
	}

	return refs
}

func (c *PluginCatalog) openPluginSessions(
	ctx context.Context,
	references []pluginStageCapabilityRef,
	mode pluginprotocol.SessionMode,
) (sessions map[string]*pluginhost.Session, closeSessions func(context.Context), err error) {
	allowed := make(map[string]map[string]struct{})
	for _, reference := range references {
		entry := c.capabilities[reference.capabilityName]
		if allowed[entry.pluginID] == nil {
			allowed[entry.pluginID] = make(map[string]struct{})
		}
		allowed[entry.pluginID][reference.capabilityName] = struct{}{}
	}

	pluginIDs := make([]string, 0, len(allowed))
	for pluginID := range allowed {
		pluginIDs = append(pluginIDs, pluginID)
	}
	sort.Strings(pluginIDs)

	sessions = make(map[string]*pluginhost.Session, len(pluginIDs))
	closeSessions = func(closeCtx context.Context) {
		for _, pluginID := range pluginIDs {
			if session := sessions[pluginID]; session != nil {
				_ = session.Close(closeCtx)
			}
		}
	}

	for _, pluginID := range pluginIDs {
		plugin := c.loaded.Plugins[pluginID]
		names := make([]string, 0, len(allowed[pluginID]))
		for name := range allowed[pluginID] {
			names = append(names, name)
		}
		sort.Strings(names)

		session, _, err := pluginhost.Open(ctx, plugin, pluginhost.OpenConfig{
			Mode:                mode,
			AllowedCapabilities: names,
			SessionConfig:       plugin.Config.Config,
		})
		if err != nil {
			closeSessions(ctx)
			return nil, func(context.Context) {}, err
		}
		sessions[pluginID] = session
	}

	return sessions, closeSessions, nil
}

func pluginPhaseTimeoutContext(ctx context.Context, raw string) (context.Context, context.CancelFunc) {
	if raw == "" {
		return context.WithCancel(ctx)
	}

	timeout, err := time.ParseDuration(raw)
	if err != nil || timeout <= 0 {
		return context.WithCancel(ctx)
	}

	return context.WithTimeout(ctx, timeout)
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}

	return cloned
}

func pluginDiagnosticPath(basePath, pointer string) string {
	if basePath == "" || pointer == "" || pointer == "/" {
		return basePath
	}

	trimmed := strings.TrimPrefix(pointer, "/")
	if trimmed == "" {
		return basePath
	}

	segments := strings.Split(trimmed, "/")
	path := basePath
	for _, segment := range segments {
		segment = strings.ReplaceAll(segment, "~1", "/")
		segment = strings.ReplaceAll(segment, "~0", "~")
		path = bindingChildPath(path, segment)
	}

	return path
}

func (r pluginStageCapabilityRef) propertiesJSON() (properties map[string]any, dynamicPaths []string, err error) {
	if len(r.bindingProperties) != 0 {
		return partialBindingsJSON(r.bindingProperties)
	}

	propertiesValue, err := jsonCompatibleValue(r.staticProperties)
	if err != nil {
		return nil, nil, err
	}
	if propertiesValue == nil {
		return map[string]any{}, nil, nil
	}

	properties, _ = propertiesValue.(map[string]any)
	if properties == nil {
		return map[string]any{}, nil, nil
	}

	return properties, nil, nil
}

type pluginRuntime struct {
	callContext func() context.Context
	sessions    map[string]*pluginhost.Session
}

func (c *PluginCatalog) setRuntime(runtime *pluginRuntime) {
	c.runtimeMu.Lock()
	defer c.runtimeMu.Unlock()
	c.runtime = runtime
}

func (c *PluginCatalog) currentRuntime() *pluginRuntime {
	c.runtimeMu.RLock()
	defer c.runtimeMu.RUnlock()
	return c.runtime
}

type pluginRuntimeContextKey struct{}

func withPluginRuntime(ctx context.Context, runtime *pluginRuntime) context.Context {
	return context.WithValue(ctx, pluginRuntimeContextKey{}, runtime)
}

func pluginRuntimeFromContext(ctx context.Context) *pluginRuntime {
	runtime, _ := ctx.Value(pluginRuntimeContextKey{}).(*pluginRuntime)
	return runtime
}

type pluginAction struct {
	pluginID       string
	capabilityName string
	capability     internalpluginregistry.LoadedCapability
	contract       ActionContract
}

type pluginInventory struct {
	pluginID       string
	capabilityName string
	capability     internalpluginregistry.LoadedCapability
	contract       InventoryContract
}

type pluginTransform struct {
	catalog        *PluginCatalog
	pluginID       string
	capabilityName string
	capability     internalpluginregistry.LoadedCapability
	contract       DecoratorContract
}

type pluginMatcherDef struct {
	pluginID       string
	capabilityName string
	capability     internalpluginregistry.LoadedCapability
	args           []MatcherArg
	actual         ValueContract
}

type pluginMatcher struct {
	pluginID       string
	capabilityName string
	capability     internalpluginregistry.LoadedCapability
	args           Values
	actual         ValueContract
}

type pluginActionError struct {
	cause          error
	summary        string
	partialOutputs Outputs
}

type pluginRedactedError struct {
	message string
	cause   error
}

func (a pluginAction) Contract() ActionContract {
	return ActionContract{
		Inputs:  cloneValueContracts(a.contract.Inputs),
		Outputs: cloneValueContracts(a.contract.Outputs),
	}
}

func (i pluginInventory) Contract() InventoryContract {
	contract := InventoryContract{
		Summary:  i.contract.Summary,
		Produces: i.contract.Produces.Clone(),
	}
	if len(i.contract.Args) != 0 {
		contract.Args = make([]ArgSpec, len(i.contract.Args))
		copy(contract.Args, i.contract.Args)
		for idx := range contract.Args {
			contract.Args[idx].Accepts = i.contract.Args[idx].Accepts.Clone()
		}
	}
	return contract
}

func (t pluginTransform) Compile(args Values) (DecoratorFunc, error) {
	resolvedArgs, err := resolveDecoratorArgs(args, t.contract.Params)
	if err != nil {
		return nil, err
	}

	return func(value any) (any, error) {
		runtime := t.catalog.currentRuntime()
		if runtime == nil || runtime.sessions[t.pluginID] == nil {
			return nil, errors.New("plugin runtime session is not available")
		}

		properties, err := jsonCompatibleValue(map[string]any(resolvedArgs))
		if err != nil {
			return nil, err
		}
		propertiesMap, _ := properties.(map[string]any)
		if err := validateJSONCompatibleSchema(t.capability.PropertyResolved, propertiesMap); err != nil {
			return nil, err
		}

		inputValue, err := jsonCompatibleValue(value)
		if err != nil {
			return nil, err
		}

		callRedactor := redactorForPluginValueInput(
			propertiesMap,
			inputValue,
			t.capability.Manifest.Annotations.SensitiveInputPaths,
		)
		var result pluginprotocol.TransformApplyResult
		callCtx := context.Background()
		if runtime.callContext != nil && runtime.callContext() != nil {
			callCtx = runtime.callContext()
		}
		if err := runtime.sessions[t.pluginID].Call(
			callCtx,
			pluginprotocol.MethodTransformApply,
			pluginprotocol.TransformApplyParams{
				Capability: t.capabilityName,
				Properties: propertiesMap,
				Value:      inputValue,
			},
			pluginActionSink{redactor: callRedactor},
			&result,
		); err != nil {
			var callErr *pluginhost.CallError
			if errors.As(err, &callErr) {
				return nil, callErr.Redact(callRedactor.RedactText)
			}
			return nil, redactPluginError(err, callRedactor.RedactText)
		}

		resolvedValue, err := jsonCompatibleValue(result.Value)
		if err != nil {
			return nil, err
		}
		if err := validateJSONCompatibleSchema(t.capability.ResultResolved, resolvedValue); err != nil {
			return nil, err
		}
		resolvedValue, err = protectJSONCompatibleValue(resolvedValue, t.capability.Manifest.Annotations.SensitiveOutputPaths)
		if err != nil {
			return nil, err
		}

		return resolvedValue, nil
	}, nil
}

func (m pluginMatcherDef) Compile(_ MatcherCompileContext, args Values) (Matcher, error) {
	resolvedArgs, err := resolvePluginMatcherArgs(args, m.args)
	if err != nil {
		return nil, err
	}

	return pluginMatcher{
		pluginID:       m.pluginID,
		capabilityName: m.capabilityName,
		capability:     m.capability,
		args:           resolvedArgs,
		actual:         m.actual.Clone(),
	}, nil
}

func (m pluginMatcher) Check(ctx context.Context, actual any) error {
	runtime := pluginRuntimeFromContext(ctx)
	if runtime == nil || runtime.sessions[m.pluginID] == nil {
		return errors.New("plugin runtime session is not available")
	}
	if err := validateResolvedContract("actual", m.actual, actual); err != nil {
		return err
	}

	properties, err := jsonCompatibleValue(map[string]any(m.args))
	if err != nil {
		return err
	}
	propertiesMap, _ := properties.(map[string]any)
	if err := validateJSONCompatibleSchema(m.capability.PropertyResolved, propertiesMap); err != nil {
		return err
	}

	actualValue, err := jsonCompatibleValue(actual)
	if err != nil {
		return err
	}

	callRedactor := redactorForPluginValueInput(
		propertiesMap,
		actualValue,
		m.capability.Manifest.Annotations.SensitiveInputPaths,
	)
	if err := runtime.sessions[m.pluginID].Call(
		ctx,
		pluginprotocol.MethodMatcherCheck,
		pluginprotocol.MatcherCheckParams{
			Capability: m.capabilityName,
			Properties: propertiesMap,
			Actual:     actualValue,
		},
		pluginActionSink{redactor: callRedactor},
		nil,
	); err != nil {
		var callErr *pluginhost.CallError
		if errors.As(err, &callErr) {
			return callErr.Redact(callRedactor.RedactText)
		}
		return redactPluginError(err, callRedactor.RedactText)
	}

	return nil
}

func (a pluginAction) Run(ctx context.Context, request ActionRequest) (Outputs, error) {
	runtime := pluginRuntimeFromContext(ctx)
	if runtime == nil || runtime.sessions[a.pluginID] == nil {
		return nil, errors.New("plugin runtime session is not available")
	}

	properties, err := jsonCompatibleValue(map[string]any(request.Args))
	if err != nil {
		return nil, err
	}
	propertiesMap, _ := properties.(map[string]any)
	if err := validateJSONCompatibleSchema(a.capability.PropertyResolved, propertiesMap); err != nil {
		return nil, err
	}
	callRedactor := redactorForJSONPointers(propertiesMap, a.capability.Manifest.Annotations.SensitiveInputPaths)

	var result pluginprotocol.ActionInvokeResult
	err = runtime.sessions[a.pluginID].Call(
		ctx,
		pluginprotocol.MethodActionInvoke,
		pluginprotocol.ActionInvokeParams{
			Capability: a.capabilityName,
			Context:    pluginActionCallContext(ctx, request),
			Properties: propertiesMap,
		},
		pluginActionSink{reporter: request.Reporter, redactor: callRedactor},
		&result,
	)
	if err != nil {
		var callErr *pluginhost.CallError
		if errors.As(err, &callErr) {
			redacted := callErr.Redact(callRedactor.RedactText)
			partial, partialErr := pluginActionOutputs(a.capability, redacted.PartialOutputs())
			if partialErr != nil {
				return nil, partialErr
			}
			return partial, pluginActionError{
				cause:          redacted,
				summary:        "plugin action failed",
				partialOutputs: partial,
			}
		}
		return nil, redactPluginError(err, callRedactor.RedactText)
	}

	return pluginActionOutputs(a.capability, result.Outputs)
}

func (i pluginInventory) Acquire(ctx context.Context, request InventoryRequest) (any, error) {
	runtime := pluginRuntimeFromContext(ctx)
	if runtime == nil || runtime.sessions[i.pluginID] == nil {
		return nil, errors.New("plugin runtime session is not available")
	}

	properties, err := jsonCompatibleValue(map[string]any(request.Args))
	if err != nil {
		return nil, err
	}
	propertiesMap, _ := properties.(map[string]any)
	if err := validateJSONCompatibleSchema(i.capability.PropertyResolved, propertiesMap); err != nil {
		return nil, err
	}
	callRedactor := redactorForJSONPointers(propertiesMap, i.capability.Manifest.Annotations.SensitiveInputPaths)

	var result pluginprotocol.InventoryResolveResult
	if err := runtime.sessions[i.pluginID].Call(
		ctx,
		pluginprotocol.MethodInventoryResolve,
		pluginprotocol.InventoryResolveParams{
			Capability: i.capabilityName,
			Context:    pluginInventoryCallContext(ctx, request),
			Properties: propertiesMap,
		},
		pluginActionSink{redactor: callRedactor},
		&result,
	); err != nil {
		var callErr *pluginhost.CallError
		if errors.As(err, &callErr) {
			return nil, callErr.Redact(callRedactor.RedactText)
		}
		return nil, redactPluginError(err, callRedactor.RedactText)
	}

	value, err := jsonCompatibleValue(result.Value)
	if err != nil {
		return nil, err
	}
	if err := validateJSONCompatibleSchema(i.capability.ResultResolved, value); err != nil {
		return nil, err
	}
	value, err = protectJSONCompatibleValue(value, i.capability.Manifest.Annotations.SensitiveOutputPaths)
	if err != nil {
		return nil, err
	}

	return value, nil
}

func (e pluginActionError) Error() string {
	if e.cause != nil {
		return e.cause.Error()
	}

	return e.summary
}

func (e pluginActionError) Unwrap() error {
	return e.cause
}

func (e pluginActionError) FailureSummary() string {
	return e.summary
}

func (e pluginActionError) PartialOutputs() Outputs {
	if e.partialOutputs == nil {
		return nil
	}

	cloned := make(Outputs, len(e.partialOutputs))
	for key, value := range e.partialOutputs {
		cloned[key] = value
	}
	return cloned
}

func (e pluginRedactedError) Error() string {
	return e.message
}

func (e pluginRedactedError) Unwrap() error {
	return e.cause
}

func redactPluginError(err error, redact func(string) string) error {
	if err == nil || redact == nil {
		return err
	}

	message := redact(err.Error())
	if message == err.Error() {
		return err
	}

	return pluginRedactedError{
		message: message,
		cause:   err,
	}
}

func redactPluginCallError(err error, redactor pluginredact.Redactor) error {
	return redactPluginCallErrorText(err, redactor.RedactText)
}

func redactPluginCallErrorText(err error, redact func(string) string) error {
	var callErr *pluginhost.CallError
	if errors.As(err, &callErr) {
		return callErr.Redact(redact)
	}
	return redactPluginError(err, redact)
}

func pluginActionOutputs(capability internalpluginregistry.LoadedCapability, outputs map[string]any) (Outputs, error) {
	if outputs == nil {
		outputs = map[string]any{}
	}
	if capability.ResultResolved == nil && len(outputs) != 0 {
		return nil, errors.New("plugin action returned outputs without result_schema")
	}

	value, err := jsonCompatibleValue(outputs)
	if err != nil {
		return nil, err
	}
	if err := validateJSONCompatibleSchema(capability.ResultResolved, value); err != nil {
		return nil, err
	}
	mapValue, err := protectJSONCompatibleObject(value, capability.Manifest.Annotations.SensitiveOutputPaths)
	if err != nil {
		return nil, err
	}

	resolved := make(Outputs, len(mapValue))
	for key, child := range mapValue {
		resolved[key] = child
	}

	return resolved, nil
}

func pluginActionCallContext(ctx context.Context, request ActionRequest) pluginprotocol.CallContext {
	call := pluginprotocol.CallContext{
		StagePath: request.Paths.StagePath,
		ActID:     pathLastToken(request.Paths.ActPath),
		Path:      request.Paths.ActPath + "/action",
		Attempt:   request.Attempt,
	}
	if deadline, ok := ctx.Deadline(); ok {
		call.Deadline = &deadline
	}

	return call
}

func pluginInventoryCallContext(ctx context.Context, request InventoryRequest) pluginprotocol.CallContext {
	call := pluginprotocol.CallContext{
		StagePath: request.Paths.StagePath,
		ActID:     pathLastToken(request.Paths.ActPath),
		Path:      request.Paths.PropertyPath,
		Attempt:   request.Attempt,
	}
	if deadline, ok := ctx.Deadline(); ok {
		call.Deadline = &deadline
	}

	return call
}

func pathLastToken(path string) string {
	if path == "" {
		return ""
	}

	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

type pluginActionSink struct {
	reporter observe.Reporter
	redactor pluginredact.Redactor
}

func (s pluginActionSink) Log(params pluginprotocol.LogParams) {
	params.Message = s.redactor.RedactText(params.Message)
	params.Fields = s.redactor.RedactFields(params.Fields)
	if s.reporter == nil {
		return
	}

	s.reporter.Diagnostic(observe.Diagnostic{
		Message: params.Message,
		Fields:  cloneStringMap(params.Fields),
	})
}

func (s pluginActionSink) Progress(params pluginprotocol.ProgressParams) {
	params.Message = s.redactor.RedactText(params.Message)
	params.Phase = s.redactor.RedactText(params.Phase)
	params.Unit = s.redactor.RedactText(params.Unit)
	if s.reporter == nil {
		return
	}

	s.reporter.Progress(observe.Progress{
		Phase:         params.Phase,
		Message:       params.Message,
		Current:       params.Current,
		Total:         params.Total,
		Unit:          params.Unit,
		Percent:       params.Percent,
		Indeterminate: params.Indeterminate,
	})
}

type pluginReportExporter struct {
	pluginID       string
	plugin         internalpluginregistry.LoadedPlugin
	capability     internalpluginregistry.LoadedCapability
	capabilityName string
}

type pluginStateBackendDef struct {
	pluginID       string
	capability     internalpluginregistry.LoadedCapability
	capabilityName string
	descriptor     StateDescriptor
}

type pluginStateBackend struct {
	pluginID       string
	capability     internalpluginregistry.LoadedCapability
	capabilityName string
	descriptor     statemodel.Descriptor
	config         map[string]any
}

func (e pluginReportExporter) Export(ctx context.Context, config Values, document RunDocument) error {
	session, closeSession, err := openSinglePluginSession(ctx, e.plugin, e.capabilityName, pluginprotocol.SessionModeRun)
	if err != nil {
		return err
	}
	defer closeSession(context.WithoutCancel(ctx))

	properties, err := jsonCompatibleValue(map[string]any(config))
	if err != nil {
		return err
	}
	propertiesMap, _ := properties.(map[string]any)
	if err := validateJSONCompatibleSchema(e.capability.PropertyResolved, propertiesMap); err != nil {
		return err
	}
	callRedactor := redactorForJSONPointers(propertiesMap, e.capability.Manifest.Annotations.SensitiveInputPaths)

	if e.capability.Manifest.Annotations.SupportsPrepare {
		callCtx, cancel := pluginPhaseTimeoutContext(ctx, e.plugin.Config.Timeouts.Prepare)
		err = session.Call(callCtx, pluginprotocol.MethodPrepare, pluginprotocol.PrepareParams{
			Capability: e.capabilityName,
			Properties: propertiesMap,
		}, pluginActionSink{redactor: callRedactor}, nil)
		cancel()
		if err != nil {
			return redactPluginCallError(err, callRedactor)
		}
	}

	callCtx, cancel := pluginPhaseTimeoutContext(ctx, e.plugin.Config.Timeouts.RequestDefault)
	defer cancel()
	if err := session.Call(callCtx, pluginprotocol.MethodReportExport, pluginprotocol.ReportExportParams{
		Capability: e.capabilityName,
		Properties: propertiesMap,
		Document:   document,
	}, pluginActionSink{redactor: callRedactor}, nil); err != nil {
		return redactPluginCallError(err, callRedactor)
	}

	return nil
}

func (d pluginStateBackendDef) Describe(_ Values) (StateDescriptor, error) {
	return d.descriptor, nil
}

func (d pluginStateBackendDef) Open(config Values) (StateBackend, error) {
	properties, err := jsonCompatibleValue(map[string]any(config))
	if err != nil {
		return nil, err
	}
	propertiesMap, _ := properties.(map[string]any)
	if err := validateJSONCompatibleSchema(d.capability.PropertyResolved, propertiesMap); err != nil {
		return nil, err
	}

	return pluginStateBackend{
		pluginID:       d.pluginID,
		capability:     d.capability,
		capabilityName: d.capabilityName,
		descriptor:     d.descriptor,
		config:         propertiesMap,
	}, nil
}

func (b pluginStateBackend) Describe(context.Context) (statemodel.Descriptor, error) {
	return b.descriptor, nil
}

func (b pluginStateBackend) ReadRecord(ctx context.Context, key string) (statemodel.RecordSnapshot, error) {
	runtime := pluginRuntimeFromContext(ctx)
	if runtime == nil || runtime.sessions[b.pluginID] == nil {
		return statemodel.RecordSnapshot{}, errors.New("plugin runtime session is not available")
	}

	var result pluginprotocol.StateReadResult
	callRedactor := redactorForJSONPointers(b.config, b.capability.Manifest.Annotations.SensitiveInputPaths)
	if err := runtime.sessions[b.pluginID].Call(ctx, pluginprotocol.MethodStateRead, pluginprotocol.StateReadParams{
		Capability: b.capabilityName,
		Config:     cloneValues(Values(b.config)),
		Key:        key,
	}, pluginActionSink{redactor: callRedactor}, &result); err != nil {
		return statemodel.RecordSnapshot{}, redactPluginCallError(err, callRedactor)
	}

	snapshot := result.Snapshot
	protected, err := protectJSONCompatibleObject(snapshot.Value, b.capability.Manifest.Annotations.SensitiveOutputPaths)
	if err != nil {
		return statemodel.RecordSnapshot{}, err
	}
	snapshot.Value = protected
	return snapshot, nil
}

func (b pluginStateBackend) CompareAndSetRecord(
	ctx context.Context,
	key, expectedVersion string,
	value map[string]any,
) (statemodel.RecordSnapshot, error) {
	runtime := pluginRuntimeFromContext(ctx)
	if runtime == nil || runtime.sessions[b.pluginID] == nil {
		return statemodel.RecordSnapshot{}, errors.New("plugin runtime session is not available")
	}

	payload, err := jsonCompatibleValue(value)
	if err != nil {
		return statemodel.RecordSnapshot{}, err
	}
	payloadMap, _ := payload.(map[string]any)
	var result pluginprotocol.StateCASResult
	callRedactor := redactorForJSONPointers(b.config, b.capability.Manifest.Annotations.SensitiveInputPaths)
	if err := runtime.sessions[b.pluginID].Call(ctx, pluginprotocol.MethodStateCAS, pluginprotocol.StateCASParams{
		Capability:      b.capabilityName,
		Config:          cloneValues(Values(b.config)),
		Key:             key,
		ExpectedVersion: expectedVersion,
		Value:           payloadMap,
	}, pluginActionSink{redactor: callRedactor}, &result); err != nil {
		return statemodel.RecordSnapshot{}, redactPluginCallError(err, callRedactor)
	}

	snapshot := result.Snapshot
	protected, err := protectJSONCompatibleObject(snapshot.Value, b.capability.Manifest.Annotations.SensitiveOutputPaths)
	if err != nil {
		return statemodel.RecordSnapshot{}, err
	}
	snapshot.Value = protected
	return snapshot, nil
}

func (b pluginStateBackend) Claim(
	ctx context.Context,
	pool string,
	selector statemodel.Selector,
	lease statemodel.LeaseSpec,
) (statemodel.ClaimResult, error) {
	runtime := pluginRuntimeFromContext(ctx)
	if runtime == nil || runtime.sessions[b.pluginID] == nil {
		return statemodel.ClaimResult{}, errors.New("plugin runtime session is not available")
	}

	var result pluginprotocol.StateClaimResult
	callRedactor := redactorForJSONPointers(b.config, b.capability.Manifest.Annotations.SensitiveInputPaths)
	if err := runtime.sessions[b.pluginID].Call(ctx, pluginprotocol.MethodStateClaim, pluginprotocol.StateClaimParams{
		Capability: b.capabilityName,
		Config:     cloneValues(Values(b.config)),
		Pool:       pool,
		Selector:   selector,
		Lease:      lease,
	}, pluginActionSink{redactor: callRedactor}, &result); err != nil {
		return statemodel.ClaimResult{}, redactPluginCallError(err, callRedactor)
	}

	protected, err := protectJSONCompatibleObject(result.Result.Item, b.capability.Manifest.Annotations.SensitiveOutputPaths)
	if err != nil {
		return statemodel.ClaimResult{}, err
	}
	result.Result.Item = protected
	return result.Result, nil
}

func (b pluginStateBackend) Renew(ctx context.Context, claim statemodel.ClaimHandle, ttl time.Duration) (statemodel.ClaimHandle, error) {
	runtime := pluginRuntimeFromContext(ctx)
	if runtime == nil || runtime.sessions[b.pluginID] == nil {
		return statemodel.ClaimHandle{}, errors.New("plugin runtime session is not available")
	}

	var result pluginprotocol.StateRenewResult
	callRedactor := redactorForJSONPointers(b.config, b.capability.Manifest.Annotations.SensitiveInputPaths)
	if err := runtime.sessions[b.pluginID].Call(ctx, pluginprotocol.MethodStateRenew, pluginprotocol.StateRenewParams{
		Capability: b.capabilityName,
		Config:     cloneValues(Values(b.config)),
		Claim:      claim,
		TTL:        ttl,
	}, pluginActionSink{redactor: callRedactor}, &result); err != nil {
		return statemodel.ClaimHandle{}, redactPluginCallError(err, callRedactor)
	}

	return result.Claim, nil
}

func (b pluginStateBackend) Release(ctx context.Context, claim statemodel.ClaimHandle, reason string) error {
	runtime := pluginRuntimeFromContext(ctx)
	if runtime == nil || runtime.sessions[b.pluginID] == nil {
		return errors.New("plugin runtime session is not available")
	}

	callRedactor := redactorForJSONPointers(b.config, b.capability.Manifest.Annotations.SensitiveInputPaths)
	if err := runtime.sessions[b.pluginID].Call(ctx, pluginprotocol.MethodStateRelease, pluginprotocol.StateReleaseParams{
		Capability: b.capabilityName,
		Config:     cloneValues(Values(b.config)),
		Claim:      claim,
		Reason:     reason,
	}, pluginActionSink{redactor: callRedactor}, nil); err != nil {
		return redactPluginCallError(err, callRedactor)
	}

	return nil
}

func (b pluginStateBackend) Consume(ctx context.Context, claim statemodel.ClaimHandle, reason string, tombstone map[string]any) error {
	runtime := pluginRuntimeFromContext(ctx)
	if runtime == nil || runtime.sessions[b.pluginID] == nil {
		return errors.New("plugin runtime session is not available")
	}

	value, err := jsonCompatibleValue(tombstone)
	if err != nil {
		return err
	}
	valueMap, _ := value.(map[string]any)
	callRedactor := redactorForJSONPointers(b.config, b.capability.Manifest.Annotations.SensitiveInputPaths)
	if err := runtime.sessions[b.pluginID].Call(ctx, pluginprotocol.MethodStateConsume, pluginprotocol.StateConsumeParams{
		Capability: b.capabilityName,
		Config:     cloneValues(Values(b.config)),
		Claim:      claim,
		Reason:     reason,
		Tombstone:  valueMap,
	}, pluginActionSink{redactor: callRedactor}, nil); err != nil {
		return redactPluginCallError(err, callRedactor)
	}

	return nil
}

func openSinglePluginSession(
	ctx context.Context,
	plugin internalpluginregistry.LoadedPlugin,
	capability string,
	mode pluginprotocol.SessionMode,
) (*pluginhost.Session, func(context.Context), error) {
	session, _, err := pluginhost.Open(ctx, plugin, pluginhost.OpenConfig{
		Mode:                mode,
		AllowedCapabilities: []string{capability},
		SessionConfig:       plugin.Config.Config,
	})
	if err != nil {
		return nil, func(context.Context) {}, err
	}

	return session, func(closeCtx context.Context) {
		_ = session.Close(closeCtx)
	}, nil
}

func (c *PluginCatalog) pluginTransformDef(
	pluginID string,
	_ internalpluginregistry.LoadedPlugin,
	capability internalpluginregistry.LoadedCapability,
) (DecoratorDef, error) {
	contract, err := pluginTransformContract(capability)
	if err != nil {
		return DecoratorDef{}, err
	}

	transform := pluginTransform{
		catalog:        c,
		pluginID:       pluginID,
		capabilityName: capability.Manifest.Name,
		capability:     capability,
		contract:       contract,
	}

	return DecoratorDef{
		Contract: cloneDecoratorContract(contract),
		Compile:  transform.Compile,
	}, nil
}

func pluginMatcherDescriptor(
	pluginID string,
	capability internalpluginregistry.LoadedCapability,
) (MatcherDescriptor, error) {
	if capability.Manifest.Annotations.Matcher == nil {
		return MatcherDescriptor{}, errors.New("matcher metadata is required")
	}

	args, err := pluginMatcherArgs(capability)
	if err != nil {
		return MatcherDescriptor{}, err
	}

	descriptor := MatcherDescriptor{
		Ref:     capability.Manifest.Name,
		Summary: capability.Manifest.Summary,
		Args:    args,
		Actual:  capability.Manifest.Annotations.Matcher.Actual.Clone(),
	}
	descriptor.Compile = pluginMatcherDef{
		pluginID:       pluginID,
		capabilityName: capability.Manifest.Name,
		capability:     capability,
		args:           args,
		actual:         descriptor.Actual.Clone(),
	}.Compile

	return descriptor, nil
}

func resolvePluginMatcherArgs(args Values, specs []MatcherArg) (Values, error) {
	resolved := cloneValues(args)
	specIndex := make(map[string]MatcherArg, len(specs))
	for i := range specs {
		specIndex[specs[i].Name] = specs[i]
	}

	for key := range args {
		if _, ok := specIndex[key]; !ok {
			return nil, fmt.Errorf("matcher does not support arg %q", key)
		}
	}

	for i := range specs {
		spec := specs[i]
		value, ok := resolved[spec.Name]
		if !ok {
			if spec.Required {
				return nil, fmt.Errorf("matcher requires arg %q", spec.Name)
			}
			continue
		}

		if err := validateResolvedContract(spec.Name, spec.Accepts, value); err != nil {
			return nil, err
		}
	}

	return protectMatcherArgs(resolved, specs), nil
}
