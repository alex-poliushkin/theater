package docscheck

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alex-poliushkin/theater/internal/theatercli"
)

const (
	directivePrefix       = "<!-- theater-doc:"
	directiveSuffix       = "-->"
	defaultCommandTimeout = 10 * time.Second

	languageBash    = "bash"
	languageConsole = "console"
	languageGo      = "go"
	languageJSON    = "json"
	languageSh      = "sh"
	languageShell   = "shell"
	languageText    = "text"
	languageTHTR    = "thtr"
	languageYAML    = "yaml"
	languageYML     = "yml"
)

var publicURLPattern = regexp.MustCompile(`https?://[^\s"'` + "`" + `)\]}]+`)

func Check(options Options) error {
	return Checker{}.Check(options)
}

type Options struct {
	RepoRoot       string
	DocsDir        string
	MarkdownFiles  []string
	CommandTimeout time.Duration
	Runner         CommandRunner
}

type Checker struct{}

type CommandRunner interface {
	Run(ctx context.Context, command Command) CommandResult
}

type Command struct {
	Args []string
	Dir  string
}

type CommandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Err      error
}

type Finding struct {
	File    string
	Line    int
	Message string
}

type CheckError struct {
	Findings []Finding
}

type TheaterCommandRunner struct{}

func (c Checker) Check(options Options) error {
	normalized := normalizeOptions(options)
	markdownFiles, findings := collectMarkdownFiles(normalized)

	state := newCheckState()
	for _, path := range markdownFiles {
		findings = append(findings, checkMarkdownFile(normalized, state, path)...)
	}
	findings = append(findings, state.pairFindings()...)
	if len(findings) != 0 {
		return CheckError{Findings: findings}
	}
	return nil
}

func (e CheckError) Error() string {
	var builder strings.Builder
	builder.WriteString("documentation examples check failed:")
	for _, finding := range e.Findings {
		builder.WriteString("\n- ")
		builder.WriteString(finding.File)
		if finding.Line > 0 {
			builder.WriteString(":")
			builder.WriteString(strconv.Itoa(finding.Line))
		}
		builder.WriteString(": ")
		builder.WriteString(finding.Message)
	}
	return builder.String()
}

func (r TheaterCommandRunner) Run(ctx context.Context, command Command) CommandResult {
	if len(command.Args) == 0 {
		return CommandResult{ExitCode: -1, Err: errors.New("command is empty")}
	}

	if command.Args[0] == "theater" {
		//nolint:contextcheck // theatercli.Run has no context-aware entrypoint; docscheck uses it for exact CLI parity.
		return runTheaterCommand(command)
	}

	cmd := exec.CommandContext(ctx, command.Args[0], command.Args[1:]...)
	cmd.Dir = command.Dir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		exitCode = -1
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		}
	}

	return CommandResult{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Err:      nonExitError(err),
	}
}

type checkOptions struct {
	repoRoot       string
	docsDir        string
	markdownFiles  []string
	commandTimeout time.Duration
	runner         CommandRunner
}

type checkState struct {
	ids   map[string]sourceLocation
	pairs map[string]map[string]sourceLocation
}

type sourceLocation struct {
	file string
	line int
}

type directive struct {
	action string
	attrs  map[string]string
	line   int
}

type codeFence struct {
	language string
	content  string
	line     int
}

var theaterCommandMu sync.Mutex

func normalizeOptions(options Options) checkOptions {
	repoRoot := options.RepoRoot
	if repoRoot == "" {
		repoRoot = "."
	}
	repoRoot = absoluteCleanPath(repoRoot)
	docsDir := absoluteCleanPath(options.DocsDir)
	markdownFiles := make([]string, 0, len(options.MarkdownFiles))
	for _, path := range options.MarkdownFiles {
		markdownFiles = append(markdownFiles, absoluteCleanPath(path))
	}
	timeout := options.CommandTimeout
	if timeout == 0 {
		timeout = defaultCommandTimeout
	}
	runner := options.Runner
	if runner == nil {
		runner = TheaterCommandRunner{}
	}

	return checkOptions{
		repoRoot:       filepath.Clean(repoRoot),
		docsDir:        docsDir,
		markdownFiles:  markdownFiles,
		commandTimeout: timeout,
		runner:         runner,
	}
}

