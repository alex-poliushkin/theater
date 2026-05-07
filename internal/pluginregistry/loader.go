package pluginregistry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alex-poliushkin/theater/plugin/manifest"
	pluginregistry "github.com/alex-poliushkin/theater/plugin/registry"

	"github.com/google/jsonschema-go/jsonschema"
)

type LoadedRegistry struct {
	ConfigPath string
	LockPath   string
	Plugins    map[string]LoadedPlugin
}

type LoadedPlugin struct {
	ID               string
	ManifestPath     string
	ManifestSHA256   string
	ExecutablePath   string
	ExecutableSHA256 string
	Config           pluginregistry.PluginEntry
	Lock             pluginregistry.LockEntry
	Manifest         manifest.File
	ConfigSchema     *jsonschema.Resolved
	Capabilities     map[string]LoadedCapability
}

type LoadedCapability struct {
	Manifest         manifest.Capability
	PropertySchema   SchemaShape
	ResultSchema     SchemaShape
	PropertyResolved *jsonschema.Resolved
	ResultResolved   *jsonschema.Resolved
}

type SchemaShape struct {
	Any                  bool
	Types                []string
	Description          string
	Properties           map[string]SchemaShape
	Required             map[string]bool
	Items                *SchemaShape
	AdditionalProperties *SchemaShape
}

type runtimeArtifact struct {
	executablePath string
	executableSum  string
}

func Load(configPath, lockPath string) (*LoadedRegistry, error) {
	config, err := pluginregistry.LoadConfigFile(configPath)
	if err != nil {
		return nil, err
	}

	lock := pluginregistry.LockFile{}
	if lockPath != "" {
		lock, err = pluginregistry.LoadLockFile(lockPath)
		if err != nil {
			return nil, err
		}
	}

	root := filepath.Dir(configPath)
	return Build(root, configPath, lockPath, config, lock, lockPath != "")
}

func LoadDescriptors(configPath, lockPath string) (*LoadedRegistry, error) {
	config, err := pluginregistry.LoadConfigFile(configPath)
	if err != nil {
		return nil, err
	}

	lock := pluginregistry.LockFile{}
	if lockPath != "" {
		lock, err = pluginregistry.LoadLockFile(lockPath)
		if err != nil {
			return nil, err
		}
	}

	root := filepath.Dir(configPath)
	return BuildDescriptors(root, configPath, lockPath, config, lock, lockPath != "")
}

func Build(
	root string,
	configPath string,
	lockPath string,
	config pluginregistry.ConfigFile,
	lock pluginregistry.LockFile,
	lockRequired bool,
) (*LoadedRegistry, error) {
	loaded := &LoadedRegistry{
		ConfigPath: configPath,
		LockPath:   lockPath,
		Plugins:    make(map[string]LoadedPlugin, len(config.Plugins)),
	}

	ids := make([]string, 0, len(config.Plugins))
	for id := range config.Plugins {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		entry := config.Plugins[id]
		loadedPlugin, err := loadPlugin(root, id, entry, lock.Plugins[id], lockRequired)
		if err != nil {
			return nil, err
		}
		loaded.Plugins[id] = loadedPlugin
	}

	return loaded, nil
}

func BuildDescriptors(
	root string,
	configPath string,
	lockPath string,
	config pluginregistry.ConfigFile,
	lock pluginregistry.LockFile,
	lockRequired bool,
) (*LoadedRegistry, error) {
	loaded := &LoadedRegistry{
		ConfigPath: configPath,
		LockPath:   lockPath,
		Plugins:    make(map[string]LoadedPlugin, len(config.Plugins)),
	}

	ids := make([]string, 0, len(config.Plugins))
	for id := range config.Plugins {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		entry := config.Plugins[id]
		loadedPlugin, err := loadPluginDescriptor(root, id, entry, lock.Plugins[id], lockRequired)
		if err != nil {
			return nil, err
		}
		loaded.Plugins[id] = loadedPlugin
	}

	return loaded, nil
}

