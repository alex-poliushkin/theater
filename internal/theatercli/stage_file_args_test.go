package theatercli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseCommandOptionsAcceptsPositionalStagePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		command  string
		args     []string
		wantFile string
	}{
		{
			name:     "validate first arg",
			command:  commandValidate,
			args:     []string{"stage.yaml"},
			wantFile: "stage.yaml",
		},
		{
			name:     "validate after flags",
			command:  commandValidate,
			args:     []string{"-format", "json", "stage.yaml"},
			wantFile: "stage.yaml",
		},
		{
			name:     "run before flags",
			command:  commandRun,
			args:     []string{"stage.yaml", "-live", "off"},
			wantFile: "stage.yaml",
		},
		{
			name:     "run after flags",
			command:  commandRun,
			args:     []string{"-live", "off", "stage.yaml", "-format", "json"},
			wantFile: "stage.yaml",
		},
		{
			name:     "validate after double dash",
			command:  commandValidate,
			args:     []string{"--", "-stage.yaml"},
			wantFile: "-stage.yaml",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var stdout strings.Builder
			var stderr strings.Builder

			app := newApplication(&stdout, &stderr)
			options, ok := app.parseCommandOptions(test.command, test.args)
			if !ok {
				t.Fatalf("parse command options returned false: stderr=%q", stderr.String())
			}
			if got, want := options.file, test.wantFile; got != want {
				t.Fatalf("file mismatch: got %q want %q", got, want)
			}
		})
	}
}

func TestParseCommandOptionsRejectsMultipleStageFileSources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "positional and file flag",
			args: []string{"stage.yaml", "-file", "other.yaml"},
			want: "choose a positional path or --file",
		},
		{
			name: "extra positional",
			args: []string{"stage.yaml", "other.yaml"},
			want: "accepts exactly one stage file path",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var stdout strings.Builder
			var stderr strings.Builder

			app := newApplication(&stdout, &stderr)
			if _, ok := app.parseCommandOptions(commandRun, test.args); ok {
				t.Fatal("parse command options returned true")
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("stderr missing %q: %q", test.want, stderr.String())
			}
		})
	}
}

func TestParseAuthoringOptionsAcceptsPositionalStagePathWithFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		parse func(*application, []string) (commandOptions, bool)
		args  []string
		want  commandOptions
	}{
		{
			name:  "fmt check before path",
			parse: (*application).parseFormatOptions,
			args:  []string{"--check", "stage.thtr"},
			want:  commandOptions{file: "stage.thtr", check: true},
		},
		{
			name:  "lower map before path",
			parse: (*application).parseLowerOptions,
			args:  []string{"--map", "stage.map.json", "stage.thtr"},
			want:  commandOptions{file: "stage.thtr", mapPath: "stage.map.json"},
		},
		{
			name:  "lower map after path",
			parse: (*application).parseLowerOptions,
			args:  []string{"stage.thtr", "--map", "stage.map.json"},
			want:  commandOptions{file: "stage.thtr", mapPath: "stage.map.json"},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var stdout strings.Builder
			var stderr strings.Builder

			app := newApplication(&stdout, &stderr)
			options, ok := test.parse(app, test.args)
			if !ok {
				t.Fatalf("parse authoring options returned false: stderr=%q", stderr.String())
			}
			if got, want := options.file, test.want.file; got != want {
				t.Fatalf("file mismatch: got %q want %q", got, want)
			}
			if got, want := options.mapPath, test.want.mapPath; got != want {
				t.Fatalf("map path mismatch: got %q want %q", got, want)
			}
			if got, want := options.check, test.want.check; got != want {
				t.Fatalf("check mismatch: got %t want %t", got, want)
			}
		})
	}
}

func TestParseAuthoringOptionsRejectsMultipleStageFileSources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		parse func(*application, []string) (commandOptions, bool)
		args  []string
		want  string
	}{
		{
			name:  "fmt extra positional after file flag",
			parse: (*application).parseFormatOptions,
			args:  []string{"--file", "stage.thtr", "extra.thtr"},
			want:  "fmt accepts exactly one .thtr file path",
		},
		{
			name:  "fmt positional and file flag",
			parse: (*application).parseFormatOptions,
			args:  []string{"stage.thtr", "--file", "other.thtr"},
			want:  "choose a positional path or --file",
		},
		{
			name:  "lower positional and file flag",
			parse: (*application).parseLowerOptions,
			args:  []string{"stage.thtr", "--file", "other.thtr"},
			want:  "choose a positional path or --file",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var stdout strings.Builder
			var stderr strings.Builder

			app := newApplication(&stdout, &stderr)
			if _, ok := test.parse(app, test.args); ok {
				t.Fatal("parse authoring options returned true")
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("stderr missing %q: %q", test.want, stderr.String())
			}
		})
	}
}

func TestParseCommandOptionsKeepsLegacyFileFlagTrailingArgsBehavior(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	app := newApplication(&stdout, &stderr)
	options, ok := app.parseCommandOptions(commandValidate, []string{"-file", "stage.yaml", "extra"})
	if !ok {
		t.Fatalf("parse command options returned false: stderr=%q", stderr.String())
	}
	if got, want := options.file, "stage.yaml"; got != want {
		t.Fatalf("file mismatch: got %q want %q", got, want)
	}
}

func TestValidateAcceptsPositionalStagePath(t *testing.T) {
	t.Parallel()

	path := writeStageYAML(t, `
id: main
scenario_calls: []
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandValidate, path}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q", got, want, stderr.String())
	}
	if got, want := strings.TrimSpace(stdout.String()), path+": valid"; got != want {
		t.Fatalf("stdout mismatch: got %q want %q", got, want)
	}
}

func TestRunAcceptsPositionalStagePath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("ok"))
	}))
	defer server.Close()

	t.Setenv("THEATER_STAGE_URL", server.URL)

	path := writeStageYAML(t, `
id: main
scenarios:
  - id: probe
    acts:
      - id: request
        properties:
          url:
            inventory:
              use: inventory.env
              with:
                name:
                  kind: literal
                  value: THEATER_STAGE_URL
        action:
          use: action.http
          with:
            url:
              kind: ref
              ref: url
scenario_calls:
  - id: probe-server
    scenario_id: probe
`)

	var stdout strings.Builder
	var stderr strings.Builder

	code := run([]string{commandRun, path, "-live", "off"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q", got, want, stderr.String())
	}
	if !strings.Contains(stdout.String(), path+": passed") {
		t.Fatalf("stdout mismatch: %q", stdout.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr mismatch: got %q want empty", stderr.String())
	}
}
