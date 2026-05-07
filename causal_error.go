package theater

import "errors"

type causalError struct {
	summary string
	cause   error
}

func newCausalError(summary string, cause error) error {
	if cause == nil {
		return errors.New(summary)
	}

	return causalError{summary: summary, cause: cause}
}

func (e causalError) Error() string {
	return e.summary
}

func (e causalError) Unwrap() error {
	return e.cause
}
