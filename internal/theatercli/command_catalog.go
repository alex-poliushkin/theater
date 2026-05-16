package theatercli

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

const (
	commandGroupAuthoring   = "Authoring"
	commandGroupDiscover    = "Discover"
	commandGroupEnvironment = "Environment"
	commandGroupOperations  = "Operations"
	commandGroupPlugins     = "Plugins"
	commandGroupStartHere   = "Start Here"
	flagGroupBehavior       = "Behavior"
	flagGroupDebug          = "Debug"
	flagGroupFiles          = "Files"
	flagGroupOutput         = "Output"
	flagGroupPlugins        = "Plugins"
)

type commandFlagProfile string

const (
	commandFlagProfileDoctor         commandFlagProfile = "doctor"
	commandFlagProfileExplain        commandFlagProfile = "explain"
	commandFlagProfileFmt            commandFlagProfile = "fmt"
	commandFlagProfileInit           commandFlagProfile = "init"
	commandFlagProfileListScenarios  commandFlagProfile = "list-scenarios"
	commandFlagProfileLower          commandFlagProfile = "lower"
	commandFlagProfileMigrateYAML    commandFlagProfile = "migrate-yaml"
	commandFlagProfileNone           commandFlagProfile = ""
	commandFlagProfilePluginsDigest  commandFlagProfile = "plugins-digest"
	commandFlagProfilePluginsDoctor  commandFlagProfile = "plugins-doctor"
	commandFlagProfilePluginsInspect commandFlagProfile = "plugins-inspect"
	commandFlagProfilePluginsLock    commandFlagProfile = "plugins-lock"
	commandFlagProfileReportRender   commandFlagProfile = "report-render"
	commandFlagProfileRun            commandFlagProfile = "run"
	commandFlagProfileValidate       commandFlagProfile = "validate"
)

type commandCatalog struct {
	root   *commandSpec
	topics []*commandSpec
}

type commandHelpGroup struct {
	Title    string
	Commands []*commandSpec
}

type commandSpec struct {
	Name        string
	Path        string
	Args        string
	Short       string
	Long        string
	Aliases     []string
	Examples    []commandExample
	Sections    []commandHelpSection
	Hidden      bool
	HelpTopic   bool
	FlagProfile commandFlagProfile
	FlagGroups  []flagHelpGroup
	Environment []environmentHelpEntry
	Defaults    []string
	Subcommands []*commandSpec
	Groups      []commandHelpGroup
}

type commandExample struct {
	Title   string
	Command string
}

type commandHelpSection struct {
	Title string
	Lines []string
}

type flagHelpGroup struct {
	Title string
	Flags []string
}

type environmentHelpEntry struct {
	Name        string
	Description string
}

func newCommandCatalog() commandCatalog {
	init := newInitCommandSpec()
	run := newRunCommandSpec()
	validate := newValidateCommandSpec()
	explain := newExplainCommandSpec()
	doctor := newDoctorCommandSpec()
	list := newListCommandSpec()
	format := newFormatCommandSpec()
	lower := newLowerCommandSpec()
	migrate := newMigrateCommandSpec()
	plugins := newPluginsCommandSpec()
	report := newReportCommandSpec()
	help := newHelpCommandSpec()
	version := newVersionCommandSpec()
	completion := newCompletionCommandSpec()
	complete := newCompleteCommandSpec()
	topics := []*commandSpec{
		newEnvironmentHelpTopic(),
		newExitCodesHelpTopic(),
		newFormatsHelpTopic(),
		newDebugSelectorsHelpTopic(),
		newCompatibilityHelpTopic(),
	}

	root := &commandSpec{
		Name:  "theater",
		Path:  "theater",
		Args:  "<command> [options]",
		Short: "Validate-first CLI for reusable verification flows.",
		Long: "Use theater to validate stages early, run real verification flows, " +
			"author compact .thtr files, and inspect plugin registries without leaving the standard command line workflow.",
		Examples: []commandExample{
			{Command: "theater init"},
			{Command: "theater validate theater/flows/http/example-domain.yaml"},
			{Command: "theater run theater/flows/http/example-domain.yaml --live off"},
		},
		Environment: combineEnvironmentHelp(sharedPluginEnvironmentHelp(), sharedOutputEnvironmentHelp()),
		Defaults:    combineDefaultResolutionHelp(sharedPluginDefaultResolutionHelp(), outputDefaultResolutionHelp()),
		Subcommands: []*commandSpec{
			init,
			run,
			validate,
			explain,
			doctor,
			list,
			format,
			lower,
			migrate,
			plugins,
			report,
			help,
			version,
			completion,
			complete,
		},
		Groups: []commandHelpGroup{
			{Title: commandGroupStartHere, Commands: []*commandSpec{init, validate, run}},
			{Title: commandGroupAuthoring, Commands: []*commandSpec{format, lower, migrate}},
			{Title: commandGroupDiscover, Commands: []*commandSpec{explain, doctor, list, report}},
			{Title: commandGroupPlugins, Commands: []*commandSpec{plugins}},
			{Title: commandGroupEnvironment, Commands: []*commandSpec{help, version, completion}},
		},
	}

	return commandCatalog{root: root, topics: topics}
}

func newReportCommandSpec() *commandSpec {
	render := newReportRenderCommandSpec()
	return &commandSpec{
		Name:  commandReport,
		Path:  "theater report",
		Args:  "<command> [options]",
		Short: "Render CI artifacts from a saved Theater run document.",
		Long: "Use report when a stage has already run and you need additional CI-readable artifacts " +
			"from the canonical JSON run output without executing the stage again.",
		Examples: []commandExample{
			{
				Title: "Render JUnit from a saved run",
				Command: "theater report render --input build/example-domain.run.json " +
					"--format junit > build/example-domain.junit.xml",
			},
			{
				Title: "Render Markdown from a saved run",
				Command: "theater report render --input build/example-domain.run.json " +
					"--format markdown > build/example-domain.md",
			},
		},
		Sections: []commandHelpSection{
			{
				Title: "Workflow",
				Lines: []string{
					"Run the live flow once with theater run " + stageFileArgument + " --format json and save stdout.",
					"Render JUnit or Markdown from that saved JSON when CI needs test ingestion or a readable summary.",
					"Rendering succeeds or fails based on artifact generation, not on the saved run's pass/fail status.",
				},
			},
		},
		Subcommands: []*commandSpec{render},
		Groups: []commandHelpGroup{
			{Title: commandGroupOperations, Commands: []*commandSpec{render}},
		},
	}
}

func newReportRenderCommandSpec() *commandSpec {
	return &commandSpec{
		Name:        commandReportRender,
		Path:        "theater report render",
		Args:        "--input <run.json> [--format junit|markdown]",
		Short:       "Render one artifact from saved run JSON.",
		Long:        "Use report render to convert the public JSON emitted by theater run --format json into a CI artifact.",
		FlagProfile: commandFlagProfileReportRender,
		Examples: []commandExample{
			{
				Title: "Compact JUnit for test-result ingestion",
				Command: "theater report render --input build/example-domain.run.json " +
					"--format junit > build/example-domain.junit.xml",
			},
			{
				Title: "Detailed Markdown summary",
				Command: "theater report render --input build/example-domain.run.json " +
					"--format markdown > build/example-domain.md",
			},
		},
		Sections: []commandHelpSection{
			{
				Title: "Input",
				Lines: []string{
					"--input reads the JSON wrapper produced by theater run --format json.",
					"Use --input - to read the same JSON from stdin.",
				},
			},
			{
				Title: "Formats",
				Lines: []string{
					"junit keeps the compact scenario-call testcase contract used by theater run --format junit.",
					"markdown renders a bounded human-readable summary with scenarios, acts, expectations, logs, retries, and failures.",
				},
			},
		},
		FlagGroups: []flagHelpGroup{
			{Title: flagGroupFiles, Flags: []string{"input"}},
			{Title: flagGroupOutput, Flags: []string{"format"}},
		},
	}
}

