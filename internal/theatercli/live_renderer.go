package theatercli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/alex-poliushkin/theater/internal/streamtext"
	"github.com/alex-poliushkin/theater/observe"
)

const (
	livePartialLineLimit = 4 * 1024
	liveStatusRunning    = "running"
)

type liveMode string

const (
	liveModeAuto liveMode = "auto"
	liveModeOff  liveMode = "off"

	liveNodeKindStage    = observe.NodeKindStage
	liveNodeKindScenario = observe.NodeKindScenario
	liveNodeKindAct      = observe.NodeKindAct
	liveNodeKindAction   = observe.NodeKindAction
	liveNodeKindLog      = observe.NodeKindLog
)

type liveSession struct {
	stderr         io.Writer
	subscription   observe.Subscription
	totalScenarios int
	isTerminal     bool
	done           chan struct{}
	stopOnce       sync.Once
}

type liveRunState struct {
	stagePath      string
	stageStatus    string
	totalScenarios int
	passed         int
	failed         int
	canceled       int
	skipped        int
	scenarios      map[string]*liveScenarioNode
}

type liveScenarioNode struct {
	path     string
	label    string
	status   string
	firstSeq uint64
	acts     map[string]*liveActNode
}

type liveActNode struct {
	path     string
	label    string
	status   string
	attempt  int
	firstSeq uint64
	action   *liveActionNode
}

type liveActionNode struct {
	path     string
	status   string
	attempt  int
	firstSeq uint64
}

type liveTerminalFrameWriter struct {
	writer        io.Writer
	renderedLines int
	lastFrame     string
}

type liveLogAssembler struct {
	limit             int
	partial           map[liveLogKey][]byte
	needsContinuation map[liveLogKey]bool
}

type liveLogKey struct {
	path   string
	stream string
}

type liveLogLine struct {
	path   string
	stream string
	text   string
}

func parseLiveMode(raw string) (liveMode, error) {
	switch liveMode(raw) {
	case liveModeAuto, liveModeOff:
		return liveMode(raw), nil
	default:
		return "", fmt.Errorf("unsupported live mode %q (supported: auto, off)", raw)
	}
}

func shouldEnableLive(format outputFormat, mode liveMode) bool {
	return format == outputFormatText && mode != liveModeOff
}

func newLiveSessionWithTerminal(stderr io.Writer, totalScenarios int, subscription observe.Subscription, isTerminal bool) *liveSession {
	if stderr == nil || subscription == nil {
		return nil
	}

	session := &liveSession{
		stderr:         stderr,
		subscription:   subscription,
		totalScenarios: totalScenarios,
		isTerminal:     isTerminal,
		done:           make(chan struct{}),
	}

	go session.run()
	return session
}

func (s *liveSession) Stop() {
	if s == nil {
		return
	}

	s.stopOnce.Do(func() {
		s.subscription.Close()
		<-s.done
	})
}

func (s *liveSession) run() {
	defer close(s.done)

	state := liveRunState{
		totalScenarios: s.totalScenarios,
		scenarios:      make(map[string]*liveScenarioNode),
	}

	if s.isTerminal {
		s.runTerminal(&state)
		return
	}

	assembler := newLiveLogAssembler(livePartialLineLimit)
	s.runPlain(&state, assembler)
}

func (s *liveSession) runPlain(state *liveRunState, assembler *liveLogAssembler) {
	for envelope := range s.subscription.Events() {
		state.apply(envelope)
		for _, line := range plainTransitionLines(envelope) {
			_, _ = fmt.Fprintln(s.stderr, line)
		}
		for _, line := range assembler.consume(envelope) {
			_, _ = fmt.Fprintln(s.stderr, formatLiveLogLine(line))
		}
	}

	for _, line := range assembler.flushAll() {
		_, _ = fmt.Fprintln(s.stderr, formatLiveLogLine(line))
	}
}

