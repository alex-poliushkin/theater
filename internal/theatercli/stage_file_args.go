package theatercli

import (
	"fmt"
	"strings"
)

const (
	doubleDashToken = "--"
	stageFileFlag   = "file"
)

func normalizeStageFileArgs(command string, args []string) (normalized []string, usedPositional bool, err error) {
	if !commandAcceptsPositionalStagePath(command) {
		return append([]string(nil), args...), false, nil
	}

	normalized = make([]string, 0, len(args)+2)
	fileSource := ""
	for i := 0; i < len(args); i++ {
		raw := args[i]
		if raw == doubleDashToken {
			rest := args[i+1:]
			if len(rest) == 0 {
				return normalized, usedPositional, nil
			}
			if fileSource != "" || len(rest) > 1 {
				return nil, usedPositional, fmt.Errorf("%s accepts exactly one stage file path", command)
			}
			normalized = append(normalized, "-file", rest[0])
			usedPositional = true
			return normalized, usedPositional, nil
		}

		name, hasInlineValue, isFlag := parseCLIFlagToken(raw)
		if !isFlag {
			if fileSource == "" {
				normalized = append(normalized, "-file", raw)
				fileSource = "positional"
				usedPositional = true
				continue
			}
			normalized = append(normalized, raw)
			continue
		}

		if name == stageFileFlag {
			if fileSource != "" {
				return nil, usedPositional, fmt.Errorf("%s accepts exactly one stage file path; choose a positional path or --file", command)
			}
			fileSource = "explicit"
		}

		normalized = append(normalized, raw)
		if hasInlineValue || isStageCommandBoolFlag(command, name) {
			continue
		}
		if isStageCommandValueFlag(command, name) && i+1 < len(args) {
			i++
			normalized = append(normalized, args[i])
		}
	}

	return normalized, usedPositional, nil
}

func commandAcceptsPositionalStagePath(command string) bool {
	switch command {
	case commandRun, commandValidate, commandFmt, commandLower, commandLibrariesInspect:
		return true
	default:
		return false
	}
}

func parseCLIFlagToken(raw string) (name string, hasInlineValue, isFlag bool) {
	if raw == "" || raw == "-" || raw == doubleDashToken || raw[0] != '-' {
		return "", false, false
	}

	trimmed := strings.TrimLeft(raw, "-")
	if trimmed == "" {
		return "", false, false
	}

	name, _, hasInlineValue = strings.Cut(trimmed, "=")
	return name, hasInlineValue, true
}

func isStageCommandBoolFlag(command, name string) bool {
	switch name {
	case "check", "diff", "write":
		return command == commandFmt
	case "step", "stop-on-failure":
		return command == commandRun
	case "debug-paths":
		return command == commandValidate
	default:
		return false
	}
}

func isStageCommandValueFlag(command, name string) bool {
	switch name {
	case stageFileFlag:
		return true
	case "plugins-config", "plugins-lock":
		return command == commandRun || command == commandValidate
	case "format":
		return command == commandRun || command == commandValidate || command == commandLibrariesInspect
	case "map":
		return command == commandLower
	case "live", "debug", "break", "break-file", "debug-dump", "plugin-exporter":
		return command == commandRun
	default:
		return false
	}
}
