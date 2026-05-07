package theater

import (
	"context"
	"errors"
	"fmt"
)

// Action executes one act action and exposes the contract used for validation
// and observation.
type Action interface {
	Contract() ActionContract
	Run(ctx context.Context, request ActionRequest) (Outputs, error)
}

// ActionRegistrar registers actions by stable reference.
type ActionRegistrar interface {
	RegisterAction(ref string, action Action) error
}

// ActionErrorDetails exposes failure details that can still preserve a summary
// and partial outputs for reporting.
type ActionErrorDetails interface {
	error
	FailureSummary() string
	PartialOutputs() Outputs
}

// Inventory resolves a value for property evaluation from explicit call-site
// args and runtime context.
type Inventory interface {
	Contract() InventoryContract
	Acquire(ctx context.Context, req InventoryRequest) (any, error)
}

// InventoryRegistrar registers inventories by stable reference.
type InventoryRegistrar interface {
	RegisterInventory(ref string, inventory Inventory) error
}

// DecoratorRegistrar registers decorators by stable reference.
type DecoratorRegistrar interface {
	RegisterDecorator(ref string, decorator DecoratorDef) error
}

// Catalog stores runtime adapters and scenario-scope initializers used by the
// validator and runner.
type Catalog struct {
	actions                         map[string]Action
	decorators                      map[string]DecoratorDef
	generators                      map[string]GeneratorDef
	inventories                     map[string]Inventory
	reportExporters                 map[string]ReportExporterDef
	stateBackends                   map[string]StateBackendDef
	scenarioScopeInitializerEntries []scenarioScopeInitializerEntry
}

// NewCatalog creates an empty catalog ready for adapter registration.
func NewCatalog() *Catalog {
	return &Catalog{
		actions:         make(map[string]Action),
		decorators:      make(map[string]DecoratorDef),
		generators:      make(map[string]GeneratorDef),
		inventories:     make(map[string]Inventory),
		reportExporters: make(map[string]ReportExporterDef),
		stateBackends:   make(map[string]StateBackendDef),
	}
}

// RegisterAction stores an action implementation under ref.
func (c *Catalog) RegisterAction(ref string, action Action) error {
	if c == nil {
		return errors.New("catalog is required")
	}

	if ref == "" {
		return errors.New("action ref is required")
	}

	if action == nil {
		return errors.New("action is required")
	}

	if _, ok := c.actions[ref]; ok {
		return fmt.Errorf("action %q is already registered", ref)
	}

	c.actions[ref] = action
	return nil
}

// RegisterInventory stores an inventory implementation under ref.
func (c *Catalog) RegisterInventory(ref string, inventory Inventory) error {
	if c == nil {
		return errors.New("catalog is required")
	}

	if ref == "" {
		return errors.New("inventory ref is required")
	}

	if inventory == nil {
		return errors.New("inventory is required")
	}

	if _, ok := c.inventories[ref]; ok {
		return fmt.Errorf("inventory %q is already registered", ref)
	}

	c.inventories[ref] = inventory
	return nil
}

// RegisterGenerator stores a generator implementation under ref.
func (c *Catalog) RegisterGenerator(ref string, generator GeneratorDef) error {
	if c == nil {
		return errors.New("catalog is required")
	}

	if ref == "" {
		return errors.New("generator ref is required")
	}

	if !generator.Contract.Produces.Valid() {
		return errors.New("generator contract produces is invalid")
	}

	if generator.Generate == nil {
		return errors.New("generator generate is required")
	}

	if _, ok := c.generators[ref]; ok {
		return fmt.Errorf("generator %q is already registered", ref)
	}

	c.generators[ref] = cloneGeneratorDef(generator)
	return nil
}

// RegisterStateBackend stores a persistent-state backend implementation under ref.
func (c *Catalog) RegisterStateBackend(ref string, backend StateBackendDef) error {
	if c == nil {
		return errors.New("catalog is required")
	}

	if ref == "" {
		return errors.New("state backend ref is required")
	}

	if backend.Describe == nil {
		return errors.New("state backend describe is required")
	}

	if backend.Open == nil {
		return errors.New("state backend open is required")
	}

	if _, ok := c.stateBackends[ref]; ok {
		return fmt.Errorf("state backend %q is already registered", ref)
	}

	c.stateBackends[ref] = wrapStateBackendDef(backend)
	return nil
}

// RegisterReportExporter stores a report exporter implementation under ref.
func (c *Catalog) RegisterReportExporter(ref string, exporter ReportExporterDef) error {
	if c == nil {
		return errors.New("catalog is required")
	}

	if ref == "" {
		return errors.New("report exporter ref is required")
	}

	if exporter.Export == nil {
		return errors.New("report exporter export is required")
	}

	if _, ok := c.reportExporters[ref]; ok {
		return fmt.Errorf("report exporter %q is already registered", ref)
	}

	c.reportExporters[ref] = wrapReportExporterDef(exporter)
	return nil
}

// RegisterDecorator stores a decorator implementation under ref.
func (c *Catalog) RegisterDecorator(ref string, decorator DecoratorDef) error {
	if c == nil {
		return errors.New("catalog is required")
	}

	if ref == "" {
		return errors.New("decorator ref is required")
	}

	if decorator.Compile == nil {
		return errors.New("decorator compile is required")
	}

	if _, ok := c.decorators[ref]; ok {
		return fmt.Errorf("decorator %q is already registered", ref)
	}

	c.decorators[ref] = wrapDecoratorDef(decorator)
	return nil
}

