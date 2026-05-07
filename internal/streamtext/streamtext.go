package streamtext

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

func Render(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.Grow(len(data))

	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		if r == utf8.RuneError && size == 1 {
			appendHexEscape(&builder, data[0])
			data = data[1:]
			continue
		}

		switch r {
		case '\t':
			builder.WriteString("\\t")
		case '\n':
			builder.WriteString("\\n")
		case '\r':
			builder.WriteString("\\r")
		default:
			if r < 0x20 || r == 0x7f {
				appendHexEscape(&builder, data[0])
			} else {
				builder.WriteRune(r)
			}
		}

		data = data[size:]
	}

	return builder.String()
}

func SafePrefixLen(data []byte, limit int) int {
	if len(data) == 0 || limit <= 0 {
		return 0
	}

	if len(data) <= limit {
		return len(data)
	}

	prefix := 0
	for prefix < len(data) && prefix < limit {
		_, size := utf8.DecodeRune(data[prefix:])
		if prefix+size > limit {
			break
		}

		prefix += size
	}

	if prefix == 0 {
		return 1
	}

	return prefix
}

func TruncateSuffix(text string, limit int, marker string) (string, bool) {
	if limit <= 0 {
		return marker, text != ""
	}

	if len(text) <= limit {
		return text, false
	}

	return safePrefix(text, limit) + marker, true
}

func TruncateMiddle(text string, limit int, marker string) (string, bool) {
	if limit <= 0 {
		return "", text != ""
	}

	if len(text) <= limit {
		return text, false
	}

	if limit <= len(marker) {
		return marker[:limit], true
	}

	headLimit := (limit - len(marker)) / 2
	tailLimit := limit - len(marker) - headLimit
	return safePrefix(text, headLimit) + marker + safeSuffix(text, tailLimit), true
}

func safePrefix(text string, limit int) string {
	if limit <= 0 || text == "" {
		return ""
	}

	if len(text) <= limit {
		return text
	}

	prefix := limit
	for prefix > 0 && prefix < len(text) && !utf8.RuneStart(text[prefix]) {
		prefix--
	}

	return text[:prefix]
}

func safeSuffix(text string, limit int) string {
	if limit <= 0 || text == "" {
		return ""
	}

	if len(text) <= limit {
		return text
	}

	start := len(text) - limit
	for start < len(text) && !utf8.RuneStart(text[start]) {
		start++
	}

	if start >= len(text) {
		return ""
	}

	return text[start:]
}

func appendHexEscape(builder *strings.Builder, value byte) {
	_, _ = fmt.Fprintf(builder, "\\x%02X", value)
}
