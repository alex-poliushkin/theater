package theatercli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func loadDebugBreakpointFiles(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}

	breakpoints := make([]string, 0, len(paths))
	for i := range paths {
		entries, err := loadDebugBreakpointFile(paths[i])
		if err != nil {
			return nil, err
		}

		breakpoints = append(breakpoints, entries...)
	}

	return breakpoints, nil
}

func loadDebugBreakpointFile(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read debug breakpoint file %q: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	specs := make([]string, 0, 8)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		specs = append(specs, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read debug breakpoint file %q: %w", path, err)
	}

	return specs, nil
}
