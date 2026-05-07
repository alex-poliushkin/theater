package manifest_test

import (
	"encoding/json"
	"strings"
	"testing"

	pluginmanifest "github.com/alex-poliushkin/theater/plugin/manifest"
)

func TestFinalizeSetsDescriptorDigest(t *testing.T) {
	t.Parallel()

	file := smokeManifest()
	file.DescriptorDigest = "sha256:stale"

	finalized, err := pluginmanifest.Finalize(file)
	if err != nil {
		t.Fatalf("finalize manifest failed: %v", err)
	}
	if finalized.DescriptorDigest == "" || finalized.DescriptorDigest == "sha256:stale" {
		t.Fatalf("finalized digest mismatch: got %q", finalized.DescriptorDigest)
	}
	if err := finalized.Validate(); err != nil {
		t.Fatalf("finalized manifest must validate: %v", err)
	}
}

func TestUnmarshalDraftFileRejectsTrailingJSON(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(smokeManifest())
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	raw = append(raw, []byte("\n{}")...)

	_, err = pluginmanifest.UnmarshalDraftFile(raw)
	if err == nil {
		t.Fatal("draft manifest with trailing JSON must fail")
	}
	if !strings.Contains(err.Error(), "trailing JSON value") {
		t.Fatalf("draft manifest error mismatch: %v", err)
	}
}

func smokeManifest() pluginmanifest.File {
	return pluginmanifest.File{
		Schema: pluginmanifest.SchemaVersion,
		Plugin: pluginmanifest.Plugin{
			ID:      "smoke",
			Version: "0.1.0",
		},
		Protocol: pluginmanifest.Protocol{
			Name:  pluginmanifest.ProtocolName,
			Major: 1,
		},
		Capabilities: []pluginmanifest.Capability{
			{
				Kind:           pluginmanifest.CapabilityKindAction,
				Name:           "action.smoke.echo",
				PropertySchema: json.RawMessage(`{"type":"object"}`),
			},
		},
	}
}
