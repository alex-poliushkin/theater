//go:build !unix

package theater

import (
	"errors"
	"io"
)

func prepareDebugPromptInput(input io.Reader) (io.Reader, io.Closer, error) {
	return nil, nil, errors.New("interactive debug is not supported on this platform")
}