func (s *liveSession) runTerminal(state *liveRunState) {
	writer := newLiveTerminalFrameWriter(s.stderr)
	assembler := newLiveLogAssembler(livePartialLineLimit)

	for envelope := range s.subscription.Events() {
		state.apply(envelope)
		for _, line := range assembler.consume(envelope) {
			writer.PrintLine(formatLiveLogLine(line))
		}
		writer.Render(state.renderFrame())
	}

	for _, line := range assembler.flushAll() {
		writer.PrintLine(formatLiveLogLine(line))
	}
}

func (s *liveRunState) apply(envelope observe.Envelope) {
	if envelope.Kind != observe.KindTransition {
		return
	}

	s.applyTransition(envelope)
}

func (s *liveRunState) applyTransition(envelope observe.Envelope) {
	if envelope.Transition == nil {
		return
	}

	if envelope.Node.Kind == liveNodeKindStage {
		s.stagePath = envelope.Node.Path
		s.stageStatus = envelope.Transition.Status
	}

	switch envelope.Node.Kind {
	case liveNodeKindScenario:
		s.applyScenarioTransition(envelope)
	case liveNodeKindAct:
		s.applyActTransition(envelope)
	case liveNodeKindAction:
		s.applyActionTransition(envelope)
	}
}

func (s *liveRunState) applyScenarioTransition(envelope observe.Envelope) {
	scenario := s.ensureScenario(envelope.Node, envelope.Seq)
	scenario.status = envelope.Transition.Status
	if envelope.Transition.Status == liveStatusRunning {
		return
	}

	s.countScenarioStatus(envelope.Transition.Status)
	delete(s.scenarios, scenario.path)
}

func (s *liveRunState) applyActTransition(envelope observe.Envelope) {
	scenario := s.ensureScenario(envelope.Node, envelope.Seq)
	act := scenario.ensureAct(envelope.Node.Path, envelope.Seq)
	act.status = envelope.Transition.Status
	act.attempt = maxLiveAttempt(act.attempt, envelope.Node.Attempt)
}

func (s *liveRunState) applyActionTransition(envelope observe.Envelope) {
	scenario := s.ensureScenario(envelope.Node, envelope.Seq)
	actPath := liveActPath(envelope.Node)
	if actPath == "" {
		return
	}

	act := scenario.ensureAct(actPath, envelope.Seq)
	if act.status == "" {
		act.status = liveStatusRunning
	}
	act.attempt = maxLiveAttempt(act.attempt, envelope.Node.Attempt)
	if act.action == nil {
		act.action = &liveActionNode{
			path:     envelope.Node.Path,
			attempt:  envelope.Node.Attempt,
			firstSeq: envelope.Seq,
		}
	}
	act.action.status = envelope.Transition.Status
	act.action.attempt = maxLiveAttempt(act.action.attempt, envelope.Node.Attempt)
}

func (s *liveRunState) ensureScenario(node observe.NodeRef, seq uint64) *liveScenarioNode {
	path := liveScenarioPath(node)
	if path == "" {
		path = node.Path
	}

	scenario := s.scenarios[path]
	if scenario == nil {
		scenario = &liveScenarioNode{
			path:     path,
			label:    liveScenarioLabel(node, path),
			status:   liveStatusRunning,
			firstSeq: seq,
			acts:     make(map[string]*liveActNode),
		}
		s.scenarios[path] = scenario
	}
	if scenario.label == "" {
		scenario.label = liveScenarioLabel(node, path)
	}
	if scenario.status == "" {
		scenario.status = liveStatusRunning
	}

	return scenario
}

func (s *liveRunState) countScenarioStatus(status string) {
	switch status {
	case statusPassed:
		s.passed++
	case statusFailed:
		s.failed++
	case statusCanceled:
		s.canceled++
	case statusSkipped:
		s.skipped++
	}
}

