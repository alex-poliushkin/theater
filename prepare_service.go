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
	if !dependencyMissing(p.catalog) {
		for i := range stage.ScenarioCalls {
			if err := p.prepareScenarioCall(&stage.ScenarioCalls[i]); err != nil {
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
		if err := p.prepareActSelectors(act); err != nil {
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

func (p planPreparer) prepareScenarioCall(call *scenarioCallPlan) error {
	for key := range call.Bindings {
		binding := call.Bindings[key]
		if err := p.prepareBindingSelectors(&binding); err != nil {
			return err
		}
		call.Bindings[key] = binding
	}

	for i := range call.Exports {
		if err := p.prepareExportSelectors(&call.Exports[i]); err != nil {
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
		for key := range property.Inventory.With {
			binding := property.Inventory.With[key]
			if err := p.prepareBindingSelectors(&binding); err != nil {
				return err
			}
			property.Inventory.With[key] = binding
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

func (p planPreparer) prepareActSelectors(act *actPlan) error {
	for key := range act.Action.With {
		binding := act.Action.With[key]
		if err := p.prepareBindingSelectors(&binding); err != nil {
			return err
		}
		act.Action.With[key] = binding
	}

	for i := range act.Expectations {
		expectation := &act.Expectations[i]
		subjectPath := joinChildPath(act.Path, "expectation", expectation.ID) + "/subject"
		if err := p.prepareSelector(subjectPath, &expectation.Subject.selectorPlan); err != nil {
			return err
		}
		for key := range expectation.Assert.Args {
			binding := expectation.Assert.Args[key]
			if err := p.prepareBindingSelectors(&binding); err != nil {
				return err
			}
			expectation.Assert.Args[key] = binding
		}
	}

	for i := range act.Logs {
		if err := p.prepareLogValueSelectors(&act.Logs[i].Value); err != nil {
			return err
		}
		for key := range act.Logs[i].Fields {
			value := act.Logs[i].Fields[key]
			if err := p.prepareLogValueSelectors(&value); err != nil {
				return err
			}
			act.Logs[i].Fields[key] = value
		}
	}

	for i := range act.Exports {
		if err := p.prepareExportSelectors(&act.Exports[i]); err != nil {
			return err
		}
	}

	return nil
}

func (p planPreparer) prepareExportSelectors(export *exportPlan) error {
	if export.Ref != nil {
		if err := p.prepareSelector(export.Path+"/ref", &export.Ref.selectorPlan); err != nil {
			return err
		}
	}
	return p.prepareSelector(export.Path, &export.selectorPlan)
}

func (p planPreparer) prepareLogValueSelectors(value *logValuePlan) error {
	if err := p.prepareSelector(value.Path, &value.selectorPlan); err != nil {
		return err
	}
	for key := range value.Object {
		child := value.Object[key]
		if err := p.prepareLogValueSelectors(&child); err != nil {
			return err
		}
		value.Object[key] = child
	}
	for i := range value.List {
		if err := p.prepareLogValueSelectors(&value.List[i]); err != nil {
			return err
		}
	}
	return nil
}

func (p planPreparer) prepareBindingSelectors(binding *bindingPlan) error {
	if binding.Ref != nil {
		if err := p.prepareSelector(binding.Path, &binding.Ref.selectorPlan); err != nil {
			return err
		}
	}
	for key := range binding.Object {
		child := binding.Object[key]
		if err := p.prepareBindingSelectors(&child); err != nil {
			return err
		}
		binding.Object[key] = child
	}
	for i := range binding.List {
		if err := p.prepareBindingSelectors(&binding.List[i]); err != nil {
			return err
		}
	}
	for i := range binding.Parts {
		if err := p.prepareBindingSelectors(&binding.Parts[i]); err != nil {
			return err
		}
	}
	for key := range binding.Args {
		child := binding.Args[key]
		if err := p.prepareBindingSelectors(&child); err != nil {
			return err
		}
		binding.Args[key] = child
	}
	return nil
}

func (p planPreparer) prepareSelector(path string, selector *selectorPlan) error {
	for i := range selector.Through {
		transform := selector.Through[i].Transform
		if transform == nil || transform.Use == "" {
			continue
		}

		if err := p.prepareThroughTransform(fmt.Sprintf("%s/through[%d]/transform", path, i), transform); err != nil {
			return err
		}
	}
	return nil
}

func (p planPreparer) prepareThroughTransform(path string, transform *decoratorPlan) error {
	def, err := p.catalog.ResolveDecorator(transform.Use)
	if err != nil {
		return newPlanPreparationError(path, err)
	}

	resolvedArgs, err := resolveDecoratorArgs(transform.With, def.Contract.Params)
	if err != nil {
		return newPlanPreparationError(path, err)
	}

	compiled, err := def.Compile(cloneValues(resolvedArgs))
	if err != nil {
		return newPlanPreparationError(path, err)
	}

	transform.With = resolvedArgs
	transform.Contract = cloneDecoratorContract(def.Contract)
	transform.Transform = compiled
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
