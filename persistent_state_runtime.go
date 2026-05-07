package theater

import (
	"context"
	"fmt"

	statemodel "github.com/alex-poliushkin/theater/state"
)

func openStateManager(ctx context.Context, spec *StateSpec, resolver StateBackendResolver) (*statemodel.Manager, error) {
	return openStateManagerWithDecorator(ctx, spec, resolver, nil)
}

func openStateManagerWithDecorator(
	ctx context.Context,
	spec *StateSpec,
	resolver StateBackendResolver,
	decorate func(name string, backend statemodel.Backend) statemodel.Backend,
) (*statemodel.Manager, error) {
	if spec == nil || len(spec.Backends) == 0 {
		return statemodel.NewManager(nil), nil
	}

	backends := make(map[string]statemodel.Backend, len(spec.Backends))
	for name, backendSpec := range spec.Backends {
		def, err := resolver.ResolveStateBackend(backendSpec.Use)
		if err != nil {
			return nil, fmt.Errorf("state backend %q: %w", name, err)
		}

		backend, err := def.Open(Values(backendSpec.With))
		if err != nil {
			return nil, fmt.Errorf("state backend %q open failed: %w", name, err)
		}
		if _, err := backend.Describe(ctx); err != nil {
			return nil, fmt.Errorf("state backend %q describe failed: %w", name, err)
		}
		if decorate != nil {
			backend = decorate(name, backend)
		}

		backends[name] = backend
	}

	return statemodel.NewManager(backends), nil
}
