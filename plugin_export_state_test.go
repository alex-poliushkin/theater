package theater_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	theater "github.com/alex-poliushkin/theater"
	builtin "github.com/alex-poliushkin/theater/builtin"
	builtinexpectation "github.com/alex-poliushkin/theater/builtin/expectation"
	"github.com/alex-poliushkin/theater/observe"
	pluginregistry "github.com/alex-poliushkin/theater/plugin/registry"
)

func TestRunnerExportsReportThroughPluginExporter(t *testing.T) {
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
		ID: "plugin-export",
		Scenarios: []theater.ScenarioSpec{{
			ID: "echo",
			Acts: []theater.ActSpec{{
				ID: "submit",
				Action: theater.ActionSpec{
					Use: "action.smoke.echo",
					With: map[string]theater.BindingSpec{
						"value": literalBinding("hello"),
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
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "echo-call", ScenarioID: "echo"}},
	}

	exportPath := filepath.Join(t.TempDir(), "run-document.json")
	result, err := theater.NewRunner(plugins, plugins).Run(context.Background(), spec, theater.RunOptions{
		ReportExporters: []theater.ReportExportSpec{{
			Ref:  "report_exporter.smoke.write",
			With: theater.Values{"path": exportPath},
		}},
	})
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}
	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	raw, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("read exported document: %v", err)
	}

	var document theater.RunDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode exported document: %v", err)
	}
	if err := document.Validate(); err != nil {
		t.Fatalf("validate exported document: %v", err)
	}
	if got, want := document.Report.StageID, result.Report.StageID; got != want {
		t.Fatalf("exported stage id mismatch: got %q want %q", got, want)
	}
}

