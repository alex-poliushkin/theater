# Editor Tooling Reference

Theater ships `.thtr` editor-tooling APIs, a stdio LSP server and a native
JetBrains plugin. The LSP server and native JetBrains plugin are separate editor
surfaces.

Source of truth:

- `thtr/tooling.go`
- `cmd/thtr-lsp`
- `internal/thtrlsp`
- `tools/jetbrains-thtr-plugin`

## Checked Tooling Build

<!-- theater-doc: command id=reference-editor-tooling-tests cwd=../.. expect-stdout=github.com/alex-poliushkin/theater/thtr expect-stdout-2=github.com/alex-poliushkin/theater/internal/thtrlsp -->
```sh
go test ./thtr ./internal/thtrlsp -count=1
```

## Shipped Pieces

| Surface | Contract |
| --- | --- |
| `thtr.Analyze` | Lower `.thtr` bytes into `StageSpec`, canonical YAML, and source-map data |
| `thtr.AnalyzeFile` | Analyze one `.thtr` file from disk |
| `thtr.Format` / `thtr.FormatFile` | Format `.thtr` source without writing unless caller writes the result |
| `thtr.Tokenize` | Return stable lexer token categories with source coordinates |
| `thtr.Analysis.RewriteDiagnostics` | Map validation diagnostics back to `.thtr` source spans when source-map entries exist |
| `cmd/thtr-lsp` | Stdio LSP server for editor integrations |
| `tools/jetbrains-thtr-plugin` | Native JetBrains plugin for `.thtr` files |

## LSP Capabilities

The current server handles initialize/shutdown, full document sync,
diagnostics, completion, hover, signature help, formatting, and semantic tokens.
Completion and diagnostics cover the current `.thtr` property value surface,
including `coalesce(...)`, `env("NAME")`, generator bindings and descriptor-backed
inventory calls.

Plugin-aware completion and diagnostics require readable plugin registry and
lock file paths. Editor integrations that start `thtr-lsp` pass those paths as
`THEATER_PLUGINS_CONFIG` and `THEATER_PLUGINS_LOCK`.

## Native JetBrains Plugin

The JetBrains plugin registers `.thtr` files and keeps native project settings
for Theater plugin registry and lock file paths. It is wired for the IntelliJ
Platform Language API and Grammar-Kit generated parser/PSI sources. It includes
the native lexer, parser definition, PSI file and accepted syntax-family PSI
anchors for the PSI MVP. It also provides native syntax highlighting, a color
settings page and PSI-backed semantic highlighting for declarations, data keys,
local refs and capability refs where the current parser exposes reliable
context. Supported call targets, transition targets and `$ref` values have
native PSI references; repo-local scenario calls can resolve declarations under
`theater/lib/` through the project file index when indexes are available.
Supported unresolved references produce stable editor diagnostics. Native
completion covers syntax-valid structural keywords, including act-local
`log <id> = <log-value>` statements, local symbols, repo-local scenarios,
built-in capability refs, descriptor-backed plugin refs from project plugin
manifests, property value helpers such as `coalesce(...)` and `env("NAME")`,
and Theater DSL log-value roots such as `field(...)`, `$ref`, `object { ... }`
and `list [ ... ]`, plus selector steps after log-value pipelines. Quick
documentation is available for supported built-in and descriptor-backed
capability refs. Native static diagnostics cover
malformed syntax, supported unresolved references, invalid selector call shapes,
invalid `coalesce(...)` and `env("NAME")` property value shapes, unknown
capability refs and missing required capability arguments. A native
inspection reports removed `state.cas` syntax and offers the bounded mechanical
replacement to `state.update`. Native formatter support is registered through
the IntelliJ formatter extension point and a post-format normalization pass for
the line-oriented `.thtr` layout. The native plugin also registers 2-space code
style defaults and the `#` line commenter. Supported references power native
declaration navigation targets. The plugin also registers find-usages reference
search, `.thtr` name validation, local declaration/reference rename handling and
a veto for repo-library scenario rename scopes that are not bounded to one file.
Native structure view shows stages, scenarios, calls and scenario acts. Folding
regions cover scenario, act, object/list and large call-data blocks. Quick
documentation is available for supported built-in and descriptor-backed
capability refs, for stage/scenario/act/call/prop/log/export/capture_auth
declarations and, where references resolve through native PSI, for scenario
calls, act transitions and `$ref` value references.

## Native JetBrains Packaging And Compatibility

