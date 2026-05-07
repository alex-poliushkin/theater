package theatercli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCommandWritesDefaultYAMLStarterAndLayout(t *testing.T) {
	repoRoot := t.TempDir()
	restore := chdirForTest(t, repoRoot)
	defer restore()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandInit}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}

	stagePath := filepath.Join(repoRoot, "theater", "flows", "http", "starter.yaml")
	stageData, err := os.ReadFile(stagePath)
	if err != nil {
		t.Fatalf("read starter stage: %v", err)
	}
	stage := string(stageData)
	for _, want := range []string{
		"id: starter",
		"https://example.com",
		"session: none",
		"contains: Example Domain",
	} {
		if !strings.Contains(stage, want) {
			t.Fatalf("starter stage missing %q: %q", want, stage)
		}
	}
	requireHTTPActionsUseSessionNone(t, requireValidStageFile(t, stagePath))

	libPath := filepath.Join(repoRoot, "theater", "lib")
	info, err := os.Stat(libPath)
	if err != nil {
		t.Fatalf("stat lib root: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("lib root must be a directory: %s", libPath)
	}

	for _, want := range []string{
		"wrote theater/flows/http/starter.yaml",
		"prepared theater/lib",
		"theater validate theater/flows/http/starter.yaml",
		"theater run theater/flows/http/starter.yaml --live off",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q: %q", want, stdout.String())
		}
	}
}

func TestInitCommandWritesTHTRStarterWhenRequested(t *testing.T) {
	repoRoot := t.TempDir()
	restore := chdirForTest(t, repoRoot)
	defer restore()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandInit, "--syntax", "thtr"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}

	stagePath := filepath.Join(repoRoot, "theater", "flows", "http", "starter.thtr")
	stageData, err := os.ReadFile(stagePath)
	if err != nil {
		t.Fatalf("read thtr starter stage: %v", err)
	}
	stage := string(stageData)
	for _, want := range []string{
		"stage starter",
		`do action.http(method: "GET", url: "https://example.com", session: "none")`,
		`expect page-text: field(body) matches r"Example Domain"`,
	} {
		if !strings.Contains(stage, want) {
			t.Fatalf("starter thtr missing %q: %q", want, stage)
		}
	}
	requireHTTPActionsUseSessionNone(t, requireValidStageFile(t, stagePath))

	if !strings.Contains(stdout.String(), "wrote theater/flows/http/starter.thtr") {
		t.Fatalf("stdout missing thtr target: %q", stdout.String())
	}
}

func TestInitCommandRejectsExistingTarget(t *testing.T) {
	repoRoot := t.TempDir()
	restore := chdirForTest(t, repoRoot)
	defer restore()

	stagePath := filepath.Join(repoRoot, "theater", "flows", "http", "starter.yaml")
	if err := os.MkdirAll(filepath.Dir(stagePath), 0o755); err != nil {
		t.Fatalf("mkdir starter dir: %v", err)
	}
	if err := os.WriteFile(stagePath, []byte("existing"), 0o600); err != nil {
		t.Fatalf("write existing starter: %v", err)
	}

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandInit}, &stdout, &stderr)
	if got, want := code, exitCodeCommandError; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q", got, want, stderr.String())
	}
	if !strings.Contains(stderr.String(), "init target already exists") {
		t.Fatalf("stderr missing existing target error: %q", stderr.String())
	}
}

func TestInitCommandRejectsTargetsOutsideTheaterFlows(t *testing.T) {
	repoRoot := t.TempDir()
	restore := chdirForTest(t, repoRoot)
	defer restore()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandInit, "starter.yaml"}, &stdout, &stderr)
	if got, want := code, exitCodeCommandError; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q", got, want, stderr.String())
	}
	if !strings.Contains(stderr.String(), "init target must stay under theater/flows/") {
		t.Fatalf("stderr missing repo-layout guidance: %q", stderr.String())
	}
}