func collectMarkdownFiles(options checkOptions) ([]string, []Finding) {
	var findings []Finding
	seen := make(map[string]bool)
	paths := make([]string, 0, len(options.markdownFiles))

	if options.docsDir == "." || options.docsDir == "" {
		findings = append(findings, Finding{Message: "DocsDir is required"})
		return nil, findings
	}

	if err := filepath.WalkDir(options.docsDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			findings = append(findings, Finding{File: path, Message: fmt.Sprintf("walk markdown file: %v", err)})
			return nil
		}
		if entry.IsDir() || strings.ToLower(filepath.Ext(path)) != ".md" {
			return nil
		}
		clean := filepath.Clean(path)
		if !seen[clean] {
			seen[clean] = true
			paths = append(paths, clean)
		}
		return nil
	}); err != nil {
		findings = append(findings, Finding{File: options.docsDir, Message: fmt.Sprintf("walk docs dir: %v", err)})
	}

	for _, path := range options.markdownFiles {
		clean := filepath.Clean(path)
		if seen[clean] {
			continue
		}
		seen[clean] = true
		paths = append(paths, clean)
	}

	sort.Strings(paths)
	return paths, findings
}

func newCheckState() *checkState {
	return &checkState{
		ids:   make(map[string]sourceLocation),
		pairs: make(map[string]map[string]sourceLocation),
	}
}

func checkMarkdownFile(options checkOptions, state *checkState, path string) []Finding {
	data, err := os.ReadFile(path)
	if err != nil {
		return []Finding{{File: path, Message: fmt.Sprintf("read markdown file: %v", err)}}
	}

	var findings []Finding
	lines := strings.Split(string(data), "\n")
	var pending *directive
	for i := 0; i < len(lines); i++ {
		lineNo := i + 1
		line := lines[i]
		if directive, ok, err := parseDirective(line, lineNo); err != nil {
			findings = append(findings, Finding{File: path, Line: lineNo, Message: err.Error()})
			continue
		} else if ok {
			if pending != nil {
				findings = append(findings, Finding{
					File:    path,
					Line:    pending.line,
					Message: "theater-doc marker is not followed by a code fence",
				})
			}
			pending = &directive
			continue
		}

		if pending != nil && strings.TrimSpace(line) != "" && !isFenceOpen(line) {
			findings = append(findings, Finding{
				File:    path,
				Line:    pending.line,
				Message: "theater-doc marker must be directly followed by a code fence",
			})
			pending = nil
		}

		if !isFenceOpen(line) {
			continue
		}

		fence, next, closed := collectFence(lines, i)
		if !closed {
			findings = append(findings, Finding{File: path, Line: lineNo, Message: "code fence is not closed"})
			break
		}
		i = next

		if !isTrackedLanguage(fence.language) {
			if pending != nil {
				findings = append(findings, Finding{
					File:    path,
					Line:    pending.line,
					Message: "theater-doc marker is attached to an unsupported code fence language",
				})
				pending = nil
			}
			continue
		}

		if pending == nil {
			findings = append(findings, Finding{
				File:    path,
				Line:    fence.line,
				Message: fmt.Sprintf("code fence with language %q requires a theater-doc marker", fence.language),
			})
			continue
		}

		findings = append(findings, checkFence(options, state, path, *pending, fence)...)
		pending = nil
	}

	if pending != nil {
		findings = append(findings, Finding{
			File:    path,
			Line:    pending.line,
			Message: "theater-doc marker is not followed by a code fence",
		})
	}

	return findings
}

func checkFence(options checkOptions, state *checkState, markdownPath string, directive directive, fence codeFence) []Finding {
	switch directive.action {
	case "source":
		return checkSourceFence(options, state, markdownPath, directive, fence)
	case "command":
		return checkCommandFence(options, state, markdownPath, directive, fence)
	default:
		return []Finding{{
			File:    markdownPath,
			Line:    directive.line,
			Message: fmt.Sprintf("unsupported theater-doc directive %q", directive.action),
		}}
	}
}

