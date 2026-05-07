package theater

import "strings"

const internalScenarioSegment = "internal"

type scenarioAddress string
type internalScenarioAccessPolicy struct{}

type scenarioAddressRegistry struct {
	byAddress map[scenarioAddress]*scenarioPlan
}

func newScenarioAddressRegistry(scenarios []scenarioPlan) scenarioAddressRegistry {
	registry := scenarioAddressRegistry{
		byAddress: make(map[scenarioAddress]*scenarioPlan, len(scenarios)),
	}

	for i := range scenarios {
		scenario := &scenarios[i]
		registry.byAddress[scenarioAddress(scenario.ID)] = scenario
	}

	return registry
}

func (r scenarioAddressRegistry) Resolve(address scenarioAddress) (*scenarioPlan, bool) {
	scenario, ok := r.byAddress[address]
	return scenario, ok
}

func (p internalScenarioAccessPolicy) AllowsDirectCall(address scenarioAddress) bool {
	return !address.hasInternalHelperNamespace()
}

func (a scenarioAddress) hasInternalHelperNamespace() bool {
	segments := strings.Split(string(a), "/")
	for i := range segments {
		if segments[i] != internalScenarioSegment {
			continue
		}
		if i == len(segments)-1 {
			return false
		}

		return true
	}

	return false
}
