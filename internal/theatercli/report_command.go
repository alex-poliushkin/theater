package theatercli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/alex-poliushkin/theater/junit"
	reportmodel "github.com/alex-poliushkin/theater/report"
)

const commandReportRender = "render"

const reportRenderMaxInputBytes int64 = 10 * 1024 * 1024

type runJSONEnvelope struct {
	File   string                  `json:"file"`
	Result reportmodel.RunDocument `json:"result"`
}

func (a *application) runReportCommand(args []string) int {
	command, rest, ok := a.resolveRequiredSubcommand(commandReport, args)
	if !ok {
		return exitCodeCommandError
	}

	switch command.Name {
	case commandReportRender:
		return a.renderReport(rest)
	default:
		fmt.Fprintf(a.stderr, "unknown report subcommand %q\n", args[0])
		a.commands.PrintCommand(a.stderr, a.commands.Must(commandReport), nil, a.textStyler(a.stderr))
		return exitCodeCommandError
	}
}

func (a *application) renderReport(args []string) int {
	options, ok := a.parseReportRenderOptions(args)
	if !ok {
		return exitCodeCommandError
	}

	file, document, err := a.readReportRenderInput(options.input)
	if err != nil {
		fmt.Fprintf(a.stderr, "render report: %v\n", err)
		return exitCodeCommandError
	}

	if err := renderRunDocumentArtifact(a.stdout, file, document, options.format); err != nil {
		fmt.Fprintf(a.stderr, "render report: %v\n", err)
		return exitCodeCommandError
	}
	return 0
}

func (a *application) parseReportRenderOptions(args []string) (reportRenderOptions, bool) {
	flags, options, values := a.newReportRenderCommandFlagSet()
	if err := flags.Parse(args); err != nil {
		return reportRenderOptions{}, false
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(a.stderr, "report render does not accept positional arguments")
		return reportRenderOptions{}, false
	}
	if options.input == "" {
		fmt.Fprintln(a.stderr, "report render requires --input")
		return reportRenderOptions{}, false
	}

	parsedFormat, err := parseReportOutputFormat(values.format)
	if err != nil {
		fmt.Fprintf(a.stderr, "%v\n", err)
		return reportRenderOptions{}, false
	}
	options.format = parsedFormat
	return *options, true
}

func (a *application) readReportRenderInput(input string) (string, reportmodel.RunDocument, error) {
	var data []byte
	var err error
	if input == "-" {
		data, err = readBoundedRunJSON(a.stdin)
	} else {
		info, statErr := os.Stat(input)
		if statErr != nil {
			return "", reportmodel.RunDocument{}, statErr
		}
		if info.Size() > reportRenderMaxInputBytes {
			return "", reportmodel.RunDocument{}, fmt.Errorf("input exceeds %d bytes", reportRenderMaxInputBytes)
		}
		data, err = os.ReadFile(input)
	}
	if err != nil {
		return "", reportmodel.RunDocument{}, err
	}

	envelope, err := decodeRunJSONEnvelope(data)
	if err != nil {
		return "", reportmodel.RunDocument{}, err
	}
	if err := envelope.Result.Validate(); err != nil {
		return "", reportmodel.RunDocument{}, fmt.Errorf("run document is invalid: %w", err)
	}
	return envelope.File, envelope.Result, nil
}

func readBoundedRunJSON(reader io.Reader) ([]byte, error) {
	limited := io.LimitReader(reader, reportRenderMaxInputBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > reportRenderMaxInputBytes {
		return nil, fmt.Errorf("input exceeds %d bytes", reportRenderMaxInputBytes)
	}
	return data, nil
}

func decodeRunJSONEnvelope(data []byte) (runJSONEnvelope, error) {
	var envelope runJSONEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return runJSONEnvelope{}, err
	}
	if envelope.Result.SchemaVersion == "" {
		return runJSONEnvelope{}, errors.New("result.schema_version is required")
	}
	return envelope, nil
}

func renderRunDocumentArtifact(
	writer io.Writer,
	file string,
	document reportmodel.RunDocument,
	format outputFormat,
) error {
	switch format {
	case outputFormatJUnit:
		return junit.NewExporter().Write(writer, document)
	case outputFormatMarkdown:
		return newReportMarkdownRenderer().Write(writer, file, document)
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}
