package spec

import statemodel "github.com/alex-poliushkin/theater/state"

type StateSpec struct {
	Backends map[string]StateBackendSpec `yaml:"backends,omitempty" json:"backends,omitempty"`
}

type StateBackendSpec struct {
	Use  string         `yaml:"use" json:"use"`
	With map[string]any `yaml:"with,omitempty" json:"with,omitempty"`
}

type StateGuaranteeTier = statemodel.GuaranteeTier

type StateExpiryPolicy = statemodel.ExpiryPolicy

func (s *StateSpec) Clone() *StateSpec {
	if s == nil {
		return nil
	}

	cloned := &StateSpec{
		Backends: make(map[string]StateBackendSpec, len(s.Backends)),
	}

	for name, backend := range s.Backends {
		cloned.Backends[name] = backend.Clone()
	}

	if len(cloned.Backends) == 0 {
		cloned.Backends = nil
	}

	return cloned
}

func (s StateBackendSpec) Clone() StateBackendSpec {
	cloned := s
	if len(s.With) == 0 {
		cloned.With = nil
		return cloned
	}

	cloned.With = make(map[string]any, len(s.With))
	for key, value := range s.With {
		cloned.With[key] = value
	}

	return cloned
}
