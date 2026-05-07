package generator

import "github.com/alex-poliushkin/theater"

// Register installs the built-in generators into a narrow generator registrar.
func Register(catalog theater.GeneratorRegistrar) error {
	defs := descriptors()
	for ref := range defs {
		if err := catalog.RegisterGenerator(ref, defs[ref]); err != nil {
			return err
		}
	}

	return nil
}
