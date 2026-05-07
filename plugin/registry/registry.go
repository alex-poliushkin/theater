package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	ConfigSchemaVersion = "theater.plugin.registry/v1alpha1"
	LockSchemaVersion   = "theater.plugin.lock/v1alpha1"
)

type ConfigFile struct {
	Schema  string                 `json:"schema"`
	Plugins map[string]PluginEntry `json:"plugins"`
}

type PluginEntry struct {
	Manifest          string         `json:"manifest"`
	Exec              ExecSpec       `json:"exec"`
	AllowCapabilities []string       `json:"allow_capabilities,omitempty"`
	Grants            Grants         `json:"grants,omitempty"`
	Config            map[string]any `json:"config,omitempty"`
	Timeouts          TimeoutPolicy  `json:"timeouts,omitempty"`
}

type ExecSpec struct {
	Command []string `json:"command"`
}

type Grants struct {
	ObserveLog      bool              `json:"observe_log,omitempty"`
	ObserveProgress bool              `json:"observe_progress,omitempty"`
	Env             map[string]string `json:"env,omitempty"`
}

type TimeoutPolicy struct {
	Launch         string `json:"launch,omitempty"`
	RequestDefault string `json:"request_default,omitempty"`
	Shutdown       string `json:"shutdown,omitempty"`
	CancelGrace    string `json:"cancel_grace,omitempty"`
	Validate       string `json:"validate,omitempty"`
	Prepare        string `json:"prepare,omitempty"`
}

type LockFile struct {
	Schema  string               `json:"schema"`
	Plugins map[string]LockEntry `json:"plugins"`
}

type LockEntry struct {
	ManifestSHA256   string `json:"manifest_sha256,omitempty"`
	ExecutableSHA256 string `json:"executable_sha256,omitempty"`
}

func LoadConfigFile(path string) (ConfigFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ConfigFile{}, fmt.Errorf("read plugin registry config: %w", err)
	}

	var config ConfigFile
	if err := json.Unmarshal(raw, &config); err != nil {
		return ConfigFile{}, fmt.Errorf("decode plugin registry config: %w", err)
	}
	if err := config.Validate(); err != nil {
		return ConfigFile{}, err
	}

	return config, nil
}

func LoadLockFile(path string) (LockFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return LockFile{}, fmt.Errorf("read plugin lock file: %w", err)
	}

	var lock LockFile
	if err := json.Unmarshal(raw, &lock); err != nil {
		return LockFile{}, fmt.Errorf("decode plugin lock file: %w", err)
	}
	if err := lock.Validate(); err != nil {
		return LockFile{}, err
	}

	return lock, nil
}

func WriteLockFile(path string, lock LockFile) error {
	if err := lock.Validate(); err != nil {
		return err
	}

	raw, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return fmt.Errorf("encode plugin lock file: %w", err)
	}

	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("write plugin lock file: %w", err)
	}

	return nil
}

func (c ConfigFile) Validate() error {
	if c.Schema != ConfigSchemaVersion {
		return fmt.Errorf("plugin registry schema %q is not supported", c.Schema)
	}
	if len(c.Plugins) == 0 {
		return errors.New("plugin registry must declare at least one plugin")
	}

	for id := range c.Plugins {
		if strings.TrimSpace(id) == "" {
			return errors.New("plugin registry contains an empty plugin id")
		}
		if err := c.Plugins[id].Validate(id); err != nil {
			return err
		}
	}

	return nil
}

func (e PluginEntry) Validate(id string) error {
	if strings.TrimSpace(e.Manifest) == "" {
		return fmt.Errorf("plugin %q manifest is required", id)
	}
	if len(e.Exec.Command) == 0 {
		return fmt.Errorf("plugin %q exec.command is required", id)
	}
	for i := range e.Exec.Command {
		if strings.TrimSpace(e.Exec.Command[i]) == "" {
			return fmt.Errorf("plugin %q exec.command[%d] must not be empty", id, i)
		}
	}
	if len(e.AllowCapabilities) == 0 {
		return fmt.Errorf("plugin %q allow_capabilities must declare at least one capability", id)
	}
	for name := range e.Grants.Env {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("plugin %q env grant name must not be empty", id)
		}
	}

	return nil
}
func (l LockFile) Validate() error {
	if l.Schema != LockSchemaVersion {
		return fmt.Errorf("plugin lock schema %q is not supported", l.Schema)
	}
	if len(l.Plugins) == 0 {
		return errors.New("plugin lock file must declare at least one plugin")
	}
	for id := range l.Plugins {
		if strings.TrimSpace(id) == "" {
			return errors.New("plugin lock file contains an empty plugin id")
		}
	}
	return nil
}
