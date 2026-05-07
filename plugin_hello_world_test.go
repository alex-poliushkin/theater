package theater_test

import (
	"context"
	"path/filepath"
	"testing"

	theater "github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/builtin"
	builtinexpectation "github.com/alex-poliushkin/theater/builtin/expectation"
	internalpluginregistry "github.com/alex-poliushkin/theater/internal/pluginregistry"
	pluginregistry "github.com/alex-poliushkin/theater/plugin/registry"
)

func TestRunnerExecutesHelloWorldPlugin(t *testing.T) {
	t.Parallel()

	repoRoot := repoRoot(t)
	binaryPath := filepath.Join(t.TempDir(), "hello-world-plugin")
	buildBinary(t, filepath.Join(repoRoot, "plugins", "hello-world"), binaryPath)

	configPath := filepath.Join(t.TempDir(), "plugins.json")
	lockPath := filepath.Join(t.TempDir(), "plugins.lock.json")

	config := pluginregistry.ConfigFile{
		Schema: pluginregistry.ConfigSchemaVersion,
		Plugins: map[string]pluginregistry.PluginEntry{
			"hello-world": {
				Manifest: filepath.Join(repoRoot, "plugins", "hello-world", "manifest.json"),
				Exec:     pluginregistry.ExecSpec{Command: []string{binaryPath}},
				AllowCapabilities: []string{
					"inventory.hello_world.message",
					"action.hello_world.echo",
				},
				Grants: pluginregistry.Grants{
					ObserveLog: true,
				},
			},
		},
	}

	writeJSONFile(t, configPath, config)
	loaded, err := internalpluginregistry.Load(configPath, "")
	if err != nil {
		t.Fatalf("load hello-world plugin registry: %v", err)
	}

	lock := pluginregistry.LockFile{
		Schema:  pluginregistry.LockSchemaVersion,
		Plugins: make(map[string]pluginregistry.LockEntry, len(loaded.Plugins)),
	}
	for id, plugin := range loaded.Plugins {
		lock.Plugins[id] = pluginregistry.LockEntry{
			ManifestSHA256:   plugin.ManifestSHA256,
			ExecutableSHA256: plugin.ExecutableSHA256,
		}
	}
	if err := pluginregistry.WriteLockFile(lockPath, lock); err != nil {
		t.Fatalf("write hello-world plugin lock: %v", err)
	}

	bundle, err := builtin.NewBundle()
	if err != nil {
		t.Fatalf("builtins: %v", err)
	}
	catalog := bundle.Catalog
	matchers := bundle.Matchers

	plugins, err := theater.LoadPluginCatalog(catalog, matchers, configPath, lockPath)
	if err != nil {
		t.Fatalf("load plugin catalog: %v", err)
	}

	spec := theater.StageSpec{
		ID: "plugin-hello-world",
		Scenarios: []theater.ScenarioSpec{{
			ID: "hello",
			Acts: []theater.ActSpec{{
				ID: "greet",
				Properties: map[string]theater.PropertySpec{
					"message": {
						Inventory: &theater.InventoryCall{
							Use: "inventory.hello_world.message",
							With: map[string]theater.BindingSpec{
								"greeting": literalBinding("Hello"),
								"name":     literalBinding("Theater"),
							},
						},
					},
				},
				Action: theater.ActionSpec{
					Use: "action.hello_world.echo",
					With: map[string]theater.BindingSpec{
						"message": refBinding("message"),
					},
				},
				Expectations: []theater.ExpectationSpec{{
					ID:      "message",
					Subject: theater.SubjectSpec{Field: "message"},
					Assert: theater.AssertSpec{
						Ref: builtinexpectation.EqualRef,
						Args: map[string]theater.BindingSpec{
							"expected": literalBinding("Hello, Theater!"),
						},
					},
				}},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{
			ID:         "hello",
			ScenarioID: "hello",
		}},
	}

	diagnostics := theater.NewValidator(plugins, plugins).Validate(spec)
	if len(diagnostics) != 0 {
		t.Fatalf("validate stage: %#v", diagnostics)
	}

	result, err := theater.NewRunner(plugins, plugins).Run(context.Background(), spec, theater.RunOptions{})
	if err != nil {
		t.Fatalf("run stage: %v", err)
	}
	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}
