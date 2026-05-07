package theater

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type runtimePathCodec struct{}

func (runtimePathCodec) Join(kind, id string) string {
	return kind + "." + escapeRuntimePathID(id)
}

func (runtimePathCodec) JoinChild(parentPath, kind, id string) string {
	return parentPath + "/" + kind + "." + escapeRuntimePathID(id)
}

func (c runtimePathCodec) DecodeID(encoded string) (string, error) {
	return decodeRuntimePathID(encoded)
}

func (c runtimePathCodec) SplitSegment(segment string) (kind, id string, err error) {
	if segment == string(NodeKindAction) {
		return string(NodeKindAction), "", nil
	}

	var encodedID string
	var ok bool
	kind, encodedID, ok = strings.Cut(segment, ".")
	if !ok || kind == "" {
		return "", "", fmt.Errorf("runtime path segment %q is invalid", segment)
	}

	id, err = c.DecodeID(encodedID)
	if err != nil {
		return "", "", fmt.Errorf("runtime path segment %q is invalid: %w", segment, err)
	}

	return kind, id, nil
}

func escapeRuntimePathID(id string) string {
	var builder strings.Builder
	builder.Grow(len(id))

	for i := 0; i < len(id); i++ {
		b := id[i]
		switch {
		case b == '~':
			builder.WriteString("~0")
		case b == '/':
			builder.WriteString("~1")
		case b == '.':
			builder.WriteString("~2")
		case b < 0x20 || b == 0x7f:
			builder.WriteString(fmt.Sprintf("~x%02X", b))
		default:
			builder.WriteByte(b)
		}
	}

	return builder.String()
}

func decodeRuntimePathID(encoded string) (string, error) {
	var builder strings.Builder
	builder.Grow(len(encoded))

	for i := 0; i < len(encoded); i++ {
		if encoded[i] != '~' {
			builder.WriteByte(encoded[i])
			continue
		}

		if i+1 >= len(encoded) {
			return "", errors.New("path escape is truncated")
		}

		switch encoded[i+1] {
		case '0':
			builder.WriteByte('~')
			i++
		case '1':
			builder.WriteByte('/')
			i++
		case '2':
			builder.WriteByte('.')
			i++
		case 'x':
			if i+3 >= len(encoded) {
				return "", errors.New("path hex escape is truncated")
			}

			value, err := strconv.ParseUint(encoded[i+2:i+4], 16, 8)
			if err != nil {
				return "", newCausalError(fmt.Sprintf("path hex escape %q is invalid", encoded[i:i+4]), err)
			}

			builder.WriteByte(byte(value))
			i += 3
		default:
			return "", fmt.Errorf("path escape ~%c is invalid", encoded[i+1])
		}
	}

	return builder.String(), nil
}