func newListCommandSpec() *commandSpec {
	scenarios := newListScenariosCommandSpec()
	return &commandSpec{
		Name:        commandList,
		Path:        "theater list",
		Args:        "<resource> [options]",
		Short:       "List repo-aware Theater resources.",
		Long:        "Use list for repository-local discovery that does not validate or run a stage.",
		Subcommands: []*commandSpec{scenarios},
		Sections: []commandHelpSection{
			{
				Title: "Resources",
				Lines: []string{
					"scenarios  list reusable public scenario ids from theater/lib",
				},
			},
		},
	}
}

func newListScenariosCommandSpec() *commandSpec {
	return &commandSpec{
		Name:        commandListScenarios,
		Path:        "theater list scenarios",
		Args:        "[--root <path>] [--format text|json] [--syntax all|yaml|thtr] [--call-skeleton]",
		Short:       "List reusable scenarios from theater/lib.",
		FlagProfile: commandFlagProfileListScenarios,
		Long: "Use list scenarios when you need the public scenario ids, inputs, " +
			"source locations, and compatible authoring syntax exposed by the repo-aware theater/lib tree.",
		Examples: []commandExample{
			{Title: "List scenarios from the current repository", Command: "theater list scenarios"},
			{Title: "Machine-readable output", Command: "theater list scenarios --format json"},
			{Title: "List only YAML-callable scenarios", Command: "theater list scenarios --syntax yaml"},
			{Title: "Print runnable call snippets", Command: "theater list scenarios --call-skeleton"},
			{Title: "List scenarios from another checkout", Command: "theater list scenarios --root ../service"},
		},
		Sections: []commandHelpSection{
			{
				Title: "Repository contract",
				Lines: []string{
					"The command scans theater/lib for .yaml, .yml and .thtr files and reports public scenario ids.",
					"Each row includes the scenario authoring syntax because repo-aware flow loading is syntax-scoped.",
					"Use --syntax yaml or --syntax thtr when you need scenarios callable from a specific flow syntax.",
					"JSON output includes a call snippet for each scenario; --call-skeleton prints the same snippets in text output.",
					"It does not introduce imports, includes, package manifests or runtime loading side effects.",
					"It uses syntax-level loading only; plugin descriptors stay on validate, run, explain, doctor, and plugins commands.",
					"JSON repo_root and library_root fields are local absolute paths; source.file is repo-relative when possible.",
					"Files under internal, examples, fixtures and testdata library directories are ignored.",
					"Unsupported file syntax, symlinked files, non-regular files, oversized files, " +
						"and very large library file sets are rejected before parsing.",
					"Library files that declare scenario_calls are rejected, matching repo-aware flow loading.",
				},
			},
			{
				Title: "Related",
				Lines: []string{
					"theater validate theater/flows/auth/login-smoke.yaml  validate a flow that calls a listed scenario",
					"theater explain formats  inspect text and json output conventions",
				},
			},
		},
		FlagGroups: []flagHelpGroup{
			{Title: flagGroupFiles, Flags: []string{"root"}},
			{Title: flagGroupOutput, Flags: []string{"format", "call-skeleton"}},
		},
	}
}

func newInitCommandSpec() *commandSpec {
	return &commandSpec{
		Name:        commandInit,
		Path:        "theater init",
		Args:        "[theater/flows/.../starter.{yaml|thtr}] [--syntax yaml|thtr]",
		Short:       "Create a small repo-aware starter stage.",
		FlagProfile: commandFlagProfileInit,
		Long: "Use init when you want one small starter stage under theater/flows/, " +
			"aligned with the shipped example-domain flow, plus the theater/lib root for later reusable packages.",
		Examples: []commandExample{
			{Title: "Write the default YAML starter", Command: "theater init"},
			{Title: "Start with compact .thtr authoring", Command: "theater init --syntax thtr"},
			{Title: "Choose a different flow path", Command: "theater init theater/flows/http/login-smoke.yaml"},
		},
		Sections: []commandHelpSection{
			{
				Title: "Starter contract",
				Lines: []string{
					"init writes one starter stage under theater/flows/ and prepares theater/lib for later reusable scenario packages.",
					"The generated file stays intentionally close to the shipped example-domain flow " +
						"so the next validate or run uses the real model instead of a special scaffold dialect.",
					"init never overwrites an existing target file.",
				},
			},
			{
				Title: "Syntax",
				Lines: []string{
					"yaml is the default starter syntax.",
					"Use --syntax thtr when you want the compact authoring surface from the first file.",
					"If the target path already has an extension, it must match the selected syntax.",
				},
			},
			{
				Title: "Related",
				Lines: []string{
					"theater validate theater/flows/http/starter.yaml  confirm the generated YAML starter before a live run",
					"theater run theater/flows/http/starter.yaml --live off  execute the YAML starter against https://example.com",
					"theater validate theater/flows/http/starter.thtr  confirm the compact starter when you create it with --syntax thtr",
					"theater run theater/flows/http/starter.thtr --live off  execute the compact starter against https://example.com",
				},
			},
		},
		FlagGroups: []flagHelpGroup{
			{Title: flagGroupBehavior, Flags: []string{"syntax"}},
		},
	}
}

