package theater

import (
	"fmt"
	"strconv"

	"github.com/alex-poliushkin/theater/internal/runtimevalue"
)

type planFragmentCompiler struct{}

func (c planFragmentCompiler) compileBinding(path string, spec BindingSpec) bindingPlan {
	plan := bindingPlan{
		Path:       path,
		Kind:       spec.Kind,
		Value:      runtimevalue.Clone(spec.Value),
		Ref:        c.compileRef(path, spec.Ref),
		Object:     make(map[string]bindingPlan, len(spec.Object)),
		List:       make([]bindingPlan, 0, len(spec.List)),
		Parts:      make([]bindingPlan, 0, len(spec.Parts)),
		Generator:  spec.Generator,
		Args:       make(map[string]bindingPlan, len(spec.Args)),
		SourceSpan: cloneSourceRef(spec.SourceSpan),
	}

	for key := range spec.Object {
		plan.Object[key] = c.compileBinding(bindingPath(path, key), spec.Object[key])
	}

	for i := range spec.List {
		plan.List = append(plan.List, c.compileBinding(bindingPath(path, listBindingKey(i)), spec.List[i]))
	}

	for i := range spec.Parts {
		plan.Parts = append(plan.Parts, c.compileBinding(bindingPath(path, partBindingKey(i)), spec.Parts[i]))
	}

	for key := range spec.Args {
		plan.Args[key] = c.compileBinding(bindingPath(path, key), spec.Args[key])
	}

	return plan
}

func (c planFragmentCompiler) compileExport(path string, spec ExportSpec) exportPlan {
	return exportPlan{
		Path:  path,
		As:    spec.As,
		Ref:   c.compileRef(path, spec.Ref),
		Field: spec.Field,
		selectorPlan: selectorPlan{
			Decode:  spec.Decode,
			Path:    spec.Path,
			Through: c.compileThrough(path, spec.Through),
		},
	}
}

func (c planFragmentCompiler) compileRef(path string, spec *RefSpec) *refPlan {
	if spec == nil {
		return nil
	}

	return &refPlan{
		Name: spec.Name,
		selectorPlan: selectorPlan{
			Decode:  spec.Decode,
			Path:    spec.Path,
			Through: c.compileThrough(path, spec.Through),
		},
	}
}

func (c planFragmentCompiler) compileExpectation(path string, spec ExpectationSpec) expectationPlan {
	expectation := expectationPlan{
		ID: spec.ID,
		Subject: subjectPlan{
			From:  spec.Subject.From,
			Ref:   spec.Subject.Ref,
			Field: spec.Subject.Field,
			selectorPlan: selectorPlan{
				Decode:  spec.Subject.Decode,
				Path:    spec.Subject.Path,
				Through: c.compileThrough(path+"/subject", spec.Subject.Through),
			},
		},
		Assert:     c.compileAssert(path+"/assert", spec.Assert),
		SourceSpan: cloneSourceRef(spec.SourceSpan),
	}

	return expectation
}

func (c planFragmentCompiler) compileLog(path string, spec LogSpec) logPlan {
	sensitivity, capture := normalizeVisibility(ValueContract{
		Sensitivity: spec.Sensitivity,
		Capture:     spec.Capture,
	})

	log := logPlan{
		ID:          spec.ID,
		Path:        path,
		Value:       c.compileLogValue(path+"/value", spec.Value),
		Message:     spec.Message,
		Fields:      make(map[string]logValuePlan, len(spec.Fields)),
		Format:      spec.Format,
		Capture:     capture,
		Sensitivity: sensitivity,
		Required:    spec.Required,
		SourceSpan:  cloneSourceRef(spec.SourceSpan),
	}

	for key := range spec.Fields {
		log.Fields[key] = c.compileLogValue(bindingChildPath(path+"/fields", key), spec.Fields[key])
	}

	return log
}

func (c planFragmentCompiler) compileLogValue(path string, spec LogValueSpec) logValuePlan {
	value := logValuePlan{
		Path:       path,
		Field:      spec.Field,
		Ref:        spec.Ref,
		Object:     make(map[string]logValuePlan, len(spec.Object)),
		List:       make([]logValuePlan, 0, len(spec.List)),
		SourceSpan: cloneSourceRef(spec.SourceSpan),
		selectorPlan: selectorPlan{
			Decode:  spec.Decode,
			Path:    spec.Path,
			Through: c.compileThrough(path, spec.Through),
		},
	}

	for key := range spec.Object {
		value.Object[key] = c.compileLogValue(bindingChildPath(path, key), spec.Object[key])
	}

	for i := range spec.List {
		value.List = append(value.List, c.compileLogValue(fmt.Sprintf("%s[%d]", path, i), spec.List[i]))
	}

	return value
}

func (planFragmentCompiler) compileDecorator(spec DecoratorSpec) decoratorPlan {
	return decoratorPlan{
		Use:  spec.Use,
		With: cloneValues(Values(spec.With)),
	}
}

func (c planFragmentCompiler) compileThrough(path string, specs []ThroughStepSpec) []throughStepPlan {
	if len(specs) == 0 {
		return nil
	}

	plans := make([]throughStepPlan, 0, len(specs))
	for i := range specs {
		plan := throughStepPlan{
			Path: specs[i].Path,
		}
		stepPath := joinChildPath(path, "through", strconv.Itoa(i))
		if specs[i].Pick != nil {
			plan.Pick = &pickStepPlan{
				At:     specs[i].Pick.At,
				Equals: c.compileBinding(bindingPath(stepPath, "equals"), specs[i].Pick.Equals),
				Where:  c.compilePickWhereClauses(stepPath+"/pick", specs[i].Pick.Where),
			}
		}
		if specs[i].Regexp != nil {
			plan.Regexp = &regexpStepPlan{
				Pattern: specs[i].Regexp.Pattern,
				Group:   specs[i].Regexp.Group,
			}
		}
		if specs[i].Transform != nil {
			transform := c.compileDecorator(*specs[i].Transform)
			plan.Transform = &transform
		}

		plans = append(plans, plan)
	}

	return plans
}

func (c planFragmentCompiler) compilePickWhereClauses(path string, specs []PickWhereClauseSpec) []pickWhereClausePlan {
	if len(specs) == 0 {
		return nil
	}

	clauses := make([]pickWhereClausePlan, 0, len(specs))
	for i := range specs {
		clausePath := joinChildPath(path, "where", strconv.Itoa(i))
		clauses = append(clauses, pickWhereClausePlan{
			Subject: relativeSubjectPlan{
				Decode: specs[i].Subject.Decode,
				Path:   specs[i].Subject.Path,
			},
			Assert: c.compileAssert(clausePath+"/assert", specs[i].Assert),
		})
	}

	return clauses
}

func (c planFragmentCompiler) compileAssert(path string, spec AssertSpec) assertPlan {
	assert := assertPlan{
		Ref:  spec.Ref,
		Args: make(map[string]bindingPlan, len(spec.Args)),
	}

	for key := range spec.Args {
		assert.Args[key] = c.compileBinding(bindingPath(path, key), spec.Args[key])
	}

	return assert
}

func listBindingKey(index int) string {
	return "item-" + strconv.Itoa(index)
}

func partBindingKey(index int) string {
	return "part-" + strconv.Itoa(index)
}
