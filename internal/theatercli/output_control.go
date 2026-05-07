package theatercli

import (
	"io"
	"strings"
)

const (
	ansiBold         = "\x1b[1m"
	ansiEscapePrefix = "\x1b["
	ansiGreen        = "\x1b[32m"
	ansiRed          = "\x1b[31m"
	ansiYellow       = "\x1b[33m"
	ansiReset        = "\x1b[0m"
	envCLIColor      = "CLICOLOR"
	envCLIColorForce = "CLICOLOR_FORCE"
	envNoColor       = "NO_COLOR"
	envTheaterColor  = "THEATER_COLOR"
	statusCanceled   = "canceled"
	statusFailed     = "failed"
	statusPassed     = "passed"
	statusSkipped    = "skipped"
	terminalEnvName  = "TERM"
	terminalTypeDumb = "dumb"
)

type outputColorMode string

const (
	outputColorModeAlways outputColorMode = "always"
	outputColorModeAuto   outputColorMode = "auto"
	outputColorModeNever  outputColorMode = "never"
)

type envLookup func(string) (string, bool)

type outputControl struct {
	colorMode outputColorMode
	term      string
}

type cliTextStyler struct {
	enabled bool
}

func resolveOutputControl(lookup envLookup) outputControl {
	return outputControl{
		colorMode: resolveOutputColorMode(lookup),
		term:      resolveTerminalType(lookup),
	}
}

func resolveOutputColorMode(lookup envLookup) outputColorMode {
	if value, ok := lookup(envTheaterColor); ok {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "", string(outputColorModeAuto):
			return outputColorModeAuto
		case string(outputColorModeAlways):
			return outputColorModeAlways
		case string(outputColorModeNever):
			return outputColorModeNever
		}
	}

	if _, ok := lookup(envNoColor); ok {
		return outputColorModeNever
	}
	if value, ok := lookup(envCLIColorForce); ok && strings.TrimSpace(value) != "" && strings.TrimSpace(value) != "0" {
		return outputColorModeAlways
	}
	if value, ok := lookup(envCLIColor); ok && strings.TrimSpace(value) == "0" {
		return outputColorModeNever
	}

	return outputColorModeAuto
}

func resolveTerminalType(lookup envLookup) string {
	value, _ := lookup(terminalEnvName)
	return strings.ToLower(strings.TrimSpace(value))
}

func (c outputControl) stylingEnabled(writer io.Writer, isTerminal func(io.Writer) bool) bool {
	switch c.colorMode {
	case outputColorModeAlways:
		return true
	case outputColorModeNever:
		return false
	default:
		return c.terminalPresentationEnabled(writer, isTerminal)
	}
}

func (c outputControl) terminalPresentationEnabled(writer io.Writer, isTerminal func(io.Writer) bool) bool {
	return isTerminal(writer) && c.term != terminalTypeDumb
}

func (c outputControl) styler(writer io.Writer, isTerminal func(io.Writer) bool) cliTextStyler {
	return cliTextStyler{enabled: c.stylingEnabled(writer, isTerminal)}
}

func (s cliTextStyler) Heading(text string) string {
	return s.wrap(text, ansiBold)
}

func (s cliTextStyler) Status(text string) string {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case strings.ToLower(checkStatusOK), doctorReadyStatus, statusPassed, "valid":
		return s.wrap(text, ansiGreen)
	case strings.ToLower(checkStatusFail), doctorNotReadyStatus, statusFailed, statusCanceled:
		return s.wrap(text, ansiRed)
	case strings.ToLower(checkStatusWarn):
		return s.wrap(text, ansiYellow)
	default:
		return text
	}
}

func (s cliTextStyler) wrap(text, code string) string {
	if !s.enabled || text == "" {
		return text
	}
	return code + text + ansiReset
}
