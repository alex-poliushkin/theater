# Theater

[![Tests](https://github.com/alex-poliushkin/theater/actions/workflows/test.yml/badge.svg)](https://github.com/alex-poliushkin/theater/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/alex-poliushkin/theater.svg)](https://pkg.go.dev/github.com/alex-poliushkin/theater)
[![Go Report Card](https://goreportcard.com/badge/github.com/alex-poliushkin/theater)](https://goreportcard.com/report/github.com/alex-poliushkin/theater)
[![Go Version](https://img.shields.io/badge/go-1.25%2B-00ADD8?logo=go)](go.mod)
[![License](https://img.shields.io/github/license/alex-poliushkin/theater)](LICENSE)

Theater runs repeatable API and workflow checks from `.thtr` or YAML files.
Validate a stage before execution, reuse scenarios across flows, and get a run
report that points back to the exact act, action, or expectation that passed or
failed.

It is a verification runner, not an application framework or a general scripting
host. Production behavior stays in the system under test. Theater files describe
the checks another developer or CI job should be able to run again.

## Shape

A Theater file is a stage that calls reusable scenarios. This example logs in
to a local fixture server, reuses the HTTP session, and checks a JSON response
field from the profile endpoint.

<!-- theater-doc: source id=readme-http-flow-thtr kind=thtr path=docs/examples/http-flow/profile.thtr checks=fmt,lower,validate -->
```thtr
stage http-profile

http
  session browser = http.session.browser()

scenario http/profile
  act login
    prop login_url = inventory.env(name: "THEATER_DOC_LOGIN_URL")
    do action.http(method: "GET", url: $login_url, session: "browser")
    expect login-ok: field(status_code) == 200
    on pass -> fetch-profile

  act fetch-profile
    prop profile_url = inventory.env(name: "THEATER_DOC_PROFILE_URL")
    do action.http(method: "GET", url: $profile_url, session: "browser")
    expect status-ok: field(status_code) == 200
    expect profile-id: field(body) | decode(json) | path("/data/id") == "user-123"

call run = http/profile()
```

## Run The HTTP Check

From a repository checkout, validate the HTTP profile example:

<!-- theater-doc: command id=readme-validate-thtr expect-stdout=valid reject-stdout=hint -->
```sh
go run ./cmd/theater validate docs/examples/http-flow/profile.thtr
```

Expected output: `docs/examples/http-flow/profile.thtr: valid`.

Then run it with the local fixture server. The fixture starts a loopback HTTP
server, sets the `THEATER_DOC_*` URLs for the child process, and runs the
Theater command after `--`.

<!-- theater-doc: command id=readme-run-thtr expect-stdout=passed -->
```sh
go run ./docs/examples/http-state/fixture -- go run ./cmd/theater run docs/examples/http-flow/profile.thtr --live off
```

Output starts with
`docs/examples/http-flow/profile.thtr: passed (passed=1 failed=0 canceled=0 skipped=0`;
the `duration` value varies.

## Authoring Surfaces

Theater DSL (`.thtr`) is the compact authoring format. It lowers to the same
runtime stage model that YAML uses; it is not a separate runtime.

YAML is a first-class authoring option and the canonical interchange form. The
YAML twin of the HTTP example is
[docs/examples/http-flow/profile.yaml](docs/examples/http-flow/profile.yaml):

<!-- theater-doc: command id=readme-validate-yaml expect-stdout=valid reject-stdout=hint -->
```sh
go run ./cmd/theater validate docs/examples/http-flow/profile.yaml
```

Use `theater lower` when you want to inspect the YAML produced from a `.thtr`
file.

## Why Use It

- Keep verification flows in readable repository files instead of one-off
  scripts.
- Validate authoring mistakes before running a real flow.
- Reuse scenarios for setup, login, creation, cleanup, or domain-specific
  checks.
- Emit human, JSON, and JUnit run output for local development and CI.
- Capture source-linked expectations and scenario-authored logs in the run
  report.

## Install

Released CLI archives are attached to
[GitHub Releases](https://github.com/alex-poliushkin/theater/releases). Choose
the archive for your OS and CPU:

- `theater_<version>_linux_amd64.tar.gz`
- `theater_<version>_linux_arm64.tar.gz`
- `theater_<version>_darwin_amd64.tar.gz`
- `theater_<version>_darwin_arm64.tar.gz`

Each release also includes `SHA256SUMS`.

macOS assets use the Go `darwin` OS name. After downloading the archive and
`SHA256SUMS`, verify with `sha256sum -c SHA256SUMS`, unpack with
`tar -xzf theater_<version>_<os>_<arch>.tar.gz`, then run `./theater help` or
move `theater` onto your `PATH`.

If Go 1.25 or newer is installed, you can run Theater directly from a source
checkout:

<!-- theater-doc: command id=readme-help expect-stdout="Usage:" expect-stdout-2="validate, check" -->
```sh
go run ./cmd/theater help
```

Build a local binary when you want a reusable executable inside the checkout:
run `go build -o ./bin/theater ./cmd/theater`, then run `./bin/theater help`.

Install from the checkout into `GOBIN`, or into `GOPATH/bin` when `GOBIN` is not
set, with `go install ./cmd/theater`. Make sure that install directory is on
your `PATH`, then run `theater help`.

After the repository is public, the same Go toolchain can install a released
version directly from GitHub with
`go install github.com/alex-poliushkin/theater/cmd/theater@latest`.

JetBrains IDE users can install the released
`jetbrains-thtr-plugin-<version>.zip` through
`Settings | Plugins | Install Plugin from Disk`. The native plugin enables
`.thtr` syntax highlighting, completion, diagnostics, formatting, navigation,
structure view, folding, quick documentation, find usages and rename support.
For compatibility details, use the
[Editor Tooling reference](docs/reference/editor-tooling.md).

## Where To Go Next

| Goal                                   | Start here                                                                                 |
|----------------------------------------|--------------------------------------------------------------------------------------------|
| Follow the guided tutorial path        | [First Run tutorial](docs/tutorial/01-first-run.md)                                        |
| Understand the project model           | [Why Theater](docs/concepts/why-theater.md)                                                |
| Inspect runnable files                 | [Examples](docs/examples/index.md)                                                         |
| Validate and run your own flow         | [Validate and run a flow](docs/how-to/validate-and-run-a-flow.md)                          |
| Look up CLI behavior                   | [CLI reference](docs/reference/cli/index.md)                                               |
| Look up `.thtr` syntax                 | [Theater DSL reference](docs/reference/theater-dsl/index.md)                               |
| Look up YAML syntax                    | [YAML reference](docs/reference/yaml/index.md)                                             |
| Inspect run reports and output formats | [Reports](docs/reference/reports.md) and [output formats](docs/reference/outputs/index.md) |

The full documentation map is in [docs/index.md](docs/index.md).

## License

MIT - see [LICENSE](LICENSE).
