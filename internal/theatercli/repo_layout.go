package theatercli

import (
	"os"
	"path/filepath"
)

const (
	repoLayoutFlowRootName    = "flows"
	repoLayoutLibraryRootName = "lib"
	repoLayoutRootName        = "theater"
)

type repoLayout struct {
	FlowRoot    string
	LibraryRoot string
	RepoRoot    string
}

func resolveRepoLayout(start string) (repoLayout, bool) {
	current := filepath.Clean(start)
	for {
		flowRoot := filepath.Join(current, repoLayoutRootName, repoLayoutFlowRootName)
		libraryRoot := filepath.Join(current, repoLayoutRootName, repoLayoutLibraryRootName)
		if isRepoLayoutDirectory(flowRoot) && isRepoLayoutDirectory(libraryRoot) {
			return repoLayout{
				RepoRoot:    current,
				FlowRoot:    flowRoot,
				LibraryRoot: libraryRoot,
			}, true
		}

		parent := filepath.Dir(current)
		if parent == current {
			return repoLayout{}, false
		}
		current = parent
	}
}

func isRepoLayoutDirectory(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return info.IsDir()
}
