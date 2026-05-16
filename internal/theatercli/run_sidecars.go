package theatercli

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alex-poliushkin/theater/junit"
	reportmodel "github.com/alex-poliushkin/theater/report"
)

type runSidecarOutputs struct {
	JSON      string
	JUnit     string
	Markdown  string
	Overwrite bool
}

type runSidecarOutput struct {
	format outputFormat
	path   string
}

func validateRunSidecarOutputs(outputs runSidecarOutputs) error {
	seen := make(map[string]outputFormat)
	for _, output := range outputs.requested() {
		if err := validateRunSidecarPath(output.path, outputs.Overwrite); err != nil {
			return err
		}
		cleanPath := filepath.Clean(output.path)
		if existingFormat, ok := seen[cleanPath]; ok {
			return fmt.Errorf(
				"sidecar output path %q is used for both %s and %s",
				output.path,
				existingFormat,
				output.format,
			)
		}
		seen[cleanPath] = output.format
	}
	return nil
}

func writeRunSidecarOutputs(file string, document reportmodel.RunDocument, outputs runSidecarOutputs) error {
	for _, output := range outputs.requested() {
		data, err := renderRunSidecar(file, document, output.format)
		if err != nil {
			return fmt.Errorf("%s %s: %w", output.format, output.path, err)
		}
		if err := writeRunSidecarFile(output.path, data, outputs.Overwrite); err != nil {
			return fmt.Errorf("%s %s: %w", output.format, output.path, err)
		}
	}
	return nil
}

func (o runSidecarOutputs) requested() []runSidecarOutput {
	outputs := make([]runSidecarOutput, 0, 3)
	if o.JSON != "" {
		outputs = append(outputs, runSidecarOutput{format: outputFormatJSON, path: o.JSON})
	}
	if o.JUnit != "" {
		outputs = append(outputs, runSidecarOutput{format: outputFormatJUnit, path: o.JUnit})
	}
	if o.Markdown != "" {
		outputs = append(outputs, runSidecarOutput{format: outputFormatMarkdown, path: o.Markdown})
	}
	return outputs
}

func validateRunSidecarPath(path string, overwrite bool) error {
	if path == "" {
		return errors.New("sidecar output path is required")
	}
	if path == "-" {
		return fmt.Errorf("sidecar output %q does not accept -; use an explicit file path", path)
	}
	if hasParentTraversal(path) {
		return fmt.Errorf("sidecar output path %q must not contain parent traversal", path)
	}

	cleanPath := filepath.Clean(path)
	parent := filepath.Dir(cleanPath)
	root, err := openRunSidecarParentRoot(parent)
	if err != nil {
		return err
	}
	defer func() {
		_ = root.Close()
	}()

	info, err := root.Lstat(filepath.Base(cleanPath))
	if err == nil {
		return validateExistingRunSidecarPath(path, info, overwrite)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("sidecar output path %q: %w", path, err)
	}
	return nil
}

