package theater

import "github.com/alex-poliushkin/theater/internal/runtimevalue"

type valueScope struct {
	parent *valueScope
	values Values
}

func newValueScope(parent *valueScope) *valueScope {
	return &valueScope{
		parent: parent,
		values: make(Values),
	}
}

func (s *valueScope) writeAll(values Values) {
	for key, value := range values {
		s.values[key] = runtimevalue.Clone(value)
	}
}

func (s *valueScope) lookupValue(name string) (any, bool) {
	if s == nil {
		return nil, false
	}

	value, ok := s.values[name]
	if ok {
		return value, true
	}

	return s.parent.lookupValue(name)
}
