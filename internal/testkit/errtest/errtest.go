package errtest

import (
	"errors"
	"strings"
	"testing"
)

func RequireContains(t testing.TB, err error, fragment string) {
	t.Helper()

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), fragment) {
		t.Fatalf("error mismatch: got %q want fragment %q", err.Error(), fragment)
	}
}

func RequireEqual(t testing.TB, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
}

func RequireIs(t testing.TB, err, target error) {
	t.Helper()

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, target) {
		t.Fatalf("error mismatch: got %v want %v", err, target)
	}
}

func RequireAs(t testing.TB, err error, target any) {
	t.Helper()

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.As(err, target) {
		t.Fatalf("error type mismatch: got %T", err)
	}
}
