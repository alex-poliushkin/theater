package theater_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/testkit"
)

type minimalCatalog struct {
	action theater.Action
}

type minimalMatcherResolver struct {
	descriptor theater.MatcherDescriptor
}

type synchronizedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c minimalCatalog) ResolveAction(ref string) (theater.Action, error) {
	if ref != "action.login" {
		return nil, errors.New("unexpected action ref")
	}

	return c.action, nil
}

func (minimalCatalog) ResolveInventory(string) (theater.Inventory, error) {
	return nil, errors.New("inventory resolver should not be used in this test")
}

func (minimalCatalog) ResolveGenerator(string) (theater.GeneratorDef, error) {
	return theater.GeneratorDef{}, errors.New("generator resolver should not be used in this test")
}

func (minimalCatalog) ResolveDecorator(string) (theater.DecoratorDef, error) {
	return theater.DecoratorDef{}, errors.New("decorator resolver should not be used in this test")
}

func (minimalCatalog) ResolveStateBackend(string) (theater.StateBackendDef, error) {
	return theater.StateBackendDef{}, errors.New("state backend resolver should not be used in this test")
}

func (minimalCatalog) ResolveReportExporter(string) (theater.ReportExporterDef, error) {
	return theater.ReportExporterDef{}, errors.New("report exporter resolver should not be used in this test")
}

func (b *synchronizedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

func (r minimalMatcherResolver) Resolve(ref string) (theater.MatcherDescriptor, error) {
	if ref != r.descriptor.Ref {
		return theater.MatcherDescriptor{}, errors.New("unexpected matcher ref")
	}

	return r.descriptor, nil
}

func TestValidatorValidatesSpec(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID: "submit",
						Action: theater.ActionSpec{
							Use: "action.login",
						},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "token",
								Subject: theater.SubjectSpec{Field: "token"},
								Assert:  theater.AssertSpec{Ref: "expectation.token"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString},
			},
		},
	}
	expectation := &testkit.ScriptedExpectation{}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers := matcherCatalog(t, expectation.Descriptor("expectation.token"))
	diagnostics := theater.NewValidator(catalog, matchers).Validate(spec)
	if len(diagnostics) != 0 {
		t.Fatalf("validator returned diagnostics: %#v", diagnostics)
	}
}