func renderRunSidecar(file string, document reportmodel.RunDocument, format outputFormat) ([]byte, error) {
	var buffer bytes.Buffer
	switch format {
	case outputFormatJSON:
		if err := writeRunDocumentJSON(&buffer, file, document); err != nil {
			return nil, err
		}
	case outputFormatJUnit:
		if err := junit.NewExporter().Write(&buffer, document); err != nil {
			return nil, err
		}
	case outputFormatMarkdown:
		if err := newReportMarkdownRenderer().Write(&buffer, file, document); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
	return buffer.Bytes(), nil
}

func validateExistingRunSidecarPath(path string, info os.FileInfo, overwrite bool) error {
	if info.IsDir() {
		return fmt.Errorf("sidecar output path %q is a directory", path)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("sidecar output path %q is a symlink", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("sidecar output path %q is not a regular file", path)
	}
	if !overwrite {
		return fmt.Errorf("sidecar output path %q already exists; pass --overwrite to replace it", path)
	}
	return nil
}

func writeRunSidecarFile(path string, data []byte, overwrite bool) error {
	if path == "" {
		return errors.New("sidecar output path is required")
	}
	if path == "-" {
		return fmt.Errorf("sidecar output %q does not accept -; use an explicit file path", path)
	}
	if hasParentTraversal(path) {
		return fmt.Errorf("sidecar output path %q must not contain parent traversal", path)
	}

	cleanPath := filepath.Clean(path)
	parent := filepath.Dir(cleanPath)
	base := filepath.Base(cleanPath)
	root, err := openRunSidecarParentRoot(parent)
	if err != nil {
		return err
	}
	defer func() {
		_ = root.Close()
	}()

	if overwrite {
		if err := validateRunSidecarLeaf(root, path, base, true); err != nil {
			return err
		}
		return replaceRunSidecarFile(root, base, data)
	}

	if err := validateRunSidecarLeaf(root, path, base, false); err != nil {
		return err
	}
	file, err := root.OpenFile(base, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	return writeAndCloseRunSidecarFile(file, data)
}

func openRunSidecarParentRoot(parent string) (*os.Root, error) {
	if err := validateRunSidecarParentPath(parent); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		return nil, fmt.Errorf("sidecar output parent %q: %w", parent, err)
	}
	if err := validateOpenedRunSidecarParent(parent, root); err != nil {
		closeErr := root.Close()
		return nil, errors.Join(err, closeErr)
	}
	return root, nil
}

func validateRunSidecarParentPath(parent string) error {
	cleanParent := filepath.Clean(parent)
	for current := cleanParent; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("sidecar output parent %q: %w", cleanParent, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("sidecar output parent %q must not contain symlink directory %q", cleanParent, current)
		}
		if !info.IsDir() {
			return fmt.Errorf("sidecar output parent %q is not a directory", cleanParent)
		}

		next := filepath.Dir(current)
		if next == current || current == "." {
			return nil
		}
	}
}

func validateOpenedRunSidecarParent(parent string, root *os.Root) error {
	cleanParent := filepath.Clean(parent)
	pathInfo, err := os.Lstat(cleanParent)
	if err != nil {
		return fmt.Errorf("sidecar output parent %q: %w", cleanParent, err)
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("sidecar output parent %q must not contain symlink directory %q", cleanParent, cleanParent)
	}
	if !pathInfo.IsDir() {
		return fmt.Errorf("sidecar output parent %q is not a directory", cleanParent)
	}

	rootInfo, err := root.Stat(".")
	if err != nil {
		return fmt.Errorf("sidecar output parent %q: %w", cleanParent, err)
	}
	if !os.SameFile(pathInfo, rootInfo) {
		return fmt.Errorf("sidecar output parent %q changed during validation", cleanParent)
	}
	return nil
}

func validateRunSidecarLeaf(root *os.Root, path, base string, overwrite bool) error {
	info, err := root.Lstat(base)
	if err == nil {
		return validateExistingRunSidecarPath(path, info, overwrite)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("sidecar output path %q: %w", path, err)
	}
	return nil
}

func replaceRunSidecarFile(root *os.Root, base string, data []byte) error {
	file, tempName, err := createRunSidecarTempFile(root, base)
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = root.Remove(tempName)
		}
	}()

	if err := writeAndCloseRunSidecarFile(file, data); err != nil {
		return err
	}
	if err := root.Rename(tempName, base); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func createRunSidecarTempFile(root *os.Root, base string) (*os.File, string, error) {
	for range 100 {
		name, err := runSidecarTempName(base)
		if err != nil {
			return nil, "", err
		}
		file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return file, name, err
	}
	return nil, "", fmt.Errorf("create temporary sidecar for %q: too many name collisions", base)
}

func runSidecarTempName(base string) (string, error) {
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf(".%s.%x.tmp", base, suffix), nil
}

func writeAndCloseRunSidecarFile(file *os.File, data []byte) error {
	writeErr := writeAllRunSidecarData(file, data)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func writeAllRunSidecarData(file *os.File, data []byte) error {
	for len(data) > 0 {
		n, err := file.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func hasParentTraversal(path string) bool {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i := range parts {
		if parts[i] == ".." {
			return true
		}
	}
	return false
}
