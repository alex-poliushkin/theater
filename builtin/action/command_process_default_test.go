//go:build !unix

package action

import (
	"os/exec"
	"testing"
)

func TestConfigureCommandProcessLeavesDefaultCancellationOnUnsupportedPlatforms(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("command-helper-placeholder")
	configureCommandProcess(cmd)

	if cmd.SysProcAttr != nil {
		t.Fatalf("expected no custom process attributes, got %#v", cmd.SysProcAttr)
	}
	if cmd.Cancel != nil {
		t.Fatal("expected unsupported-platform command cancellation to remain default best-effort behavior")
	}
}
