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
