package theatercli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	pluginregistry "github.com/alex-poliushkin/theater/plugin/registry"
)

func TestApplicationPluginsDigestWritesDescriptorDigestForDraftManifest(t *testing.T) {
	t.Parallel()

	manifestPath := writeDraftSmokeManifest(t, "")
	originalRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read original manifest: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	if code := app.Run([]string{"plugins", "digest", "--manifest", manifestPath}); code != 0 {
		t.Fatalf("plugins digest exit code: %d stderr=%s", code, stderr.String())
	}
	digest := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("digest output mismatch: got %q", stdout.String())
	}
	currentRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest after print mode: %v", err)
	}
	if !bytes.Equal(currentRaw, originalRaw) {
		t.Fatal("plugins digest without --write must not modify the manifest file")
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"plugins", "digest", "--manifest", manifestPath, "--write"}); code != 0 {
		t.Fatalf("plugins digest --write exit code: %d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "wrote "+manifestPath) {
		t.Fatalf("digest write output mismatch: %q", stdout.String())
	}
	writtenDigest := readManifestDescriptorDigest(t, manifestPath)
	if writtenDigest != digest {
		t.Fatalf("written digest mismatch: got %q want %q", writtenDigest, digest)
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}

	configPath, lockPath := writeSmokePluginFilesWithManifest(t, manifestPath)
	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"plugins", "lock", "-plugins-config", configPath, "-plugins-lock", lockPath}); code != 0 {
		t.Fatalf("plugins lock exit code: %d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file missing: %v", err)
	}
}

func TestApplicationPluginsDigestRejectsTrailingJSON(t *testing.T) {
	t.Parallel()

	manifestPath := writeDraftSmokeManifest(t, "")
	file, err := os.OpenFile(manifestPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open manifest: %v", err)
	}
	if _, err := file.WriteString("\n{}"); err != nil {
		_ = file.Close()
		t.Fatalf("append trailing JSON: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close manifest: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	if code := app.Run([]string{"plugins", "digest", "--manifest", manifestPath}); code != exitCodeCommandError {
		t.Fatalf("plugins digest exit code mismatch: got %d want %d stdout=%s stderr=%s", code, exitCodeCommandError, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "trailing JSON value") {
		t.Fatalf("plugins digest stderr missing trailing JSON error: %s", stderr.String())
	}
}

func TestApplicationPluginsDigestReportsManifestFailures(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	malformedPath := filepath.Join(tempDir, "malformed.manifest.json")
	if err := os.WriteFile(malformedPath, []byte(`{"schema":`), 0o644); err != nil {
		t.Fatalf("write malformed manifest: %v", err)
	}
	invalidPath := filepath.Join(tempDir, "invalid.manifest.json")
	writeJSONFile(t, invalidPath, map[string]any{
		"schema": pluginregistry.ConfigSchemaVersion,
	})

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "missing file",
			path: filepath.Join(tempDir, "missing.manifest.json"),
			want: "read plugin manifest",
		},
		{
			name: "malformed json",
			path: malformedPath,
			want: "decode plugin manifest",
		},
		{
			name: "invalid manifest",
			path: invalidPath,
			want: "plugin manifest schema",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			app := newApplication(stdout, stderr)
			if code := app.Run([]string{"plugins", "digest", "--manifest", test.path}); code != exitCodeCommandError {
				t.Fatalf("plugins digest exit code mismatch: got %d want %d stdout=%s stderr=%s", code, exitCodeCommandError, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("plugins digest stderr missing %q: %s", test.want, stderr.String())
			}
		})
	}
}

func TestApplicationPluginsDigestWritePreservesManifestPermissions(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are platform-specific on Windows")
	}

	manifestPath := writeDraftSmokeManifest(t, "")
	if err := os.Chmod(manifestPath, 0o600); err != nil {
		t.Fatalf("chmod manifest: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	if code := app.Run([]string{"plugins", "digest", "--manifest", manifestPath, "--write"}); code != 0 {
		t.Fatalf("plugins digest --write exit code: %d stderr=%s", code, stderr.String())
	}

	info, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatalf("stat manifest: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("manifest permissions mismatch: got %v want %v", got, want)
	}
}