func newRunCommandSpec() *commandSpec {
	return &commandSpec{
		Name: commandRun,
		Path: "theater run",
		Args: stageFileArgument + " [--format text|json|junit] [--live auto|off] " +
			"[--json-output <path>] [--junit-output <path>] [--markdown-output <path>] " +
			"[--overwrite] [--debug off|dump|interactive]",
		Short:       "Validate, execute, and render a stage run.",
		FlagProfile: commandFlagProfileRun,
		Long: "Use run when you want the full validate-first path: load a stage, " +
			"validate it, execute it, and render the final result as text, JSON, or JUnit. " +
			"Use sidecar output flags to write JSON, JUnit, or Markdown artifacts from the same run document. " +
			"Use a positional stage path for the common case; --file remains available " +
			"when explicit spelling helps readability, and legacy -file remains supported for compatibility.",
		Examples: []commandExample{
			{
				Title:   "Quiet text output",
				Command: "theater run theater/flows/http/example-domain.yaml --live off",
			},
			{
				Title:   "Machine-readable JSON output",
				Command: "theater run theater/flows/http/example-domain.yaml --format json > build/example-domain.run.json",
			},
			{
				Title:   "CI-friendly JUnit output",
				Command: "theater run theater/flows/http/example-domain.yaml --format junit > build/example-domain.junit.xml",
			},
			{
				Title: "Run once and write sidecar artifacts",
				Command: "theater run theater/flows/http/example-domain.yaml --live off --format text \\\n" +
					"  --json-output build/example-domain.run.json \\\n" +
					"  --junit-output build/example-domain.junit.xml \\\n" +
					"  --markdown-output build/example-domain.md",
			},
			{
				Title:   "Dump debug sidecar",
				Command: "theater run theater/flows/http/example-domain.yaml --debug dump --debug-dump build/example-domain.debug.ndjson",
			},
			{
				Title:   "Interactive debug session",
				Command: "theater run theater/flows/http/example-domain.yaml --debug interactive --step",
			},
			{
				Title: "Run with plugin registry and lock files",
				Command: "theater run theater/flows/plugins/hello-world-plugin.yaml \\\n" +
					"  --plugins-config build/hello-world.plugins.json \\\n" +
					"  --plugins-lock build/hello-world.plugins.lock.json \\\n" +
					"  --live off",
			},
			{
				Title: "Run with a plugin report exporter",
				Command: "theater run theater/flows/plugins/hello-world-plugin.yaml \\\n" +
					"  --plugins-config build/hello-world.plugins.json \\\n" +
					"  --plugins-lock build/hello-world.plugins.lock.json \\\n" +
					"  --plugin-exporter <report-exporter-capability> \\\n" +
					"  --live off",
			},
		},
		Sections: []commandHelpSection{
			{
				Title: "Output behavior",
				Lines: []string{
					"Text output keeps the final summary on stdout. With --live auto, progress lines stream on stderr during text-oriented runs.",
					"JSON and JUnit keep stdout machine-readable. Interactive debug prompts " +
						"and pause cards also stay on stderr so redirected stdout remains clean.",
					"Sidecar output flags write explicit file paths after execution and before stdout rendering. Existing files require --overwrite.",
					"Sidecar render or write failures exit with command failure status. " +
						"Failed runs still write requested sidecars when rendering succeeds.",
				},
			},
			{
				Title: "Debug workflow",
				Lines: []string{
					"Use theater validate " + stageFileArgument + " --debug-paths before you write --break or --break-file selectors.",
					"--debug dump requires --debug-dump <path> and works in automation or non-TTY shells.",
					"--debug interactive is for local terminal sessions and requires a TTY on stdin and stderr.",
				},
			},
			{
				Title: "Plugin workflow",
				Lines: []string{
					"Add --plugins-config <path> and --plugins-lock <path> when the stage depends on plugin-backed capabilities.",
					"Use --plugin-exporter <capability> when a plugin provides a report exporter to run after the final run document is frozen.",
					"Run theater explain report-exporter --plugins-config <path> to discover available plugin report exporters before a real run.",
				},
			},
			{
				Title: "Related",
				Lines: []string{
					"theater validate " + stageFileArgument + "  check stage shape and discover prepared debug selectors before the run",
					"theater help exit-codes  see the stable process-level contract for scripts and CI",
					"theater help debug-selectors  learn debug selector rules, retry filters, and selector-file rules",
				},
			},
		},
		FlagGroups: []flagHelpGroup{
			{Title: flagGroupFiles, Flags: []string{"file"}},
			{Title: flagGroupOutput, Flags: []string{"format", "live", "json-output", "junit-output", "markdown-output", "overwrite"}},
			{Title: flagGroupPlugins, Flags: []string{"plugins-config", "plugins-lock", "plugin-exporter"}},
			{Title: flagGroupDebug, Flags: []string{"debug", "break", "break-file", "step", "debug-dump", "stop-on-failure"}},
		},
		Environment: combineEnvironmentHelp(sharedPluginEnvironmentHelp(), sharedOutputEnvironmentHelp()),
		Defaults:    combineDefaultResolutionHelp(sharedPluginDefaultResolutionHelp(), outputDefaultResolutionHelp()),
	}
}

func newValidateCommandSpec() *commandSpec {
	return &commandSpec{
		Name:        commandValidate,
		Path:        "theater validate",
		Args:        stageFileArgument + " [--format text|json] [--plugins-readiness runtime|descriptor] [--debug-paths]",
		Short:       "Compile and validate a stage without live execution.",
		FlagProfile: commandFlagProfileValidate,
		Long: "Use validate when you want early feedback on stage shape, contracts, " +
			"references, and prepared debug selectors before any live work starts. " +
			"Use a positional stage path for the common case; --file remains available " +
			"when explicit spelling helps readability, and legacy -file remains supported for compatibility.",
		Aliases: []string{"check"},
		Examples: []commandExample{
			{
				Title:   "Stage validation in text",
				Command: "theater validate theater/flows/http/example-domain.thtr",
			},
			{
				Title:   "Machine-readable validation output",
				Command: "theater validate theater/flows/http/example-domain.thtr --format json > build/example-domain.validate.json",
			},
			{
				Title:   "Discover reusable debug selectors",
				Command: "theater validate theater/flows/http/example-domain.thtr --debug-paths",
			},
			{
				Title:   "Explicit file flag compatibility",
				Command: "theater validate --file theater/flows/http/example-domain.thtr",
			},
		},
		Sections: []commandHelpSection{
			{
				Title: "Validation modes",
				Lines: []string{
					"Plain validate checks the compiled stage and prints either diagnostics or a valid result.",
					"--plugins-readiness runtime is the default for plugin-backed validation and " +
						"runs plugin validate hooks with declared host environment grants.",
					"--plugins-readiness descriptor validates descriptor-backed stage structure " +
						"without launching plugin processes or resolving env_from_host grants.",
					"--debug-paths prepares the stage and prints reusable debug selectors instead of the normal valid output.",
					"--format json keeps validation and debug-path discovery automation-friendly without changing the underlying validation contract.",
				},
			},
			{
				Title: "Related",
				Lines: []string{
					"theater help debug-selectors  understand debug selector shape before using --break or --break-file",
					"theater run " + stageFileArgument + "  execute the same stage after validation passes",
					"theater lower <path.thtr>  inspect the canonical YAML form behind a .thtr file",
				},
			},
		},
		FlagGroups: []flagHelpGroup{
			{Title: flagGroupFiles, Flags: []string{"file"}},
			{Title: flagGroupOutput, Flags: []string{"format"}},
			{Title: flagGroupPlugins, Flags: []string{"plugins-config", "plugins-lock", "plugins-readiness"}},
			{Title: flagGroupDebug, Flags: []string{"debug-paths"}},
		},
		Environment: combineEnvironmentHelp(sharedPluginEnvironmentHelp(), sharedOutputEnvironmentHelp()),
		Defaults:    combineDefaultResolutionHelp(sharedPluginDefaultResolutionHelp(), outputDefaultResolutionHelp()),
	}
}

