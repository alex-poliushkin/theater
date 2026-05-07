package thtr

import "fmt"

func errFlowRepoNotFound(path string) error {
	return fmt.Errorf("repo-local theater roots not found for flow file %s", path)
}

func errFlowOutsideRoot(path, root string) error {
	return fmt.Errorf("flow file %s must be located under %s", path, root)
}
