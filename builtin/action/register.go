package action

import (
	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/builtin/internal/builtinhttp"
)

// RuntimeRegistrar registers actions together with required scenario-scoped
// runtime support.
type RuntimeRegistrar interface {
	theater.ActionRegistrar
	theater.ScenarioScopeInitializerRegistrar
}

// Register installs the built-in actions into a narrow action registrar.
func Register(catalog theater.ActionRegistrar) error {
	if err := catalog.RegisterAction(HTTPRef, httpAction{}); err != nil {
		return err
	}
	if err := catalog.RegisterAction(StateReadRef, stateReadAction{}); err != nil {
		return err
	}
	if err := catalog.RegisterAction(StateUpdateRef, stateUpdateAction{}); err != nil {
		return err
	}
	if err := catalog.RegisterAction(StateClaimRef, stateClaimAction{}); err != nil {
		return err
	}
	if err := catalog.RegisterAction(StateRenewRef, stateRenewAction{}); err != nil {
		return err
	}
	if err := catalog.RegisterAction(StateReleaseRef, stateReleaseAction{}); err != nil {
		return err
	}
	if err := catalog.RegisterAction(StateConsumeRef, stateConsumeAction{}); err != nil {
		return err
	}
	if err := catalog.RegisterAction(GenerateRef, generateAction{}); err != nil {
		return err
	}

	return catalog.RegisterAction(CommandRef, commandAction{})
}

// RegisterRuntime installs the built-in actions together with their required
// scenario-scoped runtime support.
func RegisterRuntime(catalog RuntimeRegistrar) error {
	if err := builtinhttp.RegisterScenarioScopeInitializer(catalog); err != nil {
		return err
	}

	return Register(catalog)
}
