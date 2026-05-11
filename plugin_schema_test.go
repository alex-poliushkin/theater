package theater

import (
	"testing"

	"github.com/alex-poliushkin/theater/internal/pluginregistry"
	"github.com/alex-poliushkin/theater/internal/secretvalue"
	"github.com/alex-poliushkin/theater/plugin/manifest"
)

func TestPluginInventoryContractMarksRootSensitiveOutput(t *testing.T) {
	t.Parallel()

	contract, err := pluginInventoryContract(pluginregistry.LoadedCapability{
		Manifest: manifest.Capability{
			Annotations: manifest.CapabilityMetadata{
				SensitiveOutputPaths: []string{""},
			},
		},
		PropertySchema: pluginregistry.SchemaShape{Types: []string{"object"}},
		ResultSchema:   pluginregistry.SchemaShape{Types: []string{"string"}},
	})
	if err != nil {
		t.Fatalf("inventory contract: %v", err)
	}

	if got, want := contract.Produces.Sensitivity, SensitivitySecret; got != want {
		t.Fatalf("sensitivity mismatch: got %q want %q", got, want)
	}
	if got, want := contract.Produces.Capture, CaptureSummary; got != want {
		t.Fatalf("capture mismatch: got %q want %q", got, want)
	}
}

func TestPluginTransformContractMarksRootSensitiveOutput(t *testing.T) {
	t.Parallel()

	contract, err := pluginTransformContract(pluginregistry.LoadedCapability{
		Manifest: manifest.Capability{
			Annotations: manifest.CapabilityMetadata{
				SensitiveOutputPaths: []string{""},
				Transform: &manifest.TransformCapabilityMetadata{
					Accepts:  ValueContract{Kind: ValueKindString},
					Produces: ValueContract{Kind: ValueKindString},
				},
			},
		},
		PropertySchema: pluginregistry.SchemaShape{Types: []string{"object"}},
	})
	if err != nil {
		t.Fatalf("transform contract: %v", err)
	}

	if got, want := contract.Produces.Sensitivity, SensitivitySecret; got != want {
		t.Fatalf("sensitivity mismatch: got %q want %q", got, want)
	}
	if got, want := contract.Produces.Capture, CaptureSummary; got != want {
		t.Fatalf("capture mismatch: got %q want %q", got, want)
	}
}

func TestPluginActionContractMarksRootSensitiveInputsAndOutputs(t *testing.T) {
	t.Parallel()

	contract, err := pluginActionContract(pluginregistry.LoadedCapability{
		Manifest: manifest.Capability{
			Annotations: manifest.CapabilityMetadata{
				SensitiveInputPaths:  []string{""},
				SensitiveOutputPaths: []string{""},
			},
		},
		PropertySchema: pluginregistry.SchemaShape{
			Types: []string{"object"},
			Properties: map[string]pluginregistry.SchemaShape{
				"token": {Types: []string{"string"}},
			},
		},
		ResultSchema: pluginregistry.SchemaShape{
			Types: []string{"object"},
			Properties: map[string]pluginregistry.SchemaShape{
				"echo": {Types: []string{"string"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("action contract: %v", err)
	}

	if got, want := contract.Inputs["token"].Sensitivity, SensitivitySecret; got != want {
		t.Fatalf("input sensitivity mismatch: got %q want %q", got, want)
	}
	if got, want := contract.Outputs["echo"].Sensitivity, SensitivitySecret; got != want {
		t.Fatalf("output sensitivity mismatch: got %q want %q", got, want)
	}
}

func TestPluginActionContractKeepsNonRootSensitivePointers(t *testing.T) {
	t.Parallel()

	contract, err := pluginActionContract(pluginregistry.LoadedCapability{
		Manifest: manifest.Capability{
			Annotations: manifest.CapabilityMetadata{
				SensitiveOutputPaths: []string{"/token"},
			},
		},
		PropertySchema: pluginregistry.SchemaShape{Types: []string{"object"}},
		ResultSchema: pluginregistry.SchemaShape{
			Types: []string{"object"},
			Properties: map[string]pluginregistry.SchemaShape{
				"token": {Types: []string{"string"}},
				"meta":  {Types: []string{"string"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("action contract: %v", err)
	}

	if got, want := contract.Outputs["token"].Sensitivity, SensitivitySecret; got != want {
		t.Fatalf("token sensitivity mismatch: got %q want %q", got, want)
	}
	if got := contract.Outputs["meta"].Sensitivity; got != "" {
		t.Fatalf("meta sensitivity mismatch: got %q want empty", got)
	}
}

func TestRedactorForPluginValueInputUsesRootPointerForValue(t *testing.T) {
	t.Parallel()

	redactor := redactorForPluginValueInput(
		map[string]any{"prefix": "visible-prefix"},
		"scalar-secret",
		nil,
		[]string{""},
	)

	if got, want := redactor.RedactText("input=scalar-secret prefix=visible-prefix"), "input=[redacted] prefix=visible-prefix"; got != want {
		t.Fatalf("redacted text mismatch: got %q want %q", got, want)
	}
}

func TestRedactorForPluginValueInputKeepsPropertyPointers(t *testing.T) {
	t.Parallel()

	redactor := redactorForPluginValueInput(
		map[string]any{"token": "property-secret"},
		"visible-value",
		nil,
		[]string{"/token"},
	)

	if got, want := redactor.RedactText("token=property-secret value=visible-value"), "token=[redacted] value=visible-value"; got != want {
		t.Fatalf("redacted text mismatch: got %q want %q", got, want)
	}
}

func TestRedactorForPluginValueInputIncludesRuntimeSecretValue(t *testing.T) {
	t.Parallel()

	redactor := redactorForPluginValueInput(
		map[string]any{"prefix": "visible-prefix"},
		"runtime-secret",
		NewSecret("runtime-secret"),
		nil,
	)

	if got, want := redactor.RedactText("input=runtime-secret prefix=visible-prefix"), "input=[redacted] prefix=visible-prefix"; got != want {
		t.Fatalf("redacted text mismatch: got %q want %q", got, want)
	}
}

func TestRedactorForPluginValueInputIgnoresPlainRuntimeValue(t *testing.T) {
	t.Parallel()

	redactor := redactorForPluginValueInput(
		map[string]any{"prefix": "visible-prefix"},
		"visible-value",
		"visible-value",
		nil,
	)

	if got, want := redactor.RedactText("value=visible-value"), "value=visible-value"; got != want {
		t.Fatalf("redacted text mismatch: got %q want %q", got, want)
	}
}

func TestProtectJSONCompatibleObjectPreservesShapeForRootPointer(t *testing.T) {
	t.Parallel()

	protected, err := protectJSONCompatibleObject(
		map[string]any{
			"token": "issued-token",
			"meta":  map[string]any{"id": "profile-1"},
		},
		[]string{""},
	)
	if err != nil {
		t.Fatalf("protect object: %v", err)
	}
	if _, ok := protected["token"].(secretvalue.Value); !ok {
		t.Fatalf("token must be secret, got %T", protected["token"])
	}
	if _, ok := protected["meta"].(secretvalue.Value); !ok {
		t.Fatalf("meta must be secret, got %T", protected["meta"])
	}
}
