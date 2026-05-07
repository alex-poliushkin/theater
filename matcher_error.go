package theater

import "errors"

// MatcherMismatch marks an error as an ordinary matcher mismatch rather than a
// matcher-internal or terminal failure.
type MatcherMismatch interface {
	TheaterMismatch() bool
}

// MismatchError wraps err so matcher wrappers such as expectation.not can
// distinguish ordinary non-match results from matcher-internal failures.
func MismatchError(err error) error {
	if err == nil {
		return nil
	}

	return matcherMismatchError{err: err}
}

// IsMatcherMismatch reports whether err represents an ordinary matcher
// mismatch.
func IsMatcherMismatch(err error) bool {
	if err == nil {
		return false
	}

	var marker MatcherMismatch
	return errors.As(err, &marker) && marker.TheaterMismatch()
}

type matcherMismatchError struct {
	err error
}

func (e matcherMismatchError) Error() string {
	return e.err.Error()
}

func (e matcherMismatchError) Unwrap() error {
	return e.err
}

func (matcherMismatchError) TheaterMismatch() bool {
	return true
}
