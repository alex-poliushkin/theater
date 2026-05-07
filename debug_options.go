package theater

import (
	"errors"
	"fmt"
	"io"
)

const (
	DebugModeOff         DebugMode = "off"
	DebugModeDump        DebugMode = "dump"
	DebugModeInteractive DebugMode = "interactive"
)

// DebugMode selects how a run should surface debug boundaries.
type DebugMode string

// DebugOptions enables boundary-gated debugging on the standard run path.
type DebugOptions struct {
	Mode        DebugMode
	StartPaused bool
	Breakpoints []string
	DumpPath    string
	Input       io.Reader
	Output      io.Writer
}

func buildDebugRuntime(options *DebugOptions) (*debugRuntime, bool, error) {
	mode, err := normalizeDebugMode(options.Mode)
	if err != nil {
		return nil, false, err
	}
	if mode == debugModeOff {
		if options.StartPaused || len(options.Breakpoints) != 0 || options.DumpPath != "" {
			return nil, false, errors.New("debug mode off does not accept breakpoints, step, or dump path")
		}

		return nil, false, nil
	}
	if options.StartPaused && mode != debugModeInteractive {
		return nil, false, errors.New("debug step mode requires interactive debug")
	}
	if mode == debugModeDump && options.DumpPath == "" {
		return nil, false, errors.New("debug dump mode requires a dump path")
	}

	controller := &debugController{
		mode:        mode,
		startPaused: options.StartPaused,
	}
	if mode == debugModeInteractive {
		session, err := newDebugPromptSession(options.Input, options.Output)
		if err != nil {
			return nil, false, err
		}
		controller.pause = session.Pause
		return &debugRuntime{
			controller:      controller,
			promptSession:   session,
			breakpointSpecs: cloneDebugSpecs(options.Breakpoints),
			artifactPath:    options.DumpPath,
		}, true, nil
	}

	return &debugRuntime{
		controller:      controller,
		breakpointSpecs: cloneDebugSpecs(options.Breakpoints),
		artifactPath:    options.DumpPath,
	}, true, nil
}

func normalizeDebugMode(mode DebugMode) (debugMode, error) {
	switch mode {
	case DebugModeOff:
		return debugModeOff, nil
	case DebugModeDump:
		return debugModeDump, nil
	case DebugModeInteractive:
		return debugModeInteractive, nil
	case "":
		return "", errors.New("debug mode is required")
	default:
		return "", fmt.Errorf("debug mode %q is invalid", mode)
	}
}

func cloneDebugSpecs(specs []string) []string {
	if len(specs) == 0 {
		return nil
	}

	cloned := make([]string, len(specs))
	copy(cloned, specs)
	return cloned
}
