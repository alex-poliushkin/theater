package temporalfmt

import (
	"fmt"
	"strings"
	"time"
)

const (
	DateFormatISO   = "iso"
	DateFormatBasic = "basic"
)

const (
	dateLayoutISO   = "2006-01-02"
	dateLayoutBasic = "20060102"
)

func SupportedDateFormats() []string {
	return []string{DateFormatISO, DateFormatBasic}
}

func ValidateDateFormat(format string) error {
	switch format {
	case DateFormatISO, DateFormatBasic:
		return nil
	default:
		return unsupportedDateFormatError(format)
	}
}

func FormatDate(t time.Time, format string) (string, error) {
	utc := t.UTC()
	switch format {
	case DateFormatISO:
		return utc.Format(dateLayoutISO), nil
	case DateFormatBasic:
		return utc.Format(dateLayoutBasic), nil
	default:
		return "", unsupportedDateFormatError(format)
	}
}

func SupportedDateFormatsText() string {
	return strings.Join(SupportedDateFormats(), ", ")
}

func unsupportedDateFormatError(format string) error {
	return fmt.Errorf("format %q is not supported (supported: %s)", format, SupportedDateFormatsText())
}
