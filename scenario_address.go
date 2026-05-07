package theater

import (
	"errors"
	"strings"
)

func validateScenarioAddress(address string) error {
	if address == "" {
		return errors.New("scenario address is required")
	}

	if strings.HasPrefix(address, "/") || strings.HasSuffix(address, "/") {
		return errors.New("scenario address must not start or end with /")
	}

	segments := strings.Split(address, "/")
	for _, segment := range segments {
		switch segment {
		case "":
			return errors.New("scenario address must not contain empty segments")
		case ".":
			return errors.New(`scenario address segment "." is invalid`)
		case "..":
			return errors.New(`scenario address segment ".." is invalid`)
		}
	}

	return nil
}
