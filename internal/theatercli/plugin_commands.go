package theatercli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	internalpluginregistry "github.com/alex-poliushkin/theater/internal/pluginregistry"
	pluginmanifest "github.com/alex-poliushkin/theater/plugin/manifest"
	pluginregistry "github.com/alex-poliushkin/theater/plugin/registry"
)

const (
	commandPluginsDigest  = "digest"
	commandPluginsDoctor  = "doctor"
	commandPluginsInspect = "inspect"
	commandPluginsLock    = "lock"
)

type pluginCommandOptions struct {
	globalOptions
	format       outputFormat
	manifestPath string
	write        bool
}

type pluginCommandProfile struct {
	command         string
	requireConfig   bool
	requireLock     bool
	requireManifest bool
	allowFormat     bool
	allowWrite      bool
}

type pluginInspectView struct {
	ConfigPath string                    `json:"config_path"`
	LockPath   string                    `json:"lock_path,omitempty"`
	Plugins    []pluginInspectPluginView `json:"plugins"`
}

type pluginInspectPluginView struct {
	ID               string                        `json:"id"`
	Version          string                        `json:"version"`
	ManifestPath     string                        `json:"manifest_path"`
	ExecutablePath   string                        `json:"executable_path"`
	DescriptorDigest string                        `json:"descriptor_digest"`
	Capabilities     []pluginInspectCapabilityView `json:"capabilities"`
}

type pluginInspectCapabilityView struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type pluginDoctorReport struct {
	ConfigPath string
	LockPath   string
	Healthy    bool
	Checks     []pluginDoctorCheck
	Plugins    []pluginDoctorPluginView
}

type pluginDoctorCheck struct {
	Status string
	Name   string
	Detail string
}

type pluginDoctorPluginView struct {
	ID              string
	Version         string
	ManifestPath    string
	ExecutablePath  string
	CapabilityCount int
}

func (a *application) runPluginsCommand(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.stderr, "plugins requires a subcommand")
		a.commands.PrintCommand(a.stderr, a.commands.Must(commandPlugins), nil, a.textStyler(a.stderr))
		return exitCodeCommandError
	}
	if isHelpFlag(args[0]) {
		a.commands.PrintCommand(a.stderr, a.commands.Must(commandPlugins), nil, a.textStyler(a.stderr))
		return exitCodeCommandError
	}

	command := a.commands.Must(commandPlugins).subcommand(args[0])
	if command == nil {
		fmt.Fprintf(a.stderr, "unknown plugins subcommand %q\n", args[0])
		a.commands.PrintCommand(a.stderr, a.commands.Must(commandPlugins), nil, a.textStyler(a.stderr))
		return exitCodeCommandError
	}

	switch command.Name {
	case commandPluginsDigest:
		return a.digestPluginManifest(args[1:])
	case commandPluginsDoctor:
		return a.doctorPlugins(args[1:])
	case commandPluginsInspect:
		return a.inspectPlugins(args[1:])
	case commandPluginsLock:
		return a.lockPlugins(args[1:])
	default:
		fmt.Fprintf(a.stderr, "unknown plugins subcommand %q\n", args[0])
		a.commands.PrintCommand(a.stderr, a.commands.Must(commandPlugins), nil, a.textStyler(a.stderr))
		return exitCodeCommandError
	}
}

func (a *application) digestPluginManifest(args []string) int {
	options, ok := a.parsePluginCommandOptions(commandPluginsDigest, args)
	if !ok {
		return exitCodeCommandError
	}

	file, err := loadFinalizedPluginManifest(options.manifestPath)
	if err != nil {
		a.printDigestPluginManifestError(err)
		return exitCodeCommandError
	}

	if !options.write {
		fmt.Fprintln(a.stdout, file.DescriptorDigest)
		return 0
	}

	if err := writePluginManifest(options.manifestPath, file); err != nil {
		a.printDigestPluginManifestError(err)
		return exitCodeCommandError
	}
	fmt.Fprintf(a.stdout, "wrote %s\n", sanitizeCLIText(options.manifestPath))
	return 0
}

func isHelpFlag(raw string) bool {
	switch raw {
	case "-h", "--help":
		return true
	default:
		return false
	}
}

