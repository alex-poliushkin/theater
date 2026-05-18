package theatercli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pluginmanifest "github.com/alex-poliushkin/theater/plugin/manifest"
	pluginregistry "github.com/alex-poliushkin/theater/plugin/registry"
)

func TestRequirementsInspectJSONListsAuthSlotsWithoutValues(t *testing.T) {
	t.Parallel()

	repoRoot := createCLIFlowRepo(t)
	flowPath := writeCLIRepoFile(t, repoRoot, filepath.Join("theater", "flows", "auth", "login-smoke.yaml"), `
id: login-smoke
scenarios: []
scenario_calls:
  - id: smoke-login
    scenario_id: auth/login
    bindings:
      token:
        kind: literal
        value: runtime-secret-token
`)
	writeCLIRepoFile(t, repoRoot, filepath.Join("theater", "lib", "auth", "login.yaml"), `
id: auth-lib
http:
  auth:
    service_api:
      attach:
        - bearer:
            token_slot: session_token
scenarios:
  - id: auth/login
    inputs:
      token:
        type: string
        required: true
    auth_bindings:
      service_api:
        slots:
          session_token:
            kind: ref
            ref:
              name: token
    acts:
      - id: request
        action:
          use: action.http
          with:
            auth:
              kind: literal
              value: service_api
scenario_calls: []
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{commandRequirements, commandRequirementsInspect, flowPath, "--format", "json"}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
	if strings.Contains(stdout.String(), "runtime-secret-token") {
		t.Fatalf("requirements inventory leaked binding value:\n%s", stdout.String())
	}

	var inventory requirementsInventory
	if err := json.Unmarshal(stdout.Bytes(), &inventory); err != nil {
		t.Fatalf("decode requirements inventory failed: %v\n%s", err, stdout.String())
	}
	item := requireRequirement(t, inventory.Requirements, "http_auth_slot", "session_token")
	if got, want := item.Readiness, "bound"; got != want {
		t.Fatalf("readiness mismatch: got %q want %q", got, want)
	}
	if got, want := item.ScenarioID, "auth/login"; got != want {
		t.Fatalf("scenario mismatch: got %q want %q", got, want)
	}
	if got, want := item.AuthName, "service_api"; got != want {
		t.Fatalf("auth mismatch: got %q want %q", got, want)
	}
	if !strings.Contains(item.Source.File, "theater/lib/auth/login.yaml") {
		t.Fatalf("source file mismatch: %#v", item.Source)
	}
}

func TestRequirementsInspectCheckEnvReportsPluginHostGrantsWithoutValues(t *testing.T) {
	envName := "THEATER_REQUIREMENTS_TEST_TOKEN"
	previous, hadPrevious := os.LookupEnv(envName)
	if err := os.Unsetenv(envName); err != nil {
		t.Fatalf("unset env failed: %v", err)
	}
	defer func() {
		if hadPrevious {
			_ = os.Setenv(envName, previous)
			return
		}
		_ = os.Unsetenv(envName)
	}()

	stagePath := writeStageYAML(t, `
id: plugin-use
scenarios:
  - id: echo
    acts:
      - id: echo
        action:
          use: action.smoke.echo
          with:
            value:
              kind: literal
              value: hello
scenario_calls:
  - id: echo
    scenario_id: echo
`)
	configPath := writeRequirementsPluginConfig(t, envName)

	var missingStdout bytes.Buffer
	var missingStderr bytes.Buffer
	missingCode := run([]string{
		commandRequirements,
		commandRequirementsInspect,
		stagePath,
		"--plugins-config",
		configPath,
		"--check-env",
		"--format",
		"json",
	}, &missingStdout, &missingStderr)
	if got, want := missingCode, 1; got != want {
		t.Fatalf("missing env exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, missingStderr.String(), missingStdout.String())
	}
	if got := strings.TrimSpace(missingStderr.String()); got != "" {
		t.Fatalf("missing env stderr mismatch: got %q want empty", got)
	}
	var missingInventory requirementsInventory
	if err := json.Unmarshal(missingStdout.Bytes(), &missingInventory); err != nil {
		t.Fatalf("decode missing inventory failed: %v\n%s", err, missingStdout.String())
	}
	missing := requireRequirement(t, missingInventory.Requirements, "plugin_env_from_host", envName)
	if got, want := missing.Readiness, "missing"; got != want {
		t.Fatalf("missing readiness mismatch: got %q want %q", got, want)
	}

	if err := os.Setenv(envName, "secret-env-value"); err != nil {
		t.Fatalf("set env failed: %v", err)
	}
	var readyStdout bytes.Buffer
	var readyStderr bytes.Buffer
	readyCode := run([]string{
		commandRequirements,
		commandRequirementsInspect,
		stagePath,
		"--plugins-config",
		configPath,
		"--check-env",
		"--format",
		"json",
	}, &readyStdout, &readyStderr)
	if got, want := readyCode, 0; got != want {
		t.Fatalf("ready env exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, readyStderr.String(), readyStdout.String())
	}
	if got := strings.TrimSpace(readyStderr.String()); got != "" {
		t.Fatalf("ready env stderr mismatch: got %q want empty", got)
	}
	if strings.Contains(readyStdout.String(), "secret-env-value") {
		t.Fatalf("requirements inventory leaked env value:\n%s", readyStdout.String())
	}
	var readyInventory requirementsInventory
	if err := json.Unmarshal(readyStdout.Bytes(), &readyInventory); err != nil {
		t.Fatalf("decode ready inventory failed: %v\n%s", err, readyStdout.String())
	}
	var readyRaw map[string]json.RawMessage
	if err := json.Unmarshal(readyStdout.Bytes(), &readyRaw); err != nil {
		t.Fatalf("decode ready raw inventory failed: %v\n%s", err, readyStdout.String())
	}
	if _, ok := readyRaw["plugins_config"]; ok {
		t.Fatalf("requirements JSON must not expose plugin config path:\n%s", readyStdout.String())
	}
	if _, ok := readyRaw["plugins_lock"]; ok {
		t.Fatalf("requirements JSON must not expose plugin lock path:\n%s", readyStdout.String())
	}
	if strings.Contains(readyStdout.String(), filepath.Dir(configPath)) {
		t.Fatalf("requirements JSON must not expose plugin config directory:\n%s", readyStdout.String())
	}
	ready := requireRequirement(t, readyInventory.Requirements, "plugin_env_from_host", envName)
	if got, want := ready.Readiness, "available"; got != want {
		t.Fatalf("ready readiness mismatch: got %q want %q", got, want)
	}
	if got, want := ready.PluginID, "smoke-plugin"; got != want {
		t.Fatalf("plugin id mismatch: got %q want %q", got, want)
	}
}

func TestRequirementsInspectDoesNotReportUnusedPluginGrantInMixedRegistry(t *testing.T) {
	usedEnvName := "THEATER_REQUIREMENTS_TEST_USED_TOKEN"
	unusedEnvName := "THEATER_REQUIREMENTS_TEST_UNUSED_MIXED_TOKEN"
	stagePath := writeStageYAML(t, `
id: plugin-use
scenarios:
  - id: echo
    acts:
      - id: echo
        action:
          use: action.smoke.echo
          with:
            value:
              kind: literal
              value: hello
scenario_calls:
  - id: echo
    scenario_id: echo
`)
	configPath := writeRequirementsMixedPluginConfig(t, usedEnvName, unusedEnvName)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{
		commandRequirements,
		commandRequirementsInspect,
		stagePath,
		"--plugins-config",
		configPath,
		"--format",
		"json",
	}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	output := stdout.String()
	if strings.Contains(output, unusedEnvName) || strings.Contains(output, "unused-plugin") {
		t.Fatalf("requirements inventory leaked unused plugin grant:\n%s", output)
	}
	var inventory requirementsInventory
	if err := json.Unmarshal(stdout.Bytes(), &inventory); err != nil {
		t.Fatalf("decode inventory failed: %v\n%s", err, output)
	}
	used := requireRequirement(t, inventory.Requirements, "plugin_env_from_host", usedEnvName)
	if got, want := used.PluginID, "smoke-plugin"; got != want {
		t.Fatalf("plugin id mismatch: got %q want %q", got, want)
	}
}

func TestRequirementsInspectReportsOnlyStageReferencedPluginGrants(t *testing.T) {
	envName := "THEATER_REQUIREMENTS_TEST_UNUSED_TOKEN"
	stagePath := writeStageYAML(t, `
id: no-plugin-use
scenarios:
  - id: noop
    acts:
      - id: request
        action:
          use: action.generate
scenario_calls:
  - id: noop
    scenario_id: noop
`)
	configPath := writeRequirementsPluginConfig(t, envName)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{
		commandRequirements,
		commandRequirementsInspect,
		stagePath,
		"--plugins-config",
		configPath,
		"--check-env",
		"--format",
		"json",
	}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	var inventory requirementsInventory
	if err := json.Unmarshal(stdout.Bytes(), &inventory); err != nil {
		t.Fatalf("decode inventory failed: %v\n%s", err, stdout.String())
	}
	if len(inventory.Requirements) != 0 {
		t.Fatalf("unexpected requirements for unused plugin registry: %#v", inventory.Requirements)
	}
}

func TestRequirementsInspectReportsMissingHTTPAuthSlot(t *testing.T) {
	stagePath := writeStageYAML(t, `
id: missing-auth-slot
http:
  auth:
    service_api:
      attach:
        - bearer:
            token_slot: session_token
scenarios:
  - id: auth/login
    acts:
      - id: request
        action:
          use: action.http
          with:
            auth:
              kind: literal
              value: service_api
scenario_calls:
  - id: login
    scenario_id: auth/login
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{
		commandRequirements,
		commandRequirementsInspect,
		stagePath,
		"--format",
		"json",
	}, &stdout, &stderr)
	if got, want := code, 1; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
	var inventory requirementsInventory
	if err := json.Unmarshal(stdout.Bytes(), &inventory); err != nil {
		t.Fatalf("decode inventory failed: %v\n%s", err, stdout.String())
	}
	item := requireRequirement(t, inventory.Requirements, "http_auth_slot", "session_token")
	if got, want := item.Readiness, "missing"; got != want {
		t.Fatalf("readiness mismatch: got %q want %q", got, want)
	}
	if got, want := item.AuthName, "service_api"; got != want {
		t.Fatalf("auth mismatch: got %q want %q", got, want)
	}
}

