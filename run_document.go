package theater

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	reportmodel "github.com/alex-poliushkin/theater/report"
)

// RunDocument is the serializable top-level artifact for one stage run.
type RunDocument = reportmodel.RunDocument

// Document returns the canonical run document form of r.
func (r RunResult) Document() RunDocument {
	identity := r.identity()
	return RunDocument{
		ReportSchemaVersion: RunDocumentSchemaVersion,
		TheaterVersion:      identity.theaterVersion,
		RunID:               identity.runID,
		Diagnostics:         cloneDiagnostics(r.Diagnostics),
		Report:              r.Report,
	}
}

// MarshalJSON marshals r as its canonical RunDocument form.
func (r RunResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.Document())
}

type runDocumentIdentity struct {
	runID          string
	theaterVersion string
}

func newRunDocumentIdentity(stageID string, options RunOptions) runDocumentIdentity {
	identity := runDocumentIdentity{
		runID:          options.RunID,
		theaterVersion: Version(),
	}
	if identity.runID == "" {
		identity.runID = newRunDocumentID(stageID)
	}
	return identity
}

func runDocumentIdentityFromEvents(events []Event) (runDocumentIdentity, error) {
	if len(events) == 0 {
		return runDocumentIdentity{}, errors.New("run document identity is required")
	}

	var identity runDocumentIdentity
	for i := range events {
		eventIdentity := runDocumentIdentity{
			runID:          events[i].RunID,
			theaterVersion: events[i].TheaterVersion,
		}
		if eventIdentity.runID == "" || eventIdentity.theaterVersion == "" {
			return runDocumentIdentity{}, fmt.Errorf("event %d run document identity is incomplete", i)
		}
		if identity.runID == "" {
			identity = eventIdentity
			continue
		}
		if eventIdentity != identity {
			return runDocumentIdentity{}, fmt.Errorf("event %d run document identity mismatch", i)
		}
	}
	return identity, nil
}

func (i runDocumentIdentity) result(result RunResult) RunResult {
	result.RunID = i.runID
	result.TheaterVersion = i.theaterVersion
	return result
}

func (r RunResult) identity() runDocumentIdentity {
	identity := runDocumentIdentity{
		runID:          r.RunID,
		theaterVersion: r.TheaterVersion,
	}
	if identity.runID == "" {
		identity.runID = newRunDocumentID(r.Report.StageID)
	}
	if identity.theaterVersion == "" {
		identity.theaterVersion = Version()
	}
	return identity
}

func newRunDocumentID(stageID string) string {
	if stageID == "" {
		stageID = "run"
	}

	return fmt.Sprintf("%s/%d", stageID, time.Now().UTC().UnixNano())
}

func cloneDiagnostics(diagnostics []Diagnostic) []Diagnostic {
	if len(diagnostics) == 0 {
		return nil
	}

	cloned := make([]Diagnostic, len(diagnostics))
	copy(cloned, diagnostics)
	return cloned
}