func (a *application) inspectPlugins(args []string) int {
	options, ok := a.parsePluginCommandOptions(commandPluginsInspect, args)
	if !ok {
		return exitCodeCommandError
	}

	loaded, err := internalpluginregistry.Load(options.pluginsConfig, options.pluginsLock)
	if err != nil {
		a.printPluginCommandError("inspect plugins", err)
		return exitCodeCommandError
	}

	if options.format == outputFormatJSON {
		raw, err := json.MarshalIndent(buildPluginInspectView(loaded), "", "  ")
		if err != nil {
			fmt.Fprintf(a.stderr, "inspect plugins: %v\n", err)
			return exitCodeCommandError
		}
		fmt.Fprintln(a.stdout, string(raw))
		return 0
	}

	ids := make([]string, 0, len(loaded.Plugins))
	for id := range loaded.Plugins {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		plugin := loaded.Plugins[id]
		fmt.Fprintf(a.stdout, "%s %s\n", plugin.Manifest.Plugin.ID, plugin.Manifest.Plugin.Version)
		fmt.Fprintf(a.stdout, "  manifest: %s\n", plugin.ManifestPath)
		fmt.Fprintf(a.stdout, "  executable: %s\n", plugin.ExecutablePath)
		fmt.Fprintf(a.stdout, "  digest: %s\n", plugin.Manifest.DescriptorDigest)
		names := make([]string, 0, len(plugin.Capabilities))
		for name := range plugin.Capabilities {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(a.stdout, "  capability: %s (%s)\n", name, plugin.Capabilities[name].Manifest.Kind)
		}
	}

	return 0
}