func newFormatCommandSpec() *commandSpec {
	return &commandSpec{
		Name:  commandFmt,
		Path:  "theater fmt",
		Args:  "<path.thtr> [--write] [--check] [--diff]",
		Short: "Format one .thtr file into canonical layout.",
		Long: "Use fmt to normalize .thtr source before review or lowering. " +
			"Without --write, --check, or --diff it prints the formatted source to stdout.",
		FlagProfile: commandFlagProfileFmt,
		Examples: []commandExample{
			{
				Title:   "Print canonical .thtr to stdout",
				Command: "theater fmt theater/flows/http/example-domain.thtr",
			},
			{
				Title:   "Rewrite the file in place",
				Command: "theater fmt theater/flows/http/example-domain.thtr --write",
			},
			{
				Title:   "Check formatting in CI",
				Command: "theater fmt --check theater/flows/http/example-domain.thtr",
			},
			{
				Title:   "Print the formatting diff",
				Command: "theater fmt --diff theater/flows/http/example-domain.thtr",
			},
		},
		Sections: []commandHelpSection{
			{
				Title: "Formatting behavior",
				Lines: []string{
					"fmt applies the canonical .thtr layout so small diffs stay structural instead of stylistic.",
					"--write rewrites the input file in place; without it, fmt prints the canonical .thtr form to stdout.",
					"--check exits non-zero when the file is not already formatted and writes no stdout on success.",
					"--diff prints the formatting changes without rewriting the input file.",
					"--diff may be combined with --check when CI output should include the formatting diff.",
					"--write cannot be combined with --check or --diff.",
					"--file remains available when explicit spelling helps readability.",
				},
			},
			{
				Title: "Related",
				Lines: []string{
					"theater validate <stage.thtr>  check the formatted file before a real run",
					"theater lower <path.thtr>  inspect the canonical YAML that the formatted source lowers into",
				},
			},
		},
		FlagGroups: []flagHelpGroup{
			{Title: flagGroupFiles, Flags: []string{"file"}},
			{Title: flagGroupBehavior, Flags: []string{"write", "check", "diff"}},
		},
	}
}

func newLowerCommandSpec() *commandSpec {
	return &commandSpec{
		Name:        commandLower,
		Path:        "theater lower",
		Args:        "<path.thtr> [--map <path.json>]",
		Short:       "Lower one .thtr file into canonical YAML.",
		Long:        "Use lower when you want to inspect the canonical YAML form of a .thtr file and optionally emit a source-map sidecar.",
		FlagProfile: commandFlagProfileLower,
		Examples: []commandExample{
			{
				Title:   "Print canonical YAML to stdout",
				Command: "theater lower theater/flows/http/example-domain.thtr",
			},
			{
				Title:   "Emit YAML plus a source-map sidecar",
				Command: "theater lower theater/flows/http/example-domain.thtr --map build/example-domain.map.json",
			},
		},
		Sections: []commandHelpSection{
			{
				Title: "Lowering behavior",
				Lines: []string{
					"lower always writes canonical YAML to stdout; --map adds a JSON source-map sidecar without changing stdout.",
					"Use lower when you need the interchange form, want to inspect exact YAML semantics, " +
						"or want to compare compact .thtr authoring with the canonical model.",
					"--file remains available when explicit spelling helps readability.",
				},
			},
			{
				Title: "Related",
				Lines: []string{
					"theater fmt <path.thtr>  normalize the authoring surface before lowering",
					"theater validate <stage.thtr>  validate the same .thtr stage directly without manually lowering first",
				},
			},
		},
		FlagGroups: []flagHelpGroup{
			{Title: flagGroupFiles, Flags: []string{"file", "map"}},
		},
	}
}

func newMigrateCommandSpec() *commandSpec {
	fromYAML := newMigrateFromYAMLCommandSpec()

	return &commandSpec{
		Name:  commandMigrate,
		Path:  "theater migrate",
		Args:  "<command> [options]",
		Short: "Convert existing YAML authoring into formatter-clean .thtr.",
		Long: "Use migrate when you want a one-way authoring conversion into .thtr without changing runtime semantics. " +
			"The converter prefers correct, explicit .thtr over the smallest possible output.",
		Examples: []commandExample{
			{
				Title:   "Convert one standalone YAML file",
				Command: "theater migrate from-yaml --file theater/flows/http/example-domain.yaml > theater/flows/http/example-domain.thtr",
			},
			{
				Title:   "Convert one repo-aware flow into self-contained .thtr",
				Command: "theater migrate from-yaml --file theater/flows/auth/login-smoke.yaml > build/login-smoke.thtr",
			},
		},
		Sections: []commandHelpSection{
			{
				Title: "Migration contract",
				Lines: []string{
					"migrate is a one-way authoring conversion: it emits .thtr source that stays semantically equivalent to the loaded YAML stage.",
					"For YAML flow files under theater/flows/, migrate resolves referenced library scenarios first " +
						"so the emitted .thtr stays self-contained during opportunistic adoption.",
					"The converter rewrites only the clearly safe matcher forms and keeps richer matcher objects explicit " +
						"instead of guessing at broader sugar.",
					"Treat the emitted file as semantically stable, formatter-clean source. " +
						"Exact presentation may keep evolving as new safe rewrites land.",
				},
			},
		},
		Subcommands: []*commandSpec{fromYAML},
		Groups: []commandHelpGroup{
			{Title: commandGroupAuthoring, Commands: []*commandSpec{fromYAML}},
		},
	}
}

func newMigrateFromYAMLCommandSpec() *commandSpec {
	return &commandSpec{
		Name:        commandMigrateFromYAML,
		Path:        "theater migrate from-yaml",
		Args:        "--file <stage.yaml|stage.yml> [--plugins-config <path.json> --plugins-lock <path.lock.json>]",
		Short:       "Convert one YAML stage into formatter-clean .thtr on stdout.",
		Long:        "Use migrate from-yaml when you want the current YAML stage semantics rendered as .thtr without hand-rewriting the file.",
		FlagProfile: commandFlagProfileMigrateYAML,
		Examples: []commandExample{
			{
				Title:   "Convert one standalone YAML file",
				Command: "theater migrate from-yaml --file theater/flows/http/example-domain.yaml > theater/flows/http/example-domain.thtr",
			},
			{
				Title: "Convert a plugin-enabled YAML file",
				Command: "theater migrate from-yaml --file theater/flows/plugins/hello-world-plugin.yaml " +
					"--plugins-config build/hello-world.plugins.json --plugins-lock build/hello-world.plugins.lock.json > build/hello-world-plugin.thtr",
			},
		},
		Sections: []commandHelpSection{
			{
				Title: "Emission behavior",
				Lines: []string{
					"from-yaml writes formatter-clean .thtr to stdout.",
					"The output keeps runtime semantics explicit and only applies shipped conservative rewrites, " +
						"including safe matcher sugar and repeated state-handle hoisting into record/pool aliases when the canonical pattern is unambiguous.",
					"Complex matcher objects stay canonical so the converter does not invent a second lowering path.",
					"Treat the emitted file as semantically stable output, " +
						"not as a byte-for-byte compatibility promise across future formatter or sugar expansions.",
				},
			},
			{
				Title: "Related",
				Lines: []string{
					"theater fmt <path.thtr>  normalize an edited .thtr file after conversion",
					"theater lower <path.thtr>  inspect the canonical YAML that the migrated .thtr lowers into",
					"theater validate <stage.thtr>  validate the migrated file directly before a live run",
				},
			},
		},
		FlagGroups: []flagHelpGroup{
			{Title: flagGroupFiles, Flags: []string{"file"}},
			{Title: flagGroupPlugins, Flags: []string{"plugins-config", "plugins-lock"}},
		},
		Environment: combineEnvironmentHelp(sharedPluginEnvironmentHelp()),
		Defaults:    combineDefaultResolutionHelp(sharedPluginDefaultResolutionHelp()),
	}
}

