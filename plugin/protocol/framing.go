package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const contentLengthHeader = "Content-Length"

// ReadFrame reads one LSP-style Content-Length framed JSON payload.
func ReadFrame(r *bufio.Reader) ([]byte, error) {
	if r == nil {
		return nil, errors.New("frame reader is required")
	}

	length := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}

		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("malformed frame header %q", line)
		}

		if strings.EqualFold(strings.TrimSpace(name), contentLengthHeader) {
			parsed, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return nil, fmt.Errorf("parse %s: %w", contentLengthHeader, err)
			}
			if parsed < 0 {
				return nil, fmt.Errorf("%s must not be negative", contentLengthHeader)
			}

			length = parsed
		}
	}

	if length < 0 {
		return nil, fmt.Errorf("%s header is required", contentLengthHeader)
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}

	return payload, nil
}

// WriteFrame writes one JSON payload with LSP-style Content-Length framing.
func WriteFrame(w io.Writer, payload any) error {
	if w == nil {
		return errors.New("frame writer is required")
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode JSON payload: %w", err)
	}

	var buf bytes.Buffer
	if _, err := fmt.Fprintf(&buf, "%s: %d\r\n\r\n", contentLengthHeader, len(raw)); err != nil {
		return fmt.Errorf("encode frame header: %w", err)
	}
	if _, err := buf.Write(raw); err != nil {
		return fmt.Errorf("encode frame payload: %w", err)
	}

	if _, err := w.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}

	return nil
}