func loadPlugin(
	root string,
	id string,
	entry pluginregistry.PluginEntry,
	lock pluginregistry.LockEntry,
	lockRequired bool,
) (LoadedPlugin, error) {
	manifestPath, err := resolvePath(root, entry.Manifest)
	if err != nil {
		return LoadedPlugin{}, fmt.Errorf("plugin %q manifest: %w", id, err)
	}

	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		return LoadedPlugin{}, fmt.Errorf("plugin %q manifest %s: %w", id, manifestPath, err)
	}

	parsedManifest, err := manifest.UnmarshalFile(manifestRaw)
	if err != nil {
		return LoadedPlugin{}, fmt.Errorf("plugin %q manifest %s: %w", id, manifestPath, err)
	}
	if parsedManifest.Plugin.ID != id {
		return LoadedPlugin{}, fmt.Errorf("plugin %q manifest plugin.id %q does not match registry key", id, parsedManifest.Plugin.ID)
	}

	var configSchema *jsonschema.Resolved
	if len(parsedManifest.ConfigSchema) != 0 {
		configSchema, err = manifest.ResolveJSONSchema(parsedManifest.ConfigSchema)
		if err != nil {
			return LoadedPlugin{}, fmt.Errorf("plugin %q config_schema: %w", id, err)
		}
		if err := configSchema.Validate(entry.Config); err != nil {
			return LoadedPlugin{}, fmt.Errorf("plugin %q config: %w", id, err)
		}
	}

	manifestSum := checksumBytes(manifestRaw)
	artifact, err := resolveRuntimeArtifact(root, id, entry)
	if err != nil {
		return LoadedPlugin{}, err
	}
	if err := validateLockEntry(id, lock, lockRequired, manifestSum, artifact); err != nil {
		return LoadedPlugin{}, err
	}

	capabilities := make(map[string]LoadedCapability, len(parsedManifest.Capabilities))
	for i := range parsedManifest.Capabilities {
		capability, err := loadCapability(parsedManifest.Capabilities[i])
		if err != nil {
			return LoadedPlugin{}, fmt.Errorf("plugin %q capability %q: %w", id, parsedManifest.Capabilities[i].Name, err)
		}
		capabilities[capability.Manifest.Name] = capability
	}

	return LoadedPlugin{
		ID:               id,
		ManifestPath:     manifestPath,
		ManifestSHA256:   manifestSum,
		ExecutablePath:   artifact.executablePath,
		ExecutableSHA256: artifact.executableSum,
		Config:           entry,
		Lock:             lock,
		Manifest:         parsedManifest,
		ConfigSchema:     configSchema,
		Capabilities:     capabilities,
	}, nil
}

func loadPluginDescriptor(
	root string,
	id string,
	entry pluginregistry.PluginEntry,
	lock pluginregistry.LockEntry,
	lockRequired bool,
) (LoadedPlugin, error) {
	manifestPath, err := resolvePath(root, entry.Manifest)
	if err != nil {
		return LoadedPlugin{}, fmt.Errorf("plugin %q manifest: %w", id, err)
	}

	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		return LoadedPlugin{}, fmt.Errorf("plugin %q manifest %s: %w", id, manifestPath, err)
	}

	parsedManifest, err := manifest.UnmarshalFile(manifestRaw)
	if err != nil {
		return LoadedPlugin{}, fmt.Errorf("plugin %q manifest %s: %w", id, manifestPath, err)
	}
	if parsedManifest.Plugin.ID != id {
		return LoadedPlugin{}, fmt.Errorf("plugin %q manifest plugin.id %q does not match registry key", id, parsedManifest.Plugin.ID)
	}

	var configSchema *jsonschema.Resolved
	if len(parsedManifest.ConfigSchema) != 0 {
		configSchema, err = manifest.ResolveJSONSchema(parsedManifest.ConfigSchema)
		if err != nil {
			return LoadedPlugin{}, fmt.Errorf("plugin %q config_schema: %w", id, err)
		}
		if err := configSchema.Validate(entry.Config); err != nil {
			return LoadedPlugin{}, fmt.Errorf("plugin %q config: %w", id, err)
		}
	}

	manifestSum := checksumBytes(manifestRaw)
	if err := validateDescriptorLockEntry(id, lock, lockRequired, manifestSum); err != nil {
		return LoadedPlugin{}, err
	}

	capabilities := make(map[string]LoadedCapability, len(parsedManifest.Capabilities))
	for i := range parsedManifest.Capabilities {
		capability, err := loadCapability(parsedManifest.Capabilities[i])
		if err != nil {
			return LoadedPlugin{}, fmt.Errorf("plugin %q capability %q: %w", id, parsedManifest.Capabilities[i].Name, err)
		}
		capabilities[capability.Manifest.Name] = capability
	}

	return LoadedPlugin{
		ID:             id,
		ManifestPath:   manifestPath,
		ManifestSHA256: manifestSum,
		Config:         entry,
		Lock:           lock,
		Manifest:       parsedManifest,
		ConfigSchema:   configSchema,
		Capabilities:   capabilities,
	}, nil
}

func resolveRuntimeArtifact(root, id string, entry pluginregistry.PluginEntry) (runtimeArtifact, error) {
	executablePath, err := resolveCommandPath(root, entry.Exec.Command[0])
	if err != nil {
		return runtimeArtifact{}, fmt.Errorf("plugin %q executable: %w", id, err)
	}
	executableSum, err := checksumFile(executablePath)
	if err != nil {
		return runtimeArtifact{}, fmt.Errorf("plugin %q executable: %w", id, err)
	}

	return runtimeArtifact{
		executablePath: executablePath,
		executableSum:  executableSum,
	}, nil
}

