package yaml

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type FlowFileLocation struct {
	// Path is the canonical file path that will be loaded after resolving
	// symlinks.
	Path       string
	Layout     FlowRepoLayout
	RepoFound  bool
	InFlowRoot bool
}

type FlowRepoLayout struct {
	FlowRoot    string
	LibraryRoot string
}

func ResolveFlowFileLocation(path string) (FlowFileLocation, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return FlowFileLocation{}, err
	}

	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return FlowFileLocation{}, err
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		return FlowFileLocation{}, err
	}
	if info.IsDir() {
		return FlowFileLocation{}, fmt.Errorf("flow file must be a file: %s", resolvedPath)
	}

	location := FlowFileLocation{Path: resolvedPath}

	repoRoot, ok := findFlowRepoRoot(resolvedPath)
	if !ok {
		return location, nil
	}

	location.RepoFound = true
	location.Layout = FlowRepoLayout{
		FlowRoot:    filepath.Join(repoRoot, flowRepoRootName, flowStageRootName),
		LibraryRoot: filepath.Join(repoRoot, flowRepoRootName, flowLibraryRootName),
	}
	location.InFlowRoot = pathWithin(resolvedPath, location.Layout.FlowRoot)
	return location, nil
}

func findFlowRepoRoot(path string) (string, bool) {
	current := filepath.Dir(path)
	for {
		flowRoot := filepath.Join(current, flowRepoRootName, flowStageRootName)
		libraryRoot := filepath.Join(current, flowRepoRootName, flowLibraryRootName)
		if isDirectory(flowRoot) && isDirectory(libraryRoot) {
			return current, true
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}

		current = parent
	}
}

func pathWithin(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}

	return relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return info.IsDir()
}
