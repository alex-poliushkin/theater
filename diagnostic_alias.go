package theater

import reportmodel "github.com/alex-poliushkin/theater/report"

// DiagnosticSeverity classifies compile and validation diagnostics.
type DiagnosticSeverity = reportmodel.DiagnosticSeverity

// SourceRef points to a source location in an authoring file when known.
type SourceRef = reportmodel.SourceRef

// Diagnostic reports one compile or validate issue against a plan path.
type Diagnostic = reportmodel.Diagnostic
