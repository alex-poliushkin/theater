package theater_test

import (
	"context"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/builtin"
)

func newBuiltins() (*theater.Catalog, *theater.MatcherCatalog, error) {
	bundle, err := builtin.NewBundle()
	if err != nil {
		return nil, nil, err
	}

	return bundle.Catalog, bundle.Matchers, nil
}

func newMatcherCatalog(descriptors ...theater.MatcherDescriptor) (*theater.MatcherCatalog, error) {
	return theater.NewMatcherCatalog(descriptors...)
}

func runStage(
	ctx context.Context,
	spec theater.StageSpec,
	catalog *theater.Catalog,
	matchers *theater.MatcherCatalog,
) (theater.RunResult, error) {
	return runStageWithOptions(ctx, spec, catalog, matchers, theater.RunOptions{})
}

func runStageWithOptions(
	ctx context.Context,
	spec theater.StageSpec,
	catalog *theater.Catalog,
	matchers *theater.MatcherCatalog,
	options theater.RunOptions,
) (theater.RunResult, error) {
	return theater.NewRunner(catalog, matchers).Run(ctx, spec, options)
}

func validateStage(
	spec theater.StageSpec,
	catalog *theater.Catalog,
	matchers *theater.MatcherCatalog,
) []theater.Diagnostic {
	return theater.NewValidator(catalog, matchers).Validate(spec)
}

func projectReport(events []theater.Event) (theater.Report, error) {
	return theater.NewProjector().Project(events)
}

func projectRunDocument(events []theater.Event) (theater.RunDocument, error) {
	return theater.NewProjector().Document(events)
}
