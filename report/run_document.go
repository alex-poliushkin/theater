package report

import "errors"

// RunDocument is the serializable top-level artifact for one stage run.
type RunDocument struct {
	SchemaVersion string       `json:"schema_version"`
	Diagnostics   []Diagnostic `json:"diagnostics,omitempty"`
	Report        Report       `json:"report"`
}

// Validate checks the public run document contract.
func (d RunDocument) Validate() error {
	if d.SchemaVersion == "" {
		return errors.New("schema_version is required")
	}
	if d.SchemaVersion != RunDocumentSchemaVersion {
		return errors.New("unsupported schema_version")
	}

	return d.Report.Validate()
}