func checkSourceFence(options checkOptions, state *checkState, markdownPath string, directive directive, fence codeFence) []Finding {
	var findings []Finding
	if idFindings := state.registerID(markdownPath, directive); len(idFindings) != 0 {
		findings = append(findings, idFindings...)
	}

	kind := directive.attrs["kind"]
	if kind == "" {
		findings = append(findings, Finding{File: markdownPath, Line: directive.line, Message: "source directive requires kind"})
	}
	sourcePath := directive.attrs["path"]
	if sourcePath == "" {
		findings = append(findings, Finding{File: markdownPath, Line: directive.line, Message: "source directive requires path"})
	}
	if kind != "" && !languageMatchesKind(fence.language, kind) {
		findings = append(findings, Finding{
			File:    markdownPath,
			Line:    fence.line,
			Message: fmt.Sprintf("source kind %q does not match code fence language %q", kind, fence.language),
		})
	}
	if len(findings) != 0 {
		return findings
	}

	resolvedSource := resolveRelativePath(filepath.Dir(markdownPath), sourcePath)
	data, err := os.ReadFile(resolvedSource)
	if err != nil {
		return append(findings, Finding{
			File:    markdownPath,
			Line:    directive.line,
			Message: fmt.Sprintf("read source file %q: %v", sourcePath, err),
		})
	}
	if normalizeExampleSource(fence.content) != normalizeExampleSource(string(data)) {
		findings = append(findings, Finding{
			File:    markdownPath,
			Line:    fence.line,
			Message: fmt.Sprintf("code fence does not match source file %q", sourcePath),
		})
	}
	if urlFindings := publicInternetFindings(markdownPath, directive.line, string(data)); len(urlFindings) != 0 {
		findings = append(findings, urlFindings...)
		return findings
	}

	if pair := directive.attrs["pair"]; pair != "" {
		state.registerPair(pair, kind, markdownPath, directive.line)
	}

	for _, check := range sourceChecks(kind, directive.attrs["checks"]) {
		findings = append(findings, runSourceCheck(options, markdownPath, directive.line, resolvedSource, check)...)
	}
	return findings
}

func checkCommandFence(options checkOptions, state *checkState, markdownPath string, directive directive, fence codeFence) []Finding {
	var findings []Finding
	if idFindings := state.registerID(markdownPath, directive); len(idFindings) != 0 {
		findings = append(findings, idFindings...)
	}
	if !isShellLanguage(fence.language) {
		findings = append(findings, Finding{
			File:    markdownPath,
			Line:    fence.line,
			Message: fmt.Sprintf("command directive requires a shell code fence, got %q", fence.language),
		})
	}
	if len(findings) != 0 {
		return findings
	}

	commandLine, ok := oneLineCommand(fence.content)
	if !ok {
		findings = append(findings, Finding{
			File:    markdownPath,
			Line:    fence.line,
			Message: "command examples must contain exactly one non-empty line",
		})
		return findings
	}
	args, err := splitCommandLine(commandLine)
	if err != nil {
		return append(findings, Finding{File: markdownPath, Line: fence.line, Message: fmt.Sprintf("parse command: %v", err)})
	}
	if len(args) == 0 {
		return append(findings, Finding{File: markdownPath, Line: fence.line, Message: "command is empty"})
	}
	if urlFindings := publicInternetFindings(markdownPath, directive.line, commandLine); len(urlFindings) != 0 {
		return append(findings, urlFindings...)
	}

	dir := filepath.Dir(markdownPath)
	if cwd := directive.attrs["cwd"]; cwd != "" {
		dir = resolveRelativePath(filepath.Dir(markdownPath), cwd)
	}
	ctx, cancel := context.WithTimeout(context.Background(), options.commandTimeout)
	defer cancel()
	result := options.runner.Run(ctx, Command{Args: args, Dir: dir})
	findings = append(findings, checkCommandResult(markdownPath, directive, result)...)
	return findings
}

