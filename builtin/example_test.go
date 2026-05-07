package builtin_test

import (
	"fmt"

	"github.com/alex-poliushkin/theater/builtin"
	builtinaction "github.com/alex-poliushkin/theater/builtin/action"
	builtinexpectation "github.com/alex-poliushkin/theater/builtin/expectation"
)

func ExampleNewBundle() {
	bundle, err := builtin.NewBundle()
	if err != nil {
		panic(err)
	}

	_, actionErr := bundle.Catalog.ResolveAction(builtinaction.GenerateRef)
	_, matcherErr := bundle.Matchers.Resolve(builtinexpectation.EqualRef)
	fmt.Println(actionErr == nil)
	fmt.Println(matcherErr == nil)

	// Output:
	// true
	// true
}
