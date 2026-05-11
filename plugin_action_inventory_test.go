package theater_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	theater "github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/builtin"
	builtinexpectation "github.com/alex-poliushkin/theater/builtin/expectation"
	internalpluginregistry "github.com/alex-poliushkin/theater/internal/pluginregistry"
	pluginregistry "github.com/alex-poliushkin/theater/plugin/registry"
)

func TestValidatorRunsPluginValidateHooks(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for the non-Go smoke plugin")
	}

	paths := preparePluginFixtures(t)
	configPath, lockPath := writePluginRegistryFiles(t, paths)
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
		ID: "plugin-validate",
		Scenarios: []theater.ScenarioSpec{{
			ID: "validate",
			Acts: []theater.ActSpec{{
				ID: "query",
				Properties: map[string]theater.PropertySpec{
					"user": {
						Inventory: &theater.InventoryCall{
							Use: "inventory.smoke.echo",
							With: map[string]theater.BindingSpec{
								"value": literalBinding("invalid"),
							},
						},
					},
				},
				Action: theater.ActionSpec{
					Use: "action.generate",
					With: map[string]theater.BindingSpec{
						"outputs": objectBinding(map[string]theater.BindingSpec{}),
					},
				},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{
			ID:         "validate",
			ScenarioID: "validate",
		}},
	}

	diagnostics := theater.NewValidator(plugins, plugins).Validate(spec)
	if len(diagnostics) == 0 {
		t.Fatal("expected plugin validation diagnostic")
	}
	if got, want := diagnostics[0].Code, "plugin_validate_diagnostic"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostics[0].Path, "stage.plugin-validate/scenario.validate/act.query/property.user/inventory/with.value"; got != want {
		t.Fatalf("diagnostic path mismatch: got %q want %q", got, want)
	}
}

func TestValidatorPassesDynamicBindingShapeToPluginValidateHook(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for the non-Go smoke plugin")
	}

	paths := preparePluginFixtures(t)
	configPath, lockPath := writePluginRegistryFiles(t, paths)
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

	diagnostics := theater.NewValidator(plugins, plugins).Validate(pluginValidateProbeSpec("assert-validate-shape", false))
	if len(diagnostics) == 0 {
		t.Fatal("expected plugin validation diagnostic")
	}
	if got, want := diagnostics[0].Code, "plugin_validate_diagnostic"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got := diagnostics[0].Summary; got != "shape ok: static, nested dynamic, list dynamic, missing absent" {
		t.Fatalf("diagnostic summary mismatch: got %q", got)
	}
}

func TestRunnerPassesDynamicBindingShapeToPluginPrepareHook(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for the non-Go smoke plugin")
	}

	paths := preparePluginFixtures(t)
	configPath, lockPath := writePluginRegistryFiles(t, paths)
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

	result, err := theater.NewRunner(plugins, plugins).Run(
		context.Background(),
		pluginValidateProbeSpec("assert-prepare-shape", true),
		theater.RunOptions{},
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}
	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
}

func TestValidatorRedactsPluginValidateSensitiveStaticProperties(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for the non-Go smoke plugin")
	}

	paths := preparePluginFixtures(t)
	configPath, lockPath := writePluginRegistryFiles(t, paths)
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

	diagnostics := theater.NewValidator(plugins, plugins).Validate(pluginValidateProbeSpec("leak-validate-secret", false))
	if len(diagnostics) == 0 {
		t.Fatal("expected plugin validation diagnostic")
	}
	if got := diagnostics[0].Summary; strings.Contains(got, "validate-secret") {
		t.Fatalf("validate diagnostic leaked secret: %q", got)
	}
	if got := diagnostics[0].Summary; !strings.Contains(got, "[redacted]") {
		t.Fatalf("validate diagnostic missing redaction marker: %q", got)
	}
	if got := diagnostics[0].Path; strings.Contains(got, "validate-secret") {
		t.Fatalf("validate diagnostic path leaked secret: %q", got)
	}
	if got := diagnostics[0].Path; !strings.Contains(got, "[redacted]") {
		t.Fatalf("validate diagnostic path missing redaction marker: %q", got)
	}
}

