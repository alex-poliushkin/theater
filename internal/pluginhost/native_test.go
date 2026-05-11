package pluginhost

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	internalpluginregistry "github.com/alex-poliushkin/theater/internal/pluginregistry"
	pluginregistry "github.com/alex-poliushkin/theater/plugin/registry"
)

func TestOpenNativeTransportUsesResolvedEnvOnly(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("shell script executable permissions are Unix-specific")
	}

	scriptPath := filepath.Join(t.TempDir(), "print-env.sh")
	script := `#!/bin/sh
printf 'literal=%s\n' "$THEATER_LITERAL"
printf 'host=%s\n' "$THEATER_HOST"
if [ "${THEATER_AMBIENT+x}" = x ]; then
  printf 'ambient=%s\n' "$THEATER_AMBIENT"
else
  printf 'ambient=<missing>\n'
fi
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write plugin script: %v", err)
	}

	transport, err := openNativeTransport(internalpluginregistry.LoadedPlugin{
		ID:             "env-plugin",
		ExecutablePath: scriptPath,
		Config: pluginregistry.PluginEntry{
			Exec: pluginregistry.ExecSpec{
				Command: []string{scriptPath},
			},
		},
	}, map[string]string{
		"THEATER_LITERAL": "literal-value",
		"THEATER_HOST":    "host-value",
	})
	if err != nil {
		t.Fatalf("open native transport: %v", err)
	}

	raw, err := io.ReadAll(transport.stdout)
	if err != nil {
		t.Fatalf("read plugin stdout: %v", err)
	}
	if err := transport.close(context.Background()); err != nil {
		t.Fatalf("close transport: %v", err)
	}

	output := string(raw)
	for _, want := range []string{
		"literal=literal-value",
		"host=host-value",
		"ambient=<missing>",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("plugin stdout missing %q: %s", want, output)
		}
	}
}

func TestOpenNativeTransportDoesNotInheritAmbientEnvWithoutGrants(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script executable permissions are Unix-specific")
	}
	t.Setenv("THEATER_AMBIENT_ONLY", "ambient-value")

	scriptPath := filepath.Join(t.TempDir(), "print-ambient.sh")
	script := `#!/bin/sh
if [ "${THEATER_AMBIENT_ONLY+x}" = x ]; then
  printf 'ambient=%s\n' "$THEATER_AMBIENT_ONLY"
else
  printf 'ambient=<missing>\n'
fi
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write plugin script: %v", err)
	}

	transport, err := openNativeTransport(internalpluginregistry.LoadedPlugin{
		ID:             "env-plugin",
		ExecutablePath: scriptPath,
		Config: pluginregistry.PluginEntry{
			Exec: pluginregistry.ExecSpec{
				Command: []string{scriptPath},
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("open native transport: %v", err)
	}

	raw, err := io.ReadAll(transport.stdout)
	if err != nil {
		t.Fatalf("read plugin stdout: %v", err)
	}
	if err := transport.close(context.Background()); err != nil {
		t.Fatalf("close transport: %v", err)
	}
	if output := string(raw); !strings.Contains(output, "ambient=<missing>") {
		t.Fatalf("plugin stdout must not include ambient env without grants: %s", output)
	}
}
