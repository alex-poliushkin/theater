package decorator

import "github.com/alex-poliushkin/theater"

// Register installs the built-in decorators into catalog.
func Register(catalog theater.DecoratorRegistrar) error {
	if err := catalog.RegisterDecorator(JSONRef, jsonDecoratorDef()); err != nil {
		return err
	}

	if err := catalog.RegisterDecorator(CSVRef, csvDecoratorDef()); err != nil {
		return err
	}

	return nil
}