func (s *checkState) registerID(markdownPath string, directive directive) []Finding {
	id := directive.attrs["id"]
	if id == "" {
		return []Finding{{File: markdownPath, Line: directive.line, Message: directive.action + " directive requires id"}}
	}
	if previous, ok := s.ids[id]; ok {
		return []Finding{{
			File: markdownPath,
			Line: directive.line,
			Message: fmt.Sprintf(
				"theater-doc id %q is duplicated; first seen at %s:%d",
				id,
				previous.file,
				previous.line,
			),
		}}
	}
	s.ids[id] = sourceLocation{file: markdownPath, line: directive.line}
	return nil
}

func (s *checkState) registerPair(pair, kind, markdownPath string, line int) {
	kinds := s.pairs[pair]
	if kinds == nil {
		kinds = make(map[string]sourceLocation)
		s.pairs[pair] = kinds
	}
	kinds[kind] = sourceLocation{file: markdownPath, line: line}
}

func (s *checkState) pairFindings() []Finding {
	var findings []Finding
	pairs := make([]string, 0, len(s.pairs))
	for pair := range s.pairs {
		pairs = append(pairs, pair)
	}
	sort.Strings(pairs)
	for _, pair := range pairs {
		kinds := s.pairs[pair]
		if _, hasTHTR := kinds[languageTHTR]; !hasTHTR {
			findings = append(findings, pairFinding(pair, kinds))
			continue
		}
		if _, hasYAML := kinds[languageYAML]; !hasYAML {
			findings = append(findings, pairFinding(pair, kinds))
		}
	}
	return findings
}

func pairFinding(pair string, kinds map[string]sourceLocation) Finding {
	for _, location := range kinds {
		return Finding{
			File:    location.file,
			Line:    location.line,
			Message: fmt.Sprintf("pair %q must include both Theater DSL and YAML source examples", pair),
		}
	}
	return Finding{Message: fmt.Sprintf("pair %q must include both Theater DSL and YAML source examples", pair)}
}

func checkCommandResult(markdownPath string, directive directive, result CommandResult) []Finding {
	var findings []Finding
	if result.Err != nil {
		findings = append(findings, Finding{
			File:    markdownPath,
			Line:    directive.line,
			Message: fmt.Sprintf("run command: %v", result.Err),
		})
	}

	wantExit := 0
	if raw := directive.attrs["expect-exit"]; raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			findings = append(findings, Finding{
				File:    markdownPath,
				Line:    directive.line,
				Message: fmt.Sprintf("expect-exit must be an integer: %v", err),
			})
		} else {
			wantExit = parsed
		}
	}
	if result.ExitCode != wantExit {
		findings = append(findings, Finding{
			File: markdownPath,
			Line: directive.line,
			Message: fmt.Sprintf(
				"command exit code mismatch: got %d want %d stdout=%q stderr=%q",
				result.ExitCode,
				wantExit,
				result.Stdout,
				result.Stderr,
			),
		})
	}
	for _, want := range directiveAttrValues(directive.attrs, "expect-stdout") {
		if !strings.Contains(result.Stdout, want) {
			findings = append(findings, Finding{
				File:    markdownPath,
				Line:    directive.line,
				Message: fmt.Sprintf("command stdout does not contain %q; stdout=%q", want, result.Stdout),
			})
		}
	}
	for _, rejected := range directiveAttrValues(directive.attrs, "reject-stdout") {
		if strings.Contains(result.Stdout, rejected) {
			findings = append(findings, Finding{
				File:    markdownPath,
				Line:    directive.line,
				Message: fmt.Sprintf("command stdout contains rejected text %q; stdout=%q", rejected, result.Stdout),
			})
		}
	}
	for _, want := range directiveAttrValues(directive.attrs, "expect-stderr") {
		if !strings.Contains(result.Stderr, want) {
			findings = append(findings, Finding{
				File:    markdownPath,
				Line:    directive.line,
				Message: fmt.Sprintf("command stderr does not contain %q; stderr=%q", want, result.Stderr),
			})
		}
	}
	for _, rejected := range directiveAttrValues(directive.attrs, "reject-stderr") {
		if strings.Contains(result.Stderr, rejected) {
			findings = append(findings, Finding{
				File:    markdownPath,
				Line:    directive.line,
				Message: fmt.Sprintf("command stderr contains rejected text %q; stderr=%q", rejected, result.Stderr),
			})
		}
	}
	return findings
}

