package theater_test

import (
	"testing"

	theater "github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/builtin"
)

func TestDescribeCapabilitiesIncludesBuiltInFamilies(t *testing.T) {
	t.Parallel()

	bundle, err := builtin.NewBundle()
	if err != nil {
		t.Fatalf("builtins: %v", err)
	}

	descriptors := theater.DescribeCapabilities(bundle.Catalog, bundle.Matchers)
	if len(descriptors) == 0 {
		t.Fatal("expected built-in capability descriptors")
	}

	tests := []struct {
		ref    string
		family theater.CapabilityFamily
	}{
		{ref: "action.http", family: theater.CapabilityFamilyAction},
		{ref: "inventory.http.get", family: theater.CapabilityFamilyInventory},
		{ref: "json.decode", family: theater.CapabilityFamilyTransform},
		{ref: "expectation.equal", family: theater.CapabilityFamilyMatcher},
		{ref: "email", family: theater.CapabilityFamilyGenerator},
		{ref: "state.backend.file", family: theater.CapabilityFamilyStateBackend},
	}

	for _, test := range tests {
		test := test
		t.Run(test.ref, func(t *testing.T) {
			t.Parallel()

			descriptor, ok := findCapability(descriptors, test.ref)
			if !ok {
				t.Fatalf("missing capability %q", test.ref)
			}
			if got, want := descriptor.Family, test.family; got != want {
				t.Fatalf("family mismatch: got %q want %q", got, want)
			}
			if got, want := descriptor.Provider.Kind, theater.CapabilityProviderBuiltin; got != want {
				t.Fatalf("provider mismatch: got %q want %q", got, want)
			}
			switch test.family {
			case theater.CapabilityFamilyAction:
				if descriptor.Action == nil {
					t.Fatalf("expected action contract for %q", test.ref)
				}
			case theater.CapabilityFamilyInventory:
				if descriptor.Inventory == nil || !descriptor.Inventory.Produces.KindsSet().Contains(theater.ValueKindBytes) {
					t.Fatalf("inventory contract mismatch for %q: %#v", test.ref, descriptor.Inventory)
				}
			case theater.CapabilityFamilyTransform:
				if descriptor.Transform == nil || descriptor.Transform.Accepts.KindsSet() == nil {
					t.Fatalf("transform contract mismatch for %q: %#v", test.ref, descriptor.Transform)
				}
			case theater.CapabilityFamilyMatcher:
				if descriptor.Matcher == nil || descriptor.Matcher.Sugar.Form != theater.SugarFormUnary {
					t.Fatalf("matcher contract mismatch for %q: %#v", test.ref, descriptor.Matcher)
				}
			case theater.CapabilityFamilyGenerator:
				if descriptor.Generator == nil || descriptor.Generator.Produces.Kind != theater.ValueKindString {
					t.Fatalf("generator contract mismatch for %q: %#v", test.ref, descriptor.Generator)
				}
			case theater.CapabilityFamilyStateBackend:
				if descriptor.StateBackend == nil || descriptor.StateBackend.Descriptor.Guarantee != theater.StateGuaranteeLocalAtomic {
					t.Fatalf("state backend contract mismatch for %q: %#v", test.ref, descriptor.StateBackend)
				}
			}
		})
	}
}

func TestDescribePluginCapabilitiesIncludesPluginProvenance(t *testing.T) {
	t.Parallel()

	fixtures := preparePluginFixtures(t)
	configPath, lockPath := writePluginRegistryFiles(t, fixtures)

	bundle, err := builtin.NewBundle()
	if err != nil {
		t.Fatalf("builtins: %v", err)
	}

	plugins, err := theater.LoadPluginCatalog(bundle.Catalog, bundle.Matchers, configPath, lockPath)
	if err != nil {
		t.Fatalf("load plugin catalog: %v", err)
	}

	descriptors := theater.DescribePluginCapabilities(plugins)
	pluginDescriptor, ok := findCapability(descriptors, "action.smoke.echo")
	if !ok {
		t.Fatalf("missing plugin capability %q", "action.smoke.echo")
	}
	if got, want := pluginDescriptor.Provider.Kind, theater.CapabilityProviderPlugin; got != want {
		t.Fatalf("provider kind mismatch: got %q want %q", got, want)
	}
	if got, want := pluginDescriptor.Provider.PluginID, "smoke-plugin"; got != want {
		t.Fatalf("plugin id mismatch: got %q want %q", got, want)
	}
	if got, want := pluginDescriptor.Provider.PluginVersion, "0.2.0"; got != want {
		t.Fatalf("plugin version mismatch: got %q want %q", got, want)
	}
	if got, want := pluginDescriptor.Summary, "Emit a simple echo output"; got != want {
		t.Fatalf("summary mismatch: got %q want %q", got, want)
	}
	if pluginDescriptor.Action == nil {
		t.Fatalf("expected action descriptor for plugin capability: %#v", pluginDescriptor)
	}
	if output, ok := pluginDescriptor.Action.Outputs["echo"]; !ok || output.Kind != theater.ValueKindString {
		t.Fatalf("plugin action outputs mismatch: %#v", pluginDescriptor.Action.Outputs)
	}

	reportExporterDescriptor, ok := findCapability(descriptors, "report_exporter.smoke.write")
	if !ok {
		t.Fatalf("missing plugin capability %q", "report_exporter.smoke.write")
	}
	if reportExporterDescriptor.ReportExporter == nil || len(reportExporterDescriptor.ReportExporter.Params) != 1 {
		t.Fatalf("report exporter descriptor mismatch: %#v", reportExporterDescriptor.ReportExporter)
	}

	transformDescriptor, ok := findCapability(descriptors, "transform.smoke.wrap")
	if !ok {
		t.Fatalf("missing plugin capability %q", "transform.smoke.wrap")
	}
	if transformDescriptor.Transform == nil || transformDescriptor.Transform.Accepts.Kind != theater.ValueKindString {
		t.Fatalf("transform descriptor mismatch: %#v", transformDescriptor.Transform)
	}

	matcherDescriptor, ok := findCapability(descriptors, "matcher.smoke.equal")
	if !ok {
		t.Fatalf("missing plugin capability %q", "matcher.smoke.equal")
	}
	if matcherDescriptor.Matcher == nil || matcherDescriptor.Matcher.Actual.Kind != theater.ValueKindString {
		t.Fatalf("matcher descriptor mismatch: %#v", matcherDescriptor.Matcher)
	}

	stateBackendDescriptor, ok := findCapability(descriptors, "state_backend.smoke.file")
	if !ok {
		t.Fatalf("missing plugin capability %q", "state_backend.smoke.file")
	}
	if stateBackendDescriptor.StateBackend == nil || !stateBackendDescriptor.StateBackend.Descriptor.SupportsCAS {
		t.Fatalf("state backend descriptor mismatch: %#v", stateBackendDescriptor.StateBackend)
	}

	builtinDescriptor, ok := findCapability(descriptors, "action.http")
	if !ok {
		t.Fatalf("missing built-in capability %q", "action.http")
	}
	if got, want := builtinDescriptor.Provider.Kind, theater.CapabilityProviderBuiltin; got != want {
		t.Fatalf("built-in provider mismatch: got %q want %q", got, want)
	}
}

func findCapability(descriptors []theater.CapabilityDescriptor, ref string) (theater.CapabilityDescriptor, bool) {
	for _, descriptor := range descriptors {
		if descriptor.Ref == ref {
			return descriptor, true
		}
	}

	return theater.CapabilityDescriptor{}, false
}
