//go:build unix

package action

import (
	"errors"
	"os"
	"os/exec"
	"testing"
)

func TestConfigureCommandProcessInstallsUnixProcessGroupCancellation(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("command-helper-placeholder")
	configureCommandProcess(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("expected unix process attributes to be configured")
	}
	if !cmd.SysProcAttr.Setpgid {
		t.Fatal("expected spawned command to run in its own process group")
	}
	if cmd.Cancel == nil {
		t.Fatal("expected unix command cancellation hook")
	}
	if err := cmd.Cancel(); !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("cancel before start mismatch: got %v want %v", err, os.ErrProcessDone)
	}
}