The native plugin is packaged with the IntelliJ Platform Gradle Plugin 2.x. The
plugin descriptor declares only `com.intellij.modules.platform` and
`com.intellij.modules.lang`, so the descriptor-level product policy is generic
IntelliJ Platform language support rather than a GoLand-only plugin. It must not
declare `com.intellij.modules.lsp`, `com.intellij.modules.ultimate`,
`com.intellij.modules.goland` or `org.jetbrains.plugins.go` for ordinary native
language support.

The current baseline is IntelliJ Platform build `252` and local GoLand
`2025.2.4`. The package does not set an `until-build` clamp, so newer compatible
IDE builds are not blocked by descriptor metadata. This is JetBrains open-ended
compatibility metadata, not a guarantee that every future IDE build remains
compatible; new target branches still need Plugin Verifier coverage before they
are treated as verified. The local Plugin Verifier matrix runs against GoLand
`2025.2.4` and IntelliJ IDEA Community `2025.2.4`. GoLand remains the local
development IDE baseline for this repository, while IntelliJ IDEA Community
verification catches accidental use of GoLand-only APIs.

When native registry settings are configured, descriptor metadata is loaded from
the registry config, plugin manifests and optional lock file. The native plugin
uses manifest checksums from the lock file but does not resolve
`exec.command`, launch plugin binaries or start validate/run sessions during
editor analysis. With empty registry settings, project `plugins/**/manifest.json`
files remain the descriptor discovery source.

The native plugin does not declare the JetBrains LSP module dependency and does
not start `thtr-lsp` for ordinary language support. Full validation-diagnostic
parity, broader navigation polish, JetBrains Marketplace publication and signed
Marketplace distribution are not shipped yet.

## Native JetBrains Build And Install

Released native plugin archives are attached to
[GitHub Releases](https://github.com/alex-poliushkin/theater/releases) as
`jetbrains-thtr-plugin-<version>.zip` files. The release workflow checks that
the plugin version matches the Git tag before publishing the ZIP. Download the
ZIP for the release you want, then install it in a JetBrains IDE through
`Settings | Plugins | Install Plugin from Disk`. Restart the IDE if prompted.

Use the native plugin when you want `.thtr` syntax highlighting, completion,
diagnostics, formatting, navigation, structure view, folding, quick
documentation, find usages and rename support.

Run the native plugin verification gate from the repository root with
`gradle -p tools/jetbrains-thtr-plugin nativePluginCheck`.

This runs native plugin tests, builds the plugin distribution, validates the
plugin project and archive structure, and runs Plugin Verifier against the
declared IDE matrix.

To build only the local plugin archive, run
`gradle -p tools/jetbrains-thtr-plugin buildPlugin`.

The local installable archive is written under
`tools/jetbrains-thtr-plugin/build/distributions/jetbrains-thtr-plugin-<version>.zip`.

Install the archive from disk in a JetBrains IDE through Settings | Plugins |
Install Plugin from Disk. Restart the IDE if prompted.

For development, run the configured sandbox IDE with
`gradle -p tools/jetbrains-thtr-plugin runIde`.

The development sandbox uses the configured GoLand `2025.2.4` baseline.

## Migration Notes

The stdio `thtr-lsp` server is retained for LSP-based editor integrations. It is
not an implementation dependency of the native JetBrains plugin.

For JetBrains IDEs, use the native plugin when you want IntelliJ Platform
language features such as PSI-backed navigation, completion, formatting,
structure view and folding. A JetBrains IDE does not need a `thtr-lsp` binary or
server path for ordinary native `.thtr` language support.

For existing local JetBrains installations built from the older LSP-backed
scaffold, install the current native plugin archive over the old build. If the
IDE still starts the old LSP integration, remove the old local plugin from the
Installed plugins page, restart the IDE and install the current archive from
disk.

Plugin registry and lock paths moved from LSP launch handoff to native project
settings. Configure them in the Theater project settings only when
descriptor-backed plugin completion, documentation or diagnostics are needed.

When changing the JetBrains plugin, run the Gradle `nativePluginCheck` task from
`tools/jetbrains-thtr-plugin`. That task runs plugin tests, builds the plugin
distribution, validates the plugin project and archive structure, and runs the
Plugin Verifier matrix.

## Setup Assumptions

The repository does not install an IDE plugin into a user's IDE automatically.
An LSP-based editor integration must point to a built `thtr-lsp` binary and,
when plugin descriptors are needed, to the project plugin registry and lock
files. The JetBrains plugin is built separately as an IDE plugin artifact.

For a local verification task, use
[Check Editor Tooling](../how-to/check-editor-tooling.md).
