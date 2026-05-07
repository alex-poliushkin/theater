package manifest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"

	specmodel "github.com/alex-poliushkin/theater/spec"
	statemodel "github.com/alex-poliushkin/theater/state"
)

const (
	SchemaVersion = "theater.plugin.manifest/v1alpha1"
	ProtocolName  = "theater-jsonrpc"
)

type CapabilityKind string

const (
	CapabilityKindAction         CapabilityKind = "action"
	CapabilityKindInventory      CapabilityKind = "inventory"
	CapabilityKindReportExporter CapabilityKind = "report_exporter"
	CapabilityKindStateBackend   CapabilityKind = "state_backend"
	CapabilityKindTransform      CapabilityKind = "transform"
	CapabilityKindMatcher        CapabilityKind = "matcher"
)

var (
	// ErrDescriptorDigestRequired reports a manifest that omits descriptor_digest.
	ErrDescriptorDigestRequired = errors.New("descriptor_digest is required")
	// ErrDescriptorDigestMismatch reports a manifest whose descriptor_digest does not match its descriptor payload.
	ErrDescriptorDigestMismatch = errors.New("descriptor_digest mismatch")
)

type File struct {
	Schema           string          `json:"schema"`
	Plugin           Plugin          `json:"plugin"`
	Protocol         Protocol        `json:"protocol"`
	ConfigSchema     json.RawMessage `json:"config_schema,omitempty"`
	Capabilities     []Capability    `json:"capabilities"`
	DescriptorDigest string          `json:"descriptor_digest,omitempty"`
}

type Plugin struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type Protocol struct {
	Name  string `json:"name"`
	Major int    `json:"major"`
	Minor int    `json:"minor"`
}

type Capability struct {
	Kind           CapabilityKind     `json:"kind"`
	Name           string             `json:"name"`
	Summary        string             `json:"summary,omitempty"`
	PropertySchema json.RawMessage    `json:"property_schema"`
	ResultSchema   json.RawMessage    `json:"result_schema,omitempty"`
	Annotations    CapabilityMetadata `json:"annotations,omitempty"`
}

type CapabilityMetadata struct {
	ReadOnly             bool                         `json:"read_only,omitempty"`
	Idempotent           bool                         `json:"idempotent,omitempty"`
	SupportsValidate     bool                         `json:"supports_validate,omitempty"`
	SupportsPrepare      bool                         `json:"supports_prepare,omitempty"`
	RequiredHostGrants   []string                     `json:"required_host_grants,omitempty"`
	SensitiveInputPaths  []string                     `json:"sensitive_input_paths,omitempty"`
	SensitiveOutputPaths []string                     `json:"sensitive_output_paths,omitempty"`
	State                *StateCapabilityMetadata     `json:"state,omitempty"`
	Transform            *TransformCapabilityMetadata `json:"transform,omitempty"`
	Matcher              *MatcherCapabilityMetadata   `json:"matcher,omitempty"`
}

type StateCapabilityMetadata struct {
	Guarantee       statemodel.GuaranteeTier `json:"guarantee,omitempty"`
	SupportsCAS     bool                     `json:"supports_cas,omitempty"`
	SupportsClaim   bool                     `json:"supports_claim,omitempty"`
	SupportsRenew   bool                     `json:"supports_renew,omitempty"`
	SupportsRelease bool                     `json:"supports_release,omitempty"`
	SupportsConsume bool                     `json:"supports_consume,omitempty"`
}

type TransformCapabilityMetadata struct {
	Accepts  specmodel.ValueContract `json:"accepts"`
	Produces specmodel.ValueContract `json:"produces"`
}

type MatcherCapabilityMetadata struct {
	Actual specmodel.ValueContract `json:"actual,omitempty"`
}

func UnmarshalFile(raw []byte) (File, error) {
	if len(raw) == 0 {
		return File{}, errors.New("plugin manifest is empty")
	}

	if err := validateDocument(raw, manifestSchemaJSON); err != nil {
		return File{}, err
	}

	file, err := decodeFile(raw)
	if err != nil {
		return File{}, err
	}

	if err := file.Validate(); err != nil {
		return File{}, err
	}

	return file, nil
}

