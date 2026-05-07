package theatercli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	internalpluginregistry "github.com/alex-poliushkin/theater/internal/pluginregistry"
	pluginregistry "github.com/alex-poliushkin/theater/plugin/registry"
)

const (
	doctorReadyStatus       = "ready"
	doctorNotReadyStatus    = "not ready"
	doctorWritableProbeName = ".theater-doctor-*"
)

type doctorOptions struct {
	globalOptions
	writePaths []string
}

type doctorReport struct {
	WorkingDir   string
	Healthy      bool
	PluginInputs bool
	Checks       []doctorCheck
}

type doctorCheck struct {
	Status string
	Name   string
	Detail string
}

func (a *application) doctorCommand(args []string) int {
	options, ok := a.parseDoctorOptions(args)
	if !ok {
		return exitCodeCommandError
	}

	report, exitCode := a.buildDoctorReport(options)
	a.renderDoctorReport(report)
	return exitCode
}

func (a *application) parseDoctorOptions(args []string) (doctorOptions, bool) {
	flags, options := a.newDoctorCommandFlagSet()
	if err := flags.Parse(args); err != nil {
		return doctorOptions{}, false
	}

	options.globalOptions = sharedGlobalOptionContract.Resolve(options.globalOptions)
	if flags.NArg() != 0 {
		fmt.Fprintln(a.stderr, "doctor does not accept positional arguments")
		return doctorOptions{}, false
	}

	return *options, true
}

func (a *application) buildDoctorReport(options doctorOptions) (report doctorReport, exitCode int) {
	report = doctorReport{
		Healthy:      true,
		PluginInputs: options.pluginsConfig != "" || options.pluginsLock != "",
	}

	workingDir, err := os.Getwd()
	if err != nil {
		report.addCheck(false, "working directory", err.Error())
		exitCode = 1
		return report, exitCode
	}
	report.WorkingDir = workingDir

	if layout, ok := resolveRepoLayout(workingDir); ok {
		report.addCheck(
			true,
			"repo-aware flow layout",
			fmt.Sprintf(
				"repo root %s with %s and %s",
				layout.RepoRoot,
				layout.FlowRoot,
				layout.LibraryRoot,
			),
		)
	} else {
		report.addCheck(
			false,
			"repo-aware flow layout",
			fmt.Sprintf(
				"current directory %s is not inside a repo with theater/flows and theater/lib",
				workingDir,
			),
		)
		exitCode = 1
	}

	stdinTTY := a.isInputTerminal(a.stdin)
	stderrTTY := a.isTerminal(a.stderr)
	if stdinTTY && stderrTTY {
		report.addCheck(true, "interactive debug TTY", "stdin and stderr are TTYs")
	} else {
		report.addAdvisory(
			false,
			"interactive debug TTY",
			fmt.Sprintf(
				"stdin_tty=%t stderr_tty=%t; interactive debug requires a TTY on stdin and stderr; use --debug dump instead",
				stdinTTY,
				stderrTTY,
			),
		)
	}

	pluginExitCode := appendDoctorPluginChecks(&report, options.pluginsConfig, options.pluginsLock)
	if pluginExitCode != 0 {
		exitCode = 1
	}

	if ok, detail := checkDoctorWritePaths(options.writePaths); ok {
		report.addCheck(true, "writable output destinations", detail)
	} else {
		report.addCheck(false, "writable output destinations", detail)
		exitCode = 1
	}

	return report, exitCode
}