func (s *liveRunState) renderFrame() string {
	lines := []string{
		fmt.Sprintf(
			"live | stage=%s | scenarios %d/%d | failed=%d canceled=%d skipped=%d",
			emptyLiveFallback(s.stageStatus, "pending"),
			s.passed+s.failed+s.canceled+s.skipped,
			s.totalScenarios,
			s.failed,
			s.canceled,
			s.skipped,
		),
	}

	for _, scenario := range s.orderedScenarios() {
		lines = append(lines, fmt.Sprintf("- scenario %s [%s]", scenario.label, emptyLiveFallback(scenario.status, liveStatusRunning)))
		for _, act := range scenario.orderedActs() {
			lines = append(lines, fmt.Sprintf("  - act %s [%s]", act.label, liveNodeStatusLabel(act.status, act.attempt)))
			if act.action != nil {
				lines = append(lines, fmt.Sprintf("    - action [%s]", liveNodeStatusLabel(act.action.status, act.action.attempt)))
			}
		}
	}

	return strings.Join(lines, "\n")
}

func (s *liveRunState) orderedScenarios() []*liveScenarioNode {
	scenarios := make([]*liveScenarioNode, 0, len(s.scenarios))
	for _, scenario := range s.scenarios {
		scenarios = append(scenarios, scenario)
	}

	sort.Slice(scenarios, func(i, j int) bool {
		if scenarios[i].firstSeq != scenarios[j].firstSeq {
			return scenarios[i].firstSeq < scenarios[j].firstSeq
		}
		return scenarios[i].path < scenarios[j].path
	})

	return scenarios
}

func (n *liveScenarioNode) ensureAct(path string, seq uint64) *liveActNode {
	act := n.acts[path]
	if act == nil {
		act = &liveActNode{
			path:     path,
			label:    liveActLabel(path),
			status:   liveStatusRunning,
			attempt:  1,
			firstSeq: seq,
		}
		n.acts[path] = act
	}
	if act.label == "" {
		act.label = liveActLabel(path)
	}

	return act
}

func (n *liveScenarioNode) orderedActs() []*liveActNode {
	acts := make([]*liveActNode, 0, len(n.acts))
	for _, act := range n.acts {
		acts = append(acts, act)
	}

	sort.Slice(acts, func(i, j int) bool {
		if acts[i].firstSeq != acts[j].firstSeq {
			return acts[i].firstSeq < acts[j].firstSeq
		}
		return acts[i].path < acts[j].path
	})

	return acts
}

func newLiveTerminalFrameWriter(writer io.Writer) *liveTerminalFrameWriter {
	return &liveTerminalFrameWriter{writer: writer}
}

func (w *liveTerminalFrameWriter) Render(frame string) {
	if w == nil || w.writer == nil || frame == "" || frame == w.lastFrame {
		return
	}

	if w.renderedLines > 0 {
		_, _ = fmt.Fprintf(w.writer, "\x1b[%dA\r\x1b[J", w.renderedLines)
	}

	_, _ = io.WriteString(w.writer, frame)
	_, _ = io.WriteString(w.writer, "\n")
	w.lastFrame = frame
	w.renderedLines = strings.Count(frame, "\n") + 1
}

func (w *liveTerminalFrameWriter) PrintLine(line string) {
	if w == nil || w.writer == nil || line == "" {
		return
	}

	if w.renderedLines > 0 {
		_, _ = fmt.Fprintf(w.writer, "\x1b[%dA\r\x1b[J", w.renderedLines)
		w.renderedLines = 0
	}

	_, _ = fmt.Fprintln(w.writer, line)
	w.lastFrame = ""
}

func newLiveLogAssembler(limit int) *liveLogAssembler {
	if limit <= 0 {
		limit = livePartialLineLimit
	}

	return &liveLogAssembler{
		limit:             limit,
		partial:           make(map[liveLogKey][]byte),
		needsContinuation: make(map[liveLogKey]bool),
	}
}

