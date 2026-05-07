package theater

import "context"

const (
	DebugBoundaryKindScenarioCall DebugBoundaryKind = debugBoundaryKindScenarioCall
	DebugBoundaryKindAct          DebugBoundaryKind = debugBoundaryKindAct
	DebugBoundaryKindAction       DebugBoundaryKind = debugBoundaryKindAction
	DebugBoundaryKindExpectation  DebugBoundaryKind = debugBoundaryKindExpectation

	DebugBoundaryPhaseBefore DebugBoundaryPhase = debugBoundaryPhaseBefore
	DebugBoundaryPhaseAfter  DebugBoundaryPhase = debugBoundaryPhaseAfter
)

// DebugBoundaryKind identifies one debuggable runtime-node family.
type DebugBoundaryKind = debugBoundaryKind

// DebugBoundaryPhase identifies whether the boundary is reached before or after
// node execution.
type DebugBoundaryPhase = debugBoundaryPhase

// DebugPath describes one debuggable runtime boundary that can be targeted by a
// breakpoint selector.
type DebugPath struct {
	Path       string             `json:"path"`
	Kind       DebugBoundaryKind  `json:"kind"`
	Phase      DebugBoundaryPhase `json:"phase"`
	RetryAware bool               `json:"retry_aware"`
}

// DebugPathListing returns either prepared debug paths or validation
// diagnostics for the supplied stage spec.
type DebugPathListing struct {
	Diagnostics []Diagnostic
	Paths       []DebugPath
}

// ListDebugPaths compiles, validates, and prepares a stage spec, then returns
// the debuggable runtime paths available for breakpoint discovery.
func (v *Validator) ListDebugPaths(spec StageSpec) (DebugPathListing, error) {
	stage := v.compile(spec)
	diagnostics := v.validate(context.Background(), stage)
	if len(diagnostics) != 0 {
		return DebugPathListing{Diagnostics: diagnostics}, nil
	}

	prepared, err := v.prepare(stage)
	if err != nil {
		return DebugPathListing{}, err
	}

	paths, err := buildDebugPaths(prepared)
	if err != nil {
		return DebugPathListing{}, err
	}

	return DebugPathListing{Paths: paths}, nil
}

func buildDebugPaths(stage *stagePlan) ([]DebugPath, error) {
	boundaries, err := collectDebugBoundaries(stage)
	if err != nil {
		return nil, err
	}

	paths := make([]DebugPath, 0, len(boundaries))
	for i := range boundaries {
		paths = append(paths, DebugPath{
			Path:       boundaries[i].Path,
			Kind:       boundaries[i].Kind,
			Phase:      boundaries[i].Phase,
			RetryAware: boundaries[i].RetryAware,
		})
	}

	return paths, nil
}