func (a *application) renderDoctorReport(report doctorReport) {
	style := a.textStyler(a.stdout)
	status := doctorReadyStatus
	if !report.Healthy {
		status = doctorNotReadyStatus
	}

	fmt.Fprintf(a.stdout, "theater doctor: %s\n", style.Status(status))
	if report.WorkingDir != "" {
		fmt.Fprintf(a.stdout, "  cwd: %s\n", sanitizeCLIText(report.WorkingDir))
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

	fmt.Fprintln(a.stdout, "next steps:")
	if report.Healthy {
		fmt.Fprintln(
			a.stdout,
			"  Reuse the same plugin registry file, plugin lock file, and writable destinations "+
				"with theater validate or theater run.",
		)
		fmt.Fprintln(a.stdout, "  Run theater plugins doctor when you need checksum-drift diagnostics for the plugin registry.")
		return
	}

	fmt.Fprintln(a.stdout, "  Fix the failing checks and rerun theater doctor before validate or run.")
	if report.PluginInputs {
		fmt.Fprintln(a.stdout, "  Run theater plugins doctor for deeper plugin registry diagnostics once the basic pair is in place.")
	}
}

func (r *doctorReport) addCheck(ok bool, name, detail string) {
	status := checkStatusOK
	if !ok {
		status = checkStatusFail
		r.Healthy = false
	}

	r.Checks = append(r.Checks, doctorCheck{
		Status: status,
		Name:   name,
		Detail: detail,
	})
}

func (r *doctorReport) addAdvisory(ok bool, name, detail string) {
	status := checkStatusOK
	if !ok {
		status = checkStatusWarn
	}

	r.Checks = append(r.Checks, doctorCheck{
		Status: status,
		Name:   name,
		Detail: detail,
	})
}

func appendDoctorPluginChecks(report *doctorReport, configPath, lockPath string) int {
	if configPath == "" && lockPath == "" {
		report.addCheck(true, "plugin registry file and lock file pairing", "skipped because no plugin registry file or lock file was provided")
		report.addCheck(true, "plugin executable reachability", "skipped because no plugin registry file was provided")
		return 0
	}

	exitCode := 0
	switch {
	case configPath == "":
		report.addCheck(false, "plugin registry file and lock file pairing", "--plugins-lock was provided without --plugins-config")
		report.addCheck(true, "plugin executable reachability", "skipped because --plugins-config was not provided")
		return 1
	case lockPath == "":
		report.addCheck(false, "plugin registry file and lock file pairing", "--plugins-config was provided without --plugins-lock")
		exitCode = 1
	default:
		if _, err := pluginregistry.LoadLockFile(lockPath); err != nil {
			report.addCheck(false, "plugin registry file and lock file pairing", err.Error())
			exitCode = 1
		} else {
			report.addCheck(
				true,
				"plugin registry file and lock file pairing",
				fmt.Sprintf("registry file %s paired with lock file %s", configPath, lockPath),
			)
		}
	}

	loaded, err := internalpluginregistry.Load(configPath, "")
	if err != nil {
		report.addCheck(false, "plugin executable reachability", err.Error())
		return 1
	}

	report.addCheck(
		true,
		"plugin executable reachability",
		fmt.Sprintf("%d plugin(s) reachable from %s", len(loaded.Plugins), loaded.ConfigPath),
	)
	return exitCode
}

func checkDoctorWritePaths(paths []string) (ok bool, detail string) {
	if len(paths) == 0 {
		return true, "skipped because no --write-path values were provided"
	}

	for i := range paths {
		if err := checkDoctorWritePath(paths[i]); err != nil {
			return false, fmt.Sprintf("%s: %v", paths[i], err)
		}
	}

	if len(paths) == 1 {
		return true, paths[0] + " is writable"
	}
	return true, fmt.Sprintf("%d path(s) are writable", len(paths))
}

func checkDoctorWritePath(path string) error {
	if pathEndsWithSeparator(path) {
		return errors.New("path must name a file")
	}

	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	info, err := os.Stat(absolutePath)
	switch {
	case err == nil:
		if info.IsDir() {
			return errors.New("path is a directory")
		}
		if !info.Mode().IsRegular() {
			return errors.New("path is not a regular file")
		}
		file, err := os.OpenFile(absolutePath, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			return err
		}
		return file.Close()
	case !os.IsNotExist(err):
		return err
	}

	parent := filepath.Dir(absolutePath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("parent directory: %w", err)
	}

	probe, err := os.CreateTemp(parent, doctorWritableProbeName)
	if err != nil {
		return err
	}
	probePath := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(probePath)
		return err
	}
	if err := os.Remove(probePath); err != nil {
		return err
	}

	return nil
}

func pathEndsWithSeparator(path string) bool {
	return path != "" && os.IsPathSeparator(path[len(path)-1])
}
