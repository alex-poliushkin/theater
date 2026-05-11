package theater_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	theater "github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/builtin"
	internalpluginregistry "github.com/alex-poliushkin/theater/internal/pluginregistry"
	pluginregistry "github.com/alex-poliushkin/theater/plugin/registry"
)

func TestRunnerUsesNativePluginActionTransformAndMatcher(t *testing.T) {
	t.Setenv("THEATER_SMOKE_PLUGIN_VALUE", "hello")

	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for the native smoke plugin")
	}

	manifestPath, scriptPath := prepareSmokePlugin(t)
	configPath, lockPath := writeSmokePluginRegistryFiles(t, manifestPath, scriptPath)

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
		ID: "plugin-native",
		Scenarios: []theater.ScenarioSpec{{
			ID: "flow",
			Acts: []theater.ActSpec{{
				ID: "echo",
				Properties: map[string]theater.PropertySpec{
					"value": {
						Inventory: &theater.InventoryCall{
							Use: "inventory.env",
							With: map[string]theater.BindingSpec{
								"name": literalBinding("THEATER_SMOKE_PLUGIN_VALUE"),
							},
						},
						Decorators: []theater.DecoratorSpec{{
							Use: "transform.smoke.wrap",
							With: map[string]any{
								"prefix": "<<",
								"suffix": ">>",
							},
						}},
					},
				},
				Action: theater.ActionSpec{
					Use: "action.smoke.echo",
					With: map[string]theater.BindingSpec{
						"value": refBinding("value"),
					},
				},
				Expectations: []theater.ExpectationSpec{{
					ID:      "wrapped",
					Subject: theater.SubjectSpec{Field: "echo"},
					Assert: theater.AssertSpec{
						Ref: "matcher.smoke.equal",
						Args: map[string]theater.BindingSpec{
							"expected": literalBinding("<<hello>>"),
						},
					},
				}},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{
			ID:         "flow",
			ScenarioID: "flow",
		}},
	}

	result, err := theater.NewRunner(plugins, plugins).Run(context.Background(), spec, theater.RunOptions{})
	if err != nil {
		t.Fatalf("run stage: %v", err)
	}
	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func TestRunnerUsesNativePluginTransformInSelectorPipeline(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for the native smoke plugin")
	}

	manifestPath, scriptPath := prepareSmokePlugin(t)
	configPath, lockPath := writeSmokePluginRegistryFiles(t, manifestPath, scriptPath)

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
		ID: "plugin-transform-selector",
		Scenarios: []theater.ScenarioSpec{{
			ID: "flow",
			Acts: []theater.ActSpec{{
				ID: "echo",
				Action: theater.ActionSpec{
					Use: "action.smoke.echo",
					With: map[string]theater.BindingSpec{
						"value": literalBinding("hello"),
					},
				},
				Expectations: []theater.ExpectationSpec{{
					ID: "wrapped",
					Subject: theater.SubjectSpec{
						Field: "echo",
						Through: []theater.ThroughStepSpec{{
							Transform: &theater.DecoratorSpec{
								Use: "transform.smoke.wrap",
								With: map[string]any{
									"prefix": "<<",
									"suffix": ">>",
								},
							},
						}},
					},
					Assert: theater.AssertSpec{
						Ref: "matcher.smoke.equal",
						Args: map[string]theater.BindingSpec{
							"expected": literalBinding("<<hello>>"),
						},
					},
				}},
				Exports: []theater.ExportSpec{{
					As:    "wrapped",
					Field: "echo",
					Through: []theater.ThroughStepSpec{{
						Transform: &theater.DecoratorSpec{
							Use: "transform.smoke.wrap",
							With: map[string]any{
								"prefix": "<<",
								"suffix": ">>",
							},
						},
					}},
				}},
				Transitions: []theater.TransitionSpec{{
					On: theater.TransitionOnPass,
					To: "verify-export",
				}},
			}, {
				ID: "verify-export",
				Action: theater.ActionSpec{
					Use: "action.smoke.echo",
					With: map[string]theater.BindingSpec{
						"value": refBinding("wrapped"),
					},
				},
				Expectations: []theater.ExpectationSpec{{
					ID:      "exported",
					Subject: theater.SubjectSpec{Field: "echo"},
					Assert: theater.AssertSpec{
						Ref: "matcher.smoke.equal",
						Args: map[string]theater.BindingSpec{
							"expected": literalBinding("<<hello>>"),
						},
					},
				}},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{
			ID:         "flow",
			ScenarioID: "flow",
		}},
	}

	result, err := theater.NewRunner(plugins, plugins).Run(context.Background(), spec, theater.RunOptions{})
	if err != nil {
		t.Fatalf("run stage: %v", err)
	}
	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func prepareSmokePlugin(t *testing.T) (string, string) {
	t.Helper()

	root := repoRoot(t)
	tempDir := t.TempDir()
	scriptPath := filepath.Join(tempDir, "smoke.py")

	raw, err := os.ReadFile(filepath.Join(root, "testdata", "plugins", "smoke", "smoke.py"))
	if err != nil {
		t.Fatalf("read smoke plugin: %v", err)
	}
	if err := os.WriteFile(scriptPath, raw, 0o755); err != nil {
		t.Fatalf("write smoke plugin: %v", err)
	}

	return filepath.Join(root, "testdata", "plugins", "smoke", "manifest.json"), scriptPath
}

func writeSmokePluginRegistryFiles(t *testing.T, manifestPath, scriptPath string) (string, string) {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), "plugins.json")
	lockPath := filepath.Join(t.TempDir(), "plugins.lock.json")

	config := pluginregistry.ConfigFile{
		Schema: pluginregistry.ConfigSchemaVersion,
		Plugins: map[string]pluginregistry.PluginEntry{
			"smoke-plugin": {
				Manifest: manifestPath,
				Exec: pluginregistry.ExecSpec{
					Command: []string{scriptPath},
				},
				AllowCapabilities: []string{
					"action.smoke.echo",
					"transform.smoke.wrap",
					"matcher.smoke.equal",
				},
				Grants: pluginregistry.Grants{
					Env: map[string]string{
						"PATH": os.Getenv("PATH"),
					},
				},
			},
		},
	}

	writeJSONFile(t, configPath, config)
	loaded, err := internalpluginregistry.Load(configPath, "")
	if err != nil {
		t.Fatalf("load smoke plugin registry: %v", err)
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
		t.Fatalf("write smoke plugin lock: %v", err)
	}

	return configPath, lockPath
}
