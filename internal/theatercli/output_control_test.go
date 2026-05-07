package theatercli

import (
	"io"
	"strings"
	"testing"
)

func TestResolveOutputControlPrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  map[string]string
		want outputColorMode
	}{
		{
			name: "default auto",
			want: outputColorModeAuto,
		},
		{
			name: "theater color always overrides common env",
			env: map[string]string{
				envTheaterColor: "always",
				envNoColor:      "1",
				envCLIColor:     "0",
			},
			want: outputColorModeAlways,
		},
		{
			name: "theater color never overrides force",
			env: map[string]string{
				envTheaterColor:  "never",
				envCLIColorForce: "1",
				envCLIColor:      "1",
			},
			want: outputColorModeNever,
		},
		{
			name: "no color beats common force when theater color is unset",
			env: map[string]string{
				envNoColor:       "1",
				envCLIColorForce: "1",
			},
			want: outputColorModeNever,
		},
		{
			name: "cli color force enables styling",
			env: map[string]string{
				envCLIColorForce: "1",
			},
			want: outputColorModeAlways,
		},
		{
			name: "cli color zero disables styling",
			env: map[string]string{
				envCLIColor: "0",
			},
			want: outputColorModeNever,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			control := resolveOutputControl(mapEnvLookup(test.env))
			if got := control.colorMode; got != test.want {
				t.Fatalf("color mode mismatch: got %q want %q", got, test.want)
			}
		})
	}
}

func TestOutputControlTerminalBehavior(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                     string
		control                  outputControl
		isTerminal               bool
		wantStyling              bool
		wantTerminalPresentation bool
	}{
		{
			name: "auto styling on capable terminal",
			control: outputControl{
				colorMode: outputColorModeAuto,
				term:      "xterm-256color",
			},
			isTerminal:               true,
			wantStyling:              true,
			wantTerminalPresentation: true,
		},
		{
			name: "auto styling off on dumb terminal",
			control: outputControl{
				colorMode: outputColorModeAuto,
				term:      terminalTypeDumb,
			},
			isTerminal:               true,
			wantStyling:              false,
			wantTerminalPresentation: false,
		},
		{
			name: "forced styling does not force terminal frames",
			control: outputControl{
				colorMode: outputColorModeAlways,
				term:      terminalTypeDumb,
			},
			isTerminal:               false,
			wantStyling:              true,
			wantTerminalPresentation: false,
		},
		{
			name: "never styling still allows terminal presentation",
			control: outputControl{
				colorMode: outputColorModeNever,
				term:      "xterm-256color",
			},
			isTerminal:               true,
			wantStyling:              false,
			wantTerminalPresentation: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			isTerminal := func(io.Writer) bool { return test.isTerminal }
			if got := test.control.stylingEnabled(io.Discard, isTerminal); got != test.wantStyling {
				t.Fatalf("styling mismatch: got %t want %t", got, test.wantStyling)
			}
			if got := test.control.terminalPresentationEnabled(io.Discard, isTerminal); got != test.wantTerminalPresentation {
				t.Fatalf("terminal presentation mismatch: got %t want %t", got, test.wantTerminalPresentation)
			}
		})
	}
}

func TestHelpOutputRespectsForcedAndDisabledColorEnvironment(t *testing.T) {
	t.Run("forced color", func(t *testing.T) {
		t.Setenv(envTheaterColor, string(outputColorModeAlways))

		var stdout strings.Builder
		var stderr strings.Builder

		code := run([]string{commandHelp, commandRun}, &stdout, &stderr)
		if got, want := code, 0; got != want {
			t.Fatalf("exit code mismatch: got %d want %d stderr=%q", got, want, stderr.String())
		}
		if !strings.Contains(stdout.String(), ansiEscapePrefix) {
			t.Fatalf("forced color help must contain ANSI styling: %q", stdout.String())
		}
		if !strings.Contains(stdout.String(), ansiBold+"Usage:"+ansiReset) {
			t.Fatalf("forced color help must style section headings: %q", stdout.String())
		}
	})

	t.Run("no color beats common force", func(t *testing.T) {
		t.Setenv(envNoColor, "1")
		t.Setenv(envCLIColorForce, "1")

		var stdout strings.Builder
		var stderr strings.Builder

		code := run([]string{commandHelp, commandRun}, &stdout, &stderr)
		if got, want := code, 0; got != want {
			t.Fatalf("exit code mismatch: got %d want %d stderr=%q", got, want, stderr.String())
		}
		if strings.Contains(stdout.String(), ansiEscapePrefix) {
			t.Fatalf("NO_COLOR must disable ANSI styling when THEATER_COLOR is unset: %q", stdout.String())
		}
	})
}

func TestCLITextStylerColorsWarnStatus(t *testing.T) {
	t.Parallel()

	styler := cliTextStyler{enabled: true}
	if got, want := styler.Status(checkStatusWarn), ansiYellow+checkStatusWarn+ansiReset; got != want {
		t.Fatalf("warn status styling mismatch: got %q want %q", got, want)
	}
}

func mapEnvLookup(values map[string]string) envLookup {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}
