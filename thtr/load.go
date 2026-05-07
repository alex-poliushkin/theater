package thtr

import (
	"io"

	"github.com/alex-poliushkin/theater"
	authoringthtr "github.com/alex-poliushkin/theater/internal/authoring/thtr"
)

// Decode reads `.thtr` source from reader and lowers it into a StageSpec.
func Decode(reader io.Reader, matchers theater.MatcherSugarResolver) (theater.StageSpec, error) {
	return authoringthtr.Decode(reader, matchers)
}

// LoadFile loads one `.thtr` file into a StageSpec.
func LoadFile(path string, matchers theater.MatcherSugarResolver) (theater.StageSpec, error) {
	return authoringthtr.LoadFile(path, matchers)
}

// LoadFlowFile loads a repo-aware `.thtr` flow file and linked library files
// into one StageSpec.
func LoadFlowFile(path string, matchers theater.MatcherSugarResolver) (theater.StageSpec, error) {
	return authoringthtr.LoadFlowFile(path, matchers)
}

// Parse lowers `.thtr` bytes into a StageSpec.
func Parse(data []byte, matchers theater.MatcherSugarResolver) (theater.StageSpec, error) {
	return authoringthtr.Parse(data, matchers)
}
