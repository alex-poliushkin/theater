package theater

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

const (
	debugPromptCommandWhere    = "where"
	debugPromptCommandInspect  = "inspect"
	debugPromptCommandDump     = "dump"
	debugPromptCommandContinue = "continue"
	debugPromptCommandStep     = "step"
	debugPromptCommandQuit     = "quit"
	debugPromptCommandHelp     = "help"
)

type debugPromptSession struct {
	input       *bufio.Reader
	inputCloser io.Closer
	output      io.Writer
	lines       chan debugPromptLine
	stop        chan struct{}
	once        sync.Once
	closed      sync.Once
}

type debugPromptLine struct {
	text string
	err  error
}

func newDebugPromptSession(input io.Reader, output io.Writer) (*debugPromptSession, error) {
	if input == nil {
		return nil, errors.New("interactive debug requires an input reader")
	}
	if output == nil {
		return nil, errors.New("interactive debug requires an output writer")
	}

	reader, closer, err := prepareDebugPromptInput(input)
	if err != nil {
		return nil, err
	}

	return &debugPromptSession{
		input:       bufio.NewReader(reader),
		inputCloser: closer,
		output:      output,
		lines:       make(chan debugPromptLine, 1),
		stop:        make(chan struct{}),
	}, nil
}

func (s *debugPromptSession) Pause(ctx context.Context, pause debugPause) (debugResumeCommand, error) {
	s.startReader()

	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := s.writePauseBanner(pause); err != nil {
		return "", err
	}

	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if _, err := fmt.Fprint(s.output, "(debug) "); err != nil {
			return "", err
		}

		line, err := s.readLine(ctx)
		command := parseDebugPromptCommand(strings.TrimSpace(line))
		if err != nil && command.name == "" {
			return "", err
		}

		resume, done, err := s.handlePauseCommand(command, pause)
		if err != nil {
			return "", err
		}
		if done {
			return resume, nil
		}
	}
}

func (s *debugPromptSession) Close() {
	if s == nil {
		return
	}

	s.closed.Do(func() {
		close(s.stop)
		if s.inputCloser != nil {
			_ = s.inputCloser.Close()
		}
	})
}

func (s *debugPromptSession) writePauseBanner(pause debugPause) error {
	_, err := fmt.Fprintf(
		s.output,
		"PAUSED %s\n  path: %s\n  kind: %s\n  phase: %s\n  attempt: %d\n  lane: %s\n",
		pause.Reason,
		pause.State.Ref.Path,
		pause.State.Ref.Kind,
		pause.State.Ref.Phase,
		pause.State.Ref.Attempt,
		pause.State.Ref.ScenarioCallID,
	)
	return err
}

func (s *debugPromptSession) handlePauseCommand(
	command debugPromptCommand,
	pause debugPause,
) (debugResumeCommand, bool, error) {
	switch command.name {
	case debugPromptCommandWhere, "w":
		return "", false, s.writeWhere(pause)
	case debugPromptCommandInspect, "i":
		return "", false, s.handleInspect(command, pause.State)
	case debugPromptCommandDump:
		return "", false, s.handleDump(command, pause)
	case "", debugPromptCommandContinue, "c":
		return debugResumeContinue, true, nil
	case debugPromptCommandStep, "s":
		return debugResumeStep, true, nil
	case debugPromptCommandQuit, "q":
		return debugResumeQuit, true, nil
	case debugPromptCommandHelp, "h":
		return "", false, s.writeHelp()
	default:
		if _, err := fmt.Fprintf(s.output, "unknown debug command %q\n", command.raw); err != nil {
			return "", false, err
		}

		return "", false, s.writeHelp()
	}
}

func (s *debugPromptSession) writeHelp() error {
	return s.writeLines([]string{
		"commands: where | inspect <section> | dump [path] | continue | step | quit | help",
	})
}

