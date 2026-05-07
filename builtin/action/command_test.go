package action_test

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/alex-poliushkin/theater"
	builtinaction "github.com/alex-poliushkin/theater/builtin/action"
	"github.com/alex-poliushkin/theater/internal/testkit"
	"github.com/alex-poliushkin/theater/observe"
)

const (
	expectedCommandCaptureLimitBytes = 1 << 20
	expectedCommandTailLimitBytes    = 4 * 1024
)

func TestRegisterRegistersCommandAction(t *testing.T) {
	t.Parallel()

	catalog := theater.NewCatalog()
	if err := builtinaction.Register(catalog); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	if _, err := catalog.ResolveAction(builtinaction.CommandRef); err != nil {
		t.Fatalf("resolve command action failed: %v", err)
	}
}

func TestCommandActionExecutesProcessAndReturnsStreams(t *testing.T) {
	t.Parallel()

	helper := testkit.BuildCommandHelper(t)
	action := resolveCommandAction(t)

	outputs, err := action.Run(context.Background(), theater.ActionRequest{
		Args: theater.Args{
			"executable": helper,
			"args":       []any{"emit", "--stdout", "ok", "--stderr", "warn"},
		},
	})
	if err != nil {
		t.Fatalf("run command action failed: %v", err)
	}

	if got, want := outputs["exit_code"], 0; got != want {
		t.Fatalf("exit code mismatch: got %v want %v", got, want)
	}

	if got, want := outputs["stdout"], "ok"; got != want {
		t.Fatalf("stdout mismatch: got %v want %v", got, want)
	}

	if got, want := outputs["stderr"], "warn"; got != want {
		t.Fatalf("stderr mismatch: got %v want %v", got, want)
	}
}

func TestCommandActionTreatsNonZeroExitAsCompletedOutput(t *testing.T) {
	t.Parallel()

	helper := testkit.BuildCommandHelper(t)
	action := resolveCommandAction(t)

	outputs, err := action.Run(context.Background(), theater.ActionRequest{
		Args: theater.Args{
			"executable": helper,
			"args":       []any{"emit", "--stdout", "failed", "--exit-code", "7"},
		},
	})
	if err != nil {
		t.Fatalf("run command action failed: %v", err)
	}

	if got, want := outputs["exit_code"], 7; got != want {
		t.Fatalf("exit code mismatch: got %v want %v", got, want)
	}

	if got, want := outputs["stdout"], "failed"; got != want {
		t.Fatalf("stdout mismatch: got %v want %v", got, want)
	}
}

func TestCommandActionReturnsFullOutputBeyondTailRetention(t *testing.T) {
	t.Parallel()

	helper := testkit.BuildCommandHelper(t)
	action := resolveCommandAction(t)

	stdout := repeatPattern("stdout-", expectedCommandTailLimitBytes*3+17)
	stderr := repeatPattern("stderr-", expectedCommandTailLimitBytes*2+9)

	outputs, err := action.Run(context.Background(), theater.ActionRequest{
		Args: theater.Args{
			"executable": helper,
			"args": []any{
				"emit",
				"--stdout", stdout,
				"--stderr", stderr,
			},
		},
	})
	if err != nil {
		t.Fatalf("run command action failed: %v", err)
	}

	if got, want := outputs["stdout"], stdout; got != want {
		t.Fatalf("stdout mismatch: got %d bytes want %d bytes", len(got.(string)), len(want))
	}

	if got, want := outputs["stderr"], stderr; got != want {
		t.Fatalf("stderr mismatch: got %d bytes want %d bytes", len(got.(string)), len(want))
	}
}

