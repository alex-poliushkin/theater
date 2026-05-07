package docscheck

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCheckAcceptsMarkedRunnableDocs(t *testing.T) {
	root := filepath.Join("..", "..")
	docsDir := filepath.Join(root, "testdata", "docs", "valid", "docs")

	if err := Check(Options{RepoRoot: root, DocsDir: docsDir}); err != nil {
		t.Fatalf("check valid docs failed: %v", err)
	}
}

func TestCheckRejectsUnmarkedRunnableFence(t *testing.T) {
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	writeDocCheckFile(t, filepath.Join(docsDir, "tutorial", "page.md"), "```thtr\nstage skipped\n```\n")

	err := Check(Options{RepoRoot: root, DocsDir: docsDir})
	assertCheckErrorContains(t, err, "code fence with language \"thtr\" requires a theater-doc marker")
}

func TestCheckRejectsStaleSourceSnippet(t *testing.T) {
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	writeDocCheckFile(t, filepath.Join(docsDir, "examples", "stage.thtr"), minimalTHTRSource())
	writeDocCheckFile(t, filepath.Join(docsDir, "tutorial", "page.md"), `<!-- theater-doc: source id=stale kind=thtr path=../examples/stage.thtr -->
`+"```thtr\nstage wrong\n```\n")

	err := Check(Options{RepoRoot: root, DocsDir: docsDir})
	assertCheckErrorContains(t, err, "does not match source file")
}

func TestCheckRejectsMissingTheaterDSLYAMLPair(t *testing.T) {
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	writeDocCheckFile(t, filepath.Join(docsDir, "examples", "stage.thtr"), minimalTHTRSource())
	writeDocCheckFile(t, filepath.Join(docsDir, "tutorial", "page.md"), `<!-- theater-doc: source id=lonely-thtr kind=thtr path=../examples/stage.thtr pair=lonely -->
`+"```thtr\n"+strings.TrimSpace(minimalTHTRSource())+"\n```\n")

	err := Check(Options{RepoRoot: root, DocsDir: docsDir})
	assertCheckErrorContains(t, err, "pair \"lonely\" must include both Theater DSL and YAML source examples")
}

func TestCheckRejectsSkippedExampleMarker(t *testing.T) {
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	writeDocCheckFile(t, filepath.Join(docsDir, "tutorial", "page.md"), `<!-- theater-doc: skip id=skipped reason=too-hard -->
`+"```sh\ntheater validate stage.thtr\n```\n")

	err := Check(Options{RepoRoot: root, DocsDir: docsDir})
	assertCheckErrorContains(t, err, "unsupported theater-doc directive \"skip\"")
}

func TestCheckRejectsPublicInternetURL(t *testing.T) {
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	writeDocCheckFile(t, filepath.Join(docsDir, "examples", "external.thtr"), `stage external

scenario call-external
  act request
    do action.http(method: "GET", url: "https://api.example.com/users")

call run = call-external()
`)
	writeDocCheckFile(t, filepath.Join(docsDir, "tutorial", "page.md"), `<!-- theater-doc: source id=external kind=thtr path=../examples/external.thtr checks=validate -->
`+"```thtr\n"+`stage external

scenario call-external
  act request
    do action.http(method: "GET", url: "https://api.example.com/users")

call run = call-external()
`+"```\n")

	err := Check(Options{RepoRoot: root, DocsDir: docsDir})
	assertCheckErrorContains(t, err, "public internet URL \"https://api.example.com/users\" is not allowed")
}

func TestCheckRejectsCommandOutputDrift(t *testing.T) {
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	writeDocCheckFile(t, filepath.Join(docsDir, "tutorial", "page.md"), `<!-- theater-doc: command id=validate expect-stdout=all-good expect-stdout-2=also-good -->
`+"```sh\ntheater validate stage.thtr\n```\n")

	runner := &fakeRunner{
		result: CommandResult{
			ExitCode: 0,
			Stdout:   "all-good\n",
		},
	}

	err := Check(Options{
		RepoRoot: root,
		DocsDir:  docsDir,
		Runner:   runner,
	})
	assertCheckErrorContains(t, err, "command stdout does not contain \"also-good\"")
}