// UnmarshalDraftFile decodes a manifest draft before descriptor_digest has been
// filled or refreshed.
func UnmarshalDraftFile(raw []byte) (File, error) {
	if len(raw) == 0 {
		return File{}, errors.New("plugin manifest is empty")
	}

	return decodeFile(raw)
}

// Finalize returns a validated manifest with descriptor_digest set to the
// canonical digest of its protocol and capability descriptors.
func Finalize(file File) (File, error) {
	digest, err := CanonicalDescriptorDigest(file)
	if err != nil {
		return File{}, err
	}

	file.DescriptorDigest = digest
	if err := file.Validate(); err != nil {
		return File{}, err
	}

	return file, nil
}

func (f File) Validate() error {
	if f.Schema != SchemaVersion {
		return fmt.Errorf("plugin manifest schema %q is not supported", f.Schema)
	}

	if strings.TrimSpace(f.Plugin.ID) == "" {
		return errors.New("plugin id is required")
	}
	if strings.TrimSpace(f.Plugin.Version) == "" {
		return errors.New("plugin version is required")
	}

	if f.Protocol.Name != ProtocolName {
		return fmt.Errorf("plugin protocol %q is not supported", f.Protocol.Name)
	}
	if f.Protocol.Major <= 0 {
		return errors.New("plugin protocol major version must be positive")
	}
	if len(f.Capabilities) == 0 {
		return errors.New("plugin must declare at least one capability")
	}

	seen := make(map[string]struct{}, len(f.Capabilities))
	for i := range f.Capabilities {
		capability := f.Capabilities[i]
		if err := capability.Validate(); err != nil {
			return fmt.Errorf("capability %d: %w", i, err)
		}

		if _, ok := seen[capability.Name]; ok {
			return fmt.Errorf("capability %q is declared more than once", capability.Name)
		}
		seen[capability.Name] = struct{}{}
	}

	if len(f.ConfigSchema) != 0 {
		if err := validateJSONSchema(f.ConfigSchema); err != nil {
			return fmt.Errorf("config_schema is invalid: %w", err)
		}
	}

	digest, err := CanonicalDescriptorDigest(f)
	if err != nil {
		return err
	}
	if f.DescriptorDigest == "" {
		return ErrDescriptorDigestRequired
	}
	if f.DescriptorDigest != digest {
		return fmt.Errorf("%w: got %q want %q", ErrDescriptorDigestMismatch, f.DescriptorDigest, digest)
	}

	return nil
}

func (c Capability) Validate() error {
	switch c.Kind {
	case CapabilityKindAction,
		CapabilityKindInventory,
		CapabilityKindReportExporter,
		CapabilityKindStateBackend,
		CapabilityKindTransform,
		CapabilityKindMatcher:
	default:
		return fmt.Errorf("capability kind %q is not supported", c.Kind)
	}

	if strings.TrimSpace(c.Name) == "" {
		return errors.New("capability name is required")
	}
	if len(c.PropertySchema) == 0 {
		return fmt.Errorf("capability %q property_schema is required", c.Name)
	}
	if err := validateJSONSchema(c.PropertySchema); err != nil {
		return fmt.Errorf("capability %q property_schema is invalid: %w", c.Name, err)
	}
	if len(c.ResultSchema) != 0 {
		if err := validateJSONSchema(c.ResultSchema); err != nil {
			return fmt.Errorf("capability %q result_schema is invalid: %w", c.Name, err)
		}
	}
	if err := c.Annotations.Validate(c.Kind); err != nil {
		return fmt.Errorf("capability %q annotations are invalid: %w", c.Name, err)
	}

	return nil
}

