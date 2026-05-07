package testkit

import "github.com/alex-poliushkin/theater"

type EventRecorder struct {
	events []theater.Event
}

func ReplayReport(events []theater.Event) (theater.Report, error) {
	return theater.NewProjector().Project(events)
}

func ReplayRunDocument(events []theater.Event) (theater.RunDocument, error) {
	return theater.NewProjector().Document(events)
}

func (r *EventRecorder) Events() []theater.Event {
	copied := make([]theater.Event, len(r.events))
	copy(copied, r.events)
	return copied
}

func (r *EventRecorder) Record(event theater.Event) error {
	if err := event.Validate(); err != nil {
		return err
	}

	r.events = append(r.events, event)
	return nil
}
