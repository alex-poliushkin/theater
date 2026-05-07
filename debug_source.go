package theater

import (
	"fmt"
	"strconv"
)

func debugSourceRefJSONObject(source *SourceRef) map[string]any {
	return map[string]any{
		"file":   source.File,
		"line":   source.Line,
		"column": source.Column,
	}
}

func debugSourceRefText(source *SourceRef) string {
	if source == nil {
		return ""
	}
	if source.File != "" && source.Line > 0 && source.Column > 0 {
		return fmt.Sprintf("%s:%d:%d", source.File, source.Line, source.Column)
	}
	if source.File != "" && source.Line > 0 {
		return fmt.Sprintf("%s:%d", source.File, source.Line)
	}
	if source.File != "" {
		return source.File
	}
	if source.Line > 0 && source.Column > 0 {
		return fmt.Sprintf("%d:%d", source.Line, source.Column)
	}
	if source.Line > 0 {
		return strconv.Itoa(source.Line)
	}

	return ""
}
