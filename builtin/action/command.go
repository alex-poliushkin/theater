package action

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/runtimevalue"
	"github.com/alex-poliushkin/theater/observe"
)

const (
	commandCaptureLimitBytes      = 1 << 20
	commandStreamTailLimitBytes   = 4 * 1024
	commandSpillFilePatternPrefix = "theater-command-"
)

type commandAction struct{}

type commandArgs struct {
	Executable string
	Args       []string
	Env        map[string]string
	WorkingDir string
	Stdin      string
	Timeout    string
}

type commandError struct {
	summary string
	cause   error
	partial theater.Outputs
}

type commandOutputLimitError struct {
	stream string
	limit  int
}

type commandInvocation struct {
	contextErr func() error
	cancel     context.CancelFunc
	cmd        *exec.Cmd
	stdout     *commandStreamCapture
	stderr     *commandStreamCapture
}

type commandStreamCapture struct {
	stream    string
	limit     int
	tailLimit int
	cancel    context.CancelFunc
	reporter  observe.Reporter
	file      *os.File
	filePath  string
	sizeBytes int
	tail      []byte
	writeErr  error
	exceeded  bool
}

func (commandAction) Contract() theater.ActionContract {
	stringValue := theater.ValueContract{
		Kind:        theater.ValueKindString,
		Sensitivity: theater.SensitivityInternal,
		Capture:     theater.CaptureSummary,
	}

	return theater.ActionContract{
		Inputs: map[string]theater.ValueContract{
			"executable": {
				Kind:        theater.ValueKindString,
				Required:    true,
				Sensitivity: theater.SensitivityInternal,
				Capture:     theater.CaptureSummary,
			},
			"args": {
				Kind:        theater.ValueKindList,
				Elem:        &stringValue,
				Sensitivity: theater.SensitivityInternal,
				Capture:     theater.CaptureSummary,
			},
			"env": {
				Kind:        theater.ValueKindObject,
				Elem:        &stringValue,
				Sensitivity: theater.SensitivitySecret,
				Capture:     theater.CaptureOmit,
			},
			"working_dir": stringValue,
			"stdin": {
				Kind:        theater.ValueKindString,
				Sensitivity: theater.SensitivityInternal,
				Capture:     theater.CaptureOmit,
			},
			"timeout": stringValue,
		},
		Outputs: map[string]theater.ValueContract{
			"exit_code": {
				Kind:        theater.ValueKindNumber,
				Sensitivity: theater.SensitivityInternal,
				Capture:     theater.CaptureSummary,
			},
			"stdout": stringValue,
			"stderr": stringValue,
		},
	}
}

func (commandAction) Run(ctx context.Context, request theater.ActionRequest) (theater.Outputs, error) {
	resolvedArgs, err := resolveCommandArgs(request.Args)
	if err != nil {
		return theater.Outputs{}, err
	}

	invocation, err := newCommandInvocation(ctx, resolvedArgs, request.Reporter)
	if err != nil {
		return theater.Outputs{}, err
	}

	return invocation.Run()
}

func newCommandInvocation(parent context.Context, args commandArgs, reporter observe.Reporter) (*commandInvocation, error) {
	runCtx, cancel, err := args.context(parent)
	if err != nil {
		return nil, &commandError{
			summary: "command timeout is invalid",
			cause:   err,
		}
	}

	cmd := exec.CommandContext(runCtx, args.Executable, args.Args...)
	configureCommandProcess(cmd)
	if args.WorkingDir != "" {
		cmd.Dir = args.WorkingDir
	}
	if args.Stdin != "" {
		cmd.Stdin = strings.NewReader(args.Stdin)
	}
	if len(args.Env) != 0 {
		cmd.Env = mergeCommandEnv(cmd.Environ(), args.Env)
	}

	stdout, err := newCommandStreamCapture("stdout", cancel, reporter)
	if err != nil {
		cancel()
		return nil, &commandError{
			summary: "command capture setup failed",
			cause:   err,
		}
	}

	stderr, err := newCommandStreamCapture("stderr", cancel, reporter)
	if err != nil {
		stdout.dispose()
		cancel()
		return nil, &commandError{
			summary: "command capture setup failed",
			cause:   err,
		}
	}

	invocation := &commandInvocation{
		contextErr: runCtx.Err,
		cancel:     cancel,
		cmd:        cmd,
		stdout:     stdout,
		stderr:     stderr,
	}
	invocation.cmd.Stdout = invocation.stdout
	invocation.cmd.Stderr = invocation.stderr
	return invocation, nil
}

