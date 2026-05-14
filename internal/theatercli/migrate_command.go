package theatercli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alex-poliushkin/theater"
	authoringthtr "github.com/alex-poliushkin/theater/internal/authoring/thtr"
	authoringyaml "github.com/alex-poliushkin/theater/internal/authoring/yaml"
)

const commandMigrateFromYAML = "from-yaml"

type migrateCommandOptions struct {
	globalOptions
	file string
}

func (a *application) runMigrateCommand(args []string) int {
	command, rest, ok := a.resolveRequiredSubcommand(commandMigrate, args)
	if !ok {
		return exitCodeCommandError
	}

	switch command.Name {
	case commandMigrateFromYAML:
		return a.migrateFromYAML(rest)
	default:
		fmt.Fprintf(a.stderr, "unknown migrate subcommand %q\n", args[0])
		a.commands.PrintCommand(a.stderr, a.commands.Must(commandMigrate), nil, a.textStyler(a.stderr))
		return exitCodeCommandError
	}
}

func (a *application) migrateFromYAML(args []string) int {
	options, ok := a.parseMigrateFromYAMLOptions(args)
	if !ok {
		return exitCodeCommandError
	}

	services, err := a.ensureServices(options.pluginsConfig, options.pluginsLock, pluginReadinessRuntime)
	if err != nil {
		fmt.Fprintf(a.stderr, "build built-in catalogs: %v\n", err)
		return exitCodeCommandError
	}

	spec, err := loadYAMLForMigration(options.file, services.matcherSugar)
	if err != nil {
		fmt.Fprintf(a.stderr, "migrate from-yaml: %v\n", err)
		return exitCodeCommandError
	}

	source, err := authoringthtr.MarshalStage(spec)
	if err != nil {
		fmt.Fprintf(a.stderr, "migrate from-yaml: %v\n", err)
		return exitCodeCommandError
	}

	if _, err := a.stdout.Write(source); err != nil {
		fmt.Fprintf(a.stderr, "write migrated source: %v\n", err)
		return exitCodeCommandError
	}

	return 0
}

func (a *application) parseMigrateFromYAMLOptions(args []string) (migrateCommandOptions, bool) {
	flags, options := a.newMigrateFromYAMLFlagSet()
	if err := flags.Parse(args); err != nil {
		return migrateCommandOptions{}, false
	}
	options.globalOptions = sharedGlobalOptionContract.Resolve(options.globalOptions)
	if flags.NArg() != 0 {
		fmt.Fprintln(a.stderr, "migrate from-yaml does not accept positional arguments")
		return migrateCommandOptions{}, false
	}
	if options.file == "" {
		fmt.Fprintln(a.stderr, "migrate from-yaml requires --file")
		return migrateCommandOptions{}, false
	}

	ext := strings.ToLower(filepath.Ext(options.file))
	if ext != yamlFileExtension && ext != ymlFileExtension {
		fmt.Fprintln(a.stderr, "migrate from-yaml requires a .yaml or .yml file")
		return migrateCommandOptions{}, false
	}

	return *options, true
}

func loadYAMLForMigration(path string, matchers theater.MatcherSugarResolver) (theater.StageSpec, error) {
	location, err := authoringyaml.ResolveFlowFileLocation(path)
	if err != nil {
		return theater.StageSpec{}, err
	}

	return loadYAMLStageSpec(location, matchers)
}