func TestValidatorRedactsPluginValidateSensitiveStaticPropertyError(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for the non-Go smoke plugin")
	}

	paths := preparePluginFixtures(t)
	configPath, lockPath := writePluginRegistryFiles(t, paths)
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

	diagnostics := theater.NewValidator(plugins, plugins).Validate(pluginValidateProbeSpec("leak-validate-error-secret", false))
	if len(diagnostics) == 0 {
		t.Fatal("expected plugin validation diagnostic")
	}
	if got, want := diagnostics[0].Code, "plugin_validate_call_failed"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got := diagnostics[0].Summary; strings.Contains(got, "validate-secret") {
		t.Fatalf("validate error leaked secret: %q", got)
	}
	if got := diagnostics[0].Summary; !strings.Contains(got, "[redacted]") {
		t.Fatalf("validate error missing redaction marker: %q", got)
	}
}

func TestRunnerRedactsPluginPrepareSensitiveStaticPropertyError(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for the non-Go smoke plugin")
	}

	paths := preparePluginFixtures(t)
	configPath, lockPath := writePluginRegistryFiles(t, paths)
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

	result, err := theater.NewRunner(plugins, plugins).Run(
		context.Background(),
		pluginValidateProbeSpec("leak-prepare-secret", false),
		theater.RunOptions{},
	)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}
	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if result.Report.Failure == nil || result.Report.Failure.Cause == nil {
		t.Fatal("expected prepare failure cause")
	}
	if got := result.Report.Failure.Cause.Error(); strings.Contains(got, "validate-secret") {
		t.Fatalf("prepare error leaked secret: %q", got)
	}
	if got := result.Report.Failure.Cause.Error(); !strings.Contains(got, "[redacted]") {
		t.Fatalf("prepare error missing redaction marker: %q", got)
	}
}

