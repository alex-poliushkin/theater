package theatercli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/alex-poliushkin/theater"
)

const (
	completionShellBash       = "bash"
	completionShellFish       = "fish"
	completionShellPowerShell = "powershell"
	completionShellZsh        = "zsh"
)

func (a *application) completeCommand(args []string) int {
	for _, suggestion := range a.completionSuggestions(args) {
		fmt.Fprintln(a.stdout, formatCompletionSuggestion(suggestion))
	}
	return 0
}

func (a *application) helpCommand(args []string) int {
	if len(args) == 0 {
		a.printExplicitHelp(a.stdout, nil)
		return 0
	}

	target := args
	if isHelpFlag(args[len(args)-1]) {
		target = args[:len(args)-1]
	}
	if len(target) == 0 {
		a.printExplicitHelp(a.stdout, a.commands.Must(commandHelp))
		return 0
	}

	spec, ok := a.commands.LookupHelpTarget(target...)
	if !ok || (spec.Hidden && !spec.HelpTopic) {
		fmt.Fprintf(a.stderr, "unknown help target %q\n", strings.Join(args, " "))
		a.printUsage()
		return exitCodeCommandError
	}

	a.printExplicitHelp(a.stdout, spec)
	return 0
}

func (a *application) resolveRequiredSubcommand(parent string, args []string) (*commandSpec, []string, bool) {
	if len(args) == 0 {
		fmt.Fprintf(a.stderr, "%s requires a subcommand\n", parent)
		a.commands.PrintCommand(a.stderr, a.commands.Must(parent), nil, a.textStyler(a.stderr))
		return nil, nil, false
	}
	if isHelpFlag(args[0]) {
		a.commands.PrintCommand(a.stderr, a.commands.Must(parent), nil, a.textStyler(a.stderr))
		return nil, nil, false
	}

	command := a.commands.Must(parent).subcommand(args[0])
	if command == nil {
		fmt.Fprintf(a.stderr, "unknown %s subcommand %q\n", parent, args[0])
		a.commands.PrintCommand(a.stderr, a.commands.Must(parent), nil, a.textStyler(a.stderr))
		return nil, nil, false
	}

	return command, args[1:], true
}

func (a *application) versionCommand(args []string) int {
	spec := a.commands.Must(commandVersion)
	flags := a.newFlagSet(spec)
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return exitCodeCommandError
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(a.stderr, "version does not accept positional arguments")
		return exitCodeCommandError
	}
	_, _ = fmt.Fprintf(a.stdout, "theater %s\n", theater.Version())
	return 0
}

func (a *application) completionCommand(args []string) int {
	spec := a.commands.Must(commandCompletion)
	flags := a.newFlagSet(spec)
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return exitCodeCommandError
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(a.stderr, "completion requires one shell: bash, zsh, fish, or powershell")
		return exitCodeCommandError
	}

	script, ok := completionScript(flags.Arg(0))
	if !ok {
		fmt.Fprintf(a.stderr, "unsupported shell %q\n", flags.Arg(0))
		return exitCodeCommandError
	}
	if _, err := io.WriteString(a.stdout, script); err != nil {
		fmt.Fprintf(a.stderr, "write completion script: %v\n", err)
		return exitCodeCommandError
	}
	return 0
}

func completionScript(shell string) (string, bool) {
	switch shell {
	case completionShellBash:
		return bashCompletionScript, true
	case completionShellZsh:
		return zshCompletionScript, true
	case completionShellFish:
		return fishCompletionScript, true
	case completionShellPowerShell:
		return powershellCompletionScript, true
	default:
		return "", false
	}
}

const bashCompletionScript = `#!/usr/bin/env bash
_theater_complete() {
  local out line value
  COMPREPLY=()
  out="$(theater __complete "${COMP_WORDS[@]:1}")"
  if [[ -z "${out}" ]]; then
    return 0
  fi
  while IFS= read -r line; do
    value="${line%%$'\t'*}"
    COMPREPLY+=("${value}")
  done <<< "${out}"
}
complete -F _theater_complete theater
`

const zshCompletionScript = `#compdef theater
_theater_complete() {
  local out line value desc
  local -a completions descriptions
  typeset -ga _theater_completion_descriptions
  out="$(env THEATER_COMPLETE_DESCRIPTIONS=1 theater __complete "${words[@]:1}")"
  while IFS=$'\t' read -r value desc; do
    [[ -z "${value}" ]] && continue
    completions+=("${value}")
    descriptions+=("${desc:-${value}}")
  done <<< "${out}"
  _theater_completion_descriptions=("${descriptions[@]}")
  compadd -d _theater_completion_descriptions -- $completions
}
compdef _theater_complete theater
`

const fishCompletionScript = `complete -c theater -f -a '(env THEATER_COMPLETE_DESCRIPTIONS=1 theater __complete (commandline -opc)[2..-1])'
`

const powershellCompletionScript = `Register-ArgumentCompleter -CommandName theater -ScriptBlock {
  param($wordToComplete, $commandAst, $cursorPosition)
  $tokens = @()
  foreach ($element in $commandAst.CommandElements) {
    if ($element.Extent.Text -ne 'theater') {
      $tokens += $element.Extent.Text
    }
  }
  $previous = $env:THEATER_COMPLETE_DESCRIPTIONS
  $env:THEATER_COMPLETE_DESCRIPTIONS = '1'
  try {
    theater __complete @tokens | ForEach-Object {
      $parts = $_ -split [char]9, 2
      $completion = $parts[0]
      $description = if ($parts.Length -gt 1) { $parts[1] } else { $parts[0] }
      [System.Management.Automation.CompletionResult]::new($completion, $completion, 'ParameterValue', $description)
    }
  } finally {
    if ($null -eq $previous) {
      Remove-Item Env:THEATER_COMPLETE_DESCRIPTIONS -ErrorAction SilentlyContinue
    } else {
      $env:THEATER_COMPLETE_DESCRIPTIONS = $previous
    }
  }
}
`

func commandCompletionValues() []string {
	return []string{
		completionShellBash,
		completionShellFish,
		completionShellPowerShell,
		completionShellZsh,
	}
}

func completionCandidates(spec *commandSpec) []string {
	if spec == nil {
		return nil
	}

	candidates := make([]string, 0, len(spec.Subcommands)+8)
	for _, child := range spec.Subcommands {
		if child.Hidden {
			continue
		}
		candidates = append(candidates, child.Name)
		candidates = append(candidates, child.Aliases...)
	}
	if spec.Name == commandCompletion {
		candidates = append(candidates, commandCompletionValues()...)
	}
	for _, group := range spec.FlagGroups {
		for _, name := range group.Flags {
			candidates = append(candidates, "--"+name, "-"+name)
		}
	}

	return uniqueSorted(candidates)
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	slices.Sort(values)
	compacted := values[:0]
	for _, value := range values {
		if value == "" {
			continue
		}
		if len(compacted) != 0 && compacted[len(compacted)-1] == value {
			continue
		}
		compacted = append(compacted, value)
	}
	return compacted
}