func TestCheckRejectsUnexpectedCommandOutput(t *testing.T) {
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	writeDocCheckFile(t, filepath.Join(docsDir, "tutorial", "page.md"), `<!-- theater-doc: command id=validate expect-stdout=valid reject-stdout=hint reject-stderr=warning -->
`+"```sh\ntheater validate stage.yaml\n```\n")

	runner := &fakeRunner{
		result: CommandResult{
			ExitCode: 0,
			Stdout:   "stage.yaml: valid with 1 hint(s)\n",
			Stderr:   "warning: extra output\n",
		},
	}

	err := Check(Options{
		RepoRoot: root,
		DocsDir:  docsDir,
		Runner:   runner,
	})
	assertCheckErrorContains(t, err, "command stdout contains rejected text \"hint\"")
	assertCheckErrorContains(t, err, "command stderr contains rejected text \"warning\"")
}

func TestCheckAcceptsGoSourceWithCommandMarker(t *testing.T) {
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	writeDocCheckFile(t, filepath.Join(docsDir, "examples", "go", "example_test.go"), `package example

import "testing"

func TestExample(t *testing.T) {}
`)
	writeDocCheckFile(t, filepath.Join(docsDir, "tutorial", "page.md"), `<!-- theater-doc: source id=go-source kind=go path=../examples/go/example_test.go -->
`+"```go\n"+`package example

import "testing"

func TestExample(t *testing.T) {}
`+"```\n\n"+`<!-- theater-doc: command id=go-test cwd=../examples/go expect-stdout=ok -->
`+"```sh\ngo test .\n```\n")

	runner := &fakeRunner{
		result: CommandResult{
			ExitCode: 0,
			Stdout:   "ok\n",
		},
	}

	err := Check(Options{
		RepoRoot: root,
		DocsDir:  docsDir,
		Runner:   runner,
	})
	if err != nil {
		t.Fatalf("check go source fixture failed: %v", err)
	}
	if got, want := len(runner.commands), 1; got != want {
		t.Fatalf("command count mismatch: got %d want %d", got, want)
	}
	command := runner.commands[0]
	if want := []string{"go", "test", "."}; !slices.Equal(command.Args, want) {
		t.Fatalf("command args mismatch: got %#v want %#v", command.Args, want)
	}
	if got, want := command.Dir, filepath.Join(docsDir, "examples", "go"); got != want {
		t.Fatalf("command dir mismatch: got %q want %q", got, want)
	}
}

func TestPublicDocsExamples(t *testing.T) {
	root := filepath.Join("..", "..")
	docsDir := filepath.Join(root, "docs")
	if _, err := os.Stat(docsDir); errors.Is(err, os.ErrNotExist) {
		t.Skip("public docs tree is created by HDOC-003")
	} else if err != nil {
		t.Fatalf("stat docs dir failed: %v", err)
	}

	options := Options{RepoRoot: root, DocsDir: docsDir}
	readmePath := filepath.Join(root, "README.md")
	if _, err := os.Stat(readmePath); err == nil {
		options.MarkdownFiles = append(options.MarkdownFiles, readmePath)
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat README.md failed: %v", err)
	}

	if err := Check(options); err != nil {
		t.Fatalf("check public docs failed: %v", err)
	}
}

func assertCheckErrorContains(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected check error containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("check error mismatch:\nwant substring: %q\nerror: %v", want, err)
	}
}

func writeDocCheckFile(t *testing.T, path, contents string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create fixture dir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write fixture file failed: %v", err)
	}
}

func minimalTHTRSource() string {
	return `stage docs-first

scenario hello
  act say-hello
    do action.generate
      outputs:
        message: "hello"
    expect message: field(values) | path("/message") == "hello"

call run = hello()
`
}

type fakeRunner struct {
	result   CommandResult
	commands []Command
}

func (r *fakeRunner) Run(_ context.Context, command Command) CommandResult {
	r.commands = append(r.commands, command)
	return r.result
}