func newPluginsCommandSpec() *commandSpec {
	digest := newPluginsDigestCommandSpec()
	inspect := newPluginsInspectCommandSpec()
	lock := newPluginsLockCommandSpec()
	doctor := newPluginsDoctorCommandSpec()

	return &commandSpec{
		Name:  commandPlugins,
		Path:  "theater plugins",
		Args:  "<command> [options]",
		Short: "Inspect, digest, lock, and diagnose plugin registries.",
		Long: "Use plugins when you need one operational loop for plugin-enabled runs: " +
			"inspect the resolved set, refresh manifest descriptor digests, write or refresh the lock file, diagnose readiness, " +
			"then validate and run with the same plugin registry file and lock file paths.",
		Examples: []commandExample{
			{
				Title:   "Refresh a manifest descriptor digest",
				Command: "theater plugins digest --manifest plugins/sqlite/manifest.json --write",
			},
			{
				Title:   "Inspect the resolved plugin set",
				Command: "theater plugins inspect --plugins-config build/sqlite.plugins.json",
			},
			{
				Title: "Write the plugin lock file",
				Command: "theater plugins lock --plugins-config build/sqlite.plugins.json " +
					"--plugins-lock build/sqlite.plugins.lock.json",
			},
			{
				Title: "Diagnose plugin readiness",
				Command: "theater plugins doctor --plugins-config build/sqlite.plugins.json " +
					"--plugins-lock build/sqlite.plugins.lock.json",
			},
		},
		Sections: []commandHelpSection{
			{
				Title: "Workflow",
				Lines: []string{
					"1. theater plugins inspect  confirm the resolved plugin set and capability names",
					"2. theater plugins digest  refresh descriptor_digest after intentional manifest descriptor changes",
					"3. theater plugins lock  freeze manifest and executable checksums into the lock file",
					"4. theater plugins doctor  verify plugin registry file validity, path reachability, and optional lock drift",
					"5. theater validate " + stageFileArgument + "  validate the stage against the same plugin registry file and lock file",
					"6. theater run " + stageFileArgument + "  execute the stage against the same plugin registry file and lock file",
				},
			},
			{
				Title: "Path contract",
				Lines: []string{
					"--plugins-config always points to the plugin registry file.",
					"--plugins-lock points to the plugin lock file that validate and run should consume in automation.",
				},
			},
		},
		Environment: combineEnvironmentHelp(sharedPluginEnvironmentHelp(), sharedOutputEnvironmentHelp()),
		Defaults:    combineDefaultResolutionHelp(pluginFamilyDefaultResolutionHelp(), outputDefaultResolutionHelp()),
		Subcommands: []*commandSpec{digest, inspect, lock, doctor},
		Groups: []commandHelpGroup{
			{Title: commandGroupOperations, Commands: []*commandSpec{digest, inspect, lock, doctor}},
		},
	}
}

func newPluginsDigestCommandSpec() *commandSpec {
	return &commandSpec{
		Name:        commandPluginsDigest,
		Path:        "theater plugins digest",
		Args:        "--manifest <path> [--write]",
		Short:       "Print or update a plugin manifest descriptor digest.",
		FlagProfile: commandFlagProfilePluginsDigest,
		Long: "Use plugins digest after editing a plugin manifest protocol or capability descriptor. " +
			"Without --write it prints the canonical descriptor digest; with --write it rewrites descriptor_digest in the manifest file.",
		Examples: []commandExample{
			{
				Title:   "Print the current canonical descriptor digest",
				Command: "theater plugins digest --manifest plugins/sqlite/manifest.json",
			},
			{
				Title:   "Update descriptor_digest in place",
				Command: "theater plugins digest --manifest plugins/sqlite/manifest.json --write",
			},
		},
		Sections: []commandHelpSection{
			{
				Title: "Trust boundary",
				Lines: []string{
					"digest updates only the manifest descriptor digest derived from protocol and capability descriptors.",
					"Run theater plugins lock separately to freeze manifest and executable checksums for validate and run.",
				},
			},
			{
				Title: "Related",
				Lines: []string{
					"theater plugins inspect  review the resolved set after refreshing the manifest descriptor digest",
					"theater plugins lock  refresh the host-owned lock file after intentional manifest or executable changes",
				},
			},
		},
		FlagGroups: []flagHelpGroup{
			{Title: flagGroupFiles, Flags: []string{"manifest"}},
			{Title: flagGroupBehavior, Flags: []string{"write"}},
		},
	}
}

func newPluginsInspectCommandSpec() *commandSpec {
	return &commandSpec{
		Name:        commandPluginsInspect,
		Path:        "theater plugins inspect",
		Args:        "--plugins-config <path> [--plugins-lock <path>] [--format text|json]",
		Short:       "Resolve and print the current plugin set.",
		FlagProfile: commandFlagProfilePluginsInspect,
		Long: "Use plugins inspect to verify what the host resolves from the plugin registry file " +
			"and optional lock file before validate or run.",
		Aliases: []string{"ls"},
		Examples: []commandExample{
			{Title: "Inspect in text mode", Command: "theater plugins inspect --plugins-config build/sqlite.plugins.json"},
			{
				Title: "Inspect in JSON mode with a lock file",
				Command: "theater plugins inspect --plugins-config build/sqlite.plugins.json " +
					"--plugins-lock build/sqlite.plugins.lock.json --format json",
			},
		},
		Sections: []commandHelpSection{
			{
				Title: "Output",
				Lines: []string{
					"inspect prints the resolved plugin ids, versions, manifest paths, executable paths, and allowed capabilities.",
					"Add --plugins-lock when you want inspect to confirm checksum drift against the lock file before validate or run.",
				},
			},
			{
				Title: "Related",
				Lines: []string{
					"theater plugins lock  write the lock file after you confirm the resolved set",
					"theater plugins doctor  check manifest reachability, executable reachability, and lock drift",
				},
			},
		},
		FlagGroups: []flagHelpGroup{
			{Title: flagGroupPlugins, Flags: []string{"plugins-config", "plugins-lock"}},
			{Title: flagGroupOutput, Flags: []string{"format"}},
		},
		Environment: combineEnvironmentHelp(sharedPluginEnvironmentHelp(), sharedOutputEnvironmentHelp()),
		Defaults:    combineDefaultResolutionHelp(pluginInspectDefaultResolutionHelp(), outputDefaultResolutionHelp()),
	}
}

func newPluginsLockCommandSpec() *commandSpec {
	return &commandSpec{
		Name:        commandPluginsLock,
		Path:        "theater plugins lock",
		Args:        "--plugins-config <path> --plugins-lock <path>",
		Short:       "Write the plugin checksum lock file.",
		Long:        "Use plugins lock after checking the resolved plugin set so later validate and run commands can reject silent plugin drift.",
		FlagProfile: commandFlagProfilePluginsLock,
		Examples: []commandExample{
			{
				Title: "Write the lock file",
				Command: "theater plugins lock --plugins-config build/sqlite.plugins.json " +
					"--plugins-lock build/sqlite.plugins.lock.json",
			},
		},
		Sections: []commandHelpSection{
			{
				Title: "Output",
				Lines: []string{
					"lock writes one checksum file that records the current manifest and executable digests for each allowed plugin.",
					"Re-run lock when you intentionally change plugin manifests or executable artifacts.",
				},
			},
			{
				Title: "Related",
				Lines: []string{
					"theater plugins inspect  review the resolved set before freezing it into a lock file",
					"theater plugins doctor  verify the plugin registry file, executable paths, and resulting lock state before validate or run",
				},
			},
		},
		FlagGroups: []flagHelpGroup{
			{Title: flagGroupPlugins, Flags: []string{"plugins-config", "plugins-lock"}},
		},
		Environment: combineEnvironmentHelp(sharedPluginEnvironmentHelp(), sharedOutputEnvironmentHelp()),
		Defaults:    combineDefaultResolutionHelp(pluginLockDefaultResolutionHelp(), outputDefaultResolutionHelp()),
	}
}

