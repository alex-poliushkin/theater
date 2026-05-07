package theater

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDebugPromptSessionPauseHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	reader, _ := io.Pipe()
	session, err := newDebugPromptSession(reader, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("create debug prompt session failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() {
		_, err := session.Pause(ctx, debugPause{})
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("pause error mismatch: got %v want %v", err, context.Canceled)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("pause did not return after context cancellation")
	}
}

func TestDebugPromptSessionCloseUnblocksPromptReader(t *testing.T) {
	t.Parallel()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe failed: %v", err)
	}
	defer func() {
		_ = reader.Close()
		_ = writer.Close()
	}()

	session, err := newDebugPromptSession(reader, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("create debug prompt session failed: %v", err)
	}
	if session.inputCloser == nil {
		t.Skip("closeable prompt input is not available on this platform")
	}

	done := make(chan error, 1)
	go func() {
		_, err := session.Pause(context.Background(), debugPause{})
		done <- err
	}()

	time.Sleep(25 * time.Millisecond)
	session.Close()

	select {
	case err := <-done:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("pause error mismatch: got %v want %v", err, io.EOF)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("pause did not return after prompt session close")
	}
}

func TestDebugPromptSessionSupportsWhereInspectAndDumpCommands(t *testing.T) {
	t.Parallel()

	output := &bytes.Buffer{}
	dumpPath := filepath.Join(t.TempDir(), "pause.json")
	session, err := newDebugPromptSession(
		strings.NewReader("where\ninspect scope\ninspect inputs\ninspect output\ninspect state\ninspect recent\ninspect scheduler\ndump "+dumpPath+"\ncontinue\n"),
		output,
	)
	if err != nil {
		t.Fatalf("create debug prompt session failed: %v", err)
	}

	command, err := session.Pause(context.Background(), debugPause{
		Seq:             3,
		Reason:          debugPauseReasonBreakpoint,
		Breakpoint:      "after-token",
		DurableEventSeq: 12,
		State: debugBoundaryState{
			Ref: debugBoundaryRef{
				StageID:        "main",
				StagePath:      "stage.main",
				ScenarioID:     "login",
				ScenarioCallID: "login-user",
				ScenarioPath:   "stage.main/call.login-user",
				ActID:          "submit",
				Path:           "stage.main/call.login-user/act.submit/expectation.token",
				Kind:           debugBoundaryKindExpectation,
				Phase:          debugBoundaryPhaseAfter,
				Attempt:        2,
				SourceSpan:     &SourceRef{File: "login.thtr", Line: 7, Column: 5},
			},
			Status: StatusFailed,
			Failure: &Failure{
				Kind:    FailureKindExpectation,
				Phase:   PhaseRun,
				At:      "stage.main/call.login-user/act.submit/expectation.token",
				Summary: "expectation failed",
			},
			Scope: debugSnapshotSection{
				Fields: []debugSnapshotField{{
					Key:        "token",
					Origin:     "scope.current",
					SourceSpan: &SourceRef{File: "login.thtr", Line: 6, Column: 11},
					Value:      debugSafeValue{Kind: "string", Text: "issued-token"},
				}},
			},
			Inputs: debugSnapshotSection{
				Fields: []debugSnapshotField{{
					Key:        "actual",
					Origin:     "expectation.actual",
					SourceSpan: &SourceRef{File: "login.thtr", Line: 8, Column: 37},
					Value:      debugSafeValue{Kind: "string", Text: "issued-token"},
				}},
			},
			Output: debugSnapshotSection{
				Fields: []debugSnapshotField{{
					Key:        "status",
					Origin:     "expectation.output.status",
					SourceSpan: &SourceRef{File: "login.thtr", Line: 9, Column: 5},
					Value:      debugSafeValue{Kind: "string", Text: "failed"},
				}},
			},
			State: debugStateSnapshot{
				Accesses: []debugStateAccess{{
					Seq:   1,
					Op:    "put",
					Key:   "debug/record/session",
					Value: debugSafeValue{Kind: "string", Text: "ready"},
				}},
			},
			Recent: debugRecentSnapshot{
				Items: []debugEventSummary{{
					Seq:     7,
					Kind:    "progress",
					Path:    "stage.main/call.login-user/act.submit/action",
					Attempt: 2,
					Text:    "warming up",
				}},
			},
			Scheduler: debugSchedulerSummary{
				FocusedLane: "stage.main/call.login-user",
				Active:      1,
				Ready:       0,
				Blocked:     1,
				ReadyPaths:  []string{"stage.main/call.other"},
			},
		},
	})
	if err != nil {
		t.Fatalf("pause failed: %v", err)
	}
	if got, want := command, debugResumeContinue; got != want {
		t.Fatalf("resume command mismatch: got %q want %q", got, want)
	}

	text := output.String()
	for _, want := range []string{
		"PAUSED breakpoint",
		"where:",
		"breakpoint: after-token",
		"source: login.thtr:7:5",
		"scope:",
		"token [scope.current] source=login.thtr:6:11: issued-token",
		"inputs:",
		"actual [expectation.actual] source=login.thtr:8:37: issued-token",
		"output:",
		"status [expectation.output.status] source=login.thtr:9:5: failed",
		"state:",
		"[1] put debug/record/session => ready",
		"recent:",
		"[7] progress stage.main/call.login-user/act.submit/action attempt=2 warming up",
		"scheduler:",
		"focused_lane: stage.main/call.login-user",
		"dumped snapshot to ",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("prompt output missing %q:\n%s", want, text)
		}
	}

	data, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read dump file failed: %v", err)
	}

	var dumped map[string]any
	if err := json.Unmarshal(data, &dumped); err != nil {
		t.Fatalf("decode dump file failed: %v", err)
	}
	if got, want := dumped["reason"], "breakpoint"; got != want {
		t.Fatalf("dump reason mismatch: got %v want %v", got, want)
	}
	if got, want := dumped["breakpoint"], "after-token"; got != want {
		t.Fatalf("dump breakpoint mismatch: got %v want %v", got, want)
	}
}

