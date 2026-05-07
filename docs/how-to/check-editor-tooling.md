# Check Editor Tooling

Use this when changing `.thtr` tooling or verifying that the public editor APIs,
the LSP package or the native JetBrains plugin still compile and pass their
checks.

Run the focused package check:

<!-- theater-doc: command id=howto-editor-tooling-tests cwd=../.. expect-stdout=github.com/alex-poliushkin/theater/thtr expect-stdout-2=github.com/alex-poliushkin/theater/internal/thtrlsp -->
```sh
go test ./thtr ./internal/thtrlsp -count=1
```

This checks the public `thtr` package and the stdio LSP implementation. It does
not install any IDE plugin.

Run the native JetBrains plugin gate with
`gradle -p tools/jetbrains-thtr-plugin nativePluginCheck`.

That task is separate from the Go package check. It runs plugin tests, builds
the plugin distribution, validates the plugin project/archive structure and runs
Plugin Verifier for the declared IDE matrix.

Build the installable local archive with
`gradle -p tools/jetbrains-thtr-plugin buildPlugin` when you only need the ZIP.

The installable archive is written to
`tools/jetbrains-thtr-plugin/build/distributions/jetbrains-thtr-plugin-<version>.zip`.

Install it in a JetBrains IDE from Settings | Plugins | Install Plugin from
Disk. Restart the IDE if prompted.

Run the sandbox IDE for development checks with
`gradle -p tools/jetbrains-thtr-plugin runIde`.

The native JetBrains plugin does not start `thtr-lsp` for ordinary `.thtr`
language support. Keep using `thtr-lsp` for LSP-based editor integrations.

For exact contracts, open [Editor Tooling](../reference/editor-tooling.md).
