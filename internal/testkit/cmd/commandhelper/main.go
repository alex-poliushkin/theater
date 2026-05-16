package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"time"
)

const spamChunkSize = 1024

func main() {
	if len(os.Args) < 2 {
		fail("command is required")
	}

	switch os.Args[1] {
	case "emit":
		runEmit(os.Args[2:])
	case "stdin-echo":
		runStdinEcho()
	case "env":
		runEnv(os.Args[2:])
	case "cwd":
		runCWD()
	case "spam":
		runSpam(os.Args[2:])
	case "spawn-marker":
		runSpawnMarker(os.Args[2:])
	case "write-marker":
		runWriteMarker(os.Args[2:])
	case "append-marker":
		runAppendMarker(os.Args[2:])
	case "remove-path":
		runRemovePath(os.Args[2:])
	case "replace-with-symlink":
		runReplaceWithSymlink(os.Args[2:])
	default:
		fail("unknown command: " + os.Args[1])
	}
}

func runEmit(args []string) {
	flags := flag.NewFlagSet("emit", flag.ExitOnError)
	stdout := flags.String("stdout", "", "")
	stderr := flags.String("stderr", "", "")
	exitCode := flags.Int("exit-code", 0, "")
	sleepBefore := flags.Int("sleep-before-ms", 0, "")
	sleepAfter := flags.Int("sleep-after-ms", 0, "")
	_ = flags.Parse(args)

	sleepMillis(*sleepBefore)
	writeAll(os.Stdout, *stdout)
	writeAll(os.Stderr, *stderr)
	sleepMillis(*sleepAfter)
	os.Exit(*exitCode)
}

func runStdinEcho() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		fail(err.Error())
	}

	writeAll(os.Stdout, string(data))
}

func runEnv(args []string) {
	if len(args) != 1 {
		fail("env requires exactly one variable name")
	}

	writeAll(os.Stdout, os.Getenv(args[0]))
}

func runCWD() {
	wd, err := os.Getwd()
	if err != nil {
		fail(err.Error())
	}

	writeAll(os.Stdout, wd)
}

func runSpam(args []string) {
	flags := flag.NewFlagSet("spam", flag.ExitOnError)
	stream := flags.String("stream", "stdout", "")
	size := flags.Int("bytes", 0, "")
	pattern := flags.String("pattern", "x", "")
	_ = flags.Parse(args)

	var writer io.Writer = os.Stdout
	if *stream == "stderr" {
		writer = os.Stderr
	}
	if *pattern == "" {
		fail("pattern must not be empty")
	}

	if err := writePatternedStream(writer, *pattern, *size); err != nil {
		fail(err.Error())
	}
}

func writePatternedStream(writer io.Writer, pattern string, size int) error {
	if size < 0 {
		return errors.New("bytes must be non-negative")
	}
	if size == 0 {
		return nil
	}
	if pattern == "" {
		return errors.New("pattern must not be empty")
	}

	// Keep memory bounded regardless of total emitted size by reusing one chunk.
	scratchSize := spamChunkSize
	if size < scratchSize {
		scratchSize = size
	}
	scratch := make([]byte, scratchSize)

	remaining := size
	offset := 0
	for remaining > 0 {
		partSize := len(scratch)
		if remaining < partSize {
			partSize = remaining
		}

		fillPatternedChunk(scratch[:partSize], pattern, offset)
		if _, err := writer.Write(scratch[:partSize]); err != nil {
			return err
		}

		offset += partSize
		remaining -= partSize
	}

	return nil
}

func fillPatternedChunk(chunk []byte, pattern string, offset int) {
	if len(chunk) == 0 {
		return
	}

	start := offset % len(pattern)
	written := copy(chunk, pattern[start:])
	for written < len(chunk) {
		written += copy(chunk[written:], pattern)
	}
}

func runSpawnMarker(args []string) {
	flags := flag.NewFlagSet("spawn-marker", flag.ExitOnError)
	readyPath := flags.String("ready", "", "")
	markerPath := flags.String("marker", "", "")
	childDelay := flags.Int("child-delay-ms", 0, "")
	parentSleep := flags.Int("parent-sleep-ms", 0, "")
	_ = flags.Parse(args)

	if *readyPath == "" {
		fail("ready path is required")
	}
	if *markerPath == "" {
		fail("marker path is required")
	}

	self, err := os.Executable()
	if err != nil {
		fail(err.Error())
	}

	child := exec.Command(
		self,
		"write-marker",
		"--path", *markerPath,
		"--delay-ms", strconv.Itoa(*childDelay),
	)
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	child.Env = os.Environ()
	if err := child.Start(); err != nil {
		fail(err.Error())
	}

	if err := os.WriteFile(*readyPath, []byte("ready"), 0o644); err != nil {
		fail(err.Error())
	}

	sleepMillis(*parentSleep)
	if err := child.Wait(); err != nil {
		fail(err.Error())
	}
}

func runWriteMarker(args []string) {
	flags := flag.NewFlagSet("write-marker", flag.ExitOnError)
	path := flags.String("path", "", "")
	delay := flags.Int("delay-ms", 0, "")
	_ = flags.Parse(args)

	if *path == "" {
		fail("path is required")
	}

	sleepMillis(*delay)
	if err := os.WriteFile(*path, []byte("marker"), 0o644); err != nil {
		fail(err.Error())
	}
}

func runAppendMarker(args []string) {
	flags := flag.NewFlagSet("append-marker", flag.ExitOnError)
	path := flags.String("path", "", "")
	_ = flags.Parse(args)

	if *path == "" {
		fail("path is required")
	}

	file, err := os.OpenFile(*path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fail(err.Error())
	}
	defer func() {
		if err := file.Close(); err != nil {
			fail(err.Error())
		}
	}()

	if _, err := file.WriteString("marker\n"); err != nil {
		fail(err.Error())
	}
}

func runRemovePath(args []string) {
	flags := flag.NewFlagSet("remove-path", flag.ExitOnError)
	path := flags.String("path", "", "")
	_ = flags.Parse(args)

	if *path == "" {
		fail("path is required")
	}

	if err := os.RemoveAll(*path); err != nil {
		fail(err.Error())
	}
}

func runReplaceWithSymlink(args []string) {
	flags := flag.NewFlagSet("replace-with-symlink", flag.ExitOnError)
	path := flags.String("path", "", "")
	target := flags.String("target", "", "")
	_ = flags.Parse(args)

	if *path == "" {
		fail("path is required")
	}
	if *target == "" {
		fail("target is required")
	}

	if err := os.RemoveAll(*path); err != nil {
		fail(err.Error())
	}
	if err := os.Symlink(*target, *path); err != nil {
		fail(err.Error())
	}
}

func sleepMillis(ms int) {
	if ms <= 0 {
		return
	}

	time.Sleep(time.Duration(ms) * time.Millisecond)
}

func writeAll(writer io.Writer, value string) {
	if value == "" {
		return
	}

	if _, err := io.WriteString(writer, value); err != nil {
		fail(err.Error())
	}
}

func fail(message string) {
	_, _ = fmt.Fprintln(os.Stderr, message)
	os.Exit(2)
}