func TestRunnerRedactsPluginReportExporterSensitiveConfigError(t *testing.T) {
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
		ID: "plugin-export-redaction",
		Scenarios: []theater.ScenarioSpec{{
			ID: "echo",
			Acts: []theater.ActSpec{{
				ID: "submit",
				Action: theater.ActionSpec{
					Use: "action.smoke.echo",
					With: map[string]theater.BindingSpec{
						"value": literalBinding("hello"),
					},
				},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "echo-call", ScenarioID: "echo"}},
	}

	secret := "leak-report-secret"
	_, err = theater.NewRunner(plugins, plugins).Run(context.Background(), spec, theater.RunOptions{
		ReportExporters: []theater.ReportExportSpec{{
			Ref:  "report_exporter.smoke.write",
			With: theater.Values{"path": secret},
		}},
	})
	if err == nil {
		t.Fatal("expected report exporter error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("report exporter error leaked config value: %q", err)
	}
	if !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("report exporter error missing redaction marker: %q", err)
	}
}

func TestRunnerUsesPluginStateBackend(t *testing.T) {
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

	statePath := filepath.Join(t.TempDir(), "state.json")
	writeJSONFile(t, statePath, map[string]any{
		"records": map[string]any{
			"settings": map[string]any{
				"version": "1",
				"value": map[string]any{
					"token": "initial",
				},
			},
		},
		"pools": map[string]any{
			"fixtures": map[string]any{
				"items": []any{
					map[string]any{
						"id":      "fixture-1",
						"fields":  map[string]any{"email": "fixture-1@example.test", "phone": "+15550000001", "purpose": "registration"},
						"state":   "available",
						"version": "0",
					},
					map[string]any{
						"id":      "fixture-2",
						"fields":  map[string]any{"email": "fixture-2@example.test", "phone": "+15550000002", "purpose": "other"},
						"state":   "available",
						"version": "0",
					},
				},
			},
		},
	})

	spec := theater.StageSpec{
		ID: "plugin-state",
		State: &theater.StateSpec{
			Backends: map[string]theater.StateBackendSpec{
				"plugin": {Use: "state_backend.smoke.file", With: map[string]any{"path": statePath}},
			},
		},
		Scenarios: []theater.ScenarioSpec{{
			ID: "flow",
			Acts: []theater.ActSpec{
				{
					ID: "read",
					Properties: map[string]theater.PropertySpec{
						"settings_record": {
							Inventory: &theater.InventoryCall{
								Use: "inventory.state.record",
								With: map[string]theater.BindingSpec{
									"backend": literalBinding("plugin"),
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
									"backend": literalBinding("plugin"),
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
									"backend": literalBinding("plugin"),
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
				},
			},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "flow-call", ScenarioID: "flow"}},
	}

	diagnostics := theater.NewValidator(plugins, plugins).Validate(spec)
	if len(diagnostics) != 0 {
		t.Fatalf("validate stage: %#v", diagnostics)
	}

	result, err := theater.NewRunner(plugins, plugins).Run(context.Background(), spec, theater.RunOptions{})
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}
	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	raw, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state store: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"token": "updated"`) {
		t.Fatalf("updated record token not found in store: %s", text)
	}
	if !strings.Contains(text, `"state": "used"`) {
		t.Fatalf("consumed fixture state not found in store: %s", text)
	}
}

func TestRunnerRedactsPluginSensitiveFailuresAndDiagnostics(t *testing.T) {
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

	live := &recordingSink{}
	secret := "issued-token"
	spec := theater.StageSpec{
		ID: "plugin-redaction",
		Scenarios: []theater.ScenarioSpec{{
			ID: "fail",
			Acts: []theater.ActSpec{{
				ID: "submit",
				Action: theater.ActionSpec{
					Use: "action.smoke.secret_fail",
					With: map[string]theater.BindingSpec{
						"secret": literalBinding(secret),
					},
				},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "fail-call", ScenarioID: "fail"}},
	}

	result, err := theater.NewRunner(plugins, plugins).Run(context.Background(), spec, theater.RunOptions{Live: live})
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}
	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if result.Report.Failure == nil || result.Report.Failure.Cause == nil {
		t.Fatal("expected failed report cause")
	}
	if strings.Contains(result.Report.Failure.Cause.Error(), secret) {
		t.Fatalf("report failure leaked secret: %q", result.Report.Failure.Cause.Error())
	}

	actionNode := firstActionNode(result.Report.Nodes)
	if actionNode == nil || actionNode.Observations == nil {
		t.Fatal("action observations are required")
	}
	if strings.Contains(actionNode.Failure.Cause.Error(), secret) {
		t.Fatalf("action failure leaked secret: %q", actionNode.Failure.Cause.Error())
	}
	secretOutput, ok := actionNode.Observations.Outputs["secret_echo"]
	if !ok {
		t.Fatalf("action partial output shape lost: %#v", actionNode.Observations.Outputs)
	}
	if secretOutput.Preview == nil || !secretOutput.Preview.Redacted {
		t.Fatalf("action partial output must be redacted: %#v", secretOutput)
	}

	raw, err := json.Marshal(result.Document())
	if err != nil {
		t.Fatalf("marshal run document: %v", err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatalf("run document leaked secret: %s", raw)
	}

	envelopes := live.Snapshot()
	foundDiagnostic := false
	for _, env := range envelopes {
		if env.Kind != observe.KindDiagnostic || env.Diagnostic == nil {
			continue
		}
		foundDiagnostic = true
		if strings.Contains(env.Diagnostic.Message, secret) {
			t.Fatalf("live diagnostic leaked secret: %q", env.Diagnostic.Message)
		}
	}
	if !foundDiagnostic {
		t.Fatal("expected plugin diagnostic envelope")
	}
}

func TestRunnerRedactsPluginTransformRootSensitiveInput(t *testing.T) {
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

	secret := "leak-transform-secret"
	spec := theater.StageSpec{
		ID: "plugin-transform-redaction",
		Scenarios: []theater.ScenarioSpec{{
			ID: "fail",
			Acts: []theater.ActSpec{{
				ID: "transform",
				Properties: map[string]theater.PropertySpec{
					"value": {
						Inventory: &theater.InventoryCall{
							Use: "inventory.smoke.echo",
							With: map[string]theater.BindingSpec{
								"value": literalBinding(secret),
							},
						},
						Decorators: []theater.DecoratorSpec{{
							Use: "transform.smoke.wrap",
							With: map[string]any{
								"prefix": "",
								"suffix": "",
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
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "fail-call", ScenarioID: "fail"}},
	}

	result, err := theater.NewRunner(plugins, plugins).Run(context.Background(), spec, theater.RunOptions{})
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}
	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	if result.Report.Failure == nil || result.Report.Failure.Cause == nil {
		t.Fatal("expected failed report cause")
	}
	if strings.Contains(result.Report.Failure.Cause.Error(), secret) {
		t.Fatalf("report failure leaked transform input: %q", result.Report.Failure.Cause.Error())
	}

	raw, err := json.Marshal(result.Document())
	if err != nil {
		t.Fatalf("marshal run document: %v", err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatalf("run document leaked transform input: %s", raw)
	}
}

func TestRunnerTimesOutHungPluginAction(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for the non-Go smoke plugin")
	}

	paths := preparePluginFixtures(t)
	configPath, lockPath := writePluginRegistryFiles(t, paths)
	config, err := pluginregistry.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("load plugin config: %v", err)
	}
	smoke := config.Plugins["smoke-plugin"]
	smoke.Timeouts.RequestDefault = "50ms"
	smoke.Timeouts.CancelGrace = "25ms"
	config.Plugins["smoke-plugin"] = smoke
	writeJSONFile(t, configPath, config)

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
		ID: "plugin-timeout",
		Scenarios: []theater.ScenarioSpec{{
			ID: "sleep",
			Acts: []theater.ActSpec{{
				ID: "wait",
				Action: theater.ActionSpec{
					Use: "action.smoke.sleep",
					With: map[string]theater.BindingSpec{
						"ms": literalBinding(1000),
					},
				},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "sleep-call", ScenarioID: "sleep"}},
	}

	startedAt := time.Now()
	result, err := theater.NewRunner(plugins, plugins).Run(context.Background(), spec, theater.RunOptions{})
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}
	if got, want := result.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}
	actionNode := firstActionNode(result.Report.Nodes)
	if actionNode == nil || actionNode.Failure == nil {
		t.Fatal("expected action timeout failure")
	}
	if got, want := actionNode.Failure.Kind, theater.FailureKindTimeout; got != want {
		t.Fatalf(
			"failure kind mismatch: got %q want %q cause=%v",
			got,
			want,
			actionNode.Failure.Cause,
		)
	}
	if elapsed := time.Since(startedAt); elapsed >= 500*time.Millisecond {
		t.Fatalf("timeout handling took too long: %s", elapsed)
	}
}

type recordingSink struct {
	mu        sync.Mutex
	envelopes []observe.Envelope
}

func (s *recordingSink) Publish(env observe.Envelope) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.envelopes = append(s.envelopes, env)
	return 0
}

func (s *recordingSink) Snapshot() []observe.Envelope {
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := make([]observe.Envelope, len(s.envelopes))
	copy(cloned, s.envelopes)
	return cloned
}

func firstActionNode(nodes []theater.NodeReport) *theater.NodeReport {
	for i := range nodes {
		if nodes[i].Kind == theater.NodeKindAction {
			return &nodes[i]
		}
	}

	return nil
}
