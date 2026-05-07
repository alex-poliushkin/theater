package builtin_test

import (
	"testing"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/builtin"
	builtinaction "github.com/alex-poliushkin/theater/builtin/action"
	builtindecorator "github.com/alex-poliushkin/theater/builtin/decorator"
	builtinexpectation "github.com/alex-poliushkin/theater/builtin/expectation"
	builtingenerator "github.com/alex-poliushkin/theater/builtin/generator"
	builtininventory "github.com/alex-poliushkin/theater/builtin/inventory"
)

type recordingBuiltinRegistrar struct {
	actions     map[string]theater.Action
	generators  map[string]theater.GeneratorDef
	inventories map[string]theater.Inventory
	state       map[string]theater.StateBackendDef
	decorators  map[string]theater.DecoratorDef
}

type recordingRuntimeBuiltinRegistrar struct {
	recordingBuiltinRegistrar
	initializers map[string]theater.ScenarioScopeInitializerFactory
}

func (r *recordingBuiltinRegistrar) RegisterAction(ref string, action theater.Action) error {
	if r.actions == nil {
		r.actions = make(map[string]theater.Action)
	}

	r.actions[ref] = action
	return nil
}

func (r *recordingBuiltinRegistrar) RegisterInventory(ref string, inventory theater.Inventory) error {
	if r.inventories == nil {
		r.inventories = make(map[string]theater.Inventory)
	}

	r.inventories[ref] = inventory
	return nil
}

func (r *recordingBuiltinRegistrar) RegisterGenerator(ref string, generator theater.GeneratorDef) error {
	if r.generators == nil {
		r.generators = make(map[string]theater.GeneratorDef)
	}

	r.generators[ref] = generator
	return nil
}

func (r *recordingBuiltinRegistrar) RegisterDecorator(ref string, decorator theater.DecoratorDef) error {
	if r.decorators == nil {
		r.decorators = make(map[string]theater.DecoratorDef)
	}

	r.decorators[ref] = decorator
	return nil
}

func (r *recordingBuiltinRegistrar) RegisterStateBackend(ref string, backend theater.StateBackendDef) error {
	if r.state == nil {
		r.state = make(map[string]theater.StateBackendDef)
	}

	r.state[ref] = backend
	return nil
}

func (r *recordingRuntimeBuiltinRegistrar) RegisterScenarioScopeInitializer(
	ref string,
	factory theater.ScenarioScopeInitializerFactory,
) error {
	if r.initializers == nil {
		r.initializers = make(map[string]theater.ScenarioScopeInitializerFactory)
	}

	r.initializers[ref] = factory
	return nil
}

func TestRegisterAcceptsRegistrarPortsWithoutConcreteCatalog(t *testing.T) {
	t.Parallel()

	registrar := &recordingBuiltinRegistrar{}
	if err := builtin.Register(registrar); err != nil {
		t.Fatalf("builtin register failed: %v", err)
	}

	if _, ok := registrar.actions[builtinaction.HTTPRef]; !ok {
		t.Fatalf("expected %q to be registered", builtinaction.HTTPRef)
	}

	if _, ok := registrar.actions[builtinaction.CommandRef]; !ok {
		t.Fatalf("expected %q to be registered", builtinaction.CommandRef)
	}
	if _, ok := registrar.actions[builtinaction.GenerateRef]; !ok {
		t.Fatalf("expected %q to be registered", builtinaction.GenerateRef)
	}

	if _, ok := registrar.inventories[builtininventory.EnvRef]; !ok {
		t.Fatalf("expected %q to be registered", builtininventory.EnvRef)
	}
	if _, ok := registrar.generators[builtingenerator.EmailRef]; !ok {
		t.Fatalf("expected %q to be registered", builtingenerator.EmailRef)
	}

	if _, ok := registrar.decorators[builtindecorator.JSONRef]; !ok {
		t.Fatalf("expected %q to be registered", builtindecorator.JSONRef)
	}
}

func TestRegisterRuntimeRegistersScenarioScopeInitializers(t *testing.T) {
	t.Parallel()

	registrar := &recordingRuntimeBuiltinRegistrar{}
	if err := builtin.RegisterRuntime(registrar); err != nil {
		t.Fatalf("builtin runtime register failed: %v", err)
	}

	if got, want := len(registrar.initializers), 1; got != want {
		t.Fatalf("initializer count mismatch: got %d want %d", got, want)
	}
}

func TestNewBundleProvidesPublicBuiltinsEntryPoint(t *testing.T) {
	t.Parallel()

	bundle, err := builtin.NewBundle()
	if err != nil {
		t.Fatalf("builtin bundle failed: %v", err)
	}
	catalog := bundle.Catalog
	matchers := bundle.Matchers

	if _, err := catalog.ResolveAction(builtinaction.HTTPRef); err != nil {
		t.Fatalf("resolve http action failed: %v", err)
	}

	if _, err := catalog.ResolveAction(builtinaction.CommandRef); err != nil {
		t.Fatalf("resolve command action failed: %v", err)
	}
	if _, err := catalog.ResolveAction(builtinaction.GenerateRef); err != nil {
		t.Fatalf("resolve generate action failed: %v", err)
	}

	if _, err := catalog.ResolveInventory(builtininventory.EnvRef); err != nil {
		t.Fatalf("resolve env inventory failed: %v", err)
	}
	if _, err := catalog.ResolveGenerator(builtingenerator.EmailRef); err != nil {
		t.Fatalf("resolve email generator failed: %v", err)
	}

	if _, err := catalog.ResolveStateBackend("state.backend.file"); err != nil {
		t.Fatalf("resolve file state backend failed: %v", err)
	}

	if _, err := matchers.Resolve(builtinexpectation.EqualRef); err != nil {
		t.Fatalf("resolve equal matcher failed: %v", err)
	}
}