func TestApplicationPluginsDigestWriteRejectsManifestSymlink(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions are platform-specific on Windows")
	}

	manifestPath := writeDraftSmokeManifest(t, "")
	linkPath := filepath.Join(t.TempDir(), "manifest-link.json")
	if err := os.Symlink(manifestPath, linkPath); err != nil {
		t.Fatalf("create manifest symlink: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	if code := app.Run([]string{"plugins", "digest", "--manifest", linkPath, "--write"}); code != exitCodeCommandError {
		t.Fatalf("plugins digest --write exit code mismatch: got %d want %d stdout=%s stderr=%s", code, exitCodeCommandError, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "must not be a symlink") {
		t.Fatalf("plugins digest stderr missing symlink error: %s", stderr.String())
	}
}

func TestApplicationPluginsInspectAndDoctorSuggestDigestCommandForStaleManifestDigest(t *testing.T) {
	t.Parallel()

	manifestPath := writeDraftSmokeManifest(t, "sha256:stale")
	configPath, _ := writeSmokePluginFilesWithManifest(t, manifestPath)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	if code := app.Run([]string{"plugins", "inspect", "--plugins-config", configPath}); code != exitCodeCommandError {
		t.Fatalf("plugins inspect exit code mismatch: got %d want %d stdout=%s stderr=%s", code, exitCodeCommandError, stdout.String(), stderr.String())
	}
	for _, want := range []string{manifestPath, "theater plugins digest --manifest", "--write"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("plugins inspect stderr missing %q: %s", want, stderr.String())
		}
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"plugins", "doctor", "--plugins-config", configPath}); code != 1 {
		t.Fatalf("plugins doctor exit code mismatch: got %d want 1 stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	for _, want := range []string{manifestPath, "theater plugins digest --manifest", "--write"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("plugins doctor stdout missing %q: %s", want, stdout.String())
		}
	}
}

