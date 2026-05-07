package theatercli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	initDefaultFlowDir = repoLayoutRootName + "/" + repoLayoutFlowRootName + "/http"
	initDefaultFlowID  = "starter"
	initFlagSyntax     = "syntax"
	initLibDir         = repoLayoutRootName + "/" + repoLayoutLibraryRootName
	initParentDir      = ".."
)

const (
	initStarterYAML = `id: starter

scenarios:
  - id: http/check-starter
    acts:
      - id: fetch-homepage
        action:
          use: action.http
          with:
            method: GET
            url: https://example.com
            session: none
        expectations:
          - id: status-ok
            subject: status_code
            assert:
              eq: 200
          - id: page-text
            subject: body
            assert:
              contains: Example Domain

scenario_calls:
  - id: fetch-starter
    scenario_id: http/check-starter
`
	initStarterTHTR = `stage starter

scenario http/check-starter
  act fetch-homepage
    do action.http(method: "GET", url: "https://example.com", session: "none")
    expect status-ok: field(status_code) == 200
    expect page-text: field(body) matches r"Example Domain"

call fetch-starter = http/check-starter()
`
)

type initSyntax string

const (
	initSyntaxTHTR initSyntax = "thtr"
	initSyntaxYAML initSyntax = "yaml"
)

type initCommandOptions struct {
	syntax initSyntax
	target string
}

type initCommandFlagValues struct {
	syntax string
}

type initCommandResult struct {
	libPath   string
	stagePath string
}

func (a *application) initCommand(args []string) int {
	options, ok := a.parseInitOptions(args)
	if !ok {
		return exitCodeCommandError
	}

	result, err := writeInitStarter(options)
	if err != nil {
		fmt.Fprintf(a.stderr, "%v\n", err)
		return exitCodeCommandError
	}

	fmt.Fprintf(a.stdout, "wrote %s\n", result.stagePath)
	fmt.Fprintf(a.stdout, "prepared %s\n", result.libPath)
	fmt.Fprintln(a.stdout)
	fmt.Fprintln(a.stdout, "Next:")
	fmt.Fprintf(a.stdout, "  theater validate %s\n", result.stagePath)
	fmt.Fprintf(a.stdout, "  theater run %s --live off\n", result.stagePath)
	return 0
}

func (a *application) parseInitOptions(args []string) (initCommandOptions, bool) {
	normalizedArgs, target, err := normalizeInitArgs(args)
	if err != nil {
		fmt.Fprintf(a.stderr, "%v\n", err)
		return initCommandOptions{}, false
	}

	flags, options, values := a.newInitCommandFlagSet()
	if err := flags.Parse(normalizedArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return initCommandOptions{}, false
		}
		return initCommandOptions{}, false
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(a.stderr, "init accepts at most one target path")
		return initCommandOptions{}, false
	}

	syntax, err := parseInitSyntax(values.syntax)
	if err != nil {
		fmt.Fprintf(a.stderr, "%v\n", err)
		return initCommandOptions{}, false
	}

	options.syntax = syntax
	options.target = target

	return *options, true
}

func writeInitStarter(options initCommandOptions) (initCommandResult, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return initCommandResult{}, fmt.Errorf("resolve current working directory: %w", err)
	}

	workspaceRoot := cwd
	if layout, ok := resolveRepoLayout(cwd); ok {
		workspaceRoot = layout.RepoRoot
	}

	root, err := os.OpenRoot(workspaceRoot)
	if err != nil {
		return initCommandResult{}, fmt.Errorf("open init workspace root: %w", err)
	}
	defer func() {
		_ = root.Close()
	}()

	stagePath, displayStagePath, err := resolveInitTarget(workspaceRoot, options.target, options.syntax)
	if err != nil {
		return initCommandResult{}, err
	}

	if err := root.MkdirAll(filepath.Dir(stagePath), 0o755); err != nil {
		return initCommandResult{}, fmt.Errorf("prepare init target directory: %w", err)
	}

	if err := root.MkdirAll(filepath.FromSlash(initLibDir), 0o755); err != nil {
		return initCommandResult{}, fmt.Errorf("prepare theater/lib: %w", err)
	}

	file, err := root.OpenFile(stagePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return initCommandResult{}, fmt.Errorf("init target already exists: %s", displayStagePath)
	}
	if err != nil {
		return initCommandResult{}, fmt.Errorf("write init target: %w", err)
	}
	if _, err := file.WriteString(starterTemplate(options.syntax)); err != nil {
		_ = file.Close()
		return initCommandResult{}, fmt.Errorf("write init target: %w", err)
	}
	if err := file.Close(); err != nil {
		return initCommandResult{}, fmt.Errorf("write init target: %w", err)
	}

	return initCommandResult{
		stagePath: displayStagePath,
		libPath:   filepath.ToSlash(initLibDir),
	}, nil
}

