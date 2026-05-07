package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sync/atomic"
	"time"
)

const (
	envBaseURL    = "THEATER_DOC_BASE_URL"
	envLoginURL   = "THEATER_DOC_LOGIN_URL"
	envProfileURL = "THEATER_DOC_PROFILE_URL"
	envStatusURL  = "THEATER_DOC_STATUS_URL"
	stateRoot     = "/tmp/theater-doc-state"
)

type childExitError struct {
	code int
}

func main() {
	if err := run(); err != nil {
		var childExit childExitError
		if errors.As(err, &childExit) {
			os.Exit(childExit.code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	args := os.Args[1:]
	if len(args) < 2 || args[0] != "--" {
		return errors.New("usage: go run ./docs/examples/http-state/fixture -- <command> [args...]")
	}

	if err := os.RemoveAll(stateRoot); err != nil {
		return fmt.Errorf("reset state fixture: %w", err)
	}
	defer os.RemoveAll(stateRoot)

	server, baseURL, err := startServer()
	if err != nil {
		return err
	}
	defer shutdownServer(server)

	cmd := exec.Command(args[1], args[2:]...) //nolint:gosec // The docs fixture intentionally runs the command after "--".
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		envBaseURL+"="+baseURL,
		envLoginURL+"="+baseURL+"/login",
		envProfileURL+"="+baseURL+"/profile",
		envStatusURL+"="+baseURL+"/status",
	)

	if err := cmd.Run(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return childExitError{code: exitError.ExitCode()}
		}
		return fmt.Errorf("run child command: %w", err)
	}
	return nil
}

func (e childExitError) Error() string {
	return fmt.Sprintf("child command exited with code %d", e.code)
}

func startServer() (*http.Server, string, error) {
	var statusPolls atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/login", handleLogin)
	mux.HandleFunc("/profile", handleProfile)
	mux.HandleFunc("/status", func(writer http.ResponseWriter, request *http.Request) {
		handleStatus(writer, request, &statusPolls)
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", fmt.Errorf("start fixture listener: %w", err)
	}

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
	}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "fixture server failed: %v\n", err)
		}
	}()

	return server, "http://" + listener.Addr().String(), nil
}

func shutdownServer(server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func handleLogin(writer http.ResponseWriter, _ *http.Request) {
	http.SetCookie(writer, &http.Cookie{
		Name:     "theater_docs",
		Value:    "session",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(writer, `{"ok":true}`)
}

func handleProfile(writer http.ResponseWriter, request *http.Request) {
	cookie, err := request.Cookie("theater_docs")
	if err != nil || cookie.Value != "session" {
		writer.WriteHeader(http.StatusUnauthorized)
		writeJSON(writer, `{"error":"login required"}`)
		return
	}

	writeJSON(writer, `{"data":{"id":"user-123","status":"active","email":"demo@example.test"}}`)
}

func handleStatus(writer http.ResponseWriter, _ *http.Request, statusPolls *atomic.Int64) {
	attempt := statusPolls.Add(1)
	if attempt < 2 {
		writeJSON(writer, fmt.Sprintf(`{"ready":false,"attempt":%d}`, attempt))
		return
	}

	writeJSON(writer, fmt.Sprintf(`{"ready":true,"attempt":%d}`, attempt))
}

func writeJSON(writer http.ResponseWriter, body string) {
	writer.Header().Set("Content-Type", "application/json")
	_, _ = writer.Write([]byte(body))
}
