package theater

import (
	"context"
	"errors"
	"fmt"
)

// ReportExportSpec identifies one report exporter invocation after a run.
type ReportExportSpec struct {
	Ref  string `json:"ref"`
	With Values `json:"with,omitempty"`
}

// ReportExporterDef describes a host-owned report export capability.
type ReportExporterDef struct {
	Params []ParamSpec
	Export func(ctx context.Context, config Values, document RunDocument) error
}

// ReportExporterRegistrar registers report exporters by stable ref.
type ReportExporterRegistrar interface {
	RegisterReportExporter(ref string, exporter ReportExporterDef) error
}

// ReportExporterResolver resolves report exporters by registered ref.
type ReportExporterResolver interface {
	ResolveReportExporter(ref string) (ReportExporterDef, error)
}

// ExportRunDocument invokes the requested exporters against one frozen run document.
func ExportRunDocument(
	ctx context.Context,
	resolver ReportExporterResolver,
	document RunDocument,
	specs []ReportExportSpec,
) error {
	if len(specs) == 0 {
		return nil
	}
	if dependencyMissing(resolver) {
		return errors.New("report exporter resolver is required")
	}

	for i := range specs {
		spec := specs[i]
		if spec.Ref == "" {
			return fmt.Errorf("report exporter %d ref is required", i)
		}

		def, err := resolver.ResolveReportExporter(spec.Ref)
		if err != nil {
			return err
		}
		if def.Export == nil {
			return fmt.Errorf("report exporter %q export is required", spec.Ref)
		}

		config := cloneValues(spec.With)
		if config == nil {
			config = Values{}
		}
		if err := def.Export(ctx, config, document); err != nil {
			return fmt.Errorf("report exporter %q: %w", spec.Ref, err)
		}
	}

	return nil
}
