package thtrlsp

import (
	"strings"
	"testing"

	"github.com/alex-poliushkin/theater"
)

func TestPluginDescriptorCompletionsUseDescriptorMetadata(t *testing.T) {
	t.Parallel()

	descriptors := pluginDescriptorTestCapabilities()

	actionText := `stage smoke
scenario plugin
  act echo
    do action.sm`
	actionItems := completionItemsForDocumentWithCapabilities(actionText, lspPosition{
		Line:      3,
		Character: len("    do action.sm"),
	}, descriptors)
	actionItem, ok := findCompletionItem(actionItems, "action.smoke.echo")
	if !ok {
		t.Fatalf("plugin action completion missing: %#v", actionItems)
	}
	if !strings.Contains(actionItem.Detail, "plugin action from smoke-plugin@0.2.0") ||
		!strings.Contains(actionItem.Detail, "Emit a simple echo output") {
		t.Fatalf("plugin action completion detail must come from descriptor metadata: %#v", actionItem)
	}

	matcherText := `stage smoke
scenario plugin
  act echo
    expect echoed: field(echo) assert matcher.sm`
	matcherItems := completionItemsForDocumentWithCapabilities(matcherText, lspPosition{
		Line:      3,
		Character: len("    expect echoed: field(echo) assert matcher.sm"),
	}, descriptors)
	matcherItem, ok := findCompletionItem(matcherItems, "matcher.smoke.equal")
	if !ok {
		t.Fatalf("plugin matcher completion missing: %#v", matcherItems)
	}
	if !strings.Contains(matcherItem.Detail, "plugin matcher from smoke-plugin@0.2.0") ||
		!strings.Contains(matcherItem.Detail, "Assert that the actual string equals the expected string") {
		t.Fatalf("plugin matcher completion detail must come from descriptor metadata: %#v", matcherItem)
	}

	transformText := `stage smoke
scenario plugin
  act load
    prop wrapped = inventory.http.get(url: "/payload") | transform.sm`
	transformItems := completionItemsForDocumentWithCapabilities(transformText, lspPosition{
		Line:      3,
		Character: len(`    prop wrapped = inventory.http.get(url: "/payload") | transform.sm`),
	}, descriptors)
	transformItem, ok := findCompletionItem(transformItems, "transform.smoke.wrap")
	if !ok {
		t.Fatalf("plugin transform completion missing: %#v", transformItems)
	}
	if !strings.Contains(transformItem.Detail, "plugin transform from smoke-plugin@0.2.0") ||
		!strings.Contains(transformItem.Detail, "Wrap a value for assertions") {
		t.Fatalf("plugin transform completion detail must come from descriptor metadata: %#v", transformItem)
	}

	selectorTransformText := `stage smoke
scenario plugin
  act load
    expect wrapped: field(body) | transform.sm`
	selectorTransformItems := completionItemsForDocumentWithCapabilities(selectorTransformText, lspPosition{
		Line:      3,
		Character: len(`    expect wrapped: field(body) | transform.sm`),
	}, descriptors)
	if !containsCompletionLabel(selectorTransformItems, "transform.smoke.wrap") {
		t.Fatalf("plugin selector transform completion missing: %#v", selectorTransformItems)
	}

	stateBackendText := `stage smoke
state
  backend smoke = state_backend.sm`
	stateBackendItems := completionItemsForDocumentWithCapabilities(stateBackendText, lspPosition{
		Line:      2,
		Character: len(`  backend smoke = state_backend.sm`),
	}, descriptors)
	stateBackendItem, ok := findCompletionItem(stateBackendItems, "state_backend.smoke.file")
	if !ok {
		t.Fatalf("plugin state backend completion missing: %#v", stateBackendItems)
	}
	if !strings.Contains(stateBackendItem.Detail, "plugin state backend from smoke-plugin@0.2.0") ||
		!strings.Contains(stateBackendItem.Detail, "Store state in a smoke fixture") {
		t.Fatalf("plugin state backend completion detail must come from descriptor metadata: %#v", stateBackendItem)
	}

	transformArgText := `stage smoke
scenario plugin
  act load
    prop wrapped = inventory.http.get(url: "/payload") | transform.smoke.wrap(p`
	transformArgItems := completionItemsForDocumentWithCapabilities(transformArgText, lspPosition{
		Line:      3,
		Character: len(`    prop wrapped = inventory.http.get(url: "/payload") | transform.smoke.wrap(p`),
	}, descriptors)
	if !containsCompletionLabel(transformArgItems, "prefix") {
		t.Fatalf("plugin transform argument completion missing: %#v", transformArgItems)
	}

	selectorTransformArgText := `stage smoke
scenario plugin
  act load
    expect wrapped: field(body) | transform.smoke.wrap(p`
	selectorTransformArgItems := completionItemsForDocumentWithCapabilities(selectorTransformArgText, lspPosition{
		Line:      3,
		Character: len(`    expect wrapped: field(body) | transform.smoke.wrap(p`),
	}, descriptors)
	if !containsCompletionLabel(selectorTransformArgItems, "prefix") {
		t.Fatalf("plugin selector transform argument completion missing: %#v", selectorTransformArgItems)
	}

	stateBackendArgText := `stage smoke
state
  backend smoke = state_backend.smoke.file(r`
	stateBackendArgItems := completionItemsForDocumentWithCapabilities(stateBackendArgText, lspPosition{
		Line:      2,
		Character: len(`  backend smoke = state_backend.smoke.file(r`),
	}, descriptors)
	if !containsCompletionLabel(stateBackendArgItems, "root") {
		t.Fatalf("plugin state backend argument completion missing: %#v", stateBackendArgItems)
	}
}