func TestRequirementsInspectTextOutputOmitsValues(t *testing.T) {
	envName := "THEATER_REQUIREMENTS_TEST_TEXT_TOKEN"
	t.Setenv(envName, "secret-text-env-value")
	stagePath := writeStageYAML(t, `
id: plugin-use
scenarios:
  - id: echo
    acts:
      - id: echo
        action:
          use: action.smoke.echo
          with:
            value:
              kind: literal
              value: hello
scenario_calls:
  - id: echo
    scenario_id: echo
`)
	configPath := writeRequirementsPluginConfig(t, envName)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{
		commandRequirements,
		commandRequirementsInspect,
		stagePath,
		"--plugins-config",
		configPath,
		"--check-env",
	}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Runtime requirements:",
		"Status: ready",
		"KIND",
		"NAME",
		"OWNER",
		"READINESS",
		"plugin_env_from_host",
		envName,
		"smoke-plugin",
		"available",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("text output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "secret-text-env-value") {
		t.Fatalf("text output leaked env value:\n%s", output)
	}
}

func TestRequirementsInspectAcceptsPluginFlagsBeforeStagePath(t *testing.T) {
	envName := "THEATER_REQUIREMENTS_TEST_PRE_STAGE_TOKEN"
	configPath := writeRequirementsPluginConfig(t, envName)
	stagePath := writeStageYAML(t, `
id: plugin-use
scenarios:
  - id: echo
    acts:
      - id: echo
        action:
          use: action.smoke.echo
          with:
            value:
              kind: literal
              value: hello
scenario_calls:
  - id: echo
    scenario_id: echo
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{
		commandRequirements,
		commandRequirementsInspect,
		"--plugins-config",
		configPath,
		"--format",
		"json",
		stagePath,
	}, &stdout, &stderr)
	if got, want := code, 0; got != want {
		t.Fatalf("exit code mismatch: got %d want %d stderr=%q stdout=%q", got, want, stderr.String(), stdout.String())
	}
	var inventory requirementsInventory
	if err := json.Unmarshal(stdout.Bytes(), &inventory); err != nil {
		t.Fatalf("decode requirements inventory failed: %v\n%s", err, stdout.String())
	}
	item := requireRequirement(t, inventory.Requirements, "plugin_env_from_host", envName)
	if got, want := item.Readiness, "unchecked"; got != want {
		t.Fatalf("readiness mismatch: got %q want %q", got, want)
	}
}

func requireRequirement(
	t *testing.T,
	requirements []runtimeRequirementItem,
	kind string,
	name string,
) runtimeRequirementItem {
	t.Helper()

	for i := range requirements {
		if requirements[i].Kind == kind && requirements[i].Name == name {
			return requirements[i]
		}
	}
	t.Fatalf("requirement %s/%s not found: %#v", kind, name, requirements)
	return runtimeRequirementItem{}
}

func writeRequirementsPluginConfig(t *testing.T, envName string) string {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), "plugins.json")
	writeJSONFile(t, configPath, pluginregistry.ConfigFile{
		Schema: pluginregistry.ConfigSchemaVersion,
		Plugins: map[string]pluginregistry.PluginEntry{
			"smoke-plugin": {
				Manifest: filepath.Join(repoRoot(t), "testdata", "plugins", "smoke", "manifest.json"),
				Exec: pluginregistry.ExecSpec{
					Command: []string{"missing-smoke-plugin"},
				},
				AllowCapabilities: []string{"action.smoke.echo"},
				Grants: pluginregistry.Grants{
					EnvFromHost: []string{envName},
				},
			},
		},
	})
	return configPath
}

func writeRequirementsMixedPluginConfig(t *testing.T, usedEnvName, unusedEnvName string) string {
	t.Helper()

	unusedManifestPath := writeRequirementsPluginManifest(t, "unused-plugin", "action.unused.echo")
	configPath := filepath.Join(t.TempDir(), "plugins.json")
	writeJSONFile(t, configPath, pluginregistry.ConfigFile{
		Schema: pluginregistry.ConfigSchemaVersion,
		Plugins: map[string]pluginregistry.PluginEntry{
			"smoke-plugin": {
				Manifest: filepath.Join(repoRoot(t), "testdata", "plugins", "smoke", "manifest.json"),
				Exec: pluginregistry.ExecSpec{
					Command: []string{"missing-smoke-plugin"},
				},
				AllowCapabilities: []string{"action.smoke.echo"},
				Grants: pluginregistry.Grants{
					EnvFromHost: []string{usedEnvName},
				},
			},
			"unused-plugin": {
				Manifest: unusedManifestPath,
				Exec: pluginregistry.ExecSpec{
					Command: []string{"missing-unused-plugin"},
				},
				AllowCapabilities: []string{"action.unused.echo"},
				Grants: pluginregistry.Grants{
					EnvFromHost: []string{unusedEnvName},
				},
			},
		},
	})
	return configPath
}

func writeRequirementsPluginManifest(t *testing.T, pluginID, capabilityName string) string {
	t.Helper()

	manifestFile, err := pluginmanifest.Finalize(pluginmanifest.File{
		Schema: pluginmanifest.SchemaVersion,
		Plugin: pluginmanifest.Plugin{
			ID:      pluginID,
			Version: "0.1.0",
		},
		Protocol: pluginmanifest.Protocol{
			Name:  pluginmanifest.ProtocolName,
			Major: 1,
		},
		Capabilities: []pluginmanifest.Capability{{
			Kind:           pluginmanifest.CapabilityKindAction,
			Name:           capabilityName,
			PropertySchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
			ResultSchema:   json.RawMessage(`{"type":"object","additionalProperties":false}`),
		}},
	})
	if err != nil {
		t.Fatalf("finalize plugin manifest: %v", err)
	}
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	writeJSONFile(t, manifestPath, manifestFile)
	return manifestPath
}