func TestInitCommandRejectsTargetExtensionMismatch(t *testing.T) {
	repoRoot := t.TempDir()
	restore := chdirForTest(t, repoRoot)
	defer restore()

	tests := []struct {
		args []string
	}{
		{args: []string{commandInit, "theater/flows/http/starter.thtr", "--syntax", "yaml"}},
		{args: []string{commandInit, "theater/flows/http/starter.yaml", "--syntax", "thtr"}},
	}

	for _, test := range tests {
		var stdout strings.Builder
		var stderr strings.Builder

		code := run(test.args, &stdout, &stderr)
		if got, want := code, exitCodeCommandError; got != want {
			t.Fatalf("exit code mismatch for %v: got %d want %d stderr=%q", test.args, got, want, stderr.String())
		}
		if !strings.Contains(stderr.String(), "init target extension") {
			t.Fatalf("stderr missing extension mismatch for %v: %q", test.args, stderr.String())
		}
	}
}

func TestInitCommandAcceptsAbsoluteTargetWithinWorkspace(t *testing.T) {
	repoRoot := t.TempDir()
	restore := chdirForTest(t, repoRoot)
	defer restore()

	absoluteTarget := filepath.Join(repoRoot, "theater", "flows", "http", "custom.yaml")

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandInit, absoluteTarget}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "wrote theater/flows/http/custom.yaml") {
		t.Fatalf("stdout missing normalized absolute target: %q", stdout.String())
	}
	if _, err := os.Stat(absoluteTarget); err != nil {
		t.Fatalf("stat absolute target: %v", err)
	}
}

func TestInitCommandAcceptsSyntaxFlagAfterTarget(t *testing.T) {
	repoRoot := t.TempDir()
	restore := chdirForTest(t, repoRoot)
	defer restore()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandInit, "theater/flows/http/custom.thtr", "--syntax", "thtr"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "wrote theater/flows/http/custom.thtr") {
		t.Fatalf("stdout missing target written by after-target syntax flag: %q", stdout.String())
	}
}

func TestInitCommandUsesExistingRepoRootFromNestedDirectory(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "theater", "flows"), 0o755); err != nil {
		t.Fatalf("mkdir flows root: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, "theater", "lib"), 0o755); err != nil {
		t.Fatalf("mkdir lib root: %v", err)
	}

	nestedDir := filepath.Join(repoRoot, "nested", "deep")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("mkdir nested dir: %v", err)
	}

	restore := chdirForTest(t, nestedDir)
	defer restore()

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandInit}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}

	repoStagePath := filepath.Join(repoRoot, "theater", "flows", "http", "starter.yaml")
	if _, err := os.Stat(repoStagePath); err != nil {
		t.Fatalf("stat repo-root stage: %v", err)
	}

	nestedStagePath := filepath.Join(nestedDir, "theater", "flows", "http", "starter.yaml")
	if _, err := os.Stat(nestedStagePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("nested working directory must not get its own theater tree, got err=%v", err)
	}
}

func TestInitCommandRejectsFlowsSymlinkOutsideWorkspace(t *testing.T) {
	repoRoot := t.TempDir()
	outsideRoot := t.TempDir()
	restore := chdirForTest(t, repoRoot)
	defer restore()

	theaterRoot := filepath.Join(repoRoot, "theater")
	if err := os.MkdirAll(theaterRoot, 0o755); err != nil {
		t.Fatalf("mkdir theater root: %v", err)
	}
	if err := os.Symlink(outsideRoot, filepath.Join(theaterRoot, "flows")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandInit}, &stdout, &stderr)
	if got, want := code, exitCodeCommandError; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q", got, want, stderr.String())
	}
	if !strings.Contains(stderr.String(), "prepare init target directory") {
		t.Fatalf("stderr missing symlink traversal guard: %q", stderr.String())
	}

	outsideStagePath := filepath.Join(outsideRoot, "http", "starter.yaml")
	if _, err := os.Stat(outsideStagePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside workspace must stay untouched, got err=%v", err)
	}
}