func TestPluginDescriptorHoverAndSignatureUseDescriptorMetadata(t *testing.T) {
	t.Parallel()

	descriptors := pluginDescriptorTestCapabilities()
	text := `stage smoke
scenario plugin
  act echo
    do action.smoke.echo(value: "hello")
    expect echoed: field(echo) assert matcher.smoke.equal(expected: "hello")
`

	hover := hoverForDocument(text, lspPosition{
		Line:      3,
		Character: len("    do action.smoke"),
	}, descriptors)
	if hover == nil {
		t.Fatal("expected plugin action hover")
	}
	for _, want := range []string{
		"`action.smoke.echo`",
		"Emit a simple echo output",
		"Signature: `action.smoke.echo(value: string)`",
		"`value`: `string` required",
	} {
		if !strings.Contains(hover.Contents.Value, want) {
			t.Fatalf("plugin action hover missing %q:\n%s", want, hover.Contents.Value)
		}
	}

	help := signatureHelpForDocument(text, lspPosition{
		Line:      3,
		Character: len(`    do action.smoke.echo(`),
	}, descriptors)
	if got, want := len(help.Signatures), 1; got != want {
		t.Fatalf("signature count mismatch: got %d want %d", got, want)
	}
	if got, want := help.Signatures[0].Label, "action.smoke.echo(value: string)"; got != want {
		t.Fatalf("signature label mismatch: got %q want %q", got, want)
	}

	matcherHelp := signatureHelpForDocument(text, lspPosition{
		Line:      4,
		Character: len(`    expect echoed: field(echo) assert matcher.smoke.equal(`),
	}, descriptors)
	if got, want := len(matcherHelp.Signatures), 1; got != want {
		t.Fatalf("matcher signature count mismatch: got %d want %d", got, want)
	}
	if got, want := matcherHelp.Signatures[0].Label, "matcher.smoke.equal(expected: string)"; got != want {
		t.Fatalf("matcher signature label mismatch: got %q want %q", got, want)
	}

	transformHelp := signatureHelpForDocument(`stage smoke
scenario plugin
  act load
    prop wrapped = inventory.http.get(url: "/payload") | transform.smoke.wrap(prefix: "demo")
`, lspPosition{
		Line:      3,
		Character: len(`    prop wrapped = inventory.http.get(url: "/payload") | transform.smoke.wrap(`),
	}, descriptors)
	if got, want := len(transformHelp.Signatures), 1; got != want {
		t.Fatalf("transform signature count mismatch: got %d want %d", got, want)
	}
	if got, want := transformHelp.Signatures[0].Label, "transform.smoke.wrap(prefix: string)"; got != want {
		t.Fatalf("transform signature label mismatch: got %q want %q", got, want)
	}

	stateBackendHelp := signatureHelpForDocument(`stage smoke
state
  backend smoke = state_backend.smoke.file(root: "/tmp/theater-state")
`, lspPosition{
		Line:      2,
		Character: len(`  backend smoke = state_backend.smoke.file(`),
	}, descriptors)
	if got, want := len(stateBackendHelp.Signatures), 1; got != want {
		t.Fatalf("state backend signature count mismatch: got %d want %d", got, want)
	}
	if got, want := stateBackendHelp.Signatures[0].Label, "state_backend.smoke.file(root: string)"; got != want {
		t.Fatalf("state backend signature label mismatch: got %q want %q", got, want)
	}
}

