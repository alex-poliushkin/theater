package thtr

import "github.com/alex-poliushkin/theater"

type DiagnosticError struct {
	diagnostic theater.Diagnostic
}

func (e *DiagnosticError) Error() string {
	return e.diagnostic.Summary
}

func (e *DiagnosticError) Diagnostic() theater.Diagnostic {
	return e.diagnostic
}