func directiveAttrValues(attrs map[string]string, base string) []string {
	values := make([]string, 0, 1)
	if value := attrs[base]; value != "" {
		values = append(values, value)
	}

	numberedPrefix := base + "-"
	numbered := make([]string, 0)
	for key := range attrs {
		if strings.HasPrefix(key, numberedPrefix) {
			numbered = append(numbered, key)
		}
	}
	sort.Strings(numbered)
	for _, key := range numbered {
		if value := attrs[key]; value != "" {
			values = append(values, value)
		}
	}
	return values
}

func runSourceCheck(options checkOptions, markdownPath string, line int, sourcePath, check string) []Finding {
	args := sourceCheckArgs(sourcePath, check)
	if len(args) == 0 {
		return []Finding{{
			File:    markdownPath,
			Line:    line,
			Message: fmt.Sprintf("unsupported source check %q", check),
		}}
	}

	ctx, cancel := context.WithTimeout(context.Background(), options.commandTimeout)
	defer cancel()
	result := options.runner.Run(ctx, Command{Args: args, Dir: options.repoRoot})
	if result.Err != nil {
		return []Finding{{
			File:    markdownPath,
			Line:    line,
			Message: fmt.Sprintf("%s source check failed to run: %v", check, result.Err),
		}}
	}
	if result.ExitCode != 0 {
		return []Finding{{
			File: markdownPath,
			Line: line,
			Message: fmt.Sprintf(
				"%s source check failed with exit code %d stdout=%q stderr=%q",
				check,
				result.ExitCode,
				result.Stdout,
				result.Stderr,
			),
		}}
	}
	return nil
}

func sourceCheckArgs(sourcePath, check string) []string {
	switch check {
	case "fmt":
		return []string{"theater", "fmt", "--check", sourcePath}
	case "lower":
		return []string{"theater", "lower", sourcePath}
	case "validate":
		return []string{"theater", "validate", sourcePath}
	case "run":
		return []string{"theater", "run", sourcePath, "--live", "off"}
	default:
		return nil
	}
}

func sourceChecks(kind, raw string) []string {
	if raw != "" {
		return splitCommaList(raw)
	}
	switch kind {
	case languageTHTR:
		return []string{"fmt", "lower", "validate"}
	case languageYAML:
		return []string{"validate"}
	default:
		return nil
	}
}

func splitCommaList(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func parseDirective(line string, lineNo int) (directive, bool, error) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, directivePrefix) {
		return directive{}, false, nil
	}
	if !strings.HasSuffix(trimmed, directiveSuffix) {
		return directive{}, false, errors.New("theater-doc marker must end with -->")
	}

	body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, directivePrefix), directiveSuffix))
	fields, err := splitCommandLine(body)
	if err != nil {
		return directive{}, false, fmt.Errorf("parse theater-doc marker: %w", err)
	}
	if len(fields) == 0 {
		return directive{}, false, errors.New("theater-doc marker requires a directive")
	}

	attrs := make(map[string]string, len(fields)-1)
	for _, field := range fields[1:] {
		key, value, ok := strings.Cut(field, "=")
		if !ok || key == "" {
			return directive{}, false, fmt.Errorf("theater-doc attribute %q must use key=value", field)
		}
		attrs[key] = value
	}

	return directive{
		action: fields[0],
		attrs:  attrs,
		line:   lineNo,
	}, true, nil
}

func collectFence(lines []string, openIndex int) (codeFence, int, bool) {
	line := strings.TrimSpace(lines[openIndex])
	language := strings.Fields(strings.TrimPrefix(line, "```"))
	fence := codeFence{line: openIndex + 1}
	if len(language) != 0 {
		fence.language = strings.ToLower(language[0])
	}

	var content strings.Builder
	for i := openIndex + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
			return codeFence{
				language: fence.language,
				content:  content.String(),
				line:     fence.line,
			}, i, true
		}
		content.WriteString(lines[i])
		if i != len(lines)-1 {
			content.WriteByte('\n')
		}
	}

	return fence, len(lines) - 1, false
}