func TestPluginDescriptorSignatureHelpTracksActiveParameter(t *testing.T) {
	t.Parallel()

	text := `stage smoke
scenario plugin
  act echo
    do action.smoke.pair(first: "hello", second: "world")
`

	help := signatureHelpForDocument(text, lspPosition{
		Line:      3,
		Character: len(`    do action.smoke.pair(first: "hello", second`),
	}, pluginDescriptorTestCapabilities())
	if got, want := len(help.Signatures), 1; got != want {
		t.Fatalf("signature count mismatch: got %d want %d", got, want)
	}
	if got, want := help.Signatures[0].Label, "action.smoke.pair(first: string, second: string)"; got != want {
		t.Fatalf("signature label mismatch: got %q want %q", got, want)
	}
	if got, want := help.ActiveParameter, 1; got != want {
		t.Fatalf("active parameter mismatch: got %d want %d", got, want)
	}
}

func pluginDescriptorTestCapabilities() []theater.CapabilityDescriptor {
	provider := theater.CapabilityProvider{
		Kind:          theater.CapabilityProviderPlugin,
		PluginID:      "smoke-plugin",
		PluginVersion: "0.2.0",
	}
	return []theater.CapabilityDescriptor{
		{
			Family:   theater.CapabilityFamilyAction,
			Ref:      "action.smoke.echo",
			Summary:  "Emit a simple echo output",
			Provider: provider,
			Action: &theater.ActionCapabilityDescriptor{
				Inputs: map[string]theater.ValueContract{
					"value": {
						Kind:        theater.ValueKindString,
						Required:    true,
						Description: "Value to echo.",
					},
				},
				Outputs: map[string]theater.ValueContract{
					"echo": {Kind: theater.ValueKindString, Required: true},
				},
			},
		},
		{
			Family:   theater.CapabilityFamilyAction,
			Ref:      "action.smoke.pair",
			Summary:  "Emit two string inputs",
			Provider: provider,
			Action: &theater.ActionCapabilityDescriptor{
				Inputs: map[string]theater.ValueContract{
					"first":  {Kind: theater.ValueKindString, Required: true},
					"second": {Kind: theater.ValueKindString, Required: true},
				},
				Outputs: map[string]theater.ValueContract{
					"combined": {Kind: theater.ValueKindString, Required: true},
				},
			},
		},
		{
			Family:   theater.CapabilityFamilyMatcher,
			Ref:      "matcher.smoke.equal",
			Summary:  "Assert that the actual string equals the expected string",
			Provider: provider,
			Matcher: &theater.MatcherCapabilityDescriptor{
				Args: []theater.MatcherArg{
					{
						Name:     "expected",
						Accepts:  theater.ValueContract{Kind: theater.ValueKindString},
						Required: true,
						Summary:  "Expected string.",
					},
				},
				Actual: theater.ValueContract{Kind: theater.ValueKindString},
			},
		},
		{
			Family:   theater.CapabilityFamilyTransform,
			Ref:      "transform.smoke.wrap",
			Summary:  "Wrap a value for assertions",
			Provider: provider,
			Transform: &theater.TransformCapabilityDescriptor{
				Accepts: theater.ValueContract{Kind: theater.ValueKindString},
				Params: []theater.ParamSpec{
					{
						Name:     "prefix",
						Accepts:  theater.ValueContract{Kind: theater.ValueKindString},
						Required: true,
					},
				},
				Produces: theater.ValueContract{Kind: theater.ValueKindString},
			},
		},
		{
			Family:   theater.CapabilityFamilyStateBackend,
			Ref:      "state_backend.smoke.file",
			Summary:  "Store state in a smoke fixture",
			Provider: provider,
			StateBackend: &theater.StateBackendCapabilityDescriptor{
				Descriptor: theater.StateDescriptor{
					Guarantee:   theater.StateGuaranteeLocalAtomic,
					SupportsCAS: true,
				},
				Params: []theater.ParamSpec{
					{
						Name:     "root",
						Accepts:  theater.ValueContract{Kind: theater.ValueKindString},
						Required: true,
					},
				},
			},
		},
	}
}

func findCompletionItem(items []lspCompletionItem, label string) (lspCompletionItem, bool) {
	for i := range items {
		if items[i].Label == label {
			return items[i], true
		}
	}

	return lspCompletionItem{}, false
}