func (i *commandInvocation) Run() (theater.Outputs, error) {
	defer i.cleanup()
	defer i.cancel()

	if err := i.cmd.Start(); err != nil {
		return theater.Outputs{}, &commandError{
			summary: "command start failed",
			cause:   err,
		}
	}

	waitErr := i.cmd.Wait()
	if err := firstCommandCaptureError(i.stdout.captureErr(), i.stderr.captureErr()); err != nil {
		return theater.Outputs{}, &commandError{
			summary: "command capture failed",
			cause:   err,
			partial: i.partialOutputs(),
		}
	}

	if i.outputLimitExceeded() {
		return theater.Outputs{}, &commandError{
			summary: "command output exceeded capture limit",
			cause:   firstCommandLimitError(i.stdout.err(), i.stderr.err()),
			partial: i.partialOutputs(),
		}
	}

	if waitErr != nil {
		if i.contextErr() != nil {
			return theater.Outputs{}, &commandError{
				summary: commandFailureSummary(i.contextErr(), waitErr),
				cause:   i.contextErr(),
				partial: i.partialOutputs(),
			}
		}

		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			return i.outputs()
		}

		return theater.Outputs{}, &commandError{
			summary: commandFailureSummary(i.contextErr(), waitErr),
			cause:   waitErr,
			partial: i.partialOutputs(),
		}
	}

	return i.outputs()
}

func (a commandArgs) context(parent context.Context) (context.Context, context.CancelFunc, error) {
	if a.Timeout == "" {
		ctx, cancel := context.WithCancel(parent)
		return ctx, cancel, nil
	}

	timeout, err := time.ParseDuration(a.Timeout)
	if err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	return ctx, cancel, nil
}

func (e *commandError) Error() string {
	if e.cause == nil {
		return e.summary
	}

	return e.summary + ": " + e.cause.Error()
}

func (e *commandError) Unwrap() error {
	return e.cause
}

func (e *commandError) FailureSummary() string {
	return e.summary
}

func (e *commandError) PartialOutputs() theater.Outputs {
	if len(e.partial) == 0 {
		return nil
	}

	cloned := make(theater.Outputs, len(e.partial))
	for key, value := range e.partial {
		cloned[key] = value
	}

	return cloned
}

func (e *commandOutputLimitError) Error() string {
	return fmt.Sprintf("%s exceeded %d bytes", e.stream, e.limit)
}

func newCommandStreamCapture(stream string, cancel context.CancelFunc, reporter observe.Reporter) (*commandStreamCapture, error) {
	file, err := os.CreateTemp("", commandSpillFilePatternPrefix+stream+"-*")
	if err != nil {
		return nil, err
	}

	return &commandStreamCapture{
		stream:    stream,
		limit:     commandCaptureLimitBytes,
		tailLimit: commandStreamTailLimitBytes,
		cancel:    cancel,
		reporter:  reporter,
		file:      file,
		filePath:  file.Name(),
	}, nil
}

func (c *commandStreamCapture) Write(chunk []byte) (int, error) {
	if len(chunk) == 0 {
		return 0, nil
	}

	remaining := c.limit - c.sizeBytes
	accepted := len(chunk)
	if accepted > remaining {
		accepted = remaining
	}

	written := 0
	if accepted > 0 {
		n, err := c.writeAccepted(chunk[:accepted])
		written += n
		if err != nil {
			return written, err
		}
	}

	if written < len(chunk) {
		c.exceeded = true
		if c.cancel != nil {
			c.cancel()
		}
		return written, c.err()
	}

	return written, nil
}

func (i *commandInvocation) cleanup() {
	if i == nil {
		return
	}

	if i.stdout != nil {
		i.stdout.dispose()
	}
	if i.stderr != nil {
		i.stderr.dispose()
	}
}

