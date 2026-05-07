package theater

// Projector materializes final reports and run documents from runtime events.
type Projector struct {
	projector reportProjector
}

// NewProjector constructs a Projector with the default report projection
// behavior.
func NewProjector() *Projector {
	return &Projector{projector: reportProjector{}}
}

// Project materializes a final Report from runtime events.
func (p *Projector) Project(events []Event) (Report, error) {
	if p == nil {
		return reportProjector{}.Project(events)
	}

	return p.projector.Project(events)
}

// Document materializes a RunDocument from runtime events.
func (p *Projector) Document(events []Event) (RunDocument, error) {
	report, err := p.Project(events)
	if err != nil {
		return RunDocument{}, err
	}

	return RunDocument{
		SchemaVersion: RunDocumentSchemaVersion,
		Report:        report,
	}, nil
}

type reportProjector struct{}
