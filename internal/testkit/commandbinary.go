package testkit

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

var (
	commandHelperOnce sync.Once
	commandHelperPath string
	errCommandHelper  error
)

func BuildCommandHelper(t testing.TB) string {
	t.Helper()

	commandHelperOnce.Do(func() {
		commandHelperPath, errCommandHelper = buildCommandHelper()
	})
	if errCommandHelper != nil {
		t.Fatalf("build command helper failed: %v", errCommandHelper)
	}

	return commandHelperPath
}

func buildCommandHelper() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", os.ErrNotExist
	}

	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	tempDir, err := os.MkdirTemp("", "theater-commandhelper-*")
	if err != nil {
		return "", err
	}

	binaryName := "commandhelper"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}

	binaryPath := filepath.Join(tempDir, binaryName)
	cmd := exec.Command("go", "build", "-o", binaryPath, "./internal/testkit/cmd/commandhelper")
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", &helperBuildError{cause: err, output: string(output)}
	}

	return binaryPath, nil
}

type helperBuildError struct {
	cause  error
	output string
}

func (e *helperBuildError) Error() string {
	if e.output == "" {
		return e.cause.Error()
	}

	return e.cause.Error() + ": " + e.output
}

func (e *helperBuildError) Unwrap() error {
	return e.cause
}