func (s *debugPromptSession) writeWhere(pause debugPause) error {
	lines := []string{
		"where:",
		"  reason: " + string(pause.Reason),
		"  stage: " + pause.State.Ref.StageID,
		"  scenario: " + pause.State.Ref.ScenarioID,
		"  call: " + pause.State.Ref.ScenarioCallID,
		"  act: " + pause.State.Ref.ActID,
		"  path: " + pause.State.Ref.Path,
		"  kind: " + string(pause.State.Ref.Kind),
		"  phase: " + string(pause.State.Ref.Phase),
		fmt.Sprintf("  attempt: %d", pause.State.Ref.Attempt),
		"  lane: " + pause.State.Ref.ScenarioPath,
		"  status: " + string(pause.State.Status),
		fmt.Sprintf("  durable_event_seq: %d", pause.DurableEventSeq),
	}
	if pause.Breakpoint != "" {
		lines = append(lines[:2], append([]string{"  breakpoint: " + pause.Breakpoint}, lines[2:]...)...)
	}
	if source := debugSourceRefText(pause.State.Ref.SourceSpan); source != "" {
		lines = append(lines[:8], append([]string{"  source: " + source}, lines[8:]...)...)
	}

	return s.writeLines(lines)
}

func (s *debugPromptSession) handleInspect(command debugPromptCommand, state debugBoundaryState) error {
	if len(command.args) != 1 {
		return s.writeLines([]string{
			"usage: inspect scope|inputs|output|state|recent|scheduler",
		})
	}

	switch strings.ToLower(command.args[0]) {
	case "scope":
		return s.writeSection("scope", debugPromptSectionLines(state.Scope))
	case "inputs":
		return s.writeSection("inputs", debugPromptSectionLines(state.Inputs))
	case "output":
		return s.writeSection("output", debugPromptSectionLines(state.Output))
	case "state":
		return s.writeSection("state", debugPromptStateLines(state.State))
	case "recent":
		return s.writeSection("recent", debugPromptRecentLines(state.Recent))
	case "scheduler":
		return s.writeSection("scheduler", debugPromptSchedulerLines(state.Scheduler))
	default:
		return s.writeLines([]string{
			fmt.Sprintf("unknown inspect section %q", command.args[0]),
			"usage: inspect scope|inputs|output|state|recent|scheduler",
		})
	}
}

func (s *debugPromptSession) handleDump(command debugPromptCommand, pause debugPause) error {
	data, err := json.MarshalIndent(debugPauseDumpObject(pause), "", "  ")
	if err != nil {
		return err
	}

	if len(command.args) == 0 {
		if _, err := fmt.Fprintln(s.output, string(data)); err != nil {
			return err
		}
		return nil
	}
	if len(command.args) != 1 {
		return s.writeLines([]string{"usage: dump [path]"})
	}

	if err := writePrivateDebugFile(command.args[0], append(data, '\n')); err != nil {
		return err
	}

	return s.writeLines([]string{"dumped snapshot to " + command.args[0]})
}

func (s *debugPromptSession) readLine(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-s.stop:
		return "", io.EOF
	case line, ok := <-s.lines:
		if !ok {
			return "", io.EOF
		}

		return line.text, line.err
	}
}

func (s *debugPromptSession) startReader() {
	if s == nil {
		return
	}

	s.once.Do(func() {
		go func() {
			defer close(s.lines)

			for {
				line, err := s.input.ReadString('\n')

				select {
				case s.lines <- debugPromptLine{text: line, err: err}:
				case <-s.stop:
					return
				}

				if err != nil {
					return
				}
			}
		}()
	})
}

func (s *debugPromptSession) writeSection(name string, lines []string) error {
	if len(lines) == 0 {
		lines = []string{"  <empty>"}
	}

	return s.writeLines(append([]string{name + ":"}, lines...))
}

func (s *debugPromptSession) writeLines(lines []string) error {
	for i := range lines {
		if _, err := fmt.Fprintln(s.output, lines[i]); err != nil {
			return err
		}
	}

	return nil
}

func debugPromptSectionLines(section debugSnapshotSection) []string {
	lines := make([]string, 0, len(section.Fields)+1)
	for i := range section.Fields {
		lines = append(lines, debugPromptSnapshotFieldLine("  ", section.Fields[i]))
		lines = append(lines, debugPromptValueChildrenLines("    ", section.Fields[i].Value.Children)...)
	}
	if section.Omitted > 0 {
		lines = append(lines, fmt.Sprintf("  omitted: %d", section.Omitted))
	}

	return lines
}