func TestCommandActionPassesStdinEnvAndWorkingDir(t *testing.T) {
	t.Parallel()

	helper := testkit.BuildCommandHelper(t)
	action := resolveCommandAction(t)

	t.Run("stdin", func(t *testing.T) {
		t.Parallel()

		outputs, err := action.Run(context.Background(), theater.ActionRequest{
			Args: theater.Args{
				"executable": helper,
				"args":       []any{"stdin-echo"},
				"stdin":      "payload",
			},
		})
		if err != nil {
			t.Fatalf("run command action failed: %v", err)
		}

		if got, want := outputs["stdout"], "payload"; got != want {
			t.Fatalf("stdin echo mismatch: got %v want %v", got, want)
		}
	})

	t.Run("env", func(t *testing.T) {
		t.Parallel()

		outputs, err := action.Run(context.Background(), theater.ActionRequest{
			Args: theater.Args{
				"executable": helper,
				"args":       []any{"env", "COMMAND_TEST_TOKEN"},
				"env": map[string]any{
					"COMMAND_TEST_TOKEN": "secret-value",
				},
			},
		})
		if err != nil {
			t.Fatalf("run command action failed: %v", err)
		}

		if got, want := outputs["stdout"], "secret-value"; got != want {
			t.Fatalf("env output mismatch: got %v want %v", got, want)
		}
	})

	t.Run("working_dir", func(t *testing.T) {
		t.Parallel()

		workingDir := filepath.Dir(helper)
		outputs, err := action.Run(context.Background(), theater.ActionRequest{
			Args: theater.Args{
				"executable":  helper,
				"args":        []any{"cwd"},
				"working_dir": workingDir,
			},
		})
		if err != nil {
			t.Fatalf("run command action failed: %v", err)
		}

		if got, want := filepath.Clean(outputs["stdout"].(string)), filepath.Clean(workingDir); got != want {
			t.Fatalf("cwd output mismatch: got %q want %q", got, want)
		}
	})
}

func TestCommandActionFailsWhenExecutableMissing(t *testing.T) {
	t.Parallel()

	action := resolveCommandAction(t)

	if _, err := action.Run(context.Background(), theater.ActionRequest{
		Args: theater.Args{
			"executable": "command-that-does-not-exist-theater",
		},
	}); err == nil {
		t.Fatal("expected missing executable error")
	}
}

func TestCommandActionFailsWhenWorkingDirMissing(t *testing.T) {
	t.Parallel()

	helper := testkit.BuildCommandHelper(t)
	action := resolveCommandAction(t)

	if _, err := action.Run(context.Background(), theater.ActionRequest{
		Args: theater.Args{
			"executable":  helper,
			"args":        []any{"cwd"},
			"working_dir": filepath.Join(t.TempDir(), "missing"),
		},
	}); err == nil {
		t.Fatal("expected missing working_dir error")
	}
}

func TestCommandActionFailsOnTimeout(t *testing.T) {
	t.Parallel()

	helper := testkit.BuildCommandHelper(t)
	action := resolveCommandAction(t)

	_, err := action.Run(context.Background(), theater.ActionRequest{
		Args: theater.Args{
			"executable": helper,
			"args":       []any{"emit", "--sleep-after-ms", "100"},
			"timeout":    "10ms",
		},
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error mismatch: got %v", err)
	}
}

func TestCommandActionFailsOnOutputLimitAndExposesPartialOutputs(t *testing.T) {
	t.Parallel()

	helper := testkit.BuildCommandHelper(t)
	action := resolveCommandAction(t)
	pattern := "abcdefg"
	captured := repeatPattern(pattern, expectedCommandCaptureLimitBytes)
	expectedTail := captured[len(captured)-expectedCommandTailLimitBytes:]
	if got, want := captured[:expectedCommandTailLimitBytes], expectedTail; got == want {
		t.Fatal("test setup must use different prefix and tail")
	}

	testCases := []struct {
		name        string
		stream      string
		otherStream string
	}{
		{name: "stdout", stream: "stdout", otherStream: "stderr"},
		{name: "stderr", stream: "stderr", otherStream: "stdout"},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := action.Run(context.Background(), theater.ActionRequest{
				Args: theater.Args{
					"executable": helper,
					"args": []any{
						"spam",
						"--stream", testCase.stream,
						"--bytes", "2000000",
						"--pattern", pattern,
					},
				},
			})
			if err == nil {
				t.Fatal("expected output limit error")
			}

			if !strings.Contains(err.Error(), testCase.stream+" exceeded") {
				t.Fatalf("overflow error mismatch: want stream %q in %q", testCase.stream, err.Error())
			}

			details, ok := err.(theater.ActionErrorDetails)
			if !ok {
				t.Fatalf("expected action error details, got %T", err)
			}

			if got, want := details.FailureSummary(), "command output exceeded capture limit"; got != want {
				t.Fatalf("failure summary mismatch: got %q want %q", got, want)
			}

			partial := details.PartialOutputs()
			got, ok := partial[testCase.stream].(string)
			if !ok {
				t.Fatalf("partial %s must be present", testCase.stream)
			}
			if got != expectedTail {
				t.Fatalf("partial %s tail mismatch", testCase.stream)
			}
			if len(got) != expectedCommandTailLimitBytes {
				t.Fatalf("partial %s length mismatch: got %d want %d", testCase.stream, len(got), expectedCommandTailLimitBytes)
			}
			if other := partial[testCase.otherStream]; other != nil {
				t.Fatalf("partial %s must be absent, got %v", testCase.otherStream, other)
			}
		})
	}
}

