package theater

import (
	"encoding/json"

	reportmodel "github.com/alex-poliushkin/theater/report"
)

// RunDocument is the serializable top-level artifact for one stage run.
type RunDocument = reportmodel.RunDocument

// Document returns the canonical run document form of r.
func (r RunResult) Document() RunDocument {
	return RunDocument{
		SchemaVersion: RunDocumentSchemaVersion,
		Diagnostics:   cloneDiagnostics(r.Diagnostics),
		Report:        r.Report,
	}
}

// MarshalJSON marshals r as its canonical RunDocument form.
func (r RunResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.Document())
}

func cloneDiagnostics(diagnostics []Diagnostic) []Diagnostic {
	if len(diagnostics) == 0 {
		return nil
	}

	cloned := make([]Diagnostic, len(diagnostics))
	copy(cloned, diagnostics)
	return cloned
}