func validateLockEntry(
	id string,
	lock pluginregistry.LockEntry,
	lockRequired bool,
	manifestSum string,
	artifact runtimeArtifact,
) error {
	if !lockRequired {
		return nil
	}
	if lock.ManifestSHA256 == "" {
		return fmt.Errorf("plugin %q lock entry is required", id)
	}
	if lock.ManifestSHA256 != manifestSum {
		return fmt.Errorf("plugin %q manifest checksum mismatch", id)
	}
	if lock.ExecutableSHA256 == "" {
		return fmt.Errorf("plugin %q executable checksum is required", id)
	}
	if lock.ExecutableSHA256 != artifact.executableSum {
		return fmt.Errorf("plugin %q executable checksum mismatch", id)
	}

	return nil
}

func validateDescriptorLockEntry(id string, lock pluginregistry.LockEntry, lockRequired bool, manifestSum string) error {
	if !lockRequired {
		return nil
	}
	if lock.ManifestSHA256 == "" {
		return fmt.Errorf("plugin %q lock entry is required", id)
	}
	if lock.ManifestSHA256 != manifestSum {
		return fmt.Errorf("plugin %q manifest checksum mismatch", id)
	}

	return nil
}

func loadCapability(capability manifest.Capability) (LoadedCapability, error) {
	propertyResolved, err := manifest.ResolveJSONSchema(capability.PropertySchema)
	if err != nil {
		return LoadedCapability{}, err
	}
	propertyShape, err := decodeSchemaShape(capability.PropertySchema)
	if err != nil {
		return LoadedCapability{}, err
	}

	var resultResolved *jsonschema.Resolved
	resultShape := SchemaShape{Any: true}
	if len(capability.ResultSchema) != 0 {
		resultResolved, err = manifest.ResolveJSONSchema(capability.ResultSchema)
		if err != nil {
			return LoadedCapability{}, err
		}
		resultShape, err = decodeSchemaShape(capability.ResultSchema)
		if err != nil {
			return LoadedCapability{}, err
		}
	}

	return LoadedCapability{
		Manifest:         capability,
		PropertySchema:   propertyShape,
		ResultSchema:     resultShape,
		PropertyResolved: propertyResolved,
		ResultResolved:   resultResolved,
	}, nil
}

func resolvePath(root, path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	if path == "" {
		return "", errors.New("path is required")
	}

	return filepath.Join(root, path), nil
}

func resolveCommandPath(root, raw string) (string, error) {
	if raw == "" {
		return "", errors.New("command is required")
	}

	if filepath.IsAbs(raw) {
		return raw, nil
	}
	if strings.ContainsRune(raw, filepath.Separator) {
		return filepath.Join(root, raw), nil
	}

	return exec.LookPath(raw)
}

func checksumFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return checksumBytes(raw), nil
}

func checksumBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func decodeSchemaShape(raw []byte) (SchemaShape, error) {
	var doc rawSchema
	if err := json.Unmarshal(raw, &doc); err != nil {
		return SchemaShape{}, fmt.Errorf("decode schema shape: %w", err)
	}

	return doc.toShape(), nil
}

type rawSchema struct {
	Type                 any                  `json:"type,omitempty"`
	Description          string               `json:"description,omitempty"`
	Properties           map[string]rawSchema `json:"properties,omitempty"`
	Required             []string             `json:"required,omitempty"`
	Items                *rawSchema           `json:"items,omitempty"`
	AdditionalProperties any                  `json:"additionalProperties,omitempty"`
}

func (s rawSchema) toShape() SchemaShape {
	shape := SchemaShape{
		Types:       decodeSchemaTypes(s.Type),
		Description: s.Description,
	}
	if len(shape.Types) == 0 {
		shape.Any = true
	}

	if len(s.Properties) != 0 {
		shape.Properties = make(map[string]SchemaShape, len(s.Properties))
		for name, child := range s.Properties {
			shape.Properties[name] = child.toShape()
		}
	}
	if len(s.Required) != 0 {
		shape.Required = make(map[string]bool, len(s.Required))
		for _, name := range s.Required {
			shape.Required[name] = true
		}
	}
	if s.Items != nil {
		item := s.Items.toShape()
		shape.Items = &item
	}
	switch value := s.AdditionalProperties.(type) {
	case bool:
		if value {
			anyShape := SchemaShape{Any: true}
			shape.AdditionalProperties = &anyShape
		}
	case map[string]any:
		child, err := rawSchemaFromMap(value)
		if err == nil {
			decoded := child.toShape()
			shape.AdditionalProperties = &decoded
		}
	}

	return shape
}

func rawSchemaFromMap(value map[string]any) (rawSchema, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return rawSchema{}, err
	}

	var doc rawSchema
	if err := json.Unmarshal(raw, &doc); err != nil {
		return rawSchema{}, err
	}

	return doc, nil
}

func decodeSchemaTypes(raw any) []string {
	switch value := raw.(type) {
	case string:
		if value == "" {
			return nil
		}
		return []string{value}
	case []any:
		types := make([]string, 0, len(value))
		for _, item := range value {
			text, ok := item.(string)
			if !ok || text == "" {
				continue
			}
			types = append(types, text)
		}
		if len(types) == 0 {
			return nil
		}
		return types
	default:
		return nil
	}
}
