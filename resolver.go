package theater

import "reflect"

// ActionResolver resolves actions by registered ref.
type ActionResolver interface {
	ResolveAction(ref string) (Action, error)
}

// InventoryResolver resolves inventories by registered ref.
type InventoryResolver interface {
	ResolveInventory(ref string) (Inventory, error)
}

// DecoratorResolver resolves decorators by registered ref.
type DecoratorResolver interface {
	ResolveDecorator(ref string) (DecoratorDef, error)
}

// CatalogResolver resolves the adapters and backend definitions required by
// validators and runners.
type CatalogResolver interface {
	ActionResolver
	GeneratorResolver
	InventoryResolver
	StateBackendResolver
	ReportExporterResolver
	DecoratorResolver
}

// MatcherResolver resolves matcher descriptors by registered ref.
type MatcherResolver interface {
	Resolve(ref string) (MatcherDescriptor, error)
}

// MatcherSugarResolver resolves matcher descriptors by YAML sugar key.
type MatcherSugarResolver interface {
	ResolveSugarKey(key string) (MatcherDescriptor, error)
}

type runtimeCatalog interface {
	ActionResolver
	GeneratorResolver
	InventoryResolver
}

type propertyCatalog interface {
	GeneratorResolver
	InventoryResolver
	DecoratorResolver
}

func dependencyMissing(value any) bool {
	if value == nil {
		return true
	}

	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
