package theatercli

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	checkStatusFail = "FAIL"
	checkStatusOK   = "OK"
	checkStatusWarn = "WARN"
)

func sanitizeCLIText(raw string) string {
	if raw == "" {
		return ""
	}

	var builder strings.Builder
	for _, r := range raw {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			builder.WriteRune(' ')
		case unicode.IsControl(r):
			fmt.Fprintf(&builder, "\\x%02x", r)
		default:
			builder.WriteRune(r)
		}
	}

	return builder.String()
}
