package thtr_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	builtinexpectation "github.com/alex-poliushkin/theater/builtin/expectation"
	"github.com/alex-poliushkin/theater/thtr"
)

func TestDecodeLoadsStage(t *testing.T) {
	t.Parallel()

	spec, err := thtr.Decode(strings.NewReader(`stage smoke
scenario ping
  act get-health
    do action.http(method: "GET", url: "/health")
`), nil)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if got, want := spec.ID, "smoke"; got != want {
		t.Fatalf("stage id mismatch: got %q want %q", got, want)
	}
}

func TestLoadFileLoadsStandaloneStage(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "stage.thtr")
	if err := os.WriteFile(path, []byte(`stage smoke
scenario ping
  act get-health
    do action.http(method: "GET", url: "/health")
`), 0o600); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}

	spec, err := thtr.LoadFile(path, nil)
	if err != nil {
		t.Fatalf("load file failed: %v", err)
	}

	if got, want := spec.ID, "smoke"; got != want {
		t.Fatalf("stage id mismatch: got %q want %q", got, want)
	}
}

func TestParseLoadsStage(t *testing.T) {
	t.Parallel()

	spec, err := thtr.Parse([]byte(`stage smoke
scenario ping
  act get-health
    do action.http(method: "GET", url: "/health")
`), nil)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if got, want := spec.Scenarios[0].Acts[0].Action.Use, "action.http"; got != want {
		t.Fatalf("action use mismatch: got %q want %q", got, want)
	}
}

func TestLoadFlowFileLoadsRepoAwareFlow(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	flowPath := writePublicTHTRFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "smoke.thtr"), `stage smoke

call login-user = auth/login()
`)
	writePublicTHTRFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.thtr"), `stage auth-lib

scenario auth/login
  act submit
    do action.http()
`)

	spec, err := thtr.LoadFlowFile(flowPath, nil)
	if err != nil {
		t.Fatalf("load flow file failed: %v", err)
	}

	if got, want := len(spec.Scenarios), 1; got != want {
		t.Fatalf("scenario count mismatch: got %d want %d", got, want)
	}
	if got, want := spec.Scenarios[0].ID, "auth/login"; got != want {
		t.Fatalf("scenario id mismatch: got %q want %q", got, want)
	}
}

func TestLoadFlowFileLoadsRepoAwareFlowWithExpectationSugar(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	flowPath := writePublicTHTRFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "smoke.thtr"), `stage smoke

call login-user = auth/login()
`)
	writePublicTHTRFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.thtr"), `stage auth-lib

scenario auth/login
  act submit
    do action.http(method: "GET", url: "/health")
    expect healthy-status: field(status_code) not >= 500
    expect demo-notification: field(body) | decode(json) | path("/notifications") has item where path("/receiverAddress") == "demo@example.test"
`)

	spec, err := thtr.LoadFlowFile(flowPath, nil)
	if err != nil {
		t.Fatalf("load flow file failed: %v", err)
	}

	if got, want := len(spec.Scenarios), 1; got != want {
		t.Fatalf("scenario count mismatch: got %d want %d", got, want)
	}
	if got, want := spec.Scenarios[0].Acts[0].Expectations[0].Assert.Ref, builtinexpectation.NotRef; got != want {
		t.Fatalf("first expectation ref mismatch: got %q want %q", got, want)
	}
	if got, want := spec.Scenarios[0].Acts[0].Expectations[1].Assert.Ref, builtinexpectation.HasItemRef; got != want {
		t.Fatalf("second expectation ref mismatch: got %q want %q", got, want)
	}
}

func TestLoadFlowFileLoadsRepoAwareFlowWithStateErgonomics(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	flowPath := writePublicTHTRFile(t, repoRoot, filepath.Join("theater", "flows", "state", "smoke.thtr"), `stage smoke

call verify = verify-state()
`)
	writePublicTHTRFile(
		t,
		repoRoot,
		filepath.Join("theater", "lib", "state", "verify.thtr"),
		string(readStateErgonomicsFixture(t, "success-input.thtr")),
	)

	spec, err := thtr.LoadFlowFile(flowPath, nil)
	if err != nil {
		t.Fatalf("load flow file failed: %v", err)
	}

	if got, want := len(spec.Scenarios), 1; got != want {
		t.Fatalf("scenario count mismatch: got %d want %d", got, want)
	}
	if got, want := spec.Scenarios[0].ID, "verify-state"; got != want {
		t.Fatalf("scenario id mismatch: got %q want %q", got, want)
	}

	readAct := spec.Scenarios[0].Acts[0]
	if got, want := readAct.Action.Use, "action.state.read"; got != want {
		t.Fatalf("read action use mismatch: got %q want %q", got, want)
	}
	if got, want := readAct.Properties["thtr:hidden:state:record:shared_meta"].Inventory.Use, "inventory.state.record"; got != want {
		t.Fatalf("hidden record inventory use mismatch: got %q want %q", got, want)
	}

	claimAct := spec.Scenarios[0].Acts[2]
	if got, want := claimAct.Action.Use, "action.state.claim"; got != want {
		t.Fatalf("claim action use mismatch: got %q want %q", got, want)
	}
	if got, want := claimAct.Properties["thtr:hidden:state:pool:otp_identities"].Inventory.Use, "inventory.state.pool"; got != want {
		t.Fatalf("hidden pool inventory use mismatch: got %q want %q", got, want)
	}

	consumeAct := spec.Scenarios[0].Acts[6]
	if got, want := consumeAct.Action.Use, "action.state.consume"; got != want {
		t.Fatalf("consume action use mismatch: got %q want %q", got, want)
	}
}

func writePublicTHTRFile(t *testing.T, repoRoot, relativePath, contents string) string {
	t.Helper()

	path := filepath.Join(repoRoot, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	return path
}

func readStateErgonomicsFixture(t *testing.T, name string) []byte {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve fixture path failed")
	}

	path := filepath.Join(filepath.Dir(file), "..", "testdata", "thtr-state-ergonomics", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s failed: %v", name, err)
	}

	return data
}
