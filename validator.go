package theater

import "context"

// Validator compiles, validates, and prepares stage specs without running
// them.
type Validator struct {
	catalog   CatalogResolver
	matchers  MatcherResolver
	compiler  compiler
	structure planValidator
	contracts contractValidator
	preparer  planPreparer
}

// NewValidator constructs a Validator from a catalog resolver and matcher
// resolver.
func NewValidator(catalog CatalogResolver, matchers MatcherResolver) *Validator {
	return &Validator{
		catalog:   catalog,
		matchers:  matchers,
		compiler:  compiler{},
		structure: planValidator{},
		contracts: contractValidator{catalog: catalog, matchers: matchers},
		preparer:  planPreparer{catalog: catalog, matchers: matchers},
	}
}

// Validate compiles spec and returns structural or contract diagnostics.
// A nil receiver is invalid; construct validators with NewValidator.
func (v *Validator) Validate(spec StageSpec) []Diagnostic {
	stage := v.compile(spec)
	return v.validate(context.Background(), stage)
}

func (v *Validator) compile(spec StageSpec) *stagePlan {
	return v.compiler.Compile(spec)
}

func (v *Validator) validate(ctx context.Context, stage *stagePlan) []Diagnostic {
	diagnostics := v.structure.Validate(stage)
	if len(diagnostics) > 0 {
		return diagnostics
	}

	diagnostics = v.contracts.Validate(stage)
	if len(diagnostics) != 0 {
		return diagnostics
	}

	pluginCatalog, ok := v.catalog.(pluginValidationCatalog)
	if !ok {
		return nil
	}

	return pluginCatalog.validatePlugins(ctx, stage)
}

func (v *Validator) prepare(stage *stagePlan) (*stagePlan, error) {
	return v.preparer.Prepare(stage)
}

type compiler struct{}

type planValidator struct{}

func (compiler) Compile(spec StageSpec) *stagePlan {
	return compileStageSpec(spec)
}

func (planValidator) Validate(stage *stagePlan) []Diagnostic {
	return validateStage(stage)
}
