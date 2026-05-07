package thtrlsp

import (
	"fmt"
	"os"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/builtin"
)

const (
	envPluginsConfig = "THEATER_PLUGINS_CONFIG"
	envPluginsLock   = "THEATER_PLUGINS_LOCK"
)

type languageSupport struct {
	catalog      theater.CatalogResolver
	matchers     theater.MatcherResolver
	sugar        theater.MatcherSugarResolver
	capabilities []theater.CapabilityDescriptor
}

type descriptorOnlyCatalog struct {
	delegate theater.CatalogResolver
}

func newLanguageSupportFromEnvironment() (languageSupport, error) {
	return newLanguageSupport(os.Getenv(envPluginsConfig), os.Getenv(envPluginsLock))
}

func newLanguageSupport(pluginsConfig, pluginsLock string) (languageSupport, error) {
	if pluginsConfig == "" && pluginsLock != "" {
		return languageSupport{}, fmt.Errorf("thtr-lsp requires %s when %s is set", envPluginsConfig, envPluginsLock)
	}
	if pluginsConfig != "" && pluginsLock == "" {
		return languageSupport{}, fmt.Errorf("thtr-lsp requires %s when %s is set", envPluginsLock, envPluginsConfig)
	}

	bundle, err := builtin.NewBundle()
	if err != nil {
		return languageSupport{}, err
	}

	support := languageSupport{
		catalog:      bundle.Catalog,
		matchers:     bundle.Matchers,
		sugar:        bundle.Matchers,
		capabilities: theater.DescribeCapabilities(bundle.Catalog, bundle.Matchers),
	}
	if pluginsConfig == "" {
		return support, nil
	}

	pluginCatalog, err := theater.LoadPluginDescriptorCatalog(bundle.Catalog, bundle.Matchers, pluginsConfig, pluginsLock)
	if err != nil {
		return languageSupport{}, err
	}

	support.catalog = descriptorOnlyCatalog{delegate: pluginCatalog}
	support.matchers = pluginCatalog
	support.sugar = pluginCatalog
	support.capabilities = theater.DescribePluginCapabilities(pluginCatalog)
	return support, nil
}

func (c descriptorOnlyCatalog) ResolveAction(ref string) (theater.Action, error) {
	return c.delegate.ResolveAction(ref)
}

func (c descriptorOnlyCatalog) ResolveGenerator(ref string) (theater.GeneratorDef, error) {
	return c.delegate.ResolveGenerator(ref)
}

func (c descriptorOnlyCatalog) ResolveInventory(ref string) (theater.Inventory, error) {
	return c.delegate.ResolveInventory(ref)
}

func (c descriptorOnlyCatalog) ResolveStateBackend(ref string) (theater.StateBackendDef, error) {
	return c.delegate.ResolveStateBackend(ref)
}

func (c descriptorOnlyCatalog) ResolveReportExporter(ref string) (theater.ReportExporterDef, error) {
	return c.delegate.ResolveReportExporter(ref)
}

func (c descriptorOnlyCatalog) ResolveDecorator(ref string) (theater.DecoratorDef, error) {
	return c.delegate.ResolveDecorator(ref)
}