func TestCommandActionPublishesLiveChunksBeforeProcessExit(t *testing.T) {
	t.Parallel()

	helper := testkit.BuildCommandHelper(t)
	action := resolveCommandAction(t)

	reporter := &scriptedReporter{
		logChunks: make(chan observe.LogChunk, 1),
	}

	done := make(chan error, 1)
	go func() {
		_, err := action.Run(context.Background(), theater.ActionRequest{
			Args: theater.Args{
				"executable": helper,
				"args":       []any{"emit", "--stdout", "live-out\n", "--sleep-after-ms", "150"},
			},
			Reporter: reporter,
		})
		done <- err
	}()

	select {
	case chunk := <-reporter.logChunks:
		if got, want := chunk.Stream, "stdout"; got != want {
			t.Fatalf("live stream mismatch: got %q want %q", got, want)
		}
		if got, want := string(chunk.Data), "live-out\n"; got != want {
			t.Fatalf("live chunk mismatch: got %q want %q", got, want)
		}
		select {
		case err := <-done:
			t.Fatalf("run returned before live chunk was observed: %v", err)
		default:
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live command chunk")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run command action failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for command completion")
	}
}

type scriptedReporter struct {
	logChunks chan observe.LogChunk
}

func (r *scriptedReporter) Progress(observe.Progress) {}

func (r *scriptedReporter) Diagnostic(observe.Diagnostic) {}

func (r *scriptedReporter) LogChunk(chunk observe.LogChunk) {
	if r == nil || r.logChunks == nil {
		return
	}

	select {
	case r.logChunks <- observe.LogChunk{
		Stream: chunk.Stream,
		Data:   append([]byte(nil), chunk.Data...),
	}:
	default:
	}
}

func TestCommandActionHelperBinaryBuildsOnCurrentPlatform(t *testing.T) {
	t.Parallel()

	path := testkit.BuildCommandHelper(t)
	if runtime.GOOS == "windows" && filepath.Ext(path) != ".exe" {
		t.Fatalf("helper extension mismatch: got %q", path)
	}
}

func resolveCommandAction(t *testing.T) theater.Action {
	t.Helper()

	catalog := theater.NewCatalog()
	if err := builtinaction.Register(catalog); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	action, err := catalog.ResolveAction(builtinaction.CommandRef)
	if err != nil {
		t.Fatalf("resolve action failed: %v", err)
	}

	return action
}

func repeatPattern(pattern string, size int) string {
	if size <= 0 || pattern == "" {
		return ""
	}

	var builder strings.Builder
	builder.Grow(size)
	for builder.Len() < size {
		remaining := size - builder.Len()
		if remaining >= len(pattern) {
			builder.WriteString(pattern)
			continue
		}

		builder.WriteString(pattern[:remaining])
	}

	return builder.String()
}