func TestApplicationPluginsCommandErrorsSanitizeManifestPaths(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	manifestPath := filepath.Join(tempDir, "smoke-\x1b.manifest.json")
	writeDraftSmokeManifestAt(t, manifestPath, "sha256:stale")
	configPath, _ := writeSmokePluginFilesWithManifest(t, manifestPath)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	if code := app.Run([]string{"plugins", "inspect", "--plugins-config", configPath}); code != exitCodeCommandError {
		t.Fatalf("plugins inspect exit code mismatch: got %d want %d stdout=%s stderr=%s", code, exitCodeCommandError, stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), "\x1b") {
		t.Fatalf("plugins inspect stderr contains raw control character: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), `\x1b`) {
		t.Fatalf("plugins inspect stderr missing escaped control character: %q", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	missingManifestPath := filepath.Join(tempDir, "missing-\x1b.manifest.json")
	if code := app.Run([]string{"plugins", "digest", "--manifest", missingManifestPath}); code != exitCodeCommandError {
		t.Fatalf("plugins digest exit code mismatch: got %d want %d stdout=%s stderr=%s", code, exitCodeCommandError, stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), "\x1b") {
		t.Fatalf("plugins digest stderr contains raw control character: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), `\x1b`) {
		t.Fatalf("plugins digest stderr missing escaped control character: %q", stderr.String())
	}
}

func TestApplicationPluginsLockSuggestsDigestCommandForStaleManifestDigest(t *testing.T) {
	t.Parallel()

	manifestPath := writeDraftSmokeManifest(t, "sha256:stale")
	configPath, lockPath := writeSmokePluginFilesWithManifest(t, manifestPath)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	if code := app.Run([]string{"plugins", "lock", "-plugins-config", configPath, "-plugins-lock", lockPath}); code != exitCodeCommandError {
		t.Fatalf("plugins lock exit code mismatch: got %d want %d stdout=%s stderr=%s", code, exitCodeCommandError, stdout.String(), stderr.String())
	}

	output := stderr.String()
	for _, want := range []string{
		manifestPath,
		"descriptor_digest mismatch",
		"theater plugins digest --manifest",
		"--write",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("plugins lock stderr missing %q: %s", want, output)
		}
	}
}

func TestApplicationPluginsLockAndInspect(t *testing.T) {
	t.Parallel()

	configPath, lockPath := writeSmokePluginFiles(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	if code := app.Run([]string{"plugins", "lock", "-plugins-config", configPath, "-plugins-lock", lockPath}); code != 0 {
		t.Fatalf("plugins lock exit code: %d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file missing: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"plugins", "inspect", "-plugins-config", configPath, "-plugins-lock", lockPath}); code != 0 {
		t.Fatalf("plugins inspect exit code: %d stderr=%s", code, stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "smoke-plugin 0.2.0") {
		t.Fatalf("inspect output missing plugin header: %s", output)
	}
	if !strings.Contains(output, "inventory.smoke.echo") {
		t.Fatalf("inspect output missing inventory capability: %s", output)
	}
	if !strings.Contains(output, "action.smoke.echo") {
		t.Fatalf("inspect output missing action capability: %s", output)
	}
}

func TestApplicationPluginsCommandsUseEnvironmentDefaults(t *testing.T) {
	configPath, lockPath := writeSmokePluginFiles(t)
	t.Setenv(envPluginsConfig, configPath)
	t.Setenv(envPluginsLock, lockPath)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	if code := app.Run([]string{"plugins", "lock"}); code != 0 {
		t.Fatalf("plugins lock exit code: %d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file missing: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"plugins", "inspect"}); code != 0 {
		t.Fatalf("plugins inspect exit code: %d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "smoke-plugin 0.2.0") {
		t.Fatalf("inspect output missing plugin header: %s", stdout.String())
	}
}

func TestApplicationPluginsFlagsOverrideEnvironmentDefaults(t *testing.T) {
	configPath, lockPath := writeSmokePluginFiles(t)
	t.Setenv(envPluginsConfig, filepath.Join(t.TempDir(), "missing.plugins.json"))
	t.Setenv(envPluginsLock, filepath.Join(t.TempDir(), "missing.plugins.lock.json"))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	if code := app.Run([]string{"plugins", "lock", "-plugins-config", configPath, "-plugins-lock", lockPath}); code != 0 {
		t.Fatalf("plugins lock exit code: %d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file missing: %v", err)
	}
}

func TestApplicationPluginsDoctorWithoutLockReportsReady(t *testing.T) {
	t.Parallel()

	configPath, _ := writeSmokePluginFiles(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	if code := app.Run([]string{"plugins", "doctor", "-plugins-config", configPath}); code != 0 {
		t.Fatalf("plugins doctor exit code: %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	output := stdout.String()
	for _, want := range []string{
		"plugin registry: ready",
		"config schema and plugin registry load",
		"manifest and executable reachability",
		"lock drift: skipped because --plugins-lock was not provided",
		"smoke-plugin 0.2.0",
		"Run theater plugins lock to freeze the resolved manifest and executable checksums before validate or run.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("plugins doctor output missing %q: %s", want, output)
		}
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
}

func TestApplicationPluginsDoctorWithLockReportsReady(t *testing.T) {
	t.Parallel()

	configPath, lockPath := writeSmokePluginFiles(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	if code := app.Run([]string{"plugins", "lock", "-plugins-config", configPath, "-plugins-lock", lockPath}); code != 0 {
		t.Fatalf("plugins lock exit code: %d stderr=%s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"plugins", "doctor", "-plugins-config", configPath, "-plugins-lock", lockPath}); code != 0 {
		t.Fatalf("plugins doctor exit code: %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	output := stdout.String()
	for _, want := range []string{
		"plugin registry: ready",
		"lock file and checksum drift",
		lockPath + " matches 1 plugin checksum snapshot(s)",
		"Reuse the same --plugins-config and --plugins-lock paths with theater validate and theater run.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("plugins doctor output missing %q: %s", want, output)
		}
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
}

func TestApplicationPluginsDoctorReportsLockDrift(t *testing.T) {
	t.Parallel()

	configPath, lockPath := writeSmokePluginFiles(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	if code := app.Run([]string{"plugins", "lock", "-plugins-config", configPath, "-plugins-lock", lockPath}); code != 0 {
		t.Fatalf("plugins lock exit code: %d stderr=%s", code, stderr.String())
	}

	smokeScript := filepath.Join(filepath.Dir(configPath), "smoke.py")
	if err := os.WriteFile(smokeScript, []byte("#!/usr/bin/env python3\nprint('drift')\n"), 0o755); err != nil {
		t.Fatalf("rewrite smoke plugin: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"plugins", "doctor", "-plugins-config", configPath, "-plugins-lock", lockPath}); code != 1 {
		t.Fatalf("plugins doctor exit code mismatch: got %d want 1 stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	output := stdout.String()
	for _, want := range []string{
		"plugin registry: not ready",
		"FAIL  lock file and checksum drift:",
		`plugin "smoke-plugin" executable checksum mismatch`,
		"rerun theater plugins lock if the drift is intentional",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("plugins doctor output missing %q: %s", want, output)
		}
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
}

func TestApplicationPluginsDoctorReportsExecutablePathFailureWithoutLock(t *testing.T) {
	t.Parallel()

	configPath, _ := writeSmokePluginFiles(t)

	smokeScript := filepath.Join(filepath.Dir(configPath), "smoke.py")
	if err := os.Remove(smokeScript); err != nil {
		t.Fatalf("remove smoke plugin: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	if code := app.Run([]string{"plugins", "doctor", "-plugins-config", configPath}); code != 1 {
		t.Fatalf("plugins doctor exit code mismatch: got %d want 1 stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	output := stdout.String()
	for _, want := range []string{
		"plugin registry: not ready",
		"FAIL  config, manifest, and executable checks:",
		`plugin "smoke-plugin" executable: open `,
		"rerun theater plugins doctor",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("plugins doctor output missing %q: %s", want, output)
		}
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
}

func TestRenderPluginDoctorReportSanitizesControlCharacters(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newApplication(stdout, stderr)
	app.renderPluginDoctorReport(pluginDoctorReport{
		ConfigPath: "cfg\x1b[31m",
		LockPath:   "lock\tpath",
		Healthy:    false,
		Checks: []pluginDoctorCheck{
			{
				Status: checkStatusFail,
				Name:   "detail\x07name",
				Detail: "bad\nline\x1b[0m",
			},
		},
		Plugins: []pluginDoctorPluginView{
			{
				ID:              "plug\x1b[32m",
				Version:         "v1\rtest",
				ManifestPath:    "manifest\npath",
				ExecutablePath:  "exec\tpath",
				CapabilityCount: 1,
			},
		},
	})

	output := stdout.String()
	if strings.ContainsRune(output, '\x1b') || strings.ContainsRune(output, '\a') {
		t.Fatalf("doctor report must not contain raw control characters: %q", output)
	}
	for _, want := range []string{
		`cfg\x1b[31m`,
		"lock path",
		`detail\x07name`,
		`bad line\x1b[0m`,
		`plug\x1b[32m`,
		"v1 test",
		"manifest path",
		"exec path",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor report missing sanitized value %q: %q", want, output)
		}
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr mismatch: got %q want empty", got)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve repo root: caller unavailable")
	}

	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()

	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("encode JSON %s: %v", path, err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func writeSmokePluginFiles(t *testing.T) (string, string) {
	t.Helper()

	root := repoRoot(t)
	return writeSmokePluginFilesWithManifest(t, filepath.Join(root, "testdata", "plugins", "smoke", "manifest.json"))
}

func writeSmokePluginFilesWithManifest(t *testing.T, manifestPath string) (string, string) {
	t.Helper()

	root := repoRoot(t)
	tempDir := t.TempDir()
	smokeScript := filepath.Join(tempDir, "smoke.py")
	raw, err := os.ReadFile(filepath.Join(root, "testdata", "plugins", "smoke", "smoke.py"))
	if err != nil {
		t.Fatalf("read smoke plugin: %v", err)
	}
	if err := os.WriteFile(smokeScript, raw, 0o755); err != nil {
		t.Fatalf("write smoke plugin: %v", err)
	}

	config := pluginregistry.ConfigFile{
		Schema: pluginregistry.ConfigSchemaVersion,
		Plugins: map[string]pluginregistry.PluginEntry{
			"smoke-plugin": {
				Manifest: manifestPath,
				Exec: pluginregistry.ExecSpec{
					Command: []string{smokeScript},
				},
				AllowCapabilities: []string{
					"inventory.smoke.echo",
					"action.smoke.echo",
					"action.smoke.secret_fail",
					"action.smoke.sleep",
					"report_exporter.smoke.write",
					"state_backend.smoke.file",
					"transform.smoke.wrap",
					"matcher.smoke.equal",
				},
				Grants: pluginregistry.Grants{
					Env: map[string]string{
						"PATH": os.Getenv("PATH"),
					},
				},
			},
		},
	}

	configPath := filepath.Join(tempDir, "plugins.json")
	lockPath := filepath.Join(tempDir, "plugins.lock.json")
	writeJSONFile(t, configPath, config)
	return configPath, lockPath
}

func writeDraftSmokeManifest(t *testing.T, digest string) string {
	t.Helper()

	manifestPath := filepath.Join(t.TempDir(), fmt.Sprintf("smoke-%s.manifest.json", strings.ReplaceAll(digest, ":", "-")))
	writeDraftSmokeManifestAt(t, manifestPath, digest)
	return manifestPath
}

func writeDraftSmokeManifestAt(t *testing.T, manifestPath string, digest string) {
	t.Helper()

	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "testdata", "plugins", "smoke", "manifest.json"))
	if err != nil {
		t.Fatalf("read smoke manifest: %v", err)
	}

	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode smoke manifest: %v", err)
	}
	if digest == "" {
		delete(document, "descriptor_digest")
	} else {
		document["descriptor_digest"] = digest
	}

	writeJSONFile(t, manifestPath, document)
}

func readManifestDescriptorDigest(t *testing.T, path string) string {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	digest, ok := document["descriptor_digest"].(string)
	if !ok {
		t.Fatalf("manifest descriptor_digest missing or not a string: %#v", document["descriptor_digest"])
	}
	return digest
}
