package theater

import "sync"

// DiagnosticRecorder receives typed report-safe diagnostics from an action.
type DiagnosticRecorder interface {
	RecordDiagnostic(NodeDiagnostic)
}

type actionDiagnosticCollector struct {
	mu          sync.Mutex
	diagnostics []NodeDiagnostic
}

func (c *actionDiagnosticCollector) RecordDiagnostic(diagnostic NodeDiagnostic) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.diagnostics = append(c.diagnostics, cloneNodeDiagnostic(diagnostic))
}

func (c *actionDiagnosticCollector) Snapshot() []NodeDiagnostic {
	c.mu.Lock()
	defer c.mu.Unlock()

	return cloneNodeDiagnostics(c.diagnostics)
}