func (m CapabilityMetadata) StateDescriptor() (statemodel.Descriptor, error) {
	if m.State == nil {
		return statemodel.Descriptor{}, errors.New("state descriptor metadata is required")
	}
	if !m.State.Guarantee.Valid() {
		return statemodel.Descriptor{}, fmt.Errorf("state guarantee %q is invalid", m.State.Guarantee)
	}

	return statemodel.Descriptor{
		Guarantee:       m.State.Guarantee,
		SupportsCAS:     m.State.SupportsCAS,
		SupportsClaim:   m.State.SupportsClaim,
		SupportsRenew:   m.State.SupportsRenew,
		SupportsRelease: m.State.SupportsRelease,
		SupportsConsume: m.State.SupportsConsume,
	}, nil
}

func CanonicalDescriptorDigest(f File) (string, error) {
	payload, err := json.Marshal(struct {
		Protocol     Protocol     `json:"protocol"`
		Capabilities []Capability `json:"capabilities"`
	}{
		Protocol:     f.Protocol,
		Capabilities: f.Capabilities,
	})
	if err != nil {
		return "", fmt.Errorf("marshal descriptor digest payload: %w", err)
	}

	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// ResolveJSONSchema decodes and resolves one JSON Schema document.
func ResolveJSONSchema(raw []byte) (*jsonschema.Resolved, error) {
	return resolveSchema(raw)
}

func decodeFile(raw []byte) (File, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	var file File
	if err := decoder.Decode(&file); err != nil {
		return File{}, fmt.Errorf("decode plugin manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return File{}, errors.New("decode plugin manifest: trailing JSON value")
		}
		return File{}, fmt.Errorf("decode plugin manifest: %w", err)
	}

	return file, nil
}

func validateDocument(raw, schemaRaw []byte) error {
	schema, err := resolveSchema(schemaRaw)
	if err != nil {
		return err
	}

	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("decode JSON document: %w", err)
	}

	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("validate JSON schema: %w", err)
	}

	return nil
}

func validateJSONSchema(raw []byte) error {
	_, err := resolveSchema(raw)
	return err
}

func resolveSchema(raw []byte) (*jsonschema.Resolved, error) {
	var schema jsonschema.Schema
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, fmt.Errorf("decode JSON schema: %w", err)
	}

	resolved, err := schema.Resolve(nil)
	if err != nil {
		return nil, fmt.Errorf("resolve JSON schema: %w", err)
	}

	return resolved, nil
}

func (m CapabilityMetadata) Validate(kind CapabilityKind) error {
	if err := validateJSONPointerList(m.SensitiveInputPaths); err != nil {
		return fmt.Errorf("sensitive_input_paths: %w", err)
	}
	if err := validateJSONPointerList(m.SensitiveOutputPaths); err != nil {
		return fmt.Errorf("sensitive_output_paths: %w", err)
	}

	switch kind {
	case CapabilityKindStateBackend:
		return m.validateStateCapability()
	case CapabilityKindTransform:
		return m.validateTransformCapability()
	case CapabilityKindMatcher:
		return m.validateMatcherCapability()
	default:
		return m.validatePlainCapability()
	}
}

func (m CapabilityMetadata) validateStateCapability() error {
	if err := m.requireOnly("state"); err != nil {
		return err
	}
	if m.State == nil {
		return errors.New("state metadata is required for state_backend capabilities")
	}
	if !m.State.Guarantee.Valid() {
		return fmt.Errorf("state guarantee %q is invalid", m.State.Guarantee)
	}
	return nil
}

func (m CapabilityMetadata) validateTransformCapability() error {
	if err := m.requireOnly("transform"); err != nil {
		return err
	}
	if m.Transform == nil {
		return errors.New("transform metadata is required for transform capabilities")
	}
	if !m.Transform.Accepts.Valid() {
		return errors.New("transform accepts contract is invalid")
	}
	if !m.Transform.Produces.Valid() {
		return errors.New("transform produces contract is invalid")
	}
	return nil
}

