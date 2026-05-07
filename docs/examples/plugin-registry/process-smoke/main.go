package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const registryPath = "docs/examples/plugin-registry/hello-world.plugins.json"

type registryFile struct {
	Plugins map[string]pluginEntry `json:"plugins"`
}

type pluginEntry struct {
	Exec execSpec `json:"exec"`
}

type execSpec struct {
	Command []string `json:"command"`
}

func main() {
	args, err := registryCommand(registryPath, "hello-world")
	if err != nil {
		fmt.Fprintf(os.Stderr, "read registry command: %v\n", err)
		os.Exit(1)
	}

	process := exec.Command(args[0], args[1:]...) //nolint:gosec // Docs smoke intentionally runs the checked registry command.
	process.Stdin = strings.NewReader("")
	process.Stdout = os.Stdout
	process.Stderr = os.Stderr
	if err := process.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "start hello-world plugin: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("plugin process smoke: ok")
}

func registryCommand(path, id string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var registry registryFile
	if err := json.Unmarshal(raw, &registry); err != nil {
		return nil, err
	}
	entry, ok := registry.Plugins[id]
	if !ok {
		return nil, fmt.Errorf("plugin %q is not declared", id)
	}
	if len(entry.Exec.Command) == 0 {
		return nil, errors.New("exec.command is empty")
	}
	return entry.Exec.Command, nil
}
