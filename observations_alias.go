package theater

import reportmodel "github.com/alex-poliushkin/theater/report"

// ObservedValue stores the report-safe observation of one action input or
// output.
type ObservedValue = reportmodel.ObservedValue

// ObservedStream stores the report-safe observation of one streamed output.
type ObservedStream = reportmodel.ObservedStream

// ActionObservations groups observed action inputs, outputs, and streams.
type ActionObservations = reportmodel.ActionObservations
