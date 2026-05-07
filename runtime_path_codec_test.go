package theater

import "testing"

func TestRuntimePathCodecEscapesAndDecodesReservedBytes(t *testing.T) {
	t.Parallel()

	raw := "auth/login.v1~\n\x7f"
	encoded := escapeRuntimePathID(raw)

	if got, want := encoded, "auth~1login~2v1~0~x0A~x7F"; got != want {
		t.Fatalf("encoded id mismatch: got %q want %q", got, want)
	}

	decoded, err := decodeRuntimePathID(encoded)
	if err != nil {
		t.Fatalf("decode runtime path id failed: %v", err)
	}

	if got, want := decoded, raw; got != want {
		t.Fatalf("decoded id mismatch: got %q want %q", got, want)
	}
}

func TestRuntimePathCodecSplitSegmentRoundTripsEncodedID(t *testing.T) {
	t.Parallel()

	codec := runtimePathCodec{}
	segment := codec.Join("scenario", "auth/login.v1")

	kind, id, err := codec.SplitSegment(segment)
	if err != nil {
		t.Fatalf("split segment failed: %v", err)
	}

	if got, want := kind, "scenario"; got != want {
		t.Fatalf("segment kind mismatch: got %q want %q", got, want)
	}

	if got, want := id, "auth/login.v1"; got != want {
		t.Fatalf("segment id mismatch: got %q want %q", got, want)
	}
}

func TestRuntimePathCodecSplitSegmentAcceptsLiteralActionSegment(t *testing.T) {
	t.Parallel()

	kind, id, err := runtimePathCodec{}.SplitSegment("action")
	if err != nil {
		t.Fatalf("split action segment failed: %v", err)
	}

	if got, want := kind, "action"; got != want {
		t.Fatalf("segment kind mismatch: got %q want %q", got, want)
	}

	if got, want := id, ""; got != want {
		t.Fatalf("segment id mismatch: got %q want %q", got, want)
	}
}

func TestRuntimePathCodecRejectsInvalidEscapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		encoded string
		want    string
	}{
		{
			name:    "truncated simple escape",
			encoded: "~",
			want:    "path escape is truncated",
		},
		{
			name:    "invalid escape selector",
			encoded: "~3",
			want:    "path escape ~3 is invalid",
		},
		{
			name:    "truncated hex escape",
			encoded: "~x0",
			want:    "path hex escape is truncated",
		},
		{
			name:    "invalid hex escape",
			encoded: "~xGG",
			want:    `path hex escape "~xGG" is invalid`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := runtimePathCodec{}.DecodeID(tt.encoded)
			if err == nil {
				t.Fatal("expected invalid escape error")
			}

			if got := err.Error(); got != tt.want {
				t.Fatalf("error mismatch: got %q want %q", got, tt.want)
			}
		})
	}
}

func TestRuntimePathCodecRejectsInvalidSegment(t *testing.T) {
	t.Parallel()

	_, _, err := runtimePathCodec{}.SplitSegment("scenario")
	if err == nil {
		t.Fatal("expected invalid segment error")
	}

	if got, want := err.Error(), `runtime path segment "scenario" is invalid`; got != want {
		t.Fatalf("error mismatch: got %q want %q", got, want)
	}
}
