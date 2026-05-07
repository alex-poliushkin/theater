//go:build !unix

package action

import "os/exec"

func configureCommandProcess(cmd *exec.Cmd) {}
