package theatercli

type cliTopicSpec struct {
	Name        string
	Short       string
	Long        string
	Aliases     []string
	Sections    []commandHelpSection
	Environment []environmentHelpEntry
	Defaults    []string
}

func newEnvironmentHelpTopic() *commandSpec {
	return newEnvironmentTopicSpec().helpCommandSpec()
}

func newCompatibilityHelpTopic() *commandSpec {
	return newCompatibilityTopicSpec().helpCommandSpec()
}

func newFormatsHelpTopic() *commandSpec {
	return newFormatsTopicSpec().helpCommandSpec()
}

func formatsExplainTopic() explainTopic {
	return newFormatsTopicSpec().explainTopic()
}

func newEnvironmentTopicSpec() cliTopicSpec {
	return cliTopicSpec{
		Name:        "environment",
		Short:       "Explain supported CLI environment variables.",
		Long:        "Use environment when you need the terminal-visible contract for environment overrides and default resolution.",
		Environment: combineEnvironmentHelp(sharedPluginEnvironmentHelp(), sharedOutputEnvironmentHelp()),
		Defaults:    combineDefaultResolutionHelp(environmentTopicDefaultResolutionHelp(), outputDefaultResolutionHelp()),
		Sections: []commandHelpSection{
			{
				Title: "Scope",
				Lines: []string{
					"THEATER_PLUGINS_CONFIG and THEATER_PLUGINS_LOCK apply to explain, doctor, " +
						"plugins, validate, and run when matching flags are omitted.",
					"Use command flags when you need one-off overrides; use environment variables " +
						"when you want shell-session defaults without introducing a mutable CLI config file.",
				},
			},
			{
				Title: "Output control",
				Lines: []string{
					"THEATER_COLOR accepts auto, always, or never for human-oriented text output.",
					"When THEATER_COLOR is unset, NO_COLOR disables ANSI styling, CLICOLOR_FORCE forces it, and CLICOLOR=0 disables it.",
					"When multiple common color environment variables are set together and THEATER_COLOR is unset, " +
						"precedence is NO_COLOR, then CLICOLOR_FORCE, then CLICOLOR=0.",
					"In auto mode, TERM=dumb disables terminal styling and live frame rendering.",
				},
			},
			{
				Title: "Related",
				Lines: []string{
					"theater doctor  verify the current environment, plugin registry file, plugin lock file, and write destinations before a run",
					"theater plugins doctor  inspect the plugin registry in more detail when plugin-specific checks fail",
				},
			},
		},
	}
}

func newCompatibilityTopicSpec() cliTopicSpec {
	return cliTopicSpec{
		Name:  "compatibility",
		Short: "Explain the current CLI compatibility window.",
		Long: "Use compatibility when you need the current compatibility window: " +
			"older single-dash long options still work, while command help and " +
			"current CLI guidance prefers positional stage paths plus double-dash long options.",
		Aliases: []string{"migration"},
		Sections: []commandHelpSection{
			{
				Title: "Preferred syntax",
				Lines: []string{
					"Use positional stage paths for theater validate and theater run in new commands, docs, and shell history.",
					"Use double-dash long options such as --file, --plugins-config, " +
						"--plugins-lock, --live, and --debug when you need explicit flag spelling.",
					"fmt and lower accept positional .thtr paths; --file remains available when explicit spelling helps readability.",
				},
			},
			{
				Title: "Supported legacy forms",
				Lines: []string{
					"The current compatibility window covers every existing long option " +
						"that the CLI already accepts with a single dash, not only the examples listed here.",
					"Examples include -file, -plugins-config, -plugins-lock, " +
						"-live, and -debug-paths.",
					"Root help remains reachable through theater, theater help, theater -h, and theater --help.",
					"Version remains reachable through theater version, theater -version, and theater --version.",
				},
			},
			{
				Title: "Deprecation behavior",
				Lines: []string{
					"Legacy forms continue to run without runtime deprecation warnings during " +
						"the current compatibility window so existing scripts do not start failing on unexpected stderr output alone.",
					"Command help prefers positional stage paths plus double-dash long options.",
					"This compatibility window does not schedule warning or removal behavior yet. " +
						"A later explicit deprecation cycle can define that policy when needed.",
				},
			},
			{
				Title: "Related",
				Lines: []string{
					"theater help environment  inspect the environment-variable precedence that still applies across both syntax styles",
					"theater help formats  inspect stdout and stderr guarantees before you update automation around CLI output",
				},
			},
		},
	}
}

func newFormatsTopicSpec() cliTopicSpec {
	return cliTopicSpec{
		Name:    "formats",
		Short:   "Explain text, json, and junit output surfaces.",
		Long:    "Use formats when you need the stable stdout and stderr contract around human and machine-readable theater output.",
		Aliases: []string{"format", "output-formats", "output-format"},
		Sections: []commandHelpSection{
			{
				Title: "Output formats",
				Lines: []string{
					"text  default human-readable output for run, validate, debug-path discovery, and scenario discovery",
					"json  machine-readable stdout for run, validate, debug-path discovery, plugins inspect, and scenario discovery",
					"junit  run-only JUnit XML output for CI systems and test-report ingestion",
				},
			},
			{
				Title: "Stdout and stderr",
				Lines: []string{
					"Text summaries stay on stdout.",
					"Live progress, debug prompts, and interactive pause cards stay on stderr so redirected stdout remains machine-safe.",
					"JSON and JUnit keep stdout structured while command-level failures still print on stderr.",
				},
			},
			{
				Title: "Output control",
				Lines: []string{
					"Human-oriented text output may use ANSI styling when THEATER_COLOR permits it or when common color environment conventions allow it.",
					"When THEATER_COLOR is unset and multiple common color environment variables are set together, " +
						"precedence is NO_COLOR, then CLICOLOR_FORCE, then CLICOLOR=0.",
					"JSON and JUnit remain undecorated even when color is enabled for terminal text output.",
					"In auto mode, TERM=dumb disables terminal styling and live frame rendering.",
				},
			},
			{
				Title: "Related",
				Lines: []string{
					"theater run " + stageFileArgument + " --format json  emit one machine-readable run document",
					"theater help exit-codes  inspect the process-level success and failure contract around those formats",
				},
			},
		},
	}
}

func (s cliTopicSpec) helpCommandSpec() *commandSpec {
	return &commandSpec{
		Name:        s.Name,
		Path:        "theater help " + s.Name,
		Short:       s.Short,
		Long:        s.Long,
		Aliases:     s.Aliases,
		HelpTopic:   true,
		Sections:    s.Sections,
		Environment: s.Environment,
		Defaults:    s.Defaults,
	}
}

func (s cliTopicSpec) explainTopic() explainTopic {
	return explainTopic{
		Name:     s.Name,
		Short:    s.Short,
		Aliases:  s.Aliases,
		Sections: s.Sections,
	}
}