func TestDebugPromptDumpUsesPrivateRewriteHandling(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "shared")
	if err := os.Mkdir(dir, 0o777); err != nil {
		t.Fatalf("create shared dir failed: %v", err)
	}
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatalf("chmod shared dir failed: %v", err)
	}

	session := &debugPromptSession{output: &bytes.Buffer{}}
	path := filepath.Join(dir, "pause.json")
	if err := session.handleDump(debugPromptCommand{args: []string{path}}, debugPause{
		Seq:    9,
		Reason: debugPauseReasonBreakpoint,
	}); err != nil {
		t.Fatalf("dump snapshot failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat dump file failed: %v", err)
	}
	if got := info.Mode().Perm(); got&0o077 != 0 {
		t.Fatalf("dump file must not be group/world readable or writable: mode %o", got)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dump dir failed: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got&0o022 != 0 {
		t.Fatalf("dump directory must not remain group/world writable: mode %o", got)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read dump file failed: %v", err)
	}
	if !bytes.Contains(data, []byte(`"seq": 9`)) {
		t.Fatalf("dump file content mismatch: %s", string(data))
	}

	target := filepath.Join(dir, "target.json")
	if err := os.WriteFile(target, []byte("target\n"), 0o600); err != nil {
		t.Fatalf("write target file failed: %v", err)
	}
	symlinkPath := filepath.Join(dir, "dump-symlink.json")
	if err := os.Symlink(target, symlinkPath); err != nil {
		t.Skipf("symlink is not available: %v", err)
	}
	if err := session.handleDump(debugPromptCommand{args: []string{symlinkPath}}, debugPause{}); err == nil {
		t.Fatal("expected symlink dump path to be rejected")
	} else if got, want := err.Error(), "must not be a symlink"; !strings.Contains(got, want) {
		t.Fatalf("symlink dump error mismatch: got %q want fragment %q", got, want)
	}

	hardlinkPath := filepath.Join(dir, "dump-hardlink.json")
	if err := os.Link(target, hardlinkPath); err != nil {
		t.Skipf("hardlink is not available: %v", err)
	}
	if err := session.handleDump(debugPromptCommand{args: []string{hardlinkPath}}, debugPause{}); err == nil {
		t.Fatal("expected hardlink dump path to be rejected")
	} else if got, want := err.Error(), "multiple hard links"; !strings.Contains(got, want) {
		t.Fatalf("hardlink dump error mismatch: got %q want fragment %q", got, want)
	}
}

func TestDebugPromptSessionQuitReturnsQuitCommand(t *testing.T) {
	t.Parallel()

	session, err := newDebugPromptSession(strings.NewReader("quit\n"), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("create debug prompt session failed: %v", err)
	}

	command, err := session.Pause(context.Background(), debugPause{})
	if err != nil {
		t.Fatalf("pause failed: %v", err)
	}
	if got, want := command, debugResumeQuit; got != want {
		t.Fatalf("resume command mismatch: got %q want %q", got, want)
	}
}
