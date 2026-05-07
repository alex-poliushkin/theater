package theater_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex-poliushkin/theater"
)

func TestValidateRejectsMutatingStateActionInsideEventually(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{{
			ID: "claim",
			Acts: []theater.ActSpec{{
				ID:         "reserve",
				Eventually: &theater.EventuallySpec{Timeout: "5s", Interval: "1s"},
				Action: theater.ActionSpec{
					Use: "action.state.claim",
				},
				Expectations: []theater.ExpectationSpec{{
					ID:      "noop",
					Subject: theater.SubjectSpec{Field: "item"},
					Assert:  theater.AssertSpec{Ref: "expectation.equal", Args: map[string]theater.BindingSpec{"expected": {Kind: theater.BindingKindLiteral, Value: map[string]any{}}}},
				}},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "claim-user", ScenarioID: "claim"}},
	}

	diagnostics := validateStage(spec, nil, nil)
	if !hasDiagnosticCode(diagnostics, "state_mutation_inside_eventually") {
		t.Fatalf("expected state_mutation_inside_eventually diagnostic, got %#v", diagnostics)
	}
}

func TestValidateRejectsStateGuaranteeMismatch(t *testing.T) {
	t.Parallel()

	catalog, matchers, err := newBuiltins()
	if err != nil {
		t.Fatalf("new builtins failed: %v", err)
	}

	spec := theater.StageSpec{
		ID: "main",
		State: &theater.StateSpec{
			Backends: map[string]theater.StateBackendSpec{
				"local": {Use: "state.backend.file", With: map[string]any{"root": t.TempDir()}},
			},
		},
		Scenarios: []theater.ScenarioSpec{{
			ID: "claim",
			Acts: []theater.ActSpec{{
				ID: "reserve",
				Properties: map[string]theater.PropertySpec{
					"pool_handle": {
						Inventory: &theater.InventoryCall{
							Use: "inventory.state.pool",
							With: map[string]theater.BindingSpec{
								"backend":       {Kind: theater.BindingKindLiteral, Value: "local"},
								"pool":          {Kind: theater.BindingKindLiteral, Value: "otp"},
								"min_guarantee": {Kind: theater.BindingKindLiteral, Value: "shared-atomic"},
							},
						},
					},
				},
				Action: theater.ActionSpec{Use: "action.command"},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "claim-user", ScenarioID: "claim"}},
	}

	diagnostics := validateStage(spec, catalog, matchers)
	if !hasDiagnosticCode(diagnostics, "insufficient_state_backend_guarantee") {
		t.Fatalf("expected insufficient_state_backend_guarantee diagnostic, got %#v", diagnostics)
	}
}

func TestValidateRejectsWrongStateHandleRefKind(t *testing.T) {
	t.Parallel()

	catalog, matchers, err := newBuiltins()
	if err != nil {
		t.Fatalf("new builtins failed: %v", err)
	}

	spec := theater.StageSpec{
		ID: "main",
		State: &theater.StateSpec{
			Backends: map[string]theater.StateBackendSpec{
				"local": {Use: "state.backend.file", With: map[string]any{"root": t.TempDir()}},
			},
		},
		Scenarios: []theater.ScenarioSpec{{
			ID: "probe",
			Acts: []theater.ActSpec{{
				ID: "read",
				Properties: map[string]theater.PropertySpec{
					"pool_handle": {
						Inventory: &theater.InventoryCall{
							Use: "inventory.state.pool",
							With: map[string]theater.BindingSpec{
								"backend": {Kind: theater.BindingKindLiteral, Value: "local"},
								"pool":    {Kind: theater.BindingKindLiteral, Value: "otp"},
							},
						},
					},
				},
				Action: theater.ActionSpec{
					Use: "action.state.read",
					With: map[string]theater.BindingSpec{
						"record": {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "pool_handle"}},
					},
				},
			}},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "probe-user", ScenarioID: "probe"}},
	}

	diagnostics := validateStage(spec, catalog, matchers)
	if !hasDiagnosticCode(diagnostics, "incompatible_state_handle_ref") {
		t.Fatalf("expected incompatible_state_handle_ref diagnostic, got %#v", diagnostics)
	}
}

func TestRunnerClaimsAndConsumesFileStateFixture(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pools"), 0o755); err != nil {
		t.Fatalf("mkdir pools failed: %v", err)
	}

	poolDocument := map[string]any{
		"pool": "otp",
		"items": []map[string]any{
			{
				"id":      "fixture-1",
				"fields":  map[string]any{"email": "demo@example.test", "phone": "+15550000001"},
				"state":   "available",
				"version": 0,
			},
		},
	}
	poolData, err := json.Marshal(poolDocument)
	if err != nil {
		t.Fatalf("marshal pool failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "pools", "otp.json"), poolData, 0o644); err != nil {
		t.Fatalf("write pool failed: %v", err)
	}

	catalog, matchers, err := newBuiltins()
	if err != nil {
		t.Fatalf("new builtins failed: %v", err)
	}

	spec := theater.StageSpec{
		ID: "main",
		State: &theater.StateSpec{
			Backends: map[string]theater.StateBackendSpec{
				"local": {Use: "state.backend.file", With: map[string]any{"root": root}},
			},
		},
		Scenarios: []theater.ScenarioSpec{{
			ID: "claim",
			Acts: []theater.ActSpec{
				{
					ID: "reserve",
					Properties: map[string]theater.PropertySpec{
						"otp_pool": {
							Inventory: &theater.InventoryCall{
								Use: "inventory.state.pool",
								With: map[string]theater.BindingSpec{
									"backend":       {Kind: theater.BindingKindLiteral, Value: "local"},
									"pool":          {Kind: theater.BindingKindLiteral, Value: "otp"},
									"min_guarantee": {Kind: theater.BindingKindLiteral, Value: "local-atomic"},
								},
							},
						},
					},
					Action: theater.ActionSpec{
						Use: "action.state.claim",
						With: map[string]theater.BindingSpec{
							"pool": {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "otp_pool"}},
							"lease": {
								Kind: theater.BindingKindObject,
								Object: map[string]theater.BindingSpec{
									"ttl": {Kind: theater.BindingKindLiteral, Value: "1m"},
								},
							},
						},
					},
					Exports: []theater.ExportSpec{
						{As: "otp_claim", Field: "claim"},
						{As: "otp_item", Field: "item"},
					},
					Transitions: []theater.TransitionSpec{{On: theater.TransitionOnPass, To: "consume"}},
				},
				{
					ID: "consume",
					Action: theater.ActionSpec{
						Use: "action.state.consume",
						With: map[string]theater.BindingSpec{
							"claim": {Kind: theater.BindingKindRef, Ref: &theater.RefSpec{Name: "otp_claim"}},
							"tombstone": {
								Kind: theater.BindingKindObject,
								Object: map[string]theater.BindingSpec{
									"status": {Kind: theater.BindingKindLiteral, Value: "registered"},
								},
							},
						},
					},
				},
			},
		}},
		ScenarioCalls: []theater.ScenarioCallSpec{{ID: "claim-user", ScenarioID: "claim"}},
	}

	result, err := runStage(context.Background(), spec, catalog, matchers)
	if err != nil {
		t.Fatalf("run stage failed: %v", err)
	}
	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("run status mismatch: got %s want %s", got, want)
	}

	data, err := os.ReadFile(filepath.Join(root, "pools", "otp.json"))
	if err != nil {
		t.Fatalf("read pool failed: %v", err)
	}

	if !strings.Contains(string(data), `"state": "used"`) {
		t.Fatalf("expected consumed fixture state in pool document, got %s", data)
	}
}

func hasDiagnosticCode(diagnostics []theater.Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}

	return false
}