func (i *commandInvocation) outputLimitExceeded() bool {
	return i.stdout.exceeded || i.stderr.exceeded
}

func (i *commandInvocation) partialOutputs() theater.Outputs {
	return partialCommandOutputs(i.stdout, i.stderr)
}

func (i *commandInvocation) outputs() (theater.Outputs, error) {
	return commandOutputs(i.cmd, i.stdout, i.stderr)
}

func resolveCommandArgs(args theater.Args) (commandArgs, error) {
	resolved := commandArgs{}

	executable, err := stringArg(args, "executable")
	if err != nil {
		return resolved, err
	}
	resolved.Executable = executable

	resolved.Args, err = stringListArg(args, "args")
	if err != nil {
		return resolved, err
	}

	resolved.Env, err = stringMapArg(args, "env")
	if err != nil {
		return resolved, err
	}

	resolved.WorkingDir, err = optionalStringArg(args, "working_dir")
	if err != nil {
		return resolved, err
	}

	resolved.Stdin, err = optionalStringArg(args, "stdin")
	if err != nil {
		return resolved, err
	}

	resolved.Timeout, err = optionalStringArg(args, "timeout")
	if err != nil {
		return resolved, err
	}

	return resolved, nil
}

func stringArg(args theater.Args, key string) (string, error) {
	value, ok := args[key]
	if !ok {
		return "", fmt.Errorf("%s is required", key)
	}

	return runtimevalue.String(value, key)
}

func optionalStringArg(args theater.Args, key string) (string, error) {
	value, ok := args[key]
	if !ok {
		return "", nil
	}

	return runtimevalue.String(value, key)
}

func stringListArg(args theater.Args, key string) ([]string, error) {
	value, ok := args[key]
	if !ok {
		return nil, nil
	}

	return runtimevalue.StringList(value, key)
}

func stringMapArg(args theater.Args, key string) (map[string]string, error) {
	value, ok := args[key]
	if !ok {
		return map[string]string{}, nil
	}

	return runtimevalue.StringMap(value, key)
}

func mergeCommandEnv(base []string, overrides map[string]string) []string {
	return mergeCommandEnvWithCase(base, overrides, runtime.GOOS == "windows")
}

func mergeCommandEnvWithCase(base []string, overrides map[string]string, caseInsensitive bool) []string {
	merged := newCommandEnvSet(caseInsensitive)
	for _, entry := range base {
		key, value, _ := strings.Cut(entry, "=")
		merged.Set(key, value)
	}

	keys := make([]string, 0, len(overrides))
	for key := range overrides {
		keys = append(keys, key)
	}

	sort.Slice(keys, func(i, j int) bool {
		left := normalizeCommandEnvKey(keys[i], caseInsensitive)
		right := normalizeCommandEnvKey(keys[j], caseInsensitive)
		if left != right {
			return left < right
		}

		return keys[i] < keys[j]
	})

	for _, key := range keys {
		merged.Set(key, overrides[key])
	}

	return merged.Environ()
}

type commandEnvSet struct {
	caseInsensitive bool
	entries         []commandEnvEntry
	index           map[string]int
}

type commandEnvEntry struct {
	Key   string
	Value string
}

func newCommandEnvSet(caseInsensitive bool) *commandEnvSet {
	return &commandEnvSet{
		caseInsensitive: caseInsensitive,
		index:           make(map[string]int),
	}
}

func (s *commandEnvSet) Set(key, value string) {
	normalized := normalizeCommandEnvKey(key, s.caseInsensitive)
	index, ok := s.index[normalized]
	if !ok {
		s.index[normalized] = len(s.entries)
		s.entries = append(s.entries, commandEnvEntry{Key: key, Value: value})
		return
	}

	s.entries[index] = commandEnvEntry{Key: key, Value: value}
}

func (s *commandEnvSet) Environ() []string {
	if len(s.entries) == 0 {
		return nil
	}

	env := make([]string, 0, len(s.entries))
	for _, entry := range s.entries {
		env = append(env, entry.Key+"="+entry.Value)
	}

	return env
}

func normalizeCommandEnvKey(key string, caseInsensitive bool) string {
	if !caseInsensitive {
		return key
	}

	return strings.ToUpper(key)
}