func debugPromptStateLines(snapshot debugStateSnapshot) []string {
	lines := make([]string, 0, len(snapshot.Accesses)+len(snapshot.Enrichments)+1)
	for i := range snapshot.Accesses {
		access := snapshot.Accesses[i]
		line := fmt.Sprintf("  [%d] %s %s", access.Seq, access.Op, access.Key)
		if access.Value.Kind != "" || access.Value.Text != "" {
			line += " => " + debugPromptValueText(access.Value)
		}
		if access.Err != "" {
			line += " error=" + access.Err
		}
		lines = append(lines, line)
		lines = append(lines, debugPromptValueChildrenLines("    ", access.Value.Children)...)
	}
	for i := range snapshot.Enrichments {
		enrichment := snapshot.Enrichments[i]
		line := "  enrichment " + enrichment.Backend
		if enrichment.Err != "" {
			line += " error=" + enrichment.Err
		}
		lines = append(lines, line)
		lines = append(lines, debugPromptSectionLines(enrichment.Fields)...)
	}
	if snapshot.Omitted > 0 {
		lines = append(lines, fmt.Sprintf("  omitted: %d", snapshot.Omitted))
	}

	return lines
}

func debugPromptRecentLines(snapshot debugRecentSnapshot) []string {
	lines := make([]string, 0, len(snapshot.Items)+1)
	for i := range snapshot.Items {
		item := snapshot.Items[i]
		lines = append(lines, fmt.Sprintf("  [%d] %s %s attempt=%d %s", item.Seq, item.Kind, item.Path, item.Attempt, item.Text))
	}
	if snapshot.Omitted > 0 {
		lines = append(lines, fmt.Sprintf("  omitted: %d", snapshot.Omitted))
	}

	return lines
}

func debugPromptSchedulerLines(summary debugSchedulerSummary) []string {
	lines := []string{
		"  focused_lane: " + summary.FocusedLane,
		fmt.Sprintf("  active: %d", summary.Active),
		fmt.Sprintf("  ready: %d", summary.Ready),
		fmt.Sprintf("  blocked: %d", summary.Blocked),
	}
	if len(summary.ReadyPaths) != 0 {
		lines = append(lines, "  ready_paths: "+strings.Join(summary.ReadyPaths, ", "))
	}

	return lines
}

func debugPromptSnapshotFieldLine(indent string, field debugSnapshotField) string {
	source := ""
	if text := debugSourceRefText(field.SourceSpan); text != "" {
		source = " source=" + text
	}

	return fmt.Sprintf("%s%s [%s]%s: %s", indent, field.Key, field.Origin, source, debugPromptValueText(field.Value))
}

func debugPromptValueChildrenLines(indent string, children []debugSnapshotField) []string {
	lines := make([]string, 0, len(children))
	for i := range children {
		lines = append(lines, fmt.Sprintf("%s%s: %s", indent, children[i].Key, debugPromptValueText(children[i].Value)))
		lines = append(lines, debugPromptValueChildrenLines(indent+"  ", children[i].Value.Children)...)
	}

	return lines
}

func debugPromptValueText(value debugSafeValue) string {
	text := value.Text
	if text == "" {
		text = "<" + value.Kind + ">"
	}
	if value.Redacted {
		text += " [redacted]"
	}
	if value.Truncated {
		text += " [truncated]"
	}
	if value.OmittedReason != "" {
		text += " [omitted: " + value.OmittedReason + "]"
	}
	if value.Omitted > 0 {
		text += fmt.Sprintf(" [children omitted: %d]", value.Omitted)
	}

	return text
}

func debugPauseDumpObject(pause debugPause) map[string]any {
	object := map[string]any{
		"seq":               pause.Seq,
		"reason":            pause.Reason,
		"durable_event_seq": pause.DurableEventSeq,
		"snapshot":          debugArtifactSnapshotJSONObject(debugArtifactSnapshotFromBoundaryState(pause.State)),
	}
	if pause.Breakpoint != "" {
		object["breakpoint"] = pause.Breakpoint
	}

	return object
}

type debugPromptCommand struct {
	name string
	args []string
	raw  string
}

func parseDebugPromptCommand(line string) debugPromptCommand {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return debugPromptCommand{}
	}

	return debugPromptCommand{
		name: strings.ToLower(parts[0]),
		args: parts[1:],
		raw:  line,
	}
}
