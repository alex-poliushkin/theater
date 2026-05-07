//go:build unix

package theater

import (
	"fmt"
	"io"
	"os"
	"syscall"
)

func prepareDebugPromptInput(input io.Reader) (io.Reader, io.Closer, error) {
	file, ok := input.(*os.File)
	if !ok {
		return input, nil, nil
	}

	fd, err := syscall.Dup(int(file.Fd()))
	if err != nil {
		return nil, nil, fmt.Errorf("duplicate interactive debug input: %w", err)
	}

	clone := os.NewFile(uintptr(fd), file.Name())
	if clone == nil {
		_ = syscall.Close(fd)
		return nil, nil, fmt.Errorf("duplicate interactive debug input: %w", os.ErrInvalid)
	}

	return clone, clone, nil
}
