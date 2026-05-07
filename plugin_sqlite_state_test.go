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

func TestRunnerUsesSQLitePluginStateBackend(t *testing.T) {
	t.Parallel()

	repoRoot := repoRoot(t)
	sqliteBinary := filepath.Join(t.TempDir(), "sqlite-plugin")
	buildBinary(t, filepath.Join(repoRoot, "plugins", "sqlite"), sqliteBinary)

	configPath := filepath.Join(t.TempDir(), "plugins.json")
	lockPath := filepath.Join(t.TempDir(), "plugins.lock.json")
	sqlitePath := filepath.Join(t.TempDir(), "state.sqlite")

	config := pluginregistry.ConfigFile{
		Schema: pluginregistry.ConfigSchemaVersion,
		Plugins: map[string]pluginregistry.PluginEntry{
			"sqlite": {
				Manifest: filepath.Join(repoRoot, "plugins", "sqlite", "manifest.json"),
				Exec:     pluginregistry.ExecSpec{Command: []string{sqliteBinary}},
				AllowCapabilities: []string{
					"inventory.sqlite.query",
					"action.sqlite.seed.reset",
					"state_backend.sqlite",
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
								"reset_state": []any{
									"create table if not exists theater_state_records (record_key text primary key, version integer not null, value_json text not null)",
									"create table if not exists theater_state_pool_items (pool_name text not null, item_id text not null, fields_json text not null, state text not null, claim_id text not null default '', expires_at text not null default '', expiry_policy text not null default '', tombstone_json text not null default '{}', version integer not null, primary key (pool_name, item_id))",
									"delete from theater_state_records",
									"delete from theater_state_pool_items",
									"insert into theater_state_records(record_key, version, value_json) values ('settings', 1, '{\"token\":\"initial\"}')",
									"insert into theater_state_pool_items(pool_name, item_id, fields_json, state, claim_id, expires_at, expiry_policy, tombstone_json, version) values ('fixtures', 'fixture-1', '{\"email\":\"fixture-1@example.test\",\"purpose\":\"registration\"}', 'available', '', '', '', '{}', 0)",
									"insert into theater_state_pool_items(pool_name, item_id, fields_json, state, claim_id, expires_at, expiry_policy, tombstone_json, version) values ('fixtures', 'fixture-2', '{\"email\":\"fixture-2@example.test\",\"purpose\":\"other\"}', 'available', '', '', '', '{}', 0)",
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

	valueJSONPath, err := theater.ParseJSONPointer("/value_json")
	if err != nil {
		t.Fatalf("parse settings path: %v", err)
	}
	statePath, err := theater.ParseJSONPointer("/state")
	if err != nil {
		t.Fatalf("parse state path: %v", err)
	}
	tombstonePath, err := theater.ParseJSONPointer("/tombstone_json")
	if err != nil {
		t.Fatalf("parse tombstone path: %v", err)
	}

	spec := theater.StageSpec{
		ID: "plugin-sqlite-state",
		State: &theater.StateSpec{
			Backends: map[string]theater.StateBackendSpec{
				"sqlite": {
					Use:  "state_backend.sqlite",
					With: map[string]any{"profile": "app"},
				},
			},
		},
		Scenarios: []theater.ScenarioSpec{{
			ID: "flow",
			Acts: []theater.ActSpec{
				{
					ID: "seed",
					Action: theater.ActionSpec{
						Use: "action.sqlite.seed.reset",
						With: map[string]theater.BindingSpec{
							"profile": literalBinding("app"),
							"fixture": literalBinding("reset_state"),
						},
					},
					Transitions: []theater.TransitionSpec{{On: theater.TransitionOnPass, To: "read"}},
				},
				{
					ID: "read",
					Properties: map[string]theater.PropertySpec{
						"settings_record": {
							Inventory: &theater.InventoryCall{
								Use: "inventory.state.record",
								With: map[string]theater.BindingSpec{
									"backend": literalBinding("sqlite"),
									"record":  literalBinding("settings"),
								},
							},
						},
					},
					Action: theater.ActionSpec{
						Use: "action.state.read",
						With: map[string]theater.BindingSpec{
							"record": refBinding("settings_record"),
						},
					},
					Exports:     []theater.ExportSpec{{As: "settings_version", Field: "version"}},
					Transitions: []theater.TransitionSpec{{On: theater.TransitionOnPass, To: "update"}},
				},
				{
					ID: "update",
					Properties: map[string]theater.PropertySpec{
						"settings_record": {
							Inventory: &theater.InventoryCall{
								Use: "inventory.state.record",
								With: map[string]theater.BindingSpec{
									"backend": literalBinding("sqlite"),
									"record":  literalBinding("settings"),
								},
							},
						},
					},
					Action: theater.ActionSpec{
						Use: "action.state.update",
						With: map[string]theater.BindingSpec{
							"record":           refBinding("settings_record"),
							"expected_version": refBinding("settings_version"),
							"value": objectBinding(map[string]theater.BindingSpec{
								"token": literalBinding("updated"),
							}),
						},
					},
					Transitions: []theater.TransitionSpec{{On: theater.TransitionOnPass, To: "claim"}},
				},
				{
					ID: "claim",
					Properties: map[string]theater.PropertySpec{
						"fixture_pool": {
							Inventory: &theater.InventoryCall{
								Use: "inventory.state.pool",
								With: map[string]theater.BindingSpec{
									"backend": literalBinding("sqlite"),
									"pool":    literalBinding("fixtures"),
								},
							},
						},
					},
					Action: theater.ActionSpec{
						Use: "action.state.claim",
						With: map[string]theater.BindingSpec{
							"pool": refBinding("fixture_pool"),
							"selector": objectBinding(map[string]theater.BindingSpec{
								"fields": objectBinding(map[string]theater.BindingSpec{
									"purpose": literalBinding("registration"),
								}),
							}),
							"lease": objectBinding(map[string]theater.BindingSpec{
								"ttl": literalBinding("1m"),
							}),
						},
					},
					Exports:     []theater.ExportSpec{{As: "fixture_claim", Field: "claim"}},
					Transitions: []theater.TransitionSpec{{On: theater.TransitionOnPass, To: "consume"}},
				},
				{
					ID: "consume",
					Action: theater.ActionSpec{
						Use: "action.state.consume",
						With: map[string]theater.BindingSpec{
							"claim": refBinding("fixture_claim"),
							"tombstone": objectBinding(map[string]theater.BindingSpec{
								"status": literalBinding("registered"),
							}),
						},
					},
					Transitions: []theater.TransitionSpec{{On: theater.TransitionOnPass, To: "verify"}},
				},
				{
					ID: "verify",
					Properties: map[string]theater.PropertySpec{
						"settings_row": {
							Inventory: &theater.InventoryCall{
								Use: "inventory.sqlite.query",
								With: map[string]theater.BindingSpec{
									"profile": literalBinding("app"),
									"sql":     literalBinding("select value_json from theater_state_records where record_key = 'settings'"),
								},
							},
						},
						"fixture_row": {
							Inventory: &theater.InventoryCall{
								Use: "inventory.sqlite.query",
								With: map[string]theater.BindingSpec{
									"profile": literalBinding("app"),
									"sql": literalBinding(
										"select state, tombstone_json from theater_state_pool_items where pool_name = 'fixtures' and item_id = 'fixture-1'",
									),
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
					Expectations: []theater.ExpectationSpec{
						{
							ID: "record-updated",
							Subject: theater.SubjectSpec{
								From: theater.SubjectFromProperty,
								Ref:  "settings_row",
								Path: valueJSONPath,
							},
							Assert: theater.AssertSpec{
								Ref: builtinexpectation.EqualRef,
								Args: map[string]theater.BindingSpec{
									"expected": literalBinding(`{"token":"updated"}`),
								},
							},
						},
						{
							ID: "fixture-used",
							Subject: theater.SubjectSpec{
								From: theater.SubjectFromProperty,
								Ref:  "fixture_row",
								Path: statePath,
							},
							Assert: theater.AssertSpec{
								Ref: builtinexpectation.EqualRef,
								Args: map[string]theater.BindingSpec{
									"expected": literalBinding("used"),
								},
							},
						},
						{
							ID: "tombstone-written",
							Subject: theater.SubjectSpec{
								From: theater.SubjectFromProperty,
								Ref:  "fixture_row",
								Path: tombstonePath,
							},
							Assert: theater.AssertSpec{
								Ref: builtinexpectation.EqualRef,
								Args: map[string]theater.BindingSpec{
									"expected": literalBinding(`{"status":"registered"}`),
								},
							},
						},
					},
				},
			},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{
			ID:         "flow",
			ScenarioID: "flow",
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
