package thtr_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/thtr"
)

func ExampleLoadFlowFile() {
	repoRoot, err := os.MkdirTemp("", "theater-thtr-example-*")
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = os.RemoveAll(repoRoot)
	}()

	flowPath := writeExampleTHTRFile(repoRoot, filepath.Join("theater", "flows", "auth", "smoke.thtr"), `stage smoke

call login-user = auth/login()
`)
	writeExampleTHTRFile(repoRoot, filepath.Join("theater", "lib", "auth", "login.thtr"), `stage auth-lib

scenario auth/login
  act submit
    do action.http(method: "GET", url: "/health")
`)

	spec, err := thtr.LoadFlowFile(flowPath, nil)
	if err != nil {
		panic(err)
	}

	fmt.Println(spec.ID)
	fmt.Println(exampleTHTRHasScenario(spec, "auth/login"))

	// Output:
	// smoke
	// true
}

func writeExampleTHTRFile(repoRoot, relativePath, contents string) string {
	path := filepath.Join(repoRoot, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		panic(err)
	}

	return path
}

func exampleTHTRHasScenario(spec theater.StageSpec, id string) bool {
	for _, scenario := range spec.Scenarios {
		if scenario.ID == id {
			return true
		}
	}

	return false
}
