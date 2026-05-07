package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"

	pluginmanifest "github.com/alex-poliushkin/theater/plugin/manifest"
	pluginprotocol "github.com/alex-poliushkin/theater/plugin/protocol"
	"github.com/alex-poliushkin/theater/plugin/sdk"
)

//go:embed manifest.json
var manifestJSON []byte

func main() {
	file, err := pluginmanifest.UnmarshalFile(manifestJSON)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	server := sdk.NewServer(file)
	if err := server.RegisterInventory(sdk.InventoryHandler{
		Capability: mustCapability(file, "inventory.hello_world.message"),
		Validate:   validateMessage,
		Resolve:    resolveMessage,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := server.RegisterAction(sdk.ActionHandler{
		Capability: mustCapability(file, "action.hello_world.echo"),
		Validate:   validateEcho,
		Invoke:     invokeEcho,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := server.ServeStdio(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func mustCapability(file pluginmanifest.File, name string) pluginmanifest.Capability {
	for i := range file.Capabilities {
		if file.Capabilities[i].Name == name {
			return file.Capabilities[i]
		}
	}

	panic("capability not found: " + name)
}

func validateMessage(_ context.Context, params pluginprotocol.ValidateParams) (pluginprotocol.ValidateResult, error) {
	return pluginprotocol.ValidateResult{
		Diagnostics: validateNameProperty(params.Properties),
	}, nil
}

func validateEcho(_ context.Context, params pluginprotocol.ValidateParams) (pluginprotocol.ValidateResult, error) {
	message, ok := stringProperty(params.Properties, "message")
	if ok && strings.TrimSpace(message) == "" {
		return pluginprotocol.ValidateResult{
			Diagnostics: []pluginprotocol.ValidationDiagnostic{{
				Path:    "/message",
				Message: "message must not be empty",
			}},
		}, nil
	}

	return pluginprotocol.ValidateResult{}, nil
}

func resolveMessage(
	_ context.Context,
	_ sdk.Emitter,
	params pluginprotocol.InventoryResolveParams,
) (pluginprotocol.InventoryResolveResult, error) {
	name, err := requiredStringProperty(params.Properties, "name")
	if err != nil {
		return pluginprotocol.InventoryResolveResult{}, err
	}

	greeting := stringPropertyDefault(params.Properties, "greeting", "Hello")
	return pluginprotocol.InventoryResolveResult{
		Value: fmt.Sprintf("%s, %s!", greeting, name),
	}, nil
}

func invokeEcho(
	_ context.Context,
	emitter sdk.Emitter,
	params pluginprotocol.ActionInvokeParams,
) (pluginprotocol.ActionInvokeResult, error) {
	message, err := requiredStringProperty(params.Properties, "message")
	if err != nil {
		return pluginprotocol.ActionInvokeResult{}, err
	}

	_ = emitter.Log("hello-world action invoked", map[string]string{
		"message": message,
	})

	return pluginprotocol.ActionInvokeResult{
		Outputs: map[string]any{
			"message": message,
		},
	}, nil
}

func validateNameProperty(properties map[string]any) []pluginprotocol.ValidationDiagnostic {
	name, ok := stringProperty(properties, "name")
	if ok && strings.TrimSpace(name) == "" {
		return []pluginprotocol.ValidationDiagnostic{{
			Path:    "/name",
			Message: "name must not be empty",
		}}
	}

	return nil
}

func requiredStringProperty(properties map[string]any, key string) (string, error) {
	value, ok := stringProperty(properties, key)
	if !ok {
		return "", fmt.Errorf("property %q must be a string", key)
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("property %q is required", key)
	}

	return value, nil
}

func stringProperty(properties map[string]any, key string) (string, bool) {
	if properties == nil {
		return "", false
	}

	value, ok := properties[key].(string)
	return value, ok
}

func stringPropertyDefault(properties map[string]any, key, fallback string) string {
	value, ok := stringProperty(properties, key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}

	return value
}
