package theater_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/alex-poliushkin/theater"
	theateryaml "github.com/alex-poliushkin/theater/yaml"
)

func TestRunRegistrationConfirmationLoginFixture(t *testing.T) {
	var (
		mu       sync.Mutex
		requests []string
	)

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		requests = append(requests, request.Method+" "+request.URL.Path)
		mu.Unlock()

		switch request.URL.Path {
		case "/register":
			writer.WriteHeader(http.StatusCreated)
			_, _ = writer.Write([]byte(`{"user_id":"user-1"}`))
		case "/otp":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"X-OTP":"123456"}`))
		case "/confirm":
			if got, want := request.Header.Get("X-OTP"), "123456"; got != want {
				t.Fatalf("confirm header mismatch: got %q want %q", got, want)
			}

			writer.WriteHeader(http.StatusNoContent)
		case "/login":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"token":"issued-token"}`))
		default:
			t.Fatalf("unexpected path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	t.Setenv("THEATER_REGISTER_URL", server.URL+"/register")
	t.Setenv("THEATER_OTP_URL", server.URL+"/otp")
	t.Setenv("THEATER_CONFIRM_URL", server.URL+"/confirm")
	t.Setenv("THEATER_LOGIN_URL", server.URL+"/login")

	catalog, matchers, err := newBuiltins()
	if err != nil {
		t.Fatalf("new built-ins failed: %v", err)
	}

	spec, err := theateryaml.LoadFlowFile(filepath.Join("theater", "flows", "auth", "registration-confirmation-login.yaml"), matchers)
	if err != nil {
		t.Fatalf("load fixture failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matchers)
	if err != nil {
		t.Fatalf("run fixture failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	if got, want := result.Report.Summary.TotalScenarios, 3; got != want {
		t.Fatalf("total scenarios mismatch: got %d want %d", got, want)
	}

	if got, want := result.Report.Summary.PassedScenarios, 3; got != want {
		t.Fatalf("passed scenarios mismatch: got %d want %d", got, want)
	}

	if got, want := len(result.Report.Nodes), 16; got != want {
		t.Fatalf("node count mismatch: got %d want %d", got, want)
	}

	if got, want := scenarioNodeID(t, result.Report, "register-user"), "auth/register"; got != want {
		t.Fatalf("scenario id mismatch: got %q want %q", got, want)
	}

	if got, want := requests, []string{"POST /register", "GET /otp", "POST /confirm", "POST /login"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("request order mismatch: got %v want %v", got, want)
	}
}

func TestRunStandaloneLoginFixture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.Method, http.MethodPost; got != want {
			t.Fatalf("request method mismatch: got %q want %q", got, want)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"token":"issued-token"}`))
	}))
	defer server.Close()

	t.Setenv("THEATER_LOGIN_URL", server.URL)

	catalog, matchers, err := newBuiltins()
	if err != nil {
		t.Fatalf("new built-ins failed: %v", err)
	}

	spec, err := theateryaml.LoadFlowFile(filepath.Join("theater", "flows", "auth", "login-smoke.yaml"), matchers)
	if err != nil {
		t.Fatalf("load fixture failed: %v", err)
	}

	result, err := runStage(context.Background(), spec, catalog, matchers)
	if err != nil {
		t.Fatalf("run fixture failed: %v", err)
	}

	if got, want := result.Report.Status, theater.StatusPassed; got != want {
		t.Fatalf("report status mismatch: got %q want %q", got, want)
	}

	if got, want := result.Report.Summary.TotalScenarios, 1; got != want {
		t.Fatalf("total scenarios mismatch: got %d want %d", got, want)
	}

	if got, want := len(result.Report.Nodes), 6; got != want {
		t.Fatalf("node count mismatch: got %d want %d", got, want)
	}

	if got, want := scenarioNodeID(t, result.Report, "smoke-login"), "auth/login"; got != want {
		t.Fatalf("scenario id mismatch: got %q want %q", got, want)
	}
}

func scenarioNodeID(t *testing.T, report theater.Report, callID string) string {
	t.Helper()

	for _, node := range report.Nodes {
		if node.Kind == theater.NodeKindScenario && node.ScenarioCallID == callID {
			return node.ScenarioID
		}
	}

	t.Fatalf("scenario node %q not found", callID)
	return ""
}