func (m CapabilityMetadata) validateMatcherCapability() error {
	if err := m.requireOnly("matcher"); err != nil {
		return err
	}
	if m.Matcher == nil {
		return errors.New("matcher metadata is required for matcher capabilities")
	}
	if !m.Matcher.Actual.Valid() {
		return errors.New("matcher actual contract is invalid")
	}
	return nil
}

func (m CapabilityMetadata) validatePlainCapability() error {
	return m.requireOnly("")
}

func (m CapabilityMetadata) requireOnly(kind string) error {
	if kind != "state" && m.State != nil {
		return errors.New("state metadata is only valid for state_backend capabilities")
	}
	if kind != "transform" && m.Transform != nil {
		return errors.New("transform metadata is only valid for transform capabilities")
	}
	if kind != "matcher" && m.Matcher != nil {
		return errors.New("matcher metadata is only valid for matcher capabilities")
	}
	return nil
}

func validateJSONPointerList(values []string) error {
	for i := range values {
		if _, err := specmodel.ParseJSONPointer(values[i]); err != nil {
			return fmt.Errorf("item %d: %w", i, err)
		}
	}

	return nil
}

var manifestSchemaJSON = []byte(`{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "required": ["schema", "plugin", "protocol", "capabilities", "descriptor_digest"],
  "properties": {
    "schema": { "const": "theater.plugin.manifest/v1alpha1" },
    "plugin": {
      "type": "object",
      "additionalProperties": false,
      "required": ["id", "version"],
      "properties": {
        "id": { "type": "string", "minLength": 1 },
        "version": { "type": "string", "minLength": 1 }
      }
    },
    "protocol": {
      "type": "object",
      "additionalProperties": false,
      "required": ["name", "major", "minor"],
      "properties": {
        "name": { "const": "theater-jsonrpc" },
        "major": { "type": "integer", "minimum": 1 },
        "minor": { "type": "integer", "minimum": 0 }
      }
    },
    "config_schema": { "type": "object" },
    "capabilities": {
      "type": "array",
      "minItems": 1,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["kind", "name", "property_schema"],
        "properties": {
          "kind": {
            "enum": [
              "action",
              "inventory",
              "report_exporter",
              "state_backend",
              "transform",
              "matcher"
            ]
          },
          "name": { "type": "string", "minLength": 1 },
          "summary": { "type": "string" },
          "property_schema": { "type": "object" },
          "result_schema": { "type": "object" },
          "annotations": {
            "type": "object",
            "additionalProperties": false,
            "properties": {
              "read_only": { "type": "boolean" },
              "idempotent": { "type": "boolean" },
              "supports_validate": { "type": "boolean" },
              "supports_prepare": { "type": "boolean" },
              "required_host_grants": {
                "type": "array",
                "items": { "type": "string", "minLength": 1 }
              },
              "sensitive_input_paths": {
                "type": "array",
                "items": { "type": "string", "minLength": 1 }
              },
              "sensitive_output_paths": {
                "type": "array",
                "items": { "type": "string", "minLength": 1 }
              },
              "state": {
                "type": "object",
                "additionalProperties": false,
                "properties": {
                  "guarantee": {
                    "type": "string",
                    "enum": ["read-only", "local-atomic", "shared-optimistic", "shared-atomic"]
                  },
                  "supports_cas": { "type": "boolean" },
                  "supports_claim": { "type": "boolean" },
                  "supports_renew": { "type": "boolean" },
                  "supports_release": { "type": "boolean" },
                  "supports_consume": { "type": "boolean" }
                }
              },
              "transform": {
                "type": "object",
                "additionalProperties": false,
                "required": ["accepts", "produces"],
                "properties": {
                  "accepts": { "type": "object" },
                  "produces": { "type": "object" }
                }
              },
              "matcher": {
                "type": "object",
                "additionalProperties": false,
                "required": ["actual"],
                "properties": {
                  "actual": { "type": "object" }
                }
              }
            }
          }
        }
      }
    },
    "descriptor_digest": { "type": "string", "minLength": 1 }
  }
}`)