func newPluginsDoctorCommandSpec() *commandSpec {
	return &commandSpec{
		Name:  commandPluginsDoctor,
		Path:  "theater plugins doctor",
		Args:  "--plugins-config <path> [--plugins-lock <path>] [--plugins-readiness runtime|descriptor]",
		Short: "Diagnose plugin registry readiness.",
		Long: "Use plugins doctor when you want one readiness check for plugin registry file " +
			"validity, manifest reachability, executable reachability, host environment grants, and optional lock drift.",
		FlagProfile: commandFlagProfilePluginsDoctor,
		Examples: []commandExample{
			{
				Title:   "Check the plugin registry file",
				Command: "theater plugins doctor --plugins-config build/sqlite.plugins.json",
			},
			{
				Title: "Check the plugin registry file and lock file",
				Command: "theater plugins doctor --plugins-config build/sqlite.plugins.json " +
					"--plugins-lock build/sqlite.plugins.lock.json",
			},
		},
		Sections: []commandHelpSection{
			{
				Title: "Checks",
				Lines: []string{
					"doctor validates the plugin registry file, loads each plugin manifest, " +
						"resolves each executable path, and optionally verifies manifest/executable checksums " +
						"against the lock file.",
					"--plugins-readiness runtime is the default and checks executable reachability plus host environment grants.",
					"--plugins-readiness descriptor checks descriptor and manifest-lock readiness " +
						"without resolving env_from_host grants or launching plugin code.",
					"Use doctor after inspect or lock when you need a quick preflight before validate or run.",
				},
			},
			{
				Title: "Related",
				Lines: []string{
					"theater plugins inspect  review the resolved plugin set and allowed capabilities",
					"theater plugins lock  refresh the lock file after intentional manifest or executable changes",
				},
			},
		},
		FlagGroups: []flagHelpGroup{
			{Title: flagGroupPlugins, Flags: []string{"plugins-config", "plugins-lock", "plugins-readiness"}},
		},
		Environment: combineEnvironmentHelp(sharedPluginEnvironmentHelp(), sharedOutputEnvironmentHelp()),
		Defaults:    combineDefaultResolutionHelp(pluginInspectDefaultResolutionHelp(), outputDefaultResolutionHelp()),
	}
}

func newHelpCommandSpec() *commandSpec {
	return &commandSpec{
		Name:  commandHelp,
		Path:  "theater help",
		Args:  "[command...]",
		Short: "Show help for a command or topic.",
		Long:  "Use help when you want the same command or topic documentation without triggering the command's normal execution path.",
		Examples: []commandExample{
			{Title: "Show run help", Command: "theater help run"},
			{Title: "Show plugin inspect help", Command: "theater help plugins inspect"},
			{Title: "Show topic help", Command: "theater help exit-codes"},
			{Title: "Show compatibility policy", Command: "theater help compatibility"},
		},
	}
}

func newExplainCommandSpec() *commandSpec {
	return &commandSpec{
		Name:  commandExplain,
		Path:  "theater explain",
		Args:  "[family|topic|query] [ref]",
		Short: "Explain discoverable runtime capabilities.",
		Long: "Use explain when you want to inspect the live capability surface from the " +
			"built-in catalog or from a plugin overlay without reading source files or " +
			"manifest JSON by hand.",
		FlagProfile: commandFlagProfileExplain,
		Examples: []commandExample{
			{Title: "List capability families", Command: "theater explain"},
			{Title: "List one family", Command: "theater explain action"},
			{Title: "Inspect one capability", Command: "theater explain action http"},
			{Title: "Search by unscoped text", Command: "theater explain http"},
			{
				Title:   "Inspect a topic instead of a capability",
				Command: "theater explain formats",
			},
			{
				Title:   "Include plugin-provided capabilities",
				Command: "theater explain action hello_world.echo --plugins-config build/hello-world.plugins.json",
			},
		},
		Sections: []commandHelpSection{
			{
				Title: "Targets",
				Lines: []string{
					"Run theater explain with no target to list capability families and built-in topics.",
					"Use a family name such as action, inventory, matcher, transform, " +
						"generator, report-exporter, or state-backend to list that slice of the catalog.",
					"Use a family plus local ref such as action http, inventory http.get, or generator email to inspect one contract in detail.",
					"Use a single non-family, non-topic query such as http to list all matching capabilities and their scoped inspect commands.",
				},
			},
			{
				Title: "Related",
				Lines: []string{
					"theater plugins inspect  review plugin manifests and allowed capabilities before you lock them",
					"theater help debug-selectors  understand the debug selectors that validate can discover",
				},
			},
		},
		FlagGroups: []flagHelpGroup{
			{Title: flagGroupPlugins, Flags: []string{"plugins-config", "plugins-lock"}},
		},
		Environment: combineEnvironmentHelp(sharedPluginEnvironmentHelp(), sharedOutputEnvironmentHelp()),
		Defaults:    combineDefaultResolutionHelp(sharedPluginDefaultResolutionHelp(), outputDefaultResolutionHelp()),
	}
}

func newDoctorCommandSpec() *commandSpec {
	return &commandSpec{
		Name:        commandDoctor,
		Path:        "theater doctor",
		Args:        "[--plugins-config <path> --plugins-lock <path>] [--write-path <path>...]",
		Short:       "Check theater workflow preconditions.",
		FlagProfile: commandFlagProfileDoctor,
		Long: "Use doctor when you want one terminal preflight for the most common theater checks: " +
			"repo-aware flow layout, plugin registry file and lock file pairing, " +
			"interactive debug TTY availability, and writable output destinations.",
		Examples: []commandExample{
			{Title: "Check the current repo workflow", Command: "theater doctor"},
			{
				Title: "Check plugin registry and lock files",
				Command: "theater doctor --plugins-config build/sqlite.plugins.json " +
					"--plugins-lock build/sqlite.plugins.lock.json",
			},
			{Title: "Check one write destination", Command: "theater doctor --write-path build/example-domain.debug.ndjson"},
		},
		Sections: []commandHelpSection{
			{
				Title: "Checks",
				Lines: []string{
					"doctor checks the current working directory for the expected theater/flows and theater/lib layout.",
					"Add a plugin registry file and plugin lock file when you want doctor " +
						"to confirm pairing and executable reachability before validate or run.",
					"Interactive debug TTY availability is advisory; non-interactive validate, run, and debug dump workflows can still be ready.",
					"Repeat --write-path for debug dumps, generated lock files, or other files that later commands must be able to write.",
					"Missing parent directories for write paths are created during the check, matching debug dump output behavior.",
				},
			},
			{
				Title: "Related",
				Lines: []string{
					"theater plugins doctor  inspect plugin registry file validity, manifest reachability, and checksum drift in more detail",
					"theater help debug-selectors  understand the interactive and dump debug workflow before a real run",
					"theater run " + stageFileArgument + "  execute the stage after doctor reports a ready environment",
				},
			},
		},
		FlagGroups: []flagHelpGroup{
			{Title: flagGroupPlugins, Flags: []string{"plugins-config", "plugins-lock"}},
			{Title: flagGroupFiles, Flags: []string{"write-path"}},
		},
		Environment: combineEnvironmentHelp(sharedPluginEnvironmentHelp(), sharedOutputEnvironmentHelp()),
		Defaults:    combineDefaultResolutionHelp(doctorDefaultResolutionHelp(), outputDefaultResolutionHelp()),
	}
}

