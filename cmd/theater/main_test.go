package main

import (
	"bytes"
	"io/fs"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestMainEntrypointDelegatesArgumentsAndOutput(t *testing.T) {
	t.Parallel()

	command := exec.Command("go", "run", ".", "--version")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		t.Fatalf("run theater wrapper failed: %v stderr=%q", err, stderr.String())
	}
	if got, want := strings.TrimSpace(stdout.String()), "theater dev"; got != want {
		t.Fatalf("stdout mismatch: got %q want %q", got, want)
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
}

func TestCommandDirectoryStaysThin(t *testing.T) {
	t.Parallel()

	var goFiles []string
	if err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		goFiles = append(goFiles, filepath.ToSlash(path))
		return nil
	}); err != nil {
		t.Fatalf("walk command directory failed: %v", err)
	}
	slices.Sort(goFiles)

	want := []string{"main.go", "main_test.go"}
	if !slices.Equal(goFiles, want) {
		t.Fatalf(
			"cmd/theater must stay limited to executable wrapper files; got %v want %v. Move CLI implementation and behavior-heavy tests to internal/theatercli.",
			goFiles,
			want,
		)
	}
}
