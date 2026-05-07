package theatercli

import (
	"regexp"
	"strings"
	"testing"

	"github.com/alex-poliushkin/theater/internal/runobserve"
	"github.com/alex-poliushkin/theater/observe"
)

func TestLiveRunStateRenderFrameShowsScenarioTree(t *testing.T) {
	t.Parallel()

	state := liveRunState{
		totalScenarios: 2,
		scenarios:      make(map[string]*liveScenarioNode),
	}

	state.apply(observe.Envelope{
		Kind:       observe.KindTransition,
		Seq:        1,
		Node:       observe.NodeRef{Kind: liveNodeKindStage, Path: "stage.main"},
		Transition: &observe.Transition{EventKind: "stage.running", Status: "running"},
	})
	state.apply(observe.Envelope{
		Kind:       observe.KindTransition,
		Seq:        2,
		Node:       observe.NodeRef{Kind: liveNodeKindScenario, Path: "stage.main/call.notify-user", ScenarioCallID: "notify-user", ScenarioID: "notifications"},
		Transition: &observe.Transition{EventKind: "scenario.running", Status: "running"},
	})
	state.apply(observe.Envelope{
		Kind:       observe.KindTransition,
		Seq:        3,
		Node:       observe.NodeRef{Kind: liveNodeKindAct, Path: "stage.main/call.notify-user/act.fetch", ScenarioCallID: "notify-user", ScenarioID: "notifications"},
		Transition: &observe.Transition{EventKind: "act.running", Status: "running"},
	})
	state.apply(observe.Envelope{
		Kind:       observe.KindTransition,
		Seq:        4,
		Node:       observe.NodeRef{Kind: liveNodeKindAction, Path: "stage.main/call.notify-user/act.fetch/action", ScenarioCallID: "notify-user", ScenarioID: "notifications"},
		Transition: &observe.Transition{EventKind: "action.running", Status: "running"},
	})

	if got, want := state.renderFrame(), strings.Join([]string{
		"live | stage=running | scenarios 0/2 | failed=0 canceled=0 skipped=0",
		"- scenario notify-user (notifications) [running]",
		"  - act fetch [running]",
		"    - action [running]",
	}, "\n"); got != want {
		t.Fatalf("frame mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestLiveRunStateRenderFrameOrdersConcurrentScenariosByFirstObservation(t *testing.T) {
	t.Parallel()

	state := liveRunState{
		totalScenarios: 2,
		scenarios:      make(map[string]*liveScenarioNode),
	}

	state.apply(observe.Envelope{
		Kind:       observe.KindTransition,
		Seq:        10,
		Node:       observe.NodeRef{Kind: liveNodeKindAction, Path: "stage.main/call.one/act.run/action"},
		Transition: &observe.Transition{EventKind: "action.running", Status: "running"},
	})
	state.apply(observe.Envelope{
		Kind:       observe.KindTransition,
		Seq:        11,
		Node:       observe.NodeRef{Kind: liveNodeKindAction, Path: "stage.main/call.two/act.run/action"},
		Transition: &observe.Transition{EventKind: "action.running", Status: "running"},
	})

	frame := state.renderFrame()
	first := strings.Index(frame, "- scenario one [running]")
	second := strings.Index(frame, "- scenario two [running]")
	if first < 0 || second < 0 {
		t.Fatalf("scenario rows missing from frame: %q", frame)
	}
	if first >= second {
		t.Fatalf("scenario order mismatch: %q", frame)
	}
}

func TestLiveRunStateKeepsFinishedActionVisibleUntilActCompletes(t *testing.T) {
	t.Parallel()

	state := liveRunState{
		totalScenarios: 1,
		scenarios:      make(map[string]*liveScenarioNode),
	}

	state.apply(observe.Envelope{
		Kind:       observe.KindTransition,
		Seq:        1,
		Node:       observe.NodeRef{Kind: liveNodeKindAction, Path: "stage.main/call.notify/act.fetch/action", ScenarioCallID: "notify"},
		Transition: &observe.Transition{EventKind: "action.running", Status: "running"},
	})
	state.apply(observe.Envelope{
		Kind:       observe.KindTransition,
		Seq:        2,
		Node:       observe.NodeRef{Kind: liveNodeKindAction, Path: "stage.main/call.notify/act.fetch/action", ScenarioCallID: "notify"},
		Transition: &observe.Transition{EventKind: "action.finished", Status: "passed"},
	})

	frame := state.renderFrame()
	if !strings.Contains(frame, "  - act fetch [running]") {
		t.Fatalf("running act row missing: %q", frame)
	}
	if !strings.Contains(frame, "    - action [passed]") {
		t.Fatalf("finished action row must remain visible: %q", frame)
	}
}

func TestLiveRunStateShowsRetryAttemptForEventuallyAct(t *testing.T) {
	t.Parallel()

	state := liveRunState{
		totalScenarios: 1,
		scenarios:      make(map[string]*liveScenarioNode),
	}

	state.apply(observe.Envelope{
		Kind:       observe.KindTransition,
		Seq:        1,
		Node:       observe.NodeRef{Kind: liveNodeKindAct, Path: "stage.main/call.notify/act.wait-phone-notification", ScenarioCallID: "notify", Attempt: 1},
		Transition: &observe.Transition{EventKind: "act.running", Status: "running"},
	})
	state.apply(observe.Envelope{
		Kind:       observe.KindTransition,
		Seq:        2,
		Node:       observe.NodeRef{Kind: liveNodeKindAction, Path: "stage.main/call.notify/act.wait-phone-notification/action", ScenarioCallID: "notify", Attempt: 1},
		Transition: &observe.Transition{EventKind: "action.finished", Status: "passed"},
	})
	state.apply(observe.Envelope{
		Kind:       observe.KindTransition,
		Seq:        3,
		Node:       observe.NodeRef{Kind: liveNodeKindAct, Path: "stage.main/call.notify/act.wait-phone-notification", ScenarioCallID: "notify", Attempt: 2},
		Transition: &observe.Transition{EventKind: "act.running", Status: "running"},
	})
	state.apply(observe.Envelope{
		Kind:       observe.KindTransition,
		Seq:        4,
		Node:       observe.NodeRef{Kind: liveNodeKindAction, Path: "stage.main/call.notify/act.wait-phone-notification/action", ScenarioCallID: "notify", Attempt: 2},
		Transition: &observe.Transition{EventKind: "action.running", Status: "running"},
	})

	frame := state.renderFrame()
	if !strings.Contains(frame, "  - act wait-phone-notification [running, attempt=2]") {
		t.Fatalf("act retry attempt must be visible: %q", frame)
	}
	if !strings.Contains(frame, "    - action [running, attempt=2]") {
		t.Fatalf("action retry attempt must be visible: %q", frame)
	}
}

func TestLiveRunStateDropsFinishedScenarioAndUpdatesCounters(t *testing.T) {
	t.Parallel()

	state := liveRunState{
		totalScenarios: 2,
		scenarios:      make(map[string]*liveScenarioNode),
	}

	state.apply(observe.Envelope{
		Kind:       observe.KindTransition,
		Seq:        1,
		Node:       observe.NodeRef{Kind: liveNodeKindScenario, Path: "stage.main/call.done", ScenarioCallID: "done"},
		Transition: &observe.Transition{EventKind: "scenario.running", Status: "running"},
	})
	state.apply(observe.Envelope{
		Kind:       observe.KindTransition,
		Seq:        2,
		Node:       observe.NodeRef{Kind: liveNodeKindScenario, Path: "stage.main/call.done", ScenarioCallID: "done"},
		Transition: &observe.Transition{EventKind: "scenario.finished", Status: "passed"},
	})

	frame := state.renderFrame()
	if strings.Contains(frame, "- scenario done [") {
		t.Fatalf("finished scenario must be removed from frame: %q", frame)
	}
	if !strings.Contains(frame, "live | stage=pending | scenarios 1/2 | failed=0 canceled=0 skipped=0") {
		t.Fatalf("summary counters mismatch: %q", frame)
	}
}

func TestLiveLogAssemblerTruncatesLongPartialLine(t *testing.T) {
	t.Parallel()

	assembler := newLiveLogAssembler(4)
	lines := assembler.consume(observe.Envelope{
		Kind: observe.KindLogChunk,
		Node: observe.NodeRef{Kind: liveNodeKindAction, Path: "stage.main/call.run/act.run/action"},
		LogChunk: &observe.LogChunk{
			Stream: "stdout",
			Data:   []byte("abcdefgh"),
		},
	})

	if got, want := len(lines), 1; got != want {
		t.Fatalf("line count mismatch: got %d want %d", got, want)
	}

	if got, want := lines[0].text, "abcd [truncated]"; got != want {
		t.Fatalf("truncated line mismatch: got %q want %q", got, want)
	}

	flushed := assembler.flushAll()
	if got, want := len(flushed), 1; got != want {
		t.Fatalf("flushed line count mismatch: got %d want %d", got, want)
	}

	if got, want := flushed[0].text, "[...continued] efgh"; got != want {
		t.Fatalf("flushed remainder mismatch: got %q want %q", got, want)
	}
}

func TestLiveLogAssemblerPrefixesContinuationAfterTruncation(t *testing.T) {
	t.Parallel()

	assembler := newLiveLogAssembler(4)
	lines := assembler.consume(observe.Envelope{
		Kind: observe.KindLogChunk,
		Node: observe.NodeRef{Kind: liveNodeKindAction, Path: "stage.main/call.run/act.run/action"},
		LogChunk: &observe.LogChunk{
			Stream: "stdout",
			Data:   []byte("abcdefgh"),
		},
	})

	if got, want := len(lines), 1; got != want {
		t.Fatalf("line count mismatch: got %d want %d", got, want)
	}

	lines = assembler.consume(observe.Envelope{
		Kind: observe.KindLogChunk,
		Node: observe.NodeRef{Kind: liveNodeKindAction, Path: "stage.main/call.run/act.run/action"},
		LogChunk: &observe.LogChunk{
			Stream: "stdout",
			Data:   []byte("ij\n"),
		},
	})

	if got, want := len(lines), 1; got != want {
		t.Fatalf("continued line count mismatch: got %d want %d", got, want)
	}

	if got, want := lines[0].text, "[...continued] efghij"; got != want {
		t.Fatalf("continued line mismatch: got %q want %q", got, want)
	}
}

func TestLiveLogAssemblerPreservesSplitUTF8AcrossChunks(t *testing.T) {
	t.Parallel()

	assembler := newLiveLogAssembler(16)
	lines := assembler.consume(observe.Envelope{
		Kind: observe.KindLogChunk,
		Node: observe.NodeRef{Kind: liveNodeKindAction, Path: "stage.main/call.run/act.run/action"},
		LogChunk: &observe.LogChunk{
			Stream: "stdout",
			Data:   []byte("caf\xc3"),
		},
	})
	if len(lines) != 0 {
		t.Fatalf("unexpected early line output: %#v", lines)
	}

	lines = assembler.consume(observe.Envelope{
		Kind: observe.KindLogChunk,
		Node: observe.NodeRef{Kind: liveNodeKindAction, Path: "stage.main/call.run/act.run/action"},
		LogChunk: &observe.LogChunk{
			Stream: "stdout",
			Data:   []byte("\xa9\n"),
		},
	})

	if got, want := len(lines), 1; got != want {
		t.Fatalf("line count mismatch: got %d want %d", got, want)
	}

	if got, want := lines[0].text, "café"; got != want {
		t.Fatalf("utf8 line mismatch: got %q want %q", got, want)
	}
}

func TestLiveLogAssemblerCombinesSplitCRLFBoundary(t *testing.T) {
	t.Parallel()

	assembler := newLiveLogAssembler(16)
	lines := assembler.consume(observe.Envelope{
		Kind: observe.KindLogChunk,
		Node: observe.NodeRef{Kind: liveNodeKindAction, Path: "stage.main/call.run/act.run/action"},
		LogChunk: &observe.LogChunk{
			Stream: "stdout",
			Data:   []byte("line\r"),
		},
	})
	if len(lines) != 0 {
		t.Fatalf("unexpected early line output: %#v", lines)
	}

	lines = assembler.consume(observe.Envelope{
		Kind: observe.KindLogChunk,
		Node: observe.NodeRef{Kind: liveNodeKindAction, Path: "stage.main/call.run/act.run/action"},
		LogChunk: &observe.LogChunk{
			Stream: "stdout",
			Data:   []byte("\n"),
		},
	})

	if got, want := len(lines), 1; got != want {
		t.Fatalf("line count mismatch: got %d want %d", got, want)
	}

	if got, want := lines[0].text, "line"; got != want {
		t.Fatalf("crlf line mismatch: got %q want %q", got, want)
	}
}

func TestLiveLogAssemblerTruncatesOnUTF8SafeBoundary(t *testing.T) {
	t.Parallel()

	assembler := newLiveLogAssembler(4)
	lines := assembler.consume(observe.Envelope{
		Kind: observe.KindLogChunk,
		Node: observe.NodeRef{Kind: liveNodeKindAction, Path: "stage.main/call.run/act.run/action"},
		LogChunk: &observe.LogChunk{
			Stream: "stdout",
			Data:   []byte("ab€x"),
		},
	})

	if got, want := len(lines), 1; got != want {
		t.Fatalf("line count mismatch: got %d want %d", got, want)
	}

	if got, want := lines[0].text, "ab [truncated]"; got != want {
		t.Fatalf("truncated line mismatch: got %q want %q", got, want)
	}

	flushed := assembler.flushAll()
	if got, want := len(flushed), 1; got != want {
		t.Fatalf("flushed line count mismatch: got %d want %d", got, want)
	}

	if got, want := flushed[0].text, "[...continued] €x"; got != want {
		t.Fatalf("flushed remainder mismatch: got %q want %q", got, want)
	}
}

func TestLiveLogAssemblerEscapesBinaryBytes(t *testing.T) {
	t.Parallel()

	assembler := newLiveLogAssembler(16)
	lines := assembler.consume(observe.Envelope{
		Kind: observe.KindLogChunk,
		Node: observe.NodeRef{Kind: liveNodeKindAction, Path: "stage.main/call.run/act.run/action"},
		LogChunk: &observe.LogChunk{
			Stream: "stderr",
			Data:   []byte{'o', 0x00, 'k', 0xff, '\n'},
		},
	})

	if got, want := len(lines), 1; got != want {
		t.Fatalf("line count mismatch: got %d want %d", got, want)
	}

	if got, want := lines[0].text, "o\\x00k\\xFF"; got != want {
		t.Fatalf("escaped line mismatch: got %q want %q", got, want)
	}
}

func TestLiveTerminalFrameWriterClearsShrinkingFrames(t *testing.T) {
	t.Parallel()

	var buffer strings.Builder
	writer := newLiveTerminalFrameWriter(&buffer)

	writer.Render("one\ntwo\nthree")
	writer.Render("one")

	if got, want := buffer.String(), "one\ntwo\nthree\n\x1b[3A\r\x1b[Jone\n"; got != want {
		t.Fatalf("writer output mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestLiveTerminalFrameWriterPrintLineClearsFrameAndAllowsNextRender(t *testing.T) {
	t.Parallel()

	var buffer strings.Builder
	writer := newLiveTerminalFrameWriter(&buffer)

	writer.Render("one\ntwo")
	writer.PrintLine("[log] response")
	writer.Render("one")

	if got, want := buffer.String(), "one\ntwo\n\x1b[2A\r\x1b[J[log] response\none\n"; got != want {
		t.Fatalf("writer output mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestLiveSessionTTYPrintsLogChunksAboveFrame(t *testing.T) {
	t.Parallel()

	var stderr strings.Builder
	bus := runobserve.NewBus("run-1", 0)
	session := newLiveSessionWithTerminal(&stderr, 1, bus.Subscribe(), true)

	bus.Publish(observe.Envelope{
		Kind:       observe.KindTransition,
		Seq:        1,
		Node:       observe.NodeRef{Kind: liveNodeKindStage, Path: "stage.main"},
		Transition: &observe.Transition{EventKind: "stage.running", Status: "running"},
	})
	bus.Publish(observe.Envelope{
		Kind: observe.KindLogChunk,
		Seq:  2,
		Node: observe.NodeRef{Kind: liveNodeKindLog, Path: "stage.main/call.notify-user/act.fetch/log.response"},
		LogChunk: &observe.LogChunk{
			Stream: "log",
			Data:   []byte("log response: created user\n"),
		},
	})

	session.Stop()

	output := stripANSIEscapeCodes(stderr.String())
	if !strings.Contains(output, "[stage.main/call.notify-user/act.fetch/log.response][log] log response: created user") {
		t.Fatalf("terminal live output must include log chunk: %q", output)
	}
	if !strings.Contains(output, "live | stage=running | scenarios 0/1") {
		t.Fatalf("terminal live output must keep rendering frame: %q", output)
	}
}

func TestLiveSessionPlainPrintsScenarioLogChunks(t *testing.T) {
	t.Parallel()

	var stderr strings.Builder
	bus := runobserve.NewBus("run-1", 0)
	session := newLiveSessionWithTerminal(&stderr, 1, bus.Subscribe(), false)

	bus.Publish(observe.Envelope{
		Kind: observe.KindLogChunk,
		Node: observe.NodeRef{Kind: liveNodeKindLog, Path: "stage.main/call.run/act.submit/log.response"},
		LogChunk: &observe.LogChunk{
			Stream: "log",
			Data:   []byte("log response: created user\n"),
		},
	})

	session.Stop()

	if !strings.Contains(stderr.String(), "[stage.main/call.run/act.submit/log.response][log] log response: created user") {
		t.Fatalf("plain live output must include scenario log chunk: %q", stderr.String())
	}
}

func TestLiveSessionTTYPrintsDroppedLogNoticesAboveFrame(t *testing.T) {
	t.Parallel()

	var stderr strings.Builder
	bus := runobserve.NewBus("run-1", 0)
	session := newLiveSessionWithTerminal(&stderr, 1, bus.Subscribe(), true)

	bus.Publish(observe.Envelope{
		Kind: observe.KindDropped,
		Node: observe.NodeRef{Kind: liveNodeKindLog, Path: "stage.main/call.run/act.submit/log.response"},
		Dropped: &observe.DroppedNotice{
			Count: 3,
		},
	})

	session.Stop()

	output := stripANSIEscapeCodes(stderr.String())
	if !strings.Contains(output, "[stage.main/call.run/act.submit/log.response] <dropped 3 live events>") {
		t.Fatalf("terminal live output must include dropped notice: %q", output)
	}
	if !strings.Contains(output, "live | stage=pending | scenarios 0/1") {
		t.Fatalf("terminal live output must keep rendering frame: %q", output)
	}
}

func TestLiveSessionPlainPrintsDroppedLogNotices(t *testing.T) {
	t.Parallel()

	var stderr strings.Builder
	bus := runobserve.NewBus("run-1", 0)
	session := newLiveSessionWithTerminal(&stderr, 1, bus.Subscribe(), false)

	bus.Publish(observe.Envelope{
		Kind: observe.KindDropped,
		Node: observe.NodeRef{Kind: liveNodeKindLog, Path: "stage.main/call.run/act.submit/log.response"},
		Dropped: &observe.DroppedNotice{
			Count: 3,
		},
	})

	session.Stop()

	if !strings.Contains(stderr.String(), "[stage.main/call.run/act.submit/log.response] <dropped 3 live events>") {
		t.Fatalf("plain live output must include dropped notice: %q", stderr.String())
	}
}

func TestLiveSessionTTYRendersScenarioTree(t *testing.T) {
	t.Parallel()

	var stderr strings.Builder
	bus := runobserve.NewBus("run-1", 0)
	session := newLiveSessionWithTerminal(&stderr, 1, bus.Subscribe(), true)

	bus.Publish(observe.Envelope{
		Kind:       observe.KindTransition,
		Seq:        1,
		Node:       observe.NodeRef{Kind: liveNodeKindStage, Path: "stage.main"},
		Transition: &observe.Transition{EventKind: "stage.running", Status: "running"},
	})
	bus.Publish(observe.Envelope{
		Kind:       observe.KindTransition,
		Seq:        2,
		Node:       observe.NodeRef{Kind: liveNodeKindScenario, Path: "stage.main/call.notify-user", ScenarioCallID: "notify-user"},
		Transition: &observe.Transition{EventKind: "scenario.running", Status: "running"},
	})
	bus.Publish(observe.Envelope{
		Kind:       observe.KindTransition,
		Seq:        3,
		Node:       observe.NodeRef{Kind: liveNodeKindAct, Path: "stage.main/call.notify-user/act.fetch~1body", ScenarioCallID: "notify-user"},
		Transition: &observe.Transition{EventKind: "act.running", Status: "running"},
	})
	bus.Publish(observe.Envelope{
		Kind:       observe.KindTransition,
		Seq:        4,
		Node:       observe.NodeRef{Kind: liveNodeKindAction, Path: "stage.main/call.notify-user/act.fetch~1body/action", ScenarioCallID: "notify-user"},
		Transition: &observe.Transition{EventKind: "action.running", Status: "running"},
	})

	session.Stop()

	output := stripANSIEscapeCodes(stderr.String())
	if !strings.Contains(output, "- scenario notify-user [running]") {
		t.Fatalf("scenario row missing: %q", output)
	}
	if !strings.Contains(output, "  - act fetch/body [running]") {
		t.Fatalf("decoded act row missing: %q", output)
	}
	if !strings.Contains(output, "    - action [running]") {
		t.Fatalf("action row missing: %q", output)
	}
}

func TestLiveSessionStopFlushesPartialLines(t *testing.T) {
	t.Parallel()

	var stderr strings.Builder
	bus := runobserve.NewBus("run-1", 0)
	session := newLiveSessionWithTerminal(&stderr, 1, bus.Subscribe(), false)

	bus.Publish(observe.Envelope{
		Kind: observe.KindLogChunk,
		Node: observe.NodeRef{Kind: liveNodeKindAction, Path: "stage.main/call.run/act.run/action"},
		LogChunk: &observe.LogChunk{
			Stream: "stdout",
			Data:   []byte("partial-without-newline"),
		},
	})

	session.Stop()

	if !strings.Contains(stderr.String(), "[stage.main/call.run/act.run/action][stdout] partial-without-newline") {
		t.Fatalf("stop must flush partial lines: %q", stderr.String())
	}
}

func TestLiveSessionStopFlushesIncompleteUTF8LossAware(t *testing.T) {
	t.Parallel()

	var stderr strings.Builder
	bus := runobserve.NewBus("run-1", 0)
	session := newLiveSessionWithTerminal(&stderr, 1, bus.Subscribe(), false)

	bus.Publish(observe.Envelope{
		Kind: observe.KindLogChunk,
		Node: observe.NodeRef{Kind: liveNodeKindAction, Path: "stage.main/call.run/act.run/action"},
		LogChunk: &observe.LogChunk{
			Stream: "stdout",
			Data:   []byte("caf\xc3"),
		},
	})

	session.Stop()

	if !strings.Contains(stderr.String(), "[stage.main/call.run/act.run/action][stdout] caf\\xC3") {
		t.Fatalf("stop must flush incomplete utf8 loss-aware: %q", stderr.String())
	}
}

func stripANSIEscapeCodes(value string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	return re.ReplaceAllString(value, "")
}