// RegisterScenarioScopeInitializer registers a factory that prepares
// scenario-local resources for each run.
func (c *Catalog) RegisterScenarioScopeInitializer(ref string, factory ScenarioScopeInitializerFactory) error {
	if c == nil {
		return errors.New("catalog is required")
	}

	if ref == "" {
		return errors.New("scenario scope initializer ref is required")
	}

	if factory == nil {
		return errors.New("scenario scope initializer factory is required")
	}

	for i := range c.scenarioScopeInitializerEntries {
		if c.scenarioScopeInitializerEntries[i].ref == ref {
			return nil
		}
	}

	c.scenarioScopeInitializerEntries = append(c.scenarioScopeInitializerEntries, scenarioScopeInitializerEntry{
		ref:     ref,
		factory: factory,
	})
	return nil
}

// ResolveAction returns the action previously registered for ref.
func (c *Catalog) ResolveAction(ref string) (Action, error) {
	if c == nil {
		return nil, errors.New("catalog is required")
	}

	action, ok := c.actions[ref]
	if !ok {
		return nil, fmt.Errorf("action %q is not registered", ref)
	}

	return action, nil
}

// ResolveInventory returns the inventory previously registered for ref.
func (c *Catalog) ResolveInventory(ref string) (Inventory, error) {
	if c == nil {
		return nil, errors.New("catalog is required")
	}

	inventory, ok := c.inventories[ref]
	if !ok {
		return nil, fmt.Errorf("inventory %q is not registered", ref)
	}

	return inventory, nil
}

// ResolveGenerator returns the generator previously registered for ref.
func (c *Catalog) ResolveGenerator(ref string) (GeneratorDef, error) {
	if c == nil {
		return GeneratorDef{}, errors.New("catalog is required")
	}

	generator, ok := c.generators[ref]
	if !ok {
		return GeneratorDef{}, fmt.Errorf("generator %q is not registered", ref)
	}

	return cloneGeneratorDef(generator), nil
}

// ResolveStateBackend returns the state backend definition previously registered for ref.
func (c *Catalog) ResolveStateBackend(ref string) (StateBackendDef, error) {
	if c == nil {
		return StateBackendDef{}, errors.New("catalog is required")
	}

	backend, ok := c.stateBackends[ref]
	if !ok {
		return StateBackendDef{}, fmt.Errorf("state backend %q is not registered", ref)
	}

	return backend, nil
}

// ResolveReportExporter returns the report exporter definition previously registered for ref.
func (c *Catalog) ResolveReportExporter(ref string) (ReportExporterDef, error) {
	if c == nil {
		return ReportExporterDef{}, errors.New("catalog is required")
	}

	exporter, ok := c.reportExporters[ref]
	if !ok {
		return ReportExporterDef{}, fmt.Errorf("report exporter %q is not registered", ref)
	}

	return exporter, nil
}

func cloneGeneratorContract(contract GeneratorContract) GeneratorContract {
	cloned := contract
	cloned.Args = append([]ArgSpec(nil), contract.Args...)
	cloned.Produces = cloneValueContract(contract.Produces)
	return cloned
}

func cloneGeneratorDef(def GeneratorDef) GeneratorDef {
	cloned := def
	cloned.Contract = cloneGeneratorContract(def.Contract)
	return cloned
}

// ResolveDecorator returns the decorator previously registered for ref.
func (c *Catalog) ResolveDecorator(ref string) (DecoratorDef, error) {
	if c == nil {
		return DecoratorDef{}, errors.New("catalog is required")
	}

	decorator, ok := c.decorators[ref]
	if !ok {
		return DecoratorDef{}, fmt.Errorf("decorator %q is not registered", ref)
	}

	return decorator, nil
}

func wrapStateBackendDef(def StateBackendDef) StateBackendDef {
	params := cloneParamSpecs(def.Params)
	describe := def.Describe
	open := def.Open

	def.Params = params
	def.Describe = func(config Values) (StateDescriptor, error) {
		resolved, err := resolveDecoratorArgs(config, params)
		if err != nil {
			return StateDescriptor{}, err
		}

		return describe(cloneValues(resolved))
	}
	def.Open = func(config Values) (StateBackend, error) {
		resolved, err := resolveDecoratorArgs(config, params)
		if err != nil {
			return nil, err
		}

		return open(cloneValues(resolved))
	}

	return def
}

func wrapReportExporterDef(def ReportExporterDef) ReportExporterDef {
	params := cloneParamSpecs(def.Params)
	export := def.Export

	def.Params = params
	def.Export = func(ctx context.Context, config Values, document RunDocument) error {
		resolved, err := resolveDecoratorArgs(config, params)
		if err != nil {
			return err
		}

		return export(ctx, cloneValues(resolved), document)
	}

	return def
}

func wrapDecoratorDef(def DecoratorDef) DecoratorDef {
	contract := cloneDecoratorContract(def.Contract)
	compile := def.Compile

	def.Contract = contract
	def.Compile = func(args Values) (DecoratorFunc, error) {
		resolved, err := resolveDecoratorArgs(args, contract.Params)
		if err != nil {
			return nil, err
		}

		return compile(cloneValues(resolved))
	}

	return def
}

type scenarioScopeInitializerEntry struct {
	ref     string
	factory ScenarioScopeInitializerFactory
}

func (c *Catalog) newScenarioScopeRun() *scenarioScopeRun {
	if len(c.scenarioScopeInitializerEntries) == 0 {
		return nil
	}

	initializers := make([]ScenarioScopeInitializer, 0, len(c.scenarioScopeInitializerEntries))
	for i := range c.scenarioScopeInitializerEntries {
		initializer := c.scenarioScopeInitializerEntries[i].factory()
		if initializer == nil {
			continue
		}

		initializers = append(initializers, initializer)
	}

	if len(initializers) == 0 {
		return nil
	}

	return &scenarioScopeRun{initializers: initializers}
}
