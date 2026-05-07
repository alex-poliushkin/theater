package theatercli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/testkit"
)

func TestWorkflowFixtureIRSRegistrationLowersToGoldenAndValidates(t *testing.T) {
	t.Parallel()

	sourcePath := workflowFixturePath(t, "irs", "individual-registration-email-v1.thtr")
	wantLowered := readFileString(t, workflowFixturePath(t, "irs", "individual-registration-email-v1.yaml"))

	var lowerStdout, lowerStderr bytes.Buffer
	lowerCode := run([]string{commandLower, "--file", sourcePath}, &lowerStdout, &lowerStderr)
	if lowerCode != 0 {
		t.Fatalf("lower exit code mismatch: got %d stderr=%q stdout=%q", lowerCode, lowerStderr.String(), lowerStdout.String())
	}
	if got := strings.TrimSpace(lowerStderr.String()); got != "" {
		t.Fatalf("lower stderr mismatch: got %q want empty", got)
	}
	if got := lowerStdout.String(); got != wantLowered {
		t.Fatalf("lowered workflow mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, wantLowered)
	}

	var validateStdout, validateStderr bytes.Buffer
	validateCode := run([]string{commandValidate, "--file", sourcePath, "--format", "json"}, &validateStdout, &validateStderr)
	if validateCode != 0 {
		t.Fatalf("validate exit code mismatch: got %d stderr=%q stdout=%q", validateCode, validateStderr.String(), validateStdout.String())
	}
	if got := strings.TrimSpace(validateStderr.String()); got != "" {
		t.Fatalf("validate stderr mismatch: got %q want empty", got)
	}
	if response := decodeValidationResponse(t, validateStdout.Bytes()); !response.Valid {
		t.Fatalf("IRS workflow fixture must validate: %#v", response.Diagnostics)
	}
}

func TestWorkflowFixtureFileBackendLifecycleRuns(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "state")
	seedWorkflowFileState(t, root)
	source := readFileString(t, workflowFixturePath(t, "state", "file-backend-lifecycle.thtr"))
	stage := strings.ReplaceAll(source, "__THEATER_TEST_FILE_STATE_ROOT__", root)
	path := writeStageFile(t, "file-backend-lifecycle.thtr", stage)

	document := runWorkflowFixtureJSON(t, path)
	if got, want := document.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("run status mismatch: got %q want %q failure=%#v", got, want, document.Report.Failure)
	}
	requireWorkflowActPassed(t, document.Report, "consume-disposable-claim")

	pool := readWorkflowJSONFile[workflowPoolDocument](t, filepath.Join(root, "pools", "otp-identities.json"))
	releaseItem := workflowPoolItemByID(t, pool, "mailbox-release")
	if got, want := releaseItem.State, "available"; got != want {
		t.Fatalf("released item state mismatch: got %q want %q", got, want)
	}
	consumeItem := workflowPoolItemByID(t, pool, "mailbox-consume")
	if got, want := consumeItem.State, "used"; got != want {
		t.Fatalf("consumed item state mismatch: got %q want %q", got, want)
	}
	if got, want := consumeItem.Tombstone["reason"], "tutorial-finished"; got != want {
		t.Fatalf("consumed item tombstone mismatch: got %#v want %#v", got, want)
	}
}

func TestWorkflowFixtureCommandGeneratedDataRuns(t *testing.T) {
	t.Parallel()

	source := readFileString(t, workflowFixturePath(t, "command", "command-generated.thtr"))
	stage := strings.ReplaceAll(source, "__THEATER_COMMAND_HELPER__", testkit.BuildCommandHelper(t))
	path := writeStageFile(t, "command-generated.thtr", stage)

	document := runWorkflowFixtureJSON(t, path)
	if got, want := document.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("run status mismatch: got %q want %q failure=%#v", got, want, document.Report.Failure)
	}
	requireWorkflowActPassed(t, document.Report, "echo-data")
}

func TestWorkflowFixtureSQLiteLifecycleLowers(t *testing.T) {
	t.Parallel()

	sourcePath := workflowFixturePath(t, "state", "sqlite-backend-lifecycle.thtr")
	var stdout, stderr bytes.Buffer
	code := run([]string{commandLower, "--file", sourcePath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("lower sqlite workflow exit code mismatch: got %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("lower sqlite workflow stderr mismatch: got %q want empty", got)
	}
	if !strings.Contains(stdout.String(), "state_backend.sqlite") {
		t.Fatalf("lowered sqlite workflow must preserve plugin state backend ref:\n%s", stdout.String())
	}
}

type workflowPoolDocument struct {
	Pool  string             `json:"pool"`
	Items []workflowPoolItem `json:"items"`
}

type workflowPoolItem struct {
	ID        string         `json:"id"`
	Fields    map[string]any `json:"fields"`
	State     string         `json:"state"`
	Tombstone map[string]any `json:"tombstone"`
	Version   int64          `json:"version"`
}

func workflowFixturePath(t *testing.T, parts ...string) string {
	t.Helper()

	segments := append([]string{repoRoot(t), "testdata", "workflows"}, parts...)
	return filepath.Join(segments...)
}

func seedWorkflowFileState(t *testing.T, root string) {
	t.Helper()

	copyWorkflowFixtureFile(
		t,
		workflowFixturePath(t, "state", "file-seed", "pools", "otp-identities.json"),
		filepath.Join(root, "pools", "otp-identities.json"),
	)
}

func runWorkflowFixtureJSON(t *testing.T, path string) theater.RunDocument {
	t.Helper()

	var stdout, stderr bytes.Buffer
	code := run([]string{commandRun, "--file", path, "--format", "json", "--live", "off"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run workflow fixture exit code mismatch: got %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("run workflow fixture stderr mismatch: got %q want empty", got)
	}

	var response struct {
		File   string              `json:"file"`
		Result theater.RunDocument `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("decode run document failed: %v\n%s", err, stdout.String())
	}
	return response.Result
}

func workflowPoolItemByID(t *testing.T, pool workflowPoolDocument, id string) workflowPoolItem {
	t.Helper()

	for i := range pool.Items {
		if pool.Items[i].ID == id {
			return pool.Items[i]
		}
	}
	t.Fatalf("pool item %q not found: %#v", id, pool.Items)
	return workflowPoolItem{}
}

func requireWorkflowActPassed(t *testing.T, report theater.Report, actID string) {
	t.Helper()

	for i := range report.Nodes {
		node := report.Nodes[i]
		if node.Kind != theater.NodeKindAct || node.Address == nil || node.Address.ActID != actID {
			continue
		}
		if node.Status != theater.StatusPassed {
			t.Fatalf("workflow act %q status mismatch: got %q want %q", actID, node.Status, theater.StatusPassed)
		}
		return
	}

	t.Fatalf("workflow act %q was not executed: %#v", actID, report.Nodes)
}

func copyWorkflowFixtureFile(t *testing.T, sourcePath, targetPath string) {
	t.Helper()

	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read workflow fixture %s: %v", sourcePath, err)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("make workflow fixture target dir %s: %v", targetPath, err)
	}
	if err := os.WriteFile(targetPath, data, 0o600); err != nil {
		t.Fatalf("write workflow fixture %s: %v", targetPath, err)
	}
}

func readWorkflowJSONFile[T any](t *testing.T, path string) T {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read workflow JSON %s: %v", path, err)
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode workflow JSON %s: %v\n%s", path, err, raw)
	}
	return value
}
