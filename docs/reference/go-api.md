# Go API Reference

The public Go API is for embedding Theater loading, authoring analysis, plugin
contracts, and report handling in Go tools.

Source of truth:

- `theater` root package
- `thtr/`
- `yaml/`
- `report/`
- `junit/`
- `builtin/`
- `spec/`
- `state/`
- `observe/`
- `plugin/manifest/`
- `plugin/registry/`
- `plugin/protocol/`
- `plugin/sdk/`

## Checked Go Example

<!-- theater-doc: source id=reference-go-embedding-example kind=go path=../examples/go-embedding/example_test.go -->
```go
package goembedding_test

import (
	"fmt"

	"github.com/alex-poliushkin/theater/thtr"
	theateryaml "github.com/alex-poliushkin/theater/yaml"
)

func Example_analyzeTheaterDSL() {
	analysis, err := thtr.Analyze([]byte(`stage embedded-thtr

scenario hello
  act say
    do action.generate
      outputs:
        message: "hello"

call run = hello()
`), thtr.AnalyzeOptions{})
	if err != nil {
		panic(err)
	}

	fmt.Println(analysis.Spec.ID)
	fmt.Println(len(analysis.CanonicalYAML) > 0)
	fmt.Println(analysis.SourceMap.Version)

	// Output:
	// embedded-thtr
	// true
	// v1alpha1
}

func Example_parseYAMLStage() {
	spec, err := theateryaml.Parse([]byte(`id: embedded-yaml
scenarios:
  - id: hello
    acts:
      - id: say
        action:
          use: action.generate
          with:
            outputs:
              message: hello
scenario_calls:
  - id: run
    scenario_id: hello
`), nil)
	if err != nil {
		panic(err)
	}

	fmt.Println(spec.ID)
	fmt.Println(len(spec.Scenarios))
	fmt.Println(len(spec.ScenarioCalls))

	// Output:
	// embedded-yaml
	// 1
	// 1
}
```

<!-- theater-doc: command id=reference-go-embedding-test cwd=../.. expect-stdout=github.com/alex-poliushkin/theater/docs/examples/go-embedding -->
```sh
go test ./docs/examples/go-embedding
```

## Checked Runtime Examples

The root package and small public packages carry runnable Go examples for
runtime embedding surfaces that are easier to understand in package tests than
inside this lookup page.

<!-- theater-doc: command id=reference-go-runtime-examples cwd=../.. expect-stdout="=== RUN   ExampleRunOptions" expect-stdout-2="=== RUN   ExampleValidator_ListDebugPaths" expect-stdout-3="=== RUN   ExampleNewPluginCatalog" -->
```sh
go test . -run Example -v
```

<!-- theater-doc: command id=reference-go-builtin-example cwd=../.. expect-stdout="=== RUN   ExampleNewBundle" -->
```sh
go test ./builtin -run ExampleNewBundle -v
```

<!-- theater-doc: command id=reference-go-thtr-flow-example cwd=../.. expect-stdout="=== RUN   ExampleLoadFlowFile" -->
```sh
go test ./thtr -run ExampleLoadFlowFile -v
```

## Core Go Surfaces

| Surface | Where to look |
| --- | --- |
| `RunOptions.Events` | `ExampleRunOptions` in the root package records raw events during a run |
| `RunOptions.Live` | `ExampleRunOptions` in the root package publishes live observation envelopes |
| `NewProjector` | `ExampleRunOptions` projects recorded events into a run document |
| `Validator.ListDebugPaths` | `ExampleValidator_ListDebugPaths` lists debuggable runtime boundaries |
| `NewPluginCatalog` | `ExampleNewPluginCatalog` overlays plugin descriptors on a built-in catalog |
| `builtin.NewBundle` | `ExampleNewBundle` constructs the default catalog and matcher pair |
| `thtr.LoadFlowFile` | `ExampleLoadFlowFile` loads a repo-aware `.thtr` flow with linked library files |
| `yaml.LoadFlowFile` | YAML equivalent for repo-aware flow loading; use the same `theater/flows` and `theater/lib` layout |

## Primary Embedding Packages

| Package | Use |
| --- | --- |
| `github.com/alex-poliushkin/theater` | Runtime contracts, stage specs, diagnostics, reports, and runner APIs |
| `github.com/alex-poliushkin/theater/builtin` | Default built-in catalog bundle for host applications |
| `github.com/alex-poliushkin/theater/thtr` | `.thtr` parse/load/format/analyze APIs |
| `github.com/alex-poliushkin/theater/yaml` | YAML parse/load APIs |
| `github.com/alex-poliushkin/theater/report` | Public run document and report model |
| `github.com/alex-poliushkin/theater/junit` | JUnit XML export from Theater run documents |
| `github.com/alex-poliushkin/theater/plugin/manifest` | Plugin manifest schema and descriptor digest helpers |
| `github.com/alex-poliushkin/theater/plugin/registry` | Plugin registry config and lock file helpers |
| `github.com/alex-poliushkin/theater/plugin/protocol` | Native plugin JSON-RPC protocol envelopes |
| `github.com/alex-poliushkin/theater/plugin/sdk` | Helpers for Go-native plugin implementations |

## Additional Public Packages

| Package | Use |
| --- | --- |
| `github.com/alex-poliushkin/theater/spec` | Canonical authoring, contract, matcher, and reference data model |
| `github.com/alex-poliushkin/theater/state` | Persistent-state data model and backend interface |
| `github.com/alex-poliushkin/theater/observe` | Live execution envelopes and sink interfaces |
| `github.com/alex-poliushkin/theater/builtin/action` | Fine-grained built-in action registration |
| `github.com/alex-poliushkin/theater/builtin/inventory` | Fine-grained built-in inventory registration |
| `github.com/alex-poliushkin/theater/builtin/decorator` | Fine-grained built-in decorator registration |
| `github.com/alex-poliushkin/theater/builtin/expectation` | Built-in matcher descriptors |
| `github.com/alex-poliushkin/theater/builtin/generator` | Fine-grained built-in generator registration |
| `github.com/alex-poliushkin/theater/builtin/statebackend` | Fine-grained built-in state-backend registration |

For task flow, use [Embed Theater In Go](../how-to/embed-theater-in-go.md).
