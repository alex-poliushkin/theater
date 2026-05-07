package inventory

import (
	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/builtin/internal/builtinhttp"
)

// RuntimeRegistrar registers inventories together with required scenario-scoped
// runtime support.
type RuntimeRegistrar interface {
	theater.InventoryRegistrar
	theater.ScenarioScopeInitializerRegistrar
}

// Register installs the built-in inventories into a narrow inventory
// registrar.
func Register(catalog theater.InventoryRegistrar) error {
	if err := catalog.RegisterInventory(EnvRef, envInventory{}); err != nil {
		return err
	}

	if err := catalog.RegisterInventory(FileRef, fileInventory{}); err != nil {
		return err
	}

	if err := catalog.RegisterInventory(HTTPGetRef, httpInventory{}); err != nil {
		return err
	}
	if err := catalog.RegisterInventory(StateRecordRef, stateRecordInventory{}); err != nil {
		return err
	}
	if err := catalog.RegisterInventory(StatePoolRef, statePoolInventory{}); err != nil {
		return err
	}

	return nil
}

// RegisterRuntime installs the built-in inventories together with their
// required scenario-scoped runtime support.
func RegisterRuntime(catalog RuntimeRegistrar) error {
	if err := builtinhttp.RegisterScenarioScopeInitializer(catalog); err != nil {
		return err
	}

	return Register(catalog)
}
