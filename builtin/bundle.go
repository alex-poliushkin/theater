package builtin

import (
	"github.com/alex-poliushkin/theater"
	builtinaction "github.com/alex-poliushkin/theater/builtin/action"
	builtindecorator "github.com/alex-poliushkin/theater/builtin/decorator"
	builtinexpectation "github.com/alex-poliushkin/theater/builtin/expectation"
	builtingenerator "github.com/alex-poliushkin/theater/builtin/generator"
	builtininventory "github.com/alex-poliushkin/theater/builtin/inventory"
	builtinstatebackend "github.com/alex-poliushkin/theater/builtin/statebackend"
)

// Bundle groups the built-in runtime adapter catalog and matcher catalog.
type Bundle struct {
	// Catalog contains the built-in runtime-capable adapters.
	Catalog *theater.Catalog
	// Matchers contains the built-in expectation matchers.
	Matchers *theater.MatcherCatalog
}

// Registrar installs built-ins into the narrow registration seams owned by the
// root theater package.
type Registrar interface {
	theater.ActionRegistrar
	theater.GeneratorRegistrar
	theater.InventoryRegistrar
	theater.StateBackendRegistrar
	theater.DecoratorRegistrar
}

// RuntimeRegistrar registers builtins together with required scenario-scoped
// runtime support.
type RuntimeRegistrar interface {
	Registrar
	theater.ScenarioScopeInitializerRegistrar
}

// Catalog constructs a catalog with the built-in runtime-capable adapters
// installed.
func Catalog() (*theater.Catalog, error) {
	catalog := theater.NewCatalog()
	if err := RegisterRuntime(catalog); err != nil {
		return nil, err
	}

	return catalog, nil
}

// NewBundle constructs the built-in catalog and matcher catalog pair.
func NewBundle() (Bundle, error) {
	catalog, err := Catalog()
	if err != nil {
		return Bundle{}, err
	}

	matchers, err := Matchers()
	if err != nil {
		return Bundle{}, err
	}

	return Bundle{
		Catalog:  catalog,
		Matchers: matchers,
	}, nil
}

// Matchers constructs the built-in matcher catalog.
func Matchers() (*theater.MatcherCatalog, error) {
	return theater.NewMatcherCatalog(builtinexpectation.Descriptors()...)
}

// Register installs built-ins into narrow registrars without runtime-only
// scenario-scope initialization.
func Register(catalog Registrar) error {
	if err := builtinaction.Register(catalog); err != nil {
		return err
	}

	if err := builtininventory.Register(catalog); err != nil {
		return err
	}
	if err := builtingenerator.Register(catalog); err != nil {
		return err
	}

	if err := builtindecorator.Register(catalog); err != nil {
		return err
	}
	if err := builtinstatebackend.Register(catalog); err != nil {
		return err
	}

	return nil
}

// RegisterRuntime installs built-ins together with required scenario-scoped
// runtime support.
func RegisterRuntime(catalog RuntimeRegistrar) error {
	if err := builtinaction.RegisterRuntime(catalog); err != nil {
		return err
	}

	if err := builtininventory.RegisterRuntime(catalog); err != nil {
		return err
	}
	if err := builtingenerator.Register(catalog); err != nil {
		return err
	}

	if err := builtindecorator.Register(catalog); err != nil {
		return err
	}
	if err := builtinstatebackend.Register(catalog); err != nil {
		return err
	}

	return nil
}
