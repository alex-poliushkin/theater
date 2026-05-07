package report

// Public diagnostic enum values used by run documents.
const (
	SeverityError DiagnosticSeverity = "error"
	SeverityHint  DiagnosticSeverity = "hint"
)

// DiagnosticSeverity classifies compile and validation diagnostics.
type DiagnosticSeverity string

// SourceRef points to a source location in an authoring file when known.
type SourceRef struct {
	File   string `yaml:"file" json:"file"`
	Line   int    `yaml:"line" json:"line"`
	Column int    `yaml:"column" json:"column"`
}

// Diagnostic reports one compile or validate issue against a plan path.
type Diagnostic struct {
	Code     string             `yaml:"code" json:"code"`
	Path     string             `yaml:"path" json:"path"`
	Severity DiagnosticSeverity `yaml:"severity" json:"severity"`
	Summary  string             `yaml:"summary" json:"summary"`
	Span     SourceRef          `yaml:"span" json:"span"`
}