func newVersionCommandSpec() *commandSpec {
	return &commandSpec{
		Name:  commandVersion,
		Path:  "theater version",
		Short: "Print the theater version.",
		Long:  "Use version when you want a stable CLI version string for scripts, bug reports, or release verification.",
		Examples: []commandExample{
			{Title: "Print the current release string", Command: "theater version"},
		},
		Sections: []commandHelpSection{
			{
				Title: "Output contract",
				Lines: []string{
					"Prints exactly one stdout line in the form theater <version>.",
					"Writes nothing to stderr on success.",
				},
			},
		},
	}
}

func newCompletionCommandSpec() *commandSpec {
	return &commandSpec{
		Name:  commandCompletion,
		Path:  "theater completion",
		Args:  "<bash|zsh|fish|powershell>",
		Short: "Generate shell completion scripts.",
		Long:  "Use completion to install tab completion for the theater command tree in your shell.",
		Examples: []commandExample{
			{Title: "Generate Bash completion", Command: "theater completion bash"},
			{Title: "Generate Zsh completion", Command: "theater completion zsh"},
			{Title: "Generate Fish completion", Command: "theater completion fish"},
			{Title: "Generate PowerShell completion", Command: "theater completion powershell"},
		},
	}
}

func newExitCodesHelpTopic() *commandSpec {
	return &commandSpec{
		Name:      "exit-codes",
		Path:      "theater help exit-codes",
		Short:     "Explain theater exit behavior.",
		Long:      "Use exit-codes when you need the stable process-level contract for shell scripts, CI jobs, or wrapper tools around theater.",
		HelpTopic: true,
		Sections: []commandHelpSection{
			{
				Title: "Public exit codes",
				Lines: []string{
					"0  success",
					"1  validation diagnostics, authoring diagnostics, failed run, or canceled run",
					"2  command usage error, unsupported format, or other command-level failure before a result contract exists",
				},
			},
			{
				Title: "Related",
				Lines: []string{
					"theater run " + stageFileArgument + "  execute a stage and return the same exit-code contract in text, JSON, or JUnit mode",
					"theater validate " + stageFileArgument + "  check stage shape without live execution",
				},
			},
		},
	}
}

func newDebugSelectorsHelpTopic() *commandSpec {
	return &commandSpec{
		Name:      "debug-selectors",
		Path:      "theater help debug-selectors",
		Short:     "Explain debug selector shape for run debug mode.",
		Long:      "Use debug-selectors when you want repeatable --break or --break-file inputs instead of guessing runtime paths by hand.",
		HelpTopic: true,
		Sections: []commandHelpSection{
			{
				Title: "Selector shape",
				Lines: []string{
					"kind=<scenario_call|act|action|expectation>,phase=<before|after>,path=<prepared runtime path>",
					"Use theater validate " + stageFileArgument + " --debug-paths to list prepared debug selectors before a run.",
					"Add name=<label> when you want pause output and debug dumps to carry a stable human label.",
				},
			},
			{
				Title: "Retry and terminal filters",
				Lines: []string{
					"Retry-aware selectors also accept attempt=<n> filters.",
					"when=terminal-failure stops after retries are exhausted; --stop-on-failure adds that selector automatically.",
				},
			},
			{
				Title: "Selector files",
				Lines: []string{
					"one selector per non-empty line",
					"blank lines are ignored",
					"lines starting with # are ignored",
				},
			},
		},
	}
}

func newCompleteCommandSpec() *commandSpec {
	return &commandSpec{
		Name:   commandComplete,
		Path:   "theater __complete",
		Args:   "[words...]",
		Hidden: true,
	}
}

func (c commandCatalog) Lookup(names ...string) (*commandSpec, bool) {
	spec := c.root
	if len(names) == 0 {
		return spec, true
	}

	for _, name := range names {
		spec = spec.subcommand(name)
		if spec == nil {
			return nil, false
		}
	}

	return spec, true
}

func (c commandCatalog) LookupHelpTarget(names ...string) (*commandSpec, bool) {
	if spec, ok := c.Lookup(names...); ok {
		return spec, true
	}
	if len(names) != 1 {
		return nil, false
	}
	for _, topic := range c.topics {
		if topic.matchesName(names[0]) {
			return topic, true
		}
	}
	return nil, false
}

func (c commandCatalog) Must(names ...string) *commandSpec {
	spec, ok := c.Lookup(names...)
	if !ok {
		panic(fmt.Sprintf("missing command spec for %q", strings.Join(names, " ")))
	}
	return spec
}

func (c commandCatalog) Complete(args []string) []string {
	spec := c.root
	if len(args) == 0 {
		return completionCandidates(spec)
	}

	current := args[len(args)-1]
	context := args[:len(args)-1]
	if len(context) != 0 && context[0] == commandHelp {
		return c.completeHelpTargets(context[1:], current)
	}
	if len(context) != 0 && context[0] == commandExplain {
		return c.completeExplainTargets(context[1:], current)
	}
	resolved := c.resolveCompletionContext(context, spec)
	if current == "" {
		return completionCandidates(resolved)
	}
	return filterPrefix(completionCandidates(resolved), current)
}

func (c commandCatalog) PrintCommand(writer io.Writer, spec *commandSpec, flags *flag.FlagSet, style cliTextStyler) {
	if spec == nil {
		return
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "%s\n\n", spec.Short)
	if spec.Long != "" {
		fmt.Fprintf(&builder, "%s\n\n", spec.Long)
	}
	fmt.Fprintf(&builder, "%s\n  %s\n", style.Heading("Usage:"), spec.usageLine())
	if len(spec.Aliases) != 0 {
		fmt.Fprintf(&builder, "\n%s\n", style.Heading("Aliases:"))
		for _, alias := range spec.Aliases {
			fmt.Fprintf(&builder, "  %s\n", alias)
		}
	}
	renderExamples(&builder, spec.Examples, style)
	renderHelpSections(&builder, spec.Sections, style)
	if len(spec.Groups) != 0 {
		fmt.Fprintf(&builder, "\n%s\n", style.Heading("Commands:"))
		renderCommandGroups(&builder, spec.Groups, style)
	}
	if flags != nil && hasDefinedFlags(flags) {
		fmt.Fprintf(&builder, "\n%s\n", style.Heading("Options:"))
		renderFlagGroups(&builder, flags, spec.FlagGroups, style)
	}
	if len(spec.Environment) != 0 {
		fmt.Fprintf(&builder, "\n%s\n", style.Heading("Environment:"))
		renderEnvironmentEntries(&builder, spec.Environment)
	}
	if len(spec.Defaults) != 0 {
		fmt.Fprintf(&builder, "\n%s\n", style.Heading("Resolution:"))
		renderDefaults(&builder, spec.Defaults)
	}
	if spec != c.root {
		if spec.HelpTopic {
			builder.WriteString("\nUse `theater help` to inspect another command or topic.\n")
		} else {
			builder.WriteString("\nUse `theater` to see the top-level command list.\n")
		}
	}

	_, _ = io.WriteString(writer, builder.String())
}

