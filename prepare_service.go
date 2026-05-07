package theater

import (
	"errors"
	"fmt"
	"time"
)

type planPreparer struct {
	catalog  propertyCatalog
	matchers MatcherResolver
}

type planPreparationError struct {
	path  string
	cause error
}

func (p planPreparer) Prepare(stage *stagePlan) (*stagePlan, error) {
	if stage == nil {
		return nil, errors.New("stage plan is required")
	}

	for i := range stage.Scenarios {
		scenario := &stage.Scenarios[i]
		for j := range scenario.Acts {
			if err := p.prepareAct(&scenario.Acts[j]); err != nil {
				return nil, err
			}
		}
	}

	return stage, nil
}

func (e planPreparationError) Error() string {
	return e.cause.Error()
}

func (e planPreparationError) Unwrap() error {
	return e.cause
}

func (e planPreparationError) Path() string {
	return e.path
}

func (p planPreparer) prepareAct(act *actPlan) error {
	if err := prepareEventually(act); err != nil {
		return err
	}

	if !dependencyMissing(p.catalog) {
		if err := p.prepareProperties(act); err != nil {
			return err
		}
	}

	if !dependencyMissing(p.matchers) {
		if err := p.prepareMatchers(act); err != nil {
			return err
		}
	}

	return nil
}

func (p planPreparer) prepareProperties(act *actPlan) error {
	for i := range act.Properties {
		property := &act.Properties[i]
		if !property.Inventory.Present || property.Inventory.Use == "" {
			continue
		}

		if _, err := p.catalog.ResolveInventory(property.Inventory.Use); err != nil {
			return newPlanPreparationError(property.Path, err)
		}

		for j := range property.Decorators {
			decorator := &property.Decorators[j]
			if decorator.Use == "" {
				continue
			}

			decoratorPath := joinChildPath(property.Path, "decorator", decoratorKey(decorator, j))
			def, err := p.catalog.ResolveDecorator(decorator.Use)
			if err != nil {
				return newPlanPreparationError(decoratorPath, err)
			}

			resolvedArgs, err := resolveDecoratorArgs(decorator.With, def.Contract.Params)
			if err != nil {
				return newPlanPreparationError(decoratorPath, err)
			}

			transform, err := def.Compile(cloneValues(resolvedArgs))
			if err != nil {
				return newPlanPreparationError(decoratorPath, err)
			}

			decorator.With = resolvedArgs
			decorator.Contract = cloneDecoratorContract(def.Contract)
			decorator.Transform = transform
		}
	}

	return nil
}

func (p planPreparer) prepareMatchers(act *actPlan) error {
	for i := range act.Expectations {
		expectation := &act.Expectations[i]
		if expectation.Assert.Ref == "" {
			continue
		}

		path := joinChildPath(act.Path, "expectation", expectation.ID)
		descriptor, err := p.matchers.Resolve(expectation.Assert.Ref)
		if err != nil {
			return newPlanPreparationError(path, err)
		}

		expectation.Matcher = descriptor
	}

	return nil
}

func prepareEventually(act *actPlan) error {
	if act.Eventually == nil {
		return nil
	}

	timeout, err := time.ParseDuration(act.Eventually.TimeoutText)
	if err != nil {
		return newPlanPreparationError(
			act.Path+"/eventually/timeout",
			newCausalError(fmt.Sprintf("act %q eventually timeout must be a positive duration", act.ID), err),
		)
	}
	if timeout <= 0 {
		return newPlanPreparationError(
			act.Path+"/eventually/timeout",
			fmt.Errorf("act %q eventually timeout must be a positive duration", act.ID),
		)
	}

	interval, err := time.ParseDuration(act.Eventually.IntervalText)
	if err != nil {
		return newPlanPreparationError(
			act.Path+"/eventually/interval",
			newCausalError(fmt.Sprintf("act %q eventually interval must be a positive duration", act.ID), err),
		)
	}
	if interval <= 0 {
		return newPlanPreparationError(
			act.Path+"/eventually/interval",
			fmt.Errorf("act %q eventually interval must be a positive duration", act.ID),
		)
	}

	if interval >= timeout {
		return newPlanPreparationError(
			act.Path+"/eventually/interval",
			fmt.Errorf("act %q eventually interval must be shorter than timeout", act.ID),
		)
	}

	act.Eventually.Timeout = timeout
	act.Eventually.Interval = interval
	return nil
}

func newPlanPreparationError(path string, err error) error {
	if err == nil {
		return nil
	}

	return planPreparationError{path: path, cause: err}
}
