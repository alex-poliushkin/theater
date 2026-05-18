package theater

import (
	"context"
	"errors"
)

// Runner compiles, validates, prepares, and executes stage specs.
type Runner struct {
	catalog   CatalogResolver
	matchers  MatcherResolver
	validator *Validator
}

// NewRunner constructs a Runner from a catalog resolver and matcher resolver.
func NewRunner(catalog CatalogResolver, matchers MatcherResolver) *Runner {
	return &Runner{
		catalog:   catalog,
		matchers:  matchers,
		validator: NewValidator(catalog, matchers),
	}
}

// Run executes spec and returns either validation diagnostics or a final
// report.
func (r *Runner) Run(ctx context.Context, spec StageSpec, options RunOptions) (result RunResult, err error) {
	var debug *debugRuntime
	if options.Debug != nil {
		built, enabled, err := buildDebugRuntime(options.Debug)
		if err != nil {
			return RunResult{}, err
		}
		if enabled {
			debug = built
		}
	}
	if debug != nil {
		defer func() {
			if closeErr := debug.close(context.WithoutCancel(ctx)); closeErr != nil && err == nil {
				err = closeErr
			}
		}()
	}

	return r.runWithDebugRuntime(ctx, spec, options, debug)
}

func (r *Runner) runWithDebugRuntime(
	ctx context.Context,
	spec StageSpec,
	options RunOptions,
	debug *debugRuntime,
) (result RunResult, err error) {
	if r == nil || dependencyMissing(r.catalog) {
		return RunResult{}, errors.New("catalog is required")
	}

	if dependencyMissing(r.matchers) {
		return RunResult{}, errors.New("matcher catalog is required")
	}

	stage := r.validator.compile(spec)
	identity := newRunDocumentIdentity(stage.ID, options)
	diagnostics := r.validator.validate(ctx, stage)
	if len(diagnostics) > 0 {
		return identity.result(RunResult{
			Diagnostics: diagnostics,
			Report:      definitionFailureReport(stage.ID, stage.Path, diagnostics),
		}), nil
	}

	if failure := preflightCatalog(stage, r.catalog); failure != nil {
		return identity.result(RunResult{
			Report: setupFailureReport(stage.ID, stage.Path, failure),
		}), nil
	}

	prepared, err := r.validator.prepare(stage)
	if err != nil {
		return identity.result(prepareFailureResult(stage.ID, stage.Path, err)), nil
	}

	if err := debug.prepareBreakpoints(prepared); err != nil {
		closeErr := debug.close(context.WithoutCancel(ctx))
		if closeErr != nil {
			err = errors.Join(err, closeErr)
		}

		return identity.result(prepareFailureResult(stage.ID, stage.Path, err)), nil
	}

	closePlugins := func(context.Context) {}
	if pluginCatalog, ok := r.catalog.(pluginRunCatalog); ok {
		ctx, closePlugins, err = pluginCatalog.preparePluginRun(ctx, prepared)
		if err != nil {
			failure := setupFailure(prepared.Path, err)
			return identity.result(RunResult{
				Report: setupFailureReport(stage.ID, stage.Path, failure),
			}), nil
		}
	}
	defer closePlugins(context.WithoutCancel(ctx))

	live, closeDebug, err := debug.prepareRun(options.Live)
	if err != nil {
		failure := setupFailure(prepared.Path, err)
		return identity.result(RunResult{
			Report: setupFailureReport(stage.ID, stage.Path, failure),
		}), nil
	}
	defer func() {
		if closeErr := closeDebug(context.WithoutCancel(ctx)); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	stateManager, err := r.openRunStateManager(ctx, prepared, debug)
	if err != nil {
		failure := setupFailure(prepared.Path, err)
		return identity.result(RunResult{
			Report: setupFailureReport(stage.ID, stage.Path, failure),
		}), nil
	}

	report, err := newStageRunner(
		prepared,
		r.catalog,
		r.matchers,
		stateManager,
		live,
		options.Events,
		identity,
		debug,
	).Run(ctx)
	if err != nil {
		return RunResult{}, err
	}

	result = identity.result(RunResult{
		Report: report,
	})
	if err := ExportRunDocument(ctx, r.catalog, result.Document(), options.ReportExporters); err != nil {
		return result, err
	}

	return result, nil
}

func prepareFailureResult(stageID, stagePath string, err error) RunResult {
	failure := internalFailure(stagePath, "stage preparation failed", err)
	var prepErr planPreparationError
	if errors.As(err, &prepErr) {
		failure.At = prepErr.Path()
	}

	return RunResult{
		Report: setupFailureReport(stageID, stagePath, failure),
	}
}

func (r *Runner) openRunStateManager(
	ctx context.Context,
	prepared *stagePlan,
	debug *debugRuntime,
) (*StateManager, error) {
	if debug == nil {
		return openStateManager(ctx, prepared.State, r.catalog)
	}

	debug.ensureStateRecorder()
	return openStateManagerWithDecorator(ctx, prepared.State, r.catalog, debug.wrapStateBackend)
}
