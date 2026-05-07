package sdk_test

import (
	"encoding/json"
	"errors"
	"testing"

	pluginmanifest "github.com/alex-poliushkin/theater/plugin/manifest"
	pluginsdk "github.com/alex-poliushkin/theater/plugin/sdk"
)

func TestFinalizeManifestSetsDescriptorDigest(t *testing.T) {
	t.Parallel()

	file := smokeManifest()
	file.DescriptorDigest = ""

	finalized, err := pluginsdk.FinalizeManifest(file)
	if err != nil {
		t.Fatalf("finalize manifest failed: %v", err)
	}
	if finalized.DescriptorDigest == "" {
		t.Fatal("finalized manifest must include descriptor digest")
	}
	if err := finalized.Validate(); err != nil {
		t.Fatalf("finalized manifest must validate: %v", err)
	}
}

func TestFinalizeManifestReplacesStaleDescriptorDigest(t *testing.T) {
	t.Parallel()

	file := smokeManifest()
	file.DescriptorDigest = "sha256:stale"

	finalized, err := pluginsdk.FinalizeManifest(file)
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

func TestFinalizeManifestReturnsValidationErrors(t *testing.T) {
	t.Parallel()

	file := smokeManifest()
	file.Capabilities = nil

	_, err := pluginsdk.FinalizeManifest(file)
	if err == nil {
		t.Fatal("finalize manifest must fail for invalid manifests")
	}
	if errors.Is(err, pluginmanifest.ErrDescriptorDigestRequired) ||
		errors.Is(err, pluginmanifest.ErrDescriptorDigestMismatch) {
		t.Fatalf("finalize manifest must surface validation errors after digest finalization: %v", err)
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
