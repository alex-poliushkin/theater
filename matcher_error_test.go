package theater_test

import (
	"errors"
	"testing"

	"github.com/alex-poliushkin/theater"
)

func TestMismatchErrorMarksWrappedError(t *testing.T) {
	t.Parallel()

	cause := errors.New("not equal")
	err := theater.MismatchError(cause)

	if !theater.IsMatcherMismatch(err) {
		t.Fatal("wrapped mismatch error must be detectable")
	}

	if !errors.Is(err, cause) {
		t.Fatalf("wrapped mismatch error must preserve cause: %v", err)
	}
}

func TestMismatchErrorLeavesNilUntouched(t *testing.T) {
	t.Parallel()

	if err := theater.MismatchError(nil); err != nil {
		t.Fatalf("wrapping nil must return nil: %v", err)
	}
}
