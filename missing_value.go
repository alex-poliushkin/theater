package theater

import (
	"errors"
	"fmt"
)

type missingValue struct {
	reason string
}

func newMissingValue(reason string) missingValue {
	return missingValue{reason: reason}
}

func isMissingValue(value any) bool {
	_, ok := value.(missingValue)
	return ok
}

func missingValueError(value any) error {
	if missing, ok := value.(missingValue); ok && missing.reason != "" {
		return fmt.Errorf("%s is missing", missing.reason)
	}

	return errors.New("value is missing")
}