func buildPluginInspectView(loaded *internalpluginregistry.LoadedRegistry) pluginInspectView {
	ids := make([]string, 0, len(loaded.Plugins))
	for id := range loaded.Plugins {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	view := pluginInspectView{
		ConfigPath: loaded.ConfigPath,
		LockPath:   loaded.LockPath,
		Plugins:    make([]pluginInspectPluginView, 0, len(ids)),
	}
	for _, id := range ids {
		plugin := loaded.Plugins[id]
		names := make([]string, 0, len(plugin.Capabilities))
		for name := range plugin.Capabilities {
			names = append(names, name)
		}
		sort.Strings(names)

		item := pluginInspectPluginView{
			ID:               plugin.Manifest.Plugin.ID,
			Version:          plugin.Manifest.Plugin.Version,
			ManifestPath:     plugin.ManifestPath,
			ExecutablePath:   plugin.ExecutablePath,
			DescriptorDigest: plugin.Manifest.DescriptorDigest,
			Capabilities:     make([]pluginInspectCapabilityView, 0, len(names)),
		}
		for _, name := range names {
			item.Capabilities = append(item.Capabilities, pluginInspectCapabilityView{
				Name: name,
				Kind: string(plugin.Capabilities[name].Manifest.Kind),
			})
		}
		view.Plugins = append(view.Plugins, item)
	}

	return view
}

func (a *application) lockPlugins(args []string) int {
	options, ok := a.parsePluginCommandOptions(commandPluginsLock, args)
	if !ok {
		return exitCodeCommandError
	}

	loaded, err := internalpluginregistry.Load(options.pluginsConfig, "")
	if err != nil {
		a.printPluginCommandError("lock plugins", err)
		return exitCodeCommandError
	}

	lock := pluginregistry.LockFile{
		Schema:  pluginregistry.LockSchemaVersion,
		Plugins: make(map[string]pluginregistry.LockEntry, len(loaded.Plugins)),
	}
	ids := make([]string, 0, len(loaded.Plugins))
	for id := range loaded.Plugins {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		plugin := loaded.Plugins[id]
		lock.Plugins[id] = pluginregistry.LockEntry{
			ManifestSHA256:   plugin.ManifestSHA256,
			ExecutableSHA256: plugin.ExecutableSHA256,
		}
	}
	if err := pluginregistry.WriteLockFile(options.pluginsLock, lock); err != nil {
		fmt.Fprintf(a.stderr, "lock plugins: %v\n", err)
		return exitCodeCommandError
	}

	fmt.Fprintf(a.stdout, "wrote %s\n", options.pluginsLock)
	return 0
}

func (a *application) doctorPlugins(args []string) int {
	options, ok := a.parsePluginCommandOptions(commandPluginsDoctor, args)
	if !ok {
		return exitCodeCommandError
	}

	report, exitCode := buildPluginDoctorReport(options)
	a.renderPluginDoctorReport(report)
	return exitCode
}

func (a *application) parsePluginCommandOptions(command string, args []string) (pluginCommandOptions, bool) {
	profile := pluginCommandProfileFor(command)
	flags, options, values := a.newPluginCommandFlagSet(command)

	if err := flags.Parse(args); err != nil {
		return pluginCommandOptions{}, false
	}
	options.globalOptions = sharedGlobalOptionContract.Resolve(options.globalOptions)
	if flags.NArg() != 0 {
		fmt.Fprintf(a.stderr, "plugins %s does not accept positional arguments\n", command)
		return pluginCommandOptions{}, false
	}

	if profile.requireConfig && options.pluginsConfig == "" {
		fmt.Fprintf(a.stderr, "plugins %s requires --plugins-config\n", command)
		return pluginCommandOptions{}, false
	}
	if profile.requireLock && options.pluginsLock == "" {
		fmt.Fprintf(a.stderr, "plugins %s requires --plugins-lock\n", command)
		return pluginCommandOptions{}, false
	}
	if profile.requireManifest && options.manifestPath == "" {
		fmt.Fprintf(a.stderr, "plugins %s requires --manifest\n", command)
		return pluginCommandOptions{}, false
	}

	parsedFormat, err := parseValidationOutputFormat(values.format)
	if profile.allowFormat && err != nil {
		fmt.Fprintf(a.stderr, "%v\n", err)
		return pluginCommandOptions{}, false
	}
	if profile.allowFormat {
		options.format = parsedFormat
	}

	return *options, true
}

func (a *application) renderPluginDoctorReport(report pluginDoctorReport) {
	style := a.textStyler(a.stdout)
	status := "ready"
	if !report.Healthy {
		status = "not ready"
	}

	fmt.Fprintf(a.stdout, "plugin registry: %s\n", style.Status(status))
	fmt.Fprintf(a.stdout, "  config: %s\n", sanitizeCLIText(report.ConfigPath))
	if report.LockPath != "" {
		fmt.Fprintf(a.stdout, "  lock: %s\n", sanitizeCLIText(report.LockPath))
	}

	fmt.Fprintln(a.stdout, "checks:")
	for _, check := range report.Checks {
		if check.Detail == "" {
			fmt.Fprintf(a.stdout, "  %s  %s\n", style.Status(check.Status), sanitizeCLIText(check.Name))
			continue
		}
		fmt.Fprintf(
			a.stdout,
			"  %s  %s: %s\n",
			style.Status(check.Status),
			sanitizeCLIText(check.Name),
			sanitizeCLIText(check.Detail),
		)
	}

	if len(report.Plugins) != 0 {
		fmt.Fprintln(a.stdout, "plugins:")
		for _, plugin := range report.Plugins {
			fmt.Fprintf(a.stdout, "  %s %s\n", sanitizeCLIText(plugin.ID), sanitizeCLIText(plugin.Version))
			fmt.Fprintf(a.stdout, "    manifest: %s\n", sanitizeCLIText(plugin.ManifestPath))
			fmt.Fprintf(a.stdout, "    executable: %s\n", sanitizeCLIText(plugin.ExecutablePath))
			fmt.Fprintf(a.stdout, "    capabilities: %d\n", plugin.CapabilityCount)
		}
	}

	fmt.Fprintln(a.stdout, "next steps:")
	if report.Healthy {
		if report.LockPath == "" {
			fmt.Fprintln(a.stdout, "  Run theater plugins lock to freeze the resolved manifest and executable checksums before validate or run.")
			return
		}
		fmt.Fprintln(a.stdout, "  Reuse the same --plugins-config and --plugins-lock paths with theater validate and theater run.")
		return
	}
	if report.LockPath == "" {
		fmt.Fprintln(a.stdout, "  Fix the reported config, manifest, or executable problem, then rerun theater plugins doctor.")
		return
	}
	fmt.Fprintln(
		a.stdout,
		"  Fix the reported config, manifest, executable, or lock problem, "+
			"or rerun theater plugins lock if the drift is intentional.",
	)
}

func buildPluginDoctorReport(options pluginCommandOptions) (report pluginDoctorReport, exitCode int) {
	report = pluginDoctorReport{
		ConfigPath: options.pluginsConfig,
		LockPath:   options.pluginsLock,
		Healthy:    true,
	}

	loaded, err := internalpluginregistry.Load(options.pluginsConfig, "")
	if err != nil {
		report.Healthy = false
		report.Checks = append(report.Checks, pluginDoctorCheck{
			Status: checkStatusFail,
			Name:   "config, manifest, and executable checks",
			Detail: pluginCommandErrorDetail(err),
		})
		exitCode = 1
		return report, exitCode
	}

	report.Checks = append(report.Checks,
		pluginDoctorCheck{
			Status: checkStatusOK,
			Name:   "config schema and plugin registry load",
			Detail: fmt.Sprintf("%d plugin(s) resolved from %s", len(loaded.Plugins), loaded.ConfigPath),
		},
		pluginDoctorCheck{
			Status: checkStatusOK,
			Name:   "manifest and executable reachability",
			Detail: fmt.Sprintf("%d plugin(s) reachable with manifest and executable digests", len(loaded.Plugins)),
		},
	)
	report.Plugins = buildPluginDoctorPlugins(loaded)

	if options.pluginsLock == "" {
		report.Checks = append(report.Checks, pluginDoctorCheck{
			Status: checkStatusOK,
			Name:   "lock drift",
			Detail: "skipped because --plugins-lock was not provided",
		})
		return report, exitCode
	}

	locked, err := internalpluginregistry.Load(options.pluginsConfig, options.pluginsLock)
	if err != nil {
		report.Healthy = false
		report.Checks = append(report.Checks, pluginDoctorCheck{
			Status: checkStatusFail,
			Name:   "lock file and checksum drift",
			Detail: pluginCommandErrorDetail(err),
		})
		exitCode = 1
		return report, exitCode
	}

	report.Checks = append(report.Checks, pluginDoctorCheck{
		Status: checkStatusOK,
		Name:   "lock file and checksum drift",
		Detail: fmt.Sprintf("%s matches %d plugin checksum snapshot(s)", locked.LockPath, len(locked.Plugins)),
	})
	return report, exitCode
}

func buildPluginDoctorPlugins(loaded *internalpluginregistry.LoadedRegistry) []pluginDoctorPluginView {
	ids := make([]string, 0, len(loaded.Plugins))
	for id := range loaded.Plugins {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	plugins := make([]pluginDoctorPluginView, 0, len(ids))
	for _, id := range ids {
		plugin := loaded.Plugins[id]
		plugins = append(plugins, pluginDoctorPluginView{
			ID:              plugin.Manifest.Plugin.ID,
			Version:         plugin.Manifest.Plugin.Version,
			ManifestPath:    plugin.ManifestPath,
			ExecutablePath:  plugin.ExecutablePath,
			CapabilityCount: len(plugin.Capabilities),
		})
	}

	return plugins
}

func pluginCommandProfileFor(command string) pluginCommandProfile {
	switch command {
	case commandPluginsDigest:
		return pluginCommandProfile{
			command:         commandPluginsDigest,
			requireManifest: true,
			allowWrite:      true,
		}
	case commandPluginsInspect:
		return pluginCommandProfile{
			command:       commandPluginsInspect,
			requireConfig: true,
			allowFormat:   true,
		}
	case commandPluginsLock:
		return pluginCommandProfile{
			command:       commandPluginsLock,
			requireConfig: true,
			requireLock:   true,
		}
	case commandPluginsDoctor:
		return pluginCommandProfile{
			command:       commandPluginsDoctor,
			requireConfig: true,
		}
	default:
		return pluginCommandProfile{command: command}
	}
}

func loadFinalizedPluginManifest(path string) (pluginmanifest.File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return pluginmanifest.File{}, fmt.Errorf("read plugin manifest: %w", err)
	}

	file, err := pluginmanifest.UnmarshalDraftFile(raw)
	if err != nil {
		return pluginmanifest.File{}, err
	}

	return pluginmanifest.Finalize(file)
}

func writePluginManifest(path string, file pluginmanifest.File) error {
	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("encode plugin manifest: %w", err)
	}
	raw = append(raw, '\n')

	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat plugin manifest: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("plugin manifest path must not be a symlink")
	}

	tempFile, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary plugin manifest: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()

	if _, err := tempFile.Write(raw); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("write temporary plugin manifest: %w", err)
	}
	if err := tempFile.Chmod(info.Mode().Perm()); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("chmod temporary plugin manifest: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temporary plugin manifest: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("write plugin manifest: %w", err)
	}
	return nil
}

func (a *application) printPluginCommandError(prefix string, err error) {
	fmt.Fprintf(a.stderr, "%s: %s\n", prefix, sanitizeCLIText(pluginCommandErrorDetail(err)))
}

func (a *application) printDigestPluginManifestError(err error) {
	fmt.Fprintf(a.stderr, "digest plugin manifest: %s\n", sanitizeCLIText(err.Error()))
}

func pluginCommandErrorDetail(err error) string {
	detail := err.Error()
	if !pluginManifestDigestNeedsRefresh(err) {
		return detail
	}
	return detail + "; run theater plugins digest --manifest <manifest path> --write to refresh descriptor_digest"
}

func pluginManifestDigestNeedsRefresh(err error) bool {
	return errors.Is(err, pluginmanifest.ErrDescriptorDigestMismatch) ||
		errors.Is(err, pluginmanifest.ErrDescriptorDigestRequired)
}