func isFenceOpen(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "```")
}

func isTrackedLanguage(language string) bool {
	switch language {
	case languageBash, languageConsole, languageGo, languageJSON, languageSh, languageShell,
		languageText, languageTHTR, languageYAML, languageYML:
		return true
	default:
		return false
	}
}

func isShellLanguage(language string) bool {
	switch language {
	case languageBash, languageConsole, languageSh, languageShell:
		return true
	default:
		return false
	}
}

func languageMatchesKind(language, kind string) bool {
	switch kind {
	case languageTHTR:
		return language == languageTHTR
	case languageYAML:
		return language == languageYAML || language == languageYML
	case languageGo:
		return language == languageGo
	case languageJSON, "output", "plugin", languageText:
		return true
	default:
		return false
	}
}

func publicInternetFindings(markdownPath string, line int, content string) []Finding {
	urls := publicURLPattern.FindAllString(content, -1)
	if len(urls) == 0 {
		return nil
	}

	findings := make([]Finding, 0, len(urls))
	for _, raw := range urls {
		parsed, err := url.Parse(raw)
		if err != nil {
			continue
		}
		host := strings.ToLower(parsed.Hostname())
		if isLocalOrReservedHost(host) {
			continue
		}
		findings = append(findings, Finding{
			File:    markdownPath,
			Line:    line,
			Message: fmt.Sprintf("public internet URL %q is not allowed in docs examples", raw),
		})
	}
	return findings
}

func isLocalOrReservedHost(host string) bool {
	return host == "localhost" ||
		host == "127.0.0.1" ||
		host == "::1" ||
		strings.HasSuffix(host, ".localhost") ||
		strings.HasSuffix(host, ".test") ||
		strings.HasSuffix(host, ".invalid") ||
		strings.HasSuffix(host, ".example")
}

func normalizeExampleSource(source string) string {
	return strings.TrimSpace(strings.ReplaceAll(source, "\r\n", "\n"))
}

func resolveRelativePath(base, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(base, path))
}

func absoluteCleanPath(path string) string {
	if path == "" {
		return ""
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(absolute)
}

func oneLineCommand(content string) (string, bool) {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	var command string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if command != "" {
			return "", false
		}
		command = trimmed
	}
	return command, command != ""
}

func splitCommandLine(raw string) ([]string, error) {
	var fields []string
	var current strings.Builder
	inQuote := false
	escaped := false

	flush := func() {
		if current.Len() == 0 {
			return
		}
		fields = append(fields, current.String())
		current.Reset()
	}

	for _, r := range raw {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == '"':
			inQuote = !inQuote
		case r == ' ' || r == '\t':
			if inQuote {
				current.WriteRune(r)
				continue
			}
			flush()
		default:
			current.WriteRune(r)
		}
	}
	if escaped {
		current.WriteRune('\\')
	}
	if inQuote {
		return nil, errors.New("unterminated quote")
	}
	flush()
	return fields, nil
}

func runTheaterCommand(command Command) CommandResult {
	theaterCommandMu.Lock()
	defer theaterCommandMu.Unlock()

	restore, err := changeWorkingDirectory(command.Dir)
	if err != nil {
		return CommandResult{ExitCode: -1, Err: err}
	}
	defer restore()

	var stdout strings.Builder
	var stderr strings.Builder
	exitCode := theatercli.Run(command.Args[1:], &stdout, &stderr)
	return CommandResult{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
}

func changeWorkingDirectory(dir string) (func(), error) {
	if dir == "" {
		return func() {}, nil
	}

	previous, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if err := os.Chdir(dir); err != nil {
		return nil, err
	}

	return func() {
		_ = os.Chdir(previous)
	}, nil
}

func nonExitError(err error) error {
	if err == nil {
		return nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return nil
	}
	return err
}

var _ CommandRunner = TheaterCommandRunner{}