func firstCommandLimitError(errs ...error) error {
	for _, err := range errs {
		var limitErr *commandOutputLimitError
		if errors.As(err, &limitErr) {
			return err
		}
	}

	return nil
}

func firstCommandCaptureError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}

	return nil
}

func partialCommandOutputs(stdout, stderr *commandStreamCapture) theater.Outputs {
	outputs := make(theater.Outputs)
	if stdout.tailString() != "" {
		outputs["stdout"] = stdout.tailString()
	}

	if stderr.tailString() != "" {
		outputs["stderr"] = stderr.tailString()
	}

	if len(outputs) == 0 {
		return nil
	}

	return outputs
}

func commandOutputs(cmd *exec.Cmd, stdout, stderr *commandStreamCapture) (theater.Outputs, error) {
	stdoutValue, err := stdout.readAll()
	if err != nil {
		return nil, fmt.Errorf("read stdout capture: %w", err)
	}

	stderrValue, err := stderr.readAll()
	if err != nil {
		return nil, fmt.Errorf("read stderr capture: %w", err)
	}

	return theater.Outputs{
		"exit_code": cmd.ProcessState.ExitCode(),
		"stdout":    stdoutValue,
		"stderr":    stderrValue,
	}, nil
}

func (c *commandStreamCapture) err() error {
	if !c.exceeded {
		return nil
	}

	return &commandOutputLimitError{stream: c.stream, limit: c.limit}
}

func (c *commandStreamCapture) captureErr() error {
	return c.writeErr
}

func (c *commandStreamCapture) tailString() string {
	if len(c.tail) == 0 {
		return ""
	}

	return string(c.tail)
}

func (c *commandStreamCapture) readAll() (string, error) {
	if c == nil || c.file == nil {
		return "", nil
	}

	if _, err := c.file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	data, err := io.ReadAll(c.file)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func (c *commandStreamCapture) writeAccepted(chunk []byte) (int, error) {
	if len(chunk) == 0 {
		return 0, nil
	}

	written, err := c.file.Write(chunk)
	if written > 0 {
		accepted := chunk[:written]
		c.sizeBytes += written
		c.tail = appendCommandStreamTail(c.tail, accepted, c.tailLimit)
		c.publish(accepted)
	}

	if err != nil {
		c.writeErr = err
		if c.cancel != nil {
			c.cancel()
		}
		return written, err
	}

	if written != len(chunk) {
		c.writeErr = io.ErrShortWrite
		if c.cancel != nil {
			c.cancel()
		}
		return written, io.ErrShortWrite
	}

	return written, nil
}

func (c *commandStreamCapture) dispose() {
	if c == nil {
		return
	}

	if c.file != nil {
		_ = c.file.Close()
		c.file = nil
	}
	if c.filePath != "" {
		_ = os.Remove(c.filePath)
		c.filePath = ""
	}
}

func appendCommandStreamTail(existing, chunk []byte, limit int) []byte {
	if limit <= 0 || len(chunk) == 0 {
		return existing
	}

	if len(chunk) >= limit {
		return append([]byte(nil), chunk[len(chunk)-limit:]...)
	}

	if len(existing)+len(chunk) <= limit {
		tail := make([]byte, 0, len(existing)+len(chunk))
		tail = append(tail, existing...)
		tail = append(tail, chunk...)
		return tail
	}

	drop := len(existing) + len(chunk) - limit
	tail := make([]byte, 0, limit)
	tail = append(tail, existing[drop:]...)
	tail = append(tail, chunk...)
	return tail
}

func (c *commandStreamCapture) publish(chunk []byte) {
	if c.reporter == nil || len(chunk) == 0 {
		return
	}

	c.reporter.LogChunk(observe.LogChunk{Stream: c.stream, Data: chunk})
}

func commandFailureSummary(runErr, err error) string {
	switch {
	case errors.Is(runErr, context.DeadlineExceeded), errors.Is(err, context.DeadlineExceeded):
		return "command timed out"
	case errors.Is(runErr, context.Canceled), errors.Is(err, context.Canceled):
		return "command execution canceled"
	default:
		return "command failed"
	}
}
