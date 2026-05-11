package manifest_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	pluginmanifest "github.com/alex-poliushkin/theater/plugin/manifest"
	specmodel "github.com/alex-poliushkin/theater/spec"
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

func TestUnmarshalFileAcceptsRootSensitivePointers(t *testing.T) {
	t.Parallel()

	file := smokeManifest()
	file.Capabilities[0].Annotations.SensitiveInputPaths = []string{""}
	file.Capabilities[0].Annotations.SensitiveOutputPaths = []string{""}
	finalized, err := pluginmanifest.Finalize(file)
	if err != nil {
		t.Fatalf("finalize manifest: %v", err)
	}
	raw, err := json.Marshal(finalized)
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}

	if _, err := pluginmanifest.UnmarshalFile(raw); err != nil {
		t.Fatalf("manifest with root sensitive pointers must unmarshal: %v", err)
	}
}

func TestUnmarshalFileAcceptsTransformUnionKinds(t *testing.T) {
	t.Parallel()

	file := transformUnionManifestFromDocs(t)
	accepts := file.Capabilities[0].Annotations.Transform.Accepts
	if !accepts.Supports(specmodel.ValueKindString) || !accepts.Supports(specmodel.ValueKindObject) {
		t.Fatalf("transform accepts contract must support string and object: %#v", accepts)
	}
	if field := accepts.Fields["otp"]; !field.Required || field.Kind != specmodel.ValueKindString {
		t.Fatalf("transform object field contract mismatch: %#v", field)
	}

	finalized, err := pluginmanifest.Finalize(file)
	if err != nil {
		t.Fatalf("finalize manifest: %v", err)
	}
	raw, err := json.Marshal(finalized)
	if err != nil {
		t.Fatalf("encode finalized manifest: %v", err)
	}
	if !strings.Contains(string(raw), `"kinds":["string","object"]`) {
		t.Fatalf("finalized manifest must keep array-shaped kinds, got %s", raw)
	}
	if _, err := pluginmanifest.UnmarshalFile(raw); err != nil {
		t.Fatalf("manifest with transform union kinds must unmarshal: %v", err)
	}

	legacy, err := pluginmanifest.UnmarshalDraftFile([]byte(legacyTransformUnionManifestJSON()))
	if err != nil {
		t.Fatalf("unmarshal legacy draft manifest: %v", err)
	}
	finalizedLegacy, err := pluginmanifest.Finalize(legacy)
	if err != nil {
		t.Fatalf("finalize legacy manifest: %v", err)
	}
	if got, want := finalizedLegacy.DescriptorDigest, finalized.DescriptorDigest; got != want {
		t.Fatalf("descriptor digest must not depend on kinds shape or order: got %q want %q", got, want)
	}
}

func TestCapabilityMetadataRejectsInvalidSensitivePointers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		apply   func(*pluginmanifest.File)
		wantErr string
	}{
		{
			name: "input path",
			apply: func(file *pluginmanifest.File) {
				file.Capabilities[0].Annotations.SensitiveInputPaths = []string{"#/token"}
			},
			wantErr: "sensitive_input_paths",
		},
		{
			name: "output path",
			apply: func(file *pluginmanifest.File) {
				file.Capabilities[0].Annotations.SensitiveOutputPaths = []string{"#/token"}
			},
			wantErr: "sensitive_output_paths",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			file := smokeManifest()
			test.apply(&file)
			_, err := pluginmanifest.Finalize(file)
			if err == nil {
				t.Fatal("expected invalid sensitive pointer error")
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("invalid sensitive pointer error mismatch: %v", err)
			}
		})
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

func transformUnionManifestFromDocs(t *testing.T) pluginmanifest.File {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "examples", "plugin-registry", "transform-union-annotation.json"))
	if err != nil {
		t.Fatalf("read transform union docs example: %v", err)
	}

	var snippet struct {
		Annotations pluginmanifest.CapabilityMetadata `json:"annotations"`
	}
	if err := json.Unmarshal(raw, &snippet); err != nil {
		t.Fatalf("decode transform union docs example: %v", err)
	}

	return transformUnionManifest(snippet.Annotations)
}

func transformUnionManifest(annotations pluginmanifest.CapabilityMetadata) pluginmanifest.File {
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
				Kind:           pluginmanifest.CapabilityKindTransform,
				Name:           "transform.smoke.otp",
				PropertySchema: json.RawMessage(`{"type":"object"}`),
				Annotations:    annotations,
			},
		},
	}
}

func legacyTransformUnionManifestJSON() string {
	return `{
		"schema": "theater.plugin.manifest/v1alpha1",
		"plugin": {
			"id": "smoke",
			"version": "0.1.0"
		},
		"protocol": {
			"name": "theater-jsonrpc",
			"major": 1
		},
		"capabilities": [
			{
				"kind": "transform",
				"name": "transform.smoke.otp",
				"property_schema": {
					"type": "object"
				},
				"annotations": {
					"transform": {
						"accepts": {
							"kinds": {"object": {}, "string": {}},
							"fields": {
								"otp": {
									"type": "string",
									"required": true
								}
							}
						},
						"produces": {
							"type": "string"
						}
					}
				}
			}
		]
	}`
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}
