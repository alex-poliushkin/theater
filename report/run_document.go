package report

import "errors"

// RunDocument is the serializable top-level artifact for one stage run.
type RunDocument struct {
	ReportSchemaVersion string       `json:"report_schema_version"`
	TheaterVersion      string       `json:"theater_version"`
	RunID               string       `json:"run_id"`
	Diagnostics         []Diagnostic `json:"diagnostics,omitempty"`
	Report              Report       `json:"report"`
}

// Validate checks the public run document contract.
func (d RunDocument) Validate() error {
	if d.ReportSchemaVersion == "" {
		return errors.New("report_schema_version is required")
	}
	if d.ReportSchemaVersion != RunDocumentSchemaVersion {
		return errors.New("unsupported report_schema_version")
	}
	if d.TheaterVersion == "" {
		return errors.New("theater_version is required")
	}
	if d.RunID == "" {
		return errors.New("run_id is required")
	}

	return d.Report.Validate()
}