func parseInitSyntax(raw string) (initSyntax, error) {
	switch initSyntax(strings.ToLower(strings.TrimSpace(raw))) {
	case initSyntaxYAML:
		return initSyntaxYAML, nil
	case initSyntaxTHTR:
		return initSyntaxTHTR, nil
	default:
		return "", errors.New("init requires --syntax yaml or thtr")
	}
}

func normalizeInitArgs(args []string) (normalized []string, target string, err error) {
	normalized = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		raw := args[i]
		if raw == doubleDashToken {
			rest := args[i+1:]
			if len(rest) == 0 {
				return normalized, target, nil
			}
			if target != "" || len(rest) > 1 {
				return nil, "", errors.New("init accepts at most one target path")
			}
			return normalized, rest[0], nil
		}

		name, hasInlineValue, isFlag := parseCLIFlagToken(raw)
		if !isFlag {
			if target != "" {
				return nil, "", errors.New("init accepts at most one target path")
			}
			target = raw
			continue
		}

		normalized = append(normalized, raw)
		if hasInlineValue || isHelpFlag(raw) {
			continue
		}
		if name == initFlagSyntax && i+1 < len(args) {
			i++
			normalized = append(normalized, args[i])
		}
	}

	return normalized, target, nil
}

func resolveInitTarget(cwd, rawTarget string, syntax initSyntax) (targetPath, displayTarget string, err error) {
	target := strings.TrimSpace(rawTarget)
	if target == "" {
		target = defaultInitTarget(syntax)
	}
	target = filepath.Clean(target)
	if filepath.Ext(target) == "" {
		target += syntax.extension()
	}
	if filepath.IsAbs(target) {
		target, err = filepath.Rel(cwd, target)
		if err != nil {
			return "", "", fmt.Errorf("relativize init target: %w", err)
		}
	}

	target = filepath.Clean(target)
	if ext := strings.ToLower(filepath.Ext(target)); ext != syntax.extension() {
		return "", "", fmt.Errorf("init target extension %q does not match --syntax %s", ext, syntax)
	}

	relToFlowsRoot, err := filepath.Rel(filepath.FromSlash("theater/flows"), target)
	if err != nil {
		return "", "", fmt.Errorf("resolve init target location: %w", err)
	}
	if relToFlowsRoot == "." ||
		relToFlowsRoot == initParentDir ||
		strings.HasPrefix(relToFlowsRoot, initParentDir+string(filepath.Separator)) {
		return "", "", errors.New("init target must stay under theater/flows/")
	}

	return target, filepath.ToSlash(target), nil
}

func defaultInitTarget(syntax initSyntax) string {
	return filepath.Join(filepath.FromSlash(initDefaultFlowDir), initDefaultFlowID+syntax.extension())
}

func starterTemplate(syntax initSyntax) string {
	switch syntax {
	case initSyntaxTHTR:
		return initStarterTHTR
	default:
		return initStarterYAML
	}
}

func (s initSyntax) extension() string {
	switch s {
	case initSyntaxTHTR:
		return thtrFileExtension
	default:
		return yamlFileExtension
	}
}
