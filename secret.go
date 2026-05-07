package theater

import "github.com/alex-poliushkin/theater/internal/secretvalue"

// Secret wraps a sensitive runtime value so reporting code can redact it.
type Secret = secretvalue.Value

// NewSecret marks value as sensitive for runtime transport and reporting.
func NewSecret(value any) Secret {
	return secretvalue.New(value)
}