func (a *liveLogAssembler) consume(envelope observe.Envelope) []liveLogLine {
	switch envelope.Kind {
	case observe.KindLogChunk:
		if envelope.LogChunk == nil || len(envelope.LogChunk.Data) == 0 {
			return nil
		}
		return a.consumeChunk(envelope.Node.Path, envelope.LogChunk.Stream, envelope.LogChunk.Data)
	case observe.KindDropped:
		if envelope.Dropped == nil || envelope.Dropped.Count == 0 {
			return nil
		}
		return []liveLogLine{{
			path: envelope.Node.Path,
			text: fmt.Sprintf("<dropped %d live events>", envelope.Dropped.Count),
		}}
	default:
		return nil
	}
}

func (a *liveLogAssembler) flushAll() []liveLogLine {
	if len(a.partial) == 0 {
		return nil
	}

	keys := make([]liveLogKey, 0, len(a.partial))
	for key := range a.partial {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].path != keys[j].path {
			return keys[i].path < keys[j].path
		}
		return keys[i].stream < keys[j].stream
	})

	lines := make([]liveLogLine, 0, len(keys))
	for _, key := range keys {
		chunk := a.partial[key]
		if len(chunk) == 0 {
			continue
		}

		if chunk[len(chunk)-1] == '\r' {
			chunk = chunk[:len(chunk)-1]
		}
		if len(chunk) == 0 {
			delete(a.partial, key)
			continue
		}

		lines = append(lines, liveLogLine{
			path:   key.path,
			stream: key.stream,
			text:   a.decorateText(key, sanitizeLiveChunkText(chunk)),
		})
		delete(a.partial, key)
	}

	return lines
}

func (a *liveLogAssembler) consumeChunk(path, stream string, chunk []byte) []liveLogLine {
	key := liveLogKey{path: path, stream: stream}
	buffer := append(bytes.Clone(a.partial[key]), chunk...)
	lines := make([]liveLogLine, 0, 1)

	for len(buffer) > 0 {
		index, width, pending := findLineBreak(buffer)
		if pending {
			break
		}

		if index >= 0 {
			lines = append(lines, liveLogLine{
				path:   path,
				stream: stream,
				text:   a.decorateText(key, sanitizeLiveChunkText(buffer[:index])),
			})
			buffer = buffer[index+width:]
			continue
		}

		if len(buffer) <= a.limit {
			break
		}

		prefixLen := streamtext.SafePrefixLen(buffer, a.limit)
		lines = append(lines, liveLogLine{
			path:   path,
			stream: stream,
			text:   a.decorateText(key, sanitizeLiveChunkText(buffer[:prefixLen])) + " [truncated]",
		})
		a.needsContinuation[key] = true
		buffer = buffer[prefixLen:]
	}

	if len(buffer) == 0 {
		delete(a.partial, key)
		return lines
	}

	a.partial[key] = bytes.Clone(buffer)
	return lines
}

func (a *liveLogAssembler) decorateText(key liveLogKey, text string) string {
	if !a.needsContinuation[key] {
		return text
	}

	delete(a.needsContinuation, key)
	if text == "" {
		return text
	}

	return "[...continued] " + text
}

func plainTransitionLines(envelope observe.Envelope) []string {
	if envelope.Kind != observe.KindTransition || envelope.Transition == nil {
		return nil
	}

	line := fmt.Sprintf(
		"[live] %s status=%s path=%s",
		envelope.Transition.EventKind,
		envelope.Transition.Status,
		envelope.Node.Path,
	)
	if envelope.Transition.FailureSummary != "" {
		line += fmt.Sprintf(" summary=%q", envelope.Transition.FailureSummary)
	}

	return []string{line}
}

func formatLiveLogLine(line liveLogLine) string {
	if line.stream == "" {
		return fmt.Sprintf("[%s] %s", line.path, line.text)
	}

	return fmt.Sprintf("[%s][%s] %s", line.path, line.stream, line.text)
}