func TestValidatorListDebugPathsReturnsPreparedBoundaries(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID: "submit",
						Eventually: &theater.EventuallySpec{
							Timeout:  "3s",
							Interval: "1s",
						},
						Action: theater.ActionSpec{Use: "action.login", Repeatable: true},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "token",
								Subject: theater.SubjectSpec{Field: "token"},
								Assert:  theater.AssertSpec{Ref: "expectation.token"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	listing, err := theater.NewValidator(catalog, matcherCatalog(t, (&testkit.ScriptedExpectation{}).Descriptor("expectation.token"))).
		ListDebugPaths(spec)
	if err != nil {
		t.Fatalf("ListDebugPaths error = %v", err)
	}
	if len(listing.Diagnostics) != 0 {
		t.Fatalf("ListDebugPaths diagnostics = %#v, want empty", listing.Diagnostics)
	}

	var got []theater.DebugPath
	for _, path := range listing.Paths {
		if path.Path == "stage.main/call.login-user/act.submit" ||
			path.Path == "stage.main/call.login-user/act.submit/action" ||
			path.Path == "stage.main/call.login-user/act.submit/expectation.token" {
			got = append(got, path)
		}
	}

	want := []theater.DebugPath{
		{
			Path:       "stage.main/call.login-user/act.submit",
			Kind:       theater.DebugBoundaryKindAct,
			Phase:      theater.DebugBoundaryPhaseBefore,
			RetryAware: false,
		},
		{
			Path:       "stage.main/call.login-user/act.submit",
			Kind:       theater.DebugBoundaryKindAct,
			Phase:      theater.DebugBoundaryPhaseAfter,
			RetryAware: true,
		},
		{
			Path:       "stage.main/call.login-user/act.submit/action",
			Kind:       theater.DebugBoundaryKindAction,
			Phase:      theater.DebugBoundaryPhaseBefore,
			RetryAware: true,
		},
		{
			Path:       "stage.main/call.login-user/act.submit/action",
			Kind:       theater.DebugBoundaryKindAction,
			Phase:      theater.DebugBoundaryPhaseAfter,
			RetryAware: true,
		},
		{
			Path:       "stage.main/call.login-user/act.submit/expectation.token",
			Kind:       theater.DebugBoundaryKindExpectation,
			Phase:      theater.DebugBoundaryPhaseBefore,
			RetryAware: true,
		},
		{
			Path:       "stage.main/call.login-user/act.submit/expectation.token",
			Kind:       theater.DebugBoundaryKindExpectation,
			Phase:      theater.DebugBoundaryPhaseAfter,
			RetryAware: true,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListDebugPaths mismatch:\n got %#v\nwant %#v", got, want)
	}
}

func TestValidatorListDebugPathsReturnsDiagnosticsForInvalidSpec(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
						Transitions: []theater.TransitionSpec{
							{On: theater.TransitionOnPass, To: "missing"},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", &testkit.ScriptedAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	listing, err := theater.NewValidator(catalog, matcherCatalog(t)).ListDebugPaths(spec)
	if err != nil {
		t.Fatalf("ListDebugPaths error = %v", err)
	}
	if len(listing.Paths) != 0 {
		t.Fatalf("ListDebugPaths paths = %#v, want empty on invalid spec", listing.Paths)
	}
	if len(listing.Diagnostics) == 0 {
		t.Fatal("ListDebugPaths diagnostics = empty, want validation diagnostics")
	}
	if listing.Diagnostics[0].Code != "missing_transition_target" {
		t.Fatalf("ListDebugPaths diagnostic code = %q, want missing_transition_target", listing.Diagnostics[0].Code)
	}
}

func TestRunnerRunsSpec(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "token",
								Subject: theater.SubjectSpec{Field: "token"},
								Assert:  theater.AssertSpec{Ref: "expectation.token"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	action := &testkit.ScriptedAction{
		Output: theater.Outputs{"token": "issued-token"},
	}
	expectation := &testkit.ScriptedExpectation{
		CheckFunc: func(actual any) error {
			if got, want := actual, "issued-token"; got != want {
				t.Fatalf("token mismatch: got %v want %v", got, want)
			}

			return nil
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers := matcherCatalog(t, expectation.Descriptor("expectation.token"))
	got, err := theater.NewRunner(catalog, matchers).Run(context.Background(), spec, theater.RunOptions{})
	if err != nil {
		t.Fatalf("runner run failed: %v", err)
	}

	if got, want := got.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("run status mismatch: got %s want %s", got, want)
	}

	if len(got.Report.Nodes) == 0 {
		t.Fatal("runner returned empty report")
	}
}

func TestRunnerRunOptionsEnableDebugDump(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "token",
								Subject: theater.SubjectSpec{Field: "token"},
								Assert:  theater.AssertSpec{Ref: "expectation.token"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	action := &testkit.ScriptedAction{
		Output: theater.Outputs{"token": "issued-token"},
	}
	expectation := &testkit.ScriptedExpectation{
		CheckFunc: func(actual any) error {
			if got, want := actual, "issued-token"; got != want {
				t.Fatalf("token mismatch: got %v want %v", got, want)
			}

			return nil
		},
	}
	dumpPath := filepath.Join(t.TempDir(), "run.debug.ndjson")

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers := matcherCatalog(t, expectation.Descriptor("expectation.token"))
	got, err := theater.NewRunner(catalog, matchers).Run(context.Background(), spec, theater.RunOptions{
		Debug: &theater.DebugOptions{
			Mode:        theater.DebugModeDump,
			Breakpoints: []string{"name=before-submit,kind=action,phase=before,path=**"},
			DumpPath:    dumpPath,
		},
	})
	if err != nil {
		t.Fatalf("runner run failed: %v", err)
	}

	if got, want := got.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("run status mismatch: got %s want %s", got, want)
	}

	data, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read debug dump failed: %v", err)
	}

	dump := string(data)
	if !strings.Contains(dump, `"kind":"pause"`) {
		t.Fatalf("debug dump must contain a pause record: %q", dump)
	}
	if !strings.Contains(dump, `"reason":"breakpoint"`) {
		t.Fatalf("debug dump must contain breakpoint reason: %q", dump)
	}
	if !strings.Contains(dump, `"breakpoint":"before-submit"`) {
		t.Fatalf("debug dump must contain breakpoint label: %q", dump)
	}
}

func TestRunnerInteractiveDebugBeforeActionShowsResolvedPropertyInScope(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
						Properties: map[string]theater.PropertySpec{
							"session": {
								Inventory: &theater.InventoryCall{Use: "inventory.session"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	actionStarted := make(chan struct{}, 1)
	runs := 0
	catalog := theater.NewCatalog()
	if err := catalog.RegisterInventory("inventory.session", &testkit.ScriptedInventory{
		ContractValue: theater.InventoryContract{
			Produces: theater.ValueContract{Kind: theater.ValueKindString},
		},
		Output: "ready",
	}); err != nil {
		t.Fatalf("register inventory failed: %v", err)
	}
	if err := catalog.RegisterAction("action.login", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString},
			},
		},
		RunFunc: func(args theater.Args) (theater.Outputs, error) {
			actionStarted <- struct{}{}
			runs++
			return theater.Outputs{"token": "issued-token"}, nil
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers := matcherCatalog(t)
	inputReader, inputWriter := io.Pipe()
	defer func() {
		_ = inputReader.Close()
		_ = inputWriter.Close()
	}()

	output := &synchronizedBuffer{}
	resultCh := make(chan struct {
		result theater.RunResult
		err    error
	}, 1)
	go func() {
		result, err := theater.NewRunner(catalog, matchers).Run(context.Background(), spec, theater.RunOptions{
			Debug: &theater.DebugOptions{
				Mode:        theater.DebugModeInteractive,
				Breakpoints: []string{"name=before-submit,kind=action,phase=before,path=**"},
				Input:       inputReader,
				Output:      output,
			},
		})
		resultCh <- struct {
			result theater.RunResult
			err    error
		}{result: result, err: err}
	}()

	if _, err := io.WriteString(inputWriter, "inspect scope\n"); err != nil {
		t.Fatalf("write inspect command failed: %v", err)
	}

	text := waitForDebugPromptText(t, output, "session [scope.current]: ready")
	select {
	case <-actionStarted:
		t.Fatal("action started before continue command")
	default:
	}

	if _, err := io.WriteString(inputWriter, "continue\n"); err != nil {
		t.Fatalf("write continue command failed: %v", err)
	}
	if err := inputWriter.Close(); err != nil {
		t.Fatalf("close interactive input failed: %v", err)
	}

	done := <-resultCh
	if done.err != nil {
		t.Fatalf("runner run failed: %v", done.err)
	}
	got := done.result

	if got, want := got.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("run status mismatch: got %s want %s", got, want)
	}
	if runs != 1 {
		t.Fatalf("action run count mismatch: got %d want 1", runs)
	}

	for _, want := range []string{
		"PAUSED breakpoint",
		"kind: action",
		"phase: before",
		"scope:",
		"session [scope.current]: ready",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("interactive debug output missing %q:\n%s", want, text)
		}
	}
}

func TestRunnerRunOptionsTerminalFailureBreakpointWritesArtifact(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID: "submit",
						Action: theater.ActionSpec{
							Use:        "action.login",
							Repeatable: true,
						},
						Eventually: &theater.EventuallySpec{
							Timeout:  "100ms",
							Interval: "1ms",
						},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "token",
								Subject: theater.SubjectSpec{Field: "ready"},
								Assert:  theater.AssertSpec{Ref: "expectation.ready"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	catalog := theater.NewCatalog()
	attempts := 0
	if err := catalog.RegisterAction("action.login", &testkit.ScriptedAction{
		Output: theater.Outputs{"ready": false},
		RunFunc: func(args theater.Args) (theater.Outputs, error) {
			attempts++
			return theater.Outputs{"ready": false}, nil
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	checks := 0
	matchers := matcherCatalog(t, (&testkit.ScriptedExpectation{
		CheckFunc: func(actual any) error {
			checks++
			ready, ok := actual.(bool)
			if !ok {
				t.Fatalf("ready type mismatch: got %T want bool", actual)
			}
			if ready {
				return nil
			}
			if checks == 1 {
				return errors.New("not ready")
			}

			return testkit.TerminalError(errors.New("terminal not ready"))
		},
	}).Descriptor("expectation.ready"))
	dumpPath := filepath.Join(t.TempDir(), "run.debug.ndjson")

	got, err := theater.NewRunner(catalog, matchers).Run(context.Background(), spec, theater.RunOptions{
		Debug: &theater.DebugOptions{
			Mode: theater.DebugModeDump,
			Breakpoints: []string{
				"name=attempt-stop,path=**/expectation.token,kind=expectation,phase=after,when=attempt-failure",
				"name=terminal-stop,path=**/expectation.token,kind=expectation,phase=after,when=terminal-failure",
			},
			DumpPath: dumpPath,
		},
	})
	if err != nil {
		t.Fatalf("runner run failed: %v", err)
	}

	if got, want := got.Report.Status, theater.StatusFailed; got != want {
		t.Fatalf("run status mismatch: got %s want %s", got, want)
	}
	if got, want := attempts, 2; got != want {
		t.Fatalf("retry attempt count mismatch: got %d want %d", got, want)
	}
	if got, want := checks, 2; got != want {
		t.Fatalf("matcher check count mismatch: got %d want %d", got, want)
	}

	records := readDebugArtifactRecords(t, dumpPath)
	attemptRecords := 0
	terminalRecords := 0
	terminalPause := map[string]any(nil)
	for i := range records {
		if got, want := debugArtifactStringField(t, records[i], "kind"), "pause"; got != want {
			t.Fatalf("debug artifact kind mismatch at record %d: got %q want %q", i, got, want)
		}

		pause := debugArtifactObjectField(t, records[i], "pause")
		switch debugArtifactStringField(t, pause, "reason") {
		case "attempt-failure":
			attemptRecords++
			if got, want := debugArtifactStringField(t, pause, "breakpoint"), "attempt-stop"; got != want {
				t.Fatalf("attempt failure breakpoint mismatch: got %q want %q", got, want)
			}
		case "terminal-failure":
			terminalRecords++
			terminalPause = pause
		}
	}
	if got, want := attemptRecords, 2; got != want {
		t.Fatalf("attempt failure record count mismatch: got %d want %d", got, want)
	}
	if got, want := terminalRecords, 1; got != want {
		t.Fatalf("terminal failure record count mismatch: got %d want %d", got, want)
	}

	if got, want := debugArtifactStringField(t, terminalPause, "reason"), "terminal-failure"; got != want {
		t.Fatalf("terminal failure reason mismatch: got %q want %q", got, want)
	}
	if got, want := debugArtifactStringField(t, terminalPause, "breakpoint"), "terminal-stop"; got != want {
		t.Fatalf("terminal failure breakpoint mismatch: got %q want %q", got, want)
	}

	snapshot := debugArtifactObjectField(t, terminalPause, "snapshot")
	ref := debugArtifactObjectField(t, snapshot, "ref")
	if got, want := debugArtifactStringField(t, ref, "kind"), "expectation"; got != want {
		t.Fatalf("terminal failure boundary kind mismatch: got %q want %q", got, want)
	}
	if got, want := debugArtifactStringField(t, ref, "phase"), "after"; got != want {
		t.Fatalf("terminal failure boundary phase mismatch: got %q want %q", got, want)
	}
	if got, want := debugArtifactStringField(t, ref, "path"), "stage.main/call.login-user/act.submit/expectation.token"; got != want {
		t.Fatalf("terminal failure boundary path mismatch: got %q want %q", got, want)
	}
	if got, want := debugArtifactNumberField(t, ref, "attempt"), float64(2); got != want {
		t.Fatalf("terminal failure boundary attempt mismatch: got %.0f want %.0f", got, want)
	}
}

func TestRunnerInteractiveDebugExpectationAfterBreakpointPausesAfterMatcherCheck(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "token",
								Subject: theater.SubjectSpec{Field: "token"},
								Assert:  theater.AssertSpec{Ref: "expectation.token"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", &testkit.ScriptedAction{
		Output: theater.Outputs{"token": "issued-token"},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matcherCalled := make(chan struct{}, 1)
	checks := 0
	matchers := matcherCatalog(t, (&testkit.ScriptedExpectation{
		CheckFunc: func(actual any) error {
			checks++
			matcherCalled <- struct{}{}
			if got, want := actual, "issued-token"; got != want {
				t.Fatalf("token mismatch: got %v want %v", got, want)
			}

			return nil
		},
	}).Descriptor("expectation.token"))

	inputReader, inputWriter := io.Pipe()
	defer func() {
		_ = inputReader.Close()
		_ = inputWriter.Close()
	}()

	output := &synchronizedBuffer{}
	resultCh := make(chan struct {
		result theater.RunResult
		err    error
	}, 1)
	go func() {
		result, err := theater.NewRunner(catalog, matchers).Run(context.Background(), spec, theater.RunOptions{
			Debug: &theater.DebugOptions{
				Mode:        theater.DebugModeInteractive,
				Breakpoints: []string{"name=after-token,kind=expectation,phase=after,path=**/expectation.token"},
				Input:       inputReader,
				Output:      output,
			},
		})
		resultCh <- struct {
			result theater.RunResult
			err    error
		}{result: result, err: err}
	}()

	text := waitForDebugPromptText(t, output, "phase: after")
	select {
	case <-matcherCalled:
	default:
		t.Fatal("matcher check did not run before after-expectation pause")
	}

	if _, err := io.WriteString(inputWriter, "continue\n"); err != nil {
		t.Fatalf("write continue command failed: %v", err)
	}
	if err := inputWriter.Close(); err != nil {
		t.Fatalf("close interactive input failed: %v", err)
	}

	done := <-resultCh
	if done.err != nil {
		t.Fatalf("runner run failed: %v", done.err)
	}
	got := done.result
	if got, want := got.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("run status mismatch: got %s want %s", got, want)
	}
	if got, want := checks, 1; got != want {
		t.Fatalf("matcher call count mismatch: got %d want %d", got, want)
	}

	for _, want := range []string{
		"PAUSED breakpoint",
		"kind: expectation",
		"phase: after",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("interactive debug output missing %q:\n%s", want, text)
		}
	}
}

func TestRunnerRunOptionsInteractiveDebugWritesResumeAndSummary(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", &testkit.ScriptedAction{}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}
	matchers := matcherCatalog(t)
	dumpPath := filepath.Join(t.TempDir(), "interactive.debug.ndjson")
	var prompt bytes.Buffer

	got, err := theater.NewRunner(catalog, matchers).Run(context.Background(), spec, theater.RunOptions{
		Debug: &theater.DebugOptions{
			Mode:        theater.DebugModeInteractive,
			StartPaused: true,
			DumpPath:    dumpPath,
			Input:       strings.NewReader("continue\n"),
			Output:      &prompt,
		},
	})
	if err != nil {
		t.Fatalf("runner run failed: %v", err)
	}
	if got, want := got.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("run status mismatch: got %s want %s", got, want)
	}

	data, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read debug dump failed: %v", err)
	}

	dump := string(data)
	if !strings.Contains(dump, `"kind":"pause"`) {
		t.Fatalf("debug dump must contain a pause record: %q", dump)
	}
	if !strings.Contains(dump, `"kind":"resume"`) {
		t.Fatalf("debug dump must contain a resume record: %q", dump)
	}
	if !strings.Contains(dump, `"command":"continue"`) {
		t.Fatalf("debug dump must contain the resume command: %q", dump)
	}
	if !strings.Contains(dump, `"kind":"summary"`) {
		t.Fatalf("debug dump must contain a session summary: %q", dump)
	}
}

func TestRunnerInteractiveDebugQuitCancelsRunBeforeActionExecutes(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	runs := 0
	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", &testkit.ScriptedAction{
		RunFunc: func(args theater.Args) (theater.Outputs, error) {
			runs++
			return theater.Outputs{"token": "issued-token"}, nil
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers := matcherCatalog(t)
	var prompt bytes.Buffer
	result, err := theater.NewRunner(catalog, matchers).Run(context.Background(), spec, theater.RunOptions{
		Debug: &theater.DebugOptions{
			Mode:        theater.DebugModeInteractive,
			StartPaused: true,
			Input:       strings.NewReader("quit\n"),
			Output:      &prompt,
		},
	})
	if err != nil {
		t.Fatalf("runner run failed: %v", err)
	}
	if got, want := result.Report.Status, theater.StatusCanceled; got != want {
		t.Fatalf("run status mismatch: got %s want %s", got, want)
	}
	if runs != 0 {
		t.Fatalf("action run count mismatch: got %d want 0", runs)
	}
	if !strings.Contains(prompt.String(), "PAUSED start") {
		t.Fatalf("prompt output must include pause banner: %q", prompt.String())
	}
}

func TestRunnerInteractiveDebugPauseHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	runs := 0
	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", &testkit.ScriptedAction{
		RunFunc: func(args theater.Args) (theater.Outputs, error) {
			runs++
			return theater.Outputs{"token": "issued-token"}, nil
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers := matcherCatalog(t)
	inputReader, inputWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("create debug input pipe failed: %v", err)
	}
	defer func() {
		_ = inputReader.Close()
		_ = inputWriter.Close()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	output := &synchronizedBuffer{}
	resultCh := make(chan struct {
		result theater.RunResult
		err    error
	}, 1)
	go func() {
		result, err := theater.NewRunner(catalog, matchers).Run(ctx, spec, theater.RunOptions{
			Debug: &theater.DebugOptions{
				Mode:        theater.DebugModeInteractive,
				StartPaused: true,
				Input:       inputReader,
				Output:      output,
			},
		})
		resultCh <- struct {
			result theater.RunResult
			err    error
		}{result: result, err: err}
	}()

	waitForDebugPromptTextWithin(t, output, "PAUSED start", time.Second)
	cancel()

	select {
	case done := <-resultCh:
		if done.err != nil {
			t.Fatalf("runner run failed: %v", done.err)
		}
		if got, want := done.result.Report.Status, theater.StatusCanceled; got != want {
			t.Fatalf("run status mismatch: got %s want %s", got, want)
		}
		if done.result.Report.Failure != nil {
			t.Fatalf("run failure mismatch: got %#v want nil", done.result.Report.Failure)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for canceled interactive run")
	}

	if runs != 0 {
		t.Fatalf("action run count mismatch: got %d want 0", runs)
	}
}

func TestRunnerInteractiveDebugPauseHonorsContextDeadline(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	runs := 0
	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", &testkit.ScriptedAction{
		RunFunc: func(args theater.Args) (theater.Outputs, error) {
			runs++
			return theater.Outputs{"token": "issued-token"}, nil
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers := matcherCatalog(t)
	inputReader, inputWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("create debug input pipe failed: %v", err)
	}
	defer func() {
		_ = inputReader.Close()
		_ = inputWriter.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	output := &synchronizedBuffer{}
	resultCh := make(chan struct {
		result theater.RunResult
		err    error
	}, 1)
	go func() {
		result, err := theater.NewRunner(catalog, matchers).Run(ctx, spec, theater.RunOptions{
			Debug: &theater.DebugOptions{
				Mode:        theater.DebugModeInteractive,
				StartPaused: true,
				Input:       inputReader,
				Output:      output,
			},
		})
		resultCh <- struct {
			result theater.RunResult
			err    error
		}{result: result, err: err}
	}()

	waitForDebugPromptTextWithin(t, output, "PAUSED start", time.Second)

	select {
	case done := <-resultCh:
		if done.err != nil {
			t.Fatalf("runner run failed: %v", done.err)
		}
		if got, want := done.result.Report.Status, theater.StatusFailed; got != want {
			t.Fatalf("run status mismatch: got %s want %s", got, want)
		}
		if done.result.Report.Failure == nil {
			t.Fatal("run failure = nil, want timeout failure")
		}
		if got, want := done.result.Report.Failure.Kind, theater.FailureKindTimeout; got != want {
			t.Fatalf("run failure kind mismatch: got %s want %s", got, want)
		}
		if got, want := done.result.Report.Failure.Summary, "stage run timed out"; got != want {
			t.Fatalf("run failure summary mismatch: got %q want %q", got, want)
		}
		if !errors.Is(done.result.Report.Failure.Cause, context.DeadlineExceeded) {
			t.Fatalf("run failure cause mismatch: got %v want deadline exceeded", done.result.Report.Failure.Cause)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for deadline-limited interactive run")
	}

	if runs != 0 {
		t.Fatalf("action run count mismatch: got %d want 0", runs)
	}
}

func TestRunnerRunOptionsDebugDumpSafeProjectsSecretAndBoundedInputs(t *testing.T) {
	t.Parallel()

	items := make([]theater.BindingSpec, 0, 70)
	for i := 0; i < 70; i++ {
		items = append(items, theater.BindingSpec{
			Kind:  theater.BindingKindLiteral,
			Value: "item",
		})
	}

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID: "submit",
						Action: theater.ActionSpec{
							Use: "action.login",
							With: map[string]theater.BindingSpec{
								"token": {
									Kind:  theater.BindingKindLiteral,
									Value: "issued-token",
								},
								"note": {
									Kind:  theater.BindingKindLiteral,
									Value: strings.Repeat("a", 5000),
								},
								"items": {
									Kind: theater.BindingKindList,
									List: items,
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	dumpPath := filepath.Join(t.TempDir(), "run.debug.ndjson")
	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Inputs: map[string]theater.ValueContract{
				"token": {
					Kind:        theater.ValueKindString,
					Sensitivity: theater.SensitivitySecret,
				},
				"note": {
					Kind: theater.ValueKindString,
				},
				"items": {
					Kind: theater.ValueKindList,
					Elem: &theater.ValueContract{Kind: theater.ValueKindString},
				},
			},
		},
	}); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	result, err := theater.NewRunner(catalog, matcherCatalog(t)).Run(context.Background(), spec, theater.RunOptions{
		Debug: &theater.DebugOptions{
			Mode:        theater.DebugModeDump,
			Breakpoints: []string{"name=before-submit,kind=action,phase=before,path=**"},
			DumpPath:    dumpPath,
		},
	})
	if err != nil {
		t.Fatalf("runner run failed: %v", err)
	}
	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("run status mismatch: got %s want %s", got, want)
	}

	data, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read debug dump failed: %v", err)
	}
	if strings.Contains(string(data), "issued-token") {
		t.Fatalf("debug dump leaked secret token: %q", string(data))
	}

	records := readDebugArtifactRecords(t, dumpPath)
	pause := debugArtifactObjectField(t, records[0], "pause")
	snapshot := debugArtifactObjectField(t, pause, "snapshot")
	inputs := debugArtifactObjectField(t, snapshot, "inputs")
	fields := debugArtifactFieldArray(t, inputs, "fields")

	tokenField := debugArtifactFieldByKey(t, fields, "token")
	tokenValue := debugArtifactObjectField(t, tokenField, "value")
	if got, want := debugArtifactStringField(t, tokenValue, "text"), "[redacted]"; got != want {
		t.Fatalf("token preview mismatch: got %q want %q", got, want)
	}
	if !debugArtifactBoolField(t, tokenValue, "redacted") {
		t.Fatal("token preview must be marked redacted")
	}

	noteField := debugArtifactFieldByKey(t, fields, "note")
	noteValue := debugArtifactObjectField(t, noteField, "value")
	if !debugArtifactBoolField(t, noteValue, "truncated") {
		t.Fatal("note preview must be marked truncated")
	}
	if got := debugArtifactStringField(t, noteValue, "text"); got == strings.Repeat("a", 5000) {
		t.Fatalf("note preview must be truncated: got %q", got)
	}

	itemsField := debugArtifactFieldByKey(t, fields, "items")
	itemsValue := debugArtifactObjectField(t, itemsField, "value")
	if got, want := debugArtifactNumberField(t, itemsValue, "omitted"), float64(6); got != want {
		t.Fatalf("items omitted count mismatch: got %.0f want %.0f", got, want)
	}
	if got, want := len(debugArtifactFieldArray(t, itemsValue, "children")), 64; got != want {
		t.Fatalf("items child count mismatch: got %d want %d", got, want)
	}
}

func TestRunnerDebugModesPreserveCanonicalReportAndRunDocumentShape(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "token",
								Subject: theater.SubjectSpec{Field: "token"},
								Assert:  theater.AssertSpec{Ref: "expectation.token"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	runCase := func(t *testing.T, options theater.RunOptions) theater.RunResult {
		t.Helper()

		catalog := theater.NewCatalog()
		if err := catalog.RegisterAction("action.login", &testkit.ScriptedAction{
			Output: theater.Outputs{"token": "issued-token"},
		}); err != nil {
			t.Fatalf("register action failed: %v", err)
		}

		matchers := matcherCatalog(t, (&testkit.ScriptedExpectation{}).Descriptor("expectation.token"))
		result, err := theater.NewRunner(catalog, matchers).Run(context.Background(), spec, options)
		if err != nil {
			t.Fatalf("runner run failed: %v", err)
		}

		return result
	}

	plainResult := runCase(t, theater.RunOptions{})
	if plainResult.Report.Generation == nil {
		t.Fatal("plain run generation metadata = nil, want populated canonical field")
	}
	if plainResult.Report.Generation.Seed == "" {
		t.Fatal("plain run generation seed = empty, want populated canonical field")
	}
	if plainResult.Report.Generation.BaseTime.IsZero() {
		t.Fatal("plain run generation base time = zero, want populated canonical field")
	}
	dumpResult := runCase(t, theater.RunOptions{
		Debug: &theater.DebugOptions{
			Mode:        theater.DebugModeDump,
			Breakpoints: []string{"name=before-submit,kind=action,phase=before,path=**"},
			DumpPath:    filepath.Join(t.TempDir(), "run.debug.ndjson"),
		},
	})
	interactiveResult := runCase(t, theater.RunOptions{
		Debug: &theater.DebugOptions{
			Mode:        theater.DebugModeInteractive,
			StartPaused: true,
			Input:       strings.NewReader("continue\n"),
			Output:      &bytes.Buffer{},
		},
	})

	for _, tc := range []struct {
		name   string
		result theater.RunResult
	}{
		{name: "dump", result: dumpResult},
		{name: "interactive", result: interactiveResult},
	} {
		if tc.result.Report.Generation == nil {
			t.Fatalf("%s run generation metadata = nil, want populated canonical field", tc.name)
		}
		if tc.result.Report.Generation.Seed == "" {
			t.Fatalf("%s run generation seed = empty, want populated canonical field", tc.name)
		}
		if tc.result.Report.Generation.BaseTime.IsZero() {
			t.Fatalf("%s run generation base time = zero, want populated canonical field", tc.name)
		}
		if !reflect.DeepEqual(
			normalizedContractValue(t, plainResult.Report),
			normalizedContractValue(t, tc.result.Report),
		) {
			t.Fatalf(
				"report contract mismatch with %s debug enabled:\nplain=%#v\ndebug=%#v",
				tc.name,
				plainResult.Report,
				tc.result.Report,
			)
		}

		plainDocument := plainResult.Document()
		if err := plainDocument.Validate(); err != nil {
			t.Fatalf("plain run document validation failed: %v", err)
		}
		debugDocument := tc.result.Document()
		if err := debugDocument.Validate(); err != nil {
			t.Fatalf("%s run document validation failed: %v", tc.name, err)
		}

		if !reflect.DeepEqual(
			normalizedContractValue(t, plainDocument),
			normalizedContractValue(t, debugDocument),
		) {
			t.Fatalf(
				"run document contract mismatch with %s debug enabled:\nplain=%#v\ndebug=%#v",
				tc.name,
				plainDocument,
				debugDocument,
			)
		}
	}
}

func TestRunnerRejectsInvalidDebugOptions(t *testing.T) {
	t.Parallel()

	catalog := theater.NewCatalog()
	matchers := matcherCatalog(t)
	runner := theater.NewRunner(catalog, matchers)

	cases := []struct {
		name    string
		options theater.RunOptions
		wantErr string
	}{
		{
			name: "dump requires path",
			options: theater.RunOptions{
				Debug: &theater.DebugOptions{Mode: theater.DebugModeDump},
			},
			wantErr: "debug dump mode requires a dump path",
		},
		{
			name: "interactive requires streams",
			options: theater.RunOptions{
				Debug: &theater.DebugOptions{Mode: theater.DebugModeInteractive},
			},
			wantErr: "interactive debug requires an input reader",
		},
		{
			name: "off rejects breakpoint config",
			options: theater.RunOptions{
				Debug: &theater.DebugOptions{
					Mode:        theater.DebugModeOff,
					Breakpoints: []string{"kind=action,phase=before,path=**"},
				},
			},
			wantErr: "debug mode off does not accept breakpoints, step, or dump path",
		},
		{
			name: "mode is required",
			options: theater.RunOptions{
				Debug: &theater.DebugOptions{},
			},
			wantErr: "debug mode is required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runner.Run(context.Background(), theater.StageSpec{}, tc.options)
			if err == nil {
				t.Fatal("runner error = nil, want validation failure")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("runner error mismatch: got %q want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestServicesAcceptResolverInterfacesWithoutConcreteCatalogTypes(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "token",
								Subject: theater.SubjectSpec{Field: "token"},
								Assert:  theater.AssertSpec{Ref: "expectation.token"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	action := &testkit.ScriptedAction{
		ContractValue: theater.ActionContract{
			Outputs: map[string]theater.ValueContract{
				"token": {Kind: theater.ValueKindString},
			},
		},
		Output: theater.Outputs{"token": "issued-token"},
	}
	expectation := &testkit.ScriptedExpectation{}
	catalog := minimalCatalog{action: action}
	matchers := minimalMatcherResolver{descriptor: expectation.Descriptor("expectation.token")}

	diagnostics := theater.NewValidator(catalog, matchers).Validate(spec)
	if len(diagnostics) != 0 {
		t.Fatalf("validator returned diagnostics: %#v", diagnostics)
	}

	result, err := theater.NewRunner(catalog, matchers).Run(context.Background(), spec, theater.RunOptions{})
	if err != nil {
		t.Fatalf("runner run failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("run status mismatch: got %s want %s", got, want)
	}
}

func TestCatalogRegisterMethodsRejectNilReceiver(t *testing.T) {
	t.Parallel()

	var catalog *theater.Catalog

	if err := catalog.RegisterAction("action.login", &testkit.ScriptedAction{}); err == nil {
		t.Fatal("RegisterAction must reject nil catalog receiver")
	}
	if err := catalog.RegisterInventory("inventory.payload", &testkit.ScriptedInventory{}); err == nil {
		t.Fatal("RegisterInventory must reject nil catalog receiver")
	}
	if err := catalog.RegisterDecorator("decorator.normalize", (&testkit.ScriptedDecorator{}).Definition()); err == nil {
		t.Fatal("RegisterDecorator must reject nil catalog receiver")
	}
	if err := catalog.RegisterGenerator("generator.email", theater.GeneratorDef{}); err == nil {
		t.Fatal("RegisterGenerator must reject nil catalog receiver")
	}
	if err := catalog.RegisterScenarioScopeInitializer("test/runtime", func() theater.ScenarioScopeInitializer {
		return nil
	}); err == nil {
		t.Fatal("RegisterScenarioScopeInitializer must reject nil catalog receiver")
	}
}

func TestValidatorValidatePanicsOnNilReceiver(t *testing.T) {
	t.Parallel()

	var validator *theater.Validator
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("Validate must panic on nil validator receiver")
		}
	}()

	_ = validator.Validate(theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	})
}

func TestProjectorProjectsEvents(t *testing.T) {
	t.Parallel()

	spec := theater.StageSpec{
		ID: "main",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "login",
				Acts: []theater.ActSpec{
					{
						ID:     "submit",
						Action: theater.ActionSpec{Use: "action.login"},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "token",
								Subject: theater.SubjectSpec{Field: "token"},
								Assert:  theater.AssertSpec{Ref: "expectation.token"},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "login-user", ScenarioID: "login"},
		},
	}

	action := &testkit.ScriptedAction{
		Output: theater.Outputs{"token": "issued-token"},
	}
	expectation := &testkit.ScriptedExpectation{}

	catalog := theater.NewCatalog()
	if err := catalog.RegisterAction("action.login", action); err != nil {
		t.Fatalf("register action failed: %v", err)
	}

	matchers := matcherCatalog(t, expectation.Descriptor("expectation.token"))
	recorder := &testkit.EventRecorder{}
	result, err := theater.NewRunner(catalog, matchers).Run(context.Background(), spec, theater.RunOptions{
		Events: recorder,
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	projector := theater.NewProjector()
	gotReport, err := projector.Project(recorder.Events())
	if err != nil {
		t.Fatalf("project report failed: %v", err)
	}

	if !reflect.DeepEqual(gotReport, result.Report) {
		t.Fatalf("projected report mismatch:\n got=%#v\nwant=%#v", gotReport, result.Report)
	}

	gotDocument, err := projector.Document(recorder.Events())
	if err != nil {
		t.Fatalf("project document failed: %v", err)
	}

	wantDocument := result.Document()
	if !reflect.DeepEqual(gotDocument, wantDocument) {
		t.Fatalf("projected document mismatch:\n got=%#v\nwant=%#v", gotDocument, wantDocument)
	}
}

func readDebugArtifactRecords(t *testing.T, path string) []map[string]any {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read debug dump failed: %v", err)
	}

	lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
	records := make([]map[string]any, 0, len(lines))
	for i := range lines {
		if len(lines[i]) == 0 {
			continue
		}

		record := make(map[string]any)
		if err := json.Unmarshal(lines[i], &record); err != nil {
			t.Fatalf("decode debug record %d failed: %v", i, err)
		}
		records = append(records, record)
	}

	return records
}

func debugArtifactObjectField(t *testing.T, object map[string]any, key string) map[string]any {
	t.Helper()

	value, ok := object[key]
	if !ok {
		t.Fatalf("debug artifact field %q is missing", key)
	}

	nested, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("debug artifact field %q type mismatch: got %T want map[string]any", key, value)
	}

	return nested
}

func debugArtifactStringField(t *testing.T, object map[string]any, key string) string {
	t.Helper()

	value, ok := object[key]
	if !ok {
		t.Fatalf("debug artifact field %q is missing", key)
	}

	text, ok := value.(string)
	if !ok {
		t.Fatalf("debug artifact field %q type mismatch: got %T want string", key, value)
	}

	return text
}

func debugArtifactNumberField(t *testing.T, object map[string]any, key string) float64 {
	t.Helper()

	value, ok := object[key]
	if !ok {
		t.Fatalf("debug artifact field %q is missing", key)
	}

	number, ok := value.(float64)
	if !ok {
		t.Fatalf("debug artifact field %q type mismatch: got %T want float64", key, value)
	}

	return number
}

func debugArtifactBoolField(t *testing.T, object map[string]any, key string) bool {
	t.Helper()

	value, ok := object[key]
	if !ok {
		t.Fatalf("debug artifact field %q is missing", key)
	}

	flag, ok := value.(bool)
	if !ok {
		t.Fatalf("debug artifact field %q type mismatch: got %T want bool", key, value)
	}

	return flag
}

func debugArtifactFieldArray(t *testing.T, object map[string]any, key string) []map[string]any {
	t.Helper()

	value, ok := object[key]
	if !ok {
		t.Fatalf("debug artifact field %q is missing", key)
	}

	list, ok := value.([]any)
	if !ok {
		t.Fatalf("debug artifact field %q type mismatch: got %T want []any", key, value)
	}

	fields := make([]map[string]any, 0, len(list))
	for i := range list {
		field, ok := list[i].(map[string]any)
		if !ok {
			t.Fatalf("debug artifact field %q item %d type mismatch: got %T want map[string]any", key, i, list[i])
		}
		fields = append(fields, field)
	}

	return fields
}

func debugArtifactFieldByKey(t *testing.T, fields []map[string]any, key string) map[string]any {
	t.Helper()

	for i := range fields {
		if debugArtifactStringField(t, fields[i], "key") == key {
			return fields[i]
		}
	}

	t.Fatalf("debug artifact field with key %q is missing", key)
	return nil
}

func normalizedContractValue(t *testing.T, value any) any {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal contract value failed: %v", err)
	}

	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode contract value failed: %v", err)
	}

	return stripContractTimingFields(decoded)
}

func stripContractTimingFields(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		delete(typed, "started_at")
		delete(typed, "ended_at")
		delete(typed, "duration_ms")
		if generation, ok := typed["generation"].(map[string]any); ok {
			if _, exists := generation["seed"]; exists {
				generation["seed"] = "<seed>"
			}
			if _, exists := generation["base_time"]; exists {
				generation["base_time"] = "<time>"
			}
			typed["generation"] = generation
		}
		for key, nested := range typed {
			typed[key] = stripContractTimingFields(nested)
		}
		return typed
	case []any:
		for i := range typed {
			typed[i] = stripContractTimingFields(typed[i])
		}
		return typed
	default:
		return value
	}
}

func waitForDebugPromptText(t *testing.T, output *synchronizedBuffer, want string) string {
	return waitForDebugPromptTextWithin(t, output, want, 250*time.Millisecond)
}

func waitForDebugPromptTextWithin(t *testing.T, output *synchronizedBuffer, want string, timeout time.Duration) string {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		text := output.String()
		if strings.Contains(text, want) {
			return text
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("debug prompt output missing %q:\n%s", want, output.String())
	return ""
}
