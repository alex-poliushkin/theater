//go:build unix

package action_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/testkit"
)

func TestCommandActionTimeoutKillsSpawnedProcessTree(t *testing.T) {
	t.Parallel()

	helper := testkit.BuildCommandHelper(t)
	action := resolveCommandAction(t)
	tempDir := t.TempDir()
	readyPath := filepath.Join(tempDir, "ready")
	markerPath := filepath.Join(tempDir, "marker")

	_, err := action.Run(context.Background(), theater.ActionRequest{
		Args: theater.Args{
			"executable": helper,
			"args": []any{
				"spawn-marker",
				"--ready", readyPath,
				"--marker", markerPath,
				"--child-delay-ms", "600",
				"--parent-sleep-ms", "5000",
			},
			"timeout": "150ms",
		},
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error mismatch: got %v", err)
	}

	requireFileEventually(t, readyPath, 200*time.Millisecond)
	requireFileDoesNotAppear(t, markerPath, 800*time.Millisecond)
}

func TestCommandActionCancelKillsSpawnedProcessTree(t *testing.T) {
	t.Parallel()

	helper := testkit.BuildCommandHelper(t)
	action := resolveCommandAction(t)
	tempDir := t.TempDir()
	readyPath := filepath.Join(tempDir, "ready")
	markerPath := filepath.Join(tempDir, "marker")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := action.Run(ctx, theater.ActionRequest{
			Args: theater.Args{
				"executable": helper,
				"args": []any{
					"spawn-marker",
					"--ready", readyPath,
					"--marker", markerPath,
					"--child-delay-ms", "600",
					"--parent-sleep-ms", "5000",
				},
			},
		})
		done <- err
	}()

	requireFileEventually(t, readyPath, 2*time.Second)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected cancel error")
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel error mismatch: got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for command cancellation")
	}

	requireFileDoesNotAppear(t, markerPath, 800*time.Millisecond)
}

func requireFileEventually(t *testing.T, path string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", path)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func requireFileDoesNotAppear(t *testing.T, path string, duration time.Duration) {
	t.Helper()

	deadline := time.Now().Add(duration)
	for {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("expected %s to stay absent", path)
		}
		if time.Now().After(deadline) {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}
}