func (s *commandSpec) usageLine() string {
	if s == nil {
		return ""
	}
	if s.Args == "" {
		return s.Path
	}
	return s.Path + " " + s.Args
}

func (s *commandSpec) displayName() string {
	if s == nil {
		return ""
	}
	if len(s.Aliases) == 0 {
		return s.Name
	}
	return s.Name + ", " + strings.Join(s.Aliases, ", ")
}

func (s *commandSpec) subcommand(name string) *commandSpec {
	for _, child := range s.Subcommands {
		if child.matchesName(name) {
			return child
		}
	}
	return nil
}

func (s *commandSpec) matchesName(name string) bool {
	if s == nil {
		return false
	}
	if s.Name == name {
		return true
	}
	for _, alias := range s.Aliases {
		if alias == name {
			return true
		}
	}
	return false
}

func hasDefinedFlags(flags *flag.FlagSet) bool {
	count := 0
	flags.VisitAll(func(*flag.Flag) {
		count++
	})
	return count != 0
}

func renderCommandGroups(builder *strings.Builder, groups []commandHelpGroup, style cliTextStyler) {
	for _, group := range groups {
		if len(group.Commands) == 0 {
			continue
		}

		fmt.Fprintf(builder, "  %s\n", style.Heading(group.Title+":"))
		writer := tabwriter.NewWriter(builder, 0, 0, 2, ' ', 0)
		for _, command := range group.Commands {
			fmt.Fprintf(writer, "    %s\t%s\n", command.displayName(), command.Short)
		}
		_ = writer.Flush()
	}
}

func renderExamples(builder *strings.Builder, examples []commandExample, style cliTextStyler) {
	if len(examples) == 0 {
		return
	}

	fmt.Fprintf(builder, "\n%s\n", style.Heading("Examples:"))
	for _, example := range examples {
		if example.Command == "" {
			continue
		}
		if example.Title != "" {
			fmt.Fprintf(builder, "  %s:\n", example.Title)
			renderIndentedLines(builder, "    ", example.Command)
			continue
		}
		renderIndentedLines(builder, "  ", example.Command)
	}
}

func renderHelpSections(builder *strings.Builder, sections []commandHelpSection, style cliTextStyler) {
	for _, section := range sections {
		if section.Title == "" || len(section.Lines) == 0 {
			continue
		}
		fmt.Fprintf(builder, "\n%s\n", style.Heading(section.Title+":"))
		for _, line := range section.Lines {
			fmt.Fprintf(builder, "  %s\n", line)
		}
	}
}

func renderFlagGroups(builder *strings.Builder, flags *flag.FlagSet, groups []flagHelpGroup, style cliTextStyler) {
	covered := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		renderOneFlagGroup(builder, flags, group, style)
		for _, name := range group.Flags {
			covered[name] = struct{}{}
		}
	}

	var remaining []*flag.Flag
	flags.VisitAll(func(item *flag.Flag) {
		if _, ok := covered[item.Name]; ok {
			return
		}
		remaining = append(remaining, item)
	})
	if len(remaining) == 0 {
		return
	}

	fmt.Fprintf(builder, "  %s\n", style.Heading("Other:"))
	writer := tabwriter.NewWriter(builder, 0, 0, 2, ' ', 0)
	for _, item := range remaining {
		fmt.Fprintf(writer, "    %s\t%s\n", formatFlagLabel(item), formatFlagDescription(item))
	}
	_ = writer.Flush()
}

func renderOneFlagGroup(builder *strings.Builder, flags *flag.FlagSet, group flagHelpGroup, style cliTextStyler) {
	items := make([]*flag.Flag, 0, len(group.Flags))
	for _, name := range group.Flags {
		item := flags.Lookup(name)
		if item == nil {
			continue
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return
	}

	fmt.Fprintf(builder, "  %s\n", style.Heading(group.Title+":"))
	writer := tabwriter.NewWriter(builder, 0, 0, 2, ' ', 0)
	for _, item := range items {
		fmt.Fprintf(writer, "    %s\t%s\n", formatFlagLabel(item), formatFlagDescription(item))
	}
	_ = writer.Flush()
}

func renderEnvironmentEntries(builder *strings.Builder, entries []environmentHelpEntry) {
	writer := tabwriter.NewWriter(builder, 0, 0, 2, ' ', 0)
	for _, entry := range entries {
		fmt.Fprintf(writer, "  %s\t%s\n", entry.Name, entry.Description)
	}
	_ = writer.Flush()
}

func renderDefaults(builder *strings.Builder, defaults []string) {
	for _, line := range defaults {
		fmt.Fprintf(builder, "  %s\n", line)
	}
}

func renderIndentedLines(builder *strings.Builder, indent, text string) {
	for _, line := range strings.Split(text, "\n") {
		if line == "" {
			fmt.Fprintf(builder, "%s\n", indent)
			continue
		}
		fmt.Fprintf(builder, "%s%s\n", indent, line)
	}
}

func formatFlagLabel(item *flag.Flag) string {
	name, _ := flag.UnquoteUsage(item)
	if name == "" {
		return "--" + item.Name
	}
	return fmt.Sprintf("--%s <%s>", item.Name, name)
}

func formatFlagDescription(item *flag.Flag) string {
	_, usage := flag.UnquoteUsage(item)
	if isRepeatableFlag(item) {
		usage += " (repeatable)"
	}
	if showFlagDefault(item.DefValue) {
		return fmt.Sprintf("%s (default %q)", usage, item.DefValue)
	}
	return usage
}

func isRepeatableFlag(item *flag.Flag) bool {
	if item == nil {
		return false
	}
	_, ok := item.Value.(*repeatableStringFlag)
	return ok
}

func showFlagDefault(value string) bool {
	switch value {
	case "", "0", "false":
		return false
	default:
		return true
	}
}

func (c commandCatalog) resolveCompletionContext(context []string, root *commandSpec) *commandSpec {
	spec := root
	for _, value := range context {
		if value == "" {
			break
		}
		next := spec.subcommand(value)
		if next == nil {
			break
		}
		spec = next
	}
	return spec
}

func (c commandCatalog) completeHelpTargets(context []string, current string) []string {
	if len(context) == 0 {
		return filterPrefix(c.helpTargetCandidates(), current)
	}

	if c.root.subcommand(context[0]) == nil {
		return nil
	}

	resolved := c.resolveCompletionContext(context, c.root)
	candidates := completionCandidates(resolved)
	if current == "" {
		return candidates
	}
	return filterPrefix(candidates, current)
}

func (c commandCatalog) helpTargetCandidates() []string {
	candidates := completionCandidates(c.root)
	for _, topic := range c.topics {
		if topic.Hidden {
			continue
		}
		candidates = append(candidates, topic.Name)
		candidates = append(candidates, topic.Aliases...)
	}
	return uniqueSorted(candidates)
}

func (c commandCatalog) completeExplainTargets(context []string, current string) []string {
	if len(context) == 0 {
		return filterPrefix(explainCompletionTargets(), current)
	}
	if len(context) == 1 {
		if family, ok := normalizeExplainFamily(context[0]); ok {
			return filterPrefix(explainFamilyCompletionTargets(family), current)
		}
	}

	return nil
}

func filterPrefix(values []string, prefix string) []string {
	if prefix == "" {
		return values
	}

	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			filtered = append(filtered, value)
		}
	}
	return filtered
}
