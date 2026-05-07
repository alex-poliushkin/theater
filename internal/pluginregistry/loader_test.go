package pluginregistry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	publicregistry "github.com/alex-poliushkin/theater/plugin/registry"
)

func TestLoadDescriptorsDoesNotResolveExecutable(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	manifestPath := filepath.Join(root, "testdata", "plugins", "smoke", "manifest.json")
	manifestSHA, err := checksumFile(manifestPath)
	if err != nil {
		t.Fatalf("checksum manifest: %v", err)
	}

	configPath := filepath.Join(t.TempDir(), "plugins.json")
	lockPath := filepath.Join(t.TempDir(), "plugins.lock.json")
	writeJSONFile(t, configPath, publicregistry.ConfigFile{
		Schema: publicregistry.ConfigSchemaVersion,
		Plugins: map[string]publicregistry.PluginEntry{
			"smoke-plugin": {
				Manifest: manifestPath,
				Exec: publicregistry.ExecSpec{
					Command: []string{filepath.Join(t.TempDir(), "missing-plugin-executable")},
				},
				AllowCapabilities: []string{"action.smoke.echo"},
			},
		},
	})
	writeJSONFile(t, lockPath, publicregistry.LockFile{
		Schema: publicregistry.LockSchemaVersion,
		Plugins: map[string]publicregistry.LockEntry{
			"smoke-plugin": {
				ManifestSHA256:   manifestSHA,
				ExecutableSHA256: "sha256:descriptor-loader-must-not-read-this",
			},
		},
	})

	loaded, err := LoadDescriptors(configPath, lockPath)
	if err != nil {
		t.Fatalf("load descriptors: %v", err)
	}
	plugin := loaded.Plugins["smoke-plugin"]
	if plugin.ExecutablePath != "" || plugin.ExecutableSHA256 != "" {
		t.Fatalf("descriptor-only load must not resolve executable fields: %#v", plugin)
	}
	if _, ok := plugin.Capabilities["action.smoke.echo"]; !ok {
		t.Fatalf("descriptor-only load must retain allowed manifest capabilities: %#v", plugin.Capabilities)
	}
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()

	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("encode JSON %s: %v", path, err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write JSON %s: %v", path, err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve repo root: caller unavailable")
	}

	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}