func TestRunnerExecutesPluginInventoriesAndActions(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for the non-Go smoke plugin")
	}

	paths := preparePluginFixtures(t)
	configPath, lockPath := writePluginRegistryFiles(t, paths)
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
		ID: "plugin-run",
		Scenarios: []theater.ScenarioSpec{{
			ID: "flow",
			Acts: []theater.ActSpec{{
				ID: "smoke",
				Properties: map[string]theater.PropertySpec{
					"message": {
						Inventory: &theater.InventoryCall{
							Use: "inventory.smoke.echo",
							With: map[string]theater.BindingSpec{
								"value": literalBinding("hello"),
							},
						},
					},
				},
				Action: theater.ActionSpec{
					Use: "action.smoke.echo",
					With: map[string]theater.BindingSpec{
						"value": refBinding("message"),
					},
				},
				Expectations: []theater.ExpectationSpec{{
					ID:      "echo",
					Subject: theater.SubjectSpec{Field: "echo"},
					Assert: theater.AssertSpec{
						Ref: builtinexpectation.EqualRef,
						Args: map[string]theater.BindingSpec{
							"expected": literalBinding("hello"),
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

func TestRunnerExecutesReferenceSQLitePlugin(t *testing.T) {
	t.Parallel()

	repoRoot := repoRoot(t)
	sqliteBinary := filepath.Join(t.TempDir(), "sqlite-plugin")
	buildBinary(t, filepath.Join(repoRoot, "plugins", "sqlite"), sqliteBinary)

	configPath := filepath.Join(t.TempDir(), "plugins.json")
	lockPath := filepath.Join(t.TempDir(), "plugins.lock.json")
	sqlitePath := filepath.Join(t.TempDir(), "fixtures.sqlite")

	config := pluginregistry.ConfigFile{
		Schema: pluginregistry.ConfigSchemaVersion,
		Plugins: map[string]pluginregistry.PluginEntry{
			"sqlite": {
				Manifest: filepath.Join(repoRoot, "plugins", "sqlite", "manifest.json"),
				Exec:     pluginregistry.ExecSpec{Command: []string{sqliteBinary}},
				AllowCapabilities: []string{
					"inventory.sqlite.query",
					"action.sqlite.seed.reset",
				},
				Grants: pluginregistry.Grants{
					ObserveLog:      true,
					ObserveProgress: true,
				},
				Config: map[string]any{
					"profiles": map[string]any{
						"app": map[string]any{
							"dsn": sqlitePath,
							"fixtures": map[string]any{
								"reset_users": []any{
									"create table if not exists users (id integer primary key, email text not null)",
									"delete from users",
									"insert into users(id, email) values (1, 'user@example.test')",
								},
							},
						},
					},
				},
			},
		},
	}

	writeJSONFile(t, configPath, config)
	loaded, err := internalpluginregistry.Load(configPath, "")
	if err != nil {
		t.Fatalf("load sqlite plugin registry: %v", err)
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
		t.Fatalf("write sqlite plugin lock: %v", err)
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
	emailPath, err := theater.ParseJSONPointer("/email")
	if err != nil {
		t.Fatalf("parse JSON pointer: %v", err)
	}

	spec := theater.StageSpec{
		ID: "plugin-sqlite",
		Scenarios: []theater.ScenarioSpec{{
			ID: "flow",
			Acts: []theater.ActSpec{
				{
					ID: "reset",
					Action: theater.ActionSpec{
						Use: "action.sqlite.seed.reset",
						With: map[string]theater.BindingSpec{
							"profile": literalBinding("app"),
							"fixture": literalBinding("reset_users"),
						},
					},
					Expectations: []theater.ExpectationSpec{{
						ID:      "rows",
						Subject: theater.SubjectSpec{Field: "rows_affected"},
						Assert: theater.AssertSpec{
							Ref: builtinexpectation.GTERef,
							Args: map[string]theater.BindingSpec{
								"expected": literalBinding(1),
							},
						},
					}},
					Transitions: []theater.TransitionSpec{{On: theater.TransitionOnPass, To: "verify"}},
				},
				{
					ID: "verify",
					Properties: map[string]theater.PropertySpec{
						"user": {
							Inventory: &theater.InventoryCall{
								Use: "inventory.sqlite.query",
								With: map[string]theater.BindingSpec{
									"profile":    literalBinding("app"),
									"sql":        literalBinding("select email from users where id = 1"),
									"expect_one": literalBinding(true),
								},
							},
						},
					},
					Action: theater.ActionSpec{
						Use: "action.generate",
						With: map[string]theater.BindingSpec{
							"outputs": objectBinding(map[string]theater.BindingSpec{}),
						},
					},
					Expectations: []theater.ExpectationSpec{{
						ID: "email",
						Subject: theater.SubjectSpec{
							From: theater.SubjectFromProperty,
							Ref:  "user",
							Path: emailPath,
						},
						Assert: theater.AssertSpec{
							Ref: builtinexpectation.EqualRef,
							Args: map[string]theater.BindingSpec{
								"expected": literalBinding("user@example.test"),
							},
						},
					}},
				},
			},
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

type pluginFixturePaths struct {
	smokeScript   string
	smokeManifest string
}

func preparePluginFixtures(t *testing.T) pluginFixturePaths {
	t.Helper()

	repoRoot := repoRoot(t)
	tempDir := t.TempDir()

	smokeScript := filepath.Join(tempDir, "smoke.py")
	raw, err := os.ReadFile(filepath.Join(repoRoot, "testdata", "plugins", "smoke", "smoke.py"))
	if err != nil {
		t.Fatalf("read smoke plugin: %v", err)
	}
	if err := os.WriteFile(smokeScript, raw, 0o755); err != nil {
		t.Fatalf("write smoke plugin: %v", err)
	}

	return pluginFixturePaths{
		smokeScript:   smokeScript,
		smokeManifest: filepath.Join(repoRoot, "testdata", "plugins", "smoke", "manifest.json"),
	}
}

func writePluginRegistryFiles(t *testing.T, paths pluginFixturePaths) (string, string) {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), "plugins.json")
	lockPath := filepath.Join(t.TempDir(), "plugins.lock.json")

	config := pluginregistry.ConfigFile{
		Schema: pluginregistry.ConfigSchemaVersion,
		Plugins: map[string]pluginregistry.PluginEntry{
			"smoke-plugin": {
				Manifest: paths.smokeManifest,
				Exec:     pluginregistry.ExecSpec{Command: []string{paths.smokeScript}},
				AllowCapabilities: []string{
					"inventory.smoke.echo",
					"action.smoke.echo",
					"action.smoke.secret_fail",
					"action.smoke.sleep",
					"action.smoke.validate_probe",
					"report_exporter.smoke.write",
					"state_backend.smoke.file",
					"transform.smoke.wrap",
					"matcher.smoke.equal",
				},
				Grants: pluginregistry.Grants{
					ObserveLog: true,
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
		t.Fatalf("load plugin registry: %v", err)
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
		t.Fatalf("write plugin lock: %v", err)
	}

	return configPath, lockPath
}

func buildBinary(t *testing.T, workdir string, target string) {
	t.Helper()

	cmd := exec.Command("go", "build", "-o", target, ".")
	cmd.Dir = workdir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build plugin in %s: %v\n%s", workdir, err, output)
	}
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()

	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("encode JSON %s: %v", path, err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func pluginValidateProbeSpec(mode string, expectPrepared bool) theater.StageSpec {
	expectations := []theater.ExpectationSpec(nil)
	if expectPrepared {
		expectations = []theater.ExpectationSpec{{
			ID:      "prepared",
			Subject: theater.SubjectSpec{Field: "prepared"},
			Assert: theater.AssertSpec{
				Ref: builtinexpectation.EqualRef,
				Args: map[string]theater.BindingSpec{
					"expected": literalBinding(true),
				},
			},
		}}
	}

	return theater.StageSpec{
		ID: "plugin-validate-probe",
		Scenarios: []theater.ScenarioSpec{{
			ID: "probe",
			Inputs: map[string]theater.ValueContract{
				"runtime_value": {Kind: theater.ValueKindString, Required: true},
			},
			Acts: []theater.ActSpec{{
				ID: "check",
				Action: theater.ActionSpec{
					Use: "action.smoke.validate_probe",
					With: map[string]theater.BindingSpec{
						"mode":    literalBinding(mode),
						"literal": literalBinding("static"),
						"secret":  literalBinding("validate-secret"),
						"dynamic": refBinding("runtime_value"),
						"object": objectBinding(map[string]theater.BindingSpec{
							"literal": literalBinding("nested-static"),
							"dynamic": refBinding("runtime_value"),
						}),
						"items": listBinding(
							literalBinding("first"),
							refBinding("runtime_value"),
						),
					},
				},
				Expectations: expectations,
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{
			ID:         "probe-call",
			ScenarioID: "probe",
			Bindings: map[string]theater.BindingSpec{
				"runtime_value": literalBinding("resolved-at-runtime"),
			},
		}},
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve repo root: caller unavailable")
	}

	return filepath.Dir(file)
}

func literalBinding(value any) theater.BindingSpec {
	return theater.BindingSpec{
		Kind:  theater.BindingKindLiteral,
		Value: value,
	}
}

func objectBinding(fields map[string]theater.BindingSpec) theater.BindingSpec {
	return theater.BindingSpec{
		Kind:   theater.BindingKindObject,
		Object: fields,
	}
}

func listBinding(items ...theater.BindingSpec) theater.BindingSpec {
	return theater.BindingSpec{
		Kind: theater.BindingKindList,
		List: items,
	}
}

func refBinding(name string) theater.BindingSpec {
	return theater.BindingSpec{
		Kind: theater.BindingKindRef,
		Ref: &theater.RefSpec{
			Name: name,
		},
	}
}
