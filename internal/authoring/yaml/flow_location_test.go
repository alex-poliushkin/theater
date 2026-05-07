package yaml

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveFlowFileLocationClassifiesResolvedSymlinkTarget(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	outsideRoot := t.TempDir()

	flowDir := filepath.Join(repoRoot, flowRepoRootName, flowStageRootName, "auth")
	libraryDir := filepath.Join(repoRoot, flowRepoRootName, flowLibraryRootName)
	if err := os.MkdirAll(flowDir, 0o755); err != nil {
		t.Fatalf("mkdir flow dir failed: %v", err)
	}
	if err := os.MkdirAll(libraryDir, 0o755); err != nil {
		t.Fatalf("mkdir library dir failed: %v", err)
	}

	outsidePath := filepath.Join(outsideRoot, "external.yaml")
	if err := os.WriteFile(outsidePath, []byte("id: outside\n"), 0o600); err != nil {
		t.Fatalf("write outside file failed: %v", err)
	}

	linkPath := filepath.Join(flowDir, "external.yaml")
	if err := os.Symlink(outsidePath, linkPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	location, err := ResolveFlowFileLocation(linkPath)
	if err != nil {
		t.Fatalf("resolve flow file location failed: %v", err)
	}

	if got, want := location.Path, outsidePath; got != want {
		t.Fatalf("resolved path mismatch: got %q want %q", got, want)
	}
	if location.RepoFound {
		t.Fatalf("external symlink target must not inherit repo-aware resolution: %+v", location)
	}
	if location.InFlowRoot {
		t.Fatalf("external symlink target must not be classified inside theater/flows: %+v", location)
	}
}
