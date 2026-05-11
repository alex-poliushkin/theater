package pluginhost

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sort"

	"github.com/alex-poliushkin/theater/internal/pluginregistry"
)

type transport struct {
	stdin  io.WriteCloser
	stdout io.Reader
	stderr *bytes.Buffer
	kill   func() error
	close  func(context.Context) error
}

func openTransport(
	_ context.Context,
	plugin pluginregistry.LoadedPlugin,
	env map[string]string,
) (transport, error) {
	return openNativeTransport(plugin, env)
}

func openNativeTransport(
	plugin pluginregistry.LoadedPlugin,
	env map[string]string,
) (transport, error) {
	process := exec.Command(plugin.ExecutablePath, plugin.Config.Exec.Command[1:]...)
	process.Env = processEnv(env)

	stderr := &bytes.Buffer{}
	stdout, err := process.StdoutPipe()
	if err != nil {
		return transport{}, fmt.Errorf("open plugin stdout pipe: %w", err)
	}
	stdin, err := process.StdinPipe()
	if err != nil {
		return transport{}, fmt.Errorf("open plugin stdin pipe: %w", err)
	}

	process.Stderr = stderr
	if err := process.Start(); err != nil {
		return transport{}, fmt.Errorf("start plugin process: %w", err)
	}

	kill := func() error {
		if process.Process == nil {
			return nil
		}
		return process.Process.Kill()
	}
	closeTransport := func(ctx context.Context) error {
		done := make(chan error, 1)
		go func() {
			done <- process.Wait()
		}()

		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			_ = kill()
			return <-done
		}
	}

	return transport{
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		kill:   kill,
		close:  closeTransport,
	}, nil
}

func processEnv(env map[string]string) []string {
	if len(env) == 0 {
		return []string{}
	}

	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	values := make([]string, 0, len(keys))
	for _, key := range keys {
		values = append(values, key+"="+env[key])
	}
	return values
}
