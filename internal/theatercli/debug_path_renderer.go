package theatercli

import (
	"fmt"
	"io"
	"strings"

	"github.com/alex-poliushkin/theater"
)

type debugPathRenderer struct {
	stdout io.Writer
	stderr io.Writer
}

type debugPathTextView struct {
	file  string
	paths []theater.DebugPath
}

func newDebugPathRenderer(stdout, stderr io.Writer) debugPathRenderer {
	return debugPathRenderer{stdout: stdout, stderr: stderr}
}

func (r debugPathRenderer) Render(format outputFormat, file string, paths []theater.DebugPath) int {
	switch format {
	case outputFormatJSON:
		return r.renderJSON(file, paths)
	case outputFormatText:
		return r.renderText(file, paths)
	default:
		fmt.Fprintf(r.stderr, "unsupported format %q\n", format)
		return exitCodeCommandError
	}
}

func (r debugPathRenderer) renderJSON(file string, paths []theater.DebugPath) int {
	response := struct {
		File  string              `json:"file"`
		Paths []theater.DebugPath `json:"paths"`
	}{
		File:  file,
		Paths: paths,
	}

	if err := writeJSON(r.stdout, response); err != nil {
		fmt.Fprintf(r.stderr, "encode json: %v\n", err)
		return exitCodeCommandError
	}

	return 0
}

func (r debugPathRenderer) renderText(file string, paths []theater.DebugPath) int {
	if _, err := io.WriteString(r.stdout, newDebugPathTextView(file, paths).String()); err != nil {
		fmt.Fprintf(r.stderr, "write text: %v\n", err)
		return exitCodeCommandError
	}

	return 0
}

func newDebugPathTextView(file string, paths []theater.DebugPath) debugPathTextView {
	return debugPathTextView{file: file, paths: paths}
}

func (v debugPathTextView) String() string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# %s: %d debuggable runtime path(s)\n", v.file, len(v.paths))
	fmt.Fprintln(&builder, "# use with --break or --break-file")
	for i := range v.paths {
		selector := formatDebugPathSelector(v.paths[i])
		if v.paths[i].RetryAware {
			fmt.Fprintln(&builder, "# retry-aware selector; attempt filters are valid")
		}

		fmt.Fprintln(&builder, selector)
	}

	return builder.String()
}

func formatDebugPathSelector(path theater.DebugPath) string {
	return fmt.Sprintf("kind=%s,phase=%s,path=%s", path.Kind, path.Phase, path.Path)
}