func liveScenarioPath(node observe.NodeRef) string {
	switch node.Kind {
	case liveNodeKindScenario:
		return node.Path
	case liveNodeKindAct:
		return liveScenarioPathFromActPath(node.Path)
	case liveNodeKindAction:
		return liveScenarioPathFromActPath(liveActPath(node))
	default:
		return ""
	}
}

func liveActPath(node observe.NodeRef) string {
	switch node.Kind {
	case liveNodeKindAct:
		return node.Path
	case liveNodeKindAction:
		return strings.TrimSuffix(node.Path, "/action")
	default:
		return ""
	}
}

func liveScenarioPathFromActPath(actPath string) string {
	index := strings.LastIndex(actPath, "/act.")
	if index < 0 {
		return ""
	}

	return actPath[:index]
}

func liveScenarioLabel(node observe.NodeRef, scenarioPath string) string {
	switch {
	case node.ScenarioCallID != "" && node.ScenarioID != "" && node.ScenarioCallID != node.ScenarioID:
		return node.ScenarioCallID + " (" + node.ScenarioID + ")"
	case node.ScenarioCallID != "":
		return node.ScenarioCallID
	case node.ScenarioID != "":
		return node.ScenarioID
	default:
		return liveNodeLabelFromSegment(livePathLastSegment(scenarioPath), "call")
	}
}

func liveActLabel(path string) string {
	return liveNodeLabelFromSegment(livePathLastSegment(path), "act")
}

func livePathLastSegment(path string) string {
	index := strings.LastIndex(path, "/")
	if index < 0 {
		return path
	}

	return path[index+1:]
}

func liveNodeLabelFromSegment(segment, wantKind string) string {
	kind, encodedID, ok := strings.Cut(segment, ".")
	if !ok || kind != wantKind {
		return segment
	}

	return decodeLiveRuntimeID(encodedID)
}

func decodeLiveRuntimeID(encoded string) string {
	var builder strings.Builder
	builder.Grow(len(encoded))

	for i := 0; i < len(encoded); i++ {
		if encoded[i] != '~' {
			builder.WriteByte(encoded[i])
			continue
		}
		if i+1 >= len(encoded) {
			return encoded
		}

		switch encoded[i+1] {
		case '0':
			builder.WriteByte('~')
			i++
		case '1':
			builder.WriteByte('/')
			i++
		case '2':
			builder.WriteByte('.')
			i++
		case 'x':
			if i+3 >= len(encoded) {
				return encoded
			}

			value, err := strconv.ParseUint(encoded[i+2:i+4], 16, 8)
			if err != nil {
				return encoded
			}

			builder.WriteByte(byte(value))
			i += 3
		default:
			return encoded
		}
	}

	return builder.String()
}

func findLineBreak(buffer []byte) (index, width int, pending bool) {
	for i := 0; i < len(buffer); i++ {
		switch buffer[i] {
		case '\n':
			return i, 1, false
		case '\r':
			if i+1 < len(buffer) && buffer[i+1] == '\n' {
				return i, 2, false
			}
			if i+1 >= len(buffer) {
				return -1, 0, true
			}

			return i, 1, false
		}
	}

	return -1, 0, false
}

func sanitizeLiveChunkText(chunk []byte) string {
	return streamtext.Render(chunk)
}

func emptyLiveFallback(value, fallback string) string {
	if value != "" {
		return value
	}

	return fallback
}

func liveNodeStatusLabel(status string, attempt int) string {
	label := emptyLiveFallback(status, liveStatusRunning)
	if attempt <= 1 {
		return label
	}

	return fmt.Sprintf("%s, attempt=%d", label, attempt)
}

func maxLiveAttempt(current, candidate int) int {
	if candidate > current {
		return candidate
	}

	return current
}

func isTerminalWriter(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	return ok && isTerminalFile(file)
}

func isTerminalReader(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	return ok && isTerminalFile(file)
}

func isTerminalFile(file *os.File) bool {
	if file == nil {
		return false
	}

	info, err := file.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}
